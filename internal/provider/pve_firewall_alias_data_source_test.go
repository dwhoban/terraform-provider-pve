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

// TestPveFirewallAliasDataSource_MetadataAndSchema covers the alias data
// source's type name and schema shape.
func TestPveFirewallAliasDataSource_MetadataAndSchema(t *testing.T) {
	d := NewPveFirewallAliasDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveFirewallAlias {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveFirewallAlias)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"name", "cidr", "comment"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
}

// TestPveFirewallAliasDataSource_Read verifies the single-alias read decode.
func TestPveFirewallAliasDataSource_Read(t *testing.T) {
	d := NewPveFirewallAliasDataSource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := d.(*pveFirewallAliasDataSource)
	if !ok {
		t.Fatalf("constructor returned %T", d)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/cluster/firewall/aliases/office" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"name":"office","cidr":"203.0.113.0/24","comment":"HQ"}}`))
	})
	ctx := context.Background()
	cfg := haDSConfig(t, d, ctx, map[string]tftypes.Value{
		"name": tftypes.NewValue(tftypes.String, "office"),
	})
	resp := &datasource.ReadResponse{State: haDSNullState(t, d, ctx)}
	impl.Read(ctx, datasource.ReadRequest{Config: cfg}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %s", diagnosticsError(resp.Diagnostics))
	}
	var got pveFirewallAliasDataSourceModel
	if err := resp.State.Get(ctx, &got); err != nil {
		t.Fatalf("State.Get: %v", err)
	}
	if got.Name.ValueString() != "office" || got.Cidr.ValueString() != "203.0.113.0/24" || got.Comment.ValueString() != "HQ" {
		t.Fatalf("alias = %+v", got)
	}
}
