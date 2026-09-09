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
	_ datasource.DataSource              = &pveNodesDataSource{}
	_ datasource.DataSourceWithConfigure = &pveNodesDataSource{}
)

// NewPveNodesDataSource returns the data source implementation.
func NewPveNodesDataSource() datasource.DataSource {
	return &pveNodesDataSource{}
}

// pveNodesDataSource lists the nodes in the cluster (GET /nodes).
type pveNodesDataSource struct {
	client *pveclient.Client
}

// pveNodesDataSourceModel is the Terraform-facing shape.
type pveNodesDataSourceModel struct {
	ID     types.String                  `tfsdk:"id"`
	Node   types.String                  `tfsdk:"node"`
	Status types.String                  `tfsdk:"status"`
	Nodes  []pveNodesDataSourceNodeModel `tfsdk:"nodes"`
}

// pveNodesDataSourceNodeModel mirrors the per-row schema of the
// cluster node list. Field names line up with the pveclient struct.
type pveNodesDataSourceNodeModel struct {
	Node           types.String  `tfsdk:"node"`
	Status         types.String  `tfsdk:"status"`
	CPU            types.Float64 `tfsdk:"cpu"`
	MaxCPU         types.Int64   `tfsdk:"maxcpu"`
	Mem            types.Int64   `tfsdk:"mem"`
	MaxMem         types.Int64   `tfsdk:"maxmem"`
	Level          types.String  `tfsdk:"level"`
	Uptime         types.Int64   `tfsdk:"uptime"`
	SSLFingerprint types.String  `tfsdk:"ssl_fingerprint"`
	ID             types.String  `tfsdk:"id"`
	IP             types.String  `tfsdk:"ip"`
}

// Metadata implements datasource.DataSource.
func (d *pveNodesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveNodes
}

// Schema implements datasource.DataSource.
func (d *pveNodesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists every node in the Proxmox VE cluster as reported by `GET /nodes`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Static identifier for the cluster node index.",
			},
			"node": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Optional filter: return only the node whose name exactly matches this value.",
			},
			"status": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Optional filter: return only nodes whose `status` equals this value. PVE reports one of `online`, `offline`, or `unknown`.",
			},
			"nodes": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Cluster nodes matching the filters (or all nodes when no filter is set).",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"node":            schema.StringAttribute{Computed: true, MarkdownDescription: "Node name."},
						"status":          schema.StringAttribute{Computed: true, MarkdownDescription: "Node status: `online`, `offline`, or `unknown`."},
						"cpu":             schema.Float64Attribute{Computed: true, MarkdownDescription: "CPU usage fraction (0.0–1.0)."},
						"maxcpu":          schema.Int64Attribute{Computed: true, MarkdownDescription: "Number of CPU cores."},
						"mem":             schema.Int64Attribute{Computed: true, MarkdownDescription: "Used memory in bytes."},
						"maxmem":          schema.Int64Attribute{Computed: true, MarkdownDescription: "Total memory in bytes."},
						"level":           schema.StringAttribute{Computed: true, MarkdownDescription: "Node capability level (e.g. `c` for current generation)."},
						"uptime":          schema.Int64Attribute{Computed: true, MarkdownDescription: "Uptime in seconds."},
						"ssl_fingerprint": schema.StringAttribute{Computed: true, MarkdownDescription: "TLS certificate fingerprint (PVE 8.x)."},
						"id":              schema.StringAttribute{Computed: true, MarkdownDescription: "Cluster-internal node ID (PVE 8.x)."},
						"ip":              schema.StringAttribute{Computed: true, MarkdownDescription: "Last-known node IP address."},
					},
				},
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveNodesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
func (d *pveNodesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveNodesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	nodes, err := d.client.ListClusterNodes(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading pve_nodes", fmt.Sprintf("listing cluster nodes: %s", err))
		return
	}
	nodeFilter := data.Node.ValueString()
	statusFilter := data.Status.ValueString()
	rows := make([]pveNodesDataSourceNodeModel, 0, len(nodes))
	for _, n := range nodes {
		if nodeFilter != "" && n.Node != nodeFilter {
			continue
		}
		if statusFilter != "" && n.Status != statusFilter {
			continue
		}
		rows = append(rows, pveNodesDataSourceNodeModel{
			Node:           types.StringValue(n.Node),
			Status:         types.StringValue(n.Status),
			CPU:            types.Float64Value(n.CPU),
			MaxCPU:         types.Int64Value(int64(n.MaxCPU)),
			Mem:            types.Int64Value(n.Mem),
			MaxMem:         types.Int64Value(n.MaxMem),
			Level:          types.StringValue(n.Level),
			Uptime:         types.Int64Value(n.Uptime),
			SSLFingerprint: types.StringValue(n.SSLFingerprint),
			ID:             types.StringValue(n.ID),
			IP:             types.StringValue(n.IP),
		})
	}
	data.ID = types.StringValue("pve_nodes")
	data.Nodes = rows
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
