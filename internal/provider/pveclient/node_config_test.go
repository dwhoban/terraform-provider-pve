// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// TestNodeConfig_RoundTripMarshalUnmarshal confirms the slice ↔ map
// translation is lossless and deterministic (domain order is sorted on
// read).
func TestNodeConfig_RoundTripMarshalUnmarshal(t *testing.T) {
	in := NodeConfig{
		Description:         "primary host",
		Wakeonlan:           "AA:BB:CC:DD:EE:FF",
		StartAllOnBootDelay: 5,
		ACMEDomains: []NodeACMEDomain{
			{Domain: "zeta.example.com", Alias: []string{"zeta"}},
			{Domain: "alpha.example.com", Alias: nil},
		},
	}
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(raw), `"acme":{`) {
		t.Fatalf("marshal did not emit acme map: %s", raw)
	}
	if !strings.Contains(string(raw), `"alpha.example.com"`) || !strings.Contains(string(raw), `"zeta.example.com"`) {
		t.Fatalf("acme map missing one of the domains: %s", raw)
	}
	var out NodeConfig
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Description != in.Description || out.Wakeonlan != in.Wakeonlan || out.StartAllOnBootDelay != in.StartAllOnBootDelay {
		t.Fatalf("scalar fields drifted: %+v", out)
	}
	if len(out.ACMEDomains) != 2 {
		t.Fatalf("acme domains = %d, want 2", len(out.ACMEDomains))
	}
	// Order should be alphabetical on the way back in regardless of input order.
	if out.ACMEDomains[0].Domain != "alpha.example.com" || out.ACMEDomains[1].Domain != "zeta.example.com" {
		t.Fatalf("acme domains not sorted on read: %+v", out.ACMEDomains)
	}
}

// TestClient_GetNodeConfig_Success asserts GET shape and decoding.
func TestClient_GetNodeConfig_Success(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/nodes/pve1/config" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"description":"primary","wakeonlan":"AA:BB:CC:DD:EE:FF","acme":{"zeta.example.com":{"alias":["zeta"]},"alpha.example.com":{}},"startall_onboot_delay":7,"digest":"abc123"}}`)
	})
	cfg, err := c.GetNodeConfig(context.Background(), "pve1")
	if err != nil {
		t.Fatalf("GetNodeConfig: %v", err)
	}
	if cfg.Description != "primary" || cfg.Wakeonlan != "AA:BB:CC:DD:EE:FF" || cfg.StartAllOnBootDelay != 7 || cfg.Digest != "abc123" {
		t.Fatalf("scalar fields wrong: %+v", cfg)
	}
	if len(cfg.ACMEDomains) != 2 {
		t.Fatalf("acme domains = %d, want 2", len(cfg.ACMEDomains))
	}
}

// TestClient_UpdateNodeConfig_BodyShape confirms the body PVE receives has
// the map-shaped `acme` field (not a slice).
func TestClient_UpdateNodeConfig_BodyShape(t *testing.T) {
	var captured []byte
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/nodes/pve1/config" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		captured, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":null}`)
	})
	err := c.UpdateNodeConfig(context.Background(), "pve1", NodeConfig{
		Description: "primary",
		ACMEDomains: []NodeACMEDomain{{Domain: "example.com"}},
		Digest:      "digest-xyz",
	})
	if err != nil {
		t.Fatalf("UpdateNodeConfig: %v", err)
	}
	body := string(captured)
	if !strings.Contains(body, `"description":"primary"`) {
		t.Fatalf("body missing description: %s", body)
	}
	if !strings.Contains(body, `"acme":`) || !strings.Contains(body, `"example.com":`) {
		t.Fatalf("body missing acme map: %s", body)
	}
	if !strings.Contains(body, `"digest":"digest-xyz"`) {
		t.Fatalf("body missing digest: %s", body)
	}
	// Must not contain the slice-shaped default tag name.
	if strings.Contains(body, `"ACMEDomains"`) {
		t.Fatalf("body leaked struct field name: %s", body)
	}
}

// TestClient_GetNodeConfig_404SurfacesError asserts the typed APIError
// surfaces from a 404.
func TestClient_GetNodeConfig_404SurfacesError(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"errors":"Configuration for node 'ghost' not available"}`)
	})
	_, err := c.GetNodeConfig(context.Background(), "ghost")
	if err == nil {
		t.Fatal("expected error for missing node, got nil")
	}
	if !strings.Contains(err.Error(), "HTTP 404") {
		t.Fatalf("error missing HTTP 404: %v", err)
	}
}
