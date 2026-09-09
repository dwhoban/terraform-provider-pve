// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
)

// TestPveCephStatusDataSource_SchemaAndMetadata covers the ceph status
// data source's type name and schema shape.
func TestPveCephStatusDataSource_SchemaAndMetadata(t *testing.T) {
	d := NewPveCephStatusDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveCephStatus {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveCephStatus)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"id", "node", "health", "quorum_names", "versions", "monitors", "status_json", "metadata_json"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
}
