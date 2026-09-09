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
	_ datasource.DataSource              = &pveSdnSubnetDataSource{}
	_ datasource.DataSourceWithConfigure = &pveSdnSubnetDataSource{}
)

// NewPveSdnSubnetDataSource returns the data source implementation.
func NewPveSdnSubnetDataSource() datasource.DataSource {
	return &pveSdnSubnetDataSource{}
}

// pveSdnSubnetDataSource reads a single SDN subnet object
// (GET /cluster/sdn/vnets/{vnet}/subnets/{subnet}).
type pveSdnSubnetDataSource struct {
	client *pveclient.Client
}

// pveSdnSubnetDataSourceModel is the Terraform-facing shape.
type pveSdnSubnetDataSourceModel struct {
	Vnet          types.String `tfsdk:"vnet"`
	Subnet        types.String `tfsdk:"subnet"`
	Gateway       types.String `tfsdk:"gateway"`
	Snat          types.Bool   `tfsdk:"snat"`
	DhcpDnsServer types.String `tfsdk:"dhcp_dns_server"`
	DhcpRange     types.List   `tfsdk:"dhcp_range"`
	Dnszoneprefix types.String `tfsdk:"dnszoneprefix"`
}

// Metadata implements datasource.DataSource.
func (d *pveSdnSubnetDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveSdnSubnet
}

// Schema implements datasource.DataSource.
func (d *pveSdnSubnetDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads a single SDN subnet object (`GET /cluster/sdn/vnets/{vnet}/subnets/{subnet}`).",
		Attributes: map[string]schema.Attribute{
			"vnet": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The vnet the subnet belongs to.",
			},
			"subnet": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The subnet identifier in CIDR form; PVE writes the mask separator as a hyphen (for example `10.0.0.0-24`).",
			},
			"gateway": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Subnet gateway, assigned on the vnet for layer3 zones.",
			},
			"snat": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether masquerade is enabled for this subnet when the pve-firewall is used.",
			},
			"dhcp_dns_server": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "IP address of the DHCP DNS server.",
			},
			"dhcp_range": schema.ListAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "DHCP ranges for this subnet, each written as `start-end`.",
			},
			"dnszoneprefix": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "DNS domain zone prefix for hostname registration.",
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveSdnSubnetDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = haConfigureDataSource(req, resp)
}

// Read implements datasource.DataSource.
func (d *pveSdnSubnetDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state pveSdnSubnetDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Error reading pve_sdn_subnet data source", "provider client is not configured")
		return
	}
	subnet, err := d.client.GetSdnSubnet(ctx, state.Vnet.ValueString(), state.Subnet.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_sdn_subnet data source",
			fmt.Sprintf("reading SDN subnet %s on vnet %s: %s", state.Subnet.ValueString(), state.Vnet.ValueString(), err),
		)
		return
	}
	state.Gateway = nodeNetworkStringToTF(subnet.Gateway)
	state.Snat = nodeNetworkBoolPtrToTF(subnet.Snat)
	state.DhcpDnsServer = nodeNetworkStringToTF(subnet.DhcpDnsServer)
	state.DhcpRange = listStringToTF(subnet.DhcpRange)
	state.Dnszoneprefix = nodeNetworkStringToTF(subnet.Dnszoneprefix)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
