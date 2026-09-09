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

// TestPveHaRuleResource_MetadataAndSchema covers the rule resource's type
// name and schema shape.
func TestPveHaRuleResource_MetadataAndSchema(t *testing.T) {
	r := NewPveHaRuleResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveHaRule {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveHaRule)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"rule", "type", "affinity", "nodes", "resources", "strict", "disable", "comment"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	if !schemaResp.Schema.Attributes["type"].IsRequired() || !schemaResp.Schema.Attributes["resources"].IsRequired() {
		t.Fatal("type and resources attributes should be Required")
	}
}

// haRuleAttrTypes returns the rule resource schema's attribute types.
func haRuleAttrTypes() map[string]tftypes.Type {
	return map[string]tftypes.Type{
		"rule":      tftypes.String,
		"type":      tftypes.String,
		"affinity":  tftypes.String,
		"nodes":     tftypes.List{ElementType: tftypes.String},
		"resources": tftypes.List{ElementType: tftypes.String},
		"strict":    tftypes.Bool,
		"disable":   tftypes.Bool,
		"comment":   tftypes.String,
	}
}

// haRuleList builds a list-of-string tftypes value from entries.
func haRuleList(entries ...string) tftypes.Value {
	vals := make([]tftypes.Value, 0, len(entries))
	for _, e := range entries {
		vals = append(vals, tftypes.NewValue(tftypes.String, e))
	}
	return tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, vals)
}

// haRuleConfig builds a plan/config raw object for the rule resource.
func haRuleConfig() tftypes.Value {
	vals := map[string]tftypes.Value{
		"rule":      tftypes.NewValue(tftypes.String, "r1"),
		"type":      tftypes.NewValue(tftypes.String, "node-affinity"),
		"affinity":  tftypes.NewValue(tftypes.String, nil),
		"nodes":     haRuleList("n1:2", "n2"),
		"resources": haRuleList("vm:100", "ct:101"),
		"strict":    tftypes.NewValue(tftypes.Bool, true),
		"disable":   tftypes.NewValue(tftypes.Bool, nil),
		"comment":   tftypes.NewValue(tftypes.String, nil),
	}
	return tftypes.NewValue(tftypes.Object{AttributeTypes: haRuleAttrTypes()}, vals)
}

// TestPveHaRuleResource_CreateAndDelete runs create (POST then read-back
// GET) and delete for a node-affinity rule against a fake API.
func TestPveHaRuleResource_CreateAndDelete(t *testing.T) {
	var sawCreateBody []byte
	r := NewPveHaRuleResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveHaRuleResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case req.Method == http.MethodPost && req.URL.Path == "/cluster/ha/rules":
			sawCreateBody, _ = io.ReadAll(req.Body)
			_, _ = io.WriteString(w, `{"data":null}`)
		case req.Method == http.MethodGet && req.URL.Path == "/cluster/ha/rules/r1":
			_, _ = io.WriteString(w, `{"data":{"rule":"r1","type":"node-affinity","affinity":"positive","nodes":"n1:2,n2","resources":"vm:100,ct:101","strict":1,"disable":0}}`)
		case req.Method == http.MethodDelete && req.URL.Path == "/cluster/ha/rules/r1":
			_, _ = io.WriteString(w, `{"data":null}`)
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
		}
	})
	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	raw := haRuleConfig()

	createResp := &resource.CreateResponse{State: tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(tftypes.Object{AttributeTypes: haRuleAttrTypes()}, nil)}}
	r.Create(ctx, resource.CreateRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw},
		Plan:   tfsdk.Plan{Schema: schemaResp.Schema, Raw: raw},
	}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create diagnostics: %s", diagnosticsError(createResp.Diagnostics))
	}
	body := string(sawCreateBody)
	if !strings.Contains(body, `"rule":"r1"`) || !strings.Contains(body, `"type":"node-affinity"`) ||
		!strings.Contains(body, `"nodes":"n1:2,n2"`) || !strings.Contains(body, `"resources":"vm:100,ct:101"`) {
		t.Fatalf("create body = %q", body)
	}

	var created pveHaRuleResourceModel
	if err := createResp.State.Get(ctx, &created); err != nil {
		t.Fatalf("State.Get after create: %v", err)
	}
	if created.Affinity.ValueString() != "positive" || len(created.Nodes.Elements()) != 2 || !created.Strict.ValueBool() {
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

// TestPveHaRuleResource_Read404Removes verifies Read drops the rule from
// state when the upstream rule is gone.
func TestPveHaRuleResource_Read404Removes(t *testing.T) {
	r := NewPveHaRuleResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveHaRuleResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"errors":"no such rule"}`)
	})
	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	vals := map[string]tftypes.Value{
		"rule":      tftypes.NewValue(tftypes.String, "gone"),
		"type":      tftypes.NewValue(tftypes.String, "node-affinity"),
		"affinity":  tftypes.NewValue(tftypes.String, nil),
		"nodes":     tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, nil),
		"resources": tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, nil),
		"strict":    tftypes.NewValue(tftypes.Bool, nil),
		"disable":   tftypes.NewValue(tftypes.Bool, nil),
		"comment":   tftypes.NewValue(tftypes.String, nil),
	}
	obj := tftypes.Object{AttributeTypes: haRuleAttrTypes()}
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

// TestPveHaRuleResource_ValidateConfig verifies the per-type requirements:
// node-affinity rules need nodes; resource-affinity rules need affinity
// and reject nodes/strict.
func TestPveHaRuleResource_ValidateConfig(t *testing.T) {
	r, ok := NewPveHaRuleResource().(*pveHaRuleResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)

	build := func(ruleType string, nodes, affinity, strict any) tftypes.Value {
		boolVal := func(v any) tftypes.Value {
			if v == nil {
				return tftypes.NewValue(tftypes.Bool, nil)
			}
			return tftypes.NewValue(tftypes.Bool, v)
		}
		strVal := func(v any) tftypes.Value {
			if v == nil {
				return tftypes.NewValue(tftypes.String, nil)
			}
			return tftypes.NewValue(tftypes.String, v)
		}
		nodesVal := tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, nil)
		if ns, ok := nodes.([]string); ok {
			nodesVal = haRuleList(ns...)
		}
		vals := map[string]tftypes.Value{
			"rule":      tftypes.NewValue(tftypes.String, "r1"),
			"type":      tftypes.NewValue(tftypes.String, ruleType),
			"affinity":  strVal(affinity),
			"nodes":     nodesVal,
			"resources": haRuleList("vm:100"),
			"strict":    boolVal(strict),
			"disable":   tftypes.NewValue(tftypes.Bool, nil),
			"comment":   tftypes.NewValue(tftypes.String, nil),
		}
		return tftypes.NewValue(tftypes.Object{AttributeTypes: haRuleAttrTypes()}, vals)
	}

	for _, tc := range []struct {
		name      string
		raw       tftypes.Value
		wantError bool
	}{
		{name: "node-affinity with nodes", raw: build("node-affinity", []string{"n1"}, nil, nil)},
		{name: "node-affinity without nodes", raw: build("node-affinity", nil, nil, nil), wantError: true},
		{name: "resource-affinity with affinity", raw: build("resource-affinity", nil, "positive", nil)},
		{name: "resource-affinity without affinity", raw: build("resource-affinity", nil, nil, nil), wantError: true},
		{name: "resource-affinity with nodes", raw: build("resource-affinity", []string{"n1"}, "positive", nil), wantError: true},
		{name: "resource-affinity with strict", raw: build("resource-affinity", nil, "positive", true), wantError: true},
	} {
		vcResp := &resource.ValidateConfigResponse{}
		r.ValidateConfig(ctx, resource.ValidateConfigRequest{
			Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: tc.raw},
		}, vcResp)
		if tc.wantError && !vcResp.Diagnostics.HasError() {
			t.Fatalf("%s: expected validation error", tc.name)
		}
		if !tc.wantError && vcResp.Diagnostics.HasError() {
			t.Fatalf("%s: unexpected diagnostics: %s", tc.name, diagnosticsError(vcResp.Diagnostics))
		}
	}
}
