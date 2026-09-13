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
	_ resource.Resource                = &pveSecurityGroupFirewallRulesResource{}
	_ resource.ResourceWithConfigure   = &pveSecurityGroupFirewallRulesResource{}
	_ resource.ResourceWithImportState = &pveSecurityGroupFirewallRulesResource{}
)

// NewPveSecurityGroupFirewallRulesResource returns the resource implementation.
func NewPveSecurityGroupFirewallRulesResource() resource.Resource {
	return &pveSecurityGroupFirewallRulesResource{}
}

// pveSecurityGroupFirewallRulesResource manages the ordered rules of one
// security group via /cluster/firewall/groups/{group}/rules. The group
// itself is managed by pve_firewall_security_group; the two resources
// can coexist because this one owns only the group's rules.
type pveSecurityGroupFirewallRulesResource struct {
	client *pveclient.Client
}

// pveSecurityGroupFirewallRulesResourceModel is the Terraform-facing shape.
type pveSecurityGroupFirewallRulesResourceModel struct {
	ID    types.String             `tfsdk:"id"`
	Group types.String             `tfsdk:"group"`
	Rules []firewallRulesRuleModel `tfsdk:"rules"`
}

// Metadata implements resource.Resource.
func (r *pveSecurityGroupFirewallRulesResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveSecurityGroupFirewallRules
}

// Schema implements resource.Resource.
func (r *pveSecurityGroupFirewallRulesResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the ordered rules of one security group (`GET/POST /cluster/firewall/groups/{group}/rules`, `GET/PUT/DELETE /cluster/firewall/groups/{group}/rules/{pos}`). The group itself is managed by `pve_firewall_security_group`. The `rules` list in configuration is authoritative: applies converge PVE to exactly this list and order, removing or rewriting entries made out of band. PVE renumbers positions, so `pos` is computed and resynced after every apply.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Identifier of the ruleset; the security group name.",
			},
			"group": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The security group whose rules this resource manages. Changing this value forces recreation.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"rules": schema.ListNestedAttribute{
				Required:            true,
				MarkdownDescription: "The ordered security group rules. Order is significant: PVE evaluates rules top to bottom and the first match wins.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: firewallRulesRuleAttributes(),
				},
			},
		},
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveSecurityGroupFirewallRulesResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = firewallRulesConfigureResource(req, resp)
}

// Create implements resource.Resource.
func (r *pveSecurityGroupFirewallRulesResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveSecurityGroupFirewallRulesResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error creating pve_security_group_firewall_rules", "provider client is not configured")
		return
	}
	fresh, err := firewallRulesApplyDiff(ctx, r.client, pveclient.FirewallRulesPathSecurityGroup(plan.Group.ValueString()), firewallRulesFromModel(plan.Rules))
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_security_group_firewall_rules",
			fmt.Sprintf("applying rules of security group %s: %s", plan.Group.ValueString(), err),
		)
		return
	}
	plan.Rules = firewallRulesToModel(fresh)
	plan.ID = plan.Group
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read implements resource.Resource.
func (r *pveSecurityGroupFirewallRulesResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveSecurityGroupFirewallRulesResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := firewallRulesReadInto(ctx, r.client, pveclient.FirewallRulesPathSecurityGroup(state.Group.ValueString()), &state.Rules); err != nil {
		if isPVEClientNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error reading pve_security_group_firewall_rules",
			fmt.Sprintf("reading rules of security group %s: %s", state.Group.ValueString(), err),
		)
		return
	}
	state.ID = state.Group
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource.
func (r *pveSecurityGroupFirewallRulesResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveSecurityGroupFirewallRulesResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	fresh, err := firewallRulesApplyDiff(ctx, r.client, pveclient.FirewallRulesPathSecurityGroup(plan.Group.ValueString()), firewallRulesFromModel(plan.Rules))
	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating pve_security_group_firewall_rules",
			fmt.Sprintf("applying rules of security group %s: %s", plan.Group.ValueString(), err),
		)
		return
	}
	plan.Rules = firewallRulesToModel(fresh)
	plan.ID = plan.Group
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete implements resource.Resource. Deleting removes every rule,
// highest position first; an already-empty or vanished ruleset counts
// as deleted (the group itself is left alone).
func (r *pveSecurityGroupFirewallRulesResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveSecurityGroupFirewallRulesResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := firewallRulesDeleteAll(ctx, r.client, pveclient.FirewallRulesPathSecurityGroup(state.Group.ValueString())); err != nil {
		if isPVEClientNotFound(err) {
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting pve_security_group_firewall_rules",
			fmt.Sprintf("deleting rules of security group %s: %s", state.Group.ValueString(), err),
		)
	}
}

// ImportState parses an import ID of the form `<group>`.
func (r *pveSecurityGroupFirewallRulesResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts, err := firewallRulesSplitImportID(req.ID, 1)
	if err != nil {
		resp.Diagnostics.AddError("Invalid pve_security_group_firewall_rules import ID", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("group"), parts[0])...)
}
