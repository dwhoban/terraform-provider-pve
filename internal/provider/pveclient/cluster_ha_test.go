// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

// TestHA_ArmHa verifies ArmHA POSTs /cluster/ha/status/arm-ha with no body.
func TestHA_ArmHa(t *testing.T) {
	var sawBody []byte
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/cluster/ha/status/arm-ha" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		sawBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":null}`)
	})
	if err := c.ArmHA(context.Background()); err != nil {
		t.Fatalf("ArmHA: %v", err)
	}
	if len(sawBody) != 0 {
		t.Fatalf("arm-ha must not carry a body, got %q", sawBody)
	}
}

// TestHA_DisarmHa verifies DisarmHA POSTs /cluster/ha/status/disarm-ha and
// only includes resource-mode when the caller supplies one.
func TestHA_DisarmHa(t *testing.T) {
	var sawBody []byte
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/cluster/ha/status/disarm-ha" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		sawBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":null}`)
	})
	if err := c.DisarmHA(context.Background(), "freeze"); err != nil {
		t.Fatalf("DisarmHA: %v", err)
	}
	var sent map[string]any
	if err := json.Unmarshal(sawBody, &sent); err != nil {
		t.Fatalf("body %q is not JSON: %v", sawBody, err)
	}
	if sent["resource-mode"] != "freeze" {
		t.Fatalf("resource-mode = %v, want freeze", sent["resource-mode"])
	}

	if err := c.DisarmHA(context.Background(), ""); err != nil {
		t.Fatalf("DisarmHA without mode: %v", err)
	}
	if strings.Contains(string(sawBody), "resource-mode") {
		t.Fatalf("resource-mode must be omitted when empty, got %q", sawBody)
	}
}

// TestHA_GetStatus decodes the /cluster/ha/status/current entry array,
// including the boolish int encodings and the hyphenated wire keys.
func TestHA_GetStatus(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/cluster/ha/status/current" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[`+
			`{"id":"quorum","type":"quorum","node":"pve1","quorate":1,"status":"OK"},`+
			`{"id":"master","type":"master","node":"pve1","status":"M","timestamp":1700000000},`+
			`{"id":"lrm:pve2","type":"lrm","node":"pve2","status":"idle"},`+
			`{"id":"service:vm:100","type":"service","node":"pve1","sid":"vm:100","state":"started","crm_state":"started","request_state":"started","max_restart":3,"max_relocate":1,"failback":1,"auto-rebalance":0}]}`)
	})
	entries, err := c.GetHAStatus(context.Background())
	if err != nil {
		t.Fatalf("GetHAStatus: %v", err)
	}
	if len(entries) != 4 {
		t.Fatalf("got %d entries, want 4", len(entries))
	}
	quorum := entries[0]
	if quorum.Type != "quorum" || quorum.Quorate == nil || !*quorum.Quorate || quorum.Status != "OK" {
		t.Fatalf("quorum entry = %+v", quorum)
	}
	svc := entries[3]
	if svc.SID != "vm:100" || svc.CRMState != "started" || svc.RequestState != "started" {
		t.Fatalf("service entry = %+v", svc)
	}
	if svc.MaxRestart == nil || *svc.MaxRestart != 3 {
		t.Fatalf("max_restart = %+v, want 3", svc.MaxRestart)
	}
	if svc.Failback == nil || !*svc.Failback {
		t.Fatalf("failback = %+v, want true", svc.Failback)
	}
	if svc.AutoRebalance == nil || *svc.AutoRebalance {
		t.Fatalf("auto-rebalance = %+v, want false", svc.AutoRebalance)
	}
}

// TestHA_GetManagerStatus surfaces the manager_status document verbatim as
// raw JSON.
func TestHA_GetManagerStatus(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/cluster/ha/status/manager_status" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"manager_status":{"master_state":"wait for quorum","quorate":1},"nodes":{"pve1":{"rate":""}}}}`)
	})
	raw, err := c.GetHAManagerStatus(context.Background())
	if err != nil {
		t.Fatalf("GetHAManagerStatus: %v", err)
	}
	var doc struct {
		ManagerStatus struct {
			MasterState string `json:"master_state"`
		} `json:"manager_status"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil || doc.ManagerStatus.MasterState != "wait for quorum" {
		t.Fatalf("manager_status decode = %s", raw)
	}
}

// TestHA_Groups_CRUD covers the group wire set: create body, list and read
// decode with node-list splitting and boolish flags, update with delete
// query, and delete.
func TestHA_Groups_CRUD(t *testing.T) {
	var lastBody []byte
	var lastQuery string
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		lastBody, _ = io.ReadAll(r.Body)
		lastQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/cluster/ha/groups":
			_, _ = io.WriteString(w, `{"data":null}`)
		case r.Method == http.MethodGet && r.URL.Path == "/cluster/ha/groups":
			_, _ = io.WriteString(w, `{"data":[{"group":"g1","nodes":"n1:2,n2","restricted":1,"nofailback":0,"comment":"core"}]}`)
		case r.Method == http.MethodGet && r.URL.Path == "/cluster/ha/groups/g1":
			_, _ = io.WriteString(w, `{"data":{"group":"g1","nodes":"n1:2,n2","restricted":true,"nofailback":false,"digest":"d1"}}`)
		case r.Method == http.MethodPut && r.URL.Path == "/cluster/ha/groups/g1":
			_, _ = io.WriteString(w, `{"data":null}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/cluster/ha/groups/g1":
			_, _ = io.WriteString(w, `{"data":null}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	ctx := context.Background()

	group := HAGroup{Group: "g1", Nodes: []string{"n1:2", "n2"}, Restricted: HABoolPtr(true), Comment: "core"}
	if err := c.CreateHAGroup(ctx, group); err != nil {
		t.Fatalf("CreateHAGroup: %v", err)
	}
	var sent map[string]any
	if err := json.Unmarshal(lastBody, &sent); err != nil {
		t.Fatalf("create body %q is not JSON: %v", lastBody, err)
	}
	if sent["nodes"] != "n1:2,n2" || sent["group"] != "g1" || sent["restricted"] != true {
		t.Fatalf("create body = %v", sent)
	}

	groups, err := c.ListHAGroups(ctx)
	if err != nil {
		t.Fatalf("ListHAGroups: %v", err)
	}
	if len(groups) != 1 || groups[0].Group != "g1" {
		t.Fatalf("groups = %+v", groups)
	}
	g := groups[0]
	if len(g.Nodes) != 2 || g.Nodes[0] != "n1:2" || g.Nodes[1] != "n2" {
		t.Fatalf("nodes split = %+v", g.Nodes)
	}
	if g.Restricted == nil || !*g.Restricted || g.NoFailback == nil || *g.NoFailback {
		t.Fatalf("boolish flags = %+v", g)
	}

	read, err := c.GetHAGroup(ctx, "g1")
	if err != nil {
		t.Fatalf("GetHAGroup: %v", err)
	}
	if read.Digest != "d1" || read.Restricted == nil || !*read.Restricted {
		t.Fatalf("read group = %+v", read)
	}

	update := HAGroup{Group: "g1", Nodes: []string{"n1:1"}, NoFailback: HABoolPtr(true)}
	if err := c.UpdateHAGroup(ctx, "g1", update, []string{"comment", "restricted"}); err != nil {
		t.Fatalf("UpdateHAGroup: %v", err)
	}
	if !strings.HasPrefix(lastQuery, "delete=") || !strings.Contains(lastQuery, "comment") || !strings.Contains(lastQuery, "restricted") {
		t.Fatalf("update query = %q", lastQuery)
	}
	if err := json.Unmarshal(lastBody, &sent); err != nil {
		t.Fatalf("update body %q is not JSON: %v", lastBody, err)
	}
	if sent["nofailback"] != true {
		t.Fatalf("update body = %v", sent)
	}

	if err := c.DeleteHAGroup(ctx, "g1"); err != nil {
		t.Fatalf("DeleteHAGroup: %v", err)
	}
}

// TestHA_DeleteHAGroup_404IsAPIError confirms a missing group surfaces as
// a 404 *APIError so the resource layer can treat already-absent as success.
func TestHA_DeleteHAGroup_404IsAPIError(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/cluster/ha/groups/gone" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"errors":"no such ha group"}`)
	})
	err := c.DeleteHAGroup(context.Background(), "gone")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 APIError, got %v", err)
	}
}

// TestHA_Resources_CRUD covers the resource wire set: create body with the
// hyphenated auto-rebalance key, read decode of boolish flags, update with
// delete query, and delete with the purge query parameter.
func TestHA_Resources_CRUD(t *testing.T) {
	var lastBody []byte
	var lastQuery string
	var lastPath string
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		lastBody, _ = io.ReadAll(r.Body)
		lastQuery = r.URL.RawQuery
		lastPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/cluster/ha/resources":
			_, _ = io.WriteString(w, `{"data":null}`)
		case r.Method == http.MethodGet && r.URL.Path == "/cluster/ha/resources":
			_, _ = io.WriteString(w, `{"data":[{"sid":"vm:100","type":"vm","state":"started"}]}`)
		case r.Method == http.MethodGet && r.URL.Path == "/cluster/ha/resources/vm:100":
			_, _ = io.WriteString(w, `{"data":{"sid":"vm:100","type":"vm","state":"started","group":"g1","max_restart":1,"max_relocate":2,"failback":1,"auto-rebalance":0,"digest":"d2"}}`)
		case r.Method == http.MethodPut && r.URL.Path == "/cluster/ha/resources/vm:100":
			_, _ = io.WriteString(w, `{"data":null}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/cluster/ha/resources/vm:100":
			_, _ = io.WriteString(w, `{"data":null}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	ctx := context.Background()

	res := HAResource{SID: "vm:100", State: "started", MaxRestart: HAInt64Ptr(2), Failback: HABoolPtr(true), AutoRebalance: HABoolPtr(false)}
	if err := c.CreateHAResource(ctx, res); err != nil {
		t.Fatalf("CreateHAResource: %v", err)
	}
	var sent map[string]any
	if err := json.Unmarshal(lastBody, &sent); err != nil {
		t.Fatalf("create body %q is not JSON: %v", lastBody, err)
	}
	if sent["sid"] != "vm:100" || sent["max_restart"] != float64(2) || sent["failback"] != true || sent["auto-rebalance"] != false {
		t.Fatalf("create body = %v", sent)
	}

	list, err := c.ListHAResources(ctx, "")
	if err != nil {
		t.Fatalf("ListHAResources: %v", err)
	}
	if len(list) != 1 || list[0].SID != "vm:100" || list[0].Type != "vm" {
		t.Fatalf("list = %+v", list)
	}

	read, err := c.GetHAResource(ctx, "vm:100")
	if err != nil {
		t.Fatalf("GetHAResource: %v", err)
	}
	if read.Group != "g1" || read.MaxRelocate == nil || *read.MaxRelocate != 2 {
		t.Fatalf("read resource = %+v", read)
	}
	if read.Failback == nil || !*read.Failback || read.AutoRebalance == nil || *read.AutoRebalance {
		t.Fatalf("boolish flags = %+v", read)
	}

	update := HAResource{SID: "vm:100", State: "stopped", Comment: "paused"}
	if err := c.UpdateHAResource(ctx, "vm:100", update, []string{"group"}); err != nil {
		t.Fatalf("UpdateHAResource: %v", err)
	}
	if !strings.HasPrefix(lastQuery, "delete=") || !strings.Contains(lastQuery, "group") {
		t.Fatalf("update query = %q", lastQuery)
	}
	if err := json.Unmarshal(lastBody, &sent); err != nil {
		t.Fatalf("update body %q is not JSON: %v", lastBody, err)
	}
	if sent["state"] != "stopped" || sent["comment"] != "paused" {
		t.Fatalf("update body = %v", sent)
	}

	if err := c.DeleteHAResource(ctx, "vm:100", false); err != nil {
		t.Fatalf("DeleteHAResource: %v", err)
	}
	if lastPath != "/cluster/ha/resources/vm:100" || !strings.Contains(lastQuery, "purge=false") {
		t.Fatalf("delete path/query = %s?%s", lastPath, lastQuery)
	}
}

// TestHA_Rules_CRUD covers the rule wire set for a node-affinity rule:
// create body with joined node/resource lists, read decode with list
// splitting and boolish flags, update with delete query, and delete.
func TestHA_Rules_CRUD(t *testing.T) {
	var lastBody []byte
	var lastQuery string
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		lastBody, _ = io.ReadAll(r.Body)
		lastQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/cluster/ha/rules":
			_, _ = io.WriteString(w, `{"data":null}`)
		case r.Method == http.MethodGet && r.URL.Path == "/cluster/ha/rules":
			_, _ = io.WriteString(w, `{"data":[{"rule":"r1","type":"node-affinity"}]}`)
		case r.Method == http.MethodGet && r.URL.Path == "/cluster/ha/rules/r1":
			_, _ = io.WriteString(w, `{"data":{"rule":"r1","type":"node-affinity","affinity":"negative","nodes":"n1:2,n2","resources":"vm:100,ct:101","strict":1,"disable":0,"comment":"keep away"}}`)
		case r.Method == http.MethodPut && r.URL.Path == "/cluster/ha/rules/r1":
			_, _ = io.WriteString(w, `{"data":null}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/cluster/ha/rules/r1":
			_, _ = io.WriteString(w, `{"data":null}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	ctx := context.Background()

	rule := HARule{
		Rule:      "r1",
		Type:      "node-affinity",
		Affinity:  "negative",
		Nodes:     []string{"n1:2", "n2"},
		Resources: []string{"vm:100", "ct:101"},
		Strict:    HABoolPtr(true),
		Comment:   "keep away",
	}
	if err := c.CreateHARule(ctx, rule); err != nil {
		t.Fatalf("CreateHARule: %v", err)
	}
	var sent map[string]any
	if err := json.Unmarshal(lastBody, &sent); err != nil {
		t.Fatalf("create body %q is not JSON: %v", lastBody, err)
	}
	if sent["rule"] != "r1" || sent["type"] != "node-affinity" || sent["nodes"] != "n1:2,n2" || sent["resources"] != "vm:100,ct:101" || sent["strict"] != true {
		t.Fatalf("create body = %v", sent)
	}

	rules, err := c.ListHARules(ctx, "")
	if err != nil {
		t.Fatalf("ListHARules: %v", err)
	}
	if len(rules) != 1 || rules[0].Rule != "r1" || rules[0].Type != "node-affinity" {
		t.Fatalf("rules = %+v", rules)
	}

	read, err := c.GetHARule(ctx, "r1")
	if err != nil {
		t.Fatalf("GetHARule: %v", err)
	}
	if read.Affinity != "negative" || len(read.Nodes) != 2 || len(read.Resources) != 2 {
		t.Fatalf("read rule = %+v", read)
	}
	if read.Strict == nil || !*read.Strict || read.Disable == nil || *read.Disable {
		t.Fatalf("boolish flags = %+v", read)
	}

	update := HARule{Rule: "r1", Type: "node-affinity", Affinity: "positive"}
	if err := c.UpdateHARule(ctx, "r1", update, []string{"comment"}); err != nil {
		t.Fatalf("UpdateHARule: %v", err)
	}
	if !strings.HasPrefix(lastQuery, "delete=") || !strings.Contains(lastQuery, "comment") {
		t.Fatalf("update query = %q", lastQuery)
	}
	if err := c.DeleteHARule(ctx, "r1"); err != nil {
		t.Fatalf("DeleteHARule: %v", err)
	}
}

// TestHA_ListRules_TypeFilter verifies the optional type filter is passed
// as a query parameter on the rules index.
func TestHA_ListRules_TypeFilter(t *testing.T) {
	var lastQuery string
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		lastQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[]}`)
	})
	if _, err := c.ListHARules(context.Background(), "resource-affinity"); err != nil {
		t.Fatalf("ListHARules: %v", err)
	}
	if !strings.Contains(lastQuery, "type=resource-affinity") {
		t.Fatalf("query = %q, want type filter", lastQuery)
	}
}
