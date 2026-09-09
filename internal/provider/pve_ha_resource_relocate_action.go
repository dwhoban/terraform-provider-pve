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
	_ action.Action              = &pveHaResourceRelocateAction{}
	_ action.ActionWithConfigure = &pveHaResourceRelocateAction{}
)

// NewPveHaResourceRelocateAction returns the action implementation.
func NewPveHaResourceRelocateAction() action.Action {
	return &pveHaResourceRelocateAction{}
}

// pveHaResourceRelocateAction requests relocation of an HA resource to
// another node (POST /cluster/ha/resources/{sid}/relocate): the service is
// stopped on the old node and restarted on the target.
type pveHaResourceRelocateAction struct {
	client *pveclient.Client
}

// pveHaResourceRelocateActionModel is the Terraform-facing shape.
type pveHaResourceRelocateActionModel struct {
	SID  types.String `tfsdk:"sid"`
	Node types.String `tfsdk:"node"`
}

// Metadata implements action.Action.
func (a *pveHaResourceRelocateAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveHaResourceRelocate
}

// Schema implements action.Action.
func (a *pveHaResourceRelocateAction) Schema(_ context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "Requests relocation of an HA resource to another node " +
			"(`POST /cluster/ha/resources/{sid}/relocate`): unlike a migrate, the service is stopped on the old node " +
			"and restarted on the target, causing downtime. The request is queued with the HA manager; this action " +
			"returns once the request is accepted, because upstream reports no task UPID to wait on. " +
			"Requires the `Sys.Console` privilege on `/`.",
		Attributes: map[string]actionschema.Attribute{
			"sid": &actionschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "HA resource ID, `<type>:<name>` (e.g. `vm:100` / `ct:100`); the bare VM or CT ID (e.g. `100`) is also accepted.",
			},
			"node": &actionschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Target node to relocate the HA resource to.",
			},
		},
	}
}

// Configure implements action.ActionWithConfigure.
func (a *pveHaResourceRelocateAction) Configure(ctx context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	a.client = containerActionConfigure(req, resp)
}

// Invoke implements action.Action.
func (a *pveHaResourceRelocateAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var config pveHaResourceRelocateActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if a.client == nil {
		resp.Diagnostics.AddError(
			"Unconfigured pve_ha_resource_relocate",
			fmt.Sprintf("The provider client was not configured; cannot relocate HA resource %s.", config.SID.ValueString()),
		)
		return
	}
	sid := config.SID.ValueString()
	node := config.Node.ValueString()
	tflog.Info(ctx, "requesting HA resource relocation", map[string]any{"sid": sid, "target": node})
	if err := a.client.RelocateHAResource(ctx, sid, node); err != nil {
		resp.Diagnostics.AddError(
			"Error invoking pve_ha_resource_relocate",
			fmt.Sprintf("requesting relocation of HA resource %s to node %s: %s", sid, node, err),
		)
		return
	}
	if resp.SendProgress != nil {
		resp.SendProgress(action.InvokeProgressEvent{Message: fmt.Sprintf("Relocation of HA resource %s to %s requested; the HA manager stops and restarts the service", sid, node)})
	}
}
