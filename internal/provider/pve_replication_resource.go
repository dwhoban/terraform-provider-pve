// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/float64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ resource.Resource                = &pveReplicationResource{}
	_ resource.ResourceWithConfigure   = &pveReplicationResource{}
	_ resource.ResourceWithImportState = &pveReplicationResource{}
)

// replicationJobIDPattern is the pin's `pve-replication-job-id` format:
// a guest ID followed by a job number, hyphen-separated.
var replicationJobIDPattern = regexp.MustCompile(`^[1-9][0-9]{2,8}-[0-9]{1,9}$`)

// NewPveReplicationResource returns the resource implementation.
func NewPveReplicationResource() resource.Resource {
	return &pveReplicationResource{}
}

// pveReplicationResource manages a storage replication job via
// /cluster/replication.
type pveReplicationResource struct {
	client *pveclient.Client
}

// pveReplicationResourceModel is the Terraform-facing shape.
type pveReplicationResourceModel struct {
	ID       types.String  `tfsdk:"id"`
	Target   types.String  `tfsdk:"target"`
	Schedule types.String  `tfsdk:"schedule"`
	Rate     types.Float64 `tfsdk:"rate"`
	Comment  types.String  `tfsdk:"comment"`
	Disable  types.Bool    `tfsdk:"disable"`
	Type     types.String  `tfsdk:"type"`
	Guest    types.Int64   `tfsdk:"guest"`
	JobNum   types.Int64   `tfsdk:"jobnum"`
	Digest   types.String  `tfsdk:"digest"`
}

// Metadata implements resource.Resource.
func (r *pveReplicationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveReplication
}

// Schema implements resource.Resource.
func (r *pveReplicationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a storage replication job in the cluster configuration (`/cluster/replication`). " +
			"A replication job replicates a guest's volumes to another node; the guest binding travels in the job ID, " +
			"whose first component is the guest's VMID. Jobs run on PVE's own schedule; this resource manages the " +
			"configuration only and does not wait for syncs.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "Replication Job ID, composed of a Guest ID and a job number separated by a " +
					"hyphen, i.e. `<GUEST>-<JOBNUM>` (e.g. `100-0`). The guest ID binds the job to that guest. " +
					"Changing this value forces recreation.",
				Validators: []validator.String{
					stringvalidator.RegexMatches(replicationJobIDPattern,
						"must match <GUEST>-<JOBNUM>, e.g. `100-0` (guest ID of 3 to 9 digits, job number of 1 to 9 digits)"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"target": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "Target node that receives the replicated volumes. The pin's update verb carries " +
					"no target parameter, so changing this value forces recreation.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"schedule": schema.StringAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "Storage replication schedule as a subset of a `systemd` calendar event " +
					"(e.g. `*/15`, `Mon..Fri *-*-* 02:00`). PVE defaults to `*/15` when unset.",
			},
			"rate": schema.Float64Attribute{
				Optional: true,
				MarkdownDescription: "Rate limit in MB/s (megabytes per second) applied to the replication stream. " +
					"Must be at least 1. Removing the attribute from configuration clears the limit upstream.",
				Validators: []validator.Float64{
					float64validator.AtLeast(1),
				},
			},
			"comment": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Free-form description (up to 4096 characters). Removing it from configuration clears the comment upstream.",
			},
			"disable": schema.BoolAttribute{
				Optional: true,
				MarkdownDescription: "Flag to disable/deactivate the job without deleting it. Leaving the attribute " +
					"unset keeps the job enabled; removing a previously set `true` re-enables the job upstream.",
			},
			"type": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Section type as reported by PVE; always `local`.",
			},
			"guest": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Guest ID the job replicates, decoded by PVE from the job ID.",
			},
			"jobnum": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Job number component of the replication job ID.",
			},
			"digest": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Configuration digest of the job, usable to detect concurrent upstream modifications.",
			},
		},
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveReplicationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
func (r *pveReplicationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveReplicationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error creating pve_replication", "provider client is not configured")
		return
	}
	if err := r.client.CreateReplication(ctx, replicationJobFromModel(plan)); err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_replication",
			fmt.Sprintf("creating replication job %s: %s", plan.ID.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_replication after create",
			fmt.Sprintf("reading replication job %s: %s", plan.ID.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	tflog.Debug(ctx, "created PVE replication job", map[string]any{"id": plan.ID.ValueString()})
}

// Read implements resource.Resource.
func (r *pveReplicationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveReplicationResourceModel
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
			"Error reading pve_replication",
			fmt.Sprintf("reading replication job %s: %s", state.ID.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource.
func (r *pveReplicationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state pveReplicationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error updating pve_replication", "provider client is not configured")
		return
	}
	upd := replicationUpdateFromModels(plan, state)
	if err := r.client.UpdateReplication(ctx, plan.ID.ValueString(), upd); err != nil {
		resp.Diagnostics.AddError(
			"Error updating pve_replication",
			fmt.Sprintf("updating replication job %s: %s", plan.ID.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_replication after update",
			fmt.Sprintf("reading replication job %s: %s", plan.ID.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	tflog.Debug(ctx, "updated PVE replication job", map[string]any{"id": plan.ID.ValueString()})
}

// Delete implements resource.Resource.
func (r *pveReplicationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveReplicationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error deleting pve_replication", "provider client is not configured")
		return
	}
	tflog.Debug(ctx, "deleting PVE replication job", map[string]any{"id": state.ID.ValueString()})
	if err := r.client.DeleteReplication(ctx, state.ID.ValueString()); err != nil {
		if isPVEClientNotFound(err) {
			// Already absent counts as deleted.
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting pve_replication",
			fmt.Sprintf("deleting replication job %s: %s", state.ID.ValueString(), err),
		)
		return
	}
	tflog.Debug(ctx, "deleted PVE replication job", map[string]any{"id": state.ID.ValueString()})
}

// ImportState parses an import ID of the form `<id>` (e.g. `100-0`).
func (r *pveReplicationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		resp.Diagnostics.AddError("Invalid pve_replication import ID", "import ID must be the replication job ID, e.g. `100-0`")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

// readInto refreshes the model from PVE.
func (r *pveReplicationResource) readInto(ctx context.Context, m *pveReplicationResourceModel) error {
	job, err := r.client.GetReplication(ctx, m.ID.ValueString())
	if err != nil {
		return err
	}
	m.Target = types.StringValue(job.Target)
	m.Schedule = nodeNetworkStringToTF(job.Schedule)
	m.Rate = replicationFloatPtrToTF(job.Rate)
	m.Comment = nodeNetworkStringToTF(job.Comment)
	if job.Disable {
		m.Disable = types.BoolValue(true)
	} else {
		m.Disable = types.BoolNull()
	}
	m.Type = types.StringValue(job.Type)
	m.Guest = types.Int64Value(job.Guest)
	m.JobNum = types.Int64Value(job.JobNum)
	m.Digest = nodeNetworkStringToTF(job.Digest)
	return nil
}

// replicationJobFromModel projects the Terraform model into the create
// body. Unset optional fields are omitted so PVE applies its defaults.
func replicationJobFromModel(m pveReplicationResourceModel) pveclient.ReplicationJob {
	job := pveclient.ReplicationJob{
		ID:       m.ID.ValueString(),
		Target:   m.Target.ValueString(),
		Schedule: m.Schedule.ValueString(),
		Comment:  m.Comment.ValueString(),
		Disable:  m.Disable.ValueBool(),
	}
	if !m.Rate.IsNull() {
		rate := m.Rate.ValueFloat64()
		job.Rate = &rate
	}
	return job
}

// replicationUpdateFromModels builds the update body from the plan and
// state models: present plan values travel in the body, and optional
// settings that vanished from configuration travel in the PVE delete
// parameter instead (the schedule has a server-side default and is managed
// as Optional+Computed, so it never needs clearing).
func replicationUpdateFromModels(plan, state pveReplicationResourceModel) pveclient.ReplicationJobUpdate {
	upd := pveclient.ReplicationJobUpdate{
		Schedule: plan.Schedule.ValueString(),
		Comment:  plan.Comment.ValueString(),
		Disable:  plan.Disable.ValueBool(),
	}
	if !plan.Rate.IsNull() {
		rate := plan.Rate.ValueFloat64()
		upd.Rate = &rate
	}
	if plan.Rate.IsNull() && !state.Rate.IsNull() {
		upd.Delete = append(upd.Delete, "rate")
	}
	if plan.Comment.IsNull() && !state.Comment.IsNull() {
		upd.Delete = append(upd.Delete, "comment")
	}
	if plan.Disable.IsNull() && state.Disable.ValueBool() {
		upd.Delete = append(upd.Delete, "disable")
	}
	return upd
}

// replicationFloatPtrToTF converts an optional wire float to Terraform,
// mapping a nil pointer to null.
func replicationFloatPtrToTF(v *float64) types.Float64 {
	if v == nil {
		return types.Float64Null()
	}
	return types.Float64Value(*v)
}
