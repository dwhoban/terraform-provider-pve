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
	_ datasource.DataSource              = &pveSdnFabricOspfDataSource{}
	_ datasource.DataSourceWithConfigure = &pveSdnFabricOspfDataSource{}
)

// NewPveSdnFabricOspfDataSource returns the data source implementation.
func NewPveSdnFabricOspfDataSource() datasource.DataSource {
	return &pveSdnFabricOspfDataSource{}
}

// pveSdnFabricOspfDataSource reads a single OSPF SDN fabric
// (GET /cluster/sdn/fabrics/fabric/{id}) with its node members, and
// exposes per-node runtime state behind the optional node argument.
type pveSdnFabricOspfDataSource struct {
	client *pveclient.Client
}

// pveSdnFabricOspfDataSourceModel is the Terraform-facing shape.
type pveSdnFabricOspfDataSourceModel struct {
	FabricID     types.String                        `tfsdk:"fabric_id"`
	Node         types.String                        `tfsdk:"node"`
	IPPrefix     types.String                        `tfsdk:"ip_prefix"`
	IP6Prefix    types.String                        `tfsdk:"ip6_prefix"`
	Area         types.String                        `tfsdk:"area"`
	RouteFilter  types.String                        `tfsdk:"route_filter"`
	Redistribute []pveSdnFabricOspfRedistributeModel `tfsdk:"redistribute"`
	Nodes        []pveSdnFabricOspfNodeModel         `tfsdk:"nodes"`
	Digest       types.String                        `tfsdk:"digest"`
	Interfaces   []sdnFabricRuntimeInterfaceModel    `tfsdk:"interfaces"`
	Neighbors    []sdnFabricRuntimeNeighborModel     `tfsdk:"neighbors"`
	Routes       []sdnFabricRuntimeRouteModel        `tfsdk:"routes"`
}

// Metadata implements datasource.DataSource.
func (d *pveSdnFabricOspfDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveSdnFabricOspf
}

// Schema implements datasource.DataSource.
func (d *pveSdnFabricOspfDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads a single OSPF SDN fabric (`GET /cluster/sdn/fabrics/fabric/{id}`) with its node members. When `node` is set, the live per-node fabric state (interfaces, neighbors, routes as reported by FRR) is read from `GET /nodes/{node}/sdn/fabrics/{fabric}/...`; the runtime attributes stay null otherwise. Runtime state only exists once `pve_sdn_apply` has pushed the configuration.",
		Attributes: map[string]schema.Attribute{
			"fabric_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The fabric identifier to look up.",
			},
			"node": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Cluster node whose live fabric state (interfaces, neighbors, routes) should be read. When unset those attributes remain null.",
			},
			"ip_prefix": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "IPv4 prefix (CIDR) node addresses are drawn from.",
			},
			"ip6_prefix": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "IPv6 prefix (CIDR) node addresses are drawn from.",
			},
			"area": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "OSPF area, either an IPv4 address or a 32-bit number.",
			},
			"route_filter": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Prefix list used to filter routes installed into the kernel routing table.",
			},
			"redistribute": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Route sources redistributed into OSPF, each with an optional route map filter.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"source": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The protocol routes are redistributed from: `bgp`, `connected`, `kernel`, or `static`.",
						},
						"route_map": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Route map filtering or transforming routes redistributed from this source.",
						},
					},
				},
			},
			"nodes": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Per-node member entries of the fabric.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"node_id": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The cluster node name.",
						},
						"ip": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "IPv4 address of this node inside the fabric.",
						},
						"ip6": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "IPv6 address of this node inside the fabric.",
						},
						"interfaces": schema.ListNestedAttribute{
							Computed:            true,
							MarkdownDescription: "Network interfaces participating in OSPF on this node.",
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"name": schema.StringAttribute{
										Computed:            true,
										MarkdownDescription: "Name of the network interface.",
									},
									"ip": schema.StringAttribute{
										Computed:            true,
										MarkdownDescription: "IPv4 address (CIDRv4) for this interface.",
									},
									"ip6": schema.StringAttribute{
										Computed:            true,
										MarkdownDescription: "IPv6 address (CIDRv6) for this interface.",
									},
									"network_type": schema.StringAttribute{
										Computed:            true,
										MarkdownDescription: "OSPF network type of the interface: `broadcast`, `non-broadcast`, `point-to-multipoint`, or `point-to-point`.",
									},
								},
							},
						},
					},
				},
			},
			"digest": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Configuration digest of the fabric.",
			},
			"interfaces": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Live fabric interfaces on `node` (from `GET /nodes/{node}/sdn/fabrics/{fabric}/interfaces`); null when `node` is unset.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: sdnFabricRuntimeInterfaceAttrs(),
				},
			},
			"neighbors": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Live fabric neighbors on `node` (from `GET /nodes/{node}/sdn/fabrics/{fabric}/neighbors`); null when `node` is unset.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: sdnFabricRuntimeNeighborAttrs(),
				},
			},
			"routes": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Live fabric routes on `node` (from `GET /nodes/{node}/sdn/fabrics/{fabric}/routes`); null when `node` is unset.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: sdnFabricRuntimeRouteAttrs(),
				},
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveSdnFabricOspfDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = haConfigureDataSource(req, resp)
}

// Read implements datasource.DataSource.
func (d *pveSdnFabricOspfDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveSdnFabricOspfDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Error reading pve_sdn_fabric_ospf", "provider client is not configured")
		return
	}
	fabric, err := d.client.GetSdnFabric(ctx, data.FabricID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_sdn_fabric_ospf",
			fmt.Sprintf("reading OSPF fabric %s: %s", data.FabricID.ValueString(), err),
		)
		return
	}
	data.IPPrefix = nodeNetworkStringToTF(fabric.IPPrefix)
	data.IP6Prefix = nodeNetworkStringToTF(fabric.IP6Prefix)
	data.Area = nodeNetworkStringToTF(fabric.Area)
	data.RouteFilter = nodeNetworkStringToTF(fabric.RouteFilter)
	data.Digest = nodeNetworkStringToTF(fabric.Digest)
	data.Redistribute = sdnFabricOspfRedistributeFromWire(fabric.Redistribute)
	nodes, err := d.client.ListSdnFabricNodes(ctx, data.FabricID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_sdn_fabric_ospf",
			fmt.Sprintf("reading node members of OSPF fabric %s: %s", data.FabricID.ValueString(), err),
		)
		return
	}
	data.Nodes = sdnFabricOspfNodesFromWire(nodes)
	if !data.Node.IsNull() && !data.Node.IsUnknown() {
		if err := sdnFabricLoadRuntime(ctx, d.client, data.Node.ValueString(), data.FabricID.ValueString(),
			&data.Interfaces, &data.Neighbors, &data.Routes); err != nil {
			resp.Diagnostics.AddError(
				"Error reading pve_sdn_fabric_ospf",
				fmt.Sprintf("reading runtime state of OSPF fabric %s on node %s: %s",
					data.FabricID.ValueString(), data.Node.ValueString(), err),
			)
			return
		}
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// sdnFabricRuntimeInterfaceModel is one live fabric interface row.
type sdnFabricRuntimeInterfaceModel struct {
	Name  types.String `tfsdk:"name"`
	State types.String `tfsdk:"state"`
	Type  types.String `tfsdk:"type"`
}

// sdnFabricRuntimeNeighborModel is one live fabric neighbor row.
type sdnFabricRuntimeNeighborModel struct {
	Neighbor types.String `tfsdk:"neighbor"`
	Status   types.String `tfsdk:"status"`
	Uptime   types.String `tfsdk:"uptime"`
}

// sdnFabricRuntimeRouteModel is one live fabric route row.
type sdnFabricRuntimeRouteModel struct {
	Route types.String `tfsdk:"route"`
	Via   types.List   `tfsdk:"via"`
}

// sdnFabricRuntimeInterfaceAttrs returns the nested attribute schema shared
// by both fabric data sources' runtime interface lists.
func sdnFabricRuntimeInterfaceAttrs() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"name": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Name of the network interface.",
		},
		"state": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Current state of the interface as reported by FRR.",
		},
		"type": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Interface type inside the fabric (e.g. `Point-to-Point`, `Broadcast`).",
		},
	}
}

// sdnFabricRuntimeNeighborAttrs returns the nested attribute schema shared
// by both fabric data sources' runtime neighbor lists.
func sdnFabricRuntimeNeighborAttrs() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"neighbor": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "IP or hostname of the neighbor.",
		},
		"status": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Adjacency status of the neighbor as reported by FRR (e.g. `Full`).",
		},
		"uptime": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Uptime of the adjacency as reported by FRR (e.g. `8h24m12s`).",
		},
	}
}

// sdnFabricRuntimeRouteAttrs returns the nested attribute schema shared by
// both fabric data sources' runtime route lists.
func sdnFabricRuntimeRouteAttrs() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"route": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "CIDR block of the routing table entry.",
		},
		"via": schema.ListAttribute{
			Computed:            true,
			ElementType:         types.StringType,
			MarkdownDescription: "Next-hop addresses for the route.",
		},
	}
}

// sdnFabricLoadRuntime reads the three per-node fabric status endpoints and
// projects them into the Terraform models.
func sdnFabricLoadRuntime(ctx context.Context, client *pveclient.Client, node, fabricID string,
	ifaces *[]sdnFabricRuntimeInterfaceModel, neighbors *[]sdnFabricRuntimeNeighborModel, routes *[]sdnFabricRuntimeRouteModel) error {
	wireIfaces, err := client.ListSdnFabricRuntimeInterfaces(ctx, node, fabricID)
	if err != nil {
		return err
	}
	*ifaces = make([]sdnFabricRuntimeInterfaceModel, 0, len(wireIfaces))
	for _, i := range wireIfaces {
		*ifaces = append(*ifaces, sdnFabricRuntimeInterfaceModel{
			Name:  types.StringValue(i.Name),
			State: types.StringValue(i.State),
			Type:  types.StringValue(i.Type),
		})
	}
	wireNeighbors, err := client.ListSdnFabricRuntimeNeighbors(ctx, node, fabricID)
	if err != nil {
		return err
	}
	*neighbors = make([]sdnFabricRuntimeNeighborModel, 0, len(wireNeighbors))
	for _, n := range wireNeighbors {
		*neighbors = append(*neighbors, sdnFabricRuntimeNeighborModel{
			Neighbor: types.StringValue(n.Neighbor),
			Status:   types.StringValue(n.Status),
			Uptime:   types.StringValue(n.Uptime),
		})
	}
	wireRoutes, err := client.ListSdnFabricRuntimeRoutes(ctx, node, fabricID)
	if err != nil {
		return err
	}
	*routes = make([]sdnFabricRuntimeRouteModel, 0, len(wireRoutes))
	for _, r := range wireRoutes {
		*routes = append(*routes, sdnFabricRuntimeRouteModel{
			Route: types.StringValue(r.Route),
			Via:   listStringToTF(r.Via),
		})
	}
	return nil
}
