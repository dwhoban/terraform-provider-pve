// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// TestPveVmResource_SchemaAndMetadata pins the pve_vm resource type name
// and the schema shape.
func TestPveVmResource_SchemaAndMetadata(t *testing.T) {
	r := NewPveVmResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveVm {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveVm)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"id", "vmid", "node", "name", "description", "tags", "started", "stop_on_destroy",
		"onboot", "protection", "template", "agent", "bios", "machine", "ostype", "cores", "sockets", "memory",
		"cpu_type", "scsihw", "boot_order", "clone", "disks", "network_interfaces", "cloud_init",
		"migrate_with_local_disks", "target_storage", "status", "pending_changes", "uptime", "maxmem", "maxcpu"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	disks, ok := schemaResp.Schema.Attributes["disks"].(schema.ListNestedAttribute)
	if !ok {
		t.Fatalf("disks must be a schema.ListNestedAttribute, got %T", schemaResp.Schema.Attributes["disks"])
	}
	for _, key := range []string{"id", "storage", "size", "format", "cache", "discard", "iothread", "ssd"} {
		if disks.NestedObject.Attributes[key] == nil {
			t.Fatalf("disks nested object missing %s attribute", key)
		}
	}
	nets, ok := schemaResp.Schema.Attributes["network_interfaces"].(schema.ListNestedAttribute)
	if !ok {
		t.Fatalf("network_interfaces must be a schema.ListNestedAttribute, got %T", schemaResp.Schema.Attributes["network_interfaces"])
	}
	for _, key := range []string{"id", "model", "bridge", "vlan_tag", "firewall", "macaddr", "queues"} {
		if nets.NestedObject.Attributes[key] == nil {
			t.Fatalf("network_interfaces nested object missing %s attribute", key)
		}
	}
	if _, ok := schemaResp.Schema.Attributes["clone"].(schema.SingleNestedAttribute); !ok {
		t.Fatalf("clone must be a schema.SingleNestedAttribute, got %T", schemaResp.Schema.Attributes["clone"])
	}
	if _, ok := schemaResp.Schema.Attributes["cloud_init"].(schema.SingleNestedAttribute); !ok {
		t.Fatalf("cloud_init must be a schema.SingleNestedAttribute, got %T", schemaResp.Schema.Attributes["cloud_init"])
	}
}

// TestPveVmDataSource_SchemaAndMetadata pins the pve_vm data source type
// name and the schema shape.
func TestPveVmDataSource_SchemaAndMetadata(t *testing.T) {
	d := NewPveVmDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveVm {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveVm)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"id", "node", "vmid", "name", "disks", "network_interfaces", "cloud_init",
		"status", "qmpstatus", "uptime", "maxmem", "maxcpu"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
}

// TestPveVmDriveRenderParseRoundTrip pins the drive value rendering and
// parsing contract against the PVE config syntax.
func TestPveVmDriveRenderParseRoundTrip(t *testing.T) {
	disk := pveVmDiskModel{
		ID:       types.StringValue("scsi0"),
		Storage:  types.StringValue("local-lvm"),
		Size:     types.StringValue("32G"),
		Cache:    types.StringValue("none"),
		Discard:  types.StringValue("on"),
		IOThread: types.BoolValue(true),
		SSD:      types.BoolNull(),
	}
	rendered, err := vmRenderNewDrive(disk)
	if err != nil {
		t.Fatalf("vmRenderNewDrive: %v", err)
	}
	if rendered != "local-lvm:32,cache=none,discard=on,iothread=1" {
		t.Fatalf("rendered = %q", rendered)
	}

	live := vmParseDriveValue("local-lvm:vm-100-disk-0,size=32G,cache=none,discard=on,iothread=1")
	if live.Volume != "local-lvm:vm-100-disk-0" || live.Storage != "local-lvm" || live.Size != "32G" {
		t.Fatalf("live = %+v", live)
	}
	if !live.IOThread || live.SSD || live.Cache != "none" || live.Discard != "on" {
		t.Fatalf("live = %+v", live)
	}

	reRendered := vmRenderExistingDrive(live.Volume, disk)
	if reRendered != "local-lvm:vm-100-disk-0,size=32G,cache=none,discard=on,iothread=1" {
		t.Fatalf("re-rendered = %q", reRendered)
	}

	if _, err := vmRenderNewDrive(pveVmDiskModel{ID: types.StringValue("virtio1"), Storage: types.StringNull(), Size: types.StringValue("8G")}); err == nil {
		t.Fatalf("new drive without storage must error")
	}
	efidisk, err := vmRenderNewDrive(pveVmDiskModel{ID: types.StringValue("efidisk0"), Storage: types.StringValue("local-lvm")})
	if err != nil || efidisk != "local-lvm:1" {
		t.Fatalf("efidisk0 rendered = %q err = %v", efidisk, err)
	}
}

// TestPveVmNetRenderParseRoundTrip pins the network device value format.
func TestPveVmNetRenderParseRoundTrip(t *testing.T) {
	net := pveVmNetModel{
		ID:       types.StringValue("net0"),
		Model:    types.StringValue("virtio"),
		Bridge:   types.StringValue("vmbr0"),
		VlanTag:  types.Int64Value(10),
		Firewall: types.BoolValue(true),
		MACAddr:  types.StringNull(),
		Queues:   types.Int64Null(),
	}
	if got := vmRenderNet(net); got != "virtio,bridge=vmbr0,tag=10,firewall=1" {
		t.Fatalf("rendered = %q", got)
	}
	live := vmParseNetValue("virtio=BC:24:11:2F:4E:8D,bridge=vmbr0,tag=10,firewall=1,queues=4")
	if live.Model != "virtio" || live.MACAddr != "BC:24:11:2F:4E:8D" || live.Bridge != "vmbr0" {
		t.Fatalf("live = %+v", live)
	}
	if live.Tag != "10" || live.Queues != "4" || live.Firewall == nil || !*live.Firewall {
		t.Fatalf("live = %+v", live)
	}
}

// TestPveVmSizeAndImportIDHelpers covers the size parse/format helpers
// and the <node>/<vmid> import ID.
func TestPveVmSizeAndImportIDHelpers(t *testing.T) {
	cases := map[string]int64{"32G": 32 << 30, "512M": 512 << 20, "1T": 1 << 40, "2048K": 2 << 20}
	for text, want := range cases {
		got, err := vmDiskSizeBytes(text)
		if err != nil || got != want {
			t.Fatalf("vmDiskSizeBytes(%q) = %d, %v; want %d", text, got, err, want)
		}
	}
	if got := vmFormatSizeBytes(64 << 30); got != "64G" {
		t.Fatalf("vmFormatSizeBytes(64GiB) = %q", got)
	}
	node, vmid, err := vmParseImportID("pve1/100")
	if err != nil || node != "pve1" || vmid != 100 {
		t.Fatalf("vmParseImportID = %q, %d, %v", node, vmid, err)
	}
	if _, _, err := vmParseImportID("pve1"); err == nil {
		t.Fatalf("missing vmid must error")
	}
}

// TestPveVmMergePending pins the pending merge: staged values overlay the
// config map, pending deletes remove the key, and the key list is
// returned.
func TestPveVmMergePending(t *testing.T) {
	config := map[string]json.RawMessage{
		"name":   json.RawMessage(`"web01"`),
		"memory": json.RawMessage(`2048`),
		"net0":   json.RawMessage(`"virtio,bridge=vmbr0"`),
	}
	pending := []pveclient.QemuVMPendingChange{
		{Key: "memory", Value: "2048", Pending: strPtrHelper("4096")},
		{Key: "net0", Value: "virtio,bridge=vmbr0", Delete: int64PtrHelper(1)},
	}
	keys := vmMergePending(config, pending)
	if len(keys) != 2 {
		t.Fatalf("keys = %v", keys)
	}
	if string(config["memory"]) != `"4096"` {
		t.Fatalf("memory = %s", config["memory"])
	}
	if _, ok := config["net0"]; ok {
		t.Fatalf("pending delete must remove net0 from the view")
	}
}

// TestPveVmAllocateID_NextID exercises the create path's VMID allocation
// against a fake /cluster/nextid.
func TestPveVmAllocateID_NextID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/cluster/nextid" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":104}`)
	}))
	defer srv.Close()
	client, err := pveclient.NewClient(pveclient.Credentials{Endpoint: srv.URL, Token: "root@pam!tf=x"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	r := &pveVmResource{client: client}
	plan := &pveVmResourceModel{VMID: types.Int64Null()}
	if err := r.allocateVMID(context.Background(), plan); err != nil {
		t.Fatalf("allocateVMID: %v", err)
	}
	if plan.VMID.ValueInt64() != 104 {
		t.Fatalf("allocated vmid = %d, want 104", plan.VMID.ValueInt64())
	}

	explicit := &pveVmResourceModel{VMID: types.Int64Value(200)}
	if err := r.allocateVMID(context.Background(), explicit); err != nil {
		t.Fatalf("allocateVMID explicit: %v", err)
	}
	if explicit.VMID.ValueInt64() != 200 {
		t.Fatalf("explicit vmid must be kept, got %d", explicit.VMID.ValueInt64())
	}
}

// TestPveVmDiskDiff_GrowShrink pins the size-change policy: growth
// produces an absolute resize, shrinking is rejected with a diagnostic
// naming the drive and both sizes.
func TestPveVmDiskDiff_GrowShrink(t *testing.T) {
	live := map[string]json.RawMessage{
		"scsi0": json.RawMessage(`"local-lvm:vm-100-disk-0,size=32G"`),
	}
	state := []pveVmDiskModel{{
		ID:   types.StringValue("scsi0"),
		Size: types.StringValue("32G"),
	}}

	moves, resizes, diags := vmDiffDriveOps(state, []pveVmDiskModel{{
		ID:   types.StringValue("scsi0"),
		Size: types.StringValue("64G"),
	}}, live)
	if diags.HasError() {
		t.Fatalf("grow diagnostics: %s", vmDiagsText(diags))
	}
	if len(moves) != 0 {
		t.Fatalf("same-storage grow must not move disks: %+v", moves)
	}
	if len(resizes) != 1 || resizes[0].Size != "64G" || resizes[0].Disk != "scsi0" {
		t.Fatalf("resize ops = %+v", resizes)
	}

	_, _, diags = vmDiffDriveOps(state, []pveVmDiskModel{{
		ID:   types.StringValue("scsi0"),
		Size: types.StringValue("16G"),
	}}, live)
	if !diags.HasError() {
		t.Fatalf("shrink must produce an error diagnostic")
	}
	text := vmDiagsText(diags)
	for _, want := range []string{"scsi0", "32G", "16G"} {
		if !strings.Contains(text, want) {
			t.Fatalf("shrink diagnostic %q must name %q", text, want)
		}
	}
}

// TestPveVmMigrate_OnNodeChange exercises migrateInto against a fake
// server, asserting the target, the online flag derived from the live
// status, and the task wait.
func TestPveVmMigrate_OnNodeChange(t *testing.T) {
	var migrateBody map[string]any
	var migrateSeen, taskSeen bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/nodes/pve1/qemu/100/status/current":
			_, _ = io.WriteString(w, `{"data":{"status":"running","template":0}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/nodes/pve1/qemu/100/migrate":
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &migrateBody)
			migrateSeen = true
			_, _ = io.WriteString(w, `{"data":"UPID:pve1:0001:0001:qmmigrate:root@pam:"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/nodes/pve1/tasks/UPID:pve1:0001:0001:qmmigrate:root@pam:/status":
			taskSeen = true
			_, _ = io.WriteString(w, `{"data":{"status":"stopped","exitstatus":"OK"}}`)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	client, err := pveclient.NewClient(pveclient.Credentials{Endpoint: srv.URL, Token: "root@pam!tf=x"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	r := &pveVmResource{client: client}
	plan := &pveVmResourceModel{MigrateWithLocalDisks: types.BoolValue(true)}

	if err := r.migrateInto(context.Background(), "pve1", "pve2", 100, plan); err != nil {
		t.Fatalf("migrateInto: %v", err)
	}
	if !migrateSeen || !taskSeen {
		t.Fatalf("migrateSeen = %v, taskSeen = %v", migrateSeen, taskSeen)
	}
	if migrateBody["target"] != "pve2" || migrateBody["online"] != true || migrateBody["with-local-disks"] != true {
		t.Fatalf("migrate body = %v", migrateBody)
	}
}

// TestPveVmCloneChanged pins the create-only clone block immutability
// check used by Update.
func TestPveVmCloneChanged(t *testing.T) {
	if vmCloneChanged(nil, nil) {
		t.Fatalf("nil clone blocks must not be flagged as changed")
	}
	if !vmCloneChanged(nil, &pveVmCloneModel{}) {
		t.Fatalf("adding a clone block must be flagged")
	}
	state := &pveVmCloneModel{SourceVmid: types.Int64Value(42)}
	if !vmCloneChanged(state, &pveVmCloneModel{SourceVmid: types.Int64Value(43)}) {
		t.Fatalf("source change must be flagged")
	}
	if vmCloneChanged(state, &pveVmCloneModel{SourceVmid: types.Int64Value(42)}) {
		t.Fatalf("identical clone blocks must not be flagged")
	}
}

// TestPveVmConfigIntoModel covers the read-side merge of config, status,
// and the started/template derivations.
func TestPveVmConfigIntoModel(t *testing.T) {
	config := map[string]json.RawMessage{
		"name":      json.RawMessage(`"web01"`),
		"tags":      json.RawMessage(`"prod;web"`),
		"onboot":    json.RawMessage(`1`),
		"agent":     json.RawMessage(`"enabled=1"`),
		"bios":      json.RawMessage(`"ovmf"`),
		"ostype":    json.RawMessage(`"l26"`),
		"cpu":       json.RawMessage(`"cputype=host,flags=+aes"`),
		"cores":     json.RawMessage(`2`),
		"memory":    json.RawMessage(`2048`),
		"boot":      json.RawMessage(`"order=scsi0;net0"`),
		"scsi0":     json.RawMessage(`"local-lvm:vm-100-disk-0,size=32G,ssd=1"`),
		"net0":      json.RawMessage(`"virtio=BC:24:11:2F:4E:8D,bridge=vmbr0,firewall=1"`),
		"ciuser":    json.RawMessage(`"admin"`),
		"ipconfig0": json.RawMessage(`"ip=dhcp"`),
	}
	status := &pveclient.QemuVMStatus{Status: "running", Template: true, Uptime: 60, MaxMem: 2 << 30, MaxCPU: 4}
	model := &pveVmResourceModel{}
	vmConfigIntoResourceModel(config, status, model)

	if model.Name.ValueString() != "web01" {
		t.Fatalf("name = %q", model.Name.ValueString())
	}
	tags := listStringFromTF(model.Tags)
	if len(tags) != 2 || tags[1] != "web" {
		t.Fatalf("tags = %v", tags)
	}
	if !model.Onboot.ValueBool() || !model.Agent.ValueBool() {
		t.Fatalf("onboot/agent = %v/%v", model.Onboot, model.Agent)
	}
	if model.CPUType.ValueString() != "host" {
		t.Fatalf("cpu_type = %q", model.CPUType.ValueString())
	}
	boot := listStringFromTF(model.BootOrder)
	if len(boot) != 2 || boot[0] != "scsi0" {
		t.Fatalf("boot_order = %v", boot)
	}
	if len(model.Disks) != 1 || model.Disks[0].Storage.ValueString() != "local-lvm" || !model.Disks[0].SSD.ValueBool() {
		t.Fatalf("disks = %+v", model.Disks)
	}
	if len(model.NetworkInterfaces) != 1 || model.NetworkInterfaces[0].MACAddr.ValueString() != "BC:24:11:2F:4E:8D" {
		t.Fatalf("network_interfaces = %+v", model.NetworkInterfaces)
	}
	if model.CloudInit == nil || model.CloudInit.User.ValueString() != "admin" || model.CloudInit.Ipconfig["ipconfig0"] != "ip=dhcp" {
		t.Fatalf("cloud_init = %+v", model.CloudInit)
	}
	if !model.Started.ValueBool() || model.Status.ValueString() != "running" {
		t.Fatalf("started/status = %v/%v", model.Started, model.Status)
	}
	if !model.Template.ValueBool() {
		t.Fatalf("template must fall back to the status read when the config omits it")
	}
}

// strPtrHelper builds a *string for pending-change fixtures.
func strPtrHelper(s string) *string { return &s }

// int64PtrHelper builds a *int64 for pending-change fixtures.
func int64PtrHelper(i int64) *int64 { return &i }
