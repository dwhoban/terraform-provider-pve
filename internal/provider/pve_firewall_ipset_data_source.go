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
	_ datasource.DataSource              = &pveFirewallIpsetDataSource{}
	_ datasource.DataSourceWithConfigure = &pveFirewallIpsetDataSource{}
)

// NewPveFirewallIpsetDataSource returns the data source implementation.
func NewPveFirewallIpsetDataSource() datasource.DataSource {
	return &pveFirewallIpsetDataSource{}
}

// pveFirewallIpsetDataSource reads a single firewall IP set and its members
// (GET /cluster/firewall/ipset/{name}).
type pveFirewallIpsetDataSource struct {
	client *pveclient.Client
}

// pveFirewallIpsetDataSourceModel is the Terraform-facing shape.
type pveFirewallIpsetDataSourceModel struct {
	Name    types.String `tfsdk:"name"`
	Comment types.String `tfsdk:"comment"`
	Cidrs   types.Set    `tfsdk:"cidrs"`
}

// Metadata implements datasource.DataSource.
func (d *pveFirewallIpsetDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveFirewallIpset
}

// Schema implements datasource.DataSource.
func (d *pveFirewallIpsetDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads a single IP set and its members from `GET /cluster/firewall/ipset/{name}`.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "IP set name to look up.",
			},
			"comment": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Free-form comment, or null when the set has none.",
			},
			"cidrs": schema.SetAttribute{
				Computed:    true,
				ElementType: types.StringType,
				MarkdownDescription: "Set members, each a network in CIDR format, a single IP address, or an alias reference (`+aliasname`). " +
					"Members added out of band (per-member comments are not modeled) still appear here.",
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveFirewallIpsetDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = firewallEntityConfigureDataSource(req, resp)
}

// Read implements datasource.DataSource.
func (d *pveFirewallIpsetDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveFirewallIpsetDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Error reading pve_firewall_ipset", "provider client is not configured")
		return
	}

	name := data.Name.ValueString()
	members, err := d.client.ListFirewallIpsetMembers(ctx, name)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_firewall_ipset",
			fmt.Sprintf("reading firewall ipset %s: %s", name, err),
		)
		return
	}
	cidrs := make([]string, 0, len(members))
	for _, member := range members {
		cidrs = append(cidrs, member.Cidr)
	}
	data.Cidrs, err = firewallIpsetCidrsToTF(cidrs)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_firewall_ipset",
			fmt.Sprintf("building firewall ipset %s members: %s", name, err),
		)
		return
	}
	// The set's own comment lives only on the collection listing.
	sets, err := d.client.ListFirewallIpsets(ctx)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_firewall_ipset",
			fmt.Sprintf("listing firewall ipsets for %s: %s", name, err),
		)
		return
	}
	for _, set := range sets {
		if set.Name == name {
			data.Comment = nodeNetworkStringToTF(set.Comment)
			break
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
