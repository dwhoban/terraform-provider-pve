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
	_ resource.Resource                = &pveHaGroupResource{}
	_ resource.ResourceWithConfigure   = &pveHaGroupResource{}
	_ resource.ResourceWithImportState = &pveHaGroupResource{}
)

// NewPveHaGroupResource returns the resource implementation.
func NewPveHaGroupResource() resource.Resource {
	return &pveHaGroupResource{}
}

// haConfigureResource extracts the shared client from provider data for HA
// resources. Nil provider data leaves the resource unconfigured (unit tests).
func haConfigureResource(req resource.ConfigureRequest, resp *resource.ConfigureResponse) *pveclient.Client {
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

// pveHaGroupResource manages an HA group via /cluster/ha/groups. PVE marks
// HA groups as deprecated in favor of HA rules (see pve_ha_rule).
type pveHaGroupResource struct {
	client *pveclient.Client
}

// pveHaGroupResourceModel is the Terraform-facing shape.
type pveHaGroupResourceModel struct {
	Group      types.String `tfsdk:"group"`
	Nodes      types.List   `tfsdk:"nodes"`
	Restricted types.Bool   `tfsdk:"restricted"`
	NoFailback types.Bool   `tfsdk:"nofailback"`
	Comment    types.String `tfsdk:"comment"`
}

// Metadata implements resource.Resource.
func (r *pveHaGroupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveHaGroup
}

// Schema implements resource.Resource.
func (r *pveHaGroupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages an HA group (`/cluster/ha/groups`). PVE marks HA groups as deprecated in favor of HA rules (`pve_ha_rule`); new cluster configurations should prefer rules. Every mutation is synchronous per the pin (no task is spawned).",
		Attributes: map[string]schema.Attribute{
			"group": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The HA group identifier (PVE `pve-configid` format). Changing this value forces recreation.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"nodes": schema.ListAttribute{
				Required:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Cluster node members, each entry formatted `<node>` or `<node>:<priority>` (PVE `pve-ha-node-list`, sent on the wire as a comma-separated string). A resource runs on the available nodes with the highest priority; priorities are relative only.",
			},
			"restricted": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Resources bound to a restricted group may only run on the nodes defined by the group; they are stopped when no group node is online. On unrestricted groups resources may run on any node when all group members are offline.",
			},
			"nofailback": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Prevents automatic migration back to the node with the highest priority when it comes online.",
			},
			"comment": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Description of the HA group.",
			},
		},
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveHaGroupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = haConfigureResource(req, resp)
}

// Create implements resource.Resource.
func (r *pveHaGroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveHaGroupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error creating pve_ha_group", "provider client is not configured")
		return
	}
	if err := r.client.CreateHAGroup(ctx, haGroupFromModel(plan)); err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_ha_group",
			fmt.Sprintf("creating HA group %s: %s", plan.Group.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_ha_group after create",
			fmt.Sprintf("reading HA group %s: %s", plan.Group.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read implements resource.Resource.
func (r *pveHaGroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveHaGroupResourceModel
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
			"Error reading pve_ha_group",
			fmt.Sprintf("reading HA group %s: %s", state.Group.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource.
func (r *pveHaGroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveHaGroupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state pveHaGroupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	deleteFields := haGroupDeleteFields(plan, state)
	if err := r.client.UpdateHAGroup(ctx, plan.Group.ValueString(), haGroupFromModel(plan), deleteFields); err != nil {
		resp.Diagnostics.AddError(
			"Error updating pve_ha_group",
			fmt.Sprintf("updating HA group %s: %s", plan.Group.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_ha_group after update",
			fmt.Sprintf("reading HA group %s: %s", plan.Group.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete implements resource.Resource.
func (r *pveHaGroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveHaGroupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteHAGroup(ctx, state.Group.ValueString()); err != nil {
		if isPVEClientNotFound(err) {
			// Already absent counts as deleted.
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting pve_ha_group",
			fmt.Sprintf("deleting HA group %s: %s", state.Group.ValueString(), err),
		)
	}
}

// ImportState parses an import ID of the form `<group>`.
func (r *pveHaGroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		resp.Diagnostics.AddError("Invalid pve_ha_group import ID", "import ID must be the HA group identifier, e.g. `primary`")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("group"), req.ID)...)
}

// readInto refreshes the model from PVE.
func (r *pveHaGroupResource) readInto(ctx context.Context, m *pveHaGroupResourceModel) error {
	group, err := r.client.GetHAGroup(ctx, m.Group.ValueString())
	if err != nil {
		return err
	}
	m.Nodes = listStringToTF(group.Nodes)
	m.Restricted = nodeNetworkBoolPtrToTF(group.Restricted)
	m.NoFailback = nodeNetworkBoolPtrToTF(group.NoFailback)
	m.Comment = nodeNetworkStringToTF(group.Comment)
	return nil
}

// haGroupFromModel projects the Terraform model into the wire body.
func haGroupFromModel(m pveHaGroupResourceModel) pveclient.HAGroup {
	body := pveclient.HAGroup{
		Group: m.Group.ValueString(),
		Nodes: listStringFromTF(m.Nodes),
	}
	if !m.Restricted.IsNull() && !m.Restricted.IsUnknown() {
		body.Restricted = pveclient.HABoolPtr(m.Restricted.ValueBool())
	}
	if !m.NoFailback.IsNull() && !m.NoFailback.IsUnknown() {
		body.NoFailback = pveclient.HABoolPtr(m.NoFailback.ValueBool())
	}
	if !m.Comment.IsNull() && !m.Comment.IsUnknown() {
		body.Comment = m.Comment.ValueString()
	}
	return body
}

// haGroupDeleteFields returns the PVE field names to clear on update:
// optional attributes present in state but null in plan.
func haGroupDeleteFields(plan, state pveHaGroupResourceModel) []string {
	var out []string
	if plan.Restricted.IsNull() && !state.Restricted.IsNull() {
		out = append(out, "restricted")
	}
	if plan.NoFailback.IsNull() && !state.NoFailback.IsNull() {
		out = append(out, "nofailback")
	}
	if plan.Comment.IsNull() && !state.Comment.IsNull() {
		out = append(out, "comment")
	}
	return out
}
