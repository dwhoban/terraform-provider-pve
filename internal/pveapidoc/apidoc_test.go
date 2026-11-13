// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package pveapidoc

import (
	"encoding/json"
	"testing"
)

func TestParseEmbeddedSchema(t *testing.T) {
	doc, err := Parse(SchemaJS)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(doc.Families) != 449 {
		t.Fatalf("families = %d, want 449 (ROADMAP.md Appendix A)", len(doc.Families))
	}

	// Sorted, unique paths.
	for i := 1; i < len(doc.Families); i++ {
		if doc.Families[i-1].Path >= doc.Families[i].Path {
			t.Fatalf("families not strictly sorted at %d: %q >= %q", i, doc.Families[i-1].Path, doc.Families[i].Path)
		}
	}

	// Spot-check Phase 1 endpoints.
	for path, methods := range map[string]map[string]bool{
		"/version":        {"GET": true},
		"/access/ticket":  {"GET": true, "POST": true},
		"/cluster/status": {"GET": true},
	} {
		fam, ok := doc.Find(path)
		if !ok {
			t.Fatalf("family %s not found", path)
		}
		for m := range methods {
			if fam.Methods[m] == nil {
				t.Errorf("%s: missing method %s", path, m)
			}
		}
	}
}

func TestParseParameters(t *testing.T) {
	doc, err := Parse(SchemaJS)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	fam, ok := doc.Find("/storage")
	if !ok {
		t.Fatal("/storage not found")
	}
	post := fam.Methods["POST"]
	if post == nil {
		t.Fatal("/storage POST missing")
	}
	byName := map[string]Parameter{}
	for _, p := range post.Parameters {
		byName[p.Name] = p
	}
	if p := byName["storage"]; p.Optional || p.Type != "string" {
		t.Errorf("storage param = %+v, want required string", p)
	}
	if p := byName["type"]; len(p.Enum) == 0 {
		t.Errorf("type param has no enum: %+v", p)
	}
	if p := byName["authsupported"]; !p.Optional {
		t.Errorf("authsupported param Optional=false, want true (UseNumber regression guard): %+v", p)
	}
	if post.Protected != true {
		t.Errorf("/storage POST Protected=false, want true: %+v", post)
	}
	if n := len(post.Parameters); n < 50 {
		t.Errorf("/storage POST has only %d parameters, expected the full backend matrix", n)
	}

	get := fam.Methods["GET"]
	if get == nil || len(get.Returns) == 0 {
		t.Fatal("/storage GET returns schema missing")
	}
	var returns struct {
		Type  string `json:"type"`
		Items struct {
			Properties map[string]json.RawMessage `json:"properties"`
		} `json:"items"`
	}
	if err := json.Unmarshal(get.Returns, &returns); err != nil {
		t.Fatalf("unmarshal returns: %v", err)
	}
	if returns.Type != "array" || len(returns.Items.Properties) == 0 {
		t.Fatalf("unexpected returns shape: %s", get.Returns)
	}
}

// TestParseFormatProperties pins the format-object layer: the named
// properties of a property-string parameter, their default_key flag, and
// alias links.
func TestParseFormatProperties(t *testing.T) {
	p, err := parseParameter("test[n]", map[string]any{
		"type":        "string",
		"description": "format-bearing parameter",
		"format": map[string]any{
			"file":   map[string]any{"default_key": json.Number("1"), "type": "string"},
			"volume": map[string]any{"alias": "file", "optional": json.Number("1")},
		},
	})
	if err != nil {
		t.Fatalf("parseParameter: %v", err)
	}
	if len(p.FormatProperties) != 2 {
		t.Fatalf("FormatProperties = %+v, want 2", p.FormatProperties)
	}
	if fp := p.FormatProperties[0]; fp.Key != "file" || !fp.DefaultKey {
		t.Errorf("property 0 = %+v, want file default_key", fp)
	}
	if fp := p.FormatProperties[1]; fp.Key != "volume" || fp.Alias != "file" || !fp.Optional || fp.DefaultKey {
		t.Errorf("property 1 = %+v, want volume alias file optional", fp)
	}
}

func TestNormalizedPath(t *testing.T) {
	tests := []struct{ in, want string }{
		{"/nodes/{node}/qemu/{vmid}", "/nodes/{*}/qemu/{*}"},
		{"/version", "/version"},
		{"/nodes/{node}/storage/{storeid}/content/{volid}", "/nodes/{*}/storage/{*}/content/{*}"},
	}
	for _, tt := range tests {
		f := &Family{Path: tt.in}
		if got := f.NormalizedPath(); got != tt.want {
			t.Errorf("NormalizedPath(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestParseMalformed(t *testing.T) {
	for name, src := range map[string]string{
		"no marker":  `const other = [];`,
		"not array":  `const apiSchema = {"a":1};`,
		"bad json":   `const apiSchema = [ {,} ];`,
		"empty file": ``,
	} {
		if _, err := Parse([]byte(src)); err == nil {
			t.Errorf("%s: Parse succeeded, want error", name)
		}
	}
}
