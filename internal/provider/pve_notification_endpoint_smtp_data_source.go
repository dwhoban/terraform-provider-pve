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
	_ datasource.DataSource              = &pveNotificationEndpointSMTPDataSource{}
	_ datasource.DataSourceWithConfigure = &pveNotificationEndpointSMTPDataSource{}
)

// NewPveNotificationEndpointSMTPDataSource returns the data source implementation.
func NewPveNotificationEndpointSMTPDataSource() datasource.DataSource {
	return &pveNotificationEndpointSMTPDataSource{}
}

// pveNotificationEndpointSMTPDataSource reads a single SMTP
// notification endpoint (GET /cluster/notifications/endpoints/smtp/{name}).
type pveNotificationEndpointSMTPDataSource struct {
	client *pveclient.Client
}

// pveNotificationEndpointSMTPDataSourceModel is the Terraform-facing shape.
type pveNotificationEndpointSMTPDataSourceModel struct {
	Name        types.String `tfsdk:"name"`
	Server      types.String `tfsdk:"server"`
	FromAddress types.String `tfsdk:"from_address"`
	Username    types.String `tfsdk:"username"`
	Port        types.Int64  `tfsdk:"port"`
	Mode        types.String `tfsdk:"mode"`
	MailTo      types.List   `tfsdk:"mailto"`
	MailToUser  types.List   `tfsdk:"mailto_user"`
	Author      types.String `tfsdk:"author"`
	Comment     types.String `tfsdk:"comment"`
	Disable     types.Bool   `tfsdk:"disable"`
}

// Metadata implements datasource.DataSource.
func (d *pveNotificationEndpointSMTPDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveNotificationEndpointSmtp
}

// Schema implements datasource.DataSource.
func (d *pveNotificationEndpointSMTPDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads a single SMTP notification endpoint from `GET /cluster/notifications/endpoints/smtp/{name}`.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The name of the endpoint to look up.",
			},
			"server": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The address of the SMTP server.",
			},
			"from_address": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "`From` address for the mail.",
			},
			"username": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Username for SMTP authentication.",
			},
			"port": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "The port to be used.",
			},
			"mode": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Determine which encryption method shall be used for the connection. Must be one of: `insecure`, `starttls`, `tls`.",
			},
			"mailto": schema.ListAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "List of email recipients.",
			},
			"mailto_user": schema.ListAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "List of PVE users receiving the notification.",
			},
			"author": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Author of the mail.",
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
func (d *pveNotificationEndpointSMTPDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationEndpointConfigureDataSource(req, resp)
}

// Read implements datasource.DataSource.
func (d *pveNotificationEndpointSMTPDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveNotificationEndpointSMTPDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Error reading pve_notification_endpoint_smtp", "provider client is not configured")
		return
	}
	ep, err := d.client.GetNotificationEndpoint(ctx, pveclient.NotificationEndpointTypeSMTP, data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_notification_endpoint_smtp",
			fmt.Sprintf("reading SMTP endpoint %s: %s", data.Name.ValueString(), err),
		)
		return
	}
	data.Server = types.StringValue(ep.Server)
	data.FromAddress = types.StringValue(ep.FromAddress)
	data.Username = nodeNetworkStringToTF(ep.Username)
	data.Mode = nodeNetworkStringToTF(ep.Mode)
	if ep.Port != nil {
		data.Port = types.Int64Value(*ep.Port)
	} else {
		data.Port = types.Int64Null()
	}
	data.MailTo = notificationEndpointListToTF(ep.MailTo)
	data.MailToUser = notificationEndpointListToTF(ep.MailToUser)
	data.Author = nodeNetworkStringToTF(ep.Author)
	data.Comment = nodeNetworkStringToTF(ep.Comment)
	data.Disable = nodeNetworkBoolPtrToTF(ep.Disable)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
