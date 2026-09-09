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
	_ action.Action = &pveNodeMigrateAllAction{}
)

// NewPveNodeMigrateAllAction returns the action implementation.
func NewPveNodeMigrateAllAction() action.Action {
	return &pveNodeMigrateAllAction{}
}

// pveNodeMigrateAllAction migrates all guests off a node via POST
// /nodes/{node}/migrateall and waits for the worker task.
type pveNodeMigrateAllAction struct {
	client *pveclient.Client
}

// pveNodeMigrateAllActionModel is the Terraform-facing shape.
type pveNodeMigrateAllActionModel struct {
	Node           types.String `tfsdk:"node"`
	Target         types.String `tfsdk:"target"`
	MaxWorkers     types.Int64  `tfsdk:"max_workers"`
	Vms            types.String `tfsdk:"vms"`
	WithLocalDisks types.Bool   `tfsdk:"with_local_disks"`
}

// Metadata implements action.Action.
func (a *pveNodeMigrateAllAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveNodeMigrateAll
}

// Schema implements action.Action.
func (a *pveNodeMigrateAllAction) Schema(_ context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "**Disruptive:** migrates all VMs and containers from this node to a target node via " +
			"`POST /nodes/{node}/migrateall` and waits for the worker task to finish.",
		Attributes: map[string]actionschema.Attribute{
			"node": &actionschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The cluster node whose guests are migrated away.",
			},
			"target": &actionschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Target node for the migration.",
			},
			"max_workers": &actionschema.Int64Attribute{
				Optional:            true,
				Validators:          []validator.Int64{int64validator.Between(1, 64)},
				MarkdownDescription: "Maximum number of parallel migration jobs. Must be between 1 and 64. When unset, PVE uses `max_workers` from datacenter.cfg.",
			},
			"vms": &actionschema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Only consider guests with these IDs (comma-separated VMID list), e.g. `100,101`.",
			},
			"with_local_disks": &actionschema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Enable live storage migration for local disks.",
			},
		},
	}
}

// Configure implements action.ActionWithConfigure.
func (a *pveNodeMigrateAllAction) Configure(_ context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
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
func (a *pveNodeMigrateAllAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var config pveNodeMigrateAllActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if a.client == nil {
		resp.Diagnostics.AddError(
			"Unconfigured pve_node_migrate_all",
			fmt.Sprintf("The provider client was not configured; cannot migrate all guests from node %s.", config.Node.ValueString()),
		)
		return
	}
	node := config.Node.ValueString()
	opts := pveclient.MigrateAllNodeGuestsOptions{
		Target: config.Target.ValueString(),
	}
	if !config.MaxWorkers.IsNull() && !config.MaxWorkers.IsUnknown() {
		v := config.MaxWorkers.ValueInt64()
		opts.MaxWorkers = &v
	}
	if !config.Vms.IsNull() && !config.Vms.IsUnknown() {
		opts.Vms = config.Vms.ValueString()
	}
	if !config.WithLocalDisks.IsNull() && !config.WithLocalDisks.IsUnknown() {
		v := config.WithLocalDisks.ValueBool()
		opts.WithLocalDisks = &v
	}
	progress := func(message string) {
		if resp.SendProgress != nil {
			resp.SendProgress(action.InvokeProgressEvent{Message: message})
		}
	}
	tflog.Info(ctx, "migrating all guests from node", map[string]any{"node": node, "target": opts.Target})
	progress(fmt.Sprintf("Migrating guests from node %s to %s", node, opts.Target))
	upid, err := a.client.MigrateAllNodeGuests(ctx, node, opts)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error invoking pve_node_migrate_all",
			fmt.Sprintf("migrating all guests from node %s: %s", node, err),
		)
		return
	}
	taskNode, err := realmNodeFromUpid(upid)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error invoking pve_node_migrate_all",
			fmt.Sprintf("determining the task node for the migrateall worker on node %s: %s", node, err),
		)
		return
	}
	progress(fmt.Sprintf("Migrateall worker %s running on node %s; waiting for it to finish", upid, taskNode))
	if _, err := a.client.WaitForTask(ctx, taskNode, upid, pveclient.WaitForTaskOptions{}); err != nil {
		resp.Diagnostics.AddError(
			"Error invoking pve_node_migrate_all",
			fmt.Sprintf("waiting for the migrateall worker on node %s: %s", node, err),
		)
		return
	}
	progress(fmt.Sprintf("All requested guests migrated from node %s to %s", node, opts.Target))
}
