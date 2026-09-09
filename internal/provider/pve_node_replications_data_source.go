// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ datasource.DataSource              = &pveNodeReplicationsDataSource{}
	_ datasource.DataSourceWithConfigure = &pveNodeReplicationsDataSource{}
)

// NewPveNodeReplicationsDataSource returns the data source implementation.
func NewPveNodeReplicationsDataSource() datasource.DataSource {
	return &pveNodeReplicationsDataSource{}
}

// pveNodeReplicationsDataSource lists the replication jobs known to one
// node (GET /nodes/{node}/replication), including their runtime sync state
// and job logs.
type pveNodeReplicationsDataSource struct {
	client *pveclient.Client
}

// pveNodeReplicationsDataSourceModel is the Terraform-facing shape.
type pveNodeReplicationsDataSourceModel struct {
	ID           types.String                                    `tfsdk:"id"`
	Node         types.String                                    `tfsdk:"node"`
	Guest        types.Int64                                     `tfsdk:"guest"`
	Replications []pveNodeReplicationsDataSourceReplicationModel `tfsdk:"replications"`
}

// pveNodeReplicationsDataSourceReplicationModel mirrors one row of the
// /nodes/{node}/replication response plus its job log.
type pveNodeReplicationsDataSourceReplicationModel struct {
	ID        types.String  `tfsdk:"id"`
	Type      types.String  `tfsdk:"type"`
	Target    types.String  `tfsdk:"target"`
	Comment   types.String  `tfsdk:"comment"`
	Disable   types.Bool    `tfsdk:"disable"`
	Guest     types.Int64   `tfsdk:"guest"`
	GuestName types.String  `tfsdk:"guest_name"`
	JobNum    types.Int64   `tfsdk:"jobnum"`
	Schedule  types.String  `tfsdk:"schedule"`
	Rate      types.Float64 `tfsdk:"rate"`
	Removal   types.Bool    `tfsdk:"removal"`
	LastSync  types.Int64   `tfsdk:"last_sync"`
	LastTry   types.Int64   `tfsdk:"last_try"`
	NextSync  types.Int64   `tfsdk:"next_sync"`
	FailCount types.Int64   `tfsdk:"fail_count"`
	Duration  types.Int64   `tfsdk:"duration"`
	Status    types.String  `tfsdk:"status"`
	Error     types.String  `tfsdk:"error"`
	Log       types.List    `tfsdk:"log"`
}

// Metadata implements datasource.DataSource.
func (d *pveNodeReplicationsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveNodeReplications
}

// Schema implements datasource.DataSource.
func (d *pveNodeReplicationsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists the replication jobs on one node as reported by `GET /nodes/{node}/replication`, " +
			"including each job's runtime sync state and job log. Fields the node omits for a given job are empty.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Static identifier for the node replication index.",
			},
			"node": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The cluster node whose replication jobs to list.",
			},
			"guest": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Optional server-side filter listing only the jobs of this guest. Must be between 100 and 999999999.",
				Validators: []validator.Int64{
					int64validator.Between(100, 999999999),
				},
			},
			"replications": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Replication jobs on the node, with their runtime sync state.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":         schema.StringAttribute{Computed: true, MarkdownDescription: "Replication Job ID in the `<GUEST>-<JOBNUM>` form."},
						"type":       schema.StringAttribute{Computed: true, MarkdownDescription: "Section type; `local` for storage replication."},
						"target":     schema.StringAttribute{Computed: true, MarkdownDescription: "Target node that receives the replicated volumes."},
						"comment":    schema.StringAttribute{Computed: true, MarkdownDescription: "Free-form description, empty when the job has none."},
						"disable":    schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the job is disabled/deactivated."},
						"guest":      schema.Int64Attribute{Computed: true, MarkdownDescription: "Guest ID the job replicates."},
						"guest_name": schema.StringAttribute{Computed: true, MarkdownDescription: "Name of the replicated guest (VM name or container hostname)."},
						"jobnum":     schema.Int64Attribute{Computed: true, MarkdownDescription: "Job number component of the replication job ID."},
						"schedule":   schema.StringAttribute{Computed: true, MarkdownDescription: "Replication schedule as a `systemd` calendar event subset."},
						"rate":       schema.Float64Attribute{Computed: true, MarkdownDescription: "Rate limit in MB/s, or null when the job is unlimited."},
						"removal":    schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the job is marked for removal and its local snapshots are being cleaned up."},
						"last_sync":  schema.Int64Attribute{Computed: true, MarkdownDescription: "Unix epoch time of the last successful sync, or 0 when the job never synced."},
						"last_try":   schema.Int64Attribute{Computed: true, MarkdownDescription: "Unix epoch time of the last sync attempt, successful or not."},
						"next_sync":  schema.Int64Attribute{Computed: true, MarkdownDescription: "Unix epoch time of the next scheduled sync."},
						"fail_count": schema.Int64Attribute{Computed: true, MarkdownDescription: "Number of consecutive failed syncs; 0 when the job is healthy."},
						"duration":   schema.Int64Attribute{Computed: true, MarkdownDescription: "Duration of the last sync in seconds."},
						"status":     schema.StringAttribute{Computed: true, MarkdownDescription: "Job state, e.g. `idle`, `syncing`, or `error`."},
						"error":      schema.StringAttribute{Computed: true, MarkdownDescription: "Last sync error text, empty when the job is healthy."},
						"log": schema.ListAttribute{
							Computed:    true,
							ElementType: types.StringType,
							MarkdownDescription: "Full replication job log lines, fetched per job from " +
								"`GET /nodes/{node}/replication/{id}/log`. This issues one additional read-only " +
								"API request per listed job; the endpoint has no side effects, so every row is populated.",
						},
					},
				},
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveNodeReplicationsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
func (d *pveNodeReplicationsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveNodeReplicationsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Error reading pve_node_replications", "provider client is not configured")
		return
	}
	node := data.Node.ValueString()
	rows, err := d.client.ListNodeReplications(ctx, node, data.Guest.ValueInt64())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_node_replications",
			fmt.Sprintf("listing replication jobs on node %s: %s", node, err),
		)
		return
	}
	replications := make([]pveNodeReplicationsDataSourceReplicationModel, 0, len(rows))
	for _, row := range rows {
		// The pin's log endpoint is a plain read-only GET with no side
		// effects, so every row's log is fetched eagerly.
		log, err := d.client.GetReplicationLog(ctx, node, row.ID)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error reading pve_node_replications",
				fmt.Sprintf("reading log of replication job %s on node %s: %s", row.ID, node, err),
			)
			return
		}
		replications = append(replications, pveNodeReplicationsDataSourceReplicationModel{
			ID:        types.StringValue(row.ID),
			Type:      types.StringValue(row.Type),
			Target:    types.StringValue(row.Target),
			Comment:   types.StringValue(row.Comment),
			Disable:   types.BoolValue(row.Disable),
			Guest:     types.Int64Value(row.Guest),
			GuestName: types.StringValue(row.GuestName),
			JobNum:    types.Int64Value(row.JobNum),
			Schedule:  types.StringValue(row.Schedule),
			Rate:      replicationFloatPtrToTF(row.Rate),
			Removal:   types.BoolValue(row.Removal),
			LastSync:  types.Int64Value(row.LastSync),
			LastTry:   types.Int64Value(row.LastTry),
			NextSync:  types.Int64Value(row.NextSync),
			FailCount: types.Int64Value(row.FailCount),
			Duration:  types.Int64Value(row.Duration),
			Status:    types.StringValue(row.Status),
			Error:     types.StringValue(row.Error),
			Log:       listStringToTF(log),
		})
	}
	data.ID = types.StringValue("pve_node_replications")
	data.Replications = replications
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
