// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
)

// TestPveRealmSyncJobDataSource_SchemaAndMetadata covers the realm-sync
// job data source's type name and schema shape.
func TestPveRealmSyncJobDataSource_SchemaAndMetadata(t *testing.T) {
	d := NewPveRealmSyncJobDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveRealmSyncJob {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveRealmSyncJob)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"id", "realm", "schedule", "scope", "remove_vanished", "enable_new", "enabled", "comment", "last_run", "next_run"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	if !schemaResp.Schema.Attributes["id"].IsRequired() {
		t.Fatal("id attribute should be Required")
	}
}
