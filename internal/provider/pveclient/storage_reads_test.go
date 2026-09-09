// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"context"
	"io"
	"net/http"
	"testing"
)

// TestClient_ListNodeStorages_ContentFilterQuery verifies the content filter
// travels as a query parameter and a full index row decodes, including the
// comma-separated content list and the boolish flags PVE emits as 0/1.
func TestClient_ListNodeStorages_ContentFilterQuery(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/nodes/pve1/storage" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.URL.Query().Get("content"); got != "iso" {
			t.Fatalf("content query = %q, want iso", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"storage":"local","type":"dir","content":"iso,vztmpl,backup","shared":0,"enabled":1,"active":1,"total":93985794048,"used":39187787776,"avail":54798006272,"used_fraction":0.41716}]}`)
	})
	storages, err := c.ListNodeStorages(context.Background(), "pve1", "iso")
	if err != nil {
		t.Fatalf("ListNodeStorages: %v", err)
	}
	if len(storages) != 1 {
		t.Fatalf("storages = %+v, want one row", storages)
	}
	s := storages[0]
	if s.Storage != "local" || s.Type != "dir" {
		t.Fatalf("storage = %q type = %q, want local/dir", s.Storage, s.Type)
	}
	if len(s.Content) != 3 || s.Content[0] != "iso" || s.Content[1] != "vztmpl" || s.Content[2] != "backup" {
		t.Fatalf("content = %v, want [iso vztmpl backup]", s.Content)
	}
	if s.Shared == nil || *s.Shared {
		t.Fatalf("shared = %v, want false", s.Shared)
	}
	if s.Enabled == nil || !*s.Enabled || s.Active == nil || !*s.Active {
		t.Fatalf("enabled/active = %v/%v, want true/true", s.Enabled, s.Active)
	}
	if s.Total == nil || *s.Total != 93985794048 || s.Used == nil || *s.Used != 39187787776 || s.Avail == nil || *s.Avail != 54798006272 {
		t.Fatalf("byte counters = %v/%v/%v, want used/total/avail populated", s.Used, s.Total, s.Avail)
	}
	if s.UsedFraction == nil || *s.UsedFraction != 0.41716 {
		t.Fatalf("used_fraction = %v, want 0.41716", s.UsedFraction)
	}
}

// TestClient_ListNodeStorages_NoFilterOmitsQuery verifies an empty content
// filter sends no query string, absent optional fields stay nil, and the
// content list also decodes from the JSON array encoding.
func TestClient_ListNodeStorages_NoFilterOmitsQuery(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" {
			t.Fatalf("raw query = %q, want empty", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"storage":"cephfs-1","type":"cephfs","content":"snippets"},{"storage":"nfs-1","type":"nfs","content":["iso","backup"],"shared":1}]}`)
	})
	storages, err := c.ListNodeStorages(context.Background(), "pve1", "")
	if err != nil {
		t.Fatalf("ListNodeStorages: %v", err)
	}
	if len(storages) != 2 {
		t.Fatalf("storages = %+v, want two rows", storages)
	}
	first, second := storages[0], storages[1]
	if len(first.Content) != 1 || first.Content[0] != "snippets" {
		t.Fatalf("first content = %v, want [snippets]", first.Content)
	}
	if first.Shared != nil || first.Enabled != nil || first.Active != nil || first.Used != nil || first.Total != nil || first.Avail != nil || first.UsedFraction != nil {
		t.Fatalf("first row optional fields = %+v, want all nil", first)
	}
	if len(second.Content) != 2 || second.Content[0] != "iso" || second.Content[1] != "backup" {
		t.Fatalf("second content = %v, want [iso backup]", second.Content)
	}
	if second.Shared == nil || !*second.Shared {
		t.Fatalf("second shared = %v, want true", second.Shared)
	}
}

// TestClient_ListStorageContent_ContentFilterQuery verifies the content
// filter query and the decode of every field the pin defines on a content
// row, including the boolish protected flag and the nested verification
// object.
func TestClient_ListStorageContent_ContentFilterQuery(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/nodes/pve1/storage/local/content" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.URL.Query().Get("content"); got != "iso" {
			t.Fatalf("content query = %q, want iso", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"volid":"local:iso/debian-12.iso","format":"iso","size":658505728,"used":658505728,"vmid":100,"notes":"netinst","ctime":1700000000,"parent":"local:base-100-disk-0","protected":1,"encrypted":"ab:cd:ef","verification":{"state":"ok","upid":"UPID:pve1:0000ABCD"},"approximate-size":658505728}]}`)
	})
	files, err := c.ListStorageContent(context.Background(), "pve1", "local", "iso")
	if err != nil {
		t.Fatalf("ListStorageContent: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("files = %+v, want one row", files)
	}
	f := files[0]
	if f.Volid != "local:iso/debian-12.iso" || f.Format != "iso" {
		t.Fatalf("volid = %q format = %q", f.Volid, f.Format)
	}
	if f.Size == nil || *f.Size != 658505728 || f.Used == nil || *f.Used != 658505728 || f.ApproximateSize == nil || *f.ApproximateSize != 658505728 {
		t.Fatalf("sizes = %v/%v/%v, want 658505728 each", f.Size, f.Used, f.ApproximateSize)
	}
	if f.VMID == nil || *f.VMID != 100 {
		t.Fatalf("vmid = %v, want 100", f.VMID)
	}
	if f.Notes == nil || *f.Notes != "netinst" {
		t.Fatalf("notes = %v, want netinst", f.Notes)
	}
	if f.Ctime == nil || *f.Ctime != 1700000000 {
		t.Fatalf("ctime = %v, want 1700000000", f.Ctime)
	}
	if f.Parent == nil || *f.Parent != "local:base-100-disk-0" {
		t.Fatalf("parent = %v, want local:base-100-disk-0", f.Parent)
	}
	if f.Protected == nil || !*f.Protected {
		t.Fatalf("protected = %v, want true from int encoding", f.Protected)
	}
	if f.Encrypted == nil || *f.Encrypted != "ab:cd:ef" {
		t.Fatalf("encrypted = %v, want ab:cd:ef", f.Encrypted)
	}
	if f.Verification == nil || f.Verification.State == nil || *f.Verification.State != "ok" || f.Verification.Upid == nil || *f.Verification.Upid != "UPID:pve1:0000ABCD" {
		t.Fatalf("verification = %+v, want state ok with upid", f.Verification)
	}
}

// TestClient_ListStorageContent_MinimalRowNoFilter verifies an empty content
// filter sends no query string and a bare row leaves the optional fields nil.
func TestClient_ListStorageContent_MinimalRowNoFilter(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" {
			t.Fatalf("raw query = %q, want empty", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"volid":"local:vztmpl/alpine-3.20-default_20240812_x86_64.tar.xz","format":"vztmpl","size":2712}]}`)
	})
	files, err := c.ListStorageContent(context.Background(), "pve1", "local", "")
	if err != nil {
		t.Fatalf("ListStorageContent: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("files = %+v, want one row", files)
	}
	f := files[0]
	if f.Volid != "local:vztmpl/alpine-3.20-default_20240812_x86_64.tar.xz" || f.Format != "vztmpl" || f.Size == nil || *f.Size != 2712 {
		t.Fatalf("row = %+v, want volid/format/size populated", f)
	}
	if f.Used != nil || f.VMID != nil || f.Notes != nil || f.Ctime != nil || f.Parent != nil || f.Protected != nil || f.Encrypted != nil || f.Verification != nil || f.ApproximateSize != nil {
		t.Fatalf("optional fields = %+v, want all nil", f)
	}
}
