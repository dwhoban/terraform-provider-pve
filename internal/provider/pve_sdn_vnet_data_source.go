// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ datasource.DataSource              = &pveSdnVnetDataSource{}
	_ datasource.DataSourceWithConfigure = &pveSdnVnetDataSource{}
)

// NewPveSdnVnetDataSource returns the data source implementation.
func NewPveSdnVnetDataSource() datasource.DataSource {
	return &pveSdnVnetDataSource{}
}

// pveSdnVnetDataSource reads a single SDN vnet object
// (GET /cluster/sdn/vnets/{vnet}).
type pveSdnVnetDataSource struct {
	client *pveclient.Client
}

// pveSdnVnetMacVrfModel is one MAC VRF route entry.
type pveSdnVnetMacVrfModel struct {
	IP      types.String `tfsdk:"ip"`
	MAC     types.String `tfsdk:"mac"`
	Nexthop types.String `tfsdk:"nexthop"`
}

// pveSdnVnetDataSourceModel is the Terraform-facing shape.
type pveSdnVnetDataSourceModel struct {
	Vnet         types.String `tfsdk:"vnet"`
	Node         types.String `tfsdk:"node"`
	Zone         types.String `tfsdk:"zone"`
	Alias        types.String `tfsdk:"alias"`
	Tag          types.Int64  `tfsdk:"tag"`
	VlanAware    types.Bool   `tfsdk:"vlanaware"`
	IsolatePorts types.Bool   `tfsdk:"isolate_ports"`
	State        types.String `tfsdk:"state"`
	Digest       types.String `tfsdk:"digest"`
	MacVrf       types.List   `tfsdk:"mac_vrf"`
}

// Metadata implements datasource.DataSource.
func (d *pveSdnVnetDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveSdnVnet
}

// Schema implements datasource.DataSource.
func (d *pveSdnVnetDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads a single SDN vnet object (`GET /cluster/sdn/vnets/{vnet}`). Set `node` to additionally read the per-node MAC VRF routes (`GET /nodes/{node}/sdn/vnets/{vnet}/mac-vrf`, EVPN zones).",
		Attributes: map[string]schema.Attribute{
			"vnet": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The SDN vnet object identifier to read.",
			},
			"node": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Cluster node name. When set, the `mac_vrf` attribute carries the vnet's per-node MAC VRF routes.",
			},
			"zone": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Name of the zone this vnet belongs to.",
			},
			"alias": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Alias name of the vnet.",
			},
			"tag": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "VLAN tag (for VLAN or QinQ zones) or VXLAN VNI (for VXLAN or EVPN zones).",
			},
			"vlanaware": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether VLANs pass through this vnet.",
			},
			"isolate_ports": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether the isolated property is set for all interfaces on the bridge of this vnet.",
			},
			"state": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "State of the SDN configuration object; one of `new`, `changed`, or `deleted` when the configuration has not been applied yet.",
			},
			"digest": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Digest of the vnet section.",
			},
			"mac_vrf": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Routes from the MAC VRF that the chosen node self-originates or has learned via BGP (EVPN zones); empty unless `node` is set.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"ip": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "IP address of the MAC VRF entry.",
						},
						"mac": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "MAC address of the MAC VRF entry.",
						},
						"nexthop": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "IP address of the nexthop.",
						},
					},
				},
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveSdnVnetDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = haConfigureDataSource(req, resp)
}

// Read implements datasource.DataSource.
func (d *pveSdnVnetDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state pveSdnVnetDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Error reading pve_sdn_vnet data source", "provider client is not configured")
		return
	}
	vnet, err := d.client.GetSdnVnet(ctx, state.Vnet.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_sdn_vnet data source",
			fmt.Sprintf("reading SDN vnet %s: %s", state.Vnet.ValueString(), err),
		)
		return
	}
	state.Zone = nodeNetworkStringToTF(vnet.Zone)
	state.Alias = nodeNetworkStringToTF(vnet.Alias)
	state.Tag = haInt64PtrToTF(vnet.Tag)
	state.VlanAware = nodeNetworkBoolPtrToTF(vnet.VlanAware)
	state.IsolatePorts = nodeNetworkBoolPtrToTF(vnet.IsolatePorts)
	state.State = nodeNetworkStringToTF(vnet.State)
	state.Digest = nodeNetworkStringToTF(vnet.Digest)
	if state.Node.IsNull() {
		state.MacVrf = types.ListNull(types.ObjectType{AttrTypes: pveSdnVnetMacVrfAttrTypes()})
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
		return
	}
	entries, err := d.client.GetSdnVnetMacVrf(ctx, state.Node.ValueString(), state.Vnet.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_sdn_vnet data source",
			fmt.Sprintf("reading MAC VRF of SDN vnet %s on node %s: %s", state.Vnet.ValueString(), state.Node.ValueString(), err),
		)
		return
	}
	models := make([]pveSdnVnetMacVrfModel, 0, len(entries))
	for _, entry := range entries {
		models = append(models, pveSdnVnetMacVrfModel{
			IP:      types.StringValue(entry.IP),
			MAC:     types.StringValue(entry.MAC),
			Nexthop: types.StringValue(entry.Nexthop),
		})
	}
	list, diags := types.ListValueFrom(ctx, types.ObjectType{AttrTypes: pveSdnVnetMacVrfAttrTypes()}, models)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	state.MacVrf = list
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// pveSdnVnetMacVrfAttrTypes returns the attribute types of a mac_vrf entry.
func pveSdnVnetMacVrfAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"ip":      types.StringType,
		"mac":     types.StringType,
		"nexthop": types.StringType,
	}
}
