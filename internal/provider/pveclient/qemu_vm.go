// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// QemuVMConfigInput is the modeled subset of the PVE VM configuration that
// the provider sends to POST /nodes/{node}/qemu and PUT .../config. Simple
// typed keys map to their pin parameters; compound keys (drives, network
// devices, cloud-init ipconfig entries) arrive pre-rendered in the PVE
// config syntax, because only the caller knows how the Terraform model maps
// onto them. Nil fields are omitted from the request body — PVE config PUTs
// are merges, so unspecified keys are never sent and never removed.
type QemuVMConfigInput struct {
	Name        *string
	Description *string
	// Tags renders as the pin's pve-tag-list string ("a;b").
	Tags []string
	// BootOrder renders as the pin's boot parameter "order=a;b".
	BootOrder []string

	Onboot     *bool
	Protection *bool
	Template   *bool
	Agent      *bool

	BIOS    *string
	Machine *string
	OSType  *string
	// CPUType renders into the pin's cpu parameter as "cputype=<value>".
	CPUType *string
	SCSIHW  *string

	Cores     *int64
	Sockets   *int64
	MemoryMiB *int64

	CIUser         *string
	CIPassword     *string
	CISearchDomain *string
	CINameserver   *string
	// CISSHKeys is percent-encoded on the wire (the pin marks sshkeys as
	// format urlencoded; raw newlines would break the line-based config).
	CISSHKeys   *string
	CIIpconfigs map[string]string

	// Drives maps disk ids (scsi0, virtio1, efidisk0, ...) to rendered
	// drive values in PVE config syntax.
	Drives map[string]string
	// Networks maps net ids (net0, ...) to rendered device values.
	Networks map[string]string
}

// QemuVMConfigBody is the JSON request body rendered from a
// QemuVMConfigInput.
type QemuVMConfigBody map[string]any

// qemuVMConfigBody renders the input into a JSON request body.
func (in QemuVMConfigInput) qemuVMConfigBody() QemuVMConfigBody {
	body := QemuVMConfigBody{}
	setStr := func(key string, v *string) {
		if v != nil {
			body[key] = *v
		}
	}
	setInt := func(key string, v *int64) {
		if v != nil {
			body[key] = *v
		}
	}
	setBool := func(key string, v *bool) {
		if v != nil {
			body[key] = *v
		}
	}
	setStr("name", in.Name)
	setStr("description", in.Description)
	setStr("bios", in.BIOS)
	setStr("machine", in.Machine)
	setStr("ostype", in.OSType)
	setStr("scsihw", in.SCSIHW)
	setStr("ciuser", in.CIUser)
	setStr("cipassword", in.CIPassword)
	setStr("searchdomain", in.CISearchDomain)
	setStr("nameserver", in.CINameserver)
	setInt("cores", in.Cores)
	setInt("sockets", in.Sockets)
	setInt("memory", in.MemoryMiB)
	setBool("onboot", in.Onboot)
	setBool("protection", in.Protection)
	setBool("template", in.Template)
	// agent is a string-typed pin parameter (format "[enabled=]<1|0> ..."),
	// so render the enabled flag explicitly.
	setBoolAgent := in.Agent
	if setBoolAgent != nil {
		body["agent"] = qemuVMBoolFlag(*setBoolAgent)
	}
	if in.CPUType != nil && *in.CPUType != "" {
		body["cpu"] = "cputype=" + *in.CPUType
	}
	if len(in.Tags) > 0 {
		body["tags"] = strings.Join(in.Tags, ";")
	}
	if len(in.BootOrder) > 0 {
		body["boot"] = "order=" + strings.Join(in.BootOrder, ";")
	}
	if in.CISSHKeys != nil {
		body["sshkeys"] = qemuVMEncodeValue(*in.CISSHKeys)
	}
	for key, value := range in.CIIpconfigs {
		body[key] = value
	}
	for key, value := range in.Drives {
		body[key] = value
	}
	for key, value := range in.Networks {
		body[key] = value
	}
	return body
}

// qemuVMBoolFlag renders a boolean as the PVE "1"/"0" string flag used by
// string-typed compound parameters such as agent.
func qemuVMBoolFlag(v bool) string {
	if v {
		return "1"
	}
	return "0"
}

// qemuVMEncodeValue percent-encodes a value the way PVE's urlencoded
// format expects: every reserved byte escaped, spaces as %20.
func qemuVMEncodeValue(value string) string {
	return strings.ReplaceAll(url.QueryEscape(value), "+", "%20")
}

// CreateQemuVM POSTs /nodes/{node}/qemu and returns the create task UPID.
func (c *Client) CreateQemuVM(ctx context.Context, node string, vmid int64, in QemuVMConfigInput) (string, error) {
	body := in.qemuVMConfigBody()
	body["vmid"] = vmid
	var upid string
	if err := c.Do(ctx, http.MethodPost, "/nodes/"+node+"/qemu", body, &upid); err != nil {
		return "", fmt.Errorf("create qemu VM %d on node %s: %w", vmid, node, err)
	}
	return upid, nil
}

// QemuVMCloneOptions carries the pin's POST .../clone parameters.
type QemuVMCloneOptions struct {
	Name        string
	Description string
	// Full selects a full copy; nil omits the parameter and lets PVE
	// choose (linked clones for templates).
	Full      *bool
	Storage   string
	Format    string
	Pool      string
	Target    string
	Snapshot  string
	Bandwidth *int64
}

// CloneQemuVM POSTs .../clone and returns the clone task UPID.
func (c *Client) CloneQemuVM(ctx context.Context, node string, sourceVmid, newVmid int64, opts QemuVMCloneOptions) (string, error) {
	body := map[string]any{"newid": newVmid}
	if opts.Name != "" {
		body["name"] = opts.Name
	}
	if opts.Description != "" {
		body["description"] = opts.Description
	}
	if opts.Full != nil {
		body["full"] = *opts.Full
	}
	if opts.Storage != "" {
		body["storage"] = opts.Storage
	}
	if opts.Format != "" {
		body["format"] = opts.Format
	}
	if opts.Pool != "" {
		body["pool"] = opts.Pool
	}
	if opts.Target != "" {
		body["target"] = opts.Target
	}
	if opts.Snapshot != "" {
		body["snapname"] = opts.Snapshot
	}
	if opts.Bandwidth != nil {
		body["bwlimit"] = *opts.Bandwidth
	}
	var upid string
	path := fmt.Sprintf("/nodes/%s/qemu/%d/clone", node, sourceVmid)
	if err := c.Do(ctx, http.MethodPost, path, body, &upid); err != nil {
		return "", fmt.Errorf("clone qemu VM %d to %d on node %s: %w", sourceVmid, newVmid, node, err)
	}
	return upid, nil
}

// GetQemuVMConfig reads GET .../config as the raw config key map, so the
// provider can parse modeled keys while leaving unmodeled ones untouched.
func (c *Client) GetQemuVMConfig(ctx context.Context, node string, vmid int64) (map[string]json.RawMessage, error) {
	var config map[string]json.RawMessage
	path := fmt.Sprintf("/nodes/%s/qemu/%d/config", node, vmid)
	if err := c.Do(ctx, http.MethodGet, path, nil, &config); err != nil {
		return nil, fmt.Errorf("read qemu VM %d config on node %s: %w", vmid, node, err)
	}
	return config, nil
}

// qemuVMStatusRaw shadows QemuVMStatus for wire decoding, keeping the
// boolean and numeric fields raw so the lenient decoders can parse them.
type qemuVMStatusRaw struct {
	Status         string          `json:"status"`
	QMPStatus      string          `json:"qmpstatus"`
	Lock           string          `json:"lock"`
	Name           string          `json:"name"`
	Tags           string          `json:"tags"`
	Template       json.RawMessage `json:"template"`
	Agent          json.RawMessage `json:"agent"`
	Uptime         json.RawMessage `json:"uptime"`
	MaxMem         json.RawMessage `json:"maxmem"`
	MaxDisk        json.RawMessage `json:"maxdisk"`
	CPUs           json.RawMessage `json:"cpus"`
	PID            json.RawMessage `json:"pid"`
	RunningQemu    string          `json:"running-qemu"`
	RunningMachine string          `json:"running-machine"`
}

// QemuVMStatus is the decoded body of GET .../status/current.
type QemuVMStatus struct {
	Status    string
	QMPStatus string
	Lock      string
	Name      string
	Tags      string
	Template  bool
	Agent     bool
	Uptime    int64
	MaxMem    int64
	MaxDisk   int64
	MaxCPU    int64
	PID       int64
	// RunningQemu and RunningMachine are set while the VM is running.
	RunningQemu    string
	RunningMachine string
}

// qemuVMRawInt decodes an optional numeric field, tolerating both integer
// and float encodings, returning 0 when absent.
func qemuVMRawInt(raw json.RawMessage) int64 {
	if len(raw) == 0 {
		return 0
	}
	var number float64
	if err := json.Unmarshal(raw, &number); err != nil {
		return 0
	}
	return int64(number)
}

func (r qemuVMStatusRaw) toStatus() *QemuVMStatus {
	return &QemuVMStatus{
		Status:         r.Status,
		QMPStatus:      r.QMPStatus,
		Lock:           r.Lock,
		Name:           r.Name,
		Tags:           r.Tags,
		Template:       decodeBoolish(r.Template),
		Agent:          decodeBoolish(r.Agent),
		Uptime:         qemuVMRawInt(r.Uptime),
		MaxMem:         qemuVMRawInt(r.MaxMem),
		MaxDisk:        qemuVMRawInt(r.MaxDisk),
		MaxCPU:         qemuVMRawInt(r.CPUs),
		PID:            qemuVMRawInt(r.PID),
		RunningQemu:    r.RunningQemu,
		RunningMachine: r.RunningMachine,
	}
}

// GetQemuVMStatusCurrent reads the full GET .../status/current body.
func (c *Client) GetQemuVMStatusCurrent(ctx context.Context, node string, vmid int64) (*QemuVMStatus, error) {
	var raw qemuVMStatusRaw
	path := fmt.Sprintf("/nodes/%s/qemu/%d/status/current", node, vmid)
	if err := c.Do(ctx, http.MethodGet, path, nil, &raw); err != nil {
		return nil, fmt.Errorf("read qemu VM %d status on node %s: %w", vmid, node, err)
	}
	return raw.toStatus(), nil
}

// QemuVMStatusMinimal is the cheap subset of .../status/current used for
// polling power state.
type QemuVMStatusMinimal struct {
	Status    string
	QMPStatus string
	Lock      string
	Template  bool
}

// GetQemuVMStatusCurrentMinimal reads only the power-relevant fields of
// GET .../status/current.
func (c *Client) GetQemuVMStatusCurrentMinimal(ctx context.Context, node string, vmid int64) (*QemuVMStatusMinimal, error) {
	var raw qemuVMStatusRaw
	path := fmt.Sprintf("/nodes/%s/qemu/%d/status/current", node, vmid)
	if err := c.Do(ctx, http.MethodGet, path, nil, &raw); err != nil {
		return nil, fmt.Errorf("read qemu VM %d status on node %s: %w", vmid, node, err)
	}
	return &QemuVMStatusMinimal{
		Status:    raw.Status,
		QMPStatus: raw.QMPStatus,
		Lock:      raw.Lock,
		Template:  raw.toStatus().Template,
	}, nil
}

// UpdateQemuVMConfig PUTs .../config. The body merges the rendered input
// keys; deleteKeys (comma-joined into the pin's delete parameter) removes
// listed config keys. Unmodeled keys are never part of either.
func (c *Client) UpdateQemuVMConfig(ctx context.Context, node string, vmid int64, in QemuVMConfigInput, deleteKeys []string) error {
	body := in.qemuVMConfigBody()
	if len(deleteKeys) > 0 {
		body["delete"] = strings.Join(deleteKeys, ",")
	}
	path := fmt.Sprintf("/nodes/%s/qemu/%d/config", node, vmid)
	if err := c.Do(ctx, http.MethodPut, path, body, nil); err != nil {
		return fmt.Errorf("update qemu VM %d config on node %s: %w", vmid, node, err)
	}
	return nil
}

// QemuVMDeleteOptions carries the pin's DELETE .../{vmid} parameters.
type QemuVMDeleteOptions struct {
	DestroyUnreferencedDisks *bool
	Purge                    *bool
}

// DeleteQemuVM issues DELETE .../{vmid} and returns the destroy task UPID.
func (c *Client) DeleteQemuVM(ctx context.Context, node string, vmid int64, opts QemuVMDeleteOptions) (string, error) {
	q := url.Values{}
	if opts.DestroyUnreferencedDisks != nil {
		q.Set("destroy-unreferenced-disks", encodeBoolFlag(*opts.DestroyUnreferencedDisks))
	}
	if opts.Purge != nil {
		q.Set("purge", encodeBoolFlag(*opts.Purge))
	}
	var upid string
	path := fmt.Sprintf("/nodes/%s/qemu/%d?%s", node, vmid, q.Encode())
	if err := c.Do(ctx, http.MethodDelete, path, nil, &upid); err != nil {
		return "", fmt.Errorf("delete qemu VM %d on node %s: %w", vmid, node, err)
	}
	return upid, nil
}

// QemuVMStart POSTs .../status/start and returns the start task UPID. A
// nil timeout uses the PVE default (max(30, memory in GiB)).
func (c *Client) QemuVMStart(ctx context.Context, node string, vmid int64, timeout *int64) (string, error) {
	body := map[string]any{}
	if timeout != nil {
		body["timeout"] = *timeout
	}
	var upid string
	path := fmt.Sprintf("/nodes/%s/qemu/%d/status/start", node, vmid)
	if err := c.Do(ctx, http.MethodPost, path, body, &upid); err != nil {
		return "", fmt.Errorf("start qemu VM %d on node %s: %w", vmid, node, err)
	}
	return upid, nil
}

// QemuVMStop POSTs .../status/stop (immediate power-off) and returns the
// stop task UPID.
func (c *Client) QemuVMStop(ctx context.Context, node string, vmid int64, timeout *int64) (string, error) {
	body := map[string]any{}
	if timeout != nil {
		body["timeout"] = *timeout
	}
	var upid string
	path := fmt.Sprintf("/nodes/%s/qemu/%d/status/stop", node, vmid)
	if err := c.Do(ctx, http.MethodPost, path, body, &upid); err != nil {
		return "", fmt.Errorf("stop qemu VM %d on node %s: %w", vmid, node, err)
	}
	return upid, nil
}

// QemuVMShutdown POSTs .../status/shutdown (ACPI shutdown) and returns the
// shutdown task UPID. forceStop maps to the pin's forceStop parameter.
func (c *Client) QemuVMShutdown(ctx context.Context, node string, vmid int64, timeout *int64, forceStop *bool) (string, error) {
	body := map[string]any{}
	if timeout != nil {
		body["timeout"] = *timeout
	}
	if forceStop != nil {
		body["forceStop"] = *forceStop
	}
	var upid string
	path := fmt.Sprintf("/nodes/%s/qemu/%d/status/shutdown", node, vmid)
	if err := c.Do(ctx, http.MethodPost, path, body, &upid); err != nil {
		return "", fmt.Errorf("shutdown qemu VM %d on node %s: %w", vmid, node, err)
	}
	return upid, nil
}

// QemuVMMigrateOptions carries the pin's POST .../migrate parameters.
type QemuVMMigrateOptions struct {
	Target string
	// TargetStorage maps source storages ("storage-pair-list"; the special
	// value "1" maps every source storage to itself).
	TargetStorage      string
	MigrationNetwork   string
	MigrationType      string
	Bandwidth          *int64
	Online             *bool
	WithLocalDisks     *bool
	Force              *bool
	WithConntrackState *bool
}

// MigrateQemuVM POSTs .../migrate and returns the migration task UPID.
func (c *Client) MigrateQemuVM(ctx context.Context, node string, vmid int64, opts QemuVMMigrateOptions) (string, error) {
	body := map[string]any{"target": opts.Target}
	if opts.TargetStorage != "" {
		body["targetstorage"] = opts.TargetStorage
	}
	if opts.MigrationNetwork != "" {
		body["migration_network"] = opts.MigrationNetwork
	}
	if opts.MigrationType != "" {
		body["migration_type"] = opts.MigrationType
	}
	if opts.Bandwidth != nil {
		body["bwlimit"] = *opts.Bandwidth
	}
	if opts.Online != nil {
		body["online"] = *opts.Online
	}
	if opts.WithLocalDisks != nil {
		body["with-local-disks"] = *opts.WithLocalDisks
	}
	if opts.Force != nil {
		body["force"] = *opts.Force
	}
	if opts.WithConntrackState != nil {
		body["with-conntrack-state"] = *opts.WithConntrackState
	}
	var upid string
	path := fmt.Sprintf("/nodes/%s/qemu/%d/migrate", node, vmid)
	if err := c.Do(ctx, http.MethodPost, path, body, &upid); err != nil {
		return "", fmt.Errorf("migrate qemu VM %d from node %s to %s: %w", vmid, node, opts.Target, err)
	}
	return upid, nil
}

// ResizeQemuDisk PUTs .../resize with the new absolute size (the pin also
// accepts "+<size>" growth syntax, which callers do not need because the
// provider always sends absolutes). digest optionally guards concurrent
// modifications.
func (c *Client) ResizeQemuDisk(ctx context.Context, node string, vmid int64, disk, size, digest string) (string, error) {
	body := map[string]any{"disk": disk, "size": size}
	if digest != "" {
		body["digest"] = digest
	}
	var upid string
	path := fmt.Sprintf("/nodes/%s/qemu/%d/resize", node, vmid)
	if err := c.Do(ctx, http.MethodPut, path, body, &upid); err != nil {
		return "", fmt.Errorf("resize disk %s of qemu VM %d on node %s to %s: %w", disk, vmid, node, size, err)
	}
	return upid, nil
}

// QemuVMMoveDiskOptions carries the pin's POST .../move_disk parameters.
type QemuVMMoveDiskOptions struct {
	Disk           string
	Storage        string
	Format         string
	Digest         string
	DeleteOriginal *bool
	Bandwidth      *int64
}

// MoveQemuDisk POSTs .../move_disk and returns the move task UPID.
func (c *Client) MoveQemuDisk(ctx context.Context, node string, vmid int64, opts QemuVMMoveDiskOptions) (string, error) {
	body := map[string]any{"disk": opts.Disk, "storage": opts.Storage}
	if opts.Format != "" {
		body["format"] = opts.Format
	}
	if opts.Digest != "" {
		body["digest"] = opts.Digest
	}
	if opts.DeleteOriginal != nil {
		body["delete"] = *opts.DeleteOriginal
	}
	if opts.Bandwidth != nil {
		body["bwlimit"] = *opts.Bandwidth
	}
	var upid string
	path := fmt.Sprintf("/nodes/%s/qemu/%d/move_disk", node, vmid)
	if err := c.Do(ctx, http.MethodPost, path, body, &upid); err != nil {
		return "", fmt.Errorf("move disk %s of qemu VM %d on node %s to storage %s: %w", opts.Disk, vmid, node, opts.Storage, err)
	}
	return upid, nil
}

// QemuVMPendingChange is one row of GET .../pending. Pending is non-nil
// when a value is staged but not yet applied; Delete is non-nil (1, or 2
// for force-delete) when the key has a pending delete request.
type QemuVMPendingChange struct {
	Key     string
	Value   string
	Pending *string
	Delete  *int64
}

// qemuVMPendingRowRaw shadows QemuVMPendingChange for wire decoding.
type qemuVMPendingRowRaw struct {
	Key     string          `json:"key"`
	Value   json.RawMessage `json:"value"`
	Pending json.RawMessage `json:"pending"`
	Delete  *int64          `json:"delete"`
}

// GetQemuVMPending reads GET .../pending.
func (c *Client) GetQemuVMPending(ctx context.Context, node string, vmid int64) ([]QemuVMPendingChange, error) {
	var rows []qemuVMPendingRowRaw
	path := fmt.Sprintf("/nodes/%s/qemu/%d/pending", node, vmid)
	if err := c.Do(ctx, http.MethodGet, path, nil, &rows); err != nil {
		return nil, fmt.Errorf("read qemu VM %d pending changes on node %s: %w", vmid, node, err)
	}
	changes := make([]QemuVMPendingChange, 0, len(rows))
	for _, row := range rows {
		change := QemuVMPendingChange{
			Key:    row.Key,
			Value:  clusterRawScalar(row.Value),
			Delete: row.Delete,
		}
		if len(row.Pending) > 0 && string(row.Pending) != "null" {
			pending := clusterRawScalar(row.Pending)
			change.Pending = &pending
		}
		changes = append(changes, change)
	}
	return changes, nil
}
