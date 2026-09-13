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
	_ datasource.DataSource              = &pveSdnFirewallOptionsDataSource{}
	_ datasource.DataSourceWithConfigure = &pveSdnFirewallOptionsDataSource{}
)

// NewPveSdnFirewallOptionsDataSource returns the data source
// implementation.
func NewPveSdnFirewallOptionsDataSource() datasource.DataSource {
	return &pveSdnFirewallOptionsDataSource{}
}

// pveSdnFirewallOptionsDataSource reads the firewall options singleton of
// one SDN vnet (GET /cluster/sdn/vnets/{vnet}/firewall/options).
type pveSdnFirewallOptionsDataSource struct {
	client *pveclient.Client
}

// pveSdnFirewallOptionsDataSourceModel is the Terraform-facing shape.
type pveSdnFirewallOptionsDataSourceModel struct {
	VNet types.String `tfsdk:"vnet"`
	pveSdnFirewallOptionsOptionSet
	ID types.String `tfsdk:"id"`
}

// sdnFirewallOptionsDataSourceAttributes renders the full computed
// attribute set.
func sdnFirewallOptionsDataSourceAttributes() map[string]datasourceschema.Attribute {
	attrs := make(map[string]datasourceschema.Attribute, len(sdnFirewallOptionsFieldSpecs)+2)
	for _, f := range sdnFirewallOptionsFieldSpecs {
		attrs[f.Name] = clusterOptionsDataSourceLeaf(f)
	}
	attrs["vnet"] = datasourceschema.StringAttribute{
		Required:            true,
		MarkdownDescription: "The SDN vnet object identifier to look up.",
	}
	attrs["id"] = datasourceschema.StringAttribute{
		Computed:            true,
		MarkdownDescription: "Identifier of the vnet firewall options singleton; equals the `vnet` name.",
	}
	return attrs
}

// Metadata implements datasource.DataSource.
func (d *pveSdnFirewallOptionsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveSdnFirewallOptions
}

// Schema implements datasource.DataSource.
func (d *pveSdnFirewallOptionsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Reads the firewall options singleton of one SDN vnet from `GET /cluster/sdn/vnets/{vnet}/firewall/options`.",
		Attributes:          sdnFirewallOptionsDataSourceAttributes(),
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveSdnFirewallOptionsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
func (d *pveSdnFirewallOptionsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveSdnFirewallOptionsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Error reading pve_sdn_firewall_options", "provider client is not configured")
		return
	}
	if err := sdnFirewallOptionsReadInto(ctx, d.client, data.VNet.ValueString(), &data.pveSdnFirewallOptionsOptionSet); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_sdn_firewall_options",
			fmt.Sprintf("reading firewall options of vnet %s: %s", data.VNet.ValueString(), err),
		)
		return
	}
	data.ID = data.VNet
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
