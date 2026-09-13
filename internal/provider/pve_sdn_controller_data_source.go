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
	_ datasource.DataSource              = &pveSdnControllerDataSource{}
	_ datasource.DataSourceWithConfigure = &pveSdnControllerDataSource{}
)

// NewPveSdnControllerDataSource returns the data source implementation.
func NewPveSdnControllerDataSource() datasource.DataSource {
	return &pveSdnControllerDataSource{}
}

// pveSdnControllerDataSource reads a single SDN controller object
// (GET /cluster/sdn/controllers/{controller}).
type pveSdnControllerDataSource struct {
	client *pveclient.Client
}

// pveSdnControllerDataSourceModel is the Terraform-facing shape.
type pveSdnControllerDataSourceModel struct {
	Controller              types.String `tfsdk:"controller"`
	Type                    types.String `tfsdk:"type"`
	Asn                     types.Int64  `tfsdk:"asn"`
	BgpMode                 types.String `tfsdk:"bgp_mode"`
	BgpMultipathAsPathRelax types.Bool   `tfsdk:"bgp_multipath_as_path_relax"`
	Ebgp                    types.Bool   `tfsdk:"ebgp"`
	EbgpMultihop            types.Int64  `tfsdk:"ebgp_multihop"`
	Fabric                  types.String `tfsdk:"fabric"`
	IsisDomain              types.String `tfsdk:"isis_domain"`
	IsisIfaces              types.List   `tfsdk:"isis_ifaces"`
	IsisNet                 types.String `tfsdk:"isis_net"`
	Loopback                types.String `tfsdk:"loopback"`
	Node                    types.String `tfsdk:"node"`
	Nodes                   types.List   `tfsdk:"nodes"`
	PeerGroupName           types.String `tfsdk:"peer_group_name"`
	Peers                   types.List   `tfsdk:"peers"`
	RouteMapIn              types.String `tfsdk:"route_map_in"`
	RouteMapOut             types.String `tfsdk:"route_map_out"`
	Digest                  types.String `tfsdk:"digest"`
	State                   types.String `tfsdk:"state"`
}

// Metadata implements datasource.DataSource.
func (d *pveSdnControllerDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveSdnController
}

// Schema implements datasource.DataSource.
func (d *pveSdnControllerDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads a single SDN controller object (`GET /cluster/sdn/controllers/{controller}`).",
		Attributes: map[string]schema.Attribute{
			"controller": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The SDN controller object identifier to read.",
			},
			"type": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Controller plugin type; one of `bgp`, `evpn`, `faucet`, `isis`.",
			},
			"asn": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Autonomous system number (BGP and EVPN only).",
			},
			"bgp_mode": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Whether eBGP or iBGP is used; one of `auto`, `external`, `internal` (BGP only).",
			},
			"bgp_multipath_as_path_relax": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether different AS paths of equal length are considered for multipath computation (BGP only).",
			},
			"ebgp": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether eBGP (remote-as external) is enabled (BGP only).",
			},
			"ebgp_multihop": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Maximum amount of hops for eBGP peers (BGP only).",
			},
			"fabric": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "SDN fabric used as underlay for this EVPN controller.",
			},
			"isis_domain": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Name of the IS-IS domain (IS-IS only).",
			},
			"isis_ifaces": schema.ListAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Interfaces where IS-IS is active (IS-IS only).",
			},
			"isis_net": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Network Entity title for this node in the IS-IS network (IS-IS only).",
			},
			"loopback": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Loopback/dummy interface providing the Router-IP (BGP only).",
			},
			"node": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Node where this controller is active.",
			},
			"nodes": schema.ListAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Cluster node names where this controller is active.",
			},
			"peer_group_name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Name of the peer group for this EVPN controller.",
			},
			"peers": schema.ListAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Peer IP addresses.",
			},
			"route_map_in": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Route map applied to incoming routes.",
			},
			"route_map_out": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Route map applied to outgoing routes.",
			},
			"digest": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Digest of the controller section.",
			},
			"state": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "State of the SDN configuration object; one of `new`, `changed`, or `deleted` when the configuration has not been applied yet.",
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveSdnControllerDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = haConfigureDataSource(req, resp)
}

// Read implements datasource.DataSource.
func (d *pveSdnControllerDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state pveSdnControllerDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Error reading pve_sdn_controller data source", "provider client is not configured")
		return
	}
	controller, err := d.client.GetSdnController(ctx, state.Controller.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_sdn_controller data source",
			fmt.Sprintf("reading SDN controller %s: %s", state.Controller.ValueString(), err),
		)
		return
	}
	state.Type = nodeNetworkStringToTF(controller.Type)
	state.Asn = haInt64PtrToTF(controller.ASN)
	state.BgpMode = nodeNetworkStringToTF(controller.BgpMode)
	state.BgpMultipathAsPathRelax = nodeNetworkBoolPtrToTF(controller.BgpMultipathAsPathRelax)
	state.Ebgp = nodeNetworkBoolPtrToTF(controller.Ebgp)
	state.EbgpMultihop = haInt64PtrToTF(controller.EbgpMultihop)
	state.Fabric = nodeNetworkStringToTF(controller.Fabric)
	state.IsisDomain = nodeNetworkStringToTF(controller.IsisDomain)
	state.IsisIfaces = listStringToTF(controller.IsisIfaces)
	state.IsisNet = nodeNetworkStringToTF(controller.IsisNet)
	state.Loopback = nodeNetworkStringToTF(controller.Loopback)
	state.Node = nodeNetworkStringToTF(controller.Node)
	state.Nodes = listStringToTF(controller.Nodes)
	state.PeerGroupName = nodeNetworkStringToTF(controller.PeerGroupName)
	state.Peers = listStringToTF(controller.Peers)
	state.RouteMapIn = nodeNetworkStringToTF(controller.RouteMapIn)
	state.RouteMapOut = nodeNetworkStringToTF(controller.RouteMapOut)
	state.Digest = nodeNetworkStringToTF(controller.Digest)
	state.State = nodeNetworkStringToTF(controller.State)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
