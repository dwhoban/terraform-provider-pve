// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// Disk describes one entry returned by GET /nodes/{node}/disks/list. Fields
// are best-effort populated because PVE only populates the columns
// relevant to the device (USB sticks report no RAID info, etc.). GPT and
// Mounted are encoded as 0/1 integers in PVE's JSON, hence the custom
// UnmarshalJSON.
type Disk struct {
	DevPath string `json:"devpath"`
	Size    int64  `json:"size"`
	Used    string `json:"used,omitempty"`
	GPT     bool   `json:"-"`
	Mounted bool   `json:"-"`
	OSDID   int    `json:"osdid,omitempty"`
	Vendor  string `json:"vendor,omitempty"`
	Model   string `json:"model,omitempty"`
	Serial  string `json:"serial,omitempty"`
	WWN     string `json:"wwn,omitempty"`
	Health  string `json:"health,omitempty"`
	Parent  string `json:"parent,omitempty"`
	Type    string `json:"type,omitempty"`
}

// diskRaw is the wire shape with boolish fields as json.RawMessage so we
// can decode int 0/1 and bool true/false interchangeably.
type diskRaw struct {
	DevPath string          `json:"devpath"`
	Size    int64           `json:"size"`
	Used    string          `json:"used,omitempty"`
	GPT     json.RawMessage `json:"gpt"`
	Mounted json.RawMessage `json:"mounted,omitempty"`
	OSDID   int             `json:"osdid,omitempty"`
	Vendor  string          `json:"vendor,omitempty"`
	Model   string          `json:"model,omitempty"`
	Serial  string          `json:"serial,omitempty"`
	WWN     string          `json:"wwn,omitempty"`
	Health  string          `json:"health,omitempty"`
	Parent  string          `json:"parent,omitempty"`
	Type    string          `json:"type,omitempty"`
}

func (d *Disk) UnmarshalJSON(data []byte) error {
	var r diskRaw
	if err := json.Unmarshal(data, &r); err != nil {
		return err
	}
	d.DevPath = r.DevPath
	d.Size = r.Size
	d.Used = r.Used
	d.GPT = decodeBoolish(r.GPT)
	d.Mounted = decodeBoolish(r.Mounted)
	d.OSDID = r.OSDID
	d.Vendor = r.Vendor
	d.Model = r.Model
	d.Serial = r.Serial
	d.WWN = r.WWN
	d.Health = r.Health
	d.Parent = r.Parent
	d.Type = r.Type
	return nil
}

// ListNodeDisks enumerates /nodes/{node}/disks/list.
func (c *Client) ListNodeDisks(ctx context.Context, node string) ([]Disk, error) {
	var out []Disk
	path := fmt.Sprintf("/nodes/%s/disks/list", node)
	if err := c.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// DiskSmart is one row of /nodes/{node}/disks/smart — used to merge health
// and temperature metadata onto the disk list when callers ask for it.
type DiskSmart struct {
	Name      string `json:"name"`
	Health    string `json:"health,omitempty"`
	Type      string `json:"type,omitempty"`
	Attribute string `json:"attribute,omitempty"`
	RawValue  string `json:"raw_value,omitempty"`
	Wearout   string `json:"wearout,omitempty"`
}

// ListNodeDiskSmart enumerates /nodes/{node}/disks/smart. The result is
// keyed by disk name (e.g. "/dev/sda") which callers match against
// Disk.DevPath.
func (c *Client) ListNodeDiskSmart(ctx context.Context, node string) ([]DiskSmart, error) {
	var out []DiskSmart
	path := fmt.Sprintf("/nodes/%s/disks/smart", node)
	if err := c.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ZFSPool describes one entry returned by GET /nodes/{node}/disks/zfs and
// GET /nodes/{node}/disks/zfs/{name}. Children carry the vdev tree as a
// recursive any-typed slice — Terraform does not need to introspect the
// tree, just preserve the JSON so a refresh round-trip is lossless.
type ZFSPool struct {
	Name        string `json:"name"`
	State       string `json:"state"`
	Scan        string `json:"scan,omitempty"`
	Errors      string `json:"errors,omitempty"`
	Children    []any  `json:"children,omitempty"`
	RaidLevel   string `json:"raidlevel"`
	Compression string `json:"compression,omitempty"`
	Ashift      int    `json:"ashift,omitempty"`
	Dedup       string `json:"dedup,omitempty"`
	FRAG        string `json:"frag,omitempty"`
	Free        int64  `json:"free,omitempty"`
	Size        int64  `json:"size,omitempty"`
	Alloc       int64  `json:"alloc,omitempty"`
}

// ListZFSPools enumerates /nodes/{node}/disks/zfs.
func (c *Client) ListZFSPools(ctx context.Context, node string) ([]ZFSPool, error) {
	var out []ZFSPool
	path := fmt.Sprintf("/nodes/%s/disks/zfs", node)
	if err := c.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetZFSPool reads /nodes/{node}/disks/zfs/{name}.
func (c *Client) GetZFSPool(ctx context.Context, node, name string) (*ZFSPool, error) {
	var out ZFSPool
	path := fmt.Sprintf("/nodes/%s/disks/zfs/%s", node, name)
	if err := c.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateZFSPoolInput is the payload for POST /nodes/{node}/disks/zfs.
type CreateZFSPoolInput struct {
	Name        string
	RaidLevel   string
	Devices     []string
	Ashift      int
	Compression string
	DRAIDConfig string
	AddStorage  bool
}

// CreateZFSPool POSTs /nodes/{node}/disks/zfs and returns the upid. The
// caller is expected to feed the upid to WaitForTask.
func (c *Client) CreateZFSPool(ctx context.Context, node string, in CreateZFSPoolInput) (string, error) {
	body := map[string]any{
		"name":      in.Name,
		"raidlevel": in.RaidLevel,
		"devices":   strings.Join(in.Devices, ","),
	}
	if in.Ashift > 0 {
		body["ashift"] = in.Ashift
	}
	if in.Compression != "" {
		body["compression"] = in.Compression
	}
	if in.DRAIDConfig != "" {
		body["draidconfig"] = in.DRAIDConfig
	}
	if in.AddStorage {
		body["add_storage"] = true
	}
	var upid string
	path := fmt.Sprintf("/nodes/%s/disks/zfs", node)
	if err := c.Do(ctx, "POST", path, body, &upid); err != nil {
		return "", err
	}
	return upid, nil
}

// DeleteZFSPool DELETEs /nodes/{node}/disks/zfs/{name} and returns the
// resulting upid.
func (c *Client) DeleteZFSPool(ctx context.Context, node, name string) (string, error) {
	var upid string
	path := fmt.Sprintf("/nodes/%s/disks/zfs/%s", node, name)
	if err := c.Do(ctx, "DELETE", path, nil, &upid); err != nil {
		return "", err
	}
	return upid, nil
}

// LVMVolumeGroup describes one entry returned by GET /nodes/{node}/disks/lvm.
type LVMVolumeGroup struct {
	Name string `json:"vg"`
	Size int64  `json:"size,omitempty"`
	Free int64  `json:"free,omitempty"`
}

// ListLVMVolumeGroups enumerates /nodes/{node}/disks/lvm.
func (c *Client) ListLVMVolumeGroups(ctx context.Context, node string) ([]LVMVolumeGroup, error) {
	var out []LVMVolumeGroup
	path := fmt.Sprintf("/nodes/%s/disks/lvm", node)
	if err := c.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// CreateLVMVGInput is the payload for POST /nodes/{node}/disks/lvm.
type CreateLVMVGInput struct {
	Name       string
	Devices    []string
	AddStorage bool
}

// CreateLVMVG POSTs /nodes/{node}/disks/lvm and returns the upid.
func (c *Client) CreateLVMVG(ctx context.Context, node string, in CreateLVMVGInput) (string, error) {
	body := map[string]any{
		"name":    in.Name,
		"devices": strings.Join(in.Devices, ","),
	}
	if in.AddStorage {
		body["add_storage"] = true
	}
	var upid string
	path := fmt.Sprintf("/nodes/%s/disks/lvm", node)
	if err := c.Do(ctx, "POST", path, body, &upid); err != nil {
		return "", err
	}
	return upid, nil
}

// DeleteLVMVG DELETEs /nodes/{node}/disks/lvm/{name} with the supplied
// cleanup flags and returns the upid. cleanupDisks removes leftover PVs
// from the underlying disks; cleanupConfig removes the auto-created storage
// entry when add_storage was used at create time.
//
// PVE expects the cleanup-* query parameters as 0/1 integers; the values
// go through encodeBoolFlag to match.
func (c *Client) DeleteLVMVG(ctx context.Context, node, name string, cleanupConfig, cleanupDisks bool) (string, error) {
	q := url.Values{}
	q.Set("cleanup-config", encodeBoolFlag(cleanupConfig))
	q.Set("cleanup-disks", encodeBoolFlag(cleanupDisks))
	var upid string
	path := fmt.Sprintf("/nodes/%s/disks/lvm/%s?%s", node, name, q.Encode())
	if err := c.Do(ctx, "DELETE", path, nil, &upid); err != nil {
		return "", err
	}
	return upid, nil
}

// encodeBoolFlag formats a bool as the 0/1 integer PVE expects on query
// parameters such as cleanup-disks and cleanup-config.
func encodeBoolFlag(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// silence unused-import lint when strconv is otherwise idle.
var _ = strconv.Itoa
