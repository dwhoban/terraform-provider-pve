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

// TestPveNotificationTestAction_MetadataAndSchema covers the test
// notification action's type name and schema shape.
func TestPveNotificationTestAction_MetadataAndSchema(t *testing.T) {
	a := NewPveNotificationTestAction()
	ctx := context.Background()
	metaResp := &action.MetadataResponse{}
	a.Metadata(ctx, action.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveNotificationTest {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveNotificationTest)
	}
	schemaResp := &action.SchemaResponse{}
	a.Schema(ctx, action.SchemaRequest{}, schemaResp)
	if schemaResp.Schema.Attributes["target"] == nil {
		t.Fatal("schema missing target attribute")
	}
	if !schemaResp.Schema.Attributes["target"].IsRequired() {
		t.Fatal("target attribute should be Required")
	}
}

// notificationTestInvoke runs the action against client with the given
// target name.
func notificationTestInvoke(t *testing.T, client any, target string) *action.InvokeResponse {
	t.Helper()
	a := &pveNotificationTestAction{}
	ctx := context.Background()
	cfgResp := &action.ConfigureResponse{}
	a.Configure(ctx, action.ConfigureRequest{ProviderData: client}, cfgResp)
	if cfgResp.Diagnostics.HasError() {
		t.Fatalf("Configure diagnostics: %s", diagnosticsError(cfgResp.Diagnostics))
	}
	schemaResp := &action.SchemaResponse{}
	a.Schema(ctx, action.SchemaRequest{}, schemaResp)
	raw := tftypes.NewValue(tftypes.Object{AttributeTypes: map[string]tftypes.Type{"target": tftypes.String}},
		map[string]tftypes.Value{"target": tftypes.NewValue(tftypes.String, target)})
	req := action.InvokeRequest{Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw}}
	resp := &action.InvokeResponse{}
	a.Invoke(ctx, req, resp)
	return resp
}

// TestPveNotificationTestAction_Invoke verifies the action POSTs the test
// endpoint of the named target with no body.
func TestPveNotificationTestAction_Invoke(t *testing.T) {
	var sawBody []byte
	client := newHaTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/cluster/notifications/targets/mail-to-root/test" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		sawBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":null}`)
	})
	resp := notificationTestInvoke(t, client, "mail-to-root")
	if resp.Diagnostics.HasError() {
		t.Fatalf("Invoke diagnostics: %s", diagnosticsError(resp.Diagnostics))
	}
	if len(sawBody) != 0 {
		t.Fatalf("test endpoint must not carry a body, got %q", sawBody)
	}
}

// TestPveNotificationTestAction_InvokeErrorSurfaces verifies an upstream
// failure produces an error diagnostic naming the operation.
func TestPveNotificationTestAction_InvokeErrorSurfaces(t *testing.T) {
	client := newHaTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `{"errors":"sendmail binary not found"}`)
	})
	resp := notificationTestInvoke(t, client, "mail-to-root")
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error diagnostic")
	}
	if diags := diagnosticsError(resp.Diagnostics); !strings.Contains(diags, "sending a test notification through mail-to-root") {
		t.Fatalf("diagnostics should name operation and target, got: %s", diags)
	}
}
