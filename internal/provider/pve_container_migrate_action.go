// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	float64validator "github.com/hashicorp/terraform-plugin-framework-validators/float64validator"
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
	_ action.Action = &pveContainerMigrateAction{}
)

// NewPveContainerMigrateAction returns the action implementation.
func NewPveContainerMigrateAction() action.Action {
	return &pveContainerMigrateAction{}
}

// pveContainerMigrateAction migrates an LXC container to another node via
// POST /nodes/{node}/lxc/{vmid}/migrate and waits for the migration task.
type pveContainerMigrateAction struct {
	client *pveclient.Client
}

// pveContainerMigrateActionModel is the Terraform-facing shape.
type pveContainerMigrateActionModel struct {
	Node          types.String  `tfsdk:"node"`
	VMID          types.Int64   `tfsdk:"vmid"`
	Target        types.String  `tfsdk:"target"`
	TargetStorage types.String  `tfsdk:"target_storage"`
	Online        types.Bool    `tfsdk:"online"`
	Restart       types.Bool    `tfsdk:"restart"`
	Bwlimit       types.Float64 `tfsdk:"bwlimit"`
	Timeout       types.Int64   `tfsdk:"timeout"`
}

// Metadata implements action.Action.
func (a *pveContainerMigrateAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveContainerMigrate
}

// Schema implements action.Action.
func (a *pveContainerMigrateAction) Schema(_ context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "Migrates an LXC container to another cluster node (`POST /nodes/{node}/lxc/{vmid}/migrate`). " +
			"**Destructive:** a restart migration (`restart`) stops the container for the move; an offline migration of a " +
			"running container fails unless the container is stopped first. Requires the `VM.Migrate` privilege on `/vms/{vmid}`.",
		Attributes: map[string]actionschema.Attribute{
			"node": &actionschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The cluster node the container currently runs on; the migration task runs here.",
			},
			"vmid": &actionschema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "The (unique) ID of the container. Must be between 100 and 999999999.",
				Validators:          []validator.Int64{containerActionVMIDValidator()},
			},
			"target": &actionschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Target node to migrate the container to.",
			},
			"target_storage": &actionschema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Mapping from source to target storages (`storage-pair-list`). Providing only a single storage ID maps all source storages to that storage; the special value `1` maps each source storage to itself.",
			},
			"online": &actionschema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Use online (live) migration when the container supports it.",
			},
			"restart": &actionschema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Use restart migration: the container is stopped, moved, and started on the target node.",
			},
			"bwlimit": &actionschema.Float64Attribute{
				Optional:            true,
				MarkdownDescription: "Override I/O bandwidth limit in KiB/s. Must be 0 or greater; omit to use the migrate limit from datacenter or storage config.",
				Validators:          []validator.Float64{float64validator.AtLeast(0)},
			},
			"timeout": &actionschema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Timeout in seconds for the shutdown phase of a restart migration. Must be 0 or greater; omit for the PVE default (180).",
				Validators:          []validator.Int64{int64validator.AtLeast(0)},
			},
		},
	}
}

// Configure implements action.ActionWithConfigure.
func (a *pveContainerMigrateAction) Configure(_ context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	a.client = containerActionConfigure(req, resp)
}

// Invoke implements action.Action.
func (a *pveContainerMigrateAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var config pveContainerMigrateActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if a.client == nil {
		resp.Diagnostics.AddError(
			"Unconfigured pve_container_migrate",
			fmt.Sprintf("The provider client was not configured; cannot migrate container %d from %s.", config.VMID.ValueInt64(), config.Node.ValueString()),
		)
		return
	}
	node := config.Node.ValueString()
	vmid := config.VMID.ValueInt64()
	target := config.Target.ValueString()
	params := pveclient.LxcMigrateParams{Target: target}
	if !config.TargetStorage.IsNull() && !config.TargetStorage.IsUnknown() {
		params.TargetStorage = config.TargetStorage.ValueString()
	}
	if !config.Online.IsNull() && !config.Online.IsUnknown() {
		online := config.Online.ValueBool()
		params.Online = &online
	}
	if !config.Restart.IsNull() && !config.Restart.IsUnknown() {
		restart := config.Restart.ValueBool()
		params.Restart = &restart
	}
	if !config.Bwlimit.IsNull() && !config.Bwlimit.IsUnknown() {
		bwlimit := config.Bwlimit.ValueFloat64()
		params.Bwlimit = &bwlimit
	}
	if !config.Timeout.IsNull() && !config.Timeout.IsUnknown() {
		timeout := config.Timeout.ValueInt64()
		params.Timeout = &timeout
	}
	progress := func(message string) {
		if resp.SendProgress != nil {
			resp.SendProgress(action.InvokeProgressEvent{Message: message})
		}
	}
	tflog.Info(ctx, "migrating container", map[string]any{"node": node, "vmid": vmid, "target": target})
	progress(fmt.Sprintf("Starting migration of container %d from %s to %s", vmid, node, target))
	upid, err := a.client.MigrateLxcContainer(ctx, node, vmid, params)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error invoking pve_container_migrate",
			fmt.Sprintf("migrating container %d from %s to %s: %s", vmid, node, target, err),
		)
		return
	}
	progress(fmt.Sprintf("Waiting for migration task on %s to finish", node))
	if _, err := a.client.WaitForTask(ctx, node, upid, defaultWaitOptions()); err != nil {
		resp.Diagnostics.AddError(
			"Error waiting for pve_container_migrate",
			fmt.Sprintf("waiting for migration of container %d from %s to %s: %s", vmid, node, target, err),
		)
		return
	}
	tflog.Info(ctx, "container migrated", map[string]any{"node": node, "vmid": vmid, "target": target})
	progress(fmt.Sprintf("Container %d migrated to %s", vmid, target))
}
