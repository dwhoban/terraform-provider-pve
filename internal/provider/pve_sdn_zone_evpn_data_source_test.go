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

// TestPveSdnZoneEvpnDataSource_MetadataAndSchema covers the data source's
// type name and computed attribute set.
func TestPveSdnZoneEvpnDataSource_MetadataAndSchema(t *testing.T) {
	d := NewPveSdnZoneEvpnDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveSdnZoneEvpn {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveSdnZoneEvpn)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"zone", "nodes", "mtu", "ipam", "dns", "dnszone", "reversedns", "dhcp", "digest",
		"controller", "secondary_controllers", "advertise_subnets", "disable_arp_nd_suppression",
		"exitnodes", "exitnodes_local_routing", "exitnodes_primary", "mac", "rt_import", "vrf_vxlan"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	if !schemaResp.Schema.Attributes["zone"].IsRequired() {
		t.Fatal("zone attribute should be Required")
	}
	if !schemaResp.Schema.Attributes["vrf_vxlan"].IsComputed() {
		t.Fatal("vrf_vxlan attribute should be Computed")
	}
}

// TestPveSdnZoneEvpnDataSource_Read verifies the zone read decode,
// including the boolish encodings and the secondary controllers list.
func TestPveSdnZoneEvpnDataSource_Read(t *testing.T) {
	d := NewPveSdnZoneEvpnDataSource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := d.(*pveSdnZoneEvpnDataSource)
	if !ok {
		t.Fatalf("constructor returned %T", d)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/cluster/sdn/zones/evpn1" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"zone":"evpn1","type":"evpn","controller":"ctrl1",` +
			`"secondary-controllers":["ctrl2"],"advertise-subnets":1,"exitnodes":"pve1,pve2",` +
			`"vrf-vxlan":"100","mtu":1450,"nodes":"pve1","digest":"ee55"}}`))
	})
	ctx := context.Background()
	cfg := haDSConfig(t, d, ctx, map[string]tftypes.Value{
		"zone": tftypes.NewValue(tftypes.String, "evpn1"),
	})
	resp := &datasource.ReadResponse{State: haDSNullState(t, d, ctx)}
	impl.Read(ctx, datasource.ReadRequest{Config: cfg}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %s", diagnosticsError(resp.Diagnostics))
	}
	var got pveSdnZoneEvpnDataSourceModel
	if err := resp.State.Get(ctx, &got); err != nil {
		t.Fatalf("State.Get: %v", err)
	}
	if got.Zone.ValueString() != "evpn1" || got.Controller.ValueString() != "ctrl1" || !got.AdvertiseSubnets.ValueBool() {
		t.Fatalf("evpn zone = %+v", got)
	}
	if got.ExitNodes.ValueString() != "pve1,pve2" || got.VrfVxlan.ValueInt64() != 100 || got.MTU.ValueInt64() != 1450 {
		t.Fatalf("evpn shared fields = %+v", got)
	}
	if len(got.SecondaryControllers.Elements()) != 1 || got.Digest.ValueString() != "ee55" {
		t.Fatalf("evpn list fields = %+v", got)
	}
}
