// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
)

// TestPveNodeHostsDataSource_SchemaAndMetadata covers the /etc/hosts
// data source.
func TestPveNodeHostsDataSource_SchemaAndMetadata(t *testing.T) {
	d := NewPveNodeHostsDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveNodeHosts {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveNodeHosts)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"node", "entries", "digest", "id"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	entries, ok := schemaResp.Schema.Attributes["entries"].(schema.ListNestedAttribute)
	if !ok {
		t.Fatalf("entries must be a schema.ListNestedAttribute, got %T", schemaResp.Schema.Attributes["entries"])
	}
	for _, key := range []string{"address", "hostnames"} {
		if entries.NestedObject.Attributes[key] == nil {
			t.Fatalf("entries nested object missing %s attribute", key)
		}
	}
}
