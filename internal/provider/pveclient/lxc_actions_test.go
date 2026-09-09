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

// lxcActionsUPID is the canned UPID returned by every mutating LXC
// action endpoint in these tests.
const lxcActionsUPID = "UPID:pve1:00000010:abcdef01:lxc_reboot:root@pam:"

// lxcActionsRespondUPID writes the standard {"data":"UPID:..."} envelope.
func lxcActionsRespondUPID(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, `{"data":"`+lxcActionsUPID+`"}`)
}

// TestLxcRebootWireBody asserts POST .../status/reboot carries the optional
// timeout and decodes the task UPID.
func TestLxcRebootWireBody(t *testing.T) {
	var captured map[string]any
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/nodes/pve1/lxc/100/status/reboot" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&captured)
		lxcActionsRespondUPID(w)
	})
	timeout := int64(30)
	upid, err := c.LxcReboot(context.Background(), "pve1", 100, &LxcRebootParams{TimeoutSeconds: &timeout})
	if err != nil {
		t.Fatalf("LxcReboot: %v", err)
	}
	if upid != lxcActionsUPID {
		t.Fatalf("upid = %q, want %q", upid, lxcActionsUPID)
	}
	if captured["timeout"] != float64(30) {
		t.Fatalf("unexpected body: %+v", captured)
	}
}

// TestLxcRebootNilParamsOmitsTimeout asserts a nil params struct sends no
// timeout key.
func TestLxcRebootNilParamsOmitsTimeout(t *testing.T) {
	var captured map[string]any
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&captured)
		lxcActionsRespondUPID(w)
	})
	if _, err := c.LxcReboot(context.Background(), "pve1", 100, nil); err != nil {
		t.Fatalf("LxcReboot: %v", err)
	}
	if _, ok := captured["timeout"]; ok {
		t.Fatalf("timeout should be omitted, got: %+v", captured)
	}
}

// TestLxcSuspendWireBody asserts POST .../status/suspend takes only the URL
// keys and returns the task UPID.
func TestLxcSuspendWireBody(t *testing.T) {
	var sawBody []byte
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/nodes/pve1/lxc/100/status/suspend" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		sawBody, _ = io.ReadAll(r.Body)
		lxcActionsRespondUPID(w)
	})
	if _, err := c.LxcSuspend(context.Background(), "pve1", 100); err != nil {
		t.Fatalf("LxcSuspend: %v", err)
	}
	if len(sawBody) != 0 {
		t.Fatalf("suspend takes no body, got %q", sawBody)
	}
}

// TestLxcResumeWireBody asserts POST .../status/resume takes only the URL
// keys and returns the task UPID.
func TestLxcResumeWireBody(t *testing.T) {
	var sawBody []byte
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/nodes/pve1/lxc/100/status/resume" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		sawBody, _ = io.ReadAll(r.Body)
		lxcActionsRespondUPID(w)
	})
	if _, err := c.LxcResume(context.Background(), "pve1", 100); err != nil {
		t.Fatalf("LxcResume: %v", err)
	}
	if len(sawBody) != 0 {
		t.Fatalf("resume takes no body, got %q", sawBody)
	}
}

// TestCreateLxcSnapshotWireBody asserts POST .../snapshot carries snapname
// and description and returns the task UPID.
func TestCreateLxcSnapshotWireBody(t *testing.T) {
	var captured map[string]any
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/nodes/pve1/lxc/100/snapshot" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&captured)
		lxcActionsRespondUPID(w)
	})
	upid, err := c.CreateLxcSnapshot(context.Background(), "pve1", 100, "pre-upgrade", "before upgrade")
	if err != nil {
		t.Fatalf("CreateLxcSnapshot: %v", err)
	}
	if upid != lxcActionsUPID {
		t.Fatalf("upid = %q, want %q", upid, lxcActionsUPID)
	}
	if captured["snapname"] != "pre-upgrade" || captured["description"] != "before upgrade" {
		t.Fatalf("unexpected body: %+v", captured)
	}
}

// TestCreateLxcSnapshotEmptyDescriptionOmitsKey asserts an empty description
// is not sent.
func TestCreateLxcSnapshotEmptyDescriptionOmitsKey(t *testing.T) {
	var captured map[string]any
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&captured)
		lxcActionsRespondUPID(w)
	})
	if _, err := c.CreateLxcSnapshot(context.Background(), "pve1", 100, "snap1", ""); err != nil {
		t.Fatalf("CreateLxcSnapshot: %v", err)
	}
	if _, ok := captured["description"]; ok {
		t.Fatalf("description should be omitted, got: %+v", captured)
	}
}

// TestListLxcSnapshotsDecodesItems asserts GET .../snapshot decodes the
// snapshot item list.
func TestListLxcSnapshotsDecodesItems(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/nodes/pve1/lxc/100/snapshot" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[`+
			`{"name":"current","description":"You are here!"},`+
			`{"name":"pre-upgrade","description":"before","parent":"current","snaptime":1700000000}]}`)
	})
	snaps, err := c.ListLxcSnapshots(context.Background(), "pve1", 100)
	if err != nil {
		t.Fatalf("ListLxcSnapshots: %v", err)
	}
	if len(snaps) != 2 {
		t.Fatalf("len = %d, want 2", len(snaps))
	}
	if snaps[1].Name != "pre-upgrade" || snaps[1].Parent != "current" {
		t.Fatalf("unexpected second entry: %+v", snaps[1])
	}
	if snaps[1].Snaptime == nil || *snaps[1].Snaptime != 1700000000 {
		t.Fatalf("unexpected snaptime: %+v", snaps[1].Snaptime)
	}
}

// TestGetLxcSnapshotConfigDecodesBody asserts GET .../snapshot/{snap}/config
// decodes the snapshot configuration.
func TestGetLxcSnapshotConfigDecodesBody(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/nodes/pve1/lxc/100/snapshot/pre-upgrade/config" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"description":"before","digest":"abc123","parent":"current","snaptime":1700000000}}`)
	})
	cfg, err := c.GetLxcSnapshotConfig(context.Background(), "pve1", 100, "pre-upgrade")
	if err != nil {
		t.Fatalf("GetLxcSnapshotConfig: %v", err)
	}
	if cfg.Description != "before" || cfg.Digest != "abc123" || cfg.Parent != "current" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	if cfg.Snaptime == nil || *cfg.Snaptime != 1700000000 {
		t.Fatalf("unexpected snaptime: %+v", cfg.Snaptime)
	}
}

// TestGetLxcSnapshotConfigNotFound asserts a missing snapshot surfaces the
// 404 APIError callers use to drop state.
func TestGetLxcSnapshotConfigNotFound(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"errors":"no such snapshot"}`)
	})
	_, err := c.GetLxcSnapshotConfig(context.Background(), "pve1", 100, "gone")
	if err == nil {
		t.Fatal("expected error for missing snapshot")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 APIError, got %v", err)
	}
}

// TestUpdateLxcSnapshotConfigWireBody asserts PUT .../snapshot/{snap}/config
// carries the description, including an empty string that clears it.
func TestUpdateLxcSnapshotConfigWireBody(t *testing.T) {
	var captured map[string]any
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/nodes/pve1/lxc/100/snapshot/pre-upgrade/config" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&captured)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":null}`)
	})
	if err := c.UpdateLxcSnapshotConfig(context.Background(), "pve1", 100, "pre-upgrade", "new text"); err != nil {
		t.Fatalf("UpdateLxcSnapshotConfig: %v", err)
	}
	if captured["description"] != "new text" {
		t.Fatalf("unexpected body: %+v", captured)
	}
}

// TestDeleteLxcSnapshotWireBody asserts DELETE .../snapshot/{snap} returns a
// task UPID and forwards force.
func TestDeleteLxcSnapshotWireBody(t *testing.T) {
	var captured map[string]any
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/nodes/pve1/lxc/100/snapshot/pre-upgrade" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&captured)
		lxcActionsRespondUPID(w)
	})
	upid, err := c.DeleteLxcSnapshot(context.Background(), "pve1", 100, "pre-upgrade", true)
	if err != nil {
		t.Fatalf("DeleteLxcSnapshot: %v", err)
	}
	if upid != lxcActionsUPID {
		t.Fatalf("upid = %q, want %q", upid, lxcActionsUPID)
	}
	if captured["force"] != true {
		t.Fatalf("unexpected body: %+v", captured)
	}
}

// TestRollbackLxcSnapshotWireBody asserts POST .../snapshot/{snap}/rollback
// carries the start flag and returns the task UPID.
func TestRollbackLxcSnapshotWireBody(t *testing.T) {
	var captured map[string]any
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/nodes/pve1/lxc/100/snapshot/pre-upgrade/rollback" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&captured)
		lxcActionsRespondUPID(w)
	})
	upid, err := c.RollbackLxcSnapshot(context.Background(), "pve1", 100, "pre-upgrade", true)
	if err != nil {
		t.Fatalf("RollbackLxcSnapshot: %v", err)
	}
	if upid != lxcActionsUPID {
		t.Fatalf("upid = %q, want %q", upid, lxcActionsUPID)
	}
	if captured["start"] != true {
		t.Fatalf("unexpected body: %+v", captured)
	}
}
