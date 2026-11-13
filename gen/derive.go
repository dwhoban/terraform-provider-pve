// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/dwhoban/terraform-provider-pve/internal/provider"
	"github.com/dwhoban/terraform-provider-pve/internal/pveapidoc"

	"github.com/dwhoban/terraform-provider-pve/gen/mapping"
)

// derive converts one mapping.Type into a full TypeDescriptor, deriving
// attributes from the API schema: request parameters become inputs, read
// responses become computed attributes, path placeholders become required
// force-new identifiers.
func derive(doc *pveapidoc.Document, m mapping.Type) (*provider.TypeDescriptor, error) {
	if m.Type == "" || m.Phase == 0 || m.Summary == "" {
		return nil, fmt.Errorf("mapping type needs Type, Phase and Summary")
	}
	var cloneWire []string
	// wire records each derived op's API parameter names; Params are bound
	// to the final attribute set once all rules have applied.
	wire := map[*provider.Op][]string{}
	d := &provider.TypeDescriptor{
		Name:     m.Type,
		Phase:    m.Phase,
		Summary:  m.Summary,
		Behavior: provider.Behavior(m.Behavior),
		Scopes:   m.Scopes,
		Scoped:   len(m.Scopes) > 0,
		IDSep:    m.IDSep,
	}

	var err error
	attrs := map[string]*provider.Attr{}
	add := func(a provider.Attr) {
		if existing, ok := attrs[a.Name]; ok {
			mergeAttr(existing, a)
			return
		}
		attrs[a.Name] = &a
	}

	switch m.Kind {
	case "resource":
		d.Kind = provider.KindResource
		if d.Create, err = deriveOp(doc, m, m.Create, "POST", m.Phase, wire); err != nil {
			return nil, fmt.Errorf("create: %w", err)
		}
		if d.Read, err = deriveOp(doc, m, m.Read, "GET", m.Phase, wire); err != nil {
			return nil, fmt.Errorf("read: %w", err)
		}
		if m.Update != nil {
			if d.Update, err = deriveOp(doc, m, m.Update, "PUT", m.Phase, wire); err != nil {
				return nil, fmt.Errorf("update: %w", err)
			}
		}
		if m.Delete != nil {
			if d.Delete, err = deriveOp(doc, m, m.Delete, "DELETE", m.Phase, wire); err != nil {
				return nil, fmt.Errorf("delete: %w", err)
			}
		}
		if m.Clone != nil {
			cloneOp := &provider.Op{Method: orDefault(m.Clone.Method, "POST"), Path: m.Clone.Path, WaitTask: true}
			// Forward the clone endpoint's own parameters (name, target,
			// …) when the corresponding attributes are set.
			if fam, ok := doc.Find(m.Clone.Path); ok {
				if mi, has := fam.Methods[cloneOp.Method]; has {
					for _, prm := range mi.Parameters {
						name := attrNameOfParam(prm.Name)
						if name == m.Clone.NewIDParam || name == m.Clone.SourceAttr || prm.Name == m.Clone.SourceParam {
							continue // engine supplies these; the source id lives in the URI
						}
						cloneWire = append(cloneWire, prm.Name)
					}
				}
			}
			d.Clone = &provider.CloneSpec{
				Op:          cloneOp,
				SourceAttr:  m.Clone.SourceAttr,
				SourceParam: m.Clone.SourceParam,
				NewIDParam:  m.Clone.NewIDParam,
				OnlyParams:  m.Clone.OnlyParams,
				ConstParams: m.Clone.ConstParams,
			}
		}

		// Input attributes: create (or update for singletons) parameters.
		inputOp := m.Create
		inputDefault := "POST"
		if inputOp == nil {
			inputOp = m.Update
			inputDefault = "PUT"
		}
		if inputOp != nil {
			for _, p := range paramsOf(doc, inputOp, inputDefault) {
				add(attrFromParam(p))
			}
		}
		// Path placeholders across all lifecycle endpoints become
		// required, force-new identifiers (create's placeholders first,
		// then any only appearing on read/update/delete paths).
		var allPlaceholders []string
		for _, e := range []*mapping.Endpoint{inputOp, m.Read, m.Update, m.Delete} {
			for _, ph := range placeholdersOf(e) {
				if !contains(allPlaceholders, ph) {
					allPlaceholders = append(allPlaceholders, ph)
				}
			}
		}
		for _, ph := range allPlaceholders {
			a := placeholderAttr(ph)
			for _, e := range []*mapping.Endpoint{inputOp, m.Read, m.Update, m.Delete} {
				fam, ok := doc.Find(endpointPath(e))
				if !ok {
					continue
				}
				if prm := familyParam(fam, ph); prm != nil {
					a.Type = attrTypeOf(*prm)
					if prm.Description != "" {
						a.Description = prm.Description
					}
					break
				}
			}
			a.ForceNew = true
			add(a)
		}
		// Update-only parameters (digest, delete lists, ...) are user
		// inputs as well.
		if m.Update != nil && m.Update != inputOp {
			for _, p := range paramsOf(doc, m.Update, "PUT") {
				a := attrFromParam(p)
				a.Optional = true
				a.Required = false
				add(a)
			}
		}
		// Delete-only parameters (force flags etc.).
		if m.Delete != nil {
			for _, p := range paramsOf(doc, m.Delete, "DELETE") {
				a := attrFromParam(p)
				a.Optional, a.Computed = true, false
				a.Required = false
				add(a)
			}
		}
		// Read response fields become computed attributes.
		if d.Read != nil {
			for _, a := range attrsFromReturns(doc, m.Read, "") {
				add(a)
			}
		}
		// Default ID: the read path placeholders in order (mapped to
		// their derived attribute names).
		d.IDAttrs = dedupeStrings(m.IDAttrs)
		if len(d.IDAttrs) == 0 {
			for _, ph := range placeholdersOf(m.Read) {
				d.IDAttrs = append(d.IDAttrs, attrNameOfParam(ph))
			}
		}

	case "data_source":
		d.Kind = provider.KindDataSource
		for _, r := range m.Reads {
			op, err := deriveOp(doc, m, r, "GET", m.Phase, wire)
			if err != nil {
				return nil, fmt.Errorf("read %s: %w", r.Path, err)
			}
			d.Reads = append(d.Reads, op)
			for _, ph := range placeholdersOf(r) {
				a := placeholderAttr(ph)
				if fam, ok := doc.Find(r.Path); ok {
					if prm := familyParam(fam, ph); prm != nil {
						a.Type = attrTypeOf(*prm)
						if prm.Description != "" {
							a.Description = prm.Description
						}
					}
				}
				add(a)
			}
			readParams := paramsOf(doc, r, "GET")
			for _, p := range readParams {
				if r.ResultName != "" && attrNameOfParam(p.Name) == r.ResultName {
					return nil, fmt.Errorf("type %s: read %s: result attribute %q shadows a request parameter of the same name", m.Type, r.Path, r.ResultName)
				}
			}
			for _, p := range readParams {
				if !p.Optional {
					// Required query parameters are rare; treat as inputs.
					a := attrFromParam(p)
					a.Required = true
					add(a)
					continue
				}
				add(attrFromParam(p))
			}
			for _, a := range attrsFromReturns(doc, r, r.ResultName) {
				if r.ResultName != "" && a.Name == r.ResultName {
					// Result attributes are pure outputs: never merge
					// with same-named query parameters (the merge would
					// demote Computed and Terraform would discard the
					// provider's value).
					if existing, ok := attrs[a.Name]; ok && existing.Description != "" {
						a.Description = existing.Description
					}
					attrs[a.Name] = &a
					continue
				}
				add(a)
			}
		}
		d.IDAttrs = dedupeStrings(m.IDAttrs)
		if len(d.IDAttrs) == 0 {
			var ids []string
			for _, r := range m.Reads {
				for _, ph := range placeholdersOf(r) {
					ids = append(ids, attrNameOfParam(ph))
				}
			}
			d.IDAttrs = dedupeStrings(ids)
		}

	case "action":
		d.Kind = provider.KindAction
		if d.Invoke, err = deriveOp(doc, m, m.Invoke, "POST", m.Phase, wire); err != nil {
			return nil, fmt.Errorf("invoke: %w", err)
		}
		for _, ph := range placeholdersOf(m.Invoke) {
			a := placeholderAttr(ph)
			a.ForceNew = true
			if fam, ok := doc.Find(m.Invoke.Path); ok {
				if prm := familyParam(fam, ph); prm != nil {
					a.Type = attrTypeOf(*prm)
					if prm.Description != "" {
						a.Description = prm.Description
					}
				}
			}
			// Actions have no state; IDAttrs exists purely so progress and
			// error messages carry the operation's identifiers.
			d.IDAttrs = append(d.IDAttrs, attrNameOfParam(ph))
			add(a)
		}
		for _, p := range paramsOf(doc, m.Invoke, "POST") {
			add(attrFromParam(p))
		}

	case "function":
		d.Kind = provider.KindFunction
		if d.Call, err = deriveOp(doc, m, m.Call, "GET", m.Phase, wire); err != nil {
			return nil, fmt.Errorf("call: %w", err)
		}
		// Call params are positional function arguments, not attributes:
		// they keep their derived names.
		delete(wire, d.Call)
		d.FuncArgs, d.FuncResult, err = deriveFunction(doc, m)
		if err != nil {
			return nil, err
		}

	default:
		return nil, fmt.Errorf("type %s: unknown kind %q", m.Type, m.Kind)
	}

	// Apply overrides, then bind every op's params to the final attribute
	// set: skipped attributes stop being sent, added and slot-expanded
	// attributes start being sent where the endpoint accepts them.
	if m.Kind == "resource" {
		// "digest" is PVE's optimistic-concurrency token. Every server-side
		// change rewrites it, so as a managed attribute it can only produce
		// import/refresh drift; resources never expose it.
		delete(attrs, "digest")
	}
	for _, rule := range m.Attrs {
		if err := applyRule(attrs, rule); err != nil {
			return nil, fmt.Errorf("type %s attr %s: %w", m.Type, rule.Name, err)
		}
	}
	if err := applyBlocks(m, attrs); err != nil {
		return nil, fmt.Errorf("type %s: %w", m.Type, err)
	}
	for op, apiNames := range wire {
		op.Params = bindParams(apiNames, attrs)
	}

	if m.AllForceNew {
		for _, a := range attrs {
			if a.Optional && !a.Computed {
				a.ForceNew = true
			}
		}
	}

	// Deterministic order: required first, then name — stable output.
	names := make([]string, 0, len(attrs))
	for n := range attrs {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool {
		ai, aj := attrs[names[i]], attrs[names[j]]
		if ai.Required != aj.Required {
			return ai.Required
		}
		return names[i] < names[j]
	})
	for _, n := range names {
		a := attrs[n]
		if a.Required && a.Computed {
			a.Computed = false // framework forbids Required+Computed
		}
		attrs[n] = a
	}
	if d.Clone != nil && d.Clone.Op != nil {
		var forwarded []string
		for _, n := range bindParams(cloneWire, attrs) {
			if !attrs[n].Computed {
				forwarded = append(forwarded, n)
			}
		}
		d.Clone.Op.Params = forwarded
	}
	for _, n := range names {
		if attrs[n].Name == "scope" && d.Scoped {
			attrs[n].Enum = d.Scopes
		}
		if attrs[n].Name == "id" {
			return nil, fmt.Errorf("type %s: attribute name %q is reserved", m.Type, n)
		}
		d.Attributes = append(d.Attributes, *attrs[n])
	}
	return d, nil
}

// deriveOp validates one endpoint against the parsed schema and produces the
// engine Op, including the parameter name list.
func deriveOp(doc *pveapidoc.Document, m mapping.Type, e *mapping.Endpoint, defaultMethod string, phase int, wire map[*provider.Op][]string) (*provider.Op, error) {
	if e == nil {
		return nil, nil
	}
	method := orDefault(e.Method, defaultMethod)
	op := &provider.Op{
		Method: method, ResultName: e.ResultName, IDFromUPID: e.IDFromUPID,
		ResultAttr: e.ResultAttr, ResultField: e.ResultField,
		ConstParams: e.ConstParams, SkipIfMissingParam: e.SkipIfMissingParam,
		SkipIfMissing: e.SkipIfMissing,
	}
	paths := map[string]string{}
	if e.Path != "" {
		paths[""] = e.Path
	}
	for scope, p := range e.Paths {
		paths[scope] = p
	}
	paramNames := []string{}
	apiNames := []string{}
	for scope, p := range paths {
		fam, ok := doc.Find(p)
		if !ok {
			return nil, fmt.Errorf("path %q not found in API schema", p)
		}
		methodInfo, ok := fam.Methods[method]
		if !ok {
			return nil, fmt.Errorf("%s has no %s method in API schema", p, method)
		}
		if !e.CrossPhase {
			if fp, ok := mapping.FamilyPhases[fam.NormalizedPath()]; ok && fp != phase {
				return nil, fmt.Errorf("%s is phase %d but type %s is phase %d (set CrossPhase if intended)", p, fp, m.Type, phase)
			}
		}
		if scope == "" {
			op.Path = p
		}
		for _, prm := range selectedParams(methodInfo, e) {
			tfName := attrNameOfParam(prm.Name)
			if !contains(paramNames, tfName) {
				paramNames = append(paramNames, tfName)
			}
			if !contains(apiNames, prm.Name) {
				apiNames = append(apiNames, prm.Name)
			}
		}
	}
	if len(paths) > 0 {
		op.Paths = map[string]string{}
		for scope, p := range e.Paths {
			op.Paths[scope] = p
		}
		if e.Path != "" {
			op.Path = e.Path
		}
	}
	sort.Strings(paramNames)
	op.Params = paramNames
	wire[op] = apiNames
	// The endpoint accepts PVE's canonical unset parameter on update.
	if method == "PUT" {
		for _, scope := range paths {
			if fam, ok := doc.Find(scope); ok {
				if mi, has := fam.Methods[method]; has && mi != nil {
					for _, prm := range mi.Parameters {
						if prm.Name == "delete" {
							op.DeleteParam = true
						}
					}
				}
			}
		}
	}
	op.WaitTask = method != "GET"
	if e.WaitTask != nil {
		op.WaitTask = *e.WaitTask
	}
	for _, pair := range e.Filter {
		op.Filter = append(op.Filter, provider.FilterRule{Field: pair[0], Attr: pair[1]})
	}
	if e.Apply != nil {
		ap, err := deriveOp(doc, m, e.Apply, "PUT", phase, wire)
		if err != nil {
			return nil, fmt.Errorf("apply: %w", err)
		}
		op.Apply = ap
	}
	return op, nil
}

func selectedParams(methodInfo *pveapidoc.Method, e *mapping.Endpoint) []pveapidoc.Parameter {
	var out []pveapidoc.Parameter
	for _, p := range methodInfo.Parameters {
		if contains(e.SkipParams, p.Name) {
			continue
		}
		if len(e.Params) > 0 && !contains(e.Params, p.Name) {
			continue
		}
		out = append(out, p)
	}
	return out
}

// paramsOf lists the selected API parameters of an endpoint, resolving the
// method against the operation's role default.
func paramsOf(doc *pveapidoc.Document, e *mapping.Endpoint, defaultMethod string) []pveapidoc.Parameter {
	if e == nil {
		return nil
	}
	method := orDefault(e.Method, defaultMethod)
	paths := []string{e.Path}
	scopes := make([]string, 0, len(e.Paths))
	for sc := range e.Paths {
		scopes = append(scopes, sc)
	}
	sort.Strings(scopes)
	for _, sc := range scopes {
		paths = append(paths, e.Paths[sc])
	}
	var out []pveapidoc.Parameter
	seen := map[string]bool{}
	for _, p := range paths {
		if p == "" {
			continue
		}
		fam, ok := doc.Find(p)
		if !ok {
			continue
		}
		mi, ok := fam.Methods[method]
		if !ok {
			continue
		}
		for _, prm := range selectedParams(mi, e) {
			if !seen[prm.Name] {
				seen[prm.Name] = true
				out = append(out, prm)
			}
		}
	}
	return out
}

// placeholdersOf lists the {placeholders} of the endpoint's (first) path.
func placeholdersOf(e *mapping.Endpoint) []string {
	if e == nil {
		return nil
	}
	p := e.Path
	if p == "" {
		vals := make([]string, 0, len(e.Paths))
		for _, sp := range e.Paths {
			vals = append(vals, sp)
		}
		if len(vals) == 0 {
			return nil
		}
		sort.Strings(vals)
		p = vals[0]
	}
	var out []string
	for _, m := range placeholderRe.FindAllStringSubmatch(p, -1) {
		if !contains(out, m[1]) {
			out = append(out, m[1])
		}
	}
	return out
}

var placeholderRe = regexp.MustCompile(`\{([a-zA-Z0-9_-]+)\}`)

func attrFromParam(p pveapidoc.Parameter) provider.Attr {
	name := attrNameOfParam(p.Name)
	apiName := ""
	if name != p.Name {
		apiName = p.Name
	}
	a := provider.Attr{
		Name:        name,
		APIName:     apiName,
		Type:        attrTypeOf(p),
		Description: p.Description,
		Enum:        p.Enum,
	}
	if p.Typetext != "" && a.Description == "" {
		a.Description = p.Typetext
	}
	if p.Optional {
		a.Optional = true
		if p.Default != nil {
			a.Default = jsonRawToAny(p.Default)
		}
	} else {
		a.Required = true
	}
	a.Sensitive = sensitiveName(p.Name)
	if p.Type == "array" {
		if p.Items != nil {
			a.ElemType = attrTypeOf(*p.Items)
		} else {
			a.ElemType = provider.AttrString
		}
	}
	return a
}

func placeholderAttr(ph string) provider.Attr {
	name := attrNameOfParam(ph)
	a := provider.Attr{
		Name:      name,
		APIName:   "",
		PathParam: ph,
		Required:  true,
	}
	if intIDName(ph) {
		a.Type = provider.AttrInt64
	} else {
		a.Type = provider.AttrString
	}
	a.Description = fmt.Sprintf("Path parameter %q.", ph)
	return a
}

var intIDNames = map[string]bool{
	"vmid": true, "osdid": true, "id": true, "pos": true, "rules": true,
	"vmid-or-vm": false, "migrel": true, "restart": false,
}

func intIDName(ph string) bool {
	if v, ok := intIDNames[ph]; ok {
		return v
	}
	return strings.HasSuffix(ph, "id") && ph != "storage-id"
}

// attrsFromReturns derives computed attributes from a GET returns schema:
// array → one list attribute (under resultName), object → merged fields or
// one object attribute under resultName when set.
func attrsFromReturns(doc *pveapidoc.Document, e *mapping.Endpoint, resultName string) []provider.Attr {
	if e == nil {
		return nil
	}
	method := orDefault(e.Method, "GET")
	paths := []string{e.Path}
	scopes := make([]string, 0, len(e.Paths))
	for sc := range e.Paths {
		scopes = append(scopes, sc)
	}
	sort.Strings(scopes)
	for _, sc := range scopes {
		paths = append(paths, e.Paths[sc])
	}
	for _, p := range paths {
		if p == "" {
			continue
		}
		if attrs := returnsFromPath(doc, p, method, resultName); attrs != nil {
			return attrs
		}
	}
	return nil
}

// returnsFromPath derives computed attributes from one path's returns
// schema; nil when the path/method/returns yield nothing.
func returnsFromPath(doc *pveapidoc.Document, p, method, resultName string) []provider.Attr {
	fam, ok := doc.Find(p)
	if !ok {
		return nil
	}
	mi, ok := fam.Methods[method]
	if !ok || len(mi.Returns) == 0 {
		return nil
	}
	var schema map[string]any
	if err := jsonUnmarshal(mi.Returns, &schema); err != nil {
		return nil
	}
	switch kindOf(schema) {
	case "array":
		if resultName == "" {
			return nil
		}
		items, _ := schema["items"].(map[string]any)
		elemAttrs := attrsFromProperties(propertiesOf(items))
		attr := provider.Attr{
			Name: resultName, Type: provider.AttrList, Computed: true,
			Description: "Returned by " + p + ".",
		}
		if len(elemAttrs) > 0 {
			attr.ElemAttrs = elemAttrs
		} else if kindOf(items) == "string" || kindOf(items) == "integer" || kindOf(items) == "number" || kindOf(items) == "boolean" {
			attr.ElemType = attrTypeFromSchemaKind(kindOf(items))
		} else {
			// apidoc quirk: array declared with object items but no
			// properties (pve-docgen emits this for structured
			// singletons and unstructured rows). Field names are
			// unknowable from the schema, so carry the raw JSON.
			attr.Type = provider.AttrString
			attr.Description = "Raw JSON payload returned by " + p + " (the API schema does not describe its shape)."
		}
		return []provider.Attr{attr}
	case "object":
		props := propertiesOf(schema)
		if resultName != "" {
			return []provider.Attr{{
				Name: resultName, Type: provider.AttrObject, Computed: true,
				Attrs: attrsFromProperties(props),
			}}
		}
		return attrsFromProperties(props)
	default:
		// Scalar response (e.g. /cluster/nextid).
		if resultName == "" {
			return nil
		}
		return []provider.Attr{{
			Name: resultName, Type: attrTypeFromSchemaKind(kindOf(schema)), Computed: true,
		}}
	}
}

func attrsFromProperties(props map[string]any) []provider.Attr {
	names := make([]string, 0, len(props))
	for n := range props {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]provider.Attr, 0, len(names))
	for _, n := range names {
		ps, _ := props[n].(map[string]any)
		name := tfAttrName(n)
		// "id" is reserved for the Terraform resource id.
		apiName := apiNameOf(n)
		if name == "id" {
			name, apiName = "api_id", "id"
		}
		a := provider.Attr{
			Name:        name,
			APIName:     apiName,
			Computed:    true,
			Description: descriptionOf(ps),
		}
		switch kindOf(ps) {
		case "array":
			a.Type = provider.AttrList
			items, _ := ps["items"].(map[string]any)
			elemAttrs := attrsFromProperties(propertiesOf(items))
			if len(elemAttrs) > 0 {
				a.ElemAttrs = elemAttrs
			} else {
				a.ElemType = attrTypeFromSchemaKind(kindOf(items))
			}
		case "object":
			a.Type = provider.AttrObject
			a.Attrs = attrsFromProperties(propertiesOf(ps))
		default:
			a.Type = attrTypeFromSchemaKind(kindOf(ps))
		}
		out = append(out, a)
	}
	return out
}

// familyParam finds a request parameter with the placeholder's name on any
// method of a family, to type/describe synthesized path attributes from the
// real schema instead of guessing.
func familyParam(fam *pveapidoc.Family, name string) *pveapidoc.Parameter {
	for _, mi := range fam.Methods {
		for i := range mi.Parameters {
			if mi.Parameters[i].Name == name {
				return &mi.Parameters[i]
			}
		}
	}
	return nil
}

// endpointPath returns the (first) path of a mapping endpoint.
func endpointPath(e *mapping.Endpoint) string {
	if e == nil {
		return ""
	}
	if e.Path != "" {
		return e.Path
	}
	for _, p := range e.Paths {
		return p
	}
	return ""
}

// dedupeStrings preserves order while removing duplicates.
func dedupeStrings(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
