// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

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
	_ action.Action              = &pveHaArmAction{}
	_ action.ActionWithConfigure = &pveHaArmAction{}
)

// NewPveHaArmAction returns the action implementation.
func NewPveHaArmAction() action.Action {
	return &pveHaArmAction{}
}

// pveHaArmAction arms or disarms the cluster HA stack
// (POST /cluster/ha/status/arm-ha, POST /cluster/ha/status/disarm-ha).
// Both endpoints are synchronous per the pin (they return null).
type pveHaArmAction struct {
	client *pveclient.Client
}

// pveHaArmActionModel is the Terraform-facing shape.
type pveHaArmActionModel struct {
	Armed        types.Bool   `tfsdk:"armed"`
	ResourceMode types.String `tfsdk:"resource_mode"`
}

// Metadata implements action.Action.
func (a *pveHaArmAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveHaArm
}

// Schema implements action.Action.
func (a *pveHaArmAction) Schema(_ context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Arms or disarms the cluster HA stack. Arming re-enables the HA stack after it was disarmed. Disarming releases all watchdogs cluster-wide; HA-managed resources keep their current state unless `resource_mode` says otherwise. Both operations are synchronous on the API side.",
		Attributes: map[string]schema.Attribute{
			"armed": schema.BoolAttribute{
				Required:            true,
				MarkdownDescription: "Whether to arm (`true`) or disarm (`false`) the HA stack.",
			},
			"resource_mode": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: "Only used when `armed` is `false`: controls how HA-managed resources are handled while disarmed. " +
					"The current state of the resources is not affected. Must be one of: `freeze` (new commands and state changes are not applied), `ignore` (resources are removed from HA tracking and can be managed as if they were not HA-managed).",
				Validators: []validator.String{
					stringvalidator.OneOf("freeze", "ignore"),
				},
			},
		},
	}
}

// Configure implements action.ActionWithConfigure.
func (a *pveHaArmAction) Configure(_ context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
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
func (a *pveHaArmAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var config pveHaArmActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if a.client == nil {
		resp.Diagnostics.AddError("Error invoking pve_ha_arm", "provider client is not configured")
		return
	}

	if config.Armed.ValueBool() {
		if !config.ResourceMode.IsNull() && !config.ResourceMode.IsUnknown() {
			resp.Diagnostics.AddError(
				"Error invoking pve_ha_arm",
				"resource_mode is only valid when armed is false (the arm-ha endpoint takes no parameters)",
			)
			return
		}
		tflog.Info(ctx, "Arming HA stack via /cluster/ha/status/arm-ha")
		if resp.SendProgress != nil {
			resp.SendProgress(action.InvokeProgressEvent{Message: "Arming the HA stack"})
		}
		if err := a.client.ArmHA(ctx); err != nil {
			resp.Diagnostics.AddError(
				"Error invoking pve_ha_arm",
				fmt.Sprintf("arming the HA stack: %s", err),
			)
			return
		}
		tflog.Info(ctx, "HA stack armed")
		return
	}

	mode := ""
	if !config.ResourceMode.IsNull() && !config.ResourceMode.IsUnknown() {
		mode = config.ResourceMode.ValueString()
	}
	tflog.Info(ctx, "Disarming HA stack via /cluster/ha/status/disarm-ha", map[string]any{"resource_mode": mode})
	if resp.SendProgress != nil {
		resp.SendProgress(action.InvokeProgressEvent{Message: "Disarming the HA stack"})
	}
	if err := a.client.DisarmHA(ctx, mode); err != nil {
		resp.Diagnostics.AddError(
			"Error invoking pve_ha_arm",
			fmt.Sprintf("disarming the HA stack: %s", err),
		)
		return
	}
	tflog.Info(ctx, "HA stack disarmed")
}
