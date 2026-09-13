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
	_ action.Action              = &pveAplinfoUpdateAction{}
	_ action.ActionWithConfigure = &pveAplinfoUpdateAction{}
)

// NewPveAplinfoUpdateAction returns the action implementation.
func NewPveAplinfoUpdateAction() action.Action {
	return &pveAplinfoUpdateAction{}
}

// pveAplinfoUpdateAction downloads an appliance template
// (POST /nodes/{node}/aplinfo, upstream operation apl_download) and waits
// for the task to finish. The pinned API defines no separate
// appliance-index update endpoint; this POST is the only write verb on the
// aplinfo path.
type pveAplinfoUpdateAction struct {
	client *pveclient.Client
}

// pveAplinfoUpdateActionModel is the Terraform-facing shape.
type pveAplinfoUpdateActionModel struct {
	Node     types.String `tfsdk:"node"`
	Storage  types.String `tfsdk:"storage"`
	Template types.String `tfsdk:"template"`
}

// Metadata implements action.Action.
func (a *pveAplinfoUpdateAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveAplinfoUpdate
}

// Schema implements action.Action.
func (a *pveAplinfoUpdateAction) Schema(_ context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "Downloads an appliance template to a storage (`POST /nodes/{node}/aplinfo`; upstream " +
			"operation `apl_download`) and waits for the download task to finish. The pinned API defines no separate " +
			"appliance-index update endpoint, so this POST is the write verb on the `aplinfo` path. Available templates " +
			"are listed by the `pve_appliances` data source. Requires the `Datastore.AllocateTemplate` privilege on " +
			"`/storage/{storage}`.",
		Attributes: map[string]actionschema.Attribute{
			"node": &actionschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The cluster node downloading the template; the task runs here.",
			},
			"storage": &actionschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The storage where the template will be stored.",
			},
			"template": &actionschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The template to download, e.g. `debian-12-standard_12.2-1_amd64.tar.zst`.",
			},
		},
	}
}

// Configure implements action.ActionWithConfigure.
func (a *pveAplinfoUpdateAction) Configure(ctx context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	a.client = containerActionConfigure(req, resp)
}

// Invoke implements action.Action.
func (a *pveAplinfoUpdateAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var config pveAplinfoUpdateActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if a.client == nil {
		resp.Diagnostics.AddError(
			"Unconfigured pve_aplinfo_update",
			fmt.Sprintf("The provider client was not configured; cannot download template %s.", config.Template.ValueString()),
		)
		return
	}
	node := config.Node.ValueString()
	storage := config.Storage.ValueString()
	template := config.Template.ValueString()
	progress := func(message string) {
		if resp.SendProgress != nil {
			resp.SendProgress(action.InvokeProgressEvent{Message: message})
		}
	}
	tflog.Info(ctx, "downloading appliance template", map[string]any{"node": node, "storage": storage, "template": template})
	upid, err := a.client.DownloadApplianceTemplate(ctx, node, storage, template)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error invoking pve_aplinfo_update",
			fmt.Sprintf("downloading appliance template %s to storage %s on node %s: %s", template, storage, node, err),
		)
		return
	}
	if err := vmWaitForTask(ctx, a.client, upid, progress); err != nil {
		resp.Diagnostics.AddError(
			"Error waiting for pve_aplinfo_update",
			fmt.Sprintf("waiting for appliance template %s download on node %s: %s", template, node, err),
		)
		return
	}
	progress(fmt.Sprintf("Appliance template %s stored on %s", template, storage))
}
