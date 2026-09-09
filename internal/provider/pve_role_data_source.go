// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ datasource.DataSource              = &pveRoleDataSource{}
	_ datasource.DataSourceWithConfigure = &pveRoleDataSource{}
)

// NewPveRoleDataSource returns the data source implementation.
func NewPveRoleDataSource() datasource.DataSource {
	return &pveRoleDataSource{}
}

// pveRoleDataSource reads a single PVE role — custom or built-in — via
// /access/roles/{roleid}.
type pveRoleDataSource struct {
	client *pveclient.Client
}

// pveRoleDataSourceModel is the Terraform-facing shape.
type pveRoleDataSourceModel struct {
	ID     types.String `tfsdk:"id"`
	RoleID types.String `tfsdk:"roleid"`
	Privs  types.Set    `tfsdk:"privs"`
}

// Metadata implements datasource.DataSource.
func (d *pveRoleDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveRole
}

// Schema implements datasource.DataSource.
func (d *pveRoleDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads a single role in Proxmox VE (`GET /access/roles/{roleid}`). Works for built-in roles (e.g. `PVEVMAdmin`) as well as roles managed by `pve_role` (resource); a nonexistent identifier fails with an error naming it.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Identifier of the role (same value as `roleid`).",
			},
			"roleid": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Role identifier (PVE `pve-roleid` format), built-in or custom.",
			},
			"privs": schema.SetAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "PVE privileges granted by this role (the privilege names the role index reports as granted).",
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveRoleDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
func (d *pveRoleDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveRoleDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	grants, err := d.client.GetAccessRole(ctx, data.RoleID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_role data source",
			fmt.Sprintf("reading role %s: %s", data.RoleID.ValueString(), err),
		)
		return
	}
	granted := make([]string, 0, len(grants))
	for priv, allowed := range grants {
		if allowed {
			granted = append(granted, priv)
		}
	}
	sort.Strings(granted)
	data.ID = types.StringValue(data.RoleID.ValueString())
	privs, err := accessRolePrivsToTF(granted)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_role data source",
			fmt.Sprintf("converting privs of role %s: %s", data.RoleID.ValueString(), err),
		)
		return
	}
	data.Privs = privs
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
