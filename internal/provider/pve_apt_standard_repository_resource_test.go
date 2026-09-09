// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// TestPveAptStandardRepository_MetadataAndSchema covers the standard
// repository resource's type name and schema shape.
func TestPveAptStandardRepository_MetadataAndSchema(t *testing.T) {
	r := NewPveAptStandardRepositoryResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveAptStandardRepository {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveAptStandardRepository)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"node", "handle", "name", "status"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	for _, key := range []string{"node", "handle"} {
		if !schemaResp.Schema.Attributes[key].IsRequired() {
			t.Fatalf("%s attribute should be Required", key)
		}
	}
}

// aptStandardRepoPlanRaw builds a plan/config raw value with node and handle
// set and the computed fields null.
func aptStandardRepoPlanRaw() tftypes.Value {
	attrTypes := aptStandardRepoAttrTypes()
	return tftypes.NewValue(tftypes.Object{AttributeTypes: attrTypes}, map[string]tftypes.Value{
		"node":   tftypes.NewValue(tftypes.String, "pve1"),
		"handle": tftypes.NewValue(tftypes.String, "no-subscription"),
		"name":   tftypes.NewValue(tftypes.String, nil),
		"status": tftypes.NewValue(tftypes.Bool, nil),
	})
}

// aptStandardRepoAttrTypes returns the resource's attribute types.
func aptStandardRepoAttrTypes() map[string]tftypes.Type {
	return map[string]tftypes.Type{
		"node":   tftypes.String,
		"handle": tftypes.String,
		"name":   tftypes.String,
		"status": tftypes.Bool,
	}
}

// aptStandardRepoListing serves a repositories listing whose
// no-subscription row is configured (or not) per the flag.
func aptStandardRepoListing(t *testing.T, configured bool) func(w http.ResponseWriter, r *http.Request) {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/nodes/pve1/apt/repositories" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		status := "null"
		if configured {
			status = "true"
		}
		_, _ = io.WriteString(w, `{"data":{"digest":"dd01","standard-repos":[{"handle":"no-subscription","name":"Proxmox VE No-Subscription Repository","status":`+status+`}]}}`)
	}
}

// TestPveAptStandardRepository_CreateAddsWhenUnconfigured runs the create
// path: read (absent), PUT add, read-back (configured).
func TestPveAptStandardRepository_CreateAddsWhenUnconfigured(t *testing.T) {
	configured := false
	var sawAddBody []byte
	r := NewPveAptStandardRepositoryResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveAptStandardRepositoryResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case req.Method == http.MethodGet && req.URL.Path == "/nodes/pve1/apt/repositories":
			aptStandardRepoListing(t, configured)(w, req)
		case req.Method == http.MethodPut && req.URL.Path == "/nodes/pve1/apt/repositories":
			sawAddBody, _ = io.ReadAll(req.Body)
			configured = true
			_, _ = io.WriteString(w, `{"data":null}`)
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
		}
	})
	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	raw := aptStandardRepoPlanRaw()
	createResp := &resource.CreateResponse{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(tftypes.Object{AttributeTypes: aptStandardRepoAttrTypes()}, nil)},
	}
	r.Create(ctx, resource.CreateRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw},
		Plan:   tfsdk.Plan{Schema: schemaResp.Schema, Raw: raw},
	}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create diagnostics: %s", diagnosticsError(createResp.Diagnostics))
	}
	if string(sawAddBody) != `{"handle":"no-subscription"}` {
		t.Fatalf("add body = %q", sawAddBody)
	}
	var created pveAptStandardRepositoryResourceModel
	if err := createResp.State.Get(ctx, &created); err != nil {
		t.Fatalf("State.Get after create: %v", err)
	}
	if created.Name.ValueString() != "Proxmox VE No-Subscription Repository" || !created.Status.ValueBool() {
		t.Fatalf("created state = %+v", created)
	}
}

// TestPveAptStandardRepository_CreateAdoptsConfigured verifies create is
// idempotent against an already-configured repository: no PUT is issued.
func TestPveAptStandardRepository_CreateAdoptsConfigured(t *testing.T) {
	r := NewPveAptStandardRepositoryResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveAptStandardRepositoryResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet {
			t.Fatalf("no mutation expected, got %s %s", req.Method, req.URL.Path)
		}
		aptStandardRepoListing(t, true)(w, req)
	})
	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	raw := aptStandardRepoPlanRaw()
	createResp := &resource.CreateResponse{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(tftypes.Object{AttributeTypes: aptStandardRepoAttrTypes()}, nil)},
	}
	r.Create(ctx, resource.CreateRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw},
		Plan:   tfsdk.Plan{Schema: schemaResp.Schema, Raw: raw},
	}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create diagnostics: %s", diagnosticsError(createResp.Diagnostics))
	}
	var created pveAptStandardRepositoryResourceModel
	if err := createResp.State.Get(ctx, &created); err != nil {
		t.Fatalf("State.Get after create: %v", err)
	}
	if !created.Status.ValueBool() {
		t.Fatalf("created state = %+v", created)
	}
}

// TestPveAptStandardRepository_ReadRemovesWhenUnconfigured verifies Read
// drops the resource when the repository is no longer configured.
func TestPveAptStandardRepository_ReadRemovesWhenUnconfigured(t *testing.T) {
	r := NewPveAptStandardRepositoryResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveAptStandardRepositoryResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	impl.client = newHaTestClient(t, aptStandardRepoListing(t, false))
	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	state := tfsdk.State{Schema: schemaResp.Schema, Raw: aptStandardRepoPlanRaw()}
	readResp := &resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %s", diagnosticsError(readResp.Diagnostics))
	}
	if !readResp.State.Raw.IsNull() {
		t.Fatalf("expected state to be removed, got %v", readResp.State.Raw)
	}
}

// TestPveAptStandardRepository_DeleteMakesNoRequests documents the pinned
// API's lack of a remove verb: delete forgets without touching the node.
func TestPveAptStandardRepository_DeleteMakesNoRequests(t *testing.T) {
	r := NewPveAptStandardRepositoryResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveAptStandardRepositoryResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		t.Fatalf("delete must not call the API, got %s %s", req.Method, req.URL.Path)
	})
	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	deleteResp := &resource.DeleteResponse{}
	r.Delete(ctx, resource.DeleteRequest{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: aptStandardRepoPlanRaw()},
	}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("Delete diagnostics: %s", diagnosticsError(deleteResp.Diagnostics))
	}
}

// TestPveAptStandardRepository_ImportState checks the `<node>:<handle>`
// import ID parsing, including the rejection of malformed IDs.
func TestPveAptStandardRepository_ImportState(t *testing.T) {
	r := NewPveAptStandardRepositoryResource()
	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	objType := schemaResp.Schema.Type().TerraformType(ctx)
	importer, ok := r.(resource.ResourceWithImportState)
	if !ok {
		t.Fatal("resource should implement ResourceWithImportState")
	}
	importResp := &resource.ImportStateResponse{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(objType, nil)},
	}
	importer.ImportState(ctx, resource.ImportStateRequest{ID: "pve1:no-subscription"}, importResp)
	if importResp.Diagnostics.HasError() {
		t.Fatalf("Import diagnostics: %s", diagnosticsError(importResp.Diagnostics))
	}
	var m pveAptStandardRepositoryResourceModel
	if err := importResp.State.Get(ctx, &m); err != nil {
		t.Fatalf("get imported state: %v", err)
	}
	if m.Node.ValueString() != "pve1" || m.Handle.ValueString() != "no-subscription" {
		t.Fatalf("imported = node %q handle %q", m.Node.ValueString(), m.Handle.ValueString())
	}
	badResp := &resource.ImportStateResponse{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(objType, nil)},
	}
	importer.ImportState(ctx, resource.ImportStateRequest{ID: "onlynode"}, badResp)
	if !badResp.Diagnostics.HasError() || !strings.Contains(diagnosticsError(badResp.Diagnostics), "import ID") {
		t.Fatalf("expected malformed import ID rejection, got: %s", diagnosticsError(badResp.Diagnostics))
	}
}
