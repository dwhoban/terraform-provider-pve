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
	_ resource.Resource                = &pveSdnRouteMapResource{}
	_ resource.ResourceWithConfigure   = &pveSdnRouteMapResource{}
	_ resource.ResourceWithImportState = &pveSdnRouteMapResource{}
)

// NewPveSdnRouteMapResource returns the resource implementation.
func NewPveSdnRouteMapResource() resource.Resource {
	return &pveSdnRouteMapResource{}
}

// pveSdnRouteMapResource manages one SDN route map and its ordered
// entries via /cluster/sdn/route-maps/entries. PVE materializes a route
// map only through its entries: the pin defines no route-map creation
// or deletion verb of its own.
type pveSdnRouteMapResource struct {
	client *pveclient.Client
}

// pveSdnRouteMapResourceModel is the Terraform-facing shape.
type pveSdnRouteMapResourceModel struct {
	ID         types.String            `tfsdk:"id"`
	RouteMapID types.String            `tfsdk:"route_map_id"`
	Entries    []sdnRouteMapEntryModel `tfsdk:"entries"`
}

// Metadata implements resource.Resource.
func (r *pveSdnRouteMapResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveSdnRouteMap
}

// Schema implements resource.Resource.
func (r *pveSdnRouteMapResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages one SDN route map and its ordered entries (`GET /cluster/sdn/route-maps/entries/{route-map-id}`, `POST /cluster/sdn/route-maps/entries`, `PUT/DELETE /cluster/sdn/route-maps/entries/{route-map-id}/entry/{order}`). A route map exists only through its entries — PVE defines no separate route-map creation verb — so creating the first entry materializes the map and deleting the last entry removes it. Route evaluation walks the entries in index order and the first match wins, so the `entries` list in configuration is authoritative: applies converge PVE to exactly this list and order, removing or rewriting entries made out of band. PVE assigns entry indexes, so `order` is computed and resynced after every apply; new entries are appended after the highest existing index. Changes take effect on the SDN fabric once the configuration is applied (see the `pve_sdn_apply` action).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Identifier of the route map; equals the `route_map_id`.",
			},
			"route_map_id": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				MarkdownDescription: "The SDN route map identifier (the pin's `route-map-id` key, e.g. `rm1`). Changing this value forces recreation.",
			},
			"entries": schema.ListNestedAttribute{
				Required:            true,
				MarkdownDescription: "The ordered route map entries. Order is significant: evaluation walks the entries in index order and the first match wins.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: sdnRouteMapEntryAttributes(),
				},
			},
		},
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveSdnRouteMapResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = sdnListConfigureResource(req, resp)
}

// Create implements resource.Resource. Entry creation materializes the
// route map, so the ordered-diff engine performs the whole operation.
func (r *pveSdnRouteMapResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveSdnRouteMapResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error creating pve_sdn_route_map", "provider client is not configured")
		return
	}
	id := plan.RouteMapID.ValueString()
	entries, err := r.applyPlan(ctx, id, plan.Entries)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_sdn_route_map",
			fmt.Sprintf("creating entries of route map %s: %s", id, err),
		)
		return
	}
	plan.ID = plan.RouteMapID
	plan.Entries = entries
	tflog.Debug(ctx, "created pve_sdn_route_map", map[string]any{"route_map_id": id})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read implements resource.Resource.
func (r *pveSdnRouteMapResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveSdnRouteMapResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id := state.RouteMapID.ValueString()
	entries, err := r.client.ListSdnRouteMapEntries(ctx, id)
	if err != nil {
		if isPVEClientNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error reading pve_sdn_route_map",
			fmt.Sprintf("reading entries of route map %s: %s", id, err),
		)
		return
	}
	state.ID = state.RouteMapID
	state.Entries = sdnRouteMapEntriesToModel(entries)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource.
func (r *pveSdnRouteMapResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveSdnRouteMapResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id := plan.RouteMapID.ValueString()
	entries, err := r.applyPlan(ctx, id, plan.Entries)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating pve_sdn_route_map",
			fmt.Sprintf("applying entries of route map %s: %s", id, err),
		)
		return
	}
	plan.ID = plan.RouteMapID
	plan.Entries = entries
	tflog.Debug(ctx, "updated pve_sdn_route_map", map[string]any{"route_map_id": id})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete implements resource.Resource. Removing every entry removes the
// route map itself; an already-vanished map counts as deleted.
func (r *pveSdnRouteMapResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveSdnRouteMapResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id := state.RouteMapID.ValueString()
	if err := sdnListDeleteAll(ctx, sdnRouteMapOps{client: r.client, routeMapID: id}); err != nil {
		resp.Diagnostics.AddError(
			"Error deleting pve_sdn_route_map",
			fmt.Sprintf("deleting entries of route map %s: %s", id, err),
		)
		return
	}
	tflog.Debug(ctx, "deleted pve_sdn_route_map", map[string]any{"route_map_id": id})
}

// applyPlan converges the route map's upstream entries to the planned
// entries and resyncs the fresh order indexes into the models.
func (r *pveSdnRouteMapResource) applyPlan(ctx context.Context, id string, entries []sdnRouteMapEntryModel) ([]sdnRouteMapEntryModel, error) {
	fresh, err := sdnListApplyDiff(ctx, sdnRouteMapOps{client: r.client, routeMapID: id}, sdnRouteMapEntriesFromModel(entries))
	if err != nil {
		return nil, err
	}
	return sdnRouteMapEntriesToModel(fresh), nil
}

// ImportState parses an import ID of the form `<route-map-id>`.
func (r *pveSdnRouteMapResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts, err := firewallRulesSplitImportID(req.ID, 1)
	if err != nil {
		resp.Diagnostics.AddError("Invalid pve_sdn_route_map import ID", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("route_map_id"), parts[0])...)
}
