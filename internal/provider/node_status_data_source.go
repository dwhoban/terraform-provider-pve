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

// Ensure framework interfaces are satisfied.
var (
	_ datasource.DataSource              = &pveNodeStatusDataSource{}
	_ datasource.DataSourceWithConfigure = &pveNodeStatusDataSource{}
)

// NewPveNodeStatusDataSource returns the data source implementation.
func NewPveNodeStatusDataSource() datasource.DataSource {
	return &pveNodeStatusDataSource{}
}

// pveNodeStatusDataSource returns the runtime status of a single node.
type pveNodeStatusDataSource struct {
	client *pveclient.Client
}

// pveNodeStatusDataSourceModel is the Terraform-facing shape.
type pveNodeStatusDataSourceModel struct {
	ID          types.String  `tfsdk:"id"`
	Node        types.String  `tfsdk:"node"`
	CPU         types.Float64 `tfsdk:"cpu"`
	MaxCPU      types.Int64   `tfsdk:"maxcpu"`
	Mem         types.Int64   `tfsdk:"mem"`
	MaxMem      types.Int64   `tfsdk:"maxmem"`
	Uptime      types.Int64   `tfsdk:"uptime"`
	Level       types.String  `tfsdk:"level"`
	Kernel      types.String  `tfsdk:"kernel"`
	PVEVersion  types.String  `tfsdk:"pveversion"`
	RootFSTotal types.Int64   `tfsdk:"rootfs_total"`
	RootFSUsed  types.Int64   `tfsdk:"rootfs_used"`
	RootFSFree  types.Int64   `tfsdk:"rootfs_free"`
}

// Metadata implements datasource.DataSource.
func (d *pveNodeStatusDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveNodeStatus
}

// Schema implements datasource.DataSource.
func (d *pveNodeStatusDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reports runtime CPU, memory, kernel, and root-filesystem usage for a single Proxmox VE node (`GET /nodes/{node}/status`).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Static identifier equal to the node name.",
			},
			"node": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Name of the PVE node to query.",
			},
			"cpu": schema.Float64Attribute{
				Computed:            true,
				MarkdownDescription: "Current CPU usage fraction (0.0–1.0).",
			},
			"maxcpu": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Number of CPU cores.",
			},
			"mem": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Used memory in bytes.",
			},
			"maxmem": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Total memory in bytes.",
			},
			"uptime": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Uptime in seconds.",
			},
			"level": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Node capability level (`c` for current).",
			},
			"kernel": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Running kernel version.",
			},
			"pveversion": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Installed PVE package version.",
			},
			"rootfs_total": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Root filesystem capacity in bytes.",
			},
			"rootfs_used": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Root filesystem used in bytes.",
			},
			"rootfs_free": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Root filesystem free in bytes.",
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveNodeStatusDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
func (d *pveNodeStatusDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveNodeStatusDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	st, err := d.client.GetNodeStatus(ctx, data.Node.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading pve_node_status", fmt.Sprintf("reading status for %s: %s", data.Node.ValueString(), err))
		return
	}
	data.ID = types.StringValue(data.Node.ValueString())
	data.CPU = types.Float64Value(st.CPU)
	data.MaxCPU = types.Int64Value(int64(st.MaxCPU))
	data.Mem = types.Int64Value(st.Mem)
	data.MaxMem = types.Int64Value(st.MaxMem)
	data.Uptime = types.Int64Value(st.Uptime)
	data.Level = types.StringValue(st.Level)
	data.Kernel = types.StringValue(st.Kernel)
	data.PVEVersion = types.StringValue(st.PVEVersion)
	data.RootFSTotal = types.Int64Value(st.RootFS.Total)
	data.RootFSUsed = types.Int64Value(st.RootFS.Used)
	data.RootFSFree = types.Int64Value(st.RootFS.Free)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
