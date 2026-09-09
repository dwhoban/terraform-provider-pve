// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// TestClient_GetAcl_DecodesEntries covers GET /access/acl, including the
// boolish `propagate` encodings (int 1/0 and real booleans) PVE emits.
func TestClient_GetAcl_DecodesEntries(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/access/acl" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[`+
			`{"path":"/","roleid":"Administrator","type":"user","ugid":"root@pam","propagate":1},`+
			`{"path":"/vms/100","roleid":"PVEVMUser","type":"user","ugid":"ops@pam","propagate":false},`+
			`{"path":"/storage/local","roleid":"PVEDatastoreAdmin","type":"group","ugid":"ops"}`+
			`]}`)
	})
	entries, err := c.GetAcl(context.Background())
	if err != nil {
		t.Fatalf("GetAcl: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("len(entries) = %d, want 3", len(entries))
	}
	first := entries[0]
	if first.Path != "/" || first.RoleID != "Administrator" || first.Type != "user" || first.Ugid != "root@pam" {
		t.Fatalf("first = %+v", first)
	}
	if first.Propagate == nil || !*first.Propagate {
		t.Fatalf("first.Propagate = %v, want true (decoded from int 1)", first.Propagate)
	}
	second := entries[1]
	if second.Propagate == nil || *second.Propagate {
		t.Fatalf("second.Propagate = %v, want false", second.Propagate)
	}
	third := entries[2]
	if third.Propagate != nil {
		t.Fatalf("third.Propagate = %v, want nil (absent on the wire)", third.Propagate)
	}
}

// TestClient_PutAcl_WireShape verifies PUT /access/acl sends the entry as a
// JSON body with path/roles and the matching identity list parameter.
func TestClient_PutAcl_WireShape(t *testing.T) {
	var sawMethod, sawPath string
	var sawBody map[string]any
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		sawMethod = r.Method
		sawPath = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &sawBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":null}`)
	})
	propagate := true
	err := c.PutAcl(context.Background(), AclUpdate{
		Path:      "/vms/100",
		Roles:     "PVEVMUser",
		Propagate: &propagate,
		Users:     "ops@pam",
	})
	if err != nil {
		t.Fatalf("PutAcl: %v", err)
	}
	if sawMethod != http.MethodPut || sawPath != "/access/acl" {
		t.Fatalf("request = %s %s, want PUT /access/acl", sawMethod, sawPath)
	}
	if sawBody["path"] != "/vms/100" || sawBody["roles"] != "PVEVMUser" || sawBody["users"] != "ops@pam" {
		t.Fatalf("body = %v", sawBody)
	}
	if v, ok := sawBody["propagate"].(bool); !ok || !v {
		t.Fatalf("body propagate = %v, want true", sawBody["propagate"])
	}
	if _, ok := sawBody["delete"]; ok {
		t.Fatalf("body should omit delete when false, got %v", sawBody["delete"])
	}
}

// TestClient_DeleteAcl_WireShape verifies DeleteAcl issues PUT /access/acl
// with delete=true plus the entry identity (the pin defines no DELETE verb
// on /access/acl; removal is PUT update_acl with the delete flag).
func TestClient_DeleteAcl_WireShape(t *testing.T) {
	var sawMethod string
	var sawBody map[string]any
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		sawMethod = r.Method
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &sawBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":null}`)
	})
	err := c.DeleteAcl(context.Background(), AclEntry{
		Path:   "/vms/100",
		RoleID: "PVEVMUser",
		Type:   "user",
		Ugid:   "ops@pam",
	})
	if err != nil {
		t.Fatalf("DeleteAcl: %v", err)
	}
	if sawMethod != http.MethodPut {
		t.Fatalf("method = %s, want PUT", sawMethod)
	}
	if sawBody["delete"] != true {
		t.Fatalf("body delete = %v, want true", sawBody["delete"])
	}
	if sawBody["path"] != "/vms/100" || sawBody["roles"] != "PVEVMUser" || sawBody["users"] != "ops@pam" {
		t.Fatalf("body = %v", sawBody)
	}
	if _, ok := sawBody["groups"]; ok {
		t.Fatalf("body should omit groups for a user entry, got %v", sawBody["groups"])
	}
}

// TestClient_GetPermissions_DecodesAndQueries covers GET /access/permissions
// query encoding (userid contains @ and !) and the boolish privilege map
// decode (path => privilege => propagate flag).
func TestClient_GetPermissions_DecodesAndQueries(t *testing.T) {
	var sawRawQuery string
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/access/permissions" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		sawRawQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"/":{"Administrator":1},"/vms/100":{"PVEVMUser":0},"/storage/local":{"PVEDatastoreAdmin":true}}}`)
	})
	perms, err := c.GetPermissions(context.Background(), "", "root@pam!ci")
	if err != nil {
		t.Fatalf("GetPermissions: %v", err)
	}
	query, err := url.QueryUnescape(sawRawQuery)
	if err != nil {
		t.Fatalf("unescape query %q: %v", sawRawQuery, err)
	}
	if !strings.Contains(query, "userid=root@pam!ci") {
		t.Fatalf("query missing escaped userid: %s", query)
	}
	if strings.Contains(query, "path=") {
		t.Fatalf("empty path must not be sent, query: %s", query)
	}
	if !perms["/"]["Administrator"] || perms["/vms/100"]["PVEVMUser"] || !perms["/storage/local"]["PVEDatastoreAdmin"] {
		t.Fatalf("perms = %+v", perms)
	}
}

// TestClient_GetPermissions_PathFilter verifies the optional path filter is
// sent as a query parameter including its leading slash.
func TestClient_GetPermissions_PathFilter(t *testing.T) {
	var sawRawQuery string
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		sawRawQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"/vms/100":{"PVEVMUser":1}}}`)
	})
	if _, err := c.GetPermissions(context.Background(), "/vms/100", ""); err != nil {
		t.Fatalf("GetPermissions: %v", err)
	}
	if !strings.Contains(sawRawQuery, "path=%2Fvms%2F100") {
		t.Fatalf("query missing encoded path filter: %s", sawRawQuery)
	}
	if strings.Contains(sawRawQuery, "userid=") {
		t.Fatalf("empty userid must not be sent, query: %s", sawRawQuery)
	}
}
