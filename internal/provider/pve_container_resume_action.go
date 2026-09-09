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

	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ action.Action = &pveContainerResumeAction{}
)

// NewPveContainerResumeAction returns the action implementation.
func NewPveContainerResumeAction() action.Action {
	return &pveContainerResumeAction{}
}

// pveContainerResumeAction resumes a suspended LXC container via POST
// /nodes/{node}/lxc/{vmid}/status/resume.
type pveContainerResumeAction struct {
	client *pveclient.Client
}

// pveContainerResumeActionModel is the Terraform-facing shape.
type pveContainerResumeActionModel struct {
	Node types.String `tfsdk:"node"`
	VMID types.Int64  `tfsdk:"vmid"`
}

// Metadata implements action.Action.
func (a *pveContainerResumeAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveContainerResume
}

// Schema implements action.Action.
func (a *pveContainerResumeAction) Schema(_ context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "Resumes a suspended LXC container (`POST /nodes/{node}/lxc/{vmid}/status/resume`). " +
			"Requires the `VM.PowerMgmt` privilege on `/vms/{vmid}`.",
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
		},
	}
}

// Configure implements action.ActionWithConfigure.
func (a *pveContainerResumeAction) Configure(_ context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	a.client = containerActionConfigure(req, resp)
}

// Invoke implements action.Action.
func (a *pveContainerResumeAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var config pveContainerResumeActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if a.client == nil {
		resp.Diagnostics.AddError(
			"Unconfigured pve_container_resume",
			fmt.Sprintf("The provider client was not configured; cannot resume container %d on %s.", config.VMID.ValueInt64(), config.Node.ValueString()),
		)
		return
	}
	node := config.Node.ValueString()
	vmid := config.VMID.ValueInt64()
	progress := func(message string) {
		if resp.SendProgress != nil {
			resp.SendProgress(action.InvokeProgressEvent{Message: message})
		}
	}
	tflog.Info(ctx, "resuming container", map[string]any{"node": node, "vmid": vmid})
	progress(fmt.Sprintf("Sending resume command to container %d on %s", vmid, node))
	upid, err := a.client.LxcResume(ctx, node, vmid)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error invoking pve_container_resume",
			fmt.Sprintf("resuming container %d on %s: %s", vmid, node, err),
		)
		return
	}
	progress(fmt.Sprintf("Waiting for resume task on %s to finish", node))
	if _, err := a.client.WaitForTask(ctx, node, upid, defaultWaitOptions()); err != nil {
		resp.Diagnostics.AddError(
			"Error waiting for pve_container_resume",
			fmt.Sprintf("waiting for resume of container %d on %s: %s", vmid, node, err),
		)
		return
	}
	tflog.Info(ctx, "container resumed", map[string]any{"node": node, "vmid": vmid})
	progress(fmt.Sprintf("Container %d resumed", vmid))
}
