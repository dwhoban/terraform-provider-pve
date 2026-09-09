// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ action.Action              = &pveNotificationTestAction{}
	_ action.ActionWithConfigure = &pveNotificationTestAction{}
)

// NewPveNotificationTestAction returns the action implementation.
func NewPveNotificationTestAction() action.Action {
	return &pveNotificationTestAction{}
}

// pveNotificationTestAction sends a test notification through a named
// target or matcher (POST /cluster/notifications/targets/{name}/test).
// The endpoint is synchronous per the pin (returns null, no task).
type pveNotificationTestAction struct {
	client *pveclient.Client
}

// pveNotificationTestActionModel is the Terraform-facing shape.
type pveNotificationTestActionModel struct {
	Target types.String `tfsdk:"target"`
}

// Metadata implements action.Action.
func (a *pveNotificationTestAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveNotificationTest
}

// Schema implements action.Action.
func (a *pveNotificationTestAction) Schema(_ context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Sends a test notification to the provided target or matcher (`POST /cluster/notifications/targets/{name}/test`). Use it to verify that an endpoint routes and delivers correctly. The operation is synchronous per the pin (no task is spawned).",
		Attributes: map[string]schema.Attribute{
			"target": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Name of the target or matcher to send the test notification through. Built-in targets (e.g. `mail-to-root`) and endpoints managed by the `pve_notification_endpoint_*` resources are listed by the `pve_notification_targets` data source.",
			},
		},
	}
}

// Configure implements action.ActionWithConfigure.
func (a *pveNotificationTestAction) Configure(_ context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
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
func (a *pveNotificationTestAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var config pveNotificationTestActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if a.client == nil {
		resp.Diagnostics.AddError("Error invoking pve_notification_test", "provider client is not configured")
		return
	}

	target := config.Target.ValueString()
	tflog.Info(ctx, "sending test notification", map[string]any{"target": target})
	if resp.SendProgress != nil {
		resp.SendProgress(action.InvokeProgressEvent{Message: fmt.Sprintf("Sending a test notification through %s", target)})
	}
	if err := a.client.TestNotificationTarget(ctx, target); err != nil {
		resp.Diagnostics.AddError(
			"Error invoking pve_notification_test",
			fmt.Sprintf("sending a test notification through %s: %s", target, err),
		)
		return
	}
	tflog.Info(ctx, "test notification sent", map[string]any{"target": target})
}
