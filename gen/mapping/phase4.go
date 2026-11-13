// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package mapping

// Phase4 — LXC containers: container resource (create/clone/resize/move/
// template), snapshot resource + rollback, lifecycle actions, and the
// read-only container data source (config, status, interfaces).

var Phase4 = []Type{
	{
		Type: "pve_container", Kind: "resource", Phase: 4,
		Summary: "LXC container on a node: configuration, lifecycle, clone-at-create.",
		Create:  &Endpoint{Path: "/nodes/{node}/lxc", Method: "POST"},
		Read:    &Endpoint{Path: "/nodes/{node}/lxc/{vmid}/config"},
		Update:  &Endpoint{Path: "/nodes/{node}/lxc/{vmid}/config", Method: "PUT"},
		Delete:  &Endpoint{Path: "/nodes/{node}/lxc/{vmid}", Method: "DELETE"},
		Clone: &Clone{
			Path:        "/nodes/{node}/lxc/{vmid}/clone",
			Method:      "POST",
			SourceAttr:  "clone_source_vmid",
			SourceParam: "vmid",
			NewIDParam:  "newid",
		},
		Attrs: []AttrRule{
			{Name: "vmid", Required: BoolPtr(true), ForceNew: true, Description: "Guest id; use provider::pve::nextid() to allocate a free one."},
			// Per-write conflict token.
			{Name: "digest", Skip: true},
			{Name: "clone_source_vmid", Type: "int64", Add: true, Optional: BoolPtr(true), ForceNew: true},
			// Slot families not modeled as blocks.
			{Name: "dev_n_", Skip: true},
			{Name: "unused_n_", Skip: true},
		},
		Blocks: []Block{
			{
				Name:        "rootfs",
				Description: "Root volume. PVE normalizes an allocation to the concrete volume path, which cannot be pre-stated; leave unset to use the create-time `storage` parameter.",
				Families:    []string{"rootfs"},
				Fixed:       true,
				DefaultKey:  "volume",
				Fields: []BlockField{
					{Name: "storage", Key: "volume", Role: "volume-storage", CreateOnly: true, Description: "Storage to allocate the root volume from (with size_gb)."},
					{Name: "size_gb", Key: "size", Role: "volume-size-gb", Type: "int64", Computed: true, CreateOnly: true, Description: "Root volume size in GiB."},
					{Name: "volume_id", Key: "volume", Role: "volume-id", Computed: true, CreateOnly: true, Description: "Root volume (e.g. local-lvm:vm-120-disk-0)."},
				},
			},
			{
				Name:        "mount_point",
				Description: "Additional mount point; list position is the PVE slot (mount_point[0] is mp0).",
				Families:    []string{"mp"},
				DefaultKey:  "volume",
				Fields: []BlockField{
					{Name: "path", Key: "mp", Required: true, Description: "Mount path inside the container."},
					{Name: "storage", Key: "volume", Role: "volume-storage", CreateOnly: true, Description: "Storage to allocate a new volume from (with size_gb)."},
					{Name: "size_gb", Key: "size", Role: "volume-size-gb", Type: "int64", Computed: true, CreateOnly: true, Description: "Volume size in GiB."},
					{Name: "volume_id", Key: "volume", Role: "volume-id", Computed: true, CreateOnly: true, Description: "Mounted volume (e.g. local-lvm:vm-120-disk-1)."},
					{Name: "backup", Key: "backup", Type: "bool", Description: "Include the volume in backups."},
					{Name: "read_only", Key: "ro", Type: "bool", Description: "Mount read-only."},
				},
			},
			{
				Name:        "network",
				Description: "Network interface; list position is the PVE slot (network[0] is net0).",
				Families:    []string{"net"},
				Fields: []BlockField{
					{Name: "name", Key: "name", Required: true, Description: "Interface name inside the container (e.g. eth0)."},
					{Name: "bridge", Key: "bridge", Description: "Bridge to attach to."},
					{Name: "vlan", Key: "tag", Type: "int64", Description: "VLAN tag."},
					{Name: "firewall", Key: "firewall", Type: "bool", Description: "Enable the firewall on this interface."},
					{Name: "ipv4", Key: "ip", Description: "IPv4 (CIDR, dhcp or manual)."},
					{Name: "gateway", Key: "gw", Description: "IPv4 gateway."},
					{Name: "ipv6", Key: "ip6", Description: "IPv6 (CIDR, auto, dhcp or manual)."},
					{Name: "gateway6", Key: "gw6", Description: "IPv6 gateway."},
					{Name: "mac_address", Key: "hwaddr", Computed: true, Description: "MAC address (PVE assigns one when unset)."},
				},
			},
		},
	},
	{
		Type: "pve_container", Kind: "data_source", Phase: 4,
		Summary: "One container: configuration, pending changes, capabilities, runtime status, and guest network interfaces.",
		Reads: []*Endpoint{
			{Path: "/nodes/{node}/lxc/{vmid}/config", ResultName: "config"},
			{Path: "/nodes/{node}/lxc/{vmid}/pending", ResultName: "pending"},
			{Path: "/nodes/{node}/lxc/{vmid}/feature", ResultName: "feature_support", SkipIfMissing: []string{"feature"}},
			{Path: "/nodes/{node}/lxc/{vmid}/status", ResultName: "status"},
			{Path: "/nodes/{node}/lxc/{vmid}/status/current", ResultName: "current"},
			{Path: "/nodes/{node}/lxc/{vmid}/interfaces", ResultName: "interfaces"},
		},
		Attrs: []AttrRule{
			{Name: "feature", Required: BoolPtr(false), Optional: BoolPtr(true), Description: "Feature to check (e.g. snapshot, clone, copy); the feature read is skipped when unset."},
		},
	},
	{
		Type: "pve_container_snapshot", Kind: "resource", Phase: 4,
		IDAttrs: []string{"node", "vmid", "snapname"},
		Summary: "Container snapshot (name, description, RAM state).",
		Create:  &Endpoint{Path: "/nodes/{node}/lxc/{vmid}/snapshot", Method: "POST"},
		// The item GET returns the whole snapshot list; select this
		// snapshot from it (same shape as the QEMU snapshot endpoint).
		Read: &Endpoint{
			Path:   "/nodes/{node}/lxc/{vmid}/snapshot",
			Method: "GET",
			Filter: [][2]string{{"name", "snapname"}},
		},
		Update: &Endpoint{Path: "/nodes/{node}/lxc/{vmid}/snapshot/{snapname}/config", Method: "PUT"},
		Delete: &Endpoint{Path: "/nodes/{node}/lxc/{vmid}/snapshot/{snapname}", Method: "DELETE"},
	},

	// Container one-shot operations.
	{
		Type: "pve_container_resize", Kind: "action", Phase: 4,
		Summary: "Resize a container volume.",
		Invoke:  &Endpoint{Path: "/nodes/{node}/lxc/{vmid}/resize", Method: "PUT"},
	},
	{
		Type: "pve_container_move_volume", Kind: "action", Phase: 4,
		Summary: "Move a container volume to another storage.",
		Invoke:  &Endpoint{Path: "/nodes/{node}/lxc/{vmid}/move_volume", Method: "POST"},
	},
	{
		Type: "pve_container_template", Kind: "action", Phase: 4,
		Summary: "Convert a container to a template.",
		Invoke:  &Endpoint{Path: "/nodes/{node}/lxc/{vmid}/template", Method: "POST"},
	},
	{
		Type: "pve_container_snapshot_rollback", Kind: "action", Phase: 4,
		Summary: "Roll a container back to a snapshot.",
		Invoke:  &Endpoint{Path: "/nodes/{node}/lxc/{vmid}/snapshot/{snapname}/rollback", Method: "POST"},
	},
	{
		Type: "pve_container_migrate", Kind: "action", Phase: 4,
		Summary: "Migrate a container to another node.",
		Invoke:  &Endpoint{Path: "/nodes/{node}/lxc/{vmid}/migrate", Method: "POST"},
	},
	{
		Type: "pve_container_remote_migrate", Kind: "action", Phase: 4,
		Summary: "Migrate a container to a remote cluster.",
		Invoke:  &Endpoint{Path: "/nodes/{node}/lxc/{vmid}/remote_migrate", Method: "POST"},
	},
	{
		Type: "pve_container_start", Kind: "action", Phase: 4,
		Summary: "Start a container.",
		Invoke:  &Endpoint{Path: "/nodes/{node}/lxc/{vmid}/status/start", Method: "POST"},
	},
	{
		Type: "pve_container_stop", Kind: "action", Phase: 4,
		Summary: "Stop a container (hard power-off).",
		Invoke:  &Endpoint{Path: "/nodes/{node}/lxc/{vmid}/status/stop", Method: "POST"},
	},
	{
		Type: "pve_container_reboot", Kind: "action", Phase: 4,
		Summary: "Reboot a container.",
		Invoke:  &Endpoint{Path: "/nodes/{node}/lxc/{vmid}/status/reboot", Method: "POST"},
	},
	{
		Type: "pve_container_shutdown", Kind: "action", Phase: 4,
		Summary: "Shut a container down gracefully.",
		Invoke:  &Endpoint{Path: "/nodes/{node}/lxc/{vmid}/status/shutdown", Method: "POST"},
	},
	{
		Type: "pve_container_suspend", Kind: "action", Phase: 4,
		Summary: "Suspend a container.",
		Invoke:  &Endpoint{Path: "/nodes/{node}/lxc/{vmid}/status/suspend", Method: "POST"},
	},
	{
		Type: "pve_container_resume", Kind: "action", Phase: 4,
		Summary: "Resume a suspended container.",
		Invoke:  &Endpoint{Path: "/nodes/{node}/lxc/{vmid}/status/resume", Method: "POST"},
	},
}
