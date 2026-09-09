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

// TestClient_GetClusterResources_TypeFilter verifies the optional type
// filter is sent as a query parameter and the row payload decodes,
// including hyphenated JSON keys and boolean fields.
func TestClient_GetClusterResources_TypeFilter(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/cluster/resources" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if q := r.URL.Query().Get("type"); q != "vm" {
			t.Fatalf("type query = %q, want vm", q)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[
			{"id":"qemu/100","type":"qemu","node":"pve1","vmid":100,"name":"web","status":"running",
			 "disk":5368709120,"maxdisk":34359738368,"uptime":3600,"tags":"prod","pool":"web",
			 "hastate":"started","memhost":3221225472,"netin":1024,"netout":2048,"lock":"backup","shared":true}
		]}`)
	})
	rows, err := c.GetClusterResources(context.Background(), "vm")
	if err != nil {
		t.Fatalf("GetClusterResources: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	r := rows[0]
	if r.ID != "qemu/100" || r.Type != "qemu" || r.Node != "pve1" || r.VMID != 100 || r.Name != "web" {
		t.Fatalf("row identity mismatch: %+v", r)
	}
	if r.Template || !r.Shared {
		t.Fatalf("bool flags mismatch: template=%v shared=%v", r.Template, r.Shared)
	}
	if r.Tags != "prod" || r.Pool != "web" || r.HAState != "started" || r.MemHost != 3221225472 {
		t.Fatalf("optional strings mismatch: %+v", r)
	}
}

// TestClient_GetClusterResources_NoFilter confirms the unfiltered call sends
// an empty query string.
func TestClient_GetClusterResources_NoFilter(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/cluster/resources" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.URL.RawQuery != "" {
			t.Fatalf("unexpected query: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"id":"node/pve1","type":"node","node":"pve1","host-arch":"aarch64","cgroup-mode":2,"status":"online"}]}`)
	})
	rows, err := c.GetClusterResources(context.Background(), "")
	if err != nil {
		t.Fatalf("GetClusterResources: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != "node/pve1" || rows[0].HostArch != "aarch64" || rows[0].CgroupMode != 2 {
		t.Fatalf("rows mismatch: %+v", rows)
	}
}

// TestClient_GetClusterStatus_Decodes covers both the cluster summary entry
// and the per-node entries returned by GET /cluster/status.
func TestClient_GetClusterStatus_Decodes(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/cluster/status" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[
			{"id":"cluster","name":"prod","type":"cluster","nodes":3,"quorate":1,"version":5},
			{"id":"node/pve1","name":"pve1","type":"node","ip":"10.0.0.11","level":"c",
			 "local":1,"nodeid":1,"online":1}
		]}`)
	})
	entries, err := c.GetClusterStatus(context.Background())
	if err != nil {
		t.Fatalf("GetClusterStatus: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(entries))
	}
	cluster, node := entries[0], entries[1]
	if cluster.Type != "cluster" || cluster.Name != "prod" || cluster.Nodes != 3 || !cluster.Quorate || cluster.Version != 5 {
		t.Fatalf("cluster entry mismatch: %+v", cluster)
	}
	if node.Type != "node" || node.IP != "10.0.0.11" || !node.Local || node.NodeID != 1 || !node.Online || node.Level != "c" {
		t.Fatalf("node entry mismatch: %+v", node)
	}
}

// TestClient_GetClusterTotem_Decodes verifies totem settings decode with
// strings unquoted and non-string scalars kept as their literal token.
func TestClient_GetClusterTotem_Decodes(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/cluster/config/totem" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"cluster_name":"prod","config_version":"5","ip_version":4}}`)
	})
	totem, err := c.GetClusterTotem(context.Background())
	if err != nil {
		t.Fatalf("GetClusterTotem: %v", err)
	}
	if totem["cluster_name"] != "prod" || totem["config_version"] != "5" || totem["ip_version"] != "4" {
		t.Fatalf("totem mismatch: %+v", totem)
	}
}

// TestClient_GetClusterQDevice_Decodes verifies the generic qdevice object
// decodes into string values.
func TestClient_GetClusterQDevice_Decodes(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/cluster/config/qdevice" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"qdevice":{"index":0,"state":"OFF","tie_breaker":"0"}}}`)
	})
	qdevice, err := c.GetClusterQDevice(context.Background())
	if err != nil {
		t.Fatalf("GetClusterQDevice: %v", err)
	}
	if !strings.Contains(qdevice["qdevice"], "OFF") {
		t.Fatalf("qdevice mismatch: %+v", qdevice)
	}
}

// TestClient_ListClusterTasks_Decodes verifies the cluster-wide task list
// decodes start/end timestamps and identity fields.
func TestClient_ListClusterTasks_Decodes(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/cluster/tasks" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.URL.RawQuery != "" {
			t.Fatalf("unexpected query: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[
			{"upid":"UPID:pve1:0000ABCD:12345678:VZDUMP:root@pam:vzdump100:","node":"pve1",
			 "type":"vzdump","user":"root@pam","status":"OK","starttime":1700000000,"endtime":1700000600,
			 "id":"100","pid":1234,"pstart":987654321}
		]}`)
	})
	tasks, err := c.ListClusterTasks(context.Background())
	if err != nil {
		t.Fatalf("ListClusterTasks: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("tasks = %d, want 1", len(tasks))
	}
	task := tasks[0]
	if task.UPID != "UPID:pve1:0000ABCD:12345678:VZDUMP:root@pam:vzdump100:" ||
		task.Node != "pve1" || task.Type != "vzdump" || task.User != "root@pam" ||
		task.Status != "OK" || task.StartTime != 1700000000 || task.EndTime != 1700000600 ||
		task.ID != "100" || task.PID != 1234 || task.PStart != 987654321 {
		t.Fatalf("task mismatch: %+v", task)
	}
}

// TestClient_GetNextID_Decodes verifies the next free VMID decodes from the
// integer envelope.
func TestClient_GetNextID_Decodes(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/cluster/nextid" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.URL.RawQuery != "" {
			t.Fatalf("unexpected query: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":105}`)
	})
	id, err := c.GetNextID(context.Background())
	if err != nil {
		t.Fatalf("GetNextID: %v", err)
	}
	if id != 105 {
		t.Fatalf("id = %d, want 105", id)
	}
}

// TestClient_GetVersion_Decodes verifies the version payload decodes all
// documented fields.
func TestClient_GetVersion_Decodes(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/version" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"release":"9.0","repoid":"abcdef123456","version":"9.0.3","console":"html5"}}`)
	})
	version, err := c.GetVersion(context.Background())
	if err != nil {
		t.Fatalf("GetVersion: %v", err)
	}
	if version.Release != "9.0" || version.RepoID != "abcdef123456" || version.Version != "9.0.3" || version.Console != "html5" {
		t.Fatalf("version mismatch: %+v", version)
	}
}
