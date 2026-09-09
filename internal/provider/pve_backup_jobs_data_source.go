// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// Ensure the framework interfaces are satisfied.
var (
	_ datasource.DataSource              = &pveBackupJobsDataSource{}
	_ datasource.DataSourceWithConfigure = &pveBackupJobsDataSource{}
)

// NewPveBackupJobsDataSource returns the data source implementation.
func NewPveBackupJobsDataSource() datasource.DataSource {
	return &pveBackupJobsDataSource{}
}

// pveBackupJobsDataSource lists all vzdump backup jobs
// (GET /cluster/backup), the guests not covered by any job
// (GET /cluster/backup-info/not-backed-up), and — when `node` is set —
// the effective vzdump defaults of that node
// (GET /nodes/{node}/vzdump/defaults).
type pveBackupJobsDataSource struct {
	client *pveclient.Client
}

// pveBackupJobsDataSourceRowModel mirrors one job row of the
// /cluster/backup listing.
type pveBackupJobsDataSourceRowModel struct {
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

// pveBackupJobsDataSourceNotBackedUpModel mirrors one entry of the
// not-backed-up listing.
type pveBackupJobsDataSourceNotBackedUpModel struct {
	VMID types.Int64  `tfsdk:"vmid"`
	Name types.String `tfsdk:"name"`
	Type types.String `tfsdk:"type"`
}

// pveBackupJobsDataSourceVzdumpDefaultsModel mirrors the typed response of
// GET /nodes/{node}/vzdump/defaults. The grouped options arrive as
// property strings on this endpoint and are surfaced verbatim.
type pveBackupJobsDataSourceVzdumpDefaultsModel struct {
	All                    types.Bool   `tfsdk:"all"`
	BWLimit                types.Int64  `tfsdk:"bwlimit"`
	Compress               types.String `tfsdk:"compress"`
	Dumpdir                types.String `tfsdk:"dumpdir"`
	Exclude                types.String `tfsdk:"exclude"`
	ExcludePath            types.List   `tfsdk:"exclude_path"`
	Fleecing               types.String `tfsdk:"fleecing"`
	IONice                 types.Int64  `tfsdk:"ionice"`
	Lockwait               types.Int64  `tfsdk:"lockwait"`
	Mailnotification       types.String `tfsdk:"mailnotification"`
	Mailto                 types.String `tfsdk:"mailto"`
	Mode                   types.String `tfsdk:"mode"`
	Node                   types.String `tfsdk:"node"`
	NotesTemplate          types.String `tfsdk:"notes_template"`
	NotificationMode       types.String `tfsdk:"notification_mode"`
	PBSChangeDetectionMode types.String `tfsdk:"pbs_change_detection_mode"`
	Performance            types.String `tfsdk:"performance"`
	Pigz                   types.Int64  `tfsdk:"pigz"`
	Pool                   types.String `tfsdk:"pool"`
	Protected              types.Bool   `tfsdk:"protected"`
	PruneBackups           types.String `tfsdk:"prune_backups"`
	Quiet                  types.Bool   `tfsdk:"quiet"`
	Remove                 types.Bool   `tfsdk:"remove"`
	Script                 types.String `tfsdk:"script"`
	Stdexcludes            types.Bool   `tfsdk:"stdexcludes"`
	Stop                   types.Bool   `tfsdk:"stop"`
	Stopwait               types.Int64  `tfsdk:"stopwait"`
	Storage                types.String `tfsdk:"storage"`
	Tmpdir                 types.String `tfsdk:"tmpdir"`
	VMID                   types.String `tfsdk:"vmid"`
	Zstd                   types.Int64  `tfsdk:"zstd"`
}

// pveBackupJobsDataSourceModel is the Terraform-facing shape.
type pveBackupJobsDataSourceModel struct {
	ID             types.String                                `tfsdk:"id"`
	Node           types.String                                `tfsdk:"node"`
	Jobs           []pveBackupJobsDataSourceRowModel           `tfsdk:"jobs"`
	NotBackedUp    []pveBackupJobsDataSourceNotBackedUpModel   `tfsdk:"not_backed_up"`
	VzdumpDefaults *pveBackupJobsDataSourceVzdumpDefaultsModel `tfsdk:"vzdump_defaults"`
}

// Metadata implements datasource.DataSource.
func (d *pveBackupJobsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveBackupJobs
}

// backupJobRowAttributes returns the shared attribute map of a job row.
func backupJobRowAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "The job ID.",
		},
		"node": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Node the job is pinned to, when set on the job.",
		},
		"comment": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Description for the job.",
		},
		"schedule": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Backup schedule in PVE's systemd calendar-event subset.",
		},
		"enabled": schema.BoolAttribute{
			Computed:            true,
			MarkdownDescription: "Whether the job is enabled.",
		},
		"mode": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Backup mode. One of `snapshot`, `suspend`, `stop`.",
		},
		"compress": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Dump file compression. One of `0`, `1`, `gzip`, `lzo`, `zstd`.",
		},
		"storage": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Storage the resulting file is written to.",
		},
		"dumpdir": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Directory the resulting files are written to.",
		},
		"tmpdir": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Directory for temporary files.",
		},
		"script": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Hook script used by the job.",
		},
		"all": schema.BoolAttribute{
			Computed:            true,
			MarkdownDescription: "Whether all known guest systems on the host are backed up.",
		},
		"vmid": schema.ListAttribute{
			Computed:            true,
			ElementType:         types.StringType,
			MarkdownDescription: "Guest IDs backed up by the job.",
		},
		"exclude": schema.ListAttribute{
			Computed:            true,
			ElementType:         types.StringType,
			MarkdownDescription: "Guest IDs excluded from the job (when `all` is set).",
		},
		"exclude_path": schema.ListAttribute{
			Computed:            true,
			ElementType:         types.StringType,
			MarkdownDescription: "Shell globs excluded from container backups.",
		},
		"pool": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Pool whose guest systems are backed up.",
		},
		"notes_template": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Template string for generating notes for the backup(s).",
		},
		"mailto": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Deprecated by PVE: comma-separated list of email addresses or users receiving notifications.",
		},
		"mailnotification": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Deprecated by PVE: when a notification mail is sent. One of `always`, `failure`.",
		},
		"notification_mode": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Which notification system to use. One of `auto`, `legacy-sendmail`, `notification-system`.",
		},
		"pbs_change_detection_mode": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "PBS change detection mode for container backups. One of `legacy`, `data`, `metadata`.",
		},
		"bwlimit": schema.Int64Attribute{
			Computed:            true,
			MarkdownDescription: "I/O bandwidth limit in KiB/s.",
		},
		"zstd": schema.Int64Attribute{
			Computed:            true,
			MarkdownDescription: "Zstd thread count (`0` means half of the available cores).",
		},
		"pigz": schema.Int64Attribute{
			Computed:            true,
			MarkdownDescription: "pigz thread count for gzip compression.",
		},
		"ionice": schema.Int64Attribute{
			Computed:            true,
			MarkdownDescription: "IO priority when using the BFQ scheduler; between 0 and 8 (8 is idle priority).",
		},
		"lockwait": schema.Int64Attribute{
			Computed:            true,
			MarkdownDescription: "Maximal time to wait for the global lock in minutes.",
		},
		"stopwait": schema.Int64Attribute{
			Computed:            true,
			MarkdownDescription: "Maximal time to wait until a guest system is stopped in minutes.",
		},
		"stdexcludes": schema.BoolAttribute{
			Computed:            true,
			MarkdownDescription: "Whether temporary files and logs are excluded.",
		},
		"quiet": schema.BoolAttribute{
			Computed:            true,
			MarkdownDescription: "Whether log output is suppressed on the node.",
		},
		"stop": schema.BoolAttribute{
			Computed:            true,
			MarkdownDescription: "Whether running backup jobs on this host are stopped.",
		},
		"remove": schema.BoolAttribute{
			Computed:            true,
			MarkdownDescription: "Whether older backups are pruned according to `prune_backups`.",
		},
		"repeat_missed": schema.BoolAttribute{
			Computed:            true,
			MarkdownDescription: "Whether a missed run is repeated as soon as possible.",
		},
		"protected": schema.BoolAttribute{
			Computed:            true,
			MarkdownDescription: "Whether backup(s) are marked as protected.",
		},
		"fleecing": schema.SingleNestedAttribute{
			Computed:            true,
			MarkdownDescription: "Backup fleecing options (VM only); null when not configured.",
			Attributes: map[string]schema.Attribute{
				"enabled": schema.BoolAttribute{
					Computed:            true,
					MarkdownDescription: "Whether backup fleecing is enabled.",
				},
				"storage": schema.StringAttribute{
					Computed:            true,
					MarkdownDescription: "Storage used for fleecing images.",
				},
			},
		},
		"performance": schema.SingleNestedAttribute{
			Computed:            true,
			MarkdownDescription: "Performance-related settings; null when not configured.",
			Attributes: map[string]schema.Attribute{
				"max_workers": schema.Int64Attribute{
					Computed:            true,
					MarkdownDescription: "Maximum parallel IO workers for VMs.",
				},
				"pbs_entries_max": schema.Int64Attribute{
					Computed:            true,
					MarkdownDescription: "Entry limit held in memory for container backups sent to PBS.",
				},
			},
		},
		"prune_backups": schema.SingleNestedAttribute{
			Computed:            true,
			MarkdownDescription: "Retention options overriding the storage configuration; null when not configured.",
			Attributes: map[string]schema.Attribute{
				"keep_all": schema.BoolAttribute{
					Computed:            true,
					MarkdownDescription: "Whether all backups are kept.",
				},
				"keep_hourly": schema.Int64Attribute{
					Computed:            true,
					MarkdownDescription: "Backups kept for the last N different hours.",
				},
				"keep_daily": schema.Int64Attribute{
					Computed:            true,
					MarkdownDescription: "Backups kept for the last N different days.",
				},
				"keep_weekly": schema.Int64Attribute{
					Computed:            true,
					MarkdownDescription: "Backups kept for the last N different weeks.",
				},
				"keep_monthly": schema.Int64Attribute{
					Computed:            true,
					MarkdownDescription: "Backups kept for the last N different months.",
				},
				"keep_yearly": schema.Int64Attribute{
					Computed:            true,
					MarkdownDescription: "Backups kept for the last N different years.",
				},
				"keep_last": schema.Int64Attribute{
					Computed:            true,
					MarkdownDescription: "Number of most recent backups kept.",
				},
			},
		},
		"next_run": schema.Int64Attribute{
			Computed:            true,
			MarkdownDescription: "UNIX timestamp when this backup job will be executed next.",
		},
	}
}

// Schema implements datasource.DataSource.
func (d *pveBackupJobsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists all vzdump backup jobs (`GET /cluster/backup`), the guests not covered by any backup job (`GET /cluster/backup-info/not-backed-up`), and — when `node` is set — the effective vzdump defaults of that node (`GET /nodes/{node}/vzdump/defaults`).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Static identifier for the backup job index.",
			},
			"node": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "When set, additionally exposes this node's effective `vzdump_defaults`.",
			},
			"jobs": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "All configured vzdump backup jobs.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: backupJobRowAttributes(),
				},
			},
			"not_backed_up": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Guests not covered by any backup job.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"vmid": schema.Int64Attribute{
							Computed:            true,
							MarkdownDescription: "VMID of the guest.",
						},
						"name": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Name of the guest.",
						},
						"type": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Type of the guest. One of `qemu`, `lxc`.",
						},
					},
				},
			},
			"vzdump_defaults": schema.SingleNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Effective vzdump defaults of the `node` argument; null when `node` is not set. The `fleecing`, `performance`, and `prune_backups` fields arrive as PVE property strings on this endpoint and are surfaced verbatim.",
				Attributes:          pveNodeVzdumpDefaultsAttributes(),
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveBackupJobsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*pveclient.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *pveclient.Client, got: %T.", req.ProviderData),
		)
		return
	}
	d.client = client
}

// Read implements datasource.DataSource.
func (d *pveBackupJobsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveBackupJobsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Error reading pve_backup_jobs", "provider client is not configured")
		return
	}
	jobs, err := d.client.ListBackupJobs(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading pve_backup_jobs", fmt.Sprintf("listing backup jobs: %s", err))
		return
	}
	data.Jobs = make([]pveBackupJobsDataSourceRowModel, 0, len(jobs))
	for _, job := range jobs {
		data.Jobs = append(data.Jobs, pveBackupJobsRowFromWire(job))
	}
	notBackedUp, err := d.client.ListNotBackedUpGuests(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading pve_backup_jobs", fmt.Sprintf("listing guests not backed up: %s", err))
		return
	}
	data.NotBackedUp = make([]pveBackupJobsDataSourceNotBackedUpModel, 0, len(notBackedUp))
	for _, guest := range notBackedUp {
		data.NotBackedUp = append(data.NotBackedUp, pveBackupJobsDataSourceNotBackedUpModel{
			VMID: types.Int64Value(guest.VMID),
			Name: nodeNetworkStringToTF(guest.Name),
			Type: types.StringValue(guest.Type),
		})
	}
	if !data.Node.IsNull() && !data.Node.IsUnknown() {
		defaults, err := d.client.GetVzdumpDefaults(ctx, data.Node.ValueString(), "")
		if err != nil {
			resp.Diagnostics.AddError(
				"Error reading pve_backup_jobs",
				fmt.Sprintf("reading vzdump defaults for node %s: %s", data.Node.ValueString(), err),
			)
			return
		}
		data.VzdumpDefaults = pveBackupJobsDefaultsFromWire(defaults)
	} else {
		data.VzdumpDefaults = nil
	}
	data.ID = types.StringValue("pve_backup_jobs")
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// pveBackupJobsRowFromWire projects a wire job onto a list row.
func pveBackupJobsRowFromWire(job pveclient.BackupJob) pveBackupJobsDataSourceRowModel {
	var shared pveBackupJobResourceModel
	backupJobReadFill(&shared, &job)
	// The row model mirrors the resource model field-for-field; the
	// conversion keeps them lockstep without a field-by-field copy.
	return pveBackupJobsDataSourceRowModel(shared)
}

// pveBackupJobsDefaultsFromWire projects the vzdump defaults response onto
// the typed nested object.
func pveBackupJobsDefaultsFromWire(def *pveclient.VzdumpDefaults) *pveBackupJobsDataSourceVzdumpDefaultsModel {
	if def == nil {
		return nil
	}
	return &pveBackupJobsDataSourceVzdumpDefaultsModel{
		All:                    nodeNetworkBoolPtrToTF(def.All),
		BWLimit:                haInt64PtrToTF(def.BWLimit),
		Compress:               nodeNetworkStringToTF(def.Compress),
		Dumpdir:                nodeNetworkStringToTF(def.DumpDir),
		Exclude:                nodeNetworkStringToTF(def.Exclude),
		ExcludePath:            backupJobListToTF(def.ExcludePath),
		Fleecing:               nodeNetworkStringToTF(def.Fleecing),
		IONice:                 haInt64PtrToTF(def.IONice),
		Lockwait:               haInt64PtrToTF(def.LockWait),
		Mailnotification:       nodeNetworkStringToTF(def.MailNotification),
		Mailto:                 nodeNetworkStringToTF(def.MailTo),
		Mode:                   nodeNetworkStringToTF(def.Mode),
		Node:                   nodeNetworkStringToTF(def.Node),
		NotesTemplate:          nodeNetworkStringToTF(def.NotesTemplate),
		NotificationMode:       nodeNetworkStringToTF(def.NotificationMode),
		PBSChangeDetectionMode: nodeNetworkStringToTF(def.PBSChangeDetectionMode),
		Performance:            nodeNetworkStringToTF(def.Performance),
		Pigz:                   haInt64PtrToTF(def.Pigz),
		Pool:                   nodeNetworkStringToTF(def.Pool),
		Protected:              nodeNetworkBoolPtrToTF(def.Protected),
		PruneBackups:           nodeNetworkStringToTF(def.PruneBackups),
		Quiet:                  nodeNetworkBoolPtrToTF(def.Quiet),
		Remove:                 nodeNetworkBoolPtrToTF(def.Remove),
		Script:                 nodeNetworkStringToTF(def.Script),
		Stdexcludes:            nodeNetworkBoolPtrToTF(def.StdExcludes),
		Stop:                   nodeNetworkBoolPtrToTF(def.Stop),
		Stopwait:               haInt64PtrToTF(def.StopWait),
		Storage:                nodeNetworkStringToTF(def.Storage),
		Tmpdir:                 nodeNetworkStringToTF(def.TmpDir),
		VMID:                   nodeNetworkStringToTF(def.VMID),
		Zstd:                   haInt64PtrToTF(def.Zstd),
	}
}
