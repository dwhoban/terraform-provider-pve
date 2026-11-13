// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"math"
	"strconv"

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
	return schema.Schema{MarkdownDescription: d.Summary, Attributes: attrs}
}

func buildResourceAttrs(list []Attr) map[string]schema.Attribute {
	out := make(map[string]schema.Attribute, len(list))
	for _, a := range list {
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
	}
}

func buildActionAttrs(list []Attr) map[string]actionschema.Attribute {
	out := make(map[string]actionschema.Attribute, len(list))
	for _, a := range list {
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
