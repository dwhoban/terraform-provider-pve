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

// TestClient_UnlockUserTFA verifies the unlock request hits
// PUT /access/users/{userid}/unlock-tfa with no body.
func TestClient_UnlockUserTFA(t *testing.T) {
	var sawMethod, sawPath, sawBody string
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		sawMethod, sawPath = r.Method, r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		sawBody = string(raw)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":true}`)
	})
	if err := c.UnlockUserTFA(context.Background(), "root@pam"); err != nil {
		t.Fatalf("UnlockUserTFA: %v", err)
	}
	if sawMethod != http.MethodPut || sawPath != "/access/users/root@pam/unlock-tfa" {
		t.Fatalf("unexpected request: %s %s", sawMethod, sawPath)
	}
	if strings.TrimSpace(sawBody) != "" {
		t.Fatalf("expected empty body, got %q", sawBody)
	}
}

// TestClient_MigrateHAResource verifies the HA migrate request shape.
func TestClient_MigrateHAResource(t *testing.T) {
	var sawMethod, sawPath string
	var body map[string]any
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		sawMethod, sawPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"sid":"vm:100","requested-node":"pve2"}}`)
	})
	if err := c.MigrateHAResource(context.Background(), "vm:100", "pve2"); err != nil {
		t.Fatalf("MigrateHAResource: %v", err)
	}
	if sawMethod != http.MethodPost || sawPath != "/cluster/ha/resources/vm:100/migrate" {
		t.Fatalf("unexpected request: %s %s", sawMethod, sawPath)
	}
	if body["node"] != "pve2" {
		t.Fatalf("body node = %v, want pve2", body["node"])
	}
}

// TestClient_RelocateHAResource verifies the HA relocate request shape.
func TestClient_RelocateHAResource(t *testing.T) {
	var sawMethod, sawPath string
	var body map[string]any
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		sawMethod, sawPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"sid":"ct:101","requested-node":"pve3"}}`)
	})
	if err := c.RelocateHAResource(context.Background(), "ct:101", "pve3"); err != nil {
		t.Fatalf("RelocateHAResource: %v", err)
	}
	if sawMethod != http.MethodPost || sawPath != "/cluster/ha/resources/ct:101/relocate" {
		t.Fatalf("unexpected request: %s %s", sawMethod, sawPath)
	}
	if body["node"] != "pve3" {
		t.Fatalf("body node = %v, want pve3", body["node"])
	}
}

// TestClient_RefreshSubscription verifies the subscription refresh request,
// including the optional force flag.
func TestClient_RefreshSubscription(t *testing.T) {
	var bodies []string
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/nodes/pve1/subscription" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(raw))
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":null}`)
	})
	if err := c.RefreshSubscription(context.Background(), "pve1", nil); err != nil {
		t.Fatalf("RefreshSubscription(force=nil): %v", err)
	}
	force := true
	if err := c.RefreshSubscription(context.Background(), "pve1", &force); err != nil {
		t.Fatalf("RefreshSubscription(force=true): %v", err)
	}
	if strings.Contains(bodies[0], "force") {
		t.Fatalf("first request should omit force, got %q", bodies[0])
	}
	if !strings.Contains(bodies[1], `"force":true`) {
		t.Fatalf("second request missing force=true, got %q", bodies[1])
	}
}

// TestClient_InitializeDiskGPT verifies the initgpt request shape, the
// returned UPID, and that an empty UUID is omitted.
func TestClient_InitializeDiskGPT(t *testing.T) {
	const upid = "UPID:pve1:0000ABCD:00000000:00000000:initgpt:0:root@pam:"
	var bodies []string
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/nodes/pve1/disks/initgpt" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(raw))
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":"`+upid+`"}`)
	})
	got, err := c.InitializeDiskGPT(context.Background(), "pve1", "/dev/sdb", "")
	if err != nil {
		t.Fatalf("InitializeDiskGPT: %v", err)
	}
	if got != upid {
		t.Fatalf("upid = %q, want %q", got, upid)
	}
	if !strings.Contains(bodies[0], `"disk":"/dev/sdb"`) || strings.Contains(bodies[0], "uuid") {
		t.Fatalf("unexpected first body: %q", bodies[0])
	}
	if _, err := c.InitializeDiskGPT(context.Background(), "pve1", "/dev/sdb", "12345678-1234-1234-1234-123456789abc"); err != nil {
		t.Fatalf("InitializeDiskGPT(uuid): %v", err)
	}
	if !strings.Contains(bodies[1], `"uuid":"12345678-1234-1234-1234-123456789abc"`) {
		t.Fatalf("unexpected second body: %q", bodies[1])
	}
}

// TestClient_CancelTask verifies the stop-task request is a DELETE on
// /nodes/{node}/tasks/{upid} with no body.
func TestClient_CancelTask(t *testing.T) {
	var sawMethod, sawPath, sawBody string
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		sawMethod, sawPath = r.Method, r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		sawBody = string(raw)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":null}`)
	})
	upid := "UPID:pve1:0000ABCD:00000000:00000000:vzdump:100:root@pam:"
	if err := c.CancelTask(context.Background(), "pve1", upid); err != nil {
		t.Fatalf("CancelTask: %v", err)
	}
	if sawMethod != http.MethodDelete || sawPath != "/nodes/pve1/tasks/"+upid {
		t.Fatalf("unexpected request: %s %s", sawMethod, sawPath)
	}
	if strings.TrimSpace(sawBody) != "" {
		t.Fatalf("expected empty body, got %q", sawBody)
	}
}

// TestClient_DownloadApplianceTemplate verifies the aplinfo download
// request shape and the returned UPID.
func TestClient_DownloadApplianceTemplate(t *testing.T) {
	const upid = "UPID:pve1:0000ABCD:00000000:00000000:aplinfo:0:root@pam:"
	var body map[string]any
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/nodes/pve1/aplinfo" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":"`+upid+`"}`)
	})
	got, err := c.DownloadApplianceTemplate(context.Background(), "pve1", "local", "debian-12-standard_12.2-1_amd64.tar.zst")
	if err != nil {
		t.Fatalf("DownloadApplianceTemplate: %v", err)
	}
	if got != upid {
		t.Fatalf("upid = %q, want %q", got, upid)
	}
	if body["storage"] != "local" || body["template"] != "debian-12-standard_12.2-1_amd64.tar.zst" {
		t.Fatalf("unexpected body: %v", body)
	}
}

// TestClient_PullOCIImage verifies the oci-registry-pull request shape,
// the returned UPID, and that an empty filename is omitted.
func TestClient_PullOCIImage(t *testing.T) {
	const upid = "UPID:pve1:0000ABCD:00000000:00000000:ociregpull:0:root@pam:"
	var bodies []map[string]any
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/nodes/pve1/storage/local/oci-registry-pull" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		bodies = append(bodies, body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":"`+upid+`"}`)
	})
	got, err := c.PullOCIImage(context.Background(), "pve1", "local", "docker.io/library/alpine:3.20", "")
	if err != nil {
		t.Fatalf("PullOCIImage: %v", err)
	}
	if got != upid {
		t.Fatalf("upid = %q, want %q", got, upid)
	}
	if bodies[0]["reference"] != "docker.io/library/alpine:3.20" {
		t.Fatalf("unexpected body: %v", bodies[0])
	}
	if _, ok := bodies[0]["filename"]; ok {
		t.Fatalf("filename should be omitted, got %v", bodies[0]["filename"])
	}
	if _, err := c.PullOCIImage(context.Background(), "pve1", "local", "docker.io/library/alpine:3.20", "alpine.img"); err != nil {
		t.Fatalf("PullOCIImage(filename): %v", err)
	}
	if bodies[1]["filename"] != "alpine.img" {
		t.Fatalf("unexpected filename: %v", bodies[1]["filename"])
	}
}

// TestClient_RunVzdumpBackup verifies the vzdump request shape: joined
// guest lists, hyphenated keys, and property-string grouped options.
func TestClient_RunVzdumpBackup(t *testing.T) {
	const upid = "UPID:pve1:0000ABCD:00000000:00000000:vzdump:100:root@pam:"
	var body map[string]any
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/nodes/pve1/vzdump" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":"`+upid+`"}`)
	})
	all := true
	bwlimit := int64(5000)
	opts := VzdumpRunOptions{
		Mode:                   "snapshot",
		Compress:               "zstd",
		Storage:                "local",
		VMIDs:                  []string{"100", "101"},
		All:                    &all,
		BWLimit:                &bwlimit,
		ExcludePath:            []string{"/var/log/*.log"},
		NotesTemplate:          "{{guestname}}",
		Fleecing:               &BackupFleecing{Enabled: BackupBoolPtr(true), Storage: "local"},
		PruneBackups:           &BackupPruneBackups{KeepLast: BackupInt64Ptr(3)},
		Performance:            &BackupPerformance{MaxWorkers: BackupInt64Ptr(2)},
		PBSChangeDetectionMode: "metadata",
	}
	got, err := c.RunVzdumpBackup(context.Background(), "pve1", opts)
	if err != nil {
		t.Fatalf("RunVzdumpBackup: %v", err)
	}
	if got != upid {
		t.Fatalf("upid = %q, want %q", got, upid)
	}
	if body["vmid"] != "100,101" {
		t.Fatalf("vmid = %v, want joined list", body["vmid"])
	}
	if body["mode"] != "snapshot" || body["compress"] != "zstd" || body["storage"] != "local" {
		t.Fatalf("unexpected scalar params: %v", body)
	}
	if body["bwlimit"] != float64(5000) {
		t.Fatalf("bwlimit = %v", body["bwlimit"])
	}
	if body["all"] != true {
		t.Fatalf("all = %v", body["all"])
	}
	excludePath, ok := body["exclude-path"].([]any)
	if !ok || len(excludePath) != 1 || excludePath[0] != "/var/log/*.log" {
		t.Fatalf("exclude-path = %v", body["exclude-path"])
	}
	if body["fleecing"] != "enabled=1,storage=local" {
		t.Fatalf("fleecing = %v", body["fleecing"])
	}
	if body["prune-backups"] != "keep-last=3" {
		t.Fatalf("prune-backups = %v", body["prune-backups"])
	}
	if body["performance"] != "max-workers=2" {
		t.Fatalf("performance = %v", body["performance"])
	}
	if body["pbs-change-detection-mode"] != "metadata" {
		t.Fatalf("pbs-change-detection-mode = %v", body["pbs-change-detection-mode"])
	}
	if body["notes-template"] != "{{guestname}}" {
		t.Fatalf("notes-template = %v", body["notes-template"])
	}
}
