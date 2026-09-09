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

// TestPveGroupResource_MetadataAndSchema covers the group resource's type
// name and schema shape.
func TestPveGroupResource_MetadataAndSchema(t *testing.T) {
	r := NewPveGroupResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveGroup {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveGroup)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"groupid", "comment", "members"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	groupID, ok := schemaResp.Schema.Attributes["groupid"].(schema.StringAttribute)
	if !ok {
		t.Fatal("groupid attribute is not a StringAttribute")
	}
	if !groupID.IsRequired() {
		t.Fatal("groupid attribute should be Required")
	}
	if groupID.PlanModifiers == nil {
		t.Fatal("groupid attribute should carry plan modifiers (RequiresReplace)")
	}
	if !schemaResp.Schema.Attributes["members"].IsComputed() {
		t.Fatal("members attribute should be Computed (read-only projection)")
	}
}

// TestPveGroupDataSource_MetadataAndSchema covers the group data source.
func TestPveGroupDataSource_MetadataAndSchema(t *testing.T) {
	d := NewPveGroupDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveGroup {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveGroup)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"id", "groupid", "comment", "members"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	if !schemaResp.Schema.Attributes["groupid"].IsRequired() {
		t.Fatal("groupid attribute should be Required")
	}
}

// TestPveGroupResource_ReadInto verifies the computed projection from
// GET /access/groups/{groupid}: comment passthrough and the member array.
func TestPveGroupResource_ReadInto(t *testing.T) {
	r := &pveGroupResource{client: newNodeNetworkTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet || req.URL.Path != "/access/groups/admins" {
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"comment":"Cluster admins","members":["ci@pam","root@pam"]}}`)
	})}
	m := pveGroupResourceModel{GroupID: types.StringValue("admins")}
	if err := r.readInto(context.Background(), &m); err != nil {
		t.Fatalf("readInto: %v", err)
	}
	if m.Comment.IsNull() || m.Comment.ValueString() != "Cluster admins" {
		t.Fatalf("Comment = %v", m.Comment)
	}
	elements := m.Members.Elements()
	if len(elements) != 2 {
		t.Fatalf("members = %v, want 2 entries", elements)
	}
	first, ok := elements[0].(types.String)
	if !ok || first.ValueString() != "ci@pam" {
		t.Fatalf("members[0] = %v, want ci@pam", elements[0])
	}
}

// TestPveGroupResource_ReadInto_NoCommentAndNoMembers verifies an upstream
// group without comment or members maps to null comment and an empty set.
func TestPveGroupResource_ReadInto_NoCommentAndNoMembers(t *testing.T) {
	r := &pveGroupResource{client: newNodeNetworkTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{}}`)
	})}
	m := pveGroupResourceModel{GroupID: types.StringValue("empty")}
	if err := r.readInto(context.Background(), &m); err != nil {
		t.Fatalf("readInto: %v", err)
	}
	if !m.Comment.IsNull() {
		t.Fatalf("Comment = %v, want null", m.Comment)
	}
	if len(m.Members.Elements()) != 0 {
		t.Fatalf("members = %v, want empty", m.Members.Elements())
	}
}

// TestPveGroupResource_ReadInto_NotFound verifies a vanished group surfaces
// as a not-found error so Read can drop the resource from state.
func TestPveGroupResource_ReadInto_NotFound(t *testing.T) {
	r := &pveGroupResource{client: newNodeNetworkTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"errors":"group 'ghost' does not exist"}`)
	})}
	m := pveGroupResourceModel{GroupID: types.StringValue("ghost")}
	err := r.readInto(context.Background(), &m)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !isPVEClientNotFound(err) {
		t.Fatalf("error should classify as not-found: %v", err)
	}
}

// TestPveGroupUpdateCommentDecision pins the comment-clearing semantics:
// nil skips the PUT, an empty string clears upstream.
func TestPveGroupUpdateCommentDecision(t *testing.T) {
	set := types.StringValue("ops")
	empty := types.StringValue("")
	null := types.StringNull()

	if got := accessGroupUpdateComment(set, null); got == nil || *got != "ops" {
		t.Fatalf("set/null = %v, want ops", got)
	}
	if got := accessGroupUpdateComment(set, set); got == nil || *got != "ops" {
		t.Fatalf("set/set = %v, want ops", got)
	}
	got := accessGroupUpdateComment(null, set)
	if got == nil || *got != "" {
		t.Fatalf("null/set = %v, want pointer to empty string (clear)", got)
	}
	if got := accessGroupUpdateComment(null, null); got != nil {
		t.Fatalf("null/null = %v, want nil (skip PUT)", got)
	}
	if got := accessGroupUpdateComment(empty, set); got == nil || *got != "" {
		t.Fatalf("empty/set = %v, want pointer to empty string (clear)", got)
	}
}
