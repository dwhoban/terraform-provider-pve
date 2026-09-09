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

// TestCreateLxcContainerWireBody asserts POST /nodes/{node}/lxc carries the
// create parameter set, including the hyphenated ssh-public-keys key and the
// rendered rootfs/mpN mount point strings.
func TestCreateLxcContainerWireBody(t *testing.T) {
	var captured map[string]any
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/nodes/pve1/lxc" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&captured)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":"UPID:pve1:00000001:00000001:vzcreate:root@pam:create:"}`)
	})
	onboot := true
	unprivileged := true
	start := true
	upid, err := c.CreateLxcContainer(context.Background(), "pve1", CreateLxcContainerParams{
		VMID:          100,
		OSTemplate:    "local:vztmpl/debian-12-standard_12.7-1_amd64.tar.zst",
		Hostname:      "ct1",
		Password:      "secret",
		SSHPublicKeys: "ssh-ed25519 AAAA test",
		Onboot:        &onboot,
		Unprivileged:  &unprivileged,
		Memory:        int64Ptr(512),
		Start:         &start,
		Rootfs:        "local-lvm:8",
		MountPoints:   map[string]string{"mp0": "local-lvm:4,mp=/data"},
	})
	if err != nil {
		t.Fatalf("CreateLxcContainer: %v", err)
	}
	if upid == "" {
		t.Fatal("expected a task UPID")
	}
	v, ok := captured["vmid"].(float64)
	if !ok || v != 100 {
		t.Fatalf("vmid = %v", captured["vmid"])
	}
	if captured["ostemplate"] != "local:vztmpl/debian-12-standard_12.7-1_amd64.tar.zst" {
		t.Fatalf("ostemplate = %v", captured["ostemplate"])
	}
	if captured["ssh-public-keys"] != "ssh-ed25519 AAAA test" {
		t.Fatalf("ssh-public-keys = %v", captured["ssh-public-keys"])
	}
	if captured["rootfs"] != "local-lvm:8" {
		t.Fatalf("rootfs = %v", captured["rootfs"])
	}
	if captured["mp0"] != "local-lvm:4,mp=/data" {
		t.Fatalf("mp0 = %v", captured["mp0"])
	}
	mem, ok := captured["memory"].(float64)
	if !ok || mem != 512 {
		t.Fatalf("memory = %v", captured["memory"])
	}
	if captured["start"] != true {
		t.Fatalf("start = %v", captured["start"])
	}
}

// TestCloneLxcContainerWireBody asserts POST .../clone sends the newid and
// clone options and returns the task UPID.
func TestCloneLxcContainerWireBody(t *testing.T) {
	var captured map[string]any
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/nodes/pve1/lxc/100/clone" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&captured)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":"UPID:pve1:00000001:00000001:vzclone:root@pam:clone:"}`)
	})
	full := true
	_, err := c.CloneLxcContainer(context.Background(), "pve1", 100, CloneLxcContainerParams{
		NewID:    101,
		Hostname: "ct2",
		Full:     &full,
		Storage:  "local-lvm",
		Pool:     "ops",
	})
	if err != nil {
		t.Fatalf("CloneLxcContainer: %v", err)
	}
	newID, ok := captured["newid"].(float64)
	if !ok || newID != 101 {
		t.Fatalf("newid = %v", captured["newid"])
	}
	if captured["hostname"] != "ct2" || captured["full"] != true || captured["storage"] != "local-lvm" || captured["pool"] != "ops" {
		t.Fatalf("clone body = %v", captured)
	}
}

// TestGetLxcConfig asserts the config read decodes PVE's mixed value
// encodings (quoted numbers, 0/1 booleans) and parses rootfs/mpN strings.
func TestGetLxcConfig(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/nodes/pve1/lxc/100/config" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{
			"hostname":"ct1","description":"d","tags":"web;prod",
			"cores":"2","memory":"512","swap":"0",
			"onboot":1,"unprivileged":"1","protection":0,"template":"0",
			"nameserver":"1.1.1.1","searchdomain":"example.com",
			"rootfs":"local-lvm:vm-100-disk-0,size=8G",
			"mp0":"local-lvm:vm-100-disk-1,mp=/data,size=4G,acl=1,backup=0",
			"mp1":"/mnt/bind,mp=/bind,ro=1,quota=1",
			"digest":"cafe"
		}}`)
	})
	cfg, err := c.GetLxcConfig(context.Background(), "pve1", 100)
	if err != nil {
		t.Fatalf("GetLxcConfig: %v", err)
	}
	if cfg.Hostname != "ct1" || cfg.Description != "d" || cfg.Tags != "web;prod" {
		t.Fatalf("config scalars = %+v", cfg)
	}
	if cfg.Cores == nil || *cfg.Cores != 2 || cfg.Memory == nil || *cfg.Memory != 512 {
		t.Fatalf("config ints = %+v", cfg)
	}
	if cfg.Onboot == nil || !*cfg.Onboot || cfg.Unprivileged == nil || !*cfg.Unprivileged {
		t.Fatalf("config bools = %+v", cfg)
	}
	if cfg.Protection == nil || *cfg.Protection || cfg.Template == nil || *cfg.Template {
		t.Fatalf("config false bools = %+v", cfg)
	}
	if cfg.Nameserver != "1.1.1.1" || cfg.Searchdomain != "example.com" {
		t.Fatalf("config dns = %+v", cfg)
	}
	if cfg.Digest != "cafe" {
		t.Fatalf("digest = %q", cfg.Digest)
	}
	if cfg.Rootfs == nil {
		t.Fatal("rootfs missing")
	}
	if cfg.Rootfs.Storage() != "local-lvm" || cfg.Rootfs.VolumeID() != "vm-100-disk-0" || cfg.Rootfs.Size != "8G" {
		t.Fatalf("rootfs = %+v", cfg.Rootfs)
	}
	mp0, ok := cfg.MountPoints["mp0"]
	if !ok {
		t.Fatal("mp0 missing")
	}
	if mp0.Mountpoint != "/data" || mp0.Size != "4G" || mp0.ACL == nil || !*mp0.ACL || mp0.Backup == nil || *mp0.Backup {
		t.Fatalf("mp0 = %+v", mp0)
	}
	mp1 := cfg.MountPoints["mp1"]
	if mp1.Volume != "/mnt/bind" || mp1.Mountpoint != "/bind" || mp1.ReadOnly == nil || !*mp1.ReadOnly {
		t.Fatalf("mp1 = %+v", mp1)
	}
	if mp1.Extra["quota"] != "1" {
		t.Fatalf("mp1 extra options lost: %+v", mp1.Extra)
	}
}

// TestLxcMountPointRender asserts the volume-first rendering used for
// create and update PUTs.
func TestLxcMountPointRender(t *testing.T) {
	ro := true
	mp := LxcMountPoint{Volume: "local-lvm:4", Mountpoint: "/data", ReadOnly: &ro}
	if got := mp.Render(); got != "local-lvm:4,mp=/data,ro=1" {
		t.Fatalf("render = %q", got)
	}
	mp = LxcMountPoint{Volume: "/mnt/bind", Mountpoint: "/bind"}
	if got := mp.Render(); got != "/mnt/bind,mp=/bind" {
		t.Fatalf("bind render = %q", got)
	}
}

// TestGetLxcStatus asserts the status read decodes boolish template values.
func TestGetLxcStatus(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/nodes/pve1/lxc/100/status/current" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"name":"ct1","status":"running","template":0,"uptime":42,"lock":"backup"}}`)
	})
	st, err := c.GetLxcStatus(context.Background(), "pve1", 100)
	if err != nil {
		t.Fatalf("GetLxcStatus: %v", err)
	}
	if st.Status != "running" || st.Name != "ct1" || st.Lock != "backup" {
		t.Fatalf("status = %+v", st)
	}
	if st.Template == nil || *st.Template {
		t.Fatalf("template = %v", st.Template)
	}
	if st.Uptime == nil || *st.Uptime != 42 {
		t.Fatalf("uptime = %v", st.Uptime)
	}
}

// TestUpdateLxcConfigWireBody asserts PUT .../config carries changed keys
// and the delete list, and that create-only keys are never sent.
func TestUpdateLxcConfigWireBody(t *testing.T) {
	var captured map[string]any
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/nodes/pve1/lxc/100/config" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&captured)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":null}`)
	})
	onboot := false
	err := c.UpdateLxcConfig(context.Background(), "pve1", 100, UpdateLxcConfigParams{
		Hostname:    "renamed",
		Onboot:      &onboot,
		Memory:      int64Ptr(1024),
		Delete:      "mp1,mp2",
		MountPoints: map[string]string{"mp0": "local-lvm:vm-100-disk-1,mp=/data,size=8G"},
	})
	if err != nil {
		t.Fatalf("UpdateLxcConfig: %v", err)
	}
	mem, memOK := captured["memory"].(float64)
	if captured["hostname"] != "renamed" || captured["onboot"] != false || !memOK || mem != 1024 {
		t.Fatalf("update body = %v", captured)
	}
	if captured["delete"] != "mp1,mp2" {
		t.Fatalf("delete = %v", captured["delete"])
	}
	if captured["mp0"] != "local-lvm:vm-100-disk-1,mp=/data,size=8G" {
		t.Fatalf("mp0 = %v", captured["mp0"])
	}
	for _, forbidden := range []string{"ostemplate", "password", "ssh-public-keys", "start", "vmid"} {
		if _, present := captured[forbidden]; present {
			t.Fatalf("update body sent create-only key %q", forbidden)
		}
	}
}

// TestDeleteLxcContainerWireBody asserts DELETE .../{vmid} forwards the
// destroy options and returns the task UPID.
func TestDeleteLxcContainerWireBody(t *testing.T) {
	var captured map[string]any
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/nodes/pve1/lxc/100" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&captured)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":"UPID:pve1:00000001:00000001:vzdestroy:root@pam:destroy:"}`)
	})
	purge := true
	_, err := c.DeleteLxcContainer(context.Background(), "pve1", 100, DeleteLxcContainerParams{
		Purge:                    &purge,
		DestroyUnreferencedDisks: &purge,
	})
	if err != nil {
		t.Fatalf("DeleteLxcContainer: %v", err)
	}
	if captured["purge"] != true || captured["destroy-unreferenced-disks"] != true {
		t.Fatalf("delete body = %v", captured)
	}
}

// TestLxcPowerAndShutdownWireBodies asserts start/stop/shutdown verbs.
func TestLxcPowerAndShutdownWireBodies(t *testing.T) {
	var bodies []map[string]any
	var paths []string
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		paths = append(paths, r.URL.Path)
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		bodies = append(bodies, body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":"UPID:pve1:1:1:vz:root@pam:x:"}`)
	})
	ctx := context.Background()
	for _, fn := range []func() error{
		func() error { _, err := c.LxcStart(ctx, "pve1", 100); return err },
		func() error { _, err := c.LxcStop(ctx, "pve1", 100); return err },
		func() error {
			timeout := int64(30)
			force := true
			_, err := c.LxcShutdown(ctx, "pve1", 100, LxcShutdownParams{Timeout: &timeout, ForceStop: &force})
			return err
		},
	} {
		if err := fn(); err != nil {
			t.Fatalf("power call: %v", err)
		}
	}
	wantPaths := []string{
		"/nodes/pve1/lxc/100/status/start",
		"/nodes/pve1/lxc/100/status/stop",
		"/nodes/pve1/lxc/100/status/shutdown",
	}
	for i, want := range wantPaths {
		if paths[i] != want {
			t.Fatalf("path[%d] = %q, want %q", i, paths[i], want)
		}
	}
	timeout, timeoutOK := bodies[2]["timeout"].(float64)
	if len(bodies[2]) != 2 || !timeoutOK || timeout != 30 || bodies[2]["forceStop"] != true {
		t.Fatalf("shutdown body = %v", bodies[2])
	}
}

// TestMigrateLxcContainerWireBody asserts POST .../migrate sends the target
// and optional migration knobs.
func TestMigrateLxcContainerWireBody(t *testing.T) {
	var captured map[string]any
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/nodes/pve1/lxc/100/migrate" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&captured)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":"UPID:pve1:1:1:vzmigrate:root@pam:migrate:"}`)
	})
	restart := true
	timeout := int64(600)
	_, err := c.MigrateLxcContainer(context.Background(), "pve1", 100, LxcMigrateParams{
		Target:        "pve2",
		TargetStorage: "local-lvm",
		Restart:       &restart,
		Timeout:       &timeout,
	})
	if err != nil {
		t.Fatalf("MigrateLxcContainer: %v", err)
	}
	gotTimeout, timeoutOK := captured["timeout"].(float64)
	if captured["target"] != "pve2" || captured["target-storage"] != "local-lvm" || captured["restart"] != true || !timeoutOK || gotTimeout != 600 {
		t.Fatalf("migrate body = %v", captured)
	}
	if _, present := captured["online"]; present {
		t.Fatal("nil online should be omitted")
	}
}

// TestResizeLxcVolumes asserts the resize verb targets rootfs and mpN disks.
func TestResizeLxcVolumes(t *testing.T) {
	var bodies []map[string]any
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/nodes/pve1/lxc/100/resize" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		bodies = append(bodies, body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":"UPID:pve1:1:1:vzresize:root@pam:resize:"}`)
	})
	ctx := context.Background()
	if _, err := c.ResizeLxcRootfs(ctx, "pve1", 100, "16G"); err != nil {
		t.Fatalf("ResizeLxcRootfs: %v", err)
	}
	if _, err := c.ResizeLxcMountpoint(ctx, "pve1", 100, "mp0", "+4G"); err != nil {
		t.Fatalf("ResizeLxcMountpoint: %v", err)
	}
	if bodies[0]["disk"] != "rootfs" || bodies[0]["size"] != "16G" {
		t.Fatalf("rootfs resize body = %v", bodies[0])
	}
	if bodies[1]["disk"] != "mp0" || bodies[1]["size"] != "+4G" {
		t.Fatalf("mp resize body = %v", bodies[1])
	}
}

// TestGetLxcPending asserts the pending read decodes key/pending/delete rows.
func TestGetLxcPending(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/nodes/pve1/lxc/100/pending" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[
			{"key":"memory","value":"512","pending":"1024","delete":0},
			{"key":"mp1","delete":1},
			{"key":"hostname","value":"ct1"}
		]}`)
	})
	pending, err := c.GetLxcPending(context.Background(), "pve1", 100)
	if err != nil {
		t.Fatalf("GetLxcPending: %v", err)
	}
	if len(pending) != 3 {
		t.Fatalf("pending rows = %d", len(pending))
	}
	if pending[0].Key != "memory" || pending[0].Pending == nil || *pending[0].Pending != "1024" {
		t.Fatalf("row 0 = %+v", pending[0])
	}
	if pending[1].Delete == nil || *pending[1].Delete != 1 {
		t.Fatalf("row 1 = %+v", pending[1])
	}
	if pending[2].Value == nil || *pending[2].Value != "ct1" {
		t.Fatalf("row 2 = %+v", pending[2])
	}
}

// TestGetLxcInterfaces asserts the interfaces read decodes the IP discovery
// rows, including the hardware-address fallback.
func TestGetLxcInterfaces(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/nodes/pve1/lxc/100/interfaces" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[
			{"name":"eth0","hwaddr":"BC:24:11:2A:1D:6F","inet":"10.0.0.5/24","inet6":"fe80::1/64"},
			{"name":"lo","hardware-address":"00:00:00:00:00:00","inet":"127.0.0.1/8"}
		]}`)
	})
	ifaces, err := c.GetLxcInterfaces(context.Background(), "pve1", 100)
	if err != nil {
		t.Fatalf("GetLxcInterfaces: %v", err)
	}
	if len(ifaces) != 2 {
		t.Fatalf("interfaces = %d", len(ifaces))
	}
	if ifaces[0].Name != "eth0" || ifaces[0].HWAddr != "BC:24:11:2A:1D:6F" || ifaces[0].Inet != "10.0.0.5/24" || ifaces[0].Inet6 != "fe80::1/64" {
		t.Fatalf("iface 0 = %+v", ifaces[0])
	}
	if ifaces[1].HWAddr == "" {
		t.Fatal("hardware-address should fall back into HWAddr")
	}
	if ifaces[1].HWAddr != "00:00:00:00:00:00" {
		t.Fatalf("iface 1 = %+v", ifaces[1])
	}
}
