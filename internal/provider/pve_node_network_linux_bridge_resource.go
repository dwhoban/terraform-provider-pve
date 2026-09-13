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

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ resource.Resource                = &pveNodeNetworkLinuxBridgeResource{}
	_ resource.ResourceWithConfigure   = &pveNodeNetworkLinuxBridgeResource{}
	_ resource.ResourceWithImportState = &pveNodeNetworkLinuxBridgeResource{}
)

// NewPveNodeNetworkLinuxBridgeResource returns the resource implementation.
func NewPveNodeNetworkLinuxBridgeResource() resource.Resource {
	return &pveNodeNetworkLinuxBridgeResource{}
}

// pveNodeNetworkLinuxBridgeResource manages a Linux bridge interface
// (type `bridge`) on a PVE node via /nodes/{node}/network.
type pveNodeNetworkLinuxBridgeResource struct {
	client *pveclient.Client
}

// pveNodeNetworkLinuxBridgeResourceModel is the Terraform-facing shape.
type pveNodeNetworkLinuxBridgeResourceModel struct {
	nodeNetworkCommonModel
	BridgePorts     types.String `tfsdk:"bridge_ports"`
	BridgeVIDs      types.String `tfsdk:"bridge_vids"`
	BridgeVLANAware types.Bool   `tfsdk:"bridge_vlan_aware"`
	BridgeSTP       types.Bool   `tfsdk:"bridge_stp"`
	BridgeFD        types.Int64  `tfsdk:"bridge_fd"`
}

// Metadata implements resource.Resource.
func (r *pveNodeNetworkLinuxBridgeResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveNodeNetworkLinuxBridge
}

// Schema implements resource.Resource.
func (r *pveNodeNetworkLinuxBridgeResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := nodeNetworkCommonAttributes()
	attrs["bridge_ports"] = schema.StringAttribute{
		Optional:            true,
		MarkdownDescription: "Comma-separated list of member interfaces attached to the bridge (PVE `pve-iface-list` format).",
	}
	attrs["bridge_vids"] = schema.StringAttribute{
		Optional:            true,
		MarkdownDescription: "Allowed VLANs, e.g. `2 4 100-200`. Only used when `bridge_vlan_aware` is enabled.",
	}
	attrs["bridge_vlan_aware"] = schema.BoolAttribute{
		Optional:            true,
		MarkdownDescription: "Enable 802.1Q VLAN awareness on the bridge.",
	}
	attrs["bridge_stp"] = schema.BoolAttribute{
		Optional:            true,
		MarkdownDescription: "Enable spanning tree protocol on the bridge.",
	}
	attrs["bridge_fd"] = schema.Int64Attribute{
		Optional:            true,
		MarkdownDescription: "Bridge forward delay in seconds. Must be between 0 and 15 inclusive.",
		Validators: []validator.Int64{
			int64validator.Between(0, 15),
		},
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Linux bridge network interface (`type=bridge`) on a Proxmox VE node (`/nodes/{node}/network`). Every mutation applies the node's pending network configuration and waits for the apply task.",
		Attributes:          attrs,
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveNodeNetworkLinuxBridgeResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = nodeNetworkConfigureResource(req, resp)
}

// Create implements resource.Resource.
func (r *pveNodeNetworkLinuxBridgeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveNodeNetworkLinuxBridgeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := nodeNetworkBridgeFromModel(plan)
	if err := r.client.CreateNodeNetwork(ctx, plan.Node.ValueString(), body); err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_node_network_linux_bridge",
			fmt.Sprintf("creating bridge %s on node %s: %s", plan.Iface.ValueString(), plan.Node.ValueString(), err),
		)
		return
	}
	if err := nodeNetworkApply(ctx, r.client, plan.Node.ValueString()); err != nil {
		if rerr := r.client.RevertNodeNetwork(ctx, plan.Node.ValueString()); rerr != nil {
			err = fmt.Errorf("%w (revert of pending network changes also failed: %s)", err, rerr)
		}
		resp.Diagnostics.AddError(
			"Error applying pve_node_network_linux_bridge create",
			fmt.Sprintf("bridge %s on node %s: %s (pending changes were reverted)", plan.Iface.ValueString(), plan.Node.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_node_network_linux_bridge after create",
			fmt.Sprintf("reading bridge %s on node %s: %s", plan.Iface.ValueString(), plan.Node.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read implements resource.Resource.
func (r *pveNodeNetworkLinuxBridgeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveNodeNetworkLinuxBridgeResourceModel
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
			"Error reading pve_node_network_linux_bridge",
			fmt.Sprintf("reading bridge %s on node %s: %s", state.Iface.ValueString(), state.Node.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource.
func (r *pveNodeNetworkLinuxBridgeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveNodeNetworkLinuxBridgeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state pveNodeNetworkLinuxBridgeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := nodeNetworkBridgeFromModel(plan)
	deleteFields := append(
		nodeNetworkCommonDeleteFields(plan.nodeNetworkCommonModel, state.nodeNetworkCommonModel),
		nodeNetworkBridgeDeleteFields(plan, state)...,
	)
	if err := r.client.UpdateNodeNetwork(ctx, plan.Node.ValueString(), plan.Iface.ValueString(), body, deleteFields); err != nil {
		resp.Diagnostics.AddError(
			"Error updating pve_node_network_linux_bridge",
			fmt.Sprintf("updating bridge %s on node %s: %s", plan.Iface.ValueString(), plan.Node.ValueString(), err),
		)
		return
	}
	if err := nodeNetworkApply(ctx, r.client, plan.Node.ValueString()); err != nil {
		resp.Diagnostics.AddError(
			"Error applying pve_node_network_linux_bridge update",
			fmt.Sprintf("bridge %s on node %s: %s", plan.Iface.ValueString(), plan.Node.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_node_network_linux_bridge after update",
			fmt.Sprintf("reading bridge %s on node %s: %s", plan.Iface.ValueString(), plan.Node.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete implements resource.Resource.
func (r *pveNodeNetworkLinuxBridgeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveNodeNetworkLinuxBridgeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteNodeNetwork(ctx, state.Node.ValueString(), state.Iface.ValueString()); err != nil {
		if isPVEClientNotFound(err) {
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting pve_node_network_linux_bridge",
			fmt.Sprintf("deleting bridge %s on node %s: %s", state.Iface.ValueString(), state.Node.ValueString(), err),
		)
		return
	}
	if err := nodeNetworkApply(ctx, r.client, state.Node.ValueString()); err != nil {
		resp.Diagnostics.AddError(
			"Error applying pve_node_network_linux_bridge delete",
			fmt.Sprintf("bridge %s on node %s: %s", state.Iface.ValueString(), state.Node.ValueString(), err),
		)
	}
}

// ImportState parses an import ID of the form `<node>:<iface>`.
func (r *pveNodeNetworkLinuxBridgeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	node, iface, err := nodeNetworkParseImportID(req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Invalid pve_node_network_linux_bridge import ID", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("node"), node)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("iface"), iface)...)
}

// nodeNetworkBridgeFromModel projects the Terraform model into the wire body.
func nodeNetworkBridgeFromModel(m pveNodeNetworkLinuxBridgeResourceModel) pveclient.NetworkInterface {
	body := nodeNetworkCommonFromModel(m.nodeNetworkCommonModel, "bridge")
	if !m.BridgePorts.IsNull() && !m.BridgePorts.IsUnknown() {
		body.BridgePorts = m.BridgePorts.ValueString()
	}
	if !m.BridgeVIDs.IsNull() && !m.BridgeVIDs.IsUnknown() {
		body.BridgeVIDs = m.BridgeVIDs.ValueString()
	}
	if !m.BridgeVLANAware.IsNull() && !m.BridgeVLANAware.IsUnknown() {
		v := m.BridgeVLANAware.ValueBool()
		body.BridgeVLANAware = &v
	}
	if !m.BridgeSTP.IsNull() && !m.BridgeSTP.IsUnknown() {
		v := m.BridgeSTP.ValueBool()
		body.BridgeSTP = &v
	}
	if !m.BridgeFD.IsNull() && !m.BridgeFD.IsUnknown() {
		body.BridgeFD = int(m.BridgeFD.ValueInt64())
	}
	return body
}

// nodeNetworkBridgeDeleteFields returns bridge-specific field names to clear
// on update: attributes present in state but null in plan.
func nodeNetworkBridgeDeleteFields(plan, state pveNodeNetworkLinuxBridgeResourceModel) []string {
	var out []string
	if nodeNetworkStringCleared(plan.BridgePorts, state.BridgePorts) {
		out = append(out, "bridge_ports")
	}
	if nodeNetworkStringCleared(plan.BridgeVIDs, state.BridgeVIDs) {
		out = append(out, "bridge_vids")
	}
	if plan.BridgeVLANAware.IsNull() && !state.BridgeVLANAware.IsNull() {
		out = append(out, "bridge_vlan_aware")
	}
	if plan.BridgeSTP.IsNull() && !state.BridgeSTP.IsNull() {
		out = append(out, "bridge_stp")
	}
	if nodeNetworkIntCleared(plan.BridgeFD, state.BridgeFD) {
		out = append(out, "bridge_fd")
	}
	return out
}

// readInto populates the model from PVE; an upstream type mismatch is an
// error rather than a silent adoption.
func (r *pveNodeNetworkLinuxBridgeResource) readInto(ctx context.Context, m *pveNodeNetworkLinuxBridgeResourceModel) error {
	iface, err := nodeNetworkGetChecked(ctx, r.client, m.Node.ValueString(), m.Iface.ValueString(), "bridge")
	if err != nil {
		return err
	}
	nodeNetworkCommonIntoModel(&m.nodeNetworkCommonModel, iface)
	m.BridgePorts = nodeNetworkStringToTF(iface.BridgePorts)
	m.BridgeVIDs = nodeNetworkStringToTF(iface.BridgeVIDs)
	m.BridgeVLANAware = nodeNetworkBoolPtrToTF(iface.BridgeVLANAware)
	m.BridgeSTP = nodeNetworkBoolPtrToTF(iface.BridgeSTP)
	m.BridgeFD = nodeNetworkIntToTF(iface.BridgeFD)
	return nil
}
