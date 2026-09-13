// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// TestPveFileResource_SchemaAndMetadata is the schema test for the
// pve_file resource.
func TestPveFileResource_SchemaAndMetadata(t *testing.T) {
	r := NewPveFileResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveFile {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveFile)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"node", "storage", "file_name", "content_type", "source", "content_base64", "volid", "size", "format", "ctime", "notes"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	if got := schemaResp.Schema.Attributes["content_type"].GetMarkdownDescription(); !storageFileTestContainsAll(got, "`iso`", "`vztmpl`", "`import`") {
		t.Fatalf("content_type description must enumerate the closed set: %q", got)
	}
}

// storageFileTestContainsAll reports whether s contains every fragment.
func storageFileTestContainsAll(s string, fragments ...string) bool {
	for _, fragment := range fragments {
		if !strings.Contains(s, fragment) {
			return false
		}
	}
	return true
}

// TestStorageFileVolidAndSplit checks the volume ID construction and the
// inverse split used by import and listing lookup.
func TestStorageFileVolidAndSplit(t *testing.T) {
	volid := storageFileVolid("local", "iso", "debian-12.iso")
	if volid != "local:iso/debian-12.iso" {
		t.Fatalf("volid = %q", volid)
	}
	contentType, name, ok := storageFileSplitVolid(volid)
	if !ok || contentType != "iso" || name != "debian-12.iso" {
		t.Fatalf("split = %q %q %v", contentType, name, ok)
	}
	if _, _, ok := storageFileSplitVolid("no-separators"); ok {
		t.Fatal("expected split failure for a volid without type/name")
	}
}

// TestStorageFileFindEntry verifies that the listing lookup matches only the
// exact volume ID; a renamed volume is a miss, never a silent adoption.
func TestStorageFileFindEntry(t *testing.T) {
	entries := []pveclient.NodeStorageContentFile{
		{Volid: "local:iso/other.iso"},
		{Volid: "local:iso/debian-12.iso"},
	}
	got, found := storageFileFindEntry(entries, "local:iso/debian-12.iso")
	if !found || got.Volid != "local:iso/debian-12.iso" {
		t.Fatalf("exact match = %+v found=%v", got, found)
	}
	if _, found = storageFileFindEntry(entries, "local:iso/debian-12 (normalized).iso"); found {
		t.Fatal("expected no match for a renamed volume")
	}
	if _, found = storageFileFindEntry(entries, "local:iso/missing.iso"); found {
		t.Fatal("expected no match for an absent volume")
	}
}

// TestStorageFileParseImportID verifies the `<node>:<storage>:<volid>`
// import form, including volids that themselves contain colons.
func TestStorageFileParseImportID(t *testing.T) {
	node, storage, contentType, fileName, err := storageFileParseImportID("pve1:local:local:iso/debian-12.iso")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if node != "pve1" || storage != "local" || contentType != "iso" || fileName != "debian-12.iso" {
		t.Fatalf("parsed = %q %q %q %q", node, storage, contentType, fileName)
	}
	if _, _, _, _, err := storageFileParseImportID("only-two-parts"); err == nil {
		t.Fatal("expected error for an import ID without three segments")
	}
	if _, _, _, _, err := storageFileParseImportID("pve1:local:badvolid"); err == nil {
		t.Fatal("expected error for a volid without type/name")
	}
}
