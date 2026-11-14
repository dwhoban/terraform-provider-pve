// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"sync"
	"testing"

	"github.com/dwhoban/terraform-provider-pve/internal/pveclient"
)

type fakeAptEntry struct {
	uri, component string
	enabled        bool
}

type fakeAptHandle struct{ handle, name, uri, component string }

// aptFake is a minimal PVE /nodes/{node}/apt/repositories.
type aptFake struct {
	mu             sync.Mutex
	files          map[string][]fakeAptEntry
	fileOrder      []string
	handles        []fakeAptHandle
	statusOverride map[string]bool
	mutations      []string
}

func newAptFake() *aptFake {
	const ent = "/etc/apt/sources.list.d/pve-enterprise.sources"
	return &aptFake{
		files:     map[string][]fakeAptEntry{ent: {{"https://enterprise.proxmox.com/debian/pve", "pve-enterprise", true}}},
		fileOrder: []string{ent},
		handles: []fakeAptHandle{
			{"enterprise", "Enterprise", "https://enterprise.proxmox.com/debian/pve", "pve-enterprise"},
			{"no-subscription", "No-Subscription", "http://download.proxmox.com/debian/pve", "pve-no-subscription"},
			{"test", "Test", "http://download.proxmox.com/debian/pve", "pve-test"},
			{"ceph-squid-no-subscription", "Ceph Squid No-Subscription", "http://download.proxmox.com/debian/ceph-squid", "no-subscription"},
		},
		statusOverride: map[string]bool{},
	}
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (f *aptFake) find(h fakeAptHandle) (path string, idx int) {
	for _, p := range f.fileOrder {
		for i, e := range f.files[p] {
			if e.uri == h.uri && e.component == h.component {
				return p, i
			}
		}
	}
	return "", -1
}

func (f *aptFake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	_ = r.ParseForm()
	switch r.Method {
	case http.MethodPut:
		handle := r.Form.Get("handle")
		f.mutations = append(f.mutations, "PUT handle="+handle)
		for _, h := range f.handles {
			if h.handle != handle {
				continue
			}
			if p, i := f.find(h); i >= 0 {
				f.files[p][i].enabled = true
			} else {
				const p = "/etc/apt/sources.list.d/proxmox.sources"
				if _, ok := f.files[p]; !ok {
					f.fileOrder = append(f.fileOrder, p)
				}
				f.files[p] = append(f.files[p], fakeAptEntry{h.uri, h.component, true})
			}
		}
		_, _ = w.Write([]byte(`{"data":null}`))
	case http.MethodPost:
		p, idx := r.Form.Get("path"), r.Form.Get("index")
		f.mutations = append(f.mutations, fmt.Sprintf("POST path=%s index=%s enabled=%s", p, idx, r.Form.Get("enabled")))
		i, _ := strconv.Atoi(idx)
		f.files[p][i].enabled = r.Form.Get("enabled") == "1"
		_, _ = w.Write([]byte(`{"data":null}`))
	default:
		var files, std []map[string]any
		for _, p := range f.fileOrder {
			var repos []map[string]any
			for _, e := range f.files[p] {
				repos = append(repos, map[string]any{
					"Types": []string{"deb"}, "URIs": []string{e.uri},
					"Suites": []string{"trixie"}, "Components": []string{e.component},
					"Enabled": b2i(e.enabled),
				})
			}
			files = append(files, map[string]any{"path": p, "repositories": repos})
		}
		for _, h := range f.handles {
			s := map[string]any{"handle": h.handle, "name": h.name}
			if p, i := f.find(h); i >= 0 {
				s["status"] = f.files[p][i].enabled
			}
			if v, ok := f.statusOverride[h.handle]; ok {
				s["status"] = v
			}
			std = append(std, s)
		}
		b, _ := json.Marshal(map[string]any{"data": map[string]any{"files": files, "standard-repos": std}})
		_, _ = w.Write(b)
	}
}

func aptTestResource(t *testing.T) (*aptStandardRepositoryResource, *aptFake) {
	t.Helper()
	fake := newAptFake()
	srv := httptest.NewTLSServer(fake)
	t.Cleanup(srv.Close)
	c, err := pveclient.NewClient(pveclient.Config{
		URL: srv.URL, Insecure: true, RetryMax: 0,
		Credentials: pveclient.Credentials{APIToken: "root@pam!t=s"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return &aptStandardRepositoryResource{client: c}, fake
}

func TestAptStandardRepositoryEnable(t *testing.T) {
	r, fake := aptTestResource(t)
	name, _, ok, diags := r.reconcile(context.Background(), "pve1", "no-subscription", true)
	if diags.HasError() || !ok {
		t.Fatalf("ok=%v diags=%v", ok, diags)
	}
	if name != "No-Subscription" {
		t.Errorf("name = %q", name)
	}
	if want := []string{"PUT handle=no-subscription"}; !reflect.DeepEqual(fake.mutations, want) {
		t.Errorf("mutations = %v, want %v", fake.mutations, want)
	}
	resp, err := readAptRepositories(context.Background(), r.client, "pve1")
	if err != nil {
		t.Fatal(err)
	}
	if s, _ := resp.standard("no-subscription"); !s.enabled() {
		t.Error("no-subscription not enabled after reconcile")
	}
}

func TestAptStandardRepositoryDisable(t *testing.T) {
	r, fake := aptTestResource(t)
	_, _, ok, diags := r.reconcile(context.Background(), "pve1", "enterprise", false)
	if diags.HasError() || !ok {
		t.Fatalf("ok=%v diags=%v", ok, diags)
	}
	want := []string{"POST path=/etc/apt/sources.list.d/pve-enterprise.sources index=0 enabled=0"}
	if !reflect.DeepEqual(fake.mutations, want) {
		t.Errorf("mutations = %v, want %v", fake.mutations, want)
	}
}

func TestAptStandardRepositoryNoop(t *testing.T) {
	r, fake := aptTestResource(t)
	for _, c := range []struct {
		handle string
		want   bool
	}{{"enterprise", true}, {"test", false}} {
		if _, _, ok, diags := r.reconcile(context.Background(), "pve1", c.handle, c.want); diags.HasError() || !ok {
			t.Fatalf("%s: ok=%v diags=%v", c.handle, ok, diags)
		}
	}
	if len(fake.mutations) != 0 {
		t.Errorf("mutations = %v, want none", fake.mutations)
	}
}

func TestAptStandardRepositoryUnknownHandle(t *testing.T) {
	r, fake := aptTestResource(t)
	_, offered, ok, diags := r.reconcile(context.Background(), "pve1", "ceph-reef-enterprise", true)
	if diags.HasError() || ok {
		t.Fatalf("ok=%v diags=%v", ok, diags)
	}
	want := []string{"enterprise", "no-subscription", "test", "ceph-squid-no-subscription"}
	if !reflect.DeepEqual(offered, want) {
		t.Errorf("offered = %v, want %v", offered, want)
	}
	if len(fake.mutations) != 0 {
		t.Errorf("mutations = %v, want none", fake.mutations)
	}
}

func TestAptStandardRepositoryMirrorNotMatched(t *testing.T) {
	r, fake := aptTestResource(t)
	const p = "/etc/apt/sources.list.d/mirror.sources"
	fake.files[p] = []fakeAptEntry{{"http://mirror.local/pve", "pve-no-subscription", true}}
	fake.fileOrder = append(fake.fileOrder, p)
	fake.statusOverride["no-subscription"] = true

	_, _, _, diags := r.reconcile(context.Background(), "pve1", "no-subscription", false)
	if !diags.HasError() {
		t.Fatalf("diags=%v, want error", diags)
	}
	if got := diags.Errors()[0].Summary(); got != "Repository Entry Not Found" {
		t.Errorf("summary = %q", got)
	}
	if len(fake.mutations) != 0 {
		t.Errorf("mutations = %v, want none", fake.mutations)
	}
}

func TestAptHandleMatch(t *testing.T) {
	const ent, dl = "https://enterprise.proxmox.com/debian", "http://download.proxmox.com/debian"
	for _, c := range []struct {
		handle     string
		ok         bool
		uris, comp []string
	}{
		{"enterprise", true, []string{ent + "/pve", ent}, []string{"pve-enterprise"}},
		{"test", true, []string{dl + "/pve", dl}, []string{"pve-test", "pvetest"}},
		{"ceph-squid-no-subscription", true, []string{dl + "/ceph-squid"}, []string{"no-subscription"}},
		{"ceph-tentacle-enterprise", true, []string{ent + "/ceph-tentacle"}, []string{"enterprise"}},
		{"bogus", false, nil, nil},
		{"ceph-test", false, nil, nil},
	} {
		uris, comp, ok := aptHandleMatch(c.handle)
		if ok != c.ok || !reflect.DeepEqual(uris, c.uris) || !reflect.DeepEqual(comp, c.comp) {
			t.Errorf("%s: got (%v, %v, %v), want (%v, %v, %v)", c.handle, uris, comp, ok, c.uris, c.comp, c.ok)
		}
	}
}
