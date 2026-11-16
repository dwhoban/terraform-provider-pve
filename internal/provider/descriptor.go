// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package provider

// This file defines the descriptor model shared by the code generator
// (gen/, emitting registry.json) and the generic runtime engines. The
// descriptors are data: every generated provider type is an instance of one
// of the engines below, parameterized by a TypeDescriptor. Anything the
// engines cannot express from data alone is a bug here or in the generator —
// there are no per-type hand-written implementations.

// Kind enumerates the provider type kinds a descriptor can describe.
type Kind string

const (
	KindResource   Kind = "resource"
	KindDataSource Kind = "data_source"
	KindAction     Kind = "action"
	KindFunction   Kind = "function"
)

// Behavior selects a lifecycle variant inside the resource engine.
type Behavior string

const (
	// BehaviorCollection is standard PVE CRUD: POST a collection to create,
	// GET the item, PUT the item, DELETE the item.
	BehaviorCollection Behavior = ""
	// BehaviorSingleton models GET/PUT-only endpoints (e.g. /nodes/{node}/dns):
	// create and update both PUT the full parameter set, delete is optional.
	BehaviorSingleton Behavior = "singleton"
	// BehaviorACL models PUT-with-parameters creation where the delete is
	// DELETE-with-parameters carrying the same identifying params, and the
	// read is a filtered list read (/access/acl).
	BehaviorACL Behavior = "acl"
	// BehaviorStateOnlyDelete models resources with no API delete (e.g.
	// POST /cluster/config): destroy removes the resource from state only.
	BehaviorStateOnlyDelete Behavior = "state-only-delete"
)

// AttrType is the Terraform attribute type of an attribute.
type AttrType string

const (
	AttrString  AttrType = "string"
	AttrBool    AttrType = "bool"
	AttrInt64   AttrType = "int64"
	AttrFloat64 AttrType = "float64"
	AttrList    AttrType = "list"
	AttrSet     AttrType = "set"
	AttrObject  AttrType = "object"
)

// AttrFormat names a wire encoding applied to an attribute's request value.
type AttrFormat string

const (
	// FormatBase64 marks an attribute sent base64-encoded; PVE reads it
	// back decoded.
	FormatBase64 AttrFormat = "base64"
	// FormatBootOrder marks a list sent as order=a;b;c.
	FormatBootOrder AttrFormat = "boot-order"
)

// BlockSpec maps Terraform blocks onto an indexed PVE slot family: each
// block element is one slot (e.g. scsi0) whose value is a property string.
type BlockSpec struct {
	Families   []string          `json:"families"`             // slot prefixes; element wire key = family+index
	Fixed      bool              `json:"fixed,omitempty"`      // one block; wire key = Families[0], no index (rootfs)
	DefaultKey string            `json:"defaultKey,omitempty"` // property key PVE writes without "key=" (file, model, volume, address)
	Exclude    map[string]string `json:"exclude,omitempty"`    // read filter: drop slots whose property key matches the path.Match pattern
	Fields     []BlockField      `json:"fields"`
}

// BlockField maps one block element attribute onto a property key.
type BlockField struct {
	Name       string    `json:"name,omitempty"`   // element attribute; empty only for Const fields
	Family     string    `json:"family,omitempty"` // secondary family sharing the element index (ipconfig); "" = the element's own slot
	Key        string    `json:"key"`              // property key in the slot value
	Role       FieldRole `json:"role,omitempty"`
	Const      string    `json:"const,omitempty"` // always sent; a slot is this block's only when it carries Key=Const
	True       string    `json:"true,omitempty"`  // bool wire value, default "1"
	False      string    `json:"false,omitempty"` // bool wire value, default "0"
	CreateOnly bool      `json:"createOnly,omitempty"`
}

// PlanFrom names the configured attributes a derived computed value
// depends on and the value planned for each case.
type PlanFrom struct {
	Attrs   []string `json:"attrs"`
	IfSet   string   `json:"ifSet"`
	IfUnset string   `json:"ifUnset"`
}

// FieldRole gives a block field special encode/decode semantics.
type FieldRole string

const (
	RolePlain         FieldRole = ""
	RoleIndex         FieldRole = "index"          // value is the full wire key (disk.interface = "scsi0")
	RoleVolumeStorage FieldRole = "volume-storage" // storage part of DefaultKey; with size allocates "storage:size"
	RoleVolumeSizeGB  FieldRole = "volume-size-gb" // GiB; read from property key "size"
	RoleVolumeID      FieldRole = "volume-id"      // DefaultKey value of an allocated volume
	RoleModel         FieldRole = "model"          // DefaultKey enum; PVE writes "<model>=<mac>"
	RoleMAC           FieldRole = "mac"            // the <mac> of "<model>=<mac>"
)

// Attr describes one attribute of a generated type.
type Attr struct {
	// Name is the Terraform attribute name (snake_case).
	Name string `json:"name"`
	// APIName is the PVE request/response field name when it differs from
	// Name (PVE uses dashes in some field names).
	APIName string `json:"apiName,omitempty"`
	// EchoName is the response field that carries this attribute's value
	// when it differs from the request field (e.g. SDN subnets are created
	// with "subnet" = CIDR but read back as "cidr").
	EchoName string `json:"echoName,omitempty"`
	// PathParam names the {placeholder} in endpoint paths this attribute
	// fills, if any (e.g. "node", "vmid").
	PathParam string `json:"pathParam,omitempty"`

	Type   AttrType   `json:"type"`
	Format AttrFormat `json:"format,omitempty"` // request wire encoding
	// Block declares a slot-family block (list of elements, or one
	// element when Fixed): ElemAttrs (list) or Attrs (Fixed) describe it.
	Block    *BlockSpec `json:"block,omitempty"`
	ElemType AttrType   `json:"elemType,omitempty"` // element type of list/set
	// ElemAttrs describes the element shape when elements are objects
	// (list/set of objects, e.g. data source result rows).
	ElemAttrs []Attr `json:"elemAttrs,omitempty"`
	Attrs     []Attr `json:"attrs,omitempty"` // nested attributes of object

	Required    bool     `json:"required,omitempty"`
	Optional    bool     `json:"optional,omitempty"`
	Computed    bool     `json:"computed,omitempty"`
	Sensitive   bool     `json:"sensitive,omitempty"`
	ForceNew    bool     `json:"forceNew,omitempty"` // resource: change forces replacement
	Description string   `json:"description,omitempty"`
	Default     any      `json:"default,omitempty"`
	Enum        []string `json:"enum,omitempty"`
	// PlanFrom plans a Computed-only string the server derives from the
	// configuration, instead of leaving it unknown on create/update: IfSet
	// when any Attrs value is configured (known, non-null, non-empty),
	// IfUnset otherwise (PVE network `method`: static iff an address is sent).
	PlanFrom *PlanFrom `json:"planFrom,omitempty"`
}

// TFName returns the PVE field name for the attribute: APIName when set,
// otherwise Name.
func (a Attr) TFName() string {
	if a.APIName != "" {
		return a.APIName
	}
	return a.Name
}

// EchoKey returns the response field holding the attribute's value:
// EchoName when set, otherwise the request field name.
func (a Attr) EchoKey() string {
	if a.EchoName != "" {
		return a.EchoName
	}
	return a.TFName()
}

// Op wires one provider operation to a PVE endpoint.
type Op struct {
	Method string `json:"method"` // GET | POST | PUT | DELETE
	// Path is the endpoint path with {placeholders} bound to attributes via
	// their PathParam. For scoped types (firewall), Paths maps each scope
	// to its path instead.
	Path  string            `json:"path,omitempty"`
	Paths map[string]string `json:"paths,omitempty"` // scope name → path

	// Params lists the attribute names sent as request parameters. Only
	// non-null values are sent (PVE PUT semantics: omitted means default).
	Params []string `json:"params,omitempty"`

	// WaitTask: the endpoint answers with a UPID string; poll the task to
	// completion before proceeding.
	WaitTask bool `json:"waitTask,omitempty"`

	// ResultName namespaces a data source read response: array/object
	// responses land under this attribute. Empty merges object responses
	// into top-level computed attributes.
	ResultName string `json:"resultName,omitempty"`

	// SkipIfMissingParam (data sources): skip this read when a path
	// parameter it needs is not set (optional detail reads).
	SkipIfMissingParam bool `json:"skipIfMissingParam,omitempty"`
	// SkipIfMissing (data sources): attribute names that must be non-null
	// for this read to run.
	SkipIfMissing []string `json:"skipIfMissing,omitempty"`

	// Filter describes a list-read: the response is an array and the item
	// whose fields match these attributes is the read result (e.g. ACLs).
	Filter []FilterRule `json:"filter,omitempty"`

	// Apply commits pending changes (node network): invoked after the
	// mutation with no parameters, only when the ApplyIf attribute is a
	// known true bool.
	Apply   *Op    `json:"apply,omitempty"`
	ApplyIf string `json:"applyIf,omitempty"`

	// IDFromUPID names the attribute whose value the engine takes from the
	// UPID's id component when the server allocated it (e.g. vmid on
	// qemu create without an explicit vmid).
	IDFromUPID string `json:"idFromUPID,omitempty"`

	// ResultAttr names a computed attribute that receives the raw string
	// response of the operation (e.g. the one-time API token value
	// returned by token create).
	ResultAttr string `json:"resultAttr,omitempty"`

	// ConstParams are fixed request parameters sent with every call of
	// this operation (e.g. the delete flag on PUT /access/acl).
	ConstParams map[string]string `json:"constParams,omitempty"`

	// DeleteParam: the endpoint accepts PVE's canonical `delete` parameter
	// (comma-separated option list); the engine appends the API names of
	// attributes removed from configuration on update.
	DeleteParam bool `json:"deleteParam,omitempty"`

	// ResultField extracts an object-response field into ResultAttr (e.g.
	// the regenerated token value from PUT /access/users/…/token/…).
	ResultField string `json:"resultField,omitempty"`
}

// FilterRule matches one response field against one attribute during a
// filtered list read.
type FilterRule struct {
	Field string `json:"field"` // response JSON field
	Attr  string `json:"attr"`  // attribute holding the expected value
}

// CloneSpec enables clone-at-create (qemu/lxc): when the source attribute is
// set at create time, the clone endpoint is used instead of the collection
// POST.
type CloneSpec struct {
	Op *Op `json:"op"`
	// SourceAttr holds the source guest id.
	SourceAttr string `json:"sourceAttr"`
	// SourceParam is the path placeholder of the clone endpoint the source
	// fills (e.g. "vmid").
	SourceParam string `json:"sourceParam"`
	// NewIDParam is the request parameter receiving the new guest id.
	NewIDParam string `json:"newIdParam"`
	// OnlyParams restricts which create-time attributes are forwarded to
	// the clone call (e.g. name, target, full); empty forwards all.
	OnlyParams []string `json:"onlyParams,omitempty"`
	// ConstParams are fixed clone request parameters sent with every clone
	// call (e.g. full=1 — PVE defaults to linked clones, which require a
	// template source).
	ConstParams map[string]string `json:"constParams,omitempty"`
}

// TypeDescriptor is one generated provider type.
type TypeDescriptor struct {
	Name string `json:"name"` // full type name, e.g. "pve_vm"
	Kind Kind   `json:"kind"`
	// Phase is the ROADMAP.md phase the type belongs to (docs + audit).
	Phase int `json:"phase"`
	// Summary is a one-line description used for schema and docs.
	Summary  string   `json:"summary,omitempty"`
	Behavior Behavior `json:"behavior,omitempty"`
	// Scoped: the type has a "scope" attribute (cluster|node|vm|container)
	// selecting among Op.Paths.
	Scoped bool `json:"scoped,omitempty"`
	// ScopePaths enumerates the allowed scope values (keys of Op.Paths).
	Scopes []string `json:"scopes,omitempty"`
	// IDAttrs names the attributes composing the resource ID, joined with
	// IDSep (also the terraform import format). Null values render as empty
	// components so positions stay stable.
	IDAttrs []string `json:"idAttrs,omitempty"`
	// IDSep joins the IDAttrs into the resource ID (default "/").
	IDSep string `json:"idSep,omitempty"`

	// Resource lifecycle (Kind=resource).
	Create *Op        `json:"create,omitempty"`
	Read   *Op        `json:"read,omitempty"`
	Update *Op        `json:"update,omitempty"`
	Delete *Op        `json:"delete,omitempty"`
	Clone  *CloneSpec `json:"clone,omitempty"`

	// Data source reads (Kind=data_source): each entry populates its
	// ResultName (or top level) from one endpoint.
	Reads []*Op `json:"reads,omitempty"`

	// Action invocation (Kind=action).
	Invoke *Op `json:"invoke,omitempty"`

	// Function call (Kind=function).
	Call       *Op      `json:"call,omitempty"`
	FuncArgs   []Attr   `json:"funcArgs,omitempty"`
	FuncResult AttrType `json:"funcResult,omitempty"`
	// FuncResultAttrs describes nested fields when FuncResult is an object.
	FuncResultAttrs []Attr `json:"funcResultAttrs,omitempty"`

	Attributes []Attr `json:"attributes"`
}

// Attr looks up an attribute by Terraform name.
func (t *TypeDescriptor) Attr(name string) (Attr, bool) {
	for _, a := range t.Attributes {
		if a.Name == name {
			return a, true
		}
	}
	return Attr{}, false
}

// resultAttrs indexes the one-time-result attributes of a type: values the
// server returns once and never echoes again, which read-backs must not
// null out.
func (t *TypeDescriptor) resultAttrs() map[string]bool {
	out := map[string]bool{}
	for _, op := range []*Op{t.Create, t.Update, t.Invoke} {
		if op != nil && op.ResultAttr != "" {
			out[op.ResultAttr] = true
		}
	}
	return out
}

// Registry is the generated registry document (registry.json).
type Registry struct {
	SchemaVersion int               `json:"schemaVersion"`
	Types         []*TypeDescriptor `json:"types"`
}
