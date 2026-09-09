// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

// Ensure the framework interfaces are satisfied via the family embed.
var (
	_ resource.Resource                = &pveStorageLvmResource{}
	_ resource.ResourceWithConfigure   = &pveStorageLvmResource{}
	_ resource.ResourceWithImportState = &pveStorageLvmResource{}
)

// NewPveStorageLvmResource returns the resource implementation.
func NewPveStorageLvmResource() resource.Resource {
	return &pveStorageLvmResource{storageTypeResource{suffix: TypeNamePveStorageLvm, wireType: "lvm"}}
}

// pveStorageLvmResource manages one LVM storage configuration via /storage
// and /storage/{storage}, fixing the upstream type to `lvm`.
type pveStorageLvmResource struct {
	storageTypeResource
}

// storageLvmTypeAttrs returns the LVM-specific attributes in their
// resource-facing (optional/required) form.
func storageLvmTypeAttrs() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"vgname": schema.StringAttribute{
			Required:            true,
			MarkdownDescription: "Volume group name.",
		},
		"base": schema.StringAttribute{
			Optional:            true,
			MarkdownDescription: "Base volume. This volume is automatically activated.",
		},
		"saferemove": schema.BoolAttribute{
			Optional:            true,
			MarkdownDescription: "Zero-out data when removing LVs.",
		},
		"saferemove_stepsize": schema.Int64Attribute{
			Optional:            true,
			MarkdownDescription: "Wipe step size in MiB; it will be capped to the maximum supported by the storage (PVE default: `32`). Must be one of: `1`, `2`, `4`, `8`, `16`, `32`.",
			Validators: []validator.Int64{
				int64validator.OneOf(1, 2, 4, 8, 16, 32),
			},
		},
		"tagged_only": schema.BoolAttribute{
			Optional:            true,
			MarkdownDescription: "Only list logical volumes tagged with `pve-vm-ID`.",
		},
	}
}

// Schema implements resource.Resource.
func (r *pveStorageLvmResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := storageTypeSharedResourceAttrs("`images`, `rootdir`")
	for name, attr := range storageLvmTypeAttrs() {
		attrs[name] = attr
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages an LVM storage configuration (`/storage/{storage}` with type `lvm`). Every mutation is synchronous per the pin (no task is spawned); removed optional attributes are cleared via PVE's `delete` parameter.",
		Attributes:          attrs,
	}
}
