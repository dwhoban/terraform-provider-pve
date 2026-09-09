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

// TestStorageRemoteCreatePBS verifies POST /storage for the pbs type: only
// set fields travel, content is joined, and the secret travels only in the
// request body (the client performs no logging).
func TestStorageRemoteCreatePBS(t *testing.T) {
	var sawBody map[string]any
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/storage" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &sawBody); err != nil {
			t.Fatalf("request body not JSON: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":null}`)
	})
	if err := c.CreateStorageRemote(context.Background(), StorageRemote{
		Storage:              "pbs1",
		Type:                 "pbs",
		Server:               "192.168.1.10",
		Datastore:            "store1",
		Username:             "backup@pbs",
		Password:             "s3cret-pbs-password",
		Fingerprint:          "AA:BB",
		Port:                 int64Ptr(8007),
		Content:              "backup",
		MasterPubkey:         "bWFzdGVyLWtleQ==",
		MaxProtectedBackups:  int64Ptr(5),
		SkipCertVerification: boolPtr(true),
	}); err != nil {
		t.Fatalf("CreateStorageRemote: %v", err)
	}
	if sawBody["storage"] != "pbs1" || sawBody["type"] != "pbs" || sawBody["server"] != "192.168.1.10" {
		t.Fatalf("unexpected identity scalars: %v", sawBody)
	}
	if sawBody["datastore"] != "store1" || sawBody["username"] != "backup@pbs" || sawBody["password"] != "s3cret-pbs-password" {
		t.Fatalf("unexpected datastore credentials: %v", sawBody)
	}
	if sawBody["fingerprint"] != "AA:BB" || sawBody["port"] != float64(8007) || sawBody["content"] != "backup" {
		t.Fatalf("unexpected pbs fields: %v", sawBody)
	}
	if sawBody["master-pubkey"] != "bWFzdGVyLWtleQ==" || sawBody["max-protected-backups"] != float64(5) || sawBody["skip-cert-verification"] != true {
		t.Fatalf("unexpected pbs key fields: %v", sawBody)
	}
	// Fields of the other storage types must never leak into the body.
	for _, key := range []string{"monhost", "keyring", "pool", "fs-name", "krbd"} {
		if _, ok := sawBody[key]; ok {
			t.Fatalf("%s must not be sent when unset", key)
		}
	}
	// The client must never log or embed the secret outside the request
	// body: its non-request surface (endpoint, auth kind, error prefix)
	// carries no credential material.
	if strings.Contains(c.Endpoint()+c.AuthKind(), "s3cret-pbs-password") {
		t.Fatal("client surface leaks the pbs password")
	}
}

// TestStorageRemoteCreateCephfs verifies POST /storage for the cephfs
// type, including the hyphenated fs-name wire key and joined content.
func TestStorageRemoteCreateCephfs(t *testing.T) {
	var sawBody map[string]any
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/storage" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &sawBody); err != nil {
			t.Fatalf("request body not JSON: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":null}`)
	})
	if err := c.CreateStorageRemote(context.Background(), StorageRemote{
		Storage:  "cephfs1",
		Type:     "cephfs",
		Monhost:  "10.0.0.1,10.0.0.2",
		Username: "admin",
		Keyring:  "s3cret-keyring",
		FsName:   "cephfs",
		Subdir:   "/pve",
		Content:  "images,rootdir",
		Fuse:     boolPtr(false),
	}); err != nil {
		t.Fatalf("CreateStorageRemote: %v", err)
	}
	if sawBody["storage"] != "cephfs1" || sawBody["type"] != "cephfs" || sawBody["monhost"] != "10.0.0.1,10.0.0.2" {
		t.Fatalf("unexpected identity scalars: %v", sawBody)
	}
	if sawBody["username"] != "admin" || sawBody["keyring"] != "s3cret-keyring" {
		t.Fatalf("unexpected ceph credentials: %v", sawBody)
	}
	if sawBody["fs-name"] != "cephfs" || sawBody["subdir"] != "/pve" || sawBody["content"] != "images,rootdir" || sawBody["fuse"] != false {
		t.Fatalf("unexpected cephfs fields: %v", sawBody)
	}
	for _, key := range []string{"server", "datastore", "pool", "krbd", "password"} {
		if _, ok := sawBody[key]; ok {
			t.Fatalf("%s must not be sent when unset", key)
		}
	}
}

// TestStorageRemoteCreateRBD verifies POST /storage for the rbd type.
func TestStorageRemoteCreateRBD(t *testing.T) {
	var sawBody map[string]any
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/storage" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &sawBody); err != nil {
			t.Fatalf("request body not JSON: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":null}`)
	})
	if err := c.CreateStorageRemote(context.Background(), StorageRemote{
		Storage:       "rbd1",
		Type:          "rbd",
		Monhost:       "10.0.0.1:6789",
		Pool:          "rbd",
		Username:      "admin",
		Keyring:       "s3cret-rbd-keyring",
		Namespace:     "pve",
		Authsupported: "cephx",
		KRBD:          boolPtr(true),
		Content:       "images,rootdir",
	}); err != nil {
		t.Fatalf("CreateStorageRemote: %v", err)
	}
	if sawBody["storage"] != "rbd1" || sawBody["type"] != "rbd" || sawBody["pool"] != "rbd" || sawBody["monhost"] != "10.0.0.1:6789" {
		t.Fatalf("unexpected identity scalars: %v", sawBody)
	}
	if sawBody["keyring"] != "s3cret-rbd-keyring" || sawBody["username"] != "admin" || sawBody["authsupported"] != "cephx" {
		t.Fatalf("unexpected rbd credentials: %v", sawBody)
	}
	if sawBody["namespace"] != "pve" || sawBody["krbd"] != true || sawBody["content"] != "images,rootdir" {
		t.Fatalf("unexpected rbd fields: %v", sawBody)
	}
}

// TestStorageRemoteCreate_RejectsInvalid confirms the client rejects
// creates missing storage or type, and unsupported types, before hitting
// the wire.
func TestStorageRemoteCreate_RejectsInvalid(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("no request expected, got %s %s", r.Method, r.URL.Path)
	})
	if err := c.CreateStorageRemote(context.Background(), StorageRemote{Type: "pbs"}); err == nil {
		t.Fatal("expected error for missing storage, got nil")
	}
	if err := c.CreateStorageRemote(context.Background(), StorageRemote{Storage: "s1"}); err == nil {
		t.Fatal("expected error for missing type, got nil")
	}
	if err := c.CreateStorageRemote(context.Background(), StorageRemote{Storage: "s1", Type: "dir"}); err == nil {
		t.Fatal("expected error for unsupported type dir, got nil")
	}
}

// TestStorageRemoteGet decodes GET /storage/{storage}: the 0/1 boolean
// encodings, the string encoding of numeric fields, absent fields staying
// nil, and the read-only digest. Secrets decode into their fields.
func TestStorageRemoteGet(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/storage/pbs1" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"storage":"ignored","type":"pbs","server":"192.168.1.10",`+
			`"datastore":"store1","username":"backup@pbs","password":"s3cret-pbs-password",`+
			`"fingerprint":"AA:BB","port":"8007","content":"backup","disable":0,`+
			`"max-protected-backups":5,"digest":"a1b2c3"}}`)
	})
	s, err := c.GetStorageRemote(context.Background(), "pbs1")
	if err != nil {
		t.Fatalf("GetStorageRemote: %v", err)
	}
	if s.Storage != "pbs1" || s.Type != "pbs" || s.Server != "192.168.1.10" || s.Datastore != "store1" {
		t.Fatalf("unexpected storage: %+v", s)
	}
	if s.Password != "s3cret-pbs-password" || s.Fingerprint != "AA:BB" {
		t.Fatalf("unexpected pbs secrets: %+v", s)
	}
	if s.Port == nil || *s.Port != 8007 {
		t.Fatalf("port = %v, want 8007 (string encoding)", s.Port)
	}
	if s.Disable == nil || *s.Disable {
		t.Fatalf("disable = %v, want false", s.Disable)
	}
	if s.MaxProtectedBackups == nil || *s.MaxProtectedBackups != 5 {
		t.Fatalf("max-protected-backups = %v, want 5", s.MaxProtectedBackups)
	}
	if s.Content != "backup" || s.Digest != "a1b2c3" {
		t.Fatalf("unexpected content/digest: %+v", s)
	}
	// Ceph and RBD fields stay unset for a pbs storage.
	if s.Monhost != "" || s.Keyring != "" || s.Pool != "" || s.FsName != "" || s.Fuse != nil || s.KRBD != nil {
		t.Fatalf("foreign type fields decoded: %+v", s)
	}
}

// TestStorageRemoteGet_CephfsBoolish covers cephfs reads with the 0/1
// encoding of fuse.
func TestStorageRemoteGet_CephfsBoolish(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/storage/cephfs1" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"type":"cephfs","monhost":"10.0.0.1","fs-name":"cephfs",`+
			`"subdir":"/pve","username":"admin","keyring":"s3cret-keyring","fuse":1,"content":"images,rootdir"}}`)
	})
	s, err := c.GetStorageRemote(context.Background(), "cephfs1")
	if err != nil {
		t.Fatalf("GetStorageRemote: %v", err)
	}
	if s.Type != "cephfs" || s.FsName != "cephfs" || s.Subdir != "/pve" || s.Monhost != "10.0.0.1" {
		t.Fatalf("unexpected cephfs fields: %+v", s)
	}
	if s.Keyring != "s3cret-keyring" {
		t.Fatalf("keyring = %q, want the configured keyring", s.Keyring)
	}
	if s.Fuse == nil || !*s.Fuse {
		t.Fatalf("fuse = %v, want true (0/1 encoding)", s.Fuse)
	}
}

// TestStorageRemoteUpdate verifies PUT /storage: the body carries storage
// (required per the pin) but never type (immutable), and the delete slice
// travels as the `delete` query parameter.
func TestStorageRemoteUpdate(t *testing.T) {
	var sawQuery url.Values
	var sawBody map[string]any
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/storage" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		sawQuery = r.URL.Query()
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &sawBody); err != nil {
			t.Fatalf("request body not JSON: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":null}`)
	})
	if err := c.UpdateStorageRemote(context.Background(), "rbd1", StorageRemote{
		Storage:  "stale",
		Type:     "rbd",
		Monhost:  "10.0.0.2:6789",
		Pool:     "rbd",
		Username: "admin",
		Content:  "images",
	}, []string{"namespace", "keyring"}); err != nil {
		t.Fatalf("UpdateStorageRemote: %v", err)
	}
	if got := sawQuery.Get("delete"); got != "namespace,keyring" {
		t.Fatalf("delete param = %q, want namespace,keyring", got)
	}
	if sawBody["storage"] != "rbd1" {
		t.Fatalf("body storage = %v, want rbd1", sawBody["storage"])
	}
	if _, ok := sawBody["type"]; ok {
		t.Fatal("type must not be sent on update (pin's PUT has no type parameter)")
	}
	if sawBody["monhost"] != "10.0.0.2:6789" || sawBody["pool"] != "rbd" || sawBody["content"] != "images" {
		t.Fatalf("unexpected body scalars: %v", sawBody)
	}
}

// TestStorageRemoteDelete confirms DELETE /storage/{storage} and that a
// 404 surfaces as an error so the resource layer can map it to "already
// absent".
func TestStorageRemoteDelete(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/storage/gone" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"errors":"storage 'gone' does not exist"}`)
	})
	err := c.DeleteStorageRemote(context.Background(), "gone")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "HTTP 404") {
		t.Fatalf("error missing 404: %v", err)
	}
}
