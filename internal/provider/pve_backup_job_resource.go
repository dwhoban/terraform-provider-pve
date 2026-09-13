// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
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
	_ resource.Resource                = &pveBackupJobResource{}
	_ resource.ResourceWithConfigure   = &pveBackupJobResource{}
	_ resource.ResourceWithImportState = &pveBackupJobResource{}
)

// NewPveBackupJobResource returns the resource implementation.
func NewPveBackupJobResource() resource.Resource {
	return &pveBackupJobResource{}
}

// pveBackupJobResource manages a vzdump backup job via /cluster/backup.
// Every mutation is synchronous per the pin (no task is spawned).
type pveBackupJobResource struct {
	client *pveclient.Client
}

// pveBackupJobFleecingModel is the fleecing options block (VM only).
type pveBackupJobFleecingModel struct {
	Enabled types.Bool   `tfsdk:"enabled"`
	Storage types.String `tfsdk:"storage"`
}

// pveBackupJobPerformanceModel is the performance options block.
type pveBackupJobPerformanceModel struct {
	MaxWorkers    types.Int64 `tfsdk:"max_workers"`
	PBSEntriesMax types.Int64 `tfsdk:"pbs_entries_max"`
}

// pveBackupJobPruneBackupsModel is the retention options block.
type pveBackupJobPruneBackupsModel struct {
	KeepAll     types.Bool  `tfsdk:"keep_all"`
	KeepHourly  types.Int64 `tfsdk:"keep_hourly"`
	KeepDaily   types.Int64 `tfsdk:"keep_daily"`
	KeepWeekly  types.Int64 `tfsdk:"keep_weekly"`
	KeepMonthly types.Int64 `tfsdk:"keep_monthly"`
	KeepYearly  types.Int64 `tfsdk:"keep_yearly"`
	KeepLast    types.Int64 `tfsdk:"keep_last"`
}

// pveBackupJobResourceModel is the Terraform-facing shape.
type pveBackupJobResourceModel struct {
	ID                     types.String                   `tfsdk:"id"`
	Node                   types.String                   `tfsdk:"node"`
	Comment                types.String                   `tfsdk:"comment"`
	Schedule               types.String                   `tfsdk:"schedule"`
	Enabled                types.Bool                     `tfsdk:"enabled"`
	Mode                   types.String                   `tfsdk:"mode"`
	Compress               types.String                   `tfsdk:"compress"`
	Storage                types.String                   `tfsdk:"storage"`
	Dumpdir                types.String                   `tfsdk:"dumpdir"`
	Tmpdir                 types.String                   `tfsdk:"tmpdir"`
	Script                 types.String                   `tfsdk:"script"`
	All                    types.Bool                     `tfsdk:"all"`
	VMIDs                  types.List                     `tfsdk:"vmid"`
	Exclude                types.List                     `tfsdk:"exclude"`
	ExcludePath            types.List                     `tfsdk:"exclude_path"`
	Pool                   types.String                   `tfsdk:"pool"`
	NotesTemplate          types.String                   `tfsdk:"notes_template"`
	Mailto                 types.String                   `tfsdk:"mailto"`
	Mailnotification       types.String                   `tfsdk:"mailnotification"`
	NotificationMode       types.String                   `tfsdk:"notification_mode"`
	PBSChangeDetectionMode types.String                   `tfsdk:"pbs_change_detection_mode"`
	BWLimit                types.Int64                    `tfsdk:"bwlimit"`
	Zstd                   types.Int64                    `tfsdk:"zstd"`
	Pigz                   types.Int64                    `tfsdk:"pigz"`
	IONice                 types.Int64                    `tfsdk:"ionice"`
	Lockwait               types.Int64                    `tfsdk:"lockwait"`
	Stopwait               types.Int64                    `tfsdk:"stopwait"`
	Stdexcludes            types.Bool                     `tfsdk:"stdexcludes"`
	Quiet                  types.Bool                     `tfsdk:"quiet"`
	Stop                   types.Bool                     `tfsdk:"stop"`
	Remove                 types.Bool                     `tfsdk:"remove"`
	RepeatMissed           types.Bool                     `tfsdk:"repeat_missed"`
	Protected              types.Bool                     `tfsdk:"protected"`
	Fleecing               *pveBackupJobFleecingModel     `tfsdk:"fleecing"`
	Performance            *pveBackupJobPerformanceModel  `tfsdk:"performance"`
	PruneBackups           *pveBackupJobPruneBackupsModel `tfsdk:"prune_backups"`
	NextRun                types.Int64                    `tfsdk:"next_run"`
}

// Metadata implements resource.Resource.
func (r *pveBackupJobResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveBackupJob
}

// Schema implements resource.Resource.
func (r *pveBackupJobResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a vzdump backup job (`/cluster/backup/{id}`). Attributes left unset are not sent and can be cleared again on update (the provider translates removals into PVE's `delete` parameter); PVE applies its own defaults for unset options. Every mutation is synchronous per the pin (no task is spawned).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The job ID (PVE `pve-configid` format). Changing this value forces recreation.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"node": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Only run if executed on this node.",
			},
			"comment": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Description for the job.",
			},
			"schedule": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Backup schedule in the systemd calendar-event subset PVE accepts (PVE `pve-calendar-event` format, e.g. `mon..fri 02:00` or `*-*-* 00:00:00`). Free-form: PVE validates the value on write.",
			},
			"enabled": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Enable or disable the job. PVE enables new jobs by default.",
			},
			"mode": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Backup mode. Must be one of: `snapshot`, `suspend`, `stop`. PVE defaults to `snapshot`.",
				Validators: []validator.String{
					stringvalidator.OneOf("snapshot", "suspend", "stop"),
				},
			},
			"compress": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Compress dump file. Must be one of: `0`, `1`, `gzip`, `lzo`, `zstd`. PVE defaults to `0` (no compression).",
				Validators: []validator.String{
					stringvalidator.OneOf("0", "1", "gzip", "lzo", "zstd"),
				},
			},
			"storage": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Store resulting file to this storage.",
			},
			"dumpdir": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Store resulting files to specified directory. Use of this attribute is restricted to `root@pam` on the API side.",
			},
			"tmpdir": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Store temporary files to specified directory. Use of this attribute is restricted to `root@pam` on the API side.",
			},
			"script": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Use specified hook script. Use of this attribute is restricted to `root@pam` on the API side.",
			},
			"all": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Backup all known guest systems on this host.",
			},
			"vmid": schema.ListAttribute{
				ElementType:         types.StringType,
				Optional:            true,
				MarkdownDescription: "Ordered list of guest IDs to back up (sent on the wire as a comma-separated `pve-vmid-list`).",
			},
			"exclude": schema.ListAttribute{
				ElementType:         types.StringType,
				Optional:            true,
				MarkdownDescription: "Ordered list of guest IDs to exclude (assumes `all = true`; sent on the wire as a comma-separated `pve-vmid-list`).",
			},
			"exclude_path": schema.ListAttribute{
				ElementType:         types.StringType,
				Optional:            true,
				MarkdownDescription: "Ordered list of shell globs to exclude (container backups). Paths starting with `/` are anchored to the container's root, other paths match relative to each subdirectory.",
			},
			"pool": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Backup all known guest systems included in the specified pool.",
			},
			"notes_template": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Template string for generating notes for the backup(s). Supports the `{{cluster}}`, `{{guestname}}`, `{{node}}`, and `{{vmid}}` variables; must be a single line (escape newline and backslash as `\\n` and `\\\\`). Requires `storage` to be set.",
			},
			"mailto": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Deprecated by PVE in favor of notification targets/matchers. Comma-separated list of email addresses or users that should receive email notifications.",
			},
			"mailnotification": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Deprecated by PVE in favor of notification targets/matchers. Specify when to send a notification mail. Must be one of: `always`, `failure`.",
				Validators: []validator.String{
					stringvalidator.OneOf("always", "failure"),
				},
			},
			"notification_mode": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Determine which notification system to use. Must be one of: `auto`, `legacy-sendmail`, `notification-system`. PVE defaults to `auto`.",
				Validators: []validator.String{
					stringvalidator.OneOf("auto", "legacy-sendmail", "notification-system"),
				},
			},
			"pbs_change_detection_mode": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "PBS mode used to detect file changes and switch encoding format for container backups. Must be one of: `legacy`, `data`, `metadata`.",
				Validators: []validator.String{
					stringvalidator.OneOf("legacy", "data", "metadata"),
				},
			},
			"bwlimit": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Limit I/O bandwidth in KiB/s. Must be 0 or greater.",
				Validators: []validator.Int64{
					int64validator.AtLeast(0),
				},
			},
			"zstd": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Zstd threads. `0` uses half of the available cores; a value greater than `0` is used as the thread count.",
			},
			"pigz": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Use pigz instead of gzip when N > 0. `1` uses half of the cores, N > 1 uses N as thread count.",
			},
			"ionice": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "IO priority when using the BFQ scheduler; for VM snapshot/suspend mode this only affects the compressor. Must be between 0 and 8 inclusive (8 means idle priority).",
				Validators: []validator.Int64{
					int64validator.Between(0, 8),
				},
			},
			"lockwait": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Maximal time to wait for the global lock in minutes. Must be 0 or greater.",
				Validators: []validator.Int64{
					int64validator.AtLeast(0),
				},
			},
			"stopwait": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Maximal time to wait until a guest system is stopped in minutes. Must be 0 or greater.",
				Validators: []validator.Int64{
					int64validator.AtLeast(0),
				},
			},
			"stdexcludes": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Exclude temporary files and logs.",
			},
			"quiet": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Be quiet (suppress log output on the node).",
			},
			"stop": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Stop running backup jobs on this host.",
			},
			"remove": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Prune older backups according to `prune_backups`. PVE defaults to `true`.",
			},
			"repeat_missed": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Run the job as soon as possible if it was missed while the scheduler was not running.",
			},
			"protected": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Mark backup(s) as protected. Requires `storage` to be set.",
			},
			"fleecing": schema.SingleNestedAttribute{
				Optional:            true,
				MarkdownDescription: "Options for backup fleecing (VM only). Writing the block replaces the whole group on the PVE side.",
				Attributes: map[string]schema.Attribute{
					"enabled": schema.BoolAttribute{
						Optional:            true,
						MarkdownDescription: "Enable backup fleecing: cache backup data from blocks where new guest writes happen on the specified storage instead of copying them directly to the backup target. Improves guest IO at the cost of additional storage.",
					},
					"storage": schema.StringAttribute{
						Optional:            true,
						MarkdownDescription: "Storage for fleecing images. Best on a local storage that supports discard and either thin provisioning or sparse files.",
					},
				},
			},
			"performance": schema.SingleNestedAttribute{
				Optional:            true,
				MarkdownDescription: "Other performance-related settings. Writing the block replaces the whole group on the PVE side.",
				Attributes: map[string]schema.Attribute{
					"max_workers": schema.Int64Attribute{
						Optional:            true,
						MarkdownDescription: "Applies to VMs: allow up to this many IO workers at the same time. Must be between 1 and 256 inclusive.",
						Validators: []validator.Int64{
							int64validator.Between(1, 256),
						},
					},
					"pbs_entries_max": schema.Int64Attribute{
						Optional:            true,
						MarkdownDescription: "Applies to container backups sent to PBS: limits the number of entries allowed in memory at a given time to avoid OOM situations. Must be 1 or greater.",
						Validators: []validator.Int64{
							int64validator.AtLeast(1),
						},
					},
				},
			},
			"prune_backups": schema.SingleNestedAttribute{
				Optional:            true,
				MarkdownDescription: "Use these retention options instead of those from the storage configuration. Writing the block replaces the whole group on the PVE side. `keep_all = true` conflicts with the other options.",
				Attributes: map[string]schema.Attribute{
					"keep_all": schema.BoolAttribute{
						Optional:            true,
						MarkdownDescription: "Keep all backups. Conflicts with the other options when true.",
					},
					"keep_hourly": schema.Int64Attribute{
						Optional:            true,
						MarkdownDescription: "Keep backups for the last N different hours. Must be 0 or greater.",
						Validators: []validator.Int64{
							int64validator.AtLeast(0),
						},
					},
					"keep_daily": schema.Int64Attribute{
						Optional:            true,
						MarkdownDescription: "Keep backups for the last N different days. Must be 0 or greater.",
						Validators: []validator.Int64{
							int64validator.AtLeast(0),
						},
					},
					"keep_weekly": schema.Int64Attribute{
						Optional:            true,
						MarkdownDescription: "Keep backups for the last N different weeks. Must be 0 or greater.",
						Validators: []validator.Int64{
							int64validator.AtLeast(0),
						},
					},
					"keep_monthly": schema.Int64Attribute{
						Optional:            true,
						MarkdownDescription: "Keep backups for the last N different months. Must be 0 or greater.",
						Validators: []validator.Int64{
							int64validator.AtLeast(0),
						},
					},
					"keep_yearly": schema.Int64Attribute{
						Optional:            true,
						MarkdownDescription: "Keep backups for the last N different years. Must be 0 or greater.",
						Validators: []validator.Int64{
							int64validator.AtLeast(0),
						},
					},
					"keep_last": schema.Int64Attribute{
						Optional:            true,
						MarkdownDescription: "Keep the last N backups. Must be 0 or greater.",
						Validators: []validator.Int64{
							int64validator.AtLeast(0),
						},
					},
				},
			},
			"next_run": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "UNIX timestamp when this backup job will be executed next.",
			},
		},
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveBackupJobResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = haConfigureResource(req, resp)
}

// Create implements resource.Resource.
func (r *pveBackupJobResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveBackupJobResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error creating pve_backup_job", "provider client is not configured")
		return
	}
	if err := r.client.CreateBackupJob(ctx, backupJobFromModel(plan)); err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_backup_job",
			fmt.Sprintf("creating backup job %s: %s", plan.ID.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_backup_job after create",
			fmt.Sprintf("reading backup job %s: %s", plan.ID.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read implements resource.Resource.
func (r *pveBackupJobResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveBackupJobResourceModel
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
			"Error reading pve_backup_job",
			fmt.Sprintf("reading backup job %s: %s", state.ID.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource.
func (r *pveBackupJobResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveBackupJobResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state pveBackupJobResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	deleteFields := backupJobDeleteFields(plan, state)
	if err := r.client.UpdateBackupJob(ctx, plan.ID.ValueString(), backupJobFromModel(plan), deleteFields); err != nil {
		resp.Diagnostics.AddError(
			"Error updating pve_backup_job",
			fmt.Sprintf("updating backup job %s: %s", plan.ID.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_backup_job after update",
			fmt.Sprintf("reading backup job %s: %s", plan.ID.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete implements resource.Resource.
func (r *pveBackupJobResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveBackupJobResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteBackupJob(ctx, state.ID.ValueString()); err != nil {
		if isPVEClientNotFound(err) {
			// Already absent counts as deleted.
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting pve_backup_job",
			fmt.Sprintf("deleting backup job %s: %s", state.ID.ValueString(), err),
		)
	}
}

// ImportState parses an import ID of the form `<id>`.
func (r *pveBackupJobResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		resp.Diagnostics.AddError("Invalid pve_backup_job import ID", "import ID must be the backup job identifier, e.g. `daily`")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

// readInto refreshes the model from PVE.
func (r *pveBackupJobResource) readInto(ctx context.Context, m *pveBackupJobResourceModel) error {
	job, err := r.client.GetBackupJob(ctx, m.ID.ValueString())
	if err != nil {
		return err
	}
	backupJobReadFill(m, job)
	return nil
}

// backupJobReadFill projects a wire job onto the resource model.
func backupJobReadFill(m *pveBackupJobResourceModel, job *pveclient.BackupJob) {
	m.ID = types.StringValue(job.ID)
	m.Node = nodeNetworkStringToTF(job.Node)
	m.Comment = nodeNetworkStringToTF(job.Comment)
	m.Schedule = nodeNetworkStringToTF(job.Schedule)
	m.Enabled = nodeNetworkBoolPtrToTF(job.Enabled)
	m.Mode = nodeNetworkStringToTF(job.Mode)
	m.Compress = nodeNetworkStringToTF(job.Compress)
	m.Storage = nodeNetworkStringToTF(job.Storage)
	m.Dumpdir = nodeNetworkStringToTF(job.DumpDir)
	m.Tmpdir = nodeNetworkStringToTF(job.TmpDir)
	m.Script = nodeNetworkStringToTF(job.Script)
	m.All = nodeNetworkBoolPtrToTF(job.All)
	m.VMIDs = backupJobListToTF(job.VMIDs)
	m.Exclude = backupJobListToTF(job.Exclude)
	m.ExcludePath = backupJobListToTF(job.ExcludePath)
	m.Pool = nodeNetworkStringToTF(job.Pool)
	m.NotesTemplate = nodeNetworkStringToTF(job.NotesTemplate)
	m.Mailto = nodeNetworkStringToTF(job.MailTo)
	m.Mailnotification = nodeNetworkStringToTF(job.MailNotification)
	m.NotificationMode = nodeNetworkStringToTF(job.NotificationMode)
	m.PBSChangeDetectionMode = nodeNetworkStringToTF(job.PBSChangeDetectionMode)
	m.BWLimit = haInt64PtrToTF(job.BWLimit)
	m.Zstd = haInt64PtrToTF(job.Zstd)
	m.Pigz = haInt64PtrToTF(job.Pigz)
	m.IONice = haInt64PtrToTF(job.IONice)
	m.Lockwait = haInt64PtrToTF(job.LockWait)
	m.Stopwait = haInt64PtrToTF(job.StopWait)
	m.Stdexcludes = nodeNetworkBoolPtrToTF(job.StdExcludes)
	m.Quiet = nodeNetworkBoolPtrToTF(job.Quiet)
	m.Stop = nodeNetworkBoolPtrToTF(job.Stop)
	m.Remove = nodeNetworkBoolPtrToTF(job.Remove)
	m.RepeatMissed = nodeNetworkBoolPtrToTF(job.RepeatMissed)
	m.Protected = nodeNetworkBoolPtrToTF(job.Protected)
	m.NextRun = haInt64PtrToTF(job.NextRun)
	if job.Fleecing == nil {
		m.Fleecing = nil
	} else {
		m.Fleecing = &pveBackupJobFleecingModel{
			Enabled: nodeNetworkBoolPtrToTF(job.Fleecing.Enabled),
			Storage: nodeNetworkStringToTF(job.Fleecing.Storage),
		}
	}
	if job.Performance == nil {
		m.Performance = nil
	} else {
		m.Performance = &pveBackupJobPerformanceModel{
			MaxWorkers:    haInt64PtrToTF(job.Performance.MaxWorkers),
			PBSEntriesMax: haInt64PtrToTF(job.Performance.PBSEntriesMax),
		}
	}
	if job.PruneBackups == nil {
		m.PruneBackups = nil
	} else {
		m.PruneBackups = &pveBackupJobPruneBackupsModel{
			KeepAll:     nodeNetworkBoolPtrToTF(job.PruneBackups.KeepAll),
			KeepHourly:  haInt64PtrToTF(job.PruneBackups.KeepHourly),
			KeepDaily:   haInt64PtrToTF(job.PruneBackups.KeepDaily),
			KeepWeekly:  haInt64PtrToTF(job.PruneBackups.KeepWeekly),
			KeepMonthly: haInt64PtrToTF(job.PruneBackups.KeepMonthly),
			KeepYearly:  haInt64PtrToTF(job.PruneBackups.KeepYearly),
			KeepLast:    haInt64PtrToTF(job.PruneBackups.KeepLast),
		}
	}
}

// backupJobFromModel projects the Terraform model into the wire body.
func backupJobFromModel(m pveBackupJobResourceModel) pveclient.BackupJob {
	job := pveclient.BackupJob{
		ID:                     m.ID.ValueString(),
		Node:                   backupJobOptionalString(m.Node),
		Comment:                backupJobOptionalString(m.Comment),
		Schedule:               backupJobOptionalString(m.Schedule),
		Mode:                   backupJobOptionalString(m.Mode),
		Compress:               backupJobOptionalString(m.Compress),
		Storage:                backupJobOptionalString(m.Storage),
		DumpDir:                backupJobOptionalString(m.Dumpdir),
		TmpDir:                 backupJobOptionalString(m.Tmpdir),
		Script:                 backupJobOptionalString(m.Script),
		Pool:                   backupJobOptionalString(m.Pool),
		NotesTemplate:          backupJobOptionalString(m.NotesTemplate),
		MailTo:                 backupJobOptionalString(m.Mailto),
		MailNotification:       backupJobOptionalString(m.Mailnotification),
		NotificationMode:       backupJobOptionalString(m.NotificationMode),
		PBSChangeDetectionMode: backupJobOptionalString(m.PBSChangeDetectionMode),
		VMIDs:                  listStringFromTF(m.VMIDs),
		Exclude:                listStringFromTF(m.Exclude),
		ExcludePath:            listStringFromTF(m.ExcludePath),
	}
	job.Enabled = backupJobOptionalBool(m.Enabled)
	job.All = backupJobOptionalBool(m.All)
	job.StdExcludes = backupJobOptionalBool(m.Stdexcludes)
	job.Quiet = backupJobOptionalBool(m.Quiet)
	job.Stop = backupJobOptionalBool(m.Stop)
	job.Remove = backupJobOptionalBool(m.Remove)
	job.RepeatMissed = backupJobOptionalBool(m.RepeatMissed)
	job.Protected = backupJobOptionalBool(m.Protected)
	job.BWLimit = backupJobOptionalInt64(m.BWLimit)
	job.Zstd = backupJobOptionalInt64(m.Zstd)
	job.Pigz = backupJobOptionalInt64(m.Pigz)
	job.IONice = backupJobOptionalInt64(m.IONice)
	job.LockWait = backupJobOptionalInt64(m.Lockwait)
	job.StopWait = backupJobOptionalInt64(m.Stopwait)
	if m.Fleecing != nil {
		job.Fleecing = &pveclient.BackupFleecing{
			Enabled: backupJobOptionalBool(m.Fleecing.Enabled),
			Storage: backupJobOptionalString(m.Fleecing.Storage),
		}
	}
	if m.Performance != nil {
		job.Performance = &pveclient.BackupPerformance{
			MaxWorkers:    backupJobOptionalInt64(m.Performance.MaxWorkers),
			PBSEntriesMax: backupJobOptionalInt64(m.Performance.PBSEntriesMax),
		}
	}
	if m.PruneBackups != nil {
		job.PruneBackups = &pveclient.BackupPruneBackups{
			KeepAll:     backupJobOptionalBool(m.PruneBackups.KeepAll),
			KeepHourly:  backupJobOptionalInt64(m.PruneBackups.KeepHourly),
			KeepDaily:   backupJobOptionalInt64(m.PruneBackups.KeepDaily),
			KeepWeekly:  backupJobOptionalInt64(m.PruneBackups.KeepWeekly),
			KeepMonthly: backupJobOptionalInt64(m.PruneBackups.KeepMonthly),
			KeepYearly:  backupJobOptionalInt64(m.PruneBackups.KeepYearly),
			KeepLast:    backupJobOptionalInt64(m.PruneBackups.KeepLast),
		}
	}
	return job
}

// backupJobOptionalString returns s unless it is null or unknown.
func backupJobOptionalString(v types.String) string {
	if v.IsNull() || v.IsUnknown() {
		return ""
	}
	return v.ValueString()
}

// backupJobOptionalBool returns a pointer unless the value is null or unknown.
func backupJobOptionalBool(v types.Bool) *bool {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	return pveclient.BackupBoolPtr(v.ValueBool())
}

// backupJobOptionalInt64 returns a pointer unless the value is null or unknown.
func backupJobOptionalInt64(v types.Int64) *int64 {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	return pveclient.BackupInt64Ptr(v.ValueInt64())
}

// backupJobListToTF maps a wire list to Terraform, mapping empty lists to
// null so unset attributes stay unset.
func backupJobListToTF(in []string) types.List {
	if len(in) == 0 {
		return types.ListNull(types.StringType)
	}
	return listStringToTF(in)
}

// backupJobCleared reports whether an optional attribute is set in state
// but null in the plan (a removal the API must be told about).
func backupJobCleared(plan, state attr.Value) bool {
	return plan.IsNull() && !state.IsNull()
}

// backupJobDeleteFields returns the PVE field names to clear on update:
// optional attributes present in state but null in plan.
func backupJobDeleteFields(plan, state pveBackupJobResourceModel) []string {
	var out []string
	add := func(wire string, p, s attr.Value) {
		if backupJobCleared(p, s) {
			out = append(out, wire)
		}
	}
	add("node", plan.Node, state.Node)
	add("comment", plan.Comment, state.Comment)
	add("schedule", plan.Schedule, state.Schedule)
	add("enabled", plan.Enabled, state.Enabled)
	add("mode", plan.Mode, state.Mode)
	add("compress", plan.Compress, state.Compress)
	add("storage", plan.Storage, state.Storage)
	add("dumpdir", plan.Dumpdir, state.Dumpdir)
	add("tmpdir", plan.Tmpdir, state.Tmpdir)
	add("script", plan.Script, state.Script)
	add("all", plan.All, state.All)
	add("vmid", plan.VMIDs, state.VMIDs)
	add("exclude", plan.Exclude, state.Exclude)
	add("exclude-path", plan.ExcludePath, state.ExcludePath)
	add("pool", plan.Pool, state.Pool)
	add("notes-template", plan.NotesTemplate, state.NotesTemplate)
	add("mailto", plan.Mailto, state.Mailto)
	add("mailnotification", plan.Mailnotification, state.Mailnotification)
	add("notification-mode", plan.NotificationMode, state.NotificationMode)
	add("pbs-change-detection-mode", plan.PBSChangeDetectionMode, state.PBSChangeDetectionMode)
	add("bwlimit", plan.BWLimit, state.BWLimit)
	add("zstd", plan.Zstd, state.Zstd)
	add("pigz", plan.Pigz, state.Pigz)
	add("ionice", plan.IONice, state.IONice)
	add("lockwait", plan.Lockwait, state.Lockwait)
	add("stopwait", plan.Stopwait, state.Stopwait)
	add("stdexcludes", plan.Stdexcludes, state.Stdexcludes)
	add("quiet", plan.Quiet, state.Quiet)
	add("stop", plan.Stop, state.Stop)
	add("remove", plan.Remove, state.Remove)
	add("repeat-missed", plan.RepeatMissed, state.RepeatMissed)
	add("protected", plan.Protected, state.Protected)
	if plan.Fleecing == nil && state.Fleecing != nil {
		out = append(out, "fleecing")
	}
	if plan.Performance == nil && state.Performance != nil {
		out = append(out, "performance")
	}
	if plan.PruneBackups == nil && state.PruneBackups != nil {
		out = append(out, "prune-backups")
	}
	return out
}
