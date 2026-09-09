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

// TestPveNodeRebootClientWireBody asserts POST /nodes/{node}/status carries
// the reboot command and decodes the null response.
func TestPveNodeRebootClientWireBody(t *testing.T) {
	var captured map[string]string
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/nodes/pve1/status" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&captured)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":null}`)
	})
	if err := c.NodeReboot(context.Background(), "pve1"); err != nil {
		t.Fatalf("NodeReboot: %v", err)
	}
	if captured["command"] != "reboot" {
		t.Fatalf("unexpected body: %+v", captured)
	}
}

// TestPveNodeShutdownClientWireBody asserts POST /nodes/{node}/status carries
// the shutdown command and decodes the null response.
func TestPveNodeShutdownClientWireBody(t *testing.T) {
	var captured map[string]string
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/nodes/pve1/status" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&captured)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":null}`)
	})
	if err := c.NodeShutdown(context.Background(), "pve1"); err != nil {
		t.Fatalf("NodeShutdown: %v", err)
	}
	if captured["command"] != "shutdown" {
		t.Fatalf("unexpected body: %+v", captured)
	}
}
