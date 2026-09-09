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

// TestPveNotificationEndpointWebhook_DataSourceMetadataAndSchema covers the
// data source's type name and schema shape.
func TestPveNotificationEndpointWebhook_DataSourceMetadataAndSchema(t *testing.T) {
	d := NewPveNotificationEndpointWebhookDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveNotificationEndpointWebhook {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveNotificationEndpointWebhook)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	for _, key := range []string{"name", "url", "method", "body", "headers", "secrets", "comment", "disable"} {
		if schemaResp.Schema.Attributes[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
}

// TestPveNotificationEndpointWebhook_DataSourceRead verifies the single
// endpoint read decode against a fake API.
func TestPveNotificationEndpointWebhook_DataSourceRead(t *testing.T) {
	d := NewPveNotificationEndpointWebhookDataSource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := d.(*pveNotificationEndpointWebhookDataSource)
	if !ok {
		t.Fatalf("constructor returned %T", d)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet || req.URL.Path != "/cluster/notifications/endpoints/webhook/hook1" {
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"name":"hook1","url":"https://hooks.example.com/pve","method":"post","body":"e1sidXJsIjoiaG9vazEifQ==","header":["name=X-Token,value=YWJj"],"secret":["name=HMAC,value=a2V5"],"disable":false}}`)
	})
	ctx := context.Background()
	cfg := haDSConfig(t, d, ctx, map[string]tftypes.Value{
		"name": tftypes.NewValue(tftypes.String, "hook1"),
	})
	resp := &datasource.ReadResponse{State: haDSNullState(t, d, ctx)}
	impl.Read(ctx, datasource.ReadRequest{Config: cfg}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %s", diagnosticsError(resp.Diagnostics))
	}
	var got pveNotificationEndpointWebhookDataSourceModel
	if err := resp.State.Get(ctx, &got); err != nil {
		t.Fatalf("State.Get: %v", err)
	}
	if got.Method.ValueString() != "post" {
		t.Fatalf("data source = %+v", got)
	}
	tokenVal, tokenOK := got.Headers.Elements()["X-Token"].(types.String)
	// safetyassert: elements of a string map are always types.String values.
	if !tokenOK || tokenVal.ValueString() != "abc" {
		t.Fatalf("headers = %v", got.Headers)
	}
	hmacVal, hmacOK := got.Secrets.Elements()["HMAC"].(types.String)
	// safetyassert: elements of a string map are always types.String values.
	if !hmacOK || hmacVal.ValueString() != "key" {
		t.Fatalf("secrets = %v", got.Secrets)
	}
}
