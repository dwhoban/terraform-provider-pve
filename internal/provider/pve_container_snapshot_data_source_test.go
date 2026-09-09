// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// TestPveContainerSnapshotDataSource_MetadataAndSchema covers the snapshot
// data source's type name and schema shape.
func TestPveContainerSnapshotDataSource_MetadataAndSchema(t *testing.T) {
	d := NewPveContainerSnapshotDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveContainerSnapshot {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveContainerSnapshot)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"node", "vmid", "name", "id", "description", "snaptime", "parent"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
}

// TestPveContainerSnapshotDataSource_Read decodes the snapshot config for a
// known snapshot.
func TestPveContainerSnapshotDataSource_Read(t *testing.T) {
	d := &pveContainerSnapshotDataSource{}
	ctx := context.Background()
	client := newHaTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/nodes/pve1/lxc/100/snapshot/pre-upgrade/config" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"description":"before upgrade","digest":"abc123","parent":"current","snaptime":1700000000}}`)
	})
	cfgResp := &datasource.ConfigureResponse{}
	d.Configure(ctx, datasource.ConfigureRequest{ProviderData: client}, cfgResp)
	if cfgResp.Diagnostics.HasError() {
		t.Fatalf("Configure diagnostics: %s", diagnosticsError(cfgResp.Diagnostics))
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	attrTypes := map[string]tftypes.Type{
		"node":        tftypes.String,
		"vmid":        tftypes.Number,
		"name":        tftypes.String,
		"id":          tftypes.String,
		"description": tftypes.String,
		"snaptime":    tftypes.Number,
		"parent":      tftypes.String,
	}
	raw := tftypes.NewValue(tftypes.Object{AttributeTypes: attrTypes}, map[string]tftypes.Value{
		"node":        tftypes.NewValue(tftypes.String, "pve1"),
		"vmid":        tftypes.NewValue(tftypes.Number, 100),
		"name":        tftypes.NewValue(tftypes.String, "pre-upgrade"),
		"id":          tftypes.NewValue(tftypes.String, nil),
		"description": tftypes.NewValue(tftypes.String, nil),
		"snaptime":    tftypes.NewValue(tftypes.Number, nil),
		"parent":      tftypes.NewValue(tftypes.String, nil),
	})
	readResp := &datasource.ReadResponse{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(tftypes.Object{AttributeTypes: attrTypes}, nil)},
	}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw}}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %s", diagnosticsError(readResp.Diagnostics))
	}
	var data pveContainerSnapshotDataSourceModel
	if diags := readResp.State.Get(ctx, &data); diags.HasError() {
		t.Fatalf("state get: %s", diagnosticsError(diags))
	}
	if data.ID.ValueString() != "pve1/100/pre-upgrade" {
		t.Fatalf("id = %s, want pve1/100/pre-upgrade", data.ID.ValueString())
	}
	if data.Description.ValueString() != "before upgrade" {
		t.Fatalf("description = %s, want before upgrade", data.Description.ValueString())
	}
	if data.Snaptime.ValueInt64() != 1700000000 {
		t.Fatalf("snaptime = %d, want 1700000000", data.Snaptime.ValueInt64())
	}
	if data.Parent.ValueString() != "current" {
		t.Fatalf("parent = %s, want current", data.Parent.ValueString())
	}
}

// TestPveContainerSnapshotDataSource_ReadWithoutClient verifies the data
// source surfaces a diagnostic when the client is missing.
func TestPveContainerSnapshotDataSource_ReadWithoutClient(t *testing.T) {
	d := &pveContainerSnapshotDataSource{}
	ctx := context.Background()
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	attrTypes := map[string]tftypes.Type{"node": tftypes.String, "vmid": tftypes.Number, "name": tftypes.String, "id": tftypes.String, "description": tftypes.String, "snaptime": tftypes.Number, "parent": tftypes.String}
	raw := tftypes.NewValue(tftypes.Object{AttributeTypes: attrTypes}, map[string]tftypes.Value{
		"node":        tftypes.NewValue(tftypes.String, "pve1"),
		"vmid":        tftypes.NewValue(tftypes.Number, 100),
		"name":        tftypes.NewValue(tftypes.String, "pre-upgrade"),
		"id":          tftypes.NewValue(tftypes.String, nil),
		"description": tftypes.NewValue(tftypes.String, nil),
		"snaptime":    tftypes.NewValue(tftypes.Number, nil),
		"parent":      tftypes.NewValue(tftypes.String, nil),
	})
	readResp := &datasource.ReadResponse{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(tftypes.Object{AttributeTypes: attrTypes}, nil)},
	}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw}}, readResp)
	if !readResp.Diagnostics.HasError() {
		t.Fatal("expected error diagnostic for unconfigured client")
	}
}
