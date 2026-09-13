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
	_ datasource.DataSource              = &pveNotificationTargetsDataSource{}
	_ datasource.DataSourceWithConfigure = &pveNotificationTargetsDataSource{}
)

// NewPveNotificationTargetsDataSource returns the data source implementation.
func NewPveNotificationTargetsDataSource() datasource.DataSource {
	return &pveNotificationTargetsDataSource{}
}

// pveNotificationTargetsDataSource lists every entity that can receive
// notifications (GET /cluster/notifications/targets) — user-created
// endpoints and built-in targets such as mail-to-root alike.
type pveNotificationTargetsDataSource struct {
	client *pveclient.Client
}

// pveNotificationTargetsDataSourceModel is the Terraform-facing shape.
type pveNotificationTargetsDataSourceModel struct {
	ID      types.String                     `tfsdk:"id"`
	Targets []pveNotificationTargetsRowModel `tfsdk:"targets"`
}

// pveNotificationTargetsRowModel mirrors the per-row schema of the targets
// listing. Field names line up with the pveclient struct.
type pveNotificationTargetsRowModel struct {
	Name    types.String `tfsdk:"name"`
	Type    types.String `tfsdk:"type"`
	Comment types.String `tfsdk:"comment"`
	Disable types.Bool   `tfsdk:"disable"`
	Origin  types.String `tfsdk:"origin"`
}

// Metadata implements datasource.DataSource.
func (d *pveNotificationTargetsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveNotificationTargets
}

// Schema implements datasource.DataSource.
func (d *pveNotificationTargetsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists every entity that can be used as a notification target, as reported by `GET /cluster/notifications/targets`. Includes the built-in targets (e.g. `mail-to-root`) alongside user-created endpoints managed by the `pve_notification_endpoint_*` resources.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Static identifier for the targets listing.",
			},
			"targets": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Notification targets configured in the cluster.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Name of the target.",
						},
						"type": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Type of the target. Must be one of: `sendmail`, `gotify`, `smtp`, `webhook`.",
						},
						"comment": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Comment on the target; null when unset.",
						},
						"disable": schema.BoolAttribute{
							Computed:            true,
							MarkdownDescription: "Whether the target is disabled.",
						},
						"origin": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Whether the entry is `user-created`, `builtin`, or a `modified-builtin`.",
						},
					},
				},
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveNotificationTargetsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
func (d *pveNotificationTargetsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config pveNotificationTargetsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Error reading pve_notification_targets", "provider client is not configured")
		return
	}
	targets, err := d.client.ListNotificationTargets(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading pve_notification_targets", fmt.Sprintf("listing notification targets: %s", err))
		return
	}
	rows := make([]pveNotificationTargetsRowModel, 0, len(targets))
	for _, target := range targets {
		rows = append(rows, pveNotificationTargetsRowModel{
			Name:    types.StringValue(target.Name),
			Type:    types.StringValue(target.Type),
			Comment: nodeNetworkStringToTF(target.Comment),
			Disable: nodeNetworkBoolPtrToTF(target.Disable),
			Origin:  nodeNetworkStringToTF(target.Origin),
		})
	}
	config.ID = types.StringValue("pve_notification_targets")
	config.Targets = rows
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
