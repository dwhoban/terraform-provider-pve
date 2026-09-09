// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// TestPveReplicationDataSource_MetadataAndSchema covers the replication
// data source's type name and schema shape.
func TestPveReplicationDataSource_MetadataAndSchema(t *testing.T) {
	d := NewPveReplicationDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveReplication {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveReplication)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"id", "target", "type", "guest", "jobnum", "schedule", "rate", "comment", "disable", "digest"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	if !schemaResp.Schema.Attributes["id"].IsRequired() {
		t.Fatal("id attribute should be Required")
	}
}

// TestPveReplicationDataSource_Read verifies the single-job read decode,
// including the boolish disable flag and the null rate mapping.
func TestPveReplicationDataSource_Read(t *testing.T) {
	d := NewPveReplicationDataSource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := d.(*pveReplicationDataSource)
	if !ok {
		t.Fatalf("constructor returned %T", d)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/cluster/replication/100-0" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"id":"100-0","type":"local","target":"pve2","guest":100,` +
			`"jobnum":0,"schedule":"*/15","disable":1,"comment":"dr site","digest":"abc123"}}`))
	})
	ctx := context.Background()
	cfg := haDSConfig(t, d, ctx, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "100-0"),
	})
	resp := &datasource.ReadResponse{State: haDSNullState(t, d, ctx)}
	impl.Read(ctx, datasource.ReadRequest{Config: cfg}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %s", diagnosticsError(resp.Diagnostics))
	}
	var got pveReplicationDataSourceModel
	if err := resp.State.Get(ctx, &got); err != nil {
		t.Fatalf("State.Get: %v", err)
	}
	if got.ID.ValueString() != "100-0" || got.Target.ValueString() != "pve2" || got.Type.ValueString() != "local" {
		t.Fatalf("job = %+v", got)
	}
	if got.Guest.ValueInt64() != 100 || got.JobNum.ValueInt64() != 0 || got.Schedule.ValueString() != "*/15" {
		t.Fatalf("job = %+v", got)
	}
	if !got.Disable.ValueBool() || got.Comment.ValueString() != "dr site" || got.Digest.ValueString() != "abc123" {
		t.Fatalf("job = %+v", got)
	}
	if !got.Rate.IsNull() {
		t.Fatalf("absent rate must decode null, got %v", got.Rate)
	}
}
