// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/dwhoban/terraform-provider-pve/internal/provider"
	"github.com/dwhoban/terraform-provider-pve/internal/pveapidoc"

	"github.com/dwhoban/terraform-provider-pve/gen/mapping"
)

const registryPath = "internal/provider/registry.json"
const roadmapPath = "ROADMAP.md"

// appendixMarker starts the (final) appendix section of ROADMAP.md.
const appendixMarker = "\n## Appendix A — Full API surface mapping"

func emitRegistry(reg *provider.Registry) error {
	sort.Slice(reg.Types, func(i, j int) bool { return reg.Types[i].Name < reg.Types[j].Name })
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(reg); err != nil {
		return err
	}
	return os.WriteFile(registryPath, buf.Bytes(), 0o644)
}

// emitAppendix regenerates the Appendix A table inside ROADMAP.md from the
// parsed schema and the family assignments.
func emitAppendix(doc *pveapidoc.Document) error {
	raw, err := os.ReadFile(roadmapPath)
	if err != nil {
		return err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "\n## Appendix A — Full API surface mapping (%d families)\n\n", len(doc.Families))
	b.WriteString("| API path (`{*}` = placeholder) | Methods | Phase |\n")
	b.WriteString("|---|---|---|\n")
	for _, fam := range doc.Families {
		n := fam.NormalizedPath()
		methods := make([]string, 0, 4)
		for _, m := range []string{"GET", "POST", "PUT", "DELETE"} {
			if _, ok := fam.Methods[m]; ok {
				methods = append(methods, m)
			}
		}
		phase := "OOS"
		if p, ok := mapping.FamilyPhases[n]; ok {
			phase = fmt.Sprintf("%d — %s", p, mapping.PhaseLabels[p])
		}
		fmt.Fprintf(&b, "| `%s` | %s | %s |\n", n, strings.Join(methods, ", "), phase)
	}
	idx := strings.Index(string(raw), appendixMarker)
	if idx < 0 {
		return fmt.Errorf("appendix section not found in %s", roadmapPath)
	}
	out := string(raw[:idx]) + b.String()
	if out == string(raw) {
		return nil // already current
	}
	return os.WriteFile(roadmapPath, []byte(out), 0o644)
}

// emitExamples writes one minimal example per type.
func emitExamples(reg *provider.Registry) error {
	dirs := map[provider.Kind]string{
		provider.KindResource:   "examples/resources",
		provider.KindDataSource: "examples/data-sources",
		provider.KindAction:     "examples/actions",
		provider.KindFunction:   "examples/functions",
	}
	fileNames := map[provider.Kind]string{
		provider.KindResource:   "resource.tf",
		provider.KindDataSource: "data-source.tf",
		provider.KindAction:     "action.tf",
		provider.KindFunction:   "function.tf",
	}
	for _, t := range reg.Types {
		dir := dirs[t.Kind]
		if dir == "" {
			continue
		}
		// tfplugindocs resolves example directories by the FULL type
		// name — except functions, which live under the bare function
		// name (pve_nextid → nextid).
		dirName := t.Name
		if t.Kind == provider.KindFunction {
			dirName = strings.TrimPrefix(t.Name, "pve_")
		}
		path := filepath.Join(dir, dirName, fileNames[t.Kind])
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(exampleBody(t)), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// exampleValue picks a plausible placeholder value for required attributes.
func exampleValue(a provider.Attr) string {
	switch a.Type {
	case provider.AttrList, provider.AttrSet:
		return `["CHANGE_ME"]`
	case provider.AttrInt64:
		switch a.Name {
		case "vmid", "source_vmid", "newid":
			return "100"
		default:
			return "1"
		}
	case provider.AttrBool:
		return "true"
	default:
		if len(a.Enum) > 0 {
			return `"` + a.Enum[0] + `"`
		}
		switch a.Name {
		case "node":
			return `"pve1"`
		case "scope":
			if len(a.Enum) > 0 {
				return `"` + a.Enum[0] + `"`
			}
			return `"cluster"`
		case "storage", "storeid":
			return `"local"`
		case "upid":
			return `"UPID:pve1:000000:000000:000000:qmcreate:100:"`
		default:
			return `"CHANGE_ME"`
		}
	}
}

func exampleBody(t *provider.TypeDescriptor) string {
	name := strings.TrimPrefix(t.Name, "pve_")
	var b strings.Builder
	switch t.Kind {
	case provider.KindResource:
		fmt.Fprintf(&b, "resource %q %q {\n", t.Name, "example")
	case provider.KindDataSource:
		fmt.Fprintf(&b, "data %q %q {\n", t.Name, "example")
	case provider.KindAction:
		fmt.Fprintf(&b, "action %q %q {\n  config {\n", t.Name, "example")
	case provider.KindFunction:
		fmt.Fprintf(&b, "# %s — see docs/functions/%s.md\n", t.Name, name)
		return b.String()
	}
	indent := ""
	if t.Kind == provider.KindAction {
		indent = "  "
	}
	for _, a := range t.Attributes {
		if !a.Required {
			continue
		}
		fmt.Fprintf(&b, "%s%s = %s\n", indent, a.Name, exampleValue(a))
	}
	if t.Kind == provider.KindAction {
		b.WriteString("  }\n")
		b.WriteString("}\n\n")
		// An action without a trigger is declared but never executed: the
		// example must show the lifecycle wiring that invokes it.
		b.WriteString("# terraform_data is a no-op stand-in; wire action.pve actions to the\n")
		b.WriteString("# lifecycle events of any managed resource.\n")
		fmt.Fprintf(&b, "resource \"terraform_data\" \"trigger\" {\n  lifecycle {\n    action_trigger {\n      events  = [after_create]\n      actions = [action.%s.example]\n    }\n  }\n}\n", t.Name)
		return b.String()
	}
	b.WriteString("}\n")
	return b.String()
}

var rowRe = regexp.MustCompile(`(?m)^\| ` + "`" + `(/[^` + "`" + `]*)` + "`" + ` \| ([A-Z, ]+) \| (.+?) \|$`)
var labelRe = regexp.MustCompile(`^(\d+) — `)

// extractPhases regenerates gen/mapping/phases_data.go from ROADMAP.md.
func extractPhases() error {
	raw, err := os.ReadFile(roadmapPath)
	if err != nil {
		return err
	}
	phases := map[string]int{}
	oos := map[string]bool{}
	for _, m := range rowRe.FindAllStringSubmatch(string(raw), -1) {
		label := strings.TrimSpace(m[3])
		if label == "OOS" {
			oos[m[1]] = true
			continue
		}
		pm := labelRe.FindStringSubmatch(label)
		if pm == nil {
			return fmt.Errorf("unparsable phase label %q for %s", label, m[1])
		}
		var p int
		if _, err := fmt.Sscanf(pm[1], "%d", &p); err != nil {
			return err
		}
		phases[m[1]] = p
	}
	var b strings.Builder
	b.WriteString("// Copyright dwhoban 2026\n")
	b.WriteString("// SPDX-License-Identifier: MPL-2.0\n\npackage mapping\n\n")
	b.WriteString("// FamilyPhases and FamilyOOS are extracted from ROADMAP.md Appendix A —\n")
	b.WriteString("// the authoritative assignment of every API path family to a phase or to\n")
	b.WriteString("// Out-of-scope. Regenerate with `go run ./gen extract-phases`.\n")
	b.WriteString("var FamilyPhases = map[string]int{\n")
	for _, k := range sortedKeys(phases) {
		fmt.Fprintf(&b, "\t%q: %d,\n", k, phases[k])
	}
	b.WriteString("}\n\n")
	b.WriteString("// FamilyOOS lists out-of-scope families (see ROADMAP.md for reasons).\n")
	b.WriteString("var FamilyOOS = map[string]bool{\n")
	for _, k := range sortedKeysBool(oos) {
		fmt.Fprintf(&b, "\t%q: true,\n", k)
	}
	b.WriteString("}\n")
	return os.WriteFile("gen/mapping/phases_data.go", []byte(b.String()), 0o644)
}

func sortedKeys(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedKeysBool(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
