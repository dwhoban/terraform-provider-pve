// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ resource.Resource                = &pveSdnZoneSimpleResource{}
	_ resource.ResourceWithConfigure   = &pveSdnZoneSimpleResource{}
	_ resource.ResourceWithImportState = &pveSdnZoneSimpleResource{}
)

// NewPveSdnZoneSimpleResource returns the resource implementation.
func NewPveSdnZoneSimpleResource() resource.Resource {
	return &pveSdnZoneSimpleResource{}
}

// pveSdnZoneSimpleResource manages one simple SDN zone (POST/GET/PUT/DELETE
// on /cluster/sdn/zones with type `simple`).
type pveSdnZoneSimpleResource struct {
	client *pveclient.Client
}

// pveSdnZoneSimpleResourceModel is the Terraform-facing shape.
type pveSdnZoneSimpleResourceModel struct {
	sdnZoneCommonModel
}

// Metadata implements resource.Resource.
func (r *pveSdnZoneSimpleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveSdnZoneSimple
}

// Schema implements resource.Resource.
func (r *pveSdnZoneSimpleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a simple SDN zone (`/cluster/sdn/zones`, type `simple`) — an isolated bridge-based zone with no VLAN tagging, using the IPAM/DNS backends configured for the zone. Creating, updating, or deleting a zone only changes the pending SDN configuration; run the `pve_sdn_apply` action to apply it to the running configuration. Changing `zone` forces recreation.",
		Attributes:          sdnZoneCommonAttributes(),
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveSdnZoneSimpleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = haConfigureResource(req, resp)
}

// Create implements resource.Resource.
func (r *pveSdnZoneSimpleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveSdnZoneSimpleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error creating pve_sdn_zone_simple", "provider client is not configured")
		return
	}
	body := sdnZoneCommonFromModel(plan.sdnZoneCommonModel)
	body.Type = "simple"
	if err := r.client.CreateSdnZone(ctx, body); err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_sdn_zone_simple",
			fmt.Sprintf("creating zone %s: %s", plan.Zone.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_sdn_zone_simple after create",
			fmt.Sprintf("reading zone %s: %s", plan.Zone.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	tflog.Debug(ctx, "created PVE simple sdn zone", map[string]any{"zone": plan.Zone.ValueString()})
}

// Read implements resource.Resource.
func (r *pveSdnZoneSimpleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveSdnZoneSimpleResourceModel
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
			"Error reading pve_sdn_zone_simple",
			fmt.Sprintf("reading zone %s: %s", state.Zone.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource. Changed fields travel in the PUT
// body; attributes cleared in the plan travel in the `delete` query
// parameter.
func (r *pveSdnZoneSimpleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveSdnZoneSimpleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state pveSdnZoneSimpleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := sdnZoneCommonFromModel(plan.sdnZoneCommonModel)
	body.Type = "simple"
	deleteFields := sdnZoneCommonDeleteFields(plan.sdnZoneCommonModel, state.sdnZoneCommonModel)
	if err := r.client.UpdateSdnZone(ctx, plan.Zone.ValueString(), body, deleteFields); err != nil {
		resp.Diagnostics.AddError(
			"Error updating pve_sdn_zone_simple",
			fmt.Sprintf("updating zone %s: %s", plan.Zone.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_sdn_zone_simple after update",
			fmt.Sprintf("reading zone %s: %s", plan.Zone.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	tflog.Debug(ctx, "updated PVE simple sdn zone", map[string]any{"zone": plan.Zone.ValueString()})
}

// Delete implements resource.Resource.
func (r *pveSdnZoneSimpleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveSdnZoneSimpleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	tflog.Debug(ctx, "deleting PVE simple sdn zone", map[string]any{"zone": state.Zone.ValueString()})
	if err := r.client.DeleteSdnZone(ctx, state.Zone.ValueString()); err != nil {
		if isPVEClientNotFound(err) {
			// Already absent counts as deleted.
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting pve_sdn_zone_simple",
			fmt.Sprintf("deleting zone %s: %s", state.Zone.ValueString(), err),
		)
	}
}

// ImportState parses an import ID of the form `<zone>`.
func (r *pveSdnZoneSimpleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		resp.Diagnostics.AddError("Invalid pve_sdn_zone_simple import ID", "import ID must be the zone identifier, e.g. `zone1`")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("zone"), req.ID)...)
}

// readInto refreshes the model from PVE, erroring when the upstream type
// is no longer simple.
func (r *pveSdnZoneSimpleResource) readInto(ctx context.Context, m *pveSdnZoneSimpleResourceModel) error {
	z, err := sdnZoneGetChecked(ctx, r.client, m.Zone.ValueString(), "simple")
	if err != nil {
		return err
	}
	sdnZoneCommonApply(z, &m.sdnZoneCommonModel)
	return nil
}
