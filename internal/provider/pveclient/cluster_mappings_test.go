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

// TestMappings_Dir_CRUD covers the directory mapping wire set: create body
// with property-string map entries, read decode back into typed entries,
// update with the delete query, and delete.
func TestMappings_Dir_CRUD(t *testing.T) {
	var lastBody []byte
	var lastQuery string
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		lastBody, _ = io.ReadAll(r.Body)
		lastQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/cluster/mapping/dir":
			_, _ = io.WriteString(w, `{"data":null}`)
		case r.Method == http.MethodGet && r.URL.Path == "/cluster/mapping/dir":
			_, _ = io.WriteString(w, `{"data":[{"id":"share","description":"media dirs","map":["node=pve1,path=/mnt/share","node=pve2,path=/mnt/share2"]}]}`)
		case r.Method == http.MethodGet && r.URL.Path == "/cluster/mapping/dir/share":
			_, _ = io.WriteString(w, `{"data":{"id":"share","description":"media dirs","map":["node=pve1,path=/mnt/share","node=pve2,path=/mnt/share2"]}}`)
		case r.Method == http.MethodPut && r.URL.Path == "/cluster/mapping/dir/share":
			_, _ = io.WriteString(w, `{"data":null}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/cluster/mapping/dir/share":
			_, _ = io.WriteString(w, `{"data":null}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	ctx := context.Background()

	m := MappingDir{
		ID:          "share",
		Description: "media dirs",
		Map: []MappingDirEntry{
			{Node: "pve1", Path: "/mnt/share"},
			{Node: "pve2", Path: "/mnt/share2"},
		},
	}
	if err := c.CreateMappingDir(ctx, m); err != nil {
		t.Fatalf("CreateMappingDir: %v", err)
	}
	var sent map[string]any
	if err := json.Unmarshal(lastBody, &sent); err != nil {
		t.Fatalf("create body %q is not JSON: %v", lastBody, err)
	}
	want := []any{"node=pve1,path=/mnt/share", "node=pve2,path=/mnt/share2"}
	// Decoded JSON always yields []any for arrays.
	entries, ok := sent["map"].([]any)
	if !ok || len(entries) != 2 || entries[0] != want[0] || entries[1] != want[1] {
		t.Fatalf("create body map = %v", sent["map"])
	}
	if sent["id"] != "share" || sent["description"] != "media dirs" {
		t.Fatalf("create body = %v", sent)
	}

	list, err := c.ListMappingDirs(ctx)
	if err != nil {
		t.Fatalf("ListMappingDirs: %v", err)
	}
	if len(list) != 1 || list[0].ID != "share" || len(list[0].Map) != 2 || list[0].Map[1].Path != "/mnt/share2" {
		t.Fatalf("list = %+v", list)
	}

	read, err := c.GetMappingDir(ctx, "share")
	if err != nil {
		t.Fatalf("GetMappingDir: %v", err)
	}
	if read.Description != "media dirs" || read.Map[0].Node != "pve1" || read.Map[0].Path != "/mnt/share" {
		t.Fatalf("read = %+v", read)
	}

	update := MappingDir{ID: "share", Map: []MappingDirEntry{{Node: "pve1", Path: "/mnt/other"}}}
	if err := c.UpdateMappingDir(ctx, "share", update, []string{"description"}); err != nil {
		t.Fatalf("UpdateMappingDir: %v", err)
	}
	if !strings.HasPrefix(lastQuery, "delete=description") {
		t.Fatalf("update query = %q", lastQuery)
	}
	if err := json.Unmarshal(lastBody, &sent); err != nil {
		t.Fatalf("update body %q is not JSON: %v", lastBody, err)
	}
	// Decoded JSON always yields []any for arrays; ok was declared above.
	entries, ok = sent["map"].([]any)
	if !ok || len(entries) == 0 || entries[0] != "node=pve1,path=/mnt/other" {
		t.Fatalf("update body map = %v", sent["map"])
	}

	if err := c.DeleteMappingDir(ctx, "share"); err != nil {
		t.Fatalf("DeleteMappingDir: %v", err)
	}
}

// TestMappings_PCI_CRUD covers the PCI mapping wire set, including the
// quoted property-string encoding for description values that contain
// commas and the boolish mdev/live-migration-capable flags.
func TestMappings_PCI_CRUD(t *testing.T) {
	var lastBody []byte
	var lastQuery string
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		lastBody, _ = io.ReadAll(r.Body)
		lastQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/cluster/mapping/pci":
			_, _ = io.WriteString(w, `{"data":null}`)
		case r.Method == http.MethodGet && r.URL.Path == "/cluster/mapping/pci":
			_, _ = io.WriteString(w, `{"data":[{"id":"gpu","description":"host GPU","map":["node=pve1,id=10de:2231,iommugroup=14,path=0000:01:00.0,subsystem-id=1043:8888,description=\"GPU, top slot\""]}]}`)
		case r.Method == http.MethodGet && r.URL.Path == "/cluster/mapping/pci/gpu":
			_, _ = io.WriteString(w, `{"data":{"id":"gpu","mdev":1,"live-migration-capable":0,"map":["node=pve1,id=10de:2231,iommugroup=14,path=0000:01:00.0,subsystem-id=1043:8888,description=\"GPU, top slot\""]}}`)
		case r.Method == http.MethodPut && r.URL.Path == "/cluster/mapping/pci/gpu":
			_, _ = io.WriteString(w, `{"data":null}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/cluster/mapping/pci/gpu":
			_, _ = io.WriteString(w, `{"data":null}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	ctx := context.Background()

	m := MappingPCI{
		ID:                   "gpu",
		Description:          "host GPU",
		Mdev:                 HABoolPtr(true),
		LiveMigrationCapable: HABoolPtr(false),
		Map: []MappingPCIEntry{{
			Node:        "pve1",
			ID:          "10de:2231",
			IOMMUGroup:  HAInt64Ptr(14),
			Path:        "0000:01:00.0",
			SubsystemID: "1043:8888",
			Description: "GPU, top slot",
		}},
	}
	if err := c.CreateMappingPCI(ctx, m); err != nil {
		t.Fatalf("CreateMappingPCI: %v", err)
	}
	var sent map[string]any
	if err := json.Unmarshal(lastBody, &sent); err != nil {
		t.Fatalf("create body %q is not JSON: %v", lastBody, err)
	}
	// Decoded JSON arrays yield []any; the entry itself is a string.
	entries, ok := sent["map"].([]any)
	if !ok || len(entries) == 0 {
		t.Fatalf("create body map = %v", sent["map"])
	}
	entry, ok := entries[0].(string)
	if !ok || !strings.HasPrefix(entry, "node=pve1,id=10de:2231,iommugroup=14,path=0000:01:00.0,subsystem-id=1043:8888,description=") {
		t.Fatalf("create map entry = %q", entry)
	}
	if !strings.Contains(entry, `"GPU, top slot"`) {
		t.Fatalf("map entry does not quote the comma-containing description: %q", entry)
	}
	if sent["mdev"] != true || sent["live-migration-capable"] != false {
		t.Fatalf("create body flags = %v", sent)
	}

	list, err := c.ListMappingPCI(ctx)
	if err != nil {
		t.Fatalf("ListMappingPCI: %v", err)
	}
	if len(list) != 1 || list[0].ID != "gpu" || len(list[0].Map) != 1 {
		t.Fatalf("list = %+v", list)
	}
	e := list[0].Map[0]
	if e.Node != "pve1" || e.ID != "10de:2231" || e.IOMMUGroup == nil || *e.IOMMUGroup != 14 || e.Path != "0000:01:00.0" || e.SubsystemID != "1043:8888" || e.Description != "GPU, top slot" {
		t.Fatalf("decoded pci entry = %+v", e)
	}

	read, err := c.GetMappingPCI(ctx, "gpu")
	if err != nil {
		t.Fatalf("GetMappingPCI: %v", err)
	}
	if read.Mdev == nil || !*read.Mdev || read.LiveMigrationCapable == nil || *read.LiveMigrationCapable {
		t.Fatalf("read flags = %+v", read)
	}

	update := MappingPCI{ID: "gpu", Map: m.Map, Mdev: HABoolPtr(false)}
	if err := c.UpdateMappingPCI(ctx, "gpu", update, []string{"description"}); err != nil {
		t.Fatalf("UpdateMappingPCI: %v", err)
	}
	if !strings.HasPrefix(lastQuery, "delete=description") {
		t.Fatalf("update query = %q", lastQuery)
	}
	if err := json.Unmarshal(lastBody, &sent); err != nil {
		t.Fatalf("update body %q is not JSON: %v", lastBody, err)
	}
	if sent["mdev"] != false {
		t.Fatalf("update body = %v", sent)
	}

	if err := c.DeleteMappingPCI(ctx, "gpu"); err != nil {
		t.Fatalf("DeleteMappingPCI: %v", err)
	}
}

// TestMappings_USB_CRUD covers the USB mapping wire set: create body with
// property-string entries, read decode, update with the delete query, and
// delete.
func TestMappings_USB_CRUD(t *testing.T) {
	var lastBody []byte
	var lastQuery string
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		lastBody, _ = io.ReadAll(r.Body)
		lastQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/cluster/mapping/usb":
			_, _ = io.WriteString(w, `{"data":null}`)
		case r.Method == http.MethodGet && r.URL.Path == "/cluster/mapping/usb":
			_, _ = io.WriteString(w, `{"data":[{"id":"ups","description":"UPS link","map":["node=pve1,id=8087:0a2a,path=1-2"]}]}`)
		case r.Method == http.MethodGet && r.URL.Path == "/cluster/mapping/usb/ups":
			_, _ = io.WriteString(w, `{"data":{"id":"ups","description":"UPS link","map":["node=pve1,id=8087:0a2a,path=1-2"]}}`)
		case r.Method == http.MethodPut && r.URL.Path == "/cluster/mapping/usb/ups":
			_, _ = io.WriteString(w, `{"data":null}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/cluster/mapping/usb/ups":
			_, _ = io.WriteString(w, `{"data":null}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	ctx := context.Background()

	m := MappingUSB{
		ID:          "ups",
		Description: "UPS link",
		Map:         []MappingUSBEntry{{Node: "pve1", ID: "8087:0a2a", Path: "1-2"}},
	}
	if err := c.CreateMappingUSB(ctx, m); err != nil {
		t.Fatalf("CreateMappingUSB: %v", err)
	}
	var sent map[string]any
	if err := json.Unmarshal(lastBody, &sent); err != nil {
		t.Fatalf("create body %q is not JSON: %v", lastBody, err)
	}
	// Decoded JSON always yields []any for arrays.
	entries, ok := sent["map"].([]any)
	if !ok || len(entries) == 0 || entries[0] != "node=pve1,id=8087:0a2a,path=1-2" {
		t.Fatalf("create body map = %v", sent["map"])
	}

	list, err := c.ListMappingUSB(ctx)
	if err != nil {
		t.Fatalf("ListMappingUSB: %v", err)
	}
	if len(list) != 1 || list[0].Map[0].ID != "8087:0a2a" || list[0].Map[0].Path != "1-2" {
		t.Fatalf("list = %+v", list)
	}

	read, err := c.GetMappingUSB(ctx, "ups")
	if err != nil {
		t.Fatalf("GetMappingUSB: %v", err)
	}
	if read.Description != "UPS link" || read.Map[0].Node != "pve1" {
		t.Fatalf("read = %+v", read)
	}

	update := MappingUSB{ID: "ups", Map: []MappingUSBEntry{{Node: "pve2", ID: "8087:0a2b"}}}
	if err := c.UpdateMappingUSB(ctx, "ups", update, []string{"description"}); err != nil {
		t.Fatalf("UpdateMappingUSB: %v", err)
	}
	if !strings.HasPrefix(lastQuery, "delete=description") {
		t.Fatalf("update query = %q", lastQuery)
	}

	if err := c.DeleteMappingUSB(ctx, "ups"); err != nil {
		t.Fatalf("DeleteMappingUSB: %v", err)
	}
}
