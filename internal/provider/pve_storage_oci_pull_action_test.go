// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/action"
)

// TestPveStorageOciPull_SchemaAndMetadata covers the OCI pull action's
// type name and schema shape.
func TestPveStorageOciPull_SchemaAndMetadata(t *testing.T) {
	a := NewPveStorageOciPullAction()
	ctx := context.Background()
	metaResp := &action.MetadataResponse{}
	a.Metadata(ctx, action.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveStorageOciPull {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveStorageOciPull)
	}
	schemaResp := &action.SchemaResponse{}
	a.Schema(ctx, action.SchemaRequest{}, schemaResp)
	for _, key := range []string{"node", "storage", "reference"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
		if !schemaResp.Schema.Attributes[key].IsRequired() {
			t.Fatalf("%s attribute should be Required", key)
		}
	}
	if schemaResp.Schema.Attributes["filename"] == nil {
		t.Fatal("schema missing filename attribute")
	}
	if schemaResp.Schema.Attributes["filename"].IsRequired() {
		t.Fatal("filename attribute should be Optional")
	}
}
