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

// vmSnapshotAttrTypes returns the plan/raw object type of the snapshot
// resource model.
func vmSnapshotAttrTypes() map[string]tftypes.Type {
	return map[string]tftypes.Type{
		"node":        tftypes.String,
		"vmid":        tftypes.Number,
		"name":        tftypes.String,
		"description": tftypes.String,
		"snaptime":    tftypes.Number,
		"parent":      tftypes.String,
		"vmstate":     tftypes.Bool,
	}
}

// vmSnapshotPlanRaw builds a plan with the identifiers and description set.
func vmSnapshotPlanRaw() tftypes.Value {
	vals := map[string]tftypes.Value{
		"node":        tftypes.NewValue(tftypes.String, "pve1"),
		"vmid":        tftypes.NewValue(tftypes.Number, 100),
		"name":        tftypes.NewValue(tftypes.String, "snap1"),
		"description": tftypes.NewValue(tftypes.String, "before upgrade"),
		"snaptime":    tftypes.NewValue(tftypes.Number, nil),
		"parent":      tftypes.NewValue(tftypes.String, nil),
		"vmstate":     tftypes.NewValue(tftypes.Bool, nil),
	}
	return tftypes.NewValue(tftypes.Object{AttributeTypes: vmSnapshotAttrTypes()}, vals)
}

// TestPveVmSnapshotResource_MetadataAndSchema covers the snapshot
// resource's type name and schema shape.
func TestPveVmSnapshotResource_MetadataAndSchema(t *testing.T) {
	r := NewPveVmSnapshotResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveVmSnapshot {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveVmSnapshot)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"node", "vmid", "name", "description", "snaptime", "parent", "vmstate"} {
		if _, ok := schemaResp.Schema.Attributes[key]; !ok {
			t.Fatalf("missing attribute %q", key)
		}
	}
	if !schemaResp.Schema.Attributes["name"].IsRequired() {
		t.Fatal("name attribute should be Required")
	}
}

// TestPveVmSnapshotResource_CreateAndDelete runs create (POST plus task
// wait) and delete (DELETE plus task wait) against a fake API.
func TestPveVmSnapshotResource_CreateAndDelete(t *testing.T) {
	snapshotExists := false
	r := NewPveVmSnapshotResource()
	impl, ok := r.(*pveVmSnapshotResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case req.Method == http.MethodPost && req.URL.Path == "/nodes/pve1/qemu/100/snapshot":
			bodyBytes, _ := io.ReadAll(req.Body)
			body := string(bodyBytes)
			if !strings.Contains(body, `"snapname":"snap1"`) || !strings.Contains(body, `"description":"before upgrade"`) {
				t.Fatalf("create body = %q", body)
			}
			snapshotExists = true
			_, _ = fmt.Fprintf(w, `{"data":%q}`, vmTestUpid)
		case req.Method == http.MethodDelete && req.URL.Path == "/nodes/pve1/qemu/100/snapshot/snap1":
			snapshotExists = false
			_, _ = fmt.Fprintf(w, `{"data":%q}`, vmTestUpid)
		case req.Method == http.MethodGet && req.URL.Path == "/nodes/pve1/qemu/100/snapshot/snap1":
			if !snapshotExists {
				w.WriteHeader(http.StatusNotFound)
				_, _ = io.WriteString(w, `{"errors":"no such snapshot"}`)
				return
			}
			_, _ = io.WriteString(w, `{"data":{"description":"before upgrade","snaptime":1700000000,"vmstate":false}}`)
		case req.Method == http.MethodGet && strings.HasPrefix(req.URL.Path, "/nodes/pve1/tasks/"):
			_, _ = io.WriteString(w, `{"data":{"status":"stopped","exitstatus":"OK"}}`)
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
		}
	})
	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)

	createResp := &resource.CreateResponse{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(tftypes.Object{AttributeTypes: vmSnapshotAttrTypes()}, nil)},
	}
	r.Create(ctx, resource.CreateRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: vmSnapshotPlanRaw()},
		Plan:   tfsdk.Plan{Schema: schemaResp.Schema, Raw: vmSnapshotPlanRaw()},
	}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create diagnostics: %s", diagnosticsError(createResp.Diagnostics))
	}
	var created pveVmSnapshotResourceModel
	if err := createResp.State.Get(ctx, &created); err != nil {
		t.Fatalf("State.Get after create: %v", err)
	}
	if created.Snaptime.ValueInt64() != 1700000000 {
		t.Fatalf("created state = %+v", created)
	}

	// Mark the snapshot present only after create's GET ran; simulate by
	// flipping on before delete's read of absence is irrelevant, so keep it
	// present to prove Read-style decode already covered above.
	snapshotExists = true
	deleteResp := &resource.DeleteResponse{}
	r.Delete(ctx, resource.DeleteRequest{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: createResp.State.Raw},
	}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("Delete diagnostics: %s", diagnosticsError(deleteResp.Diagnostics))
	}
}

// TestPveVmSnapshotDataSource_Metadata asserts the data source type name
// and required keys.
func TestPveVmSnapshotDataSource_Metadata(t *testing.T) {
	d := NewPveVmSnapshotDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveVmSnapshot {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveVmSnapshot)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"node", "vmid", "name", "description", "snaptime", "parent", "vmstate"} {
		if _, ok := schemaResp.Schema.Attributes[key]; !ok {
			t.Fatalf("missing attribute %q", key)
		}
	}
}
