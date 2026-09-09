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
	_ resource.Resource                = &pveStorageZfspoolResource{}
	_ resource.ResourceWithConfigure   = &pveStorageZfspoolResource{}
	_ resource.ResourceWithImportState = &pveStorageZfspoolResource{}
)

// NewPveStorageZfspoolResource returns the resource implementation.
func NewPveStorageZfspoolResource() resource.Resource {
	return &pveStorageZfspoolResource{storageTypeResource{suffix: TypeNamePveStorageZfspool, wireType: "zfspool"}}
}

// pveStorageZfspoolResource manages one ZFS pool storage configuration via
// /storage and /storage/{storage}, fixing the upstream type to `zfspool`.
type pveStorageZfspoolResource struct {
	storageTypeResource
}

// storageZfspoolTypeAttrs returns the ZFS-pool-specific attributes in
// their resource-facing form. The pin's /storage parameter set carries no
// ashift, cachefile, or compatibility keys for this type.
func storageZfspoolTypeAttrs() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"pool": schema.StringAttribute{
			Required:            true,
			MarkdownDescription: "ZFS pool name.",
		},
		"blocksize": schema.StringAttribute{
			Optional:            true,
			MarkdownDescription: "ZFS block size: a power of 2 with an optional `k` or `m` suffix, for example `16k`.",
		},
		"sparse": schema.BoolAttribute{
			Optional:            true,
			MarkdownDescription: "Use sparse volumes.",
		},
	}
}

// Schema implements resource.Resource.
func (r *pveStorageZfspoolResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := storageTypeSharedResourceAttrs("`images`, `rootdir`")
	for name, attr := range storageZfspoolTypeAttrs() {
		attrs[name] = attr
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a ZFS pool storage configuration (`/storage/{storage}` with type `zfspool`). Every mutation is synchronous per the pin (no task is spawned); removed optional attributes are cleared via PVE's `delete` parameter.",
		Attributes:          attrs,
	}
}
