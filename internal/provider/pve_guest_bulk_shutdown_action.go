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

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ action.Action = &pveGuestBulkShutdownAction{}
)

// NewPveGuestBulkShutdownAction returns the action implementation.
func NewPveGuestBulkShutdownAction() action.Action {
	return &pveGuestBulkShutdownAction{}
}

// pveGuestBulkShutdownAction bulk shuts down all guests on the cluster
// via POST /cluster/bulk-action/guest/shutdown and waits for the worker
// task.
type pveGuestBulkShutdownAction struct {
	client *pveclient.Client
}

// pveGuestBulkShutdownActionModel is the Terraform-facing shape.
type pveGuestBulkShutdownActionModel struct {
	ForceStop  types.Bool  `tfsdk:"force_stop"`
	MaxWorkers types.Int64 `tfsdk:"max_workers"`
	Timeout    types.Int64 `tfsdk:"timeout"`
	Vms        types.Set   `tfsdk:"vms"`
}

// Metadata implements action.Action.
func (a *pveGuestBulkShutdownAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveGuestBulkShutdown
}

// Schema implements action.Action.
func (a *pveGuestBulkShutdownAction) Schema(_ context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "**Destructive:** bulk shuts down all guests on the cluster via " +
			"`POST /cluster/bulk-action/guest/shutdown` and waits for the worker task to finish. " +
			"With `force_stop` enabled, guests that do not shut down within `timeout` are hard-stopped.",
		Attributes: map[string]actionschema.Attribute{
			"force_stop": &actionschema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Makes sure the guest stops after the timeout. Defaults to true on the API.",
			},
			"max_workers": &actionschema.Int64Attribute{
				Optional:            true,
				Validators:          []validator.Int64{int64validator.Between(1, 64)},
				MarkdownDescription: "Maximum number of tasks running concurrently. Must be between 1 and 64. Defaults to 4 on the API.",
			},
			"timeout": &actionschema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Default shutdown timeout in seconds if none is configured for the guest. Defaults to 180 on the API.",
			},
			"vms": &actionschema.SetAttribute{
				Optional:            true,
				ElementType:         types.Int64Type,
				MarkdownDescription: "Only consider guests from this set of VMIDs, e.g. `[100, 101]`.",
			},
		},
	}
}

// Configure implements action.ActionWithConfigure.
func (a *pveGuestBulkShutdownAction) Configure(_ context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
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
func (a *pveGuestBulkShutdownAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var config pveGuestBulkShutdownActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if a.client == nil {
		resp.Diagnostics.AddError(
			"Unconfigured pve_guest_bulk_shutdown",
			"The provider client was not configured; cannot bulk shut down guests.",
		)
		return
	}
	opts := pveclient.BulkShutdownGuestsOptions{}
	if !config.ForceStop.IsNull() && !config.ForceStop.IsUnknown() {
		v := config.ForceStop.ValueBool()
		opts.ForceStop = &v
	}
	if !config.MaxWorkers.IsNull() && !config.MaxWorkers.IsUnknown() {
		v := config.MaxWorkers.ValueInt64()
		opts.MaxWorkers = &v
	}
	if !config.Timeout.IsNull() && !config.Timeout.IsUnknown() {
		v := config.Timeout.ValueInt64()
		opts.Timeout = &v
	}
	vms, diags := guestBulkVmsFromTF(ctx, config.Vms)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	opts.Vms = vms
	progress := func(message string) {
		if resp.SendProgress != nil {
			resp.SendProgress(action.InvokeProgressEvent{Message: message})
		}
	}
	tflog.Info(ctx, "bulk shutting down guests", map[string]any{"vms": len(vms)})
	progress("Bulk shutting down guests on the cluster")
	upid, err := a.client.BulkShutdownGuests(ctx, opts)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error invoking pve_guest_bulk_shutdown",
			fmt.Sprintf("bulk shutting down guests: %s", err),
		)
		return
	}
	taskNode, err := realmNodeFromUpid(upid)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error invoking pve_guest_bulk_shutdown",
			fmt.Sprintf("determining the task node for the bulk shutdown worker: %s", err),
		)
		return
	}
	progress(fmt.Sprintf("Bulk shutdown worker %s running on node %s; waiting for it to finish", upid, taskNode))
	if _, err := a.client.WaitForTask(ctx, taskNode, upid, pveclient.WaitForTaskOptions{}); err != nil {
		resp.Diagnostics.AddError(
			"Error invoking pve_guest_bulk_shutdown",
			fmt.Sprintf("waiting for the bulk shutdown worker: %s", err),
		)
		return
	}
	progress("Bulk shutdown finished")
}
