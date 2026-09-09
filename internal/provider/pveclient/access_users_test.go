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

// TestAccessUsersList decodes GET /access/users, including the comma-
// separated groups wire form and the 0/1 enable encoding.
func TestAccessUsersList(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/access/users" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[`+
			`{"userid":"root@pam","enable":1,"expire":0,"comment":"admin","groups":"admins,ops","email":"root@example.com"},`+
			`{"userid":"ci@pve","enable":0,"comment":"ci user"}]}`)
	})
	users, err := c.ListAccessUsers(context.Background())
	if err != nil {
		t.Fatalf("ListAccessUsers: %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("got %d users, want 2", len(users))
	}
	root := users[0]
	if root.UserID != "root@pam" || root.Comment != "admin" || root.Email != "root@example.com" {
		t.Fatalf("unexpected root: %+v", root)
	}
	if root.Enable == nil || !*root.Enable {
		t.Fatalf("enable = %v, want true", root.Enable)
	}
	if root.Expire == nil || *root.Expire != 0 {
		t.Fatalf("expire = %v, want 0", root.Expire)
	}
	if len(root.Groups) != 2 || root.Groups[0] != "admins" || root.Groups[1] != "ops" {
		t.Fatalf("groups = %v, want [admins ops]", root.Groups)
	}
	ci := users[1]
	if ci.Enable == nil || *ci.Enable {
		t.Fatalf("ci enable = %v, want false", ci.Enable)
	}
}

// TestAccessUsersCreate verifies the POST /access/users wire body: userid,
// joined groups, and the always-emitted writable strings.
func TestAccessUsersCreate(t *testing.T) {
	var sawBody map[string]any
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/access/users" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &sawBody); err != nil {
			t.Fatalf("request body not JSON: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":null}`)
	})
	user := AccessUser{
		UserID:   "ci@pve",
		Comment:  "ci user",
		Email:    "ci@example.com",
		Enable:   boolPtr(true),
		Expire:   int64Ptr(0),
		Groups:   []string{"g1", "g2"},
		Password: "s3cretpass",
	}
	if err := c.CreateAccessUser(context.Background(), user); err != nil {
		t.Fatalf("CreateAccessUser: %v", err)
	}
	if sawBody["userid"] != "ci@pve" || sawBody["comment"] != "ci user" || sawBody["email"] != "ci@example.com" {
		t.Fatalf("unexpected body scalars: %v", sawBody)
	}
	if sawBody["groups"] != "g1,g2" {
		t.Fatalf("groups = %v, want joined string g1,g2", sawBody["groups"])
	}
	if sawBody["enable"] != true || sawBody["expire"] != float64(0) {
		t.Fatalf("enable/expire = %v/%v", sawBody["enable"], sawBody["expire"])
	}
	if sawBody["password"] != "s3cretpass" {
		t.Fatalf("password = %v", sawBody["password"])
	}
}

// TestAccessUsersGet verifies GET /access/users/{userid} decodes the array-
// form groups and the boolish enable while leaving absent fields nil.
func TestAccessUsersGet(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/access/users/root@pam" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"userid":"root@pam","enable":1,"comment":"adm","groups":["admins"],"firstname":"Ro"}}`)
	})
	user, err := c.GetAccessUser(context.Background(), "root@pam")
	if err != nil {
		t.Fatalf("GetAccessUser: %v", err)
	}
	if user == nil || user.UserID != "root@pam" || user.Comment != "adm" || user.Firstname != "Ro" {
		t.Fatalf("unexpected user: %+v", user)
	}
	if len(user.Groups) != 1 || user.Groups[0] != "admins" {
		t.Fatalf("groups = %v, want [admins]", user.Groups)
	}
	if user.Enable == nil || !*user.Enable {
		t.Fatalf("enable = %v, want true", user.Enable)
	}
	if user.Expire != nil {
		t.Fatalf("expire = %v, want nil (absent)", user.Expire)
	}
}

// TestAccessUsersGet_EnableBoolEncoding covers the true/false encoding some
// PVE versions emit for enable.
func TestAccessUsersGet_EnableBoolEncoding(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"userid":"ci@pve","enable":false}}`)
	})
	user, err := c.GetAccessUser(context.Background(), "ci@pve")
	if err != nil {
		t.Fatalf("GetAccessUser: %v", err)
	}
	if user.Enable == nil || *user.Enable {
		t.Fatalf("enable = %v, want false", user.Enable)
	}
}

// TestAccessUsersUpdate verifies PUT /access/users/{userid} emits the full
// field set: cleared strings as empty values and an empty groups list as the
// empty string (PVE's update_user has no `delete` parameter).
func TestAccessUsersUpdate(t *testing.T) {
	var sawBody map[string]any
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/access/users/ci@pve" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &sawBody); err != nil {
			t.Fatalf("request body not JSON: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":null}`)
	})
	user := AccessUser{
		UserID:  "ci@pve",
		Comment: "updated",
		Email:   "new@example.com",
		Enable:  boolPtr(false),
		Expire:  int64Ptr(1735689600),
	}
	if err := c.UpdateAccessUser(context.Background(), "ci@pve", user); err != nil {
		t.Fatalf("UpdateAccessUser: %v", err)
	}
	if sawBody["comment"] != "updated" || sawBody["email"] != "new@example.com" {
		t.Fatalf("unexpected body scalars: %v", sawBody)
	}
	// Cleared strings must be present as empty values, not omitted.
	for _, key := range []string{"firstname", "lastname", "keys", "groups"} {
		if v, ok := sawBody[key]; !ok || v != "" {
			t.Fatalf("%s = %v (present=%v), want empty string", key, v, ok)
		}
	}
	if sawBody["enable"] != false || sawBody["expire"] != float64(1735689600) {
		t.Fatalf("enable/expire = %v/%v", sawBody["enable"], sawBody["expire"])
	}
	if _, ok := sawBody["password"]; ok {
		t.Fatal("password must not be sent on update")
	}
}

// TestAccessUsersDelete_404IsAPIError confirms DELETE /access/users/{userid}
// surfaces a 404 as *APIError so the resource layer can map it to
// "already absent".
func TestAccessUsersDelete_404IsAPIError(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/access/users/gone@pve" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"errors":"no such user ('gone@pve')\n"}`)
	})
	err := c.DeleteAccessUser(context.Background(), "gone@pve")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "HTTP 404") {
		t.Fatalf("error missing 404: %v", err)
	}
}

// TestAccessUserTokensList decodes GET /access/users/{userid}/token rows.
func TestAccessUserTokensList(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/access/users/root@pam/token" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"tokenid":"ci","comment":"CI token","expire":0,"privsep":1},{"tokenid":"ops","privsep":0}]}`)
	})
	tokens, err := c.ListAccessUserTokens(context.Background(), "root@pam")
	if err != nil {
		t.Fatalf("ListAccessUserTokens: %v", err)
	}
	if len(tokens) != 2 {
		t.Fatalf("got %d tokens, want 2", len(tokens))
	}
	if tokens[0].TokenID != "ci" || tokens[0].Comment != "CI token" || tokens[0].Expire == nil || *tokens[0].Expire != 0 {
		t.Fatalf("unexpected token[0]: %+v", tokens[0])
	}
	if tokens[0].Privsep == nil || !*tokens[0].Privsep {
		t.Fatalf("privsep[0] = %v, want true", tokens[0].Privsep)
	}
	if tokens[1].Privsep == nil || *tokens[1].Privsep {
		t.Fatalf("privsep[1] = %v, want false", tokens[1].Privsep)
	}
	if tokens[1].Comment != "" {
		t.Fatalf("comment[1] = %q, want empty", tokens[1].Comment)
	}
}

// TestAccessUserTokenCreate verifies POST /access/users/{userid}/token/
// {tokenid} (the pin places token create on the tokenid leaf) and that the
// one-time secret value from the response is captured.
func TestAccessUserTokenCreate(t *testing.T) {
	var sawBody map[string]any
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/access/users/root@pam/token/ci" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &sawBody); err != nil {
			t.Fatalf("request body not JSON: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"full-tokenid":"root@pam!ci","info":{"comment":"CI token","expire":0,"privsep":1},"value":"root@pam!ci=6c1f- secret"}}`)
	})
	tok, err := c.CreateAccessUserToken(context.Background(), "root@pam", "ci", AccessToken{Comment: "CI token", Expire: int64Ptr(0), Privsep: boolPtr(true)})
	if err != nil {
		t.Fatalf("CreateAccessUserToken: %v", err)
	}
	if sawBody["comment"] != "CI token" || sawBody["privsep"] != true {
		t.Fatalf("unexpected body: %v", sawBody)
	}
	if tok.Value != "root@pam!ci=6c1f- secret" {
		t.Fatalf("value = %q, want captured secret", tok.Value)
	}
	if tok.FullTokenID != "root@pam!ci" {
		t.Fatalf("full-tokenid = %q", tok.FullTokenID)
	}
	if tok.TokenID != "ci" || tok.Comment != "CI token" || tok.Privsep == nil || !*tok.Privsep {
		t.Fatalf("unexpected decoded token: %+v", tok)
	}
}

// TestAccessUserTokenGet verifies GET /access/users/{userid}/token/{tokenid}
// with the 0/1 privsep encoding and absent expire.
func TestAccessUserTokenGet(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/access/users/root@pam/token/ci" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"comment":"CI token","privsep":0}}`)
	})
	tok, err := c.GetAccessUserToken(context.Background(), "root@pam", "ci")
	if err != nil {
		t.Fatalf("GetAccessUserToken: %v", err)
	}
	if tok.TokenID != "ci" || tok.Comment != "CI token" {
		t.Fatalf("unexpected token: %+v", tok)
	}
	if tok.Privsep == nil || *tok.Privsep {
		t.Fatalf("privsep = %v, want false", tok.Privsep)
	}
	if tok.Expire != nil {
		t.Fatalf("expire = %v, want nil (absent)", tok.Expire)
	}
	if tok.Value != "" {
		t.Fatalf("value = %q, want empty (PVE never returns it on read)", tok.Value)
	}
}

// TestAccessUserTokenUpdate verifies PUT /access/users/{userid}/token/
// {tokenid} translates the delete slice into the query parameter and decodes
// the returned token info.
func TestAccessUserTokenUpdate(t *testing.T) {
	var sawQuery, sawBody string
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/access/users/root@pam/token/ci" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		sawQuery = r.URL.RawQuery
		raw, _ := io.ReadAll(r.Body)
		sawBody = string(raw)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"comment":"renamed","expire":0,"privsep":1}}`)
	})
	tok, err := c.UpdateAccessUserToken(context.Background(), "root@pam", "ci",
		AccessToken{Comment: "renamed", Expire: int64Ptr(0)}, []string{"privsep"})
	if err != nil {
		t.Fatalf("UpdateAccessUserToken: %v", err)
	}
	if !strings.HasPrefix(sawQuery, "delete=") || !strings.Contains(sawQuery, "privsep") {
		t.Fatalf("delete query missing: %s", sawQuery)
	}
	if !strings.Contains(sawBody, `"expire":0`) || strings.Contains(sawBody, `"privsep"`) {
		t.Fatalf("unexpected body: %s", sawBody)
	}
	if tok.Comment != "renamed" || tok.Privsep == nil || !*tok.Privsep {
		t.Fatalf("unexpected decoded token: %+v", tok)
	}
}

// TestAccessUserTokenDelete verifies DELETE /access/users/{userid}/token/
// {tokenid} and error wrapping.
func TestAccessUserTokenDelete(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/access/users/root@pam/token/ci" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":null}`)
	})
	if err := c.DeleteAccessUserToken(context.Background(), "root@pam", "ci"); err != nil {
		t.Fatalf("DeleteAccessUserToken: %v", err)
	}
}
