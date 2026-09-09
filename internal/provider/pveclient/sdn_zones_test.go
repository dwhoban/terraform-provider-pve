// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"
)

// sdnZoneWantBody describes the decoded-JSON create body a zone test
// expects: scalar and array values keyed by wire name.
type sdnZoneWantBody map[string]any

// TestSdnZonesList decodes GET /cluster/sdn/zones with the pin's optional
// type filter, tolerating the 0/1 boolean encoding and the string encoding
// of numeric settings that sdn.cfg emits.
func TestSdnZonesList(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/cluster/sdn/zones" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.URL.Query().Get("type"); got != "vlan" {
			t.Fatalf("type query = %q, want vlan", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[`+
			`{"zone":"zone1","type":"vlan","bridge":"vmbr0","bridge-disable-mac-learning":1,"mtu":"1499","nodes":"pve1,pve2","digest":"aa11"},`+
			`{"zone":"zone2","type":"vlan","bridge":"vmbr1","bridge-disable-mac-learning":false,"mtu":1500}]}`)
	})
	zones, err := c.ListSdnZones(context.Background(), "vlan")
	if err != nil {
		t.Fatalf("ListSdnZones: %v", err)
	}
	if len(zones) != 2 {
		t.Fatalf("got %d zones, want 2", len(zones))
	}
	first := zones[0]
	if first.Zone != "zone1" || first.Type != "vlan" || first.Bridge != "vmbr0" || first.Nodes != "pve1,pve2" {
		t.Fatalf("unexpected first zone: %+v", first)
	}
	if first.BridgeDisableMacLearning == nil || !*first.BridgeDisableMacLearning {
		t.Fatalf("bridge-disable-mac-learning = %v, want true", first.BridgeDisableMacLearning)
	}
	if first.MTU == nil || *first.MTU != 1499 {
		t.Fatalf("mtu = %v, want 1499 (string encoding)", first.MTU)
	}
	if first.Digest != "aa11" {
		t.Fatalf("digest = %q", first.Digest)
	}
	second := zones[1]
	if second.BridgeDisableMacLearning == nil || *second.BridgeDisableMacLearning {
		t.Fatalf("bridge-disable-mac-learning = %v, want false", second.BridgeDisableMacLearning)
	}
	if second.MTU == nil || *second.MTU != 1500 {
		t.Fatalf("mtu = %v, want 1500", second.MTU)
	}
}

// TestSdnZonesList_NoFilter confirms the list request carries no query
// parameters when no type filter is supplied.
func TestSdnZonesList_NoFilter(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" {
			t.Fatalf("unexpected query: %q", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[]}`)
	})
	zones, err := c.ListSdnZones(context.Background(), "")
	if err != nil {
		t.Fatalf("ListSdnZones: %v", err)
	}
	if len(zones) != 0 {
		t.Fatalf("got %d zones, want 0", len(zones))
	}
}

// TestSdnZonesGet decodes GET /cluster/sdn/zones/{zone} for the largest
// per-type field set (evpn), including the 0/1 encodings and the read-only
// digest.
func TestSdnZonesGet(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/cluster/sdn/zones/evpn1" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"zone":"evpn1","type":"evpn","controller":"ctrl1",`+
			`"secondary-controllers":["ctrl2","ctrl3"],"advertise-subnets":1,`+
			`"disable-arp-nd-suppression":0,"exitnodes":"pve1,pve2","exitnodes-local-routing":true,`+
			`"exitnodes-primary":"pve1","mac":"AA:BB:CC:DD:EE:FF","rt-import":"65000:100",`+
			`"vrf-vxlan":"16777215","mtu":1450,"nodes":"pve1","digest":"bb22"}}`)
	})
	z, err := c.GetSdnZone(context.Background(), "evpn1")
	if err != nil {
		t.Fatalf("GetSdnZone: %v", err)
	}
	if z.Zone != "evpn1" || z.Type != "evpn" || z.Controller != "ctrl1" {
		t.Fatalf("unexpected identity fields: %+v", z)
	}
	if len(z.SecondaryControllers) != 2 || z.SecondaryControllers[1] != "ctrl3" {
		t.Fatalf("secondary-controllers = %v", z.SecondaryControllers)
	}
	if z.AdvertiseSubnets == nil || !*z.AdvertiseSubnets {
		t.Fatalf("advertise-subnets = %v, want true", z.AdvertiseSubnets)
	}
	if z.DisableArpNdSuppression == nil || *z.DisableArpNdSuppression {
		t.Fatalf("disable-arp-nd-suppression = %v, want false", z.DisableArpNdSuppression)
	}
	if z.ExitNodes != "pve1,pve2" || z.ExitNodesPrimary != "pve1" {
		t.Fatalf("unexpected exitnodes: %+v", z)
	}
	if z.ExitNodesLocalRouting == nil || !*z.ExitNodesLocalRouting {
		t.Fatalf("exitnodes-local-routing = %v, want true", z.ExitNodesLocalRouting)
	}
	if z.Mac != "AA:BB:CC:DD:EE:FF" || z.RtImport != "65000:100" {
		t.Fatalf("unexpected mac/rt-import: %+v", z)
	}
	if z.VrfVxlan == nil || *z.VrfVxlan != 16777215 {
		t.Fatalf("vrf-vxlan = %v, want 16777215 (string encoding)", z.VrfVxlan)
	}
	if z.MTU == nil || *z.MTU != 1450 || z.Nodes != "pve1" || z.Digest != "bb22" {
		t.Fatalf("unexpected shared fields: %+v", z)
	}
}

// TestSdnZonesCreate_PerType verifies POST /cluster/sdn/zones for each
// per-type body: the fixed type travels and only that type's fields are
// sent — fields of other zone types must never leak into the body.
func TestSdnZonesCreate_PerType(t *testing.T) {
	cases := []struct {
		name    string
		zone    SdnZone
		want    sdnZoneWantBody
		wantNot []string
	}{
		{
			name: "simple",
			zone: SdnZone{
				Zone: "sim1", Type: "simple", MTU: int64Ptr(1500), Nodes: "pve1",
				IPAM: "pve", DNS: "dns1", DNSZone: "example.com", ReverseDNS: "rdns1", DHCP: "dnsmasq",
			},
			want: map[string]any{
				"zone": "sim1", "type": "simple", "mtu": float64(1500), "nodes": "pve1",
				"ipam": "pve", "dns": "dns1", "dnszone": "example.com", "reversedns": "rdns1", "dhcp": "dnsmasq",
			},
			wantNot: []string{"bridge", "tag", "vlan-protocol", "peers", "vxlan-port", "controller", "vrf-vxlan"},
		},
		{
			name: "vlan",
			zone: SdnZone{
				Zone: "vlan1", Type: "vlan", Bridge: "vmbr0", BridgeDisableMacLearning: boolPtr(true), MTU: int64Ptr(1500),
			},
			want: map[string]any{
				"zone": "vlan1", "type": "vlan", "bridge": "vmbr0", "bridge-disable-mac-learning": true, "mtu": float64(1500),
			},
			wantNot: []string{"tag", "vlan-protocol", "peers", "controller"},
		},
		{
			name: "qinq",
			zone: SdnZone{
				Zone: "qinq1", Type: "qinq", Bridge: "vmbr0", Tag: int64Ptr(100), VlanProtocol: "802.1ad",
			},
			want: map[string]any{
				"zone": "qinq1", "type": "qinq", "bridge": "vmbr0", "tag": float64(100), "vlan-protocol": "802.1ad",
			},
			wantNot: []string{"bridge-disable-mac-learning", "peers", "controller"},
		},
		{
			name: "vxlan",
			zone: SdnZone{
				Zone: "vx1", Type: "vxlan", Peers: "10.0.0.1,10.0.0.2", VxlanPort: int64Ptr(4789),
				Fabric: "fib1", MTU: int64Ptr(1450),
			},
			want: map[string]any{
				"zone": "vx1", "type": "vxlan", "peers": "10.0.0.1,10.0.0.2", "vxlan-port": float64(4789),
				"fabric": "fib1", "mtu": float64(1450),
			},
			wantNot: []string{"bridge", "tag", "controller", "vrf-vxlan"},
		},
		{
			name: "evpn",
			zone: SdnZone{
				Zone: "evpn1", Type: "evpn", Controller: "ctrl1",
				SecondaryControllers:    []string{"ctrl2", "ctrl3"},
				AdvertiseSubnets:        boolPtr(true),
				DisableArpNdSuppression: boolPtr(false),
				ExitNodes:               "pve1,pve2",
				ExitNodesLocalRouting:   boolPtr(true),
				ExitNodesPrimary:        "pve1",
				Mac:                     "AA:BB:CC:DD:EE:FF",
				RtImport:                "65000:100",
				VrfVxlan:                int64Ptr(16777215),
			},
			want: map[string]any{
				"zone": "evpn1", "type": "evpn", "controller": "ctrl1",
				"secondary-controllers":      []any{"ctrl2", "ctrl3"},
				"advertise-subnets":          true,
				"disable-arp-nd-suppression": false,
				"exitnodes":                  "pve1,pve2",
				"exitnodes-local-routing":    true,
				"exitnodes-primary":          "pve1",
				"mac":                        "AA:BB:CC:DD:EE:FF",
				"rt-import":                  "65000:100",
				"vrf-vxlan":                  float64(16777215),
			},
			wantNot: []string{"bridge", "tag", "peers", "vxlan-port", "fabric"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var sawBody map[string]any
			c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/cluster/sdn/zones" {
					t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				raw, _ := io.ReadAll(r.Body)
				if err := json.Unmarshal(raw, &sawBody); err != nil {
					t.Fatalf("request body not JSON: %v", err)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"data":null}`)
			})
			if err := c.CreateSdnZone(context.Background(), tc.zone); err != nil {
				t.Fatalf("CreateSdnZone: %v", err)
			}
			for key, want := range tc.want {
				got, ok := sawBody[key]
				if !ok {
					t.Fatalf("body missing %s: %v", key, sawBody)
				}
				if wantList, isList := want.([]any); isList {
					wantJSON, _ := json.Marshal(wantList)
					gotJSON, _ := json.Marshal(got)
					if string(wantJSON) != string(gotJSON) {
						t.Fatalf("%s = %v, want %v", key, got, want)
					}
				} else if got != want {
					t.Fatalf("%s = %v, want %v", key, got, want)
				}
			}
			for _, key := range tc.wantNot {
				if _, ok := sawBody[key]; ok {
					t.Fatalf("%s must not be sent for a %s zone: %v", key, tc.zone.Type, sawBody)
				}
			}
			if _, ok := sawBody["digest"]; ok {
				t.Fatalf("digest is read-only and must not be sent: %v", sawBody)
			}
		})
	}
}

// TestSdnZonesCreate_MissingRequired confirms the client rejects creates
// missing the pin's required parameters before hitting the wire.
func TestSdnZonesCreate_MissingRequired(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("no request expected, got %s %s", r.Method, r.URL.Path)
	})
	if err := c.CreateSdnZone(context.Background(), SdnZone{Type: "vlan"}); err == nil {
		t.Fatal("expected error for missing zone, got nil")
	}
	if err := c.CreateSdnZone(context.Background(), SdnZone{Zone: "vlan1"}); err == nil {
		t.Fatal("expected error for missing type, got nil")
	}
}

// TestSdnZonesUpdate verifies PUT /cluster/sdn/zones/{zone}: zone travels
// in the body (required per the pin), type never does (the zone type is
// immutable), and the delete slice travels as the `delete` query parameter.
func TestSdnZonesUpdate(t *testing.T) {
	var sawBody map[string]any
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/cluster/sdn/zones/vlan1" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.URL.Query().Get("delete"); got != "mtu,ipam" {
			t.Fatalf("delete query = %q, want mtu,ipam", got)
		}
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &sawBody); err != nil {
			t.Fatalf("request body not JSON: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":null}`)
	})
	z := SdnZone{Zone: "vlan1", Type: "vlan", Bridge: "vmbr1", MTU: int64Ptr(1500)}
	if err := c.UpdateSdnZone(context.Background(), "vlan1", z, []string{"mtu", "ipam"}); err != nil {
		t.Fatalf("UpdateSdnZone: %v", err)
	}
	if sawBody["zone"] != "vlan1" {
		t.Fatalf("zone = %v, want vlan1", sawBody["zone"])
	}
	if _, ok := sawBody["type"]; ok {
		t.Fatalf("type must not be sent on update: %v", sawBody)
	}
	if sawBody["bridge"] != "vmbr1" || sawBody["mtu"] != float64(1500) {
		t.Fatalf("unexpected update fields: %v", sawBody)
	}
}

// TestSdnZonesDelete confirms DELETE /cluster/sdn/zones/{zone} and that a
// 404 surfaces as *APIError so the resource layer can map it to "already
// absent".
func TestSdnZonesDelete(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/cluster/sdn/zones/vlan1" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":null}`)
	})
	if err := c.DeleteSdnZone(context.Background(), "vlan1"); err != nil {
		t.Fatalf("DeleteSdnZone: %v", err)
	}

	miss := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"errors":"zone 'vlan1' does not exist"}`)
	})
	err := miss.DeleteSdnZone(context.Background(), "vlan1")
	if err == nil {
		t.Fatal("expected 404 error, got nil")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 404 {
		t.Fatalf("expected *APIError 404, got %v", err)
	}
}
