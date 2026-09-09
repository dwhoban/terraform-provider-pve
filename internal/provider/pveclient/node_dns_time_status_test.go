// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

// TestClient_GetNodeDNS_Decodes covers the DNS GET path.
func TestClient_GetNodeDNS_Decodes(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nodes/pve1/dns" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"search":"example.com","dns1":"1.1.1.1","dns2":"1.0.0.1","dns3":"8.8.8.8"}}`)
	})
	dns, err := c.GetNodeDNS(context.Background(), "pve1")
	if err != nil {
		t.Fatalf("GetNodeDNS: %v", err)
	}
	if dns.Search != "example.com" || dns.DNS1 != "1.1.1.1" || dns.DNS2 != "1.0.0.1" || dns.DNS3 != "8.8.8.8" {
		t.Fatalf("unexpected dns: %+v", dns)
	}
}

// TestClient_SetNodeDNS_BodyShape asserts the PUT body matches the input.
func TestClient_SetNodeDNS_BodyShape(t *testing.T) {
	var captured map[string]string
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Fatalf("expected PUT, got %s", r.Method)
		}
		_ = json.NewDecoder(r.Body).Decode(&captured)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":null}`)
	})
	if err := c.SetNodeDNS(context.Background(), "pve1", NodeDNS{Search: "example.com", DNS1: "1.1.1.1"}); err != nil {
		t.Fatalf("SetNodeDNS: %v", err)
	}
	if captured["search"] != "example.com" || captured["dns1"] != "1.1.1.1" {
		t.Fatalf("unexpected body: %+v", captured)
	}
}

// TestClient_GetSetNodeTime covers both directions of the time endpoint.
func TestClient_GetSetNodeTime(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/nodes/pve1/time":
			_, _ = io.WriteString(w, `{"data":{"timezone":"Europe/Berlin"}}`)
		case r.Method == http.MethodPut && r.URL.Path == "/nodes/pve1/time":
			_, _ = io.WriteString(w, `{"data":null}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	t1, err := c.GetNodeTime(context.Background(), "pve1")
	if err != nil {
		t.Fatalf("GetNodeTime: %v", err)
	}
	if t1.Timezone != "Europe/Berlin" {
		t.Fatalf("Timezone = %q", t1.Timezone)
	}
	if err := c.SetNodeTime(context.Background(), "pve1", NodeTime{Timezone: "UTC"}); err != nil {
		t.Fatalf("SetNodeTime: %v", err)
	}
}

// TestClient_GetNodeStatus_Decodes checks the status endpoint round trip.
func TestClient_GetNodeStatus_Decodes(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nodes/pve1/status" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"cpu":0.07,"maxcpu":8,"mem":8589934592,"maxmem":17179869184,"uptime":1234567,"level":"c","rootfs":{"total":536870912000,"used":123456789000,"free":413414123000},"kernel":"6.5.13-1-pve","pveversion":"8.2.4"}}`)
	})
	st, err := c.GetNodeStatus(context.Background(), "pve1")
	if err != nil {
		t.Fatalf("GetNodeStatus: %v", err)
	}
	if st.MaxCPU != 8 || st.PVEVersion != "8.2.4" || st.RootFS.Total != 536870912000 {
		t.Fatalf("unexpected status: %+v", st)
	}
}

// TestClient_ListClusterNodes_Decodes checks the /nodes cluster endpoint.
func TestClient_ListClusterNodes_Decodes(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nodes" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"node":"pve1","status":"online","cpu":0.05,"maxcpu":8,"mem":1,"maxmem":2,"level":"c","uptime":1234},{"node":"pve2","status":"offline","cpu":0,"maxcpu":4,"mem":0,"maxmem":0,"level":"","uptime":0}]}`)
	})
	nodes, err := c.ListClusterNodes(context.Background())
	if err != nil {
		t.Fatalf("ListClusterNodes: %v", err)
	}
	if len(nodes) != 2 || nodes[0].Node != "pve1" || nodes[1].Status != "offline" {
		t.Fatalf("unexpected nodes: %+v", nodes)
	}
}
