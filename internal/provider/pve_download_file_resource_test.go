// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// TestPveDownloadFileResource_SchemaAndMetadata is the schema test for the
// pve_download_file resource.
func TestPveDownloadFileResource_SchemaAndMetadata(t *testing.T) {
	r := NewPveDownloadFileResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveDownloadFile {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveDownloadFile)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"node", "storage", "url", "file_name", "content_type", "checksum", "checksum_algorithm", "compression", "verify_certificates", "volid", "size", "format", "ctime", "notes"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	if got := schemaResp.Schema.Attributes["checksum_algorithm"].GetMarkdownDescription(); !downloadFileTestContainsAll(got, "`md5`", "`sha256`", "`sha512`") {
		t.Fatalf("checksum_algorithm description must enumerate the closed set: %q", got)
	}
}

// downloadFileTestContainsAll reports whether s contains every fragment.
func downloadFileTestContainsAll(s string, fragments ...string) bool {
	for _, fragment := range fragments {
		if !strings.Contains(s, fragment) {
			return false
		}
	}
	return true
}
