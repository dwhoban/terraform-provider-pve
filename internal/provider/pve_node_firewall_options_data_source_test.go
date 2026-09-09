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

// TestPveNodeFirewallOptionsDataSource_MetadataAndSchema covers the data
// source's type name and computed attribute set.
func TestPveNodeFirewallOptionsDataSource_MetadataAndSchema(t *testing.T) {
	d := NewPveNodeFirewallOptionsDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveNodeFirewallOptions {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveNodeFirewallOptions)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"id", "node", "enable", "log_level_in", "tcpflags", "nf_conntrack_max"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
}

// TestPveNodeFirewallOptionsDataSource_Read verifies the host options read
// decode, including the boolish and integer encodings.
func TestPveNodeFirewallOptionsDataSource_Read(t *testing.T) {
	d := NewPveNodeFirewallOptionsDataSource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := d.(*pveNodeFirewallOptionsDataSource)
	if !ok {
		t.Fatalf("constructor returned %T", d)
	}
	impl.client = newNodeNetworkTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/nodes/pve1/firewall/options" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"enable":1,"log_level_in":"emerg","log_level_forward":"nolog","log_nf_conntrack":0,"ndp":1,"nf_conntrack_max":262144,"nftables":0,"protection_synflood":1,"tcpflags":1}}`))
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
	var got pveNodeFirewallOptionsDataSourceModel
	if err := resp.State.Get(ctx, &got); err != nil {
		t.Fatalf("State.Get: %v", err)
	}
	if got.ID.ValueString() != "pve1" || !got.Enable.ValueBool() || got.LogNFConntrack.ValueBool() || !got.Ndp.ValueBool() {
		t.Fatalf("node firewall options = %+v", got)
	}
	if got.NFConntrackMax.ValueInt64() != 262144 || got.Nftables.ValueBool() || !got.TCPFlags.ValueBool() {
		t.Fatalf("int/bool decode = %+v", got)
	}
}
