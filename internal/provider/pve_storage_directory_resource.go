// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
)

// Ensure the framework interfaces are satisfied via the family embed.
var (
	_ resource.Resource                = &pveStorageDirectoryResource{}
	_ resource.ResourceWithConfigure   = &pveStorageDirectoryResource{}
	_ resource.ResourceWithImportState = &pveStorageDirectoryResource{}
)

// NewPveStorageDirectoryResource returns the resource implementation.
func NewPveStorageDirectoryResource() resource.Resource {
	return &pveStorageDirectoryResource{storageTypeResource{suffix: TypeNamePveStorageDirectory, wireType: "dir"}}
}

// pveStorageDirectoryResource manages one directory storage configuration
// via /storage and /storage/{storage}. PVE's upstream type value is `dir`
// per the pin.
type pveStorageDirectoryResource struct {
	storageTypeResource
}

// storageDirectoryTypeAttrs returns the directory-specific attributes in
// their resource-facing form. The pin carries no filesystem/fstype
// parameter for this endpoint; filesystem provisioning happens on the
// disk endpoints, not in the storage configuration.
func storageDirectoryTypeAttrs() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"path": schema.StringAttribute{
			Required:            true,
			MarkdownDescription: "File system path.",
		},
		"is_mountpoint": schema.StringAttribute{
			Optional:            true,
			MarkdownDescription: "Assume the given path is an externally managed mountpoint and consider the storage offline if it is not mounted. Set to `yes`/`no` as a shortcut, or to the target path (PVE default: `no`).",
		},
		"create_base_path": schema.BoolAttribute{
			Optional:            true,
			MarkdownDescription: "Create the base directory if it doesn't exist (PVE default: `true`).",
		},
		"create_subdirs": schema.BoolAttribute{
			Optional:            true,
			MarkdownDescription: "Populate the directory with the default structure (PVE default: `true`).",
		},
		"mkdir": schema.BoolAttribute{
			Optional:            true,
			MarkdownDescription: "Create the directory if it doesn't exist and populate it with default sub-dirs. Deprecated upstream; use `create_base_path` and `create_subdirs` instead.",
		},
	}
}

// Schema implements resource.Resource.
func (r *pveStorageDirectoryResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := storageTypeSharedResourceAttrs("`images`, `rootdir`, `iso`, `vztmpl`, `backup`, `snippets`")
	for name, attr := range storageDirectoryTypeAttrs() {
		attrs[name] = attr
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a directory storage configuration (`/storage/{storage}` with upstream type `dir`; the pin names the value `dir`, not `directory`). Every mutation is synchronous per the pin (no task is spawned); removed optional attributes are cleared via PVE's `delete` parameter.",
		Attributes:          attrs,
	}
}
