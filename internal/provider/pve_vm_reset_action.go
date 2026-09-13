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

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ action.Action              = &pveVmResetAction{}
	_ action.ActionWithConfigure = &pveVmResetAction{}
)

// NewPveVmResetAction returns the action implementation.
func NewPveVmResetAction() action.Action {
	return &pveVmResetAction{}
}

// pveVmResetAction hard-resets a QEMU VM
// (POST /nodes/{node}/qemu/{vmid}/status/reset) and waits for the task.
type pveVmResetAction struct {
	client *pveclient.Client
}

// pveVmResetActionModel is the Terraform-facing shape.
type pveVmResetActionModel struct {
	Node types.String `tfsdk:"node"`
	VMID types.Int64  `tfsdk:"vmid"`
}

// Metadata implements action.Action.
func (a *pveVmResetAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveVmReset
}

// Schema implements action.Action.
func (a *pveVmResetAction) Schema(_ context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Hard-resets a QEMU VM, like pressing the reset button on a physical machine; the guest OS is not given a chance to shut down cleanly (`POST /nodes/{node}/qemu/{vmid}/status/reset`). The action waits for the reset task to finish.",
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
		},
	}
}

// Configure implements action.ActionWithConfigure.
func (a *pveVmResetAction) Configure(_ context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
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
func (a *pveVmResetAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var config pveVmResetActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if a.client == nil {
		resp.Diagnostics.AddError(
			"Unconfigured pve_vm_reset",
			fmt.Sprintf("The provider client was not configured; cannot reset VM %d.", config.VMID.ValueInt64()),
		)
		return
	}
	progress := func(message string) {
		if resp.SendProgress != nil {
			resp.SendProgress(action.InvokeProgressEvent{Message: message})
		}
	}
	tflog.Info(ctx, "resetting VM", map[string]any{"node": config.Node.ValueString(), "vmid": config.VMID.ValueInt64()})
	upid, err := a.client.QemuVMReset(ctx, config.Node.ValueString(), config.VMID.ValueInt64())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error invoking pve_vm_reset",
			fmt.Sprintf("resetting VM %d on node %s: %s", config.VMID.ValueInt64(), config.Node.ValueString(), err),
		)
		return
	}
	if err := vmWaitForTask(ctx, a.client, upid, progress); err != nil {
		resp.Diagnostics.AddError(
			"Error invoking pve_vm_reset",
			fmt.Sprintf("waiting for reset of VM %d on node %s: %s", config.VMID.ValueInt64(), config.Node.ValueString(), err),
		)
	}
}
