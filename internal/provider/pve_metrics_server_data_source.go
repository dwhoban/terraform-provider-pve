// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ datasource.DataSource              = &pveMetricsServerDataSource{}
	_ datasource.DataSourceWithConfigure = &pveMetricsServerDataSource{}
)

// NewPveMetricsServerDataSource returns the data source implementation.
func NewPveMetricsServerDataSource() datasource.DataSource {
	return &pveMetricsServerDataSource{}
}

// pveMetricsServerDataSource reads a single external metric server
// configuration (GET /cluster/metrics/server/{id}), including the
// type-specific fields present upstream.
type pveMetricsServerDataSource struct {
	client *pveclient.Client
}

// pveMetricsServerDataSourceModel is the Terraform-facing shape.
type pveMetricsServerDataSourceModel struct {
	ID      types.String `tfsdk:"id"`
	Type    types.String `tfsdk:"type"`
	Server  types.String `tfsdk:"server"`
	Port    types.Int64  `tfsdk:"port"`
	Disable types.Bool   `tfsdk:"disable"`
	MTU     types.Int64  `tfsdk:"mtu"`

	// Graphite plugin type.
	Proto   types.String `tfsdk:"proto"`
	Path    types.String `tfsdk:"path"`
	Timeout types.Int64  `tfsdk:"timeout"`

	// InfluxDB plugin type.
	InfluxDBProto     types.String `tfsdk:"influxdb_proto"`
	Organization      types.String `tfsdk:"organization"`
	Bucket            types.String `tfsdk:"bucket"`
	Token             types.String `tfsdk:"token"`
	APIPathPrefix     types.String `tfsdk:"api_path_prefix"`
	MaxBodySize       types.Int64  `tfsdk:"max_body_size"`
	VerifyCertificate types.Bool   `tfsdk:"verify_certificate"`

	// OpenTelemetry plugin type.
	OTelCompression        types.String `tfsdk:"otel_compression"`
	OTelHeaders            types.String `tfsdk:"otel_headers"`
	OTelMaxBodySize        types.Int64  `tfsdk:"otel_max_body_size"`
	OTelPath               types.String `tfsdk:"otel_path"`
	OTelProtocol           types.String `tfsdk:"otel_protocol"`
	OTelResourceAttributes types.String `tfsdk:"otel_resource_attributes"`
	OTelTimeout            types.Int64  `tfsdk:"otel_timeout"`
	OTelVerifySSL          types.Bool   `tfsdk:"otel_verify_ssl"`
}

// Metadata implements datasource.DataSource.
func (d *pveMetricsServerDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveMetricsServer
}

// metricsServerDataSourceComputed renders one computed read attribute of
// type string.
func metricsServerDataSourceComputed(description string) schema.Attribute {
	return schema.StringAttribute{
		Computed:            true,
		MarkdownDescription: description,
	}
}

// metricsServerDataSourceComputedInt64 renders one computed read attribute
// of type integer.
func metricsServerDataSourceComputedInt64(description string) schema.Attribute {
	return schema.Int64Attribute{
		Computed:            true,
		MarkdownDescription: description,
	}
}

// Schema implements datasource.DataSource.
func (d *pveMetricsServerDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads an external metric server configuration from `GET /cluster/metrics/server/{id}`. Attributes that do not apply to the server's plugin `type` are null.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The metric server identifier to look up.",
			},
			"type": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Plugin type of the metric server. One of `graphite`, `influxdb`, `opentelemetry`.",
			},
			"server": metricsServerDataSourceComputed("Server DNS name or IP address."),
			"port":   metricsServerDataSourceComputedInt64("Server network port."),
			"disable": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether the plugin is disabled.",
			},
			"mtu": metricsServerDataSourceComputedInt64("MTU for metrics transmission over UDP; only set for `graphite` or `influxdb` servers."),
			"proto": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Protocol used to send graphite data. One of `udp`, `tcp`; only set for `type` = `graphite`.",
			},
			"path":    metricsServerDataSourceComputed("Root graphite path; only set for `type` = `graphite`."),
			"timeout": metricsServerDataSourceComputedInt64("Graphite TCP socket timeout in seconds; only set for `type` = `graphite`."),
			"influxdb_proto": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "InfluxDB transport protocol. One of `udp`, `http`, `https`; only set for `type` = `influxdb`.",
			},
			"organization": metricsServerDataSourceComputed("The InfluxDB organization (http v2 API only)."),
			"bucket":       metricsServerDataSourceComputed("The InfluxDB bucket/db (http v2 API only)."),
			"token": schema.StringAttribute{
				Computed:            true,
				Sensitive:           true,
				MarkdownDescription: "The InfluxDB access token; if the v2 compatibility API is used, this carries `user:password`.",
			},
			"api_path_prefix": metricsServerDataSourceComputed("API path prefix inserted between `<host>:<port>/` and `/api2/` when the InfluxDB service runs behind a reverse proxy."),
			"max_body_size":   metricsServerDataSourceComputedInt64("InfluxDB max-body-size in bytes; requests are batched up to this size."),
			"verify_certificate": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether certificate verification is enabled for https endpoints.",
			},
			"otel_compression": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Compression algorithm for requests. One of `none`, `gzip`; only set for `type` = `opentelemetry`.",
			},
			"otel_headers":       metricsServerDataSourceComputed("Custom HTTP headers in JSON format, base64 encoded."),
			"otel_max_body_size": metricsServerDataSourceComputedInt64("Maximum request body size in bytes."),
			"otel_path":          metricsServerDataSourceComputed("OTLP endpoint path."),
			"otel_protocol": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "HTTP protocol. One of `http`, `https`; only set for `type` = `opentelemetry`.",
			},
			"otel_resource_attributes": metricsServerDataSourceComputed("Additional resource attributes as JSON, base64 encoded."),
			"otel_timeout":             metricsServerDataSourceComputedInt64("HTTP request timeout in seconds."),
			"otel_verify_ssl": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether SSL certificates are verified.",
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveMetricsServerDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = haConfigureDataSource(req, resp)
}

// Read implements datasource.DataSource.
func (d *pveMetricsServerDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveMetricsServerDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Error reading pve_metrics_server", "provider client is not configured")
		return
	}

	srv, err := d.client.GetMetricsServer(ctx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_metrics_server",
			fmt.Sprintf("reading metric server %s: %s", data.ID.ValueString(), err),
		)
		return
	}
	data.Type = nodeNetworkStringToTF(srv.Type)
	data.Server = nodeNetworkStringToTF(srv.Server)
	data.Port = metricsServerInt64Value(srv.Port)
	data.Disable = nodeNetworkBoolPtrToTF(srv.Disable)
	data.MTU = metricsServerInt64Value(srv.MTU)
	data.Proto = nodeNetworkStringToTF(srv.Proto)
	data.Path = nodeNetworkStringToTF(srv.Path)
	data.Timeout = metricsServerInt64Value(srv.Timeout)
	data.InfluxDBProto = nodeNetworkStringToTF(srv.InfluxDBProto)
	data.Organization = nodeNetworkStringToTF(srv.Organization)
	data.Bucket = nodeNetworkStringToTF(srv.Bucket)
	data.Token = nodeNetworkStringToTF(srv.Token)
	data.APIPathPrefix = nodeNetworkStringToTF(srv.APIPathPrefix)
	data.MaxBodySize = metricsServerInt64Value(srv.MaxBodySize)
	data.VerifyCertificate = nodeNetworkBoolPtrToTF(srv.VerifyCertificate)
	data.OTelCompression = nodeNetworkStringToTF(srv.OTelCompression)
	data.OTelHeaders = nodeNetworkStringToTF(srv.OTelHeaders)
	data.OTelMaxBodySize = metricsServerInt64Value(srv.OTelMaxBodySize)
	data.OTelPath = nodeNetworkStringToTF(srv.OTelPath)
	data.OTelProtocol = nodeNetworkStringToTF(srv.OTelProtocol)
	data.OTelResourceAttributes = nodeNetworkStringToTF(srv.OTelResourceAttributes)
	data.OTelTimeout = metricsServerInt64Value(srv.OTelTimeout)
	data.OTelVerifySSL = nodeNetworkBoolPtrToTF(srv.OTelVerifySSL)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
