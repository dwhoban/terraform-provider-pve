// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ datasource.DataSourceWithConfigure = &pveUserDataSource{}
	_ datasource.DataSource              = &pveUserDataSource{}
)

// NewPveUserDataSource returns the data source implementation.
func NewPveUserDataSource() datasource.DataSource {
	return &pveUserDataSource{}
}

// pveUserDataSource reads a single PVE user account via
// /access/users/{userid}.
type pveUserDataSource struct {
	client *pveclient.Client
}

// pveUserDataSourceModel is the Terraform-facing shape.
type pveUserDataSourceModel struct {
	UserID    types.String `tfsdk:"userid"`
	Comment   types.String `tfsdk:"comment"`
	Email     types.String `tfsdk:"email"`
	Firstname types.String `tfsdk:"firstname"`
	Lastname  types.String `tfsdk:"lastname"`
	Keys      types.String `tfsdk:"keys"`
	Enable    types.Bool   `tfsdk:"enable"`
	Expire    types.Int64  `tfsdk:"expire"`
	Groups    types.Set    `tfsdk:"groups"`
	ID        types.String `tfsdk:"id"`
}

// Metadata implements datasource.DataSource.
func (d *pveUserDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveUser
}

// Schema implements datasource.DataSource.
func (d *pveUserDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads a single PVE user account (`GET /access/users/{userid}`).",
		Attributes: map[string]schema.Attribute{
			"userid": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Full User ID in `name@realm` format to look up, e.g. `root@pam`.",
			},
			"comment": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Free-form comment, up to 2048 characters.",
			},
			"email": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Email address, up to 254 characters.",
			},
			"firstname": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "First name, up to 1024 characters.",
			},
			"lastname": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Last name, up to 1024 characters.",
			},
			"keys": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Keys for two factor auth (yubico).",
			},
			"enable": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether the account is enabled for login.",
			},
			"expire": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Account expiration date (seconds since epoch). `0` means no expiration date.",
			},
			"groups": schema.SetAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Set of PVE group IDs the user belongs to.",
			},
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Same as `userid`.",
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveUserDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*pveclient.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *pveclient.Client, got: %T.", req.ProviderData),
		)
		return
	}
	d.client = client
}

// Read implements datasource.DataSource.
func (d *pveUserDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveUserDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	user, err := d.client.GetAccessUser(ctx, data.UserID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_user data source",
			fmt.Sprintf("reading user %s: %s", data.UserID.ValueString(), err),
		)
		return
	}
	data.Comment = accessUserStringToTF(user.Comment)
	data.Email = accessUserStringToTF(user.Email)
	data.Firstname = accessUserStringToTF(user.Firstname)
	data.Lastname = accessUserStringToTF(user.Lastname)
	data.Keys = accessUserStringToTF(user.Keys)
	// Absent enable means PVE's default: the account is enabled.
	data.Enable = types.BoolValue(user.Enable == nil || *user.Enable)
	// Absent expire means PVE's default: no expiration date.
	data.Expire = types.Int64Value(0)
	if user.Expire != nil {
		data.Expire = types.Int64Value(*user.Expire)
	}
	groups, gerr := accessUserGroupsToTF(user.Groups)
	if gerr != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_user data source",
			fmt.Sprintf("building groups set for user %s: %s", data.UserID.ValueString(), gerr),
		)
		return
	}
	data.Groups = groups
	data.ID = data.UserID
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
