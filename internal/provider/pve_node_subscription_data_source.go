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
	_ datasource.DataSource              = &pveNodeSubscriptionDataSource{}
	_ datasource.DataSourceWithConfigure = &pveNodeSubscriptionDataSource{}
)

// NewPveNodeSubscriptionDataSource returns the data source implementation.
func NewPveNodeSubscriptionDataSource() datasource.DataSource {
	return &pveNodeSubscriptionDataSource{}
}

// pveNodeSubscriptionDataSource reads the subscription info of a node
// (GET /nodes/{node}/subscription).
type pveNodeSubscriptionDataSource struct {
	client *pveclient.Client
}

// pveNodeSubscriptionDataSourceModel is the Terraform-facing shape.
type pveNodeSubscriptionDataSourceModel struct {
	ID          types.String `tfsdk:"id"`
	Node        types.String `tfsdk:"node"`
	Status      types.String `tfsdk:"status"`
	Key         types.String `tfsdk:"key"`
	Level       types.String `tfsdk:"level"`
	Message     types.String `tfsdk:"message"`
	NextDueDate types.String `tfsdk:"next_due_date"`
	ProductName types.String `tfsdk:"product_name"`
	RegDate     types.String `tfsdk:"reg_date"`
	ServerID    types.String `tfsdk:"server_id"`
	Signature   types.String `tfsdk:"signature"`
	Sockets     types.Int64  `tfsdk:"sockets"`
	CheckTime   types.Int64  `tfsdk:"check_time"`
	URL         types.String `tfsdk:"url"`
}

// Metadata implements datasource.DataSource.
func (d *pveNodeSubscriptionDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveNodeSubscription
}

// Schema implements datasource.DataSource.
func (d *pveNodeSubscriptionDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads the subscription info of a node as reported by `GET /nodes/{node}/subscription`. " +
			"Fields the API omits (e.g. on an unregistered node) are null.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Identifier of this data source (the node name).",
			},
			"node": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The cluster node whose subscription info is read.",
			},
			"status": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The current subscription status. Must be one of: `new`, `notfound`, `active`, `invalid`, `expired`, `suspended`.",
			},
			"key": schema.StringAttribute{
				Computed:            true,
				Sensitive:           true,
				MarkdownDescription: "The subscription key, if set and permitted to access.",
			},
			"level": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "A short code for the subscription level, e.g. `c` for community.",
			},
			"message": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "A more human-readable status message.",
			},
			"next_due_date": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Next due date of the set subscription, e.g. `2027-01-01`.",
			},
			"product_name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Human-readable product name of the set subscription.",
			},
			"reg_date": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Registration date of the set subscription.",
			},
			"server_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The server ID, if permitted to access.",
			},
			"signature": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Signature for offline keys.",
			},
			"sockets": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "The number of CPU sockets covered on this host.",
			},
			"check_time": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Unix timestamp of the last subscription check.",
			},
			"url": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "URL to the web shop.",
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveNodeSubscriptionDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*pveclient.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *pveclient.Client, got: %T.", req.ProviderData),
		)
		return
	}
	d.client = client
}

// Read implements datasource.DataSource.
func (d *pveNodeSubscriptionDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveNodeSubscriptionDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Unconfigured pve_node_subscription",
			fmt.Sprintf("The provider client was not configured; cannot read subscription of node %s.", data.Node.ValueString()),
		)
		return
	}
	node := data.Node.ValueString()
	sub, err := d.client.GetNodeSubscription(ctx, node)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_node_subscription",
			fmt.Sprintf("reading subscription of node %s: %s", node, err),
		)
		return
	}
	data.ID = types.StringValue(node)
	data.Status = types.StringValue(sub.Status)
	data.Key = pveNodeSubscriptionStringPtrToTF(sub.Key)
	data.Level = pveNodeSubscriptionStringPtrToTF(sub.Level)
	data.Message = pveNodeSubscriptionStringPtrToTF(sub.Message)
	data.NextDueDate = pveNodeSubscriptionStringPtrToTF(sub.NextDueDate)
	data.ProductName = pveNodeSubscriptionStringPtrToTF(sub.ProductName)
	data.RegDate = pveNodeSubscriptionStringPtrToTF(sub.RegDate)
	data.ServerID = pveNodeSubscriptionStringPtrToTF(sub.ServerID)
	data.Signature = pveNodeSubscriptionStringPtrToTF(sub.Signature)
	data.Sockets = pveNodeSubscriptionInt64PtrToTF(sub.Sockets)
	data.CheckTime = pveNodeSubscriptionInt64PtrToTF(sub.CheckTime)
	data.URL = pveNodeSubscriptionStringPtrToTF(sub.URL)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// pveNodeSubscriptionStringPtrToTF maps an optional API string to a
// Terraform string (null when absent).
func pveNodeSubscriptionStringPtrToTF(v *string) types.String {
	if v == nil {
		return types.StringNull()
	}
	return types.StringValue(*v)
}

// pveNodeSubscriptionInt64PtrToTF maps an optional API integer to a
// Terraform integer (null when absent).
func pveNodeSubscriptionInt64PtrToTF(v *int64) types.Int64 {
	if v == nil {
		return types.Int64Null()
	}
	return types.Int64Value(*v)
}
