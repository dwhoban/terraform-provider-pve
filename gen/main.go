// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

// gen transforms the Proxmox VE API schema (apidoc.js) plus the ROADMAP.md
// mapping (gen/mapping) into the provider registry (registry.json), the
// ROADMAP Appendix A coverage table, and per-type usage examples.
//
// Usage:
//
//	go run ./gen                 # derive + audit + emit (fails on audit errors)
//	go run ./gen extract-phases  # regenerate phases_data.go from ROADMAP.md
package main

import (
	"fmt"
	"os"
	"sort"

	"github.com/dwhoban/terraform-provider-pve/internal/provider"
	"github.com/dwhoban/terraform-provider-pve/internal/pveapidoc"

	"github.com/dwhoban/terraform-provider-pve/gen/mapping"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "extract-phases":
			if err := extractPhases(); err != nil {
				fatal(err)
			}
			fmt.Println("regenerated gen/mapping/phases_data.go")
			return
		case "emit-appendix":
			doc, err := pveapidoc.Parse(pveapidoc.SchemaJS)
			if err != nil {
				fatal(err)
			}
			if err := emitAppendix(doc); err != nil {
				fatal(err)
			}
			fmt.Println("regenerated ROADMAP.md Appendix A")
			return
		case "coverage-phase":
			if len(os.Args) != 3 {
				fatal(fmt.Errorf("usage: go run ./gen coverage-phase N"))
			}
			coveragePhase(os.Args[2])
			return
		default:
			fatal(fmt.Errorf("unknown subcommand %q", os.Args[1]))
		}
	}

	doc, err := pveapidoc.Parse(pveapidoc.SchemaJS)
	if err != nil {
		fatal(err)
	}

	reg := &provider.Registry{SchemaVersion: 1}
	var errs []string
	for _, m := range mapping.All() {
		d, err := derive(doc, m)
		if err != nil {
			errs = append(errs, fmt.Sprintf("type %s: %v", m.Type, err))
			continue
		}
		reg.Types = append(reg.Types, d)
	}
	if errs = append(errs, audit(doc, reg)...); len(errs) > 0 {
		for _, e := range errs {
			fmt.Fprintf(os.Stderr, "audit: %s\n", e)
		}
		os.Exit(1)
	}

	if err := emitRegistry(reg); err != nil {
		fatal(err)
	}
	if err := emitAppendix(doc); err != nil {
		fatal(err)
	}
	if err := emitExamples(reg); err != nil {
		fatal(err)
	}
	fmt.Printf("registry: %d types; appendix + examples regenerated\n", len(reg.Types))
}

// coveragePhase prints, for one phase, which of its families are not yet
// referenced by any mapped type — the per-phase todo list while mapping.
func coveragePhase(phaseArg string) {
	var phase int
	if _, err := fmt.Sscanf(phaseArg, "%d", &phase); err != nil {
		fatal(err)
	}
	doc, err := pveapidoc.Parse(pveapidoc.SchemaJS)
	if err != nil {
		fatal(err)
	}
	reg := &provider.Registry{SchemaVersion: 1}
	for _, m := range mapping.All() {
		if d, err := derive(doc, m); err == nil {
			reg.Types = append(reg.Types, d)
		}
	}
	used := map[string]bool{}
	for _, t := range reg.Types {
		for _, op := range []*provider.Op{t.Create, t.Read, t.Update, t.Delete, t.Invoke, t.Call} {
			collectUse(doc, op, used)
			if op != nil {
				for _, p := range op.Paths {
					collectUsePath(doc, p, used)
				}
			}
		}
		for _, op := range t.Reads {
			collectUse(doc, op, used)
			for _, p := range op.Paths {
				collectUsePath(doc, p, used)
			}
		}
		if t.Clone != nil {
			collectUse(doc, t.Clone.Op, used)
		}
	}
	var missing []string
	for fam, p := range mapping.FamilyPhases {
		if p == phase && !used[fam] {
			missing = append(missing, fam)
		}
	}
	sort.Strings(missing)
	fmt.Printf("phase %d: %d/%d families used\n", phase, countPhase(phase)-len(missing), countPhase(phase))
	for _, m := range missing {
		fmt.Println("  unused:", m)
	}
}

func countPhase(p int) int {
	n := 0
	for _, v := range mapping.FamilyPhases {
		if v == p {
			n++
		}
	}
	return n
}

func collectUse(doc *pveapidoc.Document, op *provider.Op, used map[string]bool) {
	if op == nil || op.Path == "" {
		return
	}
	collectUsePath(doc, op.Path, used)
}

func collectUsePath(doc *pveapidoc.Document, path string, used map[string]bool) {
	if fam, ok := doc.Find(path); ok {
		used[fam.NormalizedPath()] = true
	}
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "gen: %v\n", err)
	os.Exit(1)
}

var _ = sort.Strings
