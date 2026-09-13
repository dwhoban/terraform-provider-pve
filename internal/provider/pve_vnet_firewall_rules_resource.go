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

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ resource.Resource                = &pveVnetFirewallRulesResource{}
	_ resource.ResourceWithConfigure   = &pveVnetFirewallRulesResource{}
	_ resource.ResourceWithImportState = &pveVnetFirewallRulesResource{}
)

// NewPveVnetFirewallRulesResource returns the resource implementation.
func NewPveVnetFirewallRulesResource() resource.Resource {
	return &pveVnetFirewallRulesResource{}
}

// pveVnetFirewallRulesResource manages the ordered firewall ruleset of
// one SDN vnet via /cluster/sdn/vnets/{vnet}/firewall/rules.
type pveVnetFirewallRulesResource struct {
	client *pveclient.Client
}

// pveVnetFirewallRulesResourceModel is the Terraform-facing shape.
type pveVnetFirewallRulesResourceModel struct {
	ID    types.String             `tfsdk:"id"`
	Vnet  types.String             `tfsdk:"vnet"`
	Rules []firewallRulesRuleModel `tfsdk:"rules"`
}

// Metadata implements resource.Resource.
func (r *pveVnetFirewallRulesResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveVnetFirewallRules
}

// Schema implements resource.Resource.
func (r *pveVnetFirewallRulesResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the ordered firewall ruleset of one SDN vnet (`GET/POST /cluster/sdn/vnets/{vnet}/firewall/rules`, `GET/PUT/DELETE /cluster/sdn/vnets/{vnet}/firewall/rules/{pos}`). The `rules` list in configuration is authoritative: applies converge PVE to exactly this list and order, removing or rewriting entries made out of band. PVE renumbers positions, so `pos` is computed and resynced after every apply.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Identifier of the ruleset; the vnet name.",
			},
			"vnet": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The SDN vnet whose ruleset this resource manages. Changing this value forces recreation.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
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
func (r *pveVnetFirewallRulesResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = firewallRulesConfigureResource(req, resp)
}

// Create implements resource.Resource.
func (r *pveVnetFirewallRulesResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveVnetFirewallRulesResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error creating pve_vnet_firewall_rules", "provider client is not configured")
		return
	}
	fresh, err := firewallRulesApplyDiff(ctx, r.client, pveclient.FirewallRulesPathVnet(plan.Vnet.ValueString()), firewallRulesFromModel(plan.Rules))
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_vnet_firewall_rules",
			fmt.Sprintf("applying firewall rules of vnet %s: %s", plan.Vnet.ValueString(), err),
		)
		return
	}
	plan.Rules = firewallRulesToModel(fresh)
	plan.ID = plan.Vnet
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read implements resource.Resource.
func (r *pveVnetFirewallRulesResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveVnetFirewallRulesResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := firewallRulesReadInto(ctx, r.client, pveclient.FirewallRulesPathVnet(state.Vnet.ValueString()), &state.Rules); err != nil {
		if isPVEClientNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error reading pve_vnet_firewall_rules",
			fmt.Sprintf("reading firewall rules of vnet %s: %s", state.Vnet.ValueString(), err),
		)
		return
	}
	state.ID = state.Vnet
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource.
func (r *pveVnetFirewallRulesResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveVnetFirewallRulesResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	fresh, err := firewallRulesApplyDiff(ctx, r.client, pveclient.FirewallRulesPathVnet(plan.Vnet.ValueString()), firewallRulesFromModel(plan.Rules))
	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating pve_vnet_firewall_rules",
			fmt.Sprintf("applying firewall rules of vnet %s: %s", plan.Vnet.ValueString(), err),
		)
		return
	}
	plan.Rules = firewallRulesToModel(fresh)
	plan.ID = plan.Vnet
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete implements resource.Resource. Deleting removes every rule,
// highest position first; an already-empty or vanished ruleset counts
// as deleted.
func (r *pveVnetFirewallRulesResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveVnetFirewallRulesResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := firewallRulesDeleteAll(ctx, r.client, pveclient.FirewallRulesPathVnet(state.Vnet.ValueString())); err != nil {
		if isPVEClientNotFound(err) {
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting pve_vnet_firewall_rules",
			fmt.Sprintf("deleting firewall rules of vnet %s: %s", state.Vnet.ValueString(), err),
		)
	}
}

// ImportState parses an import ID of the form `<vnet>`.
func (r *pveVnetFirewallRulesResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts, err := firewallRulesSplitImportID(req.ID, 1)
	if err != nil {
		resp.Diagnostics.AddError("Invalid pve_vnet_firewall_rules import ID", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("vnet"), parts[0])...)
}
