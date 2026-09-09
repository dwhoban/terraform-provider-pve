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

// TestPveAptStandardRepositoryDataSource_MetadataAndSchema covers the
// standard repository data source's type name and schema shape.
func TestPveAptStandardRepositoryDataSource_MetadataAndSchema(t *testing.T) {
	d := NewPveAptStandardRepositoryDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveAptStandardRepository {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveAptStandardRepository)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"id", "node", "handle", "name", "status"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	for _, key := range []string{"node", "handle"} {
		if !schemaResp.Schema.Attributes[key].IsRequired() {
			t.Fatalf("%s attribute should be Required", key)
		}
	}
}

// TestPveAptStandardRepositoryDataSource_Read verifies the configured row
// resolves name and status, and null status stays null for unconfigured rows.
func TestPveAptStandardRepositoryDataSource_Read(t *testing.T) {
	d := NewPveAptStandardRepositoryDataSource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := d.(*pveAptStandardRepositoryDataSource)
	if !ok {
		t.Fatalf("constructor returned %T", d)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/nodes/pve1/apt/repositories" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"digest":"dd01","standard-repos":[{"handle":"enterprise","name":"Proxmox VE Enterprise Repository","status":false},{"handle":"no-subscription","name":"Proxmox VE No-Subscription Repository"}]}}`)
	})
	ctx := context.Background()
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	attrTypes := map[string]tftypes.Type{"id": tftypes.String, "node": tftypes.String, "handle": tftypes.String, "name": tftypes.String, "status": tftypes.Bool}
	raw := tftypes.NewValue(tftypes.Object{AttributeTypes: attrTypes}, map[string]tftypes.Value{
		"id":     tftypes.NewValue(tftypes.String, nil),
		"node":   tftypes.NewValue(tftypes.String, "pve1"),
		"handle": tftypes.NewValue(tftypes.String, "enterprise"),
		"name":   tftypes.NewValue(tftypes.String, nil),
		"status": tftypes.NewValue(tftypes.Bool, nil),
	})
	readResp := &datasource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(tftypes.Object{AttributeTypes: attrTypes}, nil)}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw}}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %s", diagnosticsError(readResp.Diagnostics))
	}
	var data pveAptStandardRepositoryDataSourceModel
	if err := readResp.State.Get(ctx, &data); err != nil {
		t.Fatalf("State.Get: %v", err)
	}
	if data.ID.ValueString() != "pve1:enterprise" || data.Name.ValueString() != "Proxmox VE Enterprise Repository" || data.Status.ValueBool() {
		t.Fatalf("unexpected state: %+v", data)
	}

	// Unconfigured row: status must stay null.
	rawUnconfigured := tftypes.NewValue(tftypes.Object{AttributeTypes: attrTypes}, map[string]tftypes.Value{
		"id":     tftypes.NewValue(tftypes.String, nil),
		"node":   tftypes.NewValue(tftypes.String, "pve1"),
		"handle": tftypes.NewValue(tftypes.String, "no-subscription"),
		"name":   tftypes.NewValue(tftypes.String, nil),
		"status": tftypes.NewValue(tftypes.Bool, nil),
	})
	readResp2 := &datasource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(tftypes.Object{AttributeTypes: attrTypes}, nil)}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: rawUnconfigured}}, readResp2)
	if readResp2.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %s", diagnosticsError(readResp2.Diagnostics))
	}
	var unconfigured pveAptStandardRepositoryDataSourceModel
	if err := readResp2.State.Get(ctx, &unconfigured); err != nil {
		t.Fatalf("State.Get: %v", err)
	}
	if !unconfigured.Status.IsNull() || unconfigured.Name.ValueString() != "Proxmox VE No-Subscription Repository" {
		t.Fatalf("unexpected unconfigured state: %+v", unconfigured)
	}
}

// TestPveAptStandardRepositoryDataSource_ReadUnknownHandleErrors verifies an
// unknown handle surfaces an error diagnostic.
func TestPveAptStandardRepositoryDataSource_ReadUnknownHandleErrors(t *testing.T) {
	d := NewPveAptStandardRepositoryDataSource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := d.(*pveAptStandardRepositoryDataSource)
	if !ok {
		t.Fatalf("constructor returned %T", d)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"digest":"dd01","standard-repos":[{"handle":"enterprise","name":"Enterprise","status":true}]}}`)
	})
	ctx := context.Background()
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	attrTypes := map[string]tftypes.Type{"id": tftypes.String, "node": tftypes.String, "handle": tftypes.String, "name": tftypes.String, "status": tftypes.Bool}
	raw := tftypes.NewValue(tftypes.Object{AttributeTypes: attrTypes}, map[string]tftypes.Value{
		"id":     tftypes.NewValue(tftypes.String, nil),
		"node":   tftypes.NewValue(tftypes.String, "pve1"),
		"handle": tftypes.NewValue(tftypes.String, "does-not-exist"),
		"name":   tftypes.NewValue(tftypes.String, nil),
		"status": tftypes.NewValue(tftypes.Bool, nil),
	})
	readResp := &datasource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(tftypes.Object{AttributeTypes: attrTypes}, nil)}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw}}, readResp)
	if !readResp.Diagnostics.HasError() {
		t.Fatal("expected error diagnostics for unknown handle")
	}
}
