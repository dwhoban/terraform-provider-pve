// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/action"
)

// TestPveGuestBulkStart_SchemaAndMetadata covers the guest_bulk_start
// action's type name and schema shape.
func TestPveGuestBulkStart_SchemaAndMetadata(t *testing.T) {
	a := NewPveGuestBulkStartAction()
	ctx := context.Background()
	metaResp := &action.MetadataResponse{}
	a.Metadata(ctx, action.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveGuestBulkStart {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveGuestBulkStart)
	}
	schemaResp := &action.SchemaResponse{}
	a.Schema(ctx, action.SchemaRequest{}, schemaResp)
	for _, key := range []string{"max_workers", "timeout", "vms"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
}

// TestPveGuestBulkShutdown_SchemaAndMetadata covers the guest_bulk_shutdown
// action's type name, schema shape, and its destructive warning.
func TestPveGuestBulkShutdown_SchemaAndMetadata(t *testing.T) {
	a := NewPveGuestBulkShutdownAction()
	ctx := context.Background()
	metaResp := &action.MetadataResponse{}
	a.Metadata(ctx, action.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveGuestBulkShutdown {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveGuestBulkShutdown)
	}
	schemaResp := &action.SchemaResponse{}
	a.Schema(ctx, action.SchemaRequest{}, schemaResp)
	for _, key := range []string{"force_stop", "max_workers", "timeout", "vms"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
}

// TestPveGuestBulkSuspend_SchemaAndMetadata covers the guest_bulk_suspend
// action's type name and schema shape.
func TestPveGuestBulkSuspend_SchemaAndMetadata(t *testing.T) {
	a := NewPveGuestBulkSuspendAction()
	ctx := context.Background()
	metaResp := &action.MetadataResponse{}
	a.Metadata(ctx, action.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveGuestBulkSuspend {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveGuestBulkSuspend)
	}
	schemaResp := &action.SchemaResponse{}
	a.Schema(ctx, action.SchemaRequest{}, schemaResp)
	for _, key := range []string{"max_workers", "state_storage", "to_disk", "vms"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
}

// TestPveGuestBulkMigrate_SchemaAndMetadata covers the guest_bulk_migrate
// action's type name and schema shape.
func TestPveGuestBulkMigrate_SchemaAndMetadata(t *testing.T) {
	a := NewPveGuestBulkMigrateAction()
	ctx := context.Background()
	metaResp := &action.MetadataResponse{}
	a.Metadata(ctx, action.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveGuestBulkMigrate {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveGuestBulkMigrate)
	}
	schemaResp := &action.SchemaResponse{}
	a.Schema(ctx, action.SchemaRequest{}, schemaResp)
	for _, key := range []string{"target", "max_workers", "online", "vms", "with_local_disks"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	if !schemaResp.Schema.Attributes["target"].IsRequired() {
		t.Fatal("target attribute should be Required")
	}
}
