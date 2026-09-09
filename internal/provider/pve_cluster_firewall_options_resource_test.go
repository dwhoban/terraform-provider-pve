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

// clusterFirewallOptionsAttrTypes maps the resource schema to Terraform
// types.
func clusterFirewallOptionsAttrTypes() map[string]tftypes.Type {
	return map[string]tftypes.Type{
		"id":             tftypes.String,
		"ebtables":       tftypes.Bool,
		"enable":         tftypes.Number,
		"log_ratelimit":  tftypes.String,
		"policy_forward": tftypes.String,
		"policy_in":      tftypes.String,
		"policy_out":     tftypes.String,
	}
}

// clusterFirewallOptionsVals builds an attribute value map with the given
// overrides and every other attribute null.
func clusterFirewallOptionsVals(overrides map[string]tftypes.Value) map[string]tftypes.Value {
	vals := make(map[string]tftypes.Value, len(clusterFirewallOptionsAttrTypes()))
	for name, at := range clusterFirewallOptionsAttrTypes() {
		vals[name] = tftypes.NewValue(at, nil)
	}
	for name, v := range overrides {
		vals[name] = v
	}
	return vals
}

// TestPveClusterFirewallOptionsResource_MetadataAndSchema covers the
// resource's type name, singleton id semantics, and closed-set/range
// descriptions.
func TestPveClusterFirewallOptionsResource_MetadataAndSchema(t *testing.T) {
	r := NewPveClusterFirewallOptionsResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveClusterFirewallOptions {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveClusterFirewallOptions)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"id", "ebtables", "enable", "log_ratelimit", "policy_forward", "policy_in", "policy_out"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	id, ok := schemaResp.Schema.Attributes["id"].(schema.StringAttribute)
	if !ok {
		t.Fatal("id attribute is not a StringAttribute")
	}
	if !id.IsComputed() || id.IsOptional() {
		t.Fatal("id must be computed-only (singleton semantics)")
	}
	policyIn, ok := schemaResp.Schema.Attributes["policy_in"].(schema.StringAttribute)
	if !ok {
		t.Fatal("policy_in attribute is not a StringAttribute")
	}
	if !strings.Contains(policyIn.MarkdownDescription, "REJECT") || !strings.Contains(policyIn.MarkdownDescription, "Must be one of") {
		t.Fatalf("policy_in description must enumerate the closed set: %q", policyIn.MarkdownDescription)
	}
	enable, ok := schemaResp.Schema.Attributes["enable"].(schema.Int64Attribute)
	if !ok {
		t.Fatal("enable attribute is not an Int64Attribute")
	}
	if !strings.Contains(enable.MarkdownDescription, "Must be at least 0") {
		t.Fatalf("enable description must state the range: %q", enable.MarkdownDescription)
	}
	policyForward, ok := schemaResp.Schema.Attributes["policy_forward"].(schema.StringAttribute)
	if !ok {
		t.Fatal("policy_forward attribute is not a StringAttribute")
	}
	if strings.Contains(policyForward.MarkdownDescription, "`REJECT`") {
		t.Fatalf("policy_forward must not offer REJECT per the pin: %q", policyForward.MarkdownDescription)
	}
}

// TestPveClusterFirewallOptionsResource_CreateUpdateRead drives create
// (PUT then read-back GET) and update (PUT with the delete query parameter)
// against a fake API.
func TestPveClusterFirewallOptionsResource_CreateUpdateRead(t *testing.T) {
	r := NewPveClusterFirewallOptionsResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveClusterFirewallOptionsResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	var lastQuery string
	impl.client = newNodeNetworkTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case req.Method == http.MethodPut && req.URL.Path == "/cluster/firewall/options":
			lastQuery = req.URL.RawQuery
			body, _ := io.ReadAll(req.Body)
			if !strings.Contains(string(body), `"policy_forward":"ACCEPT"`) {
				t.Fatalf("create body missing policy_forward: %q", body)
			}
			_, _ = io.WriteString(w, `{"data":null}`)
		case req.Method == http.MethodGet && req.URL.Path == "/cluster/firewall/options":
			_, _ = io.WriteString(w, `{"data":{"ebtables":1,"enable":1,"log_ratelimit":"enable=1,burst=5,rate=1/second","policy_forward":"ACCEPT","policy_in":"REJECT"}}`)
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
		}
	})
	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)

	attrTypes := clusterFirewallOptionsAttrTypes()
	newVal := func(vals map[string]tftypes.Value) tftypes.Value {
		return tftypes.NewValue(tftypes.Object{AttributeTypes: attrTypes}, vals)
	}
	createVals := clusterFirewallOptionsVals(map[string]tftypes.Value{
		"ebtables":       tftypes.NewValue(tftypes.Bool, true),
		"enable":         tftypes.NewValue(tftypes.Number, 1),
		"policy_forward": tftypes.NewValue(tftypes.String, "ACCEPT"),
		"policy_in":      tftypes.NewValue(tftypes.String, "REJECT"),
	})

	createResp := &resource.CreateResponse{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: newVal(clusterFirewallOptionsVals(nil))},
	}
	r.Create(ctx, resource.CreateRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: newVal(createVals)},
		Plan:   tfsdk.Plan{Schema: schemaResp.Schema, Raw: newVal(createVals)},
	}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create diagnostics: %s", diagnosticsError(createResp.Diagnostics))
	}
	var created pveClusterFirewallOptionsResourceModel
	if err := createResp.State.Get(ctx, &created); err != nil {
		t.Fatalf("State.Get after create: %v", err)
	}
	if created.ID.ValueString() != "cluster" || !created.Ebitables.ValueBool() || created.Enable.ValueInt64() != 1 {
		t.Fatalf("created state = %+v", created)
	}
	if created.PolicyIn.ValueString() != "REJECT" {
		t.Fatalf("read-back decode failed: %+v", created)
	}

	// Update: policy_in cleared in the plan must travel in the delete query.
	updateVals := clusterFirewallOptionsVals(map[string]tftypes.Value{
		"ebtables":       tftypes.NewValue(tftypes.Bool, true),
		"enable":         tftypes.NewValue(tftypes.Number, 0),
		"policy_forward": tftypes.NewValue(tftypes.String, "ACCEPT"),
	})
	updateResp := &resource.UpdateResponse{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: createResp.State.Raw},
	}
	r.Update(ctx, resource.UpdateRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: newVal(updateVals)},
		Plan:   tfsdk.Plan{Schema: schemaResp.Schema, Raw: newVal(updateVals)},
		State:  tfsdk.State{Schema: schemaResp.Schema, Raw: createResp.State.Raw},
	}, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("Update diagnostics: %s", diagnosticsError(updateResp.Diagnostics))
	}
	if !strings.HasPrefix(lastQuery, "delete=") || !strings.Contains(lastQuery, "policy_in") {
		t.Fatalf("update query = %q", lastQuery)
	}
}

// TestPveClusterFirewallOptionsImportID pins the singleton import
// semantics: the identifier is always "cluster".
func TestPveClusterFirewallOptionsImportID(t *testing.T) {
	if pveClusterFirewallOptionsID != "cluster" {
		t.Fatalf("pveClusterFirewallOptionsID = %q, want cluster", pveClusterFirewallOptionsID)
	}
}
