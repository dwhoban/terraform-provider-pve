// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package mapping

// Phase12 — Ceph: OSD/pool/service resources, flags, lifecycle actions
// (init, start/stop/restart, osd in/out/scrub, health mutes, rolling
// restarts) and status/capability data sources.
var Phase12 = []Type{
	{
		Type: "pve_ceph_osd", Kind: "resource", Phase: 12,
		Summary: "One Ceph OSD created on a disk (optionally encrypted, with dedicated db/wal devices); osdid is assigned from the creation task.",
		Create:  &Endpoint{Path: "/nodes/{node}/ceph/osd", IDFromUPID: "osdid"},
		Read:    &Endpoint{Path: "/nodes/{node}/ceph/osd/{osdid}"},
		Delete:  &Endpoint{Path: "/nodes/{node}/ceph/osd/{osdid}"},
		Attrs: []AttrRule{
			{Name: "node", ForceNew: true},
			{Name: "osdid", Required: BoolPtr(false), Optional: BoolPtr(true), Computed: BoolPtr(true)},
		},
		AllForceNew: true,
	},
	{
		Type: "pve_ceph_pool", Kind: "resource", Phase: 12,
		Summary: "Ceph pool (replicated or erasure-coded) manageable through the POST/PUT settings surface.",
		Create:  &Endpoint{Path: "/nodes/{node}/ceph/pool"},
		Read:    &Endpoint{Path: "/nodes/{node}/ceph/pool/{name}"},
		Update:  &Endpoint{Path: "/nodes/{node}/ceph/pool/{name}"},
		Delete:  &Endpoint{Path: "/nodes/{node}/ceph/pool/{name}"},
		Attrs: []AttrRule{
			{Name: "node", ForceNew: true},
			{Name: "name", ForceNew: true},
		},
	},
	{
		Type: "pve_ceph_pool_status", Kind: "data_source", Phase: 12,
		Summary: "Current status of one Ceph pool (application, autoscaler, placement groups).",
		Reads: []*Endpoint{
			{Path: "/nodes/{node}/ceph/pool/{name}/status", ResultName: "status"},
		},
	},
	{
		Type: "pve_ceph_mon", Kind: "resource", Phase: 12,
		Summary: "One Ceph monitor on a node (creating the first monitor auto-creates a manager).",
		Create:  &Endpoint{Path: "/nodes/{node}/ceph/mon/{monid}"},
		Read:    &Endpoint{Path: "/nodes/{node}/ceph/mon", Filter: [][2]string{{"name", "monid"}}},
		Delete:  &Endpoint{Path: "/nodes/{node}/ceph/mon/{monid}"},
		IDAttrs: []string{"node", "monid"},
		Attrs: []AttrRule{
			{Name: "node", ForceNew: true},
			{Name: "monid", Required: BoolPtr(true), Optional: BoolPtr(false), ForceNew: true},
		},
		AllForceNew: true,
	},
	{
		Type: "pve_ceph_mgr", Kind: "resource", Phase: 12,
		Summary: "One Ceph manager on a node; item_id is the manager id (API placeholder id).",
		Create:  &Endpoint{Path: "/nodes/{node}/ceph/mgr/{id}"},
		Read:    &Endpoint{Path: "/nodes/{node}/ceph/mgr", Filter: [][2]string{{"name", "item_id"}}},
		Delete:  &Endpoint{Path: "/nodes/{node}/ceph/mgr/{id}"},
		IDAttrs: []string{"node", "item_id"},
		Attrs: []AttrRule{
			{Name: "node", ForceNew: true},
			{Name: "item_id", Required: BoolPtr(true), Optional: BoolPtr(false), ForceNew: true, PathParam: "id"},
		},
		AllForceNew: true,
	},
	{
		Type: "pve_ceph_mds", Kind: "resource", Phase: 12,
		Summary: "One Ceph metadata server (CephFS MDS) on a node.",
		Create:  &Endpoint{Path: "/nodes/{node}/ceph/mds/{name}"},
		Read:    &Endpoint{Path: "/nodes/{node}/ceph/mds", Filter: [][2]string{{"name", "name"}}},
		Delete:  &Endpoint{Path: "/nodes/{node}/ceph/mds/{name}"},
		IDAttrs: []string{"node", "name"},
		Attrs: []AttrRule{
			{Name: "node", ForceNew: true},
			{Name: "name", Required: BoolPtr(true), Optional: BoolPtr(false), ForceNew: true},
		},
		AllForceNew: true,
	},
	{
		Type: "pve_ceph_fs", Kind: "resource", Phase: 12,
		Summary: "One Ceph filesystem (metadata + data pools), optionally registered as a storage.",
		Create:  &Endpoint{Path: "/nodes/{node}/ceph/fs/{name}"},
		Read:    &Endpoint{Path: "/nodes/{node}/ceph/fs", Filter: [][2]string{{"name", "name"}}},
		Delete:  &Endpoint{Path: "/nodes/{node}/ceph/fs/{name}"},
		IDAttrs: []string{"node", "name"},
		Attrs: []AttrRule{
			{Name: "node", ForceNew: true},
			{Name: "name", Required: BoolPtr(true), Optional: BoolPtr(false), ForceNew: true},
		},
		AllForceNew: true,
	},
	{
		Type: "pve_ceph_flags", Kind: "resource", Phase: 12, Behavior: "singleton",
		Summary: "All cluster-wide Ceph flags at once (nobackfill, noout, noup, ...); cluster is a constant id anchor.",
		Create:  &Endpoint{Path: "/cluster/ceph/flags", Method: "PUT"},
		Read:    &Endpoint{Path: "/cluster/ceph/flags", ResultName: "flags"},
		Update:  &Endpoint{Path: "/cluster/ceph/flags", Method: "PUT"},
		IDAttrs: []string{"cluster"},
		Attrs:   []AttrRule{{Name: "cluster", Add: true, Type: "string", Required: BoolPtr(true), Default: "cluster"}},
	},
	{
		Type: "pve_ceph_flag", Kind: "resource", Phase: 12, Behavior: "singleton",
		Summary: "One cluster-wide Ceph flag (e.g. noout, noup) set or cleared by name.",
		Create:  &Endpoint{Path: "/cluster/ceph/flags/{flag}", Method: "PUT"},
		Read:    &Endpoint{Path: "/cluster/ceph/flags/{flag}", ResultName: "value"},
		Update:  &Endpoint{Path: "/cluster/ceph/flags/{flag}", Method: "PUT"},
		Attrs:   []AttrRule{{Name: "value", Required: BoolPtr(false), Optional: BoolPtr(true)}},
	},
	{
		Type: "pve_ceph_init", Kind: "action", Phase: 12,
		Summary: "Create the initial Ceph default configuration on a node (idempotent). Ceph must already be installed on the node — run the `pve_ceph_install` action first (it needs the provider's `ssh` block).",
		Invoke:  &Endpoint{Path: "/nodes/{node}/ceph/init"},
	},
	{
		Type: "pve_ceph_start", Kind: "action", Phase: 12,
		Summary: "Start Ceph services on a node.",
		Invoke:  &Endpoint{Path: "/nodes/{node}/ceph/start"},
	},
	{
		Type: "pve_ceph_stop", Kind: "action", Phase: 12,
		Summary: "Stop Ceph services on a node.",
		Invoke:  &Endpoint{Path: "/nodes/{node}/ceph/stop"},
	},
	{
		Type: "pve_ceph_restart", Kind: "action", Phase: 12,
		Summary: "Restart Ceph services on a node.",
		Invoke:  &Endpoint{Path: "/nodes/{node}/ceph/restart"},
	},
	{
		Type: "pve_ceph_osd_in", Kind: "action", Phase: 12,
		Summary: "Mark one OSD in (ceph osd in).",
		Invoke:  &Endpoint{Path: "/nodes/{node}/ceph/osd/{osdid}/in"},
	},
	{
		Type: "pve_ceph_osd_out", Kind: "action", Phase: 12,
		Summary: "Mark one OSD out (ceph osd out).",
		Invoke:  &Endpoint{Path: "/nodes/{node}/ceph/osd/{osdid}/out"},
	},
	{
		Type: "pve_ceph_osd_scrub", Kind: "action", Phase: 12,
		Summary: "Instruct one OSD to scrub (optionally deep).",
		Invoke:  &Endpoint{Path: "/nodes/{node}/ceph/osd/{osdid}/scrub"},
	},
	{
		Type: "pve_ceph_health_mute", Kind: "action", Phase: 12,
		Summary: "Mute or unmute one Ceph health check.",
		Invoke:  &Endpoint{Path: "/cluster/ceph/health-mute/{code}", Method: "PUT"},
	},
	{
		Type: "pve_ceph_restart_bulk", Kind: "action", Phase: 12,
		Summary: "Cluster-wide rolling restart of all Ceph daemons of one service type.",
		Invoke:  &Endpoint{Path: "/cluster/ceph/restart-bulk"},
	},
	{
		Type: "pve_ceph_node_restart_bulk", Kind: "action", Phase: 12,
		Summary: "Rolling restart of all Ceph OSDs on one node.",
		Invoke:  &Endpoint{Path: "/nodes/{node}/ceph/restart-bulk"},
	},
	{
		Type: "pve_ceph_status", Kind: "data_source", Phase: 12,
		Summary: "Ceph cluster status (cluster-wide and per-node alias) plus the service metadata inventory.",
		Reads: []*Endpoint{
			{Path: "/cluster/ceph/status", ResultName: "status"},
			{Path: "/nodes/{node}/ceph/status", ResultName: "node_status", SkipIfMissingParam: true},
			{Path: "/cluster/ceph/metadata", ResultName: "metadata"},
		},
		Attrs: []AttrRule{
			{Name: "node", Required: BoolPtr(false), Optional: BoolPtr(true)},
		},
	},
	{
		Type: "pve_ceph_health_mutes", Kind: "data_source", Phase: 12,
		Summary: "All currently muted Ceph health checks.",
		Reads: []*Endpoint{
			{Path: "/cluster/ceph/health-mute", ResultName: "mutes"},
		},
	},
	{
		Type: "pve_ceph_crush", Kind: "data_source", Phase: 12,
		Summary: "The Ceph CRUSH map of the cluster.",
		Reads: []*Endpoint{
			{Path: "/nodes/{node}/ceph/crush", ResultName: "crush_map"},
		},
	},
	{
		Type: "pve_ceph_rules", Kind: "data_source", Phase: 12,
		Summary: "The Ceph CRUSH rules of the cluster.",
		Reads: []*Endpoint{
			{Path: "/nodes/{node}/ceph/rules", ResultName: "rules"},
		},
	},
	{
		Type: "pve_ceph_releases", Kind: "data_source", Phase: 12,
		Summary: "Ceph releases known to a node, marking which are installable there.",
		Reads: []*Endpoint{
			{Path: "/nodes/{node}/ceph/releases", ResultName: "releases"},
		},
	},
	{
		Type: "pve_ceph_cfg", Kind: "data_source", Phase: 12,
		Summary: "Ceph configuration on a node: cfg database, resolved values, and the raw ceph.conf file.",
		Reads: []*Endpoint{
			{Path: "/nodes/{node}/ceph/cfg", ResultName: "index"},
			{Path: "/nodes/{node}/ceph/cfg/db", ResultName: "db"},
			{Path: "/nodes/{node}/ceph/cfg/value", ResultName: "values"},
			{Path: "/nodes/{node}/ceph/cfg/raw", ResultName: "raw"},
		},
	},
	{
		Type: "pve_ceph_cmd_safety", Kind: "data_source", Phase: 12,
		Summary: "Heuristical Ceph safety check for a destructive action (stop/destroy) on one service.",
		Reads: []*Endpoint{
			{Path: "/nodes/{node}/ceph/cmd-safety"},
		},
	},
	{
		Type: "pve_ceph_osd_metadata", Kind: "data_source", Phase: 12,
		Summary: "Details (devices, versions, locations) of one OSD.",
		Reads: []*Endpoint{
			{Path: "/nodes/{node}/ceph/osd/{osdid}/metadata", ResultName: "metadata"},
		},
	},
	{
		Type: "pve_ceph_osd_lv_info", Kind: "data_source", Phase: 12,
		Summary: "LVM volume details of one OSD (block, db, or wal).",
		Reads: []*Endpoint{
			{Path: "/nodes/{node}/ceph/osd/{osdid}/lv-info", ResultName: "lv_info"},
		},
	},
	{
		Type: "pve_ceph", Kind: "data_source", Phase: 12,
		Summary: "Ceph endpoint indexes: the cluster-wide and the per-node API surface.",
		Reads: []*Endpoint{
			{Path: "/cluster/ceph", ResultName: "cluster_index"},
			{Path: "/nodes/{node}/ceph", ResultName: "node_index", SkipIfMissingParam: true},
		},
		Attrs: []AttrRule{
			{Name: "node", Required: BoolPtr(false), Optional: BoolPtr(true)},
		},
	},
}
