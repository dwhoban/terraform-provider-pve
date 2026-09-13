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

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ action.Action              = &pveTaskCancelAction{}
	_ action.ActionWithConfigure = &pveTaskCancelAction{}
)

// NewPveTaskCancelAction returns the action implementation.
func NewPveTaskCancelAction() action.Action {
	return &pveTaskCancelAction{}
}

// pveTaskCancelAction stops a running task
// (DELETE /nodes/{node}/tasks/{upid}).
type pveTaskCancelAction struct {
	client *pveclient.Client
}

// pveTaskCancelActionModel is the Terraform-facing shape.
type pveTaskCancelActionModel struct {
	Node types.String `tfsdk:"node"`
	UPID types.String `tfsdk:"upid"`
}

// Metadata implements action.Action.
func (a *pveTaskCancelAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveTaskCancel
}

// Schema implements action.Action.
func (a *pveTaskCancelAction) Schema(_ context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "Stops a running task (`DELETE /nodes/{node}/tasks/{upid}`, upstream `stop_task`). The call " +
			"returns once the stop request is delivered, not when the worker has exited; the cancelled task finishes " +
			"with a non-OK exit status, so a subsequent `WaitForTask` on it reports failure by design. Requires the " +
			"`Sys.Modify` privilege on `/nodes/{node}` unless the caller owns the task.",
		Attributes: map[string]actionschema.Attribute{
			"node": &actionschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The cluster node the task runs on.",
			},
			"upid": &actionschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The task UPID to stop, e.g. `UPID:pve1:0000ABCD:...::vzdump:100:root@pam:`.",
			},
		},
	}
}

// Configure implements action.ActionWithConfigure.
func (a *pveTaskCancelAction) Configure(ctx context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	a.client = containerActionConfigure(req, resp)
}

// Invoke implements action.Action.
func (a *pveTaskCancelAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var config pveTaskCancelActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if a.client == nil {
		resp.Diagnostics.AddError(
			"Unconfigured pve_task_cancel",
			fmt.Sprintf("The provider client was not configured; cannot cancel task %s.", config.UPID.ValueString()),
		)
		return
	}
	node := config.Node.ValueString()
	upid := config.UPID.ValueString()
	tflog.Info(ctx, "cancelling task", map[string]any{"node": node, "upid": upid})
	if err := a.client.CancelTask(ctx, node, upid); err != nil {
		resp.Diagnostics.AddError(
			"Error invoking pve_task_cancel",
			fmt.Sprintf("stopping task %s on node %s: %s", upid, node, err),
		)
		return
	}
	if resp.SendProgress != nil {
		resp.SendProgress(action.InvokeProgressEvent{Message: fmt.Sprintf("Stop request delivered for task %s on node %s", upid, node)})
	}
}
