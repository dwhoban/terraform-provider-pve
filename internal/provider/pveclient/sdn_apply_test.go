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

// TestApplySdn verifies ApplySdn PUTs /cluster/sdn, omits the body when no
// options are set, and returns the reload task UPID.
func TestApplySdn(t *testing.T) {
	upid := "UPID:pve1:00000001:00000001:sdnreload:root@pam:"
	var sawBody []byte
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/cluster/sdn" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		sawBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":"`+upid+`"}`)
	})
	got, err := c.ApplySdn(context.Background(), SdnLockOptions{})
	if err != nil {
		t.Fatalf("ApplySdn: %v", err)
	}
	if got != upid {
		t.Fatalf("ApplySdn = %q, want %q", got, upid)
	}
	if len(sawBody) != 0 {
		t.Fatalf("default apply must not carry a body, got %q", sawBody)
	}

	if _, err := c.ApplySdn(context.Background(), SdnLockOptions{LockToken: "tok", ReleaseLock: boolPtr(false)}); err != nil {
		t.Fatalf("ApplySdn with options: %v", err)
	}
	var sent map[string]any
	if err := json.Unmarshal(sawBody, &sent); err != nil {
		t.Fatalf("body %q is not JSON: %v", sawBody, err)
	}
	if sent["lock-token"] != "tok" {
		t.Fatalf("lock-token = %v, want tok", sent["lock-token"])
	}
	if v, ok := sent["release-lock"].(bool); !ok || v {
		t.Fatalf("release-lock = %v, want false", sent["release-lock"])
	}
}

// TestRollbackSdn verifies RollbackSdn POSTs /cluster/sdn/rollback, which
// returns null per the pin (no task to wait on).
func TestRollbackSdn(t *testing.T) {
	var sawMethod, sawPath string
	var sawBody []byte
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		sawMethod, sawPath = r.Method, r.URL.Path
		sawBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":null}`)
	})
	if err := c.RollbackSdn(context.Background(), SdnLockOptions{}); err != nil {
		t.Fatalf("RollbackSdn: %v", err)
	}
	if sawMethod != http.MethodPost || sawPath != "/cluster/sdn/rollback" {
		t.Fatalf("rollback request = %s %s", sawMethod, sawPath)
	}
	if len(sawBody) != 0 {
		t.Fatalf("default rollback must not carry a body, got %q", sawBody)
	}

	if err := c.RollbackSdn(context.Background(), SdnLockOptions{LockToken: "tok"}); err != nil {
		t.Fatalf("RollbackSdn with token: %v", err)
	}
	if !strings.Contains(string(sawBody), `"lock-token":"tok"`) {
		t.Fatalf("lock-token missing from body: %q", sawBody)
	}
}

// TestSdnFabricCRUD covers the fabric wire round-trip: create body carries
// the protocol, get decodes property-string redistribute entries, update
// carries the pin's delete list, delete uses the fabric path.
func TestSdnFabricCRUD(t *testing.T) {
	exists := false
	var sawBody []byte
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/cluster/sdn/fabrics/fabric":
			sawBody, _ = io.ReadAll(r.Body)
			exists = true
			_, _ = io.WriteString(w, `{"data":null}`)
		case r.Method == http.MethodGet && r.URL.Path == "/cluster/sdn/fabrics/fabric/f1":
			if !exists {
				w.WriteHeader(http.StatusNotFound)
				_, _ = io.WriteString(w, `{"errors":"no such fabric"}`)
				return
			}
			_, _ = io.WriteString(w, `{"data":{"id":"f1","protocol":"ospf","area":"0.0.0.0",`+
				`"ip_prefix":"10.0.0.0/24","route_filter":"pl1",`+
				`"redistribute":["source=connected,route-map=rm1","source=static"],"digest":"abc123"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/cluster/sdn/fabrics/fabric":
			_, _ = io.WriteString(w, `{"data":[{"id":"f1","protocol":"ospf"},{"id":"f2","protocol":"openfabric","csnp_interval":5,"hello_interval":10}]}`)
		case r.Method == http.MethodPut && r.URL.Path == "/cluster/sdn/fabrics/fabric/f1":
			sawBody, _ = io.ReadAll(r.Body)
			_, _ = io.WriteString(w, `{"data":null}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/cluster/sdn/fabrics/fabric/f1":
			exists = false
			_, _ = io.WriteString(w, `{"data":null}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	ctx := context.Background()

	if err := c.CreateSdnFabric(ctx, SdnFabric{ID: "f1", Protocol: SdnFabricProtocolOSPF, Area: "0.0.0.0", IPPrefix: "10.0.0.0/24", Redistribute: []SdnFabricRedistribute{{Source: "connected"}}}); err != nil {
		t.Fatalf("CreateSdnFabric: %v", err)
	}
	var created map[string]any
	if err := json.Unmarshal(sawBody, &created); err != nil {
		t.Fatalf("create body %q is not JSON: %v", sawBody, err)
	}
	if created["protocol"] != "ospf" || created["area"] != "0.0.0.0" {
		t.Fatalf("create body = %+v", created)
	}
	redist, ok := created["redistribute"].([]any)
	first, firstOK := redist[0].(map[string]any)
	if !ok || len(redist) != 1 || !firstOK || first["source"] != "connected" {
		t.Fatalf("redistribute = %+v, want one object with source=connected", created["redistribute"])
	}

	fabric, err := c.GetSdnFabric(ctx, "f1")
	if err != nil {
		t.Fatalf("GetSdnFabric: %v", err)
	}
	if fabric.ID != "f1" || fabric.Protocol != "ospf" || fabric.Area != "0.0.0.0" || fabric.IPPrefix != "10.0.0.0/24" || fabric.RouteFilter != "pl1" || fabric.Digest != "abc123" {
		t.Fatalf("fabric = %+v", fabric)
	}
	if len(fabric.Redistribute) != 2 || fabric.Redistribute[0].Source != "connected" || fabric.Redistribute[0].RouteMap != "rm1" || fabric.Redistribute[1].Source != "static" {
		t.Fatalf("redistribute = %+v", fabric.Redistribute)
	}

	if err := c.UpdateSdnFabric(ctx, "f1", SdnFabricUpdate{IPPrefix: "10.0.1.0/24", Delete: []string{"area", "redistribute"}}); err != nil {
		t.Fatalf("UpdateSdnFabric: %v", err)
	}
	var updated map[string]any
	if err := json.Unmarshal(sawBody, &updated); err != nil {
		t.Fatalf("update body %q is not JSON: %v", sawBody, err)
	}
	if updated["ip_prefix"] != "10.0.1.0/24" {
		t.Fatalf("update body = %+v", updated)
	}
	del, ok := updated["delete"].([]any)
	if !ok || len(del) != 2 || del[0] != "area" || del[1] != "redistribute" {
		t.Fatalf("delete = %+v, want [area redistribute]", updated["delete"])
	}

	fabrics, err := c.ListSdnFabrics(ctx)
	if err != nil {
		t.Fatalf("ListSdnFabrics: %v", err)
	}
	if len(fabrics) != 2 || fabrics[1].Protocol != "openfabric" {
		t.Fatalf("fabrics = %+v", fabrics)
	}
	if fabrics[1].CsnpInterval == nil || *fabrics[1].CsnpInterval != 5 || fabrics[1].HelloInterval == nil || *fabrics[1].HelloInterval != 10 {
		t.Fatalf("openfabric intervals = %+v", fabrics[1])
	}

	if err := c.DeleteSdnFabric(ctx, "f1"); err != nil {
		t.Fatalf("DeleteSdnFabric: %v", err)
	}
}

// TestSdnFabricNodesCRUD covers the fabric node member round-trip, including
// property-string interface decoding on reads.
func TestSdnFabricNodesCRUD(t *testing.T) {
	exists := false
	var sawBody []byte
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/cluster/sdn/fabrics/node/f1":
			sawBody, _ = io.ReadAll(r.Body)
			exists = true
			_, _ = io.WriteString(w, `{"data":null}`)
		case r.Method == http.MethodGet && r.URL.Path == "/cluster/sdn/fabrics/node/f1":
			_, _ = io.WriteString(w, `{"data":[{"fabric_id":"f1","node_id":"pve1","protocol":"ospf","ip":"10.0.0.1",`+
				`"interfaces":["name=ens19,ip=10.0.0.1/24,network_type=broadcast"],"digest":"def456"}]}`)
		case r.Method == http.MethodGet && r.URL.Path == "/cluster/sdn/fabrics/node/f1/pve1":
			if !exists {
				w.WriteHeader(http.StatusNotFound)
				_, _ = io.WriteString(w, `{"errors":"no such node"}`)
				return
			}
			_, _ = io.WriteString(w, `{"data":{"fabric_id":"f1","node_id":"pve1","protocol":"ospf",`+
				`"interfaces":[{"name":"ens19","ip":"10.0.0.1/24"}],"digest":"def456"}}`)
		case r.Method == http.MethodPut && r.URL.Path == "/cluster/sdn/fabrics/node/f1/pve1":
			sawBody, _ = io.ReadAll(r.Body)
			_, _ = io.WriteString(w, `{"data":null}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/cluster/sdn/fabrics/node/f1/pve1":
			exists = false
			_, _ = io.WriteString(w, `{"data":null}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	ctx := context.Background()

	node := SdnFabricNode{
		FabricID:   "f1",
		NodeID:     "pve1",
		Protocol:   SdnFabricProtocolOSPF,
		IP:         "10.0.0.1",
		Interfaces: []SdnFabricInterface{{Name: "ens19", IP: "10.0.0.1/24", NetworkType: "broadcast"}},
	}
	if err := c.CreateSdnFabricNode(ctx, node); err != nil {
		t.Fatalf("CreateSdnFabricNode: %v", err)
	}
	var created map[string]any
	if err := json.Unmarshal(sawBody, &created); err != nil {
		t.Fatalf("create body %q is not JSON: %v", sawBody, err)
	}
	if created["fabric_id"] != "f1" || created["node_id"] != "pve1" || created["protocol"] != "ospf" {
		t.Fatalf("create body = %+v", created)
	}
	ifaces, ok := created["interfaces"].([]any)
	first, firstOK := ifaces[0].(map[string]any)
	if !ok || len(ifaces) != 1 || !firstOK || first["name"] != "ens19" {
		t.Fatalf("interfaces = %+v, want one object named ens19", created["interfaces"])
	}

	nodes, err := c.ListSdnFabricNodes(ctx, "f1")
	if err != nil {
		t.Fatalf("ListSdnFabricNodes: %v", err)
	}
	if len(nodes) != 1 || nodes[0].NodeID != "pve1" || nodes[0].IP != "10.0.0.1" {
		t.Fatalf("nodes = %+v", nodes)
	}
	if len(nodes[0].Interfaces) != 1 || nodes[0].Interfaces[0].Name != "ens19" || nodes[0].Interfaces[0].IP != "10.0.0.1/24" || nodes[0].Interfaces[0].NetworkType != "broadcast" {
		t.Fatalf("interfaces = %+v", nodes[0].Interfaces)
	}

	got, err := c.GetSdnFabricNode(ctx, "f1", "pve1")
	if err != nil {
		t.Fatalf("GetSdnFabricNode: %v", err)
	}
	if got.Digest != "def456" || len(got.Interfaces) != 1 || got.Interfaces[0].IP != "10.0.0.1/24" {
		t.Fatalf("node = %+v", got)
	}

	if err := c.UpdateSdnFabricNode(ctx, "f1", "pve1", SdnFabricNodeUpdate{
		Interfaces: []SdnFabricInterface{{Name: "ens20", IP: "10.0.0.2/24"}},
		Delete:     []string{"ip", "interfaces"},
	}); err != nil {
		t.Fatalf("UpdateSdnFabricNode: %v", err)
	}
	var updated map[string]any
	if err := json.Unmarshal(sawBody, &updated); err != nil {
		t.Fatalf("update body %q is not JSON: %v", sawBody, err)
	}
	del, ok := updated["delete"].([]any)
	if !ok || len(del) != 2 || del[0] != "ip" || del[1] != "interfaces" {
		t.Fatalf("delete = %+v, want [ip interfaces]", updated["delete"])
	}

	if err := c.DeleteSdnFabricNode(ctx, "f1", "pve1"); err != nil {
		t.Fatalf("DeleteSdnFabricNode: %v", err)
	}
}

// TestSdnFabricRuntimeReads covers the per-node fabric status endpoints the
// data sources expose behind the optional node argument.
func TestSdnFabricRuntimeReads(t *testing.T) {
	var sawPath string
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		sawPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(sawPath, "/interfaces"):
			_, _ = io.WriteString(w, `{"data":[{"name":"ens19","state":"up","type":"Point-to-Point"}]}`)
		case strings.HasSuffix(sawPath, "/neighbors"):
			_, _ = io.WriteString(w, `{"data":[{"neighbor":"10.0.0.2","status":"Full","uptime":"8h24m12s"}]}`)
		case strings.HasSuffix(sawPath, "/routes"):
			_, _ = io.WriteString(w, `{"data":[{"route":"10.0.2.0/24","via":["10.0.0.2","10.0.0.3"]}]}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	ctx := context.Background()

	ifaces, err := c.ListSdnFabricRuntimeInterfaces(ctx, "pve1", "f1")
	if err != nil {
		t.Fatalf("ListSdnFabricRuntimeInterfaces: %v", err)
	}
	if len(ifaces) != 1 || ifaces[0].Name != "ens19" || ifaces[0].State != "up" || ifaces[0].Type != "Point-to-Point" {
		t.Fatalf("interfaces = %+v", ifaces)
	}

	neighbors, err := c.ListSdnFabricRuntimeNeighbors(ctx, "pve1", "f1")
	if err != nil {
		t.Fatalf("ListSdnFabricRuntimeNeighbors: %v", err)
	}
	if len(neighbors) != 1 || neighbors[0].Neighbor != "10.0.0.2" || neighbors[0].Status != "Full" || neighbors[0].Uptime != "8h24m12s" {
		t.Fatalf("neighbors = %+v", neighbors)
	}

	routes, err := c.ListSdnFabricRuntimeRoutes(ctx, "pve1", "f1")
	if err != nil {
		t.Fatalf("ListSdnFabricRuntimeRoutes: %v", err)
	}
	if len(routes) != 1 || routes[0].Route != "10.0.2.0/24" || len(routes[0].Via) != 2 || routes[0].Via[1] != "10.0.0.3" {
		t.Fatalf("routes = %+v", routes)
	}
}
