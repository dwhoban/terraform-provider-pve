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

	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ resource.Resource                = &pveSdnFirewallOptionsResource{}
	_ resource.ResourceWithConfigure   = &pveSdnFirewallOptionsResource{}
	_ resource.ResourceWithImportState = &pveSdnFirewallOptionsResource{}
)

// NewPveSdnFirewallOptionsResource returns the resource implementation.
func NewPveSdnFirewallOptionsResource() resource.Resource {
	return &pveSdnFirewallOptionsResource{}
}

// pveSdnFirewallOptionsResource manages the firewall options singleton of
// one SDN vnet via GET/PUT /cluster/sdn/vnets/{vnet}/firewall/options.
type pveSdnFirewallOptionsResource struct {
	client *pveclient.Client
}

// pveSdnFirewallOptionsOptionSet carries the option fields shared by the
// resource and data source models; both embed it with its tfsdk tags.
type pveSdnFirewallOptionsOptionSet struct {
	Enable          types.Bool   `tfsdk:"enable"`
	LogLevelForward types.String `tfsdk:"log_level_forward"`
	PolicyForward   types.String `tfsdk:"policy_forward"`
}

// pveSdnFirewallOptionsResourceModel is the Terraform-facing shape of the
// resource.
type pveSdnFirewallOptionsResourceModel struct {
	VNet types.String `tfsdk:"vnet"`
	pveSdnFirewallOptionsOptionSet
	ID types.String `tfsdk:"id"`
}

// sdnFirewallOptionsFieldSpecs is the vnet firewall options attribute set,
// in wire order, transcribed from the api-spec pin
// (GET/PUT /cluster/sdn/vnets/{vnet}/firewall/options).
var sdnFirewallOptionsFieldSpecs = []clusterOptionsField{
	{Name: "enable", Wire: "enable", Kind: clusterOptionsKindBool, Description: "Enable/disable firewall rules."},
	{Name: "log_level_forward", Wire: "log_level_forward", Kind: clusterOptionsKindString, Enum: firewallOptionsLogLevelValues, Description: "Log level for forwarded traffic."},
	{Name: "policy_forward", Wire: "policy_forward", Kind: clusterOptionsKindString, Enum: firewallOptionsPolicyForwardValues, Description: "Forward policy."},
}

// sdnFirewallOptionsResourceAttributes renders the full resource attribute
// set.
func sdnFirewallOptionsResourceAttributes() map[string]schema.Attribute {
	attrs := make(map[string]schema.Attribute, len(sdnFirewallOptionsFieldSpecs)+2)
	for _, f := range sdnFirewallOptionsFieldSpecs {
		attrs[f.Name] = clusterOptionsResourceLeaf(f)
	}
	attrs["vnet"] = schema.StringAttribute{
		Required:            true,
		MarkdownDescription: "The SDN vnet object identifier. Changing this value forces recreation.",
		PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
	}
	attrs["id"] = schema.StringAttribute{
		Computed:            true,
		MarkdownDescription: "Identifier of the vnet firewall options singleton; equals the `vnet` name.",
	}
	return attrs
}

// Metadata implements resource.Resource.
func (r *pveSdnFirewallOptionsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveSdnFirewallOptions
}

// Schema implements resource.Resource.
func (r *pveSdnFirewallOptionsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the firewall options singleton of one SDN vnet (`GET/PUT /cluster/sdn/vnets/{vnet}/firewall/options`). Every listed attribute is managed: removing an attribute from configuration clears the option via the `delete` parameter. The pin defines no delete verb for this scope, so destroy only forgets the state. Requires `SDN.Allocate` on `/sdn/zones/<zone>/<vnet>`.",
		Attributes:          sdnFirewallOptionsResourceAttributes(),
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveSdnFirewallOptionsResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*pveclient.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *pveclient.Client, got: %T.", req.ProviderData),
		)
		return
	}
	r.client = client
}

// Create implements resource.Resource. A singleton has no upstream create
// verb; writing the planned options is the whole operation.
func (r *pveSdnFirewallOptionsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveSdnFirewallOptionsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error creating pve_sdn_firewall_options", "provider client is not configured")
		return
	}
	if err := r.client.UpdateVNetFirewallOptions(ctx, plan.VNet.ValueString(), sdnFirewallOptionsFromModel(&plan.pveSdnFirewallOptionsOptionSet), nil); err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_sdn_firewall_options",
			fmt.Sprintf("writing firewall options of vnet %s: %s", plan.VNet.ValueString(), err),
		)
		return
	}
	if err := sdnFirewallOptionsReadInto(ctx, r.client, plan.VNet.ValueString(), &plan.pveSdnFirewallOptionsOptionSet); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_sdn_firewall_options after create",
			fmt.Sprintf("reading firewall options of vnet %s: %s", plan.VNet.ValueString(), err),
		)
		return
	}
	plan.ID = plan.VNet
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read implements resource.Resource.
func (r *pveSdnFirewallOptionsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveSdnFirewallOptionsResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := sdnFirewallOptionsReadInto(ctx, r.client, state.VNet.ValueString(), &state.pveSdnFirewallOptionsOptionSet); err != nil {
		if isPVEClientNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error reading pve_sdn_firewall_options",
			fmt.Sprintf("reading firewall options of vnet %s: %s", state.VNet.ValueString(), err),
		)
		return
	}
	state.ID = state.VNet
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource. Set fields travel in the PUT body;
// fields cleared in the plan travel in the `delete` query parameter.
func (r *pveSdnFirewallOptionsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveSdnFirewallOptionsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state pveSdnFirewallOptionsResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	deleteFields := sdnFirewallOptionsDeleteFields(&plan.pveSdnFirewallOptionsOptionSet, &state.pveSdnFirewallOptionsOptionSet)
	if err := r.client.UpdateVNetFirewallOptions(ctx, plan.VNet.ValueString(), sdnFirewallOptionsFromModel(&plan.pveSdnFirewallOptionsOptionSet), deleteFields); err != nil {
		resp.Diagnostics.AddError(
			"Error updating pve_sdn_firewall_options",
			fmt.Sprintf("updating firewall options of vnet %s: %s", plan.VNet.ValueString(), err),
		)
		return
	}
	if err := sdnFirewallOptionsReadInto(ctx, r.client, plan.VNet.ValueString(), &plan.pveSdnFirewallOptionsOptionSet); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_sdn_firewall_options after update",
			fmt.Sprintf("reading firewall options of vnet %s: %s", plan.VNet.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete implements resource.Resource. The pin defines no delete verb for
// the vnet firewall options, so destroy only forgets the state.
func (r *pveSdnFirewallOptionsResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

// ImportState parses an import ID of the form `<vnet>`.
func (r *pveSdnFirewallOptionsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		resp.Diagnostics.AddError("Invalid pve_sdn_firewall_options import ID", "import ID must be the vnet identifier, e.g. `vnet1`")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("vnet"), req.ID)...)
}

// sdnFirewallOptionsReadInto refreshes the option set from the vnet.
func sdnFirewallOptionsReadInto(ctx context.Context, client *pveclient.Client, vnet string, m *pveSdnFirewallOptionsOptionSet) error {
	opts, err := client.GetVNetFirewallOptions(ctx, vnet)
	if err != nil {
		return err
	}
	sdnFirewallOptionsApply(m, opts)
	return nil
}

// sdnFirewallOptionsFromModel projects the Terraform model into the wire
// struct; null values become nil pointers and are omitted from the PUT body.
func sdnFirewallOptionsFromModel(m *pveSdnFirewallOptionsOptionSet) pveclient.FirewallOptions {
	return pveclient.FirewallOptions{
		Enable:          firewallOptionsBoolPtr(m.Enable),
		LogLevelForward: firewallOptionsStrPtr(m.LogLevelForward),
		PolicyForward:   firewallOptionsStrPtr(m.PolicyForward),
	}
}

// sdnFirewallOptionsApply writes fetched options into the model; absent
// options become null.
func sdnFirewallOptionsApply(m *pveSdnFirewallOptionsOptionSet, o *pveclient.FirewallOptions) {
	m.Enable = nodeNetworkBoolPtrToTF(o.Enable)
	m.LogLevelForward = firewallOptionsStringValue(o.LogLevelForward)
	m.PolicyForward = firewallOptionsStringValue(o.PolicyForward)
}

// sdnFirewallOptionsDeleteFields returns the wire names of options present
// in state but cleared in the plan.
func sdnFirewallOptionsDeleteFields(plan, state *pveSdnFirewallOptionsOptionSet) []string {
	var out []string
	if plan.Enable.IsNull() && !state.Enable.IsNull() {
		out = append(out, "enable")
	}
	if plan.LogLevelForward.IsNull() && !state.LogLevelForward.IsNull() {
		out = append(out, "log_level_forward")
	}
	if plan.PolicyForward.IsNull() && !state.PolicyForward.IsNull() {
		out = append(out, "policy_forward")
	}
	return out
}
