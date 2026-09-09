// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/action"
	actionschema "github.com/hashicorp/terraform-plugin-framework/action/schema"
)

// TestPveBackupRun_SchemaAndMetadata covers the backup run action's type
// name and schema shape.
func TestPveBackupRun_SchemaAndMetadata(t *testing.T) {
	a := NewPveBackupRunAction()
	ctx := context.Background()
	metaResp := &action.MetadataResponse{}
	a.Metadata(ctx, action.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveBackupRun {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveBackupRun)
	}
	schemaResp := &action.SchemaResponse{}
	a.Schema(ctx, action.SchemaRequest{}, schemaResp)
	if schemaResp.Schema.Attributes["node"] == nil {
		t.Fatal("schema missing node attribute")
	}
	if !schemaResp.Schema.Attributes["node"].IsRequired() {
		t.Fatal("node attribute should be Required")
	}
	for _, key := range []string{
		"mode", "compress", "storage", "vmid", "exclude", "exclude_path", "all",
		"bwlimit", "ionice", "lockwait", "stopwait", "pigz", "zstd",
		"stdexcludes", "quiet", "stop", "remove", "protected", "stdout",
		"fleecing", "performance", "prune_backups",
		"pool", "notes_template", "mailto", "mailnotification", "notification_mode",
		"pbs_change_detection_mode", "job_id", "dumpdir", "tmpdir", "script",
	} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	fleecing, ok := schemaResp.Schema.Attributes["fleecing"].(*actionschema.SingleNestedAttribute)
	if !ok {
		t.Fatal("fleecing should be a SingleNestedAttribute")
	}
	for _, sub := range []string{"enabled", "storage"} {
		if fleecing.Attributes[sub] == nil {
			t.Fatalf("fleecing missing %s attribute", sub)
		}
	}
	prune, ok := schemaResp.Schema.Attributes["prune_backups"].(*actionschema.SingleNestedAttribute)
	if !ok {
		t.Fatal("prune_backups should be a SingleNestedAttribute")
	}
	for _, sub := range []string{"keep_all", "keep_last", "keep_daily"} {
		if prune.Attributes[sub] == nil {
			t.Fatalf("prune_backups missing %s attribute", sub)
		}
	}
	performance, ok := schemaResp.Schema.Attributes["performance"].(*actionschema.SingleNestedAttribute)
	if !ok {
		t.Fatal("performance should be a SingleNestedAttribute")
	}
	for _, sub := range []string{"max_workers", "pbs_entries_max"} {
		if performance.Attributes[sub] == nil {
			t.Fatalf("performance missing %s attribute", sub)
		}
	}
}
