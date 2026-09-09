// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"context"
	"fmt"
	"strings"
)

// UnlockUserTFA unlocks a user's TFA authentication
// (PUT /access/users/{userid}/unlock-tfa). The call is synchronous per the
// pin (returns a boolean) and is security-sensitive: it clears the TFA
// lockout so the user can re-enroll second factors.
func (c *Client) UnlockUserTFA(ctx context.Context, userid string) error {
	path := fmt.Sprintf("/access/users/%s/unlock-tfa", userid)
	return c.Do(ctx, "PUT", path, nil, nil)
}

// MigrateHAResource requests online migration of an HA resource to another
// node (POST /cluster/ha/resources/{sid}/migrate). The pin returns a
// description object, not a task UPID: the HA manager performs the actual
// migration asynchronously after the request is accepted, so there is
// nothing to wait on here.
func (c *Client) MigrateHAResource(ctx context.Context, sid, targetNode string) error {
	body := struct {
		Node string `json:"node"`
	}{Node: targetNode}
	path := fmt.Sprintf("/cluster/ha/resources/%s/migrate", sid)
	return c.Do(ctx, "POST", path, body, nil)
}

// RelocateHAResource requests relocation of an HA resource to another node
// (POST /cluster/ha/resources/{sid}/relocate): the service is stopped on
// the old node and restarted on the target. Like MigrateHAResource the pin
// returns a description object, not a task UPID.
func (c *Client) RelocateHAResource(ctx context.Context, sid, targetNode string) error {
	body := struct {
		Node string `json:"node"`
	}{Node: targetNode}
	path := fmt.Sprintf("/cluster/ha/resources/%s/relocate", sid)
	return c.Do(ctx, "POST", path, body, nil)
}

// VzdumpRunOptions carries the POST /nodes/{node}/vzdump (backup run)
// parameters. Unset fields are omitted from the request. The grouped
// options reuse the backup-job types; they travel on the wire as property
// strings and the guest lists as comma-separated strings, matching PVE's
// parameter encoding.
type VzdumpRunOptions struct {
	Mode                   string
	Compress               string
	Storage                string
	DumpDir                string
	TmpDir                 string
	Script                 string
	JobID                  string
	Pool                   string
	NotesTemplate          string
	MailTo                 string
	MailNotification       string
	NotificationMode       string
	PBSChangeDetectionMode string
	VMIDs                  []string
	Exclude                []string
	ExcludePath            []string
	All                    *bool
	BWLimit                *int64
	IONice                 *int64
	LockWait               *int64
	StopWait               *int64
	Pigz                   *int64
	Zstd                   *int64
	StdExcludes            *bool
	Quiet                  *bool
	Stop                   *bool
	Remove                 *bool
	Protected              *bool
	Stdout                 *bool
	Fleecing               *BackupFleecing
	Performance            *BackupPerformance
	PruneBackups           *BackupPruneBackups
}

// vzdumpRunWire mirrors the wire shape of VzdumpRunOptions with PVE's
// hyphenated parameter keys.
type vzdumpRunWire struct {
	Mode                   string   `json:"mode,omitempty"`
	Compress               string   `json:"compress,omitempty"`
	Storage                string   `json:"storage,omitempty"`
	DumpDir                string   `json:"dumpdir,omitempty"`
	TmpDir                 string   `json:"tmpdir,omitempty"`
	Script                 string   `json:"script,omitempty"`
	JobID                  string   `json:"job-id,omitempty"`
	Pool                   string   `json:"pool,omitempty"`
	NotesTemplate          string   `json:"notes-template,omitempty"`
	MailTo                 string   `json:"mailto,omitempty"`
	MailNotification       string   `json:"mailnotification,omitempty"`
	NotificationMode       string   `json:"notification-mode,omitempty"`
	PBSChangeDetectionMode string   `json:"pbs-change-detection-mode,omitempty"`
	VMIDs                  string   `json:"vmid,omitempty"`
	Exclude                string   `json:"exclude,omitempty"`
	ExcludePath            []string `json:"exclude-path,omitempty"`
	All                    *bool    `json:"all,omitempty"`
	BWLimit                *int64   `json:"bwlimit,omitempty"`
	IONice                 *int64   `json:"ionice,omitempty"`
	LockWait               *int64   `json:"lockwait,omitempty"`
	StopWait               *int64   `json:"stopwait,omitempty"`
	Pigz                   *int64   `json:"pigz,omitempty"`
	Zstd                   *int64   `json:"zstd,omitempty"`
	StdExcludes            *bool    `json:"stdexcludes,omitempty"`
	Quiet                  *bool    `json:"quiet,omitempty"`
	Stop                   *bool    `json:"stop,omitempty"`
	Remove                 *bool    `json:"remove,omitempty"`
	Protected              *bool    `json:"protected,omitempty"`
	Stdout                 *bool    `json:"stdout,omitempty"`
	Fleecing               string   `json:"fleecing,omitempty"`
	Performance            string   `json:"performance,omitempty"`
	PruneBackups           string   `json:"prune-backups,omitempty"`
}

// wire converts the options into the wire shape, joining the guest lists
// and rendering the grouped options as property strings.
func (o VzdumpRunOptions) wire() vzdumpRunWire {
	w := vzdumpRunWire{
		Mode:                   o.Mode,
		Compress:               o.Compress,
		Storage:                o.Storage,
		DumpDir:                o.DumpDir,
		TmpDir:                 o.TmpDir,
		Script:                 o.Script,
		JobID:                  o.JobID,
		Pool:                   o.Pool,
		NotesTemplate:          o.NotesTemplate,
		MailTo:                 o.MailTo,
		MailNotification:       o.MailNotification,
		NotificationMode:       o.NotificationMode,
		PBSChangeDetectionMode: o.PBSChangeDetectionMode,
		VMIDs:                  strings.Join(o.VMIDs, ","),
		Exclude:                strings.Join(o.Exclude, ","),
		ExcludePath:            o.ExcludePath,
		All:                    o.All,
		BWLimit:                o.BWLimit,
		IONice:                 o.IONice,
		LockWait:               o.LockWait,
		StopWait:               o.StopWait,
		Pigz:                   o.Pigz,
		Zstd:                   o.Zstd,
		StdExcludes:            o.StdExcludes,
		Quiet:                  o.Quiet,
		Stop:                   o.Stop,
		Remove:                 o.Remove,
		Protected:              o.Protected,
		Stdout:                 o.Stdout,
	}
	if o.Fleecing != nil {
		w.Fleecing = backupFleecingString(o.Fleecing)
	}
	if o.Performance != nil {
		w.Performance = backupPerformanceString(o.Performance)
	}
	if o.PruneBackups != nil {
		w.PruneBackups = backupPruneBackupsString(o.PruneBackups)
	}
	return w
}

// RunVzdumpBackup starts a backup with vzdump
// (POST /nodes/{node}/vzdump) and returns the worker task UPID for the
// caller to wait on.
func (c *Client) RunVzdumpBackup(ctx context.Context, node string, opts VzdumpRunOptions) (string, error) {
	path := fmt.Sprintf("/nodes/%s/vzdump", node)
	var upid string
	if err := c.Do(ctx, "POST", path, opts.wire(), &upid); err != nil {
		return "", fmt.Errorf("pveclient: start vzdump backup on node %s: %w", node, err)
	}
	return upid, nil
}

// RefreshSubscription updates the node's subscription info
// (POST /nodes/{node}/subscription): the node contacts the Proxmox
// subscription server and re-runs the subscription post-check. The call is
// synchronous per the pin (returns null); force connects to the server
// even if the local cache is still valid.
func (c *Client) RefreshSubscription(ctx context.Context, node string, force *bool) error {
	body := struct {
		Force *bool `json:"force,omitempty"`
	}{Force: force}
	path := fmt.Sprintf("/nodes/%s/subscription", node)
	return c.Do(ctx, "POST", path, body, nil)
}

// InitializeDiskGPT initializes a block device with a GPT table
// (POST /nodes/{node}/disks/initgpt) and returns the worker task UPID.
// An empty uuid omits the parameter, letting PVE generate one.
func (c *Client) InitializeDiskGPT(ctx context.Context, node, disk, uuid string) (string, error) {
	body := struct {
		Disk string `json:"disk"`
		UUID string `json:"uuid,omitempty"`
	}{Disk: disk, UUID: uuid}
	path := fmt.Sprintf("/nodes/%s/disks/initgpt", node)
	var upid string
	if err := c.Do(ctx, "POST", path, body, &upid); err != nil {
		return "", fmt.Errorf("pveclient: initialize GPT on %s at node %s: %w", disk, node, err)
	}
	return upid, nil
}

// CancelTask stops a running task (DELETE /nodes/{node}/tasks/{upid}).
// The call is synchronous per the pin (returns null): the worker receives
// the stop request and exits with a failure status; callers that care
// about the final status can poll WaitForTask, which reports the non-OK
// exit.
func (c *Client) CancelTask(ctx context.Context, node, upid string) error {
	path := fmt.Sprintf("/nodes/%s/tasks/%s", node, upid)
	return c.Do(ctx, "DELETE", path, nil, nil)
}

// DownloadApplianceTemplate downloads an appliance template to a storage
// (POST /nodes/{node}/aplinfo; upstream operation apl_download) and
// returns the worker task UPID.
func (c *Client) DownloadApplianceTemplate(ctx context.Context, node, storage, template string) (string, error) {
	body := struct {
		Storage  string `json:"storage"`
		Template string `json:"template"`
	}{Storage: storage, Template: template}
	path := fmt.Sprintf("/nodes/%s/aplinfo", node)
	var upid string
	if err := c.Do(ctx, "POST", path, body, &upid); err != nil {
		return "", fmt.Errorf("pveclient: download appliance template %s to storage %s at node %s: %w", template, storage, node, err)
	}
	return upid, nil
}

// PullOCIImage pulls an OCI image from a registry into a storage
// (POST /nodes/{node}/storage/{storage}/oci-registry-pull) and returns the
// worker task UPID. An empty filename omits the parameter, letting PVE
// derive the normalized destination name.
func (c *Client) PullOCIImage(ctx context.Context, node, storage, reference, filename string) (string, error) {
	body := struct {
		Reference string `json:"reference"`
		Filename  string `json:"filename,omitempty"`
	}{Reference: reference, Filename: filename}
	path := fmt.Sprintf("/nodes/%s/storage/%s/oci-registry-pull", node, storage)
	var upid string
	if err := c.Do(ctx, "POST", path, body, &upid); err != nil {
		return "", fmt.Errorf("pveclient: pull OCI image %s into storage %s at node %s: %w", reference, storage, node, err)
	}
	return upid, nil
}
