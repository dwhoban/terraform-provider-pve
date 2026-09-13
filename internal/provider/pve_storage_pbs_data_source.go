// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ datasource.DataSource              = &pveStoragePbsDataSource{}
	_ datasource.DataSourceWithConfigure = &pveStoragePbsDataSource{}
)

// NewPveStoragePbsDataSource returns the data source implementation.
func NewPveStoragePbsDataSource() datasource.DataSource {
	return &pveStoragePbsDataSource{}
}

// pveStoragePbsDataSource reads one Proxmox Backup Server storage
// definition (GET /storage/{storage}, type `pbs`), including the computed
// PBS identity attributes the storage configuration carries (server
// certificate fingerprint and the master public key).
type pveStoragePbsDataSource struct {
	client *pveclient.Client
}

// pveStoragePbsDataSourceModel is the Terraform-facing shape.
type pveStoragePbsDataSourceModel struct {
	Storage              types.String `tfsdk:"storage"`
	Type                 types.String `tfsdk:"type"`
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

// Metadata implements datasource.DataSource.
func (d *pveStoragePbsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveStoragePbs
}

// Schema implements datasource.DataSource.
func (d *pveStoragePbsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads a Proxmox Backup Server storage definition from `GET /storage/{storage}` (type `pbs`). The computed `fingerprint` and `master_pubkey` attributes carry the PBS identity material returned by the storage configuration.",
		Attributes: map[string]schema.Attribute{
			"storage": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The storage identifier to look up.",
			},
			"type": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The storage type, always `pbs` for this data source.",
			},
			"content": schema.SetAttribute{
				ElementType:         types.StringType,
				Computed:            true,
				MarkdownDescription: "Allowed content types (PVE `pve-storage-content-list`), for `pbs` typically only `backup`.",
			},
			"disable": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether the storage is disabled.",
			},
			"nodes":     storageRemoteDataSourceComputed("Comma-separated list of cluster node names the storage applies to."),
			"server":    storageRemoteDataSourceComputed("The Proxmox Backup Server address."),
			"port":      storageRemoteDataSourceComputedInt64("Optional port used to connect to the server."),
			"datastore": storageRemoteDataSourceComputed("The Proxmox Backup Server datastore name."),
			"username":  storageRemoteDataSourceComputed("The user name or API token ID used to authenticate."),
			"password":  storageRemoteDataSourceComputedSensitive("The user password or API token secret used to authenticate."),
			"fingerprint": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The server certificate SHA 256 fingerprint — part of the PBS identity of this storage.",
			},
			"namespace": storageRemoteDataSourceComputed("The Proxmox Backup Server namespace."),
			"master_pubkey": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Base64-encoded, PEM-formatted public RSA master key (PVE `master-pubkey`) — part of the PBS identity of this storage.",
			},
			"max_protected_backups":  storageRemoteDataSourceComputedInt64("Maximal number of protected backups per guest (PVE `max-protected-backups`)."),
			"prune_backups":          storageRemoteDataSourceComputed("The retention options in PVE `prune-backups` format."),
			"skip_cert_verification": storageRemoteDataSourceComputedBool("Whether TLS certificate verification is disabled."),
			"digest":                 storageRemoteDataSourceComputed("The read-only storage configuration revision (PVE `digest`)."),
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveStoragePbsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = haConfigureDataSource(req, resp)
}

// Read implements datasource.DataSource.
func (d *pveStoragePbsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveStoragePbsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Error reading pve_storage_pbs", "provider client is not configured")
		return
	}
	s, err := d.client.GetStorageRemote(ctx, data.Storage.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_storage_pbs",
			fmt.Sprintf("reading storage %s: %s", data.Storage.ValueString(), err),
		)
		return
	}
	if s.Type != "pbs" {
		resp.Diagnostics.AddError(
			"Error reading pve_storage_pbs",
			fmt.Sprintf("storage %s is of type %q, want %q", s.Storage, s.Type, "pbs"),
		)
		return
	}
	data.Type = nodeNetworkStringToTF(s.Type)
	data.Content = storageRemoteStringsToSet(storageRemoteSplitContent(s.Content))
	data.Disable = nodeNetworkBoolPtrToTF(s.Disable)
	data.Nodes = nodeNetworkStringToTF(s.Nodes)
	data.Server = nodeNetworkStringToTF(s.Server)
	data.Port = metricsServerInt64Value(s.Port)
	data.Datastore = nodeNetworkStringToTF(s.Datastore)
	data.Username = nodeNetworkStringToTF(s.Username)
	data.Password = nodeNetworkStringToTF(s.Password)
	data.Fingerprint = nodeNetworkStringToTF(s.Fingerprint)
	data.Namespace = nodeNetworkStringToTF(s.Namespace)
	data.MasterPubkey = nodeNetworkStringToTF(s.MasterPubkey)
	data.MaxProtectedBackups = metricsServerInt64Value(s.MaxProtectedBackups)
	data.PruneBackups = nodeNetworkStringToTF(s.PruneBackups)
	data.SkipCertVerification = nodeNetworkBoolPtrToTF(s.SkipCertVerification)
	data.Digest = nodeNetworkStringToTF(s.Digest)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
