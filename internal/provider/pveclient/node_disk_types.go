// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"context"
	"fmt"
	"net/url"
)

// LVMThinpool describes one entry returned by GET
// /nodes/{node}/disks/lvmthin.
type LVMThinpool struct {
	LV           string `json:"lv"`
	LVSize       int64  `json:"lv_size,omitempty"`
	MetadataSize int64  `json:"metadata_size,omitempty"`
	MetadataUsed int64  `json:"metadata_used,omitempty"`
	Used         int64  `json:"used,omitempty"`
	VG           string `json:"vg"`
}

// ListLVMThinpools enumerates /nodes/{node}/disks/lvmthin.
func (c *Client) ListLVMThinpools(ctx context.Context, node string) ([]LVMThinpool, error) {
	var out []LVMThinpool
	path := fmt.Sprintf("/nodes/%s/disks/lvmthin", node)
	if err := c.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// CreateLVMThinpoolInput is the payload for POST /nodes/{node}/disks/lvmthin.
type CreateLVMThinpoolInput struct {
	Name       string
	Device     string
	AddStorage bool
}

// CreateLVMThinpool POSTs /nodes/{node}/disks/lvmthin and returns the upid.
// The caller is expected to feed the upid to WaitForTask.
func (c *Client) CreateLVMThinpool(ctx context.Context, node string, in CreateLVMThinpoolInput) (string, error) {
	body := map[string]any{
		"name":   in.Name,
		"device": in.Device,
	}
	if in.AddStorage {
		body["add_storage"] = true
	}
	var upid string
	path := fmt.Sprintf("/nodes/%s/disks/lvmthin", node)
	if err := c.Do(ctx, "POST", path, body, &upid); err != nil {
		return "", err
	}
	return upid, nil
}

// DeleteLVMThinpool DELETEs /nodes/{node}/disks/lvmthin/{name} with the
// supplied cleanup flags and returns the upid. cleanupDisks wipes the
// underlying disk so it can be repurposed; cleanupConfig removes the
// auto-created storage entry when add_storage was used at create time.
//
// Like DeleteLVMVG, the flags go on the query string as 0/1 integers via
// encodeBoolFlag.
func (c *Client) DeleteLVMThinpool(ctx context.Context, node, name string, cleanupConfig, cleanupDisks bool) (string, error) {
	q := url.Values{}
	q.Set("cleanup-config", encodeBoolFlag(cleanupConfig))
	q.Set("cleanup-disks", encodeBoolFlag(cleanupDisks))
	var upid string
	path := fmt.Sprintf("/nodes/%s/disks/lvmthin/%s?%s", node, name, q.Encode())
	if err := c.Do(ctx, "DELETE", path, nil, &upid); err != nil {
		return "", err
	}
	return upid, nil
}

// DirectoryStorage describes one entry returned by GET
// /nodes/{node}/disks/directory. PVE mounts the filesystem under
// /mnt/pve/<name>, which callers match via Path.
type DirectoryStorage struct {
	Device   string `json:"device,omitempty"`
	Options  string `json:"options,omitempty"`
	Path     string `json:"path,omitempty"`
	Type     string `json:"type,omitempty"`
	UnitFile string `json:"unitfile,omitempty"`
}

// ListNodeDirectories enumerates /nodes/{node}/disks/directory.
func (c *Client) ListNodeDirectories(ctx context.Context, node string) ([]DirectoryStorage, error) {
	var out []DirectoryStorage
	path := fmt.Sprintf("/nodes/%s/disks/directory", node)
	if err := c.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// CreateNodeDirectoryInput is the payload for POST
// /nodes/{node}/disks/directory. Filesystem accepts ext4 or xfs per the
// pinned API; an empty value is omitted so PVE applies its ext4 default.
type CreateNodeDirectoryInput struct {
	Name       string
	Device     string
	Filesystem string
	AddStorage bool
}

func (c *Client) CreateNodeDirectory(ctx context.Context, node string, in CreateNodeDirectoryInput) (string, error) {
	body := map[string]any{
		"name":   in.Name,
		"device": in.Device,
	}
	if in.Filesystem != "" {
		body["filesystem"] = in.Filesystem
	}
	if in.AddStorage {
		body["add_storage"] = true
	}
	var upid string
	path := fmt.Sprintf("/nodes/%s/disks/directory", node)
	if err := c.Do(ctx, "POST", path, body, &upid); err != nil {
		return "", err
	}
	return upid, nil
}

// DeleteNodeDirectory DELETEs /nodes/{node}/disks/directory/{name}: it
// unmounts the storage and removes the mount unit. cleanupDisks wipes the
// underlying disk; cleanupConfig removes the auto-created storage entry
// when add_storage was used at create time. The flags go on the query
// string as 0/1 integers via encodeBoolFlag.
func (c *Client) DeleteNodeDirectory(ctx context.Context, node, name string, cleanupConfig, cleanupDisks bool) (string, error) {
	q := url.Values{}
	q.Set("cleanup-config", encodeBoolFlag(cleanupConfig))
	q.Set("cleanup-disks", encodeBoolFlag(cleanupDisks))
	var upid string
	path := fmt.Sprintf("/nodes/%s/disks/directory/%s?%s", node, name, q.Encode())
	if err := c.Do(ctx, "DELETE", path, nil, &upid); err != nil {
		return "", err
	}
	return upid, nil
}
