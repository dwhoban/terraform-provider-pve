// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"testing"
)

// TestPruneStorageBackups verifies DELETE against
// /nodes/{node}/storage/{storage}/prunebackups: the retention options travel
// as the prune-backups property string, type and vmid as filters, and the
// returned task UPID reaches the caller.
func TestPruneStorageBackups(t *testing.T) {
	var sawQuery url.Values
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/nodes/pve1/storage/local/prunebackups" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		sawQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":"`+fakeUpid+`"}`)
	})
	upid, err := c.PruneStorageBackups(context.Background(), "pve1", "local", StoragePruneOptions{
		KeepDaily: int64Ptr(7),
		KeepLast:  int64Ptr(3),
		Type:      "qemu",
		VMID:      int64Ptr(100),
	})
	if err != nil {
		t.Fatalf("PruneStorageBackups: %v", err)
	}
	if upid != fakeUpid {
		t.Fatalf("upid = %q, want %q", upid, fakeUpid)
	}
	if got := sawQuery.Get("prune-backups"); got != "keep-daily=7,keep-last=3" {
		t.Fatalf("prune-backups = %q, want keep-daily=7,keep-last=3", got)
	}
	if got := sawQuery.Get("type"); got != "qemu" {
		t.Fatalf("type = %q, want qemu", got)
	}
	if got := sawQuery.Get("vmid"); got != "100" {
		t.Fatalf("vmid = %q, want 100", got)
	}
}

// TestPruneStorageBackupsWithoutOptions verifies that with every option
// unset the request carries no query parameters at all, letting PVE fall
// back to the storage configuration's retention, and that keep-all renders
// as the 1/0 property-string encoding.
func TestPruneStorageBackupsWithoutOptions(t *testing.T) {
	var sawRawQuery string
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/nodes/pve1/storage/local/prunebackups" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		sawRawQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":"`+fakeUpid+`"}`)
	})
	if _, err := c.PruneStorageBackups(context.Background(), "pve1", "local", StoragePruneOptions{}); err != nil {
		t.Fatalf("PruneStorageBackups: %v", err)
	}
	if sawRawQuery != "" {
		t.Fatalf("raw query = %q, want empty", sawRawQuery)
	}
	if _, err := c.PruneStorageBackups(context.Background(), "pve1", "local", StoragePruneOptions{KeepAll: boolPtr(true), KeepHourly: int64Ptr(2)}); err != nil {
		t.Fatalf("PruneStorageBackups keep-all: %v", err)
	}
	if sawRawQuery != "prune-backups=keep-all%3D1%2Ckeep-hourly%3D2" {
		t.Fatalf("raw query = %q, want prune-backups=keep-all=1,keep-hourly=2", sawRawQuery)
	}
}

// TestDryRunStoragePruneBackups verifies GET against the same path returns
// the prune preview listing, decoding the optional vmid as nil when absent.
func TestDryRunStoragePruneBackups(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/nodes/pve1/storage/local/prunebackups" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.URL.Query().Get("prune-backups"); got != "keep-last=1" {
			t.Fatalf("prune-backups = %q, want keep-last=1", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[`+
			`{"volid":"local:backup/vzdump-qemu-100-2026_01_01-00_00_00.vma.zst","mark":"remove","type":"qemu","vmid":100,"ctime":1767225600},`+
			`{"volid":"local:backup/vzdump-lxc-200-2026_01_02-00_00_00.tar.zst","mark":"keep","type":"lxc","ctime":1767312000}]}`)
	})
	entries, err := c.DryRunStoragePruneBackups(context.Background(), "pve1", "local", StoragePruneOptions{KeepLast: int64Ptr(1)})
	if err != nil {
		t.Fatalf("DryRunStoragePruneBackups: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2", len(entries))
	}
	if entries[0].Mark != "remove" || entries[0].VMID == nil || *entries[0].VMID != 100 {
		t.Fatalf("entries[0] = %+v, want mark remove with vmid 100", entries[0])
	}
	if entries[1].Mark != "keep" || entries[1].VMID != nil {
		t.Fatalf("entries[1] = %+v, want mark keep without vmid", entries[1])
	}
}
