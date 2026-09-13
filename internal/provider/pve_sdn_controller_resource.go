// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// sdnControllerTypes enumerates the pin's closed controller plugin type
// enum.
var sdnControllerTypes = []string{"bgp", "evpn", "faucet", "isis"}

// sdnControllerBgpModes enumerates the pin's closed bgp-mode enum.
var sdnControllerBgpModes = []string{"auto", "external", "internal"}

// Ensure framework interfaces are satisfied.
var (
	_ resource.Resource                = &pveSdnControllerResource{}
	_ resource.ResourceWithConfigure   = &pveSdnControllerResource{}
	_ resource.ResourceWithImportState = &pveSdnControllerResource{}
)

// NewPveSdnControllerResource returns the resource implementation.
func NewPveSdnControllerResource() resource.Resource {
	return &pveSdnControllerResource{}
}

// pveSdnControllerResource manages an SDN controller object
// (/cluster/sdn/controllers). Every mutation is synchronous per the pin.
type pveSdnControllerResource struct {
	client *pveclient.Client
}

// pveSdnControllerResourceModel is the Terraform-facing shape.
type pveSdnControllerResourceModel struct {
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

// Metadata implements resource.Resource.
func (r *pveSdnControllerResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveSdnController
}

// Schema implements resource.Resource.
func (r *pveSdnControllerResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages an SDN controller object (`/cluster/sdn/controllers`). Controllers run the routing backbone for EVPN zones (`bgp`, `evpn`, `isis`) or the OpenFlow controller for faucet zones. Changes stay pending until the `pve_sdn_apply` action runs.",
		Attributes: map[string]schema.Attribute{
			"controller": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The SDN controller object identifier (2 to 64 characters, starting with a letter; letters, digits, underscores, and hyphens). Changing this value forces recreation.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"type": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Controller plugin type. Must be one of: `bgp`, `evpn`, `faucet`, `isis`. Changing this value forces recreation.",
				Validators: []validator.String{
					stringvalidator.OneOf(sdnControllerTypes...),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"asn": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Autonomous system number. BGP and EVPN only. Must be between 0 and 4294967295.",
				Validators: []validator.Int64{
					int64validator.Between(0, 4294967295),
				},
			},
			"bgp_mode": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Whether to use eBGP or iBGP; `auto` chooses depending on the BGP controller and falls back to iBGP. BGP only. Must be one of: `auto`, `external`, `internal`.",
				Validators: []validator.String{
					stringvalidator.OneOf(sdnControllerBgpModes...),
				},
			},
			"bgp_multipath_as_path_relax": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Consider different AS paths of equal length for multipath computation. BGP only.",
			},
			"ebgp": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Enable eBGP (remote-as external). BGP only.",
			},
			"ebgp_multihop": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Maximum amount of hops for eBGP peers; requires `ebgp` to be enabled. BGP only.",
			},
			"fabric": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "SDN fabric used as underlay for this EVPN controller. EVPN only.",
			},
			"isis_domain": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Name of the IS-IS domain. IS-IS only.",
			},
			"isis_ifaces": schema.ListAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Interfaces where IS-IS should be active. IS-IS only.",
			},
			"isis_net": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Network Entity title for this node in the IS-IS network, 20 to 50 characters. IS-IS only.",
			},
			"loopback": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Name of the loopback/dummy interface that provides the Router-IP. BGP only.",
			},
			"node": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Node where this controller is active.",
			},
			"nodes": schema.ListAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Cluster node names where this controller is active.",
			},
			"peer_group_name": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Name of the peer group for this EVPN controller; PVE defaults to `VTEP`. EVPN only.",
			},
			"peers": schema.ListAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Peer IP addresses.",
			},
			"route_map_in": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Route map applied to incoming routes.",
			},
			"route_map_out": schema.StringAttribute{
				Optional:            true,
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

// Configure implements resource.ResourceWithConfigure.
func (r *pveSdnControllerResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = haConfigureResource(req, resp)
}

// Create implements resource.Resource.
func (r *pveSdnControllerResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveSdnControllerResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error creating pve_sdn_controller", "provider client is not configured")
		return
	}
	if err := r.client.CreateSdnController(ctx, sdnControllerFromModel(plan)); err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_sdn_controller",
			fmt.Sprintf("creating SDN controller %s: %s", plan.Controller.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_sdn_controller after create",
			fmt.Sprintf("reading SDN controller %s: %s", plan.Controller.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read implements resource.Resource.
func (r *pveSdnControllerResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveSdnControllerResourceModel
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
			"Error reading pve_sdn_controller",
			fmt.Sprintf("reading SDN controller %s: %s", state.Controller.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource.
func (r *pveSdnControllerResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveSdnControllerResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state pveSdnControllerResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error updating pve_sdn_controller", "provider client is not configured")
		return
	}
	deleteFields := sdnControllerDeleteFields(plan, state)
	if err := r.client.UpdateSdnController(ctx, plan.Controller.ValueString(), sdnControllerFromModel(plan), deleteFields); err != nil {
		resp.Diagnostics.AddError(
			"Error updating pve_sdn_controller",
			fmt.Sprintf("updating SDN controller %s: %s", plan.Controller.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_sdn_controller after update",
			fmt.Sprintf("reading SDN controller %s: %s", plan.Controller.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete implements resource.Resource.
func (r *pveSdnControllerResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveSdnControllerResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error deleting pve_sdn_controller", "provider client is not configured")
		return
	}
	if err := r.client.DeleteSdnController(ctx, state.Controller.ValueString()); err != nil {
		if isPVEClientNotFound(err) {
			// Already absent counts as deleted.
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting pve_sdn_controller",
			fmt.Sprintf("deleting SDN controller %s: %s", state.Controller.ValueString(), err),
		)
	}
}

// ImportState parses an import ID of the form `<controller>`.
func (r *pveSdnControllerResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		resp.Diagnostics.AddError("Invalid pve_sdn_controller import ID", "import ID must be the controller identifier, e.g. `bgp1`")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("controller"), req.ID)...)
}

// readInto refreshes the model from PVE.
func (r *pveSdnControllerResource) readInto(ctx context.Context, m *pveSdnControllerResourceModel) error {
	controller, err := r.client.GetSdnController(ctx, m.Controller.ValueString())
	if err != nil {
		return err
	}
	sdnControllerReadIntoModel(m, *controller)
	return nil
}

// sdnControllerReadIntoModel copies a wire controller into a model,
// preserving the required identifier.
func sdnControllerReadIntoModel(m *pveSdnControllerResourceModel, c pveclient.SdnController) {
	m.Type = nodeNetworkStringToTF(c.Type)
	m.Asn = haInt64PtrToTF(c.ASN)
	m.BgpMode = nodeNetworkStringToTF(c.BgpMode)
	m.BgpMultipathAsPathRelax = nodeNetworkBoolPtrToTF(c.BgpMultipathAsPathRelax)
	m.Ebgp = nodeNetworkBoolPtrToTF(c.Ebgp)
	m.EbgpMultihop = haInt64PtrToTF(c.EbgpMultihop)
	m.Fabric = nodeNetworkStringToTF(c.Fabric)
	m.IsisDomain = nodeNetworkStringToTF(c.IsisDomain)
	m.IsisIfaces = listStringToTF(c.IsisIfaces)
	m.IsisNet = nodeNetworkStringToTF(c.IsisNet)
	m.Loopback = nodeNetworkStringToTF(c.Loopback)
	m.Node = nodeNetworkStringToTF(c.Node)
	m.Nodes = listStringToTF(c.Nodes)
	m.PeerGroupName = nodeNetworkStringToTF(c.PeerGroupName)
	m.Peers = listStringToTF(c.Peers)
	m.RouteMapIn = nodeNetworkStringToTF(c.RouteMapIn)
	m.RouteMapOut = nodeNetworkStringToTF(c.RouteMapOut)
	m.Digest = nodeNetworkStringToTF(c.Digest)
	m.State = nodeNetworkStringToTF(c.State)
}

// sdnControllerFromModel projects the Terraform model into the wire body.
func sdnControllerFromModel(m pveSdnControllerResourceModel) pveclient.SdnController {
	body := pveclient.SdnController{
		Controller: m.Controller.ValueString(),
		Type:       m.Type.ValueString(),
	}
	if !m.Asn.IsNull() && !m.Asn.IsUnknown() {
		body.ASN = pveclient.HAInt64Ptr(m.Asn.ValueInt64())
	}
	if !m.BgpMode.IsNull() && !m.BgpMode.IsUnknown() {
		body.BgpMode = m.BgpMode.ValueString()
	}
	if !m.BgpMultipathAsPathRelax.IsNull() && !m.BgpMultipathAsPathRelax.IsUnknown() {
		body.BgpMultipathAsPathRelax = pveclient.HABoolPtr(m.BgpMultipathAsPathRelax.ValueBool())
	}
	if !m.Ebgp.IsNull() && !m.Ebgp.IsUnknown() {
		body.Ebgp = pveclient.HABoolPtr(m.Ebgp.ValueBool())
	}
	if !m.EbgpMultihop.IsNull() && !m.EbgpMultihop.IsUnknown() {
		body.EbgpMultihop = pveclient.HAInt64Ptr(m.EbgpMultihop.ValueInt64())
	}
	if !m.Fabric.IsNull() && !m.Fabric.IsUnknown() {
		body.Fabric = m.Fabric.ValueString()
	}
	if !m.IsisDomain.IsNull() && !m.IsisDomain.IsUnknown() {
		body.IsisDomain = m.IsisDomain.ValueString()
	}
	body.IsisIfaces = listStringFromTF(m.IsisIfaces)
	if !m.IsisNet.IsNull() && !m.IsisNet.IsUnknown() {
		body.IsisNet = m.IsisNet.ValueString()
	}
	if !m.Loopback.IsNull() && !m.Loopback.IsUnknown() {
		body.Loopback = m.Loopback.ValueString()
	}
	if !m.Node.IsNull() && !m.Node.IsUnknown() {
		body.Node = m.Node.ValueString()
	}
	body.Nodes = listStringFromTF(m.Nodes)
	if !m.PeerGroupName.IsNull() && !m.PeerGroupName.IsUnknown() {
		body.PeerGroupName = m.PeerGroupName.ValueString()
	}
	body.Peers = listStringFromTF(m.Peers)
	if !m.RouteMapIn.IsNull() && !m.RouteMapIn.IsUnknown() {
		body.RouteMapIn = m.RouteMapIn.ValueString()
	}
	if !m.RouteMapOut.IsNull() && !m.RouteMapOut.IsUnknown() {
		body.RouteMapOut = m.RouteMapOut.ValueString()
	}
	return body
}

// sdnControllerDeleteFields returns the PVE field names to clear on update:
// optional attributes present in state but null in plan.
func sdnControllerDeleteFields(plan, state pveSdnControllerResourceModel) []string {
	var out []string
	if plan.Asn.IsNull() && !state.Asn.IsNull() {
		out = append(out, "asn")
	}
	if plan.BgpMode.IsNull() && !state.BgpMode.IsNull() {
		out = append(out, "bgp-mode")
	}
	if plan.BgpMultipathAsPathRelax.IsNull() && !state.BgpMultipathAsPathRelax.IsNull() {
		out = append(out, "bgp-multipath-as-path-relax")
	}
	if plan.Ebgp.IsNull() && !state.Ebgp.IsNull() {
		out = append(out, "ebgp")
	}
	if plan.EbgpMultihop.IsNull() && !state.EbgpMultihop.IsNull() {
		out = append(out, "ebgp-multihop")
	}
	if plan.Fabric.IsNull() && !state.Fabric.IsNull() {
		out = append(out, "fabric")
	}
	if plan.IsisDomain.IsNull() && !state.IsisDomain.IsNull() {
		out = append(out, "isis-domain")
	}
	if plan.IsisIfaces.IsNull() && !state.IsisIfaces.IsNull() {
		out = append(out, "isis-ifaces")
	}
	if plan.IsisNet.IsNull() && !state.IsisNet.IsNull() {
		out = append(out, "isis-net")
	}
	if plan.Loopback.IsNull() && !state.Loopback.IsNull() {
		out = append(out, "loopback")
	}
	if plan.Node.IsNull() && !state.Node.IsNull() {
		out = append(out, "node")
	}
	if plan.Nodes.IsNull() && !state.Nodes.IsNull() {
		out = append(out, "nodes")
	}
	if plan.PeerGroupName.IsNull() && !state.PeerGroupName.IsNull() {
		out = append(out, "peer-group-name")
	}
	if plan.Peers.IsNull() && !state.Peers.IsNull() {
		out = append(out, "peers")
	}
	if plan.RouteMapIn.IsNull() && !state.RouteMapIn.IsNull() {
		out = append(out, "route-map-in")
	}
	if plan.RouteMapOut.IsNull() && !state.RouteMapOut.IsNull() {
		out = append(out, "route-map-out")
	}
	return out
}
