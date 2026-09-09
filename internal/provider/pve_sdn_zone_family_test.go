// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// sdnZoneTestAttrTypes derives the tftypes attribute-type map from a zone
// family schema.
func sdnZoneTestAttrTypes(t *testing.T, s schema.Schema) map[string]tftypes.Type {
	t.Helper()
	out := make(map[string]tftypes.Type, len(s.Attributes))
	for name, attr := range s.Attributes {
		switch a := attr.(type) {
		case schema.StringAttribute:
			out[name] = tftypes.String
		case schema.Int64Attribute:
			out[name] = tftypes.Number
		case schema.BoolAttribute:
			out[name] = tftypes.Bool
		case schema.ListAttribute:
			if a.ElementType != types.StringType {
				t.Fatalf("attribute %s has unexpected list element type %v", name, a.ElementType)
			}
			out[name] = tftypes.List{ElementType: tftypes.String}
		default:
			t.Fatalf("unhandled attribute type for %s: %T", name, attr)
		}
	}
	return out
}

// sdnZoneTestObject builds a zone model object, nulling every attribute
// absent from vals.
func sdnZoneTestObject(t *testing.T, s schema.Schema, vals map[string]tftypes.Value) tftypes.Value {
	t.Helper()
	attrTypes := sdnZoneTestAttrTypes(t, s)
	obj := make(map[string]tftypes.Value, len(attrTypes))
	for name, at := range attrTypes {
		if v, ok := vals[name]; ok {
			obj[name] = v
			continue
		}
		obj[name] = tftypes.NewValue(at, nil)
	}
	return tftypes.NewValue(tftypes.Object{AttributeTypes: attrTypes}, obj)
}

// TestSdnZoneGetChecked_WrongTypeErrors verifies the family type guard: a
// zone whose upstream type does not match the component's fixed type is an
// error naming both types, never a silent adoption.
func TestSdnZoneGetChecked_WrongTypeErrors(t *testing.T) {
	client := newNodeNetworkTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/cluster/sdn/zones/zone1" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"zone":"zone1","type":"simple","nodes":"pve1"}}`))
	})
	_, err := sdnZoneGetChecked(context.Background(), client, "zone1", "vlan")
	if err == nil {
		t.Fatal("expected type mismatch error, got nil")
	}
	if !strings.Contains(err.Error(), `"simple"`) || !strings.Contains(err.Error(), `"vlan"`) {
		t.Fatalf("mismatch error must name both types, got: %s", err)
	}
}

// TestSdnZoneCommonDeleteFields verifies the update-side clearing: only
// shared attributes present in state but null in the plan travel in the
// PVE `delete` parameter.
func TestSdnZoneCommonDeleteFields(t *testing.T) {
	plan := sdnZoneCommonModel{
		Zone:    types.StringValue("zone1"),
		Nodes:   types.StringNull(),
		MTU:     types.Int64Value(1500),
		IPAM:    types.StringNull(),
		DNS:     types.StringValue("dns1"),
		DNSZone: types.StringNull(),
		DHCP:    types.StringValue("dnsmasq"),
		Digest:  types.StringNull(),
	}
	state := sdnZoneCommonModel{
		Zone:       types.StringValue("zone1"),
		Nodes:      types.StringValue("pve1"),
		MTU:        types.Int64Value(1500),
		IPAM:       types.StringValue("pve"),
		DNS:        types.StringValue("dns1"),
		DNSZone:    types.StringValue("example.com"),
		ReverseDNS: types.StringValue("rdns1"),
		DHCP:       types.StringValue("dnsmasq"),
		Digest:     types.StringNull(),
	}
	got := sdnZoneCommonDeleteFields(plan, state)
	want := []string{"nodes", "ipam", "dnszone", "reversedns"}
	if len(got) != len(want) {
		t.Fatalf("delete fields = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("delete fields = %v, want %v", got, want)
		}
	}
}
