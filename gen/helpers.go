// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/dwhoban/terraform-provider-pve/internal/provider"
	"github.com/dwhoban/terraform-provider-pve/internal/pveapidoc"

	"github.com/dwhoban/terraform-provider-pve/gen/mapping"
)

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// joinBackticked renders each item as inline code, joined by sep.
func joinBackticked(items []string, sep string) string {
	quoted := make([]string, len(items))
	for i, s := range items {
		quoted[i] = "`" + s + "`"
	}
	return strings.Join(quoted, sep)
}

var badIdentRe = regexp.MustCompile(`[^a-z0-9_]`)

// tfAttrName converts an API field name to a Terraform attribute name:
// dashes and other non-identifier characters become underscores (APIName
// preserves the original for the wire).
func tfAttrName(api string) string {
	return badIdentRe.ReplaceAllString(strings.ToLower(api), "_")
}

// attrNameOfParam maps an API parameter name to its derived attribute
// name, including the reserved "id" → "item_id" rename. deriveOp and
// attrFromParam must agree through this function.
func attrNameOfParam(api string) string {
	name := tfAttrName(api)
	if name == "id" {
		return "item_id"
	}
	return name
}

// apiNameOf returns the original API name when the conversion changed it.
func apiNameOf(api string) string {
	if tfAttrName(api) != api {
		return api
	}
	return ""
}

func attrTypeOf(p pveapidoc.Parameter) provider.AttrType {
	switch p.Type {
	case "boolean":
		return provider.AttrBool
	case "integer":
		return provider.AttrInt64
	case "number":
		return provider.AttrFloat64
	case "array":
		return provider.AttrList
	default:
		return provider.AttrString
	}
}

func attrTypeFromSchemaKind(kind string) provider.AttrType {
	switch kind {
	case "boolean":
		return provider.AttrBool
	case "integer":
		return provider.AttrInt64
	case "number":
		return provider.AttrFloat64
	default:
		// PVE omits "type" for string properties (and for malformed
		// entries); strings are the universal fallback.
		return provider.AttrString
	}
}

func kindOf(schema map[string]any) string {
	if schema == nil {
		return ""
	}
	k, _ := schema["type"].(string)
	return k
}

func propertiesOf(schema map[string]any) map[string]any {
	if schema == nil {
		return nil
	}
	props, _ := schema["properties"].(map[string]any)
	return props
}

func descriptionOf(schema map[string]any) string {
	if schema == nil {
		return ""
	}
	d, _ := schema["description"].(string)
	return d
}

func jsonUnmarshal(raw []byte, v any) error {
	return json.Unmarshal(raw, v)
}

func jsonRawToAny(raw []byte) any {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil
	}
	return v
}

// sensitiveName flags credentials in schemas.
var sensitiveRe = regexp.MustCompile(`(?i)(password|secret|privkey|passphrase|api[-_]?key|apikey|^token$|^token_value$|private[-_]?key)`)

func sensitiveName(apiName string) bool {
	return sensitiveRe.MatchString(apiName)
}

// mergeAttr folds derived attribute b into existing a: response-computed
// meets settable input → Optional+Computed.
func mergeAttr(a *provider.Attr, b provider.Attr) {
	if b.Computed && !b.Optional && !b.Required {
		// A response field meeting a settable request parameter stays
		// plain Optional: PVE config endpoints echo only SET keys, so an
		// Optional+Computed merge would leave unset attributes as
		// "known after apply" forever. Pure response fields (no param
		// counterpart) remain Computed.
		if a.Optional || a.Required {
			if b.Description != "" && a.Description == "" {
				a.Description = b.Description
			}
			return
		}
		a.Computed = true
	}
	if b.Optional {
		if a.Computed && !a.Optional {
			a.Optional = true
		}
		if !a.Optional && !a.Required {
			a.Optional = true
		}
		if a.Description == "" {
			a.Description = b.Description
		}
		if b.Enum != nil {
			a.Enum = b.Enum
		}
		if b.Default != nil {
			a.Default = b.Default
		}
		return
	}
	if b.Required && !a.Required {
		a.Required = false // response/params disagree: prefer optional
		a.Optional = true
	}
	if b.PathParam != "" {
		a.PathParam = b.PathParam
	}
}

// applyRule applies one AttrRule to the derived attribute set.
func applyRule(attrs map[string]*provider.Attr, rule mapping.AttrRule) error {
	a, ok := attrs[rule.Name]
	if !ok {
		if rule.Skip {
			return nil
		}
		if !rule.Add {
			return fmt.Errorf("no derived attribute %q (use Add to declare a new one)", rule.Name)
		}
		a = &provider.Attr{Name: rule.Name}
		attrs[rule.Name] = a
	}
	if rule.Skip {
		delete(attrs, rule.Name)
		return nil
	}
	applyRuleFields(a, rule)
	return nil
}

func applyRuleFields(a *provider.Attr, rule mapping.AttrRule) {
	if rule.APIName != "" {
		a.APIName = rule.APIName
	}
	if rule.NoPathParam {
		a.PathParam = ""
	}
	if rule.EchoName != "" {
		a.EchoName = rule.EchoName
	}
	if rule.Type != "" {
		a.Type = provider.AttrType(rule.Type)
	}
	if rule.Elem != "" {
		a.ElemType = provider.AttrType(rule.Elem)
	}
	if rule.Required != nil {
		a.Required = *rule.Required
	}
	if rule.Optional != nil {
		a.Optional = *rule.Optional
	}
	if rule.Computed != nil {
		a.Computed = *rule.Computed
	}
	// The framework forbids these combinations; an explicit rule wins.
	if a.Optional {
		a.Required = false
	}
	if a.Required {
		a.Optional = false
		a.Computed = false
	}
	if rule.Sensitive {
		a.Sensitive = true
	}
	if rule.ForceNew {
		a.ForceNew = true
	}
	if rule.Format != "" {
		a.Format = provider.AttrFormat(rule.Format)
	}
	if rule.Description != "" {
		a.Description = rule.Description
	}
	if rule.PathParam != "" {
		a.PathParam = rule.PathParam
	}
	if rule.Enum != nil {
		a.Enum = rule.Enum
	}
	if rule.Default != "" {
		a.Default = rule.Default
	}
	if rule.NoDefault {
		a.Default = nil
	}
}

// bindParams lists, sorted, the attributes an endpoint accepts given its
// API parameter names, by derived name or wire name. Computed-only
// attributes are response fields (e.g. `api_id` echoing `id`) and never
// request parameters.
func bindParams(apiNames []string, attrs map[string]*provider.Attr) []string {
	var out []string
	for name, a := range attrs {
		if acceptsAttr(apiNames, a) {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

func acceptsAttr(apiNames []string, a *provider.Attr) bool {
	if a.Computed && !a.Optional && !a.Required {
		return false
	}
	if a.Block != nil {
		for _, p := range apiNames {
			for _, fam := range a.Block.Families {
				if p == fam+"[n]" || (a.Block.Fixed && p == fam) {
					return true
				}
			}
			for _, f := range a.Block.Fields {
				if f.Family != "" && p == f.Family+"[n]" {
					return true
				}
			}
		}
		return false
	}
	wire := a.TFName()
	for _, p := range apiNames {
		if a.Name == attrNameOfParam(p) || wire == p {
			return true
		}
	}
	return false
}

// deriveFunction builds function args and result from the call endpoint.
func deriveFunction(doc *pveapidoc.Document, m mapping.Type) ([]provider.Attr, provider.AttrType, error) {
	e := m.Call
	if e == nil {
		return nil, "", fmt.Errorf("type %s: function needs Call", m.Type)
	}
	params := paramsOf(doc, e, orDefault(e.Method, "GET"))
	// Arguments follow the endpoint's parameter order unless Params set.
	if len(e.Params) > 0 {
		var ordered []pveapidoc.Parameter
		for _, name := range e.Params {
			for _, p := range params {
				if p.Name == name {
					ordered = append(ordered, p)
				}
			}
		}
		params = ordered
	}
	args := make([]provider.Attr, 0, len(params))
	for _, p := range params {
		if contains(e.SkipParams, p.Name) {
			continue
		}
		// Provider function parameters are positional and required; the
		// framework has no optional arguments, so optional API parameters
		// (e.g. nextid's vmid hint) cannot become function arguments.
		if p.Optional {
			continue
		}
		a := attrFromParam(p)
		args = append(args, a)
	}
	fam, ok := doc.Find(e.Path)
	if !ok {
		return nil, "", fmt.Errorf("type %s: call path %q not found", m.Type, e.Path)
	}
	mi, ok := fam.Methods[orDefault(e.Method, "GET")]
	if !ok {
		return nil, "", fmt.Errorf("type %s: %s has no GET", m.Type, e.Path)
	}
	result := provider.AttrString
	if len(mi.Returns) > 0 {
		var schema map[string]any
		if err := jsonUnmarshal(mi.Returns, &schema); err == nil {
			switch kindOf(schema) {
			case "integer":
				result = provider.AttrInt64
			case "number":
				result = provider.AttrFloat64
			case "boolean":
				result = provider.AttrBool
			}
		}
	}
	return args, result, nil
}

var _ = strings.TrimSpace
