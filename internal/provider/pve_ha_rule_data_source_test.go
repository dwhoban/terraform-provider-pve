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

// TestPveHaRuleDataSource_MetadataAndSchema covers the rule data source.
func TestPveHaRuleDataSource_MetadataAndSchema(t *testing.T) {
	d := NewPveHaRuleDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveHaRule {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveHaRule)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"rule", "type", "affinity", "nodes", "resources", "strict", "disable", "comment"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
}

// TestPveHaRuleDataSource_Read verifies the single-rule read decode.
func TestPveHaRuleDataSource_Read(t *testing.T) {
	d := NewPveHaRuleDataSource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := d.(*pveHaRuleDataSource)
	if !ok {
		t.Fatalf("constructor returned %T", d)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/cluster/ha/rules/r1" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"rule":"r1","type":"resource-affinity","affinity":"negative","resources":"vm:100,ct:101","disable":1,"comment":"apart"}}`))
	})
	ctx := context.Background()
	cfg := haDSConfig(t, d, ctx, map[string]tftypes.Value{
		"rule": tftypes.NewValue(tftypes.String, "r1"),
	})
	resp := &datasource.ReadResponse{State: haDSNullState(t, d, ctx)}
	impl.Read(ctx, datasource.ReadRequest{Config: cfg}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %s", diagnosticsError(resp.Diagnostics))
	}
	var got pveHaRuleDataSourceModel
	if err := resp.State.Get(ctx, &got); err != nil {
		t.Fatalf("State.Get: %v", err)
	}
	if got.Type.ValueString() != "resource-affinity" || got.Affinity.ValueString() != "negative" {
		t.Fatalf("rule = %+v", got)
	}
	if len(got.Resources.Elements()) != 2 || !got.Disable.ValueBool() || got.Comment.ValueString() != "apart" {
		t.Fatalf("rule = %+v", got)
	}
}
