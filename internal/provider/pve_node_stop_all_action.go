// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework/action"
	actionschema "github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ action.Action = &pveNodeStopAllAction{}
)

// NewPveNodeStopAllAction returns the action implementation.
func NewPveNodeStopAllAction() action.Action {
	return &pveNodeStopAllAction{}
}

// pveNodeStopAllAction stops the guests on a node via POST
// /nodes/{node}/stopall and waits for the worker task.
type pveNodeStopAllAction struct {
	client *pveclient.Client
}

// pveNodeStopAllActionModel is the Terraform-facing shape.
type pveNodeStopAllActionModel struct {
	Node       types.String `tfsdk:"node"`
	ForceStop  types.Bool   `tfsdk:"force_stop"`
	MaxWorkers types.Int64  `tfsdk:"max_workers"`
	Timeout    types.Int64  `tfsdk:"timeout"`
	Vms        types.String `tfsdk:"vms"`
}

// Metadata implements action.Action.
func (a *pveNodeStopAllAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveNodeStopAll
}

// Schema implements action.Action.
func (a *pveNodeStopAllAction) Schema(_ context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "**Destructive:** stops all VMs and containers on this node via " +
			"`POST /nodes/{node}/stopall` and waits for the worker task to finish. Depending on `force_stop`, " +
			"guests that do not shut down within `timeout` are simply aborted or hard-stopped.",
		Attributes: map[string]actionschema.Attribute{
			"node": &actionschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The cluster node whose guests are stopped.",
			},
			"force_stop": &actionschema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Force a hard-stop after the timeout. Defaults to true on the API.",
			},
			"max_workers": &actionschema.Int64Attribute{
				Optional:            true,
				Validators:          []validator.Int64{int64validator.Between(1, 64)},
				MarkdownDescription: "Maximum number of tasks running concurrently. Must be between 1 and 64. When unset, PVE uses `max_workers` from datacenter.cfg, falling back to the available CPU threads clamped to a maximum of 8.",
			},
			"timeout": &actionschema.Int64Attribute{
				Optional:            true,
				Validators:          []validator.Int64{int64validator.Between(0, 7200)},
				MarkdownDescription: "Timeout in seconds for each guest shutdown task; must be between 0 and 7200. Depending on `force_stop`, the shutdown is then simply aborted or a hard-stop is forced. Defaults to 180 on the API.",
			},
			"vms": &actionschema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Only consider guests with these IDs (comma-separated VMID list), e.g. `100,101`.",
			},
		},
	}
}

// Configure implements action.ActionWithConfigure.
func (a *pveNodeStopAllAction) Configure(_ context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
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
func (a *pveNodeStopAllAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var config pveNodeStopAllActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if a.client == nil {
		resp.Diagnostics.AddError(
			"Unconfigured pve_node_stop_all",
			fmt.Sprintf("The provider client was not configured; cannot stop all guests on node %s.", config.Node.ValueString()),
		)
		return
	}
	node := config.Node.ValueString()
	opts := pveclient.StopAllNodeGuestsOptions{}
	if !config.ForceStop.IsNull() && !config.ForceStop.IsUnknown() {
		v := config.ForceStop.ValueBool()
		opts.ForceStop = &v
	}
	if !config.MaxWorkers.IsNull() && !config.MaxWorkers.IsUnknown() {
		v := config.MaxWorkers.ValueInt64()
		opts.MaxWorkers = &v
	}
	if !config.Timeout.IsNull() && !config.Timeout.IsUnknown() {
		v := config.Timeout.ValueInt64()
		opts.Timeout = &v
	}
	if !config.Vms.IsNull() && !config.Vms.IsUnknown() {
		opts.Vms = config.Vms.ValueString()
	}
	progress := func(message string) {
		if resp.SendProgress != nil {
			resp.SendProgress(action.InvokeProgressEvent{Message: message})
		}
	}
	tflog.Info(ctx, "stopping all guests on node", map[string]any{"node": node})
	progress(fmt.Sprintf("Stopping guests on node %s", node))
	upid, err := a.client.StopAllNodeGuests(ctx, node, opts)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error invoking pve_node_stop_all",
			fmt.Sprintf("stopping all guests on node %s: %s", node, err),
		)
		return
	}
	taskNode, err := realmNodeFromUpid(upid)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error invoking pve_node_stop_all",
			fmt.Sprintf("determining the task node for the stopall worker on node %s: %s", node, err),
		)
		return
	}
	progress(fmt.Sprintf("Stopall worker %s running on node %s; waiting for it to finish", upid, taskNode))
	if _, err := a.client.WaitForTask(ctx, taskNode, upid, pveclient.WaitForTaskOptions{}); err != nil {
		resp.Diagnostics.AddError(
			"Error invoking pve_node_stop_all",
			fmt.Sprintf("waiting for the stopall worker on node %s: %s", node, err),
		)
		return
	}
	progress(fmt.Sprintf("All requested guests on node %s stopped", node))
}
