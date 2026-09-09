// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

// TestClient_ListNodeServices decodes the service list, including the
// hyphenated systemd state keys the pin defines.
func TestClient_ListNodeServices(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/nodes/pve1/services" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"name":"pveproxy","service":"pveproxy.service","desc":"PVE API Proxy Server","state":"running","active-state":"active","unit-state":"enabled"},{"name":"cron","service":"cron.service","desc":"Regular background program processing daemon","state":"dead","active-state":"inactive"}]}`)
	})
	svcs, err := c.ListNodeServices(context.Background(), "pve1")
	if err != nil {
		t.Fatalf("ListNodeServices: %v", err)
	}
	if len(svcs) != 2 {
		t.Fatalf("len = %d, want 2", len(svcs))
	}
	if svcs[0].Name != "pveproxy" || svcs[0].Description != "PVE API Proxy Server" || svcs[0].ActiveState != "active" || svcs[0].UnitState != "enabled" {
		t.Fatalf("first row = %+v", svcs[0])
	}
	if svcs[1].ActiveState != "inactive" || svcs[1].State != "dead" {
		t.Fatalf("second row = %+v", svcs[1])
	}
}

// TestClient_NodeServiceOp_PostsVerbAndReturnsUpid verifies the verb is
// appended to the path and the returned UPID is unwrapped.
func TestClient_NodeServiceOp_PostsVerbAndReturnsUpid(t *testing.T) {
	upid := "UPID:pve1:00001234:12345678:svcrestart:root@pam:"
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/nodes/pve1/services/pveproxy/restart" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":"`+upid+`"}`)
	})
	got, err := c.NodeServiceOp(context.Background(), "pve1", "pveproxy", "restart")
	if err != nil {
		t.Fatalf("NodeServiceOp: %v", err)
	}
	if got != upid {
		t.Fatalf("got upid %q, want %q", got, upid)
	}
}

// TestClient_NodeServiceOp_RejectsUnknownOperation verifies unknown verbs
// fail locally without issuing an HTTP request.
func TestClient_NodeServiceOp_RejectsUnknownOperation(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("no request expected, got %s %s", r.Method, r.URL.Path)
	})
	if _, err := c.NodeServiceOp(context.Background(), "pve1", "pveproxy", "dance"); err == nil {
		t.Fatal("expected error for unknown operation")
	}
}

// TestClient_GetNodeAptRepositories decodes the full repositories payload:
// digest, errors, per-file repositories, infos, and standard-repos with an
// absent status on unconfigured entries.
func TestClient_GetNodeAptRepositories(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/nodes/pve1/apt/repositories" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"digest":"a1b2c3","errors":[{"path":"/etc/apt/sources.list.d/broken.list","error":"parse error"}],"files":[{"file-type":"list","path":"/etc/apt/sources.list","repositories":[{"Comment":" security repo","Components":["main"],"Enabled":true,"FileType":"list","Suites":["bookworm/updates"],"Types":["deb"],"URIs":["http://security.debian.org"]}]},{"file-type":"sources","path":"/etc/apt/sources.list.d/pve-enterprise.sources","repositories":[{"Components":["pve-enterprise"],"Enabled":false,"FileType":"sources","Options":[{"Key":"Signed-By","Values":["/usr/share/keyrings/proxmox-archive-keyring.gpg"]}],"Suites":["bookworm"],"Types":["deb"],"URIs":["https://enterprise.proxmox.com/debian/pve"]}]}],"infos":[{"index":"0","kind":"warning","message":"disabled","path":"/etc/apt/sources.list.d/pve-enterprise.sources","property":"Enabled"}],"standard-repos":[{"handle":"enterprise","name":"Proxmox VE Enterprise Repository","status":true},{"handle":"no-subscription","name":"Proxmox VE No-Subscription Repository","status":false},{"handle":"ceph-quincy","name":"Ceph Quincy Repository"}]}}`)
	})
	repos, err := c.GetNodeAptRepositories(context.Background(), "pve1")
	if err != nil {
		t.Fatalf("GetNodeAptRepositories: %v", err)
	}
	if repos.Digest != "a1b2c3" {
		t.Fatalf("digest = %q", repos.Digest)
	}
	if len(repos.Errors) != 1 || repos.Errors[0].Path != "/etc/apt/sources.list.d/broken.list" {
		t.Fatalf("errors = %+v", repos.Errors)
	}
	if len(repos.Infos) != 1 || repos.Infos[0].Kind != "warning" || repos.Infos[0].Property != "Enabled" {
		t.Fatalf("infos = %+v", repos.Infos)
	}
	if len(repos.Files) != 2 {
		t.Fatalf("files = %+v", repos.Files)
	}
	first := repos.Files[0].Repositories[0]
	if !first.Enabled || len(first.Components) != 1 || first.Components[0] != "main" || first.Comment != " security repo" {
		t.Fatalf("first repo = %+v", first)
	}
	second := repos.Files[1].Repositories[0]
	if second.Enabled || second.FileType != "sources" || len(second.Options) != 1 || second.Options[0].Key != "Signed-By" || len(second.Options[0].Values) != 1 {
		t.Fatalf("second repo = %+v", second)
	}
	if len(repos.StandardRepositories) != 3 {
		t.Fatalf("standard-repos = %+v", repos.StandardRepositories)
	}
	if repos.StandardRepositories[0].Handle != "enterprise" || repos.StandardRepositories[0].Status == nil || !*repos.StandardRepositories[0].Status {
		t.Fatalf("enterprise row = %+v", repos.StandardRepositories[0])
	}
	if repos.StandardRepositories[2].Status != nil {
		t.Fatalf("unconfigured ceph row should have nil status: %+v", repos.StandardRepositories[2])
	}
}

// TestClient_NodeAptUpdate_PostsFlagsAndReturnsUpid verifies the boolean
// flags reach the POST body and the UPID is unwrapped.
func TestClient_NodeAptUpdate_PostsFlagsAndReturnsUpid(t *testing.T) {
	upid := "UPID:pve1:00004321:12345678:aptupdate:root@pam:"
	var sawBody []byte
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/nodes/pve1/apt/update" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		sawBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":"`+upid+`"}`)
	})
	quiet := true
	got, err := c.NodeAptUpdate(context.Background(), "pve1", NodeAptUpdateInput{Quiet: &quiet})
	if err != nil {
		t.Fatalf("NodeAptUpdate: %v", err)
	}
	if got != upid {
		t.Fatalf("got upid %q, want %q", got, upid)
	}
	if !strings.Contains(string(sawBody), `"quiet":true`) {
		t.Fatalf("body missing quiet flag: %q", sawBody)
	}
}

// TestClient_AddAptStandardRepository_PutsHandle verifies the add form is a
// PUT carrying only the handle, matching the pin's add_repository verb.
func TestClient_AddAptStandardRepository_PutsHandle(t *testing.T) {
	var sawBody []byte
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/nodes/pve1/apt/repositories" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		sawBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":null}`)
	})
	if err := c.AddAptStandardRepository(context.Background(), "pve1", "no-subscription"); err != nil {
		t.Fatalf("AddAptStandardRepository: %v", err)
	}
	if string(sawBody) != `{"handle":"no-subscription"}` {
		t.Fatalf("body = %q", sawBody)
	}
}

// TestClient_ChangeAptRepository_PostsBody verifies the change form POSTs
// index, path, and the enabled flag.
func TestClient_ChangeAptRepository_PostsBody(t *testing.T) {
	var sawBody []byte
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/nodes/pve1/apt/repositories" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		sawBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":null}`)
	})
	in := ChangeAptRepositoryInput{Index: 1, Path: "/etc/apt/sources.list", Enabled: boolPtr(false), Digest: "a1b2c3"}
	if err := c.ChangeAptRepository(context.Background(), "pve1", in); err != nil {
		t.Fatalf("ChangeAptRepository: %v", err)
	}
	for _, want := range []string{`"index":1`, `"path":"/etc/apt/sources.list"`, `"enabled":false`, `"digest":"a1b2c3"`} {
		if !strings.Contains(string(sawBody), want) {
			t.Fatalf("body %q missing %s", sawBody, want)
		}
	}
}

// TestClient_GetNodeAptChangelog verifies the changelog endpoint receives
// the package name as a query parameter and the raw changelog string is
// unwrapped from the data envelope.
func TestClient_GetNodeAptChangelog(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/nodes/pve1/apt/changelog" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.URL.Query().Get("name"); got != "pve-manager" {
			t.Fatalf("name query = %q, want pve-manager", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":"pve-manager (8.2.2) stable; urgency=medium\n\n  * Update based on Debian 12.5\n"}`)
	})
	changelog, err := c.GetNodeAptChangelog(context.Background(), "pve1", "pve-manager")
	if err != nil {
		t.Fatalf("GetNodeAptChangelog: %v", err)
	}
	if !strings.Contains(changelog, "pve-manager (8.2.2)") || !strings.Contains(changelog, "urgency=medium") {
		t.Fatalf("changelog = %q", changelog)
	}
}
