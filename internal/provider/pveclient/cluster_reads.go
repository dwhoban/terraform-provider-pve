// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
)

// ClusterResource is one row of GET /cluster/resources. The pointer-free
// shape follows the other list clients: absent optional fields decode as
// zero values, which the provider layer renders as null. Shared and
// Template tolerate the 0/1 int encoding PVE emits on some versions.
type ClusterResource struct {
	CgroupMode  int     `json:"cgroup-mode,omitempty"`
	Content     string  `json:"content,omitempty"`
	CPU         float64 `json:"cpu,omitempty"`
	Disk        int64   `json:"disk,omitempty"`
	DiskRead    int64   `json:"diskread,omitempty"`
	DiskWrite   int64   `json:"diskwrite,omitempty"`
	HAState     string  `json:"hastate,omitempty"`
	HostArch    string  `json:"host-arch,omitempty"`
	ID          string  `json:"id"`
	Level       string  `json:"level,omitempty"`
	Lock        string  `json:"lock,omitempty"`
	MaxCPU      float64 `json:"maxcpu,omitempty"`
	MaxDisk     int64   `json:"maxdisk,omitempty"`
	MaxMem      int64   `json:"maxmem,omitempty"`
	Mem         int64   `json:"mem,omitempty"`
	MemHost     int64   `json:"memhost,omitempty"`
	Name        string  `json:"name,omitempty"`
	NetIn       int64   `json:"netin,omitempty"`
	NetOut      int64   `json:"netout,omitempty"`
	Network     string  `json:"network,omitempty"`
	NetworkType string  `json:"network-type,omitempty"`
	Node        string  `json:"node,omitempty"`
	PluginType  string  `json:"plugintype,omitempty"`
	Pool        string  `json:"pool,omitempty"`
	Protocol    string  `json:"protocol,omitempty"`
	SDN         string  `json:"sdn,omitempty"`
	Shared      bool    `json:"shared,omitempty"`
	Status      string  `json:"status,omitempty"`
	Storage     string  `json:"storage,omitempty"`
	Tags        string  `json:"tags,omitempty"`
	Template    bool    `json:"template,omitempty"`
	Type        string  `json:"type"`
	Uptime      int64   `json:"uptime,omitempty"`
	VMID        int64   `json:"vmid,omitempty"`
	ZoneType    string  `json:"zone-type,omitempty"`
}

// clusterResourceRaw shadows ClusterResource for wire decoding, keeping
// the boolean fields raw so the lenient decodeBoolish can parse them.
type clusterResourceRaw struct {
	CgroupMode  json.RawMessage `json:"cgroup-mode,omitempty"`
	Content     string          `json:"content,omitempty"`
	CPU         float64         `json:"cpu,omitempty"`
	Disk        int64           `json:"disk,omitempty"`
	DiskRead    int64           `json:"diskread,omitempty"`
	DiskWrite   int64           `json:"diskwrite,omitempty"`
	HAState     string          `json:"hastate,omitempty"`
	HostArch    string          `json:"host-arch,omitempty"`
	ID          string          `json:"id"`
	Level       string          `json:"level,omitempty"`
	Lock        string          `json:"lock,omitempty"`
	MaxCPU      float64         `json:"maxcpu,omitempty"`
	MaxDisk     int64           `json:"maxdisk,omitempty"`
	MaxMem      int64           `json:"maxmem,omitempty"`
	Mem         int64           `json:"mem,omitempty"`
	MemHost     int64           `json:"memhost,omitempty"`
	Name        string          `json:"name,omitempty"`
	NetIn       int64           `json:"netin,omitempty"`
	NetOut      int64           `json:"netout,omitempty"`
	Network     string          `json:"network,omitempty"`
	NetworkType string          `json:"network-type,omitempty"`
	Node        string          `json:"node,omitempty"`
	PluginType  string          `json:"plugintype,omitempty"`
	Pool        string          `json:"pool,omitempty"`
	Protocol    string          `json:"protocol,omitempty"`
	SDN         string          `json:"sdn,omitempty"`
	Shared      json.RawMessage `json:"shared,omitempty"`
	Status      string          `json:"status,omitempty"`
	Storage     string          `json:"storage,omitempty"`
	Tags        string          `json:"tags,omitempty"`
	Template    json.RawMessage `json:"template,omitempty"`
	Type        string          `json:"type"`
	Uptime      int64           `json:"uptime,omitempty"`
	VMID        int64           `json:"vmid,omitempty"`
	ZoneType    string          `json:"zone-type,omitempty"`
}

// UnmarshalJSON tolerates the 0/1 int encoding of shared and template
// that PVE emits on some versions.
func (r *ClusterResource) UnmarshalJSON(data []byte) error {
	var raw clusterResourceRaw
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	r.CgroupMode = intFromRaw(raw.CgroupMode)
	r.Content = raw.Content
	r.CPU = raw.CPU
	r.Disk = raw.Disk
	r.DiskRead = raw.DiskRead
	r.DiskWrite = raw.DiskWrite
	r.HAState = raw.HAState
	r.HostArch = raw.HostArch
	r.ID = raw.ID
	r.Level = raw.Level
	r.Lock = raw.Lock
	r.MaxCPU = raw.MaxCPU
	r.MaxDisk = raw.MaxDisk
	r.MaxMem = raw.MaxMem
	r.Mem = raw.Mem
	r.MemHost = raw.MemHost
	r.Name = raw.Name
	r.NetIn = raw.NetIn
	r.NetOut = raw.NetOut
	r.Network = raw.Network
	r.NetworkType = raw.NetworkType
	r.Node = raw.Node
	r.PluginType = raw.PluginType
	r.Pool = raw.Pool
	r.Protocol = raw.Protocol
	r.SDN = raw.SDN
	r.Shared = decodeBoolish(raw.Shared)
	r.Status = raw.Status
	r.Storage = raw.Storage
	r.Tags = raw.Tags
	r.Template = decodeBoolish(raw.Template)
	r.Type = raw.Type
	r.Uptime = raw.Uptime
	r.VMID = raw.VMID
	r.ZoneType = raw.ZoneType
	return nil
}

// ClusterStatusEntry is one entry of GET /cluster/status. PVE returns one
// summary entry with type "cluster" plus one entry per node with type
// "node"; the fields not relevant to an entry's type are absent.
type ClusterStatusEntry struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	IP      string `json:"ip,omitempty"`
	Level   string `json:"level,omitempty"`
	Local   bool   `json:"local,omitempty"`
	NodeID  int    `json:"nodeid,omitempty"`
	Nodes   int    `json:"nodes,omitempty"`
	Online  bool   `json:"online,omitempty"`
	Quorate bool   `json:"quorate,omitempty"`
	Version int64  `json:"version,omitempty"`
}

// clusterStatusEntryRaw shadows ClusterStatusEntry for wire decoding,
// keeping the boolean fields raw for lenient decoding.
type clusterStatusEntryRaw struct {
	ID      string          `json:"id"`
	Name    string          `json:"name"`
	Type    string          `json:"type"`
	IP      string          `json:"ip,omitempty"`
	Level   string          `json:"level,omitempty"`
	Local   json.RawMessage `json:"local,omitempty"`
	NodeID  int             `json:"nodeid,omitempty"`
	Nodes   int             `json:"nodes,omitempty"`
	Online  json.RawMessage `json:"online,omitempty"`
	Quorate json.RawMessage `json:"quorate,omitempty"`
	Version int64           `json:"version,omitempty"`
}

// UnmarshalJSON tolerates the 0/1 int encoding of local, online, and
// quorate that PVE emits on some versions.
func (e *ClusterStatusEntry) UnmarshalJSON(data []byte) error {
	var raw clusterStatusEntryRaw
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	e.ID = raw.ID
	e.Name = raw.Name
	e.Type = raw.Type
	e.IP = raw.IP
	e.Level = raw.Level
	e.Local = decodeBoolish(raw.Local)
	e.NodeID = raw.NodeID
	e.Nodes = raw.Nodes
	e.Online = decodeBoolish(raw.Online)
	e.Quorate = decodeBoolish(raw.Quorate)
	e.Version = raw.Version
	return nil
}

// ClusterTask is one entry of GET /cluster/tasks (recent tasks cluster
// wide). The shape mirrors the task list entries the pin documents for
// /nodes/{node}/tasks; the endpoint's own return schema only names upid.
type ClusterTask struct {
	UPID      string `json:"upid"`
	Node      string `json:"node,omitempty"`
	Type      string `json:"type,omitempty"`
	User      string `json:"user,omitempty"`
	Status    string `json:"status,omitempty"`
	StartTime int64  `json:"starttime,omitempty"`
	EndTime   int64  `json:"endtime,omitempty"`
	ID        string `json:"id,omitempty"`
	PID       int    `json:"pid,omitempty"`
	PStart    int64  `json:"pstart,omitempty"`
}

// PVEVersion is the decoded body of GET /version.
type PVEVersion struct {
	Console string `json:"console,omitempty"`
	Release string `json:"release"`
	RepoID  string `json:"repoid"`
	Version string `json:"version"`
}

// intFromRaw decodes an optional raw integer field, returning 0 when
// absent.
func intFromRaw(raw json.RawMessage) int {
	var out int
	if len(raw) == 0 {
		return 0
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return 0
	}
	return out
}

// GetClusterResources enumerates GET /cluster/resources. resourceType
// optionally filters server-side; valid values are the pin's enum "vm",
// "storage", "node", and "sdn" (empty string sends no filter).
func (c *Client) GetClusterResources(ctx context.Context, resourceType string) ([]ClusterResource, error) {
	path := "/cluster/resources"
	if resourceType != "" {
		path += "?type=" + url.QueryEscape(resourceType)
	}
	var out []ClusterResource
	if err := c.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetClusterStatus reads GET /cluster/status (summary plus per-node
// entries).
func (c *Client) GetClusterStatus(ctx context.Context) ([]ClusterStatusEntry, error) {
	var out []ClusterStatusEntry
	if err := c.Do(ctx, "GET", "/cluster/status", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetClusterTotem reads GET /cluster/config/totem. The pin types the
// response as an opaque object, so the settings are returned as a map of
// string values (JSON strings unquoted, other scalars rendered literally).
func (c *Client) GetClusterTotem(ctx context.Context) (map[string]string, error) {
	return c.getClusterStringMap(ctx, "/cluster/config/totem")
}

// GetClusterQDevice reads GET /cluster/config/qdevice ("Get QDevice
// status"). Like totem, the pin types the response as an opaque object.
func (c *Client) GetClusterQDevice(ctx context.Context) (map[string]string, error) {
	return c.getClusterStringMap(ctx, "/cluster/config/qdevice")
}

// getClusterStringMap fetches path and decodes its object envelope into
// string values via clusterRawScalar.
func (c *Client) getClusterStringMap(ctx context.Context, path string) (map[string]string, error) {
	raw := map[string]json.RawMessage{}
	if err := c.Do(ctx, "GET", path, nil, &raw); err != nil {
		return nil, err
	}
	out := make(map[string]string, len(raw))
	for key, value := range raw {
		out[key] = clusterRawScalar(value)
	}
	return out, nil
}

// clusterRawScalar renders one JSON value as a string: quoted strings are
// unquoted, every other value keeps its literal JSON token text.
func clusterRawScalar(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return strings.TrimSpace(string(raw))
}

// ListClusterTasks enumerates GET /cluster/tasks (recent tasks cluster
// wide).
func (c *Client) ListClusterTasks(ctx context.Context) ([]ClusterTask, error) {
	var out []ClusterTask
	if err := c.Do(ctx, "GET", "/cluster/tasks", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetNextID reads GET /cluster/nextid and returns the next free VMID.
func (c *Client) GetNextID(ctx context.Context) (int64, error) {
	var out int64
	if err := c.Do(ctx, "GET", "/cluster/nextid", nil, &out); err != nil {
		return 0, err
	}
	return out, nil
}

// GetVersion reads GET /version.
func (c *Client) GetVersion(ctx context.Context) (*PVEVersion, error) {
	var out PVEVersion
	if err := c.Do(ctx, "GET", "/version", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
