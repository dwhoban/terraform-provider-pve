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
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// TestPveStoragePruneBackupsAction_MetadataAndSchema covers the prune
// action's type name and schema shape.
func TestPveStoragePruneBackupsAction_MetadataAndSchema(t *testing.T) {
	a := NewPveStoragePruneBackupsAction()
	ctx := context.Background()
	metaResp := &action.MetadataResponse{}
	a.Metadata(ctx, action.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveStoragePruneBackups {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveStoragePruneBackups)
	}
	schemaResp := &action.SchemaResponse{}
	a.Schema(ctx, action.SchemaRequest{}, schemaResp)
	for _, key := range []string{
		"node", "storage", "dry_run", "keep_all", "keep_hourly", "keep_daily",
		"keep_weekly", "keep_monthly", "keep_yearly", "keep_last", "type", "vmid",
	} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	if !schemaResp.Schema.Attributes["node"].IsRequired() || !schemaResp.Schema.Attributes["storage"].IsRequired() {
		t.Fatal("node and storage attributes should be Required")
	}
}

// TestPveStoragePruneBackupsAction_ValidateConfigRequiresRetention verifies
// the action rejects a configuration without any retention option, listing
// the retention parameters, and accepts one with a retention option set.
func TestPveStoragePruneBackupsAction_ValidateConfigRequiresRetention(t *testing.T) {
	ctx := context.Background()
	// The constructor always returns the concrete action type.
	a, ok := NewPveStoragePruneBackupsAction().(*pveStoragePruneBackupsAction)
	if !ok {
		t.Fatal("constructor returned unexpected action type")
	}
	schemaResp := &action.SchemaResponse{}
	a.Schema(ctx, action.SchemaRequest{}, schemaResp)

	validate := func(model pveStoragePruneBackupsActionModel) *action.ValidateConfigResponse {
		req := action.ValidateConfigRequest{Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: storagePruneConfigRaw(model)}}
		resp := &action.ValidateConfigResponse{}
		a.ValidateConfig(ctx, req, resp)
		return resp
	}

	without := validate(pveStoragePruneBackupsActionModel{
		Node:    types.StringValue("pve1"),
		Storage: types.StringValue("local"),
	})
	if !without.Diagnostics.HasError() {
		t.Fatal("expected validation error without retention options")
	}
	if msg := diagnosticsError(without.Diagnostics); !strings.Contains(msg, "keep_all") || !strings.Contains(msg, "keep_last") {
		t.Fatalf("validation error must list the retention options, got: %s", msg)
	}

	with := validate(pveStoragePruneBackupsActionModel{
		Node:     types.StringValue("pve1"),
		Storage:  types.StringValue("local"),
		KeepLast: types.Int64Value(3),
	})
	if with.Diagnostics.HasError() {
		t.Fatalf("unexpected validation diagnostics: %s", diagnosticsError(with.Diagnostics))
	}
}

// TestPveStoragePruneBackupsAction_InvokeDryRun verifies dry_run issues the
// GET preview with the retention property string and reports counts in a
// progress message instead of echoing volumes.
func TestPveStoragePruneBackupsAction_InvokeDryRun(t *testing.T) {
	var sawMethod string
	client := newHaTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/nodes/pve1/storage/local/prunebackups" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		sawMethod = r.Method
		if got := r.URL.Query().Get("prune-backups"); got != "keep-last=3" {
			t.Fatalf("prune-backups = %q, want keep-last=3", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[`+
			`{"volid":"local:backup/vzdump-qemu-100-a.vma.zst","mark":"remove","type":"qemu","vmid":100,"ctime":1},`+
			`{"volid":"local:backup/vzdump-qemu-101-b.vma.zst","mark":"remove","type":"qemu","vmid":101,"ctime":2},`+
			`{"volid":"local:backup/vzdump-lxc-200-c.tar.zst","mark":"keep","type":"lxc","ctime":3},`+
			`{"volid":"local:backup/vzdump-qemu-102-d.vma.zst","mark":"protected","type":"qemu","ctime":4}]}`)
	})
	resp, progress := storagePruneInvoke(t, client, pveStoragePruneBackupsActionModel{
		Node:     types.StringValue("pve1"),
		Storage:  types.StringValue("local"),
		DryRun:   types.BoolValue(true),
		KeepLast: types.Int64Value(3),
	})
	if resp.Diagnostics.HasError() {
		t.Fatalf("Invoke diagnostics: %s", diagnosticsError(resp.Diagnostics))
	}
	if sawMethod != http.MethodGet {
		t.Fatalf("dry run must use GET, saw %s", sawMethod)
	}
	joined := strings.Join(progress, "\n")
	if !strings.Contains(joined, "2 backup(s) would be removed") || !strings.Contains(joined, "1 kept") || !strings.Contains(joined, "1 protected") {
		t.Fatalf("progress must report prune counts, got: %s", joined)
	}
	if strings.Contains(joined, "vzdump") {
		t.Fatalf("progress must not echo volumes, got: %s", joined)
	}
}

// TestPveStoragePruneBackupsAction_InvokePrune verifies the real prune
// issues DELETE with the retention parameters and waits for the returned
// task to finish.
func TestPveStoragePruneBackupsAction_InvokePrune(t *testing.T) {
	sawDelete := false
	sawStatus := false
	client := newHaTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodDelete && r.URL.Path == "/nodes/pve1/storage/local/prunebackups":
			if got := r.URL.Query().Get("prune-backups"); got != "keep-daily=7,keep-last=3" {
				t.Fatalf("prune-backups = %q, want keep-daily=7,keep-last=3", got)
			}
			if got := r.URL.Query().Get("type"); got != "lxc" {
				t.Fatalf("type = %q, want lxc", got)
			}
			sawDelete = true
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"data":"UPID:pve1:00001234:12345678:prune:root@pam:"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/nodes/pve1/tasks/UPID:pve1:00001234:12345678:prune:root@pam:/status":
			sawStatus = true
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"data":{"status":"stopped","exitstatus":"OK"}}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	resp, progress := storagePruneInvoke(t, client, pveStoragePruneBackupsActionModel{
		Node:      types.StringValue("pve1"),
		Storage:   types.StringValue("local"),
		KeepLast:  types.Int64Value(3),
		KeepDaily: types.Int64Value(7),
		Type:      types.StringValue("lxc"),
	})
	if resp.Diagnostics.HasError() {
		t.Fatalf("Invoke diagnostics: %s", diagnosticsError(resp.Diagnostics))
	}
	if !sawDelete || !sawStatus {
		t.Fatalf("prune request (delete=%v) or task wait (status=%v) missing", sawDelete, sawStatus)
	}
	if joined := strings.Join(progress, "\n"); !strings.Contains(joined, "finished") {
		t.Fatalf("progress must report completion, got: %s", joined)
	}
}

// TestPveStoragePruneBackupsAction_InvokePruneTaskFailure verifies a failed
// prune task surfaces as an Invoke diagnostic.
func TestPveStoragePruneBackupsAction_InvokePruneTaskFailure(t *testing.T) {
	client := newHaTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodDelete && r.URL.Path == "/nodes/pve1/storage/local/prunebackups":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"data":"UPID:pve1:00001234:12345678:prune:root@pam:"}`)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/status"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"data":{"status":"stopped","exitstatus":"ABORT: prune failed"}}`)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/log"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"data":[]}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	resp, _ := storagePruneInvoke(t, client, pveStoragePruneBackupsActionModel{
		Node:     types.StringValue("pve1"),
		Storage:  types.StringValue("local"),
		KeepLast: types.Int64Value(1),
	})
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error diagnostics from failed prune task")
	}
}

// storagePruneConfigRaw converts the action model into the raw tftypes
// object the framework hands back to ValidateConfig and Invoke.
func storagePruneConfigRaw(model pveStoragePruneBackupsActionModel) tftypes.Value {
	str := func(v types.String) tftypes.Value {
		if v.IsNull() || v.IsUnknown() {
			return tftypes.NewValue(tftypes.String, nil)
		}
		return tftypes.NewValue(tftypes.String, v.ValueString())
	}
	boolean := func(v types.Bool) tftypes.Value {
		if v.IsNull() || v.IsUnknown() {
			return tftypes.NewValue(tftypes.Bool, nil)
		}
		return tftypes.NewValue(tftypes.Bool, v.ValueBool())
	}
	num := func(v types.Int64) tftypes.Value {
		if v.IsNull() || v.IsUnknown() {
			return tftypes.NewValue(tftypes.Number, nil)
		}
		return tftypes.NewValue(tftypes.Number, v.ValueInt64())
	}
	attrTypes := map[string]tftypes.Type{
		"node": tftypes.String, "storage": tftypes.String, "dry_run": tftypes.Bool,
		"keep_all": tftypes.Bool, "keep_hourly": tftypes.Number, "keep_daily": tftypes.Number,
		"keep_weekly": tftypes.Number, "keep_monthly": tftypes.Number, "keep_yearly": tftypes.Number,
		"keep_last": tftypes.Number, "type": tftypes.String, "vmid": tftypes.Number,
	}
	return tftypes.NewValue(tftypes.Object{AttributeTypes: attrTypes}, map[string]tftypes.Value{
		"node": str(model.Node), "storage": str(model.Storage), "dry_run": boolean(model.DryRun),
		"keep_all": boolean(model.KeepAll), "keep_hourly": num(model.KeepHourly), "keep_daily": num(model.KeepDaily),
		"keep_weekly": num(model.KeepWeekly), "keep_monthly": num(model.KeepMonthly), "keep_yearly": num(model.KeepYearly),
		"keep_last": num(model.KeepLast), "type": str(model.Type), "vmid": num(model.VMID),
	})
}

// storagePruneInvoke runs the prune action against client with the given
// model, collecting progress messages.
func storagePruneInvoke(t *testing.T, client any, model pveStoragePruneBackupsActionModel) (*action.InvokeResponse, []string) {
	t.Helper()
	a := &pveStoragePruneBackupsAction{}
	ctx := context.Background()
	cfgResp := &action.ConfigureResponse{}
	a.Configure(ctx, action.ConfigureRequest{ProviderData: client}, cfgResp)
	if cfgResp.Diagnostics.HasError() {
		t.Fatalf("Configure diagnostics: %s", diagnosticsError(cfgResp.Diagnostics))
	}
	schemaResp := &action.SchemaResponse{}
	a.Schema(ctx, action.SchemaRequest{}, schemaResp)
	req := action.InvokeRequest{Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: storagePruneConfigRaw(model)}}
	resp := &action.InvokeResponse{}
	var progress []string
	resp.SendProgress = func(event action.InvokeProgressEvent) {
		progress = append(progress, event.Message)
	}
	a.Invoke(ctx, req, resp)
	return resp, progress
}
