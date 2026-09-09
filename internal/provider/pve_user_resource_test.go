// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// TestPveUserResource_SchemaAndMetadata is the schema test for the
// pve_user resource.
func TestPveUserResource_SchemaAndMetadata(t *testing.T) {
	r := NewPveUserResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveUser {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveUser)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"userid", "comment", "email", "firstname", "lastname", "keys", "enable", "expire", "groups", "password"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
}
