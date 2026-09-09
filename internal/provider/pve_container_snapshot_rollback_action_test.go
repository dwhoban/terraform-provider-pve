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

// TestPveContainerSnapshotRollbackAction_MetadataAndSchema covers the
// rollback action's type name and schema shape.
func TestPveContainerSnapshotRollbackAction_MetadataAndSchema(t *testing.T) {
	a := NewPveContainerSnapshotRollbackAction()
	ctx := context.Background()
	metaResp := &action.MetadataResponse{}
	a.Metadata(ctx, action.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveContainerSnapshotRollback {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveContainerSnapshotRollback)
	}
	schemaResp := &action.SchemaResponse{}
	a.Schema(ctx, action.SchemaRequest{}, schemaResp)
	for _, key := range []string{"node", "vmid", "name", "start"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
}

// containerSnapshotRollbackInvoke runs the rollback action against client
// with the given start flag.
func containerSnapshotRollbackInvoke(t *testing.T, client any, start any) *action.InvokeResponse {
	t.Helper()
	a := &pveContainerSnapshotRollbackAction{}
	ctx := context.Background()
	cfgResp := &action.ConfigureResponse{}
	a.Configure(ctx, action.ConfigureRequest{ProviderData: client}, cfgResp)
	if cfgResp.Diagnostics.HasError() {
		t.Fatalf("Configure diagnostics: %s", diagnosticsError(cfgResp.Diagnostics))
	}
	schemaResp := &action.SchemaResponse{}
	a.Schema(ctx, action.SchemaRequest{}, schemaResp)
	attrTypes := map[string]tftypes.Type{
		"node":  tftypes.String,
		"vmid":  tftypes.Number,
		"name":  tftypes.String,
		"start": tftypes.Bool,
	}
	raw := tftypes.NewValue(tftypes.Object{AttributeTypes: attrTypes}, map[string]tftypes.Value{
		"node":  tftypes.NewValue(tftypes.String, "pve1"),
		"vmid":  tftypes.NewValue(tftypes.Number, 100),
		"name":  tftypes.NewValue(tftypes.String, "pre-upgrade"),
		"start": tftypes.NewValue(tftypes.Bool, start),
	})
	req := action.InvokeRequest{Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw}}
	resp := &action.InvokeResponse{}
	a.Invoke(ctx, req, resp)
	return resp
}

// TestPveContainerSnapshotRollbackAction_InvokeRollsBack verifies the action
// POSTs to the rollback endpoint with the start flag and waits for the task.
func TestPveContainerSnapshotRollbackAction_InvokeRollsBack(t *testing.T) {
	upid := "UPID:pve1:000000F7:abcdef07:lxcmroll:root@pam:"
	var captured map[string]any
	sawStatus := false
	client := newHaTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/nodes/pve1/lxc/100/snapshot/pre-upgrade/rollback":
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
	resp := containerSnapshotRollbackInvoke(t, client, true)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Invoke diagnostics: %s", diagnosticsError(resp.Diagnostics))
	}
	if captured["start"] != true {
		t.Fatalf("unexpected body: %+v", captured)
	}
	if !sawStatus {
		t.Fatal("task status was never polled")
	}
}

// TestPveContainerSnapshotRollbackAction_InvokeWithoutStartOmitsKey verifies
// the start key is absent when unset (PVE default false).
func TestPveContainerSnapshotRollbackAction_InvokeWithoutStartOmitsKey(t *testing.T) {
	var captured map[string]any
	client := newHaTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/nodes/pve1/lxc/100/snapshot/pre-upgrade/rollback":
			_ = json.NewDecoder(r.Body).Decode(&captured)
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"data":"UPID:pve1:1:2:lxcmroll:root@pam:"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/nodes/pve1/tasks/UPID:pve1:1:2:lxcmroll:root@pam:/status":
			_, _ = io.WriteString(w, `{"data":{"status":"stopped","exitstatus":"OK"}}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	resp := containerSnapshotRollbackInvoke(t, client, nil)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Invoke diagnostics: %s", diagnosticsError(resp.Diagnostics))
	}
	if _, ok := captured["start"]; ok {
		t.Fatalf("start should be omitted, got: %+v", captured)
	}
}

// TestPveContainerSnapshotRollbackAction_InvokeWithoutClient verifies the
// action surfaces a diagnostic when the client is missing.
func TestPveContainerSnapshotRollbackAction_InvokeWithoutClient(t *testing.T) {
	resp := containerSnapshotRollbackInvoke(t, nil, nil)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error diagnostic for unconfigured client")
	}
}
