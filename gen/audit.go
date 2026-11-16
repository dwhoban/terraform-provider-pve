// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dwhoban/terraform-provider-pve/internal/provider"
	"github.com/dwhoban/terraform-provider-pve/internal/pveapidoc"

	"github.com/dwhoban/terraform-provider-pve/gen/mapping"
)

// audit verifies the registry against the ROADMAP contract. It returns the
// list of violations; empty means the registry is honest.
func audit(doc *pveapidoc.Document, reg *provider.Registry) []string {
	var errs []string

	// 1. Family bookkeeping: every API family has exactly one Appendix A
	// assignment, and the assignments cover the schema exactly.
	assigned := map[string]bool{}
	for fam := range mapping.FamilyPhases {
		if _, oos := mapping.FamilyOOS[fam]; oos {
			errs = append(errs, fmt.Sprintf("family %s is both phased and OOS", fam))
		}
		assigned[fam] = true
	}
	for fam := range mapping.FamilyOOS {
		assigned[fam] = true
	}
	inSchema := map[string]bool{}
	for _, fam := range doc.Families {
		inSchema[fam.NormalizedPath()] = true
	}
	for n := range inSchema {
		if !assigned[n] {
			errs = append(errs, fmt.Sprintf("family %s missing from Appendix A assignments", n))
		}
	}
	for fam := range assigned {
		if !inSchema[fam] {
			errs = append(errs, fmt.Sprintf("Appendix A assigns %s but it is not in the API schema", fam))
		}
	}

	// 2. Per-type checks (type names are unique per kind — a resource and
	// its same-named read data source is the roadmap convention).
	names := map[provider.Kind]map[string]bool{}
	used := map[string]bool{} // normalized family → used by a type
	collect := func(path string) {
		if path == "" {
			return
		}
		if fam, ok := doc.Find(path); ok {
			used[fam.NormalizedPath()] = true
		}
	}
	for _, t := range reg.Types {
		if names[t.Kind] == nil {
			names[t.Kind] = map[string]bool{}
		}
		if names[t.Kind][t.Name] {
			errs = append(errs, fmt.Sprintf("duplicate %s type name %s", t.Kind, t.Name))
		}
		names[t.Kind][t.Name] = true
		if !strings.HasPrefix(t.Name, "pve_") {
			errs = append(errs, fmt.Sprintf("type %s: name must start with pve_", t.Name))
		}
		switch t.Behavior {
		case provider.BehaviorCollection, provider.BehaviorSingleton, provider.BehaviorACL, provider.BehaviorStateOnlyDelete:
		default:
			errs = append(errs, fmt.Sprintf("type %s: unknown behavior %q", t.Name, t.Behavior))
		}
		switch t.Kind {
		case provider.KindResource:
			if t.Create == nil {
				errs = append(errs, fmt.Sprintf("resource %s has no create", t.Name))
			}
			if t.Read == nil {
				errs = append(errs, fmt.Sprintf("resource %s has no read", t.Name))
			}
			if len(t.IDAttrs) == 0 && t.Behavior != provider.BehaviorSingleton {
				errs = append(errs, fmt.Sprintf("resource %s has no id composition", t.Name))
			}
			if t.Behavior == provider.BehaviorACL && (t.Delete == nil || len(t.Read.Filter) == 0) {
				errs = append(errs, fmt.Sprintf("acl resource %s needs delete + filtered read", t.Name))
			}
			if t.Update == nil && t.Behavior != provider.BehaviorSingleton {
				for _, a := range t.Attributes {
					if a.Optional && !a.Computed && !a.ForceNew {
						errs = append(errs, fmt.Sprintf("resource %s has no update but optional attr %q is not force-new (changes would be silently ignored; set AllForceNew)", t.Name, a.Name))
					}
				}
			}
		case provider.KindDataSource:
			if len(t.Reads) == 0 {
				errs = append(errs, fmt.Sprintf("data source %s has no reads", t.Name))
			}
		case provider.KindAction:
			if t.Invoke == nil {
				errs = append(errs, fmt.Sprintf("action %s has no invoke", t.Name))
			}
		case provider.KindFunction:
			if t.Call == nil {
				errs = append(errs, fmt.Sprintf("function %s has no call", t.Name))
			}
		default:
			errs = append(errs, fmt.Sprintf("type %s has no kind", t.Name))
		}

		ops := []*provider.Op{t.Create, t.Read, t.Update, t.Delete, t.Invoke, t.Call}
		if t.Clone != nil {
			ops = append(ops, t.Clone.Op)
		}
		ops = append(ops, t.Reads...)
		for _, op := range ops {
			if op == nil {
				continue
			}
			collect(op.Path)
			for _, p := range op.Paths {
				collect(p)
			}
			if op.Apply != nil {
				collect(op.Apply.Path)
				if a, ok := t.Attr(op.ApplyIf); !ok || a.Type != provider.AttrBool || !a.Optional || a.Computed {
					errs = append(errs, fmt.Sprintf("type %s: %s %s: apply requires applyIf naming an optional bool attribute", t.Name, op.Method, op.Path))
				}
			}
		}
		// 3. Attr ⇄ Params: every required API parameter is satisfiable
		// and every writable attribute reaches the API.
		errs = append(errs, auditWiring(doc, t)...)
		// 4. IDAttrs resolve.
		for _, n := range t.IDAttrs {
			if _, ok := t.Attr(n); !ok {
				errs = append(errs, fmt.Sprintf("type %s: id attr %q has no attribute", t.Name, n))
			}
		}
		// 4b. Attribute wire formats are known to the codec.
		for _, a := range t.Attributes {
			if a.Format != "" && a.Format != provider.FormatBase64 && a.Format != provider.FormatBootOrder {
				errs = append(errs, fmt.Sprintf("type %s: attribute %s: unknown format %q", t.Name, a.Name, a.Format))
			}
		}
		// 4c. Wire names are real PVE parameter names.
		for _, a := range t.Attributes {
			if strings.ContainsAny(a.TFName(), "[]") {
				errs = append(errs, fmt.Sprintf("type %s: attribute %s: wire name %q is a slot pattern (declare a Block or Skip it)", t.Name, a.Name, a.TFName()))
			}
		}
		// 5. Path params of every op resolve.
		for _, op := range ops {
			if op == nil {
				continue
			}
			for _, path := range append([]string{op.Path}, valuesOf(op.Paths)...) {
				for _, ph := range placeholders(path) {
					if !pathParamBound(t, ph) {
						errs = append(errs, fmt.Sprintf("type %s: path param {%s} unbound", t.Name, ph))
					}
				}
			}
		}
	}

	// 6. Coverage: every phased family is used (by a type or by the
	// hand-written Phase 1 foundation); no OOS family is used.
	for fam := range mapping.FamilyPhases {
		if !used[fam] && !mapping.FamilyImplemented[fam] {
			errs = append(errs, fmt.Sprintf("family %s assigned to a phase but unused by any type", fam))
		}
	}
	for fam := range used {
		if mapping.FamilyOOS[fam] {
			errs = append(errs, fmt.Sprintf("out-of-scope family %s is referenced by a type", fam))
		}
	}
	sort.Strings(errs)
	return errs
}

// auditWiring checks attributes and operation params in both directions:
// every required API parameter of an operation binds to a writable
// attribute (or a placeholder, constant, or engine-supplied value), and
// every writable attribute reaches some operation. Function calls bind
// positional arguments, not attributes.
func auditWiring(doc *pveapidoc.Document, t *provider.TypeDescriptor) []string {
	if t.Kind == provider.KindFunction {
		return nil
	}
	var errs []string
	ops := wiredOps(t)

	reach := map[string]bool{}
	if t.Scoped {
		reach["scope"] = true // selects the scoped path
	}
	if t.Clone != nil {
		reach[t.Clone.SourceAttr] = true
	}
	for _, n := range t.IDAttrs {
		reach[n] = true // ID composition (e.g. a constant singleton anchor)
	}
	for _, op := range ops {
		if op.ApplyIf != "" {
			reach[op.ApplyIf] = true
		}
		for _, n := range op.Params {
			reach[n] = true
		}
		for _, f := range op.Filter {
			reach[f.Attr] = true
		}
		for _, n := range op.SkipIfMissing {
			reach[n] = true
		}
		for _, path := range opPaths(op) {
			for _, ph := range placeholders(path) {
				for _, a := range t.Attributes {
					if a.PathParam == ph || a.Name == ph {
						reach[a.Name] = true
					}
				}
			}
		}
	}
	for _, a := range t.Attributes {
		if (a.Required || a.Optional) && !reach[a.Name] {
			errs = append(errs, fmt.Sprintf("type %s: writable attribute %s is sent by no operation", t.Name, a.Name))
		}
	}

	for _, op := range ops {
		engine := map[string]bool{}
		if t.Clone != nil && op == t.Clone.Op {
			engine[t.Clone.NewIDParam], engine[t.Clone.SourceParam] = true, true
		}
		for _, path := range opPaths(op) {
			fam, ok := doc.Find(path)
			if !ok {
				continue
			}
			mi, ok := fam.Methods[op.Method]
			if !ok {
				continue
			}
			phs := placeholders(path)
			for _, p := range mi.Parameters {
				if p.Optional || contains(phs, p.Name) || engine[p.Name] {
					continue
				}
				if _, isConst := op.ConstParams[p.Name]; isConst {
					continue
				}
				if !opBinds(t, op, p.Name) {
					errs = append(errs, fmt.Sprintf("type %s: %s %s: required parameter %q has no writable attribute", t.Name, op.Method, path, p.Name))
				}
			}
		}
	}
	return errs
}

// wiredOps lists every operation of a type that sends attribute params,
// staged-apply follow-ups included.
func wiredOps(t *provider.TypeDescriptor) []*provider.Op {
	all := []*provider.Op{t.Create, t.Read, t.Update, t.Delete, t.Invoke}
	if t.Clone != nil {
		all = append(all, t.Clone.Op)
	}
	all = append(all, t.Reads...)
	var ops []*provider.Op
	for _, op := range all {
		for ; op != nil; op = op.Apply {
			ops = append(ops, op)
		}
	}
	return ops
}

// opPaths lists an operation's path templates, scoped variants included.
func opPaths(op *provider.Op) []string {
	var out []string
	if op.Path != "" {
		out = append(out, op.Path)
	}
	scoped := valuesOf(op.Paths)
	sort.Strings(scoped)
	for _, p := range scoped {
		if !contains(out, p) {
			out = append(out, p)
		}
	}
	return out
}

// opBinds reports whether one of the operation's params carries API
// parameter api.
func opBinds(t *provider.TypeDescriptor, op *provider.Op, api string) bool {
	for _, n := range op.Params {
		if a, ok := t.Attr(n); ok && acceptsAttr([]string{api}, &a) {
			return true
		}
	}
	return false
}

func valuesOf(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}

func pathParamBound(t *provider.TypeDescriptor, ph string) bool {
	for _, a := range t.Attributes {
		if a.PathParam == ph || a.Name == ph {
			return true
		}
	}
	return false
}

// auditStructure is the subset of audit that must pass even before all
// phases are mapped (used by unit tests during incremental mapping).
func auditStructure(doc *pveapidoc.Document, reg *provider.Registry) []string {
	full := audit(doc, reg)
	var errs []string
	for _, e := range full {
		if strings.Contains(e, "assigned to a phase but unused") {
			continue
		}
		errs = append(errs, e)
	}
	return errs
}

// placeholders lists the {placeholders} of a path.
func placeholders(path string) []string {
	var out []string
	for _, m := range placeholderRe.FindAllStringSubmatch(path, -1) {
		if !contains(out, m[1]) {
			out = append(out, m[1])
		}
	}
	return out
}
