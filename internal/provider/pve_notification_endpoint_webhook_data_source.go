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
	_ datasource.DataSource              = &pveNotificationEndpointWebhookDataSource{}
	_ datasource.DataSourceWithConfigure = &pveNotificationEndpointWebhookDataSource{}
)

// NewPveNotificationEndpointWebhookDataSource returns the data source implementation.
func NewPveNotificationEndpointWebhookDataSource() datasource.DataSource {
	return &pveNotificationEndpointWebhookDataSource{}
}

// pveNotificationEndpointWebhookDataSource reads a single webhook
// notification endpoint (GET /cluster/notifications/endpoints/webhook/{name}).
type pveNotificationEndpointWebhookDataSource struct {
	client *pveclient.Client
}

// pveNotificationEndpointWebhookDataSourceModel is the Terraform-facing shape.
type pveNotificationEndpointWebhookDataSourceModel struct {
	Name    types.String `tfsdk:"name"`
	URL     types.String `tfsdk:"url"`
	Method  types.String `tfsdk:"method"`
	Body    types.String `tfsdk:"body"`
	Headers types.Map    `tfsdk:"headers"`
	Secrets types.Map    `tfsdk:"secrets"`
	Comment types.String `tfsdk:"comment"`
	Disable types.Bool   `tfsdk:"disable"`
}

// Metadata implements datasource.DataSource.
func (d *pveNotificationEndpointWebhookDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveNotificationEndpointWebhook
}

// Schema implements datasource.DataSource.
func (d *pveNotificationEndpointWebhookDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads a single webhook notification endpoint from `GET /cluster/notifications/endpoints/webhook/{name}`.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The name of the endpoint to look up.",
			},
			"url": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Server URL to send the request to.",
			},
			"method": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "HTTP method used for the request. Must be one of: `post`, `put`, `get`.",
			},
			"body": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "HTTP body template (decoded from PVE's base64 storage).",
			},
			"headers": schema.MapAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "HTTP headers to set, as a `name` to `value` map (decoded from PVE's base64 property strings).",
			},
			"secrets": schema.MapAttribute{
				Computed:            true,
				Sensitive:           true,
				ElementType:         types.StringType,
				MarkdownDescription: "Secrets, as a `name` to `value` map (decoded from PVE's base64 property strings).",
			},
			"comment": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Comment.",
			},
			"disable": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether this target is disabled.",
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveNotificationEndpointWebhookDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationEndpointConfigureDataSource(req, resp)
}

// Read implements datasource.DataSource.
func (d *pveNotificationEndpointWebhookDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveNotificationEndpointWebhookDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Error reading pve_notification_endpoint_webhook", "provider client is not configured")
		return
	}
	ep, err := d.client.GetNotificationEndpoint(ctx, pveclient.NotificationEndpointTypeWebhook, data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_notification_endpoint_webhook",
			fmt.Sprintf("reading webhook endpoint %s: %s", data.Name.ValueString(), err),
		)
		return
	}
	data.URL = types.StringValue(ep.URL)
	data.Method = types.StringValue(ep.Method)
	data.Body = nodeNetworkStringToTF(ep.Body)
	data.Headers = notificationEndpointMapToTF(ep.Headers)
	data.Secrets = notificationEndpointMapToTF(ep.Secrets)
	data.Comment = nodeNetworkStringToTF(ep.Comment)
	data.Disable = nodeNetworkBoolPtrToTF(ep.Disable)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
