// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"encoding/base64"
	"encoding/json"
	"sort"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// This file owns the translation between PVE wire values and Terraform
// values for one plain attribute: decoding responses, encoding request
// parameters, and wire-level equality. Slot-family blocks live in
// block_codec.go; engines only orchestrate.

// decodeAttr converts a decoded JSON response value into the framework
// value for a. PVE answers some string properties (e.g. cluster options
// `migration`) as JSON objects; those render as a property string
// (`key=value,...`, keys sorted).
func decodeAttr(a Attr, v any) (attr.Value, error) {
	if a.Format == FormatBootOrder {
		return decodeBootOrder(v), nil
	}
	if m, isObj := v.(map[string]any); isObj && a.Type == AttrString {
		return types.StringValue(renderPropertyString(m)), nil
	}
	return coerceValue(v, a)
}

// encodeAttr renders an attribute value as PVE request parameter values,
// applying the attribute's wire format. Null and unknown values render as
// nothing.
func encodeAttr(a Attr, v attr.Value) ([]string, error) {
	ss, err := valueParamStrings(v)
	if err != nil {
		return nil, err
	}
	switch a.Format {
	case FormatBase64:
		for i, s := range ss {
			ss[i] = base64.StdEncoding.EncodeToString([]byte(s))
		}
	case FormatBootOrder:
		if len(ss) == 0 {
			return nil, nil
		}
		return []string{"order=" + strings.Join(ss, ";")}, nil
	}
	return ss, nil
}

// decodeBootOrder reads PVE's boot property: `order=a;b` becomes [a, b];
// legacy forms (`cdn`, `legacy=…`) carry no device order and read as null.
func decodeBootOrder(v any) attr.Value {
	s, isStr := v.(string)
	order, hasOrder := strings.CutPrefix(s, "order=")
	if !isStr || !hasOrder || order == "" {
		return types.ListNull(types.StringType)
	}
	parts := strings.Split(order, ";")
	elems := make([]attr.Value, len(parts))
	for i, p := range parts {
		elems[i] = types.StringValue(p)
	}
	return types.ListValueMust(types.StringType, elems)
}

// wireEqual reports whether the current string value already describes the
// object-valued response field as a property string, so the configured form
// (key order, spelling) is kept. Plain values compare through decodeAttr.
func wireEqual(a Attr, cur attr.Value, field any) bool {
	if a.Type != AttrString || !usable(cur) {
		return false
	}
	s, isStr := cur.(types.String)
	if !isStr {
		return false
	}
	m, isObj := field.(map[string]any)
	if !isObj {
		return false
	}
	return propertyStringEqual(s.ValueString(), m)
}

// decodeResultString renders a raw response payload for a string result
// attribute: a bare JSON string unquoted, anything else (object, array) as
// its JSON text.
func decodeResultString(payload json.RawMessage) string {
	if s, ok := payloadString(payload); ok {
		return s
	}
	return string(payload)
}

// payloadString reports the payload as a bare JSON string, if it is one.
func payloadString(payload json.RawMessage) (string, bool) {
	if len(payload) == 0 {
		return "", false
	}
	var s string
	if err := json.Unmarshal(payload, &s); err != nil {
		return "", false
	}
	return s, true
}

// renderPropertyString renders an object as a PVE property string:
// `key=value` pairs joined by `,`, keys sorted, null values skipped.
func renderPropertyString(m map[string]any) string {
	keys := make([]string, 0, len(m))
	for k, v := range m {
		if v != nil {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(renderPropertyValue(m[k]))
	}
	return b.String()
}

func renderPropertyValue(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case json.Number:
		return t.String()
	case bool:
		return boolToString(t)
	default:
		raw, err := json.Marshal(t)
		if err != nil {
			return ""
		}
		return string(raw)
	}
}

// propertyStringEqual reports whether property string s carries exactly the
// non-null fields of m, in any order.
func propertyStringEqual(s string, m map[string]any) bool {
	want := 0
	for _, v := range m {
		if v != nil {
			want++
		}
	}
	if s == "" {
		return want == 0
	}
	parts := strings.Split(s, ",")
	if len(parts) != want {
		return false
	}
	seen := make(map[string]bool, len(parts))
	for _, part := range parts {
		k, v, found := strings.Cut(part, "=")
		if !found || seen[k] {
			return false
		}
		seen[k] = true
		mv, ok := m[k]
		if !ok || mv == nil || renderPropertyValue(mv) != v {
			return false
		}
	}
	return true
}

// parsePropertyString splits a PVE property string into key/value pairs;
// a part without `=` belongs to defaultKey.
func parsePropertyString(s, defaultKey string) map[string]string {
	out := map[string]string{}
	if s == "" {
		return out
	}
	for _, part := range strings.Split(s, ",") {
		k, v, found := strings.Cut(part, "=")
		if !found {
			out[defaultKey] = part
			continue
		}
		out[k] = v
	}
	return out
}

func scalarString(v attr.Value) string {
	ss, err := valueParamStrings(v)
	if err != nil || len(ss) == 0 {
		return ""
	}
	return ss[0]
}
