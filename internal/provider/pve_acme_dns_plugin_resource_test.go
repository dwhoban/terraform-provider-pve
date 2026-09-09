// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// TestPveAcmeDnsPluginResource_MetadataAndSchema covers the DNS plugin
// resource's type name and schema shape.
func TestPveAcmeDnsPluginResource_MetadataAndSchema(t *testing.T) {
	r := NewPveAcmeDnsPluginResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveAcmeDnsPlugin {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveAcmeDnsPlugin)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"plugin", "type", "api", "data", "disable", "nodes", "validation_delay", "digest"} {
		if _, ok := schemaResp.Schema.Attributes[key]; !ok {
			t.Fatalf("missing attribute %q", key)
		}
	}
	if !schemaResp.Schema.Attributes["plugin"].IsRequired() {
		t.Fatal("plugin attribute should be Required")
	}
	if !schemaResp.Schema.Attributes["data"].IsSensitive() {
		t.Fatal("data attribute should be Sensitive")
	}
	if !strings.Contains(schemaResp.Schema.Attributes["api"].GetMarkdownDescription(), "acmedns") {
		t.Fatal("api attribute description should mention acmedns")
	}
}

// acmeDnsPluginAttrTypes returns the plan/raw object type of the model.
func acmeDnsPluginAttrTypes() map[string]tftypes.Type {
	return map[string]tftypes.Type{
		"plugin":           tftypes.String,
		"type":             tftypes.String,
		"api":              tftypes.String,
		"data":             tftypes.String,
		"disable":          tftypes.Bool,
		"nodes":            tftypes.List{ElementType: tftypes.String},
		"validation_delay": tftypes.Number,
		"digest":           tftypes.String,
	}
}

// acmeDnsPluginPlanRaw builds a plan with the identifier set.
func acmeDnsPluginPlanRaw() tftypes.Value {
	vals := map[string]tftypes.Value{
		"plugin":           tftypes.NewValue(tftypes.String, "pdns"),
		"type":             tftypes.NewValue(tftypes.String, nil),
		"api":              tftypes.NewValue(tftypes.String, "pdns"),
		"data":             tftypes.NewValue(tftypes.String, nil),
		"disable":          tftypes.NewValue(tftypes.Bool, nil),
		"nodes":            tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, nil),
		"validation_delay": tftypes.NewValue(tftypes.Number, nil),
		"digest":           tftypes.NewValue(tftypes.String, nil),
	}
	return tftypes.NewValue(tftypes.Object{AttributeTypes: acmeDnsPluginAttrTypes()}, vals)
}

// TestPveAcmeDnsPluginResource_CreateAndDelete runs create (POST with the
// fixed dns type) and delete against a fake API.
func TestPveAcmeDnsPluginResource_CreateAndDelete(t *testing.T) {
	exists := false
	r := NewPveAcmeDnsPluginResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveAcmeDnsPluginResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case req.Method == http.MethodPost && req.URL.Path == "/cluster/acme/plugins":
			body, _ := io.ReadAll(req.Body)
			if !strings.Contains(string(body), `"type":"dns"`) || !strings.Contains(string(body), `"id":"pdns"`) {
				t.Fatalf("create body missing dns type or id: %q", body)
			}
			exists = true
			_, _ = io.WriteString(w, `{"data":null}`)
		case req.Method == http.MethodGet && req.URL.Path == "/cluster/acme/plugins/pdns":
			if !exists {
				w.WriteHeader(http.StatusNotFound)
				_, _ = fmt.Fprintf(w, `{"errors":"no such plugin"}`)
				return
			}
			_, _ = io.WriteString(w, `{"data":{"plugin":"pdns","type":"dns","api":"pdns","disable":false,"validation-delay":30,"digest":"d1"}}`)
		case req.Method == http.MethodDelete && req.URL.Path == "/cluster/acme/plugins/pdns":
			exists = false
			_, _ = io.WriteString(w, `{"data":null}`)
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
		}
	})
	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	raw := acmeDnsPluginPlanRaw()

	createResp := &resource.CreateResponse{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(tftypes.Object{AttributeTypes: acmeDnsPluginAttrTypes()}, nil)},
	}
	r.Create(ctx, resource.CreateRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw},
		Plan:   tfsdk.Plan{Schema: schemaResp.Schema, Raw: raw},
	}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create diagnostics: %s", diagnosticsError(createResp.Diagnostics))
	}
	var created pveAcmeDnsPluginResourceModel
	if err := createResp.State.Get(ctx, &created); err != nil {
		t.Fatalf("State.Get after create: %v", err)
	}
	if created.Type.ValueString() != "dns" || created.Digest.ValueString() != "d1" || created.ValidationDelay.ValueInt64() != 30 {
		t.Fatalf("created state = %+v", created)
	}

	deleteResp := &resource.DeleteResponse{}
	r.Delete(ctx, resource.DeleteRequest{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: createResp.State.Raw},
	}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("Delete diagnostics: %s", diagnosticsError(deleteResp.Diagnostics))
	}
}

// TestPveAcmeDnsPluginDataSource_Metadata asserts the data source type name.
func TestPveAcmeDnsPluginDataSource_Metadata(t *testing.T) {
	d := NewPveAcmeDnsPluginDataSource()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveAcmeDnsPlugin {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveAcmeDnsPlugin)
	}
}
