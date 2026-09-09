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
	_ datasource.DataSource              = &pveStorageIscsiDataSource{}
	_ datasource.DataSourceWithConfigure = &pveStorageIscsiDataSource{}
)

// NewPveStorageIscsiDataSource returns the data source implementation.
func NewPveStorageIscsiDataSource() datasource.DataSource {
	return &pveStorageIscsiDataSource{}
}

// pveStorageIscsiDataSource reads a single iSCSI target storage definition
// (GET /storage/{storage}), erroring when the upstream type is not iscsi.
type pveStorageIscsiDataSource struct {
	client *pveclient.Client
}

// pveStorageIscsiDataSourceModel is the Terraform-facing shape.
type pveStorageIscsiDataSourceModel struct {
	storageFamilyCommonModel
	Portal        types.String `tfsdk:"portal"`
	Target        types.String `tfsdk:"target"`
	ISCSIProvider types.String `tfsdk:"iscsiprovider"`
}

// Metadata implements datasource.DataSource.
func (d *pveStorageIscsiDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveStorageIscsi
}

// Schema implements datasource.DataSource.
func (d *pveStorageIscsiDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	attrs := storageFamilyCommonComputedAttributes("Allowed content types as a set (PVE `pve-storage-content-list`). iSCSI storages only support `images`.")
	attrs["portal"] = schema.StringAttribute{
		Computed:            true,
		MarkdownDescription: "iSCSI portal (IP or DNS name with optional port), for example `192.168.1.10:3260` (PVE `pve-storage-portal-dns`).",
	}
	attrs["target"] = schema.StringAttribute{
		Computed:            true,
		MarkdownDescription: "iSCSI target, for example `iqn.2000-01.com.example:san.target0`.",
	}
	attrs["iscsiprovider"] = schema.StringAttribute{
		Computed:            true,
		MarkdownDescription: "The iSCSI provider, for example `LIO` (PVE `iscsiprovider`).",
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads an iSCSI target storage definition from `GET /storage/{storage}`. Errors when the storage's upstream type is not `iscsi`.",
		Attributes:          attrs,
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveStorageIscsiDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = haConfigureDataSource(req, resp)
}

// Read implements datasource.DataSource.
func (d *pveStorageIscsiDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveStorageIscsiDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Error reading pve_storage_iscsi", "provider client is not configured")
		return
	}
	s, err := storageFamilyGetChecked(ctx, d.client, data.Storage.ValueString(), "iscsi")
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_storage_iscsi",
			fmt.Sprintf("reading storage %s: %s", data.Storage.ValueString(), err),
		)
		return
	}
	storageFamilyCommonApply(s, &data.storageFamilyCommonModel)
	data.Portal = nodeNetworkStringToTF(s.Portal)
	data.Target = nodeNetworkStringToTF(s.Target)
	data.ISCSIProvider = nodeNetworkStringToTF(s.ISCSIProvider)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
