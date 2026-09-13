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
	_ resource.Resource                = &pveStorageIscsiResource{}
	_ resource.ResourceWithConfigure   = &pveStorageIscsiResource{}
	_ resource.ResourceWithImportState = &pveStorageIscsiResource{}
)

// NewPveStorageIscsiResource returns the resource implementation.
func NewPveStorageIscsiResource() resource.Resource {
	return &pveStorageIscsiResource{}
}

// pveStorageIscsiResource manages one iSCSI target storage definition
// (POST/GET/PUT/DELETE on /storage with type `iscsi`).
type pveStorageIscsiResource struct {
	client *pveclient.Client
}

// pveStorageIscsiResourceModel is the Terraform-facing shape.
type pveStorageIscsiResourceModel struct {
	storageFamilyCommonModel
	Portal        types.String `tfsdk:"portal"`
	Target        types.String `tfsdk:"target"`
	ISCSIProvider types.String `tfsdk:"iscsiprovider"`
}

// Metadata implements resource.Resource.
func (r *pveStorageIscsiResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveStorageIscsi
}

// Schema implements resource.Resource.
func (r *pveStorageIscsiResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := storageFamilyCommonAttributes("Allowed content types as a set (PVE `pve-storage-content-list`). iSCSI storages only support `images`. PVE validates the set upstream.")
	attrs["portal"] = schema.StringAttribute{
		Required:            true,
		MarkdownDescription: "iSCSI portal (IP or DNS name with optional port), for example `192.168.1.10:3260` (PVE `pve-storage-portal-dns`). The pin's update verb does not accept this field, so changing it forces recreation.",
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	}
	attrs["target"] = schema.StringAttribute{
		Required:            true,
		MarkdownDescription: "iSCSI target, for example `iqn.2000-01.com.example:san.target0`. The pin's update verb does not accept this field, so changing it forces recreation.",
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	}
	attrs["iscsiprovider"] = schema.StringAttribute{
		Optional:            true,
		MarkdownDescription: "The iSCSI provider, for example `LIO` (PVE `iscsiprovider`). The pin's update verb does not accept this field, so changing it forces recreation.",
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages an iSCSI target storage definition (`/storage`, type `iscsi`). iSCSI targets are used as disks for VMs only. Changing `storage`, `portal`, `target`, or `iscsiprovider` forces recreation. Deletion is synchronous per the pin (no task is spawned).",
		Attributes:          attrs,
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveStorageIscsiResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = haConfigureResource(req, resp)
}

// Create implements resource.Resource.
func (r *pveStorageIscsiResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveStorageIscsiResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error creating pve_storage_iscsi", "provider client is not configured")
		return
	}
	if err := r.client.CreateStorageNet(ctx, storageFamilyIscsiFromModel(plan)); err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_storage_iscsi",
			fmt.Sprintf("creating storage %s: %s", plan.Storage.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_storage_iscsi after create",
			fmt.Sprintf("reading storage %s: %s", plan.Storage.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	tflog.Debug(ctx, "created PVE iscsi storage", map[string]any{"storage": plan.Storage.ValueString()})
}

// Read implements resource.Resource.
func (r *pveStorageIscsiResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveStorageIscsiResourceModel
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
			"Error reading pve_storage_iscsi",
			fmt.Sprintf("reading storage %s: %s", state.Storage.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource. Set fields travel in the PUT body;
// attributes cleared in the plan travel in the `delete` query parameter.
func (r *pveStorageIscsiResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveStorageIscsiResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state pveStorageIscsiResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	deleteFields := storageFamilyCommonDeleteFields(plan.storageFamilyCommonModel, state.storageFamilyCommonModel)
	if err := r.client.UpdateStorageNet(ctx, plan.Storage.ValueString(), storageFamilyIscsiFromModel(plan), deleteFields); err != nil {
		resp.Diagnostics.AddError(
			"Error updating pve_storage_iscsi",
			fmt.Sprintf("updating storage %s: %s", plan.Storage.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_storage_iscsi after update",
			fmt.Sprintf("reading storage %s: %s", plan.Storage.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	tflog.Debug(ctx, "updated PVE iscsi storage", map[string]any{"storage": plan.Storage.ValueString()})
}

// Delete implements resource.Resource.
func (r *pveStorageIscsiResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveStorageIscsiResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	tflog.Debug(ctx, "deleting PVE iscsi storage", map[string]any{"storage": state.Storage.ValueString()})
	if err := r.client.DeleteStorageNet(ctx, state.Storage.ValueString()); err != nil {
		if isPVEClientNotFound(err) {
			// Already absent counts as deleted.
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting pve_storage_iscsi",
			fmt.Sprintf("deleting storage %s: %s", state.Storage.ValueString(), err),
		)
	}
}

// ImportState parses an import ID of the form `<storage>`.
func (r *pveStorageIscsiResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		resp.Diagnostics.AddError("Invalid pve_storage_iscsi import ID", "import ID must be the storage identifier, e.g. `san`")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("storage"), req.ID)...)
}

// readInto refreshes the model from PVE, erroring when the upstream type
// is no longer iscsi.
func (r *pveStorageIscsiResource) readInto(ctx context.Context, m *pveStorageIscsiResourceModel) error {
	s, err := storageFamilyGetChecked(ctx, r.client, m.Storage.ValueString(), "iscsi")
	if err != nil {
		return err
	}
	storageFamilyIscsiApply(s, m)
	return nil
}

// storageFamilyIscsiFromModel projects the Terraform model into the wire
// body; null values are omitted from the request.
func storageFamilyIscsiFromModel(m pveStorageIscsiResourceModel) pveclient.StorageNet {
	body := storageFamilyCommonFromModel(m.storageFamilyCommonModel)
	body.Type = "iscsi"
	if !m.Portal.IsNull() && !m.Portal.IsUnknown() {
		body.Portal = m.Portal.ValueString()
	}
	if !m.Target.IsNull() && !m.Target.IsUnknown() {
		body.Target = m.Target.ValueString()
	}
	if !m.ISCSIProvider.IsNull() && !m.ISCSIProvider.IsUnknown() {
		body.ISCSIProvider = m.ISCSIProvider.ValueString()
	}
	return body
}

// storageFamilyIscsiApply writes a fetched configuration into the model;
// absent settings become null.
func storageFamilyIscsiApply(s *pveclient.StorageNet, m *pveStorageIscsiResourceModel) {
	storageFamilyCommonApply(s, &m.storageFamilyCommonModel)
	m.Portal = nodeNetworkStringToTF(s.Portal)
	m.Target = nodeNetworkStringToTF(s.Target)
	m.ISCSIProvider = nodeNetworkStringToTF(s.ISCSIProvider)
}
