// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TestPvePoolResource_MetadataAndSchema covers the pool resource's type
// name and schema shape.
func TestPvePoolResource_MetadataAndSchema(t *testing.T) {
	r := NewPvePoolResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePvePool {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePvePool)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"poolid", "comment", "vms", "storages", "allow_move", "members"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	if !schemaResp.Schema.Attributes["poolid"].IsRequired() {
		t.Fatal("poolid attribute should be Required")
	}
	if !schemaResp.Schema.Attributes["members"].IsComputed() {
		t.Fatal("members attribute should be Computed")
	}
}

// TestPvePoolDataSource_MetadataAndSchema covers the pool data source's
// type name and schema shape.
func TestPvePoolDataSource_MetadataAndSchema(t *testing.T) {
	d := NewPvePoolDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePvePool {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePvePool)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"poolid", "comment", "members"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	if !schemaResp.Schema.Attributes["poolid"].IsRequired() {
		t.Fatal("poolid attribute should be Required")
	}
	if !schemaResp.Schema.Attributes["members"].IsComputed() {
		t.Fatal("members attribute should be Computed")
	}
}

// TestPvePoolMemberDiffHelpers covers the add/remove set diff used by
// Update: removed members go through the pin's delete form, added members
// through the add form.
func TestPvePoolMemberDiffHelpers(t *testing.T) {
	planVMs := types.SetValueMust(types.Int64Type, []attr.Value{
		types.Int64Value(100), types.Int64Value(102),
	})
	stateVMs := types.SetValueMust(types.Int64Type, []attr.Value{
		types.Int64Value(100), types.Int64Value(101),
	})
	removed := poolInt64SetRemoved(stateVMs, planVMs)
	if len(removed) != 1 || removed[0] != 101 {
		t.Fatalf("removed = %v, want [101]", removed)
	}
	added := poolInt64SetRemoved(planVMs, stateVMs)
	if len(added) != 1 || added[0] != 102 {
		t.Fatalf("added = %v, want [102]", added)
	}

	planStorages := types.SetValueMust(types.StringType, []attr.Value{types.StringValue("local")})
	stateStorages := types.SetValueMust(types.StringType, []attr.Value{
		types.StringValue("local"), types.StringValue("ceph"),
	})
	removedStorages := poolStringSetRemoved(stateStorages, planStorages)
	if len(removedStorages) != 1 || removedStorages[0] != "ceph" {
		t.Fatalf("removedStorages = %v, want [ceph]", removedStorages)
	}

	// Null sets diff as empty on both sides.
	if got := poolInt64SetRemoved(types.SetNull(types.Int64Type), planVMs); got != nil {
		t.Fatalf("removed from null = %v, want nil", got)
	}
	if got := poolInt64SetRemoved(planVMs, types.SetNull(types.Int64Type)); len(got) != 2 {
		t.Fatalf("removed to null = %v, want both", got)
	}
}
