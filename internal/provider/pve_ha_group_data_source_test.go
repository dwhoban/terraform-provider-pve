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

// TestPveHaGroupDataSource_MetadataAndSchema covers the group data source.
func TestPveHaGroupDataSource_MetadataAndSchema(t *testing.T) {
	d := NewPveHaGroupDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveHaGroup {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveHaGroup)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"group", "nodes", "restricted", "nofailback", "comment"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
}

// TestPveHaGroupDataSource_Read verifies the single-group read decode.
func TestPveHaGroupDataSource_Read(t *testing.T) {
	d := NewPveHaGroupDataSource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := d.(*pveHaGroupDataSource)
	if !ok {
		t.Fatalf("constructor returned %T", d)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/cluster/ha/groups/g1" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"group":"g1","nodes":"n1:2,n2","restricted":true,"nofailback":0,"comment":"core"}}`))
	})
	ctx := context.Background()
	cfg := haDSConfig(t, d, ctx, map[string]tftypes.Value{
		"group": tftypes.NewValue(tftypes.String, "g1"),
	})
	resp := &datasource.ReadResponse{State: haDSNullState(t, d, ctx)}
	impl.Read(ctx, datasource.ReadRequest{Config: cfg}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %s", diagnosticsError(resp.Diagnostics))
	}
	var got pveHaGroupDataSourceModel
	if err := resp.State.Get(ctx, &got); err != nil {
		t.Fatalf("State.Get: %v", err)
	}
	if got.Group.ValueString() != "g1" || len(got.Nodes.Elements()) != 2 || !got.Restricted.ValueBool() || got.NoFailback.ValueBool() {
		t.Fatalf("group = %+v", got)
	}
	if got.Comment.ValueString() != "core" {
		t.Fatalf("comment = %q", got.Comment.ValueString())
	}
}
