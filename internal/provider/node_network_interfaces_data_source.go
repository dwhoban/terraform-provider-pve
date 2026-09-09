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
	_ datasource.DataSource              = &pveNodeNetworkInterfacesDataSource{}
	_ datasource.DataSourceWithConfigure = &pveNodeNetworkInterfacesDataSource{}
)

// NewPveNodeNetworkInterfacesDataSource returns the data source implementation.
func NewPveNodeNetworkInterfacesDataSource() datasource.DataSource {
	return &pveNodeNetworkInterfacesDataSource{}
}

// pveNodeNetworkInterfacesDataSource enumerates network interfaces on a
// single node.
type pveNodeNetworkInterfacesDataSource struct {
	client *pveclient.Client
}

// pveNodeNetworkInterfacesDataSourceModel is the Terraform-facing shape.
type pveNodeNetworkInterfacesDataSourceModel struct {
	ID         types.String                                   `tfsdk:"id"`
	Node       types.String                                   `tfsdk:"node"`
	TypeFilter types.String                                   `tfsdk:"type"`
	Interfaces []pveNodeNetworkInterfacesDataSourceIfaceModel `tfsdk:"interfaces"`
}

// pveNodeNetworkInterfacesDataSourceIfaceModel mirrors one row. We omit the
// `delete` and `digest` write-only fields from the resource.
type pveNodeNetworkInterfacesDataSourceIfaceModel struct {
	Node        types.String `tfsdk:"node"`
	Iface       types.String `tfsdk:"iface"`
	Type        types.String `tfsdk:"type"`
	Autostart   types.Bool   `tfsdk:"autostart"`
	Active      types.Bool   `tfsdk:"active"`
	CIDR        types.String `tfsdk:"cidr"`
	Address     types.String `tfsdk:"address"`
	Gateway     types.String `tfsdk:"gateway"`
	MTU         types.Int64  `tfsdk:"mtu"`
	Comments    types.String `tfsdk:"comments"`
	Method      types.String `tfsdk:"method"`
	BridgePorts types.String `tfsdk:"bridge_ports"`
	BridgeSTP   types.Bool   `tfsdk:"bridge_stp"`
	BridgeFD    types.Int64  `tfsdk:"bridge_fd"`
	VLANID      types.Int64  `tfsdk:"vlan_id"`
	VLANRawDev  types.String `tfsdk:"vlan_raw_device"`
	OVSBridge   types.String `tfsdk:"ovs_bridge"`
	OVSType     types.String `tfsdk:"ovs_type"`
	OVSOptions  types.Map    `tfsdk:"ovs_options"`
	BondMode    types.String `tfsdk:"bond_mode"`
	BondPrimary types.String `tfsdk:"bond_primary"`
	Slaves      types.List   `tfsdk:"slaves"`
}

// Metadata implements datasource.DataSource.
func (d *pveNodeNetworkInterfacesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveNodeNetworkInterfaces
}

// Schema implements datasource.DataSource.
func (d *pveNodeNetworkInterfacesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists every network interface defined on a Proxmox VE node (`GET /nodes/{node}/network`).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Static identifier equal to the node name.",
			},
			"node": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Name of the PVE node to query.",
			},
			"type": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Optional filter: return only interfaces whose `type` matches this value. PVE reports one of `eth`, `bond`, `bridge`, `vlan`, `ovsbond`, `ovsbridge`, `ovsintport`, `ovsport`, or `unknown`.",
			},
			"interfaces": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Interfaces matching the filter (or all interfaces when no filter is set).",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"node":         schema.StringAttribute{Computed: true},
						"iface":        schema.StringAttribute{Computed: true, MarkdownDescription: "Interface name."},
						"type":         schema.StringAttribute{Computed: true, MarkdownDescription: "Interface type. One of `eth`, `bond`, `bridge`, `vlan`, `ovsbond`, `ovsbridge`, `ovsintport`, `ovsport`, or `unknown`."},
						"autostart":    schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the interface is brought up on boot."},
						"active":       schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the interface is currently up."},
						"cidr":         schema.StringAttribute{Computed: true},
						"address":      schema.StringAttribute{Computed: true},
						"gateway":      schema.StringAttribute{Computed: true},
						"mtu":          schema.Int64Attribute{Computed: true},
						"comments":     schema.StringAttribute{Computed: true},
						"method":       schema.StringAttribute{Computed: true},
						"bridge_ports": schema.StringAttribute{Computed: true},
						"bridge_stp":   schema.BoolAttribute{Computed: true},
						"bridge_fd":    schema.Int64Attribute{Computed: true},
						"vlan_id":      schema.Int64Attribute{Computed: true},
						"ovs_bridge":   schema.StringAttribute{Computed: true},
						"ovs_type":     schema.StringAttribute{Computed: true},
						"ovs_options":  schema.MapAttribute{Computed: true, ElementType: types.StringType},
						"bond_mode":    schema.StringAttribute{Computed: true},
						"bond_primary": schema.StringAttribute{Computed: true},
						"slaves":       schema.ListAttribute{Computed: true, ElementType: types.StringType},
					},
				},
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveNodeNetworkInterfacesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
func (d *pveNodeNetworkInterfacesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveNodeNetworkInterfacesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	node := data.Node.ValueString()
	ifaces, err := d.client.ListNodeNetwork(ctx, node)
	if err != nil {
		resp.Diagnostics.AddError("Error reading pve_node_network_interfaces", fmt.Sprintf("listing interfaces on %s: %s", node, err))
		return
	}
	typeFilter := data.TypeFilter.ValueString()
	rows := make([]pveNodeNetworkInterfacesDataSourceIfaceModel, 0, len(ifaces))
	for _, iface := range ifaces {
		if typeFilter != "" && iface.Type != typeFilter {
			continue
		}
		var stp types.Bool
		if iface.BridgeSTP != nil {
			stp = types.BoolValue(*iface.BridgeSTP)
		}
		row := pveNodeNetworkInterfacesDataSourceIfaceModel{
			Node:        types.StringValue(node),
			Iface:       types.StringValue(iface.Iface),
			Type:        types.StringValue(iface.Type),
			Autostart:   nodeNetworkBoolPtrToTF(iface.Autostart),
			Active:      types.BoolValue(iface.Active),
			CIDR:        types.StringValue(iface.CIDR),
			Address:     types.StringValue(iface.Address),
			Gateway:     types.StringValue(iface.Gateway),
			MTU:         types.Int64Value(int64(iface.MTU)),
			Comments:    types.StringValue(iface.Comments),
			Method:      types.StringValue(iface.Method),
			BridgePorts: types.StringValue(iface.BridgePorts),
			BridgeSTP:   stp,
			BridgeFD:    types.Int64Value(int64(iface.BridgeFD)),
			VLANID:      types.Int64Value(int64(iface.VLANID)),
			VLANRawDev:  types.StringValue(iface.VLANRawDevice),
			OVSBridge:   types.StringValue(iface.OVSBridge),
			OVSType:     types.StringValue(iface.OVSType),
			BondPrimary: types.StringValue(iface.BondPrimary),
		}
		if len(iface.OVSOptions) > 0 {
			elems := make(map[string]attr.Value, len(iface.OVSOptions))
			for k, v := range iface.OVSOptions {
				elems[k] = types.StringValue(v)
			}
			m, _ := types.MapValue(types.StringType, elems)
			row.OVSOptions = m
		} else {
			row.OVSOptions = types.MapNull(types.StringType)
		}
		if len(iface.Slaves) > 0 {
			elems := make([]attr.Value, 0, len(iface.Slaves))
			for _, s := range iface.Slaves {
				elems = append(elems, types.StringValue(s))
			}
			l, _ := types.ListValue(types.StringType, elems)
			row.Slaves = l
		} else {
			row.Slaves = types.ListNull(types.StringType)
		}
		rows = append(rows, row)
	}
	data.ID = types.StringValue(node)
	data.Interfaces = rows
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
