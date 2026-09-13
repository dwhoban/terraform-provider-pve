// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ datasource.DataSource              = &pveNotificationEndpointGotyDataSource{}
	_ datasource.DataSourceWithConfigure = &pveNotificationEndpointGotyDataSource{}
)

// NewPveNotificationEndpointGotyDataSource returns the data source implementation.
func NewPveNotificationEndpointGotyDataSource() datasource.DataSource {
	return &pveNotificationEndpointGotyDataSource{}
}

// pveNotificationEndpointGotyDataSource reads a single Gotify
// notification endpoint (GET /cluster/notifications/endpoints/gotify/{name}).
type pveNotificationEndpointGotyDataSource struct {
	client *pveclient.Client
}

// pveNotificationEndpointGotyDataSourceModel is the Terraform-facing shape.
type pveNotificationEndpointGotyDataSourceModel struct {
	Name    types.String `tfsdk:"name"`
	Server  types.String `tfsdk:"server"`
	Comment types.String `tfsdk:"comment"`
	Disable types.Bool   `tfsdk:"disable"`
}

// Metadata implements datasource.DataSource.
func (d *pveNotificationEndpointGotyDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveNotificationEndpointGoty
}

// Schema implements datasource.DataSource.
func (d *pveNotificationEndpointGotyDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads a single Gotify notification endpoint from `GET /cluster/notifications/endpoints/gotify/{name}`.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The name of the endpoint to look up.",
			},
			"server": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Server URL of the Gotify instance.",
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
func (d *pveNotificationEndpointGotyDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationEndpointConfigureDataSource(req, resp)
}

// Read implements datasource.DataSource.
func (d *pveNotificationEndpointGotyDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveNotificationEndpointGotyDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Error reading pve_notification_endpoint_goty", "provider client is not configured")
		return
	}
	ep, err := d.client.GetNotificationEndpoint(ctx, pveclient.NotificationEndpointTypeGotify, data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_notification_endpoint_goty",
			fmt.Sprintf("reading Gotify endpoint %s: %s", data.Name.ValueString(), err),
		)
		return
	}
	data.Server = types.StringValue(ep.Server)
	data.Comment = nodeNetworkStringToTF(ep.Comment)
	data.Disable = nodeNetworkBoolPtrToTF(ep.Disable)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
