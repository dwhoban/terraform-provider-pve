// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// Ensure the framework interfaces are satisfied.
var (
	_ datasource.DataSource              = &pveClusterStatusDataSource{}
	_ datasource.DataSourceWithConfigure = &pveClusterStatusDataSource{}
)

// NewPveClusterStatusDataSource returns the data source implementation.
func NewPveClusterStatusDataSource() datasource.DataSource {
	return &pveClusterStatusDataSource{}
}

// pveClusterStatusDataSource reads the cluster status overview
// (GET /cluster/status), enriched with the corosync totem settings
// (GET /cluster/config/totem) and QDevice status (GET
// /cluster/config/qdevice).
type pveClusterStatusDataSource struct {
	client *pveclient.Client
}

// pveClusterStatusDataSourceModel is the Terraform-facing shape.
type pveClusterStatusDataSourceModel struct {
	ID         types.String                   `tfsdk:"id"`
	Name       types.String                   `tfsdk:"name"`
	Quorate    types.Bool                     `tfsdk:"quorate"`
	NodesCount types.Int64                    `tfsdk:"nodes_count"`
	Version    types.Int64                    `tfsdk:"version"`
	Totem      types.Map                      `tfsdk:"totem"`
	QDevice    types.Map                      `tfsdk:"qdevice"`
	Nodes      []pveClusterStatusNodeRowModel `tfsdk:"nodes"`
}

// pveClusterStatusNodeRowModel mirrors the per-node entries of the
// /cluster/status response.
type pveClusterStatusNodeRowModel struct {
	ID     types.String `tfsdk:"id"`
	Name   types.String `tfsdk:"name"`
	IP     types.String `tfsdk:"ip"`
	Level  types.String `tfsdk:"level"`
	Local  types.Bool   `tfsdk:"local"`
	NodeID types.Int64  `tfsdk:"nodeid"`
	Online types.Bool   `tfsdk:"online"`
}

// Metadata implements datasource.DataSource.
func (d *pveClusterStatusDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveClusterStatus
}

// Schema implements datasource.DataSource.
func (d *pveClusterStatusDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Cluster status information from `GET /cluster/status`, plus the corosync totem settings (`GET /cluster/config/totem`) and QDevice status (`GET /cluster/config/qdevice`).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Static identifier for the cluster status data source.",
			},
			"name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Name of the cluster, from the summary entry (null on a standalone node).",
			},
			"quorate": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether a majority of nodes is online so the cluster can make decisions (null on a standalone node).",
			},
			"nodes_count": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Node count of the cluster, including offline nodes (null on a standalone node).",
			},
			"version": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Current version of the corosync configuration file (null on a standalone node).",
			},
			"totem": schema.MapAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Corosync totem protocol settings as a name/value map. PVE types this object loosely, so non-string values are rendered as their literal text.",
			},
			"qdevice": schema.MapAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "QDevice status as a name/value map, rendered the same way as `totem`.",
			},
			"nodes": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Per-node cluster membership entries.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":     schema.StringAttribute{Computed: true, MarkdownDescription: "Entry ID (e.g. `node/pve1`)."},
						"name":   schema.StringAttribute{Computed: true, MarkdownDescription: "Node name."},
						"ip":     schema.StringAttribute{Computed: true, MarkdownDescription: "IP of the resolved node name."},
						"level":  schema.StringAttribute{Computed: true, MarkdownDescription: "Proxmox VE subscription level of the node."},
						"local":  schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether this is the responding node."},
						"nodeid": schema.Int64Attribute{Computed: true, MarkdownDescription: "Node ID from the corosync configuration."},
						"online": schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the node is online."},
					},
				},
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveClusterStatusDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
func (d *pveClusterStatusDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveClusterStatusDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	entries, err := d.client.GetClusterStatus(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading pve_cluster_status", fmt.Sprintf("reading cluster status: %s", err))
		return
	}
	nodes := make([]pveClusterStatusNodeRowModel, 0, len(entries))
	data.Name = types.StringNull()
	data.Quorate = types.BoolNull()
	data.NodesCount = types.Int64Null()
	data.Version = types.Int64Null()
	for _, entry := range entries {
		switch entry.Type {
		case "cluster":
			data.Name = types.StringValue(entry.Name)
			data.Quorate = types.BoolValue(entry.Quorate)
			data.NodesCount = types.Int64Value(int64(entry.Nodes))
			data.Version = types.Int64Value(entry.Version)
		case "node":
			nodes = append(nodes, pveClusterStatusNodeRowModel{
				ID:     types.StringValue(entry.ID),
				Name:   types.StringValue(entry.Name),
				IP:     types.StringValue(entry.IP),
				Level:  types.StringValue(entry.Level),
				Local:  types.BoolValue(entry.Local),
				NodeID: types.Int64Value(int64(entry.NodeID)),
				Online: types.BoolValue(entry.Online),
			})
		}
	}
	totem, err := d.client.GetClusterTotem(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading pve_cluster_status", fmt.Sprintf("reading corosync totem settings: %s", err))
		return
	}
	qdevice, err := d.client.GetClusterQDevice(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading pve_cluster_status", fmt.Sprintf("reading QDevice status: %s", err))
		return
	}
	data.ID = types.StringValue("pve_cluster_status")
	data.Totem = pveClusterStatusStringMap(totem)
	data.QDevice = pveClusterStatusStringMap(qdevice)
	data.Nodes = nodes
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// pveClusterStatusStringMap converts a client string map into a types.Map
// of strings.
func pveClusterStatusStringMap(in map[string]string) types.Map {
	values := make(map[string]attr.Value, len(in))
	for key, value := range in {
		values[key] = types.StringValue(value)
	}
	return types.MapValueMust(types.StringType, values)
}
