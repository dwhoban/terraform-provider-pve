// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// containerDSTestSchema returns the pve_container data source with its
// metadata asserted and schema built.
func containerDSTestSchema(t *testing.T) (datasource.DataSource, schema.Schema) {
	t.Helper()
	d := NewPveContainerDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveContainer {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveContainer)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	return d, schemaResp.Schema
}

// TestPveContainerDataSource_SchemaAndMetadata covers the data source's
// type name and schema shape.
func TestPveContainerDataSource_SchemaAndMetadata(t *testing.T) {
	_, dsSchema := containerDSTestSchema(t)
	for _, key := range []string{
		"id", "node", "vmid", "hostname", "description", "tags", "onboot",
		"protection", "template", "unprivileged", "cores", "memory", "swap",
		"nameserver", "searchdomain", "status", "started", "mount_points", "interfaces",
	} {
		if dsSchema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	if !dsSchema.Attributes["node"].IsRequired() || !dsSchema.Attributes["vmid"].IsRequired() {
		t.Fatal("node and vmid should be Required")
	}
	for _, key := range []string{"hostname", "status", "started"} {
		if !dsSchema.Attributes[key].IsComputed() {
			t.Fatalf("%s should be Computed", key)
		}
	}
	mountPoints, ok := dsSchema.Attributes["mount_points"].(schema.ListNestedAttribute)
	if !ok || !mountPoints.Computed {
		t.Fatal("mount_points should be a Computed ListNestedAttribute")
	}
	for _, key := range []string{"id", "mountpoint", "storage", "volume", "size", "acl", "backup", "ro"} {
		if mountPoints.NestedObject.Attributes[key] == nil {
			t.Fatalf("mount point object missing %s attribute", key)
		}
	}
	if mountPoints.NestedObject.Attributes["id"].IsRequired() {
		t.Fatal("data source mount point attributes should all be Computed")
	}
}

// TestPveContainerDataSource_Read decodes config, status, mount points and
// interfaces into the data source state.
func TestPveContainerDataSource_Read(t *testing.T) {
	fake := defaultContainerFake(t)
	d, dsSchema := containerDSTestSchema(t)
	ds, isDS := d.(*pveContainerDataSource)
	if !isDS {
		t.Fatalf("constructor returned %T", d)
	}
	ds.client = containerTestClient(t, fake.handler)
	ctx := context.Background()

	configValue := containerTestObjectValue(t, dsSchema.Type(), map[string]tftypes.Value{
		"node": containerTestStr("pve1"),
		"vmid": containerTestInt(100),
	})
	readResp := &datasource.ReadResponse{
		State: tfsdk.State{Schema: dsSchema, Raw: configValue},
	}
	ds.Read(ctx, datasource.ReadRequest{
		Config: tfsdk.Config{Schema: dsSchema, Raw: configValue},
	}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("read: %s", diagnosticsError(readResp.Diagnostics))
	}

	var state pveContainerDataSourceModel
	if err := readResp.State.Get(ctx, &state); err != nil {
		t.Fatalf("get read state: %v", err)
	}
	if state.ID.ValueString() != "pve1/100" {
		t.Fatalf("id = %q", state.ID.ValueString())
	}
	if state.Hostname.ValueString() != "ct1" || state.Memory.ValueInt64() != 512 {
		t.Fatalf("hostname/memory = %s/%d", state.Hostname.ValueString(), state.Memory.ValueInt64())
	}
	if !state.Unprivileged.ValueBool() || !state.Onboot.ValueBool() {
		t.Fatalf("unprivileged/onboot = %v/%v", state.Unprivileged.ValueBool(), state.Onboot.ValueBool())
	}
	if state.Status.ValueString() != "running" || !state.Started.ValueBool() {
		t.Fatalf("status/started = %s/%v", state.Status.ValueString(), state.Started.ValueBool())
	}
	if len(state.MountPoints) != 3 {
		t.Fatalf("mount points = %d", len(state.MountPoints))
	}
	if state.MountPoints[1].ID.ValueString() != "mp0" || state.MountPoints[1].ACL.IsNull() || !state.MountPoints[1].ACL.ValueBool() {
		t.Fatalf("mp0 entry = %+v", state.MountPoints[1])
	}
	if len(state.Interfaces) != 1 || state.Interfaces[0].HWAddr.ValueString() != "BC:24:11:2A:1D:6F" {
		t.Fatalf("interfaces = %+v", state.Interfaces)
	}
}

// TestPveContainerDataSource_ReadStopped verifies a stopped container
// surfaces no interfaces (the endpoint requires a running container).
func TestPveContainerDataSource_ReadStopped(t *testing.T) {
	fake := defaultContainerFake(t)
	fake.status = "stopped"
	fake.ifaces = `[{"name":"eth0"}]` // must never be served
	d, dsSchema := containerDSTestSchema(t)
	ds, isDS := d.(*pveContainerDataSource)
	if !isDS {
		t.Fatalf("constructor returned %T", d)
	}
	var _ http.HandlerFunc = fake.handler
	ds.client = containerTestClient(t, fake.handler)
	ctx := context.Background()

	configValue := containerTestObjectValue(t, dsSchema.Type(), map[string]tftypes.Value{
		"node": containerTestStr("pve1"),
		"vmid": containerTestInt(100),
	})
	readResp := &datasource.ReadResponse{
		State: tfsdk.State{Schema: dsSchema, Raw: configValue},
	}
	ds.Read(ctx, datasource.ReadRequest{
		Config: tfsdk.Config{Schema: dsSchema, Raw: configValue},
	}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("read: %s", diagnosticsError(readResp.Diagnostics))
	}
	var state pveContainerDataSourceModel
	if err := readResp.State.Get(ctx, &state); err != nil {
		t.Fatalf("get read state: %v", err)
	}
	if state.Status.ValueString() != "stopped" || state.Started.ValueBool() {
		t.Fatalf("status/started = %s/%v", state.Status.ValueString(), state.Started.ValueBool())
	}
	if state.Interfaces != nil {
		t.Fatalf("interfaces should be null when stopped, got %+v", state.Interfaces)
	}
}
