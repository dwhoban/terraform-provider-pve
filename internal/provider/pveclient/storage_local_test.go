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

// TestPveStorageLvm_ClientCRUD covers the LVM storage wire set: create body
// with joined content/nodes lists and the property-string prune-backups,
// read decode with boolish flags and lenient ints, update with the delete
// query, and delete.
func TestPveStorageLvm_ClientCRUD(t *testing.T) {
	var lastBody []byte
	var lastQuery string
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		lastBody, _ = io.ReadAll(r.Body)
		lastQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/storage":
			_, _ = io.WriteString(w, `{"data":{"storage":"vmstore","type":"lvm"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/storage/vmstore":
			_, _ = io.WriteString(w, `{"data":{"type":"lvm","storage":"vmstore","digest":"abc123","vgname":"vg0","base":"pve/vm-disk","content":"images,rootdir","nodes":"pve1,pve2","disable":1,"shared":true,"saferemove":true,"saferemove-stepsize":"16","tagged_only":1,"bwlimit":"default=500","prune-backups":{"keep-last":3,"keep-daily":7},"max-protected-backups":5}}`)
		case r.Method == http.MethodPut && r.URL.Path == "/storage/vmstore":
			_, _ = io.WriteString(w, `{"data":{"storage":"vmstore","type":"lvm"}}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/storage/vmstore":
			_, _ = io.WriteString(w, `{"data":null}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	ctx := context.Background()

	cfg := StorageLocalConfig{
		Storage:             "vmstore",
		Type:                "lvm",
		Content:             []string{"images", "rootdir"},
		Nodes:               []string{"pve1", "pve2"},
		Shared:              storageLocalBoolPtr(true),
		BWLimit:             "default=500",
		PruneBackups:        &BackupPruneBackups{KeepLast: storageLocalInt64Ptr(3), KeepDaily: storageLocalInt64Ptr(7)},
		MaxProtectedBackups: storageLocalInt64Ptr(5),
		VGName:              "vg0",
		Base:                "pve/vm-disk",
		SafeRemove:          storageLocalBoolPtr(true),
		SafeRemoveStepSize:  storageLocalInt64Ptr(16),
		TaggedOnly:          storageLocalBoolPtr(true),
	}
	if err := c.CreateStorageLocal(ctx, cfg); err != nil {
		t.Fatalf("CreateStorageLocal: %v", err)
	}
	var sent map[string]any
	if err := json.Unmarshal(lastBody, &sent); err != nil {
		t.Fatalf("create body %q is not JSON: %v", lastBody, err)
	}
	if sent["storage"] != "vmstore" || sent["type"] != "lvm" || sent["vgname"] != "vg0" {
		t.Fatalf("create body = %v", sent)
	}
	if sent["content"] != "images,rootdir" || sent["nodes"] != "pve1,pve2" {
		t.Fatalf("list encoding = %v, want joined strings", sent)
	}
	if sent["shared"] != true || sent["saferemove"] != true || sent["tagged_only"] != true {
		t.Fatalf("bool encoding = %v, want JSON booleans", sent)
	}
	if sent["saferemove-stepsize"] != float64(16) || sent["max-protected-backups"] != float64(5) {
		t.Fatalf("int encoding = %v", sent)
	}
	if sent["bwlimit"] != "default=500" {
		t.Fatalf("bwlimit = %v", sent)
	}
	prune, ok := sent["prune-backups"].(string)
	if !ok || !strings.Contains(prune, "keep-last=3") || !strings.Contains(prune, "keep-daily=7") {
		t.Fatalf("prune-backups = %v, want property string", sent["prune-backups"])
	}

	read, err := c.GetStorageLocal(ctx, "vmstore")
	if err != nil {
		t.Fatalf("GetStorageLocal: %v", err)
	}
	if read.Type != "lvm" || read.VGName != "vg0" || read.Base != "pve/vm-disk" || read.Digest != "abc123" {
		t.Fatalf("read config = %+v", read)
	}
	if len(read.Content) != 2 || read.Content[0] != "images" || len(read.Nodes) != 2 || read.Nodes[1] != "pve2" {
		t.Fatalf("list split = %+v", read)
	}
	if read.Disable == nil || !*read.Disable || read.Shared == nil || !*read.Shared {
		t.Fatalf("boolish decode = %+v", read)
	}
	if read.SafeRemoveStepSize == nil || *read.SafeRemoveStepSize != 16 {
		t.Fatalf("lenient int decode = %+v", read.SafeRemoveStepSize)
	}
	if read.PruneBackups == nil || read.PruneBackups.KeepLast == nil || *read.PruneBackups.KeepLast != 3 || *read.PruneBackups.KeepDaily != 7 {
		t.Fatalf("prune-backups decode = %+v", read.PruneBackups)
	}
	if read.MaxProtectedBackups == nil || *read.MaxProtectedBackups != 5 {
		t.Fatalf("max-protected-backups decode = %+v", read.MaxProtectedBackups)
	}
	if read.BWLimit != "default=500" {
		t.Fatalf("bwlimit decode = %+v", read.BWLimit)
	}

	update := StorageLocalConfig{Storage: "vmstore", Type: "lvm", VGName: "vg0", BWLimit: "default=100"}
	if err := c.UpdateStorageLocal(ctx, "vmstore", update, []string{"disable", "shared"}); err != nil {
		t.Fatalf("UpdateStorageLocal: %v", err)
	}
	if !strings.HasPrefix(lastQuery, "delete=") || !strings.Contains(lastQuery, "disable") || !strings.Contains(lastQuery, "shared") {
		t.Fatalf("update query = %q", lastQuery)
	}
	if err := json.Unmarshal(lastBody, &sent); err != nil {
		t.Fatalf("update body %q is not JSON: %v", lastBody, err)
	}
	if sent["bwlimit"] != "default=100" {
		t.Fatalf("update body = %v", sent)
	}

	if err := c.DeleteStorageLocal(ctx, "vmstore"); err != nil {
		t.Fatalf("DeleteStorageLocal: %v", err)
	}
}

// TestPveStorageLvmthin_ClientCRUD covers the LVM-thin wire set: create
// body with vgname and thinpool, and read decode.
func TestPveStorageLvmthin_ClientCRUD(t *testing.T) {
	var lastBody []byte
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		lastBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/storage":
			_, _ = io.WriteString(w, `{"data":{"storage":"thinstore","type":"lvmthin"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/storage/thinstore":
			_, _ = io.WriteString(w, `{"data":{"type":"lvmthin","storage":"thinstore","vgname":"vg0","thinpool":"data","content":"images,rootdir","tagged_only":1}}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	ctx := context.Background()

	cfg := StorageLocalConfig{
		Storage:    "thinstore",
		Type:       "lvmthin",
		Content:    []string{"images", "rootdir"},
		VGName:     "vg0",
		ThinPool:   "data",
		TaggedOnly: storageLocalBoolPtr(true),
	}
	if err := c.CreateStorageLocal(ctx, cfg); err != nil {
		t.Fatalf("CreateStorageLocal: %v", err)
	}
	var sent map[string]any
	if err := json.Unmarshal(lastBody, &sent); err != nil {
		t.Fatalf("create body %q is not JSON: %v", lastBody, err)
	}
	if sent["type"] != "lvmthin" || sent["vgname"] != "vg0" || sent["thinpool"] != "data" {
		t.Fatalf("create body = %v", sent)
	}

	read, err := c.GetStorageLocal(ctx, "thinstore")
	if err != nil {
		t.Fatalf("GetStorageLocal: %v", err)
	}
	if read.VGName != "vg0" || read.ThinPool != "data" {
		t.Fatalf("read config = %+v", read)
	}
	if read.TaggedOnly == nil || !*read.TaggedOnly {
		t.Fatalf("tagged_only decode = %+v", read.TaggedOnly)
	}
}

// TestPveStorageZfspool_ClientCRUD covers the ZFS pool wire set: create
// body with pool, blocksize, and sparse, and read decode with boolish
// sparse.
func TestPveStorageZfspool_ClientCRUD(t *testing.T) {
	var lastBody []byte
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		lastBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/storage":
			_, _ = io.WriteString(w, `{"data":{"storage":"zfspool","type":"zfspool"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/storage/zfspool":
			_, _ = io.WriteString(w, `{"data":{"type":"zfspool","storage":"zfspool","pool":"tank","blocksize":"16k","sparse":"1","content":"images,rootdir"}}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	ctx := context.Background()

	cfg := StorageLocalConfig{
		Storage:   "zfspool",
		Type:      "zfspool",
		Content:   []string{"images", "rootdir"},
		Pool:      "tank",
		BlockSize: "16k",
		Sparse:    storageLocalBoolPtr(true),
	}
	if err := c.CreateStorageLocal(ctx, cfg); err != nil {
		t.Fatalf("CreateStorageLocal: %v", err)
	}
	var sent map[string]any
	if err := json.Unmarshal(lastBody, &sent); err != nil {
		t.Fatalf("create body %q is not JSON: %v", lastBody, err)
	}
	if sent["type"] != "zfspool" || sent["pool"] != "tank" || sent["blocksize"] != "16k" || sent["sparse"] != true {
		t.Fatalf("create body = %v", sent)
	}

	read, err := c.GetStorageLocal(ctx, "zfspool")
	if err != nil {
		t.Fatalf("GetStorageLocal: %v", err)
	}
	if read.Pool != "tank" || read.BlockSize != "16k" {
		t.Fatalf("read config = %+v", read)
	}
	if read.Sparse == nil || !*read.Sparse {
		t.Fatalf("sparse decode = %+v", read.Sparse)
	}
}

// TestPveStorageDirectory_ClientCRUD covers the directory wire set: create
// body with path and mountpoint handling, and read decode where
// is_mountpoint travels as a path string and the deprecated mkdir stays
// absent.
func TestPveStorageDirectory_ClientCRUD(t *testing.T) {
	var lastBody []byte
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		lastBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/storage":
			_, _ = io.WriteString(w, `{"data":{"storage":"dirstore","type":"dir"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/storage/dirstore":
			_, _ = io.WriteString(w, `{"data":{"type":"dir","storage":"dirstore","path":"/srv/pve","is_mountpoint":"/srv/external","content":"iso,vztmpl","create-base-path":true,"create-subdirs":0}}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	ctx := context.Background()

	cfg := StorageLocalConfig{
		Storage:        "dirstore",
		Type:           "dir",
		Content:        []string{"iso", "vztmpl"},
		Path:           "/srv/pve",
		IsMountpoint:   "yes",
		CreateBasePath: storageLocalBoolPtr(true),
		CreateSubdirs:  storageLocalBoolPtr(true),
		Mkdir:          storageLocalBoolPtr(true),
	}
	if err := c.CreateStorageLocal(ctx, cfg); err != nil {
		t.Fatalf("CreateStorageLocal: %v", err)
	}
	var sent map[string]any
	if err := json.Unmarshal(lastBody, &sent); err != nil {
		t.Fatalf("create body %q is not JSON: %v", lastBody, err)
	}
	if sent["type"] != "dir" || sent["path"] != "/srv/pve" || sent["is_mountpoint"] != "yes" {
		t.Fatalf("create body = %v", sent)
	}
	if sent["create-base-path"] != true || sent["create-subdirs"] != true || sent["mkdir"] != true {
		t.Fatalf("bool encoding = %v, want JSON booleans", sent)
	}

	read, err := c.GetStorageLocal(ctx, "dirstore")
	if err != nil {
		t.Fatalf("GetStorageLocal: %v", err)
	}
	if read.Path != "/srv/pve" || read.IsMountpoint != "/srv/external" {
		t.Fatalf("read config = %+v", read)
	}
	if read.CreateBasePath == nil || !*read.CreateBasePath {
		t.Fatalf("create-base-path decode = %+v", read.CreateBasePath)
	}
	if read.CreateSubdirs == nil || *read.CreateSubdirs {
		t.Fatalf("create-subdirs decode = %+v, want false from 0", read.CreateSubdirs)
	}
	if read.Mkdir != nil {
		t.Fatalf("mkdir decode = %+v, want nil for absent", read.Mkdir)
	}
}
