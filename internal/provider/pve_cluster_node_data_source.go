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
	_ datasource.DataSource              = &pveClusterNodeDataSource{}
	_ datasource.DataSourceWithConfigure = &pveClusterNodeDataSource{}
)

// NewPveClusterNodeDataSource returns the data source implementation.
func NewPveClusterNodeDataSource() datasource.DataSource {
	return &pveClusterNodeDataSource{}
}

// pveClusterNodeDataSource reads the cluster membership record of a single
// node from GET /cluster/config/nodes and the corosync nodelist in
// GET /cluster/config/join.
type pveClusterNodeDataSource struct {
	client *pveclient.Client
}

// pveClusterNodeDataSourceModel is the Terraform-facing shape.
type pveClusterNodeDataSourceModel struct {
	Node          types.String `tfsdk:"node"`
	NodeID        types.Int64  `tfsdk:"nodeid"`
	Votes         types.Int64  `tfsdk:"votes"`
	Link0         types.String `tfsdk:"link0"`
	IP            types.String `tfsdk:"ip"`
	Fingerprint   types.String `tfsdk:"fingerprint"`
	ConfigDigest  types.String `tfsdk:"config_digest"`
	PreferredNode types.String `tfsdk:"preferred_node"`
	ID            types.String `tfsdk:"id"`
}

// Metadata implements datasource.DataSource.
func (d *pveClusterNodeDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveClusterNode
}

// Schema implements datasource.DataSource.
func (d *pveClusterNodeDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads the cluster membership record of a single Proxmox VE node: its corosync node id, votes, link address, and certificate fingerprint.",
		Attributes: map[string]schema.Attribute{
			"node": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Name of the cluster node to read.",
			},
			"nodeid": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Node id for this node in the corosync configuration.",
			},
			"votes": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Number of corosync quorum votes for this node.",
			},
			"link0": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Corosync link0 address (`ring0_addr`) of this node.",
			},
			"ip": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Primary IP address (`pve_addr`) of this node.",
			},
			"fingerprint": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Certificate SHA 256 fingerprint (`pve_fp`) of this node.",
			},
			"config_digest": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Digest of the current cluster configuration.",
			},
			"preferred_node": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Node the cluster considers the preferred join information source.",
			},
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Identifier of this data source. Equal to `node`.",
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveClusterNodeDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	// Nil provider data leaves the data source unconfigured; Terraform
	// reaches this path during configuration validation.
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*pveclient.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected pve_cluster_node data source Configure type",
			fmt.Sprintf("Expected *pveclient.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}
	d.client = client
}

// Read implements datasource.DataSource.
func (d *pveClusterNodeDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state pveClusterNodeDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	node := state.Node.ValueString()
	member := false
	nodes, err := d.client.ListClusterNodesConfig(ctx)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_cluster_node data source",
			fmt.Sprintf("listing cluster nodes: %s", err),
		)
		return
	}
	for _, n := range nodes {
		if n.Node == node {
			member = true
			break
		}
	}
	if !member {
		resp.Diagnostics.AddError(
			"Cluster node not found",
			fmt.Sprintf("node %s is not a member of the cluster", node),
		)
		return
	}
	info, err := d.client.GetJoinClusterInfo(ctx, node)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_cluster_node data source",
			fmt.Sprintf("getting cluster join info for node %s: %s", node, err),
		)
		return
	}
	state.ID = types.StringValue(node)
	state.ConfigDigest = types.StringValue(info.ConfigDigest)
	state.PreferredNode = types.StringValue(info.PreferredNode)
	found := false
	for _, n := range info.Nodelist {
		if n.Name != node {
			continue
		}
		found = true
		if n.NodeID != nil {
			state.NodeID = types.Int64Value(*n.NodeID)
		} else {
			state.NodeID = types.Int64Null()
		}
		if n.QuorumVotes != nil {
			state.Votes = types.Int64Value(*n.QuorumVotes)
		} else {
			state.Votes = types.Int64Null()
		}
		state.Link0 = types.StringValue(n.Ring0Addr)
		state.IP = types.StringValue(n.PVEAddr)
		state.Fingerprint = types.StringValue(n.PVEFP)
		break
	}
	if !found {
		resp.Diagnostics.AddError(
			"Cluster node not found",
			fmt.Sprintf("node %s is missing from the corosync nodelist", node),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
