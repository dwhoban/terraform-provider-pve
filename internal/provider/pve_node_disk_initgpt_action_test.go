// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/action"
)

// TestPveNodeDiskInitgpt_SchemaAndMetadata covers the disk initgpt
// action's type name and schema shape.
func TestPveNodeDiskInitgpt_SchemaAndMetadata(t *testing.T) {
	a := NewPveNodeDiskInitgptAction()
	ctx := context.Background()
	metaResp := &action.MetadataResponse{}
	a.Metadata(ctx, action.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveNodeDiskInitgpt {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveNodeDiskInitgpt)
	}
	schemaResp := &action.SchemaResponse{}
	a.Schema(ctx, action.SchemaRequest{}, schemaResp)
	for _, key := range []string{"node", "disk"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
		if !schemaResp.Schema.Attributes[key].IsRequired() {
			t.Fatalf("%s attribute should be Required", key)
		}
	}
	if schemaResp.Schema.Attributes["uuid"] == nil {
		t.Fatal("schema missing uuid attribute")
	}
	if schemaResp.Schema.Attributes["uuid"].IsRequired() {
		t.Fatal("uuid attribute should be Optional")
	}
}
