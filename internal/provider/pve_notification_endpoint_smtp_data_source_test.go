// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// TestPveNotificationEndpointSMTP_DataSourceMetadataAndSchema covers the
// data source's type name and schema shape.
func TestPveNotificationEndpointSMTP_DataSourceMetadataAndSchema(t *testing.T) {
	d := NewPveNotificationEndpointSMTPDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveNotificationEndpointSmtp {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveNotificationEndpointSmtp)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"name", "server", "from_address", "username", "port", "mode", "mailto", "mailto_user", "author", "comment", "disable"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
}

// TestPveNotificationEndpointSMTP_DataSourceRead verifies the single
// endpoint read decode against a fake API.
func TestPveNotificationEndpointSMTP_DataSourceRead(t *testing.T) {
	d := NewPveNotificationEndpointSMTPDataSource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := d.(*pveNotificationEndpointSMTPDataSource)
	if !ok {
		t.Fatalf("constructor returned %T", d)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet || req.URL.Path != "/cluster/notifications/endpoints/smtp/smtp1" {
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"name":"smtp1","server":"smtp.example.com","from-address":"pve@example.com","username":"mailer","port":587,"mode":"starttls","mailto":["ops@example.com"],"comment":"Relay","disable":0}}`)
	})
	ctx := context.Background()
	cfg := haDSConfig(t, d, ctx, map[string]tftypes.Value{
		"name": tftypes.NewValue(tftypes.String, "smtp1"),
	})
	resp := &datasource.ReadResponse{State: haDSNullState(t, d, ctx)}
	impl.Read(ctx, datasource.ReadRequest{Config: cfg}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %s", diagnosticsError(resp.Diagnostics))
	}
	var got pveNotificationEndpointSMTPDataSourceModel
	if err := resp.State.Get(ctx, &got); err != nil {
		t.Fatalf("State.Get: %v", err)
	}
	if got.Server.ValueString() != "smtp.example.com" || got.Mode.ValueString() != "starttls" {
		t.Fatalf("data source = %+v", got)
	}
	if !got.Port.Equal(types.Int64Value(587)) {
		t.Fatalf("port = %v", got.Port)
	}
}
