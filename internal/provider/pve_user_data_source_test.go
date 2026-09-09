// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
)

// TestPveUserDataSource_SchemaAndMetadata is the schema test for the
// pve_user data source.
func TestPveUserDataSource_SchemaAndMetadata(t *testing.T) {
	d := NewPveUserDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveUser {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveUser)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"userid", "comment", "email", "firstname", "lastname", "keys", "enable", "expire", "groups", "id"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
}
