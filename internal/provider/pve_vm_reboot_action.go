// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	int64validator "github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// vmActionVMIDValidators is shared by every VM action: the pin constrains
// VMIDs to 100 - 999999999.
var vmActionVMIDValidators = []validator.Int64{
	int64validator.Between(100, 999999999),
}

// vmWaitForTask waits for a guest task UPID to finish, reporting progress.
func vmWaitForTask(ctx context.Context, client *pveclient.Client, upid string, progress func(string)) error {
	node, err := realmNodeFromUpid(upid)
	if err != nil {
		return err
	}
	progress(fmt.Sprintf("Waiting for task on node %s to finish", node))
	if _, err := client.WaitForTask(ctx, node, upid, pveclient.WaitForTaskOptions{}); err != nil {
		return err
	}
	progress("Task finished")
	return nil
}

// Ensure framework interfaces are satisfied.
var (
	_ action.Action              = &pveVmRebootAction{}
	_ action.ActionWithConfigure = &pveVmRebootAction{}
)

// NewPveVmRebootAction returns the action implementation.
func NewPveVmRebootAction() action.Action {
	return &pveVmRebootAction{}
}

// pveVmRebootAction reboots a QEMU VM
// (POST /nodes/{node}/qemu/{vmid}/status/reboot) and waits for the task.
type pveVmRebootAction struct {
	client *pveclient.Client
}

// pveVmRebootActionModel is the Terraform-facing shape.
type pveVmRebootActionModel struct {
	Node    types.String `tfsdk:"node"`
	VMID    types.Int64  `tfsdk:"vmid"`
	Timeout types.Int64  `tfsdk:"timeout"`
}

// Metadata implements action.Action.
func (a *pveVmRebootAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveVmReboot
}

// Schema implements action.Action.
func (a *pveVmRebootAction) Schema(_ context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reboots a QEMU VM by shutting it down and starting it again, applying pending changes (`POST /nodes/{node}/qemu/{vmid}/status/reboot`). The action waits for the reboot task to finish.",
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
			"timeout": &schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Maximum number of seconds to wait for the shutdown half of the reboot.",
			},
		},
	}
}

// Configure implements action.ActionWithConfigure.
func (a *pveVmRebootAction) Configure(_ context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
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
func (a *pveVmRebootAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var config pveVmRebootActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if a.client == nil {
		resp.Diagnostics.AddError(
			"Unconfigured pve_vm_reboot",
			fmt.Sprintf("The provider client was not configured; cannot reboot VM %d.", config.VMID.ValueInt64()),
		)
		return
	}
	progress := func(message string) {
		if resp.SendProgress != nil {
			resp.SendProgress(action.InvokeProgressEvent{Message: message})
		}
	}
	opts := pveclient.QemuVMRebootOptions{}
	if !config.Timeout.IsNull() && !config.Timeout.IsUnknown() {
		v := config.Timeout.ValueInt64()
		opts.Timeout = &v
	}
	tflog.Info(ctx, "rebooting VM", map[string]any{"node": config.Node.ValueString(), "vmid": config.VMID.ValueInt64()})
	upid, err := a.client.QemuVMReboot(ctx, config.Node.ValueString(), config.VMID.ValueInt64(), opts)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error invoking pve_vm_reboot",
			fmt.Sprintf("rebooting VM %d on node %s: %s", config.VMID.ValueInt64(), config.Node.ValueString(), err),
		)
		return
	}
	if err := vmWaitForTask(ctx, a.client, upid, progress); err != nil {
		resp.Diagnostics.AddError(
			"Error invoking pve_vm_reboot",
			fmt.Sprintf("waiting for reboot of VM %d on node %s: %s", config.VMID.ValueInt64(), config.Node.ValueString(), err),
		)
	}
}
