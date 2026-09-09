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

// sdnFirewallOptionsAttrTypes maps the resource schema to Terraform types.
func sdnFirewallOptionsAttrTypes() map[string]tftypes.Type {
	return map[string]tftypes.Type{
		"id":                tftypes.String,
		"vnet":              tftypes.String,
		"enable":            tftypes.Bool,
		"log_level_forward": tftypes.String,
		"policy_forward":    tftypes.String,
	}
}

// TestPveSdnFirewallOptionsResource_MetadataAndSchema covers the resource's
// type name, the vnet key, and the closed forward policy set.
func TestPveSdnFirewallOptionsResource_MetadataAndSchema(t *testing.T) {
	r := NewPveSdnFirewallOptionsResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveSdnFirewallOptions {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveSdnFirewallOptions)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"id", "vnet", "enable", "log_level_forward", "policy_forward"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	if !schemaResp.Schema.Attributes["vnet"].IsRequired() {
		t.Fatal("vnet attribute should be Required")
	}
	id, ok := schemaResp.Schema.Attributes["id"].(schema.StringAttribute)
	if !ok {
		t.Fatal("id attribute is not a StringAttribute")
	}
	if !id.IsComputed() || id.IsOptional() {
		t.Fatal("id must be computed-only (singleton semantics)")
	}
	policyForward, ok := schemaResp.Schema.Attributes["policy_forward"].(schema.StringAttribute)
	if !ok {
		t.Fatal("policy_forward attribute is not a StringAttribute")
	}
	if !strings.Contains(policyForward.MarkdownDescription, "`DROP`") || strings.Contains(policyForward.MarkdownDescription, "`REJECT`") {
		t.Fatalf("policy_forward must enumerate ACCEPT and DROP without REJECT: %q", policyForward.MarkdownDescription)
	}
}

// TestPveSdnFirewallOptionsResource_CreateAndUpdate runs create and update
// against a fake API, asserting the vnet path and body key.
func TestPveSdnFirewallOptionsResource_CreateAndUpdate(t *testing.T) {
	r := NewPveSdnFirewallOptionsResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveSdnFirewallOptionsResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	var lastPath string
	var lastBody []byte
	var lastQuery string
	impl.client = newNodeNetworkTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case req.Method == http.MethodPut && req.URL.Path == "/cluster/sdn/vnets/vnet1/firewall/options":
			lastPath = req.URL.Path
			lastQuery = req.URL.RawQuery
			lastBody, _ = io.ReadAll(req.Body)
			_, _ = io.WriteString(w, `{"data":null}`)
		case req.Method == http.MethodGet && req.URL.Path == "/cluster/sdn/vnets/vnet1/firewall/options":
			_, _ = io.WriteString(w, `{"data":{"enable":1,"log_level_forward":"debug","policy_forward":"DROP"}}`)
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
		}
	})
	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)

	attrTypes := sdnFirewallOptionsAttrTypes()
	newVal := func(vals map[string]tftypes.Value) tftypes.Value {
		return tftypes.NewValue(tftypes.Object{AttributeTypes: attrTypes}, vals)
	}
	set := func(overrides map[string]tftypes.Value) map[string]tftypes.Value {
		vals := make(map[string]tftypes.Value, len(attrTypes))
		for name, at := range attrTypes {
			vals[name] = tftypes.NewValue(at, nil)
		}
		for name, v := range overrides {
			vals[name] = v
		}
		return vals
	}
	createVals := set(map[string]tftypes.Value{
		"vnet":   tftypes.NewValue(tftypes.String, "vnet1"),
		"enable": tftypes.NewValue(tftypes.Bool, true),
	})

	createResp := &resource.CreateResponse{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: newVal(set(nil))},
	}
	r.Create(ctx, resource.CreateRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: newVal(createVals)},
		Plan:   tfsdk.Plan{Schema: schemaResp.Schema, Raw: newVal(createVals)},
	}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create diagnostics: %s", diagnosticsError(createResp.Diagnostics))
	}
	var created pveSdnFirewallOptionsResourceModel
	if err := createResp.State.Get(ctx, &created); err != nil {
		t.Fatalf("State.Get after create: %v", err)
	}
	if created.ID.ValueString() != "vnet1" || !created.Enable.ValueBool() || created.PolicyForward.ValueString() != "DROP" {
		t.Fatalf("created state = %+v", created)
	}
	if !strings.Contains(string(lastBody), `"vnet":"vnet1"`) {
		t.Fatalf("create body missing vnet key: %q", lastBody)
	}

	// Update: policy_forward cleared in the plan must travel in the delete
	// query.
	updateVals := set(map[string]tftypes.Value{
		"vnet":   tftypes.NewValue(tftypes.String, "vnet1"),
		"enable": tftypes.NewValue(tftypes.Bool, true),
	})
	stateVals := set(map[string]tftypes.Value{
		"vnet":           tftypes.NewValue(tftypes.String, "vnet1"),
		"enable":         tftypes.NewValue(tftypes.Bool, true),
		"policy_forward": tftypes.NewValue(tftypes.String, "DROP"),
	})
	updateResp := &resource.UpdateResponse{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: createResp.State.Raw},
	}
	r.Update(ctx, resource.UpdateRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: newVal(updateVals)},
		Plan:   tfsdk.Plan{Schema: schemaResp.Schema, Raw: newVal(updateVals)},
		State:  tfsdk.State{Schema: schemaResp.Schema, Raw: newVal(stateVals)},
	}, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("Update diagnostics: %s", diagnosticsError(updateResp.Diagnostics))
	}
	if lastPath != "/cluster/sdn/vnets/vnet1/firewall/options" {
		t.Fatalf("update path = %q", lastPath)
	}
	if !strings.HasPrefix(lastQuery, "delete=") || !strings.Contains(lastQuery, "policy_forward") {
		t.Fatalf("update query = %q", lastQuery)
	}
}

// TestPveSdnFirewallOptionsResource_Read404Removes verifies Read drops the
// singleton from state when the vnet vanished out of band.
func TestPveSdnFirewallOptionsResource_Read404Removes(t *testing.T) {
	r := NewPveSdnFirewallOptionsResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveSdnFirewallOptionsResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	impl.client = newNodeNetworkTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"errors":"no such vnet"}`)
	})
	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	attrTypes := sdnFirewallOptionsAttrTypes()
	vals := make(map[string]tftypes.Value, len(attrTypes))
	for name, at := range attrTypes {
		vals[name] = tftypes.NewValue(at, nil)
	}
	vals["vnet"] = tftypes.NewValue(tftypes.String, "gone")
	vals["id"] = tftypes.NewValue(tftypes.String, "gone")
	state := tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(tftypes.Object{AttributeTypes: attrTypes}, vals)}
	readResp := &resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %s", diagnosticsError(readResp.Diagnostics))
	}
	if !readResp.State.Raw.IsNull() {
		t.Fatalf("expected state to be removed, got %v", readResp.State.Raw)
	}
}
