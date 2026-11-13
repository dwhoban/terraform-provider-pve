// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

// Package mapping is the generator input: one Go file per roadmap phase
// declaring every provider type, its endpoints (exact apidoc.js family
// paths), and optional attribute overrides. The generator derives complete
// attribute schemas from the API schema and audits full coverage.
package mapping

// Type declares one provider type. Endpoints reference apidoc.js family
// paths verbatim (e.g. "/nodes/{node}/qemu/{vmid}/config").
type Type struct {
	Type     string // full type name, e.g. "pve_storage"
	Kind     string // "resource" | "data_source" | "action" | "function"
	Phase    int
	Summary  string
	Behavior string // "" (collection) | "singleton" | "acl" | "state-only-delete"

	// Scopes enables the scope attribute (firewall types): the map keys of
	// every endpoint's Paths must match these values.
	Scopes []string

	// IDAttrs is the ordered resource ID composition (also the import
	// format). Empty: derived from the read path placeholders.
	IDAttrs []string
	IDSep   string // default "/"

	Create *Endpoint
	Read   *Endpoint
	Update *Endpoint
	Delete *Endpoint
	Clone  *Clone

	Reads  []*Endpoint // data sources
	Invoke *Endpoint   // actions
	Call   *Endpoint   // functions

	// Attrs overrides derived attributes by name and can add new ones.
	Attrs []AttrRule

	// AllForceNew marks every optional attribute as force-new (for
	// resources without an update endpoint, e.g. cluster creation).
	AllForceNew bool
}

// Endpoint wires one operation to an API family.
type Endpoint struct {
	Method string // GET | POST | PUT | DELETE; default derived from role
	// Path is the exact apidoc.js family path. Required unless Paths is set.
	Path string
	// Paths maps each scope value to its family path (scoped types).
	Paths map[string]string

	// Params restricts the derived request parameters to these API names.
	// Empty: all parameters of the method except SkipParams.
	Params     []string
	SkipParams []string

	// WaitTask: default true for mutating methods — the engine only waits
	// when the response actually is a UPID, so this is safe everywhere.
	WaitTask *bool

	ResultName string // data source: attribute under which the response lands

	// SkipIfMissingParam: data source detail read; skipped when a path
	// parameter it needs is null.
	SkipIfMissingParam bool

	// SkipIfMissing: attribute names that must be set for this read to run.
	SkipIfMissing []string

	// Filter selects one object from an array response: (field, attr) pairs.
	Filter [][2]string

	// Apply is a staged-apply endpoint invoked after this one.
	Apply *Endpoint

	IDFromUPID  string // create: attribute filled from the UPID id component
	ResultAttr  string // create: computed attribute receiving the raw string response
	ResultField string // update: object-response field extracted into ResultAttr

	// ConstParams are fixed request parameters sent on every call.
	ConstParams map[string]string

	// CrossPhase acknowledges the family belongs to another phase (audited).
	CrossPhase bool
}

// Clone enables clone-at-create.
type Clone struct {
	Path        string // clone family path
	Method      string // default POST
	SourceAttr  string // attribute holding the source guest id
	SourceParam string // path placeholder the source fills (e.g. "vmid")
	NewIDParam  string // request param receiving the new id (e.g. "newid")
	OnlyParams  []string
	// ConstParams are fixed clone request parameters (e.g. full=1).
	ConstParams map[string]string
}

// AttrRule adjusts one derived attribute (matched by Terraform name) or
// declares an additional attribute.
type AttrRule struct {
	Name    string
	APIName string
	// EchoName is the response field when it differs from the request
	// field (see provider.Attr.EchoName).
	EchoName  string
	Type      string // string | bool | int64 | float64 | list | set | object
	Elem      string // scalar element type for list/set
	Required  *bool
	Optional  *bool
	Computed  *bool
	Sensitive bool
	ForceNew  bool
	// Description overrides or sets the attribute description.
	Description string
	// NoDefault drops an apidoc default that PVE itself would reject
	// (e.g. vcpus=0, vmgenid=1).
	NoDefault bool
	Skip      bool   // drop the derived attribute
	PathParam string // bind to this path placeholder
	// NoPathParam unbinds a derived placeholder attribute (its value is a
	// body parameter; another attribute carries the path identifier).
	NoPathParam bool
	Enum        []string
	Default     string // rendered into the description
	Add         bool   // force-add even when not derived
}

// FamilyImplemented lists families covered by the hand-written Phase 1
// foundation (the client's /version and /access/ticket usage) rather than
// by a generated type.
var FamilyImplemented = map[string]bool{
	"/version":       true,
	"/access/ticket": true,
}

// BoolPtr is a convenience for AttrRule tri-state fields.
func BoolPtr(b bool) *bool { return &b }

// Phases returns the human labels used in ROADMAP.md Appendix A.
var PhaseLabels = map[int]string{
	1:  "Foundation",
	2:  "Inventory data sources",
	3:  "Virtual machines",
	4:  "Containers",
	5:  "Storage & pools",
	6:  "Node system management",
	7:  "Access control",
	8:  "Cluster lifecycle",
	9:  "Data protection & HA",
	10: "Firewall",
	11: "SDN",
	12: "Ceph",
	13: "Integrations",
}

// All returns every declared type across all phase files.
func All() []Type {
	var out []Type
	for _, group := range [][]Type{Phase2, Phase3, Phase4, Phase5, Phase6, Phase7, Phase8, Phase9, Phase10, Phase11, Phase12, Phase13} {
		out = append(out, group...)
	}
	return out
}
