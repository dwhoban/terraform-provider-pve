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

// TestClient_ListDomains decodes the GET /access/domains array, including
// the built-in pam and pve realms.
func TestClient_ListDomains(t *testing.T) {
	client := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/access/domains" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[
			{"realm":"pam","type":"pam","comment":"Linux PAM"},
			{"realm":"corp","type":"ldap","comment":"Corporate LDAP","tfa":"oath"}
		]}`)
	})
	domains, err := client.ListDomains(context.Background())
	if err != nil {
		t.Fatalf("ListDomains: %v", err)
	}
	if len(domains) != 2 {
		t.Fatalf("len(domains) = %d, want 2", len(domains))
	}
	if domains[0].Realm != "pam" || domains[0].Type != "pam" || domains[0].Comment != "Linux PAM" {
		t.Fatalf("domains[0] = %+v", domains[0])
	}
	if domains[1].TFA != "oath" {
		t.Fatalf("domains[1].TFA = %q, want oath", domains[1].TFA)
	}
}

// TestClient_CreateDomain_LDAP verifies the POST /access/domains wire body
// carries the fixed realm type and the LDAP field set.
func TestClient_CreateDomain_LDAP(t *testing.T) {
	var got map[string]any
	client := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/access/domains" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &got); err != nil {
			t.Errorf("decoding request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":null}`)
	})
	err := client.CreateDomain(context.Background(), Domain{
		Realm:         "corp",
		Type:          "ldap",
		Server1:       "ldap.example.com",
		Port:          intPtr(636),
		Mode:          "ldaps",
		BaseDN:        "dc=example,dc=com",
		UserAttr:      "uid",
		CaseSensitive: boolPtr(false),
	})
	if err != nil {
		t.Fatalf("CreateDomain: %v", err)
	}
	if got["realm"] != "corp" || got["type"] != "ldap" {
		t.Fatalf("body realm/type = %v/%v", got["realm"], got["type"])
	}
	if got["base_dn"] != "dc=example,dc=com" || got["user_attr"] != "uid" || got["mode"] != "ldaps" {
		t.Fatalf("body ldap fields = %v", got)
	}
	if port, ok := got["port"].(float64); !ok || port != 636 {
		t.Fatalf("body port = %v, want 636", got["port"])
	}
	if v, ok := got["case-sensitive"].(bool); !ok || v {
		t.Fatalf("body case-sensitive = %v, want false", got["case-sensitive"])
	}
}

// TestClient_GetDomain_BoolishDecode covers PVE section-config encoding:
// booleans may arrive as quoted "1"/"0" strings and the port as a string.
func TestClient_GetDomain_BoolishDecode(t *testing.T) {
	client := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/access/domains/corp" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{
			"realm":"corp","type":"ldap","server1":"ldap.example.com","port":"636",
			"mode":"ldaps","base_dn":"dc=example,dc=com","user_attr":"uid",
			"case-sensitive":"1","default":"0","digest":"abc123"
		}}`)
	})
	domain, err := client.GetDomain(context.Background(), "corp")
	if err != nil {
		t.Fatalf("GetDomain: %v", err)
	}
	if domain.Type != "ldap" || domain.BaseDN != "dc=example,dc=com" {
		t.Fatalf("domain = %+v", domain)
	}
	if domain.Port == nil || *domain.Port != 636 {
		t.Fatalf("Port = %v, want 636", domain.Port)
	}
	if domain.CaseSensitive == nil || !*domain.CaseSensitive {
		t.Fatalf("CaseSensitive = %v, want true", domain.CaseSensitive)
	}
	if domain.Default == nil || *domain.Default {
		t.Fatalf("Default = %v, want false", domain.Default)
	}
	if domain.Verify != nil {
		t.Fatalf("Verify = %v, want nil (absent)", domain.Verify)
	}
	if domain.Digest != "abc123" {
		t.Fatalf("Digest = %q, want abc123", domain.Digest)
	}
}

// TestClient_UpdateDomain_DeleteTranslated verifies the `delete` field and
// the changed settings travel in the PUT body.
func TestClient_UpdateDomain_DeleteTranslated(t *testing.T) {
	var got map[string]any
	client := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/access/domains/corp" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &got); err != nil {
			t.Errorf("decoding request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":null}`)
	})
	err := client.UpdateDomain(context.Background(), "corp", Domain{
		Realm:   "corp",
		Type:    "ldap",
		Server2: "ldap2.example.com",
		Delete:  "server2,secure",
	})
	if err != nil {
		t.Fatalf("UpdateDomain: %v", err)
	}
	if got["delete"] != "server2,secure" {
		t.Fatalf("body delete = %v, want server2,secure", got["delete"])
	}
	if got["server2"] != "ldap2.example.com" {
		t.Fatalf("body server2 = %v", got["server2"])
	}
}

// TestClient_DeleteDomain_404IsAPIError confirms a missing realm surfaces as
// *APIError so the resource layer can map it to a not-found outcome.
func TestClient_DeleteDomain_404IsAPIError(t *testing.T) {
	client := newFakePVETokenServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"errors":"no such realm"}`)
	})
	err := client.DeleteDomain(context.Background(), "gone")
	if err == nil {
		t.Fatal("DeleteDomain: expected error")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %T (%v), want *APIError", err, err)
	}
	if apiErr.StatusCode != 404 {
		t.Fatalf("status = %d, want 404", apiErr.StatusCode)
	}
}

// TestClient_SyncDomain_ReturnsUpid verifies POST /access/domains/{realm}/sync
// sends the sync parameters and decodes the worker task UPID.
func TestClient_SyncDomain_ReturnsUpid(t *testing.T) {
	const upid = "UPID:pve1:00001234:12345678:realm-sync:corp:root@pam:"
	var got map[string]any
	client := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/access/domains/corp/sync" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &got); err != nil {
			t.Errorf("decoding request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":"`+upid+`"}`)
	})
	result, err := client.SyncDomain(context.Background(), "corp", SyncDomainOptions{
		Scope:          "both",
		DryRun:         boolPtr(true),
		RemoveVanished: "entry;acl",
	})
	if err != nil {
		t.Fatalf("SyncDomain: %v", err)
	}
	if result != upid {
		t.Fatalf("SyncDomain = %q, want %q", result, upid)
	}
	if got["scope"] != "both" || got["remove-vanished"] != "entry;acl" {
		t.Fatalf("body = %v", got)
	}
	if v, ok := got["dry-run"].(bool); !ok || !v {
		t.Fatalf("body dry-run = %v, want true", got["dry-run"])
	}
}

// TestClient_ListRealmSyncJobs decodes GET /cluster/jobs/realm-sync with the
// int-or-boolean encodings PVE emits for enabled and the run timestamps.
func TestClient_ListRealmSyncJobs(t *testing.T) {
	client := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/cluster/jobs/realm-sync" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{
			"id":"corp-daily","realm":"corp","schedule":"mon..fri 02:30","enabled":1,
			"scope":"both","remove-vanished":"entry","enable-new":"1",
			"last-run":1720000000,"next-run":1720100000,"comment":"nightly"
		}]}`)
	})
	jobs, err := client.ListRealmSyncJobs(context.Background())
	if err != nil {
		t.Fatalf("ListRealmSyncJobs: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1", len(jobs))
	}
	job := jobs[0]
	if job.ID != "corp-daily" || job.Realm != "corp" || job.Schedule != "mon..fri 02:30" {
		t.Fatalf("job = %+v", job)
	}
	if job.Enabled == nil || !*job.Enabled || job.EnableNew == nil || !*job.EnableNew {
		t.Fatalf("enabled/enable-new = %v/%v, want true/true", job.Enabled, job.EnableNew)
	}
	if job.LastRun == nil || *job.LastRun != 1720000000 {
		t.Fatalf("LastRun = %v", job.LastRun)
	}
	if job.NextRun == nil || *job.NextRun != 1720100000 {
		t.Fatalf("NextRun = %v", job.NextRun)
	}
}

// TestClient_RealmSyncJobLifecycle verifies the create/update/delete wire
// shapes for /cluster/jobs/realm-sync.
func TestClient_RealmSyncJobLifecycle(t *testing.T) {
	var bodies []map[string]any
	var calls []string
	client := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var m map[string]any
		_ = json.Unmarshal(body, &m)
		bodies = append(bodies, m)
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/cluster/jobs/realm-sync":
			calls = append(calls, "create")
		case r.Method == http.MethodPut && r.URL.Path == "/cluster/jobs/realm-sync/corp-daily":
			calls = append(calls, "update")
		case r.Method == http.MethodDelete && r.URL.Path == "/cluster/jobs/realm-sync/corp-daily":
			calls = append(calls, "delete")
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":null}`)
	})
	ctx := context.Background()
	if err := client.CreateRealmSyncJob(ctx, RealmSyncJob{
		ID: "corp-daily", Realm: "corp", Schedule: "mon..fri 02:30", Scope: "both",
	}); err != nil {
		t.Fatalf("CreateRealmSyncJob: %v", err)
	}
	if err := client.UpdateRealmSyncJob(ctx, "corp-daily", RealmSyncJob{
		ID: "corp-daily", Schedule: "sat 03:00", Delete: "scope",
	}); err != nil {
		t.Fatalf("UpdateRealmSyncJob: %v", err)
	}
	if err := client.DeleteRealmSyncJob(ctx, "corp-daily"); err != nil {
		t.Fatalf("DeleteRealmSyncJob: %v", err)
	}
	if strings.Join(calls, ",") != "create,update,delete" {
		t.Fatalf("calls = %v", calls)
	}
	if bodies[0]["id"] != "corp-daily" || bodies[0]["realm"] != "corp" || bodies[0]["schedule"] != "mon..fri 02:30" {
		t.Fatalf("create body = %v", bodies[0])
	}
	if bodies[1]["schedule"] != "sat 03:00" || bodies[1]["delete"] != "scope" {
		t.Fatalf("update body = %v", bodies[1])
	}
}

// TestClient_AnalyzeJobSchedule verifies the schedule validation helper's
// query encoding and result decode.
func TestClient_AnalyzeJobSchedule(t *testing.T) {
	client := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/cluster/jobs/schedule-analyze" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if q := r.URL.Query(); q.Get("schedule") != "mon..fri 02:30" || q.Get("iterations") != "1" {
			t.Errorf("query = %v", q)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"timestamp":1720000000,"utc":"2024-07-03T02:30:00Z"}]}`)
	})
	runs, err := client.AnalyzeJobSchedule(context.Background(), "mon..fri 02:30", 1, 0)
	if err != nil {
		t.Fatalf("AnalyzeJobSchedule: %v", err)
	}
	if len(runs) != 1 || runs[0].Timestamp != 1720000000 || runs[0].UTC != "2024-07-03T02:30:00Z" {
		t.Fatalf("runs = %+v", runs)
	}
}

// TestClient_GetRealmSyncJob decodes the single-job read with lenient
// boolean and timestamp encodings.
func TestClient_GetRealmSyncJob(t *testing.T) {
	client := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/cluster/jobs/realm-sync/corp-daily" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{
			"id":"corp-daily","realm":"corp","schedule":"mon..fri 02:30","enabled":"1",
			"scope":"both","remove-vanished":"entry","enable-new":0,"next-run":"1720100000"
		}}`)
	})
	job, err := client.GetRealmSyncJob(context.Background(), "corp-daily")
	if err != nil {
		t.Fatalf("GetRealmSyncJob: %v", err)
	}
	if job.ID != "corp-daily" || job.Realm != "corp" || job.Schedule != "mon..fri 02:30" {
		t.Fatalf("job = %+v", job)
	}
	if job.Enabled == nil || !*job.Enabled {
		t.Fatalf("Enabled = %v, want true", job.Enabled)
	}
	if job.EnableNew == nil || *job.EnableNew {
		t.Fatalf("EnableNew = %v, want false", job.EnableNew)
	}
	if job.NextRun == nil || *job.NextRun != 1720100000 {
		t.Fatalf("NextRun = %v, want 1720100000", job.NextRun)
	}
	if job.LastRun != nil {
		t.Fatalf("LastRun = %v, want nil (absent)", job.LastRun)
	}
}

// intPtr is a test helper for building expected *int values.
func intPtr(i int) *int { return &i }
