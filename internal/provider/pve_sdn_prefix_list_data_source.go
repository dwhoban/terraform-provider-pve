// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ datasource.DataSource              = &pveSdnPrefixListDataSource{}
	_ datasource.DataSourceWithConfigure = &pveSdnPrefixListDataSource{}
)

// NewPveSdnPrefixListDataSource returns the data source implementation.
func NewPveSdnPrefixListDataSource() datasource.DataSource {
	return &pveSdnPrefixListDataSource{}
}

// pveSdnPrefixListDataSource reads one SDN prefix list and its ordered
// entries (GET /cluster/sdn/prefix-lists/{id} and
// GET /cluster/sdn/prefix-lists/{id}/entries).
type pveSdnPrefixListDataSource struct {
	client *pveclient.Client
}

// pveSdnPrefixListDataSourceModel is the Terraform-facing shape.
type pveSdnPrefixListDataSourceModel struct {
	ID      types.String              `tfsdk:"id"`
	Entries []sdnPrefixListEntryModel `tfsdk:"entries"`
}

// Metadata implements datasource.DataSource.
func (d *pveSdnPrefixListDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveSdnPrefixList
}

// Schema implements datasource.DataSource.
func (d *pveSdnPrefixListDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Reads one SDN prefix list and its ordered entries from `GET /cluster/sdn/prefix-lists/{id}` and `GET /cluster/sdn/prefix-lists/{id}/entries`. Entries carry their upstream sequence in `seq`; PVE evaluates them sequentially and the first match wins.",
		Attributes: map[string]datasourceschema.Attribute{
			"id": datasourceschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The SDN prefix list identifier to look up.",
			},
			"entries": datasourceschema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "The prefix list entries in upstream order.",
				NestedObject: datasourceschema.NestedAttributeObject{
					Attributes: sdnPrefixListEntryDataSourceAttributes(),
				},
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveSdnPrefixListDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
func (d *pveSdnPrefixListDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveSdnPrefixListDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Error reading pve_sdn_prefix_list", "provider client is not configured")
		return
	}
	id := data.ID.ValueString()
	if _, err := d.client.GetSdnPrefixList(ctx, id); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_sdn_prefix_list",
			fmt.Sprintf("reading prefix list %s: %s", id, err),
		)
		return
	}
	entries, err := d.client.ListSdnPrefixListEntries(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_sdn_prefix_list",
			fmt.Sprintf("reading entries of prefix list %s: %s", id, err),
		)
		return
	}
	data.Entries = sdnPrefixListEntriesToModel(entries)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
