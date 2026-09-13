// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ datasource.DataSource              = &pveFirewallAliasDataSource{}
	_ datasource.DataSourceWithConfigure = &pveFirewallAliasDataSource{}
)

// NewPveFirewallAliasDataSource returns the data source implementation.
func NewPveFirewallAliasDataSource() datasource.DataSource {
	return &pveFirewallAliasDataSource{}
}

// pveFirewallAliasDataSource reads a single firewall alias
// (GET /cluster/firewall/aliases/{name}).
type pveFirewallAliasDataSource struct {
	client *pveclient.Client
}

// pveFirewallAliasDataSourceModel is the Terraform-facing shape.
type pveFirewallAliasDataSourceModel struct {
	Name    types.String `tfsdk:"name"`
	Cidr    types.String `tfsdk:"cidr"`
	Comment types.String `tfsdk:"comment"`
}

// Metadata implements datasource.DataSource.
func (d *pveFirewallAliasDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveFirewallAlias
}

// Schema implements datasource.DataSource.
func (d *pveFirewallAliasDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads a single IP or network alias from `GET /cluster/firewall/aliases/{name}`.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Alias name to look up.",
			},
			"cidr": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Network/IP specification in CIDR format the alias resolves to.",
			},
			"comment": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Free-form comment, or null when the alias has none.",
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveFirewallAliasDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = firewallEntityConfigureDataSource(req, resp)
}

// Read implements datasource.DataSource.
func (d *pveFirewallAliasDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveFirewallAliasDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Error reading pve_firewall_alias", "provider client is not configured")
		return
	}

	alias, err := d.client.GetFirewallAlias(ctx, data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_firewall_alias",
			fmt.Sprintf("reading firewall alias %s: %s", data.Name.ValueString(), err),
		)
		return
	}
	data.Cidr = types.StringValue(alias.Cidr)
	data.Comment = nodeNetworkStringToTF(alias.Comment)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
