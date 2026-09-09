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

// TestPveMappingDirResource_MetadataAndSchema covers the directory mapping
// resource's type name and schema shape.
func TestPveMappingDirResource_MetadataAndSchema(t *testing.T) {
	r := NewPveMappingDirResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveMappingDir {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveMappingDir)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"id", "description", "map"} {
		if _, ok := schemaResp.Schema.Attributes[key]; !ok {
			t.Fatalf("missing attribute %q", key)
		}
	}
	if !schemaResp.Schema.Attributes["id"].IsRequired() {
		t.Fatal("id attribute should be Required")
	}
}

// TestPveMappingDirResource_CreateAndDelete runs create and delete against
// a fake API, verifying entries are sent as property strings and the
// computed view is refreshed from the pin's property-string response.
func TestPveMappingDirResource_CreateAndDelete(t *testing.T) {
	exists := false
	r := NewPveMappingDirResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveMappingDirResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case req.Method == http.MethodPost && req.URL.Path == "/cluster/mapping/dir":
			body, _ := io.ReadAll(req.Body)
			if !strings.Contains(string(body), `node=pve1,path=/mnt/share`) {
				t.Fatalf("create body missing property-string entry: %q", body)
			}
			exists = true
			_, _ = io.WriteString(w, `{"data":null}`)
		case req.Method == http.MethodGet && req.URL.Path == "/cluster/mapping/dir/share":
			if !exists {
				w.WriteHeader(http.StatusNotFound)
				_, _ = fmt.Fprintf(w, `{"errors":"no such mapping"}`)
				return
			}
			_, _ = io.WriteString(w, `{"data":{"id":"share","description":"media dirs","map":["node=pve1,path=/mnt/share","node=pve2,path=/mnt/share2"]}}`)
		case req.Method == http.MethodDelete && req.URL.Path == "/cluster/mapping/dir/share":
			exists = false
			_, _ = io.WriteString(w, `{"data":null}`)
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
		}
	})
	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	listType, ok := schemaResp.Schema.Attributes["map"].GetType().TerraformType(ctx).(tftypes.List)
	if !ok {
		t.Fatal("map attribute type is not a list")
	}
	objType, ok := listType.ElementType.(tftypes.Object)
	if !ok {
		t.Fatal("map element type is not an object")
	}
	entry := map[string]tftypes.Value{}
	for name, at := range objType.AttributeTypes {
		entry[name] = tftypes.NewValue(at, nil)
	}
	entry["node"] = tftypes.NewValue(tftypes.String, "pve1")
	entry["path"] = tftypes.NewValue(tftypes.String, "/mnt/share")
	raw := backupRawFromSchema(ctx, t, schemaResp.Schema.Attributes, map[string]tftypes.Value{
		"id":  tftypes.NewValue(tftypes.String, "share"),
		"map": tftypes.NewValue(listType, []tftypes.Value{tftypes.NewValue(objType, entry)}),
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
	var created pveMappingDirResourceModel
	if err := createResp.State.Get(ctx, &created); err != nil {
		t.Fatalf("State.Get after create: %v", err)
	}
	if created.Description.ValueString() != "media dirs" || len(created.Map) != 2 || created.Map[1].Path.ValueString() != "/mnt/share2" {
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

// TestPveMappingDirDataSource_Metadata asserts the data source type name.
func TestPveMappingDirDataSource_Metadata(t *testing.T) {
	d := NewPveMappingDirDataSource()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveMappingDir {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveMappingDir)
	}
}
