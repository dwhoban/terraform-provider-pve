// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
)

// TestPveRealmSyncJobResource_SchemaAndMetadata covers the realm-sync job
// resource's type name and schema shape.
func TestPveRealmSyncJobResource_SchemaAndMetadata(t *testing.T) {
	r := NewPveRealmSyncJobResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveRealmSyncJob {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveRealmSyncJob)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"id", "realm", "schedule", "scope", "remove_vanished", "enable_new", "enabled", "comment", "last_run", "next_run"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	idAttr, ok := schemaResp.Schema.Attributes["id"].(schema.StringAttribute)
	if !ok {
		t.Fatal("id attribute is not a StringAttribute")
	}
	if !idAttr.IsRequired() || idAttr.PlanModifiers == nil {
		t.Fatal("id attribute should be Required with plan modifiers (RequiresReplace)")
	}
	realmAttr, ok := schemaResp.Schema.Attributes["realm"].(schema.StringAttribute)
	if !ok {
		t.Fatal("realm attribute is not a StringAttribute")
	}
	if !realmAttr.IsRequired() || realmAttr.PlanModifiers == nil {
		t.Fatal("realm attribute should be Required with plan modifiers (RequiresReplace)")
	}
	if !schemaResp.Schema.Attributes["schedule"].IsRequired() {
		t.Fatal("schedule attribute should be Required")
	}
}
