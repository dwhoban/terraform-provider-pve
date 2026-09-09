// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// CephStatus is the decoded node-scoped Ceph cluster status
// (GET /nodes/{node}/ceph/status). The pin types the payload as a raw
// object, so RawJSON carries the untouched body while Health, Quorum,
// QuorumNames and Versions surface the well-known summary fields callers
// consistently need.
type CephStatus struct {
	// RawJSON is the compact JSON encoding of the raw `ceph status` object.
	RawJSON string
	// Health is the aggregate health summary (e.g. `HEALTH_OK`).
	Health string
	// Quorum lists the ranks of the monitors forming the current quorum.
	Quorum []int
	// QuorumNames lists the names of the monitors in quorum.
	QuorumNames []string
	// Versions maps daemon version strings to the number of daemons
	// reporting that version.
	Versions map[string]int64
}

// cephStatusSummary mirrors the subset of the raw status object the typed
// summary fields are derived from.
type cephStatusSummary struct {
	Health struct {
		Status string `json:"status"`
	} `json:"health"`
	Mon struct {
		Quorum      []int    `json:"quorum"`
		QuorumNames []string `json:"quorum_names"`
	} `json:"mon"`
	Versions map[string]int64 `json:"version"`
}

// GetCephStatus fetches GET /nodes/{node}/ceph/status. The pin documents
// the payload as identical to /cluster/ceph/status; the node-scoped alias
// keeps the data source node-keyed.
func (c *Client) GetCephStatus(ctx context.Context, node string) (*CephStatus, error) {
	var raw json.RawMessage
	path := fmt.Sprintf("/nodes/%s/ceph/status", node)
	if err := c.Do(ctx, "GET", path, nil, &raw); err != nil {
		return nil, err
	}
	var summary cephStatusSummary
	if err := json.Unmarshal(raw, &summary); err != nil {
		return nil, fmt.Errorf("pveclient: decode ceph status for %s: %w", node, err)
	}
	return &CephStatus{
		RawJSON:     string(raw),
		Health:      summary.Health.Status,
		Quorum:      summary.Mon.Quorum,
		QuorumNames: summary.Mon.QuorumNames,
		Versions:    summary.Versions,
	}, nil
}

// GetCephMetadata fetches GET /cluster/ceph/metadata (cluster-scoped; the
// pin defines no node-scoped metadata alias) and returns the raw JSON.
func (c *Client) GetCephMetadata(ctx context.Context) (string, error) {
	var raw json.RawMessage
	if err := c.Do(ctx, "GET", "/cluster/ceph/metadata", nil, &raw); err != nil {
		return "", err
	}
	return string(raw), nil
}

// CephPool mirrors one entry of GET /nodes/{node}/ceph/pool. The object
// fields the pin leaves untyped (application metadata, autoscaler status)
// are preserved as raw JSON.
type CephPool struct {
	Pool                int             `json:"pool"`
	PoolName            string          `json:"pool_name"`
	Size                int             `json:"size"`
	MinSize             int             `json:"min_size"`
	PGNum               int             `json:"pg_num"`
	PGNumMin            *int            `json:"pg_num_min,omitempty"`
	PGNumFinal          *int            `json:"pg_num_final,omitempty"`
	PGAutoscaleMode     string          `json:"pg_autoscale_mode,omitempty"`
	CrushRule           int             `json:"crush_rule"`
	CrushRuleName       string          `json:"crush_rule_name,omitempty"`
	TargetSize          *int64          `json:"target_size,omitempty"`
	TargetSizeRatio     *float64        `json:"target_size_ratio,omitempty"`
	Type                string          `json:"type,omitempty"`
	BytesUsed           *int64          `json:"bytes_used,omitempty"`
	PercentUsed         *float64        `json:"percent_used,omitempty"`
	ApplicationMetadata json.RawMessage `json:"application_metadata,omitempty"`
	AutoscaleStatus     json.RawMessage `json:"autoscale_status,omitempty"`
}

// ListCephPools enumerates GET /nodes/{node}/ceph/pool.
func (c *Client) ListCephPools(ctx context.Context, node string) ([]CephPool, error) {
	var out []CephPool
	path := fmt.Sprintf("/nodes/%s/ceph/pool", node)
	if err := c.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetCephPool reads a single pool from the listing; the pin defines no
// single-pool GET with typed fields. A missing pool yields an *APIError
// with status 404 so callers can treat it like any absent object.
func (c *Client) GetCephPool(ctx context.Context, node, name string) (*CephPool, error) {
	pools, err := c.ListCephPools(ctx, node)
	if err != nil {
		return nil, err
	}
	for i := range pools {
		if pools[i].PoolName == name {
			return &pools[i], nil
		}
	}
	return nil, &APIError{
		StatusCode: http.StatusNotFound,
		Method:     http.MethodGet,
		Path:       fmt.Sprintf("/nodes/%s/ceph/pool/%s", node, name),
		Errors:     []string{fmt.Sprintf("ceph pool %q not found", name)},
	}
}

// CreateCephPoolInput is the payload for POST /nodes/{node}/ceph/pool.
// Zero values and nil pointers are omitted from the request body.
type CreateCephPoolInput struct {
	Name            string
	Size            *int
	MinSize         *int
	PGNum           *int
	PGNumMin        *int
	PGAutoscaleMode string
	Application     string
	CrushRule       string
	TargetSize      string
	TargetSizeRatio *float64
	AddStorages     *bool
	// ErasureCoding is the pin's `erasure-coding` property string
	// (`k=<int> ,m=<int> [,device-class=<class>] [,failure-domain=<domain>]
	// [,profile=<profile>]`).
	ErasureCoding string
}

// CreateCephPool POSTs /nodes/{node}/ceph/pool and returns the upid.
func (c *Client) CreateCephPool(ctx context.Context, node string, in CreateCephPoolInput) (string, error) {
	body := map[string]any{"name": in.Name}
	if in.Size != nil {
		body["size"] = *in.Size
	}
	if in.MinSize != nil {
		body["min_size"] = *in.MinSize
	}
	if in.PGNum != nil {
		body["pg_num"] = *in.PGNum
	}
	if in.PGNumMin != nil {
		body["pg_num_min"] = *in.PGNumMin
	}
	if in.PGAutoscaleMode != "" {
		body["pg_autoscale_mode"] = in.PGAutoscaleMode
	}
	if in.Application != "" {
		body["application"] = in.Application
	}
	if in.CrushRule != "" {
		body["crush_rule"] = in.CrushRule
	}
	if in.TargetSize != "" {
		body["target_size"] = in.TargetSize
	}
	if in.TargetSizeRatio != nil {
		body["target_size_ratio"] = *in.TargetSizeRatio
	}
	if in.AddStorages != nil {
		body["add_storages"] = *in.AddStorages
	}
	if in.ErasureCoding != "" {
		body["erasure-coding"] = in.ErasureCoding
	}
	var upid string
	path := fmt.Sprintf("/nodes/%s/ceph/pool", node)
	if err := c.Do(ctx, "POST", path, body, &upid); err != nil {
		return "", err
	}
	return upid, nil
}

// UpdateCephPoolInput is the payload for PUT /nodes/{node}/ceph/pool/{name}
// (setpool). Zero values and nil pointers are omitted, leaving the
// corresponding pool setting untouched.
type UpdateCephPoolInput struct {
	Size            *int
	MinSize         *int
	PGNum           *int
	PGNumMin        *int
	PGAutoscaleMode string
	Application     string
	CrushRule       string
	TargetSize      string
	TargetSizeRatio *float64
}

// UpdateCephPool PUTs /nodes/{node}/ceph/pool/{name} and returns the upid.
func (c *Client) UpdateCephPool(ctx context.Context, node, name string, in UpdateCephPoolInput) (string, error) {
	body := map[string]any{}
	if in.Size != nil {
		body["size"] = *in.Size
	}
	if in.MinSize != nil {
		body["min_size"] = *in.MinSize
	}
	if in.PGNum != nil {
		body["pg_num"] = *in.PGNum
	}
	if in.PGNumMin != nil {
		body["pg_num_min"] = *in.PGNumMin
	}
	if in.PGAutoscaleMode != "" {
		body["pg_autoscale_mode"] = in.PGAutoscaleMode
	}
	if in.Application != "" {
		body["application"] = in.Application
	}
	if in.CrushRule != "" {
		body["crush_rule"] = in.CrushRule
	}
	if in.TargetSize != "" {
		body["target_size"] = in.TargetSize
	}
	if in.TargetSizeRatio != nil {
		body["target_size_ratio"] = *in.TargetSizeRatio
	}
	var upid string
	path := fmt.Sprintf("/nodes/%s/ceph/pool/%s", node, name)
	if err := c.Do(ctx, "PUT", path, body, &upid); err != nil {
		return "", err
	}
	return upid, nil
}

// DeleteCephPool DELETEs /nodes/{node}/ceph/pool/{name} and returns the
// upid. force destroys the pool even when in use; removeECProfile drops the
// erasure code profile; removeStorages removes pveceph-managed storage
// entries backed by the pool.
func (c *Client) DeleteCephPool(ctx context.Context, node, name string, force, removeECProfile, removeStorages bool) (string, error) {
	q := url.Values{}
	q.Set("force", encodeBoolFlag(force))
	q.Set("remove_ecprofile", encodeBoolFlag(removeECProfile))
	q.Set("remove_storages", encodeBoolFlag(removeStorages))
	var upid string
	path := fmt.Sprintf("/nodes/%s/ceph/pool/%s?%s", node, name, q.Encode())
	if err := c.Do(ctx, "DELETE", path, nil, &upid); err != nil {
		return "", err
	}
	return upid, nil
}

// CephOSDDevice describes one entry of the metadata endpoint's devices
// array (block, db or wal device backing an OSD).
type CephOSDDevice struct {
	DevNode        string `json:"dev_node"`
	Device         string `json:"device"`
	PhysicalDevice string `json:"physical_device"`
	Size           int64  `json:"size"`
	SupportDiscard bool   `json:"support_discard"`
	Type           string `json:"type"`
}

// CephOSDMetadata mirrors GET /nodes/{node}/ceph/osd/{osdid}/metadata,
// merging the `osd` object and the `devices` array into one struct.
// Encrypted is decoded leniently because PVE emits it as bool or 0/1.
type CephOSDMetadata struct {
	ID             int
	Hostname       string
	OSDData        string
	OSDObjectStore string
	Encrypted      bool
	PID            *int
	Version        string
	FrontAddr      string
	BackAddr       string
	HBFrontAddr    string
	HBBackAddr     string
	MemUsage       int64
	Devices        []CephOSDDevice
}

// cephOSDMetadataRaw is the wire shape with boolish fields as raw JSON.
type cephOSDMetadataRaw struct {
	OSD struct {
		ID             int             `json:"id"`
		Hostname       string          `json:"hostname"`
		OSDData        string          `json:"osd_data"`
		OSDObjectStore string          `json:"osd_objectstore"`
		Encrypted      json.RawMessage `json:"encrypted"`
		PID            *int            `json:"pid"`
		Version        string          `json:"version"`
		FrontAddr      string          `json:"front_addr"`
		BackAddr       string          `json:"back_addr"`
		HBFrontAddr    string          `json:"hb_front_addr"`
		HBBackAddr     string          `json:"hb_back_addr"`
		MemUsage       int64           `json:"mem_usage"`
	} `json:"osd"`
	Devices []CephOSDDevice `json:"devices"`
}

// UnmarshalJSON decodes the raw metadata shape, resolving boolish fields.
func (m *CephOSDMetadata) UnmarshalJSON(data []byte) error {
	var raw cephOSDMetadataRaw
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	m.ID = raw.OSD.ID
	m.Hostname = raw.OSD.Hostname
	m.OSDData = raw.OSD.OSDData
	m.OSDObjectStore = raw.OSD.OSDObjectStore
	if v := decodeBoolishPtr(raw.OSD.Encrypted); v != nil {
		m.Encrypted = *v
	}
	m.PID = raw.OSD.PID
	m.Version = raw.OSD.Version
	m.FrontAddr = raw.OSD.FrontAddr
	m.BackAddr = raw.OSD.BackAddr
	m.HBFrontAddr = raw.OSD.HBFrontAddr
	m.HBBackAddr = raw.OSD.HBBackAddr
	m.MemUsage = raw.OSD.MemUsage
	m.Devices = raw.Devices
	return nil
}

// GetCephOSDMetadata reads GET /nodes/{node}/ceph/osd/{osdid}/metadata.
func (c *Client) GetCephOSDMetadata(ctx context.Context, node string, osdid int) (*CephOSDMetadata, error) {
	var out CephOSDMetadata
	path := fmt.Sprintf("/nodes/%s/ceph/osd/%d/metadata", node, osdid)
	if err := c.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CephOSDTreeEntry is one OSD leaf of the CRUSH tree returned by
// GET /nodes/{node}/ceph/osd.
type CephOSDTreeEntry struct {
	ID     int
	Name   string
	Type   string
	Status string
	Class  string
}

// cephOSDTreeNode is the recursive CRUSH bucket shape; only the fields the
// provider consumes are decoded.
type cephOSDTreeNode struct {
	ID       *int              `json:"id"`
	Name     string            `json:"name"`
	Type     string            `json:"type"`
	Status   string            `json:"status"`
	Class    string            `json:"class"`
	Children []cephOSDTreeNode `json:"children"`
}

// ListCephOSDTree walks the CRUSH tree returned by GET
// /nodes/{node}/ceph/osd and returns the OSD leaves (host and root buckets
// are traversed but not returned).
func (c *Client) ListCephOSDTree(ctx context.Context, node string) ([]CephOSDTreeEntry, error) {
	var payload struct {
		Root *cephOSDTreeNode `json:"root"`
	}
	path := fmt.Sprintf("/nodes/%s/ceph/osd", node)
	if err := c.Do(ctx, "GET", path, nil, &payload); err != nil {
		return nil, err
	}
	var out []CephOSDTreeEntry
	var walk func(n *cephOSDTreeNode)
	walk = func(n *cephOSDTreeNode) {
		if n == nil {
			return
		}
		if n.Type == "osd" && n.ID != nil {
			out = append(out, CephOSDTreeEntry{ID: *n.ID, Name: n.Name, Type: n.Type, Status: n.Status, Class: n.Class})
		}
		for i := range n.Children {
			walk(&n.Children[i])
		}
	}
	walk(payload.Root)
	return out, nil
}

// CreateCephOSDInput is the payload for POST /nodes/{node}/ceph/osd.
// Empty strings, zero numbers and false are omitted from the request body.
type CreateCephOSDInput struct {
	Dev              string
	DBDev            string
	DBDevSizeGiB     float64
	WALDev           string
	WALDevSizeGiB    float64
	CrushDeviceClass string
	Encrypted        bool
	// OSDsPerDevice creates multiple OSD services per physical device; it
	// is mutually exclusive with DBDev and WALDev.
	OSDsPerDevice int
}

// CreateCephOSD POSTs /nodes/{node}/ceph/osd and returns the upid. The OSD
// id is assigned by Ceph during the task, not by this call.
func (c *Client) CreateCephOSD(ctx context.Context, node string, in CreateCephOSDInput) (string, error) {
	body := map[string]any{"dev": in.Dev}
	if in.DBDev != "" {
		body["db_dev"] = in.DBDev
	}
	if in.DBDevSizeGiB > 0 {
		body["db_dev_size"] = in.DBDevSizeGiB
	}
	if in.WALDev != "" {
		body["wal_dev"] = in.WALDev
	}
	if in.WALDevSizeGiB > 0 {
		body["wal_dev_size"] = in.WALDevSizeGiB
	}
	if in.CrushDeviceClass != "" {
		body["crush-device-class"] = in.CrushDeviceClass
	}
	if in.Encrypted {
		body["encrypted"] = true
	}
	if in.OSDsPerDevice > 0 {
		body["osds-per-device"] = in.OSDsPerDevice
	}
	var upid string
	path := fmt.Sprintf("/nodes/%s/ceph/osd", node)
	if err := c.Do(ctx, "POST", path, body, &upid); err != nil {
		return "", err
	}
	return upid, nil
}

// CephOSDIn marks an OSD in (POST /nodes/{node}/ceph/osd/{osdid}/in). The
// verb returns no upid.
func (c *Client) CephOSDIn(ctx context.Context, node string, osdid int) error {
	path := fmt.Sprintf("/nodes/%s/ceph/osd/%d/in", node, osdid)
	return c.Do(ctx, "POST", path, nil, nil)
}

// CephOSDOut marks an OSD out (POST /nodes/{node}/ceph/osd/{osdid}/out).
// The verb returns no upid.
func (c *Client) CephOSDOut(ctx context.Context, node string, osdid int) error {
	path := fmt.Sprintf("/nodes/%s/ceph/osd/%d/out", node, osdid)
	return c.Do(ctx, "POST", path, nil, nil)
}

// DestroyCephOSD DELETEs /nodes/{node}/ceph/osd/{osdid} and returns the
// upid. cleanup additionally destroys the underlying logical volumes,
// removes the volume group's physical volume and wipes leftover journal or
// block.db/block.wal partitions.
func (c *Client) DestroyCephOSD(ctx context.Context, node string, osdid int, cleanup bool) (string, error) {
	q := url.Values{}
	q.Set("cleanup", encodeBoolFlag(cleanup))
	var upid string
	path := fmt.Sprintf("/nodes/%s/ceph/osd/%d?%s", node, osdid, q.Encode())
	if err := c.Do(ctx, "DELETE", path, nil, &upid); err != nil {
		return "", err
	}
	return upid, nil
}

// CephMon mirrors one entry of GET /nodes/{node}/ceph/mon. The boolean
// fields are decoded leniently because PVE emits bool or 0/1.
type CephMon struct {
	Name             string
	Addr             string
	Host             string
	State            string
	Rank             *int
	Quorum           *bool
	Service          *bool
	DirExists        *bool
	CephVersion      string
	CephVersionShort string
}

// cephMonRaw is the wire shape with boolish fields as raw JSON.
type cephMonRaw struct {
	Name             string          `json:"name"`
	Addr             string          `json:"addr"`
	Host             string          `json:"host"`
	State            string          `json:"state"`
	Rank             *int            `json:"rank"`
	Quorum           json.RawMessage `json:"quorum"`
	Service          json.RawMessage `json:"service"`
	DirExists        json.RawMessage `json:"direxists"`
	CephVersion      string          `json:"ceph_version"`
	CephVersionShort string          `json:"ceph_version_short"`
}

// UnmarshalJSON decodes the raw monitor shape, resolving boolish fields.
func (m *CephMon) UnmarshalJSON(data []byte) error {
	var raw cephMonRaw
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	m.Name = raw.Name
	m.Addr = raw.Addr
	m.Host = raw.Host
	m.State = raw.State
	m.Rank = raw.Rank
	m.Quorum = decodeBoolishPtr(raw.Quorum)
	m.Service = decodeBoolishPtr(raw.Service)
	m.DirExists = decodeBoolishPtr(raw.DirExists)
	m.CephVersion = raw.CephVersion
	m.CephVersionShort = raw.CephVersionShort
	return nil
}

// ListCephMons enumerates GET /nodes/{node}/ceph/mon.
func (c *Client) ListCephMons(ctx context.Context, node string) ([]CephMon, error) {
	var out []CephMon
	path := fmt.Sprintf("/nodes/%s/ceph/mon", node)
	if err := c.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetCephMon reads a single monitor from the listing; the pin defines no
// single-monitor GET. A missing monitor yields an *APIError with status
// 404 so callers can treat it like any absent object.
func (c *Client) GetCephMon(ctx context.Context, node, monid string) (*CephMon, error) {
	mons, err := c.ListCephMons(ctx, node)
	if err != nil {
		return nil, err
	}
	for i := range mons {
		if mons[i].Name == monid {
			return &mons[i], nil
		}
	}
	return nil, &APIError{
		StatusCode: http.StatusNotFound,
		Method:     http.MethodGet,
		Path:       fmt.Sprintf("/nodes/%s/ceph/mon/%s", node, monid),
		Errors:     []string{fmt.Sprintf("ceph monitor %q not found", monid)},
	}
}

// CreateCephMon POSTs /nodes/{node}/ceph/mon/{monid} (createmon) and
// returns the upid. An empty monid targets the pin's default (the node
// name): the request goes to /nodes/{node}/ceph/mon/<node> without a monid
// body parameter so PVE applies its own default. monAddress overwrites the
// autodetected monitor address when non-empty.
func (c *Client) CreateCephMon(ctx context.Context, node, monid, monAddress string) (string, error) {
	body := map[string]any{}
	if monid != "" {
		body["monid"] = monid
	}
	if monAddress != "" {
		body["mon-address"] = monAddress
	}
	effective := monid
	if effective == "" {
		effective = node
	}
	var upid string
	path := fmt.Sprintf("/nodes/%s/ceph/mon/%s", node, effective)
	if err := c.Do(ctx, "POST", path, body, &upid); err != nil {
		return "", err
	}
	return upid, nil
}

// DestroyCephMon DELETEs /nodes/{node}/ceph/mon/{monid} (destroymon) and
// returns the upid.
func (c *Client) DestroyCephMon(ctx context.Context, node, monid string) (string, error) {
	var upid string
	path := fmt.Sprintf("/nodes/%s/ceph/mon/%s", node, monid)
	if err := c.Do(ctx, "DELETE", path, nil, &upid); err != nil {
		return "", err
	}
	return upid, nil
}
