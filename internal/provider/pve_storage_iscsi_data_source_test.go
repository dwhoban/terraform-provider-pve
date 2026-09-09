// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
)

// TestPveStorageIscsiDataSource_MetadataAndSchema covers the data source's
// full type name and its read shape.
func TestPveStorageIscsiDataSource_MetadataAndSchema(t *testing.T) {
	d := NewPveStorageIscsiDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveStorageIscsi {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveStorageIscsi)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	attrs := schemaResp.Schema.Attributes
	for _, key := range []string{
		"storage", "content", "nodes", "disable", "shared",
		"prune_backups", "max_protected_backups", "digest",
		"portal", "target", "iscsiprovider",
	} {
		if attrs[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	if !attrs["storage"].IsRequired() {
		t.Fatal("storage should be Required")
	}
	for _, key := range []string{"portal", "target", "content", "digest"} {
		if !attrs[key].IsComputed() {
			t.Fatalf("%s should be Computed", key)
		}
	}
}
