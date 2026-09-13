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
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ resource.Resource                = &pveFirewallSecurityGroupResource{}
	_ resource.ResourceWithConfigure   = &pveFirewallSecurityGroupResource{}
	_ resource.ResourceWithImportState = &pveFirewallSecurityGroupResource{}
)

// NewPveFirewallSecurityGroupResource returns the resource implementation.
func NewPveFirewallSecurityGroupResource() resource.Resource {
	return &pveFirewallSecurityGroupResource{}
}

// pveFirewallSecurityGroupResource manages a security group container in the
// cluster firewall configuration via /cluster/firewall/groups. The group's
// rules are a separate ordered-list resource
// (pve_security_group_firewall_rules).
type pveFirewallSecurityGroupResource struct {
	client *pveclient.Client
}

// pveFirewallSecurityGroupResourceModel is the Terraform-facing shape.
type pveFirewallSecurityGroupResourceModel struct {
	Group   types.String `tfsdk:"group"`
	Comment types.String `tfsdk:"comment"`
}

// Metadata implements resource.Resource.
func (r *pveFirewallSecurityGroupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveFirewallSecurityGroup
}

// Schema implements resource.Resource.
func (r *pveFirewallSecurityGroupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a security group container in the cluster firewall configuration (`/cluster/firewall/groups`). Security groups bundle rules for reuse from other rulesets via the `pve_security_group_firewall_rules` resource; this resource manages the group's metadata only, never its rules. The pin's optional `rename` update parameter is deliberately not modeled: Terraform expresses name changes as destroy and create (`group` forces replacement).",
		Attributes: map[string]schema.Attribute{
			"group": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "Security Group name, 2 to 18 characters matching `[A-Za-z][A-Za-z0-9_-]+`. " +
					"Changing this value forces recreation.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"comment": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Optional comment or description. Removing it from configuration clears the comment upstream. Updated via the pin's update-by-create form (POST with `rename` set to the group's own name).",
			},
		},
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveFirewallSecurityGroupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = firewallEntityConfigureResource(req, resp)
}

// Create implements resource.Resource.
func (r *pveFirewallSecurityGroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveFirewallSecurityGroupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error creating pve_firewall_security_group", "provider client is not configured")
		return
	}
	if err := r.client.CreateFirewallSecurityGroup(ctx, firewallSecurityGroupFromModel(plan, "")); err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_firewall_security_group",
			fmt.Sprintf("creating firewall security group %s: %s", plan.Group.ValueString(), err),
		)
		return
	}
	if err := r.refreshInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_firewall_security_group after create",
			fmt.Sprintf("reading firewall security group %s: %s", plan.Group.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	tflog.Debug(ctx, "created PVE firewall security group", map[string]any{"group": plan.Group.ValueString()})
}

// Read implements resource.Resource.
func (r *pveFirewallSecurityGroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveFirewallSecurityGroupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	found, err := r.lookup(ctx, &state)
	if err != nil {
		if isPVEClientNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error reading pve_firewall_security_group",
			fmt.Sprintf("reading firewall security group %s: %s", state.Group.ValueString(), err),
		)
		return
	}
	if !found {
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource.
func (r *pveFirewallSecurityGroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveFirewallSecurityGroupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state pveFirewallSecurityGroupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error updating pve_firewall_security_group", "provider client is not configured")
		return
	}
	if plan.Comment.Equal(state.Comment) {
		return
	}
	// The pin exposes comment updates only through the update-by-create
	// form: POST with rename set to the group's own name.
	if err := r.client.UpdateFirewallSecurityGroup(ctx, firewallSecurityGroupFromModel(plan, plan.Group.ValueString())); err != nil {
		resp.Diagnostics.AddError(
			"Error updating pve_firewall_security_group",
			fmt.Sprintf("updating firewall security group %s: %s", plan.Group.ValueString(), err),
		)
		return
	}
	if err := r.refreshInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_firewall_security_group after update",
			fmt.Sprintf("reading firewall security group %s: %s", plan.Group.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	tflog.Debug(ctx, "updated PVE firewall security group", map[string]any{"group": plan.Group.ValueString()})
}

// Delete implements resource.Resource.
func (r *pveFirewallSecurityGroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveFirewallSecurityGroupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error deleting pve_firewall_security_group", "provider client is not configured")
		return
	}
	tflog.Debug(ctx, "deleting PVE firewall security group", map[string]any{"group": state.Group.ValueString()})
	if err := r.client.DeleteFirewallSecurityGroup(ctx, state.Group.ValueString()); err != nil {
		if isPVEClientNotFound(err) {
			// Already absent counts as deleted.
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting pve_firewall_security_group",
			fmt.Sprintf("deleting firewall security group %s: %s", state.Group.ValueString(), err),
		)
	}
	tflog.Debug(ctx, "deleted PVE firewall security group", map[string]any{"group": state.Group.ValueString()})
}

// ImportState parses an import ID of the form `<group>`.
func (r *pveFirewallSecurityGroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		resp.Diagnostics.AddError("Invalid pve_firewall_security_group import ID", "import ID must be the security group name, e.g. `web`")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("group"), req.ID)...)
}

// refreshInto refreshes the model's comment from the collection listing; the
// group's existence has just been established by the caller.
func (r *pveFirewallSecurityGroupResource) refreshInto(ctx context.Context, m *pveFirewallSecurityGroupResourceModel) error {
	_, err := r.lookup(ctx, m)
	return err
}

// lookup refreshes the model's comment from the collection listing and
// reports whether the group still exists. The pin's
// GET /cluster/firewall/groups/{group} returns the group's rules rather
// than its metadata, so the listing is the only metadata source.
func (r *pveFirewallSecurityGroupResource) lookup(ctx context.Context, m *pveFirewallSecurityGroupResourceModel) (bool, error) {
	groups, err := r.client.ListFirewallSecurityGroups(ctx)
	if err != nil {
		return false, err
	}
	for _, group := range groups {
		if group.Group == m.Group.ValueString() {
			m.Comment = nodeNetworkStringToTF(group.Comment)
			return true, nil
		}
	}
	return false, nil
}

// firewallSecurityGroupFromModel projects the Terraform model into the wire
// body. The comment is always emitted so an empty value clears it upstream;
// rename carries the pin's update-by-create form (empty on create).
func firewallSecurityGroupFromModel(m pveFirewallSecurityGroupResourceModel, rename string) pveclient.FirewallSecurityGroup {
	return pveclient.FirewallSecurityGroup{
		Group:   m.Group.ValueString(),
		Comment: m.Comment.ValueString(),
		Rename:  rename,
	}
}
