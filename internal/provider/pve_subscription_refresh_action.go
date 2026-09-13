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
	_ action.Action              = &pveSubscriptionRefreshAction{}
	_ action.ActionWithConfigure = &pveSubscriptionRefreshAction{}
)

// NewPveSubscriptionRefreshAction returns the action implementation.
func NewPveSubscriptionRefreshAction() action.Action {
	return &pveSubscriptionRefreshAction{}
}

// pveSubscriptionRefreshAction updates a node's subscription info
// (POST /nodes/{node}/subscription).
type pveSubscriptionRefreshAction struct {
	client *pveclient.Client
}

// pveSubscriptionRefreshActionModel is the Terraform-facing shape.
type pveSubscriptionRefreshActionModel struct {
	Node  types.String `tfsdk:"node"`
	Force types.Bool   `tfsdk:"force"`
}

// Metadata implements action.Action.
func (a *pveSubscriptionRefreshAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveSubscriptionRefresh
}

// Schema implements action.Action.
func (a *pveSubscriptionRefreshAction) Schema(_ context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "Updates a node's subscription info (`POST /nodes/{node}/subscription`): the node contacts " +
			"the Proxmox subscription server, refreshes the locally cached subscription status, and re-runs the " +
			"subscription post-check, so repositories and the web UI reflect the current status. The refreshed state is " +
			"readable through the `pve_node_subscription` data source. The call is synchronous per the pin (no task " +
			"UPID is returned). Requires the `Sys.Modify` privilege on `/nodes/{node}`.",
		Attributes: map[string]actionschema.Attribute{
			"node": &actionschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The cluster node whose subscription info is refreshed.",
			},
			"force": &actionschema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Always connect to the subscription server, even if the local cache is still valid.",
			},
		},
	}
}

// Configure implements action.ActionWithConfigure.
func (a *pveSubscriptionRefreshAction) Configure(ctx context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	a.client = containerActionConfigure(req, resp)
}

// Invoke implements action.Action.
func (a *pveSubscriptionRefreshAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var config pveSubscriptionRefreshActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if a.client == nil {
		resp.Diagnostics.AddError(
			"Unconfigured pve_subscription_refresh",
			fmt.Sprintf("The provider client was not configured; cannot refresh subscription on %s.", config.Node.ValueString()),
		)
		return
	}
	node := config.Node.ValueString()
	var force *bool
	if !config.Force.IsNull() && !config.Force.IsUnknown() {
		v := config.Force.ValueBool()
		force = &v
	}
	tflog.Info(ctx, "refreshing node subscription", map[string]any{"node": node})
	if err := a.client.RefreshSubscription(ctx, node, force); err != nil {
		resp.Diagnostics.AddError(
			"Error invoking pve_subscription_refresh",
			fmt.Sprintf("refreshing subscription on node %s: %s", node, err),
		)
		return
	}
	if resp.SendProgress != nil {
		resp.SendProgress(action.InvokeProgressEvent{Message: fmt.Sprintf("Subscription info refreshed for node %s", node)})
	}
}
