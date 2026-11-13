// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"cmp"
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"net/url"
	"path"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// This file owns the block codec: the translation between Terraform block
// values and PVE's indexed slot families, plus the diff and merge interface
// over them. Plain attributes live in attr_codec.go; engines only
// orchestrate.

// A block attribute (Attr.Block) maps Terraform blocks onto an indexed PVE
// slot family: each element is one slot (scsi0, net1, mp3, rootfs) whose
// value is a property string, optionally with a secondary family sharing
// the element index (net1 ⇄ ipconfig1).

// blockElem is one decoded slot of a block attribute.
type blockElem struct {
	key    string
	idx    int
	fields map[string]attr.Value
}

// blockElemAttrs lists the element attributes of a block attribute.
func blockElemAttrs(a Attr) []Attr {
	if a.Block.Fixed {
		return a.Attrs
	}
	return a.ElemAttrs
}

// blockElems lists a block value's concrete elements as attribute maps.
func blockElems(v attr.Value) []map[string]attr.Value {
	if !usable(v) {
		return nil
	}
	switch t := v.(type) {
	case types.List:
		out := make([]map[string]attr.Value, 0, len(t.Elements()))
		for _, e := range t.Elements() {
			if o, ok := e.(types.Object); ok && usable(o) {
				out = append(out, o.Attributes())
			}
		}
		return out
	case types.Object:
		return []map[string]attr.Value{t.Attributes()}
	}
	return nil
}

// blockValue builds a block attribute value from element attribute maps;
// missing element attributes are null. A list block without elements is
// an empty list (Terraform's form for "no blocks"), a Fixed one is null.
func blockValue(a Attr, elems []map[string]attr.Value) attr.Value {
	elemAttrs := blockElemAttrs(a)
	at := attrTypesOf(elemAttrs)
	build := func(f map[string]attr.Value) types.Object {
		full := make(map[string]attr.Value, len(elemAttrs))
		for _, ea := range elemAttrs {
			if v, ok := f[ea.Name]; ok && v != nil {
				full[ea.Name] = v
			} else {
				full[ea.Name] = nullValue(ea)
			}
		}
		return types.ObjectValueMust(at, full)
	}
	if a.Block.Fixed {
		if len(elems) == 0 {
			return types.ObjectNull(at)
		}
		return build(elems[0])
	}
	objs := make([]attr.Value, len(elems))
	for i, e := range elems {
		objs[i] = build(e)
	}
	return types.ListValueMust(types.ObjectType{AttrTypes: at}, objs)
}

// slotOf reports the family position and index of a slot key
// (scsi17 → scsi, 17).
func slotOf(families []string, key string) (fam, idx int, ok bool) {
	for i, f := range families {
		digits, found := strings.CutPrefix(key, f)
		if !found || digits == "" || strings.Trim(digits, "0123456789") != "" {
			continue
		}
		n, err := strconv.Atoi(digits)
		if err != nil {
			continue
		}
		return i, n, true
	}
	return 0, 0, false
}

// blockElemKey returns the wire key of a block element: the family itself
// for Fixed blocks, the index field's value when the block has one, the
// first family plus the list position otherwise.
func blockElemKey(spec *BlockSpec, elem map[string]attr.Value, pos int) (string, error) {
	if spec.Fixed {
		return spec.Families[0], nil
	}
	for _, f := range spec.Fields {
		if f.Role != RoleIndex {
			continue
		}
		v, ok := elem[f.Name].(types.String)
		if !ok || !usable(v) {
			return "", fmt.Errorf("%s is not set", f.Name)
		}
		key := v.ValueString()
		if _, _, ok := slotOf(spec.Families, key); !ok {
			return "", fmt.Errorf("%s %q must be one of %s followed by a slot number", f.Name, key, strings.Join(spec.Families, "|"))
		}
		return key, nil
	}
	return spec.Families[0] + strconv.Itoa(pos), nil
}

// slotSuffix returns the index digits of an element's wire key.
func slotSuffix(spec *BlockSpec, key string) string {
	if _, idx, ok := slotOf(spec.Families, key); ok && !spec.Fixed {
		return strconv.Itoa(idx)
	}
	return ""
}

// secondaryFamilies lists the secondary slot families of a block, in field
// declaration order.
func secondaryFamilies(spec *BlockSpec) []string {
	var out []string
	for _, f := range spec.Fields {
		if f.Family != "" && !slices.Contains(out, f.Family) {
			out = append(out, f.Family)
		}
	}
	return out
}

// specialRole reports whether a field role is encoded through the default
// key or the slot key rather than as its own `key=value`.
func specialRole(r FieldRole) bool {
	return r != RolePlain
}

func encodeFieldValue(f BlockField, v attr.Value) string {
	if b, ok := v.(types.Bool); ok {
		if b.ValueBool() {
			return cmp.Or(f.True, "1")
		}
		return cmp.Or(f.False, "0")
	}
	return scalarString(v)
}

// encodeBlock renders a block attribute as PVE request parameters: one
// property string per element slot, plus secondary-family slots carrying
// at least one value.
func encodeBlock(a Attr, v attr.Value) (url.Values, error) {
	spec := a.Block
	elems := blockElems(v)
	if len(elems) == 0 {
		return nil, nil
	}
	out := url.Values{}
	for pos, e := range elems {
		key, err := blockElemKey(spec, e, pos)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", a.Name, err)
		}
		if _, dup := out[key]; dup {
			return nil, fmt.Errorf("%s: %s is used by two blocks", a.Name, key)
		}
		primary, err := encodeSlot(a, e, key)
		if err != nil {
			return nil, err
		}
		out.Set(key, primary)
		idx := slotSuffix(spec, key)
		for _, fam := range secondaryFamilies(spec) {
			var parts []string
			for _, f := range spec.Fields {
				if f.Family == fam && usable(e[f.Name]) {
					parts = append(parts, f.Key+"="+encodeFieldValue(f, e[f.Name]))
				}
			}
			if len(parts) > 0 {
				out.Set(fam+idx, strings.Join(parts, ","))
			}
		}
	}
	return out, nil
}

// encodeSlot renders one element's primary slot value: the default-key
// value first, then plain fields as key=value, then constants.
func encodeSlot(a Attr, e map[string]attr.Value, key string) (string, error) {
	spec := a.Block
	var parts []string
	head, err := encodeDefaultKey(a, e, key)
	if err != nil {
		return "", err
	}
	if head != "" {
		parts = append(parts, head)
	}
	for _, f := range spec.Fields {
		if f.Name == "" || f.Family != "" || specialRole(f.Role) || (spec.DefaultKey != "" && f.Key == spec.DefaultKey) {
			continue
		}
		if v := e[f.Name]; usable(v) {
			parts = append(parts, f.Key+"="+encodeFieldValue(f, v))
		}
	}
	for _, f := range spec.Fields {
		if f.Const != "" {
			parts = append(parts, f.Key+"="+f.Const)
		}
	}
	return strings.Join(parts, ","), nil
}

// encodeDefaultKey renders the value PVE writes without a key: a volume
// (existing id, or storage:size to allocate), a NIC model (with its MAC
// once known), or a plain field keyed by DefaultKey.
func encodeDefaultKey(a Attr, e map[string]attr.Value, key string) (string, error) {
	spec := a.Block
	if spec.DefaultKey == "" {
		return "", nil
	}
	var id, storage, size, model, mac attr.Value
	volume, hasModel := false, false
	for _, f := range spec.Fields {
		switch f.Role {
		case RoleVolumeID:
			volume, id = true, e[f.Name]
		case RoleVolumeStorage:
			volume, storage = true, e[f.Name]
		case RoleVolumeSizeGB:
			volume, size = true, e[f.Name]
		case RoleModel:
			hasModel, model = true, e[f.Name]
		case RoleMAC:
			mac = e[f.Name]
		}
	}
	switch {
	case volume:
		if usable(id) {
			return scalarString(id), nil
		}
		if usable(storage) && usable(size) {
			return scalarString(storage) + ":" + scalarString(size), nil
		}
		return "", fmt.Errorf("%s %s: set volume_id, or storage and size_gb", a.Name, key)
	case hasModel:
		if !usable(model) {
			return "", nil
		}
		if usable(mac) {
			return scalarString(model) + "=" + scalarString(mac), nil
		}
		return scalarString(model), nil
	}
	for _, f := range spec.Fields {
		if f.Name != "" && f.Family == "" && f.Role == RolePlain && f.Key == spec.DefaultKey && usable(e[f.Name]) {
			return encodeFieldValue(f, e[f.Name]), nil
		}
	}
	return "", nil
}

// wireString renders a decoded JSON scalar as the string PVE sent.
func wireString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case json.Number:
		return t.String()
	default:
		return fmt.Sprint(t)
	}
}

// decodeBlockSlots reads a block's slots out of a config response. present
// reports whether the response carries any primary slot of the family at
// all (read endpoints that do not echo slots leave the block untouched).
func decodeBlockSlots(a Attr, m map[string]any) ([]blockElem, bool, error) {
	spec := a.Block
	type slot struct {
		key      string
		fam, idx int
		raw      string
	}
	var slots []slot
	for k, v := range m {
		if spec.Fixed {
			if k == spec.Families[0] {
				slots = append(slots, slot{key: k, raw: wireString(v)})
			}
			continue
		}
		if fam, idx, ok := slotOf(spec.Families, k); ok {
			slots = append(slots, slot{key: k, fam: fam, idx: idx, raw: wireString(v)})
		}
	}
	if len(slots) == 0 {
		return nil, false, nil
	}
	sort.Slice(slots, func(i, j int) bool {
		if slots[i].fam != slots[j].fam {
			return slots[i].fam < slots[j].fam
		}
		return slots[i].idx < slots[j].idx
	})
	elems := make([]blockElem, 0, len(slots))
	for _, s := range slots {
		props := parsePropertyString(s.raw, spec.DefaultKey)
		if !slotSelected(spec, props) {
			continue
		}
		fields, err := decodeSlotFields(a, s.key, s.idx, props, m)
		if err != nil {
			return nil, true, err
		}
		elems = append(elems, blockElem{key: s.key, idx: s.idx, fields: fields})
	}
	if !spec.Fixed && !hasIndexField(spec) {
		fam := spec.Families[0]
		for i, e := range elems {
			if e.idx != i {
				return nil, true, fmt.Errorf("%s: PVE has %s but no %s%d; %s blocks map to %s0..%sN by position", a.Name, e.key, fam, i, a.Name, fam, fam)
			}
		}
	}
	return elems, true, nil
}

func hasIndexField(spec *BlockSpec) bool {
	for _, f := range spec.Fields {
		if f.Role == RoleIndex {
			return true
		}
	}
	return false
}

// slotSelected applies a block's read filters: Exclude patterns and the
// constants that identify its slots (media=cdrom).
func slotSelected(spec *BlockSpec, props map[string]string) bool {
	for k, pat := range spec.Exclude {
		if v, ok := props[k]; ok {
			if matched, _ := path.Match(pat, v); matched {
				return false
			}
		}
	}
	for _, f := range spec.Fields {
		if f.Const != "" && props[f.Key] != f.Const {
			return false
		}
	}
	return true
}

// modelAndMAC splits a NIC slot's `<model>=<mac>` pair.
func modelAndMAC(a Attr, props map[string]string) (model, mac string) {
	spec := a.Block
	var enum []string
	macKey := ""
	for _, f := range spec.Fields {
		switch f.Role {
		case RoleModel:
			if ea, ok := findAttr(blockElemAttrs(a), f.Name); ok {
				enum = ea.Enum
			}
		case RoleMAC:
			macKey = f.Key
		}
	}
	for k, v := range props {
		if slices.Contains(enum, k) {
			return k, v
		}
	}
	return props[spec.DefaultKey], props[macKey]
}

func findAttr(list []Attr, name string) (Attr, bool) {
	for _, a := range list {
		if a.Name == name {
			return a, true
		}
	}
	return Attr{}, false
}

// decodeSlotFields decodes one slot (and its secondary-family slots) into
// element attribute values; absent keys are null.
func decodeSlotFields(a Attr, key string, idx int, props map[string]string, m map[string]any) (map[string]attr.Value, error) {
	spec := a.Block
	elemAttrs := blockElemAttrs(a)
	fields := make(map[string]attr.Value, len(elemAttrs))
	for _, ea := range elemAttrs {
		fields[ea.Name] = nullValue(ea)
	}
	model, mac := modelAndMAC(a, props)
	secondary := map[string]map[string]string{}
	for _, f := range spec.Fields {
		ea, ok := findAttr(elemAttrs, f.Name)
		if f.Name == "" || !ok {
			continue
		}
		src := props
		if f.Family != "" {
			if _, seen := secondary[f.Family]; !seen {
				secondary[f.Family] = parsePropertyString(wireString(m[f.Family+strconv.Itoa(idx)]), "")
			}
			src = secondary[f.Family]
		}
		var raw string
		var has bool
		switch f.Role {
		case RoleIndex:
			fields[f.Name] = types.StringValue(key)
			continue
		case RoleVolumeID:
			raw, has = props[spec.DefaultKey]
		case RoleVolumeStorage:
			raw, has = props[spec.DefaultKey]
			raw, _, _ = strings.Cut(raw, ":")
		case RoleVolumeSizeGB:
			s, ok := props["size"]
			if !ok {
				continue
			}
			gb, err := parseSizeGB(s)
			if err != nil {
				return nil, fmt.Errorf("%s %s: cannot parse size %q", a.Name, key, s)
			}
			fields[f.Name] = types.Int64Value(gb)
			continue
		case RoleModel:
			raw, has = model, model != ""
		case RoleMAC:
			raw, has = mac, mac != ""
		default:
			raw, has = src[f.Key]
		}
		if !has {
			continue
		}
		if ea.Type == AttrBool {
			fields[f.Name] = types.BoolValue(raw == cmp.Or(f.True, "1") || raw == "1" || raw == "on")
			continue
		}
		v, err := decodeAttr(Attr{Name: ea.Name, Type: ea.Type}, raw)
		if err != nil {
			return nil, fmt.Errorf("%s %s: %w", a.Name, key, err)
		}
		fields[f.Name] = v
	}
	return fields, nil
}

// parseSizeGB converts a PVE size (bytes, or with a K/M/G/T suffix) to
// whole GiB, rounding up.
func parseSizeGB(s string) (int64, error) {
	num, mult := s, 1.0
	if n := len(s); n > 0 {
		switch s[n-1] {
		case 'K', 'k':
			mult = 1 << 10
		case 'M', 'm':
			mult = 1 << 20
		case 'G', 'g':
			mult = 1 << 30
		case 'T', 't':
			mult = 1 << 40
		}
		if mult != 1 {
			num = s[:n-1]
		}
	}
	f, err := strconv.ParseFloat(num, 64)
	if err != nil {
		return 0, err
	}
	return int64(math.Ceil(f * mult / (1 << 30))), nil
}

// mergeBlock merges a config response into a block attribute. Configured
// elements that the server confirms keep their configured form, with
// computed fields (volume id, MAC) filled in; differing slots adopt the
// server's element (drift); slots the server added are appended. An
// unconfigured block stays unmanaged on read-backs; imports adopt every
// slot.
func mergeBlock(a Attr, cur attr.Value, m map[string]any, honorPlanNulls bool) (attr.Value, error) {
	if cur == nil || cur.IsUnknown() || (cur.IsNull() && !a.Block.Fixed) {
		cur = blockValue(a, nil)
	}
	server, present, err := decodeBlockSlots(a, m)
	if err != nil {
		return nil, err
	}
	if !present {
		return cur, nil
	}
	curElems := blockElems(cur)
	if len(curElems) == 0 {
		if honorPlanNulls {
			return cur, nil
		}
		elems := make([]map[string]attr.Value, len(server))
		for i, s := range server {
			elems[i] = s.fields
		}
		return blockValue(a, elems), nil
	}
	byKey := make(map[string]blockElem, len(server))
	for _, s := range server {
		byKey[s.key] = s
	}
	var out []map[string]attr.Value
	seen := map[string]bool{}
	for pos, ce := range curElems {
		key, err := blockElemKey(a.Block, ce, pos)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", a.Name, err)
		}
		seen[key] = true
		se, ok := byKey[key]
		switch {
		case !ok:
			// The server lacks the slot: the element is gone.
		case blockElemConfirmed(ce, se.fields):
			out = append(out, fillComputed(a, ce, se.fields))
		default:
			out = append(out, se.fields)
		}
	}
	for _, s := range server {
		if !seen[s.key] {
			out = append(out, s.fields)
		}
	}
	return blockValue(a, out), nil
}

// blockElemConfirmed reports whether every configured field of an element
// equals the server's decoded value.
func blockElemConfirmed(configured, server map[string]attr.Value) bool {
	for name, v := range configured {
		if usable(v) && !v.Equal(server[name]) {
			return false
		}
	}
	return true
}

// fillComputed keeps a configured element, filling its null or unknown
// computed fields from the server.
func fillComputed(a Attr, configured, server map[string]attr.Value) map[string]attr.Value {
	out := make(map[string]attr.Value, len(configured))
	maps.Copy(out, configured)
	for _, ea := range blockElemAttrs(a) {
		if v := out[ea.Name]; ea.Computed && (v == nil || v.IsNull() || v.IsUnknown()) {
			out[ea.Name] = server[ea.Name]
		}
	}
	return out
}

// blockFieldChange is one create-only block field whose plan value differs
// from the state on an element the plan keeps.
type blockFieldChange struct{ Key, Field, Was, Now string }

// blockDiff is the unified diff of a block attribute's plan against its
// state: dropped slots, and create-only fields changed on kept slots.
type blockDiff struct {
	Removed []string           // wire keys (and secondaries) in state, absent in plan
	Changed []blockFieldChange // create-only fields that differ on a kept element
}

// diffBlock compares a block attribute's plan and state values. Removed
// feeds PVE's `delete` request parameter (unsetting dropped slots and
// secondary-family slots the plan no longer sets); Changed feeds the
// create-only guard (PVE would ignore or reject the change; dedicated
// actions perform it).
func diffBlock(a Attr, plan, state attr.Value) blockDiff {
	spec := a.Block
	planned := map[string]map[string]attr.Value{}
	for pos, e := range blockElems(plan) {
		if key, err := blockElemKey(spec, e, pos); err == nil {
			planned[key] = e
		}
	}
	setsFamily := func(e map[string]attr.Value, fam string) bool {
		for _, f := range spec.Fields {
			if f.Family == fam && usable(e[f.Name]) {
				return true
			}
		}
		return false
	}
	var diff blockDiff
	for pos, e := range blockElems(state) {
		key, err := blockElemKey(spec, e, pos)
		if err != nil {
			continue
		}
		pe, kept := planned[key]
		if !kept {
			diff.Removed = append(diff.Removed, key)
		}
		idx := slotSuffix(spec, key)
		for _, fam := range secondaryFamilies(spec) {
			if setsFamily(e, fam) && (!kept || !setsFamily(pe, fam)) {
				diff.Removed = append(diff.Removed, fam+idx)
			}
		}
	}
	existing := map[string]map[string]attr.Value{}
	for pos, e := range blockElems(state) {
		if key, err := blockElemKey(spec, e, pos); err == nil {
			existing[key] = e
		}
	}
	for pos, pe := range blockElems(plan) {
		key, err := blockElemKey(spec, pe, pos)
		if err != nil {
			continue
		}
		se, ok := existing[key]
		if !ok {
			continue
		}
		for _, f := range spec.Fields {
			if !f.CreateOnly || f.Name == "" {
				continue
			}
			if pv, sv := pe[f.Name], se[f.Name]; usable(pv) && usable(sv) && !pv.Equal(sv) {
				diff.Changed = append(diff.Changed, blockFieldChange{Key: key, Field: f.Name, Was: scalarString(sv), Now: scalarString(pv)})
			}
		}
	}
	return diff
}
