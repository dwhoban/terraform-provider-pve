// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
)

// TestPveSdnZoneSimpleDataSource_MetadataAndSchema covers the data
// source's type name and computed attribute set.
func TestPveSdnZoneSimpleDataSource_MetadataAndSchema(t *testing.T) {
	d := NewPveSdnZoneSimpleDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveSdnZoneSimple {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveSdnZoneSimple)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"zone", "nodes", "mtu", "ipam", "dns", "dnszone", "reversedns", "dhcp", "digest"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	if !schemaResp.Schema.Attributes["zone"].IsRequired() {
		t.Fatal("zone attribute should be Required")
	}
	for _, key := range []string{"nodes", "mtu", "ipam", "digest"} {
		if !schemaResp.Schema.Attributes[key].IsComputed() {
			t.Fatalf("%s attribute should be Computed", key)
		}
	}
}
