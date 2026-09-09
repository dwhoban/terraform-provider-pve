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

	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ resource.Resource                = &pveNotificationEndpointWebhookResource{}
	_ resource.ResourceWithConfigure   = &pveNotificationEndpointWebhookResource{}
	_ resource.ResourceWithImportState = &pveNotificationEndpointWebhookResource{}
)

// NewPveNotificationEndpointWebhookResource returns the resource implementation.
func NewPveNotificationEndpointWebhookResource() resource.Resource {
	return &pveNotificationEndpointWebhookResource{}
}

// pveNotificationEndpointWebhookResource manages a webhook notification
// endpoint via /cluster/notifications/endpoints/webhook.
type pveNotificationEndpointWebhookResource struct {
	client *pveclient.Client
}

// pveNotificationEndpointWebhookResourceModel is the Terraform-facing shape.
type pveNotificationEndpointWebhookResourceModel struct {
	Name    types.String `tfsdk:"name"`
	URL     types.String `tfsdk:"url"`
	Method  types.String `tfsdk:"method"`
	Body    types.String `tfsdk:"body"`
	Headers types.Map    `tfsdk:"headers"`
	Secrets types.Map    `tfsdk:"secrets"`
	Comment types.String `tfsdk:"comment"`
	Disable types.Bool   `tfsdk:"disable"`
}

// Metadata implements resource.Resource.
func (r *pveNotificationEndpointWebhookResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveNotificationEndpointWebhook
}

// Schema implements resource.Resource.
func (r *pveNotificationEndpointWebhookResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a webhook notification endpoint (`POST/PUT/DELETE /cluster/notifications/endpoints/webhook/{name}`). Every mutation is synchronous per the pin (no task is spawned).",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The name of the endpoint (PVE `pve-configid` format). Changing this value forces recreation.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"url": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Server URL to send the request to.",
			},
			"method": schema.StringAttribute{
				Required: true,
				Validators: []validator.String{
					stringvalidator.OneOf("post", "put", "get"),
				},
				MarkdownDescription: "HTTP method used for the request. Must be one of: `post`, `put`, `get`.",
			},
			"body": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "HTTP body template; PVE notification template placeholders such as `{{ title }}` are supported. PVE stores the body base64-encoded; encoding and decoding are transparent. Removed when unset on update.",
			},
			"headers": schema.MapAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "HTTP headers to set, as a `name` to `value` map. PVE stores them as `name=<name>,value=<base64 of value>` property strings; values are encoded and decoded transparently. Removed when unset on update.",
			},
			"secrets": schema.MapAttribute{
				Optional:            true,
				Sensitive:           true,
				ElementType:         types.StringType,
				MarkdownDescription: "Secrets referenced by the body or header templates via `{{ secrets.<name> }}`, as a `name` to `value` map. PVE stores them as `name=<name>,value=<base64 of value>` property strings; values are encoded and decoded transparently. Removed when unset on update.",
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
func (r *pveNotificationEndpointWebhookResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationEndpointConfigureResource(req, resp)
}

// Create implements resource.Resource.
func (r *pveNotificationEndpointWebhookResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveNotificationEndpointWebhookResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error creating pve_notification_endpoint_webhook", "provider client is not configured")
		return
	}
	if err := r.client.CreateNotificationEndpoint(ctx, notificationEndpointWebhookFromModel(plan)); err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_notification_endpoint_webhook",
			fmt.Sprintf("creating webhook endpoint %s: %s", plan.Name.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_notification_endpoint_webhook after create",
			fmt.Sprintf("reading webhook endpoint %s: %s", plan.Name.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read implements resource.Resource.
func (r *pveNotificationEndpointWebhookResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveNotificationEndpointWebhookResourceModel
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
			"Error reading pve_notification_endpoint_webhook",
			fmt.Sprintf("reading webhook endpoint %s: %s", state.Name.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource.
func (r *pveNotificationEndpointWebhookResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveNotificationEndpointWebhookResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state pveNotificationEndpointWebhookResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error updating pve_notification_endpoint_webhook", "provider client is not configured")
		return
	}
	deleteFields := notificationEndpointWebhookDeleteFields(plan, state)
	if err := r.client.UpdateNotificationEndpoint(ctx, notificationEndpointWebhookFromModel(plan), deleteFields); err != nil {
		resp.Diagnostics.AddError(
			"Error updating pve_notification_endpoint_webhook",
			fmt.Sprintf("updating webhook endpoint %s: %s", plan.Name.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_notification_endpoint_webhook after update",
			fmt.Sprintf("reading webhook endpoint %s: %s", plan.Name.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete implements resource.Resource.
func (r *pveNotificationEndpointWebhookResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveNotificationEndpointWebhookResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error deleting pve_notification_endpoint_webhook", "provider client is not configured")
		return
	}
	if err := r.client.DeleteNotificationEndpoint(ctx, pveclient.NotificationEndpointTypeWebhook, state.Name.ValueString()); err != nil {
		if isPVEClientNotFound(err) {
			// Already absent counts as deleted.
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting pve_notification_endpoint_webhook",
			fmt.Sprintf("deleting webhook endpoint %s: %s", state.Name.ValueString(), err),
		)
	}
}

// ImportState parses an import ID of the form `<name>`.
func (r *pveNotificationEndpointWebhookResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		resp.Diagnostics.AddError("Invalid pve_notification_endpoint_webhook import ID", "import ID must be the endpoint name, e.g. `alerting`")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), req.ID)...)
}

// readInto refreshes the model from PVE. Write-only fields ((none — the body, headers and secrets are returned base64-encoded)) are
// never returned by the pin's single GET and stay at their configured value.
func (r *pveNotificationEndpointWebhookResource) readInto(ctx context.Context, m *pveNotificationEndpointWebhookResourceModel) error {
	ep, err := r.client.GetNotificationEndpoint(ctx, pveclient.NotificationEndpointTypeWebhook, m.Name.ValueString())
	if err != nil {
		return err
	}
	m.URL = types.StringValue(ep.URL)
	m.Method = types.StringValue(ep.Method)
	m.Body = nodeNetworkStringToTF(ep.Body)
	m.Headers = notificationEndpointMapToTF(ep.Headers)
	m.Secrets = notificationEndpointMapToTF(ep.Secrets)
	m.Comment = nodeNetworkStringToTF(ep.Comment)
	m.Disable = nodeNetworkBoolPtrToTF(ep.Disable)
	return nil
}
