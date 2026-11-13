// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// attrFrameworkType maps a descriptor attribute to its framework type.
func attrFrameworkType(a Attr) attr.Type {
	switch a.Type {
	case AttrString:
		return types.StringType
	case AttrBool:
		return types.BoolType
	case AttrInt64:
		return types.Int64Type
	case AttrFloat64:
		return types.Float64Type
	case AttrList:
		return types.ListType{ElemType: elemTypeOf(a)}
	case AttrSet:
		return types.SetType{ElemType: elemTypeOf(a)}
	case AttrObject:
		return types.ObjectType{AttrTypes: attrTypesOf(a.Attrs)}
	default:
		return types.StringType
	}
}

// elemTypeOf resolves the element type of a list/set attribute: object
// elements when ElemAttrs is set, otherwise the scalar ElemType.
func elemTypeOf(a Attr) attr.Type {
	if len(a.ElemAttrs) > 0 {
		return types.ObjectType{AttrTypes: attrTypesOf(a.ElemAttrs)}
	}
	return elemFrameworkType(a.ElemType)
}

func elemFrameworkType(t AttrType) attr.Type {
	switch t {
	case AttrBool:
		return types.BoolType
	case AttrInt64:
		return types.Int64Type
	case AttrFloat64:
		return types.Float64Type
	default:
		return types.StringType
	}
}

// attrTypesOf maps a slice of descriptor attributes to framework types.
func attrTypesOf(attrs []Attr) map[string]attr.Type {
	out := make(map[string]attr.Type, len(attrs))
	for _, a := range attrs {
		out[a.Name] = attrFrameworkType(a)
	}
	return out
}

// decodeJSON decodes a payload into generic Go values with numbers kept as
// json.Number so integer precision survives coercion.
func decodeJSON(raw json.RawMessage) (any, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	return v, nil
}

// coerceValue converts a decoded JSON value into the framework value for a.
// v may be nil (JSON null) — the result is a typed null.
func coerceValue(v any, a Attr) (attr.Value, error) {
	if v == nil {
		return nullValue(a), nil
	}
	switch a.Type {
	case AttrString:
		switch t := v.(type) {
		case string:
			return types.StringValue(t), nil
		case json.Number:
			return types.StringValue(t.String()), nil
		case bool:
			return types.StringValue(boolToString(t)), nil
		default:
			return nil, fmt.Errorf("cannot coerce %T to string for %q", v, a.Name)
		}
	case AttrBool:
		switch t := v.(type) {
		case bool:
			return types.BoolValue(t), nil
		case json.Number:
			b, err := strconv.ParseBool(t.String())
			if err != nil {
				f, ferr := t.Float64()
				if ferr != nil {
					return nil, fmt.Errorf("cannot coerce %v to bool for %q: %w", t.String(), a.Name, err)
				}
				b = f != 0
			}
			return types.BoolValue(b), nil
		case string:
			b, err := strconv.ParseBool(strings.TrimSpace(t))
			if err != nil {
				return nil, fmt.Errorf("cannot coerce %q to bool for %q: %w", t, a.Name, err)
			}
			return types.BoolValue(b), nil
		default:
			return nil, fmt.Errorf("cannot coerce %T to bool for %q", v, a.Name)
		}
	case AttrInt64:
		switch t := v.(type) {
		case json.Number:
			i, err := t.Int64()
			if err != nil {
				return nil, fmt.Errorf("cannot coerce %v to int64 for %q: %w", t.String(), a.Name, err)
			}
			return types.Int64Value(i), nil
		case string:
			i, err := strconv.ParseInt(strings.TrimSpace(t), 10, 64)
			if err != nil {
				return nil, fmt.Errorf("cannot coerce %q to int64 for %q: %w", t, a.Name, err)
			}
			return types.Int64Value(i), nil
		case bool:
			if t {
				return types.Int64Value(1), nil
			}
			return types.Int64Value(0), nil
		default:
			return nil, fmt.Errorf("cannot coerce %T to int64 for %q", v, a.Name)
		}
	case AttrFloat64:
		switch t := v.(type) {
		case json.Number:
			f, err := t.Float64()
			if err != nil {
				return nil, fmt.Errorf("cannot coerce %v to float for %q: %w", t.String(), a.Name, err)
			}
			return types.Float64Value(f), nil
		case string:
			f, err := strconv.ParseFloat(strings.TrimSpace(t), 64)
			if err != nil {
				return nil, fmt.Errorf("cannot coerce %q to float for %q: %w", t, a.Name, err)
			}
			return types.Float64Value(f), nil
		default:
			return nil, fmt.Errorf("cannot coerce %T to float for %q", v, a.Name)
		}
	case AttrList, AttrSet:
		items, ok := v.([]any)
		if !ok {
			switch t := v.(type) {
			case string:
				items = splitListString(t)
			case map[string]any:
				// Some apidoc-declared arrays are actually single-object
				// responses (pve-docgen emits array+links for structured
				// singletons); wrap the object as a one-element list.
				items = []any{t}
			default:
				return nil, fmt.Errorf("cannot coerce %T to list for %q", v, a.Name)
			}
		}
		var elemAttr Attr
		if len(a.ElemAttrs) > 0 {
			elemAttr = Attr{Name: a.Name, Type: AttrObject, Attrs: a.ElemAttrs}
		} else if a.ElemType != "" {
			elemAttr = Attr{Name: a.Name, Type: a.ElemType}
		} else {
			// Defensive: legacy descriptors without an element spec.
			elemAttr = Attr{Name: a.Name, Type: AttrString}
		}
		elems := make([]attr.Value, 0, len(items))
		for _, item := range items {
			ev, err := coerceValue(item, elemAttr)
			if err != nil {
				return nil, err
			}
			elems = append(elems, ev)
		}
		if a.Type == AttrSet {
			return types.SetValueMust(elemTypeOf(a), elems), nil
		}
		return types.ListValueMust(elemTypeOf(a), elems), nil
	case AttrObject:
		m, ok := v.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("cannot coerce %T to object for %q", v, a.Name)
		}
		fields := make(map[string]attr.Value, len(a.Attrs))
		for _, sub := range a.Attrs {
			fv, err := coerceValue(m[sub.TFName()], sub)
			if err != nil {
				return nil, err
			}
			fields[sub.Name] = fv
		}
		return types.ObjectValueMust(attrTypesOf(a.Attrs), fields), nil
	default:
		return nil, fmt.Errorf("unknown attribute type %q for %q", a.Type, a.Name)
	}
}

func nullValue(a Attr) attr.Value {
	switch a.Type {
	case AttrBool:
		return types.BoolNull()
	case AttrInt64:
		return types.Int64Null()
	case AttrFloat64:
		return types.Float64Null()
	case AttrList:
		return types.ListNull(elemTypeOf(a))
	case AttrSet:
		return types.SetNull(elemTypeOf(a))
	case AttrObject:
		return types.ObjectNull(attrTypesOf(a.Attrs))
	default:
		return types.StringNull()
	}
}

func boolToString(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// splitListString splits a comma-separated PVE list string. An empty string
// is an empty list.
func splitListString(s string) []any {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]any, len(parts))
	for i, p := range parts {
		out[i] = strings.TrimSpace(p)
	}
	return out
}

// usable reports whether a value carries a concrete (non-null, non-unknown)
// value. Framework plans mark unconfigured Optional+Computed attributes as
// unknown; rendering them would silently send zero values (vmid=0), and PVE
// PUT bodies treat empty strings as "delete this option".
func usable(v attr.Value) bool {
	return v != nil && !v.IsNull() && !v.IsUnknown()
}

// valueParamStrings renders an attribute value as PVE form parameter values:
// scalars become one string, lists/sets one string per element. Null and
// unknown values render as nothing.
func valueParamStrings(v attr.Value) ([]string, error) {
	if !usable(v) {
		return nil, nil
	}
	switch t := v.(type) {
	case types.String:
		return []string{t.ValueString()}, nil
	case types.Bool:
		return []string{boolToString(t.ValueBool())}, nil
	case types.Int64:
		return []string{strconv.FormatInt(t.ValueInt64(), 10)}, nil
	case types.Float64:
		return []string{strconv.FormatFloat(t.ValueFloat64(), 'f', -1, 64)}, nil
	case types.List:
		elems := t.Elements()
		out := make([]string, 0, len(elems))
		for _, e := range elems {
			ss, err := valueParamStrings(e)
			if err != nil {
				return nil, err
			}
			out = append(out, ss...)
		}
		return out, nil
	case types.Set:
		elems := t.Elements()
		out := make([]string, 0, len(elems))
		for _, e := range elems {
			ss, err := valueParamStrings(e)
			if err != nil {
				return nil, err
			}
			out = append(out, ss...)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("cannot send %T as request parameter", v)
	}
}
