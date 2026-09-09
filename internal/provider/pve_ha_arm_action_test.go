// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// TestPveHaArmAction_MetadataAndSchema covers the arm action's type name
// and schema shape.
func TestPveHaArmAction_MetadataAndSchema(t *testing.T) {
	a := NewPveHaArmAction()
	ctx := context.Background()
	metaResp := &action.MetadataResponse{}
	a.Metadata(ctx, action.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveHaArm {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveHaArm)
	}
	schemaResp := &action.SchemaResponse{}
	a.Schema(ctx, action.SchemaRequest{}, schemaResp)
	for _, key := range []string{"armed", "resource_mode"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
}

// haArmInvoke runs the arm action against client with the given armed flag
// and resource mode (empty string = null).
func haArmInvoke(t *testing.T, client any, armed bool, mode string) *action.InvokeResponse {
	t.Helper()
	a := &pveHaArmAction{}
	ctx := context.Background()
	cfgResp := &action.ConfigureResponse{}
	a.Configure(ctx, action.ConfigureRequest{ProviderData: client}, cfgResp)
	if cfgResp.Diagnostics.HasError() {
		t.Fatalf("Configure diagnostics: %s", diagnosticsError(cfgResp.Diagnostics))
	}
	schemaResp := &action.SchemaResponse{}
	a.Schema(ctx, action.SchemaRequest{}, schemaResp)

	attrTypes := map[string]tftypes.Type{"armed": tftypes.Bool, "resource_mode": tftypes.String}
	vals := map[string]tftypes.Value{
		"armed":         tftypes.NewValue(tftypes.Bool, armed),
		"resource_mode": tftypes.NewValue(tftypes.String, nil),
	}
	if mode != "" {
		vals["resource_mode"] = tftypes.NewValue(tftypes.String, mode)
	}
	raw := tftypes.NewValue(tftypes.Object{AttributeTypes: attrTypes}, vals)
	req := action.InvokeRequest{Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw}}
	resp := &action.InvokeResponse{}
	a.Invoke(ctx, req, resp)
	return resp
}

// TestPveHaArmAction_InvokeArm verifies armed=true POSTs the arm-ha
// endpoint with no body.
func TestPveHaArmAction_InvokeArm(t *testing.T) {
	var sawBody []byte
	client := newHaTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/cluster/ha/status/arm-ha" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		sawBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":null}`)
	})
	resp := haArmInvoke(t, client, true, "")
	if resp.Diagnostics.HasError() {
		t.Fatalf("Invoke diagnostics: %s", diagnosticsError(resp.Diagnostics))
	}
	if len(sawBody) != 0 {
		t.Fatalf("arm-ha must not carry a body, got %q", sawBody)
	}
}

// TestPveHaArmAction_InvokeDisarm verifies armed=false POSTs the disarm-ha
// endpoint, forwarding resource-mode.
func TestPveHaArmAction_InvokeDisarm(t *testing.T) {
	var sawBody []byte
	client := newHaTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/cluster/ha/status/disarm-ha" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		sawBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":null}`)
	})
	resp := haArmInvoke(t, client, false, "freeze")
	if resp.Diagnostics.HasError() {
		t.Fatalf("Invoke diagnostics: %s", diagnosticsError(resp.Diagnostics))
	}
	if !strings.Contains(string(sawBody), "freeze") {
		t.Fatalf("disarm body missing resource-mode freeze: %q", sawBody)
	}
}

// TestPveHaArmAction_InvokeArmWithModeRejected verifies resource_mode is
// rejected when arming (the arm-ha endpoint takes no parameters).
func TestPveHaArmAction_InvokeArmWithModeRejected(t *testing.T) {
	client := newHaTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("no request expected, got %s %s", r.Method, r.URL.Path)
	})
	resp := haArmInvoke(t, client, true, "freeze")
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected resource_mode-with-arm error")
	}
}
