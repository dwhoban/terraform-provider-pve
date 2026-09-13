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

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ resource.Resource                = &pveNotificationEndpointSMTPResource{}
	_ resource.ResourceWithConfigure   = &pveNotificationEndpointSMTPResource{}
	_ resource.ResourceWithImportState = &pveNotificationEndpointSMTPResource{}
)

// NewPveNotificationEndpointSMTPResource returns the resource implementation.
func NewPveNotificationEndpointSMTPResource() resource.Resource {
	return &pveNotificationEndpointSMTPResource{}
}

// pveNotificationEndpointSMTPResource manages a SMTP notification
// endpoint via /cluster/notifications/endpoints/smtp.
type pveNotificationEndpointSMTPResource struct {
	client *pveclient.Client
}

// pveNotificationEndpointSMTPResourceModel is the Terraform-facing shape.
type pveNotificationEndpointSMTPResourceModel struct {
	Name        types.String `tfsdk:"name"`
	Server      types.String `tfsdk:"server"`
	FromAddress types.String `tfsdk:"from_address"`
	Username    types.String `tfsdk:"username"`
	Password    types.String `tfsdk:"password"`
	Port        types.Int64  `tfsdk:"port"`
	Mode        types.String `tfsdk:"mode"`
	MailTo      types.List   `tfsdk:"mailto"`
	MailToUser  types.List   `tfsdk:"mailto_user"`
	Author      types.String `tfsdk:"author"`
	Comment     types.String `tfsdk:"comment"`
	Disable     types.Bool   `tfsdk:"disable"`
}

// Metadata implements resource.Resource.
func (r *pveNotificationEndpointSMTPResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveNotificationEndpointSmtp
}

// Schema implements resource.Resource.
func (r *pveNotificationEndpointSMTPResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a SMTP notification endpoint (`POST/PUT/DELETE /cluster/notifications/endpoints/smtp/{name}`). Every mutation is synchronous per the pin (no task is spawned).",
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
				MarkdownDescription: "The address of the SMTP server.",
			},
			"from_address": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "`From` address for the mail.",
			},
			"username": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Username for SMTP authentication. Removed when unset on update.",
			},
			"password": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "Password for SMTP authentication. Write-only: PVE never returns it, so drift cannot be detected. Removed when unset on update.",
			},
			"port": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "The port to be used. Defaults to 465 for TLS based connections, 587 for STARTTLS based connections and port 25 for insecure plain-text connections. Removed when unset on update.",
			},
			"mode": schema.StringAttribute{
				Optional: true,
				Validators: []validator.String{
					stringvalidator.OneOf("insecure", "starttls", "tls"),
				},
				MarkdownDescription: "Determine which encryption method shall be used for the connection. Must be one of: `insecure`, `starttls`, `tls`. Defaults to `tls` on the server. Removed when unset on update.",
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
			"author": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Author of the mail. Defaults to `Proxmox VE` on the server. Removed when unset on update.",
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
func (r *pveNotificationEndpointSMTPResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = notificationEndpointConfigureResource(req, resp)
}

// Create implements resource.Resource.
func (r *pveNotificationEndpointSMTPResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveNotificationEndpointSMTPResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error creating pve_notification_endpoint_smtp", "provider client is not configured")
		return
	}
	if err := r.client.CreateNotificationEndpoint(ctx, notificationEndpointSMTPFromModel(plan)); err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_notification_endpoint_smtp",
			fmt.Sprintf("creating SMTP endpoint %s: %s", plan.Name.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_notification_endpoint_smtp after create",
			fmt.Sprintf("reading SMTP endpoint %s: %s", plan.Name.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read implements resource.Resource.
func (r *pveNotificationEndpointSMTPResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveNotificationEndpointSMTPResourceModel
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
			"Error reading pve_notification_endpoint_smtp",
			fmt.Sprintf("reading SMTP endpoint %s: %s", state.Name.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource.
func (r *pveNotificationEndpointSMTPResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveNotificationEndpointSMTPResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state pveNotificationEndpointSMTPResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error updating pve_notification_endpoint_smtp", "provider client is not configured")
		return
	}
	deleteFields := notificationEndpointSMTPDeleteFields(plan, state)
	if err := r.client.UpdateNotificationEndpoint(ctx, notificationEndpointSMTPFromModel(plan), deleteFields); err != nil {
		resp.Diagnostics.AddError(
			"Error updating pve_notification_endpoint_smtp",
			fmt.Sprintf("updating SMTP endpoint %s: %s", plan.Name.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_notification_endpoint_smtp after update",
			fmt.Sprintf("reading SMTP endpoint %s: %s", plan.Name.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete implements resource.Resource.
func (r *pveNotificationEndpointSMTPResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveNotificationEndpointSMTPResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error deleting pve_notification_endpoint_smtp", "provider client is not configured")
		return
	}
	if err := r.client.DeleteNotificationEndpoint(ctx, pveclient.NotificationEndpointTypeSMTP, state.Name.ValueString()); err != nil {
		if isPVEClientNotFound(err) {
			// Already absent counts as deleted.
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting pve_notification_endpoint_smtp",
			fmt.Sprintf("deleting SMTP endpoint %s: %s", state.Name.ValueString(), err),
		)
	}
}

// ImportState parses an import ID of the form `<name>`.
func (r *pveNotificationEndpointSMTPResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		resp.Diagnostics.AddError("Invalid pve_notification_endpoint_smtp import ID", "import ID must be the endpoint name, e.g. `mailrelay`")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), req.ID)...)
}

// readInto refreshes the model from PVE. Write-only fields (password) are
// never returned by the pin's single GET and stay at their configured value.
func (r *pveNotificationEndpointSMTPResource) readInto(ctx context.Context, m *pveNotificationEndpointSMTPResourceModel) error {
	ep, err := r.client.GetNotificationEndpoint(ctx, pveclient.NotificationEndpointTypeSMTP, m.Name.ValueString())
	if err != nil {
		return err
	}
	m.Server = types.StringValue(ep.Server)
	m.FromAddress = types.StringValue(ep.FromAddress)
	m.Username = nodeNetworkStringToTF(ep.Username)
	m.Mode = nodeNetworkStringToTF(ep.Mode)
	if ep.Port != nil {
		m.Port = types.Int64Value(*ep.Port)
	} else {
		m.Port = types.Int64Null()
	}
	m.MailTo = notificationEndpointListToTF(ep.MailTo)
	m.MailToUser = notificationEndpointListToTF(ep.MailToUser)
	m.Author = nodeNetworkStringToTF(ep.Author)
	m.Comment = nodeNetworkStringToTF(ep.Comment)
	m.Disable = nodeNetworkBoolPtrToTF(ep.Disable)
	return nil
}
