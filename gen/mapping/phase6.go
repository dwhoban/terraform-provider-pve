// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package mapping

// Phase6 — node system management: network staging/apply, node singletons,
// disks, subscription, services, APT, and node power actions.
var Phase6 = []Type{
	{
		Type: "pve_node_network", Kind: "resource", Phase: 6,
		Summary: "Node network interface. Models PVE's staging semantics: create, update, and delete only stage the change; the apply endpoint (PUT /network) commits it.",
		IDAttrs: []string{"node", "iface"},
		// Every mutation is staged; the chained Apply op commits it.
		Create: &Endpoint{
			Path: "/nodes/{node}/network",
			Apply: &Endpoint{
				Path:   "/nodes/{node}/network",
				Method: "PUT",
			},
		},
		Read: &Endpoint{Path: "/nodes/{node}/network/{iface}"},
		Update: &Endpoint{
			Path: "/nodes/{node}/network/{iface}",
			Apply: &Endpoint{
				Path:   "/nodes/{node}/network",
				Method: "PUT",
			},
		},
		Delete: &Endpoint{
			Path: "/nodes/{node}/network/{iface}",
			Apply: &Endpoint{
				Path:   "/nodes/{node}/network",
				Method: "PUT",
			},
		},
		Attrs: []AttrRule{
			{Name: "iface", ForceNew: true},
		},
	},
	{
		Type: "pve_node_dns", Kind: "resource", Phase: 6,
		Summary:  "Node DNS resolver configuration (singleton).",
		Behavior: "singleton",
		Create:   &Endpoint{Path: "/nodes/{node}/dns", Method: "PUT"},
		Read:     &Endpoint{Path: "/nodes/{node}/dns"},
		Update:   &Endpoint{Path: "/nodes/{node}/dns", Method: "PUT"},
	},
	{
		Type: "pve_node_time", Kind: "resource", Phase: 6,
		Summary:  "Node time zone and NTP servers (singleton).",
		Behavior: "singleton",
		Create:   &Endpoint{Path: "/nodes/{node}/time", Method: "PUT"},
		Read:     &Endpoint{Path: "/nodes/{node}/time"},
		Update:   &Endpoint{Path: "/nodes/{node}/time", Method: "PUT"},
	},
	{
		Type: "pve_node_hosts", Kind: "resource", Phase: 6,
		Summary:  "Content of /etc/hosts on a node (singleton; written via POST per API).",
		Behavior: "singleton",
		Create:   &Endpoint{Path: "/nodes/{node}/hosts"}, // POST per apidoc
		Read:     &Endpoint{Path: "/nodes/{node}/hosts"},
	},
	{
		Type: "pve_node_config", Kind: "resource", Phase: 6,
		Summary:  "Node-specific configuration (ACME, description, wake-on-LAN, …) (singleton).",
		Behavior: "singleton",
		Create:   &Endpoint{Path: "/nodes/{node}/config", Method: "PUT"},
		Read:     &Endpoint{Path: "/nodes/{node}/config"},
		Update:   &Endpoint{Path: "/nodes/{node}/config", Method: "PUT"},
		Attrs:    []AttrRule{{Name: "acmedomain_n_", Skip: true}},
	},
	{
		Type: "pve_node_disk_lvm", Kind: "resource", Phase: 6,
		Summary: "LVM volume group created on a node's disk(s). The API offers no per-VG read; the list endpoint is filtered by VG name.",
		IDAttrs: []string{"node", "name"},
		Create:  &Endpoint{Path: "/nodes/{node}/disks/lvm"},
		Read: &Endpoint{
			Path:   "/nodes/{node}/disks/lvm",
			Filter: [][2]string{{"name", "name"}},
		},
		Delete: &Endpoint{Path: "/nodes/{node}/disks/lvm/{name}"},
		// No update endpoint: every change recreates.
		AllForceNew: true,
	},
	{
		Type: "pve_node_disk_lvmthin", Kind: "resource", Phase: 6,
		Summary: "LVM-thin pool created on a node's disk(s). Read filters the list endpoint on the thin-LV name (create `name` == list `lv`).",
		IDAttrs: []string{"node", "name"},
		Create:  &Endpoint{Path: "/nodes/{node}/disks/lvmthin"},
		Read: &Endpoint{
			Path:   "/nodes/{node}/disks/lvmthin",
			Filter: [][2]string{{"lv", "name"}},
		},
		Delete:      &Endpoint{Path: "/nodes/{node}/disks/lvmthin/{name}"},
		AllForceNew: true,
	},
	{
		Type: "pve_node_disk_zfs", Kind: "resource", Phase: 6,
		Summary:     "ZFS pool created on a node's disk(s).",
		IDAttrs:     []string{"node", "name"},
		Create:      &Endpoint{Path: "/nodes/{node}/disks/zfs"},
		Read:        &Endpoint{Path: "/nodes/{node}/disks/zfs/{name}"},
		Delete:      &Endpoint{Path: "/nodes/{node}/disks/zfs/{name}"},
		AllForceNew: true,
	},
	{
		Type: "pve_node_disk_directory", Kind: "resource", Phase: 6,
		Summary: "Directory-formatted disk mounted via systemd unit on a node. Read filters the list endpoint on the underlying device.",
		IDAttrs: []string{"node", "name"},
		Create:  &Endpoint{Path: "/nodes/{node}/disks/directory"},
		Read: &Endpoint{
			Path:   "/nodes/{node}/disks/directory",
			Filter: [][2]string{{"device", "device"}},
		},
		Delete:      &Endpoint{Path: "/nodes/{node}/disks/directory/{name}"},
		AllForceNew: true,
	},
	{
		Type: "pve_node_disk_initgpt", Kind: "action", Phase: 6,
		Summary: "Initialize a disk with a fresh GPT (destructive).",
		Invoke:  &Endpoint{Path: "/nodes/{node}/disks/initgpt"},
	},
	{
		Type: "pve_node_disk_wipe", Kind: "action", Phase: 6,
		Summary: "Wipe a disk (destructive).",
		Invoke:  &Endpoint{Path: "/nodes/{node}/disks/wipedisk", Method: "PUT"},
	},
	{
		Type: "pve_subscription", Kind: "resource", Phase: 6,
		Summary:  "Node subscription key: set via PUT, checked via the API, removable.",
		Behavior: "singleton",
		Create:   &Endpoint{Path: "/nodes/{node}/subscription", Method: "PUT"},
		Read:     &Endpoint{Path: "/nodes/{node}/subscription"},
		Update:   &Endpoint{Path: "/nodes/{node}/subscription", Method: "PUT"},
		Delete:   &Endpoint{Path: "/nodes/{node}/subscription"},
	},
	{
		Type: "pve_node_services", Kind: "data_source", Phase: 6,
		Summary: "Services of a node: list, and per-service detail/state when a service is specified.",
		Reads: []*Endpoint{
			{Path: "/nodes/{node}/services", ResultName: "services"},
			{Path: "/nodes/{node}/services/{service}", ResultName: "detail", SkipIfMissingParam: true},
			{Path: "/nodes/{node}/services/{service}/state", ResultName: "state", SkipIfMissingParam: true},
		},
		Attrs: []AttrRule{
			{Name: "service", Required: BoolPtr(false), Optional: BoolPtr(true)},
		},
	},
	{
		Type: "pve_service_start", Kind: "action", Phase: 6,
		Summary: "Start a service on a node.",
		Invoke:  &Endpoint{Path: "/nodes/{node}/services/{service}/start"},
	},
	{
		Type: "pve_service_stop", Kind: "action", Phase: 6,
		Summary: "Stop a service on a node.",
		Invoke:  &Endpoint{Path: "/nodes/{node}/services/{service}/stop"},
	},
	{
		Type: "pve_service_restart", Kind: "action", Phase: 6,
		Summary: "Restart a service on a node.",
		Invoke:  &Endpoint{Path: "/nodes/{node}/services/{service}/restart"},
	},
	{
		Type: "pve_service_reload", Kind: "action", Phase: 6,
		Summary: "Reload a service on a node.",
		Invoke:  &Endpoint{Path: "/nodes/{node}/services/{service}/reload"},
	},
	{
		Type: "pve_apt_repositories", Kind: "resource", Phase: 6,
		Summary:  "APT repository configuration of a node (singleton): create adds a standard repository by handle (PUT), update toggles enabled at path+index (POST).",
		Behavior: "singleton",
		Create:   &Endpoint{Path: "/nodes/{node}/apt/repositories", Method: "PUT"},  // PUT: add_repository by handle
		Read:     &Endpoint{Path: "/nodes/{node}/apt/repositories"},                 // GET: parsed state
		Update:   &Endpoint{Path: "/nodes/{node}/apt/repositories", Method: "POST"}, // POST: change_repository (enabled at path+index)
		Attrs: []AttrRule{
			// Create (PUT add_repository) input: the standard repo handle to add.
			{Name: "handle", Type: "string", Add: true, Optional: BoolPtr(true)},
		},
	},
	{
		Type: "pve_apt", Kind: "data_source", Phase: 6,
		Summary: "APT package index of a node (package entries).",
		Reads:   []*Endpoint{{Path: "/nodes/{node}/apt", ResultName: "entries"}},
	},
	{
		Type: "pve_apt_versions", Kind: "data_source", Phase: 6,
		Summary: "Installed/available package versions on a node.",
		Reads:   []*Endpoint{{Path: "/nodes/{node}/apt/versions", ResultName: "versions"}},
	},
	{
		Type: "pve_apt_changelog", Kind: "data_source", Phase: 6,
		Summary: "Changelog of one package version on a node.",
		Reads:   []*Endpoint{{Path: "/nodes/{node}/apt/changelog", ResultName: "changelog"}},
	},
	{
		Type: "pve_apt_update", Kind: "action", Phase: 6,
		Summary: "Refresh the APT package index on a node.",
		Invoke:  &Endpoint{Path: "/nodes/{node}/apt/update"},
	},
	{
		Type: "pve_apt_update_status", Kind: "data_source", Phase: 6,
		Summary: "Pending APT package updates of a node (the readable counterpart of the update action).",
		Reads:   []*Endpoint{{Path: "/nodes/{node}/apt/update", ResultName: "updates"}},
	},
	{
		Type: "pve_node_reboot", Kind: "action", Phase: 6,
		Summary: "Reboot a node (POST /status with command=reboot).",
		Invoke:  &Endpoint{Path: "/nodes/{node}/status", CrossPhase: true},
		Attrs:   []AttrRule{{Name: "command", Default: "reboot"}},
	},
	{
		Type: "pve_node_shutdown", Kind: "action", Phase: 6,
		Summary: "Shut a node down (POST /status with command=shutdown).",
		Invoke:  &Endpoint{Path: "/nodes/{node}/status", CrossPhase: true},
		Attrs:   []AttrRule{{Name: "command", Default: "shutdown"}},
	},
	{
		Type: "pve_node_startall", Kind: "action", Phase: 6,
		Summary: "Start all guests on a node.",
		Invoke:  &Endpoint{Path: "/nodes/{node}/startall"},
	},
	{
		Type: "pve_node_stopall", Kind: "action", Phase: 6,
		Summary: "Stop all guests on a node.",
		Invoke:  &Endpoint{Path: "/nodes/{node}/stopall"},
	},
	{
		Type: "pve_node_suspendall", Kind: "action", Phase: 6,
		Summary: "Suspend all guests on a node.",
		Invoke:  &Endpoint{Path: "/nodes/{node}/suspendall"},
	},
	{
		Type: "pve_node_wakeonlan", Kind: "action", Phase: 6,
		Summary: "Send a wake-on-LAN packet to a node.",
		Invoke:  &Endpoint{Path: "/nodes/{node}/wakeonlan"},
	},
	{
		Type: "pve_node_migrate_all", Kind: "action", Phase: 6,
		Summary: "Migrate all guests of a node to another node.",
		Invoke:  &Endpoint{Path: "/nodes/{node}/migrateall"},
	},
}
