// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TestPveRoleResource_MetadataAndSchema covers the role resource's type name
// and schema shape.
func TestPveRoleResource_MetadataAndSchema(t *testing.T) {
	r := NewPveRoleResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveRole {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveRole)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"roleid", "privs"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	roleID, ok := schemaResp.Schema.Attributes["roleid"].(schema.StringAttribute)
	if !ok {
		t.Fatal("roleid attribute is not a StringAttribute")
	}
	if !roleID.IsRequired() {
		t.Fatal("roleid attribute should be Required")
	}
	if roleID.PlanModifiers == nil {
		t.Fatal("roleid attribute should carry plan modifiers (RequiresReplace)")
	}
	privs, ok := schemaResp.Schema.Attributes["privs"].(schema.SetAttribute)
	if !ok {
		t.Fatal("privs attribute is not a SetAttribute")
	}
	if !privs.IsOptional() || !privs.IsComputed() {
		t.Fatal("privs attribute should be Optional and Computed")
	}
}

// TestPveRoleDataSource_MetadataAndSchema covers the role data source.
func TestPveRoleDataSource_MetadataAndSchema(t *testing.T) {
	d := NewPveRoleDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveRole {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveRole)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"id", "roleid", "privs"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	if !schemaResp.Schema.Attributes["roleid"].IsRequired() {
		t.Fatal("roleid attribute should be Required")
	}
}

// TestPveRoleResource_ReadInto verifies the privilege-map projection from
// GET /access/roles/{roleid}: only true-valued privileges are granted.
func TestPveRoleResource_ReadInto(t *testing.T) {
	r := &pveRoleResource{client: newNodeNetworkTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet || req.URL.Path != "/access/roles/custom-op" {
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"VM.PowerMgmt":true,"Sys.Audit":true,"VM.Audit":false}}`)
	})}
	m := pveRoleResourceModel{RoleID: types.StringValue("custom-op")}
	if err := r.readInto(context.Background(), &m); err != nil {
		t.Fatalf("readInto: %v", err)
	}
	elements := m.Privs.Elements()
	if len(elements) != 2 {
		t.Fatalf("privs = %v, want 2 granted privileges", elements)
	}
	// readInto sorts, so Sys.Audit precedes VM.PowerMgmt.
	first, ok := elements[0].(types.String)
	if !ok || first.ValueString() != "Sys.Audit" {
		t.Fatalf("privs[0] = %v, want Sys.Audit", elements[0])
	}
	second, ok := elements[1].(types.String)
	if !ok || second.ValueString() != "VM.PowerMgmt" {
		t.Fatalf("privs[1] = %v, want VM.PowerMgmt", elements[1])
	}
}

// TestPveRoleResource_ReadInto_EmptyRole verifies a role without privileges
// (all map values false, or no keys) yields an empty set.
func TestPveRoleResource_ReadInto_EmptyRole(t *testing.T) {
	r := &pveRoleResource{client: newNodeNetworkTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{}}`)
	})}
	m := pveRoleResourceModel{RoleID: types.StringValue("bare")}
	if err := r.readInto(context.Background(), &m); err != nil {
		t.Fatalf("readInto: %v", err)
	}
	if len(m.Privs.Elements()) != 0 {
		t.Fatalf("privs = %v, want empty", m.Privs.Elements())
	}
}

// TestPveRoleResource_ReadInto_NotFound verifies a vanished role surfaces as
// a not-found error so Read can drop the resource from state.
func TestPveRoleResource_ReadInto_NotFound(t *testing.T) {
	r := &pveRoleResource{client: newNodeNetworkTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"errors":"no such role"}`)
	})}
	m := pveRoleResourceModel{RoleID: types.StringValue("ghost")}
	err := r.readInto(context.Background(), &m)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !isPVEClientNotFound(err) {
		t.Fatalf("error should classify as not-found: %v", err)
	}
}
