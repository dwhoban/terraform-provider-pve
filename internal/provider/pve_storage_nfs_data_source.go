// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ datasource.DataSource              = &pveStorageNfsDataSource{}
	_ datasource.DataSourceWithConfigure = &pveStorageNfsDataSource{}
)

// NewPveStorageNfsDataSource returns the data source implementation.
func NewPveStorageNfsDataSource() datasource.DataSource {
	return &pveStorageNfsDataSource{}
}

// pveStorageNfsDataSource reads a single NFS storage definition (GET
// /storage/{storage}), erroring when the upstream type is not nfs.
type pveStorageNfsDataSource struct {
	client *pveclient.Client
}

// pveStorageNfsDataSourceModel is the Terraform-facing shape.
type pveStorageNfsDataSourceModel struct {
	storageFamilyCommonModel
	Server  types.String `tfsdk:"server"`
	Export  types.String `tfsdk:"export"`
	Options types.String `tfsdk:"options"`
}

// Metadata implements datasource.DataSource.
func (d *pveStorageNfsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveStorageNfs
}

// storageFamilyCommonComputedAttributes returns the family's shared
// attributes in read form for the data sources: everything except the
// storage lookup key is computed. contentDescription describes the
// content set per type.
func storageFamilyCommonComputedAttributes(contentDescription string) map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"storage": schema.StringAttribute{
			Required:            true,
			MarkdownDescription: "The storage identifier to look up (PVE `pve-storage-id` format).",
		},
		"content": schema.SetAttribute{
			ElementType:         types.StringType,
			Computed:            true,
			MarkdownDescription: contentDescription,
		},
		"nodes": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Comma-separated list of cluster node names the storage configuration applies to (PVE `pve-node-list`).",
		},
		"disable": schema.BoolAttribute{
			Computed:            true,
			MarkdownDescription: "Whether the storage is disabled.",
		},
		"shared": schema.BoolAttribute{
			Computed:            true,
			MarkdownDescription: "Whether the storage is marked as shared with the same contents on all nodes.",
		},
		"prune_backups": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "The retention options in PVE `prune-backups` format, for example `keep-last=3,keep-daily=7`.",
		},
		"max_protected_backups": schema.Int64Attribute{
			Computed:            true,
			MarkdownDescription: "Maximal number of protected backups per guest (PVE `max-protected-backups`). `-1` means unlimited.",
			Validators: []validator.Int64{
				int64validator.AtLeast(-1),
			},
		},
		"digest": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "The read-only storage configuration revision (PVE `digest`).",
		},
	}
}

// Schema implements datasource.DataSource.
func (d *pveStorageNfsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	attrs := storageFamilyCommonComputedAttributes("Allowed content types as a set (PVE `pve-storage-content-list`). NFS storages typically serve `images`, `iso`, `vztmpl`, and `backup`.")
	attrs["server"] = schema.StringAttribute{
		Computed:            true,
		MarkdownDescription: "Server IP or DNS name hosting the NFS export.",
	}
	attrs["export"] = schema.StringAttribute{
		Computed:            true,
		MarkdownDescription: "NFS export path on the server (PVE `pve-storage-path` format).",
	}
	attrs["options"] = schema.StringAttribute{
		Computed:            true,
		MarkdownDescription: "NFS/CIFS mount options (see `man nfs`) (PVE `pve-storage-options`).",
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads an NFS storage definition from `GET /storage/{storage}`. Errors when the storage's upstream type is not `nfs`.",
		Attributes:          attrs,
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveStorageNfsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = haConfigureDataSource(req, resp)
}

// Read implements datasource.DataSource.
func (d *pveStorageNfsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveStorageNfsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Error reading pve_storage_nfs", "provider client is not configured")
		return
	}
	s, err := storageFamilyGetChecked(ctx, d.client, data.Storage.ValueString(), "nfs")
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_storage_nfs",
			fmt.Sprintf("reading storage %s: %s", data.Storage.ValueString(), err),
		)
		return
	}
	storageFamilyCommonApply(s, &data.storageFamilyCommonModel)
	data.Server = nodeNetworkStringToTF(s.Server)
	data.Export = nodeNetworkStringToTF(s.Export)
	data.Options = nodeNetworkStringToTF(s.Options)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
