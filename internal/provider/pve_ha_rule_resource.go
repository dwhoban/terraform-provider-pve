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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ resource.Resource                   = &pveHaRuleResource{}
	_ resource.ResourceWithConfigure      = &pveHaRuleResource{}
	_ resource.ResourceWithImportState    = &pveHaRuleResource{}
	_ resource.ResourceWithValidateConfig = &pveHaRuleResource{}
)

// NewPveHaRuleResource returns the resource implementation.
func NewPveHaRuleResource() resource.Resource {
	return &pveHaRuleResource{}
}

// pveHaRuleResource manages an HA rule via /cluster/ha/rules. Rules are
// the replacement for the deprecated HA groups.
type pveHaRuleResource struct {
	client *pveclient.Client
}

// pveHaRuleResourceModel is the Terraform-facing shape.
type pveHaRuleResourceModel struct {
	Rule      types.String `tfsdk:"rule"`
	Type      types.String `tfsdk:"type"`
	Affinity  types.String `tfsdk:"affinity"`
	Nodes     types.List   `tfsdk:"nodes"`
	Resources types.List   `tfsdk:"resources"`
	Strict    types.Bool   `tfsdk:"strict"`
	Disable   types.Bool   `tfsdk:"disable"`
	Comment   types.String `tfsdk:"comment"`
}

// Metadata implements resource.Resource.
func (r *pveHaRuleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveHaRule
}

// Schema implements resource.Resource.
func (r *pveHaRuleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages an HA rule (`/cluster/ha/rules`). HA rules replace the deprecated HA groups: a `node-affinity` rule constrains which nodes HA resources may run on, a `resource-affinity` rule keeps resources together on or apart from each other's nodes. Every mutation is synchronous per the pin (no task is spawned).",
		Attributes: map[string]schema.Attribute{
			"rule": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "HA rule identifier (PVE `pve-configid` format). Changing this value forces recreation.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"type": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "HA rule type. Must be one of: `node-affinity` (place resources on, or away from, the given nodes), `resource-affinity` (keep resources on the same node, or on separate nodes). " +
					"Changing this value forces recreation.",
				Validators: []validator.String{
					stringvalidator.OneOf("node-affinity", "resource-affinity"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"affinity": schema.StringAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "Must be one of: `positive`, `negative`. " +
					"For `node-affinity` rules: place the resources on the given nodes (`positive`, the upstream default) or on any but the given nodes (`negative`). " +
					"For `resource-affinity` rules: keep the resources on the same node (`positive`) or on separate nodes (`negative`); required for this rule type.",
				Validators: []validator.String{
					stringvalidator.OneOf("positive", "negative"),
				},
			},
			"nodes": schema.ListAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Only used by `node-affinity` rules (where it is required): cluster node members, each entry formatted `<node>` or `<node>:<priority>` (PVE `pve-ha-node-list`, sent on the wire as a comma-separated string). Priorities are relative only.",
			},
			"resources": schema.ListAttribute{
				Required:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "HA resource IDs the rule applies to, each formatted `<type>:<name>` (e.g. `vm:100`, `ct:101`), sent on the wire as a comma-separated string (PVE `pve-ha-resource-id-list`).",
			},
			"strict": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Only used by `node-affinity` rules: whether the rule is strict. A strict rule stops the resources when none of the defined nodes are available; a non-strict rule lets them run anywhere else. Defaults to false upstream.",
			},
			"disable": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Whether the HA rule is disabled.",
			},
			"comment": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "HA rule description.",
			},
		},
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveHaRuleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = haConfigureResource(req, resp)
}

// ValidateConfig implements resource.ResourceWithValidateConfig. The pin's
// per-type parameter requirements are enforced here because they depend on
// the value of `type`.
func (r *pveHaRuleResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config pveHaRuleResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if config.Type.IsNull() || config.Type.IsUnknown() {
		return
	}
	switch config.Type.ValueString() {
	case "node-affinity":
		if config.Nodes.IsNull() || len(config.Nodes.Elements()) == 0 {
			resp.Diagnostics.AddAttributeError(
				path.Root("nodes"),
				"Missing nodes for node-affinity rule",
				"HA rules of type `node-affinity` require the `nodes` attribute.",
			)
		}
	case "resource-affinity":
		if config.Affinity.IsNull() || config.Affinity.IsUnknown() {
			resp.Diagnostics.AddAttributeError(
				path.Root("affinity"),
				"Missing affinity for resource-affinity rule",
				"HA rules of type `resource-affinity` require the `affinity` attribute (`positive` or `negative`).",
			)
		}
		if !config.Nodes.IsNull() {
			resp.Diagnostics.AddAttributeError(
				path.Root("nodes"),
				"nodes is not valid for resource-affinity rules",
				"The `nodes` attribute is only used by `node-affinity` rules.",
			)
		}
		if !config.Strict.IsNull() {
			resp.Diagnostics.AddAttributeError(
				path.Root("strict"),
				"strict is not valid for resource-affinity rules",
				"The `strict` attribute is only used by `node-affinity` rules.",
			)
		}
	}
}

// Create implements resource.Resource.
func (r *pveHaRuleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveHaRuleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error creating pve_ha_rule", "provider client is not configured")
		return
	}
	if err := r.client.CreateHARule(ctx, haRuleFromModel(plan)); err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_ha_rule",
			fmt.Sprintf("creating HA rule %s: %s", plan.Rule.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_ha_rule after create",
			fmt.Sprintf("reading HA rule %s: %s", plan.Rule.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read implements resource.Resource.
func (r *pveHaRuleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveHaRuleResourceModel
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
			"Error reading pve_ha_rule",
			fmt.Sprintf("reading HA rule %s: %s", state.Rule.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource.
func (r *pveHaRuleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveHaRuleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state pveHaRuleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	deleteFields := haRuleDeleteFields(plan, state)
	if err := r.client.UpdateHARule(ctx, plan.Rule.ValueString(), haRuleFromModel(plan), deleteFields); err != nil {
		resp.Diagnostics.AddError(
			"Error updating pve_ha_rule",
			fmt.Sprintf("updating HA rule %s: %s", plan.Rule.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_ha_rule after update",
			fmt.Sprintf("reading HA rule %s: %s", plan.Rule.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete implements resource.Resource.
func (r *pveHaRuleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveHaRuleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteHARule(ctx, state.Rule.ValueString()); err != nil {
		if isPVEClientNotFound(err) {
			// Already absent counts as deleted.
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting pve_ha_rule",
			fmt.Sprintf("deleting HA rule %s: %s", state.Rule.ValueString(), err),
		)
	}
}

// ImportState parses an import ID of the form `<rule>`.
func (r *pveHaRuleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		resp.Diagnostics.AddError("Invalid pve_ha_rule import ID", "import ID must be the HA rule identifier, e.g. `keep-vms-together`")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("rule"), req.ID)...)
}

// readInto refreshes the model from PVE.
func (r *pveHaRuleResource) readInto(ctx context.Context, m *pveHaRuleResourceModel) error {
	rule, err := r.client.GetHARule(ctx, m.Rule.ValueString())
	if err != nil {
		return err
	}
	m.Type = nodeNetworkStringToTF(rule.Type)
	m.Affinity = nodeNetworkStringToTF(rule.Affinity)
	m.Nodes = listStringToTF(rule.Nodes)
	m.Resources = listStringToTF(rule.Resources)
	m.Strict = nodeNetworkBoolPtrToTF(rule.Strict)
	m.Disable = nodeNetworkBoolPtrToTF(rule.Disable)
	m.Comment = nodeNetworkStringToTF(rule.Comment)
	return nil
}

// haRuleFromModel projects the Terraform model into the wire body.
func haRuleFromModel(m pveHaRuleResourceModel) pveclient.HARule {
	body := pveclient.HARule{
		Rule:      m.Rule.ValueString(),
		Type:      m.Type.ValueString(),
		Nodes:     listStringFromTF(m.Nodes),
		Resources: listStringFromTF(m.Resources),
	}
	if !m.Affinity.IsNull() && !m.Affinity.IsUnknown() {
		body.Affinity = m.Affinity.ValueString()
	}
	if !m.Strict.IsNull() && !m.Strict.IsUnknown() {
		body.Strict = pveclient.HABoolPtr(m.Strict.ValueBool())
	}
	if !m.Disable.IsNull() && !m.Disable.IsUnknown() {
		body.Disable = pveclient.HABoolPtr(m.Disable.ValueBool())
	}
	if !m.Comment.IsNull() && !m.Comment.IsUnknown() {
		body.Comment = m.Comment.ValueString()
	}
	return body
}

// haRuleDeleteFields returns the PVE field names to clear on update:
// optional attributes present in state but null in plan.
func haRuleDeleteFields(plan, state pveHaRuleResourceModel) []string {
	var out []string
	if plan.Affinity.IsNull() && !state.Affinity.IsNull() {
		out = append(out, "affinity")
	}
	if plan.Nodes.IsNull() && !state.Nodes.IsNull() {
		out = append(out, "nodes")
	}
	if plan.Strict.IsNull() && !state.Strict.IsNull() {
		out = append(out, "strict")
	}
	if plan.Disable.IsNull() && !state.Disable.IsNull() {
		out = append(out, "disable")
	}
	if plan.Comment.IsNull() && !state.Comment.IsNull() {
		out = append(out, "comment")
	}
	return out
}
