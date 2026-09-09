// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// TestPveUserTokenResource_SchemaAndMetadata is the schema test for the
// pve_user_token resource.
func TestPveUserTokenResource_SchemaAndMetadata(t *testing.T) {
	r := NewPveUserTokenResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveUserToken {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveUserToken)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"userid", "tokenid", "comment", "expire", "privsep", "token_value"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
}
