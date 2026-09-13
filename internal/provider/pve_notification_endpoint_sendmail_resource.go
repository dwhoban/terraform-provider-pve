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
	_ resource.Resource                = &pveNotificationEndpointSendmailResource{}
	_ resource.ResourceWithConfigure   = &pveNotificationEndpointSendmailResource{}
	_ resource.ResourceWithImportState = &pveNotificationEndpointSendmailResource{}
)

// NewPveNotificationEndpointSendmailResource returns the resource implementation.
func NewPveNotificationEndpointSendmailResource() resource.Resource {
	return &pveNotificationEndpointSendmailResource{}
}

// pveNotificationEndpointSendmailResource manages a sendmail notification
// endpoint via /cluster/notifications/endpoints/sendmail.
type pveNotificationEndpointSendmailResource struct {
	client *pveclient.Client
}

// pveNotificationEndpointSendmailResourceModel is the Terraform-facing shape.
type pveNotificationEndpointSendmailResourceModel struct {
	Name        types.String `tfsdk:"name"`
	MailTo      types.List   `tfsdk:"mailto"`
	MailToUser  types.List   `tfsdk:"mailto_user"`
	FromAddress types.String `tfsdk:"from_address"`
	Author      types.String `tfsdk:"author"`
	Comment     types.String `tfsdk:"comment"`
	Disable     types.Bool   `tfsdk:"disable"`
}

// Metadata implements resource.Resource.
func (r *pveNotificationEndpointSendmailResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveNotificationEndpointSendmail
}

// Schema implements resource.Resource.
func (r *pveNotificationEndpointSendmailResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a sendmail notification endpoint (`POST/PUT/DELETE /cluster/notifications/endpoints/sendmail/{name}`). Every mutation is synchronous per the pin (no task is spawned).",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The name of the endpoint (PVE `pve-configid` format). Changing this value forces recreation.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"mailto": schema.ListAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "List of email recipients (PVE `email-or-username` format). Removed when unset on update.",
			},
			"mailto_user": schema.ListAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "List of PVE users (e.g. `root@pam`) whose configured mail addresses receive the notification. Removed when unset on update.",
			},
			"from_address": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "`From` address for the mail. Removed when unset on update.",
			},
			"author": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Author of the mail. Removed when unset on update.",
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
func (r *pveNotificationEndpointSendmailResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationEndpointConfigureResource(req, resp)
}

// Create implements resource.Resource.
func (r *pveNotificationEndpointSendmailResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveNotificationEndpointSendmailResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error creating pve_notification_endpoint_sendmail", "provider client is not configured")
		return
	}
	if err := r.client.CreateNotificationEndpoint(ctx, notificationEndpointSendmailFromModel(plan)); err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_notification_endpoint_sendmail",
			fmt.Sprintf("creating sendmail endpoint %s: %s", plan.Name.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_notification_endpoint_sendmail after create",
			fmt.Sprintf("reading sendmail endpoint %s: %s", plan.Name.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read implements resource.Resource.
func (r *pveNotificationEndpointSendmailResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveNotificationEndpointSendmailResourceModel
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
			"Error reading pve_notification_endpoint_sendmail",
			fmt.Sprintf("reading sendmail endpoint %s: %s", state.Name.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource.
func (r *pveNotificationEndpointSendmailResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveNotificationEndpointSendmailResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state pveNotificationEndpointSendmailResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error updating pve_notification_endpoint_sendmail", "provider client is not configured")
		return
	}
	deleteFields := notificationEndpointSendmailDeleteFields(plan, state)
	if err := r.client.UpdateNotificationEndpoint(ctx, notificationEndpointSendmailFromModel(plan), deleteFields); err != nil {
		resp.Diagnostics.AddError(
			"Error updating pve_notification_endpoint_sendmail",
			fmt.Sprintf("updating sendmail endpoint %s: %s", plan.Name.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_notification_endpoint_sendmail after update",
			fmt.Sprintf("reading sendmail endpoint %s: %s", plan.Name.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete implements resource.Resource.
func (r *pveNotificationEndpointSendmailResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveNotificationEndpointSendmailResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error deleting pve_notification_endpoint_sendmail", "provider client is not configured")
		return
	}
	if err := r.client.DeleteNotificationEndpoint(ctx, pveclient.NotificationEndpointTypeSendmail, state.Name.ValueString()); err != nil {
		if isPVEClientNotFound(err) {
			// Already absent counts as deleted.
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting pve_notification_endpoint_sendmail",
			fmt.Sprintf("deleting sendmail endpoint %s: %s", state.Name.ValueString(), err),
		)
	}
}

// ImportState parses an import ID of the form `<name>`.
func (r *pveNotificationEndpointSendmailResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		resp.Diagnostics.AddError("Invalid pve_notification_endpoint_sendmail import ID", "import ID must be the endpoint name, e.g. `ops-mail`")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), req.ID)...)
}

// readInto refreshes the model from PVE.
func (r *pveNotificationEndpointSendmailResource) readInto(ctx context.Context, m *pveNotificationEndpointSendmailResourceModel) error {
	ep, err := r.client.GetNotificationEndpoint(ctx, pveclient.NotificationEndpointTypeSendmail, m.Name.ValueString())
	if err != nil {
		return err
	}
	m.MailTo = notificationEndpointListToTF(ep.MailTo)
	m.MailToUser = notificationEndpointListToTF(ep.MailToUser)
	m.FromAddress = nodeNetworkStringToTF(ep.FromAddress)
	m.Author = nodeNetworkStringToTF(ep.Author)
	m.Comment = nodeNetworkStringToTF(ep.Comment)
	m.Disable = nodeNetworkBoolPtrToTF(ep.Disable)
	return nil
}
