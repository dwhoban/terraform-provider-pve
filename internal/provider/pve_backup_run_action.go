// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/action"
	actionschema "github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ action.Action              = &pveBackupRunAction{}
	_ action.ActionWithConfigure = &pveBackupRunAction{}
)

// NewPveBackupRunAction returns the action implementation.
func NewPveBackupRunAction() action.Action {
	return &pveBackupRunAction{}
}

// pveBackupRunFleecingModel is the backup fleecing options block.
type pveBackupRunFleecingModel struct {
	Enabled types.Bool   `tfsdk:"enabled"`
	Storage types.String `tfsdk:"storage"`
}

// pveBackupRunPerformanceModel is the performance options block.
type pveBackupRunPerformanceModel struct {
	MaxWorkers    types.Int64 `tfsdk:"max_workers"`
	PBSEntriesMax types.Int64 `tfsdk:"pbs_entries_max"`
}

// pveBackupRunAction runs a backup with vzdump
// (POST /nodes/{node}/vzdump) and waits for the task to finish.
type pveBackupRunAction struct {
	client *pveclient.Client
}

// pveBackupRunPruneBackupsModel is the retention options block.
type pveBackupRunPruneBackupsModel struct {
	KeepAll     types.Bool  `tfsdk:"keep_all"`
	KeepHourly  types.Int64 `tfsdk:"keep_hourly"`
	KeepDaily   types.Int64 `tfsdk:"keep_daily"`
	KeepWeekly  types.Int64 `tfsdk:"keep_weekly"`
	KeepMonthly types.Int64 `tfsdk:"keep_monthly"`
	KeepYearly  types.Int64 `tfsdk:"keep_yearly"`
	KeepLast    types.Int64 `tfsdk:"keep_last"`
}

// pveBackupRunActionModel is the Terraform-facing shape.
type pveBackupRunActionModel struct {
	Node                   types.String                   `tfsdk:"node"`
	Mode                   types.String                   `tfsdk:"mode"`
	Compress               types.String                   `tfsdk:"compress"`
	Storage                types.String                   `tfsdk:"storage"`
	Dumpdir                types.String                   `tfsdk:"dumpdir"`
	Tmpdir                 types.String                   `tfsdk:"tmpdir"`
	Script                 types.String                   `tfsdk:"script"`
	JobID                  types.String                   `tfsdk:"job_id"`
	Pool                   types.String                   `tfsdk:"pool"`
	NotesTemplate          types.String                   `tfsdk:"notes_template"`
	Mailto                 types.String                   `tfsdk:"mailto"`
	Mailnotification       types.String                   `tfsdk:"mailnotification"`
	NotificationMode       types.String                   `tfsdk:"notification_mode"`
	PBSChangeDetectionMode types.String                   `tfsdk:"pbs_change_detection_mode"`
	VMIDs                  types.List                     `tfsdk:"vmid"`
	Exclude                types.List                     `tfsdk:"exclude"`
	ExcludePath            types.List                     `tfsdk:"exclude_path"`
	All                    types.Bool                     `tfsdk:"all"`
	BWLimit                types.Int64                    `tfsdk:"bwlimit"`
	IONice                 types.Int64                    `tfsdk:"ionice"`
	Lockwait               types.Int64                    `tfsdk:"lockwait"`
	Stopwait               types.Int64                    `tfsdk:"stopwait"`
	Pigz                   types.Int64                    `tfsdk:"pigz"`
	Zstd                   types.Int64                    `tfsdk:"zstd"`
	Stdexcludes            types.Bool                     `tfsdk:"stdexcludes"`
	Quiet                  types.Bool                     `tfsdk:"quiet"`
	Stop                   types.Bool                     `tfsdk:"stop"`
	Remove                 types.Bool                     `tfsdk:"remove"`
	Protected              types.Bool                     `tfsdk:"protected"`
	Stdout                 types.Bool                     `tfsdk:"stdout"`
	Fleecing               *pveBackupRunFleecingModel     `tfsdk:"fleecing"`
	Performance            *pveBackupRunPerformanceModel  `tfsdk:"performance"`
	PruneBackups           *pveBackupRunPruneBackupsModel `tfsdk:"prune_backups"`
}

// Metadata implements action.Action.
func (a *pveBackupRunAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveBackupRun
}

// Schema implements action.Action.
func (a *pveBackupRunAction) Schema(_ context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "Creates a backup with vzdump (`POST /nodes/{node}/vzdump`) and waits for the backup " +
			"task to finish. Set `vmid` to back up specific guests, or `all`/`pool` to back up groups of guests. " +
			"Requires the `VM.Backup` privilege on the backed-up guests and `Datastore.AllocateSpace` on the backup " +
			"storage; several tuning options require `Sys.Modify` on `/` and `job_id`, `dumpdir`, `tmpdir`, and `script` " +
			"are restricted to `root@pam` upstream.",
		Attributes: map[string]actionschema.Attribute{
			"node": &actionschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The cluster node running the backup; the task runs here.",
			},
			"mode": &actionschema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Backup mode. Must be one of: `snapshot`, `suspend`, `stop`. Defaults upstream to `snapshot`.",
				Validators: []validator.String{
					stringvalidator.OneOf("snapshot", "suspend", "stop"),
				},
			},
			"compress": &actionschema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Compression of the dump file. Must be one of: `0`, `1`, `gzip`, `lzo`, `zstd`. Defaults upstream to `0` (no compression).",
				Validators: []validator.String{
					stringvalidator.OneOf("0", "1", "gzip", "lzo", "zstd"),
				},
			},
			"storage": &actionschema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The backup storage the resulting file is stored on.",
			},
			"dumpdir": &actionschema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Directory the resulting files are written to (alternative to `storage`). Restricted to `root@pam` upstream.",
			},
			"tmpdir": &actionschema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Directory for temporary files. Restricted to `root@pam` upstream.",
			},
			"script": &actionschema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Hook script run at backup lifecycle events. Restricted to `root@pam` upstream.",
			},
			"job_id": &actionschema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Backup job ID recorded in the backup notification's `backup-job` metadata field. Restricted to `root@pam` upstream.",
			},
			"pool": &actionschema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Backs up all known guests included in this pool.",
			},
			"notes_template": &actionschema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Template for the backup notes; supports the `{{cluster}}`, `{{guestname}}`, `{{node}}`, and `{{vmid}}` variables. Requires `storage`.",
			},
			"mailto": &actionschema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Deprecated upstream in favor of notification targets/matchers: comma-separated list of email addresses or users receiving notifications.",
			},
			"mailnotification": &actionschema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Deprecated upstream in favor of notification targets/matchers: when to send a notification mail. Must be one of: `always`, `failure`.",
				Validators: []validator.String{
					stringvalidator.OneOf("always", "failure"),
				},
			},
			"notification_mode": &actionschema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Which notification system to use. Must be one of: `auto`, `legacy-sendmail`, `notification-system`. Defaults upstream to `auto`.",
				Validators: []validator.String{
					stringvalidator.OneOf("auto", "legacy-sendmail", "notification-system"),
				},
			},
			"pbs_change_detection_mode": &actionschema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "PBS mode used to detect file changes and switch encoding format for container backups. Must be one of: `legacy`, `data`, `metadata`.",
				Validators: []validator.String{
					stringvalidator.OneOf("legacy", "data", "metadata"),
				},
			},
			"vmid": &actionschema.ListAttribute{
				ElementType:         types.StringType,
				Optional:            true,
				MarkdownDescription: "Guest IDs to back up (sent on the wire as a comma-separated `pve-vmid-list`), e.g. `[\"100\", \"101\"]`.",
			},
			"exclude": &actionschema.ListAttribute{
				ElementType:         types.StringType,
				Optional:            true,
				MarkdownDescription: "Guest IDs to exclude (assumes `all = true`; sent on the wire as a comma-separated `pve-vmid-list`).",
			},
			"exclude_path": &actionschema.ListAttribute{
				ElementType:         types.StringType,
				Optional:            true,
				MarkdownDescription: "Shell globs of files/directories to exclude (container backups). Paths starting with `/` are anchored to the container's root, other paths match relative to each subdirectory.",
			},
			"all": &actionschema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Backs up all known guest systems on this host.",
			},
			"bwlimit": &actionschema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "I/O bandwidth limit in KiB/s. Must be 0 or greater; omit for no limit.",
				Validators: []validator.Int64{
					int64validator.AtLeast(0),
				},
			},
			"ionice": &actionschema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "IO priority when using the BFQ scheduler (affects the compressor for VM backups); `8` means idle priority. Must be between 0 and 8.",
				Validators: []validator.Int64{
					int64validator.Between(0, 8),
				},
			},
			"lockwait": &actionschema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Maximal time to wait for the global lock, in minutes. Must be 0 or greater.",
				Validators: []validator.Int64{
					int64validator.AtLeast(0),
				},
			},
			"stopwait": &actionschema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Maximal time to wait until a guest is stopped, in minutes. Must be 0 or greater.",
				Validators: []validator.Int64{
					int64validator.AtLeast(0),
				},
			},
			"pigz": &actionschema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Use pigz instead of gzip when greater than 0; `1` uses half the cores, higher values set the thread count.",
				Validators: []validator.Int64{
					int64validator.AtLeast(0),
				},
			},
			"zstd": &actionschema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Zstd thread count; `0` uses half the available cores. Must be 0 or greater.",
				Validators: []validator.Int64{
					int64validator.AtLeast(0),
				},
			},
			"stdexcludes": &actionschema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Excludes temporary files and logs. Defaults upstream to `true`.",
			},
			"quiet": &actionschema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Suppresses log output.",
			},
			"stop": &actionschema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Stops running backup jobs on this host before starting.",
			},
			"remove": &actionschema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Prunes older backups according to `prune_backups`. Defaults upstream to `true`.",
			},
			"protected": &actionschema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Marks the backup(s) as protected. Requires `storage`.",
			},
			"stdout": &actionschema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Writes the archive to stdout instead of a file; the provider discards the stream, so this is only useful with hook scripts.",
			},
			"fleecing": &actionschema.SingleNestedAttribute{
				Optional:            true,
				MarkdownDescription: "Options for backup fleecing (VM only).",
				Attributes: map[string]actionschema.Attribute{
					"enabled": &actionschema.BoolAttribute{
						Optional:            true,
						MarkdownDescription: "Enables fleecing for all equipped guests.",
					},
					"storage": &actionschema.StringAttribute{
						Optional:            true,
						MarkdownDescription: "Storage holding the fleecing images; if not set, the storage selected by the scheduler is used.",
					},
				},
			},
			"performance": &actionschema.SingleNestedAttribute{
				Optional:            true,
				MarkdownDescription: "Other performance-related settings.",
				Attributes: map[string]actionschema.Attribute{
					"max_workers": &actionschema.Int64Attribute{
						Optional:            true,
						MarkdownDescription: "Maximal number of parallel workers (per guest in stop/suspend mode, per drive in snapshot mode). Must be 1 or greater.",
						Validators: []validator.Int64{
							int64validator.AtLeast(1),
						},
					},
					"pbs_entries_max": &actionschema.Int64Attribute{
						Optional:            true,
						MarkdownDescription: "Maximal number of entries for PBS in-flight file entries. Must be 0 or greater.",
						Validators: []validator.Int64{
							int64validator.AtLeast(0),
						},
					},
				},
			},
			"prune_backups": &actionschema.SingleNestedAttribute{
				Optional:            true,
				MarkdownDescription: "Retention options overriding the storage configuration instead of it; `keep_all = true` conflicts with the other options.",
				Attributes: map[string]actionschema.Attribute{
					"keep_all": &actionschema.BoolAttribute{
						Optional:            true,
						MarkdownDescription: "Keeps all backups; conflicts with the other `keep_*` options.",
					},
					"keep_hourly": &actionschema.Int64Attribute{
						Optional:            true,
						MarkdownDescription: "Maximal number of hourly backups to keep. Must be 0 or greater.",
						Validators: []validator.Int64{
							int64validator.AtLeast(0),
						},
					},
					"keep_daily": &actionschema.Int64Attribute{
						Optional:            true,
						MarkdownDescription: "Maximal number of daily backups to keep. Must be 0 or greater.",
						Validators: []validator.Int64{
							int64validator.AtLeast(0),
						},
					},
					"keep_weekly": &actionschema.Int64Attribute{
						Optional:            true,
						MarkdownDescription: "Maximal number of weekly backups to keep. Must be 0 or greater.",
						Validators: []validator.Int64{
							int64validator.AtLeast(0),
						},
					},
					"keep_monthly": &actionschema.Int64Attribute{
						Optional:            true,
						MarkdownDescription: "Maximal number of monthly backups to keep. Must be 0 or greater.",
						Validators: []validator.Int64{
							int64validator.AtLeast(0),
						},
					},
					"keep_yearly": &actionschema.Int64Attribute{
						Optional:            true,
						MarkdownDescription: "Maximal number of yearly backups to keep. Must be 0 or greater.",
						Validators: []validator.Int64{
							int64validator.AtLeast(0),
						},
					},
					"keep_last": &actionschema.Int64Attribute{
						Optional:            true,
						MarkdownDescription: "Maximal number of most recent backups to keep. Must be 0 or greater.",
						Validators: []validator.Int64{
							int64validator.AtLeast(0),
						},
					},
				},
			},
		},
	}
}

// Configure implements action.ActionWithConfigure.
func (a *pveBackupRunAction) Configure(ctx context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	a.client = containerActionConfigure(req, resp)
}

// Invoke implements action.Action.
func (a *pveBackupRunAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var config pveBackupRunActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if a.client == nil {
		resp.Diagnostics.AddError(
			"Unconfigured pve_backup_run",
			fmt.Sprintf("The provider client was not configured; cannot start backup on %s.", config.Node.ValueString()),
		)
		return
	}
	node := config.Node.ValueString()
	opts := pveclient.VzdumpRunOptions{
		Mode:                   backupJobOptionalString(config.Mode),
		Compress:               backupJobOptionalString(config.Compress),
		Storage:                backupJobOptionalString(config.Storage),
		DumpDir:                backupJobOptionalString(config.Dumpdir),
		TmpDir:                 backupJobOptionalString(config.Tmpdir),
		Script:                 backupJobOptionalString(config.Script),
		JobID:                  backupJobOptionalString(config.JobID),
		Pool:                   backupJobOptionalString(config.Pool),
		NotesTemplate:          backupJobOptionalString(config.NotesTemplate),
		MailTo:                 backupJobOptionalString(config.Mailto),
		MailNotification:       backupJobOptionalString(config.Mailnotification),
		NotificationMode:       backupJobOptionalString(config.NotificationMode),
		PBSChangeDetectionMode: backupJobOptionalString(config.PBSChangeDetectionMode),
		VMIDs:                  listStringFromTF(config.VMIDs),
		Exclude:                listStringFromTF(config.Exclude),
		ExcludePath:            listStringFromTF(config.ExcludePath),
		All:                    backupJobOptionalBool(config.All),
		BWLimit:                backupJobOptionalInt64(config.BWLimit),
		IONice:                 backupJobOptionalInt64(config.IONice),
		LockWait:               backupJobOptionalInt64(config.Lockwait),
		StopWait:               backupJobOptionalInt64(config.Stopwait),
		Pigz:                   backupJobOptionalInt64(config.Pigz),
		Zstd:                   backupJobOptionalInt64(config.Zstd),
		StdExcludes:            backupJobOptionalBool(config.Stdexcludes),
		Quiet:                  backupJobOptionalBool(config.Quiet),
		Stop:                   backupJobOptionalBool(config.Stop),
		Remove:                 backupJobOptionalBool(config.Remove),
		Protected:              backupJobOptionalBool(config.Protected),
		Stdout:                 backupJobOptionalBool(config.Stdout),
	}
	if config.Fleecing != nil {
		opts.Fleecing = &pveclient.BackupFleecing{
			Enabled: backupJobOptionalBool(config.Fleecing.Enabled),
			Storage: backupJobOptionalString(config.Fleecing.Storage),
		}
	}
	if config.Performance != nil {
		opts.Performance = &pveclient.BackupPerformance{
			MaxWorkers:    backupJobOptionalInt64(config.Performance.MaxWorkers),
			PBSEntriesMax: backupJobOptionalInt64(config.Performance.PBSEntriesMax),
		}
	}
	if config.PruneBackups != nil {
		opts.PruneBackups = &pveclient.BackupPruneBackups{
			KeepAll:     backupJobOptionalBool(config.PruneBackups.KeepAll),
			KeepHourly:  backupJobOptionalInt64(config.PruneBackups.KeepHourly),
			KeepDaily:   backupJobOptionalInt64(config.PruneBackups.KeepDaily),
			KeepWeekly:  backupJobOptionalInt64(config.PruneBackups.KeepWeekly),
			KeepMonthly: backupJobOptionalInt64(config.PruneBackups.KeepMonthly),
			KeepYearly:  backupJobOptionalInt64(config.PruneBackups.KeepYearly),
			KeepLast:    backupJobOptionalInt64(config.PruneBackups.KeepLast),
		}
	}
	progress := func(message string) {
		if resp.SendProgress != nil {
			resp.SendProgress(action.InvokeProgressEvent{Message: message})
		}
	}
	tflog.Info(ctx, "starting vzdump backup", map[string]any{"node": node, "storage": opts.Storage})
	upid, err := a.client.RunVzdumpBackup(ctx, node, opts)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error invoking pve_backup_run",
			fmt.Sprintf("starting backup on node %s: %s", node, err),
		)
		return
	}
	progress(fmt.Sprintf("Backup started on node %s; waiting for the vzdump task to finish", node))
	if err := vmWaitForTask(ctx, a.client, upid, progress); err != nil {
		resp.Diagnostics.AddError(
			"Error waiting for pve_backup_run",
			fmt.Sprintf("waiting for backup on node %s: %s", node, err),
		)
		return
	}
	progress(fmt.Sprintf("Backup on node %s finished", node))
}
