// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ datasource.DataSource              = &pveNodeTasksDataSource{}
	_ datasource.DataSourceWithConfigure = &pveNodeTasksDataSource{}
)

// NewPveNodeTasksDataSource returns the data source implementation.
func NewPveNodeTasksDataSource() datasource.DataSource {
	return &pveNodeTasksDataSource{}
}

// pveNodeTasksDataSource reads the finished task list of one node
// (GET /nodes/{node}/tasks).
type pveNodeTasksDataSource struct {
	client *pveclient.Client
}

// pveNodeTasksDataSourceModel is the Terraform-facing shape.
type pveNodeTasksDataSourceModel struct {
	ID           types.String                           `tfsdk:"id"`
	Node         types.String                           `tfsdk:"node"`
	StatusFilter types.String                           `tfsdk:"statusfilter"`
	Limit        types.Int64                            `tfsdk:"limit"`
	Since        types.Int64                            `tfsdk:"since"`
	Until        types.Int64                            `tfsdk:"until"`
	TypeFilter   types.String                           `tfsdk:"typefilter"`
	UserFilter   types.String                           `tfsdk:"userfilter"`
	Tasks        []pveNodeTasksDataSourceTaskEntryModel `tfsdk:"tasks"`
}

// pveNodeTasksDataSourceTaskEntryModel mirrors one finished-task row.
type pveNodeTasksDataSourceTaskEntryModel struct {
	UPID      types.String `tfsdk:"upid"`
	ID        types.String `tfsdk:"id"`
	Node      types.String `tfsdk:"node"`
	Type      types.String `tfsdk:"type"`
	User      types.String `tfsdk:"user"`
	Status    types.String `tfsdk:"status"`
	Starttime types.Int64  `tfsdk:"starttime"`
	Endtime   types.Int64  `tfsdk:"endtime"`
	PID       types.Int64  `tfsdk:"pid"`
	PStart    types.Int64  `tfsdk:"pstart"`
}

// Metadata implements datasource.DataSource.
func (d *pveNodeTasksDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveNodeTasks
}

// Schema implements datasource.DataSource.
func (d *pveNodeTasksDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads the finished task list of one node (`GET /nodes/{node}/tasks`). Rows are PVE's archive of completed worker tasks.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Identifier in `node` form (same as the `node` attribute).",
			},
			"node": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The cluster node name whose tasks to list.",
			},
			"statusfilter": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Comma-separated list of task states that should be returned (e.g. `OK,ERROR,running`).",
			},
			"limit": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Only list this number of tasks. Defaults to `50` upstream; `0` disables the cap.",
			},
			"since": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Only list tasks since this UNIX epoch timestamp.",
			},
			"until": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Only list tasks until this UNIX epoch timestamp.",
			},
			"typefilter": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Only list tasks of this type (e.g. `vzstart`, `vzdump`).",
			},
			"userfilter": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Only list tasks from this user (e.g. `root@pam`).",
			},
			"tasks": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Finished tasks, most recent first.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"upid": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The task's unique ID.",
						},
						"id": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The task's object ID (e.g. the VMID); null for tasks without one.",
						},
						"node": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The node the task ran on.",
						},
						"type": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The worker type (e.g. `vzdump`, `aptupdate`).",
						},
						"user": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The user who started the task.",
						},
						"status": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The task's end status (e.g. `OK`, `ERROR: ...`); null for tasks that never recorded one.",
						},
						"starttime": schema.Int64Attribute{
							Computed:            true,
							MarkdownDescription: "Task start time as UNIX epoch seconds.",
						},
						"endtime": schema.Int64Attribute{
							Computed:            true,
							MarkdownDescription: "Task end time as UNIX epoch seconds; null for unfinished rows.",
						},
						"pid": schema.Int64Attribute{
							Computed:            true,
							MarkdownDescription: "The worker process ID; null when PVE did not report it.",
						},
						"pstart": schema.Int64Attribute{
							Computed:            true,
							MarkdownDescription: "The worker start position in the process table; null when PVE did not report it.",
						},
					},
				},
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveNodeTasksDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
func (d *pveNodeTasksDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config pveNodeTasksDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	node := config.Node.ValueString()
	opts := pveclient.ListNodeTasksOptions{}
	if !config.StatusFilter.IsNull() && !config.StatusFilter.IsUnknown() {
		opts.StatusFilter = config.StatusFilter.ValueString()
	}
	if !config.Limit.IsNull() && !config.Limit.IsUnknown() {
		v := config.Limit.ValueInt64()
		opts.Limit = &v
	}
	if !config.Since.IsNull() && !config.Since.IsUnknown() {
		v := config.Since.ValueInt64()
		opts.Since = &v
	}
	if !config.Until.IsNull() && !config.Until.IsUnknown() {
		v := config.Until.ValueInt64()
		opts.Until = &v
	}
	if !config.TypeFilter.IsNull() && !config.TypeFilter.IsUnknown() {
		opts.TypeFilter = config.TypeFilter.ValueString()
	}
	if !config.UserFilter.IsNull() && !config.UserFilter.IsUnknown() {
		opts.UserFilter = config.UserFilter.ValueString()
	}
	tflogEntries, err := d.client.ListNodeTasks(ctx, node, opts)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_node_tasks",
			fmt.Sprintf("listing tasks for node %s: %s", node, err),
		)
		return
	}
	config.Tasks = make([]pveNodeTasksDataSourceTaskEntryModel, 0, len(tflogEntries))
	for _, entry := range tflogEntries {
		model := pveNodeTasksDataSourceTaskEntryModel{
			UPID:      types.StringValue(entry.UPID),
			ID:        types.StringPointerValue(entry.ID),
			Node:      types.StringValue(entry.Node),
			Type:      types.StringValue(entry.Type),
			User:      types.StringValue(entry.User),
			Status:    types.StringPointerValue(entry.Status),
			Starttime: types.Int64Value(entry.Starttime),
			Endtime:   types.Int64PointerValue(entry.Endtime),
			PID:       types.Int64PointerValue(entry.PID),
			PStart:    types.Int64PointerValue(entry.PStart),
		}
		config.Tasks = append(config.Tasks, model)
	}
	config.ID = types.StringValue(node)
	tflog.Debug(ctx, "read node tasks", map[string]any{"node": node, "count": len(config.Tasks)})
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
