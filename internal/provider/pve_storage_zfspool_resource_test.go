// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// TestPveStorageZfspoolResource_SchemaAndMetadata asserts the resource's
// full type name and its shared plus ZFS-pool-specific attributes.
func TestPveStorageZfspoolResource_SchemaAndMetadata(t *testing.T) {
	r := NewPveStorageZfspoolResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveStorageZfspool {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveStorageZfspool)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"id", "content", "nodes", "disable", "shared", "bwlimit", "prune_backups", "max_protected_backups", "pool", "blocksize", "sparse"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
}

// TestPveStorageZfspoolDataSource_SchemaAndMetadata asserts the data
// source's full type name and its read-only attribute set.
func TestPveStorageZfspoolDataSource_SchemaAndMetadata(t *testing.T) {
	d := NewPveStorageZfspoolDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveStorageZfspool {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveStorageZfspool)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"id", "content", "pool", "blocksize", "sparse"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
}
