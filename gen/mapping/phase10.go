// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package mapping

// Phase10 — Firewall: rules, ipsets, aliases, groups, and options across
// the cluster, node, vm, and container scopes (one shared schema per type;
// per-scope families where the API provides them).
var Phase10 = []Type{
	{
		Type: "pve_firewall", Kind: "data_source", Phase: 10,
		Summary: "Firewall sub-system index of one scope (cluster, node, vm, container).",
		Scopes:  []string{"cluster", "node", "vm", "container"},
		Reads: []*Endpoint{
			{Paths: map[string]string{
				"cluster":   "/cluster/firewall",
				"node":      "/nodes/{node}/firewall",
				"vm":        "/nodes/{node}/qemu/{vmid}/firewall",
				"container": "/nodes/{node}/lxc/{vmid}/firewall",
			}, ResultName: "firewall"},
		},
		Attrs: []AttrRule{
			{Name: "scope", Type: "string", Add: true, Required: BoolPtr(true)},
			{Name: "node", Type: "string", Add: true, PathParam: "node", Optional: BoolPtr(true)},
			{Name: "vmid", Type: "int64", Add: true, PathParam: "vmid", Optional: BoolPtr(true)},
		},
	},
	{
		Type: "pve_firewall_rule", Kind: "resource", Phase: 10,
		Summary: "Firewall rule at cluster, node, vm, or container scope.",
		Scopes:  []string{"cluster", "node", "vm", "container"},
		Create: &Endpoint{Paths: map[string]string{
			"cluster":   "/cluster/firewall/rules",
			"node":      "/nodes/{node}/firewall/rules",
			"vm":        "/nodes/{node}/qemu/{vmid}/firewall/rules",
			"container": "/nodes/{node}/lxc/{vmid}/firewall/rules",
		}},
		Read: &Endpoint{Paths: map[string]string{
			"cluster":   "/cluster/firewall/rules/{pos}",
			"node":      "/nodes/{node}/firewall/rules/{pos}",
			"vm":        "/nodes/{node}/qemu/{vmid}/firewall/rules/{pos}",
			"container": "/nodes/{node}/lxc/{vmid}/firewall/rules/{pos}",
		}},
		Update: &Endpoint{Paths: map[string]string{
			"cluster":   "/cluster/firewall/rules/{pos}",
			"node":      "/nodes/{node}/firewall/rules/{pos}",
			"vm":        "/nodes/{node}/qemu/{vmid}/firewall/rules/{pos}",
			"container": "/nodes/{node}/lxc/{vmid}/firewall/rules/{pos}",
		}},
		Delete: &Endpoint{Paths: map[string]string{
			"cluster":   "/cluster/firewall/rules/{pos}",
			"node":      "/nodes/{node}/firewall/rules/{pos}",
			"vm":        "/nodes/{node}/qemu/{vmid}/firewall/rules/{pos}",
			"container": "/nodes/{node}/lxc/{vmid}/firewall/rules/{pos}",
		}},
		IDAttrs: []string{"scope", "node", "vmid", "pos"},
		Attrs: []AttrRule{
			{Name: "scope", Type: "string", Add: true, Required: BoolPtr(true)},
			{Name: "node", Type: "string", Add: true, PathParam: "node", Optional: BoolPtr(true)},
			{Name: "vmid", Type: "int64", Add: true, PathParam: "vmid", Optional: BoolPtr(true)},
			{Name: "pos", Type: "int64", PathParam: "pos", Optional: BoolPtr(true), Computed: BoolPtr(true)},
		},
	},
	{
		Type: "pve_firewall_ipset", Kind: "resource", Phase: 10,
		Summary: "Firewall IP set at cluster, vm, or container scope.",
		Scopes:  []string{"cluster", "vm", "container"},
		Create: &Endpoint{Paths: map[string]string{
			"cluster":   "/cluster/firewall/ipset",
			"vm":        "/nodes/{node}/qemu/{vmid}/firewall/ipset",
			"container": "/nodes/{node}/lxc/{vmid}/firewall/ipset",
		}},
		Read: &Endpoint{Paths: map[string]string{
			"cluster":   "/cluster/firewall/ipset/{name}",
			"vm":        "/nodes/{node}/qemu/{vmid}/firewall/ipset/{name}",
			"container": "/nodes/{node}/lxc/{vmid}/firewall/ipset/{name}",
		}},
		Delete: &Endpoint{Paths: map[string]string{
			"cluster":   "/cluster/firewall/ipset/{name}",
			"vm":        "/nodes/{node}/qemu/{vmid}/firewall/ipset/{name}",
			"container": "/nodes/{node}/lxc/{vmid}/firewall/ipset/{name}",
		}},
		IDAttrs:     []string{"scope", "node", "vmid", "name"},
		AllForceNew: true, // no PUT on the ipset families
		Attrs: []AttrRule{
			{Name: "scope", Type: "string", Add: true, Required: BoolPtr(true)},
			{Name: "node", Type: "string", Add: true, PathParam: "node", Optional: BoolPtr(true)},
			{Name: "vmid", Type: "int64", Add: true, PathParam: "vmid", Optional: BoolPtr(true)},
			{Name: "name", PathParam: "name", Required: BoolPtr(true), ForceNew: true},
		},
	},
	{
		Type: "pve_firewall_ipset_entry", Kind: "resource", Phase: 10,
		Summary: "One CIDR entry (with comment/nomatch) inside a firewall IP set.",
		Scopes:  []string{"cluster", "vm", "container"},
		Create: &Endpoint{Paths: map[string]string{
			"cluster":   "/cluster/firewall/ipset/{name}",
			"vm":        "/nodes/{node}/qemu/{vmid}/firewall/ipset/{name}",
			"container": "/nodes/{node}/lxc/{vmid}/firewall/ipset/{name}",
		}},
		Read: &Endpoint{Paths: map[string]string{
			"cluster":   "/cluster/firewall/ipset/{name}/{cidr}",
			"vm":        "/nodes/{node}/qemu/{vmid}/firewall/ipset/{name}/{cidr}",
			"container": "/nodes/{node}/lxc/{vmid}/firewall/ipset/{name}/{cidr}",
		}},
		Update: &Endpoint{Paths: map[string]string{
			"cluster":   "/cluster/firewall/ipset/{name}/{cidr}",
			"vm":        "/nodes/{node}/qemu/{vmid}/firewall/ipset/{name}/{cidr}",
			"container": "/nodes/{node}/lxc/{vmid}/firewall/ipset/{name}/{cidr}",
		}},
		Delete: &Endpoint{Paths: map[string]string{
			"cluster":   "/cluster/firewall/ipset/{name}/{cidr}",
			"vm":        "/nodes/{node}/qemu/{vmid}/firewall/ipset/{name}/{cidr}",
			"container": "/nodes/{node}/lxc/{vmid}/firewall/ipset/{name}/{cidr}",
		}},
		IDAttrs: []string{"scope", "node", "vmid", "name", "cidr"},
		Attrs: []AttrRule{
			{Name: "scope", Type: "string", Add: true, Required: BoolPtr(true)},
			{Name: "node", Type: "string", Add: true, PathParam: "node", Optional: BoolPtr(true)},
			{Name: "vmid", Type: "int64", Add: true, PathParam: "vmid", Optional: BoolPtr(true)},
			{Name: "name", PathParam: "name", Required: BoolPtr(true), ForceNew: true},
			{Name: "cidr", PathParam: "cidr", Required: BoolPtr(true), ForceNew: true},
		},
	},
	{
		Type: "pve_firewall_alias", Kind: "resource", Phase: 10,
		Summary: "Firewall alias (name to CIDR/IP mapping) at cluster, vm, or container scope.",
		Scopes:  []string{"cluster", "vm", "container"},
		Create: &Endpoint{Paths: map[string]string{
			"cluster":   "/cluster/firewall/aliases",
			"vm":        "/nodes/{node}/qemu/{vmid}/firewall/aliases",
			"container": "/nodes/{node}/lxc/{vmid}/firewall/aliases",
		}},
		Read: &Endpoint{Paths: map[string]string{
			"cluster":   "/cluster/firewall/aliases/{name}",
			"vm":        "/nodes/{node}/qemu/{vmid}/firewall/aliases/{name}",
			"container": "/nodes/{node}/lxc/{vmid}/firewall/aliases/{name}",
		}},
		Update: &Endpoint{Paths: map[string]string{
			"cluster":   "/cluster/firewall/aliases/{name}",
			"vm":        "/nodes/{node}/qemu/{vmid}/firewall/aliases/{name}",
			"container": "/nodes/{node}/lxc/{vmid}/firewall/aliases/{name}",
		}},
		Delete: &Endpoint{Paths: map[string]string{
			"cluster":   "/cluster/firewall/aliases/{name}",
			"vm":        "/nodes/{node}/qemu/{vmid}/firewall/aliases/{name}",
			"container": "/nodes/{node}/lxc/{vmid}/firewall/aliases/{name}",
		}},
		IDAttrs: []string{"scope", "node", "vmid", "name"},
		Attrs: []AttrRule{
			{Name: "scope", Type: "string", Add: true, Required: BoolPtr(true)},
			{Name: "node", Type: "string", Add: true, PathParam: "node", Optional: BoolPtr(true)},
			{Name: "vmid", Type: "int64", Add: true, PathParam: "vmid", Optional: BoolPtr(true)},
			{Name: "name", PathParam: "name", Required: BoolPtr(true), ForceNew: true},
		},
	},
	{
		Type: "pve_firewall_group", Kind: "resource", Phase: 10,
		Summary: "Firewall security group (cluster scope; rules are separate resources).",
		Create:  &Endpoint{Path: "/cluster/firewall/groups"},
		Read:    &Endpoint{Path: "/cluster/firewall/groups/{group}"},
		Delete:  &Endpoint{Path: "/cluster/firewall/groups/{group}"},
		// no PUT on /cluster/firewall/groups/{group}
		IDAttrs:     []string{"group"},
		AllForceNew: true,
		Attrs: []AttrRule{
			{Name: "group", PathParam: "group", Required: BoolPtr(true), ForceNew: true},
		},
	},
	{
		Type: "pve_firewall_group_rule", Kind: "resource", Phase: 10,
		Summary: "Member rule at one position of a firewall security group.",
		Create:  &Endpoint{Path: "/cluster/firewall/groups/{group}"},
		Read:    &Endpoint{Path: "/cluster/firewall/groups/{group}/{pos}"},
		Update:  &Endpoint{Path: "/cluster/firewall/groups/{group}/{pos}"},
		Delete:  &Endpoint{Path: "/cluster/firewall/groups/{group}/{pos}"},
		IDAttrs: []string{"group", "pos"},
		Attrs: []AttrRule{
			{Name: "pos", Type: "int64", PathParam: "pos", Optional: BoolPtr(true), Computed: BoolPtr(true)},
		},
	},
	{
		Type: "pve_firewall_options", Kind: "resource", Phase: 10,
		Summary:  "Firewall options singleton at cluster, node, vm, or container scope.",
		Behavior: "singleton",
		Scopes:   []string{"cluster", "node", "vm", "container"},
		Create: &Endpoint{Method: "PUT", Paths: map[string]string{
			"cluster":   "/cluster/firewall/options",
			"node":      "/nodes/{node}/firewall/options",
			"vm":        "/nodes/{node}/qemu/{vmid}/firewall/options",
			"container": "/nodes/{node}/lxc/{vmid}/firewall/options",
		}},
		Read: &Endpoint{Paths: map[string]string{
			"cluster":   "/cluster/firewall/options",
			"node":      "/nodes/{node}/firewall/options",
			"vm":        "/nodes/{node}/qemu/{vmid}/firewall/options",
			"container": "/nodes/{node}/lxc/{vmid}/firewall/options",
		}},
		Update: &Endpoint{Method: "PUT", Paths: map[string]string{
			"cluster":   "/cluster/firewall/options",
			"node":      "/nodes/{node}/firewall/options",
			"vm":        "/nodes/{node}/qemu/{vmid}/firewall/options",
			"container": "/nodes/{node}/lxc/{vmid}/firewall/options",
		}},
		IDAttrs: []string{"scope", "node", "vmid"},
		Attrs: []AttrRule{
			{Name: "scope", Type: "string", Add: true, Required: BoolPtr(true)},
			{Name: "node", Type: "string", Add: true, PathParam: "node", Optional: BoolPtr(true)},
			{Name: "vmid", Type: "int64", Add: true, PathParam: "vmid", Optional: BoolPtr(true)},
		},
	},
	{
		Type: "pve_firewall_macros", Kind: "data_source", Phase: 10,
		Summary: "Firewall macros (pre-defined rule sets) known to the cluster.",
		Reads:   []*Endpoint{{Path: "/cluster/firewall/macros", ResultName: "macros"}},
	},
	{
		Type: "pve_firewall_refs", Kind: "data_source", Phase: 10,
		Summary: "Firewall object references (ipsets, aliases, groups) usable in rules.",
		Scopes:  []string{"cluster", "vm", "container"},
		Reads: []*Endpoint{
			{Paths: map[string]string{
				"cluster":   "/cluster/firewall/refs",
				"vm":        "/nodes/{node}/qemu/{vmid}/firewall/refs",
				"container": "/nodes/{node}/lxc/{vmid}/firewall/refs",
			}, ResultName: "refs"},
		},
		Attrs: []AttrRule{
			{Name: "scope", Type: "string", Add: true, Required: BoolPtr(true)},
			{Name: "node", Type: "string", Add: true, PathParam: "node", Optional: BoolPtr(true)},
			{Name: "vmid", Type: "int64", Add: true, PathParam: "vmid", Optional: BoolPtr(true)},
		},
	},
}
