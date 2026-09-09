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

	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ resource.Resource                = &pveSdnPrefixListResource{}
	_ resource.ResourceWithConfigure   = &pveSdnPrefixListResource{}
	_ resource.ResourceWithImportState = &pveSdnPrefixListResource{}
)

// NewPveSdnPrefixListResource returns the resource implementation.
func NewPveSdnPrefixListResource() resource.Resource {
	return &pveSdnPrefixListResource{}
}

// pveSdnPrefixListResource manages one SDN prefix list and its ordered
// entries via /cluster/sdn/prefix-lists.
type pveSdnPrefixListResource struct {
	client *pveclient.Client
}

// pveSdnPrefixListResourceModel is the Terraform-facing shape.
type pveSdnPrefixListResourceModel struct {
	ID      types.String              `tfsdk:"id"`
	Entries []sdnPrefixListEntryModel `tfsdk:"entries"`
}

// Metadata implements resource.Resource.
func (r *pveSdnPrefixListResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveSdnPrefixList
}

// Schema implements resource.Resource.
func (r *pveSdnPrefixListResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages one SDN prefix list (`POST /cluster/sdn/prefix-lists`, `GET/DELETE /cluster/sdn/prefix-lists/{id}`, `GET/POST /cluster/sdn/prefix-lists/{id}/entries`, `PUT/DELETE /cluster/sdn/prefix-lists/{id}/entries/{seq}`). Prefix lists filter fabric routes: FRR evaluates the entries sequentially and the first match wins, so the `entries` list in configuration is authoritative — applies converge PVE to exactly this list and order, removing or rewriting entries made out of band. PVE assigns entry sequence numbers, so `seq` is computed and resynced after every apply. Changes take effect on the SDN fabric once the configuration is applied (see the `pve_sdn_apply` action).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				MarkdownDescription: "The SDN prefix list identifier (the pin's `id` key, e.g. `pl1`). This is also the Terraform identifier of the resource; changing it forces recreation.",
			},
			"entries": schema.ListNestedAttribute{
				Required:            true,
				MarkdownDescription: "The ordered prefix list entries. Order is significant: FRR evaluates entries sequentially and the first match wins.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: sdnPrefixListEntryAttributes(),
				},
			},
		},
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveSdnPrefixListResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = sdnListConfigureResource(req, resp)
}

// Create implements resource.Resource. The pin's create verb accepts
// the initial entries inline, so one POST materializes the whole list.
func (r *pveSdnPrefixListResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveSdnPrefixListResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error creating pve_sdn_prefix_list", "provider client is not configured")
		return
	}
	id := plan.ID.ValueString()
	if err := r.client.CreateSdnPrefixList(ctx, id, sdnPrefixListEntriesFromModel(plan.Entries)); err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_sdn_prefix_list",
			fmt.Sprintf("creating prefix list %s: %s", id, err),
		)
		return
	}
	fresh, err := r.client.ListSdnPrefixListEntries(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_sdn_prefix_list",
			fmt.Sprintf("reading back entries of prefix list %s: %s", id, err),
		)
		return
	}
	plan.Entries = sdnPrefixListEntriesToModel(fresh)
	tflog.Debug(ctx, "created pve_sdn_prefix_list", map[string]any{"id": id})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read implements resource.Resource.
func (r *pveSdnPrefixListResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveSdnPrefixListResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id := state.ID.ValueString()
	if _, err := r.client.GetSdnPrefixList(ctx, id); err != nil {
		if isPVEClientNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error reading pve_sdn_prefix_list",
			fmt.Sprintf("reading prefix list %s: %s", id, err),
		)
		return
	}
	entries, err := r.client.ListSdnPrefixListEntries(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_sdn_prefix_list",
			fmt.Sprintf("reading entries of prefix list %s: %s", id, err),
		)
		return
	}
	state.Entries = sdnPrefixListEntriesToModel(entries)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource.
func (r *pveSdnPrefixListResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveSdnPrefixListResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id := plan.ID.ValueString()
	fresh, err := sdnListApplyDiff(ctx, sdnPrefixListOps{client: r.client, id: id}, sdnPrefixListEntriesFromModel(plan.Entries))
	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating pve_sdn_prefix_list",
			fmt.Sprintf("applying entries of prefix list %s: %s", id, err),
		)
		return
	}
	plan.Entries = sdnPrefixListEntriesToModel(fresh)
	tflog.Debug(ctx, "updated pve_sdn_prefix_list", map[string]any{"id": id})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete implements resource.Resource. Deleting the list removes its
// entries with it; an already-vanished list counts as deleted.
func (r *pveSdnPrefixListResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveSdnPrefixListResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id := state.ID.ValueString()
	if err := r.client.DeleteSdnPrefixList(ctx, id); err != nil {
		if isPVEClientNotFound(err) {
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting pve_sdn_prefix_list",
			fmt.Sprintf("deleting prefix list %s: %s", id, err),
		)
		return
	}
	tflog.Debug(ctx, "deleted pve_sdn_prefix_list", map[string]any{"id": id})
}

// ImportState parses an import ID of the form `<id>`.
func (r *pveSdnPrefixListResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts, err := firewallRulesSplitImportID(req.ID, 1)
	if err != nil {
		resp.Diagnostics.AddError("Invalid pve_sdn_prefix_list import ID", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[0])...)
}
