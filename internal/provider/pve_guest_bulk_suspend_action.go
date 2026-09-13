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
	_ action.Action = &pveGuestBulkSuspendAction{}
)

// NewPveGuestBulkSuspendAction returns the action implementation.
func NewPveGuestBulkSuspendAction() action.Action {
	return &pveGuestBulkSuspendAction{}
}

// pveGuestBulkSuspendAction bulk suspends all guests on the cluster via
// POST /cluster/bulk-action/guest/suspend and waits for the worker task.
type pveGuestBulkSuspendAction struct {
	client *pveclient.Client
}

// pveGuestBulkSuspendActionModel is the Terraform-facing shape.
type pveGuestBulkSuspendActionModel struct {
	MaxWorkers   types.Int64  `tfsdk:"max_workers"`
	StateStorage types.String `tfsdk:"state_storage"`
	ToDisk       types.Bool   `tfsdk:"to_disk"`
	Vms          types.Set    `tfsdk:"vms"`
}

// Metadata implements action.Action.
func (a *pveGuestBulkSuspendAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveGuestBulkSuspend
}

// Schema implements action.Action.
func (a *pveGuestBulkSuspendAction) Schema(_ context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "**Disruptive:** bulk suspends all guests on the cluster via " +
			"`POST /cluster/bulk-action/guest/suspend` and waits for the worker task to finish. " +
			"With `to_disk` the guests are suspended to disk and are resumed on their next start.",
		Attributes: map[string]actionschema.Attribute{
			"max_workers": &actionschema.Int64Attribute{
				Optional:            true,
				Validators:          []validator.Int64{int64validator.Between(1, 64)},
				MarkdownDescription: "Maximum number of tasks running concurrently. Must be between 1 and 64. Defaults to 4 on the API.",
			},
			"state_storage": &actionschema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The storage for the VM state; requires `to_disk`.",
			},
			"to_disk": &actionschema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "If set, suspends the guests to disk. They are resumed on the next start. Defaults to false on the API.",
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
func (a *pveGuestBulkSuspendAction) Configure(_ context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
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
func (a *pveGuestBulkSuspendAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var config pveGuestBulkSuspendActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if a.client == nil {
		resp.Diagnostics.AddError(
			"Unconfigured pve_guest_bulk_suspend",
			"The provider client was not configured; cannot bulk suspend guests.",
		)
		return
	}
	opts := pveclient.BulkSuspendGuestsOptions{}
	if !config.MaxWorkers.IsNull() && !config.MaxWorkers.IsUnknown() {
		v := config.MaxWorkers.ValueInt64()
		opts.MaxWorkers = &v
	}
	if !config.StateStorage.IsNull() && !config.StateStorage.IsUnknown() {
		opts.StateStorage = config.StateStorage.ValueString()
	}
	if !config.ToDisk.IsNull() && !config.ToDisk.IsUnknown() {
		v := config.ToDisk.ValueBool()
		opts.ToDisk = &v
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
	tflog.Info(ctx, "bulk suspending guests", map[string]any{"vms": len(vms)})
	progress("Bulk suspending guests on the cluster")
	upid, err := a.client.BulkSuspendGuests(ctx, opts)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error invoking pve_guest_bulk_suspend",
			fmt.Sprintf("bulk suspending guests: %s", err),
		)
		return
	}
	taskNode, err := realmNodeFromUpid(upid)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error invoking pve_guest_bulk_suspend",
			fmt.Sprintf("determining the task node for the bulk suspend worker: %s", err),
		)
		return
	}
	progress(fmt.Sprintf("Bulk suspend worker %s running on node %s; waiting for it to finish", upid, taskNode))
	if _, err := a.client.WaitForTask(ctx, taskNode, upid, pveclient.WaitForTaskOptions{}); err != nil {
		resp.Diagnostics.AddError(
			"Error invoking pve_guest_bulk_suspend",
			fmt.Sprintf("waiting for the bulk suspend worker: %s", err),
		)
		return
	}
	progress("Bulk suspend finished")
}
