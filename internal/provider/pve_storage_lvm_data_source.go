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
	_ datasource.DataSource              = &pveStorageLvmDataSource{}
	_ datasource.DataSourceWithConfigure = &pveStorageLvmDataSource{}
)

// NewPveStorageLvmDataSource returns the data source implementation.
func NewPveStorageLvmDataSource() datasource.DataSource {
	return &pveStorageLvmDataSource{storageTypeDataSource{suffix: TypeNamePveStorageLvm, wireType: "lvm"}}
}

// pveStorageLvmDataSource reads one LVM storage configuration via
// /storage/{storage}, fixing the upstream type to `lvm`.
type pveStorageLvmDataSource struct {
	storageTypeDataSource
}

// Schema implements datasource.DataSource.
func (d *pveStorageLvmDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	attrs := storageTypeSharedDataSourceAttrs("`images`, `rootdir`")
	attrs["vgname"] = dsschema.StringAttribute{
		Computed:            true,
		MarkdownDescription: "Volume group name.",
	}
	attrs["base"] = dsschema.StringAttribute{
		Computed:            true,
		MarkdownDescription: "Base volume. This volume is automatically activated.",
	}
	attrs["saferemove"] = dsschema.BoolAttribute{
		Computed:            true,
		MarkdownDescription: "Zero-out data when removing LVs.",
	}
	attrs["saferemove_stepsize"] = dsschema.Int64Attribute{
		Computed:            true,
		MarkdownDescription: "Wipe step size in MiB; capped to the maximum supported by the storage (PVE default: `32`). One of: `1`, `2`, `4`, `8`, `16`, `32`.",
	}
	attrs["tagged_only"] = dsschema.BoolAttribute{
		Computed:            true,
		MarkdownDescription: "Only list logical volumes tagged with `pve-vm-ID`.",
	}
	resp.Schema = dsschema.Schema{
		MarkdownDescription: "Reads an LVM storage configuration (`/storage/{storage}` with type `lvm`). Reading a storage whose upstream type is not `lvm` fails with a diagnostic naming both types.",
		Attributes:          attrs,
	}
}
