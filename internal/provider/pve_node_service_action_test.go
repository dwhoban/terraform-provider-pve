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

// TestPveNodeService_MetadataAndSchema covers the node service action's
// type name and schema shape.
func TestPveNodeService_MetadataAndSchema(t *testing.T) {
	a := NewPveNodeServiceAction()
	ctx := context.Background()
	metaResp := &action.MetadataResponse{}
	a.Metadata(ctx, action.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveNodeService {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveNodeService)
	}
	schemaResp := &action.SchemaResponse{}
	a.Schema(ctx, action.SchemaRequest{}, schemaResp)
	for _, key := range []string{"node", "service", "operation"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	if !schemaResp.Schema.Attributes["operation"].IsRequired() {
		t.Fatal("operation attribute should be Required")
	}
}

// nodeServiceInvoke runs the node service action with the supplied config
// against client.
func nodeServiceInvoke(t *testing.T, client any, node, service, operation string) *action.InvokeResponse {
	t.Helper()
	a := &pveNodeServiceAction{}
	ctx := context.Background()
	cfgResp := &action.ConfigureResponse{}
	a.Configure(ctx, action.ConfigureRequest{ProviderData: client}, cfgResp)
	if cfgResp.Diagnostics.HasError() {
		t.Fatalf("Configure diagnostics: %s", diagnosticsError(cfgResp.Diagnostics))
	}
	schemaResp := &action.SchemaResponse{}
	a.Schema(ctx, action.SchemaRequest{}, schemaResp)
	attrTypes := map[string]tftypes.Type{"node": tftypes.String, "service": tftypes.String, "operation": tftypes.String}
	raw := tftypes.NewValue(tftypes.Object{AttributeTypes: attrTypes}, map[string]tftypes.Value{
		"node":      tftypes.NewValue(tftypes.String, node),
		"service":   tftypes.NewValue(tftypes.String, service),
		"operation": tftypes.NewValue(tftypes.String, operation),
	})
	req := action.InvokeRequest{Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw}}
	resp := &action.InvokeResponse{}
	a.Invoke(ctx, req, resp)
	return resp
}

// TestPveNodeService_InvokeRestartsAndWaitForTask verifies the action POSTs
// the pinned verb path and waits on the resulting task until it reports OK.
func TestPveNodeService_InvokeRestartsAndWaitForTask(t *testing.T) {
	upid := "UPID:pve1:00001234:12345678:svcrestart:root@pam:"
	sawStatus := false
	client := newHaTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/nodes/pve1/services/pveproxy/restart":
			_, _ = io.WriteString(w, `{"data":"`+upid+`"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/nodes/pve1/tasks/"+upid+"/status":
			sawStatus = true
			_, _ = io.WriteString(w, `{"data":{"status":"stopped","exitstatus":"OK"}}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	resp := nodeServiceInvoke(t, client, "pve1", "pveproxy", "restart")
	if resp.Diagnostics.HasError() {
		t.Fatalf("Invoke diagnostics: %s", diagnosticsError(resp.Diagnostics))
	}
	if !sawStatus {
		t.Fatal("action did not wait for the worker task status")
	}
}

// TestPveNodeService_InvokeTaskFailureSurfaces verifies a failed worker task
// produces an error diagnostic.
func TestPveNodeService_InvokeTaskFailureSurfaces(t *testing.T) {
	upid := "UPID:pve1:00001234:12345678:svcrestart:root@pam:"
	client := newHaTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/nodes/pve1/services/pveproxy/restart":
			_, _ = io.WriteString(w, `{"data":"`+upid+`"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/nodes/pve1/tasks/"+upid+"/status":
			_, _ = io.WriteString(w, `{"data":{"status":"stopped","exitstatus":"command failed"}}`)
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/nodes/pve1/tasks/"+upid+"/log"):
			_, _ = io.WriteString(w, `{"data":["unit is masked"]}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	resp := nodeServiceInvoke(t, client, "pve1", "pveproxy", "restart")
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error diagnostics for failed task")
	}
}

// TestPveNodeService_NodeFromUpid covers the UPID node extraction shared by
// the node services and APT actions.
func TestPveNodeService_NodeFromUpid(t *testing.T) {
	got, err := nodeSvcAptNodeFromUpid("UPID:pve2:0001:0002:aptupdate:root@pam:")
	if err != nil || got != "pve2" {
		t.Fatalf("got %q, %v", got, err)
	}
	if _, err := nodeSvcAptNodeFromUpid("not-a-upid"); err == nil {
		t.Fatal("expected error for malformed UPID")
	}
}
