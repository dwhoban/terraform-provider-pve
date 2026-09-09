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
	_ datasource.DataSource = &pveAptStandardRepositoryDataSource{}
)

// NewPveAptStandardRepositoryDataSource returns the data source
// implementation.
func NewPveAptStandardRepositoryDataSource() datasource.DataSource {
	return &pveAptStandardRepositoryDataSource{}
}

// pveAptStandardRepositoryDataSource reads one standard APT repository row
// (by handle) from the node's repository listing.
type pveAptStandardRepositoryDataSource struct {
	client *pveclient.Client
}

// pveAptStandardRepositoryDataSourceModel is the Terraform-facing shape.
type pveAptStandardRepositoryDataSourceModel struct {
	ID     types.String `tfsdk:"id"`
	Node   types.String `tfsdk:"node"`
	Handle types.String `tfsdk:"handle"`
	Name   types.String `tfsdk:"name"`
	Status types.Bool   `tfsdk:"status"`
}

// Metadata implements datasource.DataSource.
func (d *pveAptStandardRepositoryDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveAptStandardRepository
}

// Schema implements datasource.DataSource.
func (d *pveAptStandardRepositoryDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads one standard APT repository (identified by its PVE handle) from the node's repository listing (`GET /nodes/{node}/apt/repositories`). `status` is null when the repository is known to PVE but not configured on the node.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Identifier of the data source (`<node>:<handle>`).",
			},
			"node": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The cluster node whose repository configuration to read.",
			},
			"handle": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Handle that identifies the standard repository (e.g. `enterprise`, `no-subscription`, `ceph-quincy`).",
			},
			"name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Full name of the repository as reported by PVE.",
			},
			"status": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether the configured repository is enabled. Null when the repository is not configured.",
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveAptStandardRepositoryDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = nodeSvcAptConfigureDataSource(req, resp)
}

// Read implements datasource.DataSource.
func (d *pveAptStandardRepositoryDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveAptStandardRepositoryDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Error reading pve_apt_standard_repository", "provider client is not configured")
		return
	}
	node, handle := data.Node.ValueString(), data.Handle.ValueString()
	repos, err := d.client.GetNodeAptRepositories(ctx, node)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_apt_standard_repository",
			fmt.Sprintf("reading APT repositories on node %s: %s", node, err),
		)
		return
	}
	for _, repo := range repos.StandardRepositories {
		if repo.Handle != handle {
			continue
		}
		data.ID = types.StringValue(fmt.Sprintf("%s:%s", node, handle))
		data.Name = types.StringValue(repo.Name)
		data.Status = nodeNetworkBoolPtrToTF(repo.Status)
		resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
		return
	}
	resp.Diagnostics.AddError(
		"Error reading pve_apt_standard_repository",
		fmt.Sprintf("standard repository %s is not known on node %s", handle, node),
	)
}
