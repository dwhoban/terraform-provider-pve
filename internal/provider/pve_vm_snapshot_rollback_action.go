// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ action.Action              = &pveVmSnapshotRollbackAction{}
	_ action.ActionWithConfigure = &pveVmSnapshotRollbackAction{}
)

// NewPveVmSnapshotRollbackAction returns the action implementation.
func NewPveVmSnapshotRollbackAction() action.Action {
	return &pveVmSnapshotRollbackAction{}
}

// pveVmSnapshotRollbackAction rolls a QEMU VM back to a snapshot
// (POST /nodes/{node}/qemu/{vmid}/snapshot/{snapname}/rollback) and waits
// for the task. Rollback is destructive: the VM state reverts to the
// snapshot.
type pveVmSnapshotRollbackAction struct {
	client *pveclient.Client
}

// pveVmSnapshotRollbackActionModel is the Terraform-facing shape.
type pveVmSnapshotRollbackActionModel struct {
	Node types.String `tfsdk:"node"`
	VMID types.Int64  `tfsdk:"vmid"`
	Name types.String `tfsdk:"name"`
}

// Metadata implements action.Action.
func (a *pveVmSnapshotRollbackAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveVmSnapshotRollback
}

// Schema implements action.Action.
func (a *pveVmSnapshotRollbackAction) Schema(_ context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Destructive: rolls a QEMU VM back to a snapshot (`POST /nodes/{node}/qemu/{vmid}/snapshot/{snapname}/rollback`). The guest state, disks, and any data written after the snapshot was taken revert to the snapshot point. The action waits for the rollback task to finish.",
		Attributes: map[string]schema.Attribute{
			"node": &schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The cluster node running the VM.",
			},
			"vmid": &schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "The VM identifier. Must be between 100 and 999999999.",
				Validators:          vmActionVMIDValidators,
			},
			"name": &schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The snapshot identifier (upstream `snapname`) to roll back to.",
			},
		},
	}
}

// Configure implements action.ActionWithConfigure.
func (a *pveVmSnapshotRollbackAction) Configure(_ context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
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
func (a *pveVmSnapshotRollbackAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var config pveVmSnapshotRollbackActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if a.client == nil {
		resp.Diagnostics.AddError(
			"Unconfigured pve_vm_snapshot_rollback",
			fmt.Sprintf("The provider client was not configured; cannot roll back VM %d.", config.VMID.ValueInt64()),
		)
		return
	}
	progress := func(message string) {
		if resp.SendProgress != nil {
			resp.SendProgress(action.InvokeProgressEvent{Message: message})
		}
	}
	node := config.Node.ValueString()
	vmid := config.VMID.ValueInt64()
	name := config.Name.ValueString()
	tflog.Info(ctx, "rolling back VM snapshot", map[string]any{"node": node, "vmid": vmid, "name": name})
	upid, err := a.client.RollbackQemuVMSnapshot(ctx, node, vmid, name)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error invoking pve_vm_snapshot_rollback",
			fmt.Sprintf("rolling back VM %d on node %s to snapshot %s: %s", vmid, node, name, err),
		)
		return
	}
	if err := vmWaitForTask(ctx, a.client, upid, progress); err != nil {
		resp.Diagnostics.AddError(
			"Error invoking pve_vm_snapshot_rollback",
			fmt.Sprintf("waiting for rollback of VM %d on node %s to snapshot %s: %s", vmid, node, name, err),
		)
	}
}
