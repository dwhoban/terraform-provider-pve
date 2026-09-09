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
	_ action.Action = &pveNodeRebootAction{}
)

// NewPveNodeRebootAction returns the action implementation.
func NewPveNodeRebootAction() action.Action {
	return &pveNodeRebootAction{}
}

// pveNodeRebootAction reboots a cluster node (POST /nodes/{node}/status with
// upstream `command: reboot`).
type pveNodeRebootAction struct {
	client *pveclient.Client
}

// pveNodeRebootActionModel is the Terraform-facing shape.
type pveNodeRebootActionModel struct {
	Node types.String `tfsdk:"node"`
}

// Metadata implements action.Action.
func (a *pveNodeRebootAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveNodeReboot
}

// Schema implements action.Action.
func (a *pveNodeRebootAction) Schema(_ context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "Reboots a cluster node via `POST /nodes/{node}/status` (upstream `command: reboot`). " +
			"**Destructive:** the node goes down immediately and every guest running on it is interrupted; " +
			"only HA-managed guests are relocated to other nodes. Requires the `Sys.PowerMgmt` privilege on `/nodes/{node}`.",
		Attributes: map[string]actionschema.Attribute{
			"node": &actionschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The cluster node name to reboot.",
			},
		},
	}
}

// Configure implements action.ActionWithConfigure.
func (a *pveNodeRebootAction) Configure(_ context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
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
func (a *pveNodeRebootAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var config pveNodeRebootActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if a.client == nil {
		resp.Diagnostics.AddError(
			"Unconfigured pve_node_reboot",
			fmt.Sprintf("The provider client was not configured; cannot reboot node %s.", config.Node.ValueString()),
		)
		return
	}
	node := config.Node.ValueString()
	progress := func(message string) {
		if resp.SendProgress != nil {
			resp.SendProgress(action.InvokeProgressEvent{Message: message})
		}
	}
	tflog.Info(ctx, "rebooting node", map[string]any{"node": node})
	progress(fmt.Sprintf("Sending reboot command to node %s", node))
	if err := a.client.NodeReboot(ctx, node); err != nil {
		resp.Diagnostics.AddError(
			"Error invoking pve_node_reboot",
			fmt.Sprintf("rebooting node %s (command reboot): %s", node, err),
		)
		return
	}
	tflog.Info(ctx, "node reboot accepted", map[string]any{"node": node})
	progress(fmt.Sprintf("Node %s accepted the reboot command and is going down now", node))
}
