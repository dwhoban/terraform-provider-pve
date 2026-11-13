// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dwhoban/terraform-provider-pve/internal/pveclient"
)

// This file is the framework-free core the generic engines share: filling
// endpoint paths from attribute values, building request parameters, waiting
// on tasks, composing IDs, and merging responses back into attribute values.
// Keeping it free of framework request/response types makes it directly
// unit-testable.

var pathPlaceholderRe = regexp.MustCompile(`\{([a-zA-Z0-9_-]+)\}`)

// runOp executes one descriptor operation against the API. pathOverrides
// replaces the value bound to a {placeholder} (used by clone-at-create,
// where {vmid} binds to the source guest). It returns the raw response data
// and whether a UPID task was awaited (the payload is then the UPID string,
// left for callers that need its id component).
func runOp(ctx context.Context, client *pveclient.Client, d *TypeDescriptor, op *Op, values map[string]attr.Value, extraParams ...url.Values) (json.RawMessage, bool, error) {
	if op == nil {
		return nil, false, fmt.Errorf("type %s: operation not configured", d.Name)
	}
	path, err := resolvePath(d, op, values, nil)
	if err != nil {
		return nil, false, err
	}
	params, err := buildParams(d, op, values)
	if err != nil {
		return nil, false, err
	}
	for _, extra := range extraParams {
		for k, vs := range extra {
			params[k] = vs
		}
	}
	return executeOp(ctx, client, op, path, params, nil)
}

// executeOp sends a fully-built request and applies the task-wait policy.
func executeOp(ctx context.Context, client *pveclient.Client, op *Op, path string, params url.Values, onWait func(time.Duration, string)) (json.RawMessage, bool, error) {
	payload, err := client.DoRequest(ctx, op.Method, path, params)
	if err != nil {
		return nil, false, fmt.Errorf("%s %s: %w%s", op.Method, path, err, cephHint(path, err))
	}
	if op.WaitTask {
		upid, ok := payloadString(payload)
		if ok && strings.HasPrefix(upid, "UPID:") {
			opts := pveclient.TaskPollOptions{Timeout: client.TaskTimeout()}
			if onWait != nil {
				opts.OnWait = onWait
			}
			if werr := client.WaitForTask(ctx, upid, opts); werr != nil {
				return payload, true, fmt.Errorf("%s %s: task %s failed: %w", op.Method, path, upid, werr)
			}
			return payload, true, nil
		}
	}
	return payload, false, nil
}

// cephHint explains PVE's "binary not installed" answer on Ceph endpoints:
// the API cannot install packages, so point at the action that can.
func cephHint(path string, err error) string {
	if strings.Contains(path, "/ceph/") && strings.Contains(err.Error(), "binary not installed") {
		return "\n\nCeph is not installed on this node. Install it with the pve_ceph_install action (needs the provider's ssh block) or run `pveceph install` on the node."
	}
	return ""
}

// resolvePath picks the scoped path (when the type is scoped) and fills
// {placeholders} from attribute values: an explicit PathParam binding wins,
// then an attribute of the same name, then a pathOverrides entry.
func resolvePath(d *TypeDescriptor, op *Op, values map[string]attr.Value, overrides map[string]attr.Value) (string, error) {
	path := op.Path
	if len(op.Paths) > 0 {
		scopeVal, ok := values["scope"]
		if !ok || scopeVal.IsNull() {
			return "", fmt.Errorf("type %s: scope attribute is required", d.Name)
		}
		scope, ok := scopeVal.(types.String)
		if !ok {
			return "", fmt.Errorf("type %s: scope attribute must be a string", d.Name)
		}
		p, ok := op.Paths[scope.ValueString()]
		if !ok {
			return "", fmt.Errorf("type %s: unknown scope %q", d.Name, scope.ValueString())
		}
		path = p
	}
	var missing []string
	filled := pathPlaceholderRe.ReplaceAllStringFunc(path, func(ph string) string {
		name := ph[1 : len(ph)-1]
		for _, candidate := range []attr.Value{overrides[name], placeholderValue(d, values, name)} {
			if candidate == nil || candidate.IsNull() {
				continue
			}
			if ss, err := valueParamStrings(candidate); err == nil && len(ss) > 0 {
				return ss[0]
			}
		}
		missing = append(missing, name)
		return ph
	})
	if len(missing) > 0 {
		return "", fmt.Errorf("type %s: missing value(s) for path parameter(s): %s", d.Name, strings.Join(missing, ", "))
	}
	return filled, nil
}

// pathParamsReady reports whether every placeholder of the op's (first)
// path resolves to a non-null value.
func pathParamsReady(d *TypeDescriptor, op *Op, values map[string]attr.Value) bool {
	path := op.Path
	if path == "" {
		for _, p := range op.Paths {
			path = p
			break
		}
	}
	ok := true
	pathPlaceholderRe.ReplaceAllStringFunc(path, func(ph string) string {
		name := ph[1 : len(ph)-1]
		if placeholderValue(d, values, name) == nil {
			ok = false
		}
		return ph
	})
	return ok
}

// skipValuesReady reports whether all SkipIfMissing attributes are set to
// concrete values.
func skipValuesReady(op *Op, values map[string]attr.Value) bool {
	for _, name := range op.SkipIfMissing {
		v, ok := values[name]
		if !ok || !usable(v) {
			return false
		}
	}
	return true
}

// placeholderValue finds the concrete value bound to a path placeholder
// (unknown plan values do not count — the request would target a garbage
// path).
func placeholderValue(d *TypeDescriptor, values map[string]attr.Value, placeholder string) attr.Value {
	for _, a := range d.Attributes {
		if a.PathParam == placeholder {
			if v, ok := values[a.Name]; ok && usable(v) {
				return v
			}
		}
	}
	if v, ok := values[placeholder]; ok && usable(v) {
		return v
	}
	return nil
}

// opBindsPlaceholder reports whether the operation's path templates
// contain the given placeholder.
func opBindsPlaceholder(op *Op, placeholder string) bool {
	for _, tmpl := range opPathTemplates(op) {
		if strings.Contains(tmpl, "{"+placeholder+"}") {
			return true
		}
	}
	return false
}

// opPathTemplates lists the path templates an operation may target.
func opPathTemplates(op *Op) []string {
	out := []string{}
	if op.Path != "" {
		out = append(out, op.Path)
	}
	for _, p := range op.Paths {
		out = append(out, p)
	}
	return out
}

func buildParams(d *TypeDescriptor, op *Op, values map[string]attr.Value) (url.Values, error) {
	params := url.Values{}
	for _, name := range op.Params {
		a, ok := d.Attr(name)
		if !ok {
			// Functions carry their inputs as positional arguments rather
			// than attributes.
			for _, fa := range d.FuncArgs {
				if fa.Name == name {
					a, ok = fa, true
					break
				}
			}
		}
		if !ok {
			continue
		}
		// Attributes bound to a placeholder in this operation's path
		// TEMPLATE fill the URL; PVE rejects them duplicated as request
		// parameters. (The same attribute may legitimately be a body
		// parameter on a collection create, e.g. POST /pools poolid.)
		if a.PathParam != "" && opBindsPlaceholder(op, a.PathParam) {
			continue
		}
		if a.Block != nil {
			slots, err := encodeBlock(a, values[name])
			if err != nil {
				return nil, fmt.Errorf("type %s: %w", d.Name, err)
			}
			maps.Copy(params, slots)
			continue
		}
		ss, err := encodeAttr(a, values[name])
		if err != nil || len(ss) == 0 {
			continue
		}
		params[a.TFName()] = ss
	}
	for k, v := range op.ConstParams {
		params[k] = []string{v}
	}
	return params, nil
}

// removedParams lists API field names that were concrete in state but are
// null/absent in the plan: attribute removals that must be unset
// server-side via PVE's canonical `delete` request parameter. Block
// attributes contribute every slot (and secondary slot) of an element
// removed from configuration.
func removedParams(d *TypeDescriptor, op *Op, plan, state map[string]attr.Value) []string {
	var removed []string
	for _, name := range op.Params {
		a, ok := d.Attr(name)
		if !ok || a.Required {
			continue // required attributes are always configured
		}
		if a.Block != nil {
			removed = append(removed, diffBlock(a, plan[name], state[name]).Removed...)
			continue
		}
		if usable(state[name]) && !usable(plan[name]) {
			removed = append(removed, a.TFName())
		}
	}
	return removed
}

// upidID extracts the id component of a raw UPID (the server-allocated
// guest id on create, for example).
func upidID(raw string) string {
	u, err := pveclient.ParseUPID(raw)
	if err != nil {
		return ""
	}
	return u.ID
}

// joinID renders the resource ID from the ID attributes. Null values render
// as empty components so positions stay stable.
func joinID(d *TypeDescriptor, values map[string]attr.Value) (string, error) {
	sep := idSep(d)
	parts := make([]string, 0, len(d.IDAttrs))
	for _, name := range d.IDAttrs {
		v := values[name]
		ss, err := valueParamStrings(v)
		if err != nil {
			return "", fmt.Errorf("id attribute %s: %w", name, err)
		}
		if len(ss) == 0 {
			parts = append(parts, "")
			continue
		}
		parts = append(parts, strings.Join(ss, ","))
	}
	return strings.Join(parts, sep), nil
}

// parseID splits a resource ID back into attribute values, positionally by
// IDAttrs.
func parseID(d *TypeDescriptor, id string) (map[string]attr.Value, error) {
	sep := idSep(d)
	parts := strings.SplitN(id, sep, max(len(d.IDAttrs), 1))
	if len(d.IDAttrs) == 0 {
		// Identity-less singletons carry an opaque id.
		return map[string]attr.Value{}, nil
	}
	if len(parts) != len(d.IDAttrs) {
		return nil, fmt.Errorf("invalid %s id %q: want %d components separated by %q (example: %s)", d.Name, id, len(d.IDAttrs), sep, idExample(d))
	}
	out := map[string]attr.Value{}
	for i, name := range d.IDAttrs {
		a, ok := d.Attr(name)
		if !ok {
			return nil, fmt.Errorf("type %s: id references unknown attribute %q", d.Name, name)
		}
		v, err := stringToValue(parts[i], a)
		if err != nil {
			return nil, fmt.Errorf("invalid %s id %q: attribute %s: %w", d.Name, id, name, err)
		}
		out[name] = v
	}
	return out, nil
}

func idSep(d *TypeDescriptor) string {
	if d.IDSep != "" {
		return d.IDSep
	}
	return "/"
}

func idExample(d *TypeDescriptor) string {
	parts := make([]string, len(d.IDAttrs))
	for i, n := range d.IDAttrs {
		parts[i] = "<" + n + ">"
	}
	return strings.Join(parts, idSep(d))
}

func stringToValue(s string, a Attr) (attr.Value, error) {
	if s == "" {
		return nullValue(a), nil
	}
	return decodeAttr(a, s)
}

// reconcileMode selects how a read response merges into current values.
type reconcileMode string

const (
	reconcileAfterApply reconcileMode = "after-apply" // create/update read-backs: plan wins for unset plain-Optional attrs
	reconcileRefresh    reconcileMode = "refresh"     // managed read: same merge rules today
	reconcileImport     reconcileMode = "import"      // identity-only state: adopt the server's values wholesale
)

// applyReadResponse merges a read response payload into values. See
// mergeObject for the reconcileMode semantics.
//
//   - Filtered reads (op.Filter) select one object from an array response;
//     not finding a match is reported via ok=false (caller treats it as
//     gone).
//   - Array responses land whole under the attribute named by op.ResultName.
//   - Object responses with ResultName land whole under that attribute.
//   - Object responses without ResultName merge field-by-field: response
//     fields override; absent fields null out Computed-only attributes and
//     leave Optional attributes (the server may not echo secrets/defaults).
func applyReadResponse(d *TypeDescriptor, op *Op, payload json.RawMessage, values map[string]attr.Value, mode reconcileMode) (ok bool, err error) {
	if len(payload) == 0 {
		// Endpoints answering null still describe the object (PVE does this
		// for empty configs); keep current values.
		return true, nil
	}
	v, err := decodeJSON(payload)
	if err != nil {
		return false, fmt.Errorf("type %s: decoding read response: %w", d.Name, err)
	}

	// A string ResultAttr/ResultName carries the raw payload when the API
	// schema does not describe the response shape.
	if op.ResultName != "" {
		if a, ok := d.Attr(op.ResultName); ok && a.Type == AttrString {
			values[op.ResultName] = types.StringValue(decodeResultString(payload))
			return true, nil
		}
	}

	if len(op.Filter) > 0 {
		items, isArr := v.([]any)
		if !isArr {
			return false, fmt.Errorf("type %s: filtered read expected an array response", d.Name)
		}
		for _, item := range items {
			m, isObj := item.(map[string]any)
			if !isObj {
				continue
			}
			if filterMatches(d, op, m, values) {
				if err := mergeObject(d, m, values, mode); err != nil {
					return false, err
				}
				return true, nil
			}
		}
		return false, nil
	}

	if arr, isArr := v.([]any); isArr {
		a, ok := d.Attr(op.ResultName)
		if !ok {
			return false, fmt.Errorf("type %s: array response but no result attribute %q", d.Name, op.ResultName)
		}
		coerced, err := decodeAttr(a, arr)
		if err != nil {
			return false, err
		}
		values[op.ResultName] = coerced
		return true, nil
	}

	m, isObj := v.(map[string]any)
	if !isObj {
		return false, fmt.Errorf("type %s: unexpected read response shape %T", d.Name, v)
	}
	if op.ResultName != "" {
		a, ok := d.Attr(op.ResultName)
		if !ok {
			return false, fmt.Errorf("type %s: no result attribute %q", d.Name, op.ResultName)
		}
		coerced, err := decodeAttr(a, m)
		if err != nil {
			return false, err
		}
		values[op.ResultName] = coerced
		return true, nil
	}
	if err := mergeObject(d, m, values, mode); err != nil {
		return false, err
	}
	return true, nil
}

func filterMatches(d *TypeDescriptor, op *Op, item map[string]any, values map[string]attr.Value) bool {
	for _, rule := range op.Filter {
		field, ok := item[rule.Field]
		if !ok {
			return false
		}
		a, ok := d.Attr(rule.Attr)
		if !ok {
			return false
		}
		want, err := encodeAttr(a, values[rule.Attr])
		if err != nil || len(want) != 1 {
			return false
		}
		got, err := decodeAttr(Attr{Name: rule.Attr, Type: AttrString}, field)
		if err != nil {
			return false
		}
		gotStr, ok := got.(types.String)
		if !ok || gotStr.ValueString() != want[0] {
			return false
		}
	}
	return true
}

// mergeObject merges response object fields into attribute values. A
// response field that cannot be decoded into its attribute is an error.
//
//   - Unknown current values (unset Optional+Computed attributes) become
//     null when the response omits them — state must never contain
//     unknowns after apply.
//   - A configured property string that already describes an
//     object-valued response field is kept as configured.
//   - Attributes carrying one-time results (an op's ResultAttr, e.g. an
//     API token value the server never echoes again) keep their current
//     value when the response omits them.
//   - In after-apply mode (create/update read-backs), a plain-Optional
//     attribute whose plan value is null stays null — Terraform requires
//     final state to match the plan for non-computed attributes, so
//     server-side defaults are not adopted. In refresh and import mode,
//     present response fields are always adopted — that is how state is
//     populated and drift surfaces.
func mergeObject(d *TypeDescriptor, m map[string]any, values map[string]attr.Value, mode reconcileMode) error {
	preserve := d.resultAttrs()
	for _, a := range d.Attributes {
		cur, hasCur := values[a.Name]
		field, present := m[a.EchoKey()]
		if a.Block != nil {
			merged, err := mergeBlock(a, cur, m, mode)
			if err != nil {
				return fmt.Errorf("type %s: attribute %s: %w", d.Name, a.Name, err)
			}
			values[a.Name] = merged
			continue
		}
		if hasCur && cur != nil && cur.IsUnknown() {
			// Unset Optional+Computed attributes are unknown in plans;
			// the response (server default) resolves them, otherwise they
			// become null — state never carries unknowns.
			if present {
				cv, err := decodeAttr(a, field)
				if err != nil {
					return fmt.Errorf("type %s: attribute %s: %w", d.Name, a.Name, err)
				}
				values[a.Name] = cv
				continue
			}
			values[a.Name] = nullValue(a)
			continue
		}
		if present {
			if mode == reconcileAfterApply && hasCur && cur != nil && cur.IsNull() && a.Optional && !a.Computed {
				continue // honor the plan: attribute deliberately unset
			}
			// PVE echoes empty strings for unset options; adopting them
			// into a null plain-optional would skew imports against
			// managed state.
			if mode != reconcileAfterApply && hasCur && (cur == nil || cur.IsNull()) && a.Optional && !a.Computed {
				if str, isStr := field.(string); isStr && str == "" {
					continue
				}
			}
			if wireEqual(a, cur, field) {
				continue
			}
			cv, err := decodeAttr(a, field)
			if err != nil {
				return fmt.Errorf("type %s: attribute %s: %w", d.Name, a.Name, err)
			}
			values[a.Name] = cv
			continue
		}
		if preserve[a.Name] {
			continue // one-time result: never null it out on read-back
		}
		if a.Computed && !a.Optional && !a.Required {
			values[a.Name] = nullValue(a)
		}
	}
	return nil
}

// captureResultAttr stores a one-time operation result into its attribute:
// a bare string response directly, or the object field named by ResultField
// (e.g. the regenerated token value from PUT /access/users/…/token/…).
func captureResultAttr(op *Op, payload json.RawMessage, values map[string]attr.Value) {
	if op == nil || op.ResultAttr == "" || len(payload) == 0 {
		return
	}
	if s, ok := payloadString(payload); ok {
		values[op.ResultAttr] = types.StringValue(s)
		return
	}
	if op.ResultField != "" {
		if v, err := decodeJSON(payload); err == nil {
			if m, isObj := v.(map[string]any); isObj {
				if field, present := m[op.ResultField]; present && field != nil {
					values[op.ResultAttr] = types.StringValue(fmt.Sprintf("%v", field))
				}
			}
		}
	}
}
