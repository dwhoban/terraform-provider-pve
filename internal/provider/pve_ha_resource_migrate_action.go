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

	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ action.Action              = &pveHaResourceMigrateAction{}
	_ action.ActionWithConfigure = &pveHaResourceMigrateAction{}
)

// NewPveHaResourceMigrateAction returns the action implementation.
func NewPveHaResourceMigrateAction() action.Action {
	return &pveHaResourceMigrateAction{}
}

// pveHaResourceMigrateAction requests online migration of an HA resource
// to another node (POST /cluster/ha/resources/{sid}/migrate).
type pveHaResourceMigrateAction struct {
	client *pveclient.Client
}

// pveHaResourceMigrateActionModel is the Terraform-facing shape.
type pveHaResourceMigrateActionModel struct {
	SID  types.String `tfsdk:"sid"`
	Node types.String `tfsdk:"node"`
}

// Metadata implements action.Action.
func (a *pveHaResourceMigrateAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveHaResourceMigrate
}

// Schema implements action.Action.
func (a *pveHaResourceMigrateAction) Schema(_ context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "Requests online migration of an HA resource to another node " +
			"(`POST /cluster/ha/resources/{sid}/migrate`). The request is queued with the HA manager, which performs " +
			"the actual migration; this action returns once the request is accepted, because upstream reports no " +
			"task UPID to wait on. Requires the `Sys.Console` privilege on `/`.",
		Attributes: map[string]actionschema.Attribute{
			"sid": &actionschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "HA resource ID, `<type>:<name>` (e.g. `vm:100` / `ct:100`); the bare VM or CT ID (e.g. `100`) is also accepted.",
			},
			"node": &actionschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Target node to migrate the HA resource to.",
			},
		},
	}
}

// Configure implements action.ActionWithConfigure.
func (a *pveHaResourceMigrateAction) Configure(ctx context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	a.client = containerActionConfigure(req, resp)
}

// Invoke implements action.Action.
func (a *pveHaResourceMigrateAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var config pveHaResourceMigrateActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if a.client == nil {
		resp.Diagnostics.AddError(
			"Unconfigured pve_ha_resource_migrate",
			fmt.Sprintf("The provider client was not configured; cannot migrate HA resource %s.", config.SID.ValueString()),
		)
		return
	}
	sid := config.SID.ValueString()
	node := config.Node.ValueString()
	tflog.Info(ctx, "requesting HA resource migration", map[string]any{"sid": sid, "target": node})
	if err := a.client.MigrateHAResource(ctx, sid, node); err != nil {
		resp.Diagnostics.AddError(
			"Error invoking pve_ha_resource_migrate",
			fmt.Sprintf("requesting migration of HA resource %s to node %s: %s", sid, node, err),
		)
		return
	}
	if resp.SendProgress != nil {
		resp.SendProgress(action.InvokeProgressEvent{Message: fmt.Sprintf("Migration of HA resource %s to %s requested; the HA manager performs the migration", sid, node)})
	}
}
