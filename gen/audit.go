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
			}
			// 3. Params must resolve to attributes (function call
			// params resolve to positional arguments instead; GET
			// params without attributes are simply never sent — the
			// engine builds params from attributes it has).
			if (t.Kind != provider.KindFunction || op != t.Call) && op.Method != "GET" {
				for _, pn := range op.Params {
					if _, ok := t.Attr(pn); !ok {
						errs = append(errs, fmt.Sprintf("type %s: op param %q has no attribute", t.Name, pn))
					}
				}
			}
		}
		// 4. IDAttrs resolve.
		for _, n := range t.IDAttrs {
			if _, ok := t.Attr(n); !ok {
				errs = append(errs, fmt.Sprintf("type %s: id attr %q has no attribute", t.Name, n))
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
