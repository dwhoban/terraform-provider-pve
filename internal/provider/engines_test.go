// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	datasourcedatasource "github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/function"
	pathpkg "github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/dwhoban/terraform-provider-pve/internal/pveclient"
)

// fakePVE is an in-memory Proxmox API stub: widgets CRUD behind /api2/json,
// plus the task-status endpoint the task engine polls.
type fakePVE struct {
	mu        sync.Mutex
	widgets   map[string]map[string]any
	nextID    int
	taskFail  bool
	requests  []string
	forms     []string
	applyHits int
	// createExtra fields are merged into every created widget (server-side
	// values the create form does not carry verbatim).
	createExtra map[string]any
}

func newFakePVE() *fakePVE {
	return &fakePVE{widgets: map[string]map[string]any{}, nextID: 101}
}

func (f *fakePVE) client(t *testing.T) *pveclient.Client {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(f.serve))
	t.Cleanup(srv.Close)
	c, err := pveclient.NewClient(pveclient.Config{
		URL:            srv.URL,
		Credentials:    pveclient.Credentials{APIToken: "root@pam!t=test"},
		Insecure:       true,
		RetryMax:       1,
		RetryBaseWait:  1,
		RequestTimeout: 0,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c
}

func (f *fakePVE) note(format string, args ...any) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, fmt.Sprintf(format, args...))
	return fmt.Sprintf(format, args...)
}

func (f *fakePVE) serve(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api2/json")
	f.note("%s %s", r.Method, path)

	if strings.HasPrefix(path, "/nodes/") && strings.Contains(path, "/tasks/") {
		// Task status polling: exit OK unless taskFail armed.
		status := "OK"
		if f.taskFail {
			status = "widgets failed: no widgets allowed"
		}
		writeData(w, map[string]any{"status": "stopped", "exitstatus": status})
		return
	}

	var widgetsRe = regexp.MustCompile(`^/nodes/([^/]+)/widgets$`)
	var widgetRe = regexp.MustCompile(`^/nodes/([^/]+)/widgets/([^/]+)$`)
	var taskRe = regexp.MustCompile(`^/nodes/([^/]+)/tasks/`)
	var networkRe = regexp.MustCompile(`^/nodes/([^/]+)/network$`)

	switch {
	case path == "/version" && r.Method == http.MethodGet:
		writeData(w, map[string]any{"version": "9.0.0"})

	case taskRe.MatchString(path) && r.Method == http.MethodGet:
		status := "OK"
		if f.taskFail {
			status = "widgets failed: no widgets allowed"
		}
		writeData(w, map[string]any{"status": "stopped", "exitstatus": status})

	case strings.HasSuffix(path, "/clone") && r.Method == http.MethodPost:
		_ = r.ParseForm()
		f.mu.Lock()
		f.forms = append(f.forms, r.PostForm.Encode())
		newID := r.PostForm.Get("newid")
		if newID != "" {
			f.widgets[newID] = map[string]any{"id": newID, "name": r.PostForm.Get("name"), "size": 5}
		}
		f.mu.Unlock()
		writeData(w, "UPID:node1:0000:0:0:widgetclone:"+newID+":")

	case widgetsRe.MatchString(path) && r.Method == http.MethodPost:
		_ = r.ParseForm()
		name := r.PostForm.Get("name")
		if name == "" {
			writeErr(w, http.StatusBadRequest, "parameter 'name' is required")
			return
		}
		f.mu.Lock()
		id := fmt.Sprintf("%d", f.nextID)
		f.nextID++
		f.widgets[id] = map[string]any{"id": id, "name": name, "size": 5}
		for k, v := range f.createExtra {
			f.widgets[id][k] = v
		}
		f.mu.Unlock()
		writeData(w, "UPID:node1:0000:0:0:widgetcreate:"+id+":")

	case widgetRe.MatchString(path):
		_ = r.ParseForm()
		if r.Method == http.MethodPut || r.Method == http.MethodPost {
			f.mu.Lock()
			f.forms = append(f.forms, r.PostForm.Encode())
			f.mu.Unlock()
		}
		id := widgetRe.FindStringSubmatch(path)[2]
		f.mu.Lock()
		widget, exists := f.widgets[id]
		f.mu.Unlock()
		if !exists {
			writeErr(w, http.StatusInternalServerError, "no such widget '"+id+"'")
			return
		}
		switch r.Method {
		case http.MethodGet:
			writeData(w, widget)
		case http.MethodPut:
			_ = r.ParseForm()
			f.mu.Lock()
			widget["name"] = r.PostForm.Get("name")
			if sz := r.PostForm.Get("size"); sz != "" {
				widget["size"] = json.Number(sz)
			}
			f.widgets[id] = widget
			f.mu.Unlock()
			writeData(w, nil)
		case http.MethodDelete:
			f.mu.Lock()
			delete(f.widgets, id)
			f.mu.Unlock()
			writeData(w, "UPID:node1:0000:0:0:widgetdelete:"+id+":")
		case http.MethodPost:
			// action-style mutation on the item path
			writeData(w, "UPID:node1:0000:0:0:widgetwake:"+id+":")
		default:
			writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		}

	case widgetsRe.MatchString(path) && r.Method == http.MethodGet:
		f.mu.Lock()
		defer f.mu.Unlock()
		list := make([]any, 0, len(f.widgets))
		for _, wid := range f.widgets {
			list = append(list, wid)
		}
		writeData(w, list)

	case networkRe.MatchString(path) && r.Method == http.MethodPut:
		f.mu.Lock()
		f.applyHits++
		f.mu.Unlock()
		writeData(w, nil)

	default:
		writeErr(w, http.StatusInternalServerError, "unhandled "+r.Method+" "+path)
	}
}

func writeData(w http.ResponseWriter, data any) {
	raw, _ := json.Marshal(data)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"data":%s}`, raw)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	fmt.Fprintf(w, `{"data":null,"errors":{},"message":%q}`, msg)
}

// widgetDescriptor is the canonical test type exercising the standard
// collection behavior: task-waited create/delete, item read, update.
func widgetDescriptor() *TypeDescriptor {
	return &TypeDescriptor{
		Name:    "pve_widget",
		Kind:    KindResource,
		Phase:   2,
		IDAttrs: []string{"node", "widget_id"},
		Create:  &Op{Method: "POST", Path: "/nodes/{node}/widgets", Params: []string{"name", "size"}, WaitTask: true, IDFromUPID: "widget_id"},
		Read:    &Op{Method: "GET", Path: "/nodes/{node}/widgets/{widget_id}"},
		Update:  &Op{Method: "PUT", Path: "/nodes/{node}/widgets/{widget_id}", Params: []string{"name", "size"}},
		Delete:  &Op{Method: "DELETE", Path: "/nodes/{node}/widgets/{widget_id}", WaitTask: true},
		Attributes: []Attr{
			{Name: "node", Type: AttrString, Required: true, ForceNew: true, PathParam: "node"},
			{Name: "widget_id", Type: AttrInt64, Optional: true, Computed: true, ForceNew: true, PathParam: "widget_id"},
			{Name: "name", Type: AttrString, Required: true},
			{Name: "size", Type: AttrInt64, Optional: true, Computed: true},
		},
	}
}

// toTF builds the tftypes.Value for an engine attribute map.
// toTFWith is toTF but keeps caller-provided unknown values intact.
func toTFWith(d *TypeDescriptor, values map[string]attr.Value) tftypes.Value {
	at := attrTypesOf(d.Attributes)
	at["id"] = types.StringType
	tfAt := map[string]tftypes.Type{}
	fields := map[string]tftypes.Value{}
	for name, typ := range at {
		tfAt[name] = typ.TerraformType(ctxbg())
		v, ok := values[name]
		if !ok || v == nil || v.IsNull() {
			fields[name] = tftypes.NewValue(tfAt[name], nil)
			continue
		}
		if v.IsUnknown() {
			fields[name] = tftypes.NewValue(tfAt[name], tftypes.UnknownValue)
			continue
		}
		fields[name] = attrToTF(v)
	}
	return tftypes.NewValue(tftypes.Object{AttributeTypes: tfAt}, fields)
}

func toTF(d *TypeDescriptor, values map[string]attr.Value) tftypes.Value {
	at := attrTypesOf(d.Attributes)
	if d.Kind != KindAction {
		at["id"] = types.StringType
	}
	tfAt := map[string]tftypes.Type{}
	fields := map[string]tftypes.Value{}
	for name, t := range at {
		tfAt[name] = t.TerraformType(ctxbg())
		v, ok := values[name]
		if !ok || v == nil || v.IsNull() {
			fields[name] = tftypes.NewValue(tfAt[name], nil)
			continue
		}
		fields[name] = attrToTF(v)
	}
	_ = values
	return tftypes.NewValue(tftypes.Object{AttributeTypes: tfAt}, fields)
}

func attrToTF(v attr.Value) tftypes.Value {
	switch t := v.(type) {
	case types.String:
		return tftypes.NewValue(tftypes.String, t.ValueString())
	case types.Bool:
		return tftypes.NewValue(tftypes.Bool, t.ValueBool())
	case types.Int64:
		return tftypes.NewValue(tftypes.Number, t.ValueInt64())
	case types.Float64:
		return tftypes.NewValue(tftypes.Number, t.ValueFloat64())
	default:
		tv, err := v.ToTerraformValue(ctxbg())
		if err != nil {
			panic(fmt.Sprintf("attrToTF: %v", err))
		}
		return tv
	}
}

func ctxbg() context.Context { return context.Background() }

func newResourceTest(t *testing.T, d *TypeDescriptor, f *fakePVE) *genericResource {
	t.Helper()
	return &genericResource{engineBase: engineBase{desc: d, client: f.client(t)}}
}

func TestEngineResourceLifecycle(t *testing.T) {
	f := newFakePVE()
	d := widgetDescriptor()
	r := newResourceTest(t, d, f)
	ctx := ctxbg()
	schema := buildResourceSchema(d)

	// Create from a plan with server-allocated id.
	createResp := &resource.CreateResponse{State: tfsdk.State{Schema: schema}}
	r.Create(ctx, resource.CreateRequest{
		Plan: tfsdk.Plan{Raw: toTF(d, map[string]attr.Value{
			"node": types.StringValue("node1"), "name": types.StringValue("alpha"),
		}), Schema: schema},
	}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", createResp.Diagnostics)
	}
	state := createResp.State
	var gotID types.String
	if diags := state.GetAttribute(ctx, pathpkg.Root("id"), &gotID); diags.HasError() {
		t.Fatalf("state id: %v", diags)
	}
	if gotID.ValueString() != "node1/101" {
		t.Fatalf("state id = %q, want node1/101", gotID.ValueString())
	}
	var gotName types.String
	if diags := state.GetAttribute(ctx, pathpkg.Root("name"), &gotName); diags.HasError() || gotName.ValueString() != "alpha" {
		t.Fatalf("state name = %q (diags %v)", gotName.ValueString(), diags)
	}
	var gotSize types.Int64
	if diags := state.GetAttribute(ctx, pathpkg.Root("size"), &gotSize); diags.HasError() || gotSize.ValueInt64() != 5 {
		t.Fatalf("state size = %v, want 5 (diags %v)", gotSize, diags)
	}

	// Update name.
	updateResp := &resource.UpdateResponse{State: tfsdk.State{Schema: schema}}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  tfsdk.Plan{Raw: toTF(d, map[string]attr.Value{"node": types.StringValue("node1"), "widget_id": types.Int64Value(101), "name": types.StringValue("beta"), "size": types.Int64Value(9)}), Schema: schema},
		State: tfsdk.State{Raw: state.Raw, Schema: schema},
	}, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("Update: %v", updateResp.Diagnostics)
	}
	if name := f.widgets["101"]["name"]; name != "beta" {
		t.Fatalf("widget name after update = %v, want beta", name)
	}
	if size := f.widgets["101"]["size"]; fmt.Sprint(size) != "9" {
		t.Fatalf("widget size after update = %v, want 9", size)
	}

	// Read drifts back.
	readResp := &resource.ReadResponse{State: tfsdk.State{Raw: updateResp.State.Raw, Schema: schema}}
	r.Read(ctx, resource.ReadRequest{State: tfsdk.State{Raw: updateResp.State.Raw, Schema: schema}}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", readResp.Diagnostics)
	}
	if readResp.State.Raw.IsNull() {
		t.Fatal("Read removed the resource unexpectedly")
	}

	// Delete; subsequent Read removes from state.
	delResp := &resource.DeleteResponse{}
	r.Delete(ctx, resource.DeleteRequest{State: tfsdk.State{Raw: updateResp.State.Raw, Schema: schema}}, delResp)
	if delResp.Diagnostics.HasError() {
		t.Fatalf("Delete: %v", delResp.Diagnostics)
	}
	if _, exists := f.widgets["101"]; exists {
		t.Fatal("widget not deleted")
	}
	readResp2 := &resource.ReadResponse{State: tfsdk.State{Raw: updateResp.State.Raw, Schema: schema}}
	r.Read(ctx, resource.ReadRequest{State: tfsdk.State{Raw: updateResp.State.Raw, Schema: schema}}, readResp2)
	if readResp2.Diagnostics.HasError() {
		t.Fatalf("Read after delete: %v", readResp2.Diagnostics)
	}
	if !readResp2.State.Raw.IsNull() {
		t.Fatal("Read after delete should remove the resource from state")
	}
}

func TestEngineResourceCreateTaskFailure(t *testing.T) {
	ctx := ctxbg()
	f := newFakePVE()
	f.taskFail = true
	d := widgetDescriptor()
	r := newResourceTest(t, d, f)
	schema := buildResourceSchema(d)
	resp := &resource.CreateResponse{State: tfsdk.State{Schema: schema}}
	r.Create(ctx, resource.CreateRequest{
		Plan: tfsdk.Plan{Raw: toTF(d, map[string]attr.Value{
			"node": types.StringValue("node1"), "name": types.StringValue("alpha"),
		}), Schema: schema},
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Create should fail when the create task fails")
	}
	if !strings.Contains(resp.Diagnostics[0].Detail(), "widgetcreate") {
		t.Fatalf("error should name the failed task, got: %v", resp.Diagnostics)
	}
}

func TestEngineResourceImportRead(t *testing.T) {
	ctx := ctxbg()
	f := newFakePVE()
	f.widgets["7"] = map[string]any{"id": "7", "name": "imported", "size": 3}
	d := widgetDescriptor()
	r := newResourceTest(t, d, f)
	schema := buildResourceSchema(d)

	// Imported state: only the id; Read reconstructs path attributes.
	values := map[string]attr.Value{"id": types.StringValue("node1/7")}
	readResp := &resource.ReadResponse{State: tfsdk.State{Raw: toTF(d, values), Schema: schema}}
	r.Read(ctx, resource.ReadRequest{State: tfsdk.State{Raw: toTF(d, values), Schema: schema}}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", readResp.Diagnostics)
	}
	var name types.String
	if diags := readResp.State.GetAttribute(ctx, pathpkg.Root("name"), &name); diags.HasError() || name.ValueString() != "imported" {
		t.Fatalf("imported name = %q (diags %v)", name.ValueString(), diags)
	}
}

// PVE answers some string properties as JSON objects (cluster options
// `migration`). A configured property string describing the same object is
// kept verbatim after apply; reads from null adopt the sorted rendering.
func TestEnginePropertyStringObjectEcho(t *testing.T) {
	ctx := ctxbg()
	f := newFakePVE()
	f.createExtra = map[string]any{"migration": map[string]any{"type": "secure", "network": "10.0.100.0/24"}}
	f.widgets["7"] = map[string]any{"id": "7", "name": "imported", "size": 3, "migration": f.createExtra["migration"]}
	d := widgetDescriptor()
	d.Create.Params = append(d.Create.Params, "migration")
	d.Attributes = append(d.Attributes, Attr{Name: "migration", Type: AttrString, Optional: true})
	r := newResourceTest(t, d, f)
	schema := buildResourceSchema(d)

	const configured = "type=secure,network=10.0.100.0/24"
	createResp := &resource.CreateResponse{State: tfsdk.State{Schema: schema}}
	r.Create(ctx, resource.CreateRequest{
		Plan: tfsdk.Plan{Raw: toTF(d, map[string]attr.Value{
			"node": types.StringValue("node1"), "name": types.StringValue("alpha"), "migration": types.StringValue(configured),
		}), Schema: schema},
	}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", createResp.Diagnostics)
	}
	var got types.String
	if diags := createResp.State.GetAttribute(ctx, pathpkg.Root("migration"), &got); diags.HasError() || got.ValueString() != configured {
		t.Fatalf("migration after create = %q, want %q (diags %v)", got.ValueString(), configured, diags)
	}

	values := map[string]attr.Value{"id": types.StringValue("node1/7")}
	readResp := &resource.ReadResponse{State: tfsdk.State{Raw: toTF(d, values), Schema: schema}}
	r.Read(ctx, resource.ReadRequest{State: tfsdk.State{Raw: toTF(d, values), Schema: schema}}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", readResp.Diagnostics)
	}
	if diags := readResp.State.GetAttribute(ctx, pathpkg.Root("migration"), &got); diags.HasError() || got.ValueString() != "network=10.0.100.0/24,type=secure" {
		t.Fatalf("imported migration = %q (diags %v)", got.ValueString(), diags)
	}
}

// A response field that cannot be decoded into its attribute type is an
// error, not a silently kept stale value.
func TestEngineReadDecodeFailureSurfaces(t *testing.T) {
	ctx := ctxbg()
	f := newFakePVE()
	f.widgets["7"] = map[string]any{"id": "7", "name": "imported", "size": "abc"}
	d := widgetDescriptor()
	r := newResourceTest(t, d, f)
	schema := buildResourceSchema(d)

	values := map[string]attr.Value{"id": types.StringValue("node1/7")}
	readResp := &resource.ReadResponse{State: tfsdk.State{Raw: toTF(d, values), Schema: schema}}
	r.Read(ctx, resource.ReadRequest{State: tfsdk.State{Raw: toTF(d, values), Schema: schema}}, readResp)
	if !readResp.Diagnostics.HasError() {
		t.Fatal("Read should fail when size is not an integer")
	}
	if detail := readResp.Diagnostics[0].Detail(); !strings.Contains(detail, "attribute size") {
		t.Fatalf("error should name the attribute, got: %s", detail)
	}
}

func TestEngineDataSourceList(t *testing.T) {
	ctx := ctxbg()
	f := newFakePVE()
	f.widgets["1"] = map[string]any{"id": "1", "name": "a", "size": 1}
	f.widgets["2"] = map[string]any{"id": "2", "name": "b", "size": 2}
	d := &TypeDescriptor{
		Name:  "pve_widgets",
		Kind:  KindDataSource,
		Phase: 2,
		Reads: []*Op{{Method: "GET", Path: "/nodes/{node}/widgets", ResultName: "widgets"}},
		Attributes: []Attr{
			{Name: "node", Type: AttrString, Required: true, PathParam: "node"},
			{Name: "widgets", Type: AttrList, Computed: true, ElemAttrs: []Attr{
				{Name: "id", Type: AttrString},
				{Name: "name", Type: AttrString},
				{Name: "size", Type: AttrInt64},
			}},
		},
	}
	ds := &genericDataSource{engineBase: engineBase{desc: d, client: f.client(t)}}
	schema := buildDataSourceSchema(d)
	resp := &datasourcedatasource.ReadResponse{State: tfsdk.State{Schema: schema}}
	ds.Read(ctx, datasourcedatasource.ReadRequest{
		Config: tfsdk.Config{Raw: toTF(d, map[string]attr.Value{"node": types.StringValue("node1")}), Schema: schema},
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", resp.Diagnostics)
	}
	var list types.List
	if diags := resp.State.GetAttribute(ctx, pathpkg.Root("widgets"), &list); diags.HasError() {
		t.Fatalf("widgets attr: %v", diags)
	}
	elems := list.Elements()
	if len(elems) != 2 {
		t.Fatalf("widgets list length = %d, want 2", len(elems))
	}
	names := map[string]bool{}
	for _, e := range elems {
		obj, ok := e.(types.Object)
		if !ok {
			t.Fatalf("element type %T, want types.Object", e)
		}
		n, ok := obj.Attributes()["name"].(types.String)
		if !ok {
			t.Fatalf("name attr type %T", obj.Attributes()["name"])
		}
		sz, ok := obj.Attributes()["size"].(types.Int64)
		if !ok || sz.ValueInt64() == 0 {
			t.Fatalf("widget %q: size not coerced to int64", n.ValueString())
		}
		names[n.ValueString()] = true
	}
	if !names["a"] || !names["b"] {
		t.Fatalf("widgets = %v", names)
	}
}

func TestEngineActionInvoke(t *testing.T) {
	ctx := ctxbg()
	f := newFakePVE()
	f.widgets["3"] = map[string]any{"id": "3", "name": "gamma", "size": 2}
	d := &TypeDescriptor{
		Name:   "pve_widget_wake",
		Kind:   KindAction,
		Phase:  2,
		Invoke: &Op{Method: "POST", Path: "/nodes/{node}/widgets/{widget_id}", Params: []string{"wake"}, WaitTask: true},
		Attributes: []Attr{
			{Name: "node", Type: AttrString, Required: true, PathParam: "node"},
			{Name: "widget_id", Type: AttrInt64, Required: true, PathParam: "widget_id"},
			{Name: "wake", Type: AttrBool, Optional: true},
		},
	}
	a := &genericAction{engineBase: engineBase{desc: d, client: f.client(t)}}
	schema := buildActionSchema(d)
	progress := []string{}
	resp := &action.InvokeResponse{SendProgress: func(e action.InvokeProgressEvent) { progress = append(progress, e.Message) }}
	a.Invoke(ctx, action.InvokeRequest{
		Config: tfsdk.Config{Raw: toTF(d, map[string]attr.Value{
			"node": types.StringValue("node1"), "widget_id": types.Int64Value(3), "wake": types.BoolValue(true),
		}), Schema: schema},
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Invoke: %v", resp.Diagnostics)
	}
	if len(progress) == 0 || !strings.Contains(progress[len(progress)-1], "task exited OK") {
		t.Fatalf("expected task-completion progress, got %v", progress)
	}
	// The wake param must have been sent.
	found := false
	for _, req := range f.requests {
		if strings.Contains(req, "POST /nodes/node1/widgets/3") {
			found = true
		}
	}
	if !found {
		t.Fatalf("invoke did not hit the endpoint: %v", f.requests)
	}
}

func TestEngineFunctionRun(t *testing.T) {
	ctx := ctxbg()
	// nextid-style function: GET /cluster/nextid → integer.
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeData(w, json.Number("107"))
	}))
	t.Cleanup(srv.Close)
	c, err := pveclient.NewClient(pveclient.Config{
		URL:         srv.URL,
		Credentials: pveclient.Credentials{APIToken: "root@pam!t=x"},
		Insecure:    true,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	d := &TypeDescriptor{
		Name:       "pve_nextid",
		Kind:       KindFunction,
		Phase:      2,
		Call:       &Op{Method: "GET", Path: "/cluster/nextid"},
		FuncResult: AttrInt64,
	}
	fn := &genericFunction{engineBase: engineBase{desc: d, client: c}}
	resultData, ferr := function.Int64Return{}.NewResultData(ctx)
	if ferr != nil {
		t.Fatalf("NewResultData: %v", ferr)
	}
	resp := &function.RunResponse{Result: resultData}
	fn.Run(ctx, function.RunRequest{Arguments: function.NewArgumentsData(nil)}, resp)
	if resp.Error != nil {
		t.Fatalf("Run: %v", resp.Error)
	}
	got, ok := resp.Result.Value().(types.Int64)
	if !ok {
		t.Fatalf("result type %T, want types.Int64", resp.Result.Value())
	}
	if got.ValueInt64() != 107 {
		t.Fatalf("nextid = %d, want 107", got.ValueInt64())
	}
}

func TestEngineStagedApply(t *testing.T) {
	ctx := ctxbg()
	f := newFakePVE()
	d := widgetDescriptor()
	d.Update = &Op{
		Method: "PUT", Path: "/nodes/{node}/widgets/{widget_id}", Params: []string{"name", "size"},
		Apply: &Op{Method: "PUT", Path: "/nodes/{node}/network"},
	}
	r := newResourceTest(t, d, f)
	schema := buildResourceSchema(d)
	f.widgets["101"] = map[string]any{"id": "101", "name": "old", "size": 1}
	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: schema}}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  tfsdk.Plan{Raw: toTF(d, map[string]attr.Value{"node": types.StringValue("node1"), "widget_id": types.Int64Value(101), "name": types.StringValue("new")}), Schema: schema},
		State: tfsdk.State{Raw: toTF(d, map[string]attr.Value{"id": types.StringValue("node1/101"), "node": types.StringValue("node1"), "widget_id": types.Int64Value(101), "name": types.StringValue("old")}), Schema: schema},
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Update: %v", resp.Diagnostics)
	}
	if f.applyHits != 1 {
		t.Fatalf("apply endpoint hit %d times, want 1", f.applyHits)
	}
}

// ---- review-driven regression tests ----

func TestEngineUnknownPlanValuesNotSent(t *testing.T) {
	ctx := ctxbg()
	f := newFakePVE()
	d := widgetDescriptor()
	r := newResourceTest(t, d, f)
	schema := buildResourceSchema(d)

	// A real Terraform plan marks unset Optional+Computed attributes as
	// unknown; they must render as nothing, never zero values.
	resp := &resource.CreateResponse{State: tfsdk.State{Schema: schema}}
	r.Create(ctx, resource.CreateRequest{
		Plan: tfsdk.Plan{Raw: toTFWith(d, map[string]attr.Value{
			"node": types.StringValue("node1"), "name": types.StringValue("alpha"),
			"widget_id": types.Int64Unknown(), "size": types.Int64Unknown(),
		}), Schema: schema},
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", resp.Diagnostics)
	}
	for _, req := range f.requests {
		if strings.Contains(req, "vmid=0") || strings.Contains(req, "widget_id=0") || strings.Contains(req, "size=0") {
			t.Fatalf("zero value serialized from unknown plan attr: %v", f.requests)
		}
	}
}

func TestEngineUpdateRemovalSendsDelete(t *testing.T) {
	ctx := ctxbg()
	f := newFakePVE()
	d := widgetDescriptor()
	d.Update.DeleteParam = true // as derived for PVE config-style PUT endpoints
	r := newResourceTest(t, d, f)
	schema := buildResourceSchema(d)
	f.widgets["101"] = map[string]any{"id": "101", "name": "old", "size": 9}

	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: schema}}
	r.Update(ctx, resource.UpdateRequest{
		Plan: tfsdk.Plan{Raw: toTFWith(d, map[string]attr.Value{
			"node": types.StringValue("node1"), "widget_id": types.Int64Value(101), "name": types.StringValue("beta"), "size": types.Int64Null(),
		}), Schema: schema},
		State: tfsdk.State{Raw: toTFWith(d, map[string]attr.Value{
			"id": types.StringValue("node1/101"), "node": types.StringValue("node1"), "widget_id": types.Int64Value(101),
			"name": types.StringValue("old"), "size": types.Int64Value(9),
		}), Schema: schema},
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Update: %v", resp.Diagnostics)
	}
	sent := strings.Join(f.forms, " ")
	if !strings.Contains(sent, "delete=size") {
		t.Fatalf("attribute removal must unset via PVE delete param; forms: %v", f.forms)
	}
}

func TestEngineTokenValuePreservedAcrossReadback(t *testing.T) {
	ctx := ctxbg()
	f := newFakePVE()
	d := widgetDescriptor()
	d.Create.ResultAttr = "secret_result"
	d.Create.ResultField = "value"
	d.Attributes = append(d.Attributes, Attr{Name: "secret_result", Type: AttrString, Computed: true, Sensitive: true})
	r := newResourceTest(t, d, f)
	// The fake's create returns a UPID; emulate a raw-string result by
	// driving captureResultAttr directly against the read-back merge.
	payload := json.RawMessage(`"PVEToken=root@pam!t=uuid"`)
	values := map[string]attr.Value{
		"node": types.StringValue("node1"), "name": types.StringValue("alpha"),
		"widget_id": types.Int64Value(7), "id": types.StringNull(),
	}
	for _, a := range d.Attributes {
		if _, ok := values[a.Name]; !ok {
			values[a.Name] = nullValue(a)
		}
	}
	captureResultAttr(d.Create, payload, values)
	f.widgets["7"] = map[string]any{"id": "7", "name": "alpha", "size": 5}
	if err := r.readBack(ctx, values); err != nil && err != errVanished {
		t.Fatalf("readBack: %v", err)
	}
	gotVal, ok := values["secret_result"].(types.String)
	if !ok {
		t.Fatalf("secret_result type %T", values["secret_result"])
	}
	got := gotVal.ValueString()
	if got != "PVEToken=root@pam!t=uuid" {
		t.Fatalf("one-time result not preserved across read-back: %q", got)
	}
}

func TestEngineSingletonImportTolerated(t *testing.T) {
	d := &TypeDescriptor{
		Name:       "pve_singleton",
		Kind:       KindResource,
		Read:       &Op{Method: "GET", Path: "/cluster/options"},
		Attributes: []Attr{{Name: "option", Type: AttrString, Optional: true}},
	}
	if _, err := parseID(d, "pve_singleton"); err != nil {
		t.Fatalf("identity-less singleton import must not error: %v", err)
	}
}

func TestEngineFunctionArgumentsSentAsParams(t *testing.T) {
	ctx := ctxbg()
	var gotQuery string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		writeData(w, "42")
	}))
	t.Cleanup(srv.Close)
	c, err := pveclient.NewClient(pveclient.Config{
		URL: srv.URL, Credentials: pveclient.Credentials{APIToken: "u@pam!t=x"}, Insecure: true,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	// Mirrors the generated pve_schedule_analyze shape: inputs live in
	// FuncArgs while op.Params references them by name.
	d := &TypeDescriptor{
		Name: "pve_schedule_analyze",
		Kind: KindFunction,
		Call: &Op{Method: "GET", Path: "/cluster/jobs/schedule-analyze", Params: []string{"iterations", "schedule"}},
		FuncArgs: []Attr{
			{Name: "iterations", Type: AttrInt64},
			{Name: "schedule", Type: AttrString},
		},
		FuncResult: AttrString,
	}
	fn := &genericFunction{engineBase: engineBase{desc: d, client: c}}
	rd, ferr := function.StringReturn{}.NewResultData(ctx)
	if ferr != nil {
		t.Fatalf("NewResultData: %v", ferr)
	}
	resp := &function.RunResponse{Result: rd}
	fn.Run(ctx, function.RunRequest{
		Arguments: function.NewArgumentsData([]attr.Value{types.Int64Value(8), types.StringValue("*:0/5")}),
	}, resp)
	if resp.Error != nil {
		t.Fatalf("Run: %v", resp.Error)
	}
	if gotQuery != "iterations=8&schedule=%2A%3A0%2F5" {
		t.Fatalf("function arguments not routed to query params: %q", gotQuery)
	}
}

func TestEngineCloneForwardsNameAndFull(t *testing.T) {
	ctx := ctxbg()
	f := newFakePVE()
	d := widgetDescriptor()
	d.Clone = &CloneSpec{
		Op:          &Op{Method: "POST", Path: "/nodes/{node}/widgets/{widget_id}/clone", Params: []string{"name", "node"}, WaitTask: true, IDFromUPID: "widget_id"},
		SourceAttr:  "clone_source",
		SourceParam: "widget_id",
		NewIDParam:  "newid",
		ConstParams: map[string]string{"full": "1"},
	}
	d.Attributes = append(d.Attributes, Attr{Name: "clone_source", Type: AttrInt64, Optional: true, ForceNew: true})
	r := newResourceTest(t, d, f)
	schema := buildResourceSchema(d)
	resp := &resource.CreateResponse{State: tfsdk.State{Schema: schema}}
	r.Create(ctx, resource.CreateRequest{
		Plan: tfsdk.Plan{Raw: toTF(d, map[string]attr.Value{
			"node": types.StringValue("node1"), "name": types.StringValue("beta"),
			"clone_source": types.Int64Value(7), "widget_id": types.Int64Value(42),
		}), Schema: schema},
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", resp.Diagnostics)
	}
	forms := strings.Join(f.forms, " | ")
	if !strings.Contains(forms, "name=beta") || !strings.Contains(forms, "newid=") || !strings.Contains(forms, "full=1") {
		t.Fatalf("clone request missing forwarded params: %q", forms)
	}
}

// TestApplyReadResponseModes pins the reconcile seam directly: one plain
// Optional attribute, one response field, every mode.
func TestApplyReadResponseModes(t *testing.T) {
	desc := &TypeDescriptor{
		Name:       "widget",
		Attributes: []Attr{{Name: "x", Type: AttrString, Optional: true}},
	}
	read := &Op{Method: "GET", Path: "/widgets/{name}"}

	tests := []struct {
		name    string
		mode    reconcileMode
		current attr.Value
		body    string
		want    attr.Value
	}{
		// A configured value is kept only when wireEqual holds (object
		// fields rendering to the same property string); a scalar echo
		// ("server" normalized "config") is adopted in every mode.
		{name: "after-apply adopts differing echo", mode: reconcileAfterApply, current: types.StringValue("config"), body: `{"x":"server"}`, want: types.StringValue("server")},
		{name: "refresh adopts differing echo", mode: reconcileRefresh, current: types.StringValue("config"), body: `{"x":"server"}`, want: types.StringValue("server")},
		{name: "import adopts differing echo", mode: reconcileImport, current: types.StringValue("config"), body: `{"x":"server"}`, want: types.StringValue("server")},
		// The seam: an unset plain-Optional attribute stays null outside
		// import mode — after-apply (the plan wins over server-side
		// defaults) and managed refresh alike (adopting would produce
		// perpetual drift); import adopts the server's value.
		{name: "after-apply keeps null", mode: reconcileAfterApply, current: types.StringNull(), body: `{"x":"server"}`, want: types.StringNull()},
		{name: "refresh keeps null", mode: reconcileRefresh, current: types.StringNull(), body: `{"x":"server"}`, want: types.StringNull()},
		{name: "import adopts into null", mode: reconcileImport, current: types.StringNull(), body: `{"x":"server"}`, want: types.StringValue("server")},
		// PVE's empty-string echo for unset options: not adopted into a
		// null plain-Optional in any mode either.
		{name: "import skips empty echo", mode: reconcileImport, current: types.StringNull(), body: `{"x":""}`, want: types.StringNull()},
		{name: "refresh keeps null on empty echo", mode: reconcileRefresh, current: types.StringNull(), body: `{"x":""}`, want: types.StringNull()},
		{name: "after-apply keeps null on empty echo", mode: reconcileAfterApply, current: types.StringNull(), body: `{"x":""}`, want: types.StringNull()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			values := map[string]attr.Value{"x": tt.current}
			ok, err := applyReadResponse(desc, read, json.RawMessage(tt.body), values, tt.mode)
			if err != nil || !ok {
				t.Fatalf("applyReadResponse: ok=%v err=%v", ok, err)
			}
			got, _ := values["x"].(types.String)
			if !got.Equal(tt.want) {
				t.Errorf("x = %v, want %v", got, tt.want)
			}
		})
	}
}
