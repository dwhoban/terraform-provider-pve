// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ resource.Resource                = &pveSdnZoneEvpnResource{}
	_ resource.ResourceWithConfigure   = &pveSdnZoneEvpnResource{}
	_ resource.ResourceWithImportState = &pveSdnZoneEvpnResource{}
)

// NewPveSdnZoneEvpnResource returns the resource implementation.
func NewPveSdnZoneEvpnResource() resource.Resource {
	return &pveSdnZoneEvpnResource{}
}

// pveSdnZoneEvpnResource manages one EVPN SDN zone (POST/GET/PUT/DELETE on
// /cluster/sdn/zones with type `evpn`).
type pveSdnZoneEvpnResource struct {
	client *pveclient.Client
}

// pveSdnZoneEvpnResourceModel is the Terraform-facing shape.
type pveSdnZoneEvpnResourceModel struct {
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

// Metadata implements resource.Resource.
func (r *pveSdnZoneEvpnResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveSdnZoneEvpn
}

// Schema implements resource.Resource.
func (r *pveSdnZoneEvpnResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := sdnZoneCommonAttributes()
	attrs["controller"] = schema.StringAttribute{
		Optional:            true,
		MarkdownDescription: "ID of the primary BGP/EVPN controller (PVE `controller`) for this zone, for example a `pve_sdn_controller` of type `evpn`.",
	}
	attrs["secondary_controllers"] = schema.ListAttribute{
		ElementType:         types.StringType,
		Optional:            true,
		MarkdownDescription: "Additional controller IDs (PVE `secondary-controllers`) peering with the primary controller.",
	}
	attrs["advertise_subnets"] = schema.BoolAttribute{
		Optional:            true,
		MarkdownDescription: "Advertise IP prefixes (Type-5 routes) instead of MAC/IP pairs (Type-2 routes) (PVE `advertise-subnets`).",
	}
	attrs["disable_arp_nd_suppression"] = schema.BoolAttribute{
		Optional:            true,
		MarkdownDescription: "Suppress IPv4 ARP and IPv6 Neighbour Discovery messages (PVE `disable-arp-nd-suppression`).",
	}
	attrs["exitnodes"] = schema.StringAttribute{
		Optional:            true,
		MarkdownDescription: "Comma-separated list of cluster node names (PVE `pve-node-list`) that act as exit nodes routing EVPN traffic to the outside, for example `pve1,pve2`.",
	}
	attrs["exitnodes_local_routing"] = schema.BoolAttribute{
		Optional:            true,
		MarkdownDescription: "Create routes on the exit nodes so they can connect to EVPN guests directly (PVE `exitnodes-local-routing`).",
	}
	attrs["exitnodes_primary"] = schema.StringAttribute{
		Optional:            true,
		MarkdownDescription: "Cluster node name (PVE `pve-node`) that traffic is forced through first, e.g. `pve1`.",
	}
	attrs["mac"] = schema.StringAttribute{
		Optional:            true,
		MarkdownDescription: "Anycast logical router MAC address (PVE `mac-addr`) for this zone, for example `AA:BB:CC:DD:EE:FF`.",
	}
	attrs["rt_import"] = schema.StringAttribute{
		Optional:            true,
		MarkdownDescription: "Comma-separated list of Route Targets (PVE `pve-sdn-bgp-rt-list`) imported into the VRF of this zone via BGP, for example `65000:100`.",
	}
	attrs["vrf_vxlan"] = schema.Int64Attribute{
		Optional:            true,
		MarkdownDescription: "VNI for the zone VRF (PVE `vrf-vxlan`). Must be between 1 and 16777215.",
		Validators: []validator.Int64{
			int64validator.Between(1, 16777215),
		},
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages an EVPN SDN zone (`/cluster/sdn/zones`, type `evpn`) built on a BGP/EVPN controller. Creating, updating, or deleting a zone only changes the pending SDN configuration; run the `pve_sdn_apply` action to apply it to the running configuration. Changing `zone` forces recreation.",
		Attributes:          attrs,
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveSdnZoneEvpnResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = haConfigureResource(req, resp)
}

// Create implements resource.Resource.
func (r *pveSdnZoneEvpnResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveSdnZoneEvpnResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error creating pve_sdn_zone_evpn", "provider client is not configured")
		return
	}
	if err := r.client.CreateSdnZone(ctx, sdnZoneEvpnFromModel(plan)); err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_sdn_zone_evpn",
			fmt.Sprintf("creating zone %s: %s", plan.Zone.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_sdn_zone_evpn after create",
			fmt.Sprintf("reading zone %s: %s", plan.Zone.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	tflog.Debug(ctx, "created PVE evpn sdn zone", map[string]any{"zone": plan.Zone.ValueString()})
}

// Read implements resource.Resource.
func (r *pveSdnZoneEvpnResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveSdnZoneEvpnResourceModel
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
			"Error reading pve_sdn_zone_evpn",
			fmt.Sprintf("reading zone %s: %s", state.Zone.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource. Changed fields travel in the PUT
// body; attributes cleared in the plan travel in the `delete` query
// parameter.
func (r *pveSdnZoneEvpnResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveSdnZoneEvpnResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state pveSdnZoneEvpnResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	deleteFields := append(
		sdnZoneCommonDeleteFields(plan.sdnZoneCommonModel, state.sdnZoneCommonModel),
		sdnZoneEvpnDeleteFields(plan, state)...,
	)
	if err := r.client.UpdateSdnZone(ctx, plan.Zone.ValueString(), sdnZoneEvpnFromModel(plan), deleteFields); err != nil {
		resp.Diagnostics.AddError(
			"Error updating pve_sdn_zone_evpn",
			fmt.Sprintf("updating zone %s: %s", plan.Zone.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_sdn_zone_evpn after update",
			fmt.Sprintf("reading zone %s: %s", plan.Zone.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	tflog.Debug(ctx, "updated PVE evpn sdn zone", map[string]any{"zone": plan.Zone.ValueString()})
}

// Delete implements resource.Resource.
func (r *pveSdnZoneEvpnResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveSdnZoneEvpnResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	tflog.Debug(ctx, "deleting PVE evpn sdn zone", map[string]any{"zone": state.Zone.ValueString()})
	if err := r.client.DeleteSdnZone(ctx, state.Zone.ValueString()); err != nil {
		if isPVEClientNotFound(err) {
			// Already absent counts as deleted.
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting pve_sdn_zone_evpn",
			fmt.Sprintf("deleting zone %s: %s", state.Zone.ValueString(), err),
		)
	}
}

// ImportState parses an import ID of the form `<zone>`.
func (r *pveSdnZoneEvpnResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		resp.Diagnostics.AddError("Invalid pve_sdn_zone_evpn import ID", "import ID must be the zone identifier, e.g. `zone1`")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("zone"), req.ID)...)
}

// readInto refreshes the model from PVE, erroring when the upstream type
// is no longer evpn.
func (r *pveSdnZoneEvpnResource) readInto(ctx context.Context, m *pveSdnZoneEvpnResourceModel) error {
	z, err := sdnZoneGetChecked(ctx, r.client, m.Zone.ValueString(), "evpn")
	if err != nil {
		return err
	}
	sdnZoneEvpnApply(z, m)
	return nil
}

// sdnZoneEvpnFromModel projects the Terraform model into the wire body;
// null values are omitted from the request.
func sdnZoneEvpnFromModel(m pveSdnZoneEvpnResourceModel) pveclient.SdnZone {
	body := sdnZoneCommonFromModel(m.sdnZoneCommonModel)
	body.Type = "evpn"
	if !m.Controller.IsNull() && !m.Controller.IsUnknown() {
		body.Controller = m.Controller.ValueString()
	}
	if !m.SecondaryControllers.IsNull() && !m.SecondaryControllers.IsUnknown() {
		body.SecondaryControllers = listStringFromTF(m.SecondaryControllers)
	}
	if !m.AdvertiseSubnets.IsNull() && !m.AdvertiseSubnets.IsUnknown() {
		v := m.AdvertiseSubnets.ValueBool()
		body.AdvertiseSubnets = &v
	}
	if !m.DisableArpNdSuppression.IsNull() && !m.DisableArpNdSuppression.IsUnknown() {
		v := m.DisableArpNdSuppression.ValueBool()
		body.DisableArpNdSuppression = &v
	}
	if !m.ExitNodes.IsNull() && !m.ExitNodes.IsUnknown() {
		body.ExitNodes = m.ExitNodes.ValueString()
	}
	if !m.ExitNodesLocalRouting.IsNull() && !m.ExitNodesLocalRouting.IsUnknown() {
		v := m.ExitNodesLocalRouting.ValueBool()
		body.ExitNodesLocalRouting = &v
	}
	if !m.ExitNodesPrimary.IsNull() && !m.ExitNodesPrimary.IsUnknown() {
		body.ExitNodesPrimary = m.ExitNodesPrimary.ValueString()
	}
	if !m.Mac.IsNull() && !m.Mac.IsUnknown() {
		body.Mac = m.Mac.ValueString()
	}
	if !m.RtImport.IsNull() && !m.RtImport.IsUnknown() {
		body.RtImport = m.RtImport.ValueString()
	}
	if !m.VrfVxlan.IsNull() && !m.VrfVxlan.IsUnknown() {
		v := m.VrfVxlan.ValueInt64()
		body.VrfVxlan = &v
	}
	return body
}

// sdnZoneEvpnApply writes a fetched configuration into the model; absent
// settings become null.
func sdnZoneEvpnApply(s *pveclient.SdnZone, m *pveSdnZoneEvpnResourceModel) {
	sdnZoneCommonApply(s, &m.sdnZoneCommonModel)
	m.Controller = nodeNetworkStringToTF(s.Controller)
	m.SecondaryControllers = listStringToTF(s.SecondaryControllers)
	m.AdvertiseSubnets = nodeNetworkBoolPtrToTF(s.AdvertiseSubnets)
	m.DisableArpNdSuppression = nodeNetworkBoolPtrToTF(s.DisableArpNdSuppression)
	m.ExitNodes = nodeNetworkStringToTF(s.ExitNodes)
	m.ExitNodesLocalRouting = nodeNetworkBoolPtrToTF(s.ExitNodesLocalRouting)
	m.ExitNodesPrimary = nodeNetworkStringToTF(s.ExitNodesPrimary)
	m.Mac = nodeNetworkStringToTF(s.Mac)
	m.RtImport = nodeNetworkStringToTF(s.RtImport)
	m.VrfVxlan = haInt64PtrToTF(s.VrfVxlan)
}

// sdnZoneEvpnDeleteFields returns the PVE wire field names to clear on
// update: attributes present in state but null in the plan.
func sdnZoneEvpnDeleteFields(plan, state pveSdnZoneEvpnResourceModel) []string {
	var out []string
	if nodeNetworkStringCleared(plan.Controller, state.Controller) {
		out = append(out, "controller")
	}
	if sdnZoneListCleared(plan.SecondaryControllers, state.SecondaryControllers) {
		out = append(out, "secondary-controllers")
	}
	if sdnZoneBoolCleared(plan.AdvertiseSubnets, state.AdvertiseSubnets) {
		out = append(out, "advertise-subnets")
	}
	if sdnZoneBoolCleared(plan.DisableArpNdSuppression, state.DisableArpNdSuppression) {
		out = append(out, "disable-arp-nd-suppression")
	}
	if nodeNetworkStringCleared(plan.ExitNodes, state.ExitNodes) {
		out = append(out, "exitnodes")
	}
	if sdnZoneBoolCleared(plan.ExitNodesLocalRouting, state.ExitNodesLocalRouting) {
		out = append(out, "exitnodes-local-routing")
	}
	if nodeNetworkStringCleared(plan.ExitNodesPrimary, state.ExitNodesPrimary) {
		out = append(out, "exitnodes-primary")
	}
	if nodeNetworkStringCleared(plan.Mac, state.Mac) {
		out = append(out, "mac")
	}
	if nodeNetworkStringCleared(plan.RtImport, state.RtImport) {
		out = append(out, "rt-import")
	}
	if nodeNetworkIntCleared(plan.VrfVxlan, state.VrfVxlan) {
		out = append(out, "vrf-vxlan")
	}
	return out
}
