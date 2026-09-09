// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ datasource.DataSource              = &pveSdnRouteMapDataSource{}
	_ datasource.DataSourceWithConfigure = &pveSdnRouteMapDataSource{}
)

// NewPveSdnRouteMapDataSource returns the data source implementation.
func NewPveSdnRouteMapDataSource() datasource.DataSource {
	return &pveSdnRouteMapDataSource{}
}

// pveSdnRouteMapDataSource reads one SDN route map and its ordered
// entries (GET /cluster/sdn/route-maps/entries/{route-map-id}).
type pveSdnRouteMapDataSource struct {
	client *pveclient.Client
}

// pveSdnRouteMapDataSourceModel is the Terraform-facing shape.
type pveSdnRouteMapDataSourceModel struct {
	ID         types.String            `tfsdk:"id"`
	RouteMapID types.String            `tfsdk:"route_map_id"`
	Entries    []sdnRouteMapEntryModel `tfsdk:"entries"`
}

// Metadata implements datasource.DataSource.
func (d *pveSdnRouteMapDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveSdnRouteMap
}

// Schema implements datasource.DataSource.
func (d *pveSdnRouteMapDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Reads one SDN route map and its ordered entries from `GET /cluster/sdn/route-maps/entries/{route-map-id}`. Entries carry their upstream index in `order`; evaluation walks them in index order and the first match wins.",
		Attributes: map[string]datasourceschema.Attribute{
			"id": datasourceschema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Identifier of the route map; equals the `route_map_id`.",
			},
			"route_map_id": datasourceschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The SDN route map identifier to look up.",
			},
			"entries": datasourceschema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "The route map entries in upstream order.",
				NestedObject: datasourceschema.NestedAttributeObject{
					Attributes: sdnRouteMapEntryDataSourceAttributes(),
				},
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveSdnRouteMapDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
func (d *pveSdnRouteMapDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveSdnRouteMapDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Error reading pve_sdn_route_map", "provider client is not configured")
		return
	}
	id := data.RouteMapID.ValueString()
	entries, err := d.client.ListSdnRouteMapEntries(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_sdn_route_map",
			fmt.Sprintf("reading entries of route map %s: %s", id, err),
		)
		return
	}
	data.ID = data.RouteMapID
	data.Entries = sdnRouteMapEntriesToModel(entries)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
