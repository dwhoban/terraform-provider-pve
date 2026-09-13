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

// Ensure framework interfaces are satisfied.
var (
	_ datasource.DataSource              = &pveReplicationDataSource{}
	_ datasource.DataSourceWithConfigure = &pveReplicationDataSource{}
)

// NewPveReplicationDataSource returns the data source implementation.
func NewPveReplicationDataSource() datasource.DataSource {
	return &pveReplicationDataSource{}
}

// pveReplicationDataSource reads a single storage replication job
// (GET /cluster/replication/{id}).
type pveReplicationDataSource struct {
	client *pveclient.Client
}

// pveReplicationDataSourceModel is the Terraform-facing shape.
type pveReplicationDataSourceModel struct {
	ID       types.String  `tfsdk:"id"`
	Target   types.String  `tfsdk:"target"`
	Type     types.String  `tfsdk:"type"`
	Guest    types.Int64   `tfsdk:"guest"`
	JobNum   types.Int64   `tfsdk:"jobnum"`
	Schedule types.String  `tfsdk:"schedule"`
	Rate     types.Float64 `tfsdk:"rate"`
	Comment  types.String  `tfsdk:"comment"`
	Disable  types.Bool    `tfsdk:"disable"`
	Digest   types.String  `tfsdk:"digest"`
}

// Metadata implements datasource.DataSource.
func (d *pveReplicationDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveReplication
}

// Schema implements datasource.DataSource.
func (d *pveReplicationDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads a single storage replication job from `GET /cluster/replication/{id}`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Replication Job ID to look up, in the `<GUEST>-<JOBNUM>` form (e.g. `100-0`).",
			},
			"target": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Target node that receives the replicated volumes.",
			},
			"type": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Section type; always `local`.",
			},
			"guest": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Guest ID the job replicates, decoded by PVE from the job ID.",
			},
			"jobnum": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Job number component of the replication job ID.",
			},
			"schedule": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Replication schedule as a `systemd` calendar event subset, or null when the job carries none.",
			},
			"rate": schema.Float64Attribute{
				Computed:            true,
				MarkdownDescription: "Rate limit in MB/s, or null when the job is unlimited.",
			},
			"comment": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Free-form description, or null when the job has none.",
			},
			"disable": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether the job is disabled/deactivated.",
			},
			"digest": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Configuration digest of the job, or null when PVE reports none.",
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveReplicationDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
func (d *pveReplicationDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveReplicationDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Error reading pve_replication", "provider client is not configured")
		return
	}
	job, err := d.client.GetReplication(ctx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_replication",
			fmt.Sprintf("reading replication job %s: %s", data.ID.ValueString(), err),
		)
		return
	}
	data.Target = types.StringValue(job.Target)
	data.Type = types.StringValue(job.Type)
	data.Guest = types.Int64Value(job.Guest)
	data.JobNum = types.Int64Value(job.JobNum)
	data.Schedule = nodeNetworkStringToTF(job.Schedule)
	data.Rate = replicationFloatPtrToTF(job.Rate)
	data.Comment = nodeNetworkStringToTF(job.Comment)
	data.Disable = types.BoolValue(job.Disable)
	data.Digest = nodeNetworkStringToTF(job.Digest)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
