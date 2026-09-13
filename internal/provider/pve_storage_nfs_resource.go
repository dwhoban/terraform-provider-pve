// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ resource.Resource                = &pveStorageNfsResource{}
	_ resource.ResourceWithConfigure   = &pveStorageNfsResource{}
	_ resource.ResourceWithImportState = &pveStorageNfsResource{}
)

// NewPveStorageNfsResource returns the resource implementation.
func NewPveStorageNfsResource() resource.Resource {
	return &pveStorageNfsResource{}
}

// pveStorageNfsResource manages one NFS storage definition (POST/GET/PUT/
// DELETE on /storage with type `nfs`).
type pveStorageNfsResource struct {
	client *pveclient.Client
}

// pveStorageNfsResourceModel is the Terraform-facing shape.
type pveStorageNfsResourceModel struct {
	storageFamilyCommonModel
	Server  types.String `tfsdk:"server"`
	Export  types.String `tfsdk:"export"`
	Options types.String `tfsdk:"options"`
}

// Metadata implements resource.Resource.
func (r *pveStorageNfsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveStorageNfs
}

// Schema implements resource.Resource.
func (r *pveStorageNfsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := storageFamilyCommonAttributes("Allowed content types as a set (PVE `pve-storage-content-list`). NFS storages typically serve `images`, `iso`, `vztmpl`, and `backup`. PVE validates the set upstream.")
	attrs["server"] = schema.StringAttribute{
		Required:            true,
		MarkdownDescription: "Server IP or DNS name hosting the NFS export.",
	}
	attrs["export"] = schema.StringAttribute{
		Required:            true,
		MarkdownDescription: "NFS export path on the server (PVE `pve-storage-path` format). The pin's update verb does not accept this field, so changing it forces recreation.",
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	}
	attrs["options"] = schema.StringAttribute{
		Optional:            true,
		MarkdownDescription: "NFS/CIFS mount options (see `man nfs`), for example `vers=4.2,soft` (PVE `pve-storage-options`).",
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages an NFS storage definition (`/storage`, type `nfs`). Changing `storage` or `export` forces recreation. Deletion is synchronous per the pin (no task is spawned).",
		Attributes:          attrs,
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveStorageNfsResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = haConfigureResource(req, resp)
}

// Create implements resource.Resource.
func (r *pveStorageNfsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveStorageNfsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error creating pve_storage_nfs", "provider client is not configured")
		return
	}
	if err := r.client.CreateStorageNet(ctx, storageFamilyNfsFromModel(plan)); err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_storage_nfs",
			fmt.Sprintf("creating storage %s: %s", plan.Storage.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_storage_nfs after create",
			fmt.Sprintf("reading storage %s: %s", plan.Storage.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	tflog.Debug(ctx, "created PVE nfs storage", map[string]any{"storage": plan.Storage.ValueString()})
}

// Read implements resource.Resource.
func (r *pveStorageNfsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveStorageNfsResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.readInto(ctx, &state); err != nil {
		if isPVEClientNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error reading pve_storage_nfs",
			fmt.Sprintf("reading storage %s: %s", state.Storage.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource. Set fields travel in the PUT body;
// attributes cleared in the plan travel in the `delete` query parameter.
func (r *pveStorageNfsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveStorageNfsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state pveStorageNfsResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	deleteFields := append(
		storageFamilyCommonDeleteFields(plan.storageFamilyCommonModel, state.storageFamilyCommonModel),
		storageFamilyNfsDeleteFields(plan, state)...,
	)
	if err := r.client.UpdateStorageNet(ctx, plan.Storage.ValueString(), storageFamilyNfsFromModel(plan), deleteFields); err != nil {
		resp.Diagnostics.AddError(
			"Error updating pve_storage_nfs",
			fmt.Sprintf("updating storage %s: %s", plan.Storage.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_storage_nfs after update",
			fmt.Sprintf("reading storage %s: %s", plan.Storage.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	tflog.Debug(ctx, "updated PVE nfs storage", map[string]any{"storage": plan.Storage.ValueString()})
}

// Delete implements resource.Resource.
func (r *pveStorageNfsResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveStorageNfsResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	tflog.Debug(ctx, "deleting PVE nfs storage", map[string]any{"storage": state.Storage.ValueString()})
	if err := r.client.DeleteStorageNet(ctx, state.Storage.ValueString()); err != nil {
		if isPVEClientNotFound(err) {
			// Already absent counts as deleted.
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting pve_storage_nfs",
			fmt.Sprintf("deleting storage %s: %s", state.Storage.ValueString(), err),
		)
	}
}

// ImportState parses an import ID of the form `<storage>`.
func (r *pveStorageNfsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		resp.Diagnostics.AddError("Invalid pve_storage_nfs import ID", "import ID must be the storage identifier, e.g. `media`")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("storage"), req.ID)...)
}

// readInto refreshes the model from PVE, erroring when the upstream type
// is no longer nfs.
func (r *pveStorageNfsResource) readInto(ctx context.Context, m *pveStorageNfsResourceModel) error {
	s, err := storageFamilyGetChecked(ctx, r.client, m.Storage.ValueString(), "nfs")
	if err != nil {
		return err
	}
	storageFamilyNfsApply(s, m)
	return nil
}

// storageFamilyNfsFromModel projects the Terraform model into the wire
// body; null values are omitted from the request.
func storageFamilyNfsFromModel(m pveStorageNfsResourceModel) pveclient.StorageNet {
	body := storageFamilyCommonFromModel(m.storageFamilyCommonModel)
	body.Type = "nfs"
	if !m.Server.IsNull() && !m.Server.IsUnknown() {
		body.Server = m.Server.ValueString()
	}
	if !m.Export.IsNull() && !m.Export.IsUnknown() {
		body.Export = m.Export.ValueString()
	}
	if !m.Options.IsNull() && !m.Options.IsUnknown() {
		body.Options = m.Options.ValueString()
	}
	return body
}

// storageFamilyNfsApply writes a fetched configuration into the model;
// absent settings become null.
func storageFamilyNfsApply(s *pveclient.StorageNet, m *pveStorageNfsResourceModel) {
	storageFamilyCommonApply(s, &m.storageFamilyCommonModel)
	m.Server = nodeNetworkStringToTF(s.Server)
	m.Export = nodeNetworkStringToTF(s.Export)
	m.Options = nodeNetworkStringToTF(s.Options)
}

// storageFamilyNfsDeleteFields returns the PVE wire field names to clear
// on update: attributes present in state but null in the plan.
func storageFamilyNfsDeleteFields(plan, state pveStorageNfsResourceModel) []string {
	var out []string
	if nodeNetworkStringCleared(plan.Options, state.Options) {
		out = append(out, "options")
	}
	return out
}
