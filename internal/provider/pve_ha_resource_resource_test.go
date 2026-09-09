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
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// TestPveHaResourceResource_MetadataAndSchema covers the HA resource's
// type name and schema shape.
func TestPveHaResourceResource_MetadataAndSchema(t *testing.T) {
	r := NewPveHaResourceResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveHaResource {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveHaResource)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"sid", "type", "state", "group", "max_restart", "max_relocate", "failback", "auto_rebalance", "comment"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	if !schemaResp.Schema.Attributes["sid"].IsRequired() {
		t.Fatal("sid attribute should be Required")
	}
}

// haResourceAttrTypes returns the HA resource schema's attribute types.
func haResourceAttrTypes() map[string]tftypes.Type {
	return map[string]tftypes.Type{
		"sid":            tftypes.String,
		"type":           tftypes.String,
		"state":          tftypes.String,
		"group":          tftypes.String,
		"max_restart":    tftypes.Number,
		"max_relocate":   tftypes.Number,
		"failback":       tftypes.Bool,
		"auto_rebalance": tftypes.Bool,
		"comment":        tftypes.String,
	}
}

// TestPveHaResourceResource_CreateAndDelete runs create (POST then
// read-back GET) and delete (DELETE with purge) against a fake API.
func TestPveHaResourceResource_CreateAndDelete(t *testing.T) {
	exists := false
	var sawDeleteQuery string
	r := NewPveHaResourceResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveHaResourceResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case req.Method == http.MethodPost && req.URL.Path == "/cluster/ha/resources":
			body, _ := io.ReadAll(req.Body)
			if !strings.Contains(string(body), `"sid":"vm:100"`) {
				t.Fatalf("create body missing sid: %q", body)
			}
			exists = true
			_, _ = io.WriteString(w, `{"data":null}`)
		case req.Method == http.MethodGet && req.URL.Path == "/cluster/ha/resources/vm:100":
			if !exists {
				w.WriteHeader(http.StatusNotFound)
				_, _ = io.WriteString(w, `{"errors":"no such resource"}`)
				return
			}
			_, _ = io.WriteString(w, `{"data":{"sid":"vm:100","type":"vm","state":"started","max_restart":1,"max_relocate":1,"failback":1,"auto-rebalance":1}}`)
		case req.Method == http.MethodDelete && req.URL.Path == "/cluster/ha/resources/vm:100":
			sawDeleteQuery = req.URL.RawQuery
			exists = false
			_, _ = io.WriteString(w, `{"data":null}`)
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
		}
	})
	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)

	vals := map[string]tftypes.Value{
		"sid":            tftypes.NewValue(tftypes.String, "vm:100"),
		"type":           tftypes.NewValue(tftypes.String, nil),
		"state":          tftypes.NewValue(tftypes.String, "started"),
		"group":          tftypes.NewValue(tftypes.String, nil),
		"max_restart":    tftypes.NewValue(tftypes.Number, 2),
		"max_relocate":   tftypes.NewValue(tftypes.Number, nil),
		"failback":       tftypes.NewValue(tftypes.Bool, nil),
		"auto_rebalance": tftypes.NewValue(tftypes.Bool, nil),
		"comment":        tftypes.NewValue(tftypes.String, nil),
	}
	obj := tftypes.Object{AttributeTypes: haResourceAttrTypes()}
	raw := tftypes.NewValue(obj, vals)

	createResp := &resource.CreateResponse{State: tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(obj, nil)}}
	r.Create(ctx, resource.CreateRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw},
		Plan:   tfsdk.Plan{Schema: schemaResp.Schema, Raw: raw},
	}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create diagnostics: %s", diagnosticsError(createResp.Diagnostics))
	}
	var created pveHaResourceResourceModel
	if err := createResp.State.Get(ctx, &created); err != nil {
		t.Fatalf("State.Get after create: %v", err)
	}
	if created.Type.ValueString() != "vm" || created.State.ValueString() != "started" || !created.Failback.ValueBool() {
		t.Fatalf("created state = %+v", created)
	}
	if created.MaxRestart.ValueInt64() != 1 {
		t.Fatalf("max_restart = %v, want 1", created.MaxRestart)
	}

	deleteResp := &resource.DeleteResponse{}
	r.Delete(ctx, resource.DeleteRequest{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: createResp.State.Raw},
	}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("Delete diagnostics: %s", diagnosticsError(deleteResp.Diagnostics))
	}
	if !strings.Contains(sawDeleteQuery, "purge=true") {
		t.Fatalf("delete query = %q, want purge=true", sawDeleteQuery)
	}
}

// TestPveHaResourceResource_Read404Removes verifies Read drops the
// resource from state when the upstream resource is gone.
func TestPveHaResourceResource_Read404Removes(t *testing.T) {
	r := NewPveHaResourceResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveHaResourceResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"errors":"no such resource"}`)
	})
	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	vals := map[string]tftypes.Value{
		"sid":            tftypes.NewValue(tftypes.String, "vm:999"),
		"type":           tftypes.NewValue(tftypes.String, nil),
		"state":          tftypes.NewValue(tftypes.String, nil),
		"group":          tftypes.NewValue(tftypes.String, nil),
		"max_restart":    tftypes.NewValue(tftypes.Number, nil),
		"max_relocate":   tftypes.NewValue(tftypes.Number, nil),
		"failback":       tftypes.NewValue(tftypes.Bool, nil),
		"auto_rebalance": tftypes.NewValue(tftypes.Bool, nil),
		"comment":        tftypes.NewValue(tftypes.String, nil),
	}
	obj := tftypes.Object{AttributeTypes: haResourceAttrTypes()}
	state := tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(obj, vals)}
	readResp := &resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %s", diagnosticsError(readResp.Diagnostics))
	}
	if !readResp.State.Raw.IsNull() {
		t.Fatalf("expected state to be removed, got %v", readResp.State.Raw)
	}
}
