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
	_ resource.Resource                = &pveStorageLvmthinResource{}
	_ resource.ResourceWithConfigure   = &pveStorageLvmthinResource{}
	_ resource.ResourceWithImportState = &pveStorageLvmthinResource{}
)

// NewPveStorageLvmthinResource returns the resource implementation.
func NewPveStorageLvmthinResource() resource.Resource {
	return &pveStorageLvmthinResource{storageTypeResource{suffix: TypeNamePveStorageLvmthin, wireType: "lvmthin"}}
}

// pveStorageLvmthinResource manages one LVM-thin storage configuration via
// /storage and /storage/{storage}, fixing the upstream type to `lvmthin`.
type pveStorageLvmthinResource struct {
	storageTypeResource
}

// storageLvmthinTypeAttrs returns the LVM-thin-specific attributes in
// their resource-facing form.
func storageLvmthinTypeAttrs() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"vgname": schema.StringAttribute{
			Required:            true,
			MarkdownDescription: "Volume group name.",
		},
		"thinpool": schema.StringAttribute{
			Required:            true,
			MarkdownDescription: "LVM thin pool LV name.",
		},
		"tagged_only": schema.BoolAttribute{
			Optional:            true,
			MarkdownDescription: "Only list logical volumes tagged with `pve-vm-ID`.",
		},
	}
}

// Schema implements resource.Resource.
func (r *pveStorageLvmthinResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := storageTypeSharedResourceAttrs("`images`, `rootdir`")
	for name, attr := range storageLvmthinTypeAttrs() {
		attrs[name] = attr
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages an LVM-thin storage configuration (`/storage/{storage}` with type `lvmthin`). Every mutation is synchronous per the pin (no task is spawned); removed optional attributes are cleared via PVE's `delete` parameter.",
		Attributes:          attrs,
	}
}
