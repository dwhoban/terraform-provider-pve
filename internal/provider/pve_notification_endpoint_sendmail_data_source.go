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
	_ datasource.DataSource              = &pveNotificationEndpointSendmailDataSource{}
	_ datasource.DataSourceWithConfigure = &pveNotificationEndpointSendmailDataSource{}
)

// NewPveNotificationEndpointSendmailDataSource returns the data source implementation.
func NewPveNotificationEndpointSendmailDataSource() datasource.DataSource {
	return &pveNotificationEndpointSendmailDataSource{}
}

// pveNotificationEndpointSendmailDataSource reads a single sendmail
// notification endpoint (GET /cluster/notifications/endpoints/sendmail/{name}).
type pveNotificationEndpointSendmailDataSource struct {
	client *pveclient.Client
}

// pveNotificationEndpointSendmailDataSourceModel is the Terraform-facing shape.
type pveNotificationEndpointSendmailDataSourceModel struct {
	Name        types.String `tfsdk:"name"`
	MailTo      types.List   `tfsdk:"mailto"`
	MailToUser  types.List   `tfsdk:"mailto_user"`
	FromAddress types.String `tfsdk:"from_address"`
	Author      types.String `tfsdk:"author"`
	Comment     types.String `tfsdk:"comment"`
	Disable     types.Bool   `tfsdk:"disable"`
}

// Metadata implements datasource.DataSource.
func (d *pveNotificationEndpointSendmailDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveNotificationEndpointSendmail
}

// Schema implements datasource.DataSource.
func (d *pveNotificationEndpointSendmailDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads a single sendmail notification endpoint from `GET /cluster/notifications/endpoints/sendmail/{name}`.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The name of the endpoint to look up.",
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
			"from_address": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "`From` address for the mail.",
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
func (d *pveNotificationEndpointSendmailDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = notificationEndpointConfigureDataSource(req, resp)
}

// Read implements datasource.DataSource.
func (d *pveNotificationEndpointSendmailDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveNotificationEndpointSendmailDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Error reading pve_notification_endpoint_sendmail", "provider client is not configured")
		return
	}
	ep, err := d.client.GetNotificationEndpoint(ctx, pveclient.NotificationEndpointTypeSendmail, data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_notification_endpoint_sendmail",
			fmt.Sprintf("reading sendmail endpoint %s: %s", data.Name.ValueString(), err),
		)
		return
	}
	data.MailTo = notificationEndpointListToTF(ep.MailTo)
	data.MailToUser = notificationEndpointListToTF(ep.MailToUser)
	data.FromAddress = types.StringValue(ep.FromAddress)
	data.Author = nodeNetworkStringToTF(ep.Author)
	data.Comment = nodeNetworkStringToTF(ep.Comment)
	data.Disable = nodeNetworkBoolPtrToTF(ep.Disable)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
