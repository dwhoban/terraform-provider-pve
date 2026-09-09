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
	_ datasource.DataSource              = &pvePermissionsDataSource{}
	_ datasource.DataSourceWithConfigure = &pvePermissionsDataSource{}
)

// NewPvePermissionsDataSource returns the data source implementation.
func NewPvePermissionsDataSource() datasource.DataSource {
	return &pvePermissionsDataSource{}
}

// pvePermissionsDataSource exposes effective permissions
// (GET /access/permissions). The API returns a map of path to a map of
// privilege to propagate flag; that nesting is flattened into a sorted list
// of (path, privilege, propagate) rows for usable HCL iteration.
type pvePermissionsDataSource struct {
	client *pveclient.Client
}

// pvePermissionsDataSourceModel is the Terraform-facing shape.
type pvePermissionsDataSourceModel struct {
	ID      types.String                    `tfsdk:"id"`
	Path    types.String                    `tfsdk:"path"`
	Userid  types.String                    `tfsdk:"userid"`
	Entries []pvePermissionsDataSourceEntry `tfsdk:"entries"`
}

// pvePermissionsDataSourceEntry is one flattened (path, privilege) row.
type pvePermissionsDataSourceEntry struct {
	Path      types.String `tfsdk:"path"`
	Privilege types.String `tfsdk:"privilege"`
	Propagate types.Bool   `tfsdk:"propagate"`
}

// Metadata implements datasource.DataSource.
func (d *pvePermissionsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePvePermissions
}

// Schema implements datasource.DataSource.
func (d *pvePermissionsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Retrieves effective permissions (`GET /access/permissions`). The API's nested map (path → privilege → propagate flag) is flattened into `entries`, one row per path/privilege pair, sorted by path then privilege for stable output.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Static identifier for the permissions dump.",
			},
			"path": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Only dump this specific path, not the whole tree.",
			},
			"userid": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "User ID or full API token ID whose permissions to dump. Defaults to the credentials' own subject; dumping another user requires `Sys.Audit` on `/access`.",
			},
			"entries": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Flattened permission rows, sorted by path then privilege.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"path": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Access control path the privilege is effective on.",
						},
						"privilege": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Privilege (role) name, e.g. `Administrator`.",
						},
						"propagate": schema.BoolAttribute{
							Computed:            true,
							MarkdownDescription: "Whether the privilege propagates from this path to child paths.",
						},
					},
				},
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pvePermissionsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = aclConfigureDataSource(req, resp)
}

// Read implements datasource.DataSource.
func (d *pvePermissionsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pvePermissionsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	perms, err := d.client.GetPermissions(ctx, data.Path.ValueString(), data.Userid.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_permissions",
			fmt.Sprintf("retrieving effective permissions: %s", err),
		)
		return
	}
	data.Entries = aclPermissionsToEntries(perms)
	data.ID = types.StringValue("pve_permissions")
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// aclPermissionsToEntries flattens the API map into sorted entry rows.
func aclPermissionsToEntries(perms pveclient.AclPermissions) []pvePermissionsDataSourceEntry {
	paths := make([]string, 0, len(perms))
	for path := range perms {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	entries := make([]pvePermissionsDataSourceEntry, 0, len(paths))
	for _, path := range paths {
		privs := perms[path]
		names := make([]string, 0, len(privs))
		for name := range privs {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			entries = append(entries, pvePermissionsDataSourceEntry{
				Path:      types.StringValue(path),
				Privilege: types.StringValue(name),
				Propagate: types.BoolValue(privs[name]),
			})
		}
	}
	return entries
}
