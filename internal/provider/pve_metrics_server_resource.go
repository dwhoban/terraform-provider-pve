// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ resource.Resource                   = &pveMetricsServerResource{}
	_ resource.ResourceWithConfigure      = &pveMetricsServerResource{}
	_ resource.ResourceWithImportState    = &pveMetricsServerResource{}
	_ resource.ResourceWithValidateConfig = &pveMetricsServerResource{}
)

// NewPveMetricsServerResource returns the resource implementation.
func NewPveMetricsServerResource() resource.Resource {
	return &pveMetricsServerResource{}
}

// pveMetricsServerResource manages one external metric server via
// /cluster/metrics/server/{id}. PVE's metric server configuration is a
// section config with a plugin `type` discriminator (graphite, influxdb,
// opentelemetry), so one resource carries the union of type-conditional
// attributes and ValidateConfig rejects attributes that do not belong to
// the configured type.
type pveMetricsServerResource struct {
	client *pveclient.Client
}

// pveMetricsServerResourceModel is the Terraform-facing shape.
type pveMetricsServerResourceModel struct {
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

// metricsServerTypeField describes one type-conditional attribute: its
// Terraform attribute name, the PVE wire field name, and the plugin types
// it applies to.
type metricsServerTypeField struct {
	attr  string
	wire  string
	types []string
}

// metricsServerFieldOwnership is the full type-conditional attribute set
// from the api-spec pin. ValidateConfig uses it to reject attributes set
// for the wrong type; the update diff uses `wire` to translate attributes
// cleared in the plan into the PVE `delete` parameter.
var metricsServerFieldOwnership = []metricsServerTypeField{
	{attr: "mtu", wire: "mtu", types: []string{"graphite", "influxdb"}},
	{attr: "proto", wire: "proto", types: []string{"graphite"}},
	{attr: "path", wire: "path", types: []string{"graphite"}},
	{attr: "timeout", wire: "timeout", types: []string{"graphite"}},
	{attr: "influxdb_proto", wire: "influxdbproto", types: []string{"influxdb"}},
	{attr: "organization", wire: "organization", types: []string{"influxdb"}},
	{attr: "bucket", wire: "bucket", types: []string{"influxdb"}},
	{attr: "token", wire: "token", types: []string{"influxdb"}},
	{attr: "api_path_prefix", wire: "api-path-prefix", types: []string{"influxdb"}},
	{attr: "max_body_size", wire: "max-body-size", types: []string{"influxdb"}},
	{attr: "verify_certificate", wire: "verify-certificate", types: []string{"influxdb"}},
	{attr: "otel_compression", wire: "otel-compression", types: []string{"opentelemetry"}},
	{attr: "otel_headers", wire: "otel-headers", types: []string{"opentelemetry"}},
	{attr: "otel_max_body_size", wire: "otel-max-body-size", types: []string{"opentelemetry"}},
	{attr: "otel_path", wire: "otel-path", types: []string{"opentelemetry"}},
	{attr: "otel_protocol", wire: "otel-protocol", types: []string{"opentelemetry"}},
	{attr: "otel_resource_attributes", wire: "otel-resource-attributes", types: []string{"opentelemetry"}},
	{attr: "otel_timeout", wire: "otel-timeout", types: []string{"opentelemetry"}},
	{attr: "otel_verify_ssl", wire: "otel-verify-ssl", types: []string{"opentelemetry"}},
}

// Metadata implements resource.Resource.
func (r *pveMetricsServerResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveMetricsServer
}

// Schema implements resource.Resource.
func (r *pveMetricsServerResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages an external metric server configuration (`/cluster/metrics/server/{id}`). One resource covers every PVE plugin type; `type` selects the plugin and each type-conditional attribute below names the types it applies to. Setting an attribute that does not belong to the configured type is rejected at plan time. Changing `id` or `type` forces recreation. Every mutation is synchronous per the pin (no task is spawned).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The unique identifier of the metric server (PVE `pve-configid` format). Changing this value forces recreation.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"type": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Plugin type of the metric server. Must be one of: `graphite`, `influxdb`, `opentelemetry`. Changing this value forces recreation.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.OneOf("graphite", "influxdb", "opentelemetry"),
				},
			},
			"server": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Server DNS name or IP address (PVE `address` format).",
			},
			"port": schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "Server network port. Must be between 1 and 65536 inclusive.",
				Validators: []validator.Int64{
					int64validator.Between(1, 65536),
				},
			},
			"disable": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Flag to disable the plugin.",
			},
			"mtu": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "MTU for metrics transmission over UDP (PVE default: `1500`). Only applies to `type` = `graphite` or `influxdb`. Must be between 512 and 65536 inclusive.",
				Validators: []validator.Int64{
					int64validator.Between(512, 65536),
				},
			},
			"proto": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Protocol to send graphite data (PVE default: `udp`). Only applies to `type` = `graphite`. Must be one of: `udp`, `tcp`.",
				Validators: []validator.String{
					stringvalidator.OneOf("udp", "tcp"),
				},
			},
			"path": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Root graphite path (for example `proxmox.mycluster.mykey`). Only applies to `type` = `graphite`.",
			},
			"timeout": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Graphite TCP socket timeout in seconds (PVE default: `1`). Only applies to `type` = `graphite`. Must be 0 or greater.",
				Validators: []validator.Int64{
					int64validator.AtLeast(0),
				},
			},
			"influxdb_proto": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "InfluxDB transport protocol (PVE default: `udp`). Only applies to `type` = `influxdb`. Must be one of: `udp`, `http`, `https`.",
				Validators: []validator.String{
					stringvalidator.OneOf("udp", "http", "https"),
				},
			},
			"organization": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The InfluxDB organization. Only necessary when using the http v2 API; has no meaning when using the v2 compatibility API. Only applies to `type` = `influxdb`.",
			},
			"bucket": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The InfluxDB bucket/db. Only necessary when using the http v2 API. Only applies to `type` = `influxdb`.",
			},
			"token": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "The InfluxDB access token. Only necessary when using the http v2 API; if the v2 compatibility API is used, pass `user:password` instead. Only applies to `type` = `influxdb`.",
			},
			"api_path_prefix": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "An API path prefix inserted between `<host>:<port>/` and `/api2/`. Can be useful if the InfluxDB service runs behind a reverse proxy. Only applies to `type` = `influxdb`.",
			},
			"max_body_size": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "InfluxDB max-body-size in bytes; requests are batched up to this size (PVE default: `25000000`). Only applies to `type` = `influxdb`. Must be 1 or greater.",
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
				},
			},
			"verify_certificate": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Set to false to disable certificate verification for https endpoints (PVE default: `true`). Only applies to `type` = `influxdb`.",
			},
			"otel_compression": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Compression algorithm for requests (PVE default: `gzip`). Only applies to `type` = `opentelemetry`. Must be one of: `none`, `gzip`.",
				Validators: []validator.String{
					stringvalidator.OneOf("none", "gzip"),
				},
			},
			"otel_headers": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Custom HTTP headers in JSON format, base64 encoded. Only applies to `type` = `opentelemetry`. At most 1024 characters.",
				Validators: []validator.String{
					stringvalidator.UTF8LengthAtMost(1024),
				},
			},
			"otel_max_body_size": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Maximum request body size in bytes (PVE default: `10000000`). Only applies to `type` = `opentelemetry`. Must be 1024 or greater.",
				Validators: []validator.Int64{
					int64validator.AtLeast(1024),
				},
			},
			"otel_path": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "OTLP endpoint path (PVE default: `/v1/metrics`). Only applies to `type` = `opentelemetry`.",
			},
			"otel_protocol": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "HTTP protocol (PVE default: `https`). Only applies to `type` = `opentelemetry`. Must be one of: `http`, `https`.",
				Validators: []validator.String{
					stringvalidator.OneOf("http", "https"),
				},
			},
			"otel_resource_attributes": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Additional resource attributes as JSON, base64 encoded. Only applies to `type` = `opentelemetry`. At most 1024 characters.",
				Validators: []validator.String{
					stringvalidator.UTF8LengthAtMost(1024),
				},
			},
			"otel_timeout": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "HTTP request timeout in seconds (PVE default: `5`). Only applies to `type` = `opentelemetry`. Must be between 1 and 10 inclusive.",
				Validators: []validator.Int64{
					int64validator.Between(1, 10),
				},
			},
			"otel_verify_ssl": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Verify SSL certificates (PVE default: `true`). Only applies to `type` = `opentelemetry`.",
			},
		},
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveMetricsServerResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = haConfigureResource(req, resp)
}

// ValidateConfig implements resource.ResourceWithValidateConfig: it rejects
// type-conditional attributes set for a plugin type they do not belong to,
// naming the attribute and the configured type.
func (r *pveMetricsServerResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config pveMetricsServerResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if config.Type.IsNull() || config.Type.IsUnknown() {
		return
	}
	typeName := config.Type.ValueString()
	for _, f := range metricsServerFieldOwnership {
		value, ok := metricsServerAttrValue(&config, f.attr)
		if !ok || value.IsNull() || value.IsUnknown() {
			continue
		}
		if slices.Contains(f.types, typeName) {
			continue
		}
		resp.Diagnostics.AddAttributeError(
			path.Root(f.attr),
			fmt.Sprintf("Invalid attribute for metrics server type %q", typeName),
			fmt.Sprintf("Attribute %q only applies to metrics server type(s) %s; remove the attribute or change the type.", f.attr, strings.Join(f.types, ", ")),
		)
	}
}

// Create implements resource.Resource.
func (r *pveMetricsServerResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveMetricsServerResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error creating pve_metrics_server", "provider client is not configured")
		return
	}
	if err := r.client.CreateMetricsServer(ctx, metricsServerFromModel(plan)); err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_metrics_server",
			fmt.Sprintf("creating metric server %s: %s", plan.ID.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_metrics_server after create",
			fmt.Sprintf("reading metric server %s: %s", plan.ID.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	tflog.Debug(ctx, "created PVE metric server", map[string]any{"id": plan.ID.ValueString(), "type": plan.Type.ValueString()})
}

// Read implements resource.Resource.
func (r *pveMetricsServerResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveMetricsServerResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.readInto(ctx, &state); err != nil {
		if isPVEClientNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error reading pve_metrics_server",
			fmt.Sprintf("reading metric server %s: %s", state.ID.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource. Set fields travel in the PUT body;
// attributes cleared in the plan travel in the `delete` query parameter.
func (r *pveMetricsServerResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveMetricsServerResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state pveMetricsServerResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	deleteFields := metricsServerDeleteFields(plan, state)
	if err := r.client.UpdateMetricsServer(ctx, plan.ID.ValueString(), metricsServerFromModel(plan), deleteFields); err != nil {
		resp.Diagnostics.AddError(
			"Error updating pve_metrics_server",
			fmt.Sprintf("updating metric server %s: %s", plan.ID.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_metrics_server after update",
			fmt.Sprintf("reading metric server %s: %s", plan.ID.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	tflog.Debug(ctx, "updated PVE metric server", map[string]any{"id": plan.ID.ValueString()})
}

// Delete implements resource.Resource.
func (r *pveMetricsServerResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveMetricsServerResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	tflog.Debug(ctx, "deleting PVE metric server", map[string]any{"id": state.ID.ValueString()})
	if err := r.client.DeleteMetricsServer(ctx, state.ID.ValueString()); err != nil {
		if isPVEClientNotFound(err) {
			// Already absent counts as deleted.
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting pve_metrics_server",
			fmt.Sprintf("deleting metric server %s: %s", state.ID.ValueString(), err),
		)
	}
}

// ImportState parses an import ID of the form `<id>`.
func (r *pveMetricsServerResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		resp.Diagnostics.AddError("Invalid pve_metrics_server import ID", "import ID must be the metric server identifier, e.g. `influx1`")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

// readInto refreshes the model from PVE.
func (r *pveMetricsServerResource) readInto(ctx context.Context, m *pveMetricsServerResourceModel) error {
	srv, err := r.client.GetMetricsServer(ctx, m.ID.ValueString())
	if err != nil {
		return err
	}
	metricsServerApply(srv, m)
	return nil
}

// metricsServerAttrValue returns the model field for an attribute name from
// metricsServerFieldOwnership; ok is false for names the model does not
// carry.
func metricsServerAttrValue(m *pveMetricsServerResourceModel, attrName string) (attr.Value, bool) {
	switch attrName {
	case "mtu":
		return m.MTU, true
	case "proto":
		return m.Proto, true
	case "path":
		return m.Path, true
	case "timeout":
		return m.Timeout, true
	case "influxdb_proto":
		return m.InfluxDBProto, true
	case "organization":
		return m.Organization, true
	case "bucket":
		return m.Bucket, true
	case "token":
		return m.Token, true
	case "api_path_prefix":
		return m.APIPathPrefix, true
	case "max_body_size":
		return m.MaxBodySize, true
	case "verify_certificate":
		return m.VerifyCertificate, true
	case "otel_compression":
		return m.OTelCompression, true
	case "otel_headers":
		return m.OTelHeaders, true
	case "otel_max_body_size":
		return m.OTelMaxBodySize, true
	case "otel_path":
		return m.OTelPath, true
	case "otel_protocol":
		return m.OTelProtocol, true
	case "otel_resource_attributes":
		return m.OTelResourceAttributes, true
	case "otel_timeout":
		return m.OTelTimeout, true
	case "otel_verify_ssl":
		return m.OTelVerifySSL, true
	default:
		return nil, false
	}
}

// metricsServerDeleteFields returns the PVE wire field names to clear on
// update: type-conditional attributes present in state but null in plan,
// plus the shared disable flag.
func metricsServerDeleteFields(plan, state pveMetricsServerResourceModel) []string {
	var out []string
	if plan.Disable.IsNull() && !state.Disable.IsNull() {
		out = append(out, "disable")
	}
	for _, f := range metricsServerFieldOwnership {
		planValue, ok := metricsServerAttrValue(&plan, f.attr)
		if !ok {
			continue
		}
		stateValue, ok := metricsServerAttrValue(&state, f.attr)
		if !ok {
			continue
		}
		if planValue.IsNull() && !stateValue.IsNull() {
			out = append(out, f.wire)
		}
	}
	return out
}

// metricsServerFromModel projects the Terraform model into the wire body;
// null values become nil pointers or empty strings and are omitted from the
// request.
func metricsServerFromModel(m pveMetricsServerResourceModel) pveclient.MetricsServer {
	body := pveclient.MetricsServer{
		ID:      m.ID.ValueString(),
		Type:    m.Type.ValueString(),
		Server:  m.Server.ValueString(),
		Port:    metricsServerInt64Ptr(m.Port),
		Disable: metricsServerBoolPtr(m.Disable),
		MTU:     metricsServerInt64Ptr(m.MTU),

		Proto:   m.Proto.ValueString(),
		Path:    m.Path.ValueString(),
		Timeout: metricsServerInt64Ptr(m.Timeout),

		InfluxDBProto:     m.InfluxDBProto.ValueString(),
		Organization:      m.Organization.ValueString(),
		Bucket:            m.Bucket.ValueString(),
		Token:             m.Token.ValueString(),
		APIPathPrefix:     m.APIPathPrefix.ValueString(),
		MaxBodySize:       metricsServerInt64Ptr(m.MaxBodySize),
		VerifyCertificate: metricsServerBoolPtr(m.VerifyCertificate),

		OTelCompression:        m.OTelCompression.ValueString(),
		OTelHeaders:            m.OTelHeaders.ValueString(),
		OTelMaxBodySize:        metricsServerInt64Ptr(m.OTelMaxBodySize),
		OTelPath:               m.OTelPath.ValueString(),
		OTelProtocol:           m.OTelProtocol.ValueString(),
		OTelResourceAttributes: m.OTelResourceAttributes.ValueString(),
		OTelTimeout:            metricsServerInt64Ptr(m.OTelTimeout),
		OTelVerifySSL:          metricsServerBoolPtr(m.OTelVerifySSL),
	}
	return body
}

// metricsServerApply writes a fetched configuration into the model; absent
// settings become null.
func metricsServerApply(s *pveclient.MetricsServer, m *pveMetricsServerResourceModel) {
	m.Type = nodeNetworkStringToTF(s.Type)
	m.Server = nodeNetworkStringToTF(s.Server)
	m.Port = metricsServerInt64Value(s.Port)
	m.Disable = nodeNetworkBoolPtrToTF(s.Disable)
	m.MTU = metricsServerInt64Value(s.MTU)

	m.Proto = nodeNetworkStringToTF(s.Proto)
	m.Path = nodeNetworkStringToTF(s.Path)
	m.Timeout = metricsServerInt64Value(s.Timeout)

	m.InfluxDBProto = nodeNetworkStringToTF(s.InfluxDBProto)
	m.Organization = nodeNetworkStringToTF(s.Organization)
	m.Bucket = nodeNetworkStringToTF(s.Bucket)
	m.Token = nodeNetworkStringToTF(s.Token)
	m.APIPathPrefix = nodeNetworkStringToTF(s.APIPathPrefix)
	m.MaxBodySize = metricsServerInt64Value(s.MaxBodySize)
	m.VerifyCertificate = nodeNetworkBoolPtrToTF(s.VerifyCertificate)

	m.OTelCompression = nodeNetworkStringToTF(s.OTelCompression)
	m.OTelHeaders = nodeNetworkStringToTF(s.OTelHeaders)
	m.OTelMaxBodySize = metricsServerInt64Value(s.OTelMaxBodySize)
	m.OTelTimeout = metricsServerInt64Value(s.OTelTimeout)
	m.OTelVerifySSL = nodeNetworkBoolPtrToTF(s.OTelVerifySSL)
}

// metricsServerBoolPtr converts a Terraform bool into a wire pointer, nil
// for null or unknown values.
func metricsServerBoolPtr(v types.Bool) *bool {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	b := v.ValueBool()
	return &b
}

// metricsServerInt64Ptr converts a Terraform int64 into a wire pointer, nil
// for null or unknown values.
func metricsServerInt64Ptr(v types.Int64) *int64 {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	i := v.ValueInt64()
	return &i
}

// metricsServerInt64Value writes a wire int into the model, null when
// absent.
func metricsServerInt64Value(v *int64) types.Int64 {
	if v == nil {
		return types.Int64Null()
	}
	return types.Int64Value(*v)
}
