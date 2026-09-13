// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ resource.Resource                = &pveGuestFirewallRulesResource{}
	_ resource.ResourceWithConfigure   = &pveGuestFirewallRulesResource{}
	_ resource.ResourceWithImportState = &pveGuestFirewallRulesResource{}
)

// NewPveGuestFirewallRulesResource returns the resource implementation.
func NewPveGuestFirewallRulesResource() resource.Resource {
	return &pveGuestFirewallRulesResource{}
}

// pveGuestFirewallRulesResource manages the ordered firewall ruleset of
// one guest via /nodes/{node}/{qemu,lxc}/{vmid}/firewall/rules.
type pveGuestFirewallRulesResource struct {
	client *pveclient.Client
}

// pveGuestFirewallRulesResourceModel is the Terraform-facing shape.
type pveGuestFirewallRulesResourceModel struct {
	ID        types.String             `tfsdk:"id"`
	Node      types.String             `tfsdk:"node"`
	GuestType types.String             `tfsdk:"guest_type"`
	VMID      types.Int64              `tfsdk:"vmid"`
	Rules     []firewallRulesRuleModel `tfsdk:"rules"`
}

// Metadata implements resource.Resource.
func (r *pveGuestFirewallRulesResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveGuestFirewallRules
}

// Schema implements resource.Resource.
func (r *pveGuestFirewallRulesResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the ordered firewall ruleset of one guest (`GET/POST /nodes/{node}/{qemu,lxc}/{vmid}/firewall/rules`, `GET/PUT/DELETE .../rules/{pos}`). The `rules` list in configuration is authoritative: applies converge PVE to exactly this list and order, removing or rewriting entries made out of band. PVE renumbers positions, so `pos` is computed and resynced after every apply.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Identifier of the ruleset; `<node>:<guest_type>:<vmid>`.",
			},
			"node": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The cluster node hosting the guest. Changing this value forces recreation.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"guest_type": schema.StringAttribute{
				Required: true,
				Validators: []validator.String{
					stringvalidator.OneOf("qemu", "lxc"),
				},
				MarkdownDescription: "Guest type: `qemu` for virtual machines, `lxc` for containers. Must be one of: `qemu`, `lxc`. Changing this value forces recreation.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"vmid": schema.Int64Attribute{
				Required: true,
				Validators: []validator.Int64{
					int64validator.Between(100, 999999999),
				},
				MarkdownDescription: "The guest ID. Must be between 100 and 999999999 inclusive. Changing this value forces recreation.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"rules": schema.ListNestedAttribute{
				Required:            true,
				MarkdownDescription: "The ordered firewall rules. Order is significant: PVE evaluates rules top to bottom and the first match wins.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: firewallRulesRuleAttributes(),
				},
			},
		},
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveGuestFirewallRulesResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = firewallRulesConfigureResource(req, resp)
}

// guestBasePath builds the ruleset path for the model's keys.
func (m pveGuestFirewallRulesResourceModel) guestBasePath() string {
	return pveclient.FirewallRulesPathGuest(m.Node.ValueString(), m.GuestType.ValueString(), m.VMID.ValueInt64())
}

// guestID renders the composite identifier.
func (m pveGuestFirewallRulesResourceModel) guestID() types.String {
	return types.StringValue(fmt.Sprintf("%s:%s:%d", m.Node.ValueString(), m.GuestType.ValueString(), m.VMID.ValueInt64()))
}

// Create implements resource.Resource.
func (r *pveGuestFirewallRulesResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveGuestFirewallRulesResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error creating pve_guest_firewall_rules", "provider client is not configured")
		return
	}
	fresh, err := firewallRulesApplyDiff(ctx, r.client, plan.guestBasePath(), firewallRulesFromModel(plan.Rules))
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_guest_firewall_rules",
			fmt.Sprintf("applying firewall rules of guest %s vmid %d: %s", plan.Node.ValueString(), plan.VMID.ValueInt64(), err),
		)
		return
	}
	plan.Rules = firewallRulesToModel(fresh)
	plan.ID = plan.guestID()
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read implements resource.Resource.
func (r *pveGuestFirewallRulesResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveGuestFirewallRulesResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := firewallRulesReadInto(ctx, r.client, state.guestBasePath(), &state.Rules); err != nil {
		if isPVEClientNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error reading pve_guest_firewall_rules",
			fmt.Sprintf("reading firewall rules of guest %s vmid %d: %s", state.Node.ValueString(), state.VMID.ValueInt64(), err),
		)
		return
	}
	state.ID = state.guestID()
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource.
func (r *pveGuestFirewallRulesResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveGuestFirewallRulesResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	fresh, err := firewallRulesApplyDiff(ctx, r.client, plan.guestBasePath(), firewallRulesFromModel(plan.Rules))
	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating pve_guest_firewall_rules",
			fmt.Sprintf("applying firewall rules of guest %s vmid %d: %s", plan.Node.ValueString(), plan.VMID.ValueInt64(), err),
		)
		return
	}
	plan.Rules = firewallRulesToModel(fresh)
	plan.ID = plan.guestID()
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete implements resource.Resource. Deleting removes every rule,
// highest position first; an already-empty or vanished ruleset counts
// as deleted (the guest itself is left alone).
func (r *pveGuestFirewallRulesResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveGuestFirewallRulesResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := firewallRulesDeleteAll(ctx, r.client, state.guestBasePath()); err != nil {
		if isPVEClientNotFound(err) {
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting pve_guest_firewall_rules",
			fmt.Sprintf("deleting firewall rules of guest %s vmid %d: %s", state.Node.ValueString(), state.VMID.ValueInt64(), err),
		)
	}
}

// ImportState parses an import ID of the form `<node>:<guest_type>:<vmid>`.
func (r *pveGuestFirewallRulesResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts, err := firewallRulesSplitImportID(req.ID, 3)
	if err != nil {
		resp.Diagnostics.AddError("Invalid pve_guest_firewall_rules import ID", err.Error())
		return
	}
	vmid, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid pve_guest_firewall_rules import ID",
			fmt.Sprintf("import ID %q: vmid part %q is not a number", req.ID, parts[2]),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("node"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("guest_type"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("vmid"), vmid)...)
}
