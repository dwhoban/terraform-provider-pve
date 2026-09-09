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

// TestClient_ListNodeTasks_DecodesAndFilters covers the finished-task list.
func TestClient_ListNodeTasks_DecodesAndFilters(t *testing.T) {
	var gotQuery string
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nodes/pve1/tasks" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"upid":"UPID:pve1:0001","node":"pve1","type":"vzdump","id":"100","user":"root@pam","starttime":1700000000,"endtime":1700000100,"status":"OK","pid":42,"pstart":7}]}`)
	})
	limit := int64(10)
	since := int64(1699999999)
	entries, err := c.ListNodeTasks(context.Background(), "pve1", ListNodeTasksOptions{Limit: &limit, Since: &since, TypeFilter: "vzdump", Source: "all"})
	if err != nil {
		t.Fatalf("ListNodeTasks: %v", err)
	}
	for _, want := range []string{"limit=10", "since=1699999999", "typefilter=vzdump", "source=all"} {
		if !containsQuery(gotQuery, want) {
			t.Fatalf("query %q missing %q", gotQuery, want)
		}
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	e := entries[0]
	if e.UPID != "UPID:pve1:0001" || e.Node != "pve1" || e.Type != "vzdump" || e.ID == nil || *e.ID != "100" || e.User != "root@pam" || e.Starttime != 1700000000 || e.Status == nil || *e.Status != "OK" || e.PID == nil || *e.PID != 42 || e.PStart == nil || *e.PStart != 7 {
		t.Fatalf("unexpected entry: %+v", e)
	}
	if e.Endtime == nil || *e.Endtime != 1700000100 {
		t.Fatalf("endtime = %v, want 1700000100", e.Endtime)
	}
}

// TestClient_ListNodePciDevices_Decodes covers the PCI scan with mdev and
// optional name fields.
func TestClient_ListNodePciDevices_Decodes(t *testing.T) {
	var gotQuery string
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nodes/pve1/hardware/pci" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"id":"0000:01:00.0","class":"0x0300","device":"0x1234","device_name":"Some GPU","vendor":"0x8086","vendor_name":"Intel","iommugroup":12,"mdev":true,"subsystem_device":"0x5678","subsystem_vendor":"0x9012"}]}`)
	})
	verbose := true
	devices, err := c.ListNodePciDevices(context.Background(), "pve1", ListNodePciDevicesOptions{PCIClassBlacklist: "05", Verbose: &verbose})
	if err != nil {
		t.Fatalf("ListNodePciDevices: %v", err)
	}
	if !containsQuery(gotQuery, "pci-class-blacklist=05") || !containsQuery(gotQuery, "verbose=1") {
		t.Fatalf("unexpected query: %s", gotQuery)
	}
	if len(devices) != 1 {
		t.Fatalf("got %d devices, want 1", len(devices))
	}
	d := devices[0]
	if d.ID != "0000:01:00.0" || d.DeviceName == nil || *d.DeviceName != "Some GPU" || d.IOMMUGroup != 12 {
		t.Fatalf("unexpected device: %+v", d)
	}
	if d.MDev == nil || !*d.MDev {
		t.Fatalf("mdev = %v, want true", d.MDev)
	}
}

// TestClient_ListNodeUsbDevices_Decodes covers the USB scan.
func TestClient_ListNodeUsbDevices_Decodes(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nodes/pve1/hardware/usb" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"busnum":1,"class":9,"devnum":2,"level":0,"port":1,"prodid":"0x1234","vendid":"0x8087","manufacturer":"Intel","product":"Bluetooth","serial":"AA:BB","speed":"480","usbpath":"1-2"}]}`)
	})
	devices, err := c.ListNodeUsbDevices(context.Background(), "pve1")
	if err != nil {
		t.Fatalf("ListNodeUsbDevices: %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("got %d devices, want 1", len(devices))
	}
	d := devices[0]
	if d.ProductID != "0x1234" || d.VendorID != "0x8087" || d.BusNum != 1 || d.Port != 1 || d.Speed != "480" {
		t.Fatalf("unexpected device: %+v", d)
	}
	if d.Product == nil || *d.Product != "Bluetooth" {
		t.Fatalf("product = %v, want Bluetooth", d.Product)
	}
}

// TestClient_GetNodeCapabilities_Merges covers the merged QEMU capability
// read across the cpu, machines and migration endpoints.
func TestClient_GetNodeCapabilities_Merges(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/nodes/pve1/capabilities/qemu/cpu":
			_, _ = io.WriteString(w, `{"data":[{"name":"kvm64","vendor":"QEMU","custom":false},{"name":"custom-x","vendor":"AuthenticAMD","custom":true}]}`)
		case "/nodes/pve1/capabilities/qemu/machines":
			_, _ = io.WriteString(w, `{"data":[{"id":"pc-i440fx-8.0","version":"8.0","type":"i440fx"},{"id":"pc-q35-8.0","version":"8.0","type":"q35","changes":"some changes"}]}`)
		case "/nodes/pve1/capabilities/qemu/migration":
			_, _ = io.WriteString(w, `{"data":{"has-dbus-vmstate":true}}`)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	})
	caps, err := c.GetNodeCapabilities(context.Background(), "pve1")
	if err != nil {
		t.Fatalf("GetNodeCapabilities: %v", err)
	}
	if len(caps.CPUModels) != 2 || caps.CPUModels[0].Name != "kvm64" || !caps.CPUModels[1].Custom {
		t.Fatalf("unexpected cpu models: %+v", caps.CPUModels)
	}
	if len(caps.Machines) != 2 || caps.Machines[1].Type == nil || *caps.Machines[1].Type != "q35" || caps.Machines[1].Changes == nil {
		t.Fatalf("unexpected machines: %+v", caps.Machines)
	}
	if !caps.MigrationFeatures.HasDbusVMState {
		t.Fatalf("unexpected migration features: %+v", caps.MigrationFeatures)
	}
}

// TestClient_NodeWakeonLAN_PostsAndDecodesMac covers the wake on LAN POST.
func TestClient_NodeWakeonLAN_PostsAndDecodesMac(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/nodes/pve1/wakeonlan" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":"aa:bb:cc:dd:ee:ff"}`)
	})
	mac, err := c.NodeWakeonLAN(context.Background(), "pve1")
	if err != nil {
		t.Fatalf("NodeWakeonLAN: %v", err)
	}
	if mac != "aa:bb:cc:dd:ee:ff" {
		t.Fatalf("mac = %q, want aa:bb:cc:dd:ee:ff", mac)
	}
}

// containsQuery reports whether the needle is one of the query string's
// key=value pairs.
func containsQuery(query, needle string) bool {
	for _, part := range splitQuery(query) {
		if part == needle {
			return true
		}
	}
	return false
}

// splitQuery splits a raw query string into its key=value pairs.
func splitQuery(query string) []string {
	out := []string{}
	current := ""
	for _, r := range query {
		if r == '&' {
			out = append(out, current)
			current = ""
			continue
		}
		current += string(r)
	}
	if current != "" {
		out = append(out, current)
	}
	return out
}
