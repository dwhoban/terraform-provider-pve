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

// TestPveNotificationEndpointSendmail_DataSourceMetadataAndSchema covers the
// data source's type name and schema shape.
func TestPveNotificationEndpointSendmail_DataSourceMetadataAndSchema(t *testing.T) {
	d := NewPveNotificationEndpointSendmailDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveNotificationEndpointSendmail {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveNotificationEndpointSendmail)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"name", "mailto", "mailto_user", "from_address", "author", "comment", "disable"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
}

// TestPveNotificationEndpointSendmail_DataSourceRead verifies the single
// endpoint read decode against a fake API.
func TestPveNotificationEndpointSendmail_DataSourceRead(t *testing.T) {
	d := NewPveNotificationEndpointSendmailDataSource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := d.(*pveNotificationEndpointSendmailDataSource)
	if !ok {
		t.Fatalf("constructor returned %T", d)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet || req.URL.Path != "/cluster/notifications/endpoints/sendmail/mail1" {
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"name":"mail1","from-address":"pve@example.com","mailto":["ops@example.com"],"author":"PVE","comment":"Alerts","disable":false}}`)
	})
	ctx := context.Background()
	cfg := haDSConfig(t, d, ctx, map[string]tftypes.Value{
		"name": tftypes.NewValue(tftypes.String, "mail1"),
	})
	resp := &datasource.ReadResponse{State: haDSNullState(t, d, ctx)}
	impl.Read(ctx, datasource.ReadRequest{Config: cfg}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %s", diagnosticsError(resp.Diagnostics))
	}
	var got pveNotificationEndpointSendmailDataSourceModel
	if err := resp.State.Get(ctx, &got); err != nil {
		t.Fatalf("State.Get: %v", err)
	}
	if got.Name.ValueString() != "mail1" || got.FromAddress.ValueString() != "pve@example.com" || got.Author.ValueString() != "PVE" {
		t.Fatalf("data source = %+v", got)
	}
	if len(got.MailTo.Elements()) != 1 {
		t.Fatalf("mailto = %v", got.MailTo)
	}
}
