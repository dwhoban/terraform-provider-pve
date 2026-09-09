// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

// TestClient_ListLxcContainers_DecodesRow verifies GET /nodes/{node}/lxc
// sends no query parameters (the pin defines none for this endpoint) and
// decodes a full index row, including the boolish template flag PVE emits
// as 0/1 or true/false depending on version.
func TestClient_ListLxcContainers_DecodesRow(t *testing.T) {
	var gotPath, gotRawQuery string
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotRawQuery = r.URL.RawQuery
		if r.Method != http.MethodGet {
			t.Fatalf("method = %q, want GET", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[`+
			`{"vmid":100,"name":"web01","status":"running","template":0,`+
			`"cpu":0.0125,"cpus":2,"mem":1073741824,"maxmem":4294967296,`+
			`"swap":0,"maxswap":536870912,"disk":1048576,"maxdisk":8589934592,`+
			`"diskread":2048,"diskwrite":4096,"netin":8192,"netout":16384,`+
			`"uptime":3600,"lock":"backup","tags":"prod;web",`+
			`"pressurecpusome":0.1,"pressureiosome":0.2,"pressureiofull":0.3,`+
			`"pressurememorysome":0.4,"pressurememoryfull":0.5},`+
			`{"vmid":101,"name":"tmpl","status":"stopped","template":true}]}`)
	})
	containers, err := c.ListLxcContainers(context.Background(), "pve1")
	if err != nil {
		t.Fatalf("ListLxcContainers: %v", err)
	}
	if gotPath != "/nodes/pve1/lxc" {
		t.Fatalf("path = %q, want /nodes/pve1/lxc", gotPath)
	}
	if gotRawQuery != "" {
		t.Fatalf("raw query = %q, want none (pin defines no query parameters)", gotRawQuery)
	}
	if len(containers) != 2 {
		t.Fatalf("containers = %+v, want two rows", containers)
	}
	ct := containers[0]
	if ct.VMID != 100 || ct.Status != "running" {
		t.Fatalf("vmid/status = %d/%q, want 100/running", ct.VMID, ct.Status)
	}
	if ct.Name == nil || *ct.Name != "web01" {
		t.Fatalf("name = %v, want web01", ct.Name)
	}
	if ct.Template == nil || *ct.Template {
		t.Fatalf("template = %v, want false", ct.Template)
	}
	if ct.CPU == nil || *ct.CPU != 0.0125 {
		t.Fatalf("cpu = %v, want 0.0125", ct.CPU)
	}
	if ct.CPUs == nil || *ct.CPUs != 2 {
		t.Fatalf("cpus = %v, want 2", ct.CPUs)
	}
	if ct.Mem == nil || *ct.Mem != 1073741824 || ct.MaxMem == nil || *ct.MaxMem != 4294967296 {
		t.Fatalf("mem/maxmem = %v/%v, want populated", ct.Mem, ct.MaxMem)
	}
	if ct.MaxSwap == nil || *ct.MaxSwap != 536870912 {
		t.Fatalf("maxswap = %v, want 536870912", ct.MaxSwap)
	}
	if ct.Disk == nil || *ct.Disk != 1048576 || ct.MaxDisk == nil || *ct.MaxDisk != 8589934592 {
		t.Fatalf("disk/maxdisk = %v/%v, want populated", ct.Disk, ct.MaxDisk)
	}
	if ct.DiskRead == nil || *ct.DiskRead != 2048 || ct.DiskWrite == nil || *ct.DiskWrite != 4096 {
		t.Fatalf("diskread/diskwrite = %v/%v, want populated", ct.DiskRead, ct.DiskWrite)
	}
	if ct.NetIn == nil || *ct.NetIn != 8192 || ct.NetOut == nil || *ct.NetOut != 16384 {
		t.Fatalf("netin/netout = %v/%v, want populated", ct.NetIn, ct.NetOut)
	}
	if ct.Uptime == nil || *ct.Uptime != 3600 {
		t.Fatalf("uptime = %v, want 3600", ct.Uptime)
	}
	if ct.Lock == nil || *ct.Lock != "backup" {
		t.Fatalf("lock = %v, want backup", ct.Lock)
	}
	if ct.Tags == nil || *ct.Tags != "prod;web" {
		t.Fatalf("tags = %v, want prod;web", ct.Tags)
	}
	if ct.PressureCPUSome == nil || *ct.PressureCPUSome != 0.1 ||
		ct.PressureIOSome == nil || *ct.PressureIOSome != 0.2 ||
		ct.PressureIOFull == nil || *ct.PressureIOFull != 0.3 ||
		ct.PressureMemorySome == nil || *ct.PressureMemorySome != 0.4 ||
		ct.PressureMemoryFull == nil || *ct.PressureMemoryFull != 0.5 {
		t.Fatalf("pressure fields = %v/%v/%v/%v/%v, want populated",
			ct.PressureCPUSome, ct.PressureIOSome, ct.PressureIOFull,
			ct.PressureMemorySome, ct.PressureMemoryFull)
	}
	ct2 := containers[1]
	if ct2.Template == nil || !*ct2.Template {
		t.Fatalf("template = %v, want true (boolean encoding)", ct2.Template)
	}
	if ct2.CPU != nil || ct2.Mem != nil || ct2.Uptime != nil {
		t.Fatalf("absent optional fields = %v/%v/%v, want nil", ct2.CPU, ct2.Mem, ct2.Uptime)
	}
}

// TestClient_ListLxcContainers_MinimalRow verifies a bare index row leaves
// every optional field nil and the error path wraps the failure.
func TestClient_ListLxcContainers_MinimalRow(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"vmid":102,"status":"stopped"}]}`)
	})
	containers, err := c.ListLxcContainers(context.Background(), "pve1")
	if err != nil {
		t.Fatalf("ListLxcContainers: %v", err)
	}
	if len(containers) != 1 {
		t.Fatalf("containers = %+v, want one row", containers)
	}
	ct := containers[0]
	if ct.VMID != 102 || ct.Status != "stopped" {
		t.Fatalf("vmid/status = %d/%q, want 102/stopped", ct.VMID, ct.Status)
	}
	if ct.Name != nil || ct.Template != nil || ct.Tags != nil || ct.MaxSwap != nil {
		t.Fatalf("optional fields = %v/%v/%v/%v, want nil", ct.Name, ct.Template, ct.Tags, ct.MaxSwap)
	}
}

// TestClient_ListLxcContainers_ErrorWrapsContext verifies API failures are
// wrapped with the operation and path context.
func TestClient_ListLxcContainers_ErrorWrapsContext(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `{"errors":"boom"}`)
	})
	_, err := c.ListLxcContainers(context.Background(), "pve1")
	if err == nil {
		t.Fatal("ListLxcContainers error = nil, want wrapped failure")
	}
	if !strings.Contains(err.Error(), "list lxc containers") || !strings.Contains(err.Error(), "/nodes/pve1/lxc") {
		t.Fatalf("error = %v, want operation and path context", err)
	}
}
