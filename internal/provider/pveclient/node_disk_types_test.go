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
	"time"
)

// TestClient_ListLVMThinpools_Decodes decodes the thinpool listing.
func TestClient_ListLVMThinpools_Decodes(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nodes/pve1/disks/lvmthin" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"lv":"data","lv_size":1000000000000,"metadata_size":1048576,"metadata_used":307200,"used":123456789,"vg":"pve"}]}`)
	})
	thin, err := c.ListLVMThinpools(context.Background(), "pve1")
	if err != nil {
		t.Fatalf("ListLVMThinpools: %v", err)
	}
	if len(thin) != 1 {
		t.Fatalf("thinpools = %d, want 1", len(thin))
	}
	tp := thin[0]
	if tp.LV != "data" || tp.VG != "pve" || tp.LVSize != 1000000000000 || tp.Used != 123456789 {
		t.Fatalf("unexpected thinpool: %+v", tp)
	}
}

// TestClient_CreateLVMThinpool_BodyShape verifies the create payload carries
// the singular device string PVE expects and omits add_storage when false.
func TestClient_CreateLVMThinpool_BodyShape(t *testing.T) {
	var captured map[string]any
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/nodes/pve1/disks/lvmthin" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&captured)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":"UPID:pve1:00001234:12345678:LVMTHINCREATE:operator:root@pam:"}`)
	})
	upid, err := c.CreateLVMThinpool(context.Background(), "pve1", CreateLVMThinpoolInput{
		Name:       "data",
		Device:     "/dev/sdb",
		AddStorage: true,
	})
	if err != nil {
		t.Fatalf("CreateLVMThinpool: %v", err)
	}
	if !strings.HasPrefix(upid, "UPID:") {
		t.Fatalf("upid = %q", upid)
	}
	if captured["name"] != "data" {
		t.Fatalf("name = %v", captured["name"])
	}
	if captured["device"] != "/dev/sdb" {
		t.Fatalf("device = %v, want the singular device string", captured["device"])
	}
	if captured["add_storage"] != true {
		t.Fatalf("add_storage = %v", captured["add_storage"])
	}
}

// TestClient_CreateLVMThinpool_WaitsForTask walks the full create flow: the
// POST returns an UPID and WaitForTask polls the task status until stopped.
func TestClient_CreateLVMThinpool_WaitsForTask(t *testing.T) {
	upid := "UPID:pve1:00001234:12345678:LVMTHINCREATE:operator:root@pam:"
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost && r.URL.Path == "/nodes/pve1/disks/lvmthin" {
			_, _ = io.WriteString(w, `{"data":"`+upid+`"}`)
			return
		}
		if r.URL.Path == "/nodes/pve1/tasks/"+upid+"/status" {
			_, _ = io.WriteString(w, `{"data":{"status":"stopped","exitstatus":"OK","pid":42}}`)
			return
		}
		t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
	})
	created, err := c.CreateLVMThinpool(context.Background(), "pve1", CreateLVMThinpoolInput{Name: "data", Device: "/dev/sdb"})
	if err != nil {
		t.Fatalf("CreateLVMThinpool: %v", err)
	}
	info, err := c.WaitForTask(context.Background(), "pve1", created, WaitForTaskOptions{Interval: 100 * time.Millisecond, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("WaitForTask: %v", err)
	}
	if info == nil || info.Status != TaskStopped || info.ExitStatus != "OK" {
		t.Fatalf("TaskInfo = %+v, want stopped/OK", info)
	}
}

// TestClient_DeleteLVMThinpool_QueryFlags confirms cleanup flags land on the
// DELETE URL as 0/1 integers.
func TestClient_DeleteLVMThinpool_QueryFlags(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/nodes/pve1/disks/lvmthin/data" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if !strings.Contains(r.URL.RawQuery, "cleanup-config=1") || !strings.Contains(r.URL.RawQuery, "cleanup-disks=0") {
			t.Fatalf("missing cleanup flags: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":"UPID:pve1:00001234:12345678:LVMTHINDESTROY:operator:root@pam:"}`)
	})
	upid, err := c.DeleteLVMThinpool(context.Background(), "pve1", "data", true, false)
	if err != nil {
		t.Fatalf("DeleteLVMThinpool: %v", err)
	}
	if !strings.HasPrefix(upid, "UPID:") {
		t.Fatalf("upid = %q", upid)
	}
}

// TestClient_ListNodeDirectories_Decodes decodes the directory listing.
func TestClient_ListNodeDirectories_Decodes(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nodes/pve1/disks/directory" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"device":"/dev/sdb1","options":"rw,relatime","path":"/mnt/pve/backup","type":"ext4","unitfile":"mnt-pve-backup.mount"}]}`)
	})
	dirs, err := c.ListNodeDirectories(context.Background(), "pve1")
	if err != nil {
		t.Fatalf("ListNodeDirectories: %v", err)
	}
	if len(dirs) != 1 {
		t.Fatalf("directories = %d, want 1", len(dirs))
	}
	d := dirs[0]
	if d.Device != "/dev/sdb1" || d.Path != "/mnt/pve/backup" || d.Type != "ext4" || d.UnitFile != "mnt-pve-backup.mount" {
		t.Fatalf("unexpected directory: %+v", d)
	}
}

// TestClient_CreateNodeDirectory_BodyShape verifies the directory create
// payload, including filesystem only when set.
func TestClient_CreateNodeDirectory_BodyShape(t *testing.T) {
	var captured map[string]any
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/nodes/pve1/disks/directory" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&captured)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":"UPID:pve1:00001234:12345678:DIRCREATE:operator:root@pam:"}`)
	})
	if _, err := c.CreateNodeDirectory(context.Background(), "pve1", CreateNodeDirectoryInput{
		Name:       "backup",
		Device:     "/dev/sdb",
		Filesystem: "xfs",
		AddStorage: true,
	}); err != nil {
		t.Fatalf("CreateNodeDirectory: %v", err)
	}
	if captured["name"] != "backup" || captured["device"] != "/dev/sdb" || captured["filesystem"] != "xfs" {
		t.Fatalf("unexpected payload: %+v", captured)
	}
	if captured["add_storage"] != true {
		t.Fatalf("add_storage = %v", captured["add_storage"])
	}
}

// TestClient_CreateNodeDirectory_DefaultsOmitted verifies that an unset
// filesystem is omitted so PVE applies its own default (ext4).
func TestClient_CreateNodeDirectory_DefaultsOmitted(t *testing.T) {
	var captured map[string]any
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&captured)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":"UPID:pve1:00001234:12345678:DIRCREATE:operator:root@pam:"}`)
	})
	if _, err := c.CreateNodeDirectory(context.Background(), "pve1", CreateNodeDirectoryInput{Name: "backup", Device: "/dev/sdb"}); err != nil {
		t.Fatalf("CreateNodeDirectory: %v", err)
	}
	if _, ok := captured["filesystem"]; ok {
		t.Fatalf("filesystem should be omitted when empty: %+v", captured)
	}
	if _, ok := captured["add_storage"]; ok {
		t.Fatalf("add_storage should be omitted when false: %+v", captured)
	}
}

// TestClient_DeleteNodeDirectory_QueryFlags confirms cleanup flags land on
// the DELETE URL.
func TestClient_DeleteNodeDirectory_QueryFlags(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/nodes/pve1/disks/directory/backup" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if !strings.Contains(r.URL.RawQuery, "cleanup-config=0") || !strings.Contains(r.URL.RawQuery, "cleanup-disks=1") {
			t.Fatalf("missing cleanup flags: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":"UPID:pve1:00001234:12345678:DIRDESTROY:operator:root@pam:"}`)
	})
	if _, err := c.DeleteNodeDirectory(context.Background(), "pve1", "backup", false, true); err != nil {
		t.Fatalf("DeleteNodeDirectory: %v", err)
	}
}
