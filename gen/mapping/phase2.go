// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package mapping

// Phase2 — inventory & capability data sources (read-only) plus the nextid
// provider function. This file is the reference example for mapping syntax:
// see gen/MAPPING.md.
var Phase2 = []Type{
	{
		Type: "pve_cluster", Kind: "data_source", Phase: 2,
		Summary: "Cluster inventory: identity, quorum status, resource list, and recent tasks.",
		Reads: []*Endpoint{
			{Path: "/cluster", ResultName: "info"},
			{Path: "/cluster/status", ResultName: "members"},
			{Path: "/cluster/resources", ResultName: "resources"},
			{Path: "/cluster/tasks", ResultName: "tasks"},
		},
	},
	{
		Type: "pve_nodes", Kind: "data_source", Phase: 2,
		Summary: "All nodes visible to the API endpoint.",
		Reads: []*Endpoint{
			{Path: "/nodes", ResultName: "nodes"},
		},
	},
	{
		Type: "pve_node", Kind: "data_source", Phase: 2,
		Summary: "One node: inventory entry, runtime status, and PVE version.",
		Reads: []*Endpoint{
			{Path: "/nodes/{node}", ResultName: "info"},
			{Path: "/nodes/{node}/status", ResultName: "status"},
			{Path: "/nodes/{node}/version", ResultName: "version"},
		},
	},
	{
		Type: "pve_node_capabilities", Kind: "data_source", Phase: 2,
		Summary: "QEMU capabilities of a node: CPU models, CPU flags, machine types, and migration limits.",
		Reads: []*Endpoint{
			{Path: "/nodes/{node}/capabilities", ResultName: "capabilities"},
			{Path: "/nodes/{node}/capabilities/qemu", ResultName: "qemu"},
			{Path: "/nodes/{node}/capabilities/qemu/cpu", ResultName: "cpu_models"},
			{Path: "/nodes/{node}/capabilities/qemu/cpu-flags", ResultName: "cpu_flags"},
			{Path: "/nodes/{node}/capabilities/qemu/machines", ResultName: "machines"},
			{Path: "/nodes/{node}/capabilities/qemu/migration", ResultName: "migration"},
		},
	},
	{
		Type: "pve_node_hardware", Kind: "data_source", Phase: 2,
		Summary: "Passthrough inventory of a node: PCI devices, USB devices, and (for one PCI device) mediated device profiles.",
		Reads: []*Endpoint{
			{Path: "/nodes/{node}/hardware", ResultName: "overview"},
			{Path: "/nodes/{node}/hardware/pci", ResultName: "pci"},
			{Path: "/nodes/{node}/hardware/usb", ResultName: "usb"},
			{Path: "/nodes/{node}/hardware/pci/{pci-id-or-mapping}", ResultName: "pci_detail", SkipIfMissingParam: true},
			{Path: "/nodes/{node}/hardware/pci/{pci-id-or-mapping}/mdev", ResultName: "mdev", SkipIfMissingParam: true},
		},
		Attrs: []AttrRule{
			{Name: "pci_id_or_mapping", Required: BoolPtr(false), Optional: BoolPtr(true)},
		},
	},
	{
		Type: "pve_node_disks", Kind: "data_source", Phase: 2,
		Summary: "Physical disks of a node (list) with optional S.M.A.R.T. health for one disk.",
		Reads: []*Endpoint{
			{Path: "/nodes/{node}/disks", ResultName: "overview"},
			{Path: "/nodes/{node}/disks/list", ResultName: "disks"},
			{Path: "/nodes/{node}/disks/smart", ResultName: "smart", SkipIfMissing: []string{"disk"}},
		},
		Attrs: []AttrRule{
			{Name: "disk", Required: BoolPtr(false), Optional: BoolPtr(true)},
		},
	},
	{
		Type: "pve_node_storages", Kind: "data_source", Phase: 2,
		Summary: "Storage pools available on a node; optionally one pool's detail and status.",
		Reads: []*Endpoint{
			{Path: "/nodes/{node}/storage", ResultName: "storages"},
			{Path: "/nodes/{node}/storage/{storage}", ResultName: "detail", SkipIfMissingParam: true},
			{Path: "/nodes/{node}/storage/{storage}/status", ResultName: "status", SkipIfMissingParam: true},
		},
		Attrs: []AttrRule{
			{Name: "storage", Required: BoolPtr(false), Optional: BoolPtr(true)},
		},
	},
	{
		Type: "pve_storage_scan_nfs", Kind: "data_source", Phase: 2,
		Summary: "NFS exports discovered by scanning from a node (server optionally constrained).",
		Reads: []*Endpoint{
			// /scan lists the scanner types available on the node; one
			// canonical home for it (shared by all scan_* types).
			{Path: "/nodes/{node}/scan", ResultName: "scan_types"},
			{Path: "/nodes/{node}/scan/nfs", ResultName: "entries"},
		},
	},
	{
		Type: "pve_storage_scan_cifs", Kind: "data_source", Phase: 2,
		Summary: "CIFS shares discovered by scanning from a node.",
		Reads:   []*Endpoint{{Path: "/nodes/{node}/scan/cifs", ResultName: "entries"}},
	},
	{
		Type: "pve_storage_scan_iscsi", Kind: "data_source", Phase: 2,
		Summary: "iSCSI targets discovered by scanning from a node.",
		Reads:   []*Endpoint{{Path: "/nodes/{node}/scan/iscsi", ResultName: "entries"}},
	},
	{
		Type: "pve_storage_scan_lvm", Kind: "data_source", Phase: 2,
		Summary: "LVM volume groups discovered by scanning from a node.",
		Reads:   []*Endpoint{{Path: "/nodes/{node}/scan/lvm", ResultName: "entries"}},
	},
	{
		Type: "pve_storage_scan_lvmthin", Kind: "data_source", Phase: 2,
		Summary: "LVM-thin pools discovered by scanning from a node.",
		Reads:   []*Endpoint{{Path: "/nodes/{node}/scan/lvmthin", ResultName: "entries"}},
	},
	{
		Type: "pve_storage_scan_zfs", Kind: "data_source", Phase: 2,
		Summary: "ZFS pools discovered by scanning from a node.",
		Reads:   []*Endpoint{{Path: "/nodes/{node}/scan/zfs", ResultName: "entries"}},
	},
	{
		Type: "pve_storage_scan_pbs", Kind: "data_source", Phase: 2,
		Summary: "Proxmox Backup Server instances discovered by scanning from a node.",
		Reads:   []*Endpoint{{Path: "/nodes/{node}/scan/pbs", ResultName: "entries"}},
	},
	{
		Type: "pve_appliance_templates", Kind: "data_source", Phase: 2,
		Summary: "Turnkey appliance templates available on a node.",
		Reads:   []*Endpoint{{Path: "/nodes/{node}/aplinfo", ResultName: "templates"}},
	},
	{
		Type: "pve_task", Kind: "data_source", Phase: 2,
		Summary: "Status of one task by UPID.",
		Reads: []*Endpoint{
			{Path: "/nodes/{node}/tasks/{upid}", ResultName: "entry"},
			{Path: "/nodes/{node}/tasks/{upid}/status", ResultName: "task"},
			{Path: "/nodes/{node}/tasks", ResultName: "recent"},
		},
	},
	{
		Type: "pve_backup_info", Kind: "data_source", Phase: 2,
		Summary: "Backups known to the cluster (per storage/guest) and guests that have no backup.",
		Reads: []*Endpoint{
			{Path: "/cluster/backup-info", ResultName: "backups", CrossPhase: true},
			{Path: "/cluster/backup-info/not-backed-up", ResultName: "not_backed_up", CrossPhase: true},
		},
	},
	{
		Type: "pve_url_metadata", Kind: "data_source", Phase: 2,
		Summary: "Metadata (filename, size, checksum) of a URL as retrieved by a node.",
		Reads: []*Endpoint{
			{Path: "/nodes/{node}/query-url-metadata", ResultName: "metadata"},
		},
	},
	{
		Type: "pve_oci_repo_tags", Kind: "data_source", Phase: 2,
		Summary: "Tags of an OCI image repository as queried from a node.",
		Reads: []*Endpoint{
			{Path: "/nodes/{node}/query-oci-repo-tags", ResultName: "tags"},
		},
	},
	{
		Type: "pve_nextid", Kind: "function", Phase: 2,
		Summary: "Returns the next free VMID.",
		Call:    &Endpoint{Path: "/cluster/nextid"},
	},
}
