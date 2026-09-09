// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
)

// TestPveStorageFilesDataSource_SchemaAndMetadata covers the per-storage
// content listing data source.
func TestPveStorageFilesDataSource_SchemaAndMetadata(t *testing.T) {
	d := NewPveStorageFilesDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveStorageFiles {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveStorageFiles)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"id", "node", "storage", "content", "files"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	files, ok := schemaResp.Schema.Attributes["files"].(schema.ListNestedAttribute)
	if !ok {
		t.Fatalf("files attribute type = %T, want schema.ListNestedAttribute", schemaResp.Schema.Attributes["files"])
	}
	for _, key := range []string{"volid", "format", "size", "approximate_size", "used", "vmid", "notes", "ctime", "parent", "protected", "encrypted", "verification"} {
		if files.NestedObject.Attributes[key] == nil {
			t.Fatalf("files schema missing %s attribute", key)
		}
	}
	verification, ok := files.NestedObject.Attributes["verification"].(schema.SingleNestedAttribute)
	if !ok {
		t.Fatalf("verification attribute type = %T, want schema.SingleNestedAttribute", files.NestedObject.Attributes["verification"])
	}
	for _, key := range []string{"state", "upid"} {
		if verification.Attributes[key] == nil {
			t.Fatalf("verification schema missing %s attribute", key)
		}
	}
}
