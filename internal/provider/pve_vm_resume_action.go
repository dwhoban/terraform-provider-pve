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
	_ action.Action              = &pveVmResumeAction{}
	_ action.ActionWithConfigure = &pveVmResumeAction{}
)

// NewPveVmResumeAction returns the action implementation.
func NewPveVmResumeAction() action.Action {
	return &pveVmResumeAction{}
}

// pveVmResumeAction resumes a suspended QEMU VM
// (POST /nodes/{node}/qemu/{vmid}/status/resume) and waits for the task.
type pveVmResumeAction struct {
	client *pveclient.Client
}

// pveVmResumeActionModel is the Terraform-facing shape.
type pveVmResumeActionModel struct {
	Node types.String `tfsdk:"node"`
	VMID types.Int64  `tfsdk:"vmid"`
}

// Metadata implements action.Action.
func (a *pveVmResumeAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveVmResume
}

// Schema implements action.Action.
func (a *pveVmResumeAction) Schema(_ context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Resumes a suspended QEMU VM (`POST /nodes/{node}/qemu/{vmid}/status/resume`). The action waits for the resume task to finish.",
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
func (a *pveVmResumeAction) Configure(_ context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
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
func (a *pveVmResumeAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var config pveVmResumeActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if a.client == nil {
		resp.Diagnostics.AddError(
			"Unconfigured pve_vm_resume",
			fmt.Sprintf("The provider client was not configured; cannot resume VM %d.", config.VMID.ValueInt64()),
		)
		return
	}
	progress := func(message string) {
		if resp.SendProgress != nil {
			resp.SendProgress(action.InvokeProgressEvent{Message: message})
		}
	}
	tflog.Info(ctx, "resuming VM", map[string]any{"node": config.Node.ValueString(), "vmid": config.VMID.ValueInt64()})
	upid, err := a.client.QemuVMResume(ctx, config.Node.ValueString(), config.VMID.ValueInt64())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error invoking pve_vm_resume",
			fmt.Sprintf("resuming VM %d on node %s: %s", config.VMID.ValueInt64(), config.Node.ValueString(), err),
		)
		return
	}
	if err := vmWaitForTask(ctx, a.client, upid, progress); err != nil {
		resp.Diagnostics.AddError(
			"Error invoking pve_vm_resume",
			fmt.Sprintf("waiting for resume of VM %d on node %s: %s", config.VMID.ValueInt64(), config.Node.ValueString(), err),
		)
	}
}
