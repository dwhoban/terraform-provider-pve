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
	_ datasource.DataSource              = &pveStorageDirectoryDataSource{}
	_ datasource.DataSourceWithConfigure = &pveStorageDirectoryDataSource{}
)

// NewPveStorageDirectoryDataSource returns the data source implementation.
func NewPveStorageDirectoryDataSource() datasource.DataSource {
	return &pveStorageDirectoryDataSource{storageTypeDataSource{suffix: TypeNamePveStorageDirectory, wireType: "dir"}}
}

// pveStorageDirectoryDataSource reads one directory storage configuration
// via /storage/{storage}. PVE's upstream type value is `dir` per the pin.
type pveStorageDirectoryDataSource struct {
	storageTypeDataSource
}

// Schema implements datasource.DataSource.
func (d *pveStorageDirectoryDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	attrs := storageTypeSharedDataSourceAttrs("`images`, `rootdir`, `iso`, `vztmpl`, `backup`, `snippets`")
	attrs["path"] = dsschema.StringAttribute{
		Computed:            true,
		MarkdownDescription: "File system path.",
	}
	attrs["is_mountpoint"] = dsschema.StringAttribute{
		Computed:            true,
		MarkdownDescription: "Assume the given path is an externally managed mountpoint and consider the storage offline if it is not mounted. `yes`/`no` shortcut or the target path (PVE default: `no`).",
	}
	attrs["create_base_path"] = dsschema.BoolAttribute{
		Computed:            true,
		MarkdownDescription: "Create the base directory if it doesn't exist (PVE default: `true`).",
	}
	attrs["create_subdirs"] = dsschema.BoolAttribute{
		Computed:            true,
		MarkdownDescription: "Populate the directory with the default structure (PVE default: `true`).",
	}
	attrs["mkdir"] = dsschema.BoolAttribute{
		Computed:            true,
		MarkdownDescription: "Deprecated upstream; use `create_base_path` and `create_subdirs` instead.",
	}
	resp.Schema = dsschema.Schema{
		MarkdownDescription: "Reads a directory storage configuration (`/storage/{storage}` with upstream type `dir`; the pin names the value `dir`, not `directory`). Reading a storage whose upstream type is not `dir` fails with a diagnostic naming both types.",
		Attributes:          attrs,
	}
}
