// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// storageRemoteAttrTypes returns the tftypes shape of the remote storage
// resource models: shared attributes plus the given type-specific keys.
func storageRemoteAttrTypes(extra map[string]tftypes.Type) map[string]tftypes.Type {
	types := map[string]tftypes.Type{
		"storage": tftypes.String,
		"content": tftypes.Set{ElementType: tftypes.String},
		"disable": tftypes.Bool,
		"nodes":   tftypes.String,
		"digest":  tftypes.String,
	}
	for k, v := range extra {
		types[k] = v
	}
	return types
}

// storageRemoteTestObject builds a complete model object, nulling every
// attribute absent from vals.
func storageRemoteTestObject(attrTypes map[string]tftypes.Type, vals map[string]tftypes.Value) tftypes.Value {
	for name, typ := range attrTypes {
		if _, ok := vals[name]; !ok {
			vals[name] = tftypes.NewValue(typ, nil)
		}
	}
	return tftypes.NewValue(tftypes.Object{AttributeTypes: attrTypes}, vals)
}

// TestPveStoragePbsResource_MetadataAndSchema covers the resource's full
// type name, the required/sensitive attribute contract, and the port
// range documentation.
func TestPveStoragePbsResource_MetadataAndSchema(t *testing.T) {
	r := NewPveStoragePbsResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveStoragePbs {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveStoragePbs)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	attrs := schemaResp.Schema.Attributes
	for _, key := range []string{
		"storage", "content", "disable", "nodes", "server", "port", "datastore",
		"username", "password", "fingerprint", "namespace", "master_pubkey",
		"max_protected_backups", "prune_backups", "skip_cert_verification", "digest",
	} {
		if attrs[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	for _, key := range []string{"storage", "server", "datastore", "username"} {
		if !attrs[key].IsRequired() {
			t.Fatalf("%s should be Required", key)
		}
	}
	if !attrs["password"].IsSensitive() {
		t.Fatal("password attribute should be Sensitive")
	}
	if !attrs["digest"].IsComputed() {
		t.Fatal("digest attribute should be Computed")
	}
	if desc := attrs["port"].GetMarkdownDescription(); !strings.Contains(desc, "between 1 and 65535") {
		t.Fatalf("port description %q missing inline range", desc)
	}
}

// TestPveStoragePbsResource_CreateAndDelete runs the full create (POST
// then read-back GET) and delete paths against a fake API, including the
// already-absent delete-is-success rule and the upstream type check.
func TestPveStoragePbsResource_CreateAndDelete(t *testing.T) {
	exists := false
	r := NewPveStoragePbsResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveStoragePbsResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case req.Method == http.MethodPost && req.URL.Path == "/storage":
			body, _ := io.ReadAll(req.Body)
			for _, want := range []string{`"storage":"pbs1"`, `"type":"pbs"`, `"server":"192.168.1.10"`, `"datastore":"store1"`, `"password":"s3cret"`} {
				if !strings.Contains(string(body), want) {
					t.Fatalf("create body missing %s: %q", want, body)
				}
			}
			exists = true
			_, _ = io.WriteString(w, `{"data":null}`)
		case req.Method == http.MethodGet && req.URL.Path == "/storage/pbs1":
			if !exists {
				w.WriteHeader(http.StatusNotFound)
				_, _ = io.WriteString(w, `{"errors":"storage 'pbs1' does not exist"}`)
				return
			}
			_, _ = io.WriteString(w, `{"data":{"type":"pbs","server":"192.168.1.10","datastore":"store1",`+
				`"username":"backup@pbs","fingerprint":"AA:BB","port":"8007","content":"backup",`+
				`"disable":0,"digest":"d1"}}`)
		case req.Method == http.MethodDelete && req.URL.Path == "/storage/pbs1":
			exists = false
			_, _ = io.WriteString(w, `{"data":null}`)
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
		}
	})
	ctx := context.Background()
	attrTypes := storageRemoteAttrTypes(map[string]tftypes.Type{
		"server":                 tftypes.String,
		"port":                   tftypes.Number,
		"datastore":              tftypes.String,
		"username":               tftypes.String,
		"password":               tftypes.String,
		"fingerprint":            tftypes.String,
		"namespace":              tftypes.String,
		"master_pubkey":          tftypes.String,
		"max_protected_backups":  tftypes.Number,
		"prune_backups":          tftypes.String,
		"skip_cert_verification": tftypes.Bool,
	})
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	raw := storageRemoteTestObject(attrTypes, map[string]tftypes.Value{
		"storage":     tftypes.NewValue(tftypes.String, "pbs1"),
		"server":      tftypes.NewValue(tftypes.String, "192.168.1.10"),
		"datastore":   tftypes.NewValue(tftypes.String, "store1"),
		"username":    tftypes.NewValue(tftypes.String, "backup@pbs"),
		"password":    tftypes.NewValue(tftypes.String, "s3cret"),
		"fingerprint": tftypes.NewValue(tftypes.String, "AA:BB"),
	})

	createResp := &resource.CreateResponse{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(tftypes.Object{AttributeTypes: attrTypes}, nil)},
	}
	r.Create(ctx, resource.CreateRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw},
		Plan:   tfsdk.Plan{Schema: schemaResp.Schema, Raw: raw},
	}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create diagnostics: %s", diagnosticsError(createResp.Diagnostics))
	}
	var created pveStoragePbsResourceModel
	if err := createResp.State.Get(ctx, &created); err != nil {
		t.Fatalf("State.Get after create: %v", err)
	}
	if created.Storage.ValueString() != "pbs1" || created.Datastore.ValueString() != "store1" {
		t.Fatalf("created identity = %s/%s", created.Storage.ValueString(), created.Datastore.ValueString())
	}
	if created.Port.ValueInt64() != 8007 || created.Digest.ValueString() != "d1" {
		t.Fatalf("created read-back fields = port=%v digest=%v", created.Port, created.Digest)
	}
	if created.Disable.ValueBool() {
		t.Fatal("disable should decode false from the 0/1 encoding")
	}
	if len(created.Content.Elements()) != 1 {
		t.Fatalf("content = %v, want one element", created.Content.Elements())
	}

	deleteResp := &resource.DeleteResponse{}
	r.Delete(ctx, resource.DeleteRequest{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: createResp.State.Raw},
	}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("Delete diagnostics: %s", diagnosticsError(deleteResp.Diagnostics))
	}
}

// TestPveStoragePbsResource_UpdateClearsRemovedFields verifies the PUT
// carries the required scalars and translates attributes cleared in the
// plan into the PVE `delete` query parameter (wire names).
func TestPveStoragePbsResource_UpdateClearsRemovedFields(t *testing.T) {
	var sawQuery url.Values
	var sawBody string
	r := NewPveStoragePbsResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveStoragePbsResource)
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
			_, _ = io.WriteString(w, `{"data":{"type":"pbs","server":"192.168.1.10","datastore":"store1",`+
				`"username":"backup@pbs","port":8007,"digest":"d1"}}`)
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
		}
	})
	ctx := context.Background()
	attrTypes := storageRemoteAttrTypes(map[string]tftypes.Type{
		"server":                 tftypes.String,
		"port":                   tftypes.Number,
		"datastore":              tftypes.String,
		"username":               tftypes.String,
		"password":               tftypes.String,
		"fingerprint":            tftypes.String,
		"namespace":              tftypes.String,
		"master_pubkey":          tftypes.String,
		"max_protected_backups":  tftypes.Number,
		"prune_backups":          tftypes.String,
		"skip_cert_verification": tftypes.Bool,
	})
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)

	shared := map[string]tftypes.Value{
		"storage":   tftypes.NewValue(tftypes.String, "pbs1"),
		"server":    tftypes.NewValue(tftypes.String, "192.168.1.10"),
		"datastore": tftypes.NewValue(tftypes.String, "store1"),
		"username":  tftypes.NewValue(tftypes.String, "backup@pbs"),
		"port":      tftypes.NewValue(tftypes.Number, 8007),
	}
	stateRaw := storageRemoteTestObject(attrTypes, func() map[string]tftypes.Value {
		m := map[string]tftypes.Value{}
		for k, v := range shared {
			m[k] = v
		}
		m["password"] = tftypes.NewValue(tftypes.String, "s3cret")
		m["namespace"] = tftypes.NewValue(tftypes.String, "ns1")
		return m
	}())
	planRaw := storageRemoteTestObject(attrTypes, shared)

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
	if got := sawQuery.Get("delete"); got != "password,namespace" {
		t.Fatalf("delete param = %q, want password,namespace", got)
	}
	for _, want := range []string{`"storage":"pbs1"`, `"server":"192.168.1.10"`, `"port":8007`} {
		if !strings.Contains(sawBody, want) {
			t.Fatalf("update body missing %s: %q", want, sawBody)
		}
	}
	if strings.Contains(sawBody, `"type"`) {
		t.Fatalf("update body must not carry type (pin's PUT has no type parameter): %q", sawBody)
	}
	var updated pveStoragePbsResourceModel
	if err := updateResp.State.Get(ctx, &updated); err != nil {
		t.Fatalf("State.Get after update: %v", err)
	}
	if !updated.Password.IsNull() || !updated.Namespace.IsNull() {
		t.Fatalf("cleared fields must read back null: password=%v namespace=%v", updated.Password, updated.Namespace)
	}
}

// TestPveStoragePbsResource_Read404Removes verifies Read drops the
// resource from state when the storage vanished out of band.
func TestPveStoragePbsResource_Read404Removes(t *testing.T) {
	r := NewPveStoragePbsResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveStoragePbsResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"errors":"storage 'gone' does not exist"}`)
	})
	ctx := context.Background()
	attrTypes := storageRemoteAttrTypes(map[string]tftypes.Type{
		"server":                 tftypes.String,
		"port":                   tftypes.Number,
		"datastore":              tftypes.String,
		"username":               tftypes.String,
		"password":               tftypes.String,
		"fingerprint":            tftypes.String,
		"namespace":              tftypes.String,
		"master_pubkey":          tftypes.String,
		"max_protected_backups":  tftypes.Number,
		"prune_backups":          tftypes.String,
		"skip_cert_verification": tftypes.Bool,
	})
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	state := tfsdk.State{Schema: schemaResp.Schema, Raw: storageRemoteTestObject(attrTypes, map[string]tftypes.Value{
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
