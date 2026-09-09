// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Shared helpers for the /storage remote-type resources and data sources
// (pve_storage_pbs, pve_storage_cephfs, pve_storage_rbd). Each component
// keeps its own model, schema, and CRUD; these helpers cover the wire
// conversions every one of them repeats.

// storageRemoteStringsToSet builds a types.Set of strings from a Go slice.
func storageRemoteStringsToSet(in []string) types.Set {
	elems := make([]attr.Value, 0, len(in))
	for _, s := range in {
		elems = append(elems, types.StringValue(s))
	}
	set, _ := types.SetValue(types.StringType, elems)
	return set
}

// storageRemoteSetToStrings flattens a types.Set of strings into a
// []string.
func storageRemoteSetToStrings(in types.Set) []string {
	out := make([]string, 0, len(in.Elements()))
	for _, e := range in.Elements() {
		s, ok := e.(types.String)
		if !ok || s.IsNull() || s.IsUnknown() {
			continue
		}
		out = append(out, s.ValueString())
	}
	return out
}

// storageRemoteTypeMismatch reports an error for a storage whose upstream
// type does not match the fixed type of the component reading it.
func storageRemoteTypeMismatch(storage, want, got string) error {
	return fmt.Errorf("storage %s has upstream type %q, want %q; this resource only manages %s storages", storage, got, want, want)
}

// storageRemoteJoinContent encodes the content set into the pin's
// comma-separated pve-storage-content-list wire form.
func storageRemoteJoinContent(in []string) string {
	return strings.Join(in, ",")
}

// storageRemoteSplitContent decodes the pin's comma-separated
// pve-storage-content-list wire form.
func storageRemoteSplitContent(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}

// storageRemoteDataSourceComputed renders one computed read attribute of
// type string.
func storageRemoteDataSourceComputed(description string) schema.Attribute {
	return schema.StringAttribute{
		Computed:            true,
		MarkdownDescription: description,
	}
}

// storageRemoteDataSourceComputedSensitive renders one computed sensitive
// read attribute of type string.
func storageRemoteDataSourceComputedSensitive(description string) schema.Attribute {
	return schema.StringAttribute{
		Computed:            true,
		Sensitive:           true,
		MarkdownDescription: description,
	}
}

// storageRemoteDataSourceComputedBool renders one computed read attribute
// of type boolean.
func storageRemoteDataSourceComputedBool(description string) schema.Attribute {
	return schema.BoolAttribute{
		Computed:            true,
		MarkdownDescription: description,
	}
}

// storageRemoteDataSourceComputedInt64 renders one computed read
// attribute of type integer.
func storageRemoteDataSourceComputedInt64(description string) schema.Attribute {
	return schema.Int64Attribute{
		Computed:            true,
		MarkdownDescription: description,
	}
}
