// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"context"
	"fmt"
)

// NodeRootFS captures the embedded rootfs object on the status payload. PVE
// only includes the totals in this struct; per-mount breakdowns come from
// `/nodes/{node}/disks/list`.
type NodeRootFS struct {
	Total int64 `json:"total"`
	Used  int64 `json:"used"`
	Free  int64 `json:"free"`
}

// NodeStatus mirrors GET /nodes/{node}/status. The struct captures every
// field PVE returns for a healthy node; absent fields decode as zero values
// so older PVE versions still deserialize cleanly.
type NodeStatus struct {
	CPU        float64    `json:"cpu"`
	MaxCPU     int        `json:"maxcpu"`
	Mem        int64      `json:"mem"`
	MaxMem     int64      `json:"maxmem"`
	Uptime     int64      `json:"uptime"`
	Level      string     `json:"level"`
	RootFS     NodeRootFS `json:"rootfs"`
	Kernel     string     `json:"kernel,omitempty"`
	PVEVersion string     `json:"pveversion,omitempty"`
}

// GetNodeStatus fetches /nodes/{node}/status.
func (c *Client) GetNodeStatus(ctx context.Context, node string) (*NodeStatus, error) {
	var out NodeStatus
	path := fmt.Sprintf("/nodes/%s/status", node)
	if err := c.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
