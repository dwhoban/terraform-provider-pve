// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework/action"
	actionschema "github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ action.Action = &pveGuestBulkStartAction{}
)

// NewPveGuestBulkStartAction returns the action implementation.
func NewPveGuestBulkStartAction() action.Action {
	return &pveGuestBulkStartAction{}
}

// pveGuestBulkStartAction bulk starts or resumes all guests on the
// cluster via POST /cluster/bulk-action/guest/start and waits for the
// worker task.
type pveGuestBulkStartAction struct {
	client *pveclient.Client
}

// pveGuestBulkStartActionModel is the Terraform-facing shape.
type pveGuestBulkStartActionModel struct {
	MaxWorkers types.Int64 `tfsdk:"max_workers"`
	Timeout    types.Int64 `tfsdk:"timeout"`
	Vms        types.Set   `tfsdk:"vms"`
}

// Metadata implements action.Action.
func (a *pveGuestBulkStartAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveGuestBulkStart
}

// Schema implements action.Action.
func (a *pveGuestBulkStartAction) Schema(_ context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "Bulk starts or resumes all guests on the cluster via " +
			"`POST /cluster/bulk-action/guest/start` and waits for the worker task to finish.",
		Attributes: map[string]actionschema.Attribute{
			"max_workers": &actionschema.Int64Attribute{
				Optional:            true,
				Validators:          []validator.Int64{int64validator.Between(1, 64)},
				MarkdownDescription: "Maximum number of tasks running concurrently. Must be between 1 and 64. Defaults to 4 on the API.",
			},
			"timeout": &actionschema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Default start timeout in seconds. Only valid for VMs; the default depends on the guest configuration.",
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
func (a *pveGuestBulkStartAction) Configure(_ context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
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
func (a *pveGuestBulkStartAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var config pveGuestBulkStartActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if a.client == nil {
		resp.Diagnostics.AddError(
			"Unconfigured pve_guest_bulk_start",
			"The provider client was not configured; cannot bulk start guests.",
		)
		return
	}
	opts := pveclient.BulkStartGuestsOptions{}
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
	tflog.Info(ctx, "bulk starting guests", map[string]any{"vms": len(vms)})
	progress("Bulk starting guests on the cluster")
	upid, err := a.client.BulkStartGuests(ctx, opts)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error invoking pve_guest_bulk_start",
			fmt.Sprintf("bulk starting guests: %s", err),
		)
		return
	}
	taskNode, err := realmNodeFromUpid(upid)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error invoking pve_guest_bulk_start",
			fmt.Sprintf("determining the task node for the bulk start worker: %s", err),
		)
		return
	}
	progress(fmt.Sprintf("Bulk start worker %s running on node %s; waiting for it to finish", upid, taskNode))
	if _, err := a.client.WaitForTask(ctx, taskNode, upid, pveclient.WaitForTaskOptions{}); err != nil {
		resp.Diagnostics.AddError(
			"Error invoking pve_guest_bulk_start",
			fmt.Sprintf("waiting for the bulk start worker: %s", err),
		)
		return
	}
	progress("Bulk start finished")
}

// guestBulkVmsFromTF converts the optional vms set into the client's
// VMID slice; a null or unknown set yields no filter. Shared by the
// guest bulk actions.
func guestBulkVmsFromTF(ctx context.Context, set types.Set) ([]int64, diag.Diagnostics) {
	var diags diag.Diagnostics
	if set.IsNull() || set.IsUnknown() {
		return nil, diags
	}
	vms := make([]int64, 0, len(set.Elements()))
	diags.Append(set.ElementsAs(ctx, &vms, false)...)
	return vms, diags
}
