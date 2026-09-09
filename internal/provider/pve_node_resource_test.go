// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// TestPveNodeResource_SchemaAndMetadata asserts the schema builds and the
// resource names itself correctly under any provider type name. This is the
// cheapest contract test we can run without spinning up the protocol server.
func TestPveNodeResource_SchemaAndMetadata(t *testing.T) {
	r := NewPveNodeResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveNode {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveNode)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	if schemaResp.Schema.Attributes["node"] == nil {
		t.Fatal("schema missing node attribute")
	}
	if schemaResp.Schema.Attributes["digest"].IsComputed() == false {
		t.Fatal("digest attribute should be Computed")
	}
}

// TestPveNodeDiskZFSResource_SchemaAndMetadata covers the ZFS resource.
func TestPveNodeDiskZFSResource_SchemaAndMetadata(t *testing.T) {
	r := NewPveNodeDiskZFSResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveNodeDiskZFS {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveNodeDiskZFS)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"node", "name", "raidlevel", "devices", "state"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
}

// TestPveNodeDiskLVMResource_SchemaAndMetadata covers the LVM resource.
func TestPveNodeDiskLVMResource_SchemaAndMetadata(t *testing.T) {
	r := NewPveNodeDiskLVMResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveNodeDiskLVM {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveNodeDiskLVM)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"node", "name", "devices", "add_storage", "cleanup_config", "cleanup_disks"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
}

// TestPveNodeResource_ConfigureRejectsBadClientType asserts Configure
// surfaces a clear error when ProviderData is the wrong type.
func TestPveNodeResource_ConfigureRejectsBadClientType(t *testing.T) {
	r := NewPveNodeResource()
	cc, ok := r.(resource.ResourceWithConfigure)
	if !ok {
		t.Fatal("pve_node does not implement ResourceWithConfigure")
	}
	resp := &resource.ConfigureResponse{}
	cc.Configure(context.Background(), resource.ConfigureRequest{ProviderData: "not-a-client"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error diagnostic for wrong provider data type")
	}
}
