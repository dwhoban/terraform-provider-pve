// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// TestPveCephOSDResource_MetadataAndSchema covers the OSD resource's type
// name and schema shape.
func TestPveCephOSDResource_MetadataAndSchema(t *testing.T) {
	r := NewPveCephOSDResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveCephOsd {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveCephOsd)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"node", "device", "db_device", "db_device_size_gib", "wal_device", "wal_device_size_gib", "crush_device_class", "encrypted", "osds_per_device", "cleanup_on_destroy", "osd_id", "hostname", "osd_data", "osd_objectstore", "version"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	if !schemaResp.Schema.Attributes["device"].IsRequired() {
		t.Fatal("device attribute should be Required")
	}
}

// TestPveCephOSDDataSource_SchemaAndMetadata covers the OSD data source's
// type name and schema shape.
func TestPveCephOSDDataSource_SchemaAndMetadata(t *testing.T) {
	d := NewPveCephOSDDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveCephOsd {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveCephOsd)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"id", "node", "osd_id", "hostname", "osd_data", "osd_objectstore", "encrypted", "version", "pid", "front_addr", "back_addr", "hb_front_addr", "hb_back_addr", "mem_usage", "devices"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
}

// cephOSDAttrTypes returns the full attribute type map for the OSD
// resource state.
func cephOSDAttrTypes() map[string]tftypes.Type {
	return map[string]tftypes.Type{
		"node":                tftypes.String,
		"device":              tftypes.String,
		"db_device":           tftypes.String,
		"db_device_size_gib":  tftypes.Number,
		"wal_device":          tftypes.String,
		"wal_device_size_gib": tftypes.Number,
		"crush_device_class":  tftypes.String,
		"encrypted":           tftypes.Bool,
		"osds_per_device":     tftypes.Number,
		"cleanup_on_destroy":  tftypes.Bool,
		"osd_id":              tftypes.Number,
		"hostname":            tftypes.String,
		"osd_data":            tftypes.String,
		"osd_objectstore":     tftypes.String,
		"version":             tftypes.String,
	}
}

// TestPveCephOSDResource_Read404Removes verifies Read drops the resource
// from state when the OSD vanished out of band.
func TestPveCephOSDResource_Read404Removes(t *testing.T) {
	r := NewPveCephOSDResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveCephOSDResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprintf(w, `{"errors":"no such osd"}`)
	})
	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	vals := map[string]tftypes.Value{
		"node":                tftypes.NewValue(tftypes.String, "pve1"),
		"device":              tftypes.NewValue(tftypes.String, "/dev/sdb"),
		"db_device":           tftypes.NewValue(tftypes.String, nil),
		"db_device_size_gib":  tftypes.NewValue(tftypes.Number, nil),
		"wal_device":          tftypes.NewValue(tftypes.String, nil),
		"wal_device_size_gib": tftypes.NewValue(tftypes.Number, nil),
		"crush_device_class":  tftypes.NewValue(tftypes.String, nil),
		"encrypted":           tftypes.NewValue(tftypes.Bool, nil),
		"osds_per_device":     tftypes.NewValue(tftypes.Number, nil),
		"cleanup_on_destroy":  tftypes.NewValue(tftypes.Bool, nil),
		"osd_id":              tftypes.NewValue(tftypes.Number, 7),
		"hostname":            tftypes.NewValue(tftypes.String, nil),
		"osd_data":            tftypes.NewValue(tftypes.String, nil),
		"osd_objectstore":     tftypes.NewValue(tftypes.String, nil),
		"version":             tftypes.NewValue(tftypes.String, nil),
	}
	state := tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(tftypes.Object{AttributeTypes: cephOSDAttrTypes()}, vals)}
	readResp := &resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %s", diagnosticsError(readResp.Diagnostics))
	}
	if !readResp.State.Raw.IsNull() {
		t.Fatalf("expected state to be removed, got %v", readResp.State.Raw)
	}
}
