// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
)

// TestPveRealmOpenidResource_SchemaAndMetadata covers the OpenID realm
// resource's type name and schema shape.
func TestPveRealmOpenidResource_SchemaAndMetadata(t *testing.T) {
	r := NewPveRealmOpenidResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveRealmOpenid {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveRealmOpenid)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	for _, key := range []string{
		"realm", "comment", "default", "tfa", "digest",
		"issuer_url", "client_id", "client_key", "username_claim", "groups_claim", "groups_autocreate", "groups_overwrite", "autocreate", "query_userinfo", "scopes", "prompt", "acr_values", "audiences",
	} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	if schemaResp.Schema.Attributes["server1"] != nil {
		t.Fatal("openid realm should not carry LDAP transport attributes")
	}
	realmAttr, ok := schemaResp.Schema.Attributes["realm"].(schema.StringAttribute)
	if !ok {
		t.Fatal("realm attribute is not a StringAttribute")
	}
	if !realmAttr.IsRequired() || realmAttr.PlanModifiers == nil {
		t.Fatal("realm attribute should be Required with plan modifiers (RequiresReplace)")
	}
	if key, ok := schemaResp.Schema.Attributes["client_key"].(schema.StringAttribute); !ok || !key.Sensitive {
		t.Fatal("client_key attribute should be a Sensitive StringAttribute")
	}
}
