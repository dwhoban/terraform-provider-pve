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
	_ resource.Resource                = &pveStorageIscsidirectResource{}
	_ resource.ResourceWithConfigure   = &pveStorageIscsidirectResource{}
	_ resource.ResourceWithImportState = &pveStorageIscsidirectResource{}
)

// NewPveStorageIscsidirectResource returns the resource implementation.
func NewPveStorageIscsidirectResource() resource.Resource {
	return &pveStorageIscsidirectResource{}
}

// pveStorageIscsidirectResource manages one iSCSI direct-attached storage
// definition (POST/GET/PUT/DELETE on /storage with type `iscsidirect`).
type pveStorageIscsidirectResource struct {
	client *pveclient.Client
}

// pveStorageIscsidirectResourceModel is the Terraform-facing shape.
type pveStorageIscsidirectResourceModel struct {
	storageFamilyCommonModel
	Portal       types.String `tfsdk:"portal"`
	Target       types.String `tfsdk:"target"`
	NoWriteCache types.Bool   `tfsdk:"nowritecache"`
}

// Metadata implements resource.Resource.
func (r *pveStorageIscsidirectResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveStorageIscsidirect
}

// Schema implements resource.Resource.
func (r *pveStorageIscsidirectResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := storageFamilyCommonAttributes("Allowed content types as a set (PVE `pve-storage-content-list`). iSCSI direct storages only support `images`. PVE validates the set upstream.")
	attrs["portal"] = schema.StringAttribute{
		Required:            true,
		MarkdownDescription: "iSCSI portal (IP or DNS name with optional port), for example `192.168.1.10:3260` (PVE `pve-storage-portal-dns`). The pin's update verb does not accept this field, so changing it forces recreation.",
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	}
	attrs["target"] = schema.StringAttribute{
		Required:            true,
		MarkdownDescription: "iSCSI target, for example `iqn.2000-01.com.example:fast.target0`. The pin's update verb does not accept this field, so changing it forces recreation.",
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	}
	attrs["nowritecache"] = schema.BoolAttribute{
		Optional:            true,
		MarkdownDescription: "Disable write caching on the target (PVE `nowritecache`).",
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages an iSCSI direct-attached storage definition (`/storage`, type `iscsidirect`). Volumes are shared block devices; no snapshot or cloning is possible. Changing `storage`, `portal`, or `target` forces recreation. Deletion is synchronous per the pin (no task is spawned).",
		Attributes:          attrs,
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveStorageIscsidirectResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = haConfigureResource(req, resp)
}

// Create implements resource.Resource.
func (r *pveStorageIscsidirectResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveStorageIscsidirectResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error creating pve_storage_iscsidirect", "provider client is not configured")
		return
	}
	if err := r.client.CreateStorageNet(ctx, storageFamilyIscsidirectFromModel(plan)); err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_storage_iscsidirect",
			fmt.Sprintf("creating storage %s: %s", plan.Storage.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_storage_iscsidirect after create",
			fmt.Sprintf("reading storage %s: %s", plan.Storage.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	tflog.Debug(ctx, "created PVE iscsidirect storage", map[string]any{"storage": plan.Storage.ValueString()})
}

// Read implements resource.Resource.
func (r *pveStorageIscsidirectResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveStorageIscsidirectResourceModel
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
			"Error reading pve_storage_iscsidirect",
			fmt.Sprintf("reading storage %s: %s", state.Storage.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource. Set fields travel in the PUT body;
// attributes cleared in the plan travel in the `delete` query parameter.
func (r *pveStorageIscsidirectResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveStorageIscsidirectResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state pveStorageIscsidirectResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	deleteFields := append(
		storageFamilyCommonDeleteFields(plan.storageFamilyCommonModel, state.storageFamilyCommonModel),
		storageFamilyIscsidirectDeleteFields(plan, state)...,
	)
	if err := r.client.UpdateStorageNet(ctx, plan.Storage.ValueString(), storageFamilyIscsidirectFromModel(plan), deleteFields); err != nil {
		resp.Diagnostics.AddError(
			"Error updating pve_storage_iscsidirect",
			fmt.Sprintf("updating storage %s: %s", plan.Storage.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_storage_iscsidirect after update",
			fmt.Sprintf("reading storage %s: %s", plan.Storage.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	tflog.Debug(ctx, "updated PVE iscsidirect storage", map[string]any{"storage": plan.Storage.ValueString()})
}

// Delete implements resource.Resource.
func (r *pveStorageIscsidirectResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveStorageIscsidirectResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	tflog.Debug(ctx, "deleting PVE iscsidirect storage", map[string]any{"storage": state.Storage.ValueString()})
	if err := r.client.DeleteStorageNet(ctx, state.Storage.ValueString()); err != nil {
		if isPVEClientNotFound(err) {
			// Already absent counts as deleted.
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting pve_storage_iscsidirect",
			fmt.Sprintf("deleting storage %s: %s", state.Storage.ValueString(), err),
		)
	}
}

// ImportState parses an import ID of the form `<storage>`.
func (r *pveStorageIscsidirectResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		resp.Diagnostics.AddError("Invalid pve_storage_iscsidirect import ID", "import ID must be the storage identifier, e.g. `fast`")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("storage"), req.ID)...)
}

// readInto refreshes the model from PVE, erroring when the upstream type
// is no longer iscsidirect.
func (r *pveStorageIscsidirectResource) readInto(ctx context.Context, m *pveStorageIscsidirectResourceModel) error {
	s, err := storageFamilyGetChecked(ctx, r.client, m.Storage.ValueString(), "iscsidirect")
	if err != nil {
		return err
	}
	storageFamilyIscsidirectApply(s, m)
	return nil
}

// storageFamilyIscsidirectFromModel projects the Terraform model into the
// wire body; null values are omitted from the request.
func storageFamilyIscsidirectFromModel(m pveStorageIscsidirectResourceModel) pveclient.StorageNet {
	body := storageFamilyCommonFromModel(m.storageFamilyCommonModel)
	body.Type = "iscsidirect"
	if !m.Portal.IsNull() && !m.Portal.IsUnknown() {
		body.Portal = m.Portal.ValueString()
	}
	if !m.Target.IsNull() && !m.Target.IsUnknown() {
		body.Target = m.Target.ValueString()
	}
	if !m.NoWriteCache.IsNull() && !m.NoWriteCache.IsUnknown() {
		v := m.NoWriteCache.ValueBool()
		body.NoWriteCache = &v
	}
	return body
}

// storageFamilyIscsidirectApply writes a fetched configuration into the
// model; absent settings become null.
func storageFamilyIscsidirectApply(s *pveclient.StorageNet, m *pveStorageIscsidirectResourceModel) {
	storageFamilyCommonApply(s, &m.storageFamilyCommonModel)
	m.Portal = nodeNetworkStringToTF(s.Portal)
	m.Target = nodeNetworkStringToTF(s.Target)
	m.NoWriteCache = nodeNetworkBoolPtrToTF(s.NoWriteCache)
}

// storageFamilyIscsidirectDeleteFields returns the PVE wire field names to
// clear on update: attributes present in state but null in the plan.
func storageFamilyIscsidirectDeleteFields(plan, state pveStorageIscsidirectResourceModel) []string {
	var out []string
	if plan.NoWriteCache.IsNull() && !state.NoWriteCache.IsNull() {
		out = append(out, "nowritecache")
	}
	return out
}
