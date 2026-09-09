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

	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ resource.Resource                = &pveStoragePbsResource{}
	_ resource.ResourceWithConfigure   = &pveStoragePbsResource{}
	_ resource.ResourceWithImportState = &pveStoragePbsResource{}
)

// NewPveStoragePbsResource returns the resource implementation.
func NewPveStoragePbsResource() resource.Resource {
	return &pveStoragePbsResource{}
}

// pveStoragePbsResource manages one Proxmox Backup Server storage
// definition (POST/GET/PUT/DELETE on /storage with type `pbs`).
type pveStoragePbsResource struct {
	client *pveclient.Client
}

// pveStoragePbsResourceModel is the Terraform-facing shape.
type pveStoragePbsResourceModel struct {
	Storage              types.String `tfsdk:"storage"`
	Content              types.Set    `tfsdk:"content"`
	Disable              types.Bool   `tfsdk:"disable"`
	Nodes                types.String `tfsdk:"nodes"`
	Server               types.String `tfsdk:"server"`
	Port                 types.Int64  `tfsdk:"port"`
	Datastore            types.String `tfsdk:"datastore"`
	Username             types.String `tfsdk:"username"`
	Password             types.String `tfsdk:"password"`
	Fingerprint          types.String `tfsdk:"fingerprint"`
	Namespace            types.String `tfsdk:"namespace"`
	MasterPubkey         types.String `tfsdk:"master_pubkey"`
	MaxProtectedBackups  types.Int64  `tfsdk:"max_protected_backups"`
	PruneBackups         types.String `tfsdk:"prune_backups"`
	SkipCertVerification types.Bool   `tfsdk:"skip_cert_verification"`
	Digest               types.String `tfsdk:"digest"`
}

// Metadata implements resource.Resource.
func (r *pveStoragePbsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveStoragePbs
}

// Schema implements resource.Resource.
func (r *pveStoragePbsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Proxmox Backup Server storage definition (`/storage`, type `pbs`). Changing `storage` forces recreation. Deletion is synchronous per the pin (no task is spawned).",
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
				MarkdownDescription: "Allowed content types as a set (PVE `pve-storage-content-list`). For `pbs` storages the expected value is `backup`. PVE validates the set upstream.",
			},
			"disable": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Flag to disable the storage.",
			},
			"nodes": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Comma-separated list of cluster node names the storage configuration applies to (PVE `pve-node-list`).",
			},
			"server": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The Proxmox Backup Server address (name or IP).",
			},
			"port": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Optional port used to connect to the server instead of the default (`8007`). Must be between 1 and 65535 inclusive.",
			},
			"datastore": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The Proxmox Backup Server datastore name.",
			},
			"username": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The user name or API token ID used to authenticate against the datastore (for example `root@pam` or `backup@pbs!token`).",
			},
			"password": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "The user password or API token secret used to authenticate against the datastore.",
			},
			"fingerprint": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The server certificate SHA 256 fingerprint.",
			},
			"namespace": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The Proxmox Backup Server namespace.",
			},
			"master_pubkey": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Base64-encoded, PEM-formatted public RSA master key (PVE `master-pubkey`). Used to encrypt a copy of the encryption key added to each encrypted backup.",
			},
			"max_protected_backups": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Maximal number of protected backups per guest (PVE `max-protected-backups`). Use `-1` for unlimited. Must be -1 or greater.",
			},
			"prune_backups": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The retention options in PVE `prune-backups` format, for example `keep-last=3,keep-daily=7`.",
			},
			"skip_cert_verification": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Disable TLS certificate verification (PVE `skip-cert-verification`); only enable on fully trusted networks.",
			},
			"digest": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The read-only storage configuration revision (PVE `digest`).",
			},
		},
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveStoragePbsResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = haConfigureResource(req, resp)
}

// Create implements resource.Resource.
func (r *pveStoragePbsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveStoragePbsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error creating pve_storage_pbs", "provider client is not configured")
		return
	}
	if err := r.client.CreateStorageRemote(ctx, storageRemotePbsFromModel(plan)); err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_storage_pbs",
			fmt.Sprintf("creating storage %s: %s", plan.Storage.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_storage_pbs after create",
			fmt.Sprintf("reading storage %s: %s", plan.Storage.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	tflog.Debug(ctx, "created PVE pbs storage", map[string]any{"storage": plan.Storage.ValueString()})
}

// Read implements resource.Resource.
func (r *pveStoragePbsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveStoragePbsResourceModel
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
			"Error reading pve_storage_pbs",
			fmt.Sprintf("reading storage %s: %s", state.Storage.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource. Set fields travel in the PUT body;
// attributes cleared in the plan travel in the `delete` query parameter.
func (r *pveStoragePbsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveStoragePbsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state pveStoragePbsResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	deleteFields := storageRemotePbsDeleteFields(plan, state)
	if err := r.client.UpdateStorageRemote(ctx, plan.Storage.ValueString(), storageRemotePbsFromModel(plan), deleteFields); err != nil {
		resp.Diagnostics.AddError(
			"Error updating pve_storage_pbs",
			fmt.Sprintf("updating storage %s: %s", plan.Storage.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_storage_pbs after update",
			fmt.Sprintf("reading storage %s: %s", plan.Storage.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	tflog.Debug(ctx, "updated PVE pbs storage", map[string]any{"storage": plan.Storage.ValueString()})
}

// Delete implements resource.Resource.
func (r *pveStoragePbsResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveStoragePbsResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	tflog.Debug(ctx, "deleting PVE pbs storage", map[string]any{"storage": state.Storage.ValueString()})
	if err := r.client.DeleteStorageRemote(ctx, state.Storage.ValueString()); err != nil {
		if isPVEClientNotFound(err) {
			// Already absent counts as deleted.
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting pve_storage_pbs",
			fmt.Sprintf("deleting storage %s: %s", state.Storage.ValueString(), err),
		)
	}
}

// ImportState parses an import ID of the form `<storage>`.
func (r *pveStoragePbsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		resp.Diagnostics.AddError("Invalid pve_storage_pbs import ID", "import ID must be the storage identifier, e.g. `pbs1`")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("storage"), req.ID)...)
}

// readInto refreshes the model from PVE, erroring when the upstream type
// is no longer pbs.
func (r *pveStoragePbsResource) readInto(ctx context.Context, m *pveStoragePbsResourceModel) error {
	s, err := r.client.GetStorageRemote(ctx, m.Storage.ValueString())
	if err != nil {
		return err
	}
	if s.Type != "pbs" {
		return storageRemoteTypeMismatch(s.Storage, "pbs", s.Type)
	}
	storageRemotePbsApply(s, m)
	return nil
}

// storageRemotePbsFromModel projects the Terraform model into the wire
// body; null values are omitted from the request.
func storageRemotePbsFromModel(m pveStoragePbsResourceModel) pveclient.StorageRemote {
	return pveclient.StorageRemote{
		Storage:              m.Storage.ValueString(),
		Type:                 "pbs",
		Content:              storageRemoteJoinContent(storageRemoteSetToStrings(m.Content)),
		Disable:              metricsServerBoolPtr(m.Disable),
		Nodes:                m.Nodes.ValueString(),
		Server:               m.Server.ValueString(),
		Port:                 metricsServerInt64Ptr(m.Port),
		Datastore:            m.Datastore.ValueString(),
		Username:             m.Username.ValueString(),
		Password:             m.Password.ValueString(),
		Fingerprint:          m.Fingerprint.ValueString(),
		Namespace:            m.Namespace.ValueString(),
		MasterPubkey:         m.MasterPubkey.ValueString(),
		MaxProtectedBackups:  metricsServerInt64Ptr(m.MaxProtectedBackups),
		PruneBackups:         m.PruneBackups.ValueString(),
		SkipCertVerification: metricsServerBoolPtr(m.SkipCertVerification),
	}
}

// storageRemotePbsApply writes a fetched configuration into the model;
// absent settings become null.
func storageRemotePbsApply(s *pveclient.StorageRemote, m *pveStoragePbsResourceModel) {
	m.Content = storageRemoteStringsToSet(storageRemoteSplitContent(s.Content))
	m.Disable = nodeNetworkBoolPtrToTF(s.Disable)
	m.Nodes = nodeNetworkStringToTF(s.Nodes)
	m.Server = nodeNetworkStringToTF(s.Server)
	m.Port = metricsServerInt64Value(s.Port)
	m.Datastore = nodeNetworkStringToTF(s.Datastore)
	m.Username = nodeNetworkStringToTF(s.Username)
	m.Password = nodeNetworkStringToTF(s.Password)
	m.Fingerprint = nodeNetworkStringToTF(s.Fingerprint)
	m.Namespace = nodeNetworkStringToTF(s.Namespace)
	m.MasterPubkey = nodeNetworkStringToTF(s.MasterPubkey)
	m.MaxProtectedBackups = metricsServerInt64Value(s.MaxProtectedBackups)
	m.PruneBackups = nodeNetworkStringToTF(s.PruneBackups)
	m.SkipCertVerification = nodeNetworkBoolPtrToTF(s.SkipCertVerification)
	m.Digest = nodeNetworkStringToTF(s.Digest)
}

// storageRemotePbsDeleteFields returns the PVE wire field names to clear
// on update: attributes present in state but null in the plan.
func storageRemotePbsDeleteFields(plan, state pveStoragePbsResourceModel) []string {
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
		{plan.Password, state.Password, "password"},
		{plan.Fingerprint, state.Fingerprint, "fingerprint"},
		{plan.Namespace, state.Namespace, "namespace"},
		{plan.MasterPubkey, state.MasterPubkey, "master-pubkey"},
		{plan.PruneBackups, state.PruneBackups, "prune-backups"},
	} {
		if f.plan.IsNull() && !f.state.IsNull() {
			out = append(out, f.wire)
		}
	}
	if plan.MaxProtectedBackups.IsNull() && !state.MaxProtectedBackups.IsNull() {
		out = append(out, "max-protected-backups")
	}
	if plan.SkipCertVerification.IsNull() && !state.SkipCertVerification.IsNull() {
		out = append(out, "skip-cert-verification")
	}
	if len(storageRemoteSetToStrings(plan.Content)) == 0 && len(storageRemoteSetToStrings(state.Content)) > 0 {
		out = append(out, "content")
	}
	return out
}
