// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ datasource.DataSource              = &pveStorageCephfsDataSource{}
	_ datasource.DataSourceWithConfigure = &pveStorageCephfsDataSource{}
)

// NewPveStorageCephfsDataSource returns the data source implementation.
func NewPveStorageCephfsDataSource() datasource.DataSource {
	return &pveStorageCephfsDataSource{}
}

// pveStorageCephfsDataSource reads one CephFS storage definition
// (GET /storage/{storage}, type `cephfs`).
type pveStorageCephfsDataSource struct {
	client *pveclient.Client
}

// pveStorageCephfsDataSourceModel is the Terraform-facing shape.
type pveStorageCephfsDataSourceModel struct {
	Storage  types.String `tfsdk:"storage"`
	Type     types.String `tfsdk:"type"`
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

// Metadata implements datasource.DataSource.
func (d *pveStorageCephfsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveStorageCephfs
}

// Schema implements datasource.DataSource.
func (d *pveStorageCephfsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads a CephFS storage definition from `GET /storage/{storage}` (type `cephfs`).",
		Attributes: map[string]schema.Attribute{
			"storage": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The storage identifier to look up.",
			},
			"type": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The storage type, always `cephfs` for this data source.",
			},
			"content": schema.SetAttribute{
				ElementType:         types.StringType,
				Computed:            true,
				MarkdownDescription: "Allowed content types (PVE `pve-storage-content-list`), for example `images` or `rootdir`.",
			},
			"disable": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether the storage is disabled.",
			},
			"nodes":    storageRemoteDataSourceComputed("Comma-separated list of cluster node names the storage applies to."),
			"monhost":  storageRemoteDataSourceComputed("Comma-separated list of monitor addresses of the Ceph cluster."),
			"fs_name":  storageRemoteDataSourceComputed("The Ceph filesystem name (PVE `fs-name`)."),
			"subdir":   storageRemoteDataSourceComputed("Subdirectory mounted from the CephFS."),
			"path":     storageRemoteDataSourceComputed("File system path where the CephFS is mounted."),
			"fuse":     storageRemoteDataSourceComputedBool("Whether the CephFS is mounted through FUSE."),
			"username": storageRemoteDataSourceComputed("The Ceph user name used to authenticate."),
			"keyring":  storageRemoteDataSourceComputedSensitive("Client keyring contents for external clusters."),
			"digest":   storageRemoteDataSourceComputed("The read-only storage configuration revision (PVE `digest`)."),
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveStorageCephfsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = haConfigureDataSource(req, resp)
}

// Read implements datasource.DataSource.
func (d *pveStorageCephfsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveStorageCephfsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Error reading pve_storage_cephfs", "provider client is not configured")
		return
	}
	s, err := d.client.GetStorageRemote(ctx, data.Storage.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_storage_cephfs",
			fmt.Sprintf("reading storage %s: %s", data.Storage.ValueString(), err),
		)
		return
	}
	if s.Type != "cephfs" {
		resp.Diagnostics.AddError(
			"Error reading pve_storage_cephfs",
			fmt.Sprintf("storage %s is of type %q, want %q", s.Storage, s.Type, "cephfs"),
		)
		return
	}
	data.Type = nodeNetworkStringToTF(s.Type)
	data.Content = storageRemoteStringsToSet(storageRemoteSplitContent(s.Content))
	data.Disable = nodeNetworkBoolPtrToTF(s.Disable)
	data.Nodes = nodeNetworkStringToTF(s.Nodes)
	data.Monhost = nodeNetworkStringToTF(s.Monhost)
	data.FsName = nodeNetworkStringToTF(s.FsName)
	data.Subdir = nodeNetworkStringToTF(s.Subdir)
	data.Path = nodeNetworkStringToTF(s.Path)
	data.Fuse = nodeNetworkBoolPtrToTF(s.Fuse)
	data.Username = nodeNetworkStringToTF(s.Username)
	data.Keyring = nodeNetworkStringToTF(s.Keyring)
	data.Digest = nodeNetworkStringToTF(s.Digest)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
