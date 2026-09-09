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
	_ datasource.DataSource              = &pveNodeDiskDirectoryDataSource{}
	_ datasource.DataSourceWithConfigure = &pveNodeDiskDirectoryDataSource{}
)

// NewPveNodeDiskDirectoryDataSource returns the data source implementation.
func NewPveNodeDiskDirectoryDataSource() datasource.DataSource {
	return &pveNodeDiskDirectoryDataSource{}
}

// pveNodeDiskDirectoryDataSource looks up a single PVE-managed directory
// storage on a node.
type pveNodeDiskDirectoryDataSource struct {
	client *pveclient.Client
}

// pveNodeDiskDirectoryDataSourceModel is the Terraform-facing shape.
type pveNodeDiskDirectoryDataSourceModel struct {
	ID       types.String `tfsdk:"id"`
	Node     types.String `tfsdk:"node"`
	Name     types.String `tfsdk:"name"`
	Device   types.String `tfsdk:"device"`
	Path     types.String `tfsdk:"path"`
	Type     types.String `tfsdk:"type"`
	Options  types.String `tfsdk:"options"`
	UnitFile types.String `tfsdk:"unitfile"`
}

// Metadata implements datasource.DataSource.
func (d *pveNodeDiskDirectoryDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveNodeDiskDirectory
}

// Schema implements datasource.DataSource.
func (d *pveNodeDiskDirectoryDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up a single PVE-managed directory storage on a Proxmox VE node (`GET /nodes/{node}/disks/directory`, matched on the `/mnt/pve/<name>` mount path).",
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
				MarkdownDescription: "Storage identifier. PVE mounts the directory under `/mnt/pve/<name>`; the listing is matched on that path.",
			},
			"device": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The mounted device.",
			},
			"path": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The mount path (e.g. `/mnt/pve/backup`).",
			},
			"type": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The filesystem type. One of `ext4` or `xfs`.",
			},
			"options": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The mount options.",
			},
			"unitfile": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The path of the systemd mount unit.",
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveNodeDiskDirectoryDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
func (d *pveNodeDiskDirectoryDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveNodeDiskDirectoryDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	node := data.Node.ValueString()
	dirs, err := d.client.ListNodeDirectories(ctx, node)
	if err != nil {
		resp.Diagnostics.AddError("Error reading pve_node_disk_directory", fmt.Sprintf("listing directories on %s: %s", node, err))
		return
	}
	want := "/mnt/pve/" + data.Name.ValueString()
	for _, dir := range dirs {
		if dir.Path != want {
			continue
		}
		data.ID = types.StringValue(node)
		data.Device = types.StringValue(dir.Device)
		data.Path = types.StringValue(dir.Path)
		data.Type = types.StringValue(dir.Type)
		data.Options = types.StringValue(dir.Options)
		data.UnitFile = types.StringValue(dir.UnitFile)
		resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
		return
	}
	resp.Diagnostics.AddError(
		"Directory storage not found",
		fmt.Sprintf("No PVE-managed directory storage mounted at %s on node %s.", want, node),
	)
}
