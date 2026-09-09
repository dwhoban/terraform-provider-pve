// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
)

// TestPveStorageIscsidirectDataSource_MetadataAndSchema covers the data
// source's full type name and its read shape.
func TestPveStorageIscsidirectDataSource_MetadataAndSchema(t *testing.T) {
	d := NewPveStorageIscsidirectDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveStorageIscsidirect {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveStorageIscsidirect)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	attrs := schemaResp.Schema.Attributes
	for _, key := range []string{
		"storage", "content", "nodes", "disable", "shared",
		"prune_backups", "max_protected_backups", "digest",
		"portal", "target", "nowritecache",
	} {
		if attrs[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	if !attrs["storage"].IsRequired() {
		t.Fatal("storage should be Required")
	}
	for _, key := range []string{"portal", "target", "nowritecache", "content", "digest"} {
		if !attrs[key].IsComputed() {
			t.Fatalf("%s should be Computed", key)
		}
	}
}
