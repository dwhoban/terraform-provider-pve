// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"context"
	"encoding/json"
	"net/url"
)

// QemuVM is one row of GET /nodes/{node}/qemu (the per-node virtual
// machine index). The pointer-free shape follows the other list clients:
// absent optional fields decode as zero values, which the provider layer
// renders as null. Template and Serial tolerate the 0/1 int encoding PVE
// emits on some versions. The status fields only appear in responses
// fetched with the full flag (memhost, pid, qmpstatus, running-machine,
// running-qemu, and the pressure gauges).
type QemuVM struct {
	CPU                float64 `json:"cpu,omitempty"`
	CPUs               float64 `json:"cpus,omitempty"`
	DiskRead           int64   `json:"diskread,omitempty"`
	DiskWrite          int64   `json:"diskwrite,omitempty"`
	Lock               string  `json:"lock,omitempty"`
	MaxDisk            int64   `json:"maxdisk,omitempty"`
	MaxMem             int64   `json:"maxmem,omitempty"`
	Mem                int64   `json:"mem,omitempty"`
	MemHost            int64   `json:"memhost,omitempty"`
	Name               string  `json:"name,omitempty"`
	NetIn              int64   `json:"netin,omitempty"`
	NetOut             int64   `json:"netout,omitempty"`
	PID                int64   `json:"pid,omitempty"`
	PressureCPUFull    float64 `json:"pressurecpufull,omitempty"`
	PressureCPUSome    float64 `json:"pressurecpusome,omitempty"`
	PressureIOFull     float64 `json:"pressureiofull,omitempty"`
	PressureIOSome     float64 `json:"pressureiosome,omitempty"`
	PressureMemoryFull float64 `json:"pressurememoryfull,omitempty"`
	PressureMemorySome float64 `json:"pressurememorysome,omitempty"`
	QMPStatus          string  `json:"qmpstatus,omitempty"`
	RunningMachine     string  `json:"running-machine,omitempty"`
	RunningQEMU        string  `json:"running-qemu,omitempty"`
	Serial             bool    `json:"serial,omitempty"`
	Status             string  `json:"status,omitempty"`
	Tags               string  `json:"tags,omitempty"`
	Template           bool    `json:"template,omitempty"`
	Uptime             int64   `json:"uptime,omitempty"`
	VMID               int64   `json:"vmid,omitempty"`
}

// qemuVMRaw shadows QemuVM for wire decoding, keeping the boolean fields
// raw so the lenient decodeBoolish can parse them.
type qemuVMRaw struct {
	CPU                float64         `json:"cpu,omitempty"`
	CPUs               float64         `json:"cpus,omitempty"`
	DiskRead           int64           `json:"diskread,omitempty"`
	DiskWrite          int64           `json:"diskwrite,omitempty"`
	Lock               string          `json:"lock,omitempty"`
	MaxDisk            int64           `json:"maxdisk,omitempty"`
	MaxMem             int64           `json:"maxmem,omitempty"`
	Mem                int64           `json:"mem,omitempty"`
	MemHost            int64           `json:"memhost,omitempty"`
	Name               string          `json:"name,omitempty"`
	NetIn              int64           `json:"netin,omitempty"`
	NetOut             int64           `json:"netout,omitempty"`
	PID                int64           `json:"pid,omitempty"`
	PressureCPUFull    float64         `json:"pressurecpufull,omitempty"`
	PressureCPUSome    float64         `json:"pressurecpusome,omitempty"`
	PressureIOFull     float64         `json:"pressureiofull,omitempty"`
	PressureIOSome     float64         `json:"pressureiosome,omitempty"`
	PressureMemoryFull float64         `json:"pressurememoryfull,omitempty"`
	PressureMemorySome float64         `json:"pressurememorysome,omitempty"`
	QMPStatus          string          `json:"qmpstatus,omitempty"`
	RunningMachine     string          `json:"running-machine,omitempty"`
	RunningQEMU        string          `json:"running-qemu,omitempty"`
	Serial             json.RawMessage `json:"serial,omitempty"`
	Status             string          `json:"status,omitempty"`
	Tags               string          `json:"tags,omitempty"`
	Template           json.RawMessage `json:"template,omitempty"`
	Uptime             int64           `json:"uptime,omitempty"`
	VMID               int64           `json:"vmid,omitempty"`
}

// UnmarshalJSON tolerates the 0/1 int encoding of template and serial
// that PVE emits on some versions.
func (v *QemuVM) UnmarshalJSON(data []byte) error {
	var raw qemuVMRaw
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	v.CPU = raw.CPU
	v.CPUs = raw.CPUs
	v.DiskRead = raw.DiskRead
	v.DiskWrite = raw.DiskWrite
	v.Lock = raw.Lock
	v.MaxDisk = raw.MaxDisk
	v.MaxMem = raw.MaxMem
	v.Mem = raw.Mem
	v.MemHost = raw.MemHost
	v.Name = raw.Name
	v.NetIn = raw.NetIn
	v.NetOut = raw.NetOut
	v.PID = raw.PID
	v.PressureCPUFull = raw.PressureCPUFull
	v.PressureCPUSome = raw.PressureCPUSome
	v.PressureIOFull = raw.PressureIOFull
	v.PressureIOSome = raw.PressureIOSome
	v.PressureMemoryFull = raw.PressureMemoryFull
	v.PressureMemorySome = raw.PressureMemorySome
	v.QMPStatus = raw.QMPStatus
	v.RunningMachine = raw.RunningMachine
	v.RunningQEMU = raw.RunningQEMU
	v.Serial = decodeBoolish(raw.Serial)
	v.Status = raw.Status
	v.Tags = raw.Tags
	v.Template = decodeBoolish(raw.Template)
	v.Uptime = raw.Uptime
	v.VMID = raw.VMID
	return nil
}

// ListQemuVMs enumerates GET /nodes/{node}/qemu (the per-node virtual
// machine index). full corresponds to the pin's optional "full" query
// parameter, which adds the status field set for running VMs (memhost,
// pid, qmpstatus, running-machine, running-qemu, and the pressure
// gauges).
func (c *Client) ListQemuVMs(ctx context.Context, node string, full bool) ([]QemuVM, error) {
	path := "/nodes/" + url.PathEscape(node) + "/qemu"
	if full {
		path += "?full=1"
	}
	var out []QemuVM
	if err := c.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}
