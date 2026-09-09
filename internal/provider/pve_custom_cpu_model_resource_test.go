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

// TestPveCustomCpuModelResource_MetadataAndSchema covers the custom CPU
// model resource's type name and schema shape.
func TestPveCustomCpuModelResource_MetadataAndSchema(t *testing.T) {
	r := NewPveCustomCPUModelResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveCustomCpuModel {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveCustomCpuModel)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"name", "reported_model", "flags", "guest_phys_bits", "hidden", "hv_vendor_id", "level", "phys_bits", "digest"} {
		if _, ok := schemaResp.Schema.Attributes[key]; !ok {
			t.Fatalf("missing attribute %q", key)
		}
	}
	if !schemaResp.Schema.Attributes["name"].IsRequired() {
		t.Fatal("name attribute should be Required")
	}
	if !schemaResp.Schema.Attributes["reported_model"].IsRequired() {
		t.Fatal("reported_model attribute should be Required")
	}
	if !strings.Contains(schemaResp.Schema.Attributes["reported_model"].GetMarkdownDescription(), "Skylake-Client") {
		t.Fatal("reported_model description should enumerate the pin's models")
	}
}

// TestPveCustomCpuModelResource_CreateAndDelete runs create and delete
// against a fake API, verifying the hyphenated wire keys.
func TestPveCustomCpuModelResource_CreateAndDelete(t *testing.T) {
	exists := false
	r := NewPveCustomCPUModelResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveCustomCPUModelResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case req.Method == http.MethodPost && req.URL.Path == "/cluster/qemu/custom-cpu-models":
			body, _ := io.ReadAll(req.Body)
			if !strings.Contains(string(body), `"cputype":"lab-cpu"`) || !strings.Contains(string(body), `"reported-model"`) {
				t.Fatalf("create body missing cputype or reported-model: %q", body)
			}
			exists = true
			_, _ = io.WriteString(w, `{"data":null}`)
		case req.Method == http.MethodGet && req.URL.Path == "/cluster/qemu/custom-cpu-models/lab-cpu":
			if !exists {
				w.WriteHeader(http.StatusNotFound)
				_, _ = fmt.Fprintf(w, `{"errors":"no such model"}`)
				return
			}
			_, _ = io.WriteString(w, `{"data":{"cputype":"lab-cpu","reported-model":"Skylake-Client","guest-phys-bits":40,"hidden":1,"level":30,"phys-bits":"host","digest":"d1"}}`)
		case req.Method == http.MethodDelete && req.URL.Path == "/cluster/qemu/custom-cpu-models/lab-cpu":
			exists = false
			_, _ = io.WriteString(w, `{"data":null}`)
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
		}
	})
	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	raw := backupRawFromSchema(ctx, t, schemaResp.Schema.Attributes, map[string]tftypes.Value{
		"name":           tftypes.NewValue(tftypes.String, "lab-cpu"),
		"reported_model": tftypes.NewValue(tftypes.String, "Skylake-Client"),
	})

	createResp := &resource.CreateResponse{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: backupRawFromSchema(ctx, t, schemaResp.Schema.Attributes, nil)},
	}
	r.Create(ctx, resource.CreateRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw},
		Plan:   tfsdk.Plan{Schema: schemaResp.Schema, Raw: raw},
	}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create diagnostics: %s", diagnosticsError(createResp.Diagnostics))
	}
	var created pveCustomCPUModelResourceModel
	if err := createResp.State.Get(ctx, &created); err != nil {
		t.Fatalf("State.Get after create: %v", err)
	}
	if created.Digest.ValueString() != "d1" || !created.Hidden.ValueBool() || created.PhysBits.ValueString() != "host" {
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

// TestPveCustomCpuModelDataSource_Metadata asserts the data source type
// name.
func TestPveCustomCpuModelDataSource_Metadata(t *testing.T) {
	d := NewPveCustomCPUModelDataSource()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveCustomCpuModel {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveCustomCpuModel)
	}
}
