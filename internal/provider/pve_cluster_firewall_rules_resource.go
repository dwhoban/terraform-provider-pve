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

	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ resource.Resource                = &pveClusterFirewallRulesResource{}
	_ resource.ResourceWithConfigure   = &pveClusterFirewallRulesResource{}
	_ resource.ResourceWithImportState = &pveClusterFirewallRulesResource{}
)

// pveClusterFirewallRulesID is the static identifier of the cluster-wide
// ruleset singleton.
const pveClusterFirewallRulesID = "cluster"

// NewPveClusterFirewallRulesResource returns the resource implementation.
func NewPveClusterFirewallRulesResource() resource.Resource {
	return &pveClusterFirewallRulesResource{}
}

// pveClusterFirewallRulesResource manages the cluster-wide firewall
// ruleset as one ordered collection via /cluster/firewall/rules.
type pveClusterFirewallRulesResource struct {
	client *pveclient.Client
}

// pveClusterFirewallRulesResourceModel is the Terraform-facing shape.
type pveClusterFirewallRulesResourceModel struct {
	ID    types.String             `tfsdk:"id"`
	Rules []firewallRulesRuleModel `tfsdk:"rules"`
}

// Metadata implements resource.Resource.
func (r *pveClusterFirewallRulesResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveClusterFirewallRules
}

// Schema implements resource.Resource.
func (r *pveClusterFirewallRulesResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the cluster-wide firewall ruleset (`GET/POST /cluster/firewall/rules`, `GET/PUT/DELETE /cluster/firewall/rules/{pos}`) as one ordered collection. The `rules` list in configuration is authoritative: applies converge PVE to exactly this list and order, removing or rewriting entries made out of band. PVE renumbers positions, so `pos` is computed and resynced after every apply.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Identifier of the singleton ruleset; always `cluster`.",
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
func (r *pveClusterFirewallRulesResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = firewallRulesConfigureResource(req, resp)
}

// Create implements resource.Resource. POSTing each planned rule in
// order onto the (expected empty) upstream ruleset is the whole
// operation, so it shares the ordered-diff engine.
func (r *pveClusterFirewallRulesResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveClusterFirewallRulesResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error creating pve_cluster_firewall_rules", "provider client is not configured")
		return
	}
	fresh, err := firewallRulesApplyDiff(ctx, r.client, pveclient.FirewallRulesPathCluster(), firewallRulesFromModel(plan.Rules))
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_cluster_firewall_rules",
			fmt.Sprintf("applying cluster firewall rules: %s", err),
		)
		return
	}
	plan.Rules = firewallRulesToModel(fresh)
	plan.ID = types.StringValue(pveClusterFirewallRulesID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read implements resource.Resource.
func (r *pveClusterFirewallRulesResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveClusterFirewallRulesResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := firewallRulesReadInto(ctx, r.client, pveclient.FirewallRulesPathCluster(), &state.Rules); err != nil {
		if isPVEClientNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error reading pve_cluster_firewall_rules",
			fmt.Sprintf("reading cluster firewall rules: %s", err),
		)
		return
	}
	state.ID = types.StringValue(pveClusterFirewallRulesID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource.
func (r *pveClusterFirewallRulesResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveClusterFirewallRulesResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	fresh, err := firewallRulesApplyDiff(ctx, r.client, pveclient.FirewallRulesPathCluster(), firewallRulesFromModel(plan.Rules))
	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating pve_cluster_firewall_rules",
			fmt.Sprintf("applying cluster firewall rules: %s", err),
		)
		return
	}
	plan.Rules = firewallRulesToModel(fresh)
	plan.ID = types.StringValue(pveClusterFirewallRulesID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete implements resource.Resource. Deleting removes every rule,
// highest position first; an already-empty or vanished ruleset counts
// as deleted.
func (r *pveClusterFirewallRulesResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveClusterFirewallRulesResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := firewallRulesDeleteAll(ctx, r.client, pveclient.FirewallRulesPathCluster()); err != nil {
		if isPVEClientNotFound(err) {
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting pve_cluster_firewall_rules",
			fmt.Sprintf("deleting cluster firewall rules: %s", err),
		)
	}
}

// ImportState adopts the singleton; the identifier is always "cluster",
// whatever ID the import statement supplied.
func (r *pveClusterFirewallRulesResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), pveClusterFirewallRulesID)...)
}
