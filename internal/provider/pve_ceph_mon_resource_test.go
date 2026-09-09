// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// TestPveCephMonResource_MetadataAndSchema covers the mon resource's type
// name and schema shape.
func TestPveCephMonResource_MetadataAndSchema(t *testing.T) {
	r := NewPveCephMonResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveCephMon {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveCephMon)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"node", "monid", "mon_address", "addr", "host", "state", "rank", "in_quorum", "service", "ceph_version"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	if !schemaResp.Schema.Attributes["node"].IsRequired() {
		t.Fatal("node attribute should be Required")
	}
}

// TestPveCephMonDataSource_SchemaAndMetadata covers the mon data source's
// type name and schema shape.
func TestPveCephMonDataSource_SchemaAndMetadata(t *testing.T) {
	d := NewPveCephMonDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveCephMon {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveCephMon)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"id", "node", "monid", "addr", "host", "state", "rank", "in_quorum", "service", "dir_exists", "ceph_version", "ceph_version_short"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
}
