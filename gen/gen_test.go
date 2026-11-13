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
