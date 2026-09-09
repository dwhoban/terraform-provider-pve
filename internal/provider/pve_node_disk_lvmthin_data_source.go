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
	_ datasource.DataSource              = &pveNodeDiskLvmthinDataSource{}
	_ datasource.DataSourceWithConfigure = &pveNodeDiskLvmthinDataSource{}
)

// NewPveNodeDiskLvmthinDataSource returns the data source implementation.
func NewPveNodeDiskLvmthinDataSource() datasource.DataSource {
	return &pveNodeDiskLvmthinDataSource{}
}

// pveNodeDiskLvmthinDataSource looks up a single LVM thinpool on a node.
type pveNodeDiskLvmthinDataSource struct {
	client *pveclient.Client
}

// pveNodeDiskLvmthinDataSourceModel is the Terraform-facing shape.
type pveNodeDiskLvmthinDataSourceModel struct {
	ID           types.String `tfsdk:"id"`
	Node         types.String `tfsdk:"node"`
	Name         types.String `tfsdk:"name"`
	VG           types.String `tfsdk:"vg"`
	LVSize       types.Int64  `tfsdk:"lv_size"`
	MetadataSize types.Int64  `tfsdk:"metadata_size"`
	MetadataUsed types.Int64  `tfsdk:"metadata_used"`
	Used         types.Int64  `tfsdk:"used"`
}

// Metadata implements datasource.DataSource.
func (d *pveNodeDiskLvmthinDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveNodeDiskLvmthin
}

// Schema implements datasource.DataSource.
func (d *pveNodeDiskLvmthinDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up a single LVM thinpool on a Proxmox VE node (`GET /nodes/{node}/disks/lvmthin`, filtered by thinpool name).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Static identifier equal to the node name.",
			},
			"node": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Name of the PVE node to query.",
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Thinpool name (`lv` in the PVE listing).",
			},
			"vg": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The associated volume group.",
			},
			"lv_size": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "The size of the thinpool in bytes.",
			},
			"metadata_size": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "The size of the metadata LV in bytes.",
			},
			"metadata_used": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "The used bytes of the metadata LV.",
			},
			"used": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "The used bytes of the thinpool.",
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveNodeDiskLvmthinDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
func (d *pveNodeDiskLvmthinDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveNodeDiskLvmthinDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	node := data.Node.ValueString()
	thinpools, err := d.client.ListLVMThinpools(ctx, node)
	if err != nil {
		resp.Diagnostics.AddError("Error reading pve_node_disk_lvmthin", fmt.Sprintf("listing thinpools on %s: %s", node, err))
		return
	}
	for _, tp := range thinpools {
		if tp.LV != data.Name.ValueString() {
			continue
		}
		data.ID = types.StringValue(node)
		data.VG = types.StringValue(tp.VG)
		data.LVSize = types.Int64Value(tp.LVSize)
		data.MetadataSize = types.Int64Value(tp.MetadataSize)
		data.MetadataUsed = types.Int64Value(tp.MetadataUsed)
		data.Used = types.Int64Value(tp.Used)
		resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
		return
	}
	resp.Diagnostics.AddError(
		"LVM thinpool not found",
		fmt.Sprintf("No LVM thinpool named %q on node %s.", data.Name.ValueString(), node),
	)
}
