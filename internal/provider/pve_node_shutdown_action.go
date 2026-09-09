// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/action"
	actionschema "github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ action.Action = &pveNodeShutdownAction{}
)

// NewPveNodeShutdownAction returns the action implementation.
func NewPveNodeShutdownAction() action.Action {
	return &pveNodeShutdownAction{}
}

// pveNodeShutdownAction powers off a cluster node (POST /nodes/{node}/status
// with upstream `command: shutdown`).
type pveNodeShutdownAction struct {
	client *pveclient.Client
}

// pveNodeShutdownActionModel is the Terraform-facing shape.
type pveNodeShutdownActionModel struct {
	Node types.String `tfsdk:"node"`
}

// Metadata implements action.Action.
func (a *pveNodeShutdownAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveNodeShutdown
}

// Schema implements action.Action.
func (a *pveNodeShutdownAction) Schema(_ context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "Powers off a cluster node via `POST /nodes/{node}/status` (upstream `command: shutdown`). " +
			"**Destructive:** the node goes down and stays down; every guest running on it is interrupted, and the " +
			"node must be powered on manually afterwards. Only HA-managed guests are relocated to other nodes. " +
			"Requires the `Sys.PowerMgmt` privilege on `/nodes/{node}`.",
		Attributes: map[string]actionschema.Attribute{
			"node": &actionschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The cluster node name to power off.",
			},
		},
	}
}

// Configure implements action.ActionWithConfigure.
func (a *pveNodeShutdownAction) Configure(_ context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*pveclient.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Action Configure Type",
			fmt.Sprintf("Expected *pveclient.Client, got: %T.", req.ProviderData),
		)
		return
	}
	a.client = client
}

// Invoke implements action.Action.
func (a *pveNodeShutdownAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var config pveNodeShutdownActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if a.client == nil {
		resp.Diagnostics.AddError(
			"Unconfigured pve_node_shutdown",
			fmt.Sprintf("The provider client was not configured; cannot shut down node %s.", config.Node.ValueString()),
		)
		return
	}
	node := config.Node.ValueString()
	progress := func(message string) {
		if resp.SendProgress != nil {
			resp.SendProgress(action.InvokeProgressEvent{Message: message})
		}
	}
	tflog.Info(ctx, "shutting down node", map[string]any{"node": node})
	progress(fmt.Sprintf("Sending shutdown command to node %s", node))
	if err := a.client.NodeShutdown(ctx, node); err != nil {
		resp.Diagnostics.AddError(
			"Error invoking pve_node_shutdown",
			fmt.Sprintf("shutting down node %s (command shutdown): %s", node, err),
		)
		return
	}
	tflog.Info(ctx, "node shutdown accepted", map[string]any{"node": node})
	progress(fmt.Sprintf("Node %s accepted the shutdown command and is powering off now", node))
}
