// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
)

// TestPveFileDataSource_SchemaAndMetadata is the schema test for the
// pve_file data source.
func TestPveFileDataSource_SchemaAndMetadata(t *testing.T) {
	d := NewPveFileDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveFile {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveFile)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"node", "storage", "file_name", "content_type", "volid", "size", "format", "ctime", "notes", "id"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
}
