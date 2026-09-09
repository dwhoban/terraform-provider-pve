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

// TestPveContainerRebootAction_MetadataAndSchema covers the reboot action's
// type name and schema shape.
func TestPveContainerRebootAction_MetadataAndSchema(t *testing.T) {
	a := NewPveContainerRebootAction()
	ctx := context.Background()
	metaResp := &action.MetadataResponse{}
	a.Metadata(ctx, action.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveContainerReboot {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveContainerReboot)
	}
	schemaResp := &action.SchemaResponse{}
	a.Schema(ctx, action.SchemaRequest{}, schemaResp)
	for _, key := range []string{"node", "vmid", "timeout"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
}

// containerRebootInvoke runs the reboot action against client with the given
// timeout (nil omits it).
func containerRebootInvoke(t *testing.T, client any, timeout any) *action.InvokeResponse {
	t.Helper()
	a := &pveContainerRebootAction{}
	ctx := context.Background()
	cfgResp := &action.ConfigureResponse{}
	a.Configure(ctx, action.ConfigureRequest{ProviderData: client}, cfgResp)
	if cfgResp.Diagnostics.HasError() {
		t.Fatalf("Configure diagnostics: %s", diagnosticsError(cfgResp.Diagnostics))
	}
	schemaResp := &action.SchemaResponse{}
	a.Schema(ctx, action.SchemaRequest{}, schemaResp)
	attrTypes := map[string]tftypes.Type{
		"node":    tftypes.String,
		"vmid":    tftypes.Number,
		"timeout": tftypes.Number,
	}
	raw := tftypes.NewValue(tftypes.Object{AttributeTypes: attrTypes}, map[string]tftypes.Value{
		"node":    tftypes.NewValue(tftypes.String, "pve1"),
		"vmid":    tftypes.NewValue(tftypes.Number, 100),
		"timeout": tftypes.NewValue(tftypes.Number, timeout),
	})
	req := action.InvokeRequest{Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw}}
	resp := &action.InvokeResponse{}
	a.Invoke(ctx, req, resp)
	return resp
}

// TestPveContainerRebootAction_InvokePostsReboot verifies the action POSTs
// to the reboot endpoint, forwards the timeout, and waits for the task.
func TestPveContainerRebootAction_InvokePostsReboot(t *testing.T) {
	upid := "UPID:pve1:000000A1:abcdef01:lxcreboot:root@pam:"
	var captured map[string]any
	sawStatus := false
	client := newHaTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/nodes/pve1/lxc/100/status/reboot":
			_ = json.NewDecoder(r.Body).Decode(&captured)
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"data":"`+upid+`"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/nodes/pve1/tasks/"+upid+"/status":
			sawStatus = true
			_, _ = io.WriteString(w, `{"data":{"status":"stopped","exitstatus":"OK"}}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	resp := containerRebootInvoke(t, client, 30)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Invoke diagnostics: %s", diagnosticsError(resp.Diagnostics))
	}
	if captured["timeout"] != float64(30) {
		t.Fatalf("unexpected body: %+v", captured)
	}
	if !sawStatus {
		t.Fatal("task status was never polled")
	}
}

// TestPveContainerRebootAction_InvokeWithoutTimeoutOmitsKey verifies the
// timeout key is absent when unset.
func TestPveContainerRebootAction_InvokeWithoutTimeoutOmitsKey(t *testing.T) {
	var captured map[string]any
	client := newHaTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/nodes/pve1/lxc/100/status/reboot":
			_ = json.NewDecoder(r.Body).Decode(&captured)
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"data":"UPID:pve1:1:2:lxcreboot:root@pam:"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/nodes/pve1/tasks/UPID:pve1:1:2:lxcreboot:root@pam:/status":
			_, _ = io.WriteString(w, `{"data":{"status":"stopped","exitstatus":"OK"}}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	resp := containerRebootInvoke(t, client, nil)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Invoke diagnostics: %s", diagnosticsError(resp.Diagnostics))
	}
	if _, ok := captured["timeout"]; ok {
		t.Fatalf("timeout should be omitted, got: %+v", captured)
	}
}

// TestPveContainerRebootAction_InvokeWithoutClient verifies the action
// surfaces a diagnostic instead of panicking when the client is missing.
func TestPveContainerRebootAction_InvokeWithoutClient(t *testing.T) {
	resp := containerRebootInvoke(t, nil, nil)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error diagnostic for unconfigured client")
	}
}
