// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

// TestClusterReplication_JobCRUD covers the replication job wire set: the
// create body (pin requires id, target, and the fixed section type), list
// and single-read decode with the 0/1 boolish disable flag, the update body
// carrying the PVE delete parameter, and delete.
func TestClusterReplication_JobCRUD(t *testing.T) {
	var lastBody []byte
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		lastBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/cluster/replication":
			_, _ = io.WriteString(w, `{"data":null}`)
		case r.Method == http.MethodGet && r.URL.Path == "/cluster/replication":
			_, _ = io.WriteString(w, `{"data":[`+
				`{"id":"100-0","type":"local","target":"pve2","guest":100,"jobnum":0,"schedule":"*/15"},`+
				`{"id":"101-0","type":"local","target":"pve3","guest":101,"jobnum":0,"schedule":"*/30","disable":1}]}`)
		case r.Method == http.MethodGet && r.URL.Path == "/cluster/replication/100-0":
			_, _ = io.WriteString(w, `{"data":{"id":"100-0","type":"local","target":"pve2","guest":100,`+
				`"jobnum":0,"schedule":"*/15","rate":50.5,"disable":1,"comment":"dr site","digest":"abc123"}}`)
		case r.Method == http.MethodPut && r.URL.Path == "/cluster/replication/100-0":
			_, _ = io.WriteString(w, `{"data":null}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/cluster/replication/100-0":
			_, _ = io.WriteString(w, `{"data":null}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	ctx := context.Background()

	if err := c.CreateReplication(ctx, ReplicationJob{
		ID: "100-0", Target: "pve2", Schedule: "*/15", Rate: replicationRate(t), Comment: "dr site", Disable: true,
	}); err != nil {
		t.Fatalf("CreateReplication: %v", err)
	}
	var sent map[string]any
	if err := json.Unmarshal(lastBody, &sent); err != nil {
		t.Fatalf("create body %q is not JSON: %v", lastBody, err)
	}
	if sent["id"] != "100-0" || sent["target"] != "pve2" || sent["type"] != "local" {
		t.Fatalf("create body must carry id, target, and the fixed local type, got %v", sent)
	}
	if sent["schedule"] != "*/15" || sent["comment"] != "dr site" || sent["disable"] != true {
		t.Fatalf("create body = %v", sent)
	}
	if _, ok := sent["rate"]; !ok {
		t.Fatalf("create body must carry rate, got %v", sent)
	}

	list, err := c.ListReplications(ctx)
	if err != nil {
		t.Fatalf("ListReplications: %v", err)
	}
	if len(list) != 2 || list[0].ID != "100-0" || list[0].Target != "pve2" || list[0].Guest != 100 {
		t.Fatalf("list = %+v", list)
	}
	if !list[1].Disable {
		t.Fatalf("boolish disable:1 must decode true, got %+v", list[1])
	}

	job, err := c.GetReplication(ctx, "100-0")
	if err != nil {
		t.Fatalf("GetReplication: %v", err)
	}
	if job.Target != "pve2" || job.Schedule != "*/15" || job.Comment != "dr site" || job.Digest != "abc123" {
		t.Fatalf("read job = %+v", job)
	}
	if !job.Disable || job.Rate == nil || *job.Rate != 50.5 || job.Guest != 100 || job.JobNum != 0 {
		t.Fatalf("read job = %+v", job)
	}

	if err := c.UpdateReplication(ctx, "100-0", ReplicationJobUpdate{
		Schedule: "*/30",
		Delete:   []string{"rate", "comment"},
	}); err != nil {
		t.Fatalf("UpdateReplication: %v", err)
	}
	sent = nil
	if err := json.Unmarshal(lastBody, &sent); err != nil {
		t.Fatalf("update body %q is not JSON: %v", lastBody, err)
	}
	if sent["id"] != "100-0" || sent["schedule"] != "*/30" || sent["delete"] != "rate,comment" {
		t.Fatalf("update body = %v", sent)
	}
	if _, ok := sent["comment"]; ok {
		t.Fatalf("cleared settings must travel via delete, not an empty value: %v", sent)
	}

	// A set disable flag must travel as true; an unset one must be omitted.
	if err := c.UpdateReplication(ctx, "100-0", ReplicationJobUpdate{Disable: true}); err != nil {
		t.Fatalf("UpdateReplication: %v", err)
	}
	sent = nil
	if err := json.Unmarshal(lastBody, &sent); err != nil {
		t.Fatalf("update body %q is not JSON: %v", lastBody, err)
	}
	if sent["disable"] != true {
		t.Fatalf("update body = %v, want disable:true", sent)
	}

	if err := c.DeleteReplication(ctx, "100-0"); err != nil {
		t.Fatalf("DeleteReplication: %v", err)
	}
}

// TestClusterReplication_404IsAPIError confirms a missing job surfaces as a
// 404 *APIError so the resource layer can treat already-absent as success.
func TestClusterReplication_404IsAPIError(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/cluster/replication/199-0" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"errors":"no such replication job"}`)
	})
	_, err := c.GetReplication(context.Background(), "199-0")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 APIError, got %v", err)
	}
}

// TestNodeReplications_List covers GET /nodes/{node}/replication: the
// optional guest filter travels as a query parameter and the status rows
// decode, including null rate and the 0/1 boolish disable and removal flags.
func TestNodeReplications_List(t *testing.T) {
	var lastURI string
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		lastURI = r.RequestURI
		if r.Method != http.MethodGet || r.URL.Path != "/nodes/pve1/replication" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"id":"100-0","type":"local","target":"pve2","guest":100,`+
			`"guest_name":"vm100","jobnum":0,"schedule":"*/15","rate":null,"disable":0,"removal":0,`+
			`"last_sync":1700000000,"last_try":1700000000,"next_sync":1700000090,"fail_count":0,`+
			`"duration":12,"status":"idle"},`+
			`{"id":"101-0","type":"local","target":"pve3","guest":101,"guest_name":"ct101","jobnum":0,`+
			`"schedule":"*/30","rate":null,"disable":1,"removal":1,"last_sync":1700000000,`+
			`"last_try":1700000000,"next_sync":1700000180,"fail_count":3,"duration":44,`+
			`"status":"error","error":"sync failed"}]}`)
	})
	ctx := context.Background()

	rows, err := c.ListNodeReplications(ctx, "pve1", 100)
	if err != nil {
		t.Fatalf("ListNodeReplications: %v", err)
	}
	if !strings.Contains(lastURI, "guest=100") {
		t.Fatalf("URI = %q, want guest filter", lastURI)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %+v", rows)
	}
	first := rows[0]
	if first.ID != "100-0" || first.Target != "pve2" || first.GuestName != "vm100" || first.Status != "idle" {
		t.Fatalf("first row = %+v", first)
	}
	if first.Disable || first.Removal || first.Rate != nil || first.FailCount != 0 || first.Duration != 12 {
		t.Fatalf("first row = %+v", first)
	}
	if first.LastSync != 1700000000 || first.NextSync != 1700000090 {
		t.Fatalf("first row = %+v", first)
	}
	second := rows[1]
	if !second.Disable || !second.Removal || second.FailCount != 3 || second.Status != "error" || second.Error != "sync failed" {
		t.Fatalf("second row = %+v", second)
	}

	if _, err := c.ListNodeReplications(ctx, "pve1", 0); err != nil {
		t.Fatalf("ListNodeReplications unfiltered: %v", err)
	}
	if strings.Contains(lastURI, "guest=") {
		t.Fatalf("URI = %q, want no guest filter when unset", lastURI)
	}
}

// TestNodeReplication_Log covers GET /nodes/{node}/replication/{id}/log:
// the numbered lines decode into their texts in order.
func TestNodeReplication_Log(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/nodes/pve1/replication/100-0/log" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"n":0,"t":"start replication job"},{"n":1,"t":"end replication job"}]}`)
	})
	lines, err := c.GetReplicationLog(context.Background(), "pve1", "100-0")
	if err != nil {
		t.Fatalf("GetReplicationLog: %v", err)
	}
	if len(lines) != 2 || lines[0] != "start replication job" || lines[1] != "end replication job" {
		t.Fatalf("lines = %v", lines)
	}
}

// TestReplication_ScheduleNow covers POST /nodes/{node}/replication/{id}/schedule_now:
// the returned task UPID travels through to the caller.
func TestReplication_ScheduleNow(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/nodes/pve1/replication/100-0/schedule_now" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":"UPID:pve1:0000ABCD:00001234:00000000:vzreplication:100-0:root@pam:"}`)
	})
	upid, err := c.ScheduleReplicationNow(context.Background(), "pve1", "100-0")
	if err != nil {
		t.Fatalf("ScheduleReplicationNow: %v", err)
	}
	if !strings.HasPrefix(upid, "UPID:pve1:") {
		t.Fatalf("upid = %q", upid)
	}
}

// replicationRate returns a rate pointer for wire-body assertions.
func replicationRate(t *testing.T) *float64 {
	t.Helper()
	rate := 50.5
	return &rate
}
