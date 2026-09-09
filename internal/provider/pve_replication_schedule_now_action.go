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
	_ action.Action              = &pveReplicationScheduleNowAction{}
	_ action.ActionWithConfigure = &pveReplicationScheduleNowAction{}
)

// NewPveReplicationScheduleNowAction returns the action implementation.
func NewPveReplicationScheduleNowAction() action.Action {
	return &pveReplicationScheduleNowAction{}
}

// pveReplicationScheduleNowAction schedules a replication job to start as
// soon as possible (POST /nodes/{node}/replication/{id}/schedule_now) and
// waits for the replication task to finish.
type pveReplicationScheduleNowAction struct {
	client *pveclient.Client
}

// pveReplicationScheduleNowActionModel is the Terraform-facing shape.
type pveReplicationScheduleNowActionModel struct {
	Node types.String `tfsdk:"node"`
	ID   types.String `tfsdk:"id"`
}

// Metadata implements action.Action.
func (a *pveReplicationScheduleNowAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveReplicationScheduleNow
}

// Schema implements action.Action.
func (a *pveReplicationScheduleNowAction) Schema(_ context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "Schedules a replication job to start as soon as possible " +
			"(`POST /nodes/{node}/replication/{id}/schedule_now`) and waits for the replication task to finish. " +
			"Requires the `VM.Replicate` privilege on `/vms/{vmid}`.",
		Attributes: map[string]actionschema.Attribute{
			"node": &actionschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The cluster node the replication job is scheduled on; the task runs here.",
			},
			"id": &actionschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Replication job ID, `<GUEST>-<JOBNUM>` (e.g. `100-0`).",
			},
		},
	}
}

// Configure implements action.ActionWithConfigure.
func (a *pveReplicationScheduleNowAction) Configure(ctx context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	a.client = containerActionConfigure(req, resp)
}

// Invoke implements action.Action.
func (a *pveReplicationScheduleNowAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var config pveReplicationScheduleNowActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if a.client == nil {
		resp.Diagnostics.AddError(
			"Unconfigured pve_replication_schedule_now",
			fmt.Sprintf("The provider client was not configured; cannot schedule replication job %s.", config.ID.ValueString()),
		)
		return
	}
	node := config.Node.ValueString()
	id := config.ID.ValueString()
	progress := func(message string) {
		if resp.SendProgress != nil {
			resp.SendProgress(action.InvokeProgressEvent{Message: message})
		}
	}
	tflog.Info(ctx, "scheduling replication job", map[string]any{"node": node, "id": id})
	upid, err := a.client.ScheduleReplicationNow(ctx, node, id)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error invoking pve_replication_schedule_now",
			fmt.Sprintf("scheduling replication job %s on node %s: %s", id, node, err),
		)
		return
	}
	progress(fmt.Sprintf("Replication job %s scheduled on node %s; waiting for the replication task to finish", id, node))
	if err := vmWaitForTask(ctx, a.client, upid, progress); err != nil {
		resp.Diagnostics.AddError(
			"Error waiting for pve_replication_schedule_now",
			fmt.Sprintf("waiting for replication job %s on node %s: %s", id, node, err),
		)
		return
	}
	progress(fmt.Sprintf("Replication job %s finished", id))
}
