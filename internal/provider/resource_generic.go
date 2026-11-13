// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
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
// filtered list reads, state-only deletes, staged applies, and clone-at-
// create — all selected by descriptor data.
type genericResource struct {
	desc   *TypeDescriptor
	client *pveclient.Client
}

var (
	_ resource.Resource                = (*genericResource)(nil)
	_ resource.ResourceWithConfigure   = (*genericResource)(nil)
	_ resource.ResourceWithImportState = (*genericResource)(nil)
)

// NewGenericResource returns the resource constructor for a descriptor.
func NewGenericResource(d *TypeDescriptor) func() resource.Resource {
	return func() resource.Resource { return &genericResource{desc: d} }
}

func (r *genericResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = r.desc.Name
}

func (r *genericResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = buildResourceSchema(r.desc)
}

func (r *genericResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*pveclient.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Provider Configure Type",
			fmt.Sprintf("Expected *pveclient.Client for %s, got: %T.", r.desc.Name, req.ProviderData),
		)
		return
	}
	r.client = c
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

	payload, waited, err := r.mutateCreate(ctx, values)
	if err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Error Creating %s", r.desc.Name),
			fmt.Sprintf("creating %s (%s): %s", r.desc.Name, describeValues(r.desc, values), err),
		)
		return
	}
	r.fillAllocatedID(payload, waited, values)
	if s, ok := payloadString(payload); ok && r.desc.Create != nil && r.desc.Create.ResultAttr != "" {
		if a, found := r.desc.Attr(r.desc.Create.ResultAttr); found {
			values[a.Name] = types.StringValue(s)
		}
	}

	if r.desc.Read != nil {
		if err := r.readBack(ctx, values); err != nil {
			resp.Diagnostics.AddError(
				fmt.Sprintf("Error Reading %s After Create", r.desc.Name),
				fmt.Sprintf("reading %s (%s): %s", r.desc.Name, describeValues(r.desc, values), err),
			)
			return
		}
	}

	id, err := joinID(r.desc, values)
	if err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("Error Creating %s", r.desc.Name), err.Error())
		return
	}
	if id == "" {
		id = r.desc.Name // singletons: one instance per type
	}
	values["id"] = types.StringValue(id)
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
			if len(c.OnlyParams) > 0 && !containsString(c.OnlyParams, name) {
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

func containsString(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func (r *genericResource) readBack(ctx context.Context, values map[string]attr.Value) error {
	payload, _, err := runOp(ctx, r.client, r.desc, r.desc.Read, values)
	if err != nil {
		return err
	}
	ok, err := applyReadResponse(r.desc, r.desc.Read, payload, values, true)
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
		resp.Diagnostics.AddError(
			fmt.Sprintf("Error Reading %s", r.desc.Name),
			fmt.Sprintf("reading %s (%s): %s", r.desc.Name, describeID(r.desc, values), err),
		)
		return
	}
	// Imported state carries only the id: required attributes are still
	// null (a managed refresh always has them set). Adopt the server's
	// values wholesale so import reconstructs the resource.
	importRead := importShape(r.desc, values)
	ok, err := applyReadResponse(r.desc, r.desc.Read, payload, values, !importRead)
	if err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Error Reading %s", r.desc.Name),
			fmt.Sprintf("reading %s (%s): %s", r.desc.Name, describeID(r.desc, values), err),
		)
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
	// Carry the id from state: plan id is (correctly) unknown until set.
	state := stateValues(ctx, req.State, &resp.Diagnostics)
	if state != nil {
		if id, ok := state["id"]; ok {
			values["id"] = id
		}
		// Computed path identifiers (e.g. an SDN subnet's internal id) are
		// unknown in the plan; the item endpoints need the state value.
		for _, a := range r.desc.Attributes {
			if a.PathParam == "" || !a.Computed || a.Optional {
				continue
			}
			if cur, ok := values[a.Name]; !ok || cur == nil || cur.IsUnknown() {
				if sv, ok := state[a.Name]; ok {
					values[a.Name] = sv
				}
			}
		}
	}
	if resp.Diagnostics.HasError() {
		return
	}
	if err := createOnlyChange(r.desc, values, state); err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("Error Updating %s", r.desc.Name), err.Error())
		return
	}
	if r.desc.Update != nil {
		var extras []url.Values
		if r.desc.Update.DeleteParam {
			if removed := removedParams(r.desc, r.desc.Update, values, state); len(removed) > 0 {
				extras = append(extras, url.Values{"delete": []string{strings.Join(removed, ",")}})
			}
		}
		payload, _, err := runOp(ctx, r.client, r.desc, r.desc.Update, values, extras...)
		if err != nil {
			resp.Diagnostics.AddError(
				fmt.Sprintf("Error Updating %s", r.desc.Name),
				fmt.Sprintf("updating %s (%s): %s", r.desc.Name, describeID(r.desc, values), err),
			)
			return
		}
		captureResultAttr(r.desc.Update, payload, values)
		if r.desc.Update.Apply != nil {
			if _, _, err := runOp(ctx, r.client, r.desc, r.desc.Update.Apply, values); err != nil {
				resp.Diagnostics.AddError(
					fmt.Sprintf("Error Applying %s Changes", r.desc.Name),
					fmt.Sprintf("applying pending changes for %s (%s): %s", r.desc.Name, describeID(r.desc, values), err),
				)
				return
			}
		}
	}
	if r.desc.Read != nil {
		if err := r.readBack(ctx, values); err != nil && err != errVanished && !pveclient.IsNotFound(err) {
			resp.Diagnostics.AddError(
				fmt.Sprintf("Error Reading %s After Update", r.desc.Name),
				fmt.Sprintf("reading %s (%s): %s", r.desc.Name, describeID(r.desc, values), err),
			)
			return
		}
	}
	id, err := joinID(r.desc, values)
	if err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("Error Updating %s", r.desc.Name), err.Error())
		return
	}
	if id == "" {
		id = r.desc.Name // singletons: one instance per type
	}
	values["id"] = types.StringValue(id)
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
		resp.Diagnostics.AddError(
			fmt.Sprintf("Error Deleting %s", r.desc.Name),
			fmt.Sprintf("deleting %s (%s): %s", r.desc.Name, describeID(r.desc, values), err),
		)
	}
}

func (r *genericResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *genericResource) requireClient() error {
	if r.client == nil {
		return fmt.Errorf("%s: provider client is not configured", r.desc.Name)
	}
	return nil
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
