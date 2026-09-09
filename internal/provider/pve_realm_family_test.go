// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TestPveRealmGetChecked_TypeMismatch verifies a realm component never
// silently adopts a realm of another type.
func TestPveRealmGetChecked_TypeMismatch(t *testing.T) {
	client := newNodeNetworkTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/access/domains/corp" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"realm":"corp","type":"openid"}}`)
	})
	if _, err := realmGetChecked(context.Background(), client, "corp", realmTypeLDAP); err == nil {
		t.Fatal("realmGetChecked: expected type mismatch error")
	} else if !strings.Contains(err.Error(), `manages type "ldap"`) {
		t.Fatalf("error = %v, want type mismatch naming the managed type", err)
	}
}

// TestPveRealmNodeFromUpid covers the UPID node extraction used by the
// realm sync action.
func TestPveRealmNodeFromUpid(t *testing.T) {
	for _, tc := range []struct {
		upid      string
		want      string
		wantError bool
	}{
		{upid: "UPID:pve1:00001234:12345678:realm-sync:corp:root@pam:", want: "pve1"},
		{upid: "UPID:pve2:00001234:12345678:UPID:upgrade:", want: "pve2"},
		{upid: "", wantError: true},
		{upid: "garbage", wantError: true},
		{upid: "UPID::00001234:", wantError: true},
	} {
		got, err := realmNodeFromUpid(tc.upid)
		if tc.wantError {
			if err == nil {
				t.Fatalf("upid %q: expected error", tc.upid)
			}
			continue
		}
		if err != nil {
			t.Fatalf("upid %q: %v", tc.upid, err)
		}
		if got != tc.want {
			t.Fatalf("upid %q: got %q, want %q", tc.upid, got, tc.want)
		}
	}
}

// TestPveRealmSyncAction_Run verifies the action starts the sync, polls
// the task to completion, and reports progress.
func TestPveRealmSyncAction_Run(t *testing.T) {
	const upid = "UPID:pve1:00001234:12345678:realm-sync:corp:root@pam:"
	var syncSeen, statusSeen bool
	var syncBody map[string]any
	client := newNodeNetworkTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/access/domains/corp/sync":
			syncSeen = true
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &syncBody)
			_, _ = io.WriteString(w, `{"data":"`+upid+`"}`)
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/nodes/pve1/tasks/"):
			statusSeen = true
			_, _ = io.WriteString(w, `{"data":{"status":"stopped","exitstatus":"OK"}}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	action := &pveRealmSyncAction{client: client}
	config := pveRealmSyncActionModel{
		Realm:          types.StringValue("corp"),
		Scope:          types.StringValue("both"),
		DryRun:         types.BoolValue(false),
		RemoveVanished: types.StringValue("entry;acl"),
	}
	var messages []string
	progress := func(message string) {
		messages = append(messages, message)
	}
	if err := action.run(context.Background(), config, progress); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !syncSeen || !statusSeen {
		t.Fatalf("sync called = %v, task status polled = %v; want both true", syncSeen, statusSeen)
	}
	if syncBody["scope"] != "both" || syncBody["remove-vanished"] != "entry;acl" {
		t.Fatalf("sync body = %v", syncBody)
	}
	if len(messages) != 2 {
		t.Fatalf("progress messages = %v, want start and finish", messages)
	}
}
