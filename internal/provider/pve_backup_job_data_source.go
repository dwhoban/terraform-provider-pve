// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// Ensure the framework interfaces are satisfied.
var (
	_ datasource.DataSource              = &pveBackupJobDataSource{}
	_ datasource.DataSourceWithConfigure = &pveBackupJobDataSource{}
)

// NewPveBackupJobDataSource returns the data source implementation.
func NewPveBackupJobDataSource() datasource.DataSource {
	return &pveBackupJobDataSource{}
}

// pveBackupJobDataSource reads a single vzdump backup job definition
// (GET /cluster/backup/{id}) plus its per-job included volume tree.
type pveBackupJobDataSource struct {
	client *pveclient.Client
}

// pveBackupJobIncludedVolumeModel mirrors one volume entry of the
// included_volumes tree.
type pveBackupJobIncludedVolumeModel struct {
	ID       types.String `tfsdk:"id"`
	Included types.Bool   `tfsdk:"included"`
	Name     types.String `tfsdk:"name"`
	Reason   types.String `tfsdk:"reason"`
}

// pveBackupJobIncludedGuestModel mirrors one guest entry of the
// included_volumes tree.
type pveBackupJobIncludedGuestModel struct {
	VMID    types.Int64                       `tfsdk:"vmid"`
	Name    types.String                      `tfsdk:"name"`
	Type    types.String                      `tfsdk:"type"`
	Volumes []pveBackupJobIncludedVolumeModel `tfsdk:"volumes"`
}

// pveBackupJobDataSourceModel is the Terraform-facing shape.
type pveBackupJobDataSourceModel struct {
	ID                     types.String                     `tfsdk:"id"`
	Node                   types.String                     `tfsdk:"node"`
	Comment                types.String                     `tfsdk:"comment"`
	Schedule               types.String                     `tfsdk:"schedule"`
	Enabled                types.Bool                       `tfsdk:"enabled"`
	Mode                   types.String                     `tfsdk:"mode"`
	Compress               types.String                     `tfsdk:"compress"`
	Storage                types.String                     `tfsdk:"storage"`
	Dumpdir                types.String                     `tfsdk:"dumpdir"`
	Tmpdir                 types.String                     `tfsdk:"tmpdir"`
	Script                 types.String                     `tfsdk:"script"`
	All                    types.Bool                       `tfsdk:"all"`
	VMIDs                  types.List                       `tfsdk:"vmid"`
	Exclude                types.List                       `tfsdk:"exclude"`
	ExcludePath            types.List                       `tfsdk:"exclude_path"`
	Pool                   types.String                     `tfsdk:"pool"`
	NotesTemplate          types.String                     `tfsdk:"notes_template"`
	Mailto                 types.String                     `tfsdk:"mailto"`
	Mailnotification       types.String                     `tfsdk:"mailnotification"`
	NotificationMode       types.String                     `tfsdk:"notification_mode"`
	PBSChangeDetectionMode types.String                     `tfsdk:"pbs_change_detection_mode"`
	BWLimit                types.Int64                      `tfsdk:"bwlimit"`
	Zstd                   types.Int64                      `tfsdk:"zstd"`
	Pigz                   types.Int64                      `tfsdk:"pigz"`
	IONice                 types.Int64                      `tfsdk:"ionice"`
	Lockwait               types.Int64                      `tfsdk:"lockwait"`
	Stopwait               types.Int64                      `tfsdk:"stopwait"`
	Stdexcludes            types.Bool                       `tfsdk:"stdexcludes"`
	Quiet                  types.Bool                       `tfsdk:"quiet"`
	Stop                   types.Bool                       `tfsdk:"stop"`
	Remove                 types.Bool                       `tfsdk:"remove"`
	RepeatMissed           types.Bool                       `tfsdk:"repeat_missed"`
	Protected              types.Bool                       `tfsdk:"protected"`
	Fleecing               *pveBackupJobFleecingModel       `tfsdk:"fleecing"`
	Performance            *pveBackupJobPerformanceModel    `tfsdk:"performance"`
	PruneBackups           *pveBackupJobPruneBackupsModel   `tfsdk:"prune_backups"`
	NextRun                types.Int64                      `tfsdk:"next_run"`
	IncludedVolumes        []pveBackupJobIncludedGuestModel `tfsdk:"included_volumes"`
}

// Metadata implements datasource.DataSource.
func (d *pveBackupJobDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveBackupJob
}

// Schema implements datasource.DataSource.
func (d *pveBackupJobDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads a vzdump backup job definition (`GET /cluster/backup/{id}`) together with the per-job included volume tree (`GET /cluster/backup/{id}/included_volumes`).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The job ID to read.",
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
			"included_volumes": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Tree of guests (and their volumes) included in this job, with the inclusion decision and reason per volume. Guests that were removed but not purged appear with type `unknown`.",
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
							MarkdownDescription: "Type of the guest. One of `qemu`, `lxc`, `unknown`.",
						},
						"volumes": schema.ListNestedAttribute{
							Computed:            true,
							MarkdownDescription: "Volumes of the guest with their inclusion information.",
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"id": schema.StringAttribute{
										Computed:            true,
										MarkdownDescription: "Configuration key of the volume (e.g. `scsi0`).",
									},
									"included": schema.BoolAttribute{
										Computed:            true,
										MarkdownDescription: "Whether the volume is included in the backup.",
									},
									"name": schema.StringAttribute{
										Computed:            true,
										MarkdownDescription: "Name of the volume.",
									},
									"reason": schema.StringAttribute{
										Computed:            true,
										MarkdownDescription: "Reason why the volume is included (or excluded).",
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveBackupJobDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
func (d *pveBackupJobDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveBackupJobDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Error reading pve_backup_job", "provider client is not configured")
		return
	}
	job, err := d.client.GetBackupJob(ctx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_backup_job",
			fmt.Sprintf("reading backup job %s: %s", data.ID.ValueString(), err),
		)
		return
	}
	backupJobDataSourceReadFill(&data, job)

	included, err := d.client.GetBackupIncludedVolumes(ctx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_backup_job",
			fmt.Sprintf("reading included volumes for backup job %s: %s", data.ID.ValueString(), err),
		)
		return
	}
	data.IncludedVolumes = backupJobIncludedGuestsFromWire(included)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// backupJobDataSourceReadFill projects a wire job onto the data source
// model's shared field set; the included volume tree is filled separately.
func backupJobDataSourceReadFill(data *pveBackupJobDataSourceModel, job *pveclient.BackupJob) {
	var shared pveBackupJobResourceModel
	backupJobReadFill(&shared, job)
	data.ID = shared.ID
	data.Node = shared.Node
	data.Comment = shared.Comment
	data.Schedule = shared.Schedule
	data.Enabled = shared.Enabled
	data.Mode = shared.Mode
	data.Compress = shared.Compress
	data.Storage = shared.Storage
	data.Dumpdir = shared.Dumpdir
	data.Tmpdir = shared.Tmpdir
	data.Script = shared.Script
	data.All = shared.All
	data.VMIDs = shared.VMIDs
	data.Exclude = shared.Exclude
	data.ExcludePath = shared.ExcludePath
	data.Pool = shared.Pool
	data.NotesTemplate = shared.NotesTemplate
	data.Mailto = shared.Mailto
	data.Mailnotification = shared.Mailnotification
	data.NotificationMode = shared.NotificationMode
	data.PBSChangeDetectionMode = shared.PBSChangeDetectionMode
	data.BWLimit = shared.BWLimit
	data.Zstd = shared.Zstd
	data.Pigz = shared.Pigz
	data.IONice = shared.IONice
	data.Lockwait = shared.Lockwait
	data.Stopwait = shared.Stopwait
	data.Stdexcludes = shared.Stdexcludes
	data.Quiet = shared.Quiet
	data.Stop = shared.Stop
	data.Remove = shared.Remove
	data.RepeatMissed = shared.RepeatMissed
	data.Protected = shared.Protected
	data.Fleecing = shared.Fleecing
	data.Performance = shared.Performance
	data.PruneBackups = shared.PruneBackups
	data.NextRun = shared.NextRun
}

// backupJobIncludedGuestsFromWire projects the included volume tree onto
// the data source model.
func backupJobIncludedGuestsFromWire(tree *pveclient.BackupIncludedVolumes) []pveBackupJobIncludedGuestModel {
	if tree == nil {
		return nil
	}
	guests := make([]pveBackupJobIncludedGuestModel, 0, len(tree.Children))
	for _, g := range tree.Children {
		guest := pveBackupJobIncludedGuestModel{
			VMID:    types.Int64Value(g.VMID),
			Name:    nodeNetworkStringToTF(g.Name),
			Type:    types.StringValue(g.Type),
			Volumes: nil,
		}
		for _, v := range g.Volumes {
			guest.Volumes = append(guest.Volumes, pveBackupJobIncludedVolumeModel{
				ID:       types.StringValue(v.ID),
				Included: nodeNetworkBoolPtrToTF(v.Included),
				Name:     nodeNetworkStringToTF(v.Name),
				Reason:   nodeNetworkStringToTF(v.Reason),
			})
		}
		guests = append(guests, guest)
	}
	return guests
}
