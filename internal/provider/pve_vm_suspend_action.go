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
	_ action.Action              = &pveVmSuspendAction{}
	_ action.ActionWithConfigure = &pveVmSuspendAction{}
)

// NewPveVmSuspendAction returns the action implementation.
func NewPveVmSuspendAction() action.Action {
	return &pveVmSuspendAction{}
}

// pveVmSuspendAction suspends a QEMU VM
// (POST /nodes/{node}/qemu/{vmid}/status/suspend) and waits for the task.
type pveVmSuspendAction struct {
	client *pveclient.Client
}

// pveVmSuspendActionModel is the Terraform-facing shape.
type pveVmSuspendActionModel struct {
	Node         types.String `tfsdk:"node"`
	VMID         types.Int64  `tfsdk:"vmid"`
	Todisk       types.Bool   `tfsdk:"todisk"`
	StateStorage types.String `tfsdk:"statestorage"`
}

// Metadata implements action.Action.
func (a *pveVmSuspendAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveVmSuspend
}

// Schema implements action.Action.
func (a *pveVmSuspendAction) Schema(_ context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Suspends a QEMU VM (`POST /nodes/{node}/qemu/{vmid}/status/suspend`). The action waits for the suspend task to finish. With `todisk` the VM is suspended to disk and resumes on the next start.",
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
			"todisk": &schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "If set, suspends the VM to disk; it will be resumed on the next VM start.",
			},
			"statestorage": &schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The storage holding the VM state.",
			},
		},
	}
}

// Configure implements action.ActionWithConfigure.
func (a *pveVmSuspendAction) Configure(_ context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
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
func (a *pveVmSuspendAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var config pveVmSuspendActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if a.client == nil {
		resp.Diagnostics.AddError(
			"Unconfigured pve_vm_suspend",
			fmt.Sprintf("The provider client was not configured; cannot suspend VM %d.", config.VMID.ValueInt64()),
		)
		return
	}
	progress := func(message string) {
		if resp.SendProgress != nil {
			resp.SendProgress(action.InvokeProgressEvent{Message: message})
		}
	}
	opts := pveclient.QemuVMSuspendOptions{}
	if !config.Todisk.IsNull() && !config.Todisk.IsUnknown() {
		v := config.Todisk.ValueBool()
		opts.Todisk = &v
	}
	if !config.StateStorage.IsNull() && !config.StateStorage.IsUnknown() {
		opts.StateStorage = config.StateStorage.ValueString()
	}
	tflog.Info(ctx, "suspending VM", map[string]any{"node": config.Node.ValueString(), "vmid": config.VMID.ValueInt64()})
	upid, err := a.client.QemuVMSuspend(ctx, config.Node.ValueString(), config.VMID.ValueInt64(), opts)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error invoking pve_vm_suspend",
			fmt.Sprintf("suspending VM %d on node %s: %s", config.VMID.ValueInt64(), config.Node.ValueString(), err),
		)
		return
	}
	if err := vmWaitForTask(ctx, a.client, upid, progress); err != nil {
		resp.Diagnostics.AddError(
			"Error invoking pve_vm_suspend",
			fmt.Sprintf("waiting for suspend of VM %d on node %s: %s", config.VMID.ValueInt64(), config.Node.ValueString(), err),
		)
	}
}
