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

// TestPveSdnApplyAction_MetadataAndSchema covers the apply action's type
// name and schema shape.
func TestPveSdnApplyAction_MetadataAndSchema(t *testing.T) {
	a := NewPveSdnApplyAction()
	ctx := context.Background()
	metaResp := &action.MetadataResponse{}
	a.Metadata(ctx, action.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveSdnApply {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveSdnApply)
	}
	schemaResp := &action.SchemaResponse{}
	a.Schema(ctx, action.SchemaRequest{}, schemaResp)
	for _, key := range []string{"lock_token", "release_lock"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
}

// sdnApplyInvoke runs the apply action against client with the given
// options (nil values omit the attributes).
func sdnApplyInvoke(t *testing.T, client any, lockToken any, releaseLock any) *action.InvokeResponse {
	t.Helper()
	a := &pveSdnApplyAction{}
	ctx := context.Background()
	cfgResp := &action.ConfigureResponse{}
	a.Configure(ctx, action.ConfigureRequest{ProviderData: client}, cfgResp)
	if cfgResp.Diagnostics.HasError() {
		t.Fatalf("Configure diagnostics: %s", diagnosticsError(cfgResp.Diagnostics))
	}
	schemaResp := &action.SchemaResponse{}
	a.Schema(ctx, action.SchemaRequest{}, schemaResp)
	attrTypes := map[string]tftypes.Type{
		"lock_token":   tftypes.String,
		"release_lock": tftypes.Bool,
	}
	raw := tftypes.NewValue(tftypes.Object{AttributeTypes: attrTypes}, map[string]tftypes.Value{
		"lock_token":   tftypes.NewValue(tftypes.String, lockToken),
		"release_lock": tftypes.NewValue(tftypes.Bool, releaseLock),
	})
	req := action.InvokeRequest{Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw}}
	resp := &action.InvokeResponse{}
	a.Invoke(ctx, req, resp)
	return resp
}

// TestPveSdnApplyAction_InvokePutsSdn verifies the action PUTs /cluster/sdn
// with an empty body by default and waits for the reload task.
func TestPveSdnApplyAction_InvokePutsSdn(t *testing.T) {
	upid := "UPID:pve1:00000001:00000001:sdnreload:root@pam:"
	sawMethod := ""
	sawBody := []byte{}
	sawStatus := false
	client := newHaTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/cluster/sdn":
			sawMethod = r.Method
			sawBody, _ = io.ReadAll(r.Body)
			_, _ = io.WriteString(w, `{"data":"`+upid+`"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/nodes/pve1/tasks/"+upid+"/status":
			sawStatus = true
			_, _ = io.WriteString(w, `{"data":{"status":"stopped","exitstatus":"OK"}}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	resp := sdnApplyInvoke(t, client, nil, nil)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Invoke diagnostics: %s", diagnosticsError(resp.Diagnostics))
	}
	if sawMethod != http.MethodPut || len(sawBody) != 0 {
		t.Fatalf("apply request = %s body %q, want PUT with no body", sawMethod, sawBody)
	}
	if !sawStatus {
		t.Fatal("task status was never polled")
	}
}

// TestPveSdnApplyAction_InvokeForwardsLockOptions verifies lock_token and
// release_lock travel in the body.
func TestPveSdnApplyAction_InvokeForwardsLockOptions(t *testing.T) {
	upid := "UPID:pve1:2:3:sdnreload:root@pam:"
	var captured map[string]any
	client := newHaTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/cluster/sdn":
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &captured)
			_, _ = io.WriteString(w, `{"data":"`+upid+`"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/nodes/pve1/tasks/"+upid+"/status":
			_, _ = io.WriteString(w, `{"data":{"status":"stopped","exitstatus":"OK"}}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	resp := sdnApplyInvoke(t, client, "tok", false)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Invoke diagnostics: %s", diagnosticsError(resp.Diagnostics))
	}
	if captured["lock-token"] != "tok" || captured["release-lock"] != false {
		t.Fatalf("body = %+v", captured)
	}
}

// TestPveSdnApplyAction_InvokeWithoutClient verifies the action surfaces a
// diagnostic instead of panicking when the client is missing.
func TestPveSdnApplyAction_InvokeWithoutClient(t *testing.T) {
	resp := sdnApplyInvoke(t, nil, nil, nil)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error diagnostic for unconfigured client")
	}
}
