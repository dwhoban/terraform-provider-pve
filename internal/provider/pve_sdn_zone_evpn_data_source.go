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
	_ datasource.DataSource              = &pveSdnZoneEvpnDataSource{}
	_ datasource.DataSourceWithConfigure = &pveSdnZoneEvpnDataSource{}
)

// NewPveSdnZoneEvpnDataSource returns the data source implementation.
func NewPveSdnZoneEvpnDataSource() datasource.DataSource {
	return &pveSdnZoneEvpnDataSource{}
}

// pveSdnZoneEvpnDataSource reads a single EVPN SDN zone (GET
// /cluster/sdn/zones/{zone}), erroring when the upstream type is not evpn.
type pveSdnZoneEvpnDataSource struct {
	client *pveclient.Client
}

// pveSdnZoneEvpnDataSourceModel is the Terraform-facing shape.
type pveSdnZoneEvpnDataSourceModel struct {
	sdnZoneCommonModel
	Controller              types.String `tfsdk:"controller"`
	SecondaryControllers    types.List   `tfsdk:"secondary_controllers"`
	AdvertiseSubnets        types.Bool   `tfsdk:"advertise_subnets"`
	DisableArpNdSuppression types.Bool   `tfsdk:"disable_arp_nd_suppression"`
	ExitNodes               types.String `tfsdk:"exitnodes"`
	ExitNodesLocalRouting   types.Bool   `tfsdk:"exitnodes_local_routing"`
	ExitNodesPrimary        types.String `tfsdk:"exitnodes_primary"`
	Mac                     types.String `tfsdk:"mac"`
	RtImport                types.String `tfsdk:"rt_import"`
	VrfVxlan                types.Int64  `tfsdk:"vrf_vxlan"`
}

// Metadata implements datasource.DataSource.
func (d *pveSdnZoneEvpnDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveSdnZoneEvpn
}

// Schema implements datasource.DataSource.
func (d *pveSdnZoneEvpnDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	attrs := sdnZoneDataSourceAttributes(sdnZoneCommonComputedAttributes())
	attrs["controller"] = schema.StringAttribute{
		Computed:            true,
		MarkdownDescription: "ID of the primary BGP/EVPN controller (PVE `controller`) for this zone.",
	}
	attrs["secondary_controllers"] = schema.ListAttribute{
		ElementType:         types.StringType,
		Computed:            true,
		MarkdownDescription: "Additional controller IDs (PVE `secondary-controllers`) peering with the primary controller.",
	}
	attrs["advertise_subnets"] = schema.BoolAttribute{
		Computed:            true,
		MarkdownDescription: "Whether IP prefixes (Type-5 routes) are advertised instead of MAC/IP pairs (Type-2 routes) (PVE `advertise-subnets`).",
	}
	attrs["disable_arp_nd_suppression"] = schema.BoolAttribute{
		Computed:            true,
		MarkdownDescription: "Whether IPv4 ARP and IPv6 Neighbour Discovery messages are suppressed (PVE `disable-arp-nd-suppression`).",
	}
	attrs["exitnodes"] = schema.StringAttribute{
		Computed:            true,
		MarkdownDescription: "Comma-separated list of cluster node names (PVE `pve-node-list`) acting as exit nodes.",
	}
	attrs["exitnodes_local_routing"] = schema.BoolAttribute{
		Computed:            true,
		MarkdownDescription: "Whether routes are created on the exit nodes so they can connect to EVPN guests directly (PVE `exitnodes-local-routing`).",
	}
	attrs["exitnodes_primary"] = schema.StringAttribute{
		Computed:            true,
		MarkdownDescription: "Cluster node name (PVE `pve-node`) traffic is forced through first.",
	}
	attrs["mac"] = schema.StringAttribute{
		Computed:            true,
		MarkdownDescription: "Anycast logical router MAC address (PVE `mac-addr`) for this zone.",
	}
	attrs["rt_import"] = schema.StringAttribute{
		Computed:            true,
		MarkdownDescription: "Comma-separated list of Route Targets (PVE `pve-sdn-bgp-rt-list`) imported into the VRF of this zone via BGP.",
	}
	attrs["vrf_vxlan"] = schema.Int64Attribute{
		Computed:            true,
		MarkdownDescription: "VNI for the zone VRF (PVE `vrf-vxlan`).",
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads an EVPN SDN zone from `GET /cluster/sdn/zones/{zone}`. Errors when the zone's upstream type is not `evpn`.",
		Attributes:          attrs,
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveSdnZoneEvpnDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = haConfigureDataSource(req, resp)
}

// Read implements datasource.DataSource.
func (d *pveSdnZoneEvpnDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveSdnZoneEvpnDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Error reading pve_sdn_zone_evpn", "provider client is not configured")
		return
	}
	z, err := sdnZoneGetChecked(ctx, d.client, data.Zone.ValueString(), "evpn")
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_sdn_zone_evpn",
			fmt.Sprintf("reading zone %s: %s", data.Zone.ValueString(), err),
		)
		return
	}
	sdnZoneCommonApply(z, &data.sdnZoneCommonModel)
	data.Controller = nodeNetworkStringToTF(z.Controller)
	data.SecondaryControllers = listStringToTF(z.SecondaryControllers)
	data.AdvertiseSubnets = nodeNetworkBoolPtrToTF(z.AdvertiseSubnets)
	data.DisableArpNdSuppression = nodeNetworkBoolPtrToTF(z.DisableArpNdSuppression)
	data.ExitNodes = nodeNetworkStringToTF(z.ExitNodes)
	data.ExitNodesLocalRouting = nodeNetworkBoolPtrToTF(z.ExitNodesLocalRouting)
	data.ExitNodesPrimary = nodeNetworkStringToTF(z.ExitNodesPrimary)
	data.Mac = nodeNetworkStringToTF(z.Mac)
	data.RtImport = nodeNetworkStringToTF(z.RtImport)
	data.VrfVxlan = haInt64PtrToTF(z.VrfVxlan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
