// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// metricsServerAttrTypes is the tftypes shape of the metrics server
// resource and data source models.
func metricsServerAttrTypes() map[string]tftypes.Type {
	return map[string]tftypes.Type{
		"id":                       tftypes.String,
		"type":                     tftypes.String,
		"server":                   tftypes.String,
		"port":                     tftypes.Number,
		"disable":                  tftypes.Bool,
		"mtu":                      tftypes.Number,
		"proto":                    tftypes.String,
		"path":                     tftypes.String,
		"timeout":                  tftypes.Number,
		"influxdb_proto":           tftypes.String,
		"organization":             tftypes.String,
		"bucket":                   tftypes.String,
		"token":                    tftypes.String,
		"api_path_prefix":          tftypes.String,
		"max_body_size":            tftypes.Number,
		"verify_certificate":       tftypes.Bool,
		"otel_compression":         tftypes.String,
		"otel_headers":             tftypes.String,
		"otel_max_body_size":       tftypes.Number,
		"otel_path":                tftypes.String,
		"otel_protocol":            tftypes.String,
		"otel_resource_attributes": tftypes.String,
		"otel_timeout":             tftypes.Number,
		"otel_verify_ssl":          tftypes.Bool,
	}
}

// metricsServerTestObject builds a complete model object, nulling every
// attribute absent from vals.
func metricsServerTestObject(vals map[string]tftypes.Value) tftypes.Value {
	attrTypes := metricsServerAttrTypes()
	for name, typ := range attrTypes {
		if _, ok := vals[name]; !ok {
			vals[name] = tftypes.NewValue(typ, nil)
		}
	}
	return tftypes.NewValue(tftypes.Object{AttributeTypes: attrTypes}, vals)
}

// TestPveMetricsServerResource_MetadataAndSchema covers the resource's full
// type name and the discriminator contract: oneOf enumerations inline in
// descriptions, ranges inline, and the secret marked sensitive.
func TestPveMetricsServerResource_MetadataAndSchema(t *testing.T) {
	r := NewPveMetricsServerResource()
	ctx := context.Background()
	metaResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
	if metaResp.TypeName != "pve_"+TypeNamePveMetricsServer {
		t.Fatalf("TypeName = %q, want pve_%s", metaResp.TypeName, TypeNamePveMetricsServer)
	}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
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
	if !attrs["id"].IsRequired() || !attrs["type"].IsRequired() || !attrs["server"].IsRequired() || !attrs["port"].IsRequired() {
		t.Fatal("id, type, server, and port should be Required")
	}
	if !attrs["token"].IsSensitive() {
		t.Fatal("token attribute should be Sensitive")
	}
	// Closed sets and ranges must be enumerated inline for tfplugindocs.
	enumChecks := map[string][]string{
		"type":             {"`graphite`", "`influxdb`", "`opentelemetry`"},
		"proto":            {"`udp`", "`tcp`"},
		"influxdb_proto":   {"`udp`", "`http`", "`https`"},
		"otel_compression": {"`none`", "`gzip`"},
		"otel_protocol":    {"`http`", "`https`"},
	}
	for attrName, values := range enumChecks {
		desc := attrs[attrName].GetMarkdownDescription()
		for _, v := range values {
			if !strings.Contains(desc, v) {
				t.Fatalf("%s description %q missing enum value %s", attrName, desc, v)
			}
		}
		if !strings.Contains(desc, "Must be one of") {
			t.Fatalf("%s description missing inline enumeration: %q", attrName, desc)
		}
	}
	rangeChecks := map[string]string{
		"port":         "between 1 and 65536",
		"mtu":          "between 512 and 65536",
		"otel_timeout": "between 1 and 10",
	}
	for attrName, want := range rangeChecks {
		desc := attrs[attrName].GetMarkdownDescription()
		if !strings.Contains(desc, want) {
			t.Fatalf("%s description %q missing range %q", attrName, desc, want)
		}
	}
}

// TestPveMetricsServerResource_ValidateConfigRejectsForeignFields verifies
// the type discriminator: attributes of the wrong plugin type are rejected
// with a diagnostic naming the attribute.
func TestPveMetricsServerResource_ValidateConfigRejectsForeignFields(t *testing.T) {
	r := NewPveMetricsServerResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveMetricsServerResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)

	graphite := metricsServerTestObject(map[string]tftypes.Value{
		"id":     tftypes.NewValue(tftypes.String, "graph1"),
		"type":   tftypes.NewValue(tftypes.String, "graphite"),
		"server": tftypes.NewValue(tftypes.String, "g.example.com"),
		"port":   tftypes.NewValue(tftypes.Number, 2003),
		"bucket": tftypes.NewValue(tftypes.String, "proxmox"),
	})
	resp := &resource.ValidateConfigResponse{}
	impl.ValidateConfig(ctx, resource.ValidateConfigRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: graphite},
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected ValidateConfig error for bucket on graphite server")
	}
	if !strings.Contains(diagnosticsError(resp.Diagnostics), "bucket") {
		t.Fatalf("diagnostic must name the offending attribute: %s", diagnosticsError(resp.Diagnostics))
	}

	influx := metricsServerTestObject(map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, "influx1"),
		"type":           tftypes.NewValue(tftypes.String, "influxdb"),
		"server":         tftypes.NewValue(tftypes.String, "i.example.com"),
		"port":           tftypes.NewValue(tftypes.Number, 8089),
		"bucket":         tftypes.NewValue(tftypes.String, "proxmox"),
		"influxdb_proto": tftypes.NewValue(tftypes.String, "http"),
		"proto":          tftypes.NewValue(tftypes.String, "tcp"),
	})
	resp = &resource.ValidateConfigResponse{}
	impl.ValidateConfig(ctx, resource.ValidateConfigRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: influx},
	}, resp)
	// bucket belongs to influxdb; proto does not.
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected ValidateConfig error for proto on influxdb server")
	}
	if strings.Contains(diagnosticsError(resp.Diagnostics), `"bucket"`) {
		t.Fatalf("bucket must be accepted for influxdb: %s", diagnosticsError(resp.Diagnostics))
	}
	if !strings.Contains(diagnosticsError(resp.Diagnostics), "proto") {
		t.Fatalf("diagnostic must name proto: %s", diagnosticsError(resp.Diagnostics))
	}
}

// TestPveMetricsServerResource_CreateAndDelete runs the full create (POST
// on the id leaf then read-back GET) and delete (DELETE) paths against a
// fake API, including the already-absent delete-is-success rule.
func TestPveMetricsServerResource_CreateAndDelete(t *testing.T) {
	exists := false
	r := NewPveMetricsServerResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveMetricsServerResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case req.Method == http.MethodPost && req.URL.Path == "/cluster/metrics/server/influx1":
			body, _ := io.ReadAll(req.Body)
			for _, want := range []string{`"id":"influx1"`, `"type":"influxdb"`, `"server":"influx.example.com"`, `"port":8089`, `"token":"s3cret"`, `"bucket":"proxmox"`} {
				if !strings.Contains(string(body), want) {
					t.Fatalf("create body missing %s: %q", want, body)
				}
			}
			exists = true
			_, _ = io.WriteString(w, `{"data":null}`)
		case req.Method == http.MethodGet && req.URL.Path == "/cluster/metrics/server/influx1":
			if !exists {
				w.WriteHeader(http.StatusNotFound)
				_, _ = io.WriteString(w, `{"errors":"no such metric server"}`)
				return
			}
			_, _ = io.WriteString(w, `{"data":{"id":"influx1","type":"influxdb","server":"influx.example.com",`+
				`"port":8089,"influxdbproto":"http","bucket":"proxmox","token":"s3cret","organization":"pve",`+
				`"verify-certificate":1,"disable":0}}`)
		case req.Method == http.MethodDelete && req.URL.Path == "/cluster/metrics/server/influx1":
			exists = false
			_, _ = io.WriteString(w, `{"data":null}`)
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
		}
	})
	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	raw := metricsServerTestObject(map[string]tftypes.Value{
		"id":                 tftypes.NewValue(tftypes.String, "influx1"),
		"type":               tftypes.NewValue(tftypes.String, "influxdb"),
		"server":             tftypes.NewValue(tftypes.String, "influx.example.com"),
		"port":               tftypes.NewValue(tftypes.Number, 8089),
		"influxdb_proto":     tftypes.NewValue(tftypes.String, "http"),
		"organization":       tftypes.NewValue(tftypes.String, "pve"),
		"bucket":             tftypes.NewValue(tftypes.String, "proxmox"),
		"token":              tftypes.NewValue(tftypes.String, "s3cret"),
		"verify_certificate": tftypes.NewValue(tftypes.Bool, true),
	})

	createResp := &resource.CreateResponse{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(tftypes.Object{AttributeTypes: metricsServerAttrTypes()}, nil)},
	}
	r.Create(ctx, resource.CreateRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw},
		Plan:   tfsdk.Plan{Schema: schemaResp.Schema, Raw: raw},
	}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create diagnostics: %s", diagnosticsError(createResp.Diagnostics))
	}
	var created pveMetricsServerResourceModel
	if err := createResp.State.Get(ctx, &created); err != nil {
		t.Fatalf("State.Get after create: %v", err)
	}
	if created.ID.ValueString() != "influx1" || created.Type.ValueString() != "influxdb" {
		t.Fatalf("created identity = %s/%s", created.ID.ValueString(), created.Type.ValueString())
	}
	if created.Organization.ValueString() != "pve" || created.Port.ValueInt64() != 8089 {
		t.Fatalf("created read-back fields = %+v", created)
	}
	if !created.VerifyCertificate.ValueBool() || created.Disable.ValueBool() {
		t.Fatalf("created bool fields = verify=%v disable=%v", created.VerifyCertificate.ValueBool(), created.Disable.ValueBool())
	}

	deleteResp := &resource.DeleteResponse{}
	r.Delete(ctx, resource.DeleteRequest{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: createResp.State.Raw},
	}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("Delete diagnostics: %s", diagnosticsError(deleteResp.Diagnostics))
	}
}

// TestPveMetricsServerResource_UpdateClearsRemovedFields verifies the PUT
// carries the required scalars and translates attributes cleared in the
// plan into the PVE `delete` query parameter (wire names).
func TestPveMetricsServerResource_UpdateClearsRemovedFields(t *testing.T) {
	var sawQuery url.Values
	var sawBody string
	r := NewPveMetricsServerResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveMetricsServerResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch req.Method {
		case http.MethodPut:
			if req.URL.Path != "/cluster/metrics/server/influx1" {
				t.Fatalf("unexpected PUT path: %s", req.URL.Path)
			}
			sawQuery = req.URL.Query()
			raw, _ := io.ReadAll(req.Body)
			sawBody = string(raw)
			_, _ = io.WriteString(w, `{"data":null}`)
		case http.MethodGet:
			_, _ = io.WriteString(w, `{"data":{"id":"influx1","type":"influxdb","server":"influx.example.com",`+
				`"port":8089,"influxdbproto":"http","disable":0}}`)
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
		}
	})
	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)

	stateRaw := metricsServerTestObject(map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, "influx1"),
		"type":           tftypes.NewValue(tftypes.String, "influxdb"),
		"server":         tftypes.NewValue(tftypes.String, "influx.example.com"),
		"port":           tftypes.NewValue(tftypes.Number, 8089),
		"influxdb_proto": tftypes.NewValue(tftypes.String, "http"),
		"bucket":         tftypes.NewValue(tftypes.String, "proxmox"),
		"token":          tftypes.NewValue(tftypes.String, "s3cret"),
		"organization":   tftypes.NewValue(tftypes.String, "pve"),
	})
	planRaw := metricsServerTestObject(map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, "influx1"),
		"type":           tftypes.NewValue(tftypes.String, "influxdb"),
		"server":         tftypes.NewValue(tftypes.String, "influx.example.com"),
		"port":           tftypes.NewValue(tftypes.Number, 8089),
		"influxdb_proto": tftypes.NewValue(tftypes.String, "http"),
	})

	updateResp := &resource.UpdateResponse{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: stateRaw},
	}
	r.Update(ctx, resource.UpdateRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: planRaw},
		Plan:   tfsdk.Plan{Schema: schemaResp.Schema, Raw: planRaw},
		State:  tfsdk.State{Schema: schemaResp.Schema, Raw: stateRaw},
	}, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("Update diagnostics: %s", diagnosticsError(updateResp.Diagnostics))
	}
	if got := sawQuery.Get("delete"); got != "organization,bucket,token" {
		t.Fatalf("delete param = %q, want organization,bucket,token", got)
	}
	for _, want := range []string{`"id":"influx1"`, `"server":"influx.example.com"`, `"port":8089`} {
		if !strings.Contains(sawBody, want) {
			t.Fatalf("update body missing %s: %q", want, sawBody)
		}
	}
	if strings.Contains(sawBody, `"type"`) {
		t.Fatalf("update body must not carry type (pin's PUT has no type parameter): %q", sawBody)
	}
	var updated pveMetricsServerResourceModel
	if err := updateResp.State.Get(ctx, &updated); err != nil {
		t.Fatalf("State.Get after update: %v", err)
	}
	if !updated.Token.IsNull() || !updated.Bucket.IsNull() {
		t.Fatalf("cleared fields must read back null: token=%v bucket=%v", updated.Token, updated.Bucket)
	}
}

// TestPveMetricsServerResource_Read404Removes verifies Read drops the
// resource from state when the server vanished out of band.
func TestPveMetricsServerResource_Read404Removes(t *testing.T) {
	r := NewPveMetricsServerResource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := r.(*pveMetricsServerResource)
	if !ok {
		t.Fatalf("constructor returned %T", r)
	}
	impl.client = newHaTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"errors":"no such metric server"}`)
	})
	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	state := tfsdk.State{Schema: schemaResp.Schema, Raw: metricsServerTestObject(map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "gone"),
	})}
	readResp := &resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %s", diagnosticsError(readResp.Diagnostics))
	}
	if !readResp.State.Raw.IsNull() {
		t.Fatalf("expected state to be removed, got %v", readResp.State.Raw)
	}
}
