// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

// TestClient_ListNodeNetwork_ArrayResponse decodes a list response.
func TestClient_ListNodeNetwork_ArrayResponse(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/nodes/pve1/network" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"iface":"vmbr0","type":"bridge","autostart":1,"active":1,"bridge_ports":"eno1","cidr":"10.0.0.1/24","gateway":"10.0.0.254"}]}`)
	})
	ifaces, err := c.ListNodeNetwork(context.Background(), "pve1")
	if err != nil {
		t.Fatalf("ListNodeNetwork: %v", err)
	}
	if len(ifaces) != 1 || ifaces[0].Iface != "vmbr0" || ifaces[0].Type != "bridge" || ifaces[0].Autostart == nil || !*ifaces[0].Autostart {
		t.Fatalf("unexpected ifaces: %+v", ifaces)
	}
}

// TestClient_GetNodeNetwork_ArrayResponse decodes the per-iface single-element
// array form.
func TestClient_GetNodeNetwork_ArrayResponse(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nodes/pve1/network/vmbr0" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"iface":"vmbr0","type":"bridge","autostart":1,"active":1,"digest":"d-1"}]}`)
	})
	iface, err := c.GetNodeNetwork(context.Background(), "pve1", "vmbr0")
	if err != nil {
		t.Fatalf("GetNodeNetwork: %v", err)
	}
	if iface == nil || iface.Iface != "vmbr0" || iface.Digest != "d-1" {
		t.Fatalf("iface = %+v", iface)
	}
}

// TestClient_UpdateNodeNetwork_DeleteTranslatedToQuery verifies the `delete`
// slice is joined and URL-encoded into the query parameter.
func TestClient_UpdateNodeNetwork_DeleteTranslatedToQuery(t *testing.T) {
	var sawQuery string
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Fatalf("expected PUT, got %s", r.Method)
		}
		sawQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":null}`)
	})
	iface := NetworkInterface{Iface: "vmbr0", Type: "bridge"}
	if err := c.UpdateNodeNetwork(context.Background(), "pve1", "vmbr0", iface, []string{"address", "gateway"}); err != nil {
		t.Fatalf("UpdateNodeNetwork: %v", err)
	}
	if !strings.HasPrefix(sawQuery, "delete=") {
		t.Fatalf("delete not in query: %s", sawQuery)
	}
	if !strings.Contains(sawQuery, "address") || !strings.Contains(sawQuery, "gateway") {
		t.Fatalf("delete query missing fields: %s", sawQuery)
	}
}

// TestClient_DeleteNodeNetwork_404IsAPIErrors confirms DeleteNodeNetwork
// surfaces a 404 as *APIError so the resource layer can map it to a
// "not-found" outcome.
func TestClient_DeleteNodeNetwork_404IsAPIErrors(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"errors":"interface vmbr99 not found"}`)
	})
	err := c.DeleteNodeNetwork(context.Background(), "pve1", "vmbr99")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "HTTP 404") {
		t.Fatalf("error missing 404: %v", err)
	}
}

// TestClient_ReloadNodeNetwork_ReturnsUpid covers the upid return path
// used by the future pve_node_network_reload action.
func TestClient_ReloadNodeNetwork_ReturnsUpid(t *testing.T) {
	upid := "UPID:pve1:00001234:12345678:NETWORKRELOAD:operator:root@pam:"
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/nodes/pve1/network" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":"`+upid+`"}`)
	})
	got, err := c.ReloadNodeNetwork(context.Background(), "pve1")
	if err != nil {
		t.Fatalf("ReloadNodeNetwork: %v", err)
	}
	if got != upid {
		t.Fatalf("got upid %q, want %q", got, upid)
	}
}

// TestClient_GetNodeNetwork_BridgeVLANAwareBoolish verifies the
// bridge_vlan_aware field decodes from both the boolean and the 0/1 int
// encodings PVE emits, and stays nil when absent.
func TestClient_GetNodeNetwork_BridgeVLANAwareBoolish(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nodes/pve1/network/vmbr0" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"iface":"vmbr0","type":"bridge","bridge_vlan_aware":1,"bridge_vids":"2 4 100-200","bond_xmit_hash_policy":"layer3+4","digest":"d-1"}]}`)
	})
	iface, err := c.GetNodeNetwork(context.Background(), "pve1", "vmbr0")
	if err != nil {
		t.Fatalf("GetNodeNetwork: %v", err)
	}
	if iface.BridgeVLANAware == nil || !*iface.BridgeVLANAware {
		t.Fatalf("BridgeVLANAware = %v, want pointer to true", iface.BridgeVLANAware)
	}
	if iface.BridgeVIDs != "2 4 100-200" {
		t.Fatalf("BridgeVIDs = %q", iface.BridgeVIDs)
	}
	if iface.BondXmitHashPolicy != "layer3+4" {
		t.Fatalf("BondXmitHashPolicy = %q", iface.BondXmitHashPolicy)
	}
}

// TestClient_ListNodeNetwork_BridgeVLANAwareEncodings covers the false and
// absent encodings alongside the int-1 form.
func TestClient_ListNodeNetwork_BridgeVLANAwareEncodings(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[
			{"iface":"vmbr0","type":"bridge","bridge_vlan_aware":true},
			{"iface":"vmbr1","type":"bridge","bridge_vlan_aware":false},
			{"iface":"vmbr2","type":"bridge"}
		]}`)
	})
	ifaces, err := c.ListNodeNetwork(context.Background(), "pve1")
	if err != nil {
		t.Fatalf("ListNodeNetwork: %v", err)
	}
	want := []*bool{boolPtr(true), boolPtr(false), nil}
	for i, w := range want {
		got := ifaces[i].BridgeVLANAware
		switch {
		case w == nil && got != nil:
			t.Fatalf("iface %d: BridgeVLANAware = %v, want nil", i, *got)
		case w != nil && (got == nil || *got != *w):
			t.Fatalf("iface %d: BridgeVLANAware = %v, want %v", i, got, *w)
		}
	}
}

// boolPtr is a test helper for building expected *bool values.
func boolPtr(b bool) *bool { return &b }
