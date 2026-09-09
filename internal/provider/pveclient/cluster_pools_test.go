// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

// TestClient_ListPools_Decodes covers GET /pools: the array envelope decodes
// into Pool entries with optional comment and members.
func TestClient_ListPools_Decodes(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/pools" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[`+
			`{"poolid":"prod","comment":"production pool"},`+
			`{"poolid":"lab","members":[{"id":"qemu/100","node":"pve1","type":"qemu","vmid":100},{"id":"storage/local","node":"pve1","storage":"local","type":"storage"}]}`+
			`]}`)
	})
	pools, err := c.ListPools(context.Background())
	if err != nil {
		t.Fatalf("ListPools: %v", err)
	}
	if len(pools) != 2 {
		t.Fatalf("len(pools) = %d, want 2", len(pools))
	}
	if pools[0].PoolID != "prod" || pools[0].Comment != "production pool" || len(pools[0].Members) != 0 {
		t.Fatalf("pools[0] = %+v", pools[0])
	}
	if pools[1].PoolID != "lab" || len(pools[1].Members) != 2 {
		t.Fatalf("pools[1] = %+v", pools[1])
	}
	vm := pools[1].Members[0]
	if vm.ID != "qemu/100" || vm.Node != "pve1" || vm.Type != "qemu" || vm.VMID == nil || *vm.VMID != 100 || vm.Storage != "" {
		t.Fatalf("member[0] = %+v", vm)
	}
	st := pools[1].Members[1]
	if st.Storage != "local" || st.Type != "storage" || st.VMID != nil {
		t.Fatalf("member[1] = %+v", st)
	}
}

// TestClient_GetPool_Decodes covers GET /pools/{poolid}, which returns the
// pool configuration as a single-element array per the pin; an empty array
// decodes as a nil pool.
func TestClient_GetPool_Decodes(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/pools/prod" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"poolid":"prod","comment":"production pool","members":[{"id":"qemu/100","node":"pve1","type":"qemu","vmid":100}]}]}`)
	})
	pool, err := c.GetPool(context.Background(), "prod")
	if err != nil {
		t.Fatalf("GetPool: %v", err)
	}
	if pool == nil || pool.PoolID != "prod" || pool.Comment != "production pool" || len(pool.Members) != 1 {
		t.Fatalf("pool = %+v", pool)
	}

	empty := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[]}`)
	})
	pool, err = empty.GetPool(context.Background(), "gone")
	if err != nil {
		t.Fatalf("GetPool (empty): %v", err)
	}
	if pool != nil {
		t.Fatalf("empty array should decode as nil pool, got %+v", pool)
	}
}

// TestClient_CreatePool_BodyShape asserts POST /pools sends poolid and an
// optional comment.
func TestClient_CreatePool_BodyShape(t *testing.T) {
	var body []byte
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/pools" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		body, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":null}`)
	})
	if err := c.CreatePool(context.Background(), "prod", "production pool"); err != nil {
		t.Fatalf("CreatePool: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal body %s: %v", body, err)
	}
	if got["poolid"] != "prod" || got["comment"] != "production pool" {
		t.Fatalf("body = %s", body)
	}

	body = nil
	bare := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":null}`)
	})
	if err := bare.CreatePool(context.Background(), "bare", ""); err != nil {
		t.Fatalf("CreatePool (no comment): %v", err)
	}
	got = map[string]any{}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal body %s: %v", body, err)
	}
	if _, ok := got["comment"]; ok {
		t.Fatalf("comment should be omitted when empty, body = %s", body)
	}
}

// TestClient_UpdatePool_MemberDiffWire asserts PUT /pools/{poolid} renders
// the add form (vms/storage joined, delete absent) and the remove form
// (delete: true), plus comment and allow-move.
func TestClient_UpdatePool_MemberDiffWire(t *testing.T) {
	var body []byte
	var sawQuery string
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/pools/prod" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		sawQuery = r.URL.RawQuery
		body, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":null}`)
	})

	// Add members.
	err := c.UpdatePool(context.Background(), "prod", PoolUpdate{
		VMs:       []int64{100, 101},
		Storages:  []string{"local", "ceph"},
		AllowMove: PoolBoolPtr(true),
	})
	if err != nil {
		t.Fatalf("UpdatePool (add): %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal body %s: %v", body, err)
	}
	if got["vms"] != "100,101" || got["storage"] != "local,ceph" {
		t.Fatalf("add body = %s", body)
	}
	if got["delete"] != nil || got["allow-move"] != true {
		t.Fatalf("add body flags wrong: %s", body)
	}
	if sawQuery != "" {
		t.Fatalf("add call should carry no query, got %q", sawQuery)
	}

	// Remove members.
	err = c.UpdatePool(context.Background(), "prod", PoolUpdate{
		VMs:      []int64{101},
		Storages: []string{"ceph"},
		Delete:   true,
	})
	if err != nil {
		t.Fatalf("UpdatePool (remove): %v", err)
	}
	got = nil
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal body %s: %v", body, err)
	}
	if got["delete"] != true || got["vms"] != "101" || got["storage"] != "ceph" {
		t.Fatalf("remove body = %s", body)
	}
	if got["allow-move"] != nil {
		t.Fatalf("remove body should omit allow-move: %s", body)
	}

	// Comment-only update.
	err = c.UpdatePool(context.Background(), "prod", PoolUpdate{Comment: "new comment"})
	if err != nil {
		t.Fatalf("UpdatePool (comment): %v", err)
	}
	got = nil
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal body %s: %v", body, err)
	}
	if got["comment"] != "new comment" || len(got) != 1 {
		t.Fatalf("comment body = %s", body)
	}
}

// TestClient_DeletePool_Wire asserts DELETE /pools/{poolid}.
func TestClient_DeletePool_Wire(t *testing.T) {
	var sawMethod, sawPath string
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		sawMethod, sawPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":null}`)
	})
	if err := c.DeletePool(context.Background(), "prod"); err != nil {
		t.Fatalf("DeletePool: %v", err)
	}
	if sawMethod != http.MethodDelete || sawPath != "/pools/prod" {
		t.Fatalf("request = %s %s", sawMethod, sawPath)
	}
}
