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
	_ action.Action              = &pveNodeWakeonlanAction{}
	_ action.ActionWithConfigure = &pveNodeWakeonlanAction{}
)

// NewPveNodeWakeonlanAction returns the action implementation.
func NewPveNodeWakeonlanAction() action.Action {
	return &pveNodeWakeonlanAction{}
}

// pveNodeWakeonlanAction wakes a node by sending a wake on LAN magic packet
// (POST /nodes/{node}/wakeonlan).
type pveNodeWakeonlanAction struct {
	client *pveclient.Client
}

// pveNodeWakeonlanActionModel is the Terraform-facing shape.
type pveNodeWakeonlanActionModel struct {
	Node types.String `tfsdk:"node"`
}

// Metadata implements action.Action.
func (a *pveNodeWakeonlanAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveNodeWakeonlan
}

// Schema implements action.Action.
func (a *pveNodeWakeonlanAction) Schema(_ context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "Sends a wake on LAN magic packet to the node's configured WoL interface (`POST /nodes/{node}/wakeonlan`). The node must have its `wakeonlan` setting configured with the bind interface used for the packet; the action completes once PVE accepts the packet and does not wait for the node to come up. Requires `Sys.PowerMgmt` on `/nodes/{node}`.",
		Attributes: map[string]actionschema.Attribute{
			"node": &actionschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The target node for the wake on LAN packet.",
			},
		},
	}
}

// Configure implements action.ActionWithConfigure.
func (a *pveNodeWakeonlanAction) Configure(_ context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
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
func (a *pveNodeWakeonlanAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var config pveNodeWakeonlanActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if a.client == nil {
		resp.Diagnostics.AddError(
			"Unconfigured pve_node_wakeonlan",
			fmt.Sprintf("The provider client was not configured; cannot wake node %s.", config.Node.ValueString()),
		)
		return
	}
	node := config.Node.ValueString()
	tflog.Info(ctx, "sending wake on LAN packet", map[string]any{"node": node})
	if resp.SendProgress != nil {
		resp.SendProgress(action.InvokeProgressEvent{Message: fmt.Sprintf("Sending wake on LAN packet to node %s", node)})
	}
	// The MAC address PVE echoes back is only meaningful to the node's own
	// WoL configuration; the action surfaces no output to avoid echoing it.
	if _, err := a.client.NodeWakeonLAN(ctx, node); err != nil {
		resp.Diagnostics.AddError(
			"Error invoking pve_node_wakeonlan",
			fmt.Sprintf("waking node %s: %s", node, err),
		)
		return
	}
	if resp.SendProgress != nil {
		resp.SendProgress(action.InvokeProgressEvent{Message: fmt.Sprintf("Wake on LAN packet sent to node %s", node)})
	}
}
