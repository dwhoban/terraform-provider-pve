// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"math"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/attr"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	actionschema "github.com/hashicorp/terraform-plugin-framework/action/schema"
	dschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/float64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/float64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// The framework ships structurally identical (but distinct) schema packages
// for resources, data sources, and actions. The Attr→attrSpec projection
// below decides the shape once; each renderer then only maps spec kinds to
// its own package's constructors and applies its flavor's capabilities
// (resources take plan modifiers and defaults; actions carry inputs only).
// Only resources take plan modifiers (ForceNew) — data sources and actions
// have no plans.

// stringValidators builds the enum validator for a string attribute.
func stringValidators(enum []string) []validator.String {
	if len(enum) > 0 {
		return []validator.String{stringvalidator.OneOf(enum...)}
	}
	return nil
}

// defaultString renders an Attr.Default (decoded JSON value) as its literal
// for docs and descriptions; "" when unset.
func defaultString(a Attr) string {
	switch v := a.Default.(type) {
	case nil:
		return ""
	case string:
		return v
	case bool:
		return boolToString(v)
	case float64:
		if v == math.Trunc(v) {
			return strconv.FormatInt(int64(v), 10)
		}
		return strconv.FormatFloat(v, 'f', -1, 64)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// numericDefault extracts a numeric default value (decoded JSON numbers
// are float64; hand-written mappings may use ints).
func numericDefault(def any) (float64, bool) {
	switch v := def.(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	default:
		return 0, false
	}
}

func attrDescription(a Attr) string {
	desc := a.Description
	if dv := defaultString(a); dv != "" {
		desc += " (default: " + dv + ")"
	}
	if len(a.Enum) > 0 {
		desc += " One of: " + joinQuoted(a.Enum) + "."
	}
	return desc
}

func joinQuoted(ss []string) string {
	out := ""
	for i, s := range ss {
		if i > 0 {
			out += ", "
		}
		out += "`" + s + "`"
	}
	return out
}

func joinSlash(ss []string) string {
	out := ""
	for i, s := range ss {
		if i > 0 {
			out += "/"
		}
		out += s
	}
	return out
}

// buildResourceSchema translates a resource descriptor into a framework
// resource schema, adding the mandatory "id" attribute.
func buildResourceSchema(d *TypeDescriptor) schema.Schema {
	attrs := buildResourceAttrs(d.Attributes)
	attrs["id"] = schema.StringAttribute{
		Computed:            true,
		MarkdownDescription: fmt.Sprintf("Opaque identifier: %s (also the `terraform import` format).", joinSlash(d.IDAttrs)),
		PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
	}
	return schema.Schema{MarkdownDescription: d.Summary, Attributes: attrs, Blocks: buildResourceBlocks(d.Attributes)}
}

// buildResourceBlocks renders slot-family block attributes (Attr.Block) as
// nested blocks: a list block per family, a single block when Fixed.
func buildResourceBlocks(list []Attr) map[string]schema.Block {
	out := map[string]schema.Block{}
	for _, a := range list {
		if a.Block == nil {
			continue
		}
		desc := attrDescription(a)
		keep := blockKeepComputed{a: a}
		if a.Block.Fixed {
			out[a.Name] = schema.SingleNestedBlock{
				Attributes:          buildResourceAttrs(a.Attrs),
				MarkdownDescription: desc,
				PlanModifiers:       []planmodifier.Object{keep},
			}
			continue
		}
		out[a.Name] = schema.ListNestedBlock{
			NestedObject:        schema.NestedBlockObject{Attributes: buildResourceAttrs(a.ElemAttrs)},
			MarkdownDescription: desc,
			PlanModifiers:       []planmodifier.List{keep},
		}
	}
	return out
}

// blockKeepComputed carries server-computed block fields (volume ids, MAC
// addresses, sizes) from state into plan elements that keep their slot.
// It matches elements by wire key, not list position, so a reordered disk
// never inherits another disk's volume.
type blockKeepComputed struct{ a Attr }

var (
	_ planmodifier.List   = blockKeepComputed{}
	_ planmodifier.Object = blockKeepComputed{}
)

func (m blockKeepComputed) Description(context.Context) string {
	return "Keeps server-computed fields of block elements that keep their slot."
}

func (m blockKeepComputed) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m blockKeepComputed) PlanModifyList(_ context.Context, req planmodifier.ListRequest, resp *planmodifier.ListResponse) {
	if v, ok := keepBlockComputed(m.a, req.PlanValue, req.StateValue).(types.List); ok {
		resp.PlanValue = v
	}
}

func (m blockKeepComputed) PlanModifyObject(_ context.Context, req planmodifier.ObjectRequest, resp *planmodifier.ObjectResponse) {
	if v, ok := keepBlockComputed(m.a, req.PlanValue, req.StateValue).(types.Object); ok {
		resp.PlanValue = v
	}
}

// keepBlockComputed fills null or unknown computed fields of plan elements
// from the state element with the same wire key; plan is returned unchanged
// when it or the state is not concrete.
func keepBlockComputed(a Attr, plan, state attr.Value) attr.Value {
	if !usable(plan) || !usable(state) {
		return plan
	}
	planElems := blockElems(plan)
	if l, ok := plan.(types.List); ok && len(l.Elements()) != len(planElems) {
		return plan // an unknown element: nothing to match yet
	}
	existing := map[string]map[string]attr.Value{}
	for pos, e := range blockElems(state) {
		if key, err := blockElemKey(a.Block, e, pos); err == nil {
			existing[key] = e
		}
	}
	out := make([]map[string]attr.Value, len(planElems))
	for pos, pe := range planElems {
		out[pos] = pe
		key, err := blockElemKey(a.Block, pe, pos)
		se, ok := existing[key]
		if err != nil || !ok {
			continue
		}
		out[pos] = fillComputed(a, pe, se)
	}
	return blockValue(a, out)
}

// specKind is the neutral attribute shape shared by every schema flavor.
type specKind int

const (
	specString specKind = iota
	specBool
	specInt64
	specFloat64
	specList
	specSet
	specObject
)

// attrSpec is the neutral projection of an Attr: the one place the
// per-type matrix (kind, nesting, flags, defaults) is decided. Each
// schema flavor renders specs into its own framework constructors.
type attrSpec struct {
	Name, Desc                              string
	Kind                                    specKind
	Required, Optional, Computed, Sensitive bool
	Enum                                    []string
	Default                                 any
	ForceNew                                bool
	ElemType                                AttrType // list/set scalar elements
	Nested                                  []attrSpec
	Block                                   *BlockSpec
}

// attrSpecsOf projects a descriptor attribute list.
func attrSpecsOf(list []Attr) []attrSpec {
	out := make([]attrSpec, len(list))
	for i, a := range list {
		out[i] = attrSpecOf(a)
	}
	return out
}

// attrSpecOf projects one descriptor attribute, recursing into nested
// object and list/set element attributes.
func attrSpecOf(a Attr) attrSpec {
	s := attrSpec{
		Name: a.Name, Desc: attrDescription(a),
		Required: a.Required, Optional: a.Optional, Computed: a.Computed, Sensitive: a.Sensitive,
		Enum: a.Enum, Default: a.Default, ForceNew: a.ForceNew,
		ElemType: a.ElemType, Block: a.Block,
	}
	switch a.Type {
	case AttrBool:
		s.Kind = specBool
	case AttrInt64:
		s.Kind = specInt64
	case AttrFloat64:
		s.Kind = specFloat64
	case AttrObject:
		s.Kind = specObject
		s.Nested = attrSpecsOf(a.Attrs)
	case AttrList, AttrSet:
		if a.Type == AttrList {
			s.Kind = specList
		} else {
			s.Kind = specSet
		}
		if len(a.ElemAttrs) > 0 {
			s.Nested = attrSpecsOf(a.ElemAttrs)
		}
	default:
		s.Kind = specString
	}
	return s
}

func buildResourceAttrs(list []Attr) map[string]schema.Attribute {
	return renderResourceAttrs(attrSpecsOf(list))
}

// renderResourceAttrs renders neutral specs as resource attributes: the
// full capability set — defaults (Optional+Computed), enum validators, and
// ForceNew plan modifiers.
func renderResourceAttrs(specs []attrSpec) map[string]schema.Attribute {
	out := make(map[string]schema.Attribute, len(specs))
	for _, spec := range specs {
		if spec.Block != nil {
			continue // rendered by buildResourceBlocks
		}
		desc := spec.Desc
		switch spec.Kind {
		case specBool:
			attr := schema.BoolAttribute{
				Required: spec.Required, Optional: spec.Optional, Computed: spec.Computed,
				Sensitive: spec.Sensitive, MarkdownDescription: desc,
			}
			if b, ok := spec.Default.(bool); ok && spec.Optional && spec.Computed {
				attr.Default = booldefault.StaticBool(b)
			}
			if spec.ForceNew {
				attr.PlanModifiers = []planmodifier.Bool{boolplanmodifier.RequiresReplace()}
			}
			out[spec.Name] = attr
		case specInt64:
			attr := schema.Int64Attribute{
				Required: spec.Required, Optional: spec.Optional, Computed: spec.Computed,
				Sensitive: spec.Sensitive, MarkdownDescription: desc,
			}
			if n, ok := numericDefault(spec.Default); ok && spec.Optional && spec.Computed {
				attr.Default = int64default.StaticInt64(int64(n))
			}
			if spec.ForceNew {
				attr.PlanModifiers = []planmodifier.Int64{int64planmodifier.RequiresReplace()}
			}
			out[spec.Name] = attr
		case specFloat64:
			attr := schema.Float64Attribute{
				Required: spec.Required, Optional: spec.Optional, Computed: spec.Computed,
				Sensitive: spec.Sensitive, MarkdownDescription: desc,
			}
			if n, ok := numericDefault(spec.Default); ok && spec.Optional && spec.Computed {
				attr.Default = float64default.StaticFloat64(n)
			}
			if spec.ForceNew {
				attr.PlanModifiers = []planmodifier.Float64{float64planmodifier.RequiresReplace()}
			}
			out[spec.Name] = attr
		case specObject:
			attr := schema.SingleNestedAttribute{
				Attributes: renderResourceAttrs(spec.Nested),
				Required:   spec.Required, Optional: spec.Optional, Computed: spec.Computed,
				Sensitive: spec.Sensitive, MarkdownDescription: desc,
			}
			if spec.ForceNew {
				attr.PlanModifiers = []planmodifier.Object{objectplanmodifier.RequiresReplace()}
			}
			out[spec.Name] = attr
		case specList, specSet:
			if len(spec.Nested) > 0 {
				obj := schema.NestedAttributeObject{Attributes: renderResourceAttrs(spec.Nested)}
				if spec.Kind == specList {
					attr := schema.ListNestedAttribute{
						NestedObject: obj, Required: spec.Required, Optional: spec.Optional, Computed: spec.Computed,
						Sensitive: spec.Sensitive, MarkdownDescription: desc,
					}
					if spec.ForceNew {
						attr.PlanModifiers = []planmodifier.List{listplanmodifier.RequiresReplace()}
					}
					out[spec.Name] = attr
				} else {
					attr := schema.SetNestedAttribute{
						NestedObject: obj, Required: spec.Required, Optional: spec.Optional, Computed: spec.Computed,
						Sensitive: spec.Sensitive, MarkdownDescription: desc,
					}
					if spec.ForceNew {
						attr.PlanModifiers = []planmodifier.Set{setplanmodifier.RequiresReplace()}
					}
					out[spec.Name] = attr
				}
				continue
			}
			if spec.Kind == specList {
				attr := schema.ListAttribute{
					ElementType: elemFrameworkType(spec.ElemType), Required: spec.Required, Optional: spec.Optional, Computed: spec.Computed,
					Sensitive: spec.Sensitive, MarkdownDescription: desc,
				}
				if spec.ForceNew {
					attr.PlanModifiers = []planmodifier.List{listplanmodifier.RequiresReplace()}
				}
				out[spec.Name] = attr
			} else {
				attr := schema.SetAttribute{
					ElementType: elemFrameworkType(spec.ElemType), Required: spec.Required, Optional: spec.Optional, Computed: spec.Computed,
					Sensitive: spec.Sensitive, MarkdownDescription: desc,
				}
				if spec.ForceNew {
					attr.PlanModifiers = []planmodifier.Set{setplanmodifier.RequiresReplace()}
				}
				out[spec.Name] = attr
			}
		default: // specString
			attr := schema.StringAttribute{
				Required: spec.Required, Optional: spec.Optional, Computed: spec.Computed,
				Sensitive: spec.Sensitive, MarkdownDescription: desc,
				Validators: stringValidators(spec.Enum),
			}
			if s, ok := spec.Default.(string); ok && spec.Optional && spec.Computed {
				attr.Default = stringdefault.StaticString(s)
			}
			if spec.ForceNew {
				attr.PlanModifiers = []planmodifier.String{stringplanmodifier.RequiresReplace()}
			}
			out[spec.Name] = attr
		}
	}
	return out
}

// buildDataSourceSchema translates a data source descriptor into a framework
// data source schema.
func buildDataSourceSchema(d *TypeDescriptor) dschema.Schema {
	attrs := buildDataSourceAttrs(d.Attributes)
	if _, ok := attrs["id"]; !ok {
		attrs["id"] = dschema.StringAttribute{
			Computed:            true,
			MarkdownDescription: fmt.Sprintf("Identifier: %s.", idExample(d)),
		}
	}
	return dschema.Schema{
		MarkdownDescription: d.Summary,
		Attributes:          attrs,
	}
}

func buildDataSourceAttrs(list []Attr) map[string]dschema.Attribute {
	return renderDataSourceAttrs(attrSpecsOf(list))
}

// renderDataSourceAttrs renders neutral specs as data source attributes:
// a read-only surface — no defaults, no plan modifiers. Blocks are not
// rendered: no data source declares one today (if the mapping grows one,
// wire it here deliberately).
func renderDataSourceAttrs(specs []attrSpec) map[string]dschema.Attribute {
	out := make(map[string]dschema.Attribute, len(specs))
	for _, spec := range specs {
		if spec.Block != nil {
			continue // no data source declares blocks today
		}
		desc := spec.Desc
		switch spec.Kind {
		case specBool:
			out[spec.Name] = dschema.BoolAttribute{
				Required: spec.Required, Optional: spec.Optional, Computed: spec.Computed,
				Sensitive: spec.Sensitive, MarkdownDescription: desc,
			}
		case specInt64:
			out[spec.Name] = dschema.Int64Attribute{
				Required: spec.Required, Optional: spec.Optional, Computed: spec.Computed,
				Sensitive: spec.Sensitive, MarkdownDescription: desc,
			}
		case specFloat64:
			out[spec.Name] = dschema.Float64Attribute{
				Required: spec.Required, Optional: spec.Optional, Computed: spec.Computed,
				Sensitive: spec.Sensitive, MarkdownDescription: desc,
			}
		case specObject:
			out[spec.Name] = dschema.SingleNestedAttribute{
				Attributes: renderDataSourceAttrs(spec.Nested),
				Required:   spec.Required, Optional: spec.Optional, Computed: spec.Computed,
				Sensitive: spec.Sensitive, MarkdownDescription: desc,
			}
		case specList, specSet:
			if len(spec.Nested) > 0 {
				obj := dschema.NestedAttributeObject{Attributes: renderDataSourceAttrs(spec.Nested)}
				if spec.Kind == specList {
					out[spec.Name] = dschema.ListNestedAttribute{
						NestedObject: obj, Required: spec.Required, Optional: spec.Optional, Computed: spec.Computed,
						Sensitive: spec.Sensitive, MarkdownDescription: desc,
					}
				} else {
					out[spec.Name] = dschema.SetNestedAttribute{
						NestedObject: obj, Required: spec.Required, Optional: spec.Optional, Computed: spec.Computed,
						Sensitive: spec.Sensitive, MarkdownDescription: desc,
					}
				}
				continue
			}
			if spec.Kind == specList {
				out[spec.Name] = dschema.ListAttribute{
					ElementType: elemFrameworkType(spec.ElemType), Required: spec.Required, Optional: spec.Optional, Computed: spec.Computed,
					Sensitive: spec.Sensitive, MarkdownDescription: desc,
				}
			} else {
				out[spec.Name] = dschema.SetAttribute{
					ElementType: elemFrameworkType(spec.ElemType), Required: spec.Required, Optional: spec.Optional, Computed: spec.Computed,
					Sensitive: spec.Sensitive, MarkdownDescription: desc,
				}
			}
		default: // specString
			out[spec.Name] = dschema.StringAttribute{
				Required: spec.Required, Optional: spec.Optional, Computed: spec.Computed,
				Sensitive: spec.Sensitive, MarkdownDescription: desc,
				Validators: stringValidators(spec.Enum),
			}
		}
	}
	return out
}

// buildActionSchema translates an action descriptor into a framework action
// schema. Actions are stateless invocations: attributes are inputs only.
func buildActionSchema(d *TypeDescriptor) actionschema.Schema {
	// Actions are stateless invocations: attributes are inputs only (no
	// Computed/Sensitive), and results surface via progress events and
	// diagnostics rather than state.
	return actionschema.Schema{
		MarkdownDescription: d.Summary,
		Attributes:          buildActionAttrs(d.Attributes),
		Blocks:              buildActionBlocks(d.Attributes),
	}
}

// buildActionBlocks renders slot-family block attributes as nested blocks.
func buildActionBlocks(list []Attr) map[string]actionschema.Block {
	out := map[string]actionschema.Block{}
	for _, a := range list {
		if a.Block == nil {
			continue
		}
		desc := attrDescription(a)
		if a.Block.Fixed {
			out[a.Name] = actionschema.SingleNestedBlock{Attributes: buildActionAttrs(a.Attrs), MarkdownDescription: desc}
			continue
		}
		out[a.Name] = actionschema.ListNestedBlock{
			NestedObject:        actionschema.NestedBlockObject{Attributes: buildActionAttrs(a.ElemAttrs)},
			MarkdownDescription: desc,
		}
	}
	return out
}

func buildActionAttrs(list []Attr) map[string]actionschema.Attribute {
	return renderActionAttrs(attrSpecsOf(list))
}

// renderActionAttrs renders neutral specs as action attributes: inputs
// only (no Computed/Sensitive state) — results surface via progress
// events and diagnostics.
func renderActionAttrs(specs []attrSpec) map[string]actionschema.Attribute {
	out := make(map[string]actionschema.Attribute, len(specs))
	for _, spec := range specs {
		if spec.Block != nil {
			continue // rendered by buildActionBlocks
		}
		desc := spec.Desc
		switch spec.Kind {
		case specBool:
			out[spec.Name] = actionschema.BoolAttribute{
				Required: spec.Required, Optional: spec.Optional,
				MarkdownDescription: desc,
			}
		case specInt64:
			out[spec.Name] = actionschema.Int64Attribute{
				Required: spec.Required, Optional: spec.Optional,
				MarkdownDescription: desc,
			}
		case specFloat64:
			out[spec.Name] = actionschema.Float64Attribute{
				Required: spec.Required, Optional: spec.Optional,
				MarkdownDescription: desc,
			}
		case specObject:
			out[spec.Name] = actionschema.SingleNestedAttribute{
				Attributes: renderActionAttrs(spec.Nested),
				Required:   spec.Required, Optional: spec.Optional,
				MarkdownDescription: desc,
			}
		case specList, specSet:
			if len(spec.Nested) > 0 {
				obj := actionschema.NestedAttributeObject{Attributes: renderActionAttrs(spec.Nested)}
				if spec.Kind == specList {
					out[spec.Name] = actionschema.ListNestedAttribute{
						NestedObject: obj, Required: spec.Required, Optional: spec.Optional,
						MarkdownDescription: desc,
					}
				} else {
					out[spec.Name] = actionschema.SetNestedAttribute{
						NestedObject: obj, Required: spec.Required, Optional: spec.Optional,
						MarkdownDescription: desc,
					}
				}
				continue
			}
			if spec.Kind == specList {
				out[spec.Name] = actionschema.ListAttribute{
					ElementType: elemFrameworkType(spec.ElemType), Required: spec.Required, Optional: spec.Optional,
					MarkdownDescription: desc,
				}
			} else {
				out[spec.Name] = actionschema.SetAttribute{
					ElementType: elemFrameworkType(spec.ElemType), Required: spec.Required, Optional: spec.Optional,
					MarkdownDescription: desc,
				}
			}
		default: // specString
			out[spec.Name] = actionschema.StringAttribute{
				Required: spec.Required, Optional: spec.Optional,
				MarkdownDescription: desc,
				Validators:          stringValidators(spec.Enum),
			}
		}
	}
	return out
}
