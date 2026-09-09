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
	_ datasource.DataSource              = &pveClusterFirewallOptionsDataSource{}
	_ datasource.DataSourceWithConfigure = &pveClusterFirewallOptionsDataSource{}
)

// NewPveClusterFirewallOptionsDataSource returns the data source
// implementation.
func NewPveClusterFirewallOptionsDataSource() datasource.DataSource {
	return &pveClusterFirewallOptionsDataSource{}
}

// pveClusterFirewallOptionsDataSource reads the cluster-wide firewall
// options singleton (GET /cluster/firewall/options).
type pveClusterFirewallOptionsDataSource struct {
	client *pveclient.Client
}

// pveClusterFirewallOptionsDataSourceModel is the Terraform-facing shape.
type pveClusterFirewallOptionsDataSourceModel struct {
	pveClusterFirewallOptionsOptionSet
	ID types.String `tfsdk:"id"`
}

// clusterFirewallOptionsDataSourceAttributes renders the full computed
// attribute set.
func clusterFirewallOptionsDataSourceAttributes() map[string]datasourceschema.Attribute {
	attrs := make(map[string]datasourceschema.Attribute, len(clusterFirewallOptionsFieldSpecs)+1)
	for _, f := range clusterFirewallOptionsFieldSpecs {
		attrs[f.Name] = clusterOptionsDataSourceLeaf(f)
	}
	attrs["id"] = datasourceschema.StringAttribute{
		Computed:            true,
		MarkdownDescription: "Singleton identifier for the cluster firewall options; always `cluster`.",
	}
	return attrs
}

// Metadata implements datasource.DataSource.
func (d *pveClusterFirewallOptionsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveClusterFirewallOptions
}

// Schema implements datasource.DataSource.
func (d *pveClusterFirewallOptionsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Reads the cluster-wide firewall options singleton from `GET /cluster/firewall/options`.",
		Attributes:          clusterFirewallOptionsDataSourceAttributes(),
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveClusterFirewallOptionsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
func (d *pveClusterFirewallOptionsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveClusterFirewallOptionsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Error reading pve_cluster_firewall_options", "provider client is not configured")
		return
	}
	if err := clusterFirewallOptionsReadInto(ctx, d.client, &data.pveClusterFirewallOptionsOptionSet); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_cluster_firewall_options",
			fmt.Sprintf("reading cluster firewall options: %s", err),
		)
		return
	}
	data.ID = types.StringValue(pveClusterFirewallOptionsID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
