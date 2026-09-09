// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"context"
	"fmt"
)

// LxcRebootParams carries the optional parameters of POST
// /nodes/{node}/lxc/{vmid}/status/reboot.
type LxcRebootParams struct {
	// TimeoutSeconds waits at most this many seconds for the shutdown
	// before the reboot proceeds. Nil omits the key (PVE default 60).
	TimeoutSeconds *int64 `json:"timeout,omitempty"`
}

// LxcReboot issues POST /nodes/{node}/lxc/{vmid}/status/reboot, which shuts
// the container down and starts it again, applying pending changes. It
// returns the task UPID on the container's node.
func (c *Client) LxcReboot(ctx context.Context, node string, vmid int64, params *LxcRebootParams) (string, error) {
	body := any(nil)
	if params != nil {
		body = params
	}
	return c.lxcAction(ctx, node, vmid, "status/reboot", body)
}

// LxcSuspend issues POST /nodes/{node}/lxc/{vmid}/status/suspend. Upstream
// marks container suspend as experimental. It returns the task UPID on the
// container's node.
func (c *Client) LxcSuspend(ctx context.Context, node string, vmid int64) (string, error) {
	return c.lxcAction(ctx, node, vmid, "status/suspend", nil)
}

// LxcResume issues POST /nodes/{node}/lxc/{vmid}/status/resume and returns
// the task UPID on the container's node.
func (c *Client) LxcResume(ctx context.Context, node string, vmid int64) (string, error) {
	return c.lxcAction(ctx, node, vmid, "status/resume", nil)
}

// lxcAction posts an LXC guest action subcommand and decodes the returned
// task UPID.
func (c *Client) lxcAction(ctx context.Context, node string, vmid int64, subcommand string, body any) (string, error) {
	path := fmt.Sprintf("/nodes/%s/lxc/%d/%s", node, vmid, subcommand)
	var upid string
	if err := c.Do(ctx, "POST", path, body, &upid); err != nil {
		return "", err
	}
	return upid, nil
}

// LxcSnapshot is one entry of GET /nodes/{node}/lxc/{vmid}/snapshot. The
// list always contains a `current` pseudo-entry for the running state.
type LxcSnapshot struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parent      string `json:"parent,omitempty"`
	Snaptime    *int64 `json:"snaptime,omitempty"`
}

// CreateLxcSnapshot issues POST /nodes/{node}/lxc/{vmid}/snapshot. An empty
// description omits the key. It returns the task UPID on the container's
// node.
func (c *Client) CreateLxcSnapshot(ctx context.Context, node string, vmid int64, name, description string) (string, error) {
	path := fmt.Sprintf("/nodes/%s/lxc/%d/snapshot", node, vmid)
	body := map[string]string{"snapname": name}
	if description != "" {
		body["description"] = description
	}
	var upid string
	if err := c.Do(ctx, "POST", path, body, &upid); err != nil {
		return "", err
	}
	return upid, nil
}

// ListLxcSnapshots reads GET /nodes/{node}/lxc/{vmid}/snapshot, including
// the `current` pseudo-entry.
func (c *Client) ListLxcSnapshots(ctx context.Context, node string, vmid int64) ([]LxcSnapshot, error) {
	path := fmt.Sprintf("/nodes/%s/lxc/%d/snapshot", node, vmid)
	var snaps []LxcSnapshot
	if err := c.Do(ctx, "GET", path, nil, &snaps); err != nil {
		return nil, err
	}
	return snaps, nil
}

// LxcSnapshotConfig is the decoded body of GET
// /nodes/{node}/lxc/{vmid}/snapshot/{snapname}/config.
type LxcSnapshotConfig struct {
	Description string `json:"description,omitempty"`
	Digest      string `json:"digest,omitempty"`
	Parent      string `json:"parent,omitempty"`
	Snaptime    *int64 `json:"snaptime,omitempty"`
}

// GetLxcSnapshotConfig reads GET
// /nodes/{node}/lxc/{vmid}/snapshot/{snapname}/config; a missing snapshot
// surfaces as a 404 *APIError.
func (c *Client) GetLxcSnapshotConfig(ctx context.Context, node string, vmid int64, name string) (*LxcSnapshotConfig, error) {
	path := fmt.Sprintf("/nodes/%s/lxc/%d/snapshot/%s/config", node, vmid, name)
	var cfg LxcSnapshotConfig
	if err := c.Do(ctx, "GET", path, nil, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// UpdateLxcSnapshotConfig issues PUT
// /nodes/{node}/lxc/{vmid}/snapshot/{snapname}/config. The description is
// always sent; an empty string clears it.
func (c *Client) UpdateLxcSnapshotConfig(ctx context.Context, node string, vmid int64, name, description string) error {
	path := fmt.Sprintf("/nodes/%s/lxc/%d/snapshot/%s/config", node, vmid, name)
	body := map[string]string{"description": description}
	return c.Do(ctx, "PUT", path, body, nil)
}

// DeleteLxcSnapshot issues DELETE /nodes/{node}/lxc/{vmid}/snapshot/{snapname}.
// Force keeps the deletion going even if removing the volume snapshots
// fails. It returns the task UPID on the container's node.
func (c *Client) DeleteLxcSnapshot(ctx context.Context, node string, vmid int64, name string, force bool) (string, error) {
	path := fmt.Sprintf("/nodes/%s/lxc/%d/snapshot/%s", node, vmid, name)
	body := any(nil)
	if force {
		body = map[string]bool{"force": true}
	}
	var upid string
	if err := c.Do(ctx, "DELETE", path, body, &upid); err != nil {
		return "", err
	}
	return upid, nil
}

// RollbackLxcSnapshot issues POST
// /nodes/{node}/lxc/{vmid}/snapshot/{snapname}/rollback. Start restarts the
// container after a successful rollback. It returns the task UPID on the
// container's node.
func (c *Client) RollbackLxcSnapshot(ctx context.Context, node string, vmid int64, name string, start bool) (string, error) {
	path := fmt.Sprintf("/nodes/%s/lxc/%d/snapshot/%s/rollback", node, vmid, name)
	body := any(nil)
	if start {
		body = map[string]bool{"start": true}
	}
	var upid string
	if err := c.Do(ctx, "POST", path, body, &upid); err != nil {
		return "", err
	}
	return upid, nil
}
