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

// TestPveNodeServicesDataSource_MetadataAndSchema covers the services data
// source's type name and schema shape.
func TestPveNodeServicesDataSource_MetadataAndSchema(t *testing.T) {
	d := NewPveNodeServicesDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveNodeServices {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveNodeServices)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"id", "node", "services"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	if !schemaResp.Schema.Attributes["node"].IsRequired() {
		t.Fatal("node attribute should be Required")
	}
	if !schemaResp.Schema.Attributes["services"].IsComputed() {
		t.Fatal("services attribute should be Computed")
	}
}

// TestPveNodeServicesDataSource_Read decodes the service list into the
// nested rows against a fake API.
func TestPveNodeServicesDataSource_Read(t *testing.T) {
	d := NewPveNodeServicesDataSource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := d.(*pveNodeServicesDataSource)
	if !ok {
		t.Fatalf("constructor returned %T", d)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/nodes/pve1/services" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"name":"pveproxy","service":"pveproxy.service","desc":"PVE API Proxy Server","state":"running","active-state":"active","unit-state":"enabled"}]}`)
	})
	ctx := context.Background()
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	attrTypes := nodeServicesDataSourceAttrTypes()
	raw := tftypes.NewValue(tftypes.Object{AttributeTypes: attrTypes}, map[string]tftypes.Value{
		"id":       tftypes.NewValue(tftypes.String, nil),
		"node":     tftypes.NewValue(tftypes.String, "pve1"),
		"services": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: nodeServicesEntryAttrTypes()}}, nil),
	})
	readResp := &datasource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(tftypes.Object{AttributeTypes: attrTypes}, nil)}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw}}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %s", diagnosticsError(readResp.Diagnostics))
	}
	var data pveNodeServicesDataSourceModel
	if err := readResp.State.Get(ctx, &data); err != nil {
		t.Fatalf("State.Get: %v", err)
	}
	if data.ID.ValueString() != "pve1" || len(data.Services) != 1 {
		t.Fatalf("unexpected state: id=%s rows=%d", data.ID.ValueString(), len(data.Services))
	}
	row := data.Services[0]
	if row.Name.ValueString() != "pveproxy" || row.Description.ValueString() != "PVE API Proxy Server" || row.ActiveState.ValueString() != "active" || row.UnitState.ValueString() != "enabled" || row.State.ValueString() != "running" {
		t.Fatalf("unexpected row: %+v", row)
	}
}

// nodeServicesDataSourceAttrTypes returns the top-level attribute types of
// the services data source.
func nodeServicesDataSourceAttrTypes() map[string]tftypes.Type {
	return map[string]tftypes.Type{
		"id":       tftypes.String,
		"node":     tftypes.String,
		"services": tftypes.List{ElementType: tftypes.Object{AttributeTypes: nodeServicesEntryAttrTypes()}},
	}
}

// nodeServicesEntryAttrTypes returns the attribute types of one service row.
func nodeServicesEntryAttrTypes() map[string]tftypes.Type {
	return map[string]tftypes.Type{
		"name":         tftypes.String,
		"service":      tftypes.String,
		"description":  tftypes.String,
		"state":        tftypes.String,
		"active_state": tftypes.String,
		"unit_state":   tftypes.String,
	}
}
