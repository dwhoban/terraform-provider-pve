// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"strings"
	"testing"

	"github.com/dwhoban/terraform-provider-pve/internal/provider"
	"github.com/dwhoban/terraform-provider-pve/internal/pveapidoc"

	"github.com/dwhoban/terraform-provider-pve/gen/mapping"
)

func parsedDoc(t *testing.T) *pveapidoc.Document {
	t.Helper()
	doc, err := pveapidoc.Parse(pveapidoc.SchemaJS)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return doc
}

// TestDeriveAllMappedTypes derives every declared mapping type — the
// fastest feedback loop for mapping authors.
func TestDeriveAllMappedTypes(t *testing.T) {
	doc := parsedDoc(t)
	for _, m := range mapping.All() {
		if _, err := derive(doc, m); err != nil {
			t.Errorf("type %s: %v", m.Type, err)
		}
	}
}

// TestAuditStructure enforces everything except full-family coverage, so it
// is green while phases are still being mapped.
func TestAuditStructure(t *testing.T) {
	doc := parsedDoc(t)
	reg := deriveAll(t, doc)
	errs := auditStructure(doc, reg)
	for _, e := range errs {
		if strings.Contains(e, "assigned to a phase but unused") {
			continue
		}
		t.Errorf("audit: %s", e)
	}
}

// TestPhaseAssignmentsMatchSchema pins the Appendix A extraction against
// the parsed schema: exactly 449 families, 388 phased, 61 OOS (the 12
// imperative guest-agent families were moved from phase 3 to OOS to match
// the roadmap's out-of-scope table).
func TestPhaseAssignmentsMatchSchema(t *testing.T) {
	doc := parsedDoc(t)
	if len(doc.Families) != 449 {
		t.Fatalf("families = %d, want 449", len(doc.Families))
	}
	if len(mapping.FamilyPhases) != 388 {
		t.Errorf("FamilyPhases = %d entries, want 388", len(mapping.FamilyPhases))
	}
	if len(mapping.FamilyOOS) != 61 {
		t.Errorf("FamilyOOS = %d entries, want 61", len(mapping.FamilyOOS))
	}
	for _, fam := range doc.Families {
		n := fam.NormalizedPath()
		_, phased := mapping.FamilyPhases[n]
		_, oos := mapping.FamilyOOS[n]
		if phased == oos {
			t.Errorf("family %s: phased=%v oos=%v (exactly one required)", n, phased, oos)
		}
	}
}

// TestDeriveGuestBlocks pins the slot-family block derivation: blocks
// replace the slot-pattern attributes, bind into the mutating ops, and no
// unexpanded `x[n]` wire name survives.
func TestDeriveGuestBlocks(t *testing.T) {
	doc := parsedDoc(t)
	byName := map[string]*provider.TypeDescriptor{}
	for _, m := range mapping.All() {
		switch m.Type {
		case "pve_vm", "pve_container", "pve_cluster", "pve_cluster_join":
			d, err := derive(doc, m)
			if err != nil {
				t.Fatalf("type %s: %v", m.Type, err)
			}
			byName[m.Type+"/"+m.Kind] = d
		}
	}
	vm := byName["pve_vm/resource"]
	if vm == nil {
		t.Fatal("pve_vm resource not derived")
	}
	blocks := map[string]bool{}
	for _, a := range vm.Attributes {
		if a.Block != nil {
			blocks[a.Name] = true
		}
	}
	for _, want := range []string{"disk", "cdrom", "network"} {
		if !blocks[want] {
			t.Errorf("pve_vm: missing block %s (have %v)", want, blocks)
		}
	}
	if _, has := vm.Attr("ide0"); has {
		t.Error("pve_vm: flat ide0 attribute still present")
	}
	for _, op := range []*provider.Op{vm.Create, vm.Update} {
		names := append([]string(nil), op.Params...)
		for _, want := range []string{"disk", "cdrom", "network", "boot_order"} {
			if !contains(names, want) {
				t.Errorf("pve_vm %s params: %v misses %s", op.Method, names, want)
			}
		}
	}
	for _, key := range []string{"pve_vm/resource", "pve_container/resource", "pve_cluster/resource", "pve_cluster_join/action"} {
		d := byName[key]
		if d == nil {
			t.Fatalf("%s not derived", key)
		}
		for _, a := range d.Attributes {
			if strings.ContainsAny(a.TFName(), "[]") {
				t.Errorf("%s: attribute %s keeps slot-pattern wire name %q", key, a.Name, a.TFName())
			}
		}
	}
	ct := byName["pve_container/resource"]
	if a, ok := ct.Attr("rootfs"); !ok || a.Block == nil || !a.Block.Fixed {
		t.Error("pve_container: rootfs is not a Fixed block")
	}
	if a, ok := ct.Attr("network"); !ok || a.Block == nil {
		t.Error("pve_container: missing network block")
	}
	if a, ok := byName["pve_cluster/resource"].Attr("link"); !ok || a.Block == nil {
		t.Error("pve_cluster: missing link block")
	}
	if a, ok := byName["pve_cluster_join/action"].Attr("link"); !ok || a.Block == nil {
		t.Error("pve_cluster_join: missing link block")
	}
}

func deriveAll(t *testing.T, doc *pveapidoc.Document) *provider.Registry {
	t.Helper()
	reg := &provider.Registry{SchemaVersion: 1}
	for _, m := range mapping.All() {
		d, err := derive(doc, m)
		if err != nil {
			t.Fatalf("type %s: %v", m.Type, err)
		}
		reg.Types = append(reg.Types, d)
	}
	return reg
}
