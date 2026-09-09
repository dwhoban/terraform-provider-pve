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
	_ resource.Resource                = &pveNodeFirewallRulesResource{}
	_ resource.ResourceWithConfigure   = &pveNodeFirewallRulesResource{}
	_ resource.ResourceWithImportState = &pveNodeFirewallRulesResource{}
)

// NewPveNodeFirewallRulesResource returns the resource implementation.
func NewPveNodeFirewallRulesResource() resource.Resource {
	return &pveNodeFirewallRulesResource{}
}

// pveNodeFirewallRulesResource manages the ordered firewall ruleset of
// one node via /nodes/{node}/firewall/rules.
type pveNodeFirewallRulesResource struct {
	client *pveclient.Client
}

// pveNodeFirewallRulesResourceModel is the Terraform-facing shape.
type pveNodeFirewallRulesResourceModel struct {
	ID    types.String             `tfsdk:"id"`
	Node  types.String             `tfsdk:"node"`
	Rules []firewallRulesRuleModel `tfsdk:"rules"`
}

// Metadata implements resource.Resource.
func (r *pveNodeFirewallRulesResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveNodeFirewallRules
}

// Schema implements resource.Resource.
func (r *pveNodeFirewallRulesResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the ordered firewall ruleset of one node (`GET/POST /nodes/{node}/firewall/rules`, `GET/PUT/DELETE /nodes/{node}/firewall/rules/{pos}`). The `rules` list in configuration is authoritative: applies converge PVE to exactly this list and order, removing or rewriting entries made out of band. PVE renumbers positions, so `pos` is computed and resynced after every apply.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Identifier of the ruleset; the node name.",
			},
			"node": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The node whose ruleset this resource manages. Changing this value forces recreation.",
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
func (r *pveNodeFirewallRulesResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = firewallRulesConfigureResource(req, resp)
}

// Create implements resource.Resource.
func (r *pveNodeFirewallRulesResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveNodeFirewallRulesResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error creating pve_node_firewall_rules", "provider client is not configured")
		return
	}
	fresh, err := firewallRulesApplyDiff(ctx, r.client, pveclient.FirewallRulesPathNode(plan.Node.ValueString()), firewallRulesFromModel(plan.Rules))
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_node_firewall_rules",
			fmt.Sprintf("applying firewall rules of node %s: %s", plan.Node.ValueString(), err),
		)
		return
	}
	plan.Rules = firewallRulesToModel(fresh)
	plan.ID = plan.Node
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read implements resource.Resource.
func (r *pveNodeFirewallRulesResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveNodeFirewallRulesResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := firewallRulesReadInto(ctx, r.client, pveclient.FirewallRulesPathNode(state.Node.ValueString()), &state.Rules); err != nil {
		if isPVEClientNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error reading pve_node_firewall_rules",
			fmt.Sprintf("reading firewall rules of node %s: %s", state.Node.ValueString(), err),
		)
		return
	}
	state.ID = state.Node
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource.
func (r *pveNodeFirewallRulesResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveNodeFirewallRulesResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	fresh, err := firewallRulesApplyDiff(ctx, r.client, pveclient.FirewallRulesPathNode(plan.Node.ValueString()), firewallRulesFromModel(plan.Rules))
	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating pve_node_firewall_rules",
			fmt.Sprintf("applying firewall rules of node %s: %s", plan.Node.ValueString(), err),
		)
		return
	}
	plan.Rules = firewallRulesToModel(fresh)
	plan.ID = plan.Node
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete implements resource.Resource. Deleting removes every rule,
// highest position first; an already-empty or vanished ruleset counts
// as deleted.
func (r *pveNodeFirewallRulesResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveNodeFirewallRulesResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := firewallRulesDeleteAll(ctx, r.client, pveclient.FirewallRulesPathNode(state.Node.ValueString())); err != nil {
		if isPVEClientNotFound(err) {
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting pve_node_firewall_rules",
			fmt.Sprintf("deleting firewall rules of node %s: %s", state.Node.ValueString(), err),
		)
	}
}

// ImportState parses an import ID of the form `<node>`.
func (r *pveNodeFirewallRulesResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts, err := firewallRulesSplitImportID(req.ID, 1)
	if err != nil {
		resp.Diagnostics.AddError("Invalid pve_node_firewall_rules import ID", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("node"), parts[0])...)
}
