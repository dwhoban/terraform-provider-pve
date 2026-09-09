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

// storageIscsidirectAttrTypes is the tftypes shape of the iscsidirect
// storage resource model.
func storageIscsidirectAttrTypes() map[string]tftypes.Type {
	return map[string]tftypes.Type{
		"storage":               tftypes.String,
		"content":               tftypes.Set{ElementType: tftypes.String},
		"nodes":                 tftypes.String,
		"disable":               tftypes.Bool,
		"shared":                tftypes.Bool,
		"prune_backups":         tftypes.String,
		"max_protected_backups": tftypes.Number,
		"digest":                tftypes.String,
		"portal":                tftypes.String,
		"target":                tftypes.String,
		"nowritecache":          tftypes.Bool,
	}
}

// storageIscsidirectTestObject builds a complete model object, nulling
// every attribute absent from vals.
func storageIscsidirectTestObject(vals map[string]tftypes.Value) tftypes.Value {
	attrTypes := storageIscsidirectAttrTypes()
	for name, typ := range attrTypes {
		if _, ok := vals[name]; !ok {
			vals[name] = tftypes.NewValue(typ, nil)
		}
	}
	return tftypes.NewValue(tftypes.Object{AttributeTypes: attrTypes}, vals)
}

// TestPveStorageIscsidirectResource_MetadataAndSchema covers the
// resource's full type name and its schema contract.
func TestPveStorageIscsidirectResource_MetadataAndSchema(t *testing.T) {
	r := NewPveStorageIscsidirectResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveStorageIscsidirect {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveStorageIscsidirect)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	attrs := schemaResp.Schema.Attributes
	for _, key := range []string{
		"storage", "content", "nodes", "disable", "shared",
		"prune_backups", "max_protected_backups", "digest",
		"portal", "target", "nowritecache",
	} {
		if attrs[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	if !attrs["portal"].IsRequired() || !attrs["target"].IsRequired() {
		t.Fatal("portal and target should be Required")
	}
	if attrs["nowritecache"].IsRequired() || attrs["nowritecache"].IsComputed() {
		t.Fatal("nowritecache should be Optional")
	}
	for _, key := range []string{"storage", "portal", "target"} {
		sa, ok := attrs[key].(schema.StringAttribute)
		if !ok || len(sa.PlanModifiers) == 0 {
			t.Fatalf("%s should carry a RequiresReplace plan modifier", key)
		}
	}
}

// TestPveStorageIscsidirectResource_CreateAndDelete runs the create and
// delete paths for the iscsidirect type, including the 0/1 wire encoding
// of nowritecache on read-back.
func TestPveStorageIscsidirectResource_CreateAndDelete(t *testing.T) {
	exists := false
	r := NewPveStorageIscsidirectResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveStorageIscsidirectResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case req.Method == http.MethodPost && req.URL.Path == "/storage":
			body, _ := io.ReadAll(req.Body)
			for _, want := range []string{`"storage":"fast"`, `"type":"iscsidirect"`, `"portal":"192.168.1.10"`, `"target":"iqn.2000-01.com.example:fast.target0"`, `"nowritecache":true`} {
				if !strings.Contains(string(body), want) {
					t.Fatalf("create body missing %s: %q", want, body)
				}
			}
			exists = true
			_, _ = io.WriteString(w, `{"data":null}`)
		case req.Method == http.MethodGet && req.URL.Path == "/storage/fast":
			if !exists {
				w.WriteHeader(http.StatusNotFound)
				_, _ = io.WriteString(w, `{"errors":"storage 'fast' does not exist"}`)
				return
			}
			_, _ = io.WriteString(w, `{"data":{"storage":"fast","type":"iscsidirect",`+
				`"portal":"192.168.1.10","target":"iqn.2000-01.com.example:fast.target0",`+
				`"nowritecache":1,"content":"images","digest":"f00d8765"}}`)
		case req.Method == http.MethodDelete && req.URL.Path == "/storage/fast":
			exists = false
			_, _ = io.WriteString(w, `{"data":null}`)
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
		}
	})
	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	raw := storageIscsidirectTestObject(map[string]tftypes.Value{
		"storage":      tftypes.NewValue(tftypes.String, "fast"),
		"portal":       tftypes.NewValue(tftypes.String, "192.168.1.10"),
		"target":       tftypes.NewValue(tftypes.String, "iqn.2000-01.com.example:fast.target0"),
		"nowritecache": tftypes.NewValue(tftypes.Bool, true),
		"content":      storageNfsTestContentSet("images"),
	})

	createResp := &resource.CreateResponse{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(tftypes.Object{AttributeTypes: storageIscsidirectAttrTypes()}, nil)},
	}
	r.Create(ctx, resource.CreateRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw},
		Plan:   tfsdk.Plan{Schema: schemaResp.Schema, Raw: raw},
	}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create diagnostics: %s", diagnosticsError(createResp.Diagnostics))
	}
	var created pveStorageIscsidirectResourceModel
	if err := createResp.State.Get(ctx, &created); err != nil {
		t.Fatalf("State.Get after create: %v", err)
	}
	if created.Portal.ValueString() != "192.168.1.10" || created.Target.ValueString() != "iqn.2000-01.com.example:fast.target0" {
		t.Fatalf("created read-back fields = %+v", created)
	}
	if !created.NoWriteCache.ValueBool() {
		t.Fatal("nowritecache should read back true (0/1 wire encoding)")
	}
	if created.Digest.ValueString() != "f00d8765" {
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
