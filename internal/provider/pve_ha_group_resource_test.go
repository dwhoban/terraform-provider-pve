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

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// TestPveHaGroupResource_MetadataAndSchema covers the group resource's
// type name and schema shape.
func TestPveHaGroupResource_MetadataAndSchema(t *testing.T) {
	r := NewPveHaGroupResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveHaGroup {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveHaGroup)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"group", "nodes", "restricted", "nofailback", "comment"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	if !schemaResp.Schema.Attributes["nodes"].IsRequired() {
		t.Fatal("nodes attribute should be Required")
	}
}
func haGroupPlanRaw() tftypes.Value {
	vals := map[string]tftypes.Value{
		"group": tftypes.NewValue(tftypes.String, "g1"),
		"nodes": tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, []tftypes.Value{
			tftypes.NewValue(tftypes.String, "n1:2"),
			tftypes.NewValue(tftypes.String, "n2"),
		}),
		"restricted": tftypes.NewValue(tftypes.Bool, true),
		"nofailback": tftypes.NewValue(tftypes.Bool, nil),
		"comment":    tftypes.NewValue(tftypes.String, nil),
	}
	return tftypes.NewValue(tftypes.Object{AttributeTypes: haGroupAttrTypes()}, vals)
}
func haGroupAttrTypes() map[string]tftypes.Type {
	return map[string]tftypes.Type{
		"group":      tftypes.String,
		"nodes":      tftypes.List{ElementType: tftypes.String},
		"restricted": tftypes.Bool,
		"nofailback": tftypes.Bool,
		"comment":    tftypes.String,
	}
}

// TestPveHaGroupResource_CreateAndDelete runs the full create (POST then
// read-back GET) and delete (DELETE) paths against a fake API, including
// the already-absent delete-is-success rule.
func TestPveHaGroupResource_CreateAndDelete(t *testing.T) {
	exists := false
	r := NewPveHaGroupResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveHaGroupResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case req.Method == http.MethodPost && req.URL.Path == "/cluster/ha/groups":
			body, _ := io.ReadAll(req.Body)
			if !strings.Contains(string(body), `"nodes":"n1:2,n2"`) {
				t.Fatalf("create body missing joined nodes: %q", body)
			}
			exists = true
			_, _ = io.WriteString(w, `{"data":null}`)
		case req.Method == http.MethodGet && req.URL.Path == "/cluster/ha/groups/g1":
			if !exists {
				w.WriteHeader(http.StatusNotFound)
				_, _ = io.WriteString(w, `{"errors":"no such ha group"}`)
				return
			}
			_, _ = io.WriteString(w, `{"data":{"group":"g1","nodes":"n1:2,n2","restricted":1,"nofailback":0}}`)
		case req.Method == http.MethodDelete && req.URL.Path == "/cluster/ha/groups/g1":
			exists = false
			_, _ = io.WriteString(w, `{"data":null}`)
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
		}
	})
	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	raw := haGroupPlanRaw()

	createResp := &resource.CreateResponse{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(tftypes.Object{AttributeTypes: haGroupAttrTypes()}, nil)},
	}
	r.Create(ctx, resource.CreateRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw},
		Plan:   tfsdk.Plan{Schema: schemaResp.Schema, Raw: raw},
	}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create diagnostics: %s", diagnosticsError(createResp.Diagnostics))
	}
	var created pveHaGroupResourceModel
	if err := createResp.State.Get(ctx, &created); err != nil {
		t.Fatalf("State.Get after create: %v", err)
	}
	if created.Group.ValueString() != "g1" || len(created.Nodes.Elements()) != 2 || !created.Restricted.ValueBool() {
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

// TestPveHaGroupResource_Read404Removes verifies Read drops the resource
// from state when the group vanished out of band.
func TestPveHaGroupResource_Read404Removes(t *testing.T) {
	r := NewPveHaGroupResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveHaGroupResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprintf(w, `{"errors":"no such ha group"}`)
	})
	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	vals := map[string]tftypes.Value{
		"group":      tftypes.NewValue(tftypes.String, "gone"),
		"nodes":      tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, nil),
		"restricted": tftypes.NewValue(tftypes.Bool, nil),
		"nofailback": tftypes.NewValue(tftypes.Bool, nil),
		"comment":    tftypes.NewValue(tftypes.String, nil),
	}
	state := tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(tftypes.Object{AttributeTypes: haGroupAttrTypes()}, vals)}
	readResp := &resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %s", diagnosticsError(readResp.Diagnostics))
	}
	if !readResp.State.Raw.IsNull() {
		t.Fatalf("expected state to be removed, got %v", readResp.State.Raw)
	}
}
