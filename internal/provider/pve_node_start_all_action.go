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
	_ action.Action = &pveNodeStartAllAction{}
)

// NewPveNodeStartAllAction returns the action implementation.
func NewPveNodeStartAllAction() action.Action {
	return &pveNodeStartAllAction{}
}

// pveNodeStartAllAction starts the guests on a node via POST
// /nodes/{node}/startall and waits for the worker task.
type pveNodeStartAllAction struct {
	client *pveclient.Client
}

// pveNodeStartAllActionModel is the Terraform-facing shape.
type pveNodeStartAllActionModel struct {
	Node       types.String `tfsdk:"node"`
	Force      types.Bool   `tfsdk:"force"`
	MaxWorkers types.Int64  `tfsdk:"max_workers"`
	Vms        types.String `tfsdk:"vms"`
}

// Metadata implements action.Action.
func (a *pveNodeStartAllAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveNodeStartAll
}

// Schema implements action.Action.
func (a *pveNodeStartAllAction) Schema(_ context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "Starts all VMs and containers located on this node via `POST /nodes/{node}/startall` " +
			"(by default only those with `onboot=1`) and waits for the worker task to finish.",
		Attributes: map[string]actionschema.Attribute{
			"node": &actionschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The cluster node whose guests are started.",
			},
			"force": &actionschema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Issue the start command even if guests have `onboot` not set or set to off. Defaults to off.",
			},
			"max_workers": &actionschema.Int64Attribute{
				Optional:            true,
				Validators:          []validator.Int64{int64validator.Between(1, 64)},
				MarkdownDescription: "Maximum number of tasks running concurrently. Must be between 1 and 64. When unset, PVE uses `max_workers` from datacenter.cfg, falling back to the available CPU threads clamped to a maximum of 8.",
			},
			"vms": &actionschema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Only consider guests from this comma-separated list of VMIDs, e.g. `100,101`.",
			},
		},
	}
}

// Configure implements action.ActionWithConfigure.
func (a *pveNodeStartAllAction) Configure(_ context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
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
func (a *pveNodeStartAllAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var config pveNodeStartAllActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if a.client == nil {
		resp.Diagnostics.AddError(
			"Unconfigured pve_node_start_all",
			fmt.Sprintf("The provider client was not configured; cannot start all guests on node %s.", config.Node.ValueString()),
		)
		return
	}
	node := config.Node.ValueString()
	opts := pveclient.StartAllNodeGuestsOptions{}
	if !config.Force.IsNull() && !config.Force.IsUnknown() {
		v := config.Force.ValueBool()
		opts.Force = &v
	}
	if !config.MaxWorkers.IsNull() && !config.MaxWorkers.IsUnknown() {
		v := config.MaxWorkers.ValueInt64()
		opts.MaxWorkers = &v
	}
	if !config.Vms.IsNull() && !config.Vms.IsUnknown() {
		opts.Vms = config.Vms.ValueString()
	}
	progress := func(message string) {
		if resp.SendProgress != nil {
			resp.SendProgress(action.InvokeProgressEvent{Message: message})
		}
	}
	tflog.Info(ctx, "starting all guests on node", map[string]any{"node": node})
	progress(fmt.Sprintf("Starting guests on node %s", node))
	upid, err := a.client.StartAllNodeGuests(ctx, node, opts)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error invoking pve_node_start_all",
			fmt.Sprintf("starting all guests on node %s: %s", node, err),
		)
		return
	}
	taskNode, err := realmNodeFromUpid(upid)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error invoking pve_node_start_all",
			fmt.Sprintf("determining the task node for the startall worker on node %s: %s", node, err),
		)
		return
	}
	progress(fmt.Sprintf("Startall worker %s running on node %s; waiting for it to finish", upid, taskNode))
	if _, err := a.client.WaitForTask(ctx, taskNode, upid, pveclient.WaitForTaskOptions{}); err != nil {
		resp.Diagnostics.AddError(
			"Error invoking pve_node_start_all",
			fmt.Sprintf("waiting for the startall worker on node %s: %s", node, err),
		)
		return
	}
	progress(fmt.Sprintf("All requested guests on node %s started", node))
}
