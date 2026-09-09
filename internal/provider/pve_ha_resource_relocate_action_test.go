// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/action"
)

// TestPveHaResourceRelocate_SchemaAndMetadata covers the HA relocate
// action's type name and schema shape.
func TestPveHaResourceRelocate_SchemaAndMetadata(t *testing.T) {
	a := NewPveHaResourceRelocateAction()
	ctx := context.Background()
	metaResp := &action.MetadataResponse{}
	a.Metadata(ctx, action.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveHaResourceRelocate {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveHaResourceRelocate)
	}
	schemaResp := &action.SchemaResponse{}
	a.Schema(ctx, action.SchemaRequest{}, schemaResp)
	for _, key := range []string{"sid", "node"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
		if !schemaResp.Schema.Attributes[key].IsRequired() {
			t.Fatalf("%s attribute should be Required", key)
		}
	}
}
