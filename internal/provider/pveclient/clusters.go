// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"context"
)

// ClusterNode is the per-row shape returned by GET /nodes (the cluster-wide
// node index). Status is "online" / "offline" / "unknown"; SSLFingerprint
// and ID are PVE 8.x additions and may be absent on older releases.
type ClusterNode struct {
	Node           string  `json:"node"`
	Status         string  `json:"status"`
	CPU            float64 `json:"cpu"`
	MaxCPU         int     `json:"maxcpu"`
	Mem            int64   `json:"mem"`
	MaxMem         int64   `json:"maxmem"`
	Level          string  `json:"level"`
	Uptime         int64   `json:"uptime"`
	SSLFingerprint string  `json:"ssl_fingerprint,omitempty"`
	ID             string  `json:"id,omitempty"`
	IP             string  `json:"ip,omitempty"`
	Local          bool    `json:"local,omitempty"`
	NodeID         int     `json:"nodeid,omitempty"`
}

// ListClusterNodes enumerates GET /nodes. The result is the per-node data
// filtered against the caller's Sys.Audit on /nodes/{node}; absent
// privilege on a node yields a row with status="unknown" and zero
// counters.
func (c *Client) ListClusterNodes(ctx context.Context) ([]ClusterNode, error) {
	var out []ClusterNode
	if err := c.Do(ctx, "GET", "/nodes", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}
