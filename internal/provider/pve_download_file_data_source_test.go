// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
)

// TestPveDownloadFileDataSource_SchemaAndMetadata is the schema test for
// the pve_download_file data source.
func TestPveDownloadFileDataSource_SchemaAndMetadata(t *testing.T) {
	d := NewPveDownloadFileDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveDownloadFile {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveDownloadFile)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"node", "storage", "file_name", "content_type", "volid", "size", "format", "ctime", "notes", "id"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
}
