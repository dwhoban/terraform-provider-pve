// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// TestPveMetricsServerDataSource_MetadataAndSchema covers the data source's
// full type name and computed attribute set, including the inline
// enumerations for read-only closed sets.
func TestPveMetricsServerDataSource_MetadataAndSchema(t *testing.T) {
	d := NewPveMetricsServerDataSource()
	ctx := context.Background()
	metaResp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveMetricsServer {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveMetricsServer)
	}
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	attrs := schemaResp.Schema.Attributes
	for _, key := range []string{
		"id", "type", "server", "port", "disable", "mtu",
		"proto", "path", "timeout",
		"influxdb_proto", "organization", "bucket", "token", "api_path_prefix",
		"max_body_size", "verify_certificate",
		"otel_compression", "otel_headers", "otel_max_body_size", "otel_path",
		"otel_protocol", "otel_resource_attributes", "otel_timeout", "otel_verify_ssl",
	} {
		if attrs[key] == nil {
			t.Fatalf("schema missing %s attribute", key)
		}
	}
	if !attrs["id"].IsRequired() {
		t.Fatal("id attribute should be Required")
	}
	for _, key := range []string{"type", "server", "port", "token", "influxdb_proto", "otel_protocol"} {
		if !attrs[key].IsComputed() {
			t.Fatalf("%s attribute should be Computed", key)
		}
	}
	for attrName, values := range map[string][]string{
		"type":             {"`graphite`", "`influxdb`", "`opentelemetry`"},
		"proto":            {"`udp`", "`tcp`"},
		"influxdb_proto":   {"`udp`", "`http`", "`https`"},
		"otel_compression": {"`none`", "`gzip`"},
		"otel_protocol":    {"`http`", "`https`"},
	} {
		desc := attrs[attrName].GetMarkdownDescription()
		for _, v := range values {
			if !strings.Contains(desc, v) {
				t.Fatalf("%s description %q missing enum value %s", attrName, desc, v)
			}
		}
		if !strings.Contains(desc, "One of") {
			t.Fatalf("%s description missing inline enumeration: %q", attrName, desc)
		}
	}
}

// TestPveMetricsServerDataSource_Read decodes GET
// /cluster/metrics/server/{id} into the computed attribute set, including
// the secret token and the 0/1 disable encoding.
func TestPveMetricsServerDataSource_Read(t *testing.T) {
	d := NewPveMetricsServerDataSource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := d.(*pveMetricsServerDataSource)
	if !ok {
		t.Fatalf("constructor returned %T", d)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet || req.URL.Path != "/cluster/metrics/server/influx1" {
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"id":"influx1","type":"influxdb","server":"influx.example.com",`+
			`"port":8089,"influxdbproto":"https","bucket":"proxmox","organization":"pve","token":"abc",`+
			`"verify-certificate":0,"disable":1,"max-body-size":"25000000"}}`)
	})
	ctx := context.Background()
	schemaResp := &datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	config := tfsdk.Config{Schema: schemaResp.Schema, Raw: metricsServerTestObject(map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "influx1"),
	})}
	readResp := &datasource.ReadResponse{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: metricsServerTestObject(map[string]tftypes.Value{
			"id": tftypes.NewValue(tftypes.String, "influx1"),
		})},
	}
	d.Read(ctx, datasource.ReadRequest{Config: config}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %s", diagnosticsError(readResp.Diagnostics))
	}
	var got pveMetricsServerDataSourceModel
	if err := readResp.State.Get(ctx, &got); err != nil {
		t.Fatalf("State.Get: %v", err)
	}
	if got.Type.ValueString() != "influxdb" || got.Server.ValueString() != "influx.example.com" || got.Port.ValueInt64() != 8089 {
		t.Fatalf("unexpected identity fields: %+v", got)
	}
	if got.InfluxDBProto.ValueString() != "https" || got.Bucket.ValueString() != "proxmox" || got.Organization.ValueString() != "pve" {
		t.Fatalf("unexpected influx fields: %+v", got)
	}
	if got.Token.ValueString() != "abc" {
		t.Fatalf("token = %q, want abc", got.Token.ValueString())
	}
	if !got.Disable.ValueBool() || got.VerifyCertificate.ValueBool() {
		t.Fatalf("unexpected bools: disable=%v verify=%v", got.Disable.ValueBool(), got.VerifyCertificate.ValueBool())
	}
	if got.MaxBodySize.ValueInt64() != 25000000 {
		t.Fatalf("max_body_size = %v, want 25000000 (string wire encoding)", got.MaxBodySize)
	}
	// Graphite and OpenTelemetry fields stay null for an influxdb server.
	if !got.Proto.IsNull() || !got.Timeout.IsNull() || !got.OTelPath.IsNull() {
		t.Fatalf("foreign type fields must be null: %+v", got)
	}
}
