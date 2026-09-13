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
	_ resource.Resource                = &pveStorageRbdResource{}
	_ resource.ResourceWithConfigure   = &pveStorageRbdResource{}
	_ resource.ResourceWithImportState = &pveStorageRbdResource{}
)

// NewPveStorageRbdResource returns the resource implementation.
func NewPveStorageRbdResource() resource.Resource {
	return &pveStorageRbdResource{}
}

// pveStorageRbdResource manages one RBD (Ceph block device) storage
// definition (POST/GET/PUT/DELETE on /storage with type `rbd`).
type pveStorageRbdResource struct {
	client *pveclient.Client
}

// pveStorageRbdResourceModel is the Terraform-facing shape.
type pveStorageRbdResourceModel struct {
	Storage       types.String `tfsdk:"storage"`
	Content       types.Set    `tfsdk:"content"`
	Disable       types.Bool   `tfsdk:"disable"`
	Nodes         types.String `tfsdk:"nodes"`
	Monhost       types.String `tfsdk:"monhost"`
	Pool          types.String `tfsdk:"pool"`
	Namespace     types.String `tfsdk:"namespace"`
	DataPool      types.String `tfsdk:"data_pool"`
	Username      types.String `tfsdk:"username"`
	Authsupported types.String `tfsdk:"authsupported"`
	Keyring       types.String `tfsdk:"keyring"`
	KRBD          types.Bool   `tfsdk:"krbd"`
	Digest        types.String `tfsdk:"digest"`
}

// Metadata implements resource.Resource.
func (r *pveStorageRbdResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveStorageRbd
}

// Schema implements resource.Resource.
func (r *pveStorageRbdResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Ceph RBD storage definition (`/storage`, type `rbd`). Changing `storage` forces recreation. Deletion is synchronous per the pin (no task is spawned).",
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
				MarkdownDescription: "Comma-separated list of monitor addresses (IPs or DNS names, optionally with ports) of the Ceph cluster (PVE `pve-storage-portal-dns-list`). Required for external clusters.",
			},
			"pool": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The Ceph pool name.",
			},
			"namespace": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The RBD namespace.",
			},
			"data_pool": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The data pool used for erasure coding (PVE `data-pool`).",
			},
			"username": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The RBD Id (Ceph user name) used to authenticate against the cluster.",
			},
			"authsupported": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The supported authentication modes for the cluster, for example `cephx` or `none`.",
			},
			"keyring": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "Client keyring contents for external clusters.",
			},
			"krbd": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Always access RBD through the `krbd` kernel module (PVE `krbd`, PVE default: `false`).",
			},
			"digest": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The read-only storage configuration revision (PVE `digest`).",
			},
		},
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveStorageRbdResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = haConfigureResource(req, resp)
}

// Create implements resource.Resource.
func (r *pveStorageRbdResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveStorageRbdResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error creating pve_storage_rbd", "provider client is not configured")
		return
	}
	if err := r.client.CreateStorageRemote(ctx, storageRemoteRbdFromModel(plan)); err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_storage_rbd",
			fmt.Sprintf("creating storage %s: %s", plan.Storage.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_storage_rbd after create",
			fmt.Sprintf("reading storage %s: %s", plan.Storage.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	tflog.Debug(ctx, "created PVE rbd storage", map[string]any{"storage": plan.Storage.ValueString()})
}

// Read implements resource.Resource.
func (r *pveStorageRbdResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveStorageRbdResourceModel
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
			"Error reading pve_storage_rbd",
			fmt.Sprintf("reading storage %s: %s", state.Storage.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource. Set fields travel in the PUT body;
// attributes cleared in the plan travel in the `delete` query parameter.
func (r *pveStorageRbdResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveStorageRbdResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state pveStorageRbdResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	deleteFields := storageRemoteRbdDeleteFields(plan, state)
	if err := r.client.UpdateStorageRemote(ctx, plan.Storage.ValueString(), storageRemoteRbdFromModel(plan), deleteFields); err != nil {
		resp.Diagnostics.AddError(
			"Error updating pve_storage_rbd",
			fmt.Sprintf("updating storage %s: %s", plan.Storage.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_storage_rbd after update",
			fmt.Sprintf("reading storage %s: %s", plan.Storage.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	tflog.Debug(ctx, "updated PVE rbd storage", map[string]any{"storage": plan.Storage.ValueString()})
}

// Delete implements resource.Resource.
func (r *pveStorageRbdResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveStorageRbdResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	tflog.Debug(ctx, "deleting PVE rbd storage", map[string]any{"storage": state.Storage.ValueString()})
	if err := r.client.DeleteStorageRemote(ctx, state.Storage.ValueString()); err != nil {
		if isPVEClientNotFound(err) {
			// Already absent counts as deleted.
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting pve_storage_rbd",
			fmt.Sprintf("deleting storage %s: %s", state.Storage.ValueString(), err),
		)
	}
}

// ImportState parses an import ID of the form `<storage>`.
func (r *pveStorageRbdResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		resp.Diagnostics.AddError("Invalid pve_storage_rbd import ID", "import ID must be the storage identifier, e.g. `rbd1`")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("storage"), req.ID)...)
}

// readInto refreshes the model from PVE, erroring when the upstream type
// is no longer rbd.
func (r *pveStorageRbdResource) readInto(ctx context.Context, m *pveStorageRbdResourceModel) error {
	s, err := r.client.GetStorageRemote(ctx, m.Storage.ValueString())
	if err != nil {
		return err
	}
	if s.Type != "rbd" {
		return storageRemoteTypeMismatch(s.Storage, "rbd", s.Type)
	}
	storageRemoteRbdApply(s, m)
	return nil
}

// storageRemoteRbdFromModel projects the Terraform model into the wire
// body; null values are omitted from the request.
func storageRemoteRbdFromModel(m pveStorageRbdResourceModel) pveclient.StorageRemote {
	return pveclient.StorageRemote{
		Storage:       m.Storage.ValueString(),
		Type:          "rbd",
		Content:       storageRemoteJoinContent(storageRemoteSetToStrings(m.Content)),
		Disable:       metricsServerBoolPtr(m.Disable),
		Nodes:         m.Nodes.ValueString(),
		Monhost:       m.Monhost.ValueString(),
		Pool:          m.Pool.ValueString(),
		Namespace:     m.Namespace.ValueString(),
		DataPool:      m.DataPool.ValueString(),
		Username:      m.Username.ValueString(),
		Authsupported: m.Authsupported.ValueString(),
		Keyring:       m.Keyring.ValueString(),
		KRBD:          metricsServerBoolPtr(m.KRBD),
	}
}

// storageRemoteRbdApply writes a fetched configuration into the model;
// absent settings become null.
func storageRemoteRbdApply(s *pveclient.StorageRemote, m *pveStorageRbdResourceModel) {
	m.Content = storageRemoteStringsToSet(storageRemoteSplitContent(s.Content))
	m.Disable = nodeNetworkBoolPtrToTF(s.Disable)
	m.Nodes = nodeNetworkStringToTF(s.Nodes)
	m.Monhost = nodeNetworkStringToTF(s.Monhost)
	m.Pool = nodeNetworkStringToTF(s.Pool)
	m.Namespace = nodeNetworkStringToTF(s.Namespace)
	m.DataPool = nodeNetworkStringToTF(s.DataPool)
	m.Username = nodeNetworkStringToTF(s.Username)
	m.Authsupported = nodeNetworkStringToTF(s.Authsupported)
	m.Keyring = nodeNetworkStringToTF(s.Keyring)
	m.KRBD = nodeNetworkBoolPtrToTF(s.KRBD)
	m.Digest = nodeNetworkStringToTF(s.Digest)
}

// storageRemoteRbdDeleteFields returns the PVE wire field names to clear
// on update: attributes present in state but null in the plan.
func storageRemoteRbdDeleteFields(plan, state pveStorageRbdResourceModel) []string {
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
		{plan.Namespace, state.Namespace, "namespace"},
		{plan.DataPool, state.DataPool, "data-pool"},
		{plan.Username, state.Username, "username"},
		{plan.Authsupported, state.Authsupported, "authsupported"},
		{plan.Keyring, state.Keyring, "keyring"},
	} {
		if f.plan.IsNull() && !f.state.IsNull() {
			out = append(out, f.wire)
		}
	}
	if plan.KRBD.IsNull() && !state.KRBD.IsNull() {
		out = append(out, "krbd")
	}
	if len(storageRemoteSetToStrings(plan.Content)) == 0 && len(storageRemoteSetToStrings(state.Content)) > 0 {
		out = append(out, "content")
	}
	return out
}
