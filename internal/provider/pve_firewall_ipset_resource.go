// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ resource.Resource                = &pveFirewallIpsetResource{}
	_ resource.ResourceWithConfigure   = &pveFirewallIpsetResource{}
	_ resource.ResourceWithImportState = &pveFirewallIpsetResource{}
)

// NewPveFirewallIpsetResource returns the resource implementation.
func NewPveFirewallIpsetResource() resource.Resource {
	return &pveFirewallIpsetResource{}
}

// pveFirewallIpsetResource manages an IP set in the cluster firewall
// configuration via /cluster/firewall/ipset, including its CIDR membership.
type pveFirewallIpsetResource struct {
	client *pveclient.Client
}

// pveFirewallIpsetResourceModel is the Terraform-facing shape.
type pveFirewallIpsetResourceModel struct {
	Name    types.String `tfsdk:"name"`
	Comment types.String `tfsdk:"comment"`
	Cidrs   types.Set    `tfsdk:"cidrs"`
}

// Metadata implements resource.Resource.
func (r *pveFirewallIpsetResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveFirewallIpset
}

// Schema implements resource.Resource.
func (r *pveFirewallIpsetResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages an IP set and its members in the cluster firewall configuration (`/cluster/firewall/ipset`). Firewall rules reference the set as `+<name>`; the rules themselves are managed by `pve_cluster_firewall_rules`. The `cidrs` set is wholly managed: members removed from configuration are deleted upstream on apply and members added out of band are treated as drift. The pin's optional `rename` update parameter is deliberately not modeled: Terraform expresses name changes as destroy and create (`name` forces replacement).",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "IP set name, 2 to 64 characters matching `[A-Za-z][A-Za-z0-9_-]+`. " +
					"Changing this value forces recreation.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"comment": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Free-form comment. Removing it from configuration clears the comment upstream. Updated via the pin's update-by-create form (POST with `rename` set to the set's own name).",
			},
			"cidrs": schema.SetAttribute{
				Optional:    true,
				ElementType: types.StringType,
				MarkdownDescription: "Set members, each a network in CIDR format (e.g. `192.168.1.0/24`), a single IP address, or an alias reference (`+aliasname`). " +
					"Members are created with one POST per entry and removed with one DELETE per entry on the pin's per-CIDR member paths.",
			},
		},
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveFirewallIpsetResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = firewallEntityConfigureResource(req, resp)
}

// Create implements resource.Resource.
func (r *pveFirewallIpsetResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveFirewallIpsetResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error creating pve_firewall_ipset", "provider client is not configured")
		return
	}
	if err := r.client.CreateFirewallIpset(ctx, pveclient.FirewallIpset{
		Name:    plan.Name.ValueString(),
		Comment: plan.Comment.ValueString(),
	}); err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_firewall_ipset",
			fmt.Sprintf("creating firewall ipset %s: %s", plan.Name.ValueString(), err),
		)
		return
	}
	for _, cidr := range firewallIpsetCidrsFromTF(plan.Cidrs) {
		if err := r.client.AddFirewallIpsetMember(ctx, plan.Name.ValueString(), cidr); err != nil {
			resp.Diagnostics.AddError(
				"Error creating pve_firewall_ipset",
				fmt.Sprintf("adding member %s to firewall ipset %s: %s", cidr, plan.Name.ValueString(), err),
			)
			return
		}
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_firewall_ipset after create",
			fmt.Sprintf("reading firewall ipset %s: %s", plan.Name.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	tflog.Debug(ctx, "created PVE firewall ipset", map[string]any{"name": plan.Name.ValueString()})
}

// Read implements resource.Resource.
func (r *pveFirewallIpsetResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveFirewallIpsetResourceModel
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
			"Error reading pve_firewall_ipset",
			fmt.Sprintf("reading firewall ipset %s: %s", state.Name.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource.
func (r *pveFirewallIpsetResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveFirewallIpsetResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state pveFirewallIpsetResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error updating pve_firewall_ipset", "provider client is not configured")
		return
	}
	name := plan.Name.ValueString()

	if !plan.Comment.Equal(state.Comment) {
		if err := r.client.UpdateFirewallIpset(ctx, name, plan.Comment.ValueString()); err != nil {
			resp.Diagnostics.AddError(
				"Error updating pve_firewall_ipset",
				fmt.Sprintf("updating firewall ipset %s: %s", name, err),
			)
			return
		}
	}

	// Diff the desired membership against upstream, not against the prior
	// state, so out-of-band member changes are corrected too.
	members, err := r.client.ListFirewallIpsetMembers(ctx, name)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating pve_firewall_ipset",
			fmt.Sprintf("listing firewall ipset %s members: %s", name, err),
		)
		return
	}
	upstream := make(map[string]struct{}, len(members))
	for _, member := range members {
		upstream[member.Cidr] = struct{}{}
	}
	desired := firewallIpsetCidrsFromTF(plan.Cidrs)
	desiredSet := make(map[string]struct{}, len(desired))
	for _, cidr := range desired {
		desiredSet[cidr] = struct{}{}
		if _, ok := upstream[cidr]; !ok {
			if err := r.client.AddFirewallIpsetMember(ctx, name, cidr); err != nil {
				resp.Diagnostics.AddError(
					"Error updating pve_firewall_ipset",
					fmt.Sprintf("adding member %s to firewall ipset %s: %s", cidr, name, err),
				)
				return
			}
		}
	}
	for cidr := range upstream {
		if _, ok := desiredSet[cidr]; !ok {
			if err := r.client.RemoveFirewallIpsetMember(ctx, name, cidr); err != nil {
				resp.Diagnostics.AddError(
					"Error updating pve_firewall_ipset",
					fmt.Sprintf("removing member %s from firewall ipset %s: %s", cidr, name, err),
				)
				return
			}
		}
	}

	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_firewall_ipset after update",
			fmt.Sprintf("reading firewall ipset %s: %s", name, err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	tflog.Debug(ctx, "updated PVE firewall ipset", map[string]any{"name": name})
}

// Delete implements resource.Resource.
func (r *pveFirewallIpsetResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveFirewallIpsetResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error deleting pve_firewall_ipset", "provider client is not configured")
		return
	}
	tflog.Debug(ctx, "deleting PVE firewall ipset", map[string]any{"name": state.Name.ValueString()})
	if err := r.client.DeleteFirewallIpset(ctx, state.Name.ValueString()); err != nil {
		if isPVEClientNotFound(err) {
			// Already absent counts as deleted.
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting pve_firewall_ipset",
			fmt.Sprintf("deleting firewall ipset %s: %s", state.Name.ValueString(), err),
		)
	}
	tflog.Debug(ctx, "deleted PVE firewall ipset", map[string]any{"name": state.Name.ValueString()})
}

// ImportState parses an import ID of the form `<name>`.
func (r *pveFirewallIpsetResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		resp.Diagnostics.AddError("Invalid pve_firewall_ipset import ID", "import ID must be the IP set name, e.g. `management`")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), req.ID)...)
}

// readInto refreshes the model from PVE. Membership lives on
// GET /cluster/firewall/ipset/{name}; the set's own comment only on the
// collection listing, so both endpoints are consulted.
func (r *pveFirewallIpsetResource) readInto(ctx context.Context, m *pveFirewallIpsetResourceModel) error {
	name := m.Name.ValueString()
	members, err := r.client.ListFirewallIpsetMembers(ctx, name)
	if err != nil {
		return err
	}
	cidrs := make([]string, 0, len(members))
	for _, member := range members {
		cidrs = append(cidrs, member.Cidr)
	}
	m.Cidrs, err = firewallIpsetCidrsToTF(cidrs)
	if err != nil {
		return err
	}
	sets, err := r.client.ListFirewallIpsets(ctx)
	if err != nil {
		return err
	}
	for _, set := range sets {
		if set.Name == name {
			m.Comment = nodeNetworkStringToTF(set.Comment)
			return nil
		}
	}
	// The set exists (its membership read) but is missing from the listing;
	// leave the comment untouched rather than guessing.
	return nil
}

// firewallIpsetCidrsFromTF flattens a types.Set of strings into a []string.
func firewallIpsetCidrsFromTF(in types.Set) []string {
	out := make([]string, 0, len(in.Elements()))
	for _, e := range in.Elements() {
		s, ok := e.(types.String)
		if !ok || s.IsNull() || s.IsUnknown() {
			continue
		}
		out = append(out, s.ValueString())
	}
	return out
}

// firewallIpsetCidrsToTF builds a types.Set from a Go []string; an empty
// list becomes a null set so optional cidrs do not churn between plans.
func firewallIpsetCidrsToTF(cidrs []string) (types.Set, error) {
	if len(cidrs) == 0 {
		return types.SetNull(types.StringType), nil
	}
	elements := make([]attr.Value, 0, len(cidrs))
	for _, c := range cidrs {
		elements = append(elements, types.StringValue(c))
	}
	set, diags := types.SetValue(types.StringType, elements)
	if diags.HasError() {
		return types.SetNull(types.StringType), fmt.Errorf("building cidrs set: %d error(s)", diags.ErrorsCount())
	}
	return set, nil
}
