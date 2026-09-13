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
	_ datasource.DataSource              = &pveNodeFirewallOptionsDataSource{}
	_ datasource.DataSourceWithConfigure = &pveNodeFirewallOptionsDataSource{}
)

// NewPveNodeFirewallOptionsDataSource returns the data source
// implementation.
func NewPveNodeFirewallOptionsDataSource() datasource.DataSource {
	return &pveNodeFirewallOptionsDataSource{}
}

// pveNodeFirewallOptionsDataSource reads the host firewall options
// singleton of one node (GET /nodes/{node}/firewall/options).
type pveNodeFirewallOptionsDataSource struct {
	client *pveclient.Client
}

// pveNodeFirewallOptionsDataSourceModel is the Terraform-facing shape.
type pveNodeFirewallOptionsDataSourceModel struct {
	Node types.String `tfsdk:"node"`
	pveNodeFirewallOptionsOptionSet
	ID types.String `tfsdk:"id"`
}

// nodeFirewallOptionsDataSourceAttributes renders the full computed
// attribute set.
func nodeFirewallOptionsDataSourceAttributes() map[string]datasourceschema.Attribute {
	attrs := make(map[string]datasourceschema.Attribute, len(nodeFirewallOptionsFieldSpecs)+2)
	for _, f := range nodeFirewallOptionsFieldSpecs {
		attrs[f.Name] = clusterOptionsDataSourceLeaf(f)
	}
	attrs["node"] = datasourceschema.StringAttribute{
		Required:            true,
		MarkdownDescription: "The cluster node name to look up.",
	}
	attrs["id"] = datasourceschema.StringAttribute{
		Computed:            true,
		MarkdownDescription: "Identifier of the host firewall options singleton; equals the `node` name.",
	}
	return attrs
}

// Metadata implements datasource.DataSource.
func (d *pveNodeFirewallOptionsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveNodeFirewallOptions
}

// Schema implements datasource.DataSource.
func (d *pveNodeFirewallOptionsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Reads the host firewall options singleton of one node from `GET /nodes/{node}/firewall/options`.",
		Attributes:          nodeFirewallOptionsDataSourceAttributes(),
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveNodeFirewallOptionsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
func (d *pveNodeFirewallOptionsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveNodeFirewallOptionsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Error reading pve_node_firewall_options", "provider client is not configured")
		return
	}
	if err := nodeFirewallOptionsReadInto(ctx, d.client, data.Node.ValueString(), &data.pveNodeFirewallOptionsOptionSet); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_node_firewall_options",
			fmt.Sprintf("reading firewall options of node %s: %s", data.Node.ValueString(), err),
		)
		return
	}
	data.ID = data.Node
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
