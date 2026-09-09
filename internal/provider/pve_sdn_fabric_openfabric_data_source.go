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
	_ datasource.DataSource              = &pveSdnFabricOpenfabricDataSource{}
	_ datasource.DataSourceWithConfigure = &pveSdnFabricOpenfabricDataSource{}
)

// NewPveSdnFabricOpenfabricDataSource returns the data source
// implementation.
func NewPveSdnFabricOpenfabricDataSource() datasource.DataSource {
	return &pveSdnFabricOpenfabricDataSource{}
}

// pveSdnFabricOpenfabricDataSource reads a single OpenFabric SDN fabric
// (GET /cluster/sdn/fabrics/fabric/{id}) with its node members, and
// exposes per-node runtime state behind the optional node argument.
type pveSdnFabricOpenfabricDataSource struct {
	client *pveclient.Client
}

// pveSdnFabricOpenfabricDataSourceModel is the Terraform-facing shape.
type pveSdnFabricOpenfabricDataSourceModel struct {
	FabricID      types.String                      `tfsdk:"fabric_id"`
	Node          types.String                      `tfsdk:"node"`
	IPPrefix      types.String                      `tfsdk:"ip_prefix"`
	IP6Prefix     types.String                      `tfsdk:"ip6_prefix"`
	CsnpInterval  types.Float64                     `tfsdk:"csnp_interval"`
	HelloInterval types.Float64                     `tfsdk:"hello_interval"`
	RouteFilter   types.String                      `tfsdk:"route_filter"`
	Nodes         []pveSdnFabricOpenfabricNodeModel `tfsdk:"nodes"`
	Digest        types.String                      `tfsdk:"digest"`
	Interfaces    []sdnFabricRuntimeInterfaceModel  `tfsdk:"interfaces"`
	Neighbors     []sdnFabricRuntimeNeighborModel   `tfsdk:"neighbors"`
	Routes        []sdnFabricRuntimeRouteModel      `tfsdk:"routes"`
}

// Metadata implements datasource.DataSource.
func (d *pveSdnFabricOpenfabricDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveSdnFabricOpenfabric
}

// Schema implements datasource.DataSource.
func (d *pveSdnFabricOpenfabricDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads a single OpenFabric SDN fabric (`GET /cluster/sdn/fabrics/fabric/{id}`) with its node members. When `node` is set, the live per-node fabric state (interfaces, neighbors, routes as reported by FRR) is read from `GET /nodes/{node}/sdn/fabrics/{fabric}/...`; the runtime attributes stay null otherwise. Runtime state only exists once `pve_sdn_apply` has pushed the configuration.",
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
			"csnp_interval": schema.Float64Attribute{
				Computed:            true,
				MarkdownDescription: "Complete Sequence Number PDU interval in seconds (1 - 600).",
			},
			"hello_interval": schema.Float64Attribute{
				Computed:            true,
				MarkdownDescription: "Hello packet interval in seconds (1 - 600).",
			},
			"route_filter": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Prefix list used to filter routes installed into the kernel routing table.",
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
							MarkdownDescription: "Network interfaces participating in OpenFabric on this node.",
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
									"hello_multiplier": schema.Int64Attribute{
										Computed:            true,
										MarkdownDescription: "OpenFabric hello multiplier of the interface (2 - 100).",
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
func (d *pveSdnFabricOpenfabricDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = haConfigureDataSource(req, resp)
}

// Read implements datasource.DataSource.
func (d *pveSdnFabricOpenfabricDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveSdnFabricOpenfabricDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Error reading pve_sdn_fabric_openfabric", "provider client is not configured")
		return
	}
	fabric, err := d.client.GetSdnFabric(ctx, data.FabricID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_sdn_fabric_openfabric",
			fmt.Sprintf("reading OpenFabric fabric %s: %s", data.FabricID.ValueString(), err),
		)
		return
	}
	data.IPPrefix = nodeNetworkStringToTF(fabric.IPPrefix)
	data.IP6Prefix = nodeNetworkStringToTF(fabric.IP6Prefix)
	data.CsnpInterval = nodeStoragesFloat64ToTF(fabric.CsnpInterval)
	data.HelloInterval = nodeStoragesFloat64ToTF(fabric.HelloInterval)
	data.RouteFilter = nodeNetworkStringToTF(fabric.RouteFilter)
	data.Digest = nodeNetworkStringToTF(fabric.Digest)
	nodes, err := d.client.ListSdnFabricNodes(ctx, data.FabricID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_sdn_fabric_openfabric",
			fmt.Sprintf("reading node members of OpenFabric fabric %s: %s", data.FabricID.ValueString(), err),
		)
		return
	}
	data.Nodes = sdnFabricOpenfabricNodesFromWire(nodes)
	if !data.Node.IsNull() && !data.Node.IsUnknown() {
		if err := sdnFabricLoadRuntime(ctx, d.client, data.Node.ValueString(), data.FabricID.ValueString(),
			&data.Interfaces, &data.Neighbors, &data.Routes); err != nil {
			resp.Diagnostics.AddError(
				"Error reading pve_sdn_fabric_openfabric",
				fmt.Sprintf("reading runtime state of OpenFabric fabric %s on node %s: %s",
					data.FabricID.ValueString(), data.Node.ValueString(), err),
			)
			return
		}
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
