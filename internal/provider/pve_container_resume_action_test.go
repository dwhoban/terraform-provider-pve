// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// TestPveContainerResumeAction_MetadataAndSchema covers the resume action's
// type name and schema shape.
func TestPveContainerResumeAction_MetadataAndSchema(t *testing.T) {
	a := NewPveContainerResumeAction()
	ctx := context.Background()
	metaResp := &action.MetadataResponse{}
	a.Metadata(ctx, action.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveContainerResume {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveContainerResume)
	}
	schemaResp := &action.SchemaResponse{}
	a.Schema(ctx, action.SchemaRequest{}, schemaResp)
	for _, key := range []string{"node", "vmid"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
}

// containerResumeInvoke runs the resume action against client with node
// pve1 and vmid 100.
func containerResumeInvoke(t *testing.T, client any) *action.InvokeResponse {
	t.Helper()
	a := &pveContainerResumeAction{}
	ctx := context.Background()
	cfgResp := &action.ConfigureResponse{}
	a.Configure(ctx, action.ConfigureRequest{ProviderData: client}, cfgResp)
	if cfgResp.Diagnostics.HasError() {
		t.Fatalf("Configure diagnostics: %s", diagnosticsError(cfgResp.Diagnostics))
	}
	schemaResp := &action.SchemaResponse{}
	a.Schema(ctx, action.SchemaRequest{}, schemaResp)
	attrTypes := map[string]tftypes.Type{"node": tftypes.String, "vmid": tftypes.Number}
	raw := tftypes.NewValue(tftypes.Object{AttributeTypes: attrTypes}, map[string]tftypes.Value{
		"node": tftypes.NewValue(tftypes.String, "pve1"),
		"vmid": tftypes.NewValue(tftypes.Number, 100),
	})
	req := action.InvokeRequest{Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw}}
	resp := &action.InvokeResponse{}
	a.Invoke(ctx, req, resp)
	return resp
}

// TestPveContainerResumeAction_InvokeResumesAndWaits verifies the action
// POSTs to the resume endpoint and waits for the task UPID.
func TestPveContainerResumeAction_InvokeResumesAndWaits(t *testing.T) {
	upid := "UPID:pve1:000000C3:abcdef03:lxcresume:root@pam:"
	sawResume, sawStatus := false, false
	client := newHaTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/nodes/pve1/lxc/100/status/resume":
			sawResume = true
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"data":"`+upid+`"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/nodes/pve1/tasks/"+upid+"/status":
			sawStatus = true
			_, _ = io.WriteString(w, `{"data":{"status":"stopped","exitstatus":"OK"}}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	resp := containerResumeInvoke(t, client)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Invoke diagnostics: %s", diagnosticsError(resp.Diagnostics))
	}
	if !sawResume || !sawStatus {
		t.Fatalf("resume=%v status=%v, want both true", sawResume, sawStatus)
	}
}

// TestPveContainerResumeAction_InvokeWithoutClient verifies the action
// surfaces a diagnostic when the client is missing.
func TestPveContainerResumeAction_InvokeWithoutClient(t *testing.T) {
	resp := containerResumeInvoke(t, nil)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error diagnostic for unconfigured client")
	}
}
