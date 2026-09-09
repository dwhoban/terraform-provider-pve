// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/action"
)

// TestPveRealmSyncAction_SchemaAndMetadata covers the realm sync action's
// type name and schema shape.
func TestPveRealmSyncAction_SchemaAndMetadata(t *testing.T) {
	a := NewPveRealmSyncAction()
	ctx := context.Background()
	metaResp := &action.MetadataResponse{}
	a.Metadata(ctx, action.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveRealmSync {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveRealmSync)
	}
	schemaResp := &action.SchemaResponse{}
	a.Schema(ctx, action.SchemaRequest{}, schemaResp)
	for _, key := range []string{"realm", "scope", "dry_run", "enable_new", "remove_vanished", "full", "purge"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	if !schemaResp.Schema.Attributes["realm"].IsRequired() {
		t.Fatal("realm attribute should be Required")
	}
}
