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

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ action.Action              = &pveSdnRollbackAction{}
	_ action.ActionWithConfigure = &pveSdnRollbackAction{}
)

// NewPveSdnRollbackAction returns the action implementation.
func NewPveSdnRollbackAction() action.Action {
	return &pveSdnRollbackAction{}
}

// pveSdnRollbackAction discards all pending SDN configuration changes
// (POST /cluster/sdn/rollback). The pin returns null: the operation is
// synchronous, so there is no task to wait on.
type pveSdnRollbackAction struct {
	client *pveclient.Client
}

// pveSdnRollbackActionModel is the Terraform-facing shape.
type pveSdnRollbackActionModel struct {
	LockToken   types.String `tfsdk:"lock_token"`
	ReleaseLock types.Bool   `tfsdk:"release_lock"`
}

// Metadata implements action.Action.
func (a *pveSdnRollbackAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveSdnRollback
}

// Schema implements action.Action.
func (a *pveSdnRollbackAction) Schema(_ context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Rolls back the pending SDN configuration to the last applied state (`POST /cluster/sdn/rollback`). Warning: this discards *every* pending SDN change cluster-wide, including changes made outside Terraform; applied configuration is not touched.",
		Attributes: map[string]schema.Attribute{
			"lock_token": &schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Global SDN configuration lock token; when provided the rollback runs under this lock.",
			},
			"release_lock": &schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Release the lock supplied in `lock_token` automatically after a successful rollback. PVE defaults this to `true`.",
			},
		},
	}
}

// Configure implements action.ActionWithConfigure.
func (a *pveSdnRollbackAction) Configure(_ context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
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
func (a *pveSdnRollbackAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var config pveSdnRollbackActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if a.client == nil {
		resp.Diagnostics.AddError(
			"Unconfigured pve_sdn_rollback",
			"The provider client was not configured; cannot roll back the SDN configuration.",
		)
		return
	}
	if resp.SendProgress != nil {
		resp.SendProgress(action.InvokeProgressEvent{Message: "Rolling back pending SDN configuration"})
	}
	opts := pveclient.SdnLockOptions{}
	if !config.LockToken.IsNull() && !config.LockToken.IsUnknown() {
		opts.LockToken = config.LockToken.ValueString()
	}
	if !config.ReleaseLock.IsNull() && !config.ReleaseLock.IsUnknown() {
		v := config.ReleaseLock.ValueBool()
		opts.ReleaseLock = &v
	}
	tflog.Info(ctx, "rolling back SDN configuration")
	if err := a.client.RollbackSdn(ctx, opts); err != nil {
		resp.Diagnostics.AddError(
			"Error invoking pve_sdn_rollback",
			fmt.Sprintf("rolling back the SDN configuration: %s", err),
		)
		return
	}
	if resp.SendProgress != nil {
		resp.SendProgress(action.InvokeProgressEvent{Message: "Rollback finished"})
	}
}
