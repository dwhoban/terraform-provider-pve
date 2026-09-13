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
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ resource.Resource                = &pveNotificationMatcherResource{}
	_ resource.ResourceWithConfigure   = &pveNotificationMatcherResource{}
	_ resource.ResourceWithImportState = &pveNotificationMatcherResource{}
)

// NewPveNotificationMatcherResource returns the resource implementation.
func NewPveNotificationMatcherResource() resource.Resource {
	return &pveNotificationMatcherResource{}
}

// pveNotificationMatcherResource manages a notification matcher via
// /cluster/notifications/matchers. Every mutation is synchronous per the
// pin (no task is spawned).
type pveNotificationMatcherResource struct {
	client *pveclient.Client
}

// pveNotificationMatcherResourceModel is the Terraform-facing shape.
type pveNotificationMatcherResourceModel struct {
	Name          types.String `tfsdk:"name"`
	Target        types.List   `tfsdk:"target"`
	MatchField    types.List   `tfsdk:"match_field"`
	MatchSeverity types.List   `tfsdk:"match_severity"`
	MatchCalendar types.List   `tfsdk:"match_calendar"`
	Mode          types.String `tfsdk:"mode"`
	InvertMatch   types.Bool   `tfsdk:"invert_match"`
	Disable       types.Bool   `tfsdk:"disable"`
	Comment       types.String `tfsdk:"comment"`
}

// Metadata implements resource.Resource.
func (r *pveNotificationMatcherResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveNotificationMatcher
}

// Schema implements resource.Resource.
func (r *pveNotificationMatcherResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a notification matcher (`/cluster/notifications/matchers`). A matcher routes notifications that pass its conditions to one or more targets. All mutations are synchronous per the pin (no task is spawned). Existing targets, including built-ins such as `mail-to-root`, are listed by the `pve_notification_targets` data source.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The matcher name (PVE `pve-configid` format). Changing this value forces recreation.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"target": schema.ListAttribute{
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Names of the notification targets to notify when the matcher applies. Built-in targets such as `mail-to-root` are listed by the `pve_notification_targets` data source.",
			},
			"match_field": schema.ListAttribute{
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Notification metadata fields to match, each entry in the form `(regex|exact):<field>=<value>` (for example `exact:hostname=pve1` or `regex:hostname=^pve`). Matched notification metadata fields include `hostname`, `type`, `severity`, and `timestamp`.",
			},
			"match_severity": schema.ListAttribute{
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Notification severities to match. Typical values: `info`, `notice`, `warning`, `error`, `unknown` (the pin keeps this list open).",
			},
			"match_calendar": schema.ListAttribute{
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Calendar-event windows the notification timestamp must fall in (PVE calendar-event format, for example `sat..sun 02:30` or `mon-fri 9-17`).",
			},
			"mode": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "How multiple conditions combine. Must be one of: `all` (every condition must match, the upstream default), `any` (at least one condition must match).",
				Validators: []validator.String{
					stringvalidator.OneOf("all", "any"),
				},
			},
			"invert_match": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Invert the match result of the whole matcher: notifications that do NOT match the conditions are routed.",
			},
			"disable": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Whether this matcher is disabled; disabled matchers route nothing.",
			},
			"comment": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Description of the matcher.",
			},
		},
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveNotificationMatcherResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
func (r *pveNotificationMatcherResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveNotificationMatcherResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error creating pve_notification_matcher", "provider client is not configured")
		return
	}
	if err := r.client.CreateNotificationMatcher(ctx, notificationMatcherFromModel(plan)); err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_notification_matcher",
			fmt.Sprintf("creating notification matcher %s: %s", plan.Name.ValueString(), err),
		)
		return
	}
	tflog.Debug(ctx, "created PVE notification matcher", map[string]any{"name": plan.Name.ValueString()})
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_notification_matcher after create",
			fmt.Sprintf("reading notification matcher %s: %s", plan.Name.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read implements resource.Resource.
func (r *pveNotificationMatcherResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveNotificationMatcherResourceModel
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
			"Error reading pve_notification_matcher",
			fmt.Sprintf("reading notification matcher %s: %s", state.Name.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource.
func (r *pveNotificationMatcherResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveNotificationMatcherResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state pveNotificationMatcherResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	deleteFields := notificationMatcherDeleteFields(plan, state)
	if err := r.client.UpdateNotificationMatcher(ctx, plan.Name.ValueString(), notificationMatcherFromModel(plan), deleteFields); err != nil {
		resp.Diagnostics.AddError(
			"Error updating pve_notification_matcher",
			fmt.Sprintf("updating notification matcher %s: %s", plan.Name.ValueString(), err),
		)
		return
	}
	tflog.Debug(ctx, "updated PVE notification matcher", map[string]any{"name": plan.Name.ValueString()})
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_notification_matcher after update",
			fmt.Sprintf("reading notification matcher %s: %s", plan.Name.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete implements resource.Resource.
func (r *pveNotificationMatcherResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveNotificationMatcherResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	tflog.Debug(ctx, "deleting PVE notification matcher", map[string]any{"name": state.Name.ValueString()})
	if err := r.client.DeleteNotificationMatcher(ctx, state.Name.ValueString()); err != nil {
		if isPVEClientNotFound(err) {
			// Already absent counts as deleted.
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting pve_notification_matcher",
			fmt.Sprintf("deleting notification matcher %s: %s", state.Name.ValueString(), err),
		)
		return
	}
	tflog.Debug(ctx, "deleted PVE notification matcher", map[string]any{"name": state.Name.ValueString()})
}

// ImportState parses an import ID of the form `<name>`.
func (r *pveNotificationMatcherResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		resp.Diagnostics.AddError("Invalid pve_notification_matcher import ID", "import ID must be the matcher name, e.g. `ops`")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), req.ID)...)
}

// readInto refreshes the model from PVE.
func (r *pveNotificationMatcherResource) readInto(ctx context.Context, m *pveNotificationMatcherResourceModel) error {
	matcher, err := r.client.GetNotificationMatcher(ctx, m.Name.ValueString())
	if err != nil {
		return err
	}
	m.Target = listStringToTF(matcher.Target)
	m.MatchField = listStringToTF(matcher.MatchField)
	m.MatchSeverity = listStringToTF(matcher.MatchSeverity)
	m.MatchCalendar = listStringToTF(matcher.MatchCalendar)
	m.Mode = nodeNetworkStringToTF(matcher.Mode)
	m.InvertMatch = nodeNetworkBoolPtrToTF(matcher.InvertMatch)
	m.Disable = nodeNetworkBoolPtrToTF(matcher.Disable)
	m.Comment = nodeNetworkStringToTF(matcher.Comment)
	return nil
}

// notificationMatcherFromModel projects the Terraform model into the wire
// body. Lists carry the pin's entry formats verbatim; an explicitly empty
// list is omitted from the body and cleared through the `delete` query
// parameter instead (see notificationMatcherDeleteFields).
func notificationMatcherFromModel(m pveNotificationMatcherResourceModel) pveclient.NotificationMatcher {
	body := pveclient.NotificationMatcher{
		Name:          m.Name.ValueString(),
		Target:        listStringFromTF(m.Target),
		MatchField:    listStringFromTF(m.MatchField),
		MatchSeverity: listStringFromTF(m.MatchSeverity),
		MatchCalendar: listStringFromTF(m.MatchCalendar),
	}
	if !m.Mode.IsNull() && !m.Mode.IsUnknown() {
		body.Mode = m.Mode.ValueString()
	}
	if !m.InvertMatch.IsNull() && !m.InvertMatch.IsUnknown() {
		body.InvertMatch = pveclient.HABoolPtr(m.InvertMatch.ValueBool())
	}
	if !m.Disable.IsNull() && !m.Disable.IsUnknown() {
		body.Disable = pveclient.HABoolPtr(m.Disable.ValueBool())
	}
	if !m.Comment.IsNull() && !m.Comment.IsUnknown() {
		body.Comment = m.Comment.ValueString()
	}
	return body
}

// notificationMatcherDeleteFields returns the PVE field names to clear on
// update: list attributes that have entries in state but none in plan, and
// scalar attributes present in state but null in plan. Clearing through
// the `delete` query parameter is the only way to empty a PVE list.
func notificationMatcherDeleteFields(plan, state pveNotificationMatcherResourceModel) []string {
	var out []string
	if len(listStringFromTF(plan.Target)) == 0 && len(listStringFromTF(state.Target)) > 0 {
		out = append(out, "target")
	}
	if len(listStringFromTF(plan.MatchField)) == 0 && len(listStringFromTF(state.MatchField)) > 0 {
		out = append(out, "match-field")
	}
	if len(listStringFromTF(plan.MatchSeverity)) == 0 && len(listStringFromTF(state.MatchSeverity)) > 0 {
		out = append(out, "match-severity")
	}
	if len(listStringFromTF(plan.MatchCalendar)) == 0 && len(listStringFromTF(state.MatchCalendar)) > 0 {
		out = append(out, "match-calendar")
	}
	if plan.Mode.IsNull() && !state.Mode.IsNull() {
		out = append(out, "mode")
	}
	if plan.InvertMatch.IsNull() && !state.InvertMatch.IsNull() {
		out = append(out, "invert-match")
	}
	if plan.Disable.IsNull() && !state.Disable.IsNull() {
		out = append(out, "disable")
	}
	if plan.Comment.IsNull() && !state.Comment.IsNull() {
		out = append(out, "comment")
	}
	return out
}
