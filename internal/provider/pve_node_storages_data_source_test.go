// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
)

// TestPveNodeStoragesDataSource_SchemaAndMetadata covers the per-node
// storage index data source.
func TestPveNodeStoragesDataSource_SchemaAndMetadata(t *testing.T) {
	d := NewPveNodeStoragesDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveNodeStorages {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveNodeStorages)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"id", "node", "content", "storages"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	storages, ok := schemaResp.Schema.Attributes["storages"].(schema.ListNestedAttribute)
	if !ok {
		t.Fatalf("storages attribute type = %T, want schema.ListNestedAttribute", schemaResp.Schema.Attributes["storages"])
	}
	for _, key := range []string{"storage", "type", "content", "shared", "enabled", "active", "used", "total", "avail", "used_fraction"} {
		if storages.NestedObject.Attributes[key] == nil {
			t.Fatalf("storages schema missing %s attribute", key)
		}
	}
}
