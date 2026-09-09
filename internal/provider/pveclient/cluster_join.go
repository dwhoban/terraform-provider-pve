// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// ClusterJoinInfo is the decoded body of GET /cluster/config/join: everything
// a joining node needs to attach to this cluster, plus the current corosync
// member table.
type ClusterJoinInfo struct {
	ConfigDigest  string                     `json:"config_digest"`
	Nodelist      []ClusterJoinNode          `json:"nodelist"`
	PreferredNode string                     `json:"preferred_node"`
	Totem         map[string]json.RawMessage `json:"totem,omitempty"`
}

// ClusterJoinNode is one member of the corosync nodelist.
type ClusterJoinNode struct {
	Name        string `json:"name"`
	NodeID      *int64 `json:"nodeid,omitempty"`
	PVEAddr     string `json:"pve_addr,omitempty"`
	PVEFP       string `json:"pve_fp,omitempty"`
	QuorumVotes *int64 `json:"quorum_votes,omitempty"`
	Ring0Addr   string `json:"ring0_addr,omitempty"`
}

// ClusterJoinRequest is the body of POST /cluster/config/join. Fingerprint,
// Hostname, and Password are required by the API; the rest are optional.
// Password is the peer's root password and must never be logged.
type ClusterJoinRequest struct {
	Fingerprint string `json:"fingerprint"`
	Hostname    string `json:"hostname"`
	Password    string `json:"password"`
	NodeID      *int64 `json:"nodeid,omitempty"`
	Votes       *int64 `json:"votes,omitempty"`
	Force       *bool  `json:"force,omitempty"`
}

// ClusterNodeConfig is one entry of GET /cluster/config/nodes.
type ClusterNodeConfig struct {
	Node string `json:"node"`
}

// GetJoinClusterInfo returns the cluster join information for the cluster,
// scoped to node when non-empty (the API defaults to the connected node).
func (c *Client) GetJoinClusterInfo(ctx context.Context, node string) (*ClusterJoinInfo, error) {
	path := "/cluster/config/join"
	if node != "" {
		path += "?node=" + url.QueryEscape(node)
	}
	var info ClusterJoinInfo
	if err := c.Do(ctx, "GET", path, nil, &info); err != nil {
		return nil, fmt.Errorf("pveclient: GET cluster join info: %w", err)
	}
	return &info, nil
}

// JoinCluster joins this node into an existing cluster via
// POST /cluster/config/join and returns the task UPID string PVE answers
// with (callers should WaitForTask it on the joining node).
func (c *Client) JoinCluster(ctx context.Context, req ClusterJoinRequest) (string, error) {
	var upid string
	if err := c.Do(ctx, "POST", "/cluster/config/join", req, &upid); err != nil {
		return "", fmt.Errorf("pveclient: POST cluster join: %w", err)
	}
	return upid, nil
}

// ListClusterNodesConfig returns the corosync node list from
// GET /cluster/config/nodes.
func (c *Client) ListClusterNodesConfig(ctx context.Context) ([]ClusterNodeConfig, error) {
	var nodes []ClusterNodeConfig
	if err := c.Do(ctx, "GET", "/cluster/config/nodes", nil, &nodes); err != nil {
		return nil, fmt.Errorf("pveclient: GET cluster config nodes: %w", err)
	}
	return nodes, nil
}

// RemoveClusterNode removes a node from the cluster configuration via
// DELETE /cluster/config/nodes/{node}. The API surfaces a 404 when the node
// is not a member; callers map that to an already-absent success.
func (c *Client) RemoveClusterNode(ctx context.Context, node string) error {
	if err := c.Do(ctx, "DELETE", "/cluster/config/nodes/"+url.QueryEscape(node), nil, nil); err != nil {
		return fmt.Errorf("pveclient: DELETE cluster node %s: %w", node, err)
	}
	return nil
}
