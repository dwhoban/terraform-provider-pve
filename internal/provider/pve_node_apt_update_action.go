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
	_ action.Action = &pveNodeAptUpdateAction{}
)

// NewPveNodeAptUpdateAction returns the action implementation.
func NewPveNodeAptUpdateAction() action.Action {
	return &pveNodeAptUpdateAction{}
}

// pveNodeAptUpdateAction resynchronizes the package index of one node
// (apt-get update, POST /nodes/{node}/apt/update) and waits for the worker
// task to finish.
type pveNodeAptUpdateAction struct {
	client *pveclient.Client
}

// pveNodeAptUpdateActionModel is the Terraform-facing shape.
type pveNodeAptUpdateActionModel struct {
	Node   types.String `tfsdk:"node"`
	Notify types.Bool   `tfsdk:"notify"`
	Quiet  types.Bool   `tfsdk:"quiet"`
}

// Metadata implements action.Action.
func (a *pveNodeAptUpdateAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveNodeAptUpdate
}

// Schema implements action.Action.
func (a *pveNodeAptUpdateAction) Schema(_ context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "Resynchronizes the package index files of one node from their sources (`apt-get update`, `POST /nodes/{node}/apt/update`) and waits for the task to finish. The action updates the package index only; upgrading installed packages remains an out-of-band task.",
		Attributes: map[string]actionschema.Attribute{
			"node": &actionschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The cluster node whose package index to refresh.",
			},
			"notify": &actionschema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Send a notification about new packages. Defaults to `false`.",
			},
			"quiet": &actionschema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Only produce output suitable for logging, omitting progress indicators. Defaults to `false`.",
			},
		},
	}
}

// Configure implements action.ActionWithConfigure.
func (a *pveNodeAptUpdateAction) Configure(_ context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
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
func (a *pveNodeAptUpdateAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var config pveNodeAptUpdateActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if a.client == nil {
		resp.Diagnostics.AddError(
			"Unconfigured pve_node_apt_update",
			fmt.Sprintf("The provider client was not configured; cannot update the package index on node %s.", config.Node.ValueString()),
		)
		return
	}
	progress := func(message string) {
		if resp.SendProgress != nil {
			resp.SendProgress(action.InvokeProgressEvent{Message: message})
		}
	}
	if err := a.run(ctx, config, progress); err != nil {
		resp.Diagnostics.AddError(
			"Error invoking pve_node_apt_update",
			fmt.Sprintf("updating package index on node %s: %s", config.Node.ValueString(), err),
		)
	}
}

// run performs the package index refresh and waits for the worker task.
func (a *pveNodeAptUpdateAction) run(ctx context.Context, config pveNodeAptUpdateActionModel, progress func(string)) error {
	node := config.Node.ValueString()
	in := pveclient.NodeAptUpdateInput{}
	if !config.Notify.IsNull() && !config.Notify.IsUnknown() {
		v := config.Notify.ValueBool()
		in.Notify = &v
	}
	if !config.Quiet.IsNull() && !config.Quiet.IsUnknown() {
		v := config.Quiet.ValueBool()
		in.Quiet = &v
	}
	tflog.Info(ctx, "starting apt update", map[string]any{"node": node})
	upid, err := a.client.NodeAptUpdate(ctx, node, in)
	if err != nil {
		return fmt.Errorf("starting apt update: %w", err)
	}
	waitNode, err := nodeSvcAptNodeFromUpid(upid)
	if err != nil {
		return err
	}
	progress(fmt.Sprintf("Package index update on node %s started; waiting for task to finish", waitNode))
	tflog.Debug(ctx, "waiting for apt update task", map[string]any{"node": waitNode, "upid": upid})
	if _, err := a.client.WaitForTask(ctx, waitNode, upid, pveclient.WaitForTaskOptions{}); err != nil {
		return err
	}
	progress(fmt.Sprintf("Package index update on node %s finished", waitNode))
	return nil
}
