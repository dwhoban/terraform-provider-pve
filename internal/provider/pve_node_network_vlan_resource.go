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

	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ resource.Resource                = &pveNodeNetworkVlanResource{}
	_ resource.ResourceWithConfigure   = &pveNodeNetworkVlanResource{}
	_ resource.ResourceWithImportState = &pveNodeNetworkVlanResource{}
)

// NewPveNodeNetworkVlanResource returns the resource implementation.
func NewPveNodeNetworkVlanResource() resource.Resource {
	return &pveNodeNetworkVlanResource{}
}

// pveNodeNetworkVlanResource manages a VLAN interface (type `vlan`) on a PVE
// node via /nodes/{node}/network.
type pveNodeNetworkVlanResource struct {
	client *pveclient.Client
}

// pveNodeNetworkVlanResourceModel is the Terraform-facing shape.
type pveNodeNetworkVlanResourceModel struct {
	nodeNetworkCommonModel
	VLANID        types.Int64  `tfsdk:"vlan_id"`
	VLANRawDevice types.String `tfsdk:"vlan_raw_device"`
}

// Metadata implements resource.Resource.
func (r *pveNodeNetworkVlanResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveNodeNetworkVlan
}

// Schema implements resource.Resource.
func (r *pveNodeNetworkVlanResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := nodeNetworkCommonAttributes()
	attrs["vlan_id"] = schema.Int64Attribute{
		Required:            true,
		MarkdownDescription: "VLAN tag. Must be between 1 and 4094 inclusive (IEEE 802.1Q range).",
		Validators: []validator.Int64{
			int64validator.Between(1, 4094),
		},
	}
	attrs["vlan_raw_device"] = schema.StringAttribute{
		Required:            true,
		MarkdownDescription: "Underlying device the VLAN interface sits on (e.g. a bond or bridge name).",
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a VLAN network interface (`type=vlan`) on a Proxmox VE node (`/nodes/{node}/network`). Every mutation applies the node's pending network configuration and waits for the apply task.",
		Attributes:          attrs,
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveNodeNetworkVlanResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = nodeNetworkConfigureResource(req, resp)
}

// Create implements resource.Resource.
func (r *pveNodeNetworkVlanResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveNodeNetworkVlanResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := nodeNetworkVlanFromModel(plan)
	if err := r.client.CreateNodeNetwork(ctx, plan.Node.ValueString(), body); err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_node_network_vlan",
			fmt.Sprintf("creating vlan %s on node %s: %s", plan.Iface.ValueString(), plan.Node.ValueString(), err),
		)
		return
	}
	if err := nodeNetworkApply(ctx, r.client, plan.Node.ValueString()); err != nil {
		if rerr := r.client.RevertNodeNetwork(ctx, plan.Node.ValueString()); rerr != nil {
			err = fmt.Errorf("%w (revert of pending network changes also failed: %s)", err, rerr)
		}
		resp.Diagnostics.AddError(
			"Error applying pve_node_network_vlan create",
			fmt.Sprintf("vlan %s on node %s: %s (pending changes were reverted)", plan.Iface.ValueString(), plan.Node.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_node_network_vlan after create",
			fmt.Sprintf("reading vlan %s on node %s: %s", plan.Iface.ValueString(), plan.Node.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read implements resource.Resource.
func (r *pveNodeNetworkVlanResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveNodeNetworkVlanResourceModel
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
			"Error reading pve_node_network_vlan",
			fmt.Sprintf("reading vlan %s on node %s: %s", state.Iface.ValueString(), state.Node.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource.
func (r *pveNodeNetworkVlanResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveNodeNetworkVlanResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state pveNodeNetworkVlanResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := nodeNetworkVlanFromModel(plan)
	deleteFields := nodeNetworkCommonDeleteFields(plan.nodeNetworkCommonModel, state.nodeNetworkCommonModel)
	if err := r.client.UpdateNodeNetwork(ctx, plan.Node.ValueString(), plan.Iface.ValueString(), body, deleteFields); err != nil {
		resp.Diagnostics.AddError(
			"Error updating pve_node_network_vlan",
			fmt.Sprintf("updating vlan %s on node %s: %s", plan.Iface.ValueString(), plan.Node.ValueString(), err),
		)
		return
	}
	if err := nodeNetworkApply(ctx, r.client, plan.Node.ValueString()); err != nil {
		resp.Diagnostics.AddError(
			"Error applying pve_node_network_vlan update",
			fmt.Sprintf("vlan %s on node %s: %s", plan.Iface.ValueString(), plan.Node.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_node_network_vlan after update",
			fmt.Sprintf("reading vlan %s on node %s: %s", plan.Iface.ValueString(), plan.Node.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete implements resource.Resource.
func (r *pveNodeNetworkVlanResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveNodeNetworkVlanResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteNodeNetwork(ctx, state.Node.ValueString(), state.Iface.ValueString()); err != nil {
		if isPVEClientNotFound(err) {
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting pve_node_network_vlan",
			fmt.Sprintf("deleting vlan %s on node %s: %s", state.Iface.ValueString(), state.Node.ValueString(), err),
		)
		return
	}
	if err := nodeNetworkApply(ctx, r.client, state.Node.ValueString()); err != nil {
		resp.Diagnostics.AddError(
			"Error applying pve_node_network_vlan delete",
			fmt.Sprintf("vlan %s on node %s: %s", state.Iface.ValueString(), state.Node.ValueString(), err),
		)
	}
}

// ImportState parses an import ID of the form `<node>:<iface>`.
func (r *pveNodeNetworkVlanResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	node, iface, err := nodeNetworkParseImportID(req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Invalid pve_node_network_vlan import ID", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("node"), node)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("iface"), iface)...)
}

// nodeNetworkVlanFromModel projects the Terraform model into the wire body.
func nodeNetworkVlanFromModel(m pveNodeNetworkVlanResourceModel) pveclient.NetworkInterface {
	body := nodeNetworkCommonFromModel(m.nodeNetworkCommonModel, "vlan")
	if !m.VLANID.IsNull() && !m.VLANID.IsUnknown() {
		body.VLANID = int(m.VLANID.ValueInt64())
	}
	if !m.VLANRawDevice.IsNull() && !m.VLANRawDevice.IsUnknown() {
		body.VLANRawDevice = m.VLANRawDevice.ValueString()
	}
	return body
}

// readInto populates the model from PVE; an upstream type mismatch is an
// error rather than a silent adoption.
func (r *pveNodeNetworkVlanResource) readInto(ctx context.Context, m *pveNodeNetworkVlanResourceModel) error {
	iface, err := nodeNetworkGetChecked(ctx, r.client, m.Node.ValueString(), m.Iface.ValueString(), "vlan")
	if err != nil {
		return err
	}
	nodeNetworkCommonIntoModel(&m.nodeNetworkCommonModel, iface)
	m.VLANID = nodeNetworkIntToTF(iface.VLANID)
	m.VLANRawDevice = nodeNetworkStringToTF(iface.VLANRawDevice)
	return nil
}
