// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

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
	_ resource.Resource                   = &pveRealmSyncJobResource{}
	_ resource.ResourceWithConfigure      = &pveRealmSyncJobResource{}
	_ resource.ResourceWithImportState    = &pveRealmSyncJobResource{}
	_ resource.ResourceWithValidateConfig = &pveRealmSyncJobResource{}
)

// NewPveRealmSyncJobResource returns the resource implementation.
func NewPveRealmSyncJobResource() resource.Resource {
	return &pveRealmSyncJobResource{}
}

// pveRealmSyncJobResource manages a realm-sync job definition
// (/cluster/jobs/realm-sync) that periodically syncs a realm from its
// upstream directory.
type pveRealmSyncJobResource struct {
	client *pveclient.Client
}

// realmSyncJobModel holds the Terraform-facing shape shared by the
// pve_realm_sync_job resource and data source.
type realmSyncJobModel struct {
	ID             types.String `tfsdk:"id"`
	Realm          types.String `tfsdk:"realm"`
	Schedule       types.String `tfsdk:"schedule"`
	Scope          types.String `tfsdk:"scope"`
	RemoveVanished types.String `tfsdk:"remove_vanished"`
	EnableNew      types.Bool   `tfsdk:"enable_new"`
	Enabled        types.Bool   `tfsdk:"enabled"`
	Comment        types.String `tfsdk:"comment"`
	LastRun        types.Int64  `tfsdk:"last_run"`
	NextRun        types.Int64  `tfsdk:"next_run"`
}

// pveRealmSyncJobResourceModel is the resource form of the model.
type pveRealmSyncJobResourceModel struct {
	realmSyncJobModel
}

// Metadata implements resource.Resource.
func (r *pveRealmSyncJobResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveRealmSyncJob
}

// realmSyncJobAttributes returns the schema attribute set. computed marks
// the data-source form; replaceable adds RequiresReplace to the identity
// attributes for the resource form.
func realmSyncJobAttributes(computed, replaceable bool) map[string]schema.Attribute {
	attrs := map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Required:            !computed,
			Computed:            computed,
			MarkdownDescription: "Unique job ID (PVE config ID, up to 64 characters). Changing the ID replaces the job.",
			Validators: []validator.String{
				stringvalidator.LengthAtMost(64),
			},
		},
		"realm": schema.StringAttribute{
			Required:            !computed,
			Computed:            computed,
			MarkdownDescription: "Authentication domain ID the job syncs. Cannot be changed after creation.",
		},
		"schedule": schema.StringAttribute{
			Required:            !computed,
			Computed:            computed,
			MarkdownDescription: "Run schedule in the `systemd` calendar-event subset PVE accepts (e.g. `mon..fri 02:30`, `daily`, `sun *-1..7 02:30`), up to 128 characters. The resource validates the format against `GET /cluster/jobs/schedule-analyze` when the cluster is reachable.",
		},
		"scope": schema.StringAttribute{
			Optional:            !computed,
			Computed:            computed,
			MarkdownDescription: "Select what to sync. Must be one of `users`, `groups`, `both`.",
			Validators: []validator.String{
				stringvalidator.OneOf("users", "groups", "both"),
			},
		},
		"remove_vanished": schema.StringAttribute{
			Optional:            !computed,
			Computed:            computed,
			MarkdownDescription: "A semicolon-separated list of things to remove when they vanish during the sync: `entry` removes the user/group when not returned from the sync, `properties` removes properties of existing users/groups missing in the source, `acl` removes ACLs of vanished users/groups. `none` (the default) disables all removals, e.g. `entry;acl`.",
		},
		"enable_new": schema.BoolAttribute{
			Optional:            !computed,
			Computed:            computed,
			MarkdownDescription: "Enable newly synced users immediately. Defaults to `true`.",
		},
		"enabled": schema.BoolAttribute{
			Optional:            !computed,
			Computed:            computed,
			MarkdownDescription: "Determines if the job is enabled. Defaults to `true`.",
		},
		"comment": schema.StringAttribute{
			Optional:            !computed,
			Computed:            computed,
			MarkdownDescription: "Description for the job (up to 512 characters).",
		},
		"last_run": schema.Int64Attribute{
			Computed:            true,
			MarkdownDescription: "Last execution time of the job in seconds since the UNIX epoch; null before the first run.",
		},
		"next_run": schema.Int64Attribute{
			Computed:            true,
			MarkdownDescription: "Next planned execution time of the job in seconds since the UNIX epoch; null when unknown.",
		},
	}
	if replaceable {
		for _, name := range []string{"id", "realm"} {
			if attr, ok := attrs[name].(schema.StringAttribute); ok {
				attr.PlanModifiers = []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				}
				attrs[name] = attr
			}
		}
	}
	return attrs
}

// Schema implements resource.Resource.
func (r *pveRealmSyncJobResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a realm-sync job (`/cluster/jobs/realm-sync`) that periodically syncs users and/or groups of an LDAP/AD realm from its upstream directory.",
		Attributes:          realmSyncJobAttributes(false, true),
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveRealmSyncJobResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = realmConfigureResource(req, resp)
}

// ValidateConfig implements resource.ResourceWithValidateConfig. When the
// cluster is reachable it validates the schedule format via
// GET /cluster/jobs/schedule-analyze; connectivity failures are ignored so
// plans never hard-fail on an offline cluster.
func (r *pveRealmSyncJobResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	if r.client == nil {
		return
	}
	var config pveRealmSyncJobResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if config.Schedule.IsNull() || config.Schedule.IsUnknown() {
		return
	}
	schedule := config.Schedule.ValueString()
	if _, err := r.client.AnalyzeJobSchedule(ctx, schedule, 1, 0); err != nil {
		var apiErr *pveclient.APIError
		if errors.As(err, &apiErr) {
			resp.Diagnostics.AddError(
				"Invalid pve_realm_sync_job schedule",
				fmt.Sprintf("schedule %q was rejected by the cluster: %s", schedule, err),
			)
		}
	}
}

// Create implements resource.Resource.
func (r *pveRealmSyncJobResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveRealmSyncJobResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := realmSyncJobBody(plan.realmSyncJobModel)
	tflog.Debug(ctx, "creating realm-sync job", map[string]any{"id": plan.ID.ValueString()})
	if err := r.client.CreateRealmSyncJob(ctx, body); err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_realm_sync_job",
			fmt.Sprintf("creating job %s: %s", plan.ID.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan.realmSyncJobModel); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_realm_sync_job after create",
			fmt.Sprintf("reading job %s: %s", plan.ID.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read implements resource.Resource.
func (r *pveRealmSyncJobResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveRealmSyncJobResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.readInto(ctx, &state.realmSyncJobModel); err != nil {
		if isPVEClientNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error reading pve_realm_sync_job",
			fmt.Sprintf("reading job %s: %s", state.ID.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource.
func (r *pveRealmSyncJobResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveRealmSyncJobResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state pveRealmSyncJobResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := realmSyncJobBody(plan.realmSyncJobModel)
	var del []string
	if realmStringCleared(plan.Scope, state.Scope) {
		del = append(del, "scope")
	}
	if realmStringCleared(plan.RemoveVanished, state.RemoveVanished) {
		del = append(del, "remove-vanished")
	}
	if realmStringCleared(plan.Comment, state.Comment) {
		del = append(del, "comment")
	}
	body.Delete = strings.Join(del, ",")
	tflog.Debug(ctx, "updating realm-sync job", map[string]any{"id": plan.ID.ValueString()})
	if err := r.client.UpdateRealmSyncJob(ctx, plan.ID.ValueString(), body); err != nil {
		resp.Diagnostics.AddError(
			"Error updating pve_realm_sync_job",
			fmt.Sprintf("updating job %s: %s", plan.ID.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan.realmSyncJobModel); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_realm_sync_job after update",
			fmt.Sprintf("reading job %s: %s", plan.ID.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete implements resource.Resource.
func (r *pveRealmSyncJobResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveRealmSyncJobResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	tflog.Debug(ctx, "deleting realm-sync job", map[string]any{"id": state.ID.ValueString()})
	if err := r.client.DeleteRealmSyncJob(ctx, state.ID.ValueString()); err != nil {
		if isPVEClientNotFound(err) {
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting pve_realm_sync_job",
			fmt.Sprintf("deleting job %s: %s", state.ID.ValueString(), err),
		)
		return
	}
}

// ImportState parses an import ID of the form `<job-id>`.
func (r *pveRealmSyncJobResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

// readInto populates the model from PVE.
func (r *pveRealmSyncJobResource) readInto(ctx context.Context, m *realmSyncJobModel) error {
	job, err := r.client.GetRealmSyncJob(ctx, m.ID.ValueString())
	if err != nil {
		return err
	}
	realmSyncJobIntoModel(m, job)
	return nil
}

// realmSyncJobBody projects the model into the wire body. ID, Realm, and
// Schedule are always sent; the PUT endpoint requires id and schedule.
func realmSyncJobBody(m realmSyncJobModel) pveclient.RealmSyncJob {
	body := pveclient.RealmSyncJob{
		ID:       m.ID.ValueString(),
		Realm:    m.Realm.ValueString(),
		Schedule: m.Schedule.ValueString(),
	}
	if !m.Scope.IsNull() && !m.Scope.IsUnknown() {
		body.Scope = m.Scope.ValueString()
	}
	if !m.RemoveVanished.IsNull() && !m.RemoveVanished.IsUnknown() {
		body.RemoveVanished = m.RemoveVanished.ValueString()
	}
	if !m.EnableNew.IsNull() && !m.EnableNew.IsUnknown() {
		v := m.EnableNew.ValueBool()
		body.EnableNew = &v
	}
	if !m.Enabled.IsNull() && !m.Enabled.IsUnknown() {
		v := m.Enabled.ValueBool()
		body.Enabled = &v
	}
	if !m.Comment.IsNull() && !m.Comment.IsUnknown() {
		body.Comment = m.Comment.ValueString()
	}
	return body
}

// realmSyncJobIntoModel populates the model from a wire read. Absent
// upstream values become Terraform nulls so optional attributes do not
// churn between reads and plans.
func realmSyncJobIntoModel(m *realmSyncJobModel, job *pveclient.RealmSyncJob) {
	m.ID = types.StringValue(job.ID)
	m.Realm = types.StringValue(job.Realm)
	m.Schedule = types.StringValue(job.Schedule)
	m.Scope = realmStringToTF(job.Scope)
	m.RemoveVanished = realmStringToTF(job.RemoveVanished)
	m.EnableNew = realmBoolPtrToTF(job.EnableNew)
	m.Enabled = realmBoolPtrToTF(job.Enabled)
	m.Comment = realmStringToTF(job.Comment)
	m.LastRun = realmInt64PtrToTF(job.LastRun)
	m.NextRun = realmInt64PtrToTF(job.NextRun)
}

// realmInt64PtrToTF converts an optional wire timestamp to Terraform,
// mapping a nil pointer to null.
func realmInt64PtrToTF(v *int64) types.Int64 {
	if v == nil {
		return types.Int64Null()
	}
	return types.Int64Value(*v)
}
