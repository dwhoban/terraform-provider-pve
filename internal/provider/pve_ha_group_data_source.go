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
	_ datasource.DataSource              = &pveHaGroupDataSource{}
	_ datasource.DataSourceWithConfigure = &pveHaGroupDataSource{}
)

// NewPveHaGroupDataSource returns the data source implementation.
func NewPveHaGroupDataSource() datasource.DataSource {
	return &pveHaGroupDataSource{}
}

// pveHaGroupDataSource reads a single HA group
// (GET /cluster/ha/groups/{group}).
type pveHaGroupDataSource struct {
	client *pveclient.Client
}

// pveHaGroupDataSourceModel is the Terraform-facing shape.
type pveHaGroupDataSourceModel struct {
	Group      types.String `tfsdk:"group"`
	Nodes      types.List   `tfsdk:"nodes"`
	Restricted types.Bool   `tfsdk:"restricted"`
	NoFailback types.Bool   `tfsdk:"nofailback"`
	Comment    types.String `tfsdk:"comment"`
}

// Metadata implements datasource.DataSource.
func (d *pveHaGroupDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveHaGroup
}

// Schema implements datasource.DataSource.
func (d *pveHaGroupDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads a single HA group from `GET /cluster/ha/groups/{group}`. PVE marks HA groups as deprecated in favor of HA rules (`pve_ha_rule`).",
		Attributes: map[string]schema.Attribute{
			"group": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The HA group identifier to look up.",
			},
			"nodes": schema.ListAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Cluster node members as `<node>` or `<node>:<priority>` entries.",
			},
			"restricted": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether resources bound to the group may only run on the group's nodes.",
			},
			"nofailback": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether automatic failback to the highest-priority node is disabled.",
			},
			"comment": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Description of the HA group.",
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveHaGroupDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = haConfigureDataSource(req, resp)
}

// Read implements datasource.DataSource.
func (d *pveHaGroupDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveHaGroupDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Error reading pve_ha_group", "provider client is not configured")
		return
	}

	group, err := d.client.GetHAGroup(ctx, data.Group.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_ha_group",
			fmt.Sprintf("reading HA group %s: %s", data.Group.ValueString(), err),
		)
		return
	}
	data.Nodes = listStringToTF(group.Nodes)
	data.Restricted = nodeNetworkBoolPtrToTF(group.Restricted)
	data.NoFailback = nodeNetworkBoolPtrToTF(group.NoFailback)
	data.Comment = nodeNetworkStringToTF(group.Comment)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
