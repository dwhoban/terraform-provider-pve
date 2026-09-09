// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// TestStorageNetCreate_NFS verifies POST /storage for type nfs: the body
// carries the identity and nfs fields, `content` travels as the pin's
// comma-separated list string, and fields of the other family types never
// leak into the body.
func TestStorageNetCreate_NFS(t *testing.T) {
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
	s := StorageNet{
		Storage:             "nfsvm",
		Type:                "nfs",
		Server:              "192.168.1.10",
		Export:              "/srv/export",
		Content:             "images,iso",
		Nodes:               "pve1,pve2",
		Options:             "vers=4.2,soft",
		Disable:             boolPtr(false),
		Shared:              boolPtr(true),
		PruneBackups:        "keep-last=3",
		MaxProtectedBackups: int64Ptr(5),
	}
	if err := c.CreateStorageNet(context.Background(), s); err != nil {
		t.Fatalf("CreateStorageNet: %v", err)
	}
	if sawBody["storage"] != "nfsvm" || sawBody["type"] != "nfs" || sawBody["server"] != "192.168.1.10" || sawBody["export"] != "/srv/export" {
		t.Fatalf("unexpected identity scalars: %v", sawBody)
	}
	if sawBody["content"] != "images,iso" || sawBody["nodes"] != "pve1,pve2" || sawBody["options"] != "vers=4.2,soft" {
		t.Fatalf("unexpected list fields: %v", sawBody)
	}
	if sawBody["disable"] != false || sawBody["shared"] != true {
		t.Fatalf("unexpected flags: %v", sawBody)
	}
	if sawBody["prune-backups"] != "keep-last=3" || sawBody["max-protected-backups"] != float64(5) {
		t.Fatalf("unexpected retention fields: %v", sawBody)
	}
	for _, key := range []string{"share", "username", "password", "domain", "smbversion", "portal", "target", "iscsiprovider", "nowritecache"} {
		if _, ok := sawBody[key]; ok {
			t.Fatalf("%s must not be sent for a nfs storage", key)
		}
	}
}

// TestStorageNetCreate_CIFS verifies POST /storage for type cifs including
// the secret-bearing fields and the smbversion wire name.
func TestStorageNetCreate_CIFS(t *testing.T) {
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
	s := StorageNet{
		Storage:    "archive",
		Type:       "cifs",
		Server:     "192.168.1.10",
		Share:      "archive",
		Username:   "backup",
		Password:   "s3cret",
		Domain:     "CORP",
		SMBVersion: "3.11",
		Content:    "backup",
	}
	if err := c.CreateStorageNet(context.Background(), s); err != nil {
		t.Fatalf("CreateStorageNet: %v", err)
	}
	if sawBody["storage"] != "archive" || sawBody["type"] != "cifs" || sawBody["server"] != "192.168.1.10" || sawBody["share"] != "archive" {
		t.Fatalf("unexpected identity scalars: %v", sawBody)
	}
	if sawBody["username"] != "backup" || sawBody["password"] != "s3cret" || sawBody["domain"] != "CORP" || sawBody["smbversion"] != "3.11" {
		t.Fatalf("unexpected cifs fields: %v", sawBody)
	}
	for _, key := range []string{"export", "portal", "target", "iscsiprovider", "nowritecache"} {
		if _, ok := sawBody[key]; ok {
			t.Fatalf("%s must not be sent for a cifs storage", key)
		}
	}
}

// TestStorageNetCreate_ISCSI verifies POST /storage for both iSCSI types:
// portal and target travel, and only iscsidirect carries nowritecache.
func TestStorageNetCreate_ISCSI(t *testing.T) {
	var sawPath string
	var sawBody map[string]any
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/storage" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		sawPath = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &sawBody); err != nil {
			t.Fatalf("request body not JSON: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":null}`)
	})
	iscsi := StorageNet{
		Storage:       "san",
		Type:          "iscsi",
		Portal:        "192.168.1.10:3260",
		Target:        "iqn.2000-01.com.example:san.target0",
		ISCSIProvider: "LIO",
		Content:       "images",
	}
	if err := c.CreateStorageNet(context.Background(), iscsi); err != nil {
		t.Fatalf("CreateStorageNet iscsi: %v", err)
	}
	if sawPath != "/storage" || sawBody["type"] != "iscsi" || sawBody["portal"] != "192.168.1.10:3260" || sawBody["target"] != "iqn.2000-01.com.example:san.target0" || sawBody["iscsiprovider"] != "LIO" {
		t.Fatalf("unexpected iscsi create body: %v", sawBody)
	}
	if _, ok := sawBody["nowritecache"]; ok {
		t.Fatalf("nowritecache must not be sent for a iscsi storage")
	}

	direct := StorageNet{
		Storage:      "fast",
		Type:         "iscsidirect",
		Portal:       "192.168.1.10",
		Target:       "iqn.2000-01.com.example:fast.target0",
		NoWriteCache: boolPtr(true),
		Content:      "images",
	}
	sawBody = nil
	if err := c.CreateStorageNet(context.Background(), direct); err != nil {
		t.Fatalf("CreateStorageNet iscsidirect: %v", err)
	}
	if sawBody["type"] != "iscsidirect" || sawBody["portal"] != "192.168.1.10" || sawBody["nowritecache"] != true {
		t.Fatalf("unexpected iscsidirect create body: %v", sawBody)
	}
	if _, ok := sawBody["iscsiprovider"]; ok {
		t.Fatalf("iscsiprovider must not be sent for a iscsidirect storage")
	}
}

// TestStorageNetCreate_MissingRequired confirms the client rejects creates
// missing the type's defining fields before hitting the wire.
func TestStorageNetCreate_MissingRequired(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("no request should reach the wire: %s %s", r.Method, r.URL.Path)
	})
	ctx := context.Background()
	cases := []struct {
		name string
		body StorageNet
		want string
	}{
		{"nfs without export", StorageNet{Storage: "nfsvm", Type: "nfs", Server: "192.168.1.10"}, "export"},
		{"nfs without server", StorageNet{Storage: "nfsvm", Type: "nfs", Export: "/srv"}, "server"},
		{"cifs without share", StorageNet{Storage: "archive", Type: "cifs", Server: "192.168.1.10"}, "share"},
		{"iscsi without portal", StorageNet{Storage: "san", Type: "iscsi", Target: "iqn.1"}, "portal"},
		{"iscsidirect without target", StorageNet{Storage: "fast", Type: "iscsidirect", Portal: "192.168.1.10"}, "target"},
		{"unsupported type", StorageNet{Storage: "x", Type: "zfspool"}, "unsupported type"},
		{"missing storage id", StorageNet{Type: "nfs"}, "storage is required"},
		{"missing type", StorageNet{Storage: "nfsvm"}, "type is required"},
	}
	for _, tc := range cases {
		err := c.CreateStorageNet(ctx, tc.body)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%s: err = %v, want containing %q", tc.name, err, tc.want)
		}
	}
}

// TestStorageNetGet decodes GET /storage/{storage}: the 0/1 encoding of
// disable, the boolean encoding of shared, the string encoding of numeric
// settings, and the read-only digest.
func TestStorageNetGet(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/storage/nfsvm" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"storage":"nfsvm","type":"nfs","server":"192.168.1.10",`+
			`"export":"/srv/export","content":"images,iso","nodes":"pve1","options":"vers=4.2",`+
			`"disable":1,"shared":true,"max-protected-backups":"5","prune-backups":"keep-last=3",`+
			`"digest":"cafe1234"}}`)
	})
	s, err := c.GetStorageNet(context.Background(), "nfsvm")
	if err != nil {
		t.Fatalf("GetStorageNet: %v", err)
	}
	if s.Storage != "nfsvm" || s.Type != "nfs" || s.Server != "192.168.1.10" || s.Export != "/srv/export" {
		t.Fatalf("unexpected identity scalars: %+v", s)
	}
	if s.Content != "images,iso" || s.Nodes != "pve1" || s.Options != "vers=4.2" {
		t.Fatalf("unexpected list fields: %+v", s)
	}
	if s.Disable == nil || !*s.Disable {
		t.Fatalf("disable = %v, want true (0/1 encoding)", s.Disable)
	}
	if s.Shared == nil || !*s.Shared {
		t.Fatalf("shared = %v, want true (boolean encoding)", s.Shared)
	}
	if s.MaxProtectedBackups == nil || *s.MaxProtectedBackups != 5 {
		t.Fatalf("max-protected-backups = %v, want 5 (string encoding)", s.MaxProtectedBackups)
	}
	if s.PruneBackups != "keep-last=3" || s.Digest != "cafe1234" {
		t.Fatalf("unexpected retention/digest: %+v", s)
	}
}

// TestStorageNetGet_ISCSIDirect decodes the iscsidirect field set.
func TestStorageNetGet_ISCSIDirect(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"storage":"fast","type":"iscsidirect",`+
			`"portal":"192.168.1.10","target":"iqn.2000-01.com.example:fast.target0",`+
			`"content":"images","nowritecache":0}}`)
	})
	s, err := c.GetStorageNet(context.Background(), "fast")
	if err != nil {
		t.Fatalf("GetStorageNet: %v", err)
	}
	if s.Type != "iscsidirect" || s.Portal != "192.168.1.10" || s.Target != "iqn.2000-01.com.example:fast.target0" {
		t.Fatalf("unexpected iscsidirect scalars: %+v", s)
	}
	if s.NoWriteCache == nil || *s.NoWriteCache {
		t.Fatalf("nowritecache = %v, want false (0/1 encoding)", s.NoWriteCache)
	}
}

// TestStorageNetUpdate verifies PUT /storage: the body carries the
// updatable fields but never `type` and never the pin's create-only
// location fields, and the delete slice travels as the `delete` query
// parameter.
func TestStorageNetUpdate(t *testing.T) {
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
	s := StorageNet{
		Type:          "cifs",
		Server:        "192.168.1.20",
		Export:        "/must/not/travel",
		Share:         "must-not-travel",
		Portal:        "must:3260",
		Target:        "iqn.1",
		ISCSIProvider: "must",
		Username:      "backup",
		Content:       "backup,iso",
	}
	if err := c.UpdateStorageNet(context.Background(), "archive", s, []string{"domain", "smbversion"}); err != nil {
		t.Fatalf("UpdateStorageNet: %v", err)
	}
	if sawBody["storage"] != "archive" || sawBody["server"] != "192.168.1.20" || sawBody["content"] != "backup,iso" {
		t.Fatalf("unexpected update body: %v", sawBody)
	}
	if sawQuery.Get("delete") != "domain,smbversion" {
		t.Fatalf("delete param = %q, want domain,smbversion", sawQuery.Get("delete"))
	}
	for _, key := range []string{"type", "export", "share", "portal", "target", "iscsiprovider"} {
		if _, ok := sawBody[key]; ok {
			t.Fatalf("%s must not be sent on update (the pin's PUT verb does not accept it)", key)
		}
	}
}

// TestStorageNetDelete confirms DELETE /storage/{storage} and that a 404
// surfaces as *APIError so the resource layer can map it to already absent.
func TestStorageNetDelete(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/storage/nfsvm" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":null}`)
	})
	if err := c.DeleteStorageNet(context.Background(), "nfsvm"); err != nil {
		t.Fatalf("DeleteStorageNet: %v", err)
	}

	missing := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"errors":"storage 'nfsvm' does not exist"}`)
	})
	err := missing.DeleteStorageNet(context.Background(), "nfsvm")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusNotFound {
		t.Fatalf("DeleteStorageNet on missing storage = %v (%T), want wrapped *APIError 404", err, err)
	}
}
