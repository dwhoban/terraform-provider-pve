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
	_ action.Action              = &pveStorageOciPullAction{}
	_ action.ActionWithConfigure = &pveStorageOciPullAction{}
)

// NewPveStorageOciPullAction returns the action implementation.
func NewPveStorageOciPullAction() action.Action {
	return &pveStorageOciPullAction{}
}

// pveStorageOciPullAction pulls an OCI image from a registry into a
// storage (POST /nodes/{node}/storage/{storage}/oci-registry-pull) and
// waits for the pull task to finish.
type pveStorageOciPullAction struct {
	client *pveclient.Client
}

// pveStorageOciPullActionModel is the Terraform-facing shape.
type pveStorageOciPullActionModel struct {
	Node      types.String `tfsdk:"node"`
	Storage   types.String `tfsdk:"storage"`
	Reference types.String `tfsdk:"reference"`
	Filename  types.String `tfsdk:"filename"`
}

// Metadata implements action.Action.
func (a *pveStorageOciPullAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveStorageOciPull
}

// Schema implements action.Action.
func (a *pveStorageOciPullAction) Schema(_ context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "Pulls an OCI image from a registry into a storage " +
			"(`POST /nodes/{node}/storage/{storage}/oci-registry-pull`) and waits for the pull task to finish. " +
			"Requires the `Datastore.AllocateTemplate` privilege on `/storage/{storage}` and `Sys.AccessNetwork` on " +
			"`/nodes/{node}`.",
		Attributes: map[string]actionschema.Attribute{
			"node": &actionschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The cluster node pulling the image; the task runs here.",
			},
			"storage": &actionschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The storage identifier to pull the image into.",
			},
			"reference": &actionschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Reference to the OCI image to download, e.g. `docker.io/library/alpine:3.20`.",
			},
			"filename": &actionschema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Custom destination file name of the OCI image; upstream normalizes it. Omit to let PVE derive the name from the reference.",
			},
		},
	}
}

// Configure implements action.ActionWithConfigure.
func (a *pveStorageOciPullAction) Configure(ctx context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	a.client = containerActionConfigure(req, resp)
}

// Invoke implements action.Action.
func (a *pveStorageOciPullAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var config pveStorageOciPullActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if a.client == nil {
		resp.Diagnostics.AddError(
			"Unconfigured pve_storage_oci_pull",
			fmt.Sprintf("The provider client was not configured; cannot pull OCI image %s.", config.Reference.ValueString()),
		)
		return
	}
	node := config.Node.ValueString()
	storage := config.Storage.ValueString()
	reference := config.Reference.ValueString()
	filename := ""
	if !config.Filename.IsNull() && !config.Filename.IsUnknown() {
		filename = config.Filename.ValueString()
	}
	progress := func(message string) {
		if resp.SendProgress != nil {
			resp.SendProgress(action.InvokeProgressEvent{Message: message})
		}
	}
	tflog.Info(ctx, "pulling OCI image", map[string]any{"node": node, "storage": storage, "reference": reference})
	upid, err := a.client.PullOCIImage(ctx, node, storage, reference, filename)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error invoking pve_storage_oci_pull",
			fmt.Sprintf("pulling OCI image %s into storage %s on node %s: %s", reference, storage, node, err),
		)
		return
	}
	if err := vmWaitForTask(ctx, a.client, upid, progress); err != nil {
		resp.Diagnostics.AddError(
			"Error waiting for pve_storage_oci_pull",
			fmt.Sprintf("waiting for OCI pull of %s into storage %s on node %s: %s", reference, storage, node, err),
		)
		return
	}
	progress(fmt.Sprintf("OCI image %s pulled into storage %s", reference, storage))
}
