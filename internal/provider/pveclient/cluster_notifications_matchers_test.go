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

// TestNotificationMatcher_CreateWire verifies POST
// /cluster/notifications/matchers sends the hyphenated wire keys and the
// match lists as JSON arrays.
func TestNotificationMatcher_CreateWire(t *testing.T) {
	var sawBody []byte
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/cluster/notifications/matchers" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		sawBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":null}`)
	})
	err := c.CreateNotificationMatcher(context.Background(), NotificationMatcher{
		Name:          "ops",
		Target:        []string{"mail-to-root", "team-hook"},
		MatchField:    []string{"exact:severity=error", "regex:hostname=^pve"},
		MatchSeverity: []string{"error", "warning"},
		MatchCalendar: []string{"sat..sun 02:30"},
		Mode:          "any",
		InvertMatch:   HABoolPtr(false),
		Comment:       "page the ops channel",
	})
	if err != nil {
		t.Fatalf("CreateNotificationMatcher: %v", err)
	}
	var sent map[string]any
	if err := json.Unmarshal(sawBody, &sent); err != nil {
		t.Fatalf("body %q is not JSON: %v", sawBody, err)
	}
	if sent["name"] != "ops" {
		t.Fatalf("name = %v, want ops", sent["name"])
	}
	if sent["mode"] != "any" {
		t.Fatalf("mode = %v, want any", sent["mode"])
	}
	for _, key := range []string{"match-field", "match-severity", "match-calendar", "target", "invert-match", "comment"} {
		if _, ok := sent[key]; !ok {
			t.Fatalf("create body missing %q: %q", key, sawBody)
		}
	}
	if _, ok := sent["match-field"].([]any); !ok {
		t.Fatalf("match-field must be a JSON array, got %v", sent["match-field"])
	}
	if _, ok := sent["disable"]; ok {
		t.Fatalf("nil disable must be omitted, got %q", sawBody)
	}
}

// TestNotificationMatcher_GetDecode decodes GET /cluster/notifications/matchers/{name},
// tolerating the 0/1 int encoding of the boolish fields.
func TestNotificationMatcher_GetDecode(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/cluster/notifications/matchers/ops" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{`+
			`"name":"ops",`+
			`"target":["mail-to-root"],`+
			`"match-field":["regex:hostname=^pve"],`+
			`"match-severity":["error"],`+
			`"match-calendar":["sat..sun 02:30"],`+
			`"mode":"any",`+
			`"invert-match":1,`+
			`"disable":0,`+
			`"comment":"page the ops channel",`+
			`"digest":"abc123"}}`)
	})
	m, err := c.GetNotificationMatcher(context.Background(), "ops")
	if err != nil {
		t.Fatalf("GetNotificationMatcher: %v", err)
	}
	if m.Name != "ops" || m.Mode != "any" || m.Digest != "abc123" {
		t.Fatalf("matcher = %+v", m)
	}
	if len(m.Target) != 1 || m.Target[0] != "mail-to-root" {
		t.Fatalf("target = %v", m.Target)
	}
	if len(m.MatchField) != 1 || m.MatchField[0] != "regex:hostname=^pve" {
		t.Fatalf("match-field = %v", m.MatchField)
	}
	if m.InvertMatch == nil || !*m.InvertMatch {
		t.Fatalf("invert-match = %v, want true", m.InvertMatch)
	}
	if m.Disable == nil || *m.Disable {
		t.Fatalf("disable = %v, want false", m.Disable)
	}
}

// TestNotificationMatcher_UpdateDeleteQuery verifies the PUT path and that
// cleared fields travel in the PVE `delete` query parameter.
func TestNotificationMatcher_UpdateDeleteQuery(t *testing.T) {
	var sawQuery string
	var sawBody []byte
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/cluster/notifications/matchers/ops" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		sawQuery = r.URL.RawQuery
		sawBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":null}`)
	})
	err := c.UpdateNotificationMatcher(context.Background(), "ops", NotificationMatcher{
		Name:       "ops",
		MatchField: []string{"exact:severity=error"},
	}, []string{"match-severity", "match-calendar"})
	if err != nil {
		t.Fatalf("UpdateNotificationMatcher: %v", err)
	}
	if !strings.Contains(sawQuery, "delete=match-severity%2Cmatch-calendar") {
		t.Fatalf("delete query = %q, want comma-joined fields", sawQuery)
	}
	if !strings.Contains(string(sawBody), `"match-field":["exact:severity=error"]`) {
		t.Fatalf("update body = %q", sawBody)
	}
}

// TestNotificationMatcher_DeleteWire verifies DELETE /cluster/notifications/matchers/{name}.
func TestNotificationMatcher_DeleteWire(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/cluster/notifications/matchers/ops" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":null}`)
	})
	if err := c.DeleteNotificationMatcher(context.Background(), "ops"); err != nil {
		t.Fatalf("DeleteNotificationMatcher: %v", err)
	}
}

// TestListNotificationMatchers decodes GET /cluster/notifications/matchers,
// including the origin field that only the listing carries.
func TestListNotificationMatchers(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/cluster/notifications/matchers" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[`+
			`{"name":"ops","target":["mail-to-root"],"origin":"user-created","disable":0},`+
			`{"name":"default-matcher","origin":"builtin","disable":1}]}`)
	})
	matchers, err := c.ListNotificationMatchers(context.Background())
	if err != nil {
		t.Fatalf("ListNotificationMatchers: %v", err)
	}
	if len(matchers) != 2 {
		t.Fatalf("got %d matchers, want 2", len(matchers))
	}
	if matchers[0].Origin != "user-created" || matchers[1].Origin != "builtin" {
		t.Fatalf("origins = %q, %q", matchers[0].Origin, matchers[1].Origin)
	}
	if matchers[1].Disable == nil || !*matchers[1].Disable {
		t.Fatalf("builtin disable = %v, want true", matchers[1].Disable)
	}
}

// TestListNotificationTargets decodes GET /cluster/notifications/targets,
// which lists user-created and built-in targets alike.
func TestListNotificationTargets(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/cluster/notifications/targets" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[`+
			`{"name":"mail-to-root","type":"sendmail","origin":"builtin","comment":"Send mails to root","disable":0},`+
			`{"name":"team-hook","type":"webhook","origin":"user-created","disable":false}]}`)
	})
	targets, err := c.ListNotificationTargets(context.Background())
	if err != nil {
		t.Fatalf("ListNotificationTargets: %v", err)
	}
	if len(targets) != 2 {
		t.Fatalf("got %d targets, want 2", len(targets))
	}
	if targets[0].Name != "mail-to-root" || targets[0].Type != "sendmail" || targets[0].Origin != "builtin" {
		t.Fatalf("target[0] = %+v", targets[0])
	}
	if targets[0].Disable == nil || *targets[0].Disable {
		t.Fatalf("builtin disable = %v, want false", targets[0].Disable)
	}
	if targets[1].Comment != "" {
		t.Fatalf("absent comment decoded as %q", targets[1].Comment)
	}
}

// TestNotificationTargetTest verifies POST /cluster/notifications/targets/{name}/test
// sends no body (the pin returns null synchronously).
func TestNotificationTargetTest(t *testing.T) {
	var sawBody []byte
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/cluster/notifications/targets/mail-to-root/test" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		sawBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":null}`)
	})
	if err := c.TestNotificationTarget(context.Background(), "mail-to-root"); err != nil {
		t.Fatalf("TestNotificationTarget: %v", err)
	}
	if len(sawBody) != 0 {
		t.Fatalf("test endpoint must not carry a body, got %q", sawBody)
	}
}
