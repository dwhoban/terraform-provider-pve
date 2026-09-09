// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
)

// TestPveRealmLdapResource_SchemaAndMetadata covers the LDAP realm
// resource's type name and schema shape.
func TestPveRealmLdapResource_SchemaAndMetadata(t *testing.T) {
	r := NewPveRealmLdapResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveRealmLdap {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveRealmLdap)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	for _, key := range []string{
		"realm", "comment", "default", "tfa", "digest",
		"server1", "server2", "port", "mode", "secure", "verify", "capath", "cert", "certkey", "sslversion", "case_sensitive", "check_connection",
		"base_dn", "bind_dn", "password", "user_attr", "user_classes", "group_classes", "group_dn", "group_filter", "group_name_attr", "filter", "sync_attributes", "sync_defaults_options",
	} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	realmAttr, ok := schemaResp.Schema.Attributes["realm"].(schema.StringAttribute)
	if !ok {
		t.Fatal("realm attribute is not a StringAttribute")
	}
	if !realmAttr.IsRequired() {
		t.Fatal("realm attribute should be Required")
	}
	if realmAttr.PlanModifiers == nil {
		t.Fatal("realm attribute should carry plan modifiers (RequiresReplace)")
	}
	if pw, ok := schemaResp.Schema.Attributes["password"].(schema.StringAttribute); !ok || !pw.Sensitive {
		t.Fatal("password attribute should be a Sensitive StringAttribute")
	}
}
