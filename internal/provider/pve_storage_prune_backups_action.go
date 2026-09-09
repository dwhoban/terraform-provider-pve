// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"math"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ action.Action                   = &pveStoragePruneBackupsAction{}
	_ action.ActionWithConfigure      = &pveStoragePruneBackupsAction{}
	_ action.ActionWithValidateConfig = &pveStoragePruneBackupsAction{}
)

// NewPveStoragePruneBackupsAction returns the action implementation.
func NewPveStoragePruneBackupsAction() action.Action {
	return &pveStoragePruneBackupsAction{}
}

// pveStoragePruneBackupsAction prunes backups that follow the standard
// naming scheme on a storage (DELETE
// /nodes/{node}/storage/{storage}/prunebackups), waiting for the worker
// task to finish. With dry_run the action instead reports what would be
// pruned without deleting anything (GET .../prunebackups).
type pveStoragePruneBackupsAction struct {
	client *pveclient.Client
}

// pveStoragePruneBackupsActionModel is the Terraform-facing shape.
type pveStoragePruneBackupsActionModel struct {
	Node        types.String `tfsdk:"node"`
	Storage     types.String `tfsdk:"storage"`
	DryRun      types.Bool   `tfsdk:"dry_run"`
	KeepAll     types.Bool   `tfsdk:"keep_all"`
	KeepHourly  types.Int64  `tfsdk:"keep_hourly"`
	KeepDaily   types.Int64  `tfsdk:"keep_daily"`
	KeepWeekly  types.Int64  `tfsdk:"keep_weekly"`
	KeepMonthly types.Int64  `tfsdk:"keep_monthly"`
	KeepYearly  types.Int64  `tfsdk:"keep_yearly"`
	KeepLast    types.Int64  `tfsdk:"keep_last"`
	Type        types.String `tfsdk:"type"`
	VMID        types.Int64  `tfsdk:"vmid"`
}

// Metadata implements action.Action.
func (a *pveStoragePruneBackupsAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveStoragePruneBackups
}

// Schema implements action.Action.
func (a *pveStoragePruneBackupsAction) Schema(_ context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	retention := func(what string) schema.Attribute {
		return schema.Int64Attribute{
			Optional:            true,
			MarkdownDescription: fmt.Sprintf("%s, overriding the storage configuration's retention. Must be at least 1.", what),
			Validators: []validator.Int64{
				int64validator.Between(1, math.MaxInt64),
			},
		}
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Prunes backups on a storage that follow the standard naming scheme " +
			"(`DELETE /nodes/{node}/storage/{storage}/prunebackups`), waiting for the worker task to finish. " +
			"With `dry_run` the action instead reports what would be pruned without deleting anything " +
			"(`GET .../prunebackups`). The retention options override the storage configuration's retention; " +
			"at least one must be set.",
		Attributes: map[string]schema.Attribute{
			"node": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The cluster node name.",
			},
			"storage": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The storage identifier, e.g. `local` or the `storage` of a `pve_storage_*` resource.",
			},
			"dry_run": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Report what would be pruned without deleting anything. When `true`, the action calls the dry-run endpoint and reports the number of backups that would be removed, kept, protected, or renamed; no backups are removed. Defaults to `false`.",
			},
			"keep_all": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Keep every backup regardless of the other retention rules, overriding the storage configuration's retention.",
			},
			"keep_hourly":  retention("Keep the last N hourly backups"),
			"keep_daily":   retention("Keep the last N daily backups"),
			"keep_weekly":  retention("Keep the last N weekly backups"),
			"keep_monthly": retention("Keep the last N monthly backups"),
			"keep_yearly":  retention("Keep the last N yearly backups"),
			"keep_last":    retention("Keep the last N backups in creation order"),
			"type": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Only consider backups for guests of this type. Must be one of: `qemu`, `lxc`.",
				Validators: []validator.String{
					stringvalidator.OneOf("qemu", "lxc"),
				},
			},
			"vmid": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Only prune backups for this guest. Must be between 100 and 999999999.",
				Validators: []validator.Int64{
					int64validator.Between(100, 999999999),
				},
			},
		},
	}
}

// Configure implements action.ActionWithConfigure.
func (a *pveStoragePruneBackupsAction) Configure(_ context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
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

// ValidateConfig implements action.ActionWithValidateConfig. The pin marks
// prune-backups optional (PVE would fall back to the storage configuration's
// retention), but a prune without an explicit retention silently deletes
// unpredictably, so the action requires at least one retention option.
func (a *pveStoragePruneBackupsAction) ValidateConfig(ctx context.Context, req action.ValidateConfigRequest, resp *action.ValidateConfigResponse) {
	var model pveStoragePruneBackupsActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if storagePruneRetentionSet(model) {
		return
	}
	resp.Diagnostics.AddError(
		"Error validating pve_storage_prune_backups",
		"At least one retention option must be set; set one of: keep_all, keep_hourly, keep_daily, keep_weekly, keep_monthly, keep_yearly, keep_last.",
	)
}

// Invoke implements action.Action.
func (a *pveStoragePruneBackupsAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var model pveStoragePruneBackupsActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if a.client == nil {
		resp.Diagnostics.AddError("Error invoking pve_storage_prune_backups", "provider client is not configured")
		return
	}
	node := model.Node.ValueString()
	storage := model.Storage.ValueString()
	progress := func(message string) {
		if resp.SendProgress != nil {
			resp.SendProgress(action.InvokeProgressEvent{Message: message})
		}
	}
	opts := storagePruneOptionsFromModel(model)

	if !model.DryRun.IsNull() && !model.DryRun.IsUnknown() && model.DryRun.ValueBool() {
		tflog.Info(ctx, "starting prune dry run", map[string]any{"node": node, "storage": storage})
		progress(fmt.Sprintf("Checking prune preview for storage %s on node %s", storage, node))
		entries, err := a.client.DryRunStoragePruneBackups(ctx, node, storage, opts)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error invoking pve_storage_prune_backups",
				fmt.Sprintf("dry-running the prune of storage %s on node %s: %s", storage, node, err),
			)
			return
		}
		var removed, kept, protected, renamed int
		for _, entry := range entries {
			switch entry.Mark {
			case "remove":
				removed++
			case "keep":
				kept++
			case "protected":
				protected++
			case "renamed":
				renamed++
			}
		}
		tflog.Info(ctx, "prune dry run finished", map[string]any{
			"node": node, "storage": storage,
			"removed": removed, "kept": kept, "protected": protected, "renamed": renamed,
		})
		progress(fmt.Sprintf("Dry run for storage %s on node %s: %d backup(s) would be removed, %d kept, %d protected, %d renamed. No backups were deleted.",
			storage, node, removed, kept, protected, renamed))
		return
	}

	tflog.Info(ctx, "starting prune", map[string]any{"node": node, "storage": storage})
	progress(fmt.Sprintf("Pruning backups on storage %s (node %s)", storage, node))
	upid, err := a.client.PruneStorageBackups(ctx, node, storage, opts)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error invoking pve_storage_prune_backups",
			fmt.Sprintf("pruning storage %s on node %s: %s", storage, node, err),
		)
		return
	}
	if upid == "" {
		// The pin always returns a UPID; tolerate an empty body anyway.
		progress(fmt.Sprintf("Prune of storage %s on node %s finished (no task reported).", storage, node))
		return
	}
	progress(fmt.Sprintf("Prune of storage %s on node %s started; waiting for the task to finish", storage, node))
	tflog.Debug(ctx, "waiting for prune task", map[string]any{"node": node, "storage": storage})
	if _, err := a.client.WaitForTask(ctx, node, upid, pveclient.WaitForTaskOptions{}); err != nil {
		resp.Diagnostics.AddError(
			"Error waiting for pve_storage_prune_backups task",
			fmt.Sprintf("prune of storage %s on node %s did not complete: %s", storage, node, err),
		)
		return
	}
	progress(fmt.Sprintf("Prune of storage %s on node %s finished.", storage, node))
}

// storagePruneRetentionSet reports whether any retention option is set,
// treating unknown values (interpolation) as set.
func storagePruneRetentionSet(model pveStoragePruneBackupsActionModel) bool {
	if model.KeepAll.IsUnknown() || !model.KeepAll.IsNull() {
		return true
	}
	for _, v := range []types.Int64{model.KeepHourly, model.KeepDaily, model.KeepWeekly, model.KeepMonthly, model.KeepYearly, model.KeepLast} {
		if v.IsUnknown() || !v.IsNull() {
			return true
		}
	}
	return false
}

// storagePruneOptionsFromModel maps the Terraform-facing model onto the
// client's typed prune options, omitting null values.
func storagePruneOptionsFromModel(model pveStoragePruneBackupsActionModel) pveclient.StoragePruneOptions {
	opts := pveclient.StoragePruneOptions{}
	if !model.KeepAll.IsNull() && !model.KeepAll.IsUnknown() {
		v := model.KeepAll.ValueBool()
		opts.KeepAll = &v
	}
	for _, pair := range []struct {
		value types.Int64
		dest  **int64
	}{
		{model.KeepHourly, &opts.KeepHourly},
		{model.KeepDaily, &opts.KeepDaily},
		{model.KeepWeekly, &opts.KeepWeekly},
		{model.KeepMonthly, &opts.KeepMonthly},
		{model.KeepYearly, &opts.KeepYearly},
		{model.KeepLast, &opts.KeepLast},
	} {
		if !pair.value.IsNull() && !pair.value.IsUnknown() {
			v := pair.value.ValueInt64()
			*pair.dest = &v
		}
	}
	if !model.Type.IsNull() && !model.Type.IsUnknown() {
		opts.Type = model.Type.ValueString()
	}
	if !model.VMID.IsNull() && !model.VMID.IsUnknown() {
		v := model.VMID.ValueInt64()
		opts.VMID = &v
	}
	return opts
}
