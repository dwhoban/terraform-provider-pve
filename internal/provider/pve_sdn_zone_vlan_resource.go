// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ resource.Resource                = &pveSdnZoneVlanResource{}
	_ resource.ResourceWithConfigure   = &pveSdnZoneVlanResource{}
	_ resource.ResourceWithImportState = &pveSdnZoneVlanResource{}
)

// NewPveSdnZoneVlanResource returns the resource implementation.
func NewPveSdnZoneVlanResource() resource.Resource {
	return &pveSdnZoneVlanResource{}
}

// pveSdnZoneVlanResource manages one VLAN SDN zone (POST/GET/PUT/DELETE on
// /cluster/sdn/zones with type `vlan`).
type pveSdnZoneVlanResource struct {
	client *pveclient.Client
}

// pveSdnZoneVlanResourceModel is the Terraform-facing shape.
type pveSdnZoneVlanResourceModel struct {
	sdnZoneCommonModel
	Bridge                   types.String `tfsdk:"bridge"`
	BridgeDisableMacLearning types.Bool   `tfsdk:"bridge_disable_mac_learning"`
}

// Metadata implements resource.Resource.
func (r *pveSdnZoneVlanResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveSdnZoneVlan
}

// Schema implements resource.Resource.
func (r *pveSdnZoneVlanResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := sdnZoneCommonAttributes()
	attrs["bridge"] = schema.StringAttribute{
		Optional:            true,
		MarkdownDescription: "The local bridge (PVE `bridge`) on each node where the VLANs for this zone are managed, for example `vmbr0`.",
	}
	attrs["bridge_disable_mac_learning"] = schema.BoolAttribute{
		Optional:            true,
		MarkdownDescription: "Disable auto MAC learning (PVE `bridge-disable-mac-learning`) on the zone bridge.",
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a VLAN SDN zone (`/cluster/sdn/zones`, type `vlan`) using IEEE 802.1Q VLAN tags carried on vnets. The per-vnet tag lives on `pve_sdn_vnet`; the zone only chooses the bridge. Creating, updating, or deleting a zone only changes the pending SDN configuration; run the `pve_sdn_apply` action to apply it to the running configuration. Changing `zone` forces recreation.",
		Attributes:          attrs,
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveSdnZoneVlanResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = haConfigureResource(req, resp)
}

// Create implements resource.Resource.
func (r *pveSdnZoneVlanResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveSdnZoneVlanResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error creating pve_sdn_zone_vlan", "provider client is not configured")
		return
	}
	if err := r.client.CreateSdnZone(ctx, sdnZoneVlanFromModel(plan)); err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_sdn_zone_vlan",
			fmt.Sprintf("creating zone %s: %s", plan.Zone.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_sdn_zone_vlan after create",
			fmt.Sprintf("reading zone %s: %s", plan.Zone.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	tflog.Debug(ctx, "created PVE vlan sdn zone", map[string]any{"zone": plan.Zone.ValueString()})
}

// Read implements resource.Resource.
func (r *pveSdnZoneVlanResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveSdnZoneVlanResourceModel
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
			"Error reading pve_sdn_zone_vlan",
			fmt.Sprintf("reading zone %s: %s", state.Zone.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource. Changed fields travel in the PUT
// body; attributes cleared in the plan travel in the `delete` query
// parameter.
func (r *pveSdnZoneVlanResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveSdnZoneVlanResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state pveSdnZoneVlanResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	deleteFields := append(
		sdnZoneCommonDeleteFields(plan.sdnZoneCommonModel, state.sdnZoneCommonModel),
		sdnZoneVlanDeleteFields(plan, state)...,
	)
	if err := r.client.UpdateSdnZone(ctx, plan.Zone.ValueString(), sdnZoneVlanFromModel(plan), deleteFields); err != nil {
		resp.Diagnostics.AddError(
			"Error updating pve_sdn_zone_vlan",
			fmt.Sprintf("updating zone %s: %s", plan.Zone.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_sdn_zone_vlan after update",
			fmt.Sprintf("reading zone %s: %s", plan.Zone.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	tflog.Debug(ctx, "updated PVE vlan sdn zone", map[string]any{"zone": plan.Zone.ValueString()})
}

// Delete implements resource.Resource.
func (r *pveSdnZoneVlanResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveSdnZoneVlanResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	tflog.Debug(ctx, "deleting PVE vlan sdn zone", map[string]any{"zone": state.Zone.ValueString()})
	if err := r.client.DeleteSdnZone(ctx, state.Zone.ValueString()); err != nil {
		if isPVEClientNotFound(err) {
			// Already absent counts as deleted.
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting pve_sdn_zone_vlan",
			fmt.Sprintf("deleting zone %s: %s", state.Zone.ValueString(), err),
		)
	}
}

// ImportState parses an import ID of the form `<zone>`.
func (r *pveSdnZoneVlanResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		resp.Diagnostics.AddError("Invalid pve_sdn_zone_vlan import ID", "import ID must be the zone identifier, e.g. `zone1`")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("zone"), req.ID)...)
}

// readInto refreshes the model from PVE, erroring when the upstream type
// is no longer vlan.
func (r *pveSdnZoneVlanResource) readInto(ctx context.Context, m *pveSdnZoneVlanResourceModel) error {
	z, err := sdnZoneGetChecked(ctx, r.client, m.Zone.ValueString(), "vlan")
	if err != nil {
		return err
	}
	sdnZoneVlanApply(z, m)
	return nil
}

// sdnZoneVlanFromModel projects the Terraform model into the wire body;
// null values are omitted from the request.
func sdnZoneVlanFromModel(m pveSdnZoneVlanResourceModel) pveclient.SdnZone {
	body := sdnZoneCommonFromModel(m.sdnZoneCommonModel)
	body.Type = "vlan"
	if !m.Bridge.IsNull() && !m.Bridge.IsUnknown() {
		body.Bridge = m.Bridge.ValueString()
	}
	if !m.BridgeDisableMacLearning.IsNull() && !m.BridgeDisableMacLearning.IsUnknown() {
		v := m.BridgeDisableMacLearning.ValueBool()
		body.BridgeDisableMacLearning = &v
	}
	return body
}

// sdnZoneVlanApply writes a fetched configuration into the model; absent
// settings become null.
func sdnZoneVlanApply(s *pveclient.SdnZone, m *pveSdnZoneVlanResourceModel) {
	sdnZoneCommonApply(s, &m.sdnZoneCommonModel)
	m.Bridge = nodeNetworkStringToTF(s.Bridge)
	m.BridgeDisableMacLearning = nodeNetworkBoolPtrToTF(s.BridgeDisableMacLearning)
}

// sdnZoneVlanDeleteFields returns the PVE wire field names to clear on
// update: attributes present in state but null in the plan.
func sdnZoneVlanDeleteFields(plan, state pveSdnZoneVlanResourceModel) []string {
	var out []string
	if nodeNetworkStringCleared(plan.Bridge, state.Bridge) {
		out = append(out, "bridge")
	}
	if sdnZoneBoolCleared(plan.BridgeDisableMacLearning, state.BridgeDisableMacLearning) {
		out = append(out, "bridge-disable-mac-learning")
	}
	return out
}
