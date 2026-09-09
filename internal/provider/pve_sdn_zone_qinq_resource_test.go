// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
)

// TestPveSdnZoneQinqResource_MetadataAndSchema covers the resource's full
// type name and its schema contract: qinq-only fields carry the pin's
// validators, and the zone key forces recreation.
func TestPveSdnZoneQinqResource_MetadataAndSchema(t *testing.T) {
	r := NewPveSdnZoneQinqResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveSdnZoneQinq {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveSdnZoneQinq)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"zone", "nodes", "mtu", "ipam", "dns", "dnszone", "reversedns", "dhcp", "digest", "bridge", "tag", "vlan_protocol"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	for _, key := range []string{"bridge_disable_mac_learning", "peers", "vxlan_port", "fabric", "controller", "vrf_vxlan"} {
		if schemaResp.Schema.Attributes[key] != nil {
			t.Fatalf("qinq zone schema must not carry %s attribute", key)
		}
	}
	if !schemaResp.Schema.Attributes["zone"].IsRequired() {
		t.Fatal("zone attribute should be Required")
	}
	tagAttr, ok := schemaResp.Schema.Attributes["tag"].(schema.Int64Attribute)
	if !ok || len(tagAttr.Validators) == 0 {
		t.Fatal("tag attribute should carry a validator")
	}
	protoAttr, ok := schemaResp.Schema.Attributes["vlan_protocol"].(schema.StringAttribute)
	if !ok || len(protoAttr.Validators) == 0 {
		t.Fatal("vlan_protocol attribute should carry a validator")
	}
}
