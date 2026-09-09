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

// TestClient_ListAccessGroups_GroupIndex decodes the GET /access/groups
// index, where `users` is a single comma-separated userid string.
func TestClient_ListAccessGroups_GroupIndex(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/access/groups" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[
			{"groupid":"admins","comment":"Cluster administrators","users":"root@pam,ci@pam"},
			{"groupid":"auditors"}
		]}`)
	})
	groups, err := c.ListAccessGroups(context.Background())
	if err != nil {
		t.Fatalf("ListAccessGroups: %v", err)
	}
	if len(groups) != 2 {
		t.Fatalf("got %d groups, want 2", len(groups))
	}
	if groups[0].GroupID != "admins" || groups[0].Comment == nil || *groups[0].Comment != "Cluster administrators" {
		t.Fatalf("group[0] = %+v", groups[0])
	}
	if groups[0].Users != "root@pam,ci@pam" {
		t.Fatalf("Users = %q", groups[0].Users)
	}
	if groups[1].Comment != nil || groups[1].Users != "" {
		t.Fatalf("group[1] = %+v", groups[1])
	}
}

// TestClient_GetAccessGroup_Detail verifies the single-group GET decodes
// `members` as an array of full user IDs.
func TestClient_GetAccessGroup_Detail(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/access/groups/admins" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"comment":"Cluster administrators","members":["root@pam","ci@pam"]}}`)
	})
	group, err := c.GetAccessGroup(context.Background(), "admins")
	if err != nil {
		t.Fatalf("GetAccessGroup: %v", err)
	}
	if group == nil || group.Comment == nil || *group.Comment != "Cluster administrators" {
		t.Fatalf("group = %+v", group)
	}
	if len(group.Members) != 2 || group.Members[0] != "root@pam" || group.Members[1] != "ci@pam" {
		t.Fatalf("Members = %v", group.Members)
	}
}

// TestClient_CreateAccessGroup_WireBody asserts the POST body carries
// groupid and the optional comment.
func TestClient_CreateAccessGroup_WireBody(t *testing.T) {
	var sawBody map[string]any
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/access/groups" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&sawBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":null}`)
	})
	comment := "Cluster administrators"
	if err := c.CreateAccessGroup(context.Background(), "admins", &comment); err != nil {
		t.Fatalf("CreateAccessGroup: %v", err)
	}
	if sawBody["groupid"] != "admins" || sawBody["comment"] != "Cluster administrators" {
		t.Fatalf("body = %v", sawBody)
	}
}

// TestClient_CreateAccessGroup_NilCommentOmitted keeps an absent comment out
// of the POST body entirely.
func TestClient_CreateAccessGroup_NilCommentOmitted(t *testing.T) {
	var sawBody map[string]any
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&sawBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":null}`)
	})
	if err := c.CreateAccessGroup(context.Background(), "plain", nil); err != nil {
		t.Fatalf("CreateAccessGroup: %v", err)
	}
	if _, present := sawBody["comment"]; present {
		t.Fatalf("comment should be omitted: %v", sawBody)
	}
	if sawBody["groupid"] != "plain" {
		t.Fatalf("body = %v", sawBody)
	}
}

// TestClient_UpdateAccessGroup_EmptyCommentSent verifies a pointer to the
// empty string is still serialized (the clear-comment path — nil omits, ""
// clears).
func TestClient_UpdateAccessGroup_EmptyCommentSent(t *testing.T) {
	var sawBody map[string]any
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/access/groups/admins" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&sawBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":null}`)
	})
	if err := c.UpdateAccessGroup(context.Background(), "admins", accessGroupsRolesStringPtr("")); err != nil {
		t.Fatalf("UpdateAccessGroup: %v", err)
	}
	value, present := sawBody["comment"]
	if !present || value != "" {
		t.Fatalf("body = %v, want comment:%q present", sawBody, "")
	}
}

// TestClient_DeleteAccessGroup asserts the DELETE verb and path.
func TestClient_DeleteAccessGroup(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/access/groups/admins" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":null}`)
	})
	if err := c.DeleteAccessGroup(context.Background(), "admins"); err != nil {
		t.Fatalf("DeleteAccessGroup: %v", err)
	}
}

// TestClient_GetAccessGroup_404IsAPIError surfaces a missing group as
// *APIError so the resource layer can drop it from state.
func TestClient_GetAccessGroup_404IsAPIError(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"errors":"group 'ghost' does not exist"}`)
	})
	_, err := c.GetAccessGroup(context.Background(), "ghost")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "HTTP 404") {
		t.Fatalf("error missing 404: %v", err)
	}
}

// TestClient_ListAccessRoles_RoleIndex decodes the GET /access/roles index,
// including the `special` (built-in) flag and the comma-joined priv list.
func TestClient_ListAccessRoles_RoleIndex(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/access/roles" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[
			{"roleid":"PVEVMAdmin","privs":"VM.Allocate,VM.Audit","special":1},
			{"roleid":"custom-op","privs":"VM.PowerMgmt"}
		]}`)
	})
	roles, err := c.ListAccessRoles(context.Background())
	if err != nil {
		t.Fatalf("ListAccessRoles: %v", err)
	}
	if len(roles) != 2 {
		t.Fatalf("got %d roles, want 2", len(roles))
	}
	if roles[0].RoleID != "PVEVMAdmin" || roles[0].Privs != "VM.Allocate,VM.Audit" {
		t.Fatalf("role[0] = %+v", roles[0])
	}
	if roles[0].Special == nil || !*roles[0].Special {
		t.Fatalf("Special = %v, want pointer to true", roles[0].Special)
	}
	if roles[1].Special != nil {
		t.Fatalf("role[1].Special = %v, want nil", roles[1].Special)
	}
}

// TestClient_GetAccessRole_PrivilegeMap decodes the single-role GET, whose
// response object keys are privilege names mapped to booleans.
func TestClient_GetAccessRole_PrivilegeMap(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/access/roles/custom-op" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"VM.PowerMgmt":true,"VM.Audit":false}}`)
	})
	privs, err := c.GetAccessRole(context.Background(), "custom-op")
	if err != nil {
		t.Fatalf("GetAccessRole: %v", err)
	}
	granted, ok := privs["VM.PowerMgmt"]
	if !ok || !granted {
		t.Fatalf("VM.PowerMgmt missing or false: %v", privs)
	}
	if denied, ok := privs["VM.Audit"]; !ok || denied {
		t.Fatalf("VM.Audit should decode as false: %v", privs)
	}
}

// TestClient_CreateAccessRole_CommaJoinedPrivs asserts the pve-priv-list
// wire format: one comma-separated string in the POST body.
func TestClient_CreateAccessRole_CommaJoinedPrivs(t *testing.T) {
	var sawBody map[string]any
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/access/roles" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&sawBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":null}`)
	})
	if err := c.CreateAccessRole(context.Background(), "custom-op", []string{"VM.Allocate", "VM.Audit"}); err != nil {
		t.Fatalf("CreateAccessRole: %v", err)
	}
	if sawBody["roleid"] != "custom-op" || sawBody["privs"] != "VM.Allocate,VM.Audit" {
		t.Fatalf("body = %v", sawBody)
	}
}

// TestClient_UpdateAccessRole_ReplacesPrivs asserts PUT sends the full
// replacement priv list and never the `append` flag.
func TestClient_UpdateAccessRole_ReplacesPrivs(t *testing.T) {
	var sawBody map[string]any
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/access/roles/custom-op" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&sawBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":null}`)
	})
	if err := c.UpdateAccessRole(context.Background(), "custom-op", []string{"VM.PowerMgmt"}); err != nil {
		t.Fatalf("UpdateAccessRole: %v", err)
	}
	if sawBody["privs"] != "VM.PowerMgmt" {
		t.Fatalf("body = %v", sawBody)
	}
	if _, present := sawBody["append"]; present {
		t.Fatalf("append should never be sent: %v", sawBody)
	}
}

// TestClient_DeleteAccessRole asserts the DELETE verb and path.
func TestClient_DeleteAccessRole(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/access/roles/custom-op" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":null}`)
	})
	if err := c.DeleteAccessRole(context.Background(), "custom-op"); err != nil {
		t.Fatalf("DeleteAccessRole: %v", err)
	}
}

// accessGroupsRolesStringPtr is a test helper for building expected *string
// values.
func accessGroupsRolesStringPtr(s string) *string { return &s }
