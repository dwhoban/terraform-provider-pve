// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ resource.Resource                = &pveStorageCifsResource{}
	_ resource.ResourceWithConfigure   = &pveStorageCifsResource{}
	_ resource.ResourceWithImportState = &pveStorageCifsResource{}
)

// NewPveStorageCifsResource returns the resource implementation.
func NewPveStorageCifsResource() resource.Resource {
	return &pveStorageCifsResource{}
}

// pveStorageCifsResource manages one CIFS/SMB storage definition (POST/
// GET/PUT/DELETE on /storage with type `cifs`).
type pveStorageCifsResource struct {
	client *pveclient.Client
}

// pveStorageCifsResourceModel is the Terraform-facing shape.
type pveStorageCifsResourceModel struct {
	storageFamilyCommonModel
	Server     types.String `tfsdk:"server"`
	Share      types.String `tfsdk:"share"`
	Username   types.String `tfsdk:"username"`
	Password   types.String `tfsdk:"password"`
	Domain     types.String `tfsdk:"domain"`
	SMBVersion types.String `tfsdk:"smbversion"`
	Options    types.String `tfsdk:"options"`
}

// Metadata implements resource.Resource.
func (r *pveStorageCifsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveStorageCifs
}

// Schema implements resource.Resource.
func (r *pveStorageCifsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := storageFamilyCommonAttributes("Allowed content types as a set (PVE `pve-storage-content-list`). CIFS storages typically serve `backup`, `iso`, `vztmpl`, and `images`. PVE validates the set upstream.")
	attrs["server"] = schema.StringAttribute{
		Required:            true,
		MarkdownDescription: "Server IP or DNS name hosting the CIFS share.",
	}
	attrs["share"] = schema.StringAttribute{
		Required:            true,
		MarkdownDescription: "CIFS share name. The pin's update verb does not accept this field, so changing it forces recreation.",
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	}
	attrs["username"] = schema.StringAttribute{
		Optional:            true,
		MarkdownDescription: "User name used to access the share.",
	}
	attrs["password"] = schema.StringAttribute{
		Optional:            true,
		Sensitive:           true,
		MarkdownDescription: "Password for accessing the share. PVE never returns the password on reads.",
	}
	attrs["domain"] = schema.StringAttribute{
		Optional:            true,
		MarkdownDescription: "CIFS domain (PVE `domain`).",
	}
	attrs["smbversion"] = schema.StringAttribute{
		Optional:            true,
		MarkdownDescription: "SMB protocol version (PVE `smbversion`). `default` negotiates the highest SMB2+ version supported by both the client and server. Must be one of: `default`, `2.0`, `2.1`, `3`, `3.0`, `3.11`.",
		Validators: []validator.String{
			stringvalidator.OneOf("default", "2.0", "2.1", "3", "3.0", "3.11"),
		},
	}
	attrs["options"] = schema.StringAttribute{
		Optional:            true,
		MarkdownDescription: "NFS/CIFS mount options (see `man mount.cifs`) (PVE `pve-storage-options`).",
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a CIFS storage definition (`/storage`, type `cifs`). Changing `storage` or `share` forces recreation. Deletion is synchronous per the pin (no task is spawned).",
		Attributes:          attrs,
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveStorageCifsResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = haConfigureResource(req, resp)
}

// Create implements resource.Resource.
func (r *pveStorageCifsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveStorageCifsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error creating pve_storage_cifs", "provider client is not configured")
		return
	}
	if err := r.client.CreateStorageNet(ctx, storageFamilyCifsFromModel(plan)); err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_storage_cifs",
			fmt.Sprintf("creating storage %s: %s", plan.Storage.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_storage_cifs after create",
			fmt.Sprintf("reading storage %s: %s", plan.Storage.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	tflog.Debug(ctx, "created PVE cifs storage", map[string]any{"storage": plan.Storage.ValueString()})
}

// Read implements resource.Resource.
func (r *pveStorageCifsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveStorageCifsResourceModel
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
			"Error reading pve_storage_cifs",
			fmt.Sprintf("reading storage %s: %s", state.Storage.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource. Set fields travel in the PUT body;
// attributes cleared in the plan travel in the `delete` query parameter.
func (r *pveStorageCifsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveStorageCifsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state pveStorageCifsResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	deleteFields := append(
		storageFamilyCommonDeleteFields(plan.storageFamilyCommonModel, state.storageFamilyCommonModel),
		storageFamilyCifsDeleteFields(plan, state)...,
	)
	if err := r.client.UpdateStorageNet(ctx, plan.Storage.ValueString(), storageFamilyCifsFromModel(plan), deleteFields); err != nil {
		resp.Diagnostics.AddError(
			"Error updating pve_storage_cifs",
			fmt.Sprintf("updating storage %s: %s", plan.Storage.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_storage_cifs after update",
			fmt.Sprintf("reading storage %s: %s", plan.Storage.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	tflog.Debug(ctx, "updated PVE cifs storage", map[string]any{"storage": plan.Storage.ValueString()})
}

// Delete implements resource.Resource.
func (r *pveStorageCifsResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveStorageCifsResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	tflog.Debug(ctx, "deleting PVE cifs storage", map[string]any{"storage": state.Storage.ValueString()})
	if err := r.client.DeleteStorageNet(ctx, state.Storage.ValueString()); err != nil {
		if isPVEClientNotFound(err) {
			// Already absent counts as deleted.
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting pve_storage_cifs",
			fmt.Sprintf("deleting storage %s: %s", state.Storage.ValueString(), err),
		)
	}
}

// ImportState parses an import ID of the form `<storage>`.
func (r *pveStorageCifsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		resp.Diagnostics.AddError("Invalid pve_storage_cifs import ID", "import ID must be the storage identifier, e.g. `archive`")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("storage"), req.ID)...)
}

// readInto refreshes the model from PVE, erroring when the upstream type
// is no longer cifs.
func (r *pveStorageCifsResource) readInto(ctx context.Context, m *pveStorageCifsResourceModel) error {
	s, err := storageFamilyGetChecked(ctx, r.client, m.Storage.ValueString(), "cifs")
	if err != nil {
		return err
	}
	storageFamilyCifsApply(s, m)
	return nil
}

// storageFamilyCifsFromModel projects the Terraform model into the wire
// body; null values are omitted from the request.
func storageFamilyCifsFromModel(m pveStorageCifsResourceModel) pveclient.StorageNet {
	body := storageFamilyCommonFromModel(m.storageFamilyCommonModel)
	body.Type = "cifs"
	if !m.Server.IsNull() && !m.Server.IsUnknown() {
		body.Server = m.Server.ValueString()
	}
	if !m.Share.IsNull() && !m.Share.IsUnknown() {
		body.Share = m.Share.ValueString()
	}
	if !m.Username.IsNull() && !m.Username.IsUnknown() {
		body.Username = m.Username.ValueString()
	}
	if !m.Password.IsNull() && !m.Password.IsUnknown() {
		body.Password = m.Password.ValueString()
	}
	if !m.Domain.IsNull() && !m.Domain.IsUnknown() {
		body.Domain = m.Domain.ValueString()
	}
	if !m.SMBVersion.IsNull() && !m.SMBVersion.IsUnknown() {
		body.SMBVersion = m.SMBVersion.ValueString()
	}
	if !m.Options.IsNull() && !m.Options.IsUnknown() {
		body.Options = m.Options.ValueString()
	}
	return body
}

// storageFamilyCifsApply writes a fetched configuration into the model;
// absent settings become null. PVE never returns the password on reads, so
// the field becomes null after every refresh.
func storageFamilyCifsApply(s *pveclient.StorageNet, m *pveStorageCifsResourceModel) {
	storageFamilyCommonApply(s, &m.storageFamilyCommonModel)
	m.Server = nodeNetworkStringToTF(s.Server)
	m.Share = nodeNetworkStringToTF(s.Share)
	m.Username = nodeNetworkStringToTF(s.Username)
	m.Password = nodeNetworkStringToTF(s.Password)
	m.Domain = nodeNetworkStringToTF(s.Domain)
	m.SMBVersion = nodeNetworkStringToTF(s.SMBVersion)
	m.Options = nodeNetworkStringToTF(s.Options)
}

// storageFamilyCifsDeleteFields returns the PVE wire field names to clear
// on update: attributes present in state but null in the plan.
func storageFamilyCifsDeleteFields(plan, state pveStorageCifsResourceModel) []string {
	var out []string
	if nodeNetworkStringCleared(plan.Username, state.Username) {
		out = append(out, "username")
	}
	if nodeNetworkStringCleared(plan.Password, state.Password) {
		out = append(out, "password")
	}
	if nodeNetworkStringCleared(plan.Domain, state.Domain) {
		out = append(out, "domain")
	}
	if nodeNetworkStringCleared(plan.SMBVersion, state.SMBVersion) {
		out = append(out, "smbversion")
	}
	if nodeNetworkStringCleared(plan.Options, state.Options) {
		out = append(out, "options")
	}
	return out
}
