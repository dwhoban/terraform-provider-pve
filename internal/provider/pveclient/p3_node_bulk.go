// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"context"
	"encoding/json"
	"fmt"
)

// ApplianceTemplate is one row of GET /nodes/{node}/aplinfo. The pin
// declares the row objects as free-form; these fields mirror the values
// PVE actually returns for appliance templates.
type ApplianceTemplate struct {
	Template       string `json:"template"`
	Source         string `json:"source,omitempty"`
	Infrastructure string `json:"infrastructure,omitempty"`
	Description    string `json:"description,omitempty"`
	Package        string `json:"package,omitempty"`
	Section        string `json:"section,omitempty"`
	Version        string `json:"version,omitempty"`
	OS             string `json:"os,omitempty"`
	Info           string `json:"info,omitempty"`
	ManageURL      string `json:"manageurl,omitempty"`
	SHA512Sum      string `json:"sha512sum,omitempty"`
	Architecture   string `json:"architecture,omitempty"`
}

// NodeSubscription mirrors GET /nodes/{node}/subscription. Optional pin
// fields are pointers so an unregistered node (status-only response)
// decodes with nils.
type NodeSubscription struct {
	Status      string  `json:"status"`
	Key         *string `json:"key,omitempty"`
	Level       *string `json:"level,omitempty"`
	Message     *string `json:"message,omitempty"`
	NextDueDate *string `json:"nextduedate,omitempty"`
	ProductName *string `json:"productname,omitempty"`
	RegDate     *string `json:"regdate,omitempty"`
	ServerID    *string `json:"serverid,omitempty"`
	Signature   *string `json:"signature,omitempty"`
	Sockets     *int64  `json:"sockets,omitempty"`
	CheckTime   *int64  `json:"checktime,omitempty"`
	URL         *string `json:"url,omitempty"`
}

// StartAllNodeGuestsOptions carries the optional POST /nodes/{node}/startall
// parameters. Vms is the pin's `pve-vmid-list` (comma-separated string).
type StartAllNodeGuestsOptions struct {
	Force      *bool
	MaxWorkers *int64
	Vms        string
}

// StopAllNodeGuestsOptions carries the optional POST /nodes/{node}/stopall
// parameters.
type StopAllNodeGuestsOptions struct {
	ForceStop  *bool
	MaxWorkers *int64
	Timeout    *int64
	Vms        string
}

// SuspendAllNodeGuestsOptions carries the optional POST
// /nodes/{node}/suspendall parameters.
type SuspendAllNodeGuestsOptions struct {
	MaxWorkers *int64
	Vms        string
}

// MigrateAllNodeGuestsOptions carries the POST /nodes/{node}/migrateall
// parameters; Target is required by the pin.
type MigrateAllNodeGuestsOptions struct {
	Target         string
	MaxWorkers     *int64
	Vms            string
	WithLocalDisks *bool
}

// BulkStartGuestsOptions carries the optional POST
// /cluster/bulk-action/guest/start parameters; Vms is the pin's array of
// unique VMIDs.
type BulkStartGuestsOptions struct {
	MaxWorkers *int64
	Timeout    *int64
	Vms        []int64
}

// BulkShutdownGuestsOptions carries the optional POST
// /cluster/bulk-action/guest/shutdown parameters.
type BulkShutdownGuestsOptions struct {
	ForceStop  *bool
	MaxWorkers *int64
	Timeout    *int64
	Vms        []int64
}

// BulkSuspendGuestsOptions carries the optional POST
// /cluster/bulk-action/guest/suspend parameters; StateStorage requires
// ToDisk per the pin.
type BulkSuspendGuestsOptions struct {
	MaxWorkers   *int64
	StateStorage string
	ToDisk       *bool
	Vms          []int64
}

// BulkMigrateGuestsOptions carries the POST
// /cluster/bulk-action/guest/migrate parameters; Target is required by
// the pin.
type BulkMigrateGuestsOptions struct {
	Target         string
	MaxWorkers     *int64
	Online         *bool
	Vms            []int64
	WithLocalDisks *bool
}

// GuestAgentIPAddress is one address of a guest interface as reported by
// the network-get-interfaces agent command.
type GuestAgentIPAddress struct {
	Address string `json:"ip-address"`
	Type    string `json:"ip-address-type"`
	Prefix  int64  `json:"prefix"`
}

// GuestAgentInterface is one interface reported by the
// network-get-interfaces agent command.
type GuestAgentInterface struct {
	Name            string                `json:"name"`
	HardwareAddress string                `json:"hardware-address"`
	IPAddresses     []GuestAgentIPAddress `json:"ip-addresses,omitempty"`
}

// GuestAgentFacts merges the pin's GET qemu agent commands. Nil fields
// mean the installed guest agent did not answer that command; Version and
// the error from info are the availability gate.
type GuestAgentFacts struct {
	Version         string
	HostName        string
	OSID            string
	OSIDLike        string
	OSMachine       string
	OSKernelName    string
	OSKernelRelease string
	OSKernelVersion string
	OSName          string
	OSPrettyName    string
	OSVariant       string
	OSVariantID     string
	OSVersion       string
	OSVersionID     string
	OSHomeURL       string
	Time            *int64
	TimeZone        string
	VCPUs           *int64
	Users           []string
	Interfaces      []GuestAgentInterface
}

// ListNodeAppliances issues GET /nodes/{node}/aplinfo and returns the
// appliance templates available on the node.
func (c *Client) ListNodeAppliances(ctx context.Context, node string) ([]ApplianceTemplate, error) {
	var appliances []ApplianceTemplate
	path := fmt.Sprintf("/nodes/%s/aplinfo", node)
	if err := c.Do(ctx, "GET", path, nil, &appliances); err != nil {
		return nil, fmt.Errorf("listing appliances on node %s: %w", node, err)
	}
	return appliances, nil
}

// GetNodeSubscription issues GET /nodes/{node}/subscription and returns
// the node's subscription info.
func (c *Client) GetNodeSubscription(ctx context.Context, node string) (*NodeSubscription, error) {
	var sub NodeSubscription
	path := fmt.Sprintf("/nodes/%s/subscription", node)
	if err := c.Do(ctx, "GET", path, nil, &sub); err != nil {
		return nil, fmt.Errorf("reading subscription of node %s: %w", node, err)
	}
	return &sub, nil
}

// NodeExecute issues POST /nodes/{node}/execute with the pin's
// `commands` parameter (a JSON-encoded array of command objects). The pin
// returns one result object per command; per the ADR Consequences mandate
// those bodies are discarded here and only the command count is returned,
// so no command output ever surfaces.
func (c *Client) NodeExecute(ctx context.Context, node, commands string) (int, error) {
	path := fmt.Sprintf("/nodes/%s/execute", node)
	var results []json.RawMessage
	if err := c.Do(ctx, "POST", path, map[string]string{"commands": commands}, &results); err != nil {
		return 0, fmt.Errorf("executing commands on node %s: %w", node, err)
	}
	return len(results), nil
}

// StartAllNodeGuests issues POST /nodes/{node}/startall and returns the
// worker UPID.
func (c *Client) StartAllNodeGuests(ctx context.Context, node string, opts StartAllNodeGuestsOptions) (string, error) {
	body := map[string]any{}
	if opts.Force != nil {
		body["force"] = *opts.Force
	}
	if opts.MaxWorkers != nil {
		body["max-workers"] = *opts.MaxWorkers
	}
	if opts.Vms != "" {
		body["vms"] = opts.Vms
	}
	path := fmt.Sprintf("/nodes/%s/startall", node)
	var upid string
	if err := c.Do(ctx, "POST", path, body, &upid); err != nil {
		return "", fmt.Errorf("starting all guests on node %s: %w", node, err)
	}
	return upid, nil
}

// StopAllNodeGuests issues POST /nodes/{node}/stopall and returns the
// worker UPID.
func (c *Client) StopAllNodeGuests(ctx context.Context, node string, opts StopAllNodeGuestsOptions) (string, error) {
	body := map[string]any{}
	if opts.ForceStop != nil {
		body["force-stop"] = *opts.ForceStop
	}
	if opts.MaxWorkers != nil {
		body["max-workers"] = *opts.MaxWorkers
	}
	if opts.Timeout != nil {
		body["timeout"] = *opts.Timeout
	}
	if opts.Vms != "" {
		body["vms"] = opts.Vms
	}
	path := fmt.Sprintf("/nodes/%s/stopall", node)
	var upid string
	if err := c.Do(ctx, "POST", path, body, &upid); err != nil {
		return "", fmt.Errorf("stopping all guests on node %s: %w", node, err)
	}
	return upid, nil
}

// SuspendAllNodeGuests issues POST /nodes/{node}/suspendall and returns
// the worker UPID.
func (c *Client) SuspendAllNodeGuests(ctx context.Context, node string, opts SuspendAllNodeGuestsOptions) (string, error) {
	body := map[string]any{}
	if opts.MaxWorkers != nil {
		body["max-workers"] = *opts.MaxWorkers
	}
	if opts.Vms != "" {
		body["vms"] = opts.Vms
	}
	path := fmt.Sprintf("/nodes/%s/suspendall", node)
	var upid string
	if err := c.Do(ctx, "POST", path, body, &upid); err != nil {
		return "", fmt.Errorf("suspending all guests on node %s: %w", node, err)
	}
	return upid, nil
}

// MigrateAllNodeGuests issues POST /nodes/{node}/migrateall and returns
// the worker UPID.
func (c *Client) MigrateAllNodeGuests(ctx context.Context, node string, opts MigrateAllNodeGuestsOptions) (string, error) {
	body := map[string]any{"target": opts.Target}
	if opts.MaxWorkers != nil {
		body["max-workers"] = *opts.MaxWorkers
	}
	if opts.Vms != "" {
		body["vms"] = opts.Vms
	}
	if opts.WithLocalDisks != nil {
		body["with-local-disks"] = *opts.WithLocalDisks
	}
	path := fmt.Sprintf("/nodes/%s/migrateall", node)
	var upid string
	if err := c.Do(ctx, "POST", path, body, &upid); err != nil {
		return "", fmt.Errorf("migrating all guests from node %s: %w", node, err)
	}
	return upid, nil
}

// BulkStartGuests issues POST /cluster/bulk-action/guest/start and returns
// the worker UPID.
func (c *Client) BulkStartGuests(ctx context.Context, opts BulkStartGuestsOptions) (string, error) {
	body := map[string]any{}
	if opts.MaxWorkers != nil {
		body["max-workers"] = *opts.MaxWorkers
	}
	if opts.Timeout != nil {
		body["timeout"] = *opts.Timeout
	}
	if len(opts.Vms) > 0 {
		body["vms"] = opts.Vms
	}
	var upid string
	if err := c.Do(ctx, "POST", "/cluster/bulk-action/guest/start", body, &upid); err != nil {
		return "", fmt.Errorf("bulk starting guests: %w", err)
	}
	return upid, nil
}

// BulkShutdownGuests issues POST /cluster/bulk-action/guest/shutdown and
// returns the worker UPID.
func (c *Client) BulkShutdownGuests(ctx context.Context, opts BulkShutdownGuestsOptions) (string, error) {
	body := map[string]any{}
	if opts.ForceStop != nil {
		body["force-stop"] = *opts.ForceStop
	}
	if opts.MaxWorkers != nil {
		body["max-workers"] = *opts.MaxWorkers
	}
	if opts.Timeout != nil {
		body["timeout"] = *opts.Timeout
	}
	if len(opts.Vms) > 0 {
		body["vms"] = opts.Vms
	}
	var upid string
	if err := c.Do(ctx, "POST", "/cluster/bulk-action/guest/shutdown", body, &upid); err != nil {
		return "", fmt.Errorf("bulk shutting down guests: %w", err)
	}
	return upid, nil
}

// BulkSuspendGuests issues POST /cluster/bulk-action/guest/suspend and
// returns the worker UPID.
func (c *Client) BulkSuspendGuests(ctx context.Context, opts BulkSuspendGuestsOptions) (string, error) {
	body := map[string]any{}
	if opts.MaxWorkers != nil {
		body["max-workers"] = *opts.MaxWorkers
	}
	if opts.StateStorage != "" {
		body["statestorage"] = opts.StateStorage
	}
	if opts.ToDisk != nil {
		body["to-disk"] = *opts.ToDisk
	}
	if len(opts.Vms) > 0 {
		body["vms"] = opts.Vms
	}
	var upid string
	if err := c.Do(ctx, "POST", "/cluster/bulk-action/guest/suspend", body, &upid); err != nil {
		return "", fmt.Errorf("bulk suspending guests: %w", err)
	}
	return upid, nil
}

// BulkMigrateGuests issues POST /cluster/bulk-action/guest/migrate and
// returns the worker UPID.
func (c *Client) BulkMigrateGuests(ctx context.Context, opts BulkMigrateGuestsOptions) (string, error) {
	body := map[string]any{"target": opts.Target}
	if opts.MaxWorkers != nil {
		body["max-workers"] = *opts.MaxWorkers
	}
	if opts.Online != nil {
		body["online"] = *opts.Online
	}
	if len(opts.Vms) > 0 {
		body["vms"] = opts.Vms
	}
	if opts.WithLocalDisks != nil {
		body["with-local-disks"] = *opts.WithLocalDisks
	}
	var upid string
	if err := c.Do(ctx, "POST", "/cluster/bulk-action/guest/migrate", body, &upid); err != nil {
		return "", fmt.Errorf("bulk migrating guests: %w", err)
	}
	return upid, nil
}

// GetGuestAgentFacts runs the pin's GET qemu agent commands against the
// guest and merges the results. The `info` command is the availability
// gate: when it fails (typically "QEMU guest agent is not running") the
// error is returned. Every other command degrades gracefully — a command
// unsupported by the installed guest agent leaves its facts nil.
func (c *Client) GetGuestAgentFacts(ctx context.Context, node string, vmid int64) (*GuestAgentFacts, error) {
	facts := &GuestAgentFacts{}
	// Availability gate: info must succeed or the guest agent is unusable.
	var info struct {
		Version string `json:"version"`
	}
	if err := c.guestAgentCommand(ctx, node, vmid, "info", &info); err != nil {
		return nil, fmt.Errorf("reading guest agent of vm %d on node %s: %w", vmid, node, err)
	}
	facts.Version = info.Version

	// Everything below is best-effort: guest agents vary in supported
	// commands, and a single unsupported command must not fail the read.
	var hostName struct {
		Name string `json:"name"`
	}
	if err := c.guestAgentCommand(ctx, node, vmid, "get-host-name", &hostName); err == nil {
		facts.HostName = hostName.Name
	}
	var osInfo struct {
		ID            string `json:"id"`
		IDLike        string `json:"id_like"`
		Machine       string `json:"machine"`
		KernelName    string `json:"kernel_name"`
		KernelRelease string `json:"kernel_release"`
		KernelVersion string `json:"kernel_version"`
		Name          string `json:"name"`
		PrettyName    string `json:"pretty_name"`
		Variant       string `json:"variant"`
		VariantID     string `json:"variant_id"`
		Version       string `json:"version"`
		VersionID     string `json:"version_id"`
		HomeURL       string `json:"home_url"`
	}
	if err := c.guestAgentCommand(ctx, node, vmid, "get-osinfo", &osInfo); err == nil {
		facts.OSID = osInfo.ID
		facts.OSIDLike = osInfo.IDLike
		facts.OSMachine = osInfo.Machine
		facts.OSKernelName = osInfo.KernelName
		facts.OSKernelRelease = osInfo.KernelRelease
		facts.OSKernelVersion = osInfo.KernelVersion
		facts.OSName = osInfo.Name
		facts.OSPrettyName = osInfo.PrettyName
		facts.OSVariant = osInfo.Variant
		facts.OSVariantID = osInfo.VariantID
		facts.OSVersion = osInfo.Version
		facts.OSVersionID = osInfo.VersionID
		facts.OSHomeURL = osInfo.HomeURL
	}
	// guest-get-time returns a bare integer on some agent versions and a
	// {timestamp} object on others; fetch once, then accept both shapes.
	var rawTime json.RawMessage
	if err := c.guestAgentCommand(ctx, node, vmid, "get-time", &rawTime); err == nil && len(rawTime) > 0 {
		var timeObj struct {
			Timestamp *int64 `json:"timestamp"`
		}
		if json.Unmarshal(rawTime, &timeObj) == nil && timeObj.Timestamp != nil {
			facts.Time = timeObj.Timestamp
		} else {
			var bare int64
			if json.Unmarshal(rawTime, &bare) == nil {
				facts.Time = &bare
			}
		}
	}
	var timeZone struct {
		Zone   string `json:"zone"`
		Offset *int64 `json:"offset"`
	}
	if err := c.guestAgentCommand(ctx, node, vmid, "get-timezone", &timeZone); err == nil {
		facts.TimeZone = timeZone.Zone
	}
	var vcpus []struct {
		Online *bool `json:"online"`
	}
	if err := c.guestAgentCommand(ctx, node, vmid, "get-vcpus", &vcpus); err == nil {
		count := int64(len(vcpus))
		facts.VCPUs = &count
	}
	var users []struct {
		Username string `json:"username"`
	}
	if err := c.guestAgentCommand(ctx, node, vmid, "get-users", &users); err == nil {
		names := make([]string, 0, len(users))
		for _, u := range users {
			names = append(names, u.Username)
		}
		facts.Users = names
	}
	var interfaces []GuestAgentInterface
	if err := c.guestAgentCommand(ctx, node, vmid, "network-get-interfaces", &interfaces); err == nil {
		facts.Interfaces = interfaces
	}
	return facts, nil
}

// guestAgentCommand issues GET /nodes/{node}/qemu/{vmid}/agent/{command}
// and decodes the pin's `{result: ...}` envelope into result (nil skips
// the decode).
func (c *Client) guestAgentCommand(ctx context.Context, node string, vmid int64, command string, result any) error {
	path := fmt.Sprintf("/nodes/%s/qemu/%d/agent/%s", node, vmid, command)
	var envelope struct {
		Result json.RawMessage `json:"result"`
	}
	if err := c.Do(ctx, "GET", path, nil, &envelope); err != nil {
		return fmt.Errorf("guest agent command %s: %w", command, err)
	}
	if result == nil || len(envelope.Result) == 0 {
		return nil
	}
	return json.Unmarshal(envelope.Result, result)
}
