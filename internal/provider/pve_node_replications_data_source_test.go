// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// TestPveNodeReplicationsDataSource_MetadataAndSchema covers the node
// replications data source's type name and schema shape.
func TestPveNodeReplicationsDataSource_MetadataAndSchema(t *testing.T) {
	d := NewPveNodeReplicationsDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveNodeReplications {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveNodeReplications)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"id", "node", "guest", "replications"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	if !schemaResp.Schema.Attributes["node"].IsRequired() {
		t.Fatal("node attribute should be Required")
	}
}

// TestPveNodeReplicationsDataSource_Read verifies the list decode with
// runtime status fields and the per-job log fetch.
func TestPveNodeReplicationsDataSource_Read(t *testing.T) {
	d := NewPveNodeReplicationsDataSource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := d.(*pveNodeReplicationsDataSource)
	if !ok {
		t.Fatalf("constructor returned %T", d)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/nodes/pve1/replication":
			_, _ = w.Write([]byte(`{"data":[{"id":"100-0","type":"local","target":"pve2","guest":100,` +
				`"guest_name":"vm100","jobnum":0,"schedule":"*/15","rate":null,"disable":0,"removal":0,` +
				`"last_sync":1700000000,"last_try":1700000000,"next_sync":1700000090,"fail_count":0,` +
				`"duration":12,"status":"idle"},` +
				`{"id":"101-0","type":"local","target":"pve3","guest":101,"guest_name":"ct101","jobnum":0,` +
				`"schedule":"*/30","rate":null,"disable":1,"removal":1,"last_sync":1700000000,` +
				`"last_try":1700000000,"next_sync":1700000180,"fail_count":3,"duration":44,` +
				`"status":"error","error":"sync failed"}]}`))
		case "/nodes/pve1/replication/100-0/log":
			_, _ = w.Write([]byte(`{"data":[{"n":0,"t":"job start"},{"n":1,"t":"job end"}]}`))
		case "/nodes/pve1/replication/101-0/log":
			_, _ = w.Write([]byte(`{"data":[{"n":0,"t":"full sync attempt 1"}]}`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	ctx := context.Background()
	cfg := haDSConfig(t, d, ctx, map[string]tftypes.Value{
		"node": tftypes.NewValue(tftypes.String, "pve1"),
	})
	resp := &datasource.ReadResponse{State: haDSNullState(t, d, ctx)}
	impl.Read(ctx, datasource.ReadRequest{Config: cfg}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %s", diagnosticsError(resp.Diagnostics))
	}
	var got pveNodeReplicationsDataSourceModel
	if err := resp.State.Get(ctx, &got); err != nil {
		t.Fatalf("State.Get: %v", err)
	}
	if got.Node.ValueString() != "pve1" || len(got.Replications) != 2 {
		t.Fatalf("node = %q, replications = %d", got.Node.ValueString(), len(got.Replications))
	}
	healthy := got.Replications[0]
	if healthy.ID.ValueString() != "100-0" || healthy.Target.ValueString() != "pve2" || healthy.GuestName.ValueString() != "vm100" {
		t.Fatalf("healthy row = %+v", healthy)
	}
	if healthy.Status.ValueString() != "idle" || healthy.FailCount.ValueInt64() != 0 || healthy.Duration.ValueInt64() != 12 {
		t.Fatalf("healthy row = %+v", healthy)
	}
	if healthy.LastSync.ValueInt64() != 1700000000 || healthy.NextSync.ValueInt64() != 1700000090 {
		t.Fatalf("healthy row = %+v", healthy)
	}
	failed := got.Replications[1]
	if !failed.Disable.ValueBool() || !failed.Removal.ValueBool() {
		t.Fatalf("failed row = %+v", failed)
	}
	if failed.Status.ValueString() != "error" || failed.Error.ValueString() != "sync failed" || failed.FailCount.ValueInt64() != 3 {
		t.Fatalf("failed row = %+v", failed)
	}
	if !failed.Rate.IsNull() {
		t.Fatalf("null rate must decode null, got %v", failed.Rate)
	}
	logElems := healthy.Log.Elements()
	if len(logElems) != 2 {
		t.Fatalf("healthy log lines = %d", len(logElems))
	}
	// safetyassert: listStringToTF builds this list from strings, so every element is a types.String.
	first, ok := logElems[0].(types.String)
	if !ok || first.ValueString() != "job start" {
		t.Fatalf("healthy log[0] = %v", logElems[0])
	}
	if len(failed.Log.Elements()) != 1 {
		t.Fatalf("failed log lines = %d", len(failed.Log.Elements()))
	}
}
