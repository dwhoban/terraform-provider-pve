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
	_ datasource.DataSource              = &pveStorageRbdDataSource{}
	_ datasource.DataSourceWithConfigure = &pveStorageRbdDataSource{}
)

// NewPveStorageRbdDataSource returns the data source implementation.
func NewPveStorageRbdDataSource() datasource.DataSource {
	return &pveStorageRbdDataSource{}
}

// pveStorageRbdDataSource reads one Ceph RBD storage definition
// (GET /storage/{storage}, type `rbd`).
type pveStorageRbdDataSource struct {
	client *pveclient.Client
}

// pveStorageRbdDataSourceModel is the Terraform-facing shape.
type pveStorageRbdDataSourceModel struct {
	Storage       types.String `tfsdk:"storage"`
	Type          types.String `tfsdk:"type"`
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

// Metadata implements datasource.DataSource.
func (d *pveStorageRbdDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveStorageRbd
}

// Schema implements datasource.DataSource.
func (d *pveStorageRbdDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads a Ceph RBD storage definition from `GET /storage/{storage}` (type `rbd`).",
		Attributes: map[string]schema.Attribute{
			"storage": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The storage identifier to look up.",
			},
			"type": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The storage type, always `rbd` for this data source.",
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
			"nodes":         storageRemoteDataSourceComputed("Comma-separated list of cluster node names the storage applies to."),
			"monhost":       storageRemoteDataSourceComputed("Comma-separated list of monitor addresses of the Ceph cluster."),
			"pool":          storageRemoteDataSourceComputed("The Ceph pool name."),
			"namespace":     storageRemoteDataSourceComputed("The RBD namespace."),
			"data_pool":     storageRemoteDataSourceComputed("The data pool used for erasure coding (PVE `data-pool`)."),
			"username":      storageRemoteDataSourceComputed("The RBD Id (Ceph user name) used to authenticate."),
			"authsupported": storageRemoteDataSourceComputed("The supported authentication modes for the cluster."),
			"keyring":       storageRemoteDataSourceComputedSensitive("Client keyring contents for external clusters."),
			"krbd":          storageRemoteDataSourceComputedBool("Whether RBD is accessed through the `krbd` kernel module."),
			"digest":        storageRemoteDataSourceComputed("The read-only storage configuration revision (PVE `digest`)."),
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveStorageRbdDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = haConfigureDataSource(req, resp)
}

// Read implements datasource.DataSource.
func (d *pveStorageRbdDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveStorageRbdDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Error reading pve_storage_rbd", "provider client is not configured")
		return
	}
	s, err := d.client.GetStorageRemote(ctx, data.Storage.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_storage_rbd",
			fmt.Sprintf("reading storage %s: %s", data.Storage.ValueString(), err),
		)
		return
	}
	if s.Type != "rbd" {
		resp.Diagnostics.AddError(
			"Error reading pve_storage_rbd",
			fmt.Sprintf("storage %s is of type %q, want %q", s.Storage, s.Type, "rbd"),
		)
		return
	}
	data.Type = nodeNetworkStringToTF(s.Type)
	data.Content = storageRemoteStringsToSet(storageRemoteSplitContent(s.Content))
	data.Disable = nodeNetworkBoolPtrToTF(s.Disable)
	data.Nodes = nodeNetworkStringToTF(s.Nodes)
	data.Monhost = nodeNetworkStringToTF(s.Monhost)
	data.Pool = nodeNetworkStringToTF(s.Pool)
	data.Namespace = nodeNetworkStringToTF(s.Namespace)
	data.DataPool = nodeNetworkStringToTF(s.DataPool)
	data.Username = nodeNetworkStringToTF(s.Username)
	data.Authsupported = nodeNetworkStringToTF(s.Authsupported)
	data.Keyring = nodeNetworkStringToTF(s.Keyring)
	data.KRBD = nodeNetworkBoolPtrToTF(s.KRBD)
	data.Digest = nodeNetworkStringToTF(s.Digest)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
