// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// CreateLxcContainerParams is the POST /nodes/{node}/lxc body. MountPoints
// carries the rendered rootfs/mpN strings under their config keys ("mp0",
// "rootfs", ...) and is merged into the JSON object on marshal.
type CreateLxcContainerParams struct {
	VMID          int64             `json:"vmid"`
	OSTemplate    string            `json:"ostemplate,omitempty"`
	Hostname      string            `json:"hostname,omitempty"`
	Description   string            `json:"description,omitempty"`
	Tags          string            `json:"tags,omitempty"`
	Onboot        *bool             `json:"onboot,omitempty"`
	Protection    *bool             `json:"protection,omitempty"`
	Template      *bool             `json:"template,omitempty"`
	Unprivileged  *bool             `json:"unprivileged,omitempty"`
	Cores         *int64            `json:"cores,omitempty"`
	Memory        *int64            `json:"memory,omitempty"`
	Swap          *int64            `json:"swap,omitempty"`
	Password      string            `json:"password,omitempty"`
	SSHPublicKeys string            `json:"ssh-public-keys,omitempty"`
	Nameserver    string            `json:"nameserver,omitempty"`
	Searchdomain  string            `json:"searchdomain,omitempty"`
	Start         *bool             `json:"start,omitempty"`
	Pool          string            `json:"pool,omitempty"`
	Storage       string            `json:"storage,omitempty"`
	Rootfs        string            `json:"rootfs,omitempty"`
	MountPoints   map[string]string `json:"-"`
}

// MarshalJSON merges the mount point config keys into the flat PVE body.
func (p CreateLxcContainerParams) MarshalJSON() ([]byte, error) {
	return lxcMergeMountPoints(aliasCreateLxc(p), p.MountPoints)
}

// aliasCreateLxc avoids recursion in MarshalJSON.
type aliasCreateLxc CreateLxcContainerParams

// UpdateLxcConfigParams is the PUT /nodes/{node}/lxc/{vmid}/config body for
// the modeled subset. Delete is the comma-separated list of config keys to
// remove; MountPoints carries re-rendered mpN/rootfs strings. Create-only
// keys (ostemplate, password, ssh-public-keys, start) are deliberately
// absent: the config endpoint rejects them.
type UpdateLxcConfigParams struct {
	Digest       string            `json:"digest,omitempty"`
	Delete       string            `json:"delete,omitempty"`
	Hostname     string            `json:"hostname,omitempty"`
	Description  string            `json:"description,omitempty"`
	Tags         string            `json:"tags,omitempty"`
	Onboot       *bool             `json:"onboot,omitempty"`
	Protection   *bool             `json:"protection,omitempty"`
	Template     *bool             `json:"template,omitempty"`
	Unprivileged *bool             `json:"unprivileged,omitempty"`
	Cores        *int64            `json:"cores,omitempty"`
	Memory       *int64            `json:"memory,omitempty"`
	Swap         *int64            `json:"swap,omitempty"`
	Nameserver   string            `json:"nameserver,omitempty"`
	Searchdomain string            `json:"searchdomain,omitempty"`
	MountPoints  map[string]string `json:"-"`
}

// MarshalJSON merges the mount point config keys into the flat PVE body.
func (p UpdateLxcConfigParams) MarshalJSON() ([]byte, error) {
	return lxcMergeMountPoints(aliasUpdateLxc(p), p.MountPoints)
}

// aliasUpdateLxc avoids recursion in MarshalJSON.
type aliasUpdateLxc UpdateLxcConfigParams

// lxcMergeMountPoints marshals base and overlays the mpN/rootfs keys.
func lxcMergeMountPoints(base any, mountPoints map[string]string) ([]byte, error) {
	raw, err := json.Marshal(base)
	if err != nil {
		return nil, err
	}
	if len(mountPoints) == 0 {
		return raw, nil
	}
	merged := map[string]json.RawMessage{}
	if err := json.Unmarshal(raw, &merged); err != nil {
		return nil, err
	}
	for key, value := range mountPoints {
		encoded, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		merged[key] = encoded
	}
	return json.Marshal(merged)
}

// CloneLxcContainerParams is the POST /nodes/{node}/lxc/{vmid}/clone body.
// NewID is required by the pin; callers allocate one via GetNextID when the
// target vmid is null.
type CloneLxcContainerParams struct {
	NewID       int64  `json:"newid"`
	Hostname    string `json:"hostname,omitempty"`
	Description string `json:"description,omitempty"`
	Full        *bool  `json:"full,omitempty"`
	Pool        string `json:"pool,omitempty"`
	Storage     string `json:"storage,omitempty"`
	SnapName    string `json:"snapname,omitempty"`
	Target      string `json:"target,omitempty"`
}

// DeleteLxcContainerParams carries the DELETE /nodes/{node}/lxc/{vmid}
// destroy options.
type DeleteLxcContainerParams struct {
	Force                    *bool `json:"force,omitempty"`
	Purge                    *bool `json:"purge,omitempty"`
	DestroyUnreferencedDisks *bool `json:"destroy-unreferenced-disks,omitempty"`
}

// LxcShutdownParams carries the POST .../status/shutdown options. The pin
// spells force stop as "forceStop" (camelCase).
type LxcShutdownParams struct {
	Timeout   *int64 `json:"timeout,omitempty"`
	ForceStop *bool  `json:"forceStop,omitempty"`
}

// LxcMigrateParams is the POST .../migrate body; Target is required.
type LxcMigrateParams struct {
	Target        string   `json:"target"`
	TargetStorage string   `json:"target-storage,omitempty"`
	Restart       *bool    `json:"restart,omitempty"`
	Online        *bool    `json:"online,omitempty"`
	Bwlimit       *float64 `json:"bwlimit,omitempty"`
	Timeout       *int64   `json:"timeout,omitempty"`
}

// LxcMountPoint is one parsed rootfs/mpN config entry. Extra preserves
// options this provider does not model (quota, replicate, idmap, ...) so a
// re-render never silently drops upstream configuration.
type LxcMountPoint struct {
	Volume     string
	Mountpoint string
	Size       string
	ACL        *bool
	Backup     *bool
	ReadOnly   *bool
	Extra      map[string]string
}

// ParseLxcMountPoint parses a rootfs/mpN property string of the form
// "[volume=]<volume>[,key=value...]"; the volume segment has no "key=" form.
func ParseLxcMountPoint(raw string) LxcMountPoint {
	mp := LxcMountPoint{Extra: map[string]string{}}
	segments := strings.Split(raw, ",")
	for i, segment := range segments {
		key, value, hasKey := strings.Cut(segment, "=")
		switch {
		case i == 0 && !hasKey:
			mp.Volume = segment
		case i == 0 && key == "volume":
			mp.Volume = value
		case key == "mp":
			mp.Mountpoint = value
		case key == "size":
			mp.Size = value
		case key == "acl":
			mp.ACL = nodeNetworkBoolishPtr([]byte(value))
		case key == "backup":
			mp.Backup = nodeNetworkBoolishPtr([]byte(value))
		case key == "ro":
			mp.ReadOnly = nodeNetworkBoolishPtr([]byte(value))
		default:
			mp.Extra[key] = value
		}
	}
	return mp
}

// Render rebuilds the property string: volume first, then mp/size/flags,
// then the preserved extra options in sorted order for determinism.
func (m LxcMountPoint) Render() string {
	var sb strings.Builder
	sb.WriteString(m.Volume)
	if m.Mountpoint != "" {
		sb.WriteString(",mp=" + m.Mountpoint)
	}
	if m.Size != "" {
		sb.WriteString(",size=" + m.Size)
	}
	if m.ACL != nil {
		sb.WriteString(",acl=" + lxcBoolText(*m.ACL))
	}
	if m.Backup != nil {
		sb.WriteString(",backup=" + lxcBoolText(*m.Backup))
	}
	if m.ReadOnly != nil {
		sb.WriteString(",ro=" + lxcBoolText(*m.ReadOnly))
	}
	keys := make([]string, 0, len(m.Extra))
	for key := range m.Extra {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		sb.WriteString("," + key + "=" + m.Extra[key])
	}
	return sb.String()
}

// Storage returns the storage part of a volume-backed mount point (""
// for bind mounts and raw device paths).
func (m LxcMountPoint) Storage() string {
	storage, _, found := strings.Cut(m.Volume, ":")
	if !found {
		return ""
	}
	return storage
}

// VolumeID returns the volume id part of a volume-backed mount point, or
// the whole volume string for bind mounts and raw device paths.
func (m LxcMountPoint) VolumeID() string {
	_, volumeID, found := strings.Cut(m.Volume, ":")
	if !found {
		return m.Volume
	}
	return volumeID
}

// lxcBoolText renders a PVE config boolean as 1/0.
func lxcBoolText(v bool) string {
	if v {
		return "1"
	}
	return "0"
}

// LxcConfig is the decoded GET /nodes/{node}/lxc/{vmid}/config body for the
// modeled subset. PVE emits config values with mixed encodings (quoted
// numbers, 0/1 booleans), so decoding goes through a raw map.
type LxcConfig struct {
	Hostname     string
	Description  string
	Tags         string
	Onboot       *bool
	Protection   *bool
	Template     *bool
	Unprivileged *bool
	Cores        *int64
	Memory       *int64
	Swap         *int64
	Nameserver   string
	Searchdomain string
	Rootfs       *LxcMountPoint
	MountPoints  map[string]LxcMountPoint
	Digest       string
}

// UnmarshalJSON decodes PVE's mixed config value encodings and parses the
// rootfs/mpN property strings.
func (c *LxcConfig) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	c.Hostname = lxcStringFromRaw(raw["hostname"])
	c.Description = lxcStringFromRaw(raw["description"])
	c.Tags = lxcStringFromRaw(raw["tags"])
	c.Onboot = lxcBoolPtrFromRaw(raw["onboot"])
	c.Protection = lxcBoolPtrFromRaw(raw["protection"])
	c.Template = lxcBoolPtrFromRaw(raw["template"])
	c.Unprivileged = lxcBoolPtrFromRaw(raw["unprivileged"])
	c.Cores = lxcInt64PtrFromRaw(raw["cores"])
	c.Memory = lxcInt64PtrFromRaw(raw["memory"])
	c.Swap = lxcInt64PtrFromRaw(raw["swap"])
	c.Nameserver = lxcStringFromRaw(raw["nameserver"])
	c.Searchdomain = lxcStringFromRaw(raw["searchdomain"])
	c.Digest = lxcStringFromRaw(raw["digest"])
	c.MountPoints = map[string]LxcMountPoint{}
	for key, value := range raw {
		switch {
		case key == "rootfs":
			mp := ParseLxcMountPoint(lxcStringFromRaw(value))
			c.Rootfs = &mp
		case lxcIsMountPointKey(key):
			c.MountPoints[key] = ParseLxcMountPoint(lxcStringFromRaw(value))
		}
	}
	return nil
}

// lxcIsMountPointKey reports whether key is an mpN mount point config key.
func lxcIsMountPointKey(key string) bool {
	if !strings.HasPrefix(key, "mp") || len(key) <= 2 {
		return false
	}
	_, err := strconv.Atoi(key[2:])
	return err == nil
}

// lxcStringFromRaw decodes a JSON string value, returning "" for absent or
// null entries.
func lxcStringFromRaw(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return ""
}

// lxcInt64PtrFromRaw decodes PVE's int-or-quoted-int config values, keeping
// nil for absent or null entries.
func lxcInt64PtrFromRaw(raw json.RawMessage) *int64 {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil
	}
	trimmed := bytes.Trim(bytes.TrimSpace(raw), `"`)
	v, err := strconv.ParseInt(string(trimmed), 10, 64)
	if err != nil {
		return nil
	}
	return &v
}

// lxcBoolPtrFromRaw decodes PVE's 0/1-or-quoted config booleans, keeping
// nil for absent or null entries.
func lxcBoolPtrFromRaw(raw json.RawMessage) *bool {
	if len(raw) == 0 {
		return nil
	}
	trimmed := bytes.Trim(bytes.TrimSpace(raw), `"`)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil
	}
	v := decodeBoolish(trimmed)
	return &v
}

// LxcStatus is the decoded GET .../status/current body subset.
type LxcStatus struct {
	Name     string `json:"name,omitempty"`
	Status   string `json:"status,omitempty"`
	Lock     string `json:"lock,omitempty"`
	Template *bool  `json:"-"`
	Uptime   *int64 `json:"-"`
}

// UnmarshalJSON decodes the boolish template flag and integer uptime.
func (s *LxcStatus) UnmarshalJSON(data []byte) error {
	var raw struct {
		Name   string                     `json:"name"`
		Status string                     `json:"status"`
		Lock   string                     `json:"lock"`
		Raw    map[string]json.RawMessage `json:"-"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	s.Name = raw.Name
	s.Status = raw.Status
	s.Lock = raw.Lock
	s.Template = lxcBoolPtrFromRaw(fields["template"])
	s.Uptime = lxcInt64PtrFromRaw(fields["uptime"])
	return nil
}

// LxcPendingChange is one row of GET .../pending.
type LxcPendingChange struct {
	Key     string  `json:"key"`
	Value   *string `json:"value,omitempty"`
	Pending *string `json:"pending,omitempty"`
	Delete  *int64  `json:"delete,omitempty"`
}

// LxcInterface is one row of GET .../interfaces (guest IP discovery).
type LxcInterface struct {
	Name            string `json:"name"`
	HWAddr          string `json:"hwaddr,omitempty"`
	HardwareAddress string `json:"hardware-address,omitempty"`
	Inet            string `json:"inet,omitempty"`
	Inet6           string `json:"inet6,omitempty"`
}

// CreateLxcContainer POSTs /nodes/{node}/lxc and returns the create task
// UPID. The caller waits on it with WaitForTask.
func (c *Client) CreateLxcContainer(ctx context.Context, node string, params CreateLxcContainerParams) (string, error) {
	var upid string
	path := fmt.Sprintf("/nodes/%s/lxc", node)
	if err := c.Do(ctx, "POST", path, params, &upid); err != nil {
		return "", err
	}
	return upid, nil
}

// CloneLxcContainer POSTs .../clone and returns the clone task UPID.
func (c *Client) CloneLxcContainer(ctx context.Context, node string, vmid int64, params CloneLxcContainerParams) (string, error) {
	var upid string
	path := fmt.Sprintf("/nodes/%s/lxc/%d/clone", node, vmid)
	if err := c.Do(ctx, "POST", path, params, &upid); err != nil {
		return "", err
	}
	return upid, nil
}

// GetLxcConfig reads GET .../config (the modeled subset plus digest).
func (c *Client) GetLxcConfig(ctx context.Context, node string, vmid int64) (*LxcConfig, error) {
	var out LxcConfig
	path := fmt.Sprintf("/nodes/%s/lxc/%d/config", node, vmid)
	if err := c.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetLxcStatus reads GET .../status/current.
func (c *Client) GetLxcStatus(ctx context.Context, node string, vmid int64) (*LxcStatus, error) {
	var out LxcStatus
	path := fmt.Sprintf("/nodes/%s/lxc/%d/status/current", node, vmid)
	if err := c.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateLxcConfig PUTs .../config; the verb is synchronous and returns null.
func (c *Client) UpdateLxcConfig(ctx context.Context, node string, vmid int64, params UpdateLxcConfigParams) error {
	path := fmt.Sprintf("/nodes/%s/lxc/%d/config", node, vmid)
	return c.Do(ctx, "PUT", path, params, nil)
}

// DeleteLxcContainer DELETEs .../{vmid} and returns the destroy task UPID.
func (c *Client) DeleteLxcContainer(ctx context.Context, node string, vmid int64, params DeleteLxcContainerParams) (string, error) {
	var upid string
	path := fmt.Sprintf("/nodes/%s/lxc/%d", node, vmid)
	if err := c.Do(ctx, "DELETE", path, params, &upid); err != nil {
		return "", err
	}
	return upid, nil
}

// LxcStart POSTs .../status/start and returns the start task UPID.
func (c *Client) LxcStart(ctx context.Context, node string, vmid int64) (string, error) {
	return c.lxcPowerTask(ctx, node, vmid, "start", nil)
}

// LxcStop POSTs .../status/stop and returns the stop task UPID.
func (c *Client) LxcStop(ctx context.Context, node string, vmid int64) (string, error) {
	return c.lxcPowerTask(ctx, node, vmid, "stop", nil)
}

// LxcShutdown POSTs .../status/shutdown with the supplied options and
// returns the shutdown task UPID.
func (c *Client) LxcShutdown(ctx context.Context, node string, vmid int64, params LxcShutdownParams) (string, error) {
	return c.lxcPowerTask(ctx, node, vmid, "shutdown", params)
}

// lxcPowerTask POSTs one of the status power verbs.
func (c *Client) lxcPowerTask(ctx context.Context, node string, vmid int64, verb string, body any) (string, error) {
	var upid string
	path := fmt.Sprintf("/nodes/%s/lxc/%d/status/%s", node, vmid, verb)
	if err := c.Do(ctx, "POST", path, body, &upid); err != nil {
		return "", err
	}
	return upid, nil
}

// MigrateLxcContainer POSTs .../migrate and returns the migration task
// UPID; the task runs on the source node, so the caller waits there.
func (c *Client) MigrateLxcContainer(ctx context.Context, node string, vmid int64, params LxcMigrateParams) (string, error) {
	var upid string
	path := fmt.Sprintf("/nodes/%s/lxc/%d/migrate", node, vmid)
	if err := c.Do(ctx, "POST", path, params, &upid); err != nil {
		return "", err
	}
	return upid, nil
}

// ResizeLxcRootfs PUTs .../resize for the rootfs disk and returns the
// resize task UPID.
func (c *Client) ResizeLxcRootfs(ctx context.Context, node string, vmid int64, size string) (string, error) {
	return c.lxcResize(ctx, node, vmid, "rootfs", size)
}

// ResizeLxcMountpoint PUTs .../resize for one mpN mount point and returns
// the resize task UPID.
func (c *Client) ResizeLxcMountpoint(ctx context.Context, node string, vmid int64, mountPoint string, size string) (string, error) {
	return c.lxcResize(ctx, node, vmid, mountPoint, size)
}

// lxcResize issues the shared resize verb; PVE only grows volumes.
func (c *Client) lxcResize(ctx context.Context, node string, vmid int64, disk, size string) (string, error) {
	var upid string
	path := fmt.Sprintf("/nodes/%s/lxc/%d/resize", node, vmid)
	body := map[string]string{"disk": disk, "size": size}
	if err := c.Do(ctx, "PUT", path, body, &upid); err != nil {
		return "", err
	}
	return upid, nil
}

// GetLxcPending reads GET .../pending; rows with a pending value or a
// delete flag carry configuration not yet applied.
func (c *Client) GetLxcPending(ctx context.Context, node string, vmid int64) ([]LxcPendingChange, error) {
	var out []LxcPendingChange
	path := fmt.Sprintf("/nodes/%s/lxc/%d/pending", node, vmid)
	if err := c.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetLxcInterfaces reads GET .../interfaces (guest IP discovery, requires a
// running container). The pin's "hardware-address" alias is folded into
// HWAddr so callers see one field.
func (c *Client) GetLxcInterfaces(ctx context.Context, node string, vmid int64) ([]LxcInterface, error) {
	var out []LxcInterface
	path := fmt.Sprintf("/nodes/%s/lxc/%d/interfaces", node, vmid)
	if err := c.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	for i := range out {
		if out[i].HWAddr == "" {
			out[i].HWAddr = out[i].HardwareAddress
		}
	}
	return out, nil
}
