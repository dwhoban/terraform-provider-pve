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

// TestPveClusterFirewallOptionsDataSource_MetadataAndSchema covers the data
// source's type name and computed attribute set.
func TestPveClusterFirewallOptionsDataSource_MetadataAndSchema(t *testing.T) {
	d := NewPveClusterFirewallOptionsDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveClusterFirewallOptions {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveClusterFirewallOptions)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"id", "ebtables", "enable", "log_ratelimit", "policy_forward", "policy_in", "policy_out"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
}

// TestPveClusterFirewallOptionsDataSource_Read verifies the singleton read
// decode, including the boolish ebtables encoding.
func TestPveClusterFirewallOptionsDataSource_Read(t *testing.T) {
	d := NewPveClusterFirewallOptionsDataSource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := d.(*pveClusterFirewallOptionsDataSource)
	if !ok {
		t.Fatalf("constructor returned %T", d)
	}
	impl.client = newNodeNetworkTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/cluster/firewall/options" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"ebtables":1,"enable":0,"log_ratelimit":"enable=1,burst=5,rate=1/second","policy_forward":"DROP","policy_in":"ACCEPT","policy_out":"DROP"}}`))
	})
	ctx := context.Background()
	resp := &datasource.ReadResponse{State: haDSNullState(t, d, ctx)}
	impl.Read(ctx, datasource.ReadRequest{Config: haDSConfig(t, d, ctx, map[string]tftypes.Value{})}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %s", diagnosticsError(resp.Diagnostics))
	}
	var got pveClusterFirewallOptionsDataSourceModel
	if err := resp.State.Get(ctx, &got); err != nil {
		t.Fatalf("State.Get: %v", err)
	}
	if got.ID.ValueString() != "cluster" || !got.Ebitables.ValueBool() || got.Enable.ValueInt64() != 0 {
		t.Fatalf("cluster firewall options = %+v", got)
	}
	if got.PolicyForward.ValueString() != "DROP" || got.PolicyOut.ValueString() != "DROP" {
		t.Fatalf("policies = %+v", got)
	}
}
