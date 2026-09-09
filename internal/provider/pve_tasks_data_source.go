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
	_ datasource.DataSource              = &pveTasksDataSource{}
	_ datasource.DataSourceWithConfigure = &pveTasksDataSource{}
)

// NewPveTasksDataSource returns the data source implementation.
func NewPveTasksDataSource() datasource.DataSource {
	return &pveTasksDataSource{}
}

// pveTasksDataSource lists recent tasks cluster wide (GET /cluster/tasks).
type pveTasksDataSource struct {
	client *pveclient.Client
}

// pveTasksDataSourceModel is the Terraform-facing shape.
type pveTasksDataSourceModel struct {
	ID    types.String                 `tfsdk:"id"`
	Tasks []pveTasksDataSourceRowModel `tfsdk:"tasks"`
}

// pveTasksDataSourceRowModel mirrors one entry of the /cluster/tasks
// response.
type pveTasksDataSourceRowModel struct {
	UPID      types.String `tfsdk:"upid"`
	Node      types.String `tfsdk:"node"`
	Type      types.String `tfsdk:"type"`
	Status    types.String `tfsdk:"status"`
	User      types.String `tfsdk:"user"`
	StartTime types.Int64  `tfsdk:"starttime"`
	EndTime   types.Int64  `tfsdk:"endtime"`
}

// Metadata implements datasource.DataSource.
func (d *pveTasksDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveTasks
}

// Schema implements datasource.DataSource.
func (d *pveTasksDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists recent tasks cluster wide as reported by `GET /cluster/tasks`. Running tasks have a null `endtime` and may not yet carry a `status`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Static identifier for the cluster task list.",
			},
			"tasks": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Recent cluster tasks, most recent first.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"upid":      schema.StringAttribute{Computed: true, MarkdownDescription: "Unique process identifier of the task."},
						"node":      schema.StringAttribute{Computed: true, MarkdownDescription: "Node the task runs on."},
						"type":      schema.StringAttribute{Computed: true, MarkdownDescription: "Task type (e.g. `vzdump`, `qmstart`)."},
						"status":    schema.StringAttribute{Computed: true, MarkdownDescription: "Task status; `OK` on success, an error description on failure, empty while running."},
						"user":      schema.StringAttribute{Computed: true, MarkdownDescription: "User who started the task."},
						"starttime": schema.Int64Attribute{Computed: true, MarkdownDescription: "Start time as UNIX epoch seconds."},
						"endtime":   schema.Int64Attribute{Computed: true, MarkdownDescription: "End time as UNIX epoch seconds; 0 while the task is still running."},
					},
				},
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveTasksDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
func (d *pveTasksDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveTasksDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	tasks, err := d.client.ListClusterTasks(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading pve_tasks", fmt.Sprintf("listing cluster tasks: %s", err))
		return
	}
	rows := make([]pveTasksDataSourceRowModel, 0, len(tasks))
	for _, task := range tasks {
		rows = append(rows, pveTasksDataSourceRowModel{
			UPID:      types.StringValue(task.UPID),
			Node:      types.StringValue(task.Node),
			Type:      types.StringValue(task.Type),
			Status:    types.StringValue(task.Status),
			User:      types.StringValue(task.User),
			StartTime: types.Int64Value(task.StartTime),
			EndTime:   types.Int64Value(task.EndTime),
		})
	}
	data.ID = types.StringValue("pve_tasks")
	data.Tasks = rows
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
