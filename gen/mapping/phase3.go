// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package mapping

// Phase3 — QEMU virtual machines: VM resource (create/clone, config
// lifecycle, cloud-init), read-only VM data source (config, status,
// guest-agent facts), snapshot resource + rollback, lifecycle and disk
// actions, cluster guest bulk actions, and custom CPU models.
var Phase3 = []Type{
	{
		Type: "pve_vm", Kind: "resource", Phase: 3,
		Summary: "QEMU virtual machine on a node: full config lifecycle, cloud-init parameters, clone-at-create.",
		Create:  &Endpoint{Path: "/nodes/{node}/qemu", IDFromUPID: "vmid"},
		Read:    &Endpoint{Path: "/nodes/{node}/qemu/{vmid}/config"},
		Update:  &Endpoint{Path: "/nodes/{node}/qemu/{vmid}/config"},
		Delete:  &Endpoint{Path: "/nodes/{node}/qemu/{vmid}"},
		Clone: &Clone{
			Path:        "/nodes/{node}/qemu/{vmid}/clone",
			SourceAttr:  "clone_source_vmid",
			SourceParam: "vmid",
			NewIDParam:  "newid",
			// Full clones work from any source VM; PVE's default linked
			// clone requires a template.
			ConstParams: map[string]string{"full": "1"},
		},
		Attrs: []AttrRule{
			// apidoc defaults PVE itself rejects at create time.
			{Name: "vcpus", NoDefault: true},
			{Name: "vmgenid", NoDefault: true},
			// Per-write conflict token.
			{Name: "digest", Skip: true},
			// apidoc models indexed slots as pattern params (ide[n], net[n]);
			// expose the common slots explicitly.
			{Name: "ide0", APIName: "ide0", Type: "string", Add: true, Optional: BoolPtr(true), Description: "IDE slot 0 volume/options."},
			{Name: "ide1", APIName: "ide1", Type: "string", Add: true, Optional: BoolPtr(true), Description: "IDE slot 1 volume/options."},
			{Name: "ide2", APIName: "ide2", Type: "string", Add: true, Optional: BoolPtr(true), Description: "IDE slot 2 volume/options."},
			{Name: "ide3", APIName: "ide3", Type: "string", Add: true, Optional: BoolPtr(true), Description: "IDE slot 3 volume/options."},
			{Name: "net0", APIName: "net0", Type: "string", Add: true, Optional: BoolPtr(true), Description: "NET slot 0 volume/options."},
			{Name: "net1", APIName: "net1", Type: "string", Add: true, Optional: BoolPtr(true), Description: "NET slot 1 volume/options."},
			{Name: "net2", APIName: "net2", Type: "string", Add: true, Optional: BoolPtr(true), Description: "NET slot 2 volume/options."},
			{Name: "net3", APIName: "net3", Type: "string", Add: true, Optional: BoolPtr(true), Description: "NET slot 3 volume/options."},
			// Server-allocated VMID (create returns a UPID when vmid is
			// omitted; the engine extracts the id component).
			{Name: "vmid", Required: BoolPtr(true), ForceNew: true, Description: "Guest id; use provider::pve::nextid() to allocate a free one."},
			{Name: "clone_source_vmid", Type: "int64", Add: true, Optional: BoolPtr(true), ForceNew: true, Description: "Source guest id to clone from at creation time; when set, the VM is created via the clone endpoint instead of a fresh install."},
		},
	},
	{
		Type: "pve_vm", Kind: "data_source", Phase: 3,
		Summary: "One VM: configuration, pending changes, feature support, runtime status, cloud-init state, and guest-agent facts.",
		Reads: []*Endpoint{
			{Path: "/nodes/{node}/qemu/{vmid}/config", ResultName: "config"},
			{Path: "/nodes/{node}/qemu/{vmid}/pending", ResultName: "pending"},
			{Path: "/nodes/{node}/qemu/{vmid}/feature", ResultName: "feature"},
			{Path: "/nodes/{node}/qemu/{vmid}/status", ResultName: "status"},
			{Path: "/nodes/{node}/qemu/{vmid}/status/current", ResultName: "current"},
			{Path: "/nodes/{node}/qemu/{vmid}/cloudinit", ResultName: "cloudinit"},
			{Path: "/nodes/{node}/qemu/{vmid}/cloudinit/dump", ResultName: "cloudinit_pending"},
			// Guest-agent read facts (agent POST is the ping check).
			{Path: "/nodes/{node}/qemu/{vmid}/agent", ResultName: "agent"},
			{Path: "/nodes/{node}/qemu/{vmid}/agent/info", ResultName: "agent_info"},
			{Path: "/nodes/{node}/qemu/{vmid}/agent/ping", Method: "POST", ResultName: "agent_ping"},
			{Path: "/nodes/{node}/qemu/{vmid}/agent/get-osinfo", ResultName: "agent_osinfo"},
			{Path: "/nodes/{node}/qemu/{vmid}/agent/get-host-name", ResultName: "agent_host_name"},
			{Path: "/nodes/{node}/qemu/{vmid}/agent/network-get-interfaces", ResultName: "agent_interfaces"},
			{Path: "/nodes/{node}/qemu/{vmid}/agent/get-users", ResultName: "agent_users"},
			{Path: "/nodes/{node}/qemu/{vmid}/agent/get-time", ResultName: "agent_time"},
			{Path: "/nodes/{node}/qemu/{vmid}/agent/get-timezone", ResultName: "agent_timezone"},
			{Path: "/nodes/{node}/qemu/{vmid}/agent/get-vcpus", ResultName: "agent_vcpus"},
			{Path: "/nodes/{node}/qemu/{vmid}/agent/get-fsinfo", ResultName: "agent_fsinfo"},
			{Path: "/nodes/{node}/qemu/{vmid}/agent/get-memory-blocks", ResultName: "agent_memory_blocks"},
			{Path: "/nodes/{node}/qemu/{vmid}/agent/get-memory-block-info", ResultName: "agent_memory_block_info"},
		},
	},
	{
		Type: "pve_vm_snapshot", Kind: "resource", Phase: 3,
		IDAttrs: []string{"node", "vmid", "snapname"},
		Summary: "VM snapshot (name, description, RAM state, vmstate storage).",
		Create:  &Endpoint{Path: "/nodes/{node}/qemu/{vmid}/snapshot"},
		// The item GET returns the whole snapshot list; select this
		// snapshot from it.
		Read: &Endpoint{
			Path:   "/nodes/{node}/qemu/{vmid}/snapshot",
			Method: "GET",
			Filter: [][2]string{{"name", "snapname"}},
		},
		Update: &Endpoint{Path: "/nodes/{node}/qemu/{vmid}/snapshot/{snapname}/config"},
		Delete: &Endpoint{Path: "/nodes/{node}/qemu/{vmid}/snapshot/{snapname}"},
	},

	// VM one-shot operations.
	{
		Type: "pve_vm_snapshot_rollback", Kind: "action", Phase: 3,
		Summary: "Roll a VM back to a snapshot.",
		Invoke:  &Endpoint{Path: "/nodes/{node}/qemu/{vmid}/snapshot/{snapname}/rollback"},
	},
	{
		Type: "pve_vm_start", Kind: "action", Phase: 3,
		Summary: "Start a VM.",
		Invoke:  &Endpoint{Path: "/nodes/{node}/qemu/{vmid}/status/start"},
	},
	{
		Type: "pve_vm_stop", Kind: "action", Phase: 3,
		Summary: "Stop a VM (hard power-off).",
		Invoke:  &Endpoint{Path: "/nodes/{node}/qemu/{vmid}/status/stop"},
	},
	{
		Type: "pve_vm_reboot", Kind: "action", Phase: 3,
		Summary: "Reboot a VM by resetting it.",
		Invoke:  &Endpoint{Path: "/nodes/{node}/qemu/{vmid}/status/reboot"},
	},
	{
		Type: "pve_vm_reset", Kind: "action", Phase: 3,
		Summary: "Reset a VM (hard reset).",
		Invoke:  &Endpoint{Path: "/nodes/{node}/qemu/{vmid}/status/reset"},
	},
	{
		Type: "pve_vm_shutdown", Kind: "action", Phase: 3,
		Summary: "Shut a VM down gracefully.",
		Invoke:  &Endpoint{Path: "/nodes/{node}/qemu/{vmid}/status/shutdown"},
	},
	{
		Type: "pve_vm_suspend", Kind: "action", Phase: 3,
		Summary: "Suspend a VM.",
		Invoke:  &Endpoint{Path: "/nodes/{node}/qemu/{vmid}/status/suspend"},
	},
	{
		Type: "pve_vm_resume", Kind: "action", Phase: 3,
		Summary: "Resume a suspended VM.",
		Invoke:  &Endpoint{Path: "/nodes/{node}/qemu/{vmid}/status/resume"},
	},
	{
		Type: "pve_vm_migrate", Kind: "action", Phase: 3,
		Summary: "Migrate a VM to another node (with migration preview via the GET endpoint).",
		Invoke:  &Endpoint{Path: "/nodes/{node}/qemu/{vmid}/migrate"},
	},
	{
		Type: "pve_vm_remote_migrate", Kind: "action", Phase: 3,
		Summary: "Migrate a VM to a remote cluster.",
		Invoke:  &Endpoint{Path: "/nodes/{node}/qemu/{vmid}/remote_migrate"},
	},
	{
		Type: "pve_vm_template", Kind: "action", Phase: 3,
		Summary: "Convert a VM to a template.",
		Invoke:  &Endpoint{Path: "/nodes/{node}/qemu/{vmid}/template"},
	},
	{
		Type: "pve_vm_disk_resize", Kind: "action", Phase: 3,
		Summary: "Resize a VM volume.",
		Invoke:  &Endpoint{Path: "/nodes/{node}/qemu/{vmid}/resize", Method: "PUT"},
	},
	{
		Type: "pve_vm_disk_move", Kind: "action", Phase: 3,
		Summary: "Move a VM disk to another storage.",
		Invoke:  &Endpoint{Path: "/nodes/{node}/qemu/{vmid}/move_disk"},
	},
	{
		Type: "pve_vm_disk_unlink", Kind: "action", Phase: 3,
		Summary: "Unlink a VM disk from its base volume.",
		Invoke:  &Endpoint{Path: "/nodes/{node}/qemu/{vmid}/unlink", Method: "PUT"},
	},
	{
		Type: "pve_vm_fstrim", Kind: "action", Phase: 3,
		Summary: "Run fstrim inside the VM via the guest agent.",
		Invoke:  &Endpoint{Path: "/nodes/{node}/qemu/{vmid}/agent/fstrim"},
	},

	// Cluster-wide guest bulk actions.
	{
		Type: "pve_guest_bulk_start", Kind: "action", Phase: 3,
		Summary: "Bulk-start guests (VMs and containers) on the cluster.",
		Invoke:  &Endpoint{Path: "/cluster/bulk-action/guest/start"},
	},
	{
		Type: "pve_guest_bulk_shutdown", Kind: "action", Phase: 3,
		Summary: "Bulk-shutdown guests on the cluster.",
		Invoke:  &Endpoint{Path: "/cluster/bulk-action/guest/shutdown"},
	},
	{
		Type: "pve_guest_bulk_migrate", Kind: "action", Phase: 3,
		Summary: "Bulk-migrate guests to other nodes on the cluster.",
		Invoke:  &Endpoint{Path: "/cluster/bulk-action/guest/migrate"},
	},
	{
		Type: "pve_guest_bulk_suspend", Kind: "action", Phase: 3,
		Summary: "Bulk-suspend guests on the cluster.",
		Invoke:  &Endpoint{Path: "/cluster/bulk-action/guest/suspend"},
	},
	{
		Type: "pve_guest_bulk_actions", Kind: "data_source", Phase: 3,
		Summary: "Bulk-action framework and guest bulk-action API descriptions.",
		Reads: []*Endpoint{
			{Path: "/cluster/bulk-action", ResultName: "actions"},
			{Path: "/cluster/bulk-action/guest", ResultName: "guest_actions"},
		},
	},

	// Custom CPU models (cluster-wide QEMU custom model registry).
	{
		Type: "pve_custom_cpu_model", Kind: "resource", Phase: 3,
		Summary: "Custom QEMU CPU model definition (cluster-wide).",
		IDAttrs: []string{"cputype"},
		Create:  &Endpoint{Path: "/cluster/qemu/custom-cpu-models"},
		Read:    &Endpoint{Path: "/cluster/qemu/custom-cpu-models/{cputype}"},
		Update:  &Endpoint{Path: "/cluster/qemu/custom-cpu-models/{cputype}"},
		Delete:  &Endpoint{Path: "/cluster/qemu/custom-cpu-models/{cputype}"},
	},
	{
		Type: "pve_cpu_flags", Kind: "data_source", Phase: 3,
		Summary: "QEMU cluster info and available CPU flags for custom CPU models.",
		Reads: []*Endpoint{
			{Path: "/cluster/qemu", ResultName: "qemu"},
			{Path: "/cluster/qemu/cpu-flags", ResultName: "flags"},
		},
	},
}
