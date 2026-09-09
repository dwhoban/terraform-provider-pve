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
	_ datasource.DataSource              = &pveStorageIscsidirectDataSource{}
	_ datasource.DataSourceWithConfigure = &pveStorageIscsidirectDataSource{}
)

// NewPveStorageIscsidirectDataSource returns the data source
// implementation.
func NewPveStorageIscsidirectDataSource() datasource.DataSource {
	return &pveStorageIscsidirectDataSource{}
}

// pveStorageIscsidirectDataSource reads a single iSCSI direct-attached
// storage definition (GET /storage/{storage}), erroring when the upstream
// type is not iscsidirect.
type pveStorageIscsidirectDataSource struct {
	client *pveclient.Client
}

// pveStorageIscsidirectDataSourceModel is the Terraform-facing shape.
type pveStorageIscsidirectDataSourceModel struct {
	storageFamilyCommonModel
	Portal       types.String `tfsdk:"portal"`
	Target       types.String `tfsdk:"target"`
	NoWriteCache types.Bool   `tfsdk:"nowritecache"`
}

// Metadata implements datasource.DataSource.
func (d *pveStorageIscsidirectDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveStorageIscsidirect
}

// Schema implements datasource.DataSource.
func (d *pveStorageIscsidirectDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	attrs := storageFamilyCommonComputedAttributes("Allowed content types as a set (PVE `pve-storage-content-list`). iSCSI direct storages only support `images`.")
	attrs["portal"] = schema.StringAttribute{
		Computed:            true,
		MarkdownDescription: "iSCSI portal (IP or DNS name with optional port), for example `192.168.1.10:3260` (PVE `pve-storage-portal-dns`).",
	}
	attrs["target"] = schema.StringAttribute{
		Computed:            true,
		MarkdownDescription: "iSCSI target, for example `iqn.2000-01.com.example:fast.target0`.",
	}
	attrs["nowritecache"] = schema.BoolAttribute{
		Computed:            true,
		MarkdownDescription: "Whether write caching on the target is disabled (PVE `nowritecache`).",
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads an iSCSI direct-attached storage definition from `GET /storage/{storage}`. Errors when the storage's upstream type is not `iscsidirect`.",
		Attributes:          attrs,
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveStorageIscsidirectDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = haConfigureDataSource(req, resp)
}

// Read implements datasource.DataSource.
func (d *pveStorageIscsidirectDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveStorageIscsidirectDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Error reading pve_storage_iscsidirect", "provider client is not configured")
		return
	}
	s, err := storageFamilyGetChecked(ctx, d.client, data.Storage.ValueString(), "iscsidirect")
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_storage_iscsidirect",
			fmt.Sprintf("reading storage %s: %s", data.Storage.ValueString(), err),
		)
		return
	}
	storageFamilyCommonApply(s, &data.storageFamilyCommonModel)
	data.Portal = nodeNetworkStringToTF(s.Portal)
	data.Target = nodeNetworkStringToTF(s.Target)
	data.NoWriteCache = nodeNetworkBoolPtrToTF(s.NoWriteCache)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
