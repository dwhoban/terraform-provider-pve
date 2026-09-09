// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"regexp"

	stringvalidator "github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// sdnFabricIDPattern is the pin's pve-sdn-fabric-id format: 2 - 8
// alphanumeric or hyphen characters, starting and ending alphanumeric.
var sdnFabricIDPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9-]{0,6}[a-zA-Z0-9]$`)

// sdnFabricIDValidators is shared by both fabric resources.
var sdnFabricIDValidators = []validator.String{
	stringvalidator.LengthBetween(2, 8),
	stringvalidator.RegexMatches(sdnFabricIDPattern,
		"must be 2 - 8 characters of letters, digits, or hyphens, starting and ending with a letter or digit (PVE pve-sdn-fabric-id format)"),
}

// Ensure framework interfaces are satisfied.
var (
	_ resource.Resource                = &pveSdnFabricOspfResource{}
	_ resource.ResourceWithConfigure   = &pveSdnFabricOspfResource{}
	_ resource.ResourceWithImportState = &pveSdnFabricOspfResource{}
)

// NewPveSdnFabricOspfResource returns the resource implementation.
func NewPveSdnFabricOspfResource() resource.Resource {
	return &pveSdnFabricOspfResource{}
}

// pveSdnFabricOspfResource manages an OSPF SDN fabric
// (/cluster/sdn/fabrics/fabric) with its node members
// (/cluster/sdn/fabrics/node/{fabric_id}). Mutations are synchronous and
// stay pending until `pve_sdn_apply` runs.
type pveSdnFabricOspfResource struct {
	client *pveclient.Client
}

// pveSdnFabricOspfResourceModel is the Terraform-facing shape.
type pveSdnFabricOspfResourceModel struct {
	FabricID     types.String                        `tfsdk:"fabric_id"`
	IPPrefix     types.String                        `tfsdk:"ip_prefix"`
	IP6Prefix    types.String                        `tfsdk:"ip6_prefix"`
	Area         types.String                        `tfsdk:"area"`
	RouteFilter  types.String                        `tfsdk:"route_filter"`
	Redistribute []pveSdnFabricOspfRedistributeModel `tfsdk:"redistribute"`
	Nodes        []pveSdnFabricOspfNodeModel         `tfsdk:"nodes"`
	Digest       types.String                        `tfsdk:"digest"`
}

// pveSdnFabricOspfRedistributeModel is one OSPF redistribute entry.
type pveSdnFabricOspfRedistributeModel struct {
	Source   types.String `tfsdk:"source"`
	RouteMap types.String `tfsdk:"route_map"`
}

// pveSdnFabricOspfNodeModel is one node member of the OSPF fabric.
type pveSdnFabricOspfNodeModel struct {
	NodeID     types.String                 `tfsdk:"node_id"`
	IP         types.String                 `tfsdk:"ip"`
	IP6        types.String                 `tfsdk:"ip6"`
	Interfaces []pveSdnFabricOspfIfaceModel `tfsdk:"interfaces"`
}

// pveSdnFabricOspfIfaceModel is one OSPF interface of a node member.
type pveSdnFabricOspfIfaceModel struct {
	Name        types.String `tfsdk:"name"`
	IP          types.String `tfsdk:"ip"`
	IP6         types.String `tfsdk:"ip6"`
	NetworkType types.String `tfsdk:"network_type"`
}

// Metadata implements resource.Resource.
func (r *pveSdnFabricOspfResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveSdnFabricOspf
}

// Schema implements resource.Resource.
func (r *pveSdnFabricOspfResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages an OSPF SDN fabric (`/cluster/sdn/fabrics/fabric`, section protocol `ospf`) with its per-node member entries (`/cluster/sdn/fabrics/node/{fabric_id}`). Changes stay pending and take effect only after the `pve_sdn_apply` action runs. The pin marks `ip_prefix` and `ip6_prefix` as not deletable on OSPF fabrics: clearing them in configuration leaves the upstream value in place.",
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
			"area": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "OSPF area, either an IPv4 address or a 32-bit number (e.g. `0.0.0.0`).",
			},
			"route_filter": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Prefix list (see `pve_sdn_prefix_list`) used to filter routes installed into the kernel routing table.",
			},
			"redistribute": schema.ListNestedAttribute{
				Optional:            true,
				MarkdownDescription: "Route sources redistributed into OSPF, each with an optional route map filter.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"source": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "The protocol to redistribute routes from. Must be one of: `bgp`, `connected`, `kernel`, `static`.",
							Validators: []validator.String{
								stringvalidator.OneOf("bgp", "connected", "kernel", "static"),
							},
						},
						"route_map": schema.StringAttribute{
							Optional:            true,
							MarkdownDescription: "Route map (see `pve_sdn_route_map`) filtering or transforming routes redistributed from this source.",
						},
					},
				},
			},
			"nodes": schema.ListNestedAttribute{
				Optional:            true,
				MarkdownDescription: "Per-node member entries of the fabric. Each entry joins the named cluster node into the OSPF fabric.",
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
							MarkdownDescription: "Network interfaces participating in OSPF on this node.",
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
									"network_type": schema.StringAttribute{
										Optional:            true,
										MarkdownDescription: "OSPF network type of the interface. Must be one of: `broadcast`, `non-broadcast`, `point-to-multipoint`, `point-to-point`.",
										Validators: []validator.String{
											stringvalidator.OneOf("broadcast", "non-broadcast", "point-to-multipoint", "point-to-point"),
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
func (r *pveSdnFabricOspfResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = haConfigureResource(req, resp)
}

// Create implements resource.Resource.
func (r *pveSdnFabricOspfResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveSdnFabricOspfResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error creating pve_sdn_fabric_ospf", "provider client is not configured")
		return
	}
	if err := r.client.CreateSdnFabric(ctx, sdnFabricOspfFromModel(plan)); err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_sdn_fabric_ospf",
			fmt.Sprintf("creating OSPF fabric %s: %s", plan.FabricID.ValueString(), err),
		)
		return
	}
	for _, node := range plan.Nodes {
		if err := r.client.CreateSdnFabricNode(ctx, sdnFabricOspfNodeFromModel(node, plan.FabricID.ValueString())); err != nil {
			resp.Diagnostics.AddError(
				"Error creating pve_sdn_fabric_ospf",
				fmt.Sprintf("adding node %s to OSPF fabric %s: %s", node.NodeID.ValueString(), plan.FabricID.ValueString(), err),
			)
			return
		}
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_sdn_fabric_ospf after create",
			fmt.Sprintf("reading OSPF fabric %s: %s", plan.FabricID.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read implements resource.Resource.
func (r *pveSdnFabricOspfResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveSdnFabricOspfResourceModel
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
			"Error reading pve_sdn_fabric_ospf",
			fmt.Sprintf("reading OSPF fabric %s: %s", state.FabricID.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource.
func (r *pveSdnFabricOspfResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveSdnFabricOspfResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state pveSdnFabricOspfResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	fabricID := plan.FabricID.ValueString()
	upd := sdnFabricOspfUpdateFromModel(plan)
	upd.Delete = sdnFabricOspfDeleteFields(plan, state)
	if err := r.client.UpdateSdnFabric(ctx, fabricID, upd); err != nil {
		resp.Diagnostics.AddError(
			"Error updating pve_sdn_fabric_ospf",
			fmt.Sprintf("updating OSPF fabric %s: %s", fabricID, err),
		)
		return
	}
	if err := sdnFabricOspfDiffNodes(ctx, r.client, plan, state); err != nil {
		resp.Diagnostics.AddError(
			"Error updating pve_sdn_fabric_ospf",
			fmt.Sprintf("updating node members of OSPF fabric %s: %s", fabricID, err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_sdn_fabric_ospf after update",
			fmt.Sprintf("reading OSPF fabric %s: %s", fabricID, err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete implements resource.Resource.
func (r *pveSdnFabricOspfResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveSdnFabricOspfResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	fabricID := state.FabricID.ValueString()
	for _, node := range state.Nodes {
		if err := r.client.DeleteSdnFabricNode(ctx, fabricID, node.NodeID.ValueString()); err != nil && !isPVEClientNotFound(err) {
			resp.Diagnostics.AddError(
				"Error deleting pve_sdn_fabric_ospf",
				fmt.Sprintf("deleting node %s of OSPF fabric %s: %s", node.NodeID.ValueString(), fabricID, err),
			)
			return
		}
	}
	if err := r.client.DeleteSdnFabric(ctx, fabricID); err != nil && !isPVEClientNotFound(err) {
		resp.Diagnostics.AddError(
			"Error deleting pve_sdn_fabric_ospf",
			fmt.Sprintf("deleting OSPF fabric %s: %s", fabricID, err),
		)
	}
}

// ImportState parses an import ID of the form `<fabric_id>`.
func (r *pveSdnFabricOspfResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		resp.Diagnostics.AddError("Invalid pve_sdn_fabric_ospf import ID", "import ID must be the fabric identifier, e.g. `ospf1`")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("fabric_id"), req.ID)...)
}

// readInto refreshes the model from PVE. Empty member lists keep their
// current null-ness so an absent optional attribute does not churn.
func (r *pveSdnFabricOspfResource) readInto(ctx context.Context, m *pveSdnFabricOspfResourceModel) error {
	fabric, err := r.client.GetSdnFabric(ctx, m.FabricID.ValueString())
	if err != nil {
		return err
	}
	m.IPPrefix = nodeNetworkStringToTF(fabric.IPPrefix)
	m.IP6Prefix = nodeNetworkStringToTF(fabric.IP6Prefix)
	m.Area = nodeNetworkStringToTF(fabric.Area)
	m.RouteFilter = nodeNetworkStringToTF(fabric.RouteFilter)
	m.Digest = nodeNetworkStringToTF(fabric.Digest)
	if len(fabric.Redistribute) > 0 || m.Redistribute != nil {
		m.Redistribute = sdnFabricOspfRedistributeFromWire(fabric.Redistribute)
	}
	nodes, err := r.client.ListSdnFabricNodes(ctx, m.FabricID.ValueString())
	if err != nil {
		return err
	}
	if len(nodes) > 0 || m.Nodes != nil {
		m.Nodes = sdnFabricOspfNodesFromWire(nodes)
	}
	return nil
}

// sdnFabricOspfFromModel projects the Terraform model into the create body.
func sdnFabricOspfFromModel(m pveSdnFabricOspfResourceModel) pveclient.SdnFabric {
	return pveclient.SdnFabric{
		ID:           m.FabricID.ValueString(),
		Protocol:     pveclient.SdnFabricProtocolOSPF,
		IPPrefix:     sdnFabricStringFromTF(m.IPPrefix),
		IP6Prefix:    sdnFabricStringFromTF(m.IP6Prefix),
		Area:         sdnFabricStringFromTF(m.Area),
		RouteFilter:  sdnFabricStringFromTF(m.RouteFilter),
		Redistribute: sdnFabricOspfRedistributeToWire(m.Redistribute),
	}
}

// sdnFabricOspfUpdateFromModel projects the Terraform model into the update
// body (without the delete list, which the caller computes against state).
func sdnFabricOspfUpdateFromModel(m pveSdnFabricOspfResourceModel) pveclient.SdnFabricUpdate {
	return pveclient.SdnFabricUpdate{
		IPPrefix:     sdnFabricStringFromTF(m.IPPrefix),
		IP6Prefix:    sdnFabricStringFromTF(m.IP6Prefix),
		Area:         sdnFabricStringFromTF(m.Area),
		RouteFilter:  sdnFabricStringFromTF(m.RouteFilter),
		Redistribute: sdnFabricOspfRedistributeToWire(m.Redistribute),
	}
}

// sdnFabricOspfDeleteFields returns the pin's deletable field names for
// attributes present in state but null in plan.
func sdnFabricOspfDeleteFields(plan, state pveSdnFabricOspfResourceModel) []string {
	var out []string
	if plan.Area.IsNull() && !state.Area.IsNull() {
		out = append(out, "area")
	}
	if plan.RouteFilter.IsNull() && !state.RouteFilter.IsNull() {
		out = append(out, "route_filter")
	}
	if plan.Redistribute == nil && state.Redistribute != nil {
		out = append(out, "redistribute")
	}
	return out
}

// sdnFabricOspfDiffNodes creates, updates, and deletes node members so the
// upstream list matches the plan.
func sdnFabricOspfDiffNodes(ctx context.Context, client *pveclient.Client, plan, state pveSdnFabricOspfResourceModel) error {
	fabricID := plan.FabricID.ValueString()
	stateNodes := make(map[string]pveSdnFabricOspfNodeModel, len(state.Nodes))
	for _, n := range state.Nodes {
		stateNodes[n.NodeID.ValueString()] = n
	}
	planNodes := make(map[string]bool, len(plan.Nodes))
	for _, n := range plan.Nodes {
		nodeID := n.NodeID.ValueString()
		planNodes[nodeID] = true
		wire := sdnFabricOspfNodeFromModel(n, fabricID)
		old, exists := stateNodes[nodeID]
		if !exists {
			if err := client.CreateSdnFabricNode(ctx, wire); err != nil {
				return fmt.Errorf("adding node %s: %w", nodeID, err)
			}
			continue
		}
		if sdnFabricOspfNodeEqual(n, old) {
			continue
		}
		upd := pveclient.SdnFabricNodeUpdate{
			IP:         wire.IP,
			IP6:        wire.IP6,
			Interfaces: wire.Interfaces,
			Delete:     sdnFabricOspfNodeDeleteFields(n, old),
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

// sdnFabricOspfNodeEqual reports whether the node member is unchanged.
func sdnFabricOspfNodeEqual(a, b pveSdnFabricOspfNodeModel) bool {
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
			sdnFabricStringFromTF(x.NetworkType) != sdnFabricStringFromTF(y.NetworkType) {
			return false
		}
	}
	return true
}

// sdnFabricOspfNodeDeleteFields returns the node fields to clear on update:
// attributes present in the stored row but null in the plan.
func sdnFabricOspfNodeDeleteFields(plan, state pveSdnFabricOspfNodeModel) []string {
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

// sdnFabricOspfNodeFromModel projects a node member into the wire body.
func sdnFabricOspfNodeFromModel(m pveSdnFabricOspfNodeModel, fabricID string) pveclient.SdnFabricNode {
	wire := pveclient.SdnFabricNode{
		FabricID: fabricID,
		NodeID:   m.NodeID.ValueString(),
		Protocol: pveclient.SdnFabricProtocolOSPF,
		IP:       sdnFabricStringFromTF(m.IP),
		IP6:      sdnFabricStringFromTF(m.IP6),
	}
	for _, iface := range m.Interfaces {
		wire.Interfaces = append(wire.Interfaces, pveclient.SdnFabricInterface{
			Name:        sdnFabricStringFromTF(iface.Name),
			IP:          sdnFabricStringFromTF(iface.IP),
			IP6:         sdnFabricStringFromTF(iface.IP6),
			NetworkType: sdnFabricStringFromTF(iface.NetworkType),
		})
	}
	return wire
}

// sdnFabricOspfNodesFromWire projects node members into the Terraform model.
func sdnFabricOspfNodesFromWire(nodes []pveclient.SdnFabricNode) []pveSdnFabricOspfNodeModel {
	out := make([]pveSdnFabricOspfNodeModel, 0, len(nodes))
	for _, n := range nodes {
		row := pveSdnFabricOspfNodeModel{
			NodeID: types.StringValue(n.NodeID),
			IP:     nodeNetworkStringToTF(n.IP),
			IP6:    nodeNetworkStringToTF(n.IP6),
		}
		for _, iface := range n.Interfaces {
			row.Interfaces = append(row.Interfaces, pveSdnFabricOspfIfaceModel{
				Name:        types.StringValue(iface.Name),
				IP:          nodeNetworkStringToTF(iface.IP),
				IP6:         nodeNetworkStringToTF(iface.IP6),
				NetworkType: nodeNetworkStringToTF(iface.NetworkType),
			})
		}
		out = append(out, row)
	}
	return out
}

// sdnFabricOspfRedistributeToWire projects redistribute entries into the
// wire body.
func sdnFabricOspfRedistributeToWire(models []pveSdnFabricOspfRedistributeModel) []pveclient.SdnFabricRedistribute {
	if models == nil {
		return nil
	}
	out := make([]pveclient.SdnFabricRedistribute, 0, len(models))
	for _, m := range models {
		out = append(out, pveclient.SdnFabricRedistribute{
			Source:   sdnFabricStringFromTF(m.Source),
			RouteMap: sdnFabricStringFromTF(m.RouteMap),
		})
	}
	return out
}

// sdnFabricStringFromTF maps an optional Terraform string onto the wire,
// mapping null and unknown to the empty string so absent attributes are
// omitted from request bodies.
func sdnFabricStringFromTF(v types.String) string {
	if v.IsNull() || v.IsUnknown() {
		return ""
	}
	return v.ValueString()
}

// sdnFabricOspfRedistributeFromWire projects redistribute entries into the
// Terraform model.
func sdnFabricOspfRedistributeFromWire(entries []pveclient.SdnFabricRedistribute) []pveSdnFabricOspfRedistributeModel {
	out := make([]pveSdnFabricOspfRedistributeModel, 0, len(entries))
	for _, e := range entries {
		out = append(out, pveSdnFabricOspfRedistributeModel{
			Source:   types.StringValue(e.Source),
			RouteMap: nodeNetworkStringToTF(e.RouteMap),
		})
	}
	return out
}
