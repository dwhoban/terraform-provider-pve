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
	_ action.Action = &pveGuestBulkMigrateAction{}
)

// NewPveGuestBulkMigrateAction returns the action implementation.
func NewPveGuestBulkMigrateAction() action.Action {
	return &pveGuestBulkMigrateAction{}
}

// pveGuestBulkMigrateAction bulk migrates all guests on the cluster via
// POST /cluster/bulk-action/guest/migrate and waits for the worker task.
type pveGuestBulkMigrateAction struct {
	client *pveclient.Client
}

// pveGuestBulkMigrateActionModel is the Terraform-facing shape.
type pveGuestBulkMigrateActionModel struct {
	Target         types.String `tfsdk:"target"`
	MaxWorkers     types.Int64  `tfsdk:"max_workers"`
	Online         types.Bool   `tfsdk:"online"`
	Vms            types.Set    `tfsdk:"vms"`
	WithLocalDisks types.Bool   `tfsdk:"with_local_disks"`
}

// Metadata implements action.Action.
func (a *pveGuestBulkMigrateAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveGuestBulkMigrate
}

// Schema implements action.Action.
func (a *pveGuestBulkMigrateAction) Schema(_ context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "**Disruptive:** bulk migrates all guests on the cluster to a target node via " +
			"`POST /cluster/bulk-action/guest/migrate` and waits for the worker task to finish.",
		Attributes: map[string]actionschema.Attribute{
			"target": &actionschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Target node for the migration.",
			},
			"max_workers": &actionschema.Int64Attribute{
				Optional:            true,
				Validators:          []validator.Int64{int64validator.Between(1, 64)},
				MarkdownDescription: "Maximum number of tasks running concurrently. Must be between 1 and 64. Defaults to 1 on the API.",
			},
			"online": &actionschema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Enable live migration for VMs and restart migration for CTs.",
			},
			"vms": &actionschema.SetAttribute{
				Optional:            true,
				ElementType:         types.Int64Type,
				MarkdownDescription: "Only consider guests from this set of VMIDs, e.g. `[100, 101]`.",
			},
			"with_local_disks": &actionschema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Enable live storage migration for local disks.",
			},
		},
	}
}

// Configure implements action.ActionWithConfigure.
func (a *pveGuestBulkMigrateAction) Configure(_ context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
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
func (a *pveGuestBulkMigrateAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var config pveGuestBulkMigrateActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if a.client == nil {
		resp.Diagnostics.AddError(
			"Unconfigured pve_guest_bulk_migrate",
			"The provider client was not configured; cannot bulk migrate guests.",
		)
		return
	}
	opts := pveclient.BulkMigrateGuestsOptions{
		Target: config.Target.ValueString(),
	}
	if !config.MaxWorkers.IsNull() && !config.MaxWorkers.IsUnknown() {
		v := config.MaxWorkers.ValueInt64()
		opts.MaxWorkers = &v
	}
	if !config.Online.IsNull() && !config.Online.IsUnknown() {
		v := config.Online.ValueBool()
		opts.Online = &v
	}
	if !config.WithLocalDisks.IsNull() && !config.WithLocalDisks.IsUnknown() {
		v := config.WithLocalDisks.ValueBool()
		opts.WithLocalDisks = &v
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
	tflog.Info(ctx, "bulk migrating guests", map[string]any{"target": opts.Target, "vms": len(vms)})
	progress(fmt.Sprintf("Bulk migrating guests on the cluster to %s", opts.Target))
	upid, err := a.client.BulkMigrateGuests(ctx, opts)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error invoking pve_guest_bulk_migrate",
			fmt.Sprintf("bulk migrating guests: %s", err),
		)
		return
	}
	taskNode, err := realmNodeFromUpid(upid)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error invoking pve_guest_bulk_migrate",
			fmt.Sprintf("determining the task node for the bulk migrate worker: %s", err),
		)
		return
	}
	progress(fmt.Sprintf("Bulk migrate worker %s running on node %s; waiting for it to finish", upid, taskNode))
	if _, err := a.client.WaitForTask(ctx, taskNode, upid, pveclient.WaitForTaskOptions{}); err != nil {
		resp.Diagnostics.AddError(
			"Error invoking pve_guest_bulk_migrate",
			fmt.Sprintf("waiting for the bulk migrate worker: %s", err),
		)
		return
	}
	progress("Bulk migrate finished")
}
