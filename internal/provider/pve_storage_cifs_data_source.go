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
	_ datasource.DataSource              = &pveStorageCifsDataSource{}
	_ datasource.DataSourceWithConfigure = &pveStorageCifsDataSource{}
)

// NewPveStorageCifsDataSource returns the data source implementation.
func NewPveStorageCifsDataSource() datasource.DataSource {
	return &pveStorageCifsDataSource{}
}

// pveStorageCifsDataSource reads a single CIFS storage definition (GET
// /storage/{storage}), erroring when the upstream type is not cifs.
type pveStorageCifsDataSource struct {
	client *pveclient.Client
}

// pveStorageCifsDataSourceModel is the Terraform-facing shape. It carries
// no password because PVE never returns the share's password on reads.
type pveStorageCifsDataSourceModel struct {
	storageFamilyCommonModel
	Server     types.String `tfsdk:"server"`
	Share      types.String `tfsdk:"share"`
	Username   types.String `tfsdk:"username"`
	Domain     types.String `tfsdk:"domain"`
	SMBVersion types.String `tfsdk:"smbversion"`
	Options    types.String `tfsdk:"options"`
}

// Metadata implements datasource.DataSource.
func (d *pveStorageCifsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveStorageCifs
}

// Schema implements datasource.DataSource.
func (d *pveStorageCifsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	attrs := storageFamilyCommonComputedAttributes("Allowed content types as a set (PVE `pve-storage-content-list`). CIFS storages typically serve `backup`, `iso`, `vztmpl`, and `images`.")
	attrs["server"] = schema.StringAttribute{
		Computed:            true,
		MarkdownDescription: "Server IP or DNS name hosting the CIFS share.",
	}
	attrs["share"] = schema.StringAttribute{
		Computed:            true,
		MarkdownDescription: "CIFS share name.",
	}
	attrs["username"] = schema.StringAttribute{
		Computed:            true,
		MarkdownDescription: "User name used to access the share.",
	}
	attrs["domain"] = schema.StringAttribute{
		Computed:            true,
		MarkdownDescription: "CIFS domain (PVE `domain`).",
	}
	attrs["smbversion"] = schema.StringAttribute{
		Computed:            true,
		MarkdownDescription: "SMB protocol version (PVE `smbversion`). One of `default`, `2.0`, `2.1`, `3`, `3.0`, `3.11`.",
	}
	attrs["options"] = schema.StringAttribute{
		Computed:            true,
		MarkdownDescription: "NFS/CIFS mount options (see `man mount.cifs`) (PVE `pve-storage-options`).",
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads a CIFS storage definition from `GET /storage/{storage}`. The share's password is never returned. Errors when the storage's upstream type is not `cifs`.",
		Attributes:          attrs,
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveStorageCifsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = haConfigureDataSource(req, resp)
}

// Read implements datasource.DataSource.
func (d *pveStorageCifsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveStorageCifsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Error reading pve_storage_cifs", "provider client is not configured")
		return
	}
	s, err := storageFamilyGetChecked(ctx, d.client, data.Storage.ValueString(), "cifs")
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_storage_cifs",
			fmt.Sprintf("reading storage %s: %s", data.Storage.ValueString(), err),
		)
		return
	}
	storageFamilyCommonApply(s, &data.storageFamilyCommonModel)
	data.Server = nodeNetworkStringToTF(s.Server)
	data.Share = nodeNetworkStringToTF(s.Share)
	data.Username = nodeNetworkStringToTF(s.Username)
	data.Domain = nodeNetworkStringToTF(s.Domain)
	data.SMBVersion = nodeNetworkStringToTF(s.SMBVersion)
	data.Options = nodeNetworkStringToTF(s.Options)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
