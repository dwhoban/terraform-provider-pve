// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// vmTestUpid is a realistic worker task UPID on node pve1 handed out by the
// fake server; vmWaitForTask parses the node back out of it.
const vmTestUpid = "UPID:pve1:00001234:12345678:qmvmmigrate:root@pam:"

// vmActionServer is a fake PVE serving one QEMU VM verb endpoint plus the
// task-status poll that vmWaitForTask performs.
type vmActionServer struct {
	client    *pveclient.Client
	sawMethod string
	sawPath   string
	sawBody   string
}

// newVMActionServer fakes the given POST verb endpoint (method asserted
// internally) and the follow-up successful task-status poll.
func newVMActionServer(t *testing.T, wantPath string) *vmActionServer {
	t.Helper()
	srv := &vmActionServer{}
	srv.client = newHaTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/nodes/pve1/tasks/"+vmTestUpid+"/status" {
			_, _ = io.WriteString(w, `{"data":{"status":"stopped","exitstatus":"OK"}}`)
			return
		}
		srv.sawMethod, srv.sawPath = r.Method, r.URL.Path
		body, _ := io.ReadAll(r.Body)
		srv.sawBody = string(body)
		if srv.sawMethod != http.MethodPost || srv.sawPath != wantPath {
			t.Errorf("request = %s %s, want %s %s", srv.sawMethod, srv.sawPath, http.MethodPost, wantPath)
		}
		_, _ = fmt.Fprintf(w, `{"data":%q}`, vmTestUpid)
	})
	return srv
}

// invokeVMAction configures the action with the fake client and invokes it
// with the given config shape. ActionWithConfigure is asserted comma-ok
// because the constructors in this package return action.Action.
func invokeVMAction(t *testing.T, a action.Action, client *pveclient.Client, attrTypes map[string]tftypes.Type, vals map[string]tftypes.Value) *action.InvokeResponse {
	t.Helper()
	awc, ok := a.(action.ActionWithConfigure)
	if !ok {
		t.Fatalf("action %T does not implement ActionWithConfigure", a)
	}
	ctx := context.Background()
	cfgResp := &action.ConfigureResponse{}
	awc.Configure(ctx, action.ConfigureRequest{ProviderData: client}, cfgResp)
	if cfgResp.Diagnostics.HasError() {
		t.Fatalf("Configure diagnostics: %s", diagnosticsError(cfgResp.Diagnostics))
	}
	schemaResp := &action.SchemaResponse{}
	a.Schema(ctx, action.SchemaRequest{}, schemaResp)
	raw := tftypes.NewValue(tftypes.Object{AttributeTypes: attrTypes}, vals)
	resp := &action.InvokeResponse{}
	a.Invoke(ctx, action.InvokeRequest{Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw}}, resp)
	return resp
}

// assertVMActionMetadata asserts an action's full type name.
func assertVMActionMetadata(t *testing.T, a action.Action, suffix string) {
	t.Helper()
	metaResp := &action.MetadataResponse{}
	a.Metadata(context.Background(), action.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+suffix {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, suffix)
	}
}

// TestPveVmRebootAction_MetadataAndInvoke covers the reboot action's type
// name, timeout parameter, and task wait.
func TestPveVmRebootAction_MetadataAndInvoke(t *testing.T) {
	a := NewPveVmRebootAction()
	assertVMActionMetadata(t, a, TypeNamePveVmReboot)

	srv := newVMActionServer(t, "/nodes/pve1/qemu/100/status/reboot")
	resp := invokeVMAction(t, a, srv.client, map[string]tftypes.Type{
		"node":    tftypes.String,
		"vmid":    tftypes.Number,
		"timeout": tftypes.Number,
	}, map[string]tftypes.Value{
		"node":    tftypes.NewValue(tftypes.String, "pve1"),
		"vmid":    tftypes.NewValue(tftypes.Number, 100),
		"timeout": tftypes.NewValue(tftypes.Number, 60),
	})
	if resp.Diagnostics.HasError() {
		t.Fatalf("Invoke diagnostics: %s", diagnosticsError(resp.Diagnostics))
	}
	if !strings.Contains(srv.sawBody, `"timeout":60`) {
		t.Fatalf("reboot body missing timeout: %q", srv.sawBody)
	}
}

// TestPveVmSuspendAction_MetadataAndInvoke covers the suspend action with
// the todisk and statestorage parameters.
func TestPveVmSuspendAction_MetadataAndInvoke(t *testing.T) {
	a := NewPveVmSuspendAction()
	assertVMActionMetadata(t, a, TypeNamePveVmSuspend)

	srv := newVMActionServer(t, "/nodes/pve1/qemu/100/status/suspend")
	resp := invokeVMAction(t, a, srv.client, map[string]tftypes.Type{
		"node":         tftypes.String,
		"vmid":         tftypes.Number,
		"todisk":       tftypes.Bool,
		"statestorage": tftypes.String,
	}, map[string]tftypes.Value{
		"node":         tftypes.NewValue(tftypes.String, "pve1"),
		"vmid":         tftypes.NewValue(tftypes.Number, 100),
		"todisk":       tftypes.NewValue(tftypes.Bool, true),
		"statestorage": tftypes.NewValue(tftypes.String, "local-lvm"),
	})
	if resp.Diagnostics.HasError() {
		t.Fatalf("Invoke diagnostics: %s", diagnosticsError(resp.Diagnostics))
	}
	if !strings.Contains(srv.sawBody, `"todisk":true`) || !strings.Contains(srv.sawBody, `"statestorage":"local-lvm"`) {
		t.Fatalf("suspend body = %q", srv.sawBody)
	}
}

// TestPveVmResumeAction_MetadataAndInvoke covers the resume action.
func TestPveVmResumeAction_MetadataAndInvoke(t *testing.T) {
	a := NewPveVmResumeAction()
	assertVMActionMetadata(t, a, TypeNamePveVmResume)

	srv := newVMActionServer(t, "/nodes/pve1/qemu/100/status/resume")
	resp := invokeVMAction(t, a, srv.client, map[string]tftypes.Type{
		"node": tftypes.String,
		"vmid": tftypes.Number,
	}, map[string]tftypes.Value{
		"node": tftypes.NewValue(tftypes.String, "pve1"),
		"vmid": tftypes.NewValue(tftypes.Number, 100),
	})
	if resp.Diagnostics.HasError() {
		t.Fatalf("Invoke diagnostics: %s", diagnosticsError(resp.Diagnostics))
	}
	if srv.sawBody != "" {
		t.Fatalf("resume must send no parameters, got %q", srv.sawBody)
	}
}

// TestPveVmResetAction_MetadataAndInvoke covers the reset action.
func TestPveVmResetAction_MetadataAndInvoke(t *testing.T) {
	a := NewPveVmResetAction()
	assertVMActionMetadata(t, a, TypeNamePveVmReset)

	srv := newVMActionServer(t, "/nodes/pve1/qemu/100/status/reset")
	resp := invokeVMAction(t, a, srv.client, map[string]tftypes.Type{
		"node": tftypes.String,
		"vmid": tftypes.Number,
	}, map[string]tftypes.Value{
		"node": tftypes.NewValue(tftypes.String, "pve1"),
		"vmid": tftypes.NewValue(tftypes.Number, 100),
	})
	if resp.Diagnostics.HasError() {
		t.Fatalf("Invoke diagnostics: %s", diagnosticsError(resp.Diagnostics))
	}
	if srv.sawBody != "" {
		t.Fatalf("reset must send no parameters, got %q", srv.sawBody)
	}
}

// TestPveVmMigrateAction_MetadataAndInvoke covers the migrate action's
// parameters and the task wait.
func TestPveVmMigrateAction_MetadataAndInvoke(t *testing.T) {
	a := NewPveVmMigrateAction()
	assertVMActionMetadata(t, a, TypeNamePveVmMigrate)

	srv := newVMActionServer(t, "/nodes/pve1/qemu/100/migrate")
	resp := invokeVMAction(t, a, srv.client, map[string]tftypes.Type{
		"node":              tftypes.String,
		"vmid":              tftypes.Number,
		"target":            tftypes.String,
		"target_storage":    tftypes.String,
		"bandwidth":         tftypes.Number,
		"migration_type":    tftypes.String,
		"migration_network": tftypes.String,
		"online":            tftypes.Bool,
		"with_local_disks":  tftypes.Bool,
		"force":             tftypes.Bool,
	}, map[string]tftypes.Value{
		"node":              tftypes.NewValue(tftypes.String, "pve1"),
		"vmid":              tftypes.NewValue(tftypes.Number, 100),
		"target":            tftypes.NewValue(tftypes.String, "pve2"),
		"target_storage":    tftypes.NewValue(tftypes.String, nil),
		"bandwidth":         tftypes.NewValue(tftypes.Number, nil),
		"migration_type":    tftypes.NewValue(tftypes.String, nil),
		"migration_network": tftypes.NewValue(tftypes.String, nil),
		"online":            tftypes.NewValue(tftypes.Bool, true),
		"with_local_disks":  tftypes.NewValue(tftypes.Bool, nil),
		"force":             tftypes.NewValue(tftypes.Bool, nil),
	})
	if resp.Diagnostics.HasError() {
		t.Fatalf("Invoke diagnostics: %s", diagnosticsError(resp.Diagnostics))
	}
	if !strings.Contains(srv.sawBody, `"target":"pve2"`) || !strings.Contains(srv.sawBody, `"online":true`) {
		t.Fatalf("migrate body = %q", srv.sawBody)
	}
}

// TestPveVmSnapshotRollbackAction_MetadataAndInvoke covers the rollback
// action.
func TestPveVmSnapshotRollbackAction_MetadataAndInvoke(t *testing.T) {
	a := NewPveVmSnapshotRollbackAction()
	assertVMActionMetadata(t, a, TypeNamePveVmSnapshotRollback)

	srv := newVMActionServer(t, "/nodes/pve1/qemu/100/snapshot/snap1/rollback")
	resp := invokeVMAction(t, a, srv.client, map[string]tftypes.Type{
		"node": tftypes.String,
		"vmid": tftypes.Number,
		"name": tftypes.String,
	}, map[string]tftypes.Value{
		"node": tftypes.NewValue(tftypes.String, "pve1"),
		"vmid": tftypes.NewValue(tftypes.Number, 100),
		"name": tftypes.NewValue(tftypes.String, "snap1"),
	})
	if resp.Diagnostics.HasError() {
		t.Fatalf("Invoke diagnostics: %s", diagnosticsError(resp.Diagnostics))
	}
	if srv.sawBody != "" {
		t.Fatalf("rollback must send no parameters, got %q", srv.sawBody)
	}
}
