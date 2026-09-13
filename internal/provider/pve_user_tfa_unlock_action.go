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

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ action.Action              = &pveUserTfaUnlockAction{}
	_ action.ActionWithConfigure = &pveUserTfaUnlockAction{}
)

// NewPveUserTfaUnlockAction returns the action implementation.
func NewPveUserTfaUnlockAction() action.Action {
	return &pveUserTfaUnlockAction{}
}

// pveUserTfaUnlockAction unlocks a user's TFA authentication
// (PUT /access/users/{userid}/unlock-tfa).
type pveUserTfaUnlockAction struct {
	client *pveclient.Client
}

// pveUserTfaUnlockActionModel is the Terraform-facing shape.
type pveUserTfaUnlockActionModel struct {
	UserID types.String `tfsdk:"userid"`
}

// Metadata implements action.Action.
func (a *pveUserTfaUnlockAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveUserTfaUnlock
}

// Schema implements action.Action.
func (a *pveUserTfaUnlockAction) Schema(_ context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "Unlocks a user's TFA authentication (`PUT /access/users/{userid}/unlock-tfa`), clearing the " +
			"lockout so the user can authenticate again and re-enroll second factors.\n\n" +
			"~> **Security warning:** this is a security-sensitive operation. Unlocking TFA weakens the account's " +
			"second-factor protection; validate the provenance of any configuration invoking this action.",
		Attributes: map[string]actionschema.Attribute{
			"userid": &actionschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Full User ID to unlock, in the `name@realm` format, e.g. `root@pam`.",
			},
		},
	}
}

// Configure implements action.ActionWithConfigure.
func (a *pveUserTfaUnlockAction) Configure(ctx context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	a.client = containerActionConfigure(req, resp)
}

// Invoke implements action.Action.
func (a *pveUserTfaUnlockAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var config pveUserTfaUnlockActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if a.client == nil {
		resp.Diagnostics.AddError(
			"Unconfigured pve_user_tfa_unlock",
			fmt.Sprintf("The provider client was not configured; cannot unlock TFA for %s.", config.UserID.ValueString()),
		)
		return
	}
	userid := config.UserID.ValueString()
	tflog.Info(ctx, "unlocking user TFA", map[string]any{"userid": userid})
	if err := a.client.UnlockUserTFA(ctx, userid); err != nil {
		resp.Diagnostics.AddError(
			"Error invoking pve_user_tfa_unlock",
			fmt.Sprintf("unlocking TFA for user %s: %s", userid, err),
		)
		return
	}
	if resp.SendProgress != nil {
		resp.SendProgress(action.InvokeProgressEvent{Message: fmt.Sprintf("TFA for user %s unlocked", userid)})
	}
}
