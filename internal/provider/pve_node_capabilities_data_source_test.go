// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
)

// TestPveNodeCapabilitiesDataSource_SchemaAndMetadata covers the merged
// QEMU capabilities data source.
func TestPveNodeCapabilitiesDataSource_SchemaAndMetadata(t *testing.T) {
	d := NewPveNodeCapabilitiesDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveNodeCapabilities {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveNodeCapabilities)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"id", "node", "cpu_models", "machines", "migration_features"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	cpuModels, ok := schemaResp.Schema.Attributes["cpu_models"].(schema.ListNestedAttribute)
	if !ok {
		t.Fatalf("cpu_models attribute type = %T, want schema.ListNestedAttribute", schemaResp.Schema.Attributes["cpu_models"])
	}
	for _, key := range []string{"name", "vendor", "custom", "abstract"} {
		if cpuModels.NestedObject.Attributes[key] == nil {
			t.Fatalf("cpu_models schema missing %s attribute", key)
		}
	}
	machines, ok := schemaResp.Schema.Attributes["machines"].(schema.ListNestedAttribute)
	if !ok {
		t.Fatalf("machines attribute type = %T, want schema.ListNestedAttribute", schemaResp.Schema.Attributes["machines"])
	}
	for _, key := range []string{"id", "version", "type", "changes"} {
		if machines.NestedObject.Attributes[key] == nil {
			t.Fatalf("machines schema missing %s attribute", key)
		}
	}
	migration, ok := schemaResp.Schema.Attributes["migration_features"].(schema.ListNestedAttribute)
	if !ok {
		t.Fatalf("migration_features attribute type = %T, want schema.ListNestedAttribute", schemaResp.Schema.Attributes["migration_features"])
	}
	if migration.NestedObject.Attributes["has_dbus_vmstate"] == nil {
		t.Fatal("migration_features schema missing has_dbus_vmstate attribute")
	}
}
