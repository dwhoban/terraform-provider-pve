// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
)

// TestPveRealmAdResource_SchemaAndMetadata covers the AD realm resource's
// type name and schema shape.
func TestPveRealmAdResource_SchemaAndMetadata(t *testing.T) {
	r := NewPveRealmAdResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveRealmAd {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveRealmAd)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	for _, key := range []string{
		"realm", "comment", "default", "tfa", "digest",
		"server1", "server2", "port", "mode", "secure", "verify", "capath", "cert", "certkey", "sslversion", "case_sensitive", "check_connection",
		"domain", "bind_dn", "password",
	} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	if schemaResp.Schema.Attributes["base_dn"] != nil {
		t.Fatal("ad realm should not carry LDAP-only base_dn attribute")
	}
	realmAttr, ok := schemaResp.Schema.Attributes["realm"].(schema.StringAttribute)
	if !ok {
		t.Fatal("realm attribute is not a StringAttribute")
	}
	if !realmAttr.IsRequired() || realmAttr.PlanModifiers == nil {
		t.Fatal("realm attribute should be Required with plan modifiers (RequiresReplace)")
	}
	if pw, ok := schemaResp.Schema.Attributes["password"].(schema.StringAttribute); !ok || !pw.Sensitive {
		t.Fatal("password attribute should be a Sensitive StringAttribute")
	}
}
