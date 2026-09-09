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
	_ datasource.DataSource              = &pveAclDataSource{}
	_ datasource.DataSourceWithConfigure = &pveAclDataSource{}
)

// NewPveAclDataSource returns the data source implementation.
func NewPveAclDataSource() datasource.DataSource {
	return &pveAclDataSource{}
}

// pveAclDataSource exposes the full access control list (GET /access/acl).
// Deliberately filter-free: the server already restricts the list to entries
// the caller may modify.
type pveAclDataSource struct {
	client *pveclient.Client
}

// pveAclDataSourceModel is the Terraform-facing shape.
type pveAclDataSourceModel struct {
	ID      types.String            `tfsdk:"id"`
	Entries []pveAclDataSourceEntry `tfsdk:"entries"`
}

// pveAclDataSourceEntry mirrors one ACL row.
type pveAclDataSourceEntry struct {
	Path      types.String `tfsdk:"path"`
	Role      types.String `tfsdk:"role"`
	Type      types.String `tfsdk:"type"`
	Ugid      types.String `tfsdk:"ugid"`
	Propagate types.Bool   `tfsdk:"propagate"`
}

// Metadata implements datasource.DataSource.
func (d *pveAclDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveAcl
}

// Schema implements datasource.DataSource.
func (d *pveAclDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Retrieves the full access control list (`GET /access/acl`). The server restricts the result to entries the caller has permission to modify.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Static identifier for the ACL list.",
			},
			"entries": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Every visible ACL entry.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"path": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Access control path the entry applies to.",
						},
						"role": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Role name granted on `path`.",
						},
						"type": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Kind of subject. One of: `user`, `group`, `token`.",
						},
						"ugid": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "User or group ID (for tokens, the full token ID such as `user@realm!tokenname`).",
						},
						"propagate": schema.BoolAttribute{
							Computed:            true,
							MarkdownDescription: "Whether the permission propagates to child paths. Defaults to `true` when the server omits the flag.",
						},
					},
				},
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveAclDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = aclConfigureDataSource(req, resp)
}

// Read implements datasource.DataSource.
func (d *pveAclDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveAclDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	entries, err := d.client.GetAcl(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading pve_acl", fmt.Sprintf("listing ACLs: %s", err))
		return
	}
	rows := make([]pveAclDataSourceEntry, 0, len(entries))
	for _, entry := range entries {
		rows = append(rows, pveAclDataSourceEntry{
			Path:      types.StringValue(entry.Path),
			Role:      types.StringValue(entry.RoleID),
			Type:      types.StringValue(entry.Type),
			Ugid:      types.StringValue(entry.Ugid),
			Propagate: types.BoolValue(entry.Propagate == nil || *entry.Propagate),
		})
	}
	data.ID = types.StringValue("pve_acl")
	data.Entries = rows
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// aclConfigureDataSource extracts the configured *pveclient.Client from a
// datasource ConfigureRequest. Nil provider data leaves the data source
// unconfigured (unit tests).
func aclConfigureDataSource(req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) *pveclient.Client {
	if req.ProviderData == nil {
		return nil
	}
	client, ok := req.ProviderData.(*pveclient.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *pveclient.Client, got: %T.", req.ProviderData),
		)
		return nil
	}
	return client
}
