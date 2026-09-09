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

	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ action.Action = &pveNodeSuspendAllAction{}
)

// NewPveNodeSuspendAllAction returns the action implementation.
func NewPveNodeSuspendAllAction() action.Action {
	return &pveNodeSuspendAllAction{}
}

// pveNodeSuspendAllAction suspends the VMs on a node via POST
// /nodes/{node}/suspendall and waits for the worker task.
type pveNodeSuspendAllAction struct {
	client *pveclient.Client
}

// pveNodeSuspendAllActionModel is the Terraform-facing shape.
type pveNodeSuspendAllActionModel struct {
	Node       types.String `tfsdk:"node"`
	MaxWorkers types.Int64  `tfsdk:"max_workers"`
	Vms        types.String `tfsdk:"vms"`
}

// Metadata implements action.Action.
func (a *pveNodeSuspendAllAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveNodeSuspendAll
}

// Schema implements action.Action.
func (a *pveNodeSuspendAllAction) Schema(_ context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "**Disruptive:** suspends all VMs on this node via `POST /nodes/{node}/suspendall` " +
			"and waits for the worker task to finish. Suspended VMs keep their memory allocated on the node.",
		Attributes: map[string]actionschema.Attribute{
			"node": &actionschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The cluster node whose guests are suspended.",
			},
			"max_workers": &actionschema.Int64Attribute{
				Optional:            true,
				Validators:          []validator.Int64{int64validator.Between(1, 64)},
				MarkdownDescription: "Maximum number of parallel jobs. Must be between 1 and 64. When unset, PVE uses `max_workers` from datacenter.cfg, falling back to the available CPU threads clamped to a maximum of 8.",
			},
			"vms": &actionschema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Only consider guests with these IDs (comma-separated VMID list), e.g. `100,101`.",
			},
		},
	}
}

// Configure implements action.ActionWithConfigure.
func (a *pveNodeSuspendAllAction) Configure(_ context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
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
func (a *pveNodeSuspendAllAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var config pveNodeSuspendAllActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if a.client == nil {
		resp.Diagnostics.AddError(
			"Unconfigured pve_node_suspend_all",
			fmt.Sprintf("The provider client was not configured; cannot suspend all guests on node %s.", config.Node.ValueString()),
		)
		return
	}
	node := config.Node.ValueString()
	opts := pveclient.SuspendAllNodeGuestsOptions{}
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
	tflog.Info(ctx, "suspending all guests on node", map[string]any{"node": node})
	progress(fmt.Sprintf("Suspending guests on node %s", node))
	upid, err := a.client.SuspendAllNodeGuests(ctx, node, opts)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error invoking pve_node_suspend_all",
			fmt.Sprintf("suspending all guests on node %s: %s", node, err),
		)
		return
	}
	taskNode, err := realmNodeFromUpid(upid)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error invoking pve_node_suspend_all",
			fmt.Sprintf("determining the task node for the suspendall worker on node %s: %s", node, err),
		)
		return
	}
	progress(fmt.Sprintf("Suspendall worker %s running on node %s; waiting for it to finish", upid, taskNode))
	if _, err := a.client.WaitForTask(ctx, taskNode, upid, pveclient.WaitForTaskOptions{}); err != nil {
		resp.Diagnostics.AddError(
			"Error invoking pve_node_suspend_all",
			fmt.Sprintf("waiting for the suspendall worker on node %s: %s", node, err),
		)
		return
	}
	progress(fmt.Sprintf("All requested guests on node %s suspended", node))
}
