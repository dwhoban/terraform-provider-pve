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
	_ resource.Resource                = &pveStorageCephfsResource{}
	_ resource.ResourceWithConfigure   = &pveStorageCephfsResource{}
	_ resource.ResourceWithImportState = &pveStorageCephfsResource{}
)

// NewPveStorageCephfsResource returns the resource implementation.
func NewPveStorageCephfsResource() resource.Resource {
	return &pveStorageCephfsResource{}
}

// pveStorageCephfsResource manages one CephFS storage definition
// (POST/GET/PUT/DELETE on /storage with type `cephfs`).
type pveStorageCephfsResource struct {
	client *pveclient.Client
}

// pveStorageCephfsResourceModel is the Terraform-facing shape.
type pveStorageCephfsResourceModel struct {
	Storage  types.String `tfsdk:"storage"`
	Content  types.Set    `tfsdk:"content"`
	Disable  types.Bool   `tfsdk:"disable"`
	Nodes    types.String `tfsdk:"nodes"`
	Monhost  types.String `tfsdk:"monhost"`
	FsName   types.String `tfsdk:"fs_name"`
	Subdir   types.String `tfsdk:"subdir"`
	Path     types.String `tfsdk:"path"`
	Fuse     types.Bool   `tfsdk:"fuse"`
	Username types.String `tfsdk:"username"`
	Keyring  types.String `tfsdk:"keyring"`
	Digest   types.String `tfsdk:"digest"`
}

// Metadata implements resource.Resource.
func (r *pveStorageCephfsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveStorageCephfs
}

// Schema implements resource.Resource.
func (r *pveStorageCephfsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a CephFS storage definition (`/storage`, type `cephfs`). Changing `storage` forces recreation. Deletion is synchronous per the pin (no task is spawned).",
		Attributes: map[string]schema.Attribute{
			"storage": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The storage identifier (PVE `pve-storage-id` format). Changing this value forces recreation.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"content": schema.SetAttribute{
				ElementType:         types.StringType,
				Optional:            true,
				MarkdownDescription: "Allowed content types as a set (PVE `pve-storage-content-list`), for example `images` or `rootdir`. PVE validates the set upstream.",
			},
			"disable": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Flag to disable the storage.",
			},
			"nodes": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Comma-separated list of cluster node names the storage configuration applies to (PVE `pve-node-list`).",
			},
			"monhost": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Comma-separated list of monitor addresses (IPs or DNS names, optionally with ports) of the Ceph cluster (PVE `pve-storage-portal-dns-list`).",
			},
			"fs_name": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The Ceph filesystem name (PVE `fs-name`).",
			},
			"subdir": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Subdirectory to mount (PVE `pve-storage-path`).",
			},
			"path": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "File system path where the CephFS is mounted (PVE `pve-storage-path`).",
			},
			"fuse": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Mount CephFS through FUSE instead of the kernel client.",
			},
			"username": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The Ceph user name used to authenticate against the cluster.",
			},
			"keyring": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "Client keyring contents for external clusters.",
			},
			"digest": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The read-only storage configuration revision (PVE `digest`).",
			},
		},
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveStorageCephfsResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = haConfigureResource(req, resp)
}

// Create implements resource.Resource.
func (r *pveStorageCephfsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveStorageCephfsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error creating pve_storage_cephfs", "provider client is not configured")
		return
	}
	if err := r.client.CreateStorageRemote(ctx, storageRemoteCephfsFromModel(plan)); err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_storage_cephfs",
			fmt.Sprintf("creating storage %s: %s", plan.Storage.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_storage_cephfs after create",
			fmt.Sprintf("reading storage %s: %s", plan.Storage.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	tflog.Debug(ctx, "created PVE cephfs storage", map[string]any{"storage": plan.Storage.ValueString()})
}

// Read implements resource.Resource.
func (r *pveStorageCephfsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveStorageCephfsResourceModel
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
			"Error reading pve_storage_cephfs",
			fmt.Sprintf("reading storage %s: %s", state.Storage.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource. Set fields travel in the PUT body;
// attributes cleared in the plan travel in the `delete` query parameter.
func (r *pveStorageCephfsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveStorageCephfsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state pveStorageCephfsResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	deleteFields := storageRemoteCephfsDeleteFields(plan, state)
	if err := r.client.UpdateStorageRemote(ctx, plan.Storage.ValueString(), storageRemoteCephfsFromModel(plan), deleteFields); err != nil {
		resp.Diagnostics.AddError(
			"Error updating pve_storage_cephfs",
			fmt.Sprintf("updating storage %s: %s", plan.Storage.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_storage_cephfs after update",
			fmt.Sprintf("reading storage %s: %s", plan.Storage.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	tflog.Debug(ctx, "updated PVE cephfs storage", map[string]any{"storage": plan.Storage.ValueString()})
}

// Delete implements resource.Resource.
func (r *pveStorageCephfsResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveStorageCephfsResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	tflog.Debug(ctx, "deleting PVE cephfs storage", map[string]any{"storage": state.Storage.ValueString()})
	if err := r.client.DeleteStorageRemote(ctx, state.Storage.ValueString()); err != nil {
		if isPVEClientNotFound(err) {
			// Already absent counts as deleted.
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting pve_storage_cephfs",
			fmt.Sprintf("deleting storage %s: %s", state.Storage.ValueString(), err),
		)
	}
}

// ImportState parses an import ID of the form `<storage>`.
func (r *pveStorageCephfsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		resp.Diagnostics.AddError("Invalid pve_storage_cephfs import ID", "import ID must be the storage identifier, e.g. `cephfs1`")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("storage"), req.ID)...)
}

// readInto refreshes the model from PVE, erroring when the upstream type
// is no longer cephfs.
func (r *pveStorageCephfsResource) readInto(ctx context.Context, m *pveStorageCephfsResourceModel) error {
	s, err := r.client.GetStorageRemote(ctx, m.Storage.ValueString())
	if err != nil {
		return err
	}
	if s.Type != "cephfs" {
		return storageRemoteTypeMismatch(s.Storage, "cephfs", s.Type)
	}
	storageRemoteCephfsApply(s, m)
	return nil
}

// storageRemoteCephfsFromModel projects the Terraform model into the wire
// body; null values are omitted from the request.
func storageRemoteCephfsFromModel(m pveStorageCephfsResourceModel) pveclient.StorageRemote {
	return pveclient.StorageRemote{
		Storage:  m.Storage.ValueString(),
		Type:     "cephfs",
		Content:  storageRemoteJoinContent(storageRemoteSetToStrings(m.Content)),
		Disable:  metricsServerBoolPtr(m.Disable),
		Nodes:    m.Nodes.ValueString(),
		Monhost:  m.Monhost.ValueString(),
		FsName:   m.FsName.ValueString(),
		Subdir:   m.Subdir.ValueString(),
		Path:     m.Path.ValueString(),
		Fuse:     metricsServerBoolPtr(m.Fuse),
		Username: m.Username.ValueString(),
		Keyring:  m.Keyring.ValueString(),
	}
}

// storageRemoteCephfsApply writes a fetched configuration into the model;
// absent settings become null.
func storageRemoteCephfsApply(s *pveclient.StorageRemote, m *pveStorageCephfsResourceModel) {
	m.Content = storageRemoteStringsToSet(storageRemoteSplitContent(s.Content))
	m.Disable = nodeNetworkBoolPtrToTF(s.Disable)
	m.Nodes = nodeNetworkStringToTF(s.Nodes)
	m.Monhost = nodeNetworkStringToTF(s.Monhost)
	m.FsName = nodeNetworkStringToTF(s.FsName)
	m.Subdir = nodeNetworkStringToTF(s.Subdir)
	m.Path = nodeNetworkStringToTF(s.Path)
	m.Fuse = nodeNetworkBoolPtrToTF(s.Fuse)
	m.Username = nodeNetworkStringToTF(s.Username)
	m.Keyring = nodeNetworkStringToTF(s.Keyring)
	m.Digest = nodeNetworkStringToTF(s.Digest)
}

// storageRemoteCephfsDeleteFields returns the PVE wire field names to
// clear on update: attributes present in state but null in the plan.
func storageRemoteCephfsDeleteFields(plan, state pveStorageCephfsResourceModel) []string {
	var out []string
	if plan.Disable.IsNull() && !state.Disable.IsNull() {
		out = append(out, "disable")
	}
	for _, f := range []struct {
		plan  types.String
		state types.String
		wire  string
	}{
		{plan.Nodes, state.Nodes, "nodes"},
		{plan.Monhost, state.Monhost, "monhost"},
		{plan.FsName, state.FsName, "fs-name"},
		{plan.Subdir, state.Subdir, "subdir"},
		{plan.Path, state.Path, "path"},
		{plan.Username, state.Username, "username"},
		{plan.Keyring, state.Keyring, "keyring"},
	} {
		if f.plan.IsNull() && !f.state.IsNull() {
			out = append(out, f.wire)
		}
	}
	if plan.Fuse.IsNull() && !state.Fuse.IsNull() {
		out = append(out, "fuse")
	}
	if len(storageRemoteSetToStrings(plan.Content)) == 0 && len(storageRemoteSetToStrings(state.Content)) > 0 {
		out = append(out, "content")
	}
	return out
}
