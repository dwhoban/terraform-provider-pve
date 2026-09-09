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
	_ action.Action              = &pveSdnApplyAction{}
	_ action.ActionWithConfigure = &pveSdnApplyAction{}
)

// NewPveSdnApplyAction returns the action implementation.
func NewPveSdnApplyAction() action.Action {
	return &pveSdnApplyAction{}
}

// pveSdnApplyAction applies the pending SDN configuration cluster-wide
// (PUT /cluster/sdn) and waits for the reload task.
type pveSdnApplyAction struct {
	client *pveclient.Client
}

// pveSdnApplyActionModel is the Terraform-facing shape.
type pveSdnApplyActionModel struct {
	LockToken   types.String `tfsdk:"lock_token"`
	ReleaseLock types.Bool   `tfsdk:"release_lock"`
}

// Metadata implements action.Action.
func (a *pveSdnApplyAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveSdnApply
}

// Schema implements action.Action.
func (a *pveSdnApplyAction) Schema(_ context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Applies the pending SDN configuration cluster-wide and reloads ifreload and FRR (`PUT /cluster/sdn`), then waits for the reload task to finish. Warning: this pushes *every* pending SDN change (zones, vnets, subnets, fabrics, controllers) to all nodes at once; validate pending configuration with the SDN diff view or `pvesh get /cluster/sdn/dry-run` before invoking.",
		Attributes: map[string]schema.Attribute{
			"lock_token": &schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Global SDN configuration lock token; when provided the apply commits under this lock.",
			},
			"release_lock": &schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Release the lock supplied in `lock_token` automatically after a successful apply. PVE defaults this to `true`.",
			},
		},
	}
}

// Configure implements action.ActionWithConfigure.
func (a *pveSdnApplyAction) Configure(_ context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
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
func (a *pveSdnApplyAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var config pveSdnApplyActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if a.client == nil {
		resp.Diagnostics.AddError(
			"Unconfigured pve_sdn_apply",
			"The provider client was not configured; cannot apply the SDN configuration.",
		)
		return
	}
	progress := func(message string) {
		if resp.SendProgress != nil {
			resp.SendProgress(action.InvokeProgressEvent{Message: message})
		}
	}
	opts := pveclient.SdnLockOptions{}
	if !config.LockToken.IsNull() && !config.LockToken.IsUnknown() {
		opts.LockToken = config.LockToken.ValueString()
	}
	if !config.ReleaseLock.IsNull() && !config.ReleaseLock.IsUnknown() {
		v := config.ReleaseLock.ValueBool()
		opts.ReleaseLock = &v
	}
	tflog.Info(ctx, "applying SDN configuration")
	progress("Applying pending SDN configuration cluster-wide")
	upid, err := a.client.ApplySdn(ctx, opts)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error invoking pve_sdn_apply",
			fmt.Sprintf("applying the SDN configuration: %s", err),
		)
		return
	}
	progress("Apply task started")
	if err := vmWaitForTask(ctx, a.client, upid, progress); err != nil {
		resp.Diagnostics.AddError(
			"Error invoking pve_sdn_apply",
			fmt.Sprintf("waiting for the SDN apply task: %s", err),
		)
	}
}
