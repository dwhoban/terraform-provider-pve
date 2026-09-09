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

// TestPveContainerMigrateAction_MetadataAndSchema covers the migrate
// action's type name and schema shape.
func TestPveContainerMigrateAction_MetadataAndSchema(t *testing.T) {
	a := NewPveContainerMigrateAction()
	ctx := context.Background()
	metaResp := &action.MetadataResponse{}
	a.Metadata(ctx, action.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveContainerMigrate {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveContainerMigrate)
	}
	schemaResp := &action.SchemaResponse{}
	a.Schema(ctx, action.SchemaRequest{}, schemaResp)
	for _, key := range []string{"node", "vmid", "target", "target_storage", "online", "restart", "bwlimit", "timeout"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
}

// containerMigrateInvoke runs the migrate action against client with node
// pve1, vmid 100, and target pve2.
func containerMigrateInvoke(t *testing.T, client any) *action.InvokeResponse {
	t.Helper()
	a := &pveContainerMigrateAction{}
	ctx := context.Background()
	cfgResp := &action.ConfigureResponse{}
	a.Configure(ctx, action.ConfigureRequest{ProviderData: client}, cfgResp)
	if cfgResp.Diagnostics.HasError() {
		t.Fatalf("Configure diagnostics: %s", diagnosticsError(cfgResp.Diagnostics))
	}
	schemaResp := &action.SchemaResponse{}
	a.Schema(ctx, action.SchemaRequest{}, schemaResp)
	attrTypes := map[string]tftypes.Type{
		"node":           tftypes.String,
		"vmid":           tftypes.Number,
		"target":         tftypes.String,
		"target_storage": tftypes.String,
		"online":         tftypes.Bool,
		"restart":        tftypes.Bool,
		"bwlimit":        tftypes.Number,
		"timeout":        tftypes.Number,
	}
	raw := tftypes.NewValue(tftypes.Object{AttributeTypes: attrTypes}, map[string]tftypes.Value{
		"node":           tftypes.NewValue(tftypes.String, "pve1"),
		"vmid":           tftypes.NewValue(tftypes.Number, 100),
		"target":         tftypes.NewValue(tftypes.String, "pve2"),
		"target_storage": tftypes.NewValue(tftypes.String, "1"),
		"online":         tftypes.NewValue(tftypes.Bool, true),
		"restart":        tftypes.NewValue(tftypes.Bool, nil),
		"bwlimit":        tftypes.NewValue(tftypes.Number, 5000),
		"timeout":        tftypes.NewValue(tftypes.Number, 60),
	})
	req := action.InvokeRequest{Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw}}
	resp := &action.InvokeResponse{}
	a.Invoke(ctx, req, resp)
	return resp
}

// TestPveContainerMigrateAction_InvokeMigratesAndWaits verifies the action
// POSTs the migrate body to the source node and waits for the task there.
func TestPveContainerMigrateAction_InvokeMigratesAndWaits(t *testing.T) {
	upid := "UPID:pve1:000000D4:abcdef04:lxcmigrate:root@pam:"
	var captured map[string]any
	sawStatus := false
	client := newHaTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/nodes/pve1/lxc/100/migrate":
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
	resp := containerMigrateInvoke(t, client)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Invoke diagnostics: %s", diagnosticsError(resp.Diagnostics))
	}
	if captured["target"] != "pve2" || captured["target-storage"] != "1" || captured["online"] != true || captured["bwlimit"] != float64(5000) || captured["timeout"] != float64(60) {
		t.Fatalf("unexpected body: %+v", captured)
	}
	if _, ok := captured["restart"]; ok {
		t.Fatalf("null restart should be omitted, got: %+v", captured)
	}
	if !sawStatus {
		t.Fatal("task status was never polled on the source node")
	}
}

// TestPveContainerMigrateAction_InvokeWithoutClient verifies the action
// surfaces a diagnostic when the client is missing.
func TestPveContainerMigrateAction_InvokeWithoutClient(t *testing.T) {
	resp := containerMigrateInvoke(t, nil)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error diagnostic for unconfigured client")
	}
}
