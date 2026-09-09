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

// TestPveHardwareMappingPciResource_MetadataAndSchema covers the PCI
// mapping resource's type name and schema shape.
func TestPveHardwareMappingPciResource_MetadataAndSchema(t *testing.T) {
	r := NewPveHardwareMappingPciResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveHardwareMappingPci {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveHardwareMappingPci)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"id", "description", "mdev", "live_migration_capable", "map"} {
		if _, ok := schemaResp.Schema.Attributes[key]; !ok {
			t.Fatalf("missing attribute %q", key)
		}
	}
	if !schemaResp.Schema.Attributes["id"].IsRequired() {
		t.Fatal("id attribute should be Required")
	}
	if !schemaResp.Schema.Attributes["map"].IsRequired() {
		t.Fatal("map attribute should be Required")
	}
}

// pciPlanRaw builds a plan carrying one populated per-node entry.
func pciPlanRaw(ctx context.Context, t *testing.T, schemaResp *resource.SchemaResponse) tftypes.Value {
	t.Helper()
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
	entry["id"] = tftypes.NewValue(tftypes.String, "10de:2231")
	return backupRawFromSchema(ctx, t, schemaResp.Schema.Attributes, map[string]tftypes.Value{
		"id":  tftypes.NewValue(tftypes.String, "gpu"),
		"map": tftypes.NewValue(listType, []tftypes.Value{tftypes.NewValue(objType, entry)}),
	})
}

// TestPveHardwareMappingPciResource_CreateAndDelete runs create and delete
// against a fake API, verifying entries are sent as property strings and
// the computed view is refreshed from the pin's property-string response.
func TestPveHardwareMappingPciResource_CreateAndDelete(t *testing.T) {
	exists := false
	r := NewPveHardwareMappingPciResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveHardwareMappingPciResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case req.Method == http.MethodPost && req.URL.Path == "/cluster/mapping/pci":
			body, _ := io.ReadAll(req.Body)
			if !strings.Contains(string(body), `node=pve1,id=10de:2231`) {
				t.Fatalf("create body missing property-string entry: %q", body)
			}
			exists = true
			_, _ = io.WriteString(w, `{"data":null}`)
		case req.Method == http.MethodGet && req.URL.Path == "/cluster/mapping/pci/gpu":
			if !exists {
				w.WriteHeader(http.StatusNotFound)
				_, _ = fmt.Fprintf(w, `{"errors":"no such mapping"}`)
				return
			}
			_, _ = io.WriteString(w, `{"data":{"id":"gpu","description":"host GPU","mdev":1,"live-migration-capable":0,"map":["node=pve1,id=10de:2231,iommugroup=14,path=0000:01:00.0"]}}`)
		case req.Method == http.MethodDelete && req.URL.Path == "/cluster/mapping/pci/gpu":
			exists = false
			_, _ = io.WriteString(w, `{"data":null}`)
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
		}
	})
	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	raw := pciPlanRaw(ctx, t, schemaResp)

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
	var created pveHardwareMappingPciResourceModel
	if err := createResp.State.Get(ctx, &created); err != nil {
		t.Fatalf("State.Get after create: %v", err)
	}
	if created.Description.ValueString() != "host GPU" || !created.Mdev.ValueBool() || len(created.Map) != 1 {
		t.Fatalf("created state = %+v", created)
	}
	if created.Map[0].IommuGroup.ValueInt64() != 14 || created.Map[0].Path.ValueString() != "0000:01:00.0" {
		t.Fatalf("created map entry = %+v", created.Map[0])
	}

	deleteResp := &resource.DeleteResponse{}
	r.Delete(ctx, resource.DeleteRequest{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: createResp.State.Raw},
	}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("Delete diagnostics: %s", diagnosticsError(deleteResp.Diagnostics))
	}
}

// TestPveHardwareMappingPciDataSource_Metadata asserts the data source type
// name.
func TestPveHardwareMappingPciDataSource_Metadata(t *testing.T) {
	d := NewPveHardwareMappingPciDataSource()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveHardwareMappingPci {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveHardwareMappingPci)
	}
}
