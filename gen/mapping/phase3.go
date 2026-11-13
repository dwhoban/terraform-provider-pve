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
			// The ide2 alias and boot are covered by the cdrom block and
			// boot_order.
			{Name: "cdrom", Skip: true},
			{Name: "boot", Skip: true},
			// Slot families not yet modeled as blocks.
			{Name: "hostpci_n_", Skip: true},
			{Name: "numa_n_", Skip: true},
			{Name: "parallel_n_", Skip: true},
			{Name: "serial_n_", Skip: true},
			{Name: "unused_n_", Skip: true},
			{Name: "usb_n_", Skip: true},
			{Name: "virtiofs_n_", Skip: true},
			{Name: "boot_order", APIName: "boot", Type: "list", Elem: "string", Add: true, Optional: BoolPtr(true), Format: "boot-order", Description: "Boot devices in order, as PVE device names (e.g. scsi0, ide2, net0)."},
			// Server-allocated VMID (create returns a UPID when vmid is
			// omitted; the engine extracts the id component).
			{Name: "vmid", Required: BoolPtr(true), ForceNew: true, Description: "Guest id; use provider::pve::nextid() to allocate a free one."},
			{Name: "clone_source_vmid", Type: "int64", Add: true, Optional: BoolPtr(true), ForceNew: true, Description: "Source guest id to clone from at creation time; when set, the VM is created via the clone endpoint instead of a fresh install."},
		},
		Blocks: []Block{
			{
				Name:        "disk",
				Description: "Virtual disk on one IDE/SATA/SCSI/VirtIO slot. New volumes are allocated from storage; use action `pve_vm_disk_resize` to resize and `pve_vm_disk_move` to move an existing disk.",
				Families:    []string{"ide", "sata", "scsi", "virtio"},
				DefaultKey:  "file",
				Exclude:     map[string]string{"media": "cdrom", "file": "*cloudinit*"},
				Fields: []BlockField{
					{Name: "interface", Key: "interface", Role: "index", Required: true, Description: "PVE device name: ide0..3, sata0..5, scsi0..30, virtio0..15."},
					{Name: "storage", Key: "file", Role: "volume-storage", CreateOnly: true, Description: "Storage to allocate a new volume from (with size_gb)."},
					{Name: "size_gb", Key: "size", Role: "volume-size-gb", Type: "int64", Computed: true, CreateOnly: true, Description: "Size in GiB. Resizing an existing disk is not supported here; use action `pve_vm_disk_resize`."},
					{Name: "volume_id", Key: "file", Role: "volume-id", Computed: true, CreateOnly: true, Description: "Allocated volume (e.g. local-lvm:vm-120-disk-0)."},
					{Name: "iothread", Key: "iothread", Type: "bool", Description: "Use iothread for the disk (scsi/virtio only)."},
					{Name: "discard", Key: "discard", Type: "bool", True: "on", False: "ignore", Description: "Enable discard/TRIM support."},
					{Name: "ssd", Key: "ssd", Type: "bool", Description: "Report the disk as SSD (ide/sata/scsi only)."},
					{Name: "backup", Key: "backup", Type: "bool", Description: "Include the disk in backups."},
					{Name: "cache", Key: "cache", Enum: []string{"none", "writethrough", "writeback", "unsafe", "directsync"}, Description: "Cache mode."},
				},
			},
			{
				Name:        "cdrom",
				Description: "CD-ROM drive backed by an ISO (or ejected).",
				Families:    []string{"ide", "sata", "scsi"},
				DefaultKey:  "file",
				Exclude:     map[string]string{"file": "*cloudinit*"},
				Fields: []BlockField{
					{Name: "interface", Key: "interface", Role: "index", Computed: true, Default: "ide2", Description: "PVE device name (default ide2)."},
					{Name: "iso", Key: "file", Required: true, Description: "ISO volume (e.g. local:iso/debian-12.iso)."},
					{Key: "media", Const: "cdrom"},
				},
			},
			{
				Name:        "network",
				Description: "Network interface; list position is the PVE slot (network[0] is net0). Cloud-init addresses (ipv4/ipv6) land on the matching ipconfig slot.",
				Families:    []string{"net"},
				DefaultKey:  "model",
				Fields: []BlockField{
					{Name: "model", Key: "model", Role: "model", Computed: true, Default: "virtio", Enum: []string{"e1000", "e1000-82540em", "e1000-82544gc", "e1000-82545em", "e1000e", "i82551", "i82557b", "i82559er", "ne2k_isa", "ne2k_pci", "pcnet", "rtl8139", "virtio", "vmxnet3"}, Description: "NIC model."},
					{Name: "mac_address", Key: "macaddr", Role: "mac", Computed: true, Description: "MAC address (PVE assigns one when unset)."},
					{Name: "bridge", Key: "bridge", Description: "Bridge to attach to."},
					{Name: "vlan", Key: "tag", Type: "int64", Description: "VLAN tag."},
					{Name: "firewall", Key: "firewall", Type: "bool", Description: "Enable the firewall on this NIC."},
					{Name: "ipv4", Family: "ipconfig", Key: "ip", Description: "Cloud-init IPv4 (CIDR, dhcp or manual)."},
					{Name: "gateway", Family: "ipconfig", Key: "gw", Description: "Cloud-init IPv4 gateway."},
					{Name: "ipv6", Family: "ipconfig", Key: "ip6", Description: "Cloud-init IPv6 (CIDR, auto, dhcp or manual)."},
					{Name: "gateway6", Family: "ipconfig", Key: "gw6", Description: "Cloud-init IPv6 gateway."},
				},
			},
		},
	},
	{
		Type: "pve_vm", Kind: "data_source", Phase: 3,
		Summary: "One VM: configuration, pending changes, feature support, runtime status, cloud-init state, and guest-agent facts.",
		Reads: []*Endpoint{
			{Path: "/nodes/{node}/qemu/{vmid}/config", ResultName: "config"},
			{Path: "/nodes/{node}/qemu/{vmid}/pending", ResultName: "pending"},
			{Path: "/nodes/{node}/qemu/{vmid}/feature", ResultName: "feature_support", SkipIfMissing: []string{"feature"}},
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
		Attrs: []AttrRule{
			{Name: "feature", Required: BoolPtr(false), Optional: BoolPtr(true), Description: "Feature to check (e.g. snapshot, clone, copy); the feature read is skipped when unset."},
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
