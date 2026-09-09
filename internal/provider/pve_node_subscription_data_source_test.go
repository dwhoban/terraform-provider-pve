// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
)

// TestPveNodeSubscription_SchemaAndMetadata covers the node_subscription
// data source's type name and schema shape.
func TestPveNodeSubscription_SchemaAndMetadata(t *testing.T) {
	d := NewPveNodeSubscriptionDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveNodeSubscription {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveNodeSubscription)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	if !schemaResp.Schema.Attributes["node"].IsRequired() {
		t.Fatal("node attribute should be Required")
	}
	for _, key := range []string{"id", "node", "status", "key", "level", "message", "next_due_date", "product_name", "reg_date", "server_id", "signature", "sockets", "check_time", "url"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	if !schemaResp.Schema.Attributes["key"].IsSensitive() {
		t.Fatal("key attribute should be Sensitive")
	}
	if !schemaResp.Schema.Attributes["status"].IsComputed() {
		t.Fatal("status attribute should be Computed")
	}
}
