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

// storageIscsiAttrTypes is the tftypes shape of the iscsi storage resource
// model.
func storageIscsiAttrTypes() map[string]tftypes.Type {
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
		"iscsiprovider":         tftypes.String,
	}
}

// storageIscsiTestObject builds a complete model object, nulling every
// attribute absent from vals.
func storageIscsiTestObject(vals map[string]tftypes.Value) tftypes.Value {
	attrTypes := storageIscsiAttrTypes()
	for name, typ := range attrTypes {
		if _, ok := vals[name]; !ok {
			vals[name] = tftypes.NewValue(typ, nil)
		}
	}
	return tftypes.NewValue(tftypes.Object{AttributeTypes: attrTypes}, vals)
}

// TestPveStorageIscsiResource_MetadataAndSchema covers the resource's full
// type name and its schema contract: portal and target are required with
// recreation plan modifiers, iscsiprovider is optional, and the digest is
// computed.
func TestPveStorageIscsiResource_MetadataAndSchema(t *testing.T) {
	r := NewPveStorageIscsiResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveStorageIscsi {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveStorageIscsi)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	attrs := schemaResp.Schema.Attributes
	for _, key := range []string{
		"storage", "content", "nodes", "disable", "shared",
		"prune_backups", "max_protected_backups", "digest",
		"portal", "target", "iscsiprovider",
	} {
		if attrs[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	if !attrs["portal"].IsRequired() || !attrs["target"].IsRequired() {
		t.Fatal("portal and target should be Required")
	}
	if attrs["iscsiprovider"].IsRequired() || attrs["iscsiprovider"].IsComputed() {
		t.Fatal("iscsiprovider should be Optional")
	}
	for _, key := range []string{"storage", "portal", "target", "iscsiprovider"} {
		sa, ok := attrs[key].(schema.StringAttribute)
		if !ok || len(sa.PlanModifiers) == 0 {
			t.Fatalf("%s should carry a RequiresReplace plan modifier", key)
		}
	}
}

// TestPveStorageIscsiResource_CreateAndDelete runs the create and delete
// paths for the iscsi type.
func TestPveStorageIscsiResource_CreateAndDelete(t *testing.T) {
	exists := false
	r := NewPveStorageIscsiResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveStorageIscsiResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case req.Method == http.MethodPost && req.URL.Path == "/storage":
			body, _ := io.ReadAll(req.Body)
			for _, want := range []string{`"storage":"san"`, `"type":"iscsi"`, `"portal":"192.168.1.10:3260"`, `"target":"iqn.2000-01.com.example:san.target0"`, `"iscsiprovider":"LIO"`} {
				if !strings.Contains(string(body), want) {
					t.Fatalf("create body missing %s: %q", want, body)
				}
			}
			exists = true
			_, _ = io.WriteString(w, `{"data":null}`)
		case req.Method == http.MethodGet && req.URL.Path == "/storage/san":
			if !exists {
				w.WriteHeader(http.StatusNotFound)
				_, _ = io.WriteString(w, `{"errors":"storage 'san' does not exist"}`)
				return
			}
			_, _ = io.WriteString(w, `{"data":{"storage":"san","type":"iscsi",`+
				`"portal":"192.168.1.10:3260","target":"iqn.2000-01.com.example:san.target0",`+
				`"iscsiprovider":"LIO","content":"images","disable":0,"digest":"dead4321"}}`)
		case req.Method == http.MethodDelete && req.URL.Path == "/storage/san":
			exists = false
			_, _ = io.WriteString(w, `{"data":null}`)
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
		}
	})
	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	raw := storageIscsiTestObject(map[string]tftypes.Value{
		"storage":       tftypes.NewValue(tftypes.String, "san"),
		"portal":        tftypes.NewValue(tftypes.String, "192.168.1.10:3260"),
		"target":        tftypes.NewValue(tftypes.String, "iqn.2000-01.com.example:san.target0"),
		"iscsiprovider": tftypes.NewValue(tftypes.String, "LIO"),
		"content":       storageNfsTestContentSet("images"),
	})

	createResp := &resource.CreateResponse{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(tftypes.Object{AttributeTypes: storageIscsiAttrTypes()}, nil)},
	}
	r.Create(ctx, resource.CreateRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw},
		Plan:   tfsdk.Plan{Schema: schemaResp.Schema, Raw: raw},
	}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create diagnostics: %s", diagnosticsError(createResp.Diagnostics))
	}
	var created pveStorageIscsiResourceModel
	if err := createResp.State.Get(ctx, &created); err != nil {
		t.Fatalf("State.Get after create: %v", err)
	}
	if created.Portal.ValueString() != "192.168.1.10:3260" || created.Target.ValueString() != "iqn.2000-01.com.example:san.target0" || created.ISCSIProvider.ValueString() != "LIO" {
		t.Fatalf("created read-back fields = %+v", created)
	}
	if created.Disable.ValueBool() {
		t.Fatal("disable should read back false")
	}
	if created.Digest.ValueString() != "dead4321" {
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
