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

// qvmStrPtr is a test helper for building *string config values.
func qvmStrPtr(s string) *string { return &s }

// TestClient_CreateQemuVM_WireShape pins the POST /nodes/{node}/qemu body:
// vmid plus the rendered config keys, returning the task UPID.
func TestClient_CreateQemuVM_WireShape(t *testing.T) {
	var body map[string]any
	var gotPath string
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		gotPath = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatalf("create body not JSON: %v", err)
		}
		_, _ = io.WriteString(w, `{"data":"UPID:pve1:0001:0001:qmcreate:root@pam:"}`)
	})

	onboot := true
	upid, err := c.CreateQemuVM(context.Background(), "pve1", 100, QemuVMConfigInput{
		Name:      qvmStrPtr("web01"),
		Onboot:    &onboot,
		Cores:     int64Ptr(2),
		MemoryMiB: int64Ptr(2048),
		CPUType:   qvmStrPtr("host"),
		BootOrder: []string{"scsi0", "net0"},
		Tags:      []string{"prod", "web"},
		Drives:    map[string]string{"scsi0": "local-lvm:32,iothread=1"},
		Networks:  map[string]string{"net0": "virtio,bridge=vmbr0,firewall=1"},
	})
	if err != nil {
		t.Fatalf("CreateQemuVM: %v", err)
	}
	if !strings.HasPrefix(upid, "UPID:pve1:") {
		t.Fatalf("upid = %q", upid)
	}
	if gotPath != "/nodes/pve1/qemu" {
		t.Fatalf("path = %q", gotPath)
	}
	if body["vmid"] != float64(100) {
		t.Fatalf("vmid = %v (%T)", body["vmid"], body["vmid"])
	}
	if body["name"] != "web01" || body["cores"] != float64(2) || body["memory"] != float64(2048) {
		t.Fatalf("scalar keys = %v", body)
	}
	if body["onboot"] != true {
		t.Fatalf("onboot = %v (%T)", body["onboot"], body["onboot"])
	}
	if body["cpu"] != "cputype=host" {
		t.Fatalf("cpu = %v", body["cpu"])
	}
	if body["boot"] != "order=scsi0;net0" {
		t.Fatalf("boot = %v", body["boot"])
	}
	if body["tags"] != "prod;web" {
		t.Fatalf("tags = %v", body["tags"])
	}
	if body["scsi0"] != "local-lvm:32,iothread=1" || body["net0"] != "virtio,bridge=vmbr0,firewall=1" {
		t.Fatalf("compound keys = %v", body)
	}
}

// TestClient_CloneQemuVM_WireShape pins POST .../clone: newid plus the
// pin's clone parameters.
func TestClient_CloneQemuVM_WireShape(t *testing.T) {
	var body map[string]any
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost || r.URL.Path != "/nodes/pve1/qemu/42/clone" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatalf("clone body not JSON: %v", err)
		}
		_, _ = io.WriteString(w, `{"data":"UPID:pve1:0001:0001:qmclone:root@pam:"}`)
	})

	full := false
	bw := int64(5000)
	upid, err := c.CloneQemuVM(context.Background(), "pve1", 42, 101, QemuVMCloneOptions{
		Name:      "clone01",
		Full:      &full,
		Storage:   "local-lvm",
		Format:    "qcow2",
		Pool:      "ops",
		Bandwidth: &bw,
	})
	if err != nil {
		t.Fatalf("CloneQemuVM: %v", err)
	}
	if !strings.HasPrefix(upid, "UPID:pve1:") {
		t.Fatalf("upid = %q", upid)
	}
	if body["newid"] != float64(101) || body["name"] != "clone01" || body["storage"] != "local-lvm" {
		t.Fatalf("clone body = %v", body)
	}
	if body["full"] != false {
		t.Fatalf("full = %v (%T)", body["full"], body["full"])
	}
	if body["format"] != "qcow2" || body["pool"] != "ops" || body["bwlimit"] != float64(5000) {
		t.Fatalf("clone body = %v", body)
	}
}

// TestClient_GetQemuVMConfig_Decodes covers GET .../config returning the
// raw key map.
func TestClient_GetQemuVMConfig_Decodes(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodGet || r.URL.Path != "/nodes/pve1/qemu/100/config" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"data":{"name":"web01","cores":2,"memory":2048,`+
			`"scsi0":"local-lvm:vm-100-disk-0,size=32G","net0":"virtio=BC:24:11:2F:4E:8D,bridge=vmbr0","digest":"abc123"}}`)
	})

	config, err := c.GetQemuVMConfig(context.Background(), "pve1", 100)
	if err != nil {
		t.Fatalf("GetQemuVMConfig: %v", err)
	}
	if string(config["name"]) != `"web01"` {
		t.Fatalf("name = %s", config["name"])
	}
	if string(config["scsi0"]) != `"local-lvm:vm-100-disk-0,size=32G"` {
		t.Fatalf("scsi0 = %s", config["scsi0"])
	}
	if _, ok := config["digest"]; !ok {
		t.Fatalf("digest key missing: %v", config)
	}
}

// TestClient_GetQemuVMStatusCurrent_Decodes covers the full and minimal
// status reads, including 0/1 encodings of template.
func TestClient_GetQemuVMStatusCurrent_Decodes(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodGet || r.URL.Path != "/nodes/pve1/qemu/100/status/current" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"data":{"status":"running","qmpstatus":"running",`+
			`"name":"web01","tags":"prod;web","template":0,"agent":true,"uptime":3600,`+
			`"maxmem":2147483648,"maxdisk":34359738368,"cpus":2,"pid":1234}}`)
	})

	status, err := c.GetQemuVMStatusCurrent(context.Background(), "pve1", 100)
	if err != nil {
		t.Fatalf("GetQemuVMStatusCurrent: %v", err)
	}
	if status.Status != "running" || status.QMPStatus != "running" || status.Name != "web01" {
		t.Fatalf("status = %+v", status)
	}
	if status.Template {
		t.Fatalf("template must decode 0 as false: %+v", status)
	}
	if !status.Agent {
		t.Fatalf("agent must decode true: %+v", status)
	}
	if status.Uptime != 3600 || status.MaxMem != 2147483648 || status.MaxCPU != 2 {
		t.Fatalf("status = %+v", status)
	}

	minimal, err := c.GetQemuVMStatusCurrentMinimal(context.Background(), "pve1", 100)
	if err != nil {
		t.Fatalf("GetQemuVMStatusCurrentMinimal: %v", err)
	}
	if minimal.Status != "running" || minimal.Template {
		t.Fatalf("minimal = %+v", minimal)
	}
}

// TestClient_UpdateQemuVMConfig_WireShape pins the PUT .../config body with
// the delete list.
func TestClient_UpdateQemuVMConfig_WireShape(t *testing.T) {
	var body map[string]any
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPut || r.URL.Path != "/nodes/pve1/qemu/100/config" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatalf("update body not JSON: %v", err)
		}
		_, _ = io.WriteString(w, `{"data":null}`)
	})

	err := c.UpdateQemuVMConfig(context.Background(), "pve1", 100, QemuVMConfigInput{
		Name:     qvmStrPtr("renamed"),
		Networks: map[string]string{"net0": "virtio,bridge=vmbr0"},
	}, []string{"ide2", "ipconfig0"})
	if err != nil {
		t.Fatalf("UpdateQemuVMConfig: %v", err)
	}
	if body["name"] != "renamed" || body["net0"] != "virtio,bridge=vmbr0" {
		t.Fatalf("update body = %v", body)
	}
	if body["delete"] != "ide2,ipconfig0" {
		t.Fatalf("delete = %v", body["delete"])
	}
}

// TestClient_DeleteQemuVM_Query pins the DELETE .../{vmid} query params.
func TestClient_DeleteQemuVM_Query(t *testing.T) {
	var gotQuery string
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodDelete || r.URL.Path != "/nodes/pve1/qemu/100" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		gotQuery = r.URL.RawQuery
		_, _ = io.WriteString(w, `{"data":"UPID:pve1:0001:0001:qmdestroy:root@pam:"}`)
	})

	purge := true
	upid, err := c.DeleteQemuVM(context.Background(), "pve1", 100, QemuVMDeleteOptions{Purge: &purge})
	if err != nil {
		t.Fatalf("DeleteQemuVM: %v", err)
	}
	if !strings.HasPrefix(upid, "UPID:pve1:") {
		t.Fatalf("upid = %q", upid)
	}
	if !strings.Contains(gotQuery, "purge=1") {
		t.Fatalf("query = %q", gotQuery)
	}
	if strings.Contains(gotQuery, "destroy-unreferenced-disks") {
		t.Fatalf("nil option must be omitted, query = %q", gotQuery)
	}
}

// TestClient_QemuVMPowerActions_WireShape covers start, stop, and shutdown
// posts with their timeout bodies.
func TestClient_QemuVMPowerActions_WireShape(t *testing.T) {
	var body map[string]any
	var path string
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		body = nil
		_ = json.Unmarshal(raw, &body)
		_, _ = io.WriteString(w, `{"data":"UPID:pve1:0001:0001:qmstart:root@pam:"}`)
	})

	timeout := int64(90)
	force := true
	if _, err := c.QemuVMStart(context.Background(), "pve1", 100, &timeout); err != nil {
		t.Fatalf("QemuVMStart: %v", err)
	}
	if path != "/nodes/pve1/qemu/100/status/start" || body["timeout"] != float64(90) {
		t.Fatalf("start path=%q body=%v", path, body)
	}
	if _, err := c.QemuVMStop(context.Background(), "pve1", 100, nil); err != nil {
		t.Fatalf("QemuVMStop: %v", err)
	}
	if path != "/nodes/pve1/qemu/100/status/stop" {
		t.Fatalf("stop path = %q", path)
	}
	if _, ok := body["timeout"]; ok {
		t.Fatalf("nil timeout must be omitted, body = %v", body)
	}
	if _, err := c.QemuVMShutdown(context.Background(), "pve1", 100, &timeout, &force); err != nil {
		t.Fatalf("QemuVMShutdown: %v", err)
	}
	if path != "/nodes/pve1/qemu/100/status/shutdown" || body["forceStop"] != true {
		t.Fatalf("shutdown path=%q body=%v", path, body)
	}
}

// TestClient_MigrateQemuVM_WireShape pins the migrate body.
func TestClient_MigrateQemuVM_WireShape(t *testing.T) {
	var body map[string]any
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost || r.URL.Path != "/nodes/pve1/qemu/100/migrate" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatalf("migrate body not JSON: %v", err)
		}
		_, _ = io.WriteString(w, `{"data":"UPID:pve1:0001:0001:qmmigrate:root@pam:"}`)
	})

	online := true
	withLocal := true
	upid, err := c.MigrateQemuVM(context.Background(), "pve1", 100, QemuVMMigrateOptions{
		Target:         "pve2",
		Online:         &online,
		WithLocalDisks: &withLocal,
		TargetStorage:  "1",
	})
	if err != nil {
		t.Fatalf("MigrateQemuVM: %v", err)
	}
	if !strings.HasPrefix(upid, "UPID:pve1:") {
		t.Fatalf("upid = %q", upid)
	}
	if body["target"] != "pve2" || body["online"] != true || body["with-local-disks"] != true {
		t.Fatalf("migrate body = %v", body)
	}
	if body["targetstorage"] != "1" {
		t.Fatalf("targetstorage = %v", body["targetstorage"])
	}
}

// TestClient_ResizeAndMoveQemuDisk_WireShape pins the resize PUT and the
// move_disk POST bodies.
func TestClient_ResizeAndMoveQemuDisk_WireShape(t *testing.T) {
	var body map[string]any
	var path string
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		_, _ = io.WriteString(w, `{"data":"UPID:pve1:0001:0001:qmresize:root@pam:"}`)
	})

	upid, err := c.ResizeQemuDisk(context.Background(), "pve1", 100, "scsi0", "64G", "abc123")
	if err != nil {
		t.Fatalf("ResizeQemuDisk: %v", err)
	}
	if !strings.HasPrefix(upid, "UPID:pve1:") {
		t.Fatalf("resize upid = %q", upid)
	}
	if path != "/nodes/pve1/qemu/100/resize" || body["disk"] != "scsi0" || body["size"] != "64G" {
		t.Fatalf("resize path=%q body=%v", path, body)
	}
	if body["digest"] != "abc123" {
		t.Fatalf("resize digest = %v", body["digest"])
	}

	deleteOrig := true
	if _, err := c.MoveQemuDisk(context.Background(), "pve1", 100, QemuVMMoveDiskOptions{
		Disk: "scsi0", Storage: "bigstore", DeleteOriginal: &deleteOrig,
	}); err != nil {
		t.Fatalf("MoveQemuDisk: %v", err)
	}
	if path != "/nodes/pve1/qemu/100/move_disk" || body["disk"] != "scsi0" || body["storage"] != "bigstore" {
		t.Fatalf("move path=%q body=%v", path, body)
	}
	if body["delete"] != true {
		t.Fatalf("move delete = %v (%T)", body["delete"], body["delete"])
	}
}

// TestClient_GetQemuVMPending_Decodes covers pending rows carrying numeric
// values, pending replacements, and delete markers.
func TestClient_GetQemuVMPending_Decodes(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodGet || r.URL.Path != "/nodes/pve1/qemu/100/pending" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"data":[{"key":"name","value":"web01"},`+
			`{"key":"memory","value":2048,"pending":4096},`+
			`{"key":"net0","delete":1}]}`)
	})

	changes, err := c.GetQemuVMPending(context.Background(), "pve1", 100)
	if err != nil {
		t.Fatalf("GetQemuVMPending: %v", err)
	}
	if len(changes) != 3 {
		t.Fatalf("changes = %+v", changes)
	}
	if changes[0].Key != "name" || changes[0].Value != "web01" || changes[0].Pending != nil {
		t.Fatalf("row 0 = %+v", changes[0])
	}
	if changes[1].Value != "2048" {
		t.Fatalf("row 1 value = %q", changes[1].Value)
	}
	if changes[1].Pending == nil || *changes[1].Pending != "4096" {
		t.Fatalf("row 1 pending = %+v", changes[1].Pending)
	}
	if changes[2].Delete == nil || *changes[2].Delete != 1 {
		t.Fatalf("row 2 delete = %+v", changes[2].Delete)
	}
}

// TestClient_QemuVMSSHKeysEncoded pins the urlencoded wire format the pin
// mandates for the sshkeys cloud-init key.
func TestClient_QemuVMSSHKeysEncoded(t *testing.T) {
	var body map[string]any
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		_, _ = io.WriteString(w, `{"data":null}`)
	})

	err := c.UpdateQemuVMConfig(context.Background(), "pve1", 100, QemuVMConfigInput{
		CISSHKeys: qvmStrPtr("ssh-ed25519 AAA key1\nssh-ed25519 BBB key2"),
	}, nil)
	if err != nil {
		t.Fatalf("UpdateQemuVMConfig: %v", err)
	}
	got, _ := body["sshkeys"].(string)
	if strings.ContainsAny(got, "\n ") {
		t.Fatalf("sshkeys must be percent-encoded, got %q", got)
	}
	if got != "ssh-ed25519%20AAA%20key1%0Assh-ed25519%20BBB%20key2" {
		t.Fatalf("sshkeys = %q", got)
	}
}
