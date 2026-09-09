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
	_ action.Action = &pveContainerRebootAction{}
)

// NewPveContainerRebootAction returns the action implementation.
func NewPveContainerRebootAction() action.Action {
	return &pveContainerRebootAction{}
}

// pveContainerRebootAction reboots an LXC container via POST
// /nodes/{node}/lxc/{vmid}/status/reboot: the container is shut down and
// started again, applying pending configuration changes.
type pveContainerRebootAction struct {
	client *pveclient.Client
}

// pveContainerRebootActionModel is the Terraform-facing shape.
type pveContainerRebootActionModel struct {
	Node    types.String `tfsdk:"node"`
	VMID    types.Int64  `tfsdk:"vmid"`
	Timeout types.Int64  `tfsdk:"timeout"`
}

// Metadata implements action.Action.
func (a *pveContainerRebootAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveContainerReboot
}

// Schema implements action.Action.
func (a *pveContainerRebootAction) Schema(_ context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "Reboots an LXC container by shutting it down and starting it again " +
			"(`POST /nodes/{node}/lxc/{vmid}/status/reboot`). Pending configuration changes are applied " +
			"by the reboot. Requires the `VM.PowerMgmt` privilege on `/vms/{vmid}`.",
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
			"timeout": &actionschema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Wait maximal timeout seconds for the shutdown before the reboot proceeds. Must be 0 or greater; omit for the PVE default (60).",
				Validators:          []validator.Int64{int64validator.AtLeast(0)},
			},
		},
	}
}

// Configure implements action.ActionWithConfigure.
func (a *pveContainerRebootAction) Configure(_ context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	a.client = containerActionConfigure(req, resp)
}

// Invoke implements action.Action.
func (a *pveContainerRebootAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var config pveContainerRebootActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if a.client == nil {
		resp.Diagnostics.AddError(
			"Unconfigured pve_container_reboot",
			fmt.Sprintf("The provider client was not configured; cannot reboot container %d on %s.", config.VMID.ValueInt64(), config.Node.ValueString()),
		)
		return
	}
	node := config.Node.ValueString()
	vmid := config.VMID.ValueInt64()
	params := &pveclient.LxcRebootParams{}
	if !config.Timeout.IsNull() && !config.Timeout.IsUnknown() {
		timeout := config.Timeout.ValueInt64()
		params.TimeoutSeconds = &timeout
	}
	progress := func(message string) {
		if resp.SendProgress != nil {
			resp.SendProgress(action.InvokeProgressEvent{Message: message})
		}
	}
	tflog.Info(ctx, "rebooting container", map[string]any{"node": node, "vmid": vmid})
	progress(fmt.Sprintf("Sending reboot command to container %d on %s", vmid, node))
	upid, err := a.client.LxcReboot(ctx, node, vmid, params)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error invoking pve_container_reboot",
			fmt.Sprintf("rebooting container %d on %s: %s", vmid, node, err),
		)
		return
	}
	progress(fmt.Sprintf("Waiting for reboot task on %s to finish", node))
	if _, err := a.client.WaitForTask(ctx, node, upid, defaultWaitOptions()); err != nil {
		resp.Diagnostics.AddError(
			"Error waiting for pve_container_reboot",
			fmt.Sprintf("waiting for reboot of container %d on %s: %s", vmid, node, err),
		)
		return
	}
	tflog.Info(ctx, "container rebooted", map[string]any{"node": node, "vmid": vmid})
	progress(fmt.Sprintf("Container %d rebooted", vmid))
}

// containerActionVMIDValidator bounds the guest vmid to the pin's
// `pve-vmid` format (100 - 999999999).
func containerActionVMIDValidator() validator.Int64 {
	return int64validator.Between(100, 999999999)
}

// containerActionConfigure extracts the provider client for the
// pve_container_* actions, tolerating the unconfigured pre-apply phase.
func containerActionConfigure(req action.ConfigureRequest, resp *action.ConfigureResponse) *pveclient.Client {
	if req.ProviderData == nil {
		return nil
	}
	client, ok := req.ProviderData.(*pveclient.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Action Configure Type",
			fmt.Sprintf("Expected *pveclient.Client, got: %T.", req.ProviderData),
		)
		return nil
	}
	return client
}
