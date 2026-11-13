// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package mapping

// Phase8 — cluster lifecycle & configuration. See gen/MAPPING.md.
var Phase8 = []Type{
	{
		Type: "pve_cluster_options", Kind: "resource", Phase: 8,
		Summary:  "Global cluster options (datacenter config: migration, ha, console, …).",
		Behavior: "singleton",
		Create:   &Endpoint{Path: "/cluster/options", Method: "PUT"},
		Read:     &Endpoint{Path: "/cluster/options"},
		Update:   &Endpoint{Path: "/cluster/options", Method: "PUT"},
		Delete:   &Endpoint{Path: "/cluster/options", Method: "PUT"},
	},
	{
		Type: "pve_cluster", Kind: "resource", Phase: 8,
		Summary:     "Cluster creation on the seed node (create-only lifecycle; destroy removes it from state only — no API delete exists).",
		Behavior:    "state-only-delete",
		AllForceNew: true,
		Create:      &Endpoint{Path: "/cluster/config"},
		Read:        &Endpoint{Path: "/cluster/config"},
		IDAttrs:     []string{"clustername"},
	},
	{
		Type: "pve_cluster_join_info", Kind: "data_source", Phase: 8,
		Summary: "Join information a peer node needs (fingerprint, ticket, allowed IPs/links).",
		Reads:   []*Endpoint{{Path: "/cluster/config/join", ResultName: "join_info"}},
	},
	{
		Type: "pve_cluster_join", Kind: "action", Phase: 8,
		Summary: "Join this node to an existing cluster.",
		Invoke:  &Endpoint{Path: "/cluster/config/join"},
	},
	{
		Type: "pve_cluster_node_remove", Kind: "action", Phase: 8,
		Summary: "Remove a node from the cluster (risky).",
		Invoke:  &Endpoint{Path: "/cluster/config/nodes/{node}", Method: "DELETE"},
	},
	{
		Type: "pve_mapping_dir", Kind: "resource", Phase: 8,
		Summary: "Cluster resource mapping for directories.",
		Create:  &Endpoint{Path: "/cluster/mapping/dir"},
		Read:    &Endpoint{Path: "/cluster/mapping/dir/{id}"},
		Update:  &Endpoint{Path: "/cluster/mapping/dir/{id}"},
		Delete:  &Endpoint{Path: "/cluster/mapping/dir/{id}"},
		Attrs:   []AttrRule{{Name: "item_id", ForceNew: true}},
	},
	{
		Type: "pve_mapping_pci", Kind: "resource", Phase: 8,
		Summary: "Cluster resource mapping for PCI devices.",
		Create:  &Endpoint{Path: "/cluster/mapping/pci"},
		Read:    &Endpoint{Path: "/cluster/mapping/pci/{id}"},
		Update:  &Endpoint{Path: "/cluster/mapping/pci/{id}"},
		Delete:  &Endpoint{Path: "/cluster/mapping/pci/{id}"},
		Attrs:   []AttrRule{{Name: "item_id", ForceNew: true}},
	},
	{
		Type: "pve_mapping_usb", Kind: "resource", Phase: 8,
		Summary: "Cluster resource mapping for USB devices.",
		Create:  &Endpoint{Path: "/cluster/mapping/usb"},
		Read:    &Endpoint{Path: "/cluster/mapping/usb/{id}"},
		Update:  &Endpoint{Path: "/cluster/mapping/usb/{id}"},
		Delete:  &Endpoint{Path: "/cluster/mapping/usb/{id}"},
		Attrs:   []AttrRule{{Name: "item_id", ForceNew: true}},
	},
	{
		Type: "pve_cluster_mappings", Kind: "data_source", Phase: 8,
		Summary: "All cluster resource mappings (dir, pci, usb) in one read.",
		Reads:   []*Endpoint{{Path: "/cluster/mapping", ResultName: "mappings"}},
	},
	{
		Type: "pve_cluster_api_version", Kind: "data_source", Phase: 8,
		Summary: "Cluster configuration API version.",
		Reads:   []*Endpoint{{Path: "/cluster/config/apiversion", ResultName: "api_version"}},
	},
	{
		Type: "pve_cluster_qdevice", Kind: "data_source", Phase: 8,
		Summary: "QDevice status of the cluster.",
		Reads:   []*Endpoint{{Path: "/cluster/config/qdevice", ResultName: "qdevice"}},
	},
	{
		Type: "pve_cluster_totem", Kind: "data_source", Phase: 8,
		Summary: "Corosync totem protocol status.",
		Reads:   []*Endpoint{{Path: "/cluster/config/totem", ResultName: "totem"}},
	},
	{
		Type: "pve_cluster_nodes", Kind: "data_source", Phase: 8,
		Summary: "Cluster node inventory from the cluster configuration.",
		Reads:   []*Endpoint{{Path: "/cluster/config/nodes", ResultName: "nodes"}},
	},
}
