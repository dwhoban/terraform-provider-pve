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

// qemuWireBody is a decoded JSON request-body fixture: PVE verb bodies are
// flat string/number/bool parameter maps.
type qemuWireBody map[string]any

// TestQemuVMStatusVerbsWireBody asserts the reboot, suspend, resume, and
// reset verbs POST to the pin's paths, carry their optional parameters, and
// decode the returned task UPID.
func TestQemuVMStatusVerbsWireBody(t *testing.T) {
	cases := []struct {
		name     string
		call     func(c *Client, ctx context.Context) (string, error)
		wantPath string
		wantBody qemuWireBody
	}{
		{
			name: "reboot with timeout",
			call: func(c *Client, ctx context.Context) (string, error) {
				timeout := int64(60)
				return c.QemuVMReboot(ctx, "pve1", 100, QemuVMRebootOptions{Timeout: &timeout})
			},
			wantPath: "/nodes/pve1/qemu/100/status/reboot",
			wantBody: qemuWireBody{"timeout": float64(60)},
		},
		{
			name: "suspend to disk",
			call: func(c *Client, ctx context.Context) (string, error) {
				todisk := true
				return c.QemuVMSuspend(ctx, "pve1", 100, QemuVMSuspendOptions{Todisk: &todisk, StateStorage: "local-lvm"})
			},
			wantPath: "/nodes/pve1/qemu/100/status/suspend",
			wantBody: qemuWireBody{"todisk": true, "statestorage": "local-lvm"},
		},
		{
			name: "resume without options",
			call: func(c *Client, ctx context.Context) (string, error) {
				return c.QemuVMResume(ctx, "pve1", 100)
			},
			wantPath: "/nodes/pve1/qemu/100/status/resume",
		},
		{
			name: "reset without options",
			call: func(c *Client, ctx context.Context) (string, error) {
				return c.QemuVMReset(ctx, "pve1", 100)
			},
			wantPath: "/nodes/pve1/qemu/100/status/reset",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var body qemuWireBody
			var method, path string
			c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
				method, path = r.Method, r.URL.Path
				_ = json.NewDecoder(r.Body).Decode(&body)
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"data":"`+fakeUpid+`"}`)
			})
			upid, err := tc.call(c, context.Background())
			if err != nil {
				t.Fatalf("call: %v", err)
			}
			if method != http.MethodPost || path != tc.wantPath {
				t.Fatalf("request = %s %s, want POST %s", method, path, tc.wantPath)
			}
			if len(tc.wantBody) > 0 && !jsonEqual(body, tc.wantBody) {
				t.Fatalf("body = %v, want %v", body, tc.wantBody)
			}
			if len(tc.wantBody) == 0 && len(body) != 0 {
				t.Fatalf("body = %v, want empty", body)
			}
			if upid != fakeUpid {
				t.Fatalf("upid = %q, want %q", upid, fakeUpid)
			}
		})
	}
}

// TestQemuVMSnapshotCRUDWireBody covers create, get, list, delete (with
// force), and rollback of QEMU VM snapshots.
func TestQemuVMSnapshotCRUDWireBody(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/nodes/pve1/qemu/100/snapshot":
			var body qemuWireBody
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["snapname"] != "snap1" || body["description"] != "before upgrade" {
				t.Fatalf("create body = %v", body)
			}
			_, _ = io.WriteString(w, `{"data":"`+fakeUpid+`"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/nodes/pve1/qemu/100/snapshot/snap1":
			_, _ = io.WriteString(w, `{"data":{"description":"before upgrade","snaptime":1700000000,"vmstate":true,"parent":"base"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/nodes/pve1/qemu/100/snapshot":
			_, _ = io.WriteString(w, `{"data":[{"name":"current"},{"name":"snap1","description":"before upgrade","snaptime":1700000000}]}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/nodes/pve1/qemu/100/snapshot/snap1":
			var body qemuWireBody
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["force"] != true {
				t.Fatalf("delete body = %v", body)
			}
			_, _ = io.WriteString(w, `{"data":"`+fakeUpid+`"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/nodes/pve1/qemu/100/snapshot/snap1/rollback":
			if r.ContentLength != 0 {
				t.Fatalf("rollback should send no body, got %d bytes", r.ContentLength)
			}
			_, _ = io.WriteString(w, `{"data":"`+fakeUpid+`"}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	ctx := context.Background()

	upid, err := c.CreateQemuVMSnapshot(ctx, "pve1", 100, "snap1", QemuVMSnapshotOptions{Description: "before upgrade"})
	if err != nil || upid != fakeUpid {
		t.Fatalf("CreateQemuVMSnapshot: %q %v", upid, err)
	}
	snap, err := c.GetQemuVMSnapshot(ctx, "pve1", 100, "snap1")
	if err != nil {
		t.Fatalf("GetQemuVMSnapshot: %v", err)
	}
	if snap.Description != "before upgrade" || snap.Parent != "base" || snap.Snaptime == nil || *snap.Snaptime != 1700000000 || snap.Vmstate == nil || !*snap.Vmstate {
		t.Fatalf("snapshot = %+v", snap)
	}
	snaps, err := c.ListQemuVMSnapshots(ctx, "pve1", 100)
	if err != nil {
		t.Fatalf("ListQemuVMSnapshots: %v", err)
	}
	if len(snaps) != 1 || snaps[0].Name != "snap1" {
		t.Fatalf("snapshots = %+v", snaps)
	}
	upid, err = c.DeleteQemuVMSnapshot(ctx, "pve1", 100, "snap1", boolPtr(true))
	if err != nil || upid != fakeUpid {
		t.Fatalf("DeleteQemuVMSnapshot: %q %v", upid, err)
	}
	upid, err = c.RollbackQemuVMSnapshot(ctx, "pve1", 100, "snap1")
	if err != nil || upid != fakeUpid {
		t.Fatalf("RollbackQemuVMSnapshot: %q %v", upid, err)
	}
}

// jsonEqual compares two decoded JSON objects.
func jsonEqual(a, b qemuWireBody) bool {
	ab, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	return string(ab) == string(bb)
}
