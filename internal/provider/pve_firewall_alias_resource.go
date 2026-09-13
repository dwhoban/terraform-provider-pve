// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
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
	_ resource.Resource                = &pveFirewallAliasResource{}
	_ resource.ResourceWithConfigure   = &pveFirewallAliasResource{}
	_ resource.ResourceWithImportState = &pveFirewallAliasResource{}
)

// NewPveFirewallAliasResource returns the resource implementation.
func NewPveFirewallAliasResource() resource.Resource {
	return &pveFirewallAliasResource{}
}

// firewallEntityConfigureResource extracts the shared client from provider
// data for the firewall entity resources (alias, ipset, security group).
// Nil provider data leaves the resource unconfigured (unit tests).
func firewallEntityConfigureResource(req resource.ConfigureRequest, resp *resource.ConfigureResponse) *pveclient.Client {
	if req.ProviderData == nil {
		return nil
	}
	client, ok := req.ProviderData.(*pveclient.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *pveclient.Client, got: %T.", req.ProviderData),
		)
		return nil
	}
	return client
}

// firewallEntityConfigureDataSource extracts the shared client from provider
// data for the firewall entity data sources (alias, ipset, security group).
// Nil provider data leaves the data source unconfigured (unit tests).
func firewallEntityConfigureDataSource(req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) *pveclient.Client {
	if req.ProviderData == nil {
		return nil
	}
	client, ok := req.ProviderData.(*pveclient.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *pveclient.Client, got: %T.", req.ProviderData),
		)
		return nil
	}
	return client
}

// pveFirewallAliasResource manages an IP or network alias in the cluster
// firewall configuration via /cluster/firewall/aliases.
type pveFirewallAliasResource struct {
	client *pveclient.Client
}

// pveFirewallAliasResourceModel is the Terraform-facing shape.
type pveFirewallAliasResourceModel struct {
	Name    types.String `tfsdk:"name"`
	Cidr    types.String `tfsdk:"cidr"`
	Comment types.String `tfsdk:"comment"`
}

// Metadata implements resource.Resource.
func (r *pveFirewallAliasResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveFirewallAlias
}

// Schema implements resource.Resource.
func (r *pveFirewallAliasResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages an IP or network alias in the cluster firewall configuration (`/cluster/firewall/aliases`). Aliases bind a name to a CIDR so firewall rules can reference it as `+<name>`; the rules themselves are managed by `pve_cluster_firewall_rules`. The pin's optional `rename` update parameter is deliberately not modeled: Terraform expresses name changes as destroy and create (`name` forces replacement).",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "Alias name, 2 to 64 characters matching `[A-Za-z][A-Za-z0-9_-]+`. " +
					"Changing this value forces recreation.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"cidr": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Network/IP specification in CIDR format (e.g. `203.0.113.0/24` or a single address such as `192.0.2.1`).",
			},
			"comment": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Free-form comment. Removing it from configuration clears the comment upstream.",
			},
		},
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveFirewallAliasResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = firewallEntityConfigureResource(req, resp)
}

// Create implements resource.Resource.
func (r *pveFirewallAliasResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveFirewallAliasResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error creating pve_firewall_alias", "provider client is not configured")
		return
	}
	if err := r.client.CreateFirewallAlias(ctx, firewallAliasFromModel(plan)); err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_firewall_alias",
			fmt.Sprintf("creating firewall alias %s: %s", plan.Name.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_firewall_alias after create",
			fmt.Sprintf("reading firewall alias %s: %s", plan.Name.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	tflog.Debug(ctx, "created PVE firewall alias", map[string]any{"name": plan.Name.ValueString()})
}

// Read implements resource.Resource.
func (r *pveFirewallAliasResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveFirewallAliasResourceModel
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
			"Error reading pve_firewall_alias",
			fmt.Sprintf("reading firewall alias %s: %s", state.Name.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource.
func (r *pveFirewallAliasResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveFirewallAliasResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error updating pve_firewall_alias", "provider client is not configured")
		return
	}
	if err := r.client.UpdateFirewallAlias(ctx, plan.Name.ValueString(), firewallAliasFromModel(plan)); err != nil {
		resp.Diagnostics.AddError(
			"Error updating pve_firewall_alias",
			fmt.Sprintf("updating firewall alias %s: %s", plan.Name.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_firewall_alias after update",
			fmt.Sprintf("reading firewall alias %s: %s", plan.Name.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	tflog.Debug(ctx, "updated PVE firewall alias", map[string]any{"name": plan.Name.ValueString()})
}

// Delete implements resource.Resource.
func (r *pveFirewallAliasResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveFirewallAliasResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error deleting pve_firewall_alias", "provider client is not configured")
		return
	}
	tflog.Debug(ctx, "deleting PVE firewall alias", map[string]any{"name": state.Name.ValueString()})
	if err := r.client.DeleteFirewallAlias(ctx, state.Name.ValueString()); err != nil {
		if isPVEClientNotFound(err) {
			// Already absent counts as deleted.
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting pve_firewall_alias",
			fmt.Sprintf("deleting firewall alias %s: %s", state.Name.ValueString(), err),
		)
	}
	tflog.Debug(ctx, "deleted PVE firewall alias", map[string]any{"name": state.Name.ValueString()})
}

// ImportState parses an import ID of the form `<name>`.
func (r *pveFirewallAliasResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		resp.Diagnostics.AddError("Invalid pve_firewall_alias import ID", "import ID must be the alias name, e.g. `office`")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), req.ID)...)
}

// readInto refreshes the model from PVE.
func (r *pveFirewallAliasResource) readInto(ctx context.Context, m *pveFirewallAliasResourceModel) error {
	alias, err := r.client.GetFirewallAlias(ctx, m.Name.ValueString())
	if err != nil {
		return err
	}
	m.Cidr = types.StringValue(alias.Cidr)
	m.Comment = nodeNetworkStringToTF(alias.Comment)
	return nil
}

// firewallAliasFromModel projects the Terraform model into the wire body.
// The comment is always emitted so an empty value clears it upstream.
func firewallAliasFromModel(m pveFirewallAliasResourceModel) pveclient.FirewallAlias {
	return pveclient.FirewallAlias{
		Name:    m.Name.ValueString(),
		Cidr:    m.Cidr.ValueString(),
		Comment: m.Comment.ValueString(),
	}
}
