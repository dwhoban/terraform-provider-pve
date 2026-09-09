// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
)

// TestPveStorageCifsDataSource_MetadataAndSchema covers the data source's
// full type name and its read shape: the storage lookup key is required
// and every other attribute is computed.
func TestPveStorageCifsDataSource_MetadataAndSchema(t *testing.T) {
	d := NewPveStorageCifsDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveStorageCifs {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveStorageCifs)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	attrs := schemaResp.Schema.Attributes
	for _, key := range []string{
		"storage", "content", "nodes", "disable", "shared",
		"prune_backups", "max_protected_backups", "digest",
		"server", "share", "username", "domain", "smbversion", "options",
	} {
		if attrs[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	if !attrs["storage"].IsRequired() {
		t.Fatal("storage should be Required")
	}
	for _, key := range []string{"server", "share", "content", "digest"} {
		if !attrs[key].IsComputed() {
			t.Fatalf("%s should be Computed", key)
		}
	}
	if _, exists := attrs["password"]; exists {
		t.Fatal("the data source must not carry a password attribute (PVE never returns it)")
	}
}
