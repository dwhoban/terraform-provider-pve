// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// TestPveNodeShutdownAction_MetadataAndSchema covers the shutdown action's
// type name and schema shape.
func TestPveNodeShutdownAction_MetadataAndSchema(t *testing.T) {
	a := NewPveNodeShutdownAction()
	ctx := context.Background()
	metaResp := &action.MetadataResponse{}
	a.Metadata(ctx, action.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveNodeShutdown {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveNodeShutdown)
	}
	schemaResp := &action.SchemaResponse{}
	a.Schema(ctx, action.SchemaRequest{}, schemaResp)
	if schemaResp.Schema.Attributes["node"] == nil {
		t.Fatal("schema missing node attribute")
	}
}

// nodeShutdownInvoke runs the shutdown action against client with node pve1.
func nodeShutdownInvoke(t *testing.T, client any) *action.InvokeResponse {
	t.Helper()
	a := &pveNodeShutdownAction{}
	ctx := context.Background()
	cfgResp := &action.ConfigureResponse{}
	a.Configure(ctx, action.ConfigureRequest{ProviderData: client}, cfgResp)
	if cfgResp.Diagnostics.HasError() {
		t.Fatalf("Configure diagnostics: %s", diagnosticsError(cfgResp.Diagnostics))
	}
	schemaResp := &action.SchemaResponse{}
	a.Schema(ctx, action.SchemaRequest{}, schemaResp)
	attrTypes := map[string]tftypes.Type{"node": tftypes.String}
	raw := tftypes.NewValue(tftypes.Object{AttributeTypes: attrTypes}, map[string]tftypes.Value{
		"node": tftypes.NewValue(tftypes.String, "pve1"),
	})
	req := action.InvokeRequest{Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw}}
	resp := &action.InvokeResponse{}
	a.Invoke(ctx, req, resp)
	return resp
}

// TestPveNodeShutdownAction_InvokePostsCommand verifies the action POSTs the
// shutdown command to /nodes/{node}/status and accepts the null response.
func TestPveNodeShutdownAction_InvokePostsCommand(t *testing.T) {
	var captured map[string]string
	client := newHaTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/nodes/pve1/status" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&captured)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":null}`)
	})
	resp := nodeShutdownInvoke(t, client)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Invoke diagnostics: %s", diagnosticsError(resp.Diagnostics))
	}
	if captured["command"] != "shutdown" {
		t.Fatalf("unexpected body: %+v", captured)
	}
}

// TestPveNodeShutdownAction_InvokeWithoutClient verifies the action surfaces
// a diagnostic instead of panicking when the provider client is missing.
func TestPveNodeShutdownAction_InvokeWithoutClient(t *testing.T) {
	resp := nodeShutdownInvoke(t, nil)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error diagnostic for unconfigured client")
	}
}
