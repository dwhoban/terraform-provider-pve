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
	_ datasource.DataSource              = &pveStorageZfspoolDataSource{}
	_ datasource.DataSourceWithConfigure = &pveStorageZfspoolDataSource{}
)

// NewPveStorageZfspoolDataSource returns the data source implementation.
func NewPveStorageZfspoolDataSource() datasource.DataSource {
	return &pveStorageZfspoolDataSource{storageTypeDataSource{suffix: TypeNamePveStorageZfspool, wireType: "zfspool"}}
}

// pveStorageZfspoolDataSource reads one ZFS pool storage configuration via
// /storage/{storage}, fixing the upstream type to `zfspool`.
type pveStorageZfspoolDataSource struct {
	storageTypeDataSource
}

// Schema implements datasource.DataSource.
func (d *pveStorageZfspoolDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	attrs := storageTypeSharedDataSourceAttrs("`images`, `rootdir`")
	attrs["pool"] = dsschema.StringAttribute{
		Computed:            true,
		MarkdownDescription: "ZFS pool name.",
	}
	attrs["blocksize"] = dsschema.StringAttribute{
		Computed:            true,
		MarkdownDescription: "ZFS block size: a power of 2 with an optional `k` or `m` suffix, for example `16k`.",
	}
	attrs["sparse"] = dsschema.BoolAttribute{
		Computed:            true,
		MarkdownDescription: "Use sparse volumes.",
	}
	resp.Schema = dsschema.Schema{
		MarkdownDescription: "Reads a ZFS pool storage configuration (`/storage/{storage}` with type `zfspool`). Reading a storage whose upstream type is not `zfspool` fails with a diagnostic naming both types.",
		Attributes:          attrs,
	}
}
