// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	float64validator "github.com/hashicorp/terraform-plugin-framework-validators/float64validator"
	int64validator "github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"

	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ resource.Resource                = &pveSdnFabricOpenfabricResource{}
	_ resource.ResourceWithConfigure   = &pveSdnFabricOpenfabricResource{}
	_ resource.ResourceWithImportState = &pveSdnFabricOpenfabricResource{}
)

// NewPveSdnFabricOpenfabricResource returns the resource implementation.
func NewPveSdnFabricOpenfabricResource() resource.Resource {
	return &pveSdnFabricOpenfabricResource{}
}

// pveSdnFabricOpenfabricResource manages an OpenFabric SDN fabric
// (/cluster/sdn/fabrics/fabric) with its node members
// (/cluster/sdn/fabrics/node/{fabric_id}). Mutations are synchronous and
// stay pending until `pve_sdn_apply` runs.
type pveSdnFabricOpenfabricResource struct {
	client *pveclient.Client
}

// pveSdnFabricOpenfabricResourceModel is the Terraform-facing shape.
type pveSdnFabricOpenfabricResourceModel struct {
	FabricID      types.String                      `tfsdk:"fabric_id"`
	IPPrefix      types.String                      `tfsdk:"ip_prefix"`
	IP6Prefix     types.String                      `tfsdk:"ip6_prefix"`
	CsnpInterval  types.Float64                     `tfsdk:"csnp_interval"`
	HelloInterval types.Float64                     `tfsdk:"hello_interval"`
	RouteFilter   types.String                      `tfsdk:"route_filter"`
	Nodes         []pveSdnFabricOpenfabricNodeModel `tfsdk:"nodes"`
	Digest        types.String                      `tfsdk:"digest"`
}

// pveSdnFabricOpenfabricNodeModel is one node member of the OpenFabric
// fabric.
type pveSdnFabricOpenfabricNodeModel struct {
	NodeID     types.String                       `tfsdk:"node_id"`
	IP         types.String                       `tfsdk:"ip"`
	IP6        types.String                       `tfsdk:"ip6"`
	Interfaces []pveSdnFabricOpenfabricIfaceModel `tfsdk:"interfaces"`
}

// pveSdnFabricOpenfabricIfaceModel is one OpenFabric interface of a node
// member.
type pveSdnFabricOpenfabricIfaceModel struct {
	Name            types.String `tfsdk:"name"`
	IP              types.String `tfsdk:"ip"`
	IP6             types.String `tfsdk:"ip6"`
	HelloMultiplier types.Int64  `tfsdk:"hello_multiplier"`
}

// Metadata implements resource.Resource.
func (r *pveSdnFabricOpenfabricResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveSdnFabricOpenfabric
}

// Schema implements resource.Resource.
func (r *pveSdnFabricOpenfabricResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages an OpenFabric SDN fabric (`/cluster/sdn/fabrics/fabric`, section protocol `openfabric`) with its per-node member entries (`/cluster/sdn/fabrics/node/{fabric_id}`). Changes stay pending and take effect only after the `pve_sdn_apply` action runs.",
		Attributes: map[string]schema.Attribute{
			"fabric_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The fabric identifier (PVE `pve-sdn-fabric-id`: 2 - 8 characters of letters, digits, or hyphens, starting and ending alphanumeric). Changing this value forces recreation.",
				Validators:          sdnFabricIDValidators,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"ip_prefix": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "IPv4 prefix (CIDR) node addresses are drawn from, e.g. `10.0.0.0/24`.",
			},
			"ip6_prefix": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "IPv6 prefix (CIDR) node addresses are drawn from, e.g. `fd00::/64`.",
			},
			"csnp_interval": schema.Float64Attribute{
				Optional:            true,
				MarkdownDescription: "Complete Sequence Number PDU interval in seconds. Must be between 1 and 600.",
				Validators: []validator.Float64{
					float64validator.Between(1, 600),
				},
			},
			"hello_interval": schema.Float64Attribute{
				Optional:            true,
				MarkdownDescription: "Hello packet interval in seconds. Must be between 1 and 600.",
				Validators: []validator.Float64{
					float64validator.Between(1, 600),
				},
			},
			"route_filter": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Prefix list (see `pve_sdn_prefix_list`) used to filter routes installed into the kernel routing table.",
			},
			"nodes": schema.ListNestedAttribute{
				Optional:            true,
				MarkdownDescription: "Per-node member entries of the fabric. Each entry joins the named cluster node into the OpenFabric fabric.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"node_id": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "The cluster node name.",
						},
						"ip": schema.StringAttribute{
							Optional:            true,
							MarkdownDescription: "IPv4 address of this node inside the fabric.",
						},
						"ip6": schema.StringAttribute{
							Optional:            true,
							MarkdownDescription: "IPv6 address of this node inside the fabric.",
						},
						"interfaces": schema.ListNestedAttribute{
							Optional:            true,
							MarkdownDescription: "Network interfaces participating in OpenFabric on this node.",
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"name": schema.StringAttribute{
										Required:            true,
										MarkdownDescription: "Name of the network interface (e.g. `ens19`).",
									},
									"ip": schema.StringAttribute{
										Optional:            true,
										MarkdownDescription: "IPv4 address (CIDRv4) for this interface, e.g. `10.0.0.1/24`.",
									},
									"ip6": schema.StringAttribute{
										Optional:            true,
										MarkdownDescription: "IPv6 address (CIDRv6) for this interface.",
									},
									"hello_multiplier": schema.Int64Attribute{
										Optional:            true,
										MarkdownDescription: "OpenFabric hello multiplier of the interface. Must be between 2 and 100.",
										Validators: []validator.Int64{
											int64validator.Between(2, 100),
										},
									},
								},
							},
						},
					},
				},
			},
			"digest": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Configuration digest of the fabric; changes whenever the fabric configuration changes.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveSdnFabricOpenfabricResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = haConfigureResource(req, resp)
}

// Create implements resource.Resource.
func (r *pveSdnFabricOpenfabricResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveSdnFabricOpenfabricResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error creating pve_sdn_fabric_openfabric", "provider client is not configured")
		return
	}
	if err := r.client.CreateSdnFabric(ctx, sdnFabricOpenfabricFromModel(plan)); err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_sdn_fabric_openfabric",
			fmt.Sprintf("creating OpenFabric fabric %s: %s", plan.FabricID.ValueString(), err),
		)
		return
	}
	for _, node := range plan.Nodes {
		if err := r.client.CreateSdnFabricNode(ctx, sdnFabricOpenfabricNodeFromModel(node, plan.FabricID.ValueString())); err != nil {
			resp.Diagnostics.AddError(
				"Error creating pve_sdn_fabric_openfabric",
				fmt.Sprintf("adding node %s to OpenFabric fabric %s: %s", node.NodeID.ValueString(), plan.FabricID.ValueString(), err),
			)
			return
		}
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_sdn_fabric_openfabric after create",
			fmt.Sprintf("reading OpenFabric fabric %s: %s", plan.FabricID.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read implements resource.Resource.
func (r *pveSdnFabricOpenfabricResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveSdnFabricOpenfabricResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.readInto(ctx, &state); err != nil {
		if isPVEClientNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error reading pve_sdn_fabric_openfabric",
			fmt.Sprintf("reading OpenFabric fabric %s: %s", state.FabricID.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource.
func (r *pveSdnFabricOpenfabricResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveSdnFabricOpenfabricResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state pveSdnFabricOpenfabricResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	fabricID := plan.FabricID.ValueString()
	upd := sdnFabricOpenfabricUpdateFromModel(plan)
	upd.Delete = sdnFabricOpenfabricDeleteFields(plan, state)
	if err := r.client.UpdateSdnFabric(ctx, fabricID, upd); err != nil {
		resp.Diagnostics.AddError(
			"Error updating pve_sdn_fabric_openfabric",
			fmt.Sprintf("updating OpenFabric fabric %s: %s", fabricID, err),
		)
		return
	}
	if err := sdnFabricOpenfabricDiffNodes(ctx, r.client, plan, state); err != nil {
		resp.Diagnostics.AddError(
			"Error updating pve_sdn_fabric_openfabric",
			fmt.Sprintf("updating node members of OpenFabric fabric %s: %s", fabricID, err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_sdn_fabric_openfabric after update",
			fmt.Sprintf("reading OpenFabric fabric %s: %s", fabricID, err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete implements resource.Resource.
func (r *pveSdnFabricOpenfabricResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveSdnFabricOpenfabricResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	fabricID := state.FabricID.ValueString()
	for _, node := range state.Nodes {
		if err := r.client.DeleteSdnFabricNode(ctx, fabricID, node.NodeID.ValueString()); err != nil && !isPVEClientNotFound(err) {
			resp.Diagnostics.AddError(
				"Error deleting pve_sdn_fabric_openfabric",
				fmt.Sprintf("deleting node %s of OpenFabric fabric %s: %s", node.NodeID.ValueString(), fabricID, err),
			)
			return
		}
	}
	if err := r.client.DeleteSdnFabric(ctx, fabricID); err != nil && !isPVEClientNotFound(err) {
		resp.Diagnostics.AddError(
			"Error deleting pve_sdn_fabric_openfabric",
			fmt.Sprintf("deleting OpenFabric fabric %s: %s", fabricID, err),
		)
	}
}

// ImportState parses an import ID of the form `<fabric_id>`.
func (r *pveSdnFabricOpenfabricResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		resp.Diagnostics.AddError("Invalid pve_sdn_fabric_openfabric import ID", "import ID must be the fabric identifier, e.g. `openfabric1`")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("fabric_id"), req.ID)...)
}

// readInto refreshes the model from PVE. Empty member lists keep their
// current null-ness so an absent optional attribute does not churn.
func (r *pveSdnFabricOpenfabricResource) readInto(ctx context.Context, m *pveSdnFabricOpenfabricResourceModel) error {
	fabric, err := r.client.GetSdnFabric(ctx, m.FabricID.ValueString())
	if err != nil {
		return err
	}
	m.IPPrefix = nodeNetworkStringToTF(fabric.IPPrefix)
	m.IP6Prefix = nodeNetworkStringToTF(fabric.IP6Prefix)
	m.CsnpInterval = nodeStoragesFloat64ToTF(fabric.CsnpInterval)
	m.HelloInterval = nodeStoragesFloat64ToTF(fabric.HelloInterval)
	m.RouteFilter = nodeNetworkStringToTF(fabric.RouteFilter)
	m.Digest = nodeNetworkStringToTF(fabric.Digest)
	nodes, err := r.client.ListSdnFabricNodes(ctx, m.FabricID.ValueString())
	if err != nil {
		return err
	}
	if len(nodes) > 0 || m.Nodes != nil {
		m.Nodes = sdnFabricOpenfabricNodesFromWire(nodes)
	}
	return nil
}

// sdnFabricOpenfabricFromModel projects the Terraform model into the create
// body.
func sdnFabricOpenfabricFromModel(m pveSdnFabricOpenfabricResourceModel) pveclient.SdnFabric {
	return pveclient.SdnFabric{
		ID:            m.FabricID.ValueString(),
		Protocol:      pveclient.SdnFabricProtocolOpenfabric,
		IPPrefix:      sdnFabricStringFromTF(m.IPPrefix),
		IP6Prefix:     sdnFabricStringFromTF(m.IP6Prefix),
		CsnpInterval:  sdnFabricFloatFromTF(m.CsnpInterval),
		HelloInterval: sdnFabricFloatFromTF(m.HelloInterval),
		RouteFilter:   sdnFabricStringFromTF(m.RouteFilter),
	}
}

// sdnFabricOpenfabricUpdateFromModel projects the Terraform model into the
// update body (without the delete list, which the caller computes against
// state).
func sdnFabricOpenfabricUpdateFromModel(m pveSdnFabricOpenfabricResourceModel) pveclient.SdnFabricUpdate {
	return pveclient.SdnFabricUpdate{
		IPPrefix:      sdnFabricStringFromTF(m.IPPrefix),
		IP6Prefix:     sdnFabricStringFromTF(m.IP6Prefix),
		CsnpInterval:  sdnFabricFloatFromTF(m.CsnpInterval),
		HelloInterval: sdnFabricFloatFromTF(m.HelloInterval),
		RouteFilter:   sdnFabricStringFromTF(m.RouteFilter),
	}
}

// sdnFabricOpenfabricDeleteFields returns the pin's deletable field names
// for attributes present in state but null in plan.
func sdnFabricOpenfabricDeleteFields(plan, state pveSdnFabricOpenfabricResourceModel) []string {
	var out []string
	if plan.IPPrefix.IsNull() && !state.IPPrefix.IsNull() {
		out = append(out, "ip_prefix")
	}
	if plan.IP6Prefix.IsNull() && !state.IP6Prefix.IsNull() {
		out = append(out, "ip6_prefix")
	}
	if plan.CsnpInterval.IsNull() && !state.CsnpInterval.IsNull() {
		out = append(out, "csnp_interval")
	}
	if plan.HelloInterval.IsNull() && !state.HelloInterval.IsNull() {
		out = append(out, "hello_interval")
	}
	if plan.RouteFilter.IsNull() && !state.RouteFilter.IsNull() {
		out = append(out, "route_filter")
	}
	return out
}

// sdnFabricOpenfabricDiffNodes creates, updates, and deletes node members
// so the upstream list matches the plan.
func sdnFabricOpenfabricDiffNodes(ctx context.Context, client *pveclient.Client, plan, state pveSdnFabricOpenfabricResourceModel) error {
	fabricID := plan.FabricID.ValueString()
	stateNodes := make(map[string]pveSdnFabricOpenfabricNodeModel, len(state.Nodes))
	for _, n := range state.Nodes {
		stateNodes[n.NodeID.ValueString()] = n
	}
	planNodes := make(map[string]bool, len(plan.Nodes))
	for _, n := range plan.Nodes {
		nodeID := n.NodeID.ValueString()
		planNodes[nodeID] = true
		wire := sdnFabricOpenfabricNodeFromModel(n, fabricID)
		old, exists := stateNodes[nodeID]
		if !exists {
			if err := client.CreateSdnFabricNode(ctx, wire); err != nil {
				return fmt.Errorf("adding node %s: %w", nodeID, err)
			}
			continue
		}
		if sdnFabricOpenfabricNodeEqual(n, old) {
			continue
		}
		upd := pveclient.SdnFabricNodeUpdate{
			IP:         wire.IP,
			IP6:        wire.IP6,
			Interfaces: wire.Interfaces,
			Delete:     sdnFabricOpenfabricNodeDeleteFields(n, old),
		}
		if err := client.UpdateSdnFabricNode(ctx, fabricID, nodeID, upd); err != nil {
			return fmt.Errorf("updating node %s: %w", nodeID, err)
		}
	}
	for nodeID := range stateNodes {
		if !planNodes[nodeID] {
			if err := client.DeleteSdnFabricNode(ctx, fabricID, nodeID); err != nil {
				return fmt.Errorf("removing node %s: %w", nodeID, err)
			}
		}
	}
	return nil
}

// sdnFabricOpenfabricNodeEqual reports whether the node member is unchanged.
func sdnFabricOpenfabricNodeEqual(a, b pveSdnFabricOpenfabricNodeModel) bool {
	if sdnFabricStringFromTF(a.IP) != sdnFabricStringFromTF(b.IP) ||
		sdnFabricStringFromTF(a.IP6) != sdnFabricStringFromTF(b.IP6) ||
		len(a.Interfaces) != len(b.Interfaces) {
		return false
	}
	for i := range a.Interfaces {
		x, y := a.Interfaces[i], b.Interfaces[i]
		if sdnFabricStringFromTF(x.Name) != sdnFabricStringFromTF(y.Name) ||
			sdnFabricStringFromTF(x.IP) != sdnFabricStringFromTF(y.IP) ||
			sdnFabricStringFromTF(x.IP6) != sdnFabricStringFromTF(y.IP6) ||
			x.HelloMultiplier.ValueInt64() != y.HelloMultiplier.ValueInt64() ||
			x.HelloMultiplier.IsNull() != y.HelloMultiplier.IsNull() {
			return false
		}
	}
	return true
}

// sdnFabricOpenfabricNodeDeleteFields returns the node fields to clear on
// update: attributes present in the stored row but null in the plan.
func sdnFabricOpenfabricNodeDeleteFields(plan, state pveSdnFabricOpenfabricNodeModel) []string {
	var out []string
	if plan.IP.IsNull() && !state.IP.IsNull() {
		out = append(out, "ip")
	}
	if plan.IP6.IsNull() && !state.IP6.IsNull() {
		out = append(out, "ip6")
	}
	if plan.Interfaces == nil && state.Interfaces != nil {
		out = append(out, "interfaces")
	}
	return out
}

// sdnFabricOpenfabricNodeFromModel projects a node member into the wire
// body.
func sdnFabricOpenfabricNodeFromModel(m pveSdnFabricOpenfabricNodeModel, fabricID string) pveclient.SdnFabricNode {
	wire := pveclient.SdnFabricNode{
		FabricID: fabricID,
		NodeID:   m.NodeID.ValueString(),
		Protocol: pveclient.SdnFabricProtocolOpenfabric,
		IP:       sdnFabricStringFromTF(m.IP),
		IP6:      sdnFabricStringFromTF(m.IP6),
	}
	for _, iface := range m.Interfaces {
		entry := pveclient.SdnFabricInterface{
			Name: sdnFabricStringFromTF(iface.Name),
			IP:   sdnFabricStringFromTF(iface.IP),
			IP6:  sdnFabricStringFromTF(iface.IP6),
		}
		if !iface.HelloMultiplier.IsNull() && !iface.HelloMultiplier.IsUnknown() {
			entry.HelloMultiplier = sdnFabricInt64Ptr(iface.HelloMultiplier.ValueInt64())
		}
		wire.Interfaces = append(wire.Interfaces, entry)
	}
	return wire
}

// sdnFabricOpenfabricNodesFromWire projects node members into the
// Terraform model.
func sdnFabricOpenfabricNodesFromWire(nodes []pveclient.SdnFabricNode) []pveSdnFabricOpenfabricNodeModel {
	out := make([]pveSdnFabricOpenfabricNodeModel, 0, len(nodes))
	for _, n := range nodes {
		row := pveSdnFabricOpenfabricNodeModel{
			NodeID: types.StringValue(n.NodeID),
			IP:     nodeNetworkStringToTF(n.IP),
			IP6:    nodeNetworkStringToTF(n.IP6),
		}
		for _, iface := range n.Interfaces {
			entry := pveSdnFabricOpenfabricIfaceModel{
				Name: types.StringValue(iface.Name),
				IP:   nodeNetworkStringToTF(iface.IP),
				IP6:  nodeNetworkStringToTF(iface.IP6),
			}
			if iface.HelloMultiplier != nil {
				entry.HelloMultiplier = types.Int64Value(*iface.HelloMultiplier)
			}
			row.Interfaces = append(row.Interfaces, entry)
		}
		out = append(out, row)
	}
	return out
}

// sdnFabricFloatFromTF maps an optional Terraform float onto the wire,
// mapping null and unknown to nil so absent attributes are omitted.
func sdnFabricFloatFromTF(v types.Float64) *float64 {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	f := v.ValueFloat64()
	return &f
}

// sdnFabricInt64Ptr returns a pointer to the supplied interval value.
func sdnFabricInt64Ptr(v int64) *int64 {
	return &v
}
