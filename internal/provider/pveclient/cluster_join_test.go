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

// TestClient_GetJoinClusterInfo_DecodesNodelist verifies the join info
// request targets /cluster/config/join and the nodelist decodes.
func TestClient_GetJoinClusterInfo_DecodesNodelist(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/cluster/config/join" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.URL.Query().Get("node") != "pve1" {
			t.Fatalf("node query = %q", r.URL.Query().Get("node"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"config_digest":"abc123","preferred_node":"pve1","totem":{"cluster_name":"prod","version":"2"},"nodelist":[{"name":"pve1","nodeid":1,"pve_addr":"10.0.0.11","pve_fp":"AA:BB","quorum_votes":1,"ring0_addr":"10.0.0.11"}]}}`)
	})
	info, err := c.GetJoinClusterInfo(context.Background(), "pve1")
	if err != nil {
		t.Fatalf("GetJoinClusterInfo: %v", err)
	}
	if info.ConfigDigest != "abc123" || info.PreferredNode != "pve1" || string(info.Totem["cluster_name"]) != `"prod"` {
		t.Fatalf("unexpected info: %+v", info)
	}
	if len(info.Nodelist) != 1 {
		t.Fatalf("nodelist len = %d", len(info.Nodelist))
	}
	n := info.Nodelist[0]
	if n.Name != "pve1" || n.NodeID == nil || *n.NodeID != 1 || n.PVEAddr != "10.0.0.11" || n.PVEFP != "AA:BB" || n.QuorumVotes == nil || *n.QuorumVotes != 1 || n.Ring0Addr != "10.0.0.11" {
		t.Fatalf("unexpected node entry: %+v", n)
	}
}

// TestClient_JoinCluster_WireShape verifies the join POST body carries the
// pin parameters and the string UPID return decodes.
func TestClient_JoinCluster_WireShape(t *testing.T) {
	var sawBody []byte
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/cluster/config/join" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		sawBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":"UPID:pve1:00001:00001:CLUSTERJOINED:join:root@pam:"}`)
	})
	upid, err := c.JoinCluster(context.Background(), ClusterJoinRequest{
		Fingerprint: "AA:BB",
		Hostname:    "10.0.0.10",
		Password:    "secret",
		NodeID:      int64Ptr(2),
		Votes:       int64Ptr(1),
		Force:       boolPtr(false),
	})
	if err != nil {
		t.Fatalf("JoinCluster: %v", err)
	}
	if !strings.HasPrefix(upid, "UPID:") {
		t.Fatalf("upid = %q", upid)
	}
	var body map[string]any
	if err := json.Unmarshal(sawBody, &body); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	if body["fingerprint"] != "AA:BB" || body["hostname"] != "10.0.0.10" || body["password"] != "secret" {
		t.Fatalf("body missing required params: %s", sawBody)
	}
	if v, ok := body["nodeid"].(float64); !ok || int64(v) != 2 {
		t.Fatalf("nodeid missing or wrong: %s", sawBody)
	}
	if v, ok := body["votes"].(float64); !ok || int64(v) != 1 {
		t.Fatalf("votes missing or wrong: %s", sawBody)
	}
}

// TestClient_ListClusterNodesConfig verifies the corosync node list shape.
func TestClient_ListClusterNodesConfig(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/cluster/config/nodes" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"node":"pve1"},{"node":"pve2"}]}`)
	})
	nodes, err := c.ListClusterNodesConfig(context.Background())
	if err != nil {
		t.Fatalf("ListClusterNodesConfig: %v", err)
	}
	if len(nodes) != 2 || nodes[0].Node != "pve1" || nodes[1].Node != "pve2" {
		t.Fatalf("unexpected nodes: %+v", nodes)
	}
}

// TestClient_RemoveClusterNode_WireShape verifies the DELETE targets
// /cluster/config/nodes/{node} and a 404 surfaces as *APIError.
func TestClient_RemoveClusterNode_WireShape(t *testing.T) {
	status := http.StatusOK
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/cluster/config/nodes/pve2" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, `{"data":null}`)
	})
	if err := c.RemoveClusterNode(context.Background(), "pve2"); err != nil {
		t.Fatalf("RemoveClusterNode: %v", err)
	}
	status = http.StatusNotFound
	err := c.RemoveClusterNode(context.Background(), "pve2")
	if err == nil {
		t.Fatal("expected 404 to surface as error")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Fatalf("error missing 404: %v", err)
	}
}

func int64Ptr(v int64) *int64 { return &v }
