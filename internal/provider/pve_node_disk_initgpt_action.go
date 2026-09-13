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
	_ action.Action              = &pveNodeDiskInitgptAction{}
	_ action.ActionWithConfigure = &pveNodeDiskInitgptAction{}
)

// NewPveNodeDiskInitgptAction returns the action implementation.
func NewPveNodeDiskInitgptAction() action.Action {
	return &pveNodeDiskInitgptAction{}
}

// pveNodeDiskInitgptAction initializes a disk with a GPT table
// (POST /nodes/{node}/disks/initgpt) and waits for the task to finish.
type pveNodeDiskInitgptAction struct {
	client *pveclient.Client
}

// pveNodeDiskInitgptActionModel is the Terraform-facing shape.
type pveNodeDiskInitgptActionModel struct {
	Node types.String `tfsdk:"node"`
	Disk types.String `tfsdk:"disk"`
	UUID types.String `tfsdk:"uuid"`
}

// Metadata implements action.Action.
func (a *pveNodeDiskInitgptAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveNodeDiskInitgpt
}

// Schema implements action.Action.
func (a *pveNodeDiskInitgptAction) Schema(_ context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "Initializes a disk with a GPT table (`POST /nodes/{node}/disks/initgpt`) and waits for " +
			"the task to finish. Upstream destroys any existing partition table on the device. Requires the `Sys.Modify` " +
			"privilege on `/`.",
		Attributes: map[string]actionschema.Attribute{
			"node": &actionschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The cluster node holding the disk; the task runs here.",
			},
			"disk": &actionschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Absolute block device path to initialize, e.g. `/dev/sdb` (upstream pattern `/dev/...`).",
			},
			"uuid": &actionschema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "UUID for the new GPT table; omit to let PVE generate one.",
			},
		},
	}
}

// Configure implements action.ActionWithConfigure.
func (a *pveNodeDiskInitgptAction) Configure(ctx context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	a.client = containerActionConfigure(req, resp)
}

// Invoke implements action.Action.
func (a *pveNodeDiskInitgptAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var config pveNodeDiskInitgptActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if a.client == nil {
		resp.Diagnostics.AddError(
			"Unconfigured pve_node_disk_initgpt",
			fmt.Sprintf("The provider client was not configured; cannot initialize disk %s.", config.Disk.ValueString()),
		)
		return
	}
	node := config.Node.ValueString()
	disk := config.Disk.ValueString()
	uuid := ""
	if !config.UUID.IsNull() && !config.UUID.IsUnknown() {
		uuid = config.UUID.ValueString()
	}
	progress := func(message string) {
		if resp.SendProgress != nil {
			resp.SendProgress(action.InvokeProgressEvent{Message: message})
		}
	}
	tflog.Info(ctx, "initializing disk with GPT", map[string]any{"node": node, "disk": disk})
	upid, err := a.client.InitializeDiskGPT(ctx, node, disk, uuid)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error invoking pve_node_disk_initgpt",
			fmt.Sprintf("initializing disk %s on node %s: %s", disk, node, err),
		)
		return
	}
	if err := vmWaitForTask(ctx, a.client, upid, progress); err != nil {
		resp.Diagnostics.AddError(
			"Error waiting for pve_node_disk_initgpt",
			fmt.Sprintf("waiting for GPT initialization of disk %s on node %s: %s", disk, node, err),
		)
		return
	}
	progress(fmt.Sprintf("Disk %s on node %s initialized with GPT", disk, node))
}
