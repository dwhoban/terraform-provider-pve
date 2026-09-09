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

// TestPveNodeAptUpdate_MetadataAndSchema covers the apt update action's type
// name and schema shape.
func TestPveNodeAptUpdate_MetadataAndSchema(t *testing.T) {
	a := NewPveNodeAptUpdateAction()
	ctx := context.Background()
	metaResp := &action.MetadataResponse{}
	a.Metadata(ctx, action.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveNodeAptUpdate {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveNodeAptUpdate)
	}
	schemaResp := &action.SchemaResponse{}
	a.Schema(ctx, action.SchemaRequest{}, schemaResp)
	for _, key := range []string{"node", "notify", "quiet"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
}

// aptUpdateInvoke runs the apt update action with the supplied config
// against client.
func aptUpdateInvoke(t *testing.T, client any, node string, notify, quiet *bool) *action.InvokeResponse {
	t.Helper()
	a := &pveNodeAptUpdateAction{}
	ctx := context.Background()
	cfgResp := &action.ConfigureResponse{}
	a.Configure(ctx, action.ConfigureRequest{ProviderData: client}, cfgResp)
	if cfgResp.Diagnostics.HasError() {
		t.Fatalf("Configure diagnostics: %s", diagnosticsError(cfgResp.Diagnostics))
	}
	schemaResp := &action.SchemaResponse{}
	a.Schema(ctx, action.SchemaRequest{}, schemaResp)
	attrTypes := map[string]tftypes.Type{"node": tftypes.String, "notify": tftypes.Bool, "quiet": tftypes.Bool}
	vals := map[string]tftypes.Value{
		"node":   tftypes.NewValue(tftypes.String, node),
		"notify": tftypes.NewValue(tftypes.Bool, nil),
		"quiet":  tftypes.NewValue(tftypes.Bool, nil),
	}
	if notify != nil {
		vals["notify"] = tftypes.NewValue(tftypes.Bool, *notify)
	}
	if quiet != nil {
		vals["quiet"] = tftypes.NewValue(tftypes.Bool, *quiet)
	}
	req := action.InvokeRequest{Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: tftypes.NewValue(tftypes.Object{AttributeTypes: attrTypes}, vals)}}
	resp := &action.InvokeResponse{}
	a.Invoke(ctx, req, resp)
	return resp
}

// TestPveNodeAptUpdate_InvokeWaitsForTask verifies the action POSTs
// /apt/update with the quiet flag and waits for the resulting task.
func TestPveNodeAptUpdate_InvokeWaitsForTask(t *testing.T) {
	upid := "UPID:pve1:00004321:12345678:aptupdate:root@pam:"
	var sawBody []byte
	sawStatus := false
	client := newHaTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/nodes/pve1/apt/update":
			sawBody, _ = io.ReadAll(r.Body)
			_, _ = io.WriteString(w, `{"data":"`+upid+`"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/nodes/pve1/tasks/"+upid+"/status":
			sawStatus = true
			_, _ = io.WriteString(w, `{"data":{"status":"stopped","exitstatus":"OK"}}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	quiet := true
	resp := aptUpdateInvoke(t, client, "pve1", nil, &quiet)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Invoke diagnostics: %s", diagnosticsError(resp.Diagnostics))
	}
	if !strings.Contains(string(sawBody), `"quiet":true`) {
		t.Fatalf("body missing quiet flag: %q", sawBody)
	}
	if !sawStatus {
		t.Fatal("action did not wait for the worker task status")
	}
}

// TestPveNodeAptUpdate_InvokeNullFlagsOmitsBody verifies unset flags send
// an empty JSON object rather than explicit nulls.
func TestPveNodeAptUpdate_InvokeNullFlagsOmitsBody(t *testing.T) {
	upid := "UPID:pve1:00005678:12345678:aptupdate:root@pam:"
	var sawBody []byte
	client := newHaTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/nodes/pve1/apt/update":
			sawBody, _ = io.ReadAll(r.Body)
			_, _ = io.WriteString(w, `{"data":"`+upid+`"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/nodes/pve1/tasks/"+upid+"/status":
			_, _ = io.WriteString(w, `{"data":{"status":"stopped","exitstatus":"OK"}}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	resp := aptUpdateInvoke(t, client, "pve1", nil, nil)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Invoke diagnostics: %s", diagnosticsError(resp.Diagnostics))
	}
	if body := string(sawBody); body != "{}" {
		t.Fatalf("body should be an empty JSON object, got %q", body)
	}
}
