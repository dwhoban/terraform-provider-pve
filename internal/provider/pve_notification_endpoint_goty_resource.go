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

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ resource.Resource                = &pveNotificationEndpointGotyResource{}
	_ resource.ResourceWithConfigure   = &pveNotificationEndpointGotyResource{}
	_ resource.ResourceWithImportState = &pveNotificationEndpointGotyResource{}
)

// NewPveNotificationEndpointGotyResource returns the resource implementation.
func NewPveNotificationEndpointGotyResource() resource.Resource {
	return &pveNotificationEndpointGotyResource{}
}

// pveNotificationEndpointGotyResource manages a Gotify notification
// endpoint via /cluster/notifications/endpoints/gotify.
type pveNotificationEndpointGotyResource struct {
	client *pveclient.Client
}

// pveNotificationEndpointGotyResourceModel is the Terraform-facing shape.
type pveNotificationEndpointGotyResourceModel struct {
	Name    types.String `tfsdk:"name"`
	Server  types.String `tfsdk:"server"`
	Token   types.String `tfsdk:"token"`
	Comment types.String `tfsdk:"comment"`
	Disable types.Bool   `tfsdk:"disable"`
}

// Metadata implements resource.Resource.
func (r *pveNotificationEndpointGotyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveNotificationEndpointGoty
}

// Schema implements resource.Resource.
func (r *pveNotificationEndpointGotyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Gotify notification endpoint (`POST/PUT/DELETE /cluster/notifications/endpoints/gotify/{name}`). Every mutation is synchronous per the pin (no task is spawned).",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The name of the endpoint (PVE `pve-configid` format). Changing this value forces recreation.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"server": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Server URL of the Gotify instance.",
			},
			"token": schema.StringAttribute{
				Required:            true,
				Sensitive:           true,
				MarkdownDescription: "Secret token for the Gotify application. Write-only: PVE never returns it, so drift cannot be detected.",
			},
			"comment": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Comment. Removed when unset on update.",
			},
			"disable": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Disable this target. PVE defaults to `false`.",
			},
		},
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveNotificationEndpointGotyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationEndpointConfigureResource(req, resp)
}

// Create implements resource.Resource.
func (r *pveNotificationEndpointGotyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveNotificationEndpointGotyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error creating pve_notification_endpoint_goty", "provider client is not configured")
		return
	}
	if err := r.client.CreateNotificationEndpoint(ctx, notificationEndpointGotyFromModel(plan)); err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_notification_endpoint_goty",
			fmt.Sprintf("creating Gotify endpoint %s: %s", plan.Name.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_notification_endpoint_goty after create",
			fmt.Sprintf("reading Gotify endpoint %s: %s", plan.Name.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read implements resource.Resource.
func (r *pveNotificationEndpointGotyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveNotificationEndpointGotyResourceModel
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
			"Error reading pve_notification_endpoint_goty",
			fmt.Sprintf("reading Gotify endpoint %s: %s", state.Name.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource.
func (r *pveNotificationEndpointGotyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveNotificationEndpointGotyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state pveNotificationEndpointGotyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error updating pve_notification_endpoint_goty", "provider client is not configured")
		return
	}
	deleteFields := notificationEndpointGotyDeleteFields(plan, state)
	if err := r.client.UpdateNotificationEndpoint(ctx, notificationEndpointGotyFromModel(plan), deleteFields); err != nil {
		resp.Diagnostics.AddError(
			"Error updating pve_notification_endpoint_goty",
			fmt.Sprintf("updating Gotify endpoint %s: %s", plan.Name.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_notification_endpoint_goty after update",
			fmt.Sprintf("reading Gotify endpoint %s: %s", plan.Name.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete implements resource.Resource.
func (r *pveNotificationEndpointGotyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveNotificationEndpointGotyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error deleting pve_notification_endpoint_goty", "provider client is not configured")
		return
	}
	if err := r.client.DeleteNotificationEndpoint(ctx, pveclient.NotificationEndpointTypeGotify, state.Name.ValueString()); err != nil {
		if isPVEClientNotFound(err) {
			// Already absent counts as deleted.
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting pve_notification_endpoint_goty",
			fmt.Sprintf("deleting Gotify endpoint %s: %s", state.Name.ValueString(), err),
		)
	}
}

// ImportState parses an import ID of the form `<name>`.
func (r *pveNotificationEndpointGotyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		resp.Diagnostics.AddError("Invalid pve_notification_endpoint_goty import ID", "import ID must be the endpoint name, e.g. `gotify`")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), req.ID)...)
}

// readInto refreshes the model from PVE. Write-only fields (token) are
// never returned by the pin's single GET and stay at their configured value.
func (r *pveNotificationEndpointGotyResource) readInto(ctx context.Context, m *pveNotificationEndpointGotyResourceModel) error {
	ep, err := r.client.GetNotificationEndpoint(ctx, pveclient.NotificationEndpointTypeGotify, m.Name.ValueString())
	if err != nil {
		return err
	}
	m.Server = types.StringValue(ep.Server)
	m.Comment = nodeNetworkStringToTF(ep.Comment)
	m.Disable = nodeNetworkBoolPtrToTF(ep.Disable)
	return nil
}
