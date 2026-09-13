// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/action"
	actionschema "github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ action.Action = &pveContainerSnapshotRollbackAction{}
)

// NewPveContainerSnapshotRollbackAction returns the action implementation.
func NewPveContainerSnapshotRollbackAction() action.Action {
	return &pveContainerSnapshotRollbackAction{}
}

// pveContainerSnapshotRollbackAction rolls an LXC container back to a
// snapshot via POST /nodes/{node}/lxc/{vmid}/snapshot/{snapname}/rollback.
type pveContainerSnapshotRollbackAction struct {
	client *pveclient.Client
}

// pveContainerSnapshotRollbackActionModel is the Terraform-facing shape.
type pveContainerSnapshotRollbackActionModel struct {
	Node  types.String `tfsdk:"node"`
	VMID  types.Int64  `tfsdk:"vmid"`
	Name  types.String `tfsdk:"name"`
	Start types.Bool   `tfsdk:"start"`
}

// Metadata implements action.Action.
func (a *pveContainerSnapshotRollbackAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveContainerSnapshotRollback
}

// Schema implements action.Action.
func (a *pveContainerSnapshotRollbackAction) Schema(_ context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "Rolls an LXC container back to a snapshot (`POST /nodes/{node}/lxc/{vmid}/snapshot/{snapname}/rollback`). " +
			"**Destructive:** every change to the container's volumes and configuration made after the snapshot is " +
			"permanently discarded. Requires the `VM.Snapshot` or `VM.Snapshot.Rollback` privilege on `/vms/{vmid}`.",
		Attributes: map[string]actionschema.Attribute{
			"node": &actionschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The cluster node the container runs on.",
			},
			"vmid": &actionschema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "The (unique) ID of the container. Must be between 100 and 999999999.",
				Validators:          []validator.Int64{containerActionVMIDValidator()},
			},
			"name": &actionschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The name of the snapshot to roll back to (upstream `pve-configid`: at most 40 characters). The reserved name `current` cannot be rolled back to.",
				Validators:          []validator.String{containerSnapshotNameValidator()},
			},
			"start": &actionschema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Start the container after the rollback succeeds. PVE defaults this to false, which leaves the container stopped.",
			},
		},
	}
}

// Configure implements action.ActionWithConfigure.
func (a *pveContainerSnapshotRollbackAction) Configure(_ context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	a.client = containerActionConfigure(req, resp)
}

// Invoke implements action.Action.
func (a *pveContainerSnapshotRollbackAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var config pveContainerSnapshotRollbackActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if a.client == nil {
		resp.Diagnostics.AddError(
			"Unconfigured pve_container_snapshot_rollback",
			fmt.Sprintf("The provider client was not configured; cannot roll back container %d on %s.", config.VMID.ValueInt64(), config.Node.ValueString()),
		)
		return
	}
	node := config.Node.ValueString()
	vmid := config.VMID.ValueInt64()
	name := config.Name.ValueString()
	start := false
	if !config.Start.IsNull() && !config.Start.IsUnknown() {
		start = config.Start.ValueBool()
	}
	progress := func(message string) {
		if resp.SendProgress != nil {
			resp.SendProgress(action.InvokeProgressEvent{Message: message})
		}
	}
	tflog.Info(ctx, "rolling back container snapshot", map[string]any{"node": node, "vmid": vmid, "snapname": name})
	progress(fmt.Sprintf("Rolling back container %d on %s to snapshot %s", vmid, node, name))
	upid, err := a.client.RollbackLxcSnapshot(ctx, node, vmid, name, start)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error invoking pve_container_snapshot_rollback",
			fmt.Sprintf("rolling back container %d on %s to snapshot %s: %s", vmid, node, name, err),
		)
		return
	}
	progress(fmt.Sprintf("Waiting for rollback task on %s to finish", node))
	if _, err := a.client.WaitForTask(ctx, node, upid, defaultWaitOptions()); err != nil {
		resp.Diagnostics.AddError(
			"Error waiting for pve_container_snapshot_rollback",
			fmt.Sprintf("waiting for rollback of container %d on %s to snapshot %s: %s", vmid, node, name, err),
		)
		return
	}
	tflog.Info(ctx, "container snapshot rolled back", map[string]any{"node": node, "vmid": vmid, "snapname": name})
	progress(fmt.Sprintf("Container %d rolled back to snapshot %s", vmid, name))
}
