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
// for resources, data sources, and actions. The builders below translate the
// same Attr tree into each package's types; keep them in sync. Only resources
// take plan modifiers (ForceNew) — data sources and actions have no plans.

func stringValidators(a Attr) []validator.String {
	if len(a.Enum) > 0 {
		return []validator.String{stringvalidator.OneOf(a.Enum...)}
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

func numericDefault(a Attr) (float64, bool) {
	switch v := a.Default.(type) {
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

// keepBlockComputed fills unknown computed fields of plan elements from the
// state element with the same wire key; plan is returned unchanged when
// it or the state is not concrete.
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

func buildResourceAttrs(list []Attr) map[string]schema.Attribute {
	out := make(map[string]schema.Attribute, len(list))
	for _, a := range list {
		if a.Block != nil {
			continue // rendered by buildResourceBlocks
		}
		desc := attrDescription(a)
		switch {
		case a.Type == AttrString:
			attr := schema.StringAttribute{
				Required: a.Required, Optional: a.Optional, Computed: a.Computed,
				Sensitive: a.Sensitive, MarkdownDescription: desc,
				Validators: stringValidators(a),
			}
			if s, ok := a.Default.(string); ok && a.Optional && a.Computed {
				attr.Default = stringdefault.StaticString(s)
			}
			if a.ForceNew {
				attr.PlanModifiers = []planmodifier.String{stringplanmodifier.RequiresReplace()}
			}
			out[a.Name] = attr
		case a.Type == AttrBool:
			attr := schema.BoolAttribute{
				Required: a.Required, Optional: a.Optional, Computed: a.Computed,
				Sensitive: a.Sensitive, MarkdownDescription: desc,
			}
			if b, ok := a.Default.(bool); ok && a.Optional && a.Computed {
				attr.Default = booldefault.StaticBool(b)
			}
			if a.ForceNew {
				attr.PlanModifiers = []planmodifier.Bool{boolplanmodifier.RequiresReplace()}
			}
			out[a.Name] = attr
		case a.Type == AttrInt64:
			attr := schema.Int64Attribute{
				Required: a.Required, Optional: a.Optional, Computed: a.Computed,
				Sensitive: a.Sensitive, MarkdownDescription: desc,
			}
			if n, ok := numericDefault(a); ok && a.Optional && a.Computed {
				attr.Default = int64default.StaticInt64(int64(n))
			}
			if a.ForceNew {
				attr.PlanModifiers = []planmodifier.Int64{int64planmodifier.RequiresReplace()}
			}
			out[a.Name] = attr
		case a.Type == AttrFloat64:
			attr := schema.Float64Attribute{
				Required: a.Required, Optional: a.Optional, Computed: a.Computed,
				Sensitive: a.Sensitive, MarkdownDescription: desc,
			}
			if n, ok := numericDefault(a); ok && a.Optional && a.Computed {
				attr.Default = float64default.StaticFloat64(n)
			}
			if a.ForceNew {
				attr.PlanModifiers = []planmodifier.Float64{float64planmodifier.RequiresReplace()}
			}
			out[a.Name] = attr
		case a.Type == AttrObject:
			attr := schema.SingleNestedAttribute{
				Attributes: buildResourceAttrs(a.Attrs),
				Required:   a.Required, Optional: a.Optional, Computed: a.Computed,
				Sensitive: a.Sensitive, MarkdownDescription: desc,
			}
			if a.ForceNew {
				attr.PlanModifiers = []planmodifier.Object{objectplanmodifier.RequiresReplace()}
			}
			out[a.Name] = attr
		case (a.Type == AttrList || a.Type == AttrSet) && len(a.ElemAttrs) > 0:
			obj := schema.NestedAttributeObject{Attributes: buildResourceAttrs(a.ElemAttrs)}
			if a.Type == AttrList {
				attr := schema.ListNestedAttribute{
					NestedObject: obj, Required: a.Required, Optional: a.Optional, Computed: a.Computed,
					Sensitive: a.Sensitive, MarkdownDescription: desc,
				}
				if a.ForceNew {
					attr.PlanModifiers = []planmodifier.List{listplanmodifier.RequiresReplace()}
				}
				out[a.Name] = attr
			} else {
				attr := schema.SetNestedAttribute{
					NestedObject: obj, Required: a.Required, Optional: a.Optional, Computed: a.Computed,
					Sensitive: a.Sensitive, MarkdownDescription: desc,
				}
				if a.ForceNew {
					attr.PlanModifiers = []planmodifier.Set{setplanmodifier.RequiresReplace()}
				}
				out[a.Name] = attr
			}
		case a.Type == AttrList:
			attr := schema.ListAttribute{
				ElementType: elemTypeOf(a), Required: a.Required, Optional: a.Optional, Computed: a.Computed,
				Sensitive: a.Sensitive, MarkdownDescription: desc,
			}
			if a.ForceNew {
				attr.PlanModifiers = []planmodifier.List{listplanmodifier.RequiresReplace()}
			}
			out[a.Name] = attr
		case a.Type == AttrSet:
			attr := schema.SetAttribute{
				ElementType: elemTypeOf(a), Required: a.Required, Optional: a.Optional, Computed: a.Computed,
				Sensitive: a.Sensitive, MarkdownDescription: desc,
			}
			if a.ForceNew {
				attr.PlanModifiers = []planmodifier.Set{setplanmodifier.RequiresReplace()}
			}
			out[a.Name] = attr
		default:
			out[a.Name] = schema.StringAttribute{
				Required: a.Required, Optional: a.Optional, Computed: a.Computed,
				Sensitive: a.Sensitive, MarkdownDescription: desc,
			}
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
	out := make(map[string]dschema.Attribute, len(list))
	for _, a := range list {
		desc := attrDescription(a)
		switch {
		case a.Type == AttrString:
			out[a.Name] = dschema.StringAttribute{
				Required: a.Required, Optional: a.Optional, Computed: a.Computed,
				Sensitive: a.Sensitive, MarkdownDescription: desc,
				Validators: stringValidators(a),
			}
		case a.Type == AttrBool:
			out[a.Name] = dschema.BoolAttribute{
				Required: a.Required, Optional: a.Optional, Computed: a.Computed,
				Sensitive: a.Sensitive, MarkdownDescription: desc,
			}
		case a.Type == AttrInt64:
			out[a.Name] = dschema.Int64Attribute{
				Required: a.Required, Optional: a.Optional, Computed: a.Computed,
				Sensitive: a.Sensitive, MarkdownDescription: desc,
			}
		case a.Type == AttrFloat64:
			out[a.Name] = dschema.Float64Attribute{
				Required: a.Required, Optional: a.Optional, Computed: a.Computed,
				Sensitive: a.Sensitive, MarkdownDescription: desc,
			}
		case a.Type == AttrObject:
			out[a.Name] = dschema.SingleNestedAttribute{
				Attributes: buildDataSourceAttrs(a.Attrs),
				Required:   a.Required, Optional: a.Optional, Computed: a.Computed,
				Sensitive: a.Sensitive, MarkdownDescription: desc,
			}
		case (a.Type == AttrList || a.Type == AttrSet) && len(a.ElemAttrs) > 0:
			obj := dschema.NestedAttributeObject{Attributes: buildDataSourceAttrs(a.ElemAttrs)}
			if a.Type == AttrList {
				out[a.Name] = dschema.ListNestedAttribute{
					NestedObject: obj, Required: a.Required, Optional: a.Optional, Computed: a.Computed,
					Sensitive: a.Sensitive, MarkdownDescription: desc,
				}
			} else {
				out[a.Name] = dschema.SetNestedAttribute{
					NestedObject: obj, Required: a.Required, Optional: a.Optional, Computed: a.Computed,
					Sensitive: a.Sensitive, MarkdownDescription: desc,
				}
			}
		case a.Type == AttrList:
			out[a.Name] = dschema.ListAttribute{
				ElementType: elemTypeOf(a), Required: a.Required, Optional: a.Optional, Computed: a.Computed,
				Sensitive: a.Sensitive, MarkdownDescription: desc,
			}
		case a.Type == AttrSet:
			out[a.Name] = dschema.SetAttribute{
				ElementType: elemTypeOf(a), Required: a.Required, Optional: a.Optional, Computed: a.Computed,
				Sensitive: a.Sensitive, MarkdownDescription: desc,
			}
		default:
			out[a.Name] = dschema.StringAttribute{
				Required: a.Required, Optional: a.Optional, Computed: a.Computed,
				Sensitive: a.Sensitive, MarkdownDescription: desc,
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
	out := make(map[string]actionschema.Attribute, len(list))
	for _, a := range list {
		if a.Block != nil {
			continue // rendered by buildActionBlocks
		}
		desc := attrDescription(a)
		switch {
		case a.Type == AttrString:
			out[a.Name] = actionschema.StringAttribute{
				Required: a.Required, Optional: a.Optional,
				MarkdownDescription: desc,
				Validators:          stringValidators(a),
			}
		case a.Type == AttrBool:
			out[a.Name] = actionschema.BoolAttribute{
				Required: a.Required, Optional: a.Optional,
				MarkdownDescription: desc,
			}
		case a.Type == AttrInt64:
			out[a.Name] = actionschema.Int64Attribute{
				Required: a.Required, Optional: a.Optional,
				MarkdownDescription: desc,
			}
		case a.Type == AttrFloat64:
			out[a.Name] = actionschema.Float64Attribute{
				Required: a.Required, Optional: a.Optional,
				MarkdownDescription: desc,
			}
		case a.Type == AttrObject:
			out[a.Name] = actionschema.SingleNestedAttribute{
				Attributes: buildActionAttrs(a.Attrs),
				Required:   a.Required, Optional: a.Optional,
				MarkdownDescription: desc,
			}
		case (a.Type == AttrList || a.Type == AttrSet) && len(a.ElemAttrs) > 0:
			obj := actionschema.NestedAttributeObject{Attributes: buildActionAttrs(a.ElemAttrs)}
			if a.Type == AttrList {
				out[a.Name] = actionschema.ListNestedAttribute{
					NestedObject: obj, Required: a.Required, Optional: a.Optional,
					MarkdownDescription: desc,
				}
			} else {
				out[a.Name] = actionschema.SetNestedAttribute{
					NestedObject: obj, Required: a.Required, Optional: a.Optional,
					MarkdownDescription: desc,
				}
			}
		case a.Type == AttrList:
			out[a.Name] = actionschema.ListAttribute{
				ElementType: elemTypeOf(a), Required: a.Required, Optional: a.Optional,
				MarkdownDescription: desc,
			}
		case a.Type == AttrSet:
			out[a.Name] = actionschema.SetAttribute{
				ElementType: elemTypeOf(a), Required: a.Required, Optional: a.Optional,
				MarkdownDescription: desc,
			}
		default:
			out[a.Name] = actionschema.StringAttribute{
				Required: a.Required, Optional: a.Optional,
				MarkdownDescription: desc,
			}
		}
	}
	return out
}
