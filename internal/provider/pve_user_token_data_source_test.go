// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
)

// TestPveUserTokenDataSource_SchemaAndMetadata is the schema test for the
// pve_user_token data source.
func TestPveUserTokenDataSource_SchemaAndMetadata(t *testing.T) {
	d := NewPveUserTokenDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveUserToken {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveUserToken)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"userid", "tokenid", "comment", "expire", "privsep", "tokens", "id"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
}
