// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
)

// TestPveRealmLdapDataSource_SchemaAndMetadata covers the LDAP realm data
// source's type name and schema shape.
func TestPveRealmLdapDataSource_SchemaAndMetadata(t *testing.T) {
	d := NewPveRealmLdapDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveRealmLdap {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveRealmLdap)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
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
}
