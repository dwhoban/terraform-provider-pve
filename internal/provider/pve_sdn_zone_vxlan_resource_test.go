// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
)

// TestPveSdnZoneVxlanResource_MetadataAndSchema covers the resource's full
// type name and its schema contract: vxlan-only fields carry the pin's
// validators, and the zone key forces recreation.
func TestPveSdnZoneVxlanResource_MetadataAndSchema(t *testing.T) {
	r := NewPveSdnZoneVxlanResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveSdnZoneVxlan {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveSdnZoneVxlan)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"zone", "nodes", "mtu", "ipam", "dns", "dnszone", "reversedns", "dhcp", "digest", "peers", "vxlan_port", "fabric"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	for _, key := range []string{"bridge", "tag", "vlan_protocol", "controller", "vrf_vxlan"} {
		if schemaResp.Schema.Attributes[key] != nil {
			t.Fatalf("vxlan zone schema must not carry %s attribute", key)
		}
	}
	if !schemaResp.Schema.Attributes["zone"].IsRequired() {
		t.Fatal("zone attribute should be Required")
	}
	portAttr, ok := schemaResp.Schema.Attributes["vxlan_port"].(schema.Int64Attribute)
	if !ok || len(portAttr.Validators) == 0 {
		t.Fatal("vxlan_port attribute should carry a validator")
	}
}
