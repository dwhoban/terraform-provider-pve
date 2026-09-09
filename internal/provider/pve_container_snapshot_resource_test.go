// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// TestPveContainerSnapshotResource_MetadataAndSchema covers the snapshot
// resource's type name and schema shape.
func TestPveContainerSnapshotResource_MetadataAndSchema(t *testing.T) {
	r := NewPveContainerSnapshotResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveContainerSnapshot {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveContainerSnapshot)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"node", "vmid", "name", "description", "snaptime", "parent"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	if !schemaResp.Schema.Attributes["name"].IsRequired() {
		t.Fatal("name attribute should be Required")
	}
}

// containerSnapshotAttrTypes returns the attribute type map for the
// snapshot resource state.
func containerSnapshotAttrTypes() map[string]tftypes.Type {
	return map[string]tftypes.Type{
		"node":        tftypes.String,
		"vmid":        tftypes.Number,
		"name":        tftypes.String,
		"description": tftypes.String,
		"snaptime":    tftypes.Number,
		"parent":      tftypes.String,
	}
}

// containerSnapshotPlanRaw builds a plan for snapshot pre-upgrade of
// container 100 on pve1.
func containerSnapshotPlanRaw() tftypes.Value {
	vals := map[string]tftypes.Value{
		"node":        tftypes.NewValue(tftypes.String, "pve1"),
		"vmid":        tftypes.NewValue(tftypes.Number, 100),
		"name":        tftypes.NewValue(tftypes.String, "pre-upgrade"),
		"description": tftypes.NewValue(tftypes.String, "before upgrade"),
		"snaptime":    tftypes.NewValue(tftypes.Number, nil),
		"parent":      tftypes.NewValue(tftypes.String, nil),
	}
	return tftypes.NewValue(tftypes.Object{AttributeTypes: containerSnapshotAttrTypes()}, vals)
}

// TestPveContainerSnapshotResource_CreateAndDelete runs create (POST
// snapshot, task wait, config read) and delete (DELETE snapshot, task wait)
// against a fake API.
func TestPveContainerSnapshotResource_CreateAndDelete(t *testing.T) {
	upidCreate := "UPID:pve1:000000E5:abcdef05:lxcsnapshot:root@pam:"
	upidDelete := "UPID:pve1:000000E6:abcdef06:lxcsnapshot:root@pam:"
	created, deleted := false, false
	r := &pveContainerSnapshotResource{}
	ctx := context.Background()
	client := newHaTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		switch {
		case req.Method == http.MethodPost && req.URL.Path == "/nodes/pve1/lxc/100/snapshot":
			created = true
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"data":"`+upidCreate+`"}`)
		case req.Method == http.MethodGet && req.URL.Path == "/nodes/pve1/lxc/100/snapshot/pre-upgrade/config":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"data":{"description":"before upgrade","digest":"abc123","parent":"current","snaptime":1700000000}}`)
		case strings.HasPrefix(req.URL.Path, "/nodes/pve1/tasks/"):
			_, _ = io.WriteString(w, `{"data":{"status":"stopped","exitstatus":"OK"}}`)
		case req.Method == http.MethodDelete && req.URL.Path == "/nodes/pve1/lxc/100/snapshot/pre-upgrade":
			deleted = true
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"data":"`+upidDelete+`"}`)
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
		}
	})
	cfgResp := &resource.ConfigureResponse{}
	r.Configure(ctx, resource.ConfigureRequest{ProviderData: client}, cfgResp)
	if cfgResp.Diagnostics.HasError() {
		t.Fatalf("Configure diagnostics: %s", diagnosticsError(cfgResp.Diagnostics))
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	raw := containerSnapshotPlanRaw()

	createResp := &resource.CreateResponse{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(tftypes.Object{AttributeTypes: containerSnapshotAttrTypes()}, nil)},
	}
	r.Create(ctx, resource.CreateRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw},
		Plan:   tfsdk.Plan{Schema: schemaResp.Schema, Raw: raw},
	}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create diagnostics: %s", diagnosticsError(createResp.Diagnostics))
	}
	if !created {
		t.Fatal("snapshot create POST was never sent")
	}
	var model pveContainerSnapshotResourceModel
	if diags := createResp.State.Get(ctx, &model); diags.HasError() {
		t.Fatalf("state get: %s", diagnosticsError(diags))
	}
	if model.Snaptime.ValueInt64() != 1700000000 {
		t.Fatalf("snaptime = %s, want 1700000000", model.Snaptime)
	}
	if model.Parent.ValueString() != "current" {
		t.Fatalf("parent = %s, want current", model.Parent.ValueString())
	}

	r.Delete(ctx, resource.DeleteRequest{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: createResp.State.Raw},
	}, &resource.DeleteResponse{})
	if !deleted {
		t.Fatal("snapshot delete DELETE was never sent")
	}
}

// TestPveContainerSnapshotResource_ImportParse covers the
// `<node>/<vmid>/<name>` import ID parsing, including malformed IDs.
func TestPveContainerSnapshotResource_ImportParse(t *testing.T) {
	node, vmid, name, err := containerSnapshotParseImportID("pve1/100/pre-upgrade")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if node != "pve1" || vmid != 100 || name != "pre-upgrade" {
		t.Fatalf("got %s/%d/%s, want pve1/100/pre-upgrade", node, vmid, name)
	}
	for _, bad := range []string{"", "pve1", "pve1/100", "pve1//snap", "pve1/abc/snap"} {
		if _, _, _, err := containerSnapshotParseImportID(bad); err == nil {
			t.Fatalf("expected error for %q", bad)
		}
	}
}
