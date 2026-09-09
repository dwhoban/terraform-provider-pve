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
)

// TestPveAcmePluginsDataSource_MetadataAndSchema covers the plugins data
// source's type name and schema shape.
func TestPveAcmePluginsDataSource_MetadataAndSchema(t *testing.T) {
	d := NewPveAcmePluginsDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveAcmePlugins {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveAcmePlugins)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"type", "plugins"} {
		if _, ok := schemaResp.Schema.Attributes[key]; !ok {
			t.Fatalf("missing attribute %q", key)
		}
	}
	if !schemaResp.Schema.Attributes["plugins"].IsComputed() {
		t.Fatal("plugins attribute should be Computed")
	}
}

// TestPveAcmePluginsDataSource_Read runs the listing against a fake API,
// including the boolish disable decode and the built-in standalone entry.
func TestPveAcmePluginsDataSource_Read(t *testing.T) {
	d := NewPveAcmePluginsDataSource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := d.(*pveAcmePluginsDataSource)
	if !ok {
		t.Fatalf("constructor returned %T", d)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if req.Method != http.MethodGet || req.URL.Path != "/cluster/acme/plugins" {
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
		}
		_, _ = io.WriteString(w, `{"data":[{"plugin":"pdns","type":"dns","api":"pdns","data":"a2V5","disable":1,"nodes":"pve1,pve2","validation-delay":60},{"plugin":"standalone","type":"standalone"}]}`)
	})
	ctx := context.Background()
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	raw := backupRawFromSchema(ctx, t, schemaResp.Schema.Attributes, nil)
	readResp := &datasource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema, Raw: raw}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw}}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %s", diagnosticsError(readResp.Diagnostics))
	}
	var state pveAcmePluginsDataSourceModel
	if err := readResp.State.Get(ctx, &state); err != nil {
		t.Fatalf("State.Get: %v", err)
	}
	if len(state.Plugins) != 2 || state.Plugins[0].API.ValueString() != "pdns" || state.Plugins[1].Type.ValueString() != "standalone" {
		t.Fatalf("plugins state = %+v", state.Plugins)
	}
	if !state.Plugins[0].Disable.ValueBool() {
		t.Fatalf("boolish disable decode = %+v", state.Plugins[0])
	}
	if len(state.Plugins[0].Nodes.Elements()) != 2 {
		t.Fatalf("nodes split = %+v", state.Plugins[0].Nodes)
	}
}
