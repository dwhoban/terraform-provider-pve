// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"context"
	"fmt"
	"net/url"
)

// QemuVMRebootOptions carries the pin's optional parameters for
// POST /nodes/{node}/qemu/{vmid}/status/reboot.
type QemuVMRebootOptions struct {
	// Timeout is the maximum number of seconds to wait for the shutdown
	// half of the reboot.
	Timeout *int64 `json:"timeout,omitempty"`
}

// QemuVMSuspendOptions carries the pin's optional parameters for
// POST /nodes/{node}/qemu/{vmid}/status/suspend.
type QemuVMSuspendOptions struct {
	// Todisk suspends the VM to disk; it is resumed on the next start.
	Todisk *bool `json:"todisk,omitempty"`
	// StateStorage is the storage used to hold the VM state.
	StateStorage string `json:"statestorage,omitempty"`
}

// QemuVMReboot issues POST /nodes/{node}/qemu/{vmid}/status/reboot and
// returns the worker task UPID.
func (c *Client) QemuVMReboot(ctx context.Context, node string, vmid int64, opts QemuVMRebootOptions) (string, error) {
	return c.qemuVMStatus(ctx, node, vmid, "reboot", opts)
}

// QemuVMReset issues POST /nodes/{node}/qemu/{vmid}/status/reset and returns
// the worker task UPID. The pin defines no optional parameters.
func (c *Client) QemuVMReset(ctx context.Context, node string, vmid int64) (string, error) {
	return c.qemuVMStatus(ctx, node, vmid, "reset", nil)
}

// QemuVMSuspend issues POST /nodes/{node}/qemu/{vmid}/status/suspend and
// returns the worker task UPID.
func (c *Client) QemuVMSuspend(ctx context.Context, node string, vmid int64, opts QemuVMSuspendOptions) (string, error) {
	return c.qemuVMStatus(ctx, node, vmid, "suspend", opts)
}

// QemuVMResume issues POST /nodes/{node}/qemu/{vmid}/status/resume and
// returns the worker task UPID. The pin defines no optional parameters.
func (c *Client) QemuVMResume(ctx context.Context, node string, vmid int64) (string, error) {
	return c.qemuVMStatus(ctx, node, vmid, "resume", nil)
}

// qemuVMStatus posts the shared status-verb request body and decodes the
// returned UPID.
func (c *Client) qemuVMStatus(ctx context.Context, node string, vmid int64, verb string, opts any) (string, error) {
	path := fmt.Sprintf("/nodes/%s/qemu/%d/status/%s", node, vmid, verb)
	var body any
	if opts != nil {
		body = opts
	}
	// PVE returns the task UPID as a plain string in `data`.
	var upid string
	if err := c.Do(ctx, "POST", path, body, &upid); err != nil {
		return "", fmt.Errorf("pveclient: qemu vm %s %d %s: %w", node, vmid, verb, err)
	}
	return upid, nil
}

// QemuVMSnapshotOptions carries the pin's optional parameters for
// POST /nodes/{node}/qemu/{vmid}/snapshot.
type QemuVMSnapshotOptions struct {
	// Description is a textual description or comment.
	Description string `json:"description,omitempty"`
	// Vmstate saves the VM RAM state together with the disks.
	Vmstate *bool `json:"vmstate,omitempty"`
}

// QemuVMSnapshot is one snapshot entry as returned by
// GET /nodes/{node}/qemu/{vmid}/snapshot[/ {snapname}].
type QemuVMSnapshot struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parent      string `json:"parent,omitempty"`
	Snaptime    *int64 `json:"snaptime,omitempty"`
	Vmstate     *bool  `json:"vmstate,omitempty"`
}

// CreateQemuVMSnapshot issues POST /nodes/{node}/qemu/{vmid}/snapshot and
// returns the worker task UPID.
func (c *Client) CreateQemuVMSnapshot(ctx context.Context, node string, vmid int64, snapname string, opts QemuVMSnapshotOptions) (string, error) {
	body := struct {
		Snapname    string `json:"snapname"`
		Description string `json:"description,omitempty"`
		Vmstate     *bool  `json:"vmstate,omitempty"`
	}{Snapname: snapname, Description: opts.Description, Vmstate: opts.Vmstate}
	path := fmt.Sprintf("/nodes/%s/qemu/%d/snapshot", node, vmid)
	// PVE returns the task UPID as a plain string in `data`.
	var upid string
	if err := c.Do(ctx, "POST", path, body, &upid); err != nil {
		return "", fmt.Errorf("pveclient: create qemu vm snapshot %s/%d/%s: %w", node, vmid, snapname, err)
	}
	return upid, nil
}

// GetQemuVMSnapshot issues GET /nodes/{node}/qemu/{vmid}/snapshot/{snapname}.
// A missing snapshot surfaces as a not-found APIError.
func (c *Client) GetQemuVMSnapshot(ctx context.Context, node string, vmid int64, snapname string) (*QemuVMSnapshot, error) {
	path := fmt.Sprintf("/nodes/%s/qemu/%d/snapshot/%s", node, vmid, url.PathEscape(snapname))
	var snap QemuVMSnapshot
	if err := c.Do(ctx, "GET", path, nil, &snap); err != nil {
		return nil, fmt.Errorf("pveclient: get qemu vm snapshot %s/%d/%s: %w", node, vmid, snapname, err)
	}
	snap.Name = snapname
	return &snap, nil
}

// ListQemuVMSnapshots issues GET /nodes/{node}/qemu/{vmid}/snapshot. The
// synthetic `current` entry returned by PVE is filtered out.
func (c *Client) ListQemuVMSnapshots(ctx context.Context, node string, vmid int64) ([]QemuVMSnapshot, error) {
	path := fmt.Sprintf("/nodes/%s/qemu/%d/snapshot", node, vmid)
	var snaps []QemuVMSnapshot
	if err := c.Do(ctx, "GET", path, nil, &snaps); err != nil {
		return nil, fmt.Errorf("pveclient: list qemu vm snapshots %s/%d: %w", node, vmid, err)
	}
	out := make([]QemuVMSnapshot, 0, len(snaps))
	for _, s := range snaps {
		if s.Name == "current" {
			continue
		}
		out = append(out, s)
	}
	return out, nil
}

// DeleteQemuVMSnapshot issues DELETE /nodes/{node}/qemu/{vmid}/snapshot/{snapname}
// and returns the worker task UPID. Set force to remove the snapshot from
// the config even if removing disk snapshots fails.
func (c *Client) DeleteQemuVMSnapshot(ctx context.Context, node string, vmid int64, snapname string, force *bool) (string, error) {
	body := struct {
		Force *bool `json:"force,omitempty"`
	}{Force: force}
	path := fmt.Sprintf("/nodes/%s/qemu/%d/snapshot/%s", node, vmid, url.PathEscape(snapname))
	// PVE returns the task UPID as a plain string in `data`.
	var upid string
	if err := c.Do(ctx, "DELETE", path, body, &upid); err != nil {
		return "", fmt.Errorf("pveclient: delete qemu vm snapshot %s/%d/%s: %w", node, vmid, snapname, err)
	}
	return upid, nil
}

// RollbackQemuVMSnapshot issues POST /nodes/{node}/qemu/{vmid}/snapshot/{snapname}/rollback
// and returns the worker task UPID. The pin defines no optional parameters.
func (c *Client) RollbackQemuVMSnapshot(ctx context.Context, node string, vmid int64, snapname string) (string, error) {
	path := fmt.Sprintf("/nodes/%s/qemu/%d/snapshot/%s/rollback", node, vmid, url.PathEscape(snapname))
	// PVE returns the task UPID as a plain string in `data`.
	var upid string
	if err := c.Do(ctx, "POST", path, nil, &upid); err != nil {
		return "", fmt.Errorf("pveclient: rollback qemu vm snapshot %s/%d/%s: %w", node, vmid, snapname, err)
	}
	return upid, nil
}
