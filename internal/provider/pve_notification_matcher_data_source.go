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
	_ datasource.DataSource              = &pveNotificationMatcherDataSource{}
	_ datasource.DataSourceWithConfigure = &pveNotificationMatcherDataSource{}
)

// NewPveNotificationMatcherDataSource returns the data source implementation.
func NewPveNotificationMatcherDataSource() datasource.DataSource {
	return &pveNotificationMatcherDataSource{}
}

// pveNotificationMatcherDataSource reads a single notification matcher
// (GET /cluster/notifications/matchers/{name}).
type pveNotificationMatcherDataSource struct {
	client *pveclient.Client
}

// pveNotificationMatcherDataSourceModel is the Terraform-facing shape.
type pveNotificationMatcherDataSourceModel struct {
	Name          types.String `tfsdk:"name"`
	Target        types.List   `tfsdk:"target"`
	MatchField    types.List   `tfsdk:"match_field"`
	MatchSeverity types.List   `tfsdk:"match_severity"`
	MatchCalendar types.List   `tfsdk:"match_calendar"`
	Mode          types.String `tfsdk:"mode"`
	InvertMatch   types.Bool   `tfsdk:"invert_match"`
	Disable       types.Bool   `tfsdk:"disable"`
	Comment       types.String `tfsdk:"comment"`
	Origin        types.String `tfsdk:"origin"`
	Digest        types.String `tfsdk:"digest"`
}

// Metadata implements datasource.DataSource.
func (d *pveNotificationMatcherDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveNotificationMatcher
}

// Schema implements datasource.DataSource.
func (d *pveNotificationMatcherDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads a single notification matcher from `GET /cluster/notifications/matchers/{name}`.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The matcher name to look up.",
			},
			"target": schema.ListAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Names of the notification targets the matcher routes to.",
			},
			"match_field": schema.ListAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Metadata match conditions, each in the form `(regex|exact):<field>=<value>`.",
			},
			"match_severity": schema.ListAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Notification severities to match (e.g. `info`, `notice`, `warning`, `error`, `unknown`).",
			},
			"match_calendar": schema.ListAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Calendar-event windows the notification timestamp must fall in.",
			},
			"mode": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "How multiple conditions combine: `all` or `any`.",
			},
			"invert_match": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether the match result of the whole matcher is inverted.",
			},
			"disable": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether this matcher is disabled.",
			},
			"comment": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Description of the matcher; null when unset.",
			},
			"origin": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Whether the matcher is `user-created`, `builtin`, or a `modified-builtin`.",
			},
			"digest": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Upstream configuration digest; changes whenever the matcher is edited out of band.",
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveNotificationMatcherDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
func (d *pveNotificationMatcherDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveNotificationMatcherDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Error reading pve_notification_matcher", "provider client is not configured")
		return
	}

	matcher, err := d.client.GetNotificationMatcher(ctx, data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_notification_matcher",
			fmt.Sprintf("reading notification matcher %s: %s", data.Name.ValueString(), err),
		)
		return
	}
	data.Target = listStringToTF(matcher.Target)
	data.MatchField = listStringToTF(matcher.MatchField)
	data.MatchSeverity = listStringToTF(matcher.MatchSeverity)
	data.MatchCalendar = listStringToTF(matcher.MatchCalendar)
	data.Mode = nodeNetworkStringToTF(matcher.Mode)
	data.InvertMatch = nodeNetworkBoolPtrToTF(matcher.InvertMatch)
	data.Disable = nodeNetworkBoolPtrToTF(matcher.Disable)
	data.Comment = nodeNetworkStringToTF(matcher.Comment)
	data.Origin = nodeNetworkStringToTF(matcher.Origin)
	data.Digest = nodeNetworkStringToTF(matcher.Digest)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
