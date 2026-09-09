// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"context"
	"encoding/json"
	"fmt"
)

// LxcContainer is one entry of GET /nodes/{node}/lxc (container index for
// one node). Pointers mark the optional fields so callers can distinguish
// "unset" from zero; the pin declares every field except vmid and status
// optional. Template arrives as a boolish value (true/false or 0/1
// depending on version).
type LxcContainer struct {
	VMID               int64    `json:"vmid"`
	Status             string   `json:"status"`
	Name               *string  `json:"name,omitempty"`
	Template           *bool    `json:"-"`
	CPU                *float64 `json:"cpu,omitempty"`
	CPUs               *float64 `json:"cpus,omitempty"`
	Mem                *int64   `json:"mem,omitempty"`
	MaxMem             *int64   `json:"maxmem,omitempty"`
	MaxSwap            *int64   `json:"maxswap,omitempty"`
	Disk               *int64   `json:"disk,omitempty"`
	MaxDisk            *int64   `json:"maxdisk,omitempty"`
	DiskRead           *int64   `json:"diskread,omitempty"`
	DiskWrite          *int64   `json:"diskwrite,omitempty"`
	NetIn              *int64   `json:"netin,omitempty"`
	NetOut             *int64   `json:"netout,omitempty"`
	Uptime             *int64   `json:"uptime,omitempty"`
	Lock               *string  `json:"lock,omitempty"`
	Tags               *string  `json:"tags,omitempty"`
	PressureCPUSome    *float64 `json:"pressurecpusome,omitempty"`
	PressureIOSome     *float64 `json:"pressureiosome,omitempty"`
	PressureIOFull     *float64 `json:"pressureiofull,omitempty"`
	PressureMemorySome *float64 `json:"pressurememorysome,omitempty"`
	PressureMemoryFull *float64 `json:"pressurememoryfull,omitempty"`
}

// lxcContainerRaw is the flat wire shape with the boolish template flag in
// raw form; it cannot embed LxcContainer because that would promote the
// custom UnmarshalJSON into this struct and recurse.
type lxcContainerRaw struct {
	VMID               int64           `json:"vmid"`
	Status             string          `json:"status"`
	Name               *string         `json:"name,omitempty"`
	Template           json.RawMessage `json:"template"`
	CPU                *float64        `json:"cpu,omitempty"`
	CPUs               *float64        `json:"cpus,omitempty"`
	Mem                *int64          `json:"mem,omitempty"`
	MaxMem             *int64          `json:"maxmem,omitempty"`
	MaxSwap            *int64          `json:"maxswap,omitempty"`
	Disk               *int64          `json:"disk,omitempty"`
	MaxDisk            *int64          `json:"maxdisk,omitempty"`
	DiskRead           *int64          `json:"diskread,omitempty"`
	DiskWrite          *int64          `json:"diskwrite,omitempty"`
	NetIn              *int64          `json:"netin,omitempty"`
	NetOut             *int64          `json:"netout,omitempty"`
	Uptime             *int64          `json:"uptime,omitempty"`
	Lock               *string         `json:"lock,omitempty"`
	Tags               *string         `json:"tags,omitempty"`
	PressureCPUSome    *float64        `json:"pressurecpusome,omitempty"`
	PressureIOSome     *float64        `json:"pressureiosome,omitempty"`
	PressureIOFull     *float64        `json:"pressureiofull,omitempty"`
	PressureMemorySome *float64        `json:"pressurememorysome,omitempty"`
	PressureMemoryFull *float64        `json:"pressurememoryfull,omitempty"`
}

// UnmarshalJSON decodes a container index row, leniently decoding the
// boolish template flag.
func (l *LxcContainer) UnmarshalJSON(data []byte) error {
	var raw lxcContainerRaw
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*l = LxcContainer{
		VMID:               raw.VMID,
		Status:             raw.Status,
		Name:               raw.Name,
		Template:           decodeBoolishPtr(raw.Template),
		CPU:                raw.CPU,
		CPUs:               raw.CPUs,
		Mem:                raw.Mem,
		MaxMem:             raw.MaxMem,
		MaxSwap:            raw.MaxSwap,
		Disk:               raw.Disk,
		MaxDisk:            raw.MaxDisk,
		DiskRead:           raw.DiskRead,
		DiskWrite:          raw.DiskWrite,
		NetIn:              raw.NetIn,
		NetOut:             raw.NetOut,
		Uptime:             raw.Uptime,
		Lock:               raw.Lock,
		Tags:               raw.Tags,
		PressureCPUSome:    raw.PressureCPUSome,
		PressureIOSome:     raw.PressureIOSome,
		PressureIOFull:     raw.PressureIOFull,
		PressureMemorySome: raw.PressureMemorySome,
		PressureMemoryFull: raw.PressureMemoryFull,
	}
	return nil
}

// ListLxcContainers enumerates GET /nodes/{node}/lxc. The pin defines no
// query parameters for this endpoint (unlike the qemu index, it has no
// `full` flag), so the request sends the bare path.
func (c *Client) ListLxcContainers(ctx context.Context, node string) ([]LxcContainer, error) {
	path := fmt.Sprintf("/nodes/%s/lxc", node)
	var out []LxcContainer
	if err := c.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, fmt.Errorf("pveclient: list lxc containers %s: %w", path, err)
	}
	return out, nil
}
