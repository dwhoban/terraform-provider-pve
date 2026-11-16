// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dwhoban/terraform-provider-pve/internal/pveclient"
)

// genericResource implements every generated resource from a TypeDescriptor:
// standard collection CRUD, singletons (GET/PUT), PUT-created ACLs with
// filtered list reads, state-only deletes, opt-in apply of pending
// changes, and clone-at-create — all selected by descriptor data.
type genericResource struct {
	engineBase
}

var (
	_ resource.Resource                = (*genericResource)(nil)
	_ resource.ResourceWithConfigure   = (*genericResource)(nil)
	_ resource.ResourceWithImportState = (*genericResource)(nil)
)

// NewGenericResource returns the resource constructor for a descriptor.
func NewGenericResource(d *TypeDescriptor) func() resource.Resource {
	return func() resource.Resource { return &genericResource{engineBase: engineBase{desc: d}} }
}

func (r *genericResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = r.desc.Name
}

func (r *genericResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = buildResourceSchema(r.desc)
}

func (r *genericResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if err := r.setClient(req.ProviderData); err != nil {
		resp.Diagnostics.AddError("Unexpected Provider Configure Type", err.Error())
	}
}

func (r *genericResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	values := planValues(ctx, req.Plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.requireClient(); err != nil {
		resp.Diagnostics.AddError("Client Not Configured", err.Error())
		return
	}

	if diags := r.apply(ctx, applyCreate, values, nil); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, objectValue(r.desc, values))...)
}

// mutateCreate performs the create mutation — clone-at-create when a source
// is configured, the plain create endpoint otherwise.
func (r *genericResource) mutateCreate(ctx context.Context, values map[string]attr.Value) (json.RawMessage, bool, error) {
	d := r.desc
	c := d.Clone
	if c != nil && c.Op != nil && !values[c.SourceAttr].IsNull() {
		overrides := map[string]attr.Value{c.SourceParam: values[c.SourceAttr]}
		path, err := resolvePath(d, c.Op, values, overrides)
		if err != nil {
			return nil, false, err
		}
		params := url.Values{}
		if len(d.IDAttrs) == 0 {
			return nil, false, fmt.Errorf("type %s: clone requires an id attribute", d.Name)
		}
		idName := d.IDAttrs[len(d.IDAttrs)-1]
		if ss, err := valueParamStrings(values[idName]); err != nil {
			return nil, false, err
		} else if len(ss) != 1 {
			return nil, false, fmt.Errorf("clone requires a concrete %s", idName)
		} else {
			params[c.NewIDParam] = ss
		}
		for _, name := range c.Op.Params {
			if len(c.OnlyParams) > 0 && !slices.Contains(c.OnlyParams, name) {
				continue
			}
			if a, ok := d.Attr(name); ok {
				if ss, err := valueParamStrings(values[name]); err == nil && len(ss) > 0 {
					params[a.TFName()] = ss
				}
			}
		}
		for k, v := range c.ConstParams {
			params[k] = []string{v}
		}
		return executeOp(ctx, r.client, c.Op, path, params, nil)
	}
	return runOp(ctx, r.client, d, d.Create, values)
}

// applyPending commits the node's pending changes after a create or update (node network): it runs the mutation op's chained Apply endpoint when the op's ApplyIf attribute is true. No-op otherwise.
func (r *genericResource) applyPending(ctx context.Context, op *Op, values map[string]attr.Value) error {
	if op == nil || op.Apply == nil {
		return nil
	}
	if b, ok := values[op.ApplyIf].(types.Bool); !ok || b.IsNull() || b.IsUnknown() || !b.ValueBool() {
		return nil
	}
	if _, _, err := runOp(ctx, r.client, r.desc, op.Apply, values); err != nil {
		return fmt.Errorf("applying pending changes for %s (%s): %w", r.desc.Name, describeID(r.desc, values), err)
	}
	return nil
}

func (r *genericResource) readBack(ctx context.Context, values map[string]attr.Value) error {
	payload, _, err := runOp(ctx, r.client, r.desc, r.desc.Read, values)
	if err != nil {
		return err
	}
	ok, err := applyReadResponse(r.desc, r.desc.Read, payload, values, reconcileAfterApply)
	if err != nil {
		return err
	}
	if !ok {
		return errVanished
	}
	return nil
}

// errVanished marks a filtered read that no longer matches any entry; the
// framework-facing callers translate it to resource-gone handling.
var errVanished = fmt.Errorf("entry no longer present")

func (r *genericResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	values := stateValues(ctx, req.State, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.requireClient(); err != nil {
		resp.Diagnostics.AddError("Client Not Configured", err.Error())
		return
	}
	if err := fillIDAttrs(r.desc, values); err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("Error Reading %s", r.desc.Name), err.Error())
		return
	}
	payload, _, err := runOp(ctx, r.client, r.desc, r.desc.Read, values)
	if err != nil {
		if pveclient.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		opError(&resp.Diagnostics, "Reading", r.desc, describeID(r.desc, values), err)
		return
	}
	// Imported state carries only the id: required attributes are still
	// null (a managed refresh always has them set). Adopt the server's
	// values wholesale so import reconstructs the resource.
	mode := reconcileRefresh
	if importShape(r.desc, values) {
		mode = reconcileImport
	}
	ok, err := applyReadResponse(r.desc, r.desc.Read, payload, values, mode)
	if err != nil {
		opError(&resp.Diagnostics, "Reading", r.desc, describeID(r.desc, values), err)
		return
	}
	if !ok {
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, objectValue(r.desc, values))...)
}

// importShape reports whether the values look like a freshly imported
// resource: only the ID (and ID-composed attributes) carry values. A
// managed refresh always has the configured attributes populated, so this
// distinguishes import reads from refresh reads.
func importShape(d *TypeDescriptor, values map[string]attr.Value) bool {
	idAttrs := map[string]bool{"id": true}
	for _, n := range d.IDAttrs {
		idAttrs[n] = true
	}
	for _, a := range d.Attributes {
		if idAttrs[a.Name] {
			continue
		}
		if v, ok := values[a.Name]; ok && v != nil && !v.IsNull() {
			return false
		}
	}
	return true
}

func (r *genericResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	values := planValues(ctx, req.Plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.requireClient(); err != nil {
		resp.Diagnostics.AddError("Client Not Configured", err.Error())
		return
	}
	state := stateValues(ctx, req.State, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	if diags := r.apply(ctx, applyUpdate, values, state); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, objectValue(r.desc, values))...)
}

// createOnlyChange reports a create-only block field (a disk's storage or
// size) that the plan changes on an element the state already has: PVE
// would ignore or reject it, and the dedicated actions (resize, move)
// perform those changes.
func createOnlyChange(d *TypeDescriptor, plan, state map[string]attr.Value) error {
	for _, a := range d.Attributes {
		if a.Block == nil {
			continue
		}
		for _, ch := range diffBlock(a, plan[a.Name], state[a.Name]).Changed {
			return fmt.Errorf("%s %s: %s cannot change on an existing %s (was %s, now %s)", a.Name, ch.Key, ch.Field, a.Name, ch.Was, ch.Now)
		}
	}
	return nil
}

func (r *genericResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	if r.desc.Behavior == BehaviorStateOnlyDelete || r.desc.Delete == nil {
		return
	}
	values := stateValues(ctx, req.State, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.requireClient(); err != nil {
		resp.Diagnostics.AddError("Client Not Configured", err.Error())
		return
	}
	if err := fillIDAttrs(r.desc, values); err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("Error Deleting %s", r.desc.Name), err.Error())
		return
	}
	if _, _, err := runOp(ctx, r.client, r.desc, r.desc.Delete, values); err != nil && !pveclient.IsNotFound(err) {
		opError(&resp.Diagnostics, "Deleting", r.desc, describeID(r.desc, values), err)
		return
	}
}

func (r *genericResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// fillAllocatedID fills the first null ID attribute from the create
// response: the UPID id component when the server allocated it (e.g. qemu
// vmid), or a plain string response carrying the new identifier.
func (r *genericResource) fillAllocatedID(payload json.RawMessage, waited bool, values map[string]attr.Value) {
	s, ok := payloadString(payload)
	if !ok {
		return
	}
	if strings.HasPrefix(s, "UPID:") {
		if !waited || r.desc.Create == nil || r.desc.Create.IDFromUPID == "" {
			return
		}
		id := upidID(s)
		if id == "" {
			return
		}
		if a, found := r.desc.Attr(r.desc.Create.IDFromUPID); found {
			// Only fill server-allocated ids: never overwrite a concrete
			// value (a clone UPID carries the SOURCE id, not the new one).
			if cur := values[a.Name]; cur == nil || !usable(cur) {
				if v, err := decodeAttr(a, id); err == nil {
					values[a.Name] = v
				}
			}
		}
		return
	}
	for _, name := range r.desc.IDAttrs {
		if v := values[name]; v == nil || !usable(v) {
			if a, found := r.desc.Attr(name); found {
				if cv, err := decodeAttr(a, s); err == nil {
					values[name] = cv
				}
			}
			return
		}
	}
}

// fillIDAttrs reconstructs null path/id attributes from the resource id
// (imported resources carry only the id).
func fillIDAttrs(d *TypeDescriptor, values map[string]attr.Value) error {
	idVal, ok := values["id"]
	if !ok || idVal.IsNull() {
		return nil
	}
	id, ok := idVal.(types.String)
	if !ok || id.ValueString() == "" {
		return nil
	}
	parsed, err := parseID(d, id.ValueString())
	if err != nil {
		return err
	}
	for name, v := range parsed {
		if cur, ok := values[name]; !ok || cur.IsNull() {
			values[name] = v
		}
	}
	return nil
}

// planValues reads a tfsdk plan/config object into attribute values.
func planValues(ctx context.Context, v interface {
	Get(ctx context.Context, target any) diag.Diagnostics
}, diagnostics *diag.Diagnostics) map[string]attr.Value {
	return tfsdkValues(ctx, v, diagnostics)
}

// configValues reads a tfsdk config object into attribute values.
func configValues(ctx context.Context, v interface {
	Get(ctx context.Context, target any) diag.Diagnostics
}, diagnostics *diag.Diagnostics) map[string]attr.Value {
	return tfsdkValues(ctx, v, diagnostics)
}

// stateValues reads a tfsdk state object into attribute values.
func stateValues(ctx context.Context, v interface {
	Get(ctx context.Context, target any) diag.Diagnostics
}, diagnostics *diag.Diagnostics) map[string]attr.Value {
	return tfsdkValues(ctx, v, diagnostics)
}

func tfsdkValues(ctx context.Context, v interface {
	Get(ctx context.Context, target any) diag.Diagnostics
}, diagnostics *diag.Diagnostics) map[string]attr.Value {
	var obj types.Object
	if diags := v.Get(ctx, &obj); diags.HasError() {
		*diagnostics = append(*diagnostics, diags...)
		return nil
	}
	return mapFromObject(obj)
}

// mapFromObject normalizes an object's attributes with nulls for anything
// absent.
func mapFromObject(obj types.Object) map[string]attr.Value {
	out := map[string]attr.Value{}
	if !obj.IsNull() {
		for name, v := range obj.Attributes() {
			out[name] = v
		}
	}
	return out
}

// objectValue renders attribute values as the framework state object,
// nulling anything missing.
func objectValue(d *TypeDescriptor, values map[string]attr.Value) types.Object {
	attrs := attrTypesOf(d.Attributes)
	attrs["id"] = types.StringType
	fields := make(map[string]attr.Value, len(attrs))
	for name, t := range attrs {
		if v, ok := values[name]; ok && v != nil {
			fields[name] = v
		} else {
			fields[name] = nullOfType(t)
		}
	}
	return types.ObjectValueMust(attrs, fields)
}

func nullOfType(t attr.Type) attr.Value {
	switch ct := t.(type) {
	case types.ListType:
		return types.ListNull(ct.ElemType)
	case types.SetType:
		return types.SetNull(ct.ElemType)
	case types.ObjectType:
		return types.ObjectNull(ct.AttrTypes)
	}
	switch t {
	case types.BoolType:
		return types.BoolNull()
	case types.Int64Type:
		return types.Int64Null()
	case types.Float64Type:
		return types.Float64Null()
	default:
		return types.StringNull()
	}
}

func describeID(d *TypeDescriptor, values map[string]attr.Value) string {
	if id, ok := values["id"].(types.String); ok && !id.IsNull() && id.ValueString() != "" {
		return id.ValueString()
	}
	return describeValues(d, values)
}

func describeValues(d *TypeDescriptor, values map[string]attr.Value) string {
	parts := make([]string, 0, len(d.IDAttrs))
	for _, n := range d.IDAttrs {
		if ss, err := valueParamStrings(values[n]); err == nil && len(ss) > 0 {
			parts = append(parts, strings.Join(ss, ","))
		}
	}
	return strings.Join(parts, "/")
}
