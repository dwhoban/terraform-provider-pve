// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// TestPveNotificationEndpointGoty_DataSourceMetadataAndSchema covers the
// data source's type name and schema shape.
func TestPveNotificationEndpointGoty_DataSourceMetadataAndSchema(t *testing.T) {
	d := NewPveNotificationEndpointGotyDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveNotificationEndpointGoty {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveNotificationEndpointGoty)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"name", "server", "comment", "disable"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
}

// TestPveNotificationEndpointGoty_DataSourceRead verifies the single
// endpoint read decode against a fake API.
func TestPveNotificationEndpointGoty_DataSourceRead(t *testing.T) {
	d := NewPveNotificationEndpointGotyDataSource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := d.(*pveNotificationEndpointGotyDataSource)
	if !ok {
		t.Fatalf("constructor returned %T", d)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet || req.URL.Path != "/cluster/notifications/endpoints/gotify/got1" {
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"name":"got1","server":"https://gotify.example.com","comment":"CI","disable":false}}`)
	})
	ctx := context.Background()
	cfg := haDSConfig(t, d, ctx, map[string]tftypes.Value{
		"name": tftypes.NewValue(tftypes.String, "got1"),
	})
	resp := &datasource.ReadResponse{State: haDSNullState(t, d, ctx)}
	impl.Read(ctx, datasource.ReadRequest{Config: cfg}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %s", diagnosticsError(resp.Diagnostics))
	}
	var got pveNotificationEndpointGotyDataSourceModel
	if err := resp.State.Get(ctx, &got); err != nil {
		t.Fatalf("State.Get: %v", err)
	}
	if got.Server.ValueString() != "https://gotify.example.com" || got.Comment.ValueString() != "CI" {
		t.Fatalf("data source = %+v", got)
	}
	if got.Disable.ValueBool() {
		t.Fatalf("disable = %v, want false", got.Disable.ValueBool())
	}
}
