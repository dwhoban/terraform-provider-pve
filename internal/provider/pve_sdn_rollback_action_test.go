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

// TestPveSdnRollbackAction_MetadataAndSchema covers the rollback action's
// type name and schema shape.
func TestPveSdnRollbackAction_MetadataAndSchema(t *testing.T) {
	a := NewPveSdnRollbackAction()
	ctx := context.Background()
	metaResp := &action.MetadataResponse{}
	a.Metadata(ctx, action.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveSdnRollback {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveSdnRollback)
	}
	schemaResp := &action.SchemaResponse{}
	a.Schema(ctx, action.SchemaRequest{}, schemaResp)
	for _, key := range []string{"lock_token", "release_lock"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
}

// sdnRollbackInvoke runs the rollback action against client with the given
// options (nil values omit the attributes).
func sdnRollbackInvoke(t *testing.T, client any, lockToken any, releaseLock any) *action.InvokeResponse {
	t.Helper()
	a := &pveSdnRollbackAction{}
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

// TestPveSdnRollbackAction_InvokePostsRollback verifies the action POSTs
// /cluster/sdn/rollback with the lock token and never polls a task (the
// pin's rollback returns null, so it is synchronous).
func TestPveSdnRollbackAction_InvokePostsRollback(t *testing.T) {
	sawMethod, sawPath := "", ""
	client := newHaTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost || r.URL.Path != "/cluster/sdn/rollback" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		sawMethod, sawPath = r.Method, r.URL.Path
		_, _ = io.WriteString(w, `{"data":null}`)
	})
	resp := sdnRollbackInvoke(t, client, "tok", nil)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Invoke diagnostics: %s", diagnosticsError(resp.Diagnostics))
	}
	if sawMethod != http.MethodPost || sawPath != "/cluster/sdn/rollback" {
		t.Fatalf("rollback request = %s %s", sawMethod, sawPath)
	}
}

// TestPveSdnRollbackAction_InvokeWithoutClient verifies the action surfaces
// a diagnostic instead of panicking when the client is missing.
func TestPveSdnRollbackAction_InvokeWithoutClient(t *testing.T) {
	resp := sdnRollbackInvoke(t, nil, nil, nil)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error diagnostic for unconfigured client")
	}
}
