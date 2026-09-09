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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// storageCifsAttrTypes is the tftypes shape of the cifs storage resource
// model.
func storageCifsAttrTypes() map[string]tftypes.Type {
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
		"share":                 tftypes.String,
		"username":              tftypes.String,
		"password":              tftypes.String,
		"domain":                tftypes.String,
		"smbversion":            tftypes.String,
		"options":               tftypes.String,
	}
}

// storageCifsTestObject builds a complete model object, nulling every
// attribute absent from vals.
func storageCifsTestObject(vals map[string]tftypes.Value) tftypes.Value {
	attrTypes := storageCifsAttrTypes()
	for name, typ := range attrTypes {
		if _, ok := vals[name]; !ok {
			vals[name] = tftypes.NewValue(typ, nil)
		}
	}
	return tftypes.NewValue(tftypes.Object{AttributeTypes: attrTypes}, vals)
}

// TestPveStorageCifsResource_MetadataAndSchema covers the resource's full
// type name and its schema contract: required defining fields, the
// sensitive password, the inline smbversion enumeration, and the computed
// digest.
func TestPveStorageCifsResource_MetadataAndSchema(t *testing.T) {
	r := NewPveStorageCifsResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveStorageCifs {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveStorageCifs)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	attrs := schemaResp.Schema.Attributes
	for _, key := range []string{
		"storage", "content", "nodes", "disable", "shared",
		"prune_backups", "max_protected_backups", "digest",
		"server", "share", "username", "password", "domain", "smbversion", "options",
	} {
		if attrs[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	if !attrs["server"].IsRequired() || !attrs["share"].IsRequired() {
		t.Fatal("server and share should be Required")
	}
	if !attrs["password"].IsSensitive() {
		t.Fatal("password should be Sensitive")
	}
	sa, ok := attrs["share"].(schema.StringAttribute)
	if !ok || len(sa.PlanModifiers) == 0 {
		t.Fatal("share should carry a RequiresReplace plan modifier")
	}
	desc := attrs["smbversion"].GetMarkdownDescription()
	if !strings.Contains(desc, "Must be one of") {
		t.Fatalf("smbversion description missing inline enumeration: %q", desc)
	}
	for _, v := range []string{"`default`", "`2.0`", "`2.1`", "`3`", "`3.0`", "`3.11`"} {
		if !strings.Contains(desc, v) {
			t.Fatalf("smbversion description %q missing enum value %s", desc, v)
		}
	}
}

// TestPveStorageCifsResource_CreateAndDelete runs the create and delete
// paths, including the secret-bearing fields; PVE never returns the
// password so it reads back null.
func TestPveStorageCifsResource_CreateAndDelete(t *testing.T) {
	exists := false
	r := NewPveStorageCifsResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveStorageCifsResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case req.Method == http.MethodPost && req.URL.Path == "/storage":
			body, _ := io.ReadAll(req.Body)
			for _, want := range []string{`"storage":"archive"`, `"type":"cifs"`, `"server":"192.168.1.10"`, `"share":"archive"`, `"username":"backup"`, `"password":"s3cret"`, `"domain":"CORP"`, `"smbversion":"3.11"`} {
				if !strings.Contains(string(body), want) {
					t.Fatalf("create body missing %s: %q", want, body)
				}
			}
			exists = true
			_, _ = io.WriteString(w, `{"data":null}`)
		case req.Method == http.MethodGet && req.URL.Path == "/storage/archive":
			if !exists {
				w.WriteHeader(http.StatusNotFound)
				_, _ = io.WriteString(w, `{"errors":"storage 'archive' does not exist"}`)
				return
			}
			_, _ = io.WriteString(w, `{"data":{"storage":"archive","type":"cifs","server":"192.168.1.10",`+
				`"share":"archive","username":"backup","domain":"CORP","smbversion":"3.11",`+
				`"content":"backup","digest":"beef5678"}}`)
		case req.Method == http.MethodDelete && req.URL.Path == "/storage/archive":
			exists = false
			_, _ = io.WriteString(w, `{"data":null}`)
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
		}
	})
	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	raw := storageCifsTestObject(map[string]tftypes.Value{
		"storage":    tftypes.NewValue(tftypes.String, "archive"),
		"server":     tftypes.NewValue(tftypes.String, "192.168.1.10"),
		"share":      tftypes.NewValue(tftypes.String, "archive"),
		"username":   tftypes.NewValue(tftypes.String, "backup"),
		"password":   tftypes.NewValue(tftypes.String, "s3cret"),
		"domain":     tftypes.NewValue(tftypes.String, "CORP"),
		"smbversion": tftypes.NewValue(tftypes.String, "3.11"),
		"content":    storageNfsTestContentSet("backup"),
	})

	createResp := &resource.CreateResponse{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(tftypes.Object{AttributeTypes: storageCifsAttrTypes()}, nil)},
	}
	r.Create(ctx, resource.CreateRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw},
		Plan:   tfsdk.Plan{Schema: schemaResp.Schema, Raw: raw},
	}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create diagnostics: %s", diagnosticsError(createResp.Diagnostics))
	}
	var created pveStorageCifsResourceModel
	if err := createResp.State.Get(ctx, &created); err != nil {
		t.Fatalf("State.Get after create: %v", err)
	}
	if created.Share.ValueString() != "archive" || created.Username.ValueString() != "backup" || created.SMBVersion.ValueString() != "3.11" {
		t.Fatalf("created read-back fields = %+v", created)
	}
	if !created.Password.IsNull() {
		t.Fatalf("password must read back null (PVE never returns it), got %v", created.Password)
	}
	if created.Digest.ValueString() != "beef5678" {
		t.Fatalf("digest = %s", created.Digest.ValueString())
	}

	deleteResp := &resource.DeleteResponse{}
	r.Delete(ctx, resource.DeleteRequest{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: createResp.State.Raw},
	}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("Delete diagnostics: %s", diagnosticsError(deleteResp.Diagnostics))
	}
}
