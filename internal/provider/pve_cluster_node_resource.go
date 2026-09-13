// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ resource.Resource                = &pveClusterNodeResource{}
	_ resource.ResourceWithConfigure   = &pveClusterNodeResource{}
	_ resource.ResourceWithImportState = &pveClusterNodeResource{}
)

// NewPveClusterNodeResource returns the resource implementation.
func NewPveClusterNodeResource() resource.Resource {
	return &pveClusterNodeResource{}
}

// pveClusterNodeResource manages cluster membership for a single node. The
// provider endpoint must target the node being joined; create runs
// POST /cluster/config/join on it and delete runs
// DELETE /cluster/config/nodes/{node} on any current member.
//
// Destructive: create and delete change cluster membership for every guest
// and resource running on the node.
type pveClusterNodeResource struct {
	client *pveclient.Client
}

// pveClusterNodeResourceModel is the Terraform-facing shape.
type pveClusterNodeResourceModel struct {
	Node         types.String `tfsdk:"node"`
	PeerHost     types.String `tfsdk:"peer_host"`
	PeerPassword types.String `tfsdk:"peer_password"`
	Fingerprint  types.String `tfsdk:"fingerprint"`
	NodeID       types.Int64  `tfsdk:"nodeid"`
	Votes        types.Int64  `tfsdk:"votes"`
	Force        types.Bool   `tfsdk:"force"`
	Link0        types.String `tfsdk:"link0"`
	IP           types.String `tfsdk:"ip"`
	ID           types.String `tfsdk:"id"`
}

// Metadata implements resource.Resource.
func (r *pveClusterNodeResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveClusterNode
}

// Schema implements resource.Resource.
func (r *pveClusterNodeResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages cluster membership for a single Proxmox VE node. " +
			"The provider endpoint must target the node being joined (create calls `POST /cluster/config/join` on it). " +
			"Deleting the resource removes the node from the cluster configuration.\n\n" +
			"**Warning:** create and delete are destructive cluster-level operations — they change cluster membership " +
			"for every guest, container, and storage resource running on the node. Read-only imported nodes are " +
			"removed from the cluster configuration on destroy.",
		Attributes: map[string]schema.Attribute{
			"node": schema.StringAttribute{
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
				MarkdownDescription: "Name of the cluster node this resource manages. Must match the node the provider endpoint connects to at create time.",
			},
			"peer_host": schema.StringAttribute{
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
				MarkdownDescription: "Hostname (or IP) of an existing cluster member to join through.",
			},
			"peer_password": schema.StringAttribute{
				Required:            true,
				Sensitive:           true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
				MarkdownDescription: "Superuser (root) password of the peer cluster member. Sent to the peer's API during the join handshake; never stored by PVE.",
			},
			"fingerprint": schema.StringAttribute{
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
				MarkdownDescription: "Certificate SHA 256 fingerprint of the peer cluster member, in colon-separated hex form (see `GET /cluster/config/join` on the peer).",
			},
			"nodeid": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				Validators:          []validator.Int64{int64validator.AtLeast(1)},
				MarkdownDescription: "Node id for this node. Must be 1 or greater. Assigned automatically when omitted.",
			},
			"votes": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				Validators:          []validator.Int64{int64validator.AtLeast(0)},
				MarkdownDescription: "Number of corosync votes for this node. Must be 0 or greater. Defaults to 1 when omitted.",
			},
			"force": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Do not throw an error if the node already exists in the cluster configuration.",
			},
			"link0": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Corosync link0 address (`ring0_addr`) of this node as recorded in the cluster configuration.",
			},
			"ip": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Primary IP address (`pve_addr`) of this node as recorded in the cluster configuration.",
			},
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Identifier of this resource. Equal to `node`.",
			},
		},
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveClusterNodeResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// Nil provider data leaves the resource unconfigured; Terraform
	// reaches this path during configuration validation.
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*pveclient.Client)
	if !ok || client == nil {
		resp.Diagnostics.AddError(
			"Unexpected pve_cluster_node Configure type",
			fmt.Sprintf("Expected *pveclient.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}
	r.client = client
}

// Create joins the connected node into an existing cluster.
func (r *pveClusterNodeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveClusterNodeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	nodeReq := pveclient.ClusterJoinRequest{
		Fingerprint: plan.Fingerprint.ValueString(),
		Hostname:    plan.PeerHost.ValueString(),
		Password:    plan.PeerPassword.ValueString(),
	}
	if !plan.NodeID.IsNull() {
		nodeReq.NodeID = plan.NodeID.ValueInt64Pointer()
	}
	if !plan.Votes.IsNull() {
		nodeReq.Votes = plan.Votes.ValueInt64Pointer()
	}
	if !plan.Force.IsNull() {
		nodeReq.Force = plan.Force.ValueBoolPointer()
	}
	upid, err := r.client.JoinCluster(ctx, nodeReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_cluster_node",
			fmt.Sprintf("joining node %s into cluster via peer %s: %s", plan.Node.ValueString(), plan.PeerHost.ValueString(), err),
		)
		return
	}
	if strings.HasPrefix(upid, "UPID:") {
		if _, err := r.client.WaitForTask(ctx, plan.Node.ValueString(), upid, pveclient.WaitForTaskOptions{}); err != nil {
			resp.Diagnostics.AddError(
				"Error waiting for pve_cluster_node create task",
				fmt.Sprintf("join of node %s did not complete: %s", plan.Node.ValueString(), err),
			)
			return
		}
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_cluster_node after create",
			fmt.Sprintf("reading cluster node %s after join: %s", plan.Node.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read implements resource.Resource.
func (r *pveClusterNodeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveClusterNodeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.readInto(ctx, &state); err != nil {
		var nf *pveClusterNodeNotFoundError
		if errors.As(err, &nf) || isPVEClientNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error reading pve_cluster_node",
			fmt.Sprintf("reading cluster node %s: %s", state.Node.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is a no-op: every attribute is ForceNew, so the framework replaces
// instead of updating; refresh computed state anyway.
func (r *pveClusterNodeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveClusterNodeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_cluster_node",
			fmt.Sprintf("reading cluster node %s: %s", plan.Node.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes the node from the cluster configuration. An already-absent
// node is a successful delete; other API errors (including "last node in
// cluster") are surfaced.
func (r *pveClusterNodeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveClusterNodeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.RemoveClusterNode(ctx, state.Node.ValueString()); err != nil {
		if isPVEClientNotFound(err) {
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting pve_cluster_node",
			fmt.Sprintf("removing cluster node %s: %s", state.Node.ValueString(), err),
		)
		return
	}
	resp.State.RemoveResource(ctx)
}

// ImportState parses an import ID of the form `<node>`.
func (r *pveClusterNodeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		resp.Diagnostics.AddError(
			"Invalid pve_cluster_node import ID",
			"expected import ID `<node>` (for example `pve1`)",
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("node"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

// readInto verifies the node is still a cluster member and refreshes the
// computed attributes from the corosync nodelist.
func (r *pveClusterNodeResource) readInto(ctx context.Context, m *pveClusterNodeResourceModel) error {
	nodes, err := r.client.ListClusterNodesConfig(ctx)
	if err != nil {
		return fmt.Errorf("listing cluster nodes: %w", err)
	}
	member := false
	for _, n := range nodes {
		if n.Node == m.Node.ValueString() {
			member = true
			break
		}
	}
	if !member {
		return &pveClusterNodeNotFoundError{node: m.Node.ValueString()}
	}
	info, err := r.client.GetJoinClusterInfo(ctx, m.Node.ValueString())
	if err != nil {
		return fmt.Errorf("getting cluster join info: %w", err)
	}
	m.ID = m.Node
	for _, n := range info.Nodelist {
		if n.Name != m.Node.ValueString() {
			continue
		}
		if n.NodeID != nil {
			m.NodeID = types.Int64Value(*n.NodeID)
		} else {
			m.NodeID = types.Int64Null()
		}
		if n.QuorumVotes != nil {
			m.Votes = types.Int64Value(*n.QuorumVotes)
		} else {
			m.Votes = types.Int64Null()
		}
		m.Link0 = types.StringValue(n.Ring0Addr)
		m.IP = types.StringValue(n.PVEAddr)
		return nil
	}
	return &pveClusterNodeNotFoundError{node: m.Node.ValueString()}
}

// pveClusterNodeNotFoundError signals the node left the cluster; mapped to
// RemoveResource by Read.
type pveClusterNodeNotFoundError struct {
	node string
}

// Error implements error.
func (e *pveClusterNodeNotFoundError) Error() string {
	return fmt.Sprintf("cluster node %s is not a member of the cluster", e.node)
}
