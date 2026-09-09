// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"context"
	"io"
	"net/http"
	"testing"
)

// TestClient_ListQemuVMs_Full verifies the per-node VM index request shape
// with the full flag and decodes the pin's response field set, including
// hyphenated keys, pressure gauges, and the 0/1 integer encoding PVE emits
// for the template and serial booleans.
func TestClient_ListQemuVMs_Full(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/nodes/pve1/qemu" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if q := r.URL.Query().Get("full"); q != "1" {
			t.Fatalf("full query = %q, want 1", q)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[
			{"vmid":100,"name":"web","status":"running","template":1,"serial":0,
			 "cpu":0.03,"cpus":2,"mem":2147483648,"maxmem":4294967296,"memhost":3221225472,
			 "maxdisk":34359738368,"diskread":5368709120,"diskwrite":1073741824,
			 "netin":1024,"netout":2048,"uptime":3600,"pid":12345,
			 "qmpstatus":"running","running-machine":"pc-q35-9.2","running-qemu":"9.2.0",
			 "pressurecpufull":0.1,"pressurecpusome":0.2,"pressureiofull":0.3,
			 "pressureiosome":0.4,"pressurememoryfull":0.5,"pressurememorysome":0.6,
			 "tags":"prod;web","lock":"backup"}
		]}`)
	})
	vms, err := c.ListQemuVMs(context.Background(), "pve1", true)
	if err != nil {
		t.Fatalf("ListQemuVMs: %v", err)
	}
	if len(vms) != 1 {
		t.Fatalf("vms = %d, want 1", len(vms))
	}
	vm := vms[0]
	if vm.VMID != 100 || vm.Name != "web" || vm.Status != "running" {
		t.Fatalf("identity mismatch: %+v", vm)
	}
	if !vm.Template || vm.Serial {
		t.Fatalf("bool flags mismatch: template=%v serial=%v", vm.Template, vm.Serial)
	}
	if vm.CPU != 0.03 || vm.CPUs != 2 || vm.PressureCPUSome != 0.2 || vm.PressureMemorySome != 0.6 {
		t.Fatalf("number fields mismatch: %+v", vm)
	}
	if vm.Mem != 2147483648 || vm.MaxDisk != 34359738368 || vm.NetOut != 2048 || vm.Uptime != 3600 || vm.PID != 12345 {
		t.Fatalf("counter fields mismatch: %+v", vm)
	}
	if vm.QMPStatus != "running" || vm.RunningMachine != "pc-q35-9.2" || vm.RunningQEMU != "9.2.0" || vm.Tags != "prod;web" || vm.Lock != "backup" {
		t.Fatalf("status fields mismatch: %+v", vm)
	}
}

// TestClient_ListQemuVMs_NoFull confirms the call without the full flag
// sends no query string.
func TestClient_ListQemuVMs_NoFull(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/nodes/pve1/qemu" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.URL.RawQuery != "" {
			t.Fatalf("unexpected query: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"vmid":101,"status":"stopped"}]}`)
	})
	vms, err := c.ListQemuVMs(context.Background(), "pve1", false)
	if err != nil {
		t.Fatalf("ListQemuVMs: %v", err)
	}
	if len(vms) != 1 || vms[0].VMID != 101 || vms[0].Status != "stopped" {
		t.Fatalf("vms mismatch: %+v", vms)
	}
}
