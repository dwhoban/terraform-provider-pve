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

// TestPveSdnDnsResource_MetadataAndSchema covers the DNS resource's type
// name and schema shape.
func TestPveSdnDnsResource_MetadataAndSchema(t *testing.T) {
	r := NewPveSdnDnsResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveSdnDns {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveSdnDns)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"dns", "type", "key", "url", "fingerprint", "reversemaskv6", "ttl"} {
		if _, ok := schemaResp.Schema.Attributes[key]; !ok {
			t.Fatalf("missing attribute %q", key)
		}
	}
	if !schemaResp.Schema.Attributes["dns"].IsRequired() || !schemaResp.Schema.Attributes["key"].IsRequired() || !schemaResp.Schema.Attributes["url"].IsRequired() {
		t.Fatal("dns, key, and url attributes should be Required")
	}
	if !schemaResp.Schema.Attributes["key"].IsSensitive() {
		t.Fatal("key attribute should be Sensitive")
	}
	if !strings.Contains(schemaResp.Schema.Attributes["type"].GetMarkdownDescription(), "powerdns") {
		t.Fatal("type description should state the fixed powerdns type")
	}
}

// TestPveSdnDnsDataSource_Metadata asserts the data source type name.
func TestPveSdnDnsDataSource_Metadata(t *testing.T) {
	d := NewPveSdnDnsDataSource()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveSdnDns {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveSdnDns)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(context.Background(), datasource.SchemaRequest{}, schemaResp)
	if !schemaResp.Schema.Attributes["key"].IsSensitive() {
		t.Fatal("key attribute should be Sensitive")
	}
}
