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

// TestPveHaResourceDataSource_MetadataAndSchema covers the HA resource
// data source.
func TestPveHaResourceDataSource_MetadataAndSchema(t *testing.T) {
	d := NewPveHaResourceDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveHaResource {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveHaResource)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"sid", "type", "state", "group", "max_restart", "max_relocate", "failback", "auto_rebalance", "comment"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
}

// TestPveHaResourceDataSource_Read verifies the single-resource read
// decode, including the boolish flag encodings.
func TestPveHaResourceDataSource_Read(t *testing.T) {
	d := NewPveHaResourceDataSource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := d.(*pveHaResourceDataSource)
	if !ok {
		t.Fatalf("constructor returned %T", d)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/cluster/ha/resources/ct:101" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"sid":"ct:101","type":"ct","state":"stopped","group":"g1","max_restart":2,"max_relocate":1,"failback":0,"auto-rebalance":1,"comment":"ha ct"}}`))
	})
	ctx := context.Background()
	cfg := haDSConfig(t, d, ctx, map[string]tftypes.Value{
		"sid": tftypes.NewValue(tftypes.String, "ct:101"),
	})
	resp := &datasource.ReadResponse{State: haDSNullState(t, d, ctx)}
	impl.Read(ctx, datasource.ReadRequest{Config: cfg}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %s", diagnosticsError(resp.Diagnostics))
	}
	var got pveHaResourceDataSourceModel
	if err := resp.State.Get(ctx, &got); err != nil {
		t.Fatalf("State.Get: %v", err)
	}
	if got.Type.ValueString() != "ct" || got.State.ValueString() != "stopped" || got.Group.ValueString() != "g1" {
		t.Fatalf("resource = %+v", got)
	}
	if got.MaxRestart.ValueInt64() != 2 || got.Failback.ValueBool() || !got.AutoRebalance.ValueBool() {
		t.Fatalf("resource = %+v", got)
	}
	if got.Comment.ValueString() != "ha ct" {
		t.Fatalf("comment = %q", got.Comment.ValueString())
	}
}
