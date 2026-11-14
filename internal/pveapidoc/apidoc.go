// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

// Package pveapidoc parses the Proxmox VE API schema (apidoc.js, the source
// behind the official API viewer) into a typed tree. It is the machine-
// readable source of truth from which the provider's type registry is
// generated and audited; see ROADMAP.md.
package pveapidoc

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
)

// SchemaJS is the checked-in snapshot of
// https://pve.proxmox.com/pve-docs/api-viewer/apidoc.js from which the
// provider registry is generated.
//
//go:embed schema/apidoc.js
var SchemaJS []byte

// Document is the parsed API schema: every path family that exposes at
// least one HTTP method, in path order.
type Document struct {
	Families []*Family
}

// Family is one API path family, e.g. /nodes/{node}/qemu/{vmid}/config,
// with its per-method descriptions.
type Family struct {
	Path    string
	Methods map[string]*Method // keyed by GET/POST/PUT/DELETE
}

// NormalizedPath returns the family path with every {placeholder} replaced
// by {*} — the identifier used by ROADMAP.md Appendix A.
func (f *Family) NormalizedPath() string {
	var b strings.Builder
	rest := f.Path
	for {
		open := strings.IndexByte(rest, '{')
		if open < 0 {
			b.WriteString(rest)
			return b.String()
		}
		end := strings.IndexByte(rest[open:], '}')
		if end < 0 {
			b.WriteString(rest)
			return b.String()
		}
		b.WriteString(rest[:open])
		b.WriteString("{*}")
		rest = rest[open+end+1:]
	}
}

// Method is one HTTP method on a family.
type Method struct {
	HTTPMethod  string
	Name        string // apidoc operation name, e.g. "create", "update"
	Description string
	Protected   bool
	Parameters  []Parameter
	Returns     json.RawMessage // JSON-schema-ish description of the response
}

// Parameter is one request parameter of a method.
type Parameter struct {
	Name        string
	Type        string // "string" | "boolean" | "integer" | "number" | "array"
	Description string
	Format      string // e.g. "pve-storage-id", "ipv4"
	// FormatProperties are the properties of an object-valued format: the
	// named keys a property-string parameter carries (scsi[n]'s file, aio,
	// cache, ...). Empty when Format is a named string or absent.
	FormatProperties []FormatProperty
	Typetext         string
	Optional         bool
	Enum             []string
	Default          json.RawMessage
	MaxLength        int
	Items            *Parameter // array element descriptor
	// InstanceTypes lists the oneOf variants (values of TypeProperty) that
	// accept this parameter; nil when every variant (or no oneOf) does.
	InstanceTypes []string
	TypeProperty  string // discriminator parameter name when InstanceTypes is set
}

// FormatProperty is one property of a parameter's format object.
type FormatProperty struct {
	Key               string
	Type              string
	Alias             string // alias of another property (volume → file)
	KeyAlias          string
	FormatDescription string // value shape prose, e.g. "+FLAG[;-FLAG...]"
	Enum              []string
	Optional          bool
	DefaultKey        bool // PVE writes this property without "key="
}

// Find returns the family for an exact path.
func (d *Document) Find(path string) (*Family, bool) {
	for _, f := range d.Families {
		if f.Path == path {
			return f, true
		}
	}
	return nil, false
}

// Parse decodes the apiSchema array out of an apidoc.js source file.
func Parse(src []byte) (*Document, error) {
	marker := []byte("apiSchema")
	idx := bytes.Index(src, marker)
	if idx < 0 {
		return nil, fmt.Errorf("apidoc.js: %q marker not found", marker)
	}
	rest := src[idx+len(marker):]
	eq := bytes.IndexByte(rest, '=')
	if eq < 0 {
		return nil, fmt.Errorf("apidoc.js: no '=' after apiSchema")
	}
	rest = rest[eq+1:]

	dec := json.NewDecoder(bytes.NewReader(rest))
	dec.UseNumber()
	var raw any
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("apidoc.js: decoding apiSchema array: %w", err)
	}
	nodes, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("apidoc.js: apiSchema is %T, want array", raw)
	}

	seen := map[string]bool{}
	doc := &Document{}
	var walk func(list []any) error
	walk = func(list []any) error {
		for _, n := range list {
			node, ok := n.(map[string]any)
			if !ok {
				return fmt.Errorf("apidoc.js: node is %T, want object", n)
			}
			path, _ := node["path"].(string)
			if info, ok := node["info"].(map[string]any); ok && path != "" && !seen[path] {
				fam := &Family{Path: path, Methods: map[string]*Method{}}
				for m, mi := range info {
					minfo, ok := mi.(map[string]any)
					if !ok {
						continue
					}
					switch m {
					case "GET", "POST", "PUT", "DELETE":
					default:
						continue // non-method metadata keys
					}
					method, err := parseMethod(m, minfo)
					if err != nil {
						return fmt.Errorf("%s %s: %w", m, path, err)
					}
					fam.Methods[m] = method
				}
				if len(fam.Methods) > 0 {
					seen[path] = true
					doc.Families = append(doc.Families, fam)
				}
			}
			children, _ := node["children"].([]any)
			if err := walk(children); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(nodes); err != nil {
		return nil, err
	}
	sort.Slice(doc.Families, func(i, j int) bool { return doc.Families[i].Path < doc.Families[j].Path })
	return doc, nil
}

func parseMethod(httpMethod string, info map[string]any) (*Method, error) {
	m := &Method{HTTPMethod: httpMethod}
	m.Name, _ = info["name"].(string)
	m.Description, _ = info["description"].(string)
	m.Protected = numberFlag(info["protected"])
	if returns := info["returns"]; returns != nil {
		raw, err := json.Marshal(returns)
		if err != nil {
			return nil, fmt.Errorf("encoding returns: %w", err)
		}
		m.Returns = raw
	}
	if params, ok := info["parameters"].(map[string]any); ok {
		var all []Parameter
		props, _ := params["properties"].(map[string]any)
		parsed, err := parseProperties(props)
		if err != nil {
			return nil, err
		}
		all = append(all, parsed...)
		parts, _ := params["allOf"].([]any)
		for _, raw := range parts {
			part, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			if pp, ok := part["properties"].(map[string]any); ok {
				parsed, err := parseProperties(pp)
				if err != nil {
					return nil, err
				}
				all = append(all, parsed...)
			}
			if _, ok := part["oneOf"]; ok {
				parsed, err := parseOneOf(part)
				if err != nil {
					return nil, err
				}
				all = append(all, parsed...)
			}
		}
		seen := make(map[string]bool, len(all))
		for _, p := range all {
			if !seen[p.Name] {
				seen[p.Name] = true
				m.Parameters = append(m.Parameters, p)
			}
		}
		sort.Slice(m.Parameters, func(i, j int) bool { return m.Parameters[i].Name < m.Parameters[j].Name })
	}
	return m, nil
}

// parseProperties parses a JSON-schema properties map into parameters,
// sorted by name.
func parseProperties(props map[string]any) ([]Parameter, error) {
	names := make([]string, 0, len(props))
	for name := range props {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]Parameter, 0, len(names))
	for _, name := range names {
		pmap, ok := props[name].(map[string]any)
		if !ok {
			continue
		}
		p, err := parseParameter(name, pmap)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

// parseOneOf flattens a discriminated oneOf parameter group (PVE's
// type-property + instance-type variants) into one parameter per name:
// the discriminator itself, plus the union of the variants' parameters.
func parseOneOf(part map[string]any) ([]Parameter, error) {
	tp, _ := part["type-property"].(string)
	if tp == "" {
		return nil, fmt.Errorf("oneOf without type-property")
	}
	schema, _ := part["type-property-schema"].(map[string]any)
	disc, err := parseParameter(tp, schema)
	if err != nil {
		return nil, err
	}
	disc.Optional = false

	variants, _ := part["oneOf"].([]any)
	type seenParam struct {
		params []Parameter // one per containing variant, in variant order
		types  []string    // instance type of each entry in params
	}
	byName := map[string]*seenParam{}
	var order []string
	for _, raw := range variants {
		variant, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		it, _ := variant["instance-type"].(string)
		vprops, _ := variant["properties"].(map[string]any)
		parsed, err := parseProperties(vprops)
		if err != nil {
			return nil, err
		}
		for _, p := range parsed {
			sp := byName[p.Name]
			if sp == nil {
				sp = &seenParam{}
				byName[p.Name] = sp
				order = append(order, p.Name)
			}
			sp.params = append(sp.params, p)
			sp.types = append(sp.types, it)
		}
	}

	var allTypes []string
	for _, raw := range variants {
		if variant, ok := raw.(map[string]any); ok {
			it, _ := variant["instance-type"].(string)
			allTypes = append(allTypes, it)
		}
	}

	out := []Parameter{disc}
	for _, name := range order {
		sp := byName[name]
		merged, err := mergeVariants(sp.params, sp.types, len(allTypes))
		if err != nil {
			return nil, err
		}
		if len(sp.types) < len(allTypes) {
			merged.InstanceTypes = sp.types
			merged.TypeProperty = tp
		}
		out = append(out, merged)
	}
	return out, nil
}

// mergeVariants folds one parameter's per-variant definitions into one.
// params and types are parallel (the variants that declare it); total is
// the number of variants in the oneOf.
func mergeVariants(params []Parameter, types []string, total int) (Parameter, error) {
	m := params[0]
	m.Optional = len(params) < total
	sameDesc := true
	sameDefault := len(params) == total
	for i, p := range params {
		if p.Type != m.Type {
			return Parameter{}, fmt.Errorf("parameter %q: oneOf variants disagree on type (%s vs %s)", m.Name, m.Type, p.Type)
		}
		if p.Optional {
			m.Optional = true
		}
		if p.MaxLength > m.MaxLength {
			m.MaxLength = p.MaxLength
		}
		if p.Description != params[0].Description {
			sameDesc = false
		}
		if !bytes.Equal(p.Default, params[0].Default) {
			sameDefault = false
		}
		if i > 0 {
			for _, e := range p.Enum {
				if !slices.Contains(m.Enum, e) {
					m.Enum = append(m.Enum, e)
				}
			}
		}
	}
	if !sameDefault {
		m.Default = nil
	}
	if !sameDesc {
		segs := make([]string, len(params))
		for i, p := range params {
			segs[i] = fmt.Sprintf("`%s`: %s", types[i], p.Description)
		}
		m.Description = strings.Join(segs, " ")
	}
	return m, nil
}

// numberValue extracts an integer from a decoded JSON number, which is a
// json.Number under UseNumber. ok is false for non-numbers.
func numberValue(v any) (int, bool) {
	switch t := v.(type) {
	case json.Number:
		f, err := t.Float64()
		if err != nil {
			return 0, false
		}
		return int(f), true
	case float64:
		return int(t), true
	default:
		return 0, false
	}
}

// numberFlag reports whether a decoded JSON number is a non-zero flag.
func numberFlag(v any) bool {
	_, ok := numberValue(v)
	if !ok {
		return false
	}
	n, _ := numberValue(v)
	return n != 0
}

func parseParameter(name string, pmap map[string]any) (Parameter, error) {
	p := Parameter{Name: name}
	p.Type, _ = pmap["type"].(string)
	p.Description, _ = pmap["description"].(string)
	p.Format, _ = pmap["format"].(string)
	p.Typetext, _ = pmap["typetext"].(string)
	p.Optional = numberFlag(pmap["optional"])
	if n, ok := numberValue(pmap["maxLength"]); ok {
		p.MaxLength = n
	}
	if enums, ok := pmap["enum"].([]any); ok {
		for _, e := range enums {
			if s, ok := e.(string); ok {
				p.Enum = append(p.Enum, s)
			}
		}
	}
	if def, ok := pmap["default"]; ok && def != nil {
		raw, err := json.Marshal(def)
		if err != nil {
			return p, fmt.Errorf("parameter %q: encoding default: %w", name, err)
		}
		p.Default = raw
	}
	if items, ok := pmap["items"].(map[string]any); ok {
		ip, err := parseParameter(name+"[]", items)
		if err != nil {
			return p, err
		}
		p.Items = &ip
	}
	if formats, ok := pmap["format"].(map[string]any); ok {
		keys := make([]string, 0, len(formats))
		for k := range formats {
			keys = append(keys, k)
		}
		sort.Strings(keys) // stable regardless of JSON key order
		for _, k := range keys {
			pm, ok := formats[k].(map[string]any)
			if !ok {
				continue
			}
			p.FormatProperties = append(p.FormatProperties, parseFormatProperty(k, pm))
		}
	}
	return p, nil
}

// parseFormatProperty decodes one property of a format object.
func parseFormatProperty(key string, pmap map[string]any) FormatProperty {
	fp := FormatProperty{Key: key}
	fp.Type, _ = pmap["type"].(string)
	fp.Alias, _ = pmap["alias"].(string)
	fp.KeyAlias, _ = pmap["key_alias"].(string)
	fp.FormatDescription, _ = pmap["format_description"].(string)
	fp.Optional = numberFlag(pmap["optional"])
	fp.DefaultKey = numberFlag(pmap["default_key"])
	if enums, ok := pmap["enum"].([]any); ok {
		for _, e := range enums {
			if s, ok := e.(string); ok {
				fp.Enum = append(fp.Enum, s)
			}
		}
	}
	return fp
}
