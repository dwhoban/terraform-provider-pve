// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dsschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
)

// Ensure the framework interfaces are satisfied via the family embed.
var (
	_ datasource.DataSource              = &pveStorageLvmthinDataSource{}
	_ datasource.DataSourceWithConfigure = &pveStorageLvmthinDataSource{}
)

// NewPveStorageLvmthinDataSource returns the data source implementation.
func NewPveStorageLvmthinDataSource() datasource.DataSource {
	return &pveStorageLvmthinDataSource{storageTypeDataSource{suffix: TypeNamePveStorageLvmthin, wireType: "lvmthin"}}
}

// pveStorageLvmthinDataSource reads one LVM-thin storage configuration via
// /storage/{storage}, fixing the upstream type to `lvmthin`.
type pveStorageLvmthinDataSource struct {
	storageTypeDataSource
}

// Schema implements datasource.DataSource.
func (d *pveStorageLvmthinDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	attrs := storageTypeSharedDataSourceAttrs("`images`, `rootdir`")
	attrs["vgname"] = dsschema.StringAttribute{
		Computed:            true,
		MarkdownDescription: "Volume group name.",
	}
	attrs["thinpool"] = dsschema.StringAttribute{
		Computed:            true,
		MarkdownDescription: "LVM thin pool LV name.",
	}
	attrs["tagged_only"] = dsschema.BoolAttribute{
		Computed:            true,
		MarkdownDescription: "Only list logical volumes tagged with `pve-vm-ID`.",
	}
	resp.Schema = dsschema.Schema{
		MarkdownDescription: "Reads an LVM-thin storage configuration (`/storage/{storage}` with type `lvmthin`). Reading a storage whose upstream type is not `lvmthin` fails with a diagnostic naming both types.",
		Attributes:          attrs,
	}
}
