// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
)

// TestPveSdnZoneVlanDataSource_MetadataAndSchema covers the data source's
// type name and computed attribute set.
func TestPveSdnZoneVlanDataSource_MetadataAndSchema(t *testing.T) {
	d := NewPveSdnZoneVlanDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveSdnZoneVlan {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveSdnZoneVlan)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"zone", "nodes", "mtu", "ipam", "dns", "dnszone", "reversedns", "dhcp", "digest", "bridge", "bridge_disable_mac_learning"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	if !schemaResp.Schema.Attributes["zone"].IsRequired() {
		t.Fatal("zone attribute should be Required")
	}
	if !schemaResp.Schema.Attributes["bridge"].IsComputed() {
		t.Fatal("bridge attribute should be Computed")
	}
}
