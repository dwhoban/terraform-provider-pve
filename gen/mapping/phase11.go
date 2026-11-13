// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package mapping

// Phase11 — SDN (software-defined networking): zones, vnets, subnets,
// controllers, dns/ipam backends, prefix-lists & route-maps (FRR control
// plane), fabrics, plus apply/rollback/lock lifecycle actions and runtime
// status data sources.
var Phase11 = []Type{
	{
		Type: "pve_sdn_zone", Kind: "resource", Phase: 11,
		Summary: "SDN zone configuration (any zone type: simple, vlan, qinq, vxlan, evpn, faucet).",
		Create:  &Endpoint{Path: "/cluster/sdn/zones"},
		Read:    &Endpoint{Path: "/cluster/sdn/zones/{zone}"},
		Update:  &Endpoint{Path: "/cluster/sdn/zones/{zone}"},
		Delete:  &Endpoint{Path: "/cluster/sdn/zones/{zone}"},
		Attrs:   []AttrRule{{Name: "zone", ForceNew: true}},
	},
	{
		Type: "pve_sdn_vnet", Kind: "resource", Phase: 11,
		Summary: "SDN VNet (virtual bridge) configuration attached to a zone.",
		Create:  &Endpoint{Path: "/cluster/sdn/vnets"},
		Read:    &Endpoint{Path: "/cluster/sdn/vnets/{vnet}"},
		Update:  &Endpoint{Path: "/cluster/sdn/vnets/{vnet}"},
		Delete:  &Endpoint{Path: "/cluster/sdn/vnets/{vnet}"},
		Attrs:   []AttrRule{{Name: "vnet", ForceNew: true}, {Name: "type", Skip: true}}, // constant "vnet"
	},
	{
		Type: "pve_sdn_subnet", Kind: "resource", Phase: 11,
		Summary: "SDN subnet (IP range with optional DHCP/gateway/SNAT) inside a VNet; the resource id includes the VNet.",
		Create:  &Endpoint{Path: "/cluster/sdn/vnets/{vnet}/subnets", ConstParams: map[string]string{"type": "subnet"}},
		// PVE creates a subnet from its CIDR but addresses it as
		// "<zone>-<ip>-<prefix>" ({subnet} in the item paths), so the read
		// is a filtered list on the CIDR and the path id is a computed
		// attribute taken from the response's "subnet" field.
		Read:    &Endpoint{Path: "/cluster/sdn/vnets/{vnet}/subnets", Filter: [][2]string{{"cidr", "subnet"}}},
		Update:  &Endpoint{Path: "/cluster/sdn/vnets/{vnet}/subnets/{subnet}", SkipParams: []string{"subnet"}},
		Delete:  &Endpoint{Path: "/cluster/sdn/vnets/{vnet}/subnets/{subnet}", SkipParams: []string{"subnet"}},
		IDAttrs: []string{"vnet", "subnet"},
		Attrs: []AttrRule{
			{Name: "type", Skip: true}, // constant "subnet"; sent via ConstParams
			{Name: "vnet", ForceNew: true},
			{Name: "subnet", ForceNew: true, NoPathParam: true, EchoName: "cidr", Description: "Subnet in CIDR notation (e.g. `10.0.0.0/24`)."},
			{Name: "subnet_id", Add: true, Type: "string", Computed: BoolPtr(true), PathParam: "subnet", EchoName: "subnet", APIName: "subnet_id", Description: "PVE's internal subnet identifier (`<zone>-<ip>-<prefix>`), used in item API paths."},
		},
	},
	{
		Type: "pve_sdn_controller", Kind: "resource", Phase: 11,
		Summary: "SDN controller configuration (bgp, evpn, faucet, isis).",
		Create:  &Endpoint{Path: "/cluster/sdn/controllers"},
		Read:    &Endpoint{Path: "/cluster/sdn/controllers/{controller}"},
		Update:  &Endpoint{Path: "/cluster/sdn/controllers/{controller}"},
		Delete:  &Endpoint{Path: "/cluster/sdn/controllers/{controller}"},
		Attrs:   []AttrRule{{Name: "controller", ForceNew: true}},
	},
	{
		Type: "pve_sdn_dns", Kind: "resource", Phase: 11,
		Summary: "SDN DNS server backend configuration (powerdns).",
		Create:  &Endpoint{Path: "/cluster/sdn/dns"},
		Read:    &Endpoint{Path: "/cluster/sdn/dns/{dns}"},
		Update:  &Endpoint{Path: "/cluster/sdn/dns/{dns}"},
		Delete:  &Endpoint{Path: "/cluster/sdn/dns/{dns}"},
		Attrs: []AttrRule{
			{Name: "dns", ForceNew: true},
			{Name: "key", Sensitive: true},
		},
	},
	{
		Type: "pve_sdn_ipam", Kind: "resource", Phase: 11,
		Summary: "SDN IPAM backend configuration (pve, netbox, phpipam).",
		Create:  &Endpoint{Path: "/cluster/sdn/ipams"},
		Read:    &Endpoint{Path: "/cluster/sdn/ipams/{ipam}"},
		Update:  &Endpoint{Path: "/cluster/sdn/ipams/{ipam}"},
		Delete:  &Endpoint{Path: "/cluster/sdn/ipams/{ipam}"},
		Attrs:   []AttrRule{{Name: "ipam", ForceNew: true}},
	},
	{
		Type: "pve_sdn_ipam_status", Kind: "data_source", Phase: 11,
		Summary: "IP address mappings tracked by one SDN IPAM backend.",
		Reads: []*Endpoint{
			{Path: "/cluster/sdn/ipams/{ipam}/status", ResultName: "entries"},
		},
	},
	{
		Type: "pve_sdn_prefix_list", Kind: "resource", Phase: 11,
		Summary: "FRR prefix list used by BGP/EVPN route filtering.",
		Create:  &Endpoint{Path: "/cluster/sdn/prefix-lists"},
		Read:    &Endpoint{Path: "/cluster/sdn/prefix-lists/{id}"},
		Update:  &Endpoint{Path: "/cluster/sdn/prefix-lists/{id}"},
		Delete:  &Endpoint{Path: "/cluster/sdn/prefix-lists/{id}"},
		Attrs:   []AttrRule{{Name: "item_id", ForceNew: true, PathParam: "id"}},
	},
	{
		Type: "pve_sdn_prefix_list_entry", Kind: "resource", Phase: 11,
		Summary: "One permit/deny entry of an FRR prefix list; url_seq identifies the entry inside the list.",
		Create:  &Endpoint{Path: "/cluster/sdn/prefix-lists/{id}/entries"},
		Read:    &Endpoint{Path: "/cluster/sdn/prefix-lists/{id}/entries/{url_seq}"},
		Update:  &Endpoint{Path: "/cluster/sdn/prefix-lists/{id}/entries/{url_seq}"},
		Delete:  &Endpoint{Path: "/cluster/sdn/prefix-lists/{id}/entries/{url_seq}"},
		Attrs: []AttrRule{
			{Name: "item_id", ForceNew: true, PathParam: "id"},
			{Name: "url_seq", Type: "int64"},
		},
	},
	{
		Type: "pve_sdn_route_map_entry", Kind: "resource", Phase: 11,
		Summary: "One entry (order, match, set, action) of an FRR route map.",
		Create:  &Endpoint{Path: "/cluster/sdn/route-maps/entries"},
		Read:    &Endpoint{Path: "/cluster/sdn/route-maps/entries/{route-map-id}/entry/{order}"},
		Update:  &Endpoint{Path: "/cluster/sdn/route-maps/entries/{route-map-id}/entry/{order}"},
		Delete:  &Endpoint{Path: "/cluster/sdn/route-maps/entries/{route-map-id}/entry/{order}"},
		Attrs: []AttrRule{
			{Name: "route_map_id", ForceNew: true},
			{Name: "order", ForceNew: true},
		},
	},
	{
		Type: "pve_sdn_route_maps", Kind: "data_source", Phase: 11,
		Summary: "Configured FRR route maps: id index, flat entry list, and (optionally) all entries of one route map.",
		Reads: []*Endpoint{
			{Path: "/cluster/sdn/route-maps", ResultName: "route_maps"},
			{Path: "/cluster/sdn/route-maps/entries", ResultName: "entries"},
			{Path: "/cluster/sdn/route-maps/entries/{route-map-id}", ResultName: "route_map", SkipIfMissingParam: true},
		},
		Attrs: []AttrRule{
			{Name: "route_map_id", Required: BoolPtr(false), Optional: BoolPtr(true)},
		},
	},
	{
		Type: "pve_sdn_fabric", Kind: "resource", Phase: 11,
		Summary: "SDN routing fabric (openfabric, ospf, wireguard, bgp); item_id is the fabric id (API placeholder id).",
		Create:  &Endpoint{Path: "/cluster/sdn/fabrics/fabric"},
		Read:    &Endpoint{Path: "/cluster/sdn/fabrics/fabric/{id}"},
		Update:  &Endpoint{Path: "/cluster/sdn/fabrics/fabric/{id}"},
		Delete:  &Endpoint{Path: "/cluster/sdn/fabrics/fabric/{id}"},
		Attrs:   []AttrRule{{Name: "item_id", ForceNew: true, PathParam: "id"}},
	},
	{
		Type: "pve_sdn_fabrics", Kind: "data_source", Phase: 11,
		Summary: "SDN fabric inventory: endpoint index, all configured fabrics, and the fabric node configurations.",
		Reads: []*Endpoint{
			{Path: "/cluster/sdn/fabrics", ResultName: "index"},
			{Path: "/cluster/sdn/fabrics/all"},
			{Path: "/cluster/sdn/fabrics/node", ResultName: "nodes"},
		},
	},
	{
		Type: "pve_sdn_fabric_node", Kind: "resource", Phase: 11,
		Summary: "One node's participation in an SDN fabric (endpoint, interfaces, peers, role).",
		Create:  &Endpoint{Path: "/cluster/sdn/fabrics/node/{fabric_id}"},
		Read:    &Endpoint{Path: "/cluster/sdn/fabrics/node/{fabric_id}/{node_id}"},
		Update:  &Endpoint{Path: "/cluster/sdn/fabrics/node/{fabric_id}/{node_id}"},
		Delete:  &Endpoint{Path: "/cluster/sdn/fabrics/node/{fabric_id}/{node_id}"},
		Attrs: []AttrRule{
			{Name: "fabric_id", ForceNew: true},
			{Name: "node_id", ForceNew: true},
		},
	},
	{
		Type: "pve_sdn_apply", Kind: "action", Phase: 11,
		Summary: "Apply pending SDN configuration and reload (PUT /cluster/sdn).",
		Invoke:  &Endpoint{Path: "/cluster/sdn", Method: "PUT"},
	},
	{
		Type: "pve_sdn_rollback", Kind: "action", Phase: 11,
		Summary: "Roll back pending SDN configuration to the last applied state.",
		Invoke:  &Endpoint{Path: "/cluster/sdn/rollback"},
	},
	{
		Type: "pve_sdn_lock_release", Kind: "action", Phase: 11,
		Summary: "Release (or force-release) the global SDN configuration lock.",
		Invoke:  &Endpoint{Path: "/cluster/sdn/lock", Method: "DELETE"},
	},
	{
		Type: "pve_sdn_ip_manage", Kind: "action", Phase: 11,
		Summary: "Create/update an IP mapping (MAC/IP/VMID) in a VNet via its IPAM.",
		Invoke:  &Endpoint{Path: "/cluster/sdn/vnets/{vnet}/ips"},
	},
	{
		Type: "pve_sdn_dry_run", Kind: "data_source", Phase: 11,
		Summary: "Dry-run of the SDN apply: diff of pending vs current FRR and /etc/network/interfaces.d/sdn configuration on one node.",
		Reads: []*Endpoint{
			{Path: "/cluster/sdn/dry-run"},
		},
	},
	{
		Type: "pve_sdn_status", Kind: "data_source", Phase: 11,
		Summary: "Runtime SDN state on a node: zones (plus bridges/content/ip-vrf per zone), vnets (plus mac-vrf), and fabric interfaces/neighbors/routes.",
		Reads: []*Endpoint{
			{Path: "/nodes/{node}/sdn", ResultName: "index"},
			{Path: "/nodes/{node}/sdn/zones", ResultName: "zones"},
			{Path: "/nodes/{node}/sdn/zones/{zone}", ResultName: "zone_status", SkipIfMissingParam: true},
			{Path: "/nodes/{node}/sdn/zones/{zone}/bridges", ResultName: "bridges", SkipIfMissingParam: true},
			{Path: "/nodes/{node}/sdn/zones/{zone}/content", ResultName: "content", SkipIfMissingParam: true},
			{Path: "/nodes/{node}/sdn/zones/{zone}/ip-vrf", ResultName: "ip_vrf", SkipIfMissingParam: true},
			{Path: "/nodes/{node}/sdn/vnets/{vnet}", ResultName: "vnet_status", SkipIfMissingParam: true},
			{Path: "/nodes/{node}/sdn/vnets/{vnet}/mac-vrf", ResultName: "mac_vrf", SkipIfMissingParam: true},
			{Path: "/nodes/{node}/sdn/fabrics/{fabric}", ResultName: "fabric_status", SkipIfMissingParam: true},
			{Path: "/nodes/{node}/sdn/fabrics/{fabric}/interfaces", ResultName: "interfaces", SkipIfMissingParam: true},
			{Path: "/nodes/{node}/sdn/fabrics/{fabric}/neighbors", ResultName: "neighbors", SkipIfMissingParam: true},
			{Path: "/nodes/{node}/sdn/fabrics/{fabric}/routes", ResultName: "routes", SkipIfMissingParam: true},
		},
		Attrs: []AttrRule{
			{Name: "zone", Required: BoolPtr(false), Optional: BoolPtr(true)},
			{Name: "vnet", Required: BoolPtr(false), Optional: BoolPtr(true)},
			{Name: "fabric", Required: BoolPtr(false), Optional: BoolPtr(true)},
		},
	},
	{
		Type: "pve_sdn_vnet_firewall", Kind: "data_source", Phase: 11,
		Summary: "Firewall state scoped to one VNet: options, rule list, and (optionally) a single rule by position.",
		Reads: []*Endpoint{
			{Path: "/cluster/sdn/vnets/{vnet}/firewall", ResultName: "index"},
			{Path: "/cluster/sdn/vnets/{vnet}/firewall/options"},
			{Path: "/cluster/sdn/vnets/{vnet}/firewall/rules", ResultName: "rules"},
			{Path: "/cluster/sdn/vnets/{vnet}/firewall/rules/{pos}", ResultName: "rule", SkipIfMissingParam: true},
		},
		Attrs: []AttrRule{
			{Name: "pos", Required: BoolPtr(false), Optional: BoolPtr(true)},
		},
	},
}
