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

// TestPveSdnFirewallOptionsDataSource_MetadataAndSchema covers the data
// source's type name and computed attribute set.
func TestPveSdnFirewallOptionsDataSource_MetadataAndSchema(t *testing.T) {
	d := NewPveSdnFirewallOptionsDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveSdnFirewallOptions {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveSdnFirewallOptions)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"id", "vnet", "enable", "log_level_forward", "policy_forward"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	if !schemaResp.Schema.Attributes["vnet"].IsRequired() {
		t.Fatal("vnet attribute should be Required")
	}
}

// TestPveSdnFirewallOptionsDataSource_Read verifies the vnet options read
// decode, including the boolish enable encoding.
func TestPveSdnFirewallOptionsDataSource_Read(t *testing.T) {
	d := NewPveSdnFirewallOptionsDataSource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := d.(*pveSdnFirewallOptionsDataSource)
	if !ok {
		t.Fatalf("constructor returned %T", d)
	}
	impl.client = newNodeNetworkTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/cluster/sdn/vnets/vnet1/firewall/options" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"enable":1,"log_level_forward":"debug","policy_forward":"DROP"}}`))
	})
	ctx := context.Background()
	cfg := haDSConfig(t, d, ctx, map[string]tftypes.Value{
		"vnet": tftypes.NewValue(tftypes.String, "vnet1"),
	})
	resp := &datasource.ReadResponse{State: haDSNullState(t, d, ctx)}
	impl.Read(ctx, datasource.ReadRequest{Config: cfg}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %s", diagnosticsError(resp.Diagnostics))
	}
	var got pveSdnFirewallOptionsDataSourceModel
	if err := resp.State.Get(ctx, &got); err != nil {
		t.Fatalf("State.Get: %v", err)
	}
	if got.ID.ValueString() != "vnet1" || !got.Enable.ValueBool() || got.LogLevelForward.ValueString() != "debug" {
		t.Fatalf("vnet firewall options = %+v", got)
	}
	if got.PolicyForward.ValueString() != "DROP" {
		t.Fatalf("policy_forward = %q", got.PolicyForward.ValueString())
	}
}
