// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"fmt"

	"github.com/dwhoban/terraform-provider-pve/gen/mapping"
	"github.com/dwhoban/terraform-provider-pve/internal/provider"
	"github.com/dwhoban/terraform-provider-pve/internal/pveapidoc"
)

// applyBlocks folds declared slot-family blocks into the attribute set:
// each block consumes its families' derived slot-pattern attributes
// (apidoc `x[n]`) and adds one block attribute in their place. Called
// after the AttrRules and before op params bind to the final attrs.
func applyBlocks(doc *pveapidoc.Document, m mapping.Type, attrs map[string]*provider.Attr) error {
	// consumed tracks families already taken by an earlier block of this
	// type: sibling blocks may share a family (disk and cdrom share ide).
	consumed := map[string]bool{}
	for _, b := range m.Blocks {
		take := func(fam string, primary bool) error {
			key := attrNameOfParam(fam + "[n]")
			if b.Fixed {
				key = attrNameOfParam(fam)
			}
			a, ok := attrs[key]
			if !ok {
				if primary && !consumed[fam] {
					return fmt.Errorf("block %s: family %s has no slot parameter in the API schema", b.Name, fam)
				}
				return nil
			}
			if want := fam + "[n]"; a.TFName() != want && (!b.Fixed || a.TFName() != fam) {
				return nil // same-named attribute with another wire name: keep it
			}
			delete(attrs, key)
			consumed[fam] = true
			return nil
		}
		for _, fam := range b.Families {
			if err := take(fam, true); err != nil {
				return err
			}
		}
		for _, f := range b.Fields {
			if f.Family != "" && !consumed[f.Family] {
				if err := take(f.Family, false); err != nil {
					return err
				}
			}
		}
		// The collision check runs after the takes: a Fixed block's own
		// family attribute (rootfs) is consumed, not a collision.
		if _, exists := attrs[b.Name]; exists {
			return fmt.Errorf("block %s collides with attribute %s", b.Name, b.Name)
		}

		spec := &provider.BlockSpec{
			Families: b.Families, Fixed: b.Fixed,
			DefaultKey: b.DefaultKey, Exclude: b.Exclude,
		}
		var elems []provider.Attr
		for _, f := range b.Fields {
			if f.Const != "" {
				spec.Fields = append(spec.Fields, provider.BlockField{Key: f.Key, Const: f.Const})
				continue
			}
			if f.Name == "" {
				return fmt.Errorf("block %s: field with key %q needs a name", b.Name, f.Key)
			}
			ea := provider.Attr{Name: f.Name, Description: f.Description, Enum: f.Enum, Type: provider.AttrString}
			if f.Type != "" {
				ea.Type = provider.AttrType(f.Type)
			}
			if f.Required {
				ea.Required = true
			} else {
				ea.Optional = true
				if f.Computed {
					ea.Computed = true
				}
			}
			if f.Default != "" {
				ea.Default = f.Default
			}
			elems = append(elems, ea)
			spec.Fields = append(spec.Fields, provider.BlockField{
				Name: f.Name, Family: f.Family, Key: f.Key,
				Role: provider.FieldRole(f.Role), True: f.True, False: f.False,
				CreateOnly: f.CreateOnly,
			})
		}
		a := provider.Attr{Name: b.Name, Description: b.Description, Optional: true, Block: spec}
		if b.Fixed {
			a.Type = provider.AttrObject
			a.Attrs = elems
		} else {
			a.Type = provider.AttrList
			a.ElemAttrs = elems
		}
		attrs[b.Name] = &a
		if err := checkBlock(doc, m, b, spec); err != nil {
			return err
		}
	}
	return nil
}

// checkBlock validates a block spec against the API schema and adopts what
// the mapping left out. The slot parameter's format object (net[n]'s
// bridge, model, macaddr, ...) is the source of truth for the property
// keys a slot value may carry, and its default_key property names the
// value PVE writes without "key=". Families whose parameter has no format
// object (ipconfig[n] uses the named string "pve-qm-ipconfig") are
// skipped; a field key valid for any family's format passes (iothread is
// a scsi/virtio property, not an ide one).
func checkBlock(doc *pveapidoc.Document, m mapping.Type, b mapping.Block, spec *provider.BlockSpec) error {
	props := map[string]bool{}
	defKey := ""
	for _, fam := range b.Families {
		want := fam + "[n]"
		if b.Fixed {
			want = fam
		}
		for _, ep := range []struct {
			e   *mapping.Endpoint
			def string
		}{
			{m.Create, "POST"},
			{m.Update, "PUT"},
			{m.Invoke, "POST"},
		} {
			if ep.e == nil {
				continue
			}
			method := orDefault(ep.e.Method, ep.def)
			paths := []string{ep.e.Path}
			for _, p := range ep.e.Paths {
				paths = append(paths, p)
			}
			for _, p := range paths {
				if p == "" {
					continue
				}
				f, ok := doc.Find(p)
				if !ok {
					continue
				}
				mi, ok := f.Methods[method]
				if !ok {
					continue
				}
				for _, prm := range mi.Parameters {
					if prm.Name != want {
						continue
					}
					for _, fp := range prm.FormatProperties {
						props[fp.Key] = true
						if fp.DefaultKey {
							defKey = fp.Key
						}
					}
				}
			}
		}
	}
	if len(props) == 0 {
		return nil // no format object: nothing to validate against
	}
	switch {
	case b.DefaultKey != "" && defKey != "" && b.DefaultKey != defKey:
		return fmt.Errorf("block %s: default key %q does not match apidoc %q", b.Name, b.DefaultKey, defKey)
	case b.DefaultKey == "" && defKey != "":
		spec.DefaultKey = defKey // adopt the schema's default key
	}
	for _, f := range spec.Fields {
		if f.Family != "" || f.Role == provider.RoleIndex || f.Key == "size" {
			continue // secondary families and structural fields
		}
		if !props[f.Key] {
			return fmt.Errorf("block %s: field %s key %q is not a property of %s[n]", b.Name, f.Name, f.Key, b.Families[0])
		}
	}
	return nil
}
