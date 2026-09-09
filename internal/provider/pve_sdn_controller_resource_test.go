// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// TestPveSdnControllerResource_MetadataAndSchema covers the controller
// resource's type name and schema shape, including the closed type enum.
func TestPveSdnControllerResource_MetadataAndSchema(t *testing.T) {
	r := NewPveSdnControllerResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveSdnController {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveSdnController)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"controller", "type", "asn", "bgp_mode", "bgp_multipath_as_path_relax", "ebgp", "ebgp_multihop", "fabric", "isis_domain", "isis_ifaces", "isis_net", "loopback", "node", "nodes", "peer_group_name", "peers", "route_map_in", "route_map_out", "digest", "state"} {
		if _, ok := schemaResp.Schema.Attributes[key]; !ok {
			t.Fatalf("missing attribute %q", key)
		}
	}
	if !schemaResp.Schema.Attributes["controller"].IsRequired() || !schemaResp.Schema.Attributes["type"].IsRequired() {
		t.Fatal("controller and type attributes should be Required")
	}
	typeDesc := schemaResp.Schema.Attributes["type"].GetMarkdownDescription()
	for _, want := range []string{"`bgp`", "`evpn`", "`faucet`", "`isis`"} {
		if !strings.Contains(typeDesc, want) {
			t.Fatalf("type description should enumerate %s: %q", want, typeDesc)
		}
	}
	if !strings.Contains(schemaResp.Schema.Attributes["bgp_mode"].GetMarkdownDescription(), "`auto`") {
		t.Fatal("bgp_mode description should enumerate the enum values")
	}
	if !strings.Contains(schemaResp.Schema.Attributes["asn"].GetMarkdownDescription(), "4294967295") {
		t.Fatal("asn description should enumerate the range")
	}
}

// TestPveSdnControllerDataSource_Metadata asserts the data source type
// name.
func TestPveSdnControllerDataSource_Metadata(t *testing.T) {
	d := NewPveSdnControllerDataSource()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveSdnController {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveSdnController)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(context.Background(), datasource.SchemaRequest{}, schemaResp)
	if !schemaResp.Schema.Attributes["controller"].IsRequired() {
		t.Fatal("controller attribute should be Required")
	}
	if !schemaResp.Schema.Attributes["peers"].IsComputed() {
		t.Fatal("peers attribute should be Computed")
	}
}
