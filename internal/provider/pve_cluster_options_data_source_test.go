// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
)

// TestPveClusterOptionsDataSource_MetadataAndSchema covers the data source's
// type name and its computed singleton attribute set.
func TestPveClusterOptionsDataSource_MetadataAndSchema(t *testing.T) {
	d := NewPveClusterOptionsDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveClusterOptions {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveClusterOptions)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"id", "bwlimit", "console", "crs", "email_from", "ha", "migration", "next_id", "webauthn"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	if !schemaResp.Schema.Attributes["id"].IsComputed() {
		t.Fatal("id must be computed (singleton semantics)")
	}
	if schemaResp.Schema.Attributes["email_from"].IsOptional() {
		t.Fatal("data source attributes must be computed-only")
	}
}
