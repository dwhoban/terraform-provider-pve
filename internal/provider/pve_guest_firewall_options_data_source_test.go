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

// TestPveGuestFirewallOptionsDataSource_MetadataAndSchema covers the data
// source's type name, keys, and computed attribute set.
func TestPveGuestFirewallOptionsDataSource_MetadataAndSchema(t *testing.T) {
	d := NewPveGuestFirewallOptionsDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveGuestFirewallOptions {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveGuestFirewallOptions)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"id", "node", "guest_type", "vmid", "enable", "dhcp", "ipfilter", "macfilter", "ndp", "radv", "log_level_in", "log_level_out", "policy_in", "policy_out"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	for _, key := range []string{"node", "guest_type", "vmid"} {
		if !schemaResp.Schema.Attributes[key].IsRequired() {
			t.Fatalf("%s attribute should be Required", key)
		}
	}
}

// TestPveGuestFirewallOptionsDataSource_Read verifies the guest options
// read decode on the lxc path.
func TestPveGuestFirewallOptionsDataSource_Read(t *testing.T) {
	d := NewPveGuestFirewallOptionsDataSource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := d.(*pveGuestFirewallOptionsDataSource)
	if !ok {
		t.Fatalf("constructor returned %T", d)
	}
	impl.client = newNodeNetworkTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/nodes/pve1/lxc/101/firewall/options" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"enable":1,"dhcp":0,"ipfilter":1,"macfilter":1,"ndp":1,"radv":0,"log_level_in":"emerg","policy_in":"DROP","policy_out":"ACCEPT"}}`))
	})
	ctx := context.Background()
	cfg := haDSConfig(t, d, ctx, map[string]tftypes.Value{
		"node":       tftypes.NewValue(tftypes.String, "pve1"),
		"guest_type": tftypes.NewValue(tftypes.String, "lxc"),
		"vmid":       tftypes.NewValue(tftypes.Number, 101),
	})
	resp := &datasource.ReadResponse{State: haDSNullState(t, d, ctx)}
	impl.Read(ctx, datasource.ReadRequest{Config: cfg}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %s", diagnosticsError(resp.Diagnostics))
	}
	var got pveGuestFirewallOptionsDataSourceModel
	if err := resp.State.Get(ctx, &got); err != nil {
		t.Fatalf("State.Get: %v", err)
	}
	if got.ID.ValueString() != "pve1:lxc:101" || !got.Enable.ValueBool() || got.DHCP.ValueBool() || !got.IPFilter.ValueBool() {
		t.Fatalf("guest firewall options = %+v", got)
	}
	if got.PolicyIn.ValueString() != "DROP" || got.PolicyOut.ValueString() != "ACCEPT" {
		t.Fatalf("policies = %+v", got)
	}
}
