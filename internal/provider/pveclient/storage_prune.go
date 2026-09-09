// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
)

// StoragePruneOptions carries the parameters shared by the prune endpoints
// (GET/DELETE /nodes/{node}/storage/{storage}/prunebackups). The retention
// fields travel on the wire as PVE's `prune-backups` property string and
// override the storage configuration's retention when set; nil fields are
// omitted from the request.
type StoragePruneOptions struct {
	// KeepAll keeps every backup regardless of the other retention rules.
	KeepAll *bool
	// KeepHourly keeps the last N hourly backups.
	KeepHourly *int64
	// KeepDaily keeps the last N daily backups.
	KeepDaily *int64
	// KeepWeekly keeps the last N weekly backups.
	KeepWeekly *int64
	// KeepMonthly keeps the last N monthly backups.
	KeepMonthly *int64
	// KeepYearly keeps the last N yearly backups.
	KeepYearly *int64
	// KeepLast keeps the last N backups in creation order.
	KeepLast *int64
	// Type restricts the prune to backups of this guest type ("qemu" or
	// "lxc" per the pin); empty means no filter.
	Type string
	// VMID restricts the prune to backups of this guest; nil means no
	// filter.
	VMID *int64
}

// StoragePruneEntry is one backup considered by a prune dry run (GET
// /nodes/{node}/storage/{storage}/prunebackups).
type StoragePruneEntry struct {
	// Volid is the backup volume ID.
	Volid string `json:"volid"`
	// Mark is whether the backup would be kept or removed: "keep",
	// "remove", "protected" (protected backups are never removed), or
	// "renamed" (kept under a mangled name; the scheme is nonstandard).
	Mark string `json:"mark"`
	// Type is the guest type: "qemu", "lxc", "openvz" or "unknown".
	Type string `json:"type"`
	// VMID is the guest the backup belongs to; optional per the pin.
	VMID *int64 `json:"vmid,omitempty"`
	// CTime is the backup creation time in seconds since the UNIX epoch.
	CTime int64 `json:"ctime"`
}

// storagePruneQuery renders opts as the query string for the prunebackups
// endpoints, returning "" when no optional parameter is set.
func storagePruneQuery(opts StoragePruneOptions) string {
	q := url.Values{}
	retention := backupPruneBackupsString(&BackupPruneBackups{
		KeepAll:     opts.KeepAll,
		KeepHourly:  opts.KeepHourly,
		KeepDaily:   opts.KeepDaily,
		KeepWeekly:  opts.KeepWeekly,
		KeepMonthly: opts.KeepMonthly,
		KeepYearly:  opts.KeepYearly,
		KeepLast:    opts.KeepLast,
	})
	if retention != "" {
		q.Set("prune-backups", retention)
	}
	if opts.Type != "" {
		q.Set("type", opts.Type)
	}
	if opts.VMID != nil {
		q.Set("vmid", strconv.FormatInt(*opts.VMID, 10))
	}
	if encoded := q.Encode(); encoded != "" {
		return "?" + encoded
	}
	return ""
}

// PruneStorageBackups starts a prune of the backups on a storage that use
// the standard naming scheme (DELETE
// /nodes/{node}/storage/{storage}/prunebackups) and returns the worker
// task UPID per the pin; callers wait on it with WaitForTask on node.
func (c *Client) PruneStorageBackups(ctx context.Context, node, storage string, opts StoragePruneOptions) (string, error) {
	path := fmt.Sprintf("/nodes/%s/storage/%s/prunebackups%s", node, storage, storagePruneQuery(opts))
	var upid string
	if err := c.Do(ctx, "DELETE", path, nil, &upid); err != nil {
		return "", err
	}
	return upid, nil
}

// DryRunStoragePruneBackups reports which backups a prune would remove
// without deleting anything (GET
// /nodes/{node}/storage/{storage}/prunebackups). The pin marks the preview
// as advisory: it may differ from a later prune if backups change in the
// meantime.
func (c *Client) DryRunStoragePruneBackups(ctx context.Context, node, storage string, opts StoragePruneOptions) ([]StoragePruneEntry, error) {
	path := fmt.Sprintf("/nodes/%s/storage/%s/prunebackups%s", node, storage, storagePruneQuery(opts))
	var entries []StoragePruneEntry
	if err := c.Do(ctx, "GET", path, nil, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}
