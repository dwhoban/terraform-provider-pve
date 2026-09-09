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

// TestClient_ListNodeDisks_Decodes decodes the disk list response.
func TestClient_ListNodeDisks_Decodes(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nodes/pve1/disks/list" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"devpath":"/dev/sda","size":1099511627776,"used":"root","gpt":1,"mounted":0,"vendor":"ATA","model":"Samsung SSD 860","serial":"S123","wwn":"0x50014ee2bef00001","health":"PASSED","type":"ssd"}]}`)
	})
	disks, err := c.ListNodeDisks(context.Background(), "pve1")
	if err != nil {
		t.Fatalf("ListNodeDisks: %v", err)
	}
	if len(disks) != 1 {
		t.Fatalf("disks = %d, want 1", len(disks))
	}
	d := disks[0]
	if d.DevPath != "/dev/sda" || d.Size != 1099511627776 || !d.GPT || d.Health != "PASSED" || d.Vendor != "ATA" {
		t.Fatalf("unexpected disk: %+v", d)
	}
}

// TestClient_CreateZFSPool_BodyShape verifies the request body carries the
// fields the resource will fill in (name, raidlevel, comma-joined devices).
func TestClient_CreateZFSPool_BodyShape(t *testing.T) {
	var captured map[string]any
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/nodes/pve1/disks/zfs" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&captured)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":"UPID:pve1:00001234:12345678:CREATEZFS:operator:root@pam:"}`)
	})
	upid, err := c.CreateZFSPool(context.Background(), "pve1", CreateZFSPoolInput{
		Name:        "tank",
		RaidLevel:   "mirror",
		Devices:     []string{"/dev/sdb", "/dev/sdc"},
		Ashift:      12,
		Compression: "lz4",
		AddStorage:  true,
	})
	if err != nil {
		t.Fatalf("CreateZFSPool: %v", err)
	}
	if upid == "" {
		t.Fatal("empty upid")
	}
	if captured["name"] != "tank" {
		t.Fatalf("name = %v", captured["name"])
	}
	if captured["raidlevel"] != "mirror" {
		t.Fatalf("raidlevel = %v", captured["raidlevel"])
	}
	if captured["devices"] != "/dev/sdb,/dev/sdc" {
		t.Fatalf("devices = %v", captured["devices"])
	}
	if captured["ashift"] != float64(12) {
		t.Fatalf("ashift = %v", captured["ashift"])
	}
	if captured["compression"] != "lz4" {
		t.Fatalf("compression = %v", captured["compression"])
	}
	if captured["add_storage"] != true {
		t.Fatalf("add_storage = %v", captured["add_storage"])
	}
}

// TestClient_DeleteZFSPool_ReturnsUpid covers the symmetric delete path.
func TestClient_DeleteZFSPool_ReturnsUpid(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/nodes/pve1/disks/zfs/tank" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":"UPID:pve1:00001234:12345678:DESTROY:operator:root@pam:"}`)
	})
	upid, err := c.DeleteZFSPool(context.Background(), "pve1", "tank")
	if err != nil {
		t.Fatalf("DeleteZFSPool: %v", err)
	}
	if !strings.HasPrefix(upid, "UPID:") {
		t.Fatalf("upid = %q", upid)
	}
}

// TestClient_CreateLVMVG_BodyShape verifies the LVM create payload.
func TestClient_CreateLVMVG_BodyShape(t *testing.T) {
	var captured map[string]any
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nodes/pve1/disks/lvm" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&captured)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":"UPID:pve1:00001234:12345678:LVMCREATE:operator:root@pam:"}`)
	})
	if _, err := c.CreateLVMVG(context.Background(), "pve1", CreateLVMVGInput{
		Name:       "data",
		Devices:    []string{"/dev/sdd"},
		AddStorage: false,
	}); err != nil {
		t.Fatalf("CreateLVMVG: %v", err)
	}
	if captured["name"] != "data" || captured["devices"] != "/dev/sdd" {
		t.Fatalf("unexpected payload: %+v", captured)
	}
	if _, ok := captured["add_storage"]; ok {
		t.Fatalf("add_storage should be omitted when false: %+v", captured)
	}
}

// TestClient_DeleteLVMVG_QueryFlags confirms cleanup flags land on the URL.
func TestClient_DeleteLVMVG_QueryFlags(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Fatalf("expected DELETE, got %s", r.Method)
		}
		if !strings.Contains(r.URL.RawQuery, "cleanup-config=1") {
			t.Fatalf("missing cleanup-config=1: %s", r.URL.RawQuery)
		}
		if !strings.Contains(r.URL.RawQuery, "cleanup-disks=1") {
			t.Fatalf("missing cleanup-disks=1: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":"UPID:del"}`)
	})
	if _, err := c.DeleteLVMVG(context.Background(), "pve1", "data", true, true); err != nil {
		t.Fatalf("DeleteLVMVG: %v", err)
	}
}
