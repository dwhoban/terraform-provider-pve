// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
)

// NodeTaskListEntry is one row of GET /nodes/{node}/tasks (finished task
// list). It is distinct from TaskInfo, which is the body of the per-task
// status endpoint.
type NodeTaskListEntry struct {
	UPID      string  `json:"upid"`
	ID        *string `json:"id,omitempty"`
	Node      string  `json:"node"`
	Type      string  `json:"type"`
	User      string  `json:"user"`
	Status    *string `json:"status,omitempty"`
	Starttime int64   `json:"starttime"`
	Endtime   *int64  `json:"endtime,omitempty"`
	PID       *int64  `json:"pid,omitempty"`
	PStart    *int64  `json:"pstart,omitempty"`
}

// ListNodeTasksOptions filters GET /nodes/{node}/tasks. Zero values omit the
// corresponding query parameter.
type ListNodeTasksOptions struct {
	Errors       *bool
	Limit        *int64
	Since        *int64
	Until        *int64
	Start        *int64
	VMID         *int64
	Source       string // archive, active, all
	StatusFilter string
	TypeFilter   string
	UserFilter   string
}

// ListNodeTasks reads the task list for one node.
func (c *Client) ListNodeTasks(ctx context.Context, node string, opts ListNodeTasksOptions) ([]NodeTaskListEntry, error) {
	q := url.Values{}
	if opts.Errors != nil && *opts.Errors {
		q.Set("errors", "1")
	}
	if opts.Limit != nil {
		q.Set("limit", strconv.FormatInt(*opts.Limit, 10))
	}
	if opts.Since != nil {
		q.Set("since", strconv.FormatInt(*opts.Since, 10))
	}
	if opts.Until != nil {
		q.Set("until", strconv.FormatInt(*opts.Until, 10))
	}
	if opts.Start != nil {
		q.Set("start", strconv.FormatInt(*opts.Start, 10))
	}
	if opts.VMID != nil {
		q.Set("vmid", strconv.FormatInt(*opts.VMID, 10))
	}
	if opts.Source != "" {
		q.Set("source", opts.Source)
	}
	if opts.StatusFilter != "" {
		q.Set("statusfilter", opts.StatusFilter)
	}
	if opts.TypeFilter != "" {
		q.Set("typefilter", opts.TypeFilter)
	}
	if opts.UserFilter != "" {
		q.Set("userfilter", opts.UserFilter)
	}
	path := fmt.Sprintf("/nodes/%s/tasks", node)
	if encoded := q.Encode(); encoded != "" {
		path += "?" + encoded
	}
	var out []NodeTaskListEntry
	if err := c.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// NodePciDevice is one row of GET /nodes/{node}/hardware/pci.
type NodePciDevice struct {
	ID                 string  `json:"id"`
	ClassName          string  `json:"class"`
	DeviceID           string  `json:"device"`
	DeviceName         *string `json:"device_name,omitempty"`
	VendorID           string  `json:"vendor"`
	VendorName         *string `json:"vendor_name,omitempty"`
	SubsystemDeviceID  *string `json:"subsystem_device,omitempty"`
	SubsystemDeviceNam *string `json:"subsystem_device_name,omitempty"`
	SubsystemVendorID  *string `json:"subsystem_vendor,omitempty"`
	SubsystemVendorNam *string `json:"subsystem_vendor_name,omitempty"`
	IOMMUGroup         int64   `json:"iommugroup"`
	MDev               *bool   `json:"mdev,omitempty"`
}

// ListNodePciDevicesOptions tunes GET /nodes/{node}/hardware/pci.
type ListNodePciDevicesOptions struct {
	// PCIClassBlacklist overrides the upstream default ("05;06;0b").
	PCIClassBlacklist string
	// Verbose forces verbose output; nil keeps the upstream default (on).
	Verbose *bool
}

// ListNodePciDevices lists the local PCI devices of one node.
func (c *Client) ListNodePciDevices(ctx context.Context, node string, opts ListNodePciDevicesOptions) ([]NodePciDevice, error) {
	q := url.Values{}
	if opts.PCIClassBlacklist != "" {
		q.Set("pci-class-blacklist", opts.PCIClassBlacklist)
	}
	if opts.Verbose != nil {
		q.Set("verbose", boolToInt(*opts.Verbose))
	}
	path := fmt.Sprintf("/nodes/%s/hardware/pci", node)
	if encoded := q.Encode(); encoded != "" {
		path += "?" + encoded
	}
	var out []NodePciDevice
	if err := c.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// NodeUsbDevice is one row of GET /nodes/{node}/hardware/usb.
type NodeUsbDevice struct {
	BusNum       int64   `json:"busnum"`
	Class        int64   `json:"class"`
	DevNum       int64   `json:"devnum"`
	Level        int64   `json:"level"`
	Port         int64   `json:"port"`
	ProductID    string  `json:"prodid"`
	VendorID     string  `json:"vendid"`
	Manufacturer *string `json:"manufacturer,omitempty"`
	Product      *string `json:"product,omitempty"`
	Serial       *string `json:"serial,omitempty"`
	Speed        string  `json:"speed"`
	USBPath      *string `json:"usbpath,omitempty"`
}

// ListNodeUsbDevices lists the local USB devices of one node.
func (c *Client) ListNodeUsbDevices(ctx context.Context, node string) ([]NodeUsbDevice, error) {
	path := fmt.Sprintf("/nodes/%s/hardware/usb", node)
	var out []NodeUsbDevice
	if err := c.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// NodeCapabilitiesQemuCPU is one row of GET /nodes/{node}/capabilities/qemu/cpu.
type NodeCapabilitiesQemuCPU struct {
	Name     string `json:"name"`
	Vendor   string `json:"vendor"`
	Custom   bool   `json:"custom"`
	Abstract *bool  `json:"abstract,omitempty"`
}

// NodeCapabilitiesQemuMachine is one row of
// GET /nodes/{node}/capabilities/qemu/machines.
type NodeCapabilitiesQemuMachine struct {
	ID      string  `json:"id"`
	Version string  `json:"version"`
	Type    *string `json:"type,omitempty"`
	Changes *string `json:"changes,omitempty"`
}

// NodeCapabilitiesQemuMigration mirrors
// GET /nodes/{node}/capabilities/qemu/migration.
type NodeCapabilitiesQemuMigration struct {
	HasDbusVMState bool `json:"has-dbus-vmstate"`
}

// NodeCapabilities merges the three QEMU capability endpoints of a node:
// CPU models, machine types, and migration capabilities. The cluster-wide
// cpu-flags endpoint is deliberately not part of this type.
type NodeCapabilities struct {
	CPUModels         []NodeCapabilitiesQemuCPU     `json:"cpu_models"`
	Machines          []NodeCapabilitiesQemuMachine `json:"machines"`
	MigrationFeatures NodeCapabilitiesQemuMigration `json:"migration_features"`
}

// GetNodeCapabilities fetches qemu/cpu, qemu/machines and qemu/migration
// for one node and merges them into a single NodeCapabilities value.
func (c *Client) GetNodeCapabilities(ctx context.Context, node string) (*NodeCapabilities, error) {
	out := &NodeCapabilities{}
	cpuPath := fmt.Sprintf("/nodes/%s/capabilities/qemu/cpu", node)
	if err := c.Do(ctx, "GET", cpuPath, nil, &out.CPUModels); err != nil {
		return nil, err
	}
	machinesPath := fmt.Sprintf("/nodes/%s/capabilities/qemu/machines", node)
	if err := c.Do(ctx, "GET", machinesPath, nil, &out.Machines); err != nil {
		return nil, err
	}
	migrationPath := fmt.Sprintf("/nodes/%s/capabilities/qemu/migration", node)
	if err := c.Do(ctx, "GET", migrationPath, nil, &out.MigrationFeatures); err != nil {
		return nil, err
	}
	return out, nil
}

// NodeWakeonLAN sends a wake on LAN magic packet to the node's configured
// WoL interface and returns the MAC address PVE used to assemble the packet.
func (c *Client) NodeWakeonLAN(ctx context.Context, node string) (string, error) {
	path := fmt.Sprintf("/nodes/%s/wakeonlan", node)
	var mac string
	if err := c.Do(ctx, "POST", path, nil, &mac); err != nil {
		return "", err
	}
	return mac, nil
}

// boolToInt renders a boolean as PVE's 0/1 query-parameter encoding.
func boolToInt(v bool) string {
	if v {
		return "1"
	}
	return "0"
}
