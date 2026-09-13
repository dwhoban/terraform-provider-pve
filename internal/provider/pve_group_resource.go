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

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ resource.Resource                = &pveGroupResource{}
	_ resource.ResourceWithConfigure   = &pveGroupResource{}
	_ resource.ResourceWithImportState = &pveGroupResource{}
)

// NewPveGroupResource returns the resource implementation.
func NewPveGroupResource() resource.Resource {
	return &pveGroupResource{}
}

// pveGroupResource manages a PVE user group via /access/groups.
type pveGroupResource struct {
	client *pveclient.Client
}

// pveGroupResourceModel is the Terraform-facing shape.
type pveGroupResourceModel struct {
	GroupID types.String `tfsdk:"groupid"`
	Comment types.String `tfsdk:"comment"`
	Members types.Set    `tfsdk:"members"`
}

// Metadata implements resource.Resource.
func (r *pveGroupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveGroup
}

// Schema implements resource.Resource.
func (r *pveGroupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a user group in Proxmox VE (`/access/groups`). Group membership is " +
			"maintained by PVE through each user's group list, not through the group endpoint: manage the " +
			"`groups` attribute of `pve_user` resources to change membership, and treat `members` here as a " +
			"read-only projection.",
		Attributes: map[string]schema.Attribute{
			"groupid": schema.StringAttribute{
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
				MarkdownDescription: "Group identifier (PVE `pve-groupid` format). Changing it forces replacement.",
			},
			"comment": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Free-form comment describing the group. The PVE update endpoint has no `delete` parameter: removing the attribute from configuration clears the comment upstream by sending an empty value.",
			},
			"members": schema.SetAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Full user IDs (`name@realm`) that are members of this group. Read-only: PVE manages membership on the user (`pve_user` `groups` attribute).",
			},
		},
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveGroupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*pveclient.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *pveclient.Client, got: %T.", req.ProviderData),
		)
		return
	}
	r.client = client
}

// Create implements resource.Resource.
func (r *pveGroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveGroupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.CreateAccessGroup(ctx, plan.GroupID.ValueString(), accessGroupCommentPtr(plan.Comment)); err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_group",
			fmt.Sprintf("creating group %s: %s", plan.GroupID.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_group after create",
			fmt.Sprintf("reading group %s: %s", plan.GroupID.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read implements resource.Resource.
func (r *pveGroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveGroupResourceModel
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
			"Error reading pve_group",
			fmt.Sprintf("reading group %s: %s", state.GroupID.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource.
func (r *pveGroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveGroupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state pveGroupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// The endpoint has no `delete` parameter: nil leaves the comment
	// unchanged, an empty string clears it. Skip the PUT entirely when the
	// comment is unset in both plan and state.
	comment := accessGroupUpdateComment(plan.Comment, state.Comment)
	if comment != nil {
		if err := r.client.UpdateAccessGroup(ctx, plan.GroupID.ValueString(), comment); err != nil {
			resp.Diagnostics.AddError(
				"Error updating pve_group",
				fmt.Sprintf("updating group %s: %s", plan.GroupID.ValueString(), err),
			)
			return
		}
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_group after update",
			fmt.Sprintf("reading group %s: %s", plan.GroupID.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete implements resource.Resource.
func (r *pveGroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveGroupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteAccessGroup(ctx, state.GroupID.ValueString()); err != nil {
		if isPVEClientNotFound(err) {
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting pve_group",
			fmt.Sprintf("deleting group %s: %s", state.GroupID.ValueString(), err),
		)
		return
	}
}

// ImportState parses an import ID of the form `<groupid>`.
func (r *pveGroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		resp.Diagnostics.AddError(
			"Invalid pve_group import ID",
			"Expected the import ID to be the group identifier, e.g. `admins`.",
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("groupid"), req.ID)...)
}

// readInto populates the model's computed attributes from PVE.
func (r *pveGroupResource) readInto(ctx context.Context, m *pveGroupResourceModel) error {
	detail, err := r.client.GetAccessGroup(ctx, m.GroupID.ValueString())
	if err != nil {
		return err
	}
	m.Comment = accessGroupCommentToTF(detail.Comment)
	members, err := accessGroupMembersToTF(detail.Members)
	if err != nil {
		return fmt.Errorf("converting members of group %s: %w", m.GroupID.ValueString(), err)
	}
	m.Members = members
	return nil
}

// accessGroupCommentPtr projects the Terraform comment into the wire body.
func accessGroupCommentPtr(comment types.String) *string {
	if comment.IsNull() || comment.IsUnknown() {
		return nil
	}
	value := comment.ValueString()
	return &value
}

// accessGroupUpdateComment decides the comment to send on update: nil means
// "leave unchanged" (skip the PUT), an empty string clears the comment
// upstream.
func accessGroupUpdateComment(plan, state types.String) *string {
	if plan.IsNull() || plan.IsUnknown() {
		if state.IsNull() {
			return nil
		}
		cleared := ""
		return &cleared
	}
	value := plan.ValueString()
	return &value
}

// accessGroupCommentToTF maps an upstream *string comment, keeping null for
// absent.
func accessGroupCommentToTF(comment *string) types.String {
	if comment == nil {
		return types.StringNull()
	}
	return types.StringValue(*comment)
}

// accessGroupMembersToTF builds the members set; a missing member list
// upstream becomes an empty set. Element types cannot mismatch here, so the
// diagnostics path is defensive only.
func accessGroupMembersToTF(members []string) (types.Set, error) {
	if members == nil {
		members = []string{}
	}
	elements := make([]attr.Value, 0, len(members))
	for _, member := range members {
		elements = append(elements, types.StringValue(member))
	}
	set, diags := types.SetValue(types.StringType, elements)
	if diags.HasError() {
		return types.SetNull(types.StringType), fmt.Errorf("building members set: %d error(s)", diags.ErrorsCount())
	}
	return set, nil
}
