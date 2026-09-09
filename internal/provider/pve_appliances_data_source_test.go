// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
)

// TestPveAppliances_SchemaAndMetadata covers the appliances data source's
// type name and schema shape.
func TestPveAppliances_SchemaAndMetadata(t *testing.T) {
	d := NewPveAppliancesDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveAppliances {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveAppliances)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	if !schemaResp.Schema.Attributes["node"].IsRequired() {
		t.Fatal("node attribute should be Required")
	}
	if !schemaResp.Schema.Attributes["appliances"].IsComputed() {
		t.Fatal("appliances attribute should be Computed")
	}
	for _, key := range []string{"id", "node", "appliances"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
}
