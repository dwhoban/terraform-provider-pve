// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/action"
	actionschema "github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ action.Action = &pveNodeExecuteAction{}
)

// NewPveNodeExecuteAction returns the action implementation.
func NewPveNodeExecuteAction() action.Action {
	return &pveNodeExecuteAction{}
}

// pveNodeExecuteAction executes the pin's root-only command batch on a
// node (POST /nodes/{node}/execute).
type pveNodeExecuteAction struct {
	client *pveclient.Client
}

// pveNodeExecuteActionModel is the Terraform-facing shape.
type pveNodeExecuteActionModel struct {
	Node     types.String `tfsdk:"node"`
	Commands types.String `tfsdk:"commands"`
}

// Metadata implements action.Action.
func (a *pveNodeExecuteAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveNodeExecute
}

// Schema implements action.Action.
func (a *pveNodeExecuteAction) Schema(_ context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "**Warning: this action performs root-only arbitrary command execution on the " +
			"node — validate the provenance of any configuration using it.** Runs the pin's command batch " +
			"(`POST /nodes/{node}/execute`): `commands` is a JSON-encoded array of command objects, each with " +
			"`args` (object), `method` (GET, POST, PUT, or DELETE), and `path` (a relative API path on the node), " +
			"executed in order. The action completes once the batch has run; per-command results are " +
			"intentionally not surfaced.",
		Attributes: map[string]actionschema.Attribute{
			"node": &actionschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The cluster node whose API executes the command batch. **Root-only command execution — validate the provenance of any configuration using this action.**",
			},
			"commands": &actionschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "JSON-encoded array of command objects, each with `args` (object), `method` (GET, POST, PUT, or DELETE), and `path` (a relative API path on this node). **Root-only command execution — validate the provenance of any configuration using this action.**",
			},
		},
	}
}

// Configure implements action.ActionWithConfigure.
func (a *pveNodeExecuteAction) Configure(_ context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
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
func (a *pveNodeExecuteAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var config pveNodeExecuteActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if a.client == nil {
		resp.Diagnostics.AddError(
			"Unconfigured pve_node_execute",
			fmt.Sprintf("The provider client was not configured; cannot execute commands on node %s.", config.Node.ValueString()),
		)
		return
	}
	node := config.Node.ValueString()
	commands := strings.TrimSpace(config.Commands.ValueString())
	// Fail fast on malformed batches with a clear message instead of a
	// 500 from the node.
	var batch []json.RawMessage
	if err := json.Unmarshal([]byte(commands), &batch); err != nil {
		resp.Diagnostics.AddError(
			"Invalid pve_node_execute commands",
			fmt.Sprintf("`commands` must be a JSON-encoded array of command objects for node %s: %s", node, err),
		)
		return
	}
	progress := func(message string) {
		if resp.SendProgress != nil {
			resp.SendProgress(action.InvokeProgressEvent{Message: message})
		}
	}
	tflog.Info(ctx, "executing command batch on node", map[string]any{"node": node, "commands": len(batch)})
	progress(fmt.Sprintf("Executing %d commands on node %s", len(batch), node))
	count, err := a.client.NodeExecute(ctx, node, commands)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error invoking pve_node_execute",
			fmt.Sprintf("executing command batch on node %s: %s", node, err),
		)
		return
	}
	// Per-command results are discarded by the client (ADR Consequences
	// mandate: no output echo); only the count is reported here.
	tflog.Info(ctx, "command batch finished", map[string]any{"node": node, "commands": count})
	progress(fmt.Sprintf("Command batch on node %s finished; %d commands executed (results are not surfaced)", node, count))
}
