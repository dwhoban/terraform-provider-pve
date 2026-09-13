// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// containerTestUPID is the task id every mutating fake endpoint returns.
const containerTestUPID = "UPID:pve1:00000001:00000001:vz:root@pam:op:"

// containerTestClient spins up a fake PVE API served by h and returns a
// client pointed at it. Token auth avoids the /access/ticket exchange.
func containerTestClient(t *testing.T, h http.HandlerFunc) *pveclient.Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	client, err := pveclient.NewClient(pveclient.Credentials{
		Endpoint: srv.URL,
		Token:    "root@pam!test=00000000-0000-0000-0000-000000000000",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return client
}

// containerTestObjectValue builds a tftypes object value for the given
// attribute type, filling named attributes from values and leaving the
// rest null.
func containerTestObjectValue(t *testing.T, typ attr.Type, values map[string]tftypes.Value) tftypes.Value {
	t.Helper()
	return containerTestRawObjectValue(t, typ.TerraformType(context.Background()), values)
}

// containerTestRawObjectValue is containerTestObjectValue for an already
// lowered tftypes object type.
func containerTestRawObjectValue(t *testing.T, typ tftypes.Type, values map[string]tftypes.Value) tftypes.Value {
	t.Helper()
	objType, ok := typ.(tftypes.Object)
	if !ok {
		t.Fatalf("type %s is not an object", typ)
	}
	attrs := make(map[string]tftypes.Value, len(objType.AttributeTypes))
	for name, attrType := range objType.AttributeTypes {
		if v, present := values[name]; present {
			attrs[name] = v
			continue
		}
		attrs[name] = tftypes.NewValue(attrType, nil)
	}
	return tftypes.NewValue(objType, attrs)
}

// containerTestStr and containerTestInt are shorthands for non-null
// tftypes values; containerTestTrue covers the always-true boolean case.
func containerTestStr(s string) tftypes.Value { return tftypes.NewValue(tftypes.String, s) }
func containerTestTrue() tftypes.Value        { return tftypes.NewValue(tftypes.Bool, true) }
func containerTestInt(i int64) tftypes.Value  { return tftypes.NewValue(tftypes.Number, i) }

// containerTestMountPoints builds a mount_points list value from the
// schema's list attribute type.
func containerTestMountPoints(t *testing.T, listAttr schema.Attribute, entries []map[string]tftypes.Value) tftypes.Value {
	t.Helper()
	listType, ok := listAttr.GetType().TerraformType(context.Background()).(tftypes.List)
	if !ok {
		t.Fatalf("mount_points type %s is not a list", listAttr.GetType())
	}
	out := make([]tftypes.Value, 0, len(entries))
	for _, entry := range entries {
		out = append(out, containerTestRawObjectValue(t, listType.ElementType, entry))
	}
	return tftypes.NewValue(listType, out)
}

// containerFakePVE serves the /nodes/{node}/lxc endpoints the resource
// drives and records every mutating call.
// containerTestBody is a decoded JSON request body captured by the fake
// PVE server; keys are PVE parameter names and values are the decoded
// JSON values (string, bool, float64, nil).
type containerTestBody map[string]any

type containerFakePVE struct {
	t *testing.T
	// config holds the container config as property strings keyed by
	// config key (rootfs, mp0, hostname, ...); PUTs and resizes mutate it
	// so a follow-up read reflects the mutation.
	config   map[string]string
	scalars  containerTestBody
	status   string // status/current status value
	pending  string // pending rows JSON
	ifaces   string // interfaces rows JSON
	nextID   string // value served by /cluster/nextid, "" disables
	flipStop bool   // after a shutdown POST the status flips to stopped
	calls    []string
	bodies   []containerTestBody
}

// configJSON renders the current config store as a PVE envelope.
func (f *containerFakePVE) configJSON() string {
	out := map[string]any{"digest": "cafe"}
	for key, value := range f.scalars {
		out[key] = value
	}
	for key, value := range f.config {
		out[key] = value
	}
	raw, err := json.Marshal(out)
	if err != nil {
		f.t.Fatalf("marshal config: %v", err)
	}
	return string(raw)
}

// applyConfigPUT merges a config PUT body: delete lists drop keys, mpN
// strings replace entries, scalars update.
func (f *containerFakePVE) applyConfigPUT(body containerTestBody) {
	if rawDelete, ok := body["delete"].(string); ok {
		for _, key := range strings.Split(rawDelete, ",") {
			delete(f.config, key)
		}
	}
	for key, value := range body {
		if key == "delete" || key == "digest" {
			continue
		}
		if s, ok := value.(string); ok {
			if key == "rootfs" || (strings.HasPrefix(key, "mp") && len(key) > 2) {
				f.applyMountPointKey(key, s)
			} else {
				f.config[key] = s
			}
			continue
		}
		f.scalars[key] = value
	}
}

// applyMountPointKey merges a rootfs/mpN property string into the stored
// config: empty size fields keep the stored size, mirroring how PVE treats
// size as read-only on config PUTs.
func (f *containerFakePVE) applyMountPointKey(key, value string) {
	newMP := pveclient.ParseLxcMountPoint(value)
	if raw, ok := f.config[key]; ok {
		cur := pveclient.ParseLxcMountPoint(raw)
		if newMP.Volume != "" {
			cur.Volume = newMP.Volume
		}
		if newMP.Mountpoint != "" {
			cur.Mountpoint = newMP.Mountpoint
		}
		if newMP.Size != "" {
			cur.Size = newMP.Size
		}
		if newMP.ACL != nil {
			cur.ACL = newMP.ACL
		}
		if newMP.Backup != nil {
			cur.Backup = newMP.Backup
		}
		if newMP.ReadOnly != nil {
			cur.ReadOnly = newMP.ReadOnly
		}
		f.config[key] = cur.Render()
		return
	}
	f.config[key] = newMP.Render()
}

// applyResize rewrites the size= segment of the targeted mount point.
func (f *containerFakePVE) applyResize(body containerTestBody) {
	disk, _ := body["disk"].(string)
	size, _ := body["size"].(string)
	raw, ok := f.config[disk]
	if !ok {
		f.t.Fatalf("resize of unknown disk %q", disk)
	}
	mp := pveclient.ParseLxcMountPoint(raw)
	mp.Size = size
	f.config[disk] = mp.Render()
}

// handler routes the fake API.
func (f *containerFakePVE) handler(w http.ResponseWriter, r *http.Request) {
	f.t.Helper()
	w.Header().Set("Content-Type", "application/json")
	p := r.URL.Path
	isNodeScoped := strings.HasPrefix(p, "/nodes/")
	switch {
	case r.Method == http.MethodGet && isNodeScoped && strings.Contains(p, "/tasks/") && strings.HasSuffix(p, "/status"):
		_, _ = w.Write([]byte(`{"data":{"status":"stopped","exitstatus":"OK"}}`))
	case r.Method == http.MethodGet && p == "/cluster/nextid":
		if f.nextID == "" {
			f.t.Fatalf("unexpected nextid call")
		}
		f.calls = append(f.calls, "GET /cluster/nextid")
		f.bodies = append(f.bodies, nil)
		_, _ = fmt.Fprintf(w, `{"data":%s}`, f.nextID)
	case r.Method == http.MethodPost && strings.HasSuffix(p, "/lxc"):
		f.record(r)
		_, _ = w.Write([]byte(`{"data":"` + containerTestUPID + `"}`))
	case r.Method == http.MethodPost && strings.HasSuffix(p, "/clone"):
		f.record(r)
		_, _ = w.Write([]byte(`{"data":"` + containerTestUPID + `"}`))
	case r.Method == http.MethodPost && strings.HasSuffix(p, "/migrate"):
		f.record(r)
		_, _ = w.Write([]byte(`{"data":"` + containerTestUPID + `"}`))
	case r.Method == http.MethodPost && (strings.HasSuffix(p, "/status/start") || strings.HasSuffix(p, "/status/stop")):
		f.record(r)
		_, _ = w.Write([]byte(`{"data":"` + containerTestUPID + `"}`))
	case r.Method == http.MethodPost && strings.HasSuffix(p, "/status/shutdown"):
		f.record(r)
		if f.flipStop {
			f.status = "stopped"
		}
		_, _ = w.Write([]byte(`{"data":"` + containerTestUPID + `"}`))
	case r.Method == http.MethodPut && strings.HasSuffix(p, "/config"):
		f.record(r)
		f.applyConfigPUT(f.bodies[len(f.bodies)-1])
		_, _ = w.Write([]byte(`{"data":null}`))
	case r.Method == http.MethodPut && strings.HasSuffix(p, "/resize"):
		f.record(r)
		f.applyResize(f.bodies[len(f.bodies)-1])
		_, _ = w.Write([]byte(`{"data":"` + containerTestUPID + `"}`))
	case r.Method == http.MethodDelete:
		f.record(r)
		_, _ = w.Write([]byte(`{"data":"` + containerTestUPID + `"}`))
	case r.Method == http.MethodGet && strings.HasSuffix(p, "/config"):
		_, _ = w.Write([]byte(`{"data":` + f.configJSON() + `}`))
	case r.Method == http.MethodGet && strings.HasSuffix(p, "/status/current"):
		_, _ = fmt.Fprintf(w, `{"data":{"name":"ct1","status":%q,"uptime":42}}`, f.status)
	case r.Method == http.MethodGet && strings.HasSuffix(p, "/pending"):
		_, _ = w.Write([]byte(`{"data":` + f.pending + `}`))
	case r.Method == http.MethodGet && strings.HasSuffix(p, "/interfaces"):
		if f.status != "running" {
			f.t.Fatalf("interfaces fetched while %s", f.status)
		}
		_, _ = w.Write([]byte(`{"data":` + f.ifaces + `}`))
	default:
		f.t.Fatalf("unexpected request: %s %s", r.Method, p)
	}
}

// record captures method, path, and JSON body of a mutating call.
func (f *containerFakePVE) record(r *http.Request) {
	f.t.Helper()
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		f.t.Fatalf("reading body: %v", err)
	}
	body := map[string]any{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &body); err != nil {
			f.t.Fatalf("bad JSON body %q: %v", raw, err)
		}
	}
	f.calls = append(f.calls, r.Method+" "+r.URL.Path)
	f.bodies = append(f.bodies, body)
}

// defaultContainerFake returns a fake serving a running container with a
// rootfs and one mount point.
func defaultContainerFake(t *testing.T) *containerFakePVE {
	return &containerFakePVE{
		t: t,
		config: map[string]string{
			"rootfs": "local-lvm:vm-100-disk-0,size=8G",
			"mp0":    "local-lvm:vm-100-disk-1,mp=/data,size=4G,acl=1",
			"mp1":    "local-lvm:vm-100-disk-2,mp=/old,size=2G",
		},
		scalars: map[string]any{
			"hostname":     "ct1",
			"memory":       "512",
			"onboot":       1,
			"unprivileged": "1",
		},
		status:  "running",
		pending: `[{"key":"hostname","value":"ct1"}]`,
		ifaces:  `[{"name":"eth0","hwaddr":"BC:24:11:2A:1D:6F","inet":"10.0.0.5/24"}]`,
	}
}

// containerTestResourceSchema returns the pve_container resource wired with
// its metadata asserted and schema built.
func containerTestResourceSchema(t *testing.T) (*pveContainerResource, resource.SchemaResponse) {
	t.Helper()
	r := NewPveContainerResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveContainer {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveContainer)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	impl, ok := r.(*pveContainerResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	return impl, *schemaResp
}

// TestPveContainerResource_MetadataAndSchema covers the type name,
// attribute set, and nested shapes of the resource schema.
func TestPveContainerResource_MetadataAndSchema(t *testing.T) {
	impl, schemaResp := containerTestResourceSchema(t)
	if impl.client != nil {
		t.Fatal("fresh resource should have no client")
	}
	for _, key := range []string{
		"id", "vmid", "node", "ostemplate", "hostname", "description", "tags",
		"started", "stop_on_destroy", "onboot", "protection", "template",
		"unprivileged", "cores", "memory", "swap", "password", "ssh_public_keys",
		"nameserver", "searchdomain", "clone", "mount_points", "status",
		"pending_changes", "interfaces",
	} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	if !schemaResp.Schema.Attributes["node"].IsRequired() {
		t.Fatal("node attribute should be Required")
	}
	if schemaResp.Schema.Attributes["node"].IsComputed() {
		t.Fatal("node attribute must stay mutable (migrate on change)")
	}
	if !schemaResp.Schema.Attributes["vmid"].IsComputed() {
		t.Fatal("vmid attribute should be Computed for auto-allocation")
	}
	if schemaResp.Schema.Attributes["status"].IsRequired() {
		t.Fatal("status attribute should be Computed")
	}
	if !schemaResp.Schema.Attributes["password"].IsSensitive() {
		t.Fatal("password attribute should be Sensitive")
	}
	mountPoints, ok := schemaResp.Schema.Attributes["mount_points"].(schema.ListNestedAttribute)
	if !ok {
		t.Fatal("mount_points is not a ListNestedAttribute")
	}
	if mountPoints.Required || mountPoints.Computed {
		t.Fatal("mount_points should be Optional-only")
	}
	for _, key := range []string{"id", "mountpoint", "storage", "volume", "size", "acl", "backup", "ro"} {
		if mountPoints.NestedObject.Attributes[key] == nil {
			t.Fatalf("mount point object missing %s attribute", key)
		}
	}
	if !mountPoints.NestedObject.Attributes["id"].IsRequired() {
		t.Fatal("mount point id should be Required")
	}
	clone, ok := schemaResp.Schema.Attributes["clone"].(schema.SingleNestedAttribute)
	if !ok {
		t.Fatal("clone is not a SingleNestedAttribute")
	}
	if !clone.Attributes["source_vmid"].IsRequired() {
		t.Fatal("clone.source_vmid should be Required")
	}
	interfaces, ok := schemaResp.Schema.Attributes["interfaces"].(schema.ListNestedAttribute)
	if !ok || !interfaces.Computed {
		t.Fatal("interfaces should be a Computed ListNestedAttribute")
	}
	if _, has := any(impl).(resource.ResourceWithValidateConfig); !has {
		t.Fatal("resource should implement ResourceWithValidateConfig")
	}
}

// containerTestCreatePlan builds a create plan raw value.
func containerTestCreatePlan(t *testing.T, schemaResp resource.SchemaResponse, values map[string]tftypes.Value) tftypes.Value {
	t.Helper()
	return containerTestObjectValue(t, schemaResp.Schema.Type(), values)
}

// containerTestCreate drives Create and returns the response.
func containerTestCreate(t *testing.T, impl *pveContainerResource, schemaResp resource.SchemaResponse, planValue tftypes.Value) *resource.CreateResponse {
	t.Helper()
	ctx := context.Background()
	createResp := &resource.CreateResponse{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil)},
	}
	impl.Create(ctx, resource.CreateRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: planValue},
		Plan:   tfsdk.Plan{Schema: schemaResp.Schema, Raw: planValue},
	}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("create: %s", diagnosticsError(createResp.Diagnostics))
	}
	return createResp
}

// TestPveContainerResource_CreateWithOSTemplate drives create from an
// ostemplate and asserts the wire body and the refreshed computed state.
func TestPveContainerResource_CreateWithOSTemplate(t *testing.T) {
	impl, schemaResp := containerTestResourceSchema(t)
	fake := defaultContainerFake(t)
	impl.client = containerTestClient(t, fake.handler)

	listAttr := schemaResp.Schema.Attributes["mount_points"]
	planValue := containerTestCreatePlan(t, schemaResp, map[string]tftypes.Value{
		"vmid":         containerTestInt(100),
		"node":         containerTestStr("pve1"),
		"ostemplate":   containerTestStr("local:vztmpl/debian-12.tar.zst"),
		"hostname":     containerTestStr("ct1"),
		"started":      containerTestTrue(),
		"unprivileged": containerTestTrue(),
		"memory":       containerTestInt(512),
		"password":     containerTestStr("secret"),
		"mount_points": containerTestMountPoints(t, listAttr, []map[string]tftypes.Value{
			{"id": containerTestStr("rootfs"), "storage": containerTestStr("local-lvm"), "size": containerTestStr("8G")},
			{
				"id":         containerTestStr("mp0"),
				"mountpoint": containerTestStr("/data"),
				"storage":    containerTestStr("local-lvm"),
				"size":       containerTestStr("4G"),
				"acl":        containerTestTrue(),
			},
		}),
	})
	createResp := containerTestCreate(t, impl, schemaResp, planValue)

	var createBody map[string]any
	for i, call := range fake.calls {
		if call == "POST /nodes/pve1/lxc" {
			createBody = fake.bodies[i]
		}
	}
	if createBody == nil {
		t.Fatalf("no create POST recorded, calls = %v", fake.calls)
	}
	vmidWire, vmidOK := createBody["vmid"].(float64)
	if !vmidOK || vmidWire != 100 || createBody["ostemplate"] != "local:vztmpl/debian-12.tar.zst" {
		t.Fatalf("create body vmid/ostemplate = %v/%v", createBody["vmid"], createBody["ostemplate"])
	}
	if createBody["start"] != true {
		t.Fatalf("start = %v, want true", createBody["start"])
	}
	if createBody["password"] != "secret" {
		t.Fatalf("password = %v", createBody["password"])
	}
	if createBody["rootfs"] != "local-lvm:8G" {
		t.Fatalf("rootfs = %v", createBody["rootfs"])
	}
	if createBody["mp0"] != "local-lvm:4G,mp=/data,acl=1" {
		t.Fatalf("mp0 = %v", createBody["mp0"])
	}

	var created pveContainerResourceModel
	if err := createResp.State.Get(context.Background(), &created); err != nil {
		t.Fatalf("get created state: %v", err)
	}
	if created.ID.ValueString() != "pve1/100" {
		t.Fatalf("id = %q", created.ID.ValueString())
	}
	if created.Status.ValueString() != "running" || !created.Started.ValueBool() {
		t.Fatalf("status/started = %s/%v", created.Status.ValueString(), created.Started.ValueBool())
	}
	if len(created.MountPoints) != 3 {
		t.Fatalf("mount points = %d", len(created.MountPoints))
	}
	if created.MountPoints[0].ID.ValueString() != "rootfs" || created.MountPoints[0].Size.ValueString() != "8G" {
		t.Fatalf("rootfs entry = %+v", created.MountPoints[0])
	}
	if created.MountPoints[1].ID.ValueString() != "mp0" || created.MountPoints[1].Mountpoint.ValueString() != "/data" || created.MountPoints[1].Storage.ValueString() != "local-lvm" {
		t.Fatalf("mp0 entry = %+v", created.MountPoints[1])
	}
	if len(created.Interfaces) != 1 || created.Interfaces[0].Inet.ValueString() != "10.0.0.5/24" {
		t.Fatalf("interfaces = %+v", created.Interfaces)
	}
	if created.PendingChanges.IsNull() || len(created.PendingChanges.Elements()) != 0 {
		t.Fatalf("pending changes should exclude plain current rows: %v", created.PendingChanges)
	}
}

// TestPveContainerResource_CreateAutoAllocatesVMID covers the null-vmid
// create path through GET /cluster/nextid.
func TestPveContainerResource_CreateAutoAllocatesVMID(t *testing.T) {
	impl, schemaResp := containerTestResourceSchema(t)
	fake := defaultContainerFake(t)
	fake.nextID = "100"
	impl.client = containerTestClient(t, fake.handler)

	planValue := containerTestCreatePlan(t, schemaResp, map[string]tftypes.Value{
		"node":       containerTestStr("pve1"),
		"ostemplate": containerTestStr("local:vztmpl/debian-12.tar.zst"),
	})
	createResp := containerTestCreate(t, impl, schemaResp, planValue)

	nextSeen := false
	for _, call := range fake.calls {
		if call == "GET /cluster/nextid" {
			nextSeen = true
		}
	}
	if !nextSeen {
		t.Fatalf("nextid not consulted, calls = %v", fake.calls)
	}
	var created pveContainerResourceModel
	if err := createResp.State.Get(context.Background(), &created); err != nil {
		t.Fatalf("get created state: %v", err)
	}
	if created.VMID.ValueInt64() != 100 || created.ID.ValueString() != "pve1/100" {
		t.Fatalf("vmid/id = %d/%q", created.VMID.ValueInt64(), created.ID.ValueString())
	}
}

// TestPveContainerResource_CreateByClone drives the clone path: POST clone
// with the allocated newid, a config overlay PUT, and the start task.
func TestPveContainerResource_CreateByClone(t *testing.T) {
	impl, schemaResp := containerTestResourceSchema(t)
	fake := defaultContainerFake(t)
	impl.client = containerTestClient(t, fake.handler)
	ctx := context.Background()

	cloneType := schemaResp.Schema.Attributes["clone"].GetType().TerraformType(ctx)
	cloneValue := containerTestRawObjectValue(t, cloneType, map[string]tftypes.Value{
		"source_vmid": containerTestInt(50),
		"hostname":    containerTestStr("ct1"),
		"full":        containerTestTrue(),
		"storage":     containerTestStr("local-lvm"),
	})
	planValue := containerTestCreatePlan(t, schemaResp, map[string]tftypes.Value{
		"vmid":   containerTestInt(100),
		"node":   containerTestStr("pve1"),
		"clone":  cloneValue,
		"memory": containerTestInt(1024),
	})
	createResp := containerTestCreate(t, impl, schemaResp, planValue)

	want := []string{
		"POST /nodes/pve1/lxc/50/clone",
		"PUT /nodes/pve1/lxc/100/config",
		"POST /nodes/pve1/lxc/100/status/start",
	}
	for i, wantCall := range want {
		if i >= len(fake.calls) || fake.calls[i] != wantCall {
			t.Fatalf("call[%d] = %v, want prefix %v (all: %v)", i, fake.calls, want, fake.calls)
		}
	}
	newID, newIDOK := fake.bodies[0]["newid"].(float64)
	if !newIDOK || newID != 100 {
		t.Fatalf("clone newid = %v", fake.bodies[0]["newid"])
	}
	if fake.bodies[0]["full"] != true || fake.bodies[0]["storage"] != "local-lvm" {
		t.Fatalf("clone body = %v", fake.bodies[0])
	}
	configPUT := -1
	for i, call := range fake.calls {
		if call == "PUT /nodes/pve1/lxc/100/config" {
			configPUT = i
		}
	}
	if configPUT < 0 {
		t.Fatalf("no config overlay PUT, calls = %v", fake.calls)
	}
	mem, memOK := fake.bodies[configPUT]["memory"].(float64)
	if !memOK || mem != 1024 {
		t.Fatalf("overlay body = %v", fake.bodies[configPUT])
	}
	var created pveContainerResourceModel
	if err := createResp.State.Get(ctx, &created); err != nil {
		t.Fatalf("get created state: %v", err)
	}
	if created.Status.ValueString() != "running" {
		t.Fatalf("clone status = %s", created.Status.ValueString())
	}
}

// containerTestStateAttrs copies a state object's attribute values into an
// editable map keyed by attribute name.
func containerTestStateAttrs(t *testing.T, raw tftypes.Value) (tftypes.Type, map[string]tftypes.Value) {
	t.Helper()
	objType, ok := raw.Type().(tftypes.Object)
	if !ok {
		t.Fatalf("state type %s is not an object", raw.Type())
	}
	attrs := make(map[string]tftypes.Value, len(objType.AttributeTypes))
	for name := range objType.AttributeTypes {
		value, err := raw.ApplyTerraform5AttributePathStep(tftypes.AttributeName(name))
		if err != nil {
			t.Fatalf("reading attribute %s: %v", name, err)
		}
		convertedValue, convertedOK2 := value.(tftypes.Value)
		if !convertedOK2 {
			t.Fatalf("attribute %s did not decode to a tftypes.Value", name)
		}
		attrs[name] = convertedValue
	}
	return objType, attrs
}

// TestPveContainerResource_UpdateMountPointDiff grows the rootfs, edits a
// mount point, and removes another; asserting the resize PUT, the config
// PUT re-render, and the delete list.
func TestPveContainerResource_UpdateMountPointDiff(t *testing.T) {
	impl, schemaResp := containerTestResourceSchema(t)
	fake := defaultContainerFake(t)
	impl.client = containerTestClient(t, fake.handler)
	ctx := context.Background()

	listAttr := schemaResp.Schema.Attributes["mount_points"]
	planValue := containerTestCreatePlan(t, schemaResp, map[string]tftypes.Value{
		"vmid":       containerTestInt(100),
		"node":       containerTestStr("pve1"),
		"ostemplate": containerTestStr("local:vztmpl/debian-12.tar.zst"),
		"mount_points": containerTestMountPoints(t, listAttr, []map[string]tftypes.Value{
			{"id": containerTestStr("rootfs"), "storage": containerTestStr("local-lvm"), "size": containerTestStr("8G")},
			{"id": containerTestStr("mp0"), "mountpoint": containerTestStr("/data"), "storage": containerTestStr("local-lvm"), "size": containerTestStr("4G")},
			{"id": containerTestStr("mp1"), "mountpoint": containerTestStr("/old"), "storage": containerTestStr("local-lvm"), "size": containerTestStr("2G")},
		}),
	})
	createResp := containerTestCreate(t, impl, schemaResp, planValue)
	fake.calls = nil
	fake.bodies = nil

	objType, attrs := containerTestStateAttrs(t, createResp.State.Raw)
	attrs["mount_points"] = containerTestMountPoints(t, listAttr, []map[string]tftypes.Value{
		{"id": containerTestStr("rootfs"), "storage": containerTestStr("local-lvm"), "volume": containerTestStr("vm-100-disk-0"), "size": containerTestStr("16G")},
		{"id": containerTestStr("mp0"), "mountpoint": containerTestStr("/data2"), "storage": containerTestStr("local-lvm"), "volume": containerTestStr("vm-100-disk-1"), "size": containerTestStr("4G")},
	})
	planRaw := tftypes.NewValue(objType, attrs)
	updateResp := &resource.UpdateResponse{State: createResp.State}
	impl.Update(ctx, resource.UpdateRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: planRaw},
		Plan:   tfsdk.Plan{Schema: schemaResp.Schema, Raw: planRaw},
		State:  tfsdk.State{Schema: schemaResp.Schema, Raw: createResp.State.Raw},
	}, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("update: %s", diagnosticsError(updateResp.Diagnostics))
	}

	resizeIdx, configIdx := -1, -1
	for i, call := range fake.calls {
		switch call {
		case "PUT /nodes/pve1/lxc/100/resize":
			resizeIdx = i
		case "PUT /nodes/pve1/lxc/100/config":
			configIdx = i
		}
	}
	if resizeIdx < 0 || configIdx < 0 {
		t.Fatalf("want resize and config PUT, calls = %v", fake.calls)
	}
	if fake.bodies[resizeIdx]["disk"] != "rootfs" || fake.bodies[resizeIdx]["size"] != "16G" {
		t.Fatalf("resize body = %v", fake.bodies[resizeIdx])
	}
	if fake.bodies[configIdx]["delete"] != "mp1" {
		t.Fatalf("config delete = %v", fake.bodies[configIdx]["delete"])
	}
	if fake.bodies[configIdx]["mp0"] != "local-lvm:vm-100-disk-1,mp=/data2" {
		t.Fatalf("mp0 re-render = %v", fake.bodies[configIdx]["mp0"])
	}
	var updated pveContainerResourceModel
	if err := updateResp.State.Get(ctx, &updated); err != nil {
		t.Fatalf("get updated state: %v", err)
	}
	if len(updated.MountPoints) != 2 || updated.MountPoints[0].Size.ValueString() != "16G" {
		t.Fatalf("updated mount points = %+v", updated.MountPoints)
	}
}

// TestPveContainerResource_UpdateNodeMigrates verifies a node change issues
// the migrate task against the old node and re-points the state.
func TestPveContainerResource_UpdateNodeMigrates(t *testing.T) {
	impl, schemaResp := containerTestResourceSchema(t)
	fake := defaultContainerFake(t)
	impl.client = containerTestClient(t, fake.handler)
	ctx := context.Background()

	planValue := containerTestCreatePlan(t, schemaResp, map[string]tftypes.Value{
		"vmid":       containerTestInt(100),
		"node":       containerTestStr("pve1"),
		"ostemplate": containerTestStr("local:vztmpl/debian-12.tar.zst"),
	})
	createResp := containerTestCreate(t, impl, schemaResp, planValue)
	fake.calls = nil
	fake.bodies = nil

	objType, attrs := containerTestStateAttrs(t, createResp.State.Raw)
	attrs["node"] = containerTestStr("pve2")
	planRaw := tftypes.NewValue(objType, attrs)
	updateResp := &resource.UpdateResponse{State: createResp.State}
	impl.Update(ctx, resource.UpdateRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: planRaw},
		Plan:   tfsdk.Plan{Schema: schemaResp.Schema, Raw: planRaw},
		State:  tfsdk.State{Schema: schemaResp.Schema, Raw: createResp.State.Raw},
	}, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("update: %s", diagnosticsError(updateResp.Diagnostics))
	}
	migrated := false
	for i, call := range fake.calls {
		if call == "POST /nodes/pve1/lxc/100/migrate" {
			migrated = true
			if fake.bodies[i]["target"] != "pve2" {
				t.Fatalf("migrate target = %v", fake.bodies[i]["target"])
			}
		}
	}
	if !migrated {
		t.Fatalf("no migrate call, calls = %v", fake.calls)
	}
	var updated pveContainerResourceModel
	if err := updateResp.State.Get(ctx, &updated); err != nil {
		t.Fatalf("get updated state: %v", err)
	}
	if updated.Node.ValueString() != "pve2" || updated.ID.ValueString() != "pve2/100" {
		t.Fatalf("node/id = %s/%q", updated.Node.ValueString(), updated.ID.ValueString())
	}
}

// TestPveContainerResource_DeleteStopOnDestroy verifies the graceful
// shutdown-then-destroy sequence.
func TestPveContainerResource_DeleteStopOnDestroy(t *testing.T) {
	impl, schemaResp := containerTestResourceSchema(t)
	fake := defaultContainerFake(t)
	fake.flipStop = true
	impl.client = containerTestClient(t, fake.handler)

	planValue := containerTestCreatePlan(t, schemaResp, map[string]tftypes.Value{
		"vmid":            containerTestInt(100),
		"node":            containerTestStr("pve1"),
		"ostemplate":      containerTestStr("local:vztmpl/debian-12.tar.zst"),
		"stop_on_destroy": containerTestTrue(),
	})
	createResp := containerTestCreate(t, impl, schemaResp, planValue)
	fake.calls = nil

	delResp := &resource.DeleteResponse{}
	impl.Delete(context.Background(), resource.DeleteRequest{State: createResp.State}, delResp)
	if delResp.Diagnostics.HasError() {
		t.Fatalf("delete: %s", diagnosticsError(delResp.Diagnostics))
	}
	joined := strings.Join(fake.calls, " | ")
	if !strings.Contains(joined, "POST /nodes/pve1/lxc/100/status/shutdown") {
		t.Fatalf("no shutdown before destroy, calls = %v", fake.calls)
	}
	if strings.Contains(joined, "status/stop") {
		t.Fatalf("stop fallback ran despite successful shutdown, calls = %v", fake.calls)
	}
	if !strings.Contains(joined, "DELETE /nodes/pve1/lxc/100") {
		t.Fatalf("no destroy call, calls = %v", fake.calls)
	}
}

// TestPveContainerResource_ImportState covers the `<node>/<vmid>` import
// contract and its rejection cases.
func TestPveContainerResource_ImportState(t *testing.T) {
	r, schemaResp := containerTestResourceSchema(t)
	ctx := context.Background()
	objType := schemaResp.Schema.Type().TerraformType(ctx)
	importResp := &resource.ImportStateResponse{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(objType, nil)},
	}
	r.ImportState(ctx, resource.ImportStateRequest{ID: "pve1/100"}, importResp)
	if importResp.Diagnostics.HasError() {
		t.Fatalf("import: %s", diagnosticsError(importResp.Diagnostics))
	}
	var node string
	var vmid int64
	if err := importResp.State.GetAttribute(ctx, path.Root("node"), &node); err != nil {
		t.Fatalf("node attr: %v", err)
	}
	if err := importResp.State.GetAttribute(ctx, path.Root("vmid"), &vmid); err != nil {
		t.Fatalf("vmid attr: %v", err)
	}
	if node != "pve1" || vmid != 100 {
		t.Fatalf("imported node/vmid = %s/%d", node, vmid)
	}
	for _, bad := range []string{"pve1", "/100", "pve1/", "pve1/abc", ""} {
		resp := &resource.ImportStateResponse{}
		r.ImportState(ctx, resource.ImportStateRequest{ID: bad}, resp)
		if !resp.Diagnostics.HasError() {
			t.Fatalf("import id %q should fail", bad)
		}
	}
}

// TestPveContainerResource_ValidateConfig covers the exactly-one
// create-source rule and mount point validations.
func TestPveContainerResource_ValidateConfig(t *testing.T) {
	r, schemaResp := containerTestResourceSchema(t)
	ctx := context.Background()
	listAttr := schemaResp.Schema.Attributes["mount_points"]
	cloneType := schemaResp.Schema.Attributes["clone"].GetType().TerraformType(ctx)

	cases := []struct {
		name    string
		values  map[string]tftypes.Value
		wantErr string
	}{
		{
			name:    "no source",
			values:  map[string]tftypes.Value{"node": containerTestStr("pve1")},
			wantErr: "Exactly one of `ostemplate` or `clone`",
		},
		{
			name: "both sources",
			values: map[string]tftypes.Value{
				"node":       containerTestStr("pve1"),
				"ostemplate": containerTestStr("local:vztmpl/x.tar.zst"),
				"clone": containerTestRawObjectValue(t, cloneType, map[string]tftypes.Value{
					"source_vmid": containerTestInt(50),
				}),
			},
			wantErr: "Exactly one of `ostemplate` or `clone`",
		},
		{
			name: "hostname duplicated",
			values: map[string]tftypes.Value{
				"node":     containerTestStr("pve1"),
				"hostname": containerTestStr("a"),
				"clone": containerTestRawObjectValue(t, cloneType, map[string]tftypes.Value{
					"source_vmid": containerTestInt(50),
					"hostname":    containerTestStr("b"),
				}),
			},
			wantErr: "exactly one place",
		},
		{
			name: "duplicate mount point ids",
			values: map[string]tftypes.Value{
				"node":       containerTestStr("pve1"),
				"ostemplate": containerTestStr("local:vztmpl/x.tar.zst"),
				"mount_points": containerTestMountPoints(t, listAttr, []map[string]tftypes.Value{
					{"id": containerTestStr("mp0"), "mountpoint": containerTestStr("/a"), "storage": containerTestStr("s"), "size": containerTestStr("1G")},
					{"id": containerTestStr("mp0"), "mountpoint": containerTestStr("/b"), "storage": containerTestStr("s"), "size": containerTestStr("1G")},
				}),
			},
			wantErr: "appears more than once",
		},
		{
			name: "rootfs with mountpoint",
			values: map[string]tftypes.Value{
				"node":       containerTestStr("pve1"),
				"ostemplate": containerTestStr("local:vztmpl/x.tar.zst"),
				"mount_points": containerTestMountPoints(t, listAttr, []map[string]tftypes.Value{
					{"id": containerTestStr("rootfs"), "mountpoint": containerTestStr("/"), "storage": containerTestStr("s"), "size": containerTestStr("1G")},
				}),
			},
			wantErr: "no `mp=` option",
		},
		{
			name: "mp without mountpoint",
			values: map[string]tftypes.Value{
				"node":       containerTestStr("pve1"),
				"ostemplate": containerTestStr("local:vztmpl/x.tar.zst"),
				"mount_points": containerTestMountPoints(t, listAttr, []map[string]tftypes.Value{
					{"id": containerTestStr("mp0"), "storage": containerTestStr("s"), "size": containerTestStr("1G")},
				}),
			},
			wantErr: "`mountpoint` is required",
		},
		{
			name: "bind mount with size",
			values: map[string]tftypes.Value{
				"node":       containerTestStr("pve1"),
				"ostemplate": containerTestStr("local:vztmpl/x.tar.zst"),
				"mount_points": containerTestMountPoints(t, listAttr, []map[string]tftypes.Value{
					{"id": containerTestStr("mp0"), "mountpoint": containerTestStr("/a"), "volume": containerTestStr("/mnt/host"), "size": containerTestStr("1G")},
				}),
			},
			wantErr: "cannot have a `size`",
		},
		{
			name: "storage without size",
			values: map[string]tftypes.Value{
				"node":       containerTestStr("pve1"),
				"ostemplate": containerTestStr("local:vztmpl/x.tar.zst"),
				"mount_points": containerTestMountPoints(t, listAttr, []map[string]tftypes.Value{
					{"id": containerTestStr("mp0"), "mountpoint": containerTestStr("/a"), "storage": containerTestStr("s")},
				}),
			},
			wantErr: "`size` is required",
		},
		{
			name: "valid rootfs allocation",
			values: map[string]tftypes.Value{
				"node":       containerTestStr("pve1"),
				"ostemplate": containerTestStr("local:vztmpl/x.tar.zst"),
				"mount_points": containerTestMountPoints(t, listAttr, []map[string]tftypes.Value{
					{"id": containerTestStr("rootfs"), "storage": containerTestStr("local-lvm"), "size": containerTestStr("8G")},
					{"id": containerTestStr("mp0"), "mountpoint": containerTestStr("/data"), "volume": containerTestStr("/mnt/host")},
				}),
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := containerTestCreatePlan(t, schemaResp, tc.values)
			resp := &resource.ValidateConfigResponse{}
			r.ValidateConfig(ctx, resource.ValidateConfigRequest{
				Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw},
			}, resp)
			if tc.wantErr == "" {
				if resp.Diagnostics.HasError() {
					t.Fatalf("unexpected errors: %s", diagnosticsError(resp.Diagnostics))
				}
				return
			}
			if !resp.Diagnostics.HasError() {
				t.Fatalf("want error %q, got none", tc.wantErr)
			}
			if !strings.Contains(diagnosticsError(resp.Diagnostics), tc.wantErr) {
				t.Fatalf("error %q does not contain %q", diagnosticsError(resp.Diagnostics), tc.wantErr)
			}
		})
	}
}
