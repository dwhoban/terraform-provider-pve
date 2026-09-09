// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// storageNfsAttrTypes is the tftypes shape of the nfs storage resource and
// data source models.
func storageNfsAttrTypes() map[string]tftypes.Type {
	return map[string]tftypes.Type{
		"storage":               tftypes.String,
		"content":               tftypes.Set{ElementType: tftypes.String},
		"nodes":                 tftypes.String,
		"disable":               tftypes.Bool,
		"shared":                tftypes.Bool,
		"prune_backups":         tftypes.String,
		"max_protected_backups": tftypes.Number,
		"digest":                tftypes.String,
		"server":                tftypes.String,
		"export":                tftypes.String,
		"options":               tftypes.String,
	}
}

// storageNfsTestObject builds a complete model object, nulling every
// attribute absent from vals.
func storageNfsTestObject(vals map[string]tftypes.Value) tftypes.Value {
	attrTypes := storageNfsAttrTypes()
	for name, typ := range attrTypes {
		if _, ok := vals[name]; !ok {
			vals[name] = tftypes.NewValue(typ, nil)
		}
	}
	return tftypes.NewValue(tftypes.Object{AttributeTypes: attrTypes}, vals)
}

// storageNfsTestContentSet builds a content set value.
func storageNfsTestContentSet(values ...string) tftypes.Value {
	elems := make([]tftypes.Value, 0, len(values))
	for _, v := range values {
		elems = append(elems, tftypes.NewValue(tftypes.String, v))
	}
	return tftypes.NewValue(tftypes.Set{ElementType: tftypes.String}, elems)
}

// TestPveStorageNfsResource_MetadataAndSchema covers the resource's full
// type name and its schema contract: required defining fields, the
// recreation plan modifiers, and the computed digest.
func TestPveStorageNfsResource_MetadataAndSchema(t *testing.T) {
	r := NewPveStorageNfsResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveStorageNfs {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveStorageNfs)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	attrs := schemaResp.Schema.Attributes
	for _, key := range []string{
		"storage", "content", "nodes", "disable", "shared",
		"prune_backups", "max_protected_backups", "digest",
		"server", "export", "options",
	} {
		if attrs[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	if !attrs["server"].IsRequired() || !attrs["export"].IsRequired() || !attrs["storage"].IsRequired() {
		t.Fatal("storage, server, and export should be Required")
	}
	if !attrs["digest"].IsComputed() {
		t.Fatal("digest should be Computed")
	}
	for _, key := range []string{"storage", "export"} {
		sa, ok := attrs[key].(schema.StringAttribute)
		if !ok || len(sa.PlanModifiers) == 0 {
			t.Fatalf("%s should carry a RequiresReplace plan modifier", key)
		}
	}
}

// TestPveStorageNfsResource_CreateAndDelete runs the create (POST /storage
// then read-back GET) and delete (DELETE, including already-absent) paths
// against a fake API.
func TestPveStorageNfsResource_CreateAndDelete(t *testing.T) {
	exists := false
	deletes := 0
	r := NewPveStorageNfsResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveStorageNfsResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case req.Method == http.MethodPost && req.URL.Path == "/storage":
			body, _ := io.ReadAll(req.Body)
			for _, want := range []string{`"storage":"media"`, `"type":"nfs"`, `"server":"192.168.1.10"`, `"export":"/srv/export"`, `"content":"images,iso"`} {
				if !strings.Contains(string(body), want) {
					t.Fatalf("create body missing %s: %q", want, body)
				}
			}
			exists = true
			_, _ = io.WriteString(w, `{"data":null}`)
		case req.Method == http.MethodGet && req.URL.Path == "/storage/media":
			if !exists {
				w.WriteHeader(http.StatusNotFound)
				_, _ = io.WriteString(w, `{"errors":"storage 'media' does not exist"}`)
				return
			}
			_, _ = io.WriteString(w, `{"data":{"storage":"media","type":"nfs","server":"192.168.1.10",`+
				`"export":"/srv/export","content":"images,iso","options":"vers=4.2",`+
				`"disable":1,"shared":true,"digest":"cafe1234"}}`)
		case req.Method == http.MethodDelete && req.URL.Path == "/storage/media":
			deletes++
			if deletes > 1 {
				w.WriteHeader(http.StatusNotFound)
				_, _ = io.WriteString(w, `{"errors":"storage 'media' does not exist"}`)
				return
			}
			exists = false
			_, _ = io.WriteString(w, `{"data":null}`)
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
		}
	})
	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	raw := storageNfsTestObject(map[string]tftypes.Value{
		"storage": tftypes.NewValue(tftypes.String, "media"),
		"server":  tftypes.NewValue(tftypes.String, "192.168.1.10"),
		"export":  tftypes.NewValue(tftypes.String, "/srv/export"),
		"content": storageNfsTestContentSet("images", "iso"),
		"options": tftypes.NewValue(tftypes.String, "vers=4.2"),
		"shared":  tftypes.NewValue(tftypes.Bool, true),
	})

	createResp := &resource.CreateResponse{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(tftypes.Object{AttributeTypes: storageNfsAttrTypes()}, nil)},
	}
	r.Create(ctx, resource.CreateRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw},
		Plan:   tfsdk.Plan{Schema: schemaResp.Schema, Raw: raw},
	}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create diagnostics: %s", diagnosticsError(createResp.Diagnostics))
	}
	var created pveStorageNfsResourceModel
	if err := createResp.State.Get(ctx, &created); err != nil {
		t.Fatalf("State.Get after create: %v", err)
	}
	if created.Storage.ValueString() != "media" || created.Server.ValueString() != "192.168.1.10" || created.Export.ValueString() != "/srv/export" {
		t.Fatalf("created identity = %s/%s/%s", created.Storage.ValueString(), created.Server.ValueString(), created.Export.ValueString())
	}
	if len(created.Content.Elements()) != 2 {
		t.Fatalf("content = %v, want 2 elements", created.Content.Elements())
	}
	if created.Options.ValueString() != "vers=4.2" || created.Digest.ValueString() != "cafe1234" {
		t.Fatalf("created read-back fields = %+v", created)
	}
	if !created.Disable.ValueBool() || !created.Shared.ValueBool() {
		t.Fatalf("created flags = disable=%v shared=%v", created.Disable.ValueBool(), created.Shared.ValueBool())
	}

	// First delete succeeds; the second hits the already-absent 404 and
	// must still succeed.
	for i := 0; i < 2; i++ {
		deleteResp := &resource.DeleteResponse{}
		r.Delete(ctx, resource.DeleteRequest{
			State: tfsdk.State{Schema: schemaResp.Schema, Raw: createResp.State.Raw},
		}, deleteResp)
		if deleteResp.Diagnostics.HasError() {
			t.Fatalf("Delete %d diagnostics: %s", i, diagnosticsError(deleteResp.Diagnostics))
		}
	}
}

// TestPveStorageNfsResource_UpdateClearsRemovedFields verifies the PUT
// carries the updatable scalars and translates attributes cleared in the
// plan into the PVE `delete` query parameter, while the pin's create-only
// fields never travel on update.
func TestPveStorageNfsResource_UpdateClearsRemovedFields(t *testing.T) {
	var sawQuery url.Values
	var sawBody string
	r := NewPveStorageNfsResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveStorageNfsResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch req.Method {
		case http.MethodPut:
			if req.URL.Path != "/storage" {
				t.Fatalf("unexpected PUT path: %s", req.URL.Path)
			}
			sawQuery = req.URL.Query()
			raw, _ := io.ReadAll(req.Body)
			sawBody = string(raw)
			_, _ = io.WriteString(w, `{"data":null}`)
		case http.MethodGet:
			_, _ = io.WriteString(w, `{"data":{"storage":"media","type":"nfs","server":"192.168.1.10",`+
				`"export":"/srv/export","content":"images"}}`)
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
		}
	})
	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)

	stateRaw := storageNfsTestObject(map[string]tftypes.Value{
		"storage":       tftypes.NewValue(tftypes.String, "media"),
		"server":        tftypes.NewValue(tftypes.String, "192.168.1.10"),
		"export":        tftypes.NewValue(tftypes.String, "/srv/export"),
		"content":       storageNfsTestContentSet("images", "iso"),
		"options":       tftypes.NewValue(tftypes.String, "vers=3"),
		"prune_backups": tftypes.NewValue(tftypes.String, "keep-last=3"),
	})
	planRaw := storageNfsTestObject(map[string]tftypes.Value{
		"storage": tftypes.NewValue(tftypes.String, "media"),
		"server":  tftypes.NewValue(tftypes.String, "192.168.1.10"),
		"export":  tftypes.NewValue(tftypes.String, "/srv/export"),
		"content": storageNfsTestContentSet("images", "iso"),
	})

	updateResp := &resource.UpdateResponse{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: stateRaw},
	}
	r.Update(ctx, resource.UpdateRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: planRaw},
		Plan:   tfsdk.Plan{Schema: schemaResp.Schema, Raw: planRaw},
		State:  tfsdk.State{Schema: schemaResp.Schema, Raw: stateRaw},
	}, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("Update diagnostics: %s", diagnosticsError(updateResp.Diagnostics))
	}
	if got := sawQuery.Get("delete"); got != "prune-backups,options" {
		t.Fatalf("delete param = %q, want prune-backups,options", got)
	}
	for _, want := range []string{`"storage":"media"`, `"server":"192.168.1.10"`} {
		if !strings.Contains(sawBody, want) {
			t.Fatalf("update body missing %s: %q", want, sawBody)
		}
	}
	for _, banned := range []string{`"type"`, `"export"`} {
		if strings.Contains(sawBody, banned) {
			t.Fatalf("update body must not carry %s (the pin's PUT verb does not accept it): %q", banned, sawBody)
		}
	}
	var updated pveStorageNfsResourceModel
	if err := updateResp.State.Get(ctx, &updated); err != nil {
		t.Fatalf("State.Get after update: %v", err)
	}
	if !updated.Options.IsNull() || !updated.PruneBackups.IsNull() {
		t.Fatalf("cleared fields must read back null: options=%v prune_backups=%v", updated.Options, updated.PruneBackups)
	}
}

// TestPveStorageNfsResource_Read404Removes verifies Read drops the
// resource from state when the storage vanished out of band.
func TestPveStorageNfsResource_Read404Removes(t *testing.T) {
	r := NewPveStorageNfsResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveStorageNfsResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"errors":"storage 'gone' does not exist"}`)
	})
	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	state := tfsdk.State{Schema: schemaResp.Schema, Raw: storageNfsTestObject(map[string]tftypes.Value{
		"storage": tftypes.NewValue(tftypes.String, "gone"),
	})}
	readResp := &resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %s", diagnosticsError(readResp.Diagnostics))
	}
	if !readResp.State.Raw.IsNull() {
		t.Fatalf("expected state to be removed, got %v", readResp.State.Raw)
	}
}

// TestPveStorageNfsResource_ReadWrongTypeErrors verifies the family type
// guard: a storage whose upstream type is cifs is an error naming both
// types, never a silent adoption.
func TestPveStorageNfsResource_ReadWrongTypeErrors(t *testing.T) {
	r := NewPveStorageNfsResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveStorageNfsResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"storage":"media","type":"cifs","server":"192.168.1.10","share":"media"}}`)
	})
	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	state := tfsdk.State{Schema: schemaResp.Schema, Raw: storageNfsTestObject(map[string]tftypes.Value{
		"storage": tftypes.NewValue(tftypes.String, "media"),
	})}
	readResp := &resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if !readResp.Diagnostics.HasError() {
		t.Fatal("Read should error on an upstream type mismatch")
	}
	diag := diagnosticsError(readResp.Diagnostics)
	if !strings.Contains(diag, `"cifs"`) || !strings.Contains(diag, `"nfs"`) {
		t.Fatalf("mismatch diagnostic must name both types, got: %s", diag)
	}
}
