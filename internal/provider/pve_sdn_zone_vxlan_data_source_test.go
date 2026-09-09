// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
)

// TestPveSdnZoneVxlanDataSource_MetadataAndSchema covers the data source's
// type name and computed attribute set.
func TestPveSdnZoneVxlanDataSource_MetadataAndSchema(t *testing.T) {
	d := NewPveSdnZoneVxlanDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveSdnZoneVxlan {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveSdnZoneVxlan)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"zone", "nodes", "mtu", "ipam", "dns", "dnszone", "reversedns", "dhcp", "digest", "peers", "vxlan_port", "fabric"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	if !schemaResp.Schema.Attributes["zone"].IsRequired() {
		t.Fatal("zone attribute should be Required")
	}
	if !schemaResp.Schema.Attributes["peers"].IsComputed() {
		t.Fatal("peers attribute should be Computed")
	}
}
