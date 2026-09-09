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

// TestClient_GetClusterOptions_Decodes covers GET /cluster/options: scalar
// fields decode directly and property-string fields (ha, bwlimit, migration,
// next-id, crs, tag-style, webauthn, location) decode into typed structs.
func TestClient_GetClusterOptions_Decodes(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/cluster/options" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{`+
			`"email_from":"ci@example.com",`+
			`"console":"xtermjs",`+
			`"max_workers":4,`+
			`"migration_unsecure":1,`+
			`"ha":"shutdown_policy=migrate",`+
			`"bwlimit":"move=100,restore=200.5",`+
			`"migration":"type=insecure,network=10.0.0.1/24",`+
			`"next-id":"lower=200,upper=5000",`+
			`"crs":"ha=static,ha-rebalance-on-start=1",`+
			`"tag-style":"case-sensitive=1,ordering=config,shape=dense",`+
			`"webauthn":"allow-subdomains=0,origin=https://pve.example.com",`+
			`"location":"latitude=52.1,longitude=13.4,name=lab",`+
			`"description":"prod datacenter"}}`)
	})
	opts, err := c.GetClusterOptions(context.Background())
	if err != nil {
		t.Fatalf("GetClusterOptions: %v", err)
	}
	if opts.EmailFrom == nil || *opts.EmailFrom != "ci@example.com" {
		t.Fatalf("email_from = %v", opts.EmailFrom)
	}
	if opts.Console == nil || *opts.Console != "xtermjs" {
		t.Fatalf("console = %v", opts.Console)
	}
	if opts.MaxWorkers == nil || *opts.MaxWorkers != 4 {
		t.Fatalf("max_workers = %v", opts.MaxWorkers)
	}
	if opts.MigrationUnsecure == nil || !*opts.MigrationUnsecure {
		t.Fatalf("migration_unsecure = %v, want boolish true", opts.MigrationUnsecure)
	}
	if opts.Description == nil || *opts.Description != "prod datacenter" {
		t.Fatalf("description = %v", opts.Description)
	}
	if opts.HA == nil || opts.HA.ShutdownPolicy == nil || *opts.HA.ShutdownPolicy != "migrate" {
		t.Fatalf("ha = %+v", opts.HA)
	}
	if opts.BWLimit == nil || opts.BWLimit.Move == nil || *opts.BWLimit.Move != 100 ||
		opts.BWLimit.Restore == nil || *opts.BWLimit.Restore != 200.5 || opts.BWLimit.Clone != nil {
		t.Fatalf("bwlimit = %+v", opts.BWLimit)
	}
	if opts.Migration == nil || opts.Migration.Type == nil || *opts.Migration.Type != "insecure" ||
		opts.Migration.Network == nil || *opts.Migration.Network != "10.0.0.1/24" {
		t.Fatalf("migration = %+v", opts.Migration)
	}
	if opts.NextID == nil || opts.NextID.Lower == nil || *opts.NextID.Lower != 200 ||
		opts.NextID.Upper == nil || *opts.NextID.Upper != 5000 {
		t.Fatalf("next-id = %+v", opts.NextID)
	}
	if opts.CRS == nil || opts.CRS.Ha == nil || *opts.CRS.Ha != "static" ||
		opts.CRS.HaRebalanceOnStart == nil || !*opts.CRS.HaRebalanceOnStart {
		t.Fatalf("crs = %+v", opts.CRS)
	}
	if opts.TagStyle == nil || opts.TagStyle.CaseSensitive == nil || !*opts.TagStyle.CaseSensitive ||
		opts.TagStyle.Ordering == nil || *opts.TagStyle.Ordering != "config" ||
		opts.TagStyle.Shape == nil || *opts.TagStyle.Shape != "dense" {
		t.Fatalf("tag-style = %+v", opts.TagStyle)
	}
	if opts.WebAuthn == nil || opts.WebAuthn.AllowSubdomains == nil || *opts.WebAuthn.AllowSubdomains ||
		opts.WebAuthn.Origin == nil || *opts.WebAuthn.Origin != "https://pve.example.com" {
		t.Fatalf("webauthn = %+v", opts.WebAuthn)
	}
	if opts.Location == nil || opts.Location.Latitude == nil || *opts.Location.Latitude != 52.1 ||
		opts.Location.Longitude == nil || *opts.Location.Longitude != 13.4 ||
		opts.Location.Name == nil || *opts.Location.Name != "lab" {
		t.Fatalf("location = %+v", opts.Location)
	}
	// Absent options decode as nil, not zero values.
	if opts.Notify != nil || opts.U2F != nil || opts.UserTagAccess != nil || opts.Replication != nil {
		t.Fatalf("absent options should be nil: %+v", opts)
	}
}

// TestClient_GetClusterOptions_MalformedPropertyString verifies a malformed
// property string surfaces an error naming the offending option.
func TestClient_GetClusterOptions_MalformedPropertyString(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"ha":"garbage"}}`)
	})
	_, err := c.GetClusterOptions(context.Background())
	if err == nil {
		t.Fatal("expected error for malformed property string")
	}
	if !strings.Contains(err.Error(), "ha") {
		t.Fatalf("error should name the option: %v", err)
	}
}

// TestClient_UpdateClusterOptions_BodyShape asserts the PUT body renders
// scalars as typed JSON and property-string options as key=value strings.
func TestClient_UpdateClusterOptions_BodyShape(t *testing.T) {
	var captured map[string]any
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/cluster/options" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("reading body: %v", err)
		}
		if err := json.Unmarshal(raw, &captured); err != nil {
			t.Fatalf("decoding body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":null}`)
	})
	opts := ClusterOptions{
		EmailFrom:         clusterOptionsPtr("ci@example.com"),
		Console:           clusterOptionsPtr("xtermjs"),
		MaxWorkers:        clusterOptionsPtr(4),
		MigrationUnsecure: clusterOptionsPtr(true),
		HA:                &ClusterOptionsHA{ShutdownPolicy: clusterOptionsPtr("migrate")},
		BWLimit:           &ClusterOptionsBWLimit{Move: clusterOptionsPtr(100.0), Restore: clusterOptionsPtr(200.5)},
		Migration:         &ClusterOptionsMigration{Type: clusterOptionsPtr("insecure"), Network: clusterOptionsPtr("10.0.0.1/24")},
		NextID:            &ClusterOptionsNextID{Lower: clusterOptionsPtr(200), Upper: clusterOptionsPtr(5000)},
		CRS:               &ClusterOptionsCRS{Ha: clusterOptionsPtr("static"), HaRebalanceOnStart: clusterOptionsPtr(true)},
		WebAuthn:          &ClusterOptionsWebAuthn{AllowSubdomains: clusterOptionsPtr(false), Origin: clusterOptionsPtr("https://pve.example.com")},
		Location:          &ClusterOptionsLocation{Latitude: clusterOptionsPtr(52.1), Longitude: clusterOptionsPtr(13.4), Name: clusterOptionsPtr("lab")},
	}
	if err := c.UpdateClusterOptions(context.Background(), opts, nil); err != nil {
		t.Fatalf("UpdateClusterOptions: %v", err)
	}
	wantStr := map[string]string{
		"email_from": "ci@example.com",
		"console":    "xtermjs",
		"ha":         "shutdown_policy=migrate",
		"bwlimit":    "move=100,restore=200.5",
		"migration":  "type=insecure,network=10.0.0.1/24",
		"next-id":    "lower=200,upper=5000",
		"crs":        "ha=static,ha-rebalance-on-start=1",
		"webauthn":   "allow-subdomains=0,origin=https://pve.example.com",
		"location":   "latitude=52.1,longitude=13.4,name=lab",
	}
	for key, want := range wantStr {
		if got, _ := captured[key].(string); got != want {
			t.Fatalf("body[%q] = %q, want %q", key, got, want)
		}
	}
	if got, _ := captured["max_workers"].(float64); got != 4 {
		t.Fatalf("body[max_workers] = %v, want 4", captured["max_workers"])
	}
	if got, _ := captured["migration_unsecure"].(bool); !got {
		t.Fatalf("body[migration_unsecure] = %v, want true", captured["migration_unsecure"])
	}
	if _, ok := captured["notify"]; ok {
		t.Fatal("nil options must not appear in the body")
	}
	if _, ok := captured["delete"]; ok {
		t.Fatal("delete must travel as a query parameter, not in the body")
	}
}

// TestClient_UpdateClusterOptions_DeleteTranslatedToQuery verifies cleared
// fields travel in the PVE `delete` query parameter, comma-separated.
func TestClient_UpdateClusterOptions_DeleteTranslatedToQuery(t *testing.T) {
	var sawQuery string
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Fatalf("expected PUT, got %s", r.Method)
		}
		sawQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":null}`)
	})
	if err := c.UpdateClusterOptions(context.Background(), ClusterOptions{}, []string{"email_from", "console"}); err != nil {
		t.Fatalf("UpdateClusterOptions: %v", err)
	}
	if !strings.HasPrefix(sawQuery, "delete=") {
		t.Fatalf("delete not in query: %s", sawQuery)
	}
	if !strings.Contains(sawQuery, "email_from") || !strings.Contains(sawQuery, "console") {
		t.Fatalf("delete query missing fields: %s", sawQuery)
	}
}

// clusterOptionsPtr is a test helper building a pointer to v.
func clusterOptionsPtr[T any](v T) *T { return &v }
