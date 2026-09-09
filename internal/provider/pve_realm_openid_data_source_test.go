// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
)

// TestPveRealmOpenidDataSource_SchemaAndMetadata covers the OpenID realm
// data source's type name and schema shape.
func TestPveRealmOpenidDataSource_SchemaAndMetadata(t *testing.T) {
	d := NewPveRealmOpenidDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveRealmOpenid {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveRealmOpenid)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	for _, key := range []string{
		"realm", "comment", "default", "tfa", "digest",
		"issuer_url", "client_id", "client_key", "username_claim", "groups_claim", "groups_autocreate", "groups_overwrite", "autocreate", "query_userinfo", "scopes", "prompt", "acr_values", "audiences",
	} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
}
