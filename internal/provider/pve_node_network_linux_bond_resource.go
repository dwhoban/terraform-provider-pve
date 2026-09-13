// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ resource.Resource                = &pveNodeNetworkLinuxBondResource{}
	_ resource.ResourceWithConfigure   = &pveNodeNetworkLinuxBondResource{}
	_ resource.ResourceWithImportState = &pveNodeNetworkLinuxBondResource{}
)

// NewPveNodeNetworkLinuxBondResource returns the resource implementation.
func NewPveNodeNetworkLinuxBondResource() resource.Resource {
	return &pveNodeNetworkLinuxBondResource{}
}

// pveNodeNetworkLinuxBondResource manages a Linux bond interface
// (type `bond`) on a PVE node via /nodes/{node}/network.
type pveNodeNetworkLinuxBondResource struct {
	client *pveclient.Client
}

// pveNodeNetworkLinuxBondResourceModel is the Terraform-facing shape.
type pveNodeNetworkLinuxBondResourceModel struct {
	nodeNetworkCommonModel
	Slaves             types.List   `tfsdk:"slaves"`
	BondMode           types.String `tfsdk:"bond_mode"`
	BondPrimary        types.String `tfsdk:"bond_primary"`
	BondXmitHashPolicy types.String `tfsdk:"bond_xmit_hash_policy"`
}

// Metadata implements resource.Resource.
func (r *pveNodeNetworkLinuxBondResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveNodeNetworkLinuxBond
}

// Schema implements resource.Resource.
func (r *pveNodeNetworkLinuxBondResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := nodeNetworkCommonAttributes()
	attrs["slaves"] = schema.ListAttribute{
		Required:            true,
		ElementType:         types.StringType,
		MarkdownDescription: "Member (slave) interfaces of the bond.",
	}
	attrs["bond_mode"] = schema.StringAttribute{
		Required:            true,
		MarkdownDescription: "Linux kernel bonding driver mode. Must be one of: `balance-rr`, `active-backup`, `balance-xor`, `broadcast`, `802.3ad`, `balance-tlb`, `balance-alb`.",
		Validators: []validator.String{
			stringvalidator.OneOf(nodeNetworkBondModes...),
		},
	}
	attrs["bond_primary"] = schema.StringAttribute{
		Optional:            true,
		MarkdownDescription: "Primary slave interface (used by `active-backup`, `balance-tlb`, and `balance-alb` modes).",
	}
	attrs["bond_xmit_hash_policy"] = schema.StringAttribute{
		Optional:            true,
		MarkdownDescription: "Transmit hash policy for slave selection in `balance-xor` and `802.3ad` modes. Must be one of: `layer2`, `layer2+3`, `layer3+4`.",
		Validators: []validator.String{
			stringvalidator.OneOf(nodeNetworkBondXmitHashPolicies...),
		},
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Linux bond network interface (`type=bond`) on a Proxmox VE node (`/nodes/{node}/network`). Every mutation applies the node's pending network configuration and waits for the apply task.",
		Attributes:          attrs,
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveNodeNetworkLinuxBondResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = nodeNetworkConfigureResource(req, resp)
}

// Create implements resource.Resource.
func (r *pveNodeNetworkLinuxBondResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveNodeNetworkLinuxBondResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := nodeNetworkBondFromModel(plan)
	if err := r.client.CreateNodeNetwork(ctx, plan.Node.ValueString(), body); err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_node_network_linux_bond",
			fmt.Sprintf("creating bond %s on node %s: %s", plan.Iface.ValueString(), plan.Node.ValueString(), err),
		)
		return
	}
	if err := nodeNetworkApply(ctx, r.client, plan.Node.ValueString()); err != nil {
		if rerr := r.client.RevertNodeNetwork(ctx, plan.Node.ValueString()); rerr != nil {
			err = fmt.Errorf("%w (revert of pending network changes also failed: %s)", err, rerr)
		}
		resp.Diagnostics.AddError(
			"Error applying pve_node_network_linux_bond create",
			fmt.Sprintf("bond %s on node %s: %s (pending changes were reverted)", plan.Iface.ValueString(), plan.Node.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_node_network_linux_bond after create",
			fmt.Sprintf("reading bond %s on node %s: %s", plan.Iface.ValueString(), plan.Node.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read implements resource.Resource.
func (r *pveNodeNetworkLinuxBondResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveNodeNetworkLinuxBondResourceModel
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
			"Error reading pve_node_network_linux_bond",
			fmt.Sprintf("reading bond %s on node %s: %s", state.Iface.ValueString(), state.Node.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource.
func (r *pveNodeNetworkLinuxBondResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveNodeNetworkLinuxBondResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state pveNodeNetworkLinuxBondResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := nodeNetworkBondFromModel(plan)
	deleteFields := append(
		nodeNetworkCommonDeleteFields(plan.nodeNetworkCommonModel, state.nodeNetworkCommonModel),
		nodeNetworkBondDeleteFields(plan, state)...,
	)
	if err := r.client.UpdateNodeNetwork(ctx, plan.Node.ValueString(), plan.Iface.ValueString(), body, deleteFields); err != nil {
		resp.Diagnostics.AddError(
			"Error updating pve_node_network_linux_bond",
			fmt.Sprintf("updating bond %s on node %s: %s", plan.Iface.ValueString(), plan.Node.ValueString(), err),
		)
		return
	}
	if err := nodeNetworkApply(ctx, r.client, plan.Node.ValueString()); err != nil {
		resp.Diagnostics.AddError(
			"Error applying pve_node_network_linux_bond update",
			fmt.Sprintf("bond %s on node %s: %s", plan.Iface.ValueString(), plan.Node.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_node_network_linux_bond after update",
			fmt.Sprintf("reading bond %s on node %s: %s", plan.Iface.ValueString(), plan.Node.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete implements resource.Resource.
func (r *pveNodeNetworkLinuxBondResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveNodeNetworkLinuxBondResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteNodeNetwork(ctx, state.Node.ValueString(), state.Iface.ValueString()); err != nil {
		if isPVEClientNotFound(err) {
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting pve_node_network_linux_bond",
			fmt.Sprintf("deleting bond %s on node %s: %s", state.Iface.ValueString(), state.Node.ValueString(), err),
		)
		return
	}
	if err := nodeNetworkApply(ctx, r.client, state.Node.ValueString()); err != nil {
		resp.Diagnostics.AddError(
			"Error applying pve_node_network_linux_bond delete",
			fmt.Sprintf("bond %s on node %s: %s", state.Iface.ValueString(), state.Node.ValueString(), err),
		)
	}
}

// ImportState parses an import ID of the form `<node>:<iface>`.
func (r *pveNodeNetworkLinuxBondResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	node, iface, err := nodeNetworkParseImportID(req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Invalid pve_node_network_linux_bond import ID", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("node"), node)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("iface"), iface)...)
}

// nodeNetworkBondFromModel projects the Terraform model into the wire body.
func nodeNetworkBondFromModel(m pveNodeNetworkLinuxBondResourceModel) pveclient.NetworkInterface {
	body := nodeNetworkCommonFromModel(m.nodeNetworkCommonModel, "bond")
	if !m.Slaves.IsNull() && !m.Slaves.IsUnknown() {
		body.Slaves = listStringFromTF(m.Slaves)
	}
	if !m.BondMode.IsNull() && !m.BondMode.IsUnknown() {
		body.BondMode = m.BondMode.ValueString()
	}
	if !m.BondPrimary.IsNull() && !m.BondPrimary.IsUnknown() {
		body.BondPrimary = m.BondPrimary.ValueString()
	}
	if !m.BondXmitHashPolicy.IsNull() && !m.BondXmitHashPolicy.IsUnknown() {
		body.BondXmitHashPolicy = m.BondXmitHashPolicy.ValueString()
	}
	return body
}

// nodeNetworkBondDeleteFields returns bond-specific field names to clear on
// update: attributes present in state but null in plan.
func nodeNetworkBondDeleteFields(plan, state pveNodeNetworkLinuxBondResourceModel) []string {
	var out []string
	if nodeNetworkStringCleared(plan.BondPrimary, state.BondPrimary) {
		out = append(out, "bond_primary")
	}
	if nodeNetworkStringCleared(plan.BondXmitHashPolicy, state.BondXmitHashPolicy) {
		out = append(out, "bond_xmit_hash_policy")
	}
	return out
}

// readInto populates the model from PVE; an upstream type mismatch is an
// error rather than a silent adoption.
func (r *pveNodeNetworkLinuxBondResource) readInto(ctx context.Context, m *pveNodeNetworkLinuxBondResourceModel) error {
	iface, err := nodeNetworkGetChecked(ctx, r.client, m.Node.ValueString(), m.Iface.ValueString(), "bond")
	if err != nil {
		return err
	}
	nodeNetworkCommonIntoModel(&m.nodeNetworkCommonModel, iface)
	if len(iface.Slaves) > 0 {
		m.Slaves = listStringToTF(iface.Slaves)
	} else {
		m.Slaves = types.ListNull(types.StringType)
	}
	m.BondMode = nodeNetworkStringToTF(iface.BondMode)
	m.BondPrimary = nodeNetworkStringToTF(iface.BondPrimary)
	m.BondXmitHashPolicy = nodeNetworkStringToTF(iface.BondXmitHashPolicy)
	return nil
}
