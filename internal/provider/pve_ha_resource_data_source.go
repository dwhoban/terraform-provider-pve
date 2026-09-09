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
	_ datasource.DataSource              = &pveHaResourceDataSource{}
	_ datasource.DataSourceWithConfigure = &pveHaResourceDataSource{}
)

// NewPveHaResourceDataSource returns the data source implementation.
func NewPveHaResourceDataSource() datasource.DataSource {
	return &pveHaResourceDataSource{}
}

// pveHaResourceDataSource reads a single HA resource
// (GET /cluster/ha/resources/{sid}).
type pveHaResourceDataSource struct {
	client *pveclient.Client
}

// pveHaResourceDataSourceModel is the Terraform-facing shape.
type pveHaResourceDataSourceModel struct {
	SID           types.String `tfsdk:"sid"`
	Type          types.String `tfsdk:"type"`
	State         types.String `tfsdk:"state"`
	Group         types.String `tfsdk:"group"`
	MaxRestart    types.Int64  `tfsdk:"max_restart"`
	MaxRelocate   types.Int64  `tfsdk:"max_relocate"`
	Failback      types.Bool   `tfsdk:"failback"`
	AutoRebalance types.Bool   `tfsdk:"auto_rebalance"`
	Comment       types.String `tfsdk:"comment"`
}

// Metadata implements datasource.DataSource.
func (d *pveHaResourceDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveHaResource
}

// Schema implements datasource.DataSource.
func (d *pveHaResourceDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads a single HA resource from `GET /cluster/ha/resources/{sid}`.",
		Attributes: map[string]schema.Attribute{
			"sid": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "HA resource ID to look up, e.g. `vm:100` or `ct:101`.",
			},
			"type": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Resource type: `vm` or `ct`.",
			},
			"state": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Requested resource state: `started`, `stopped`, `enabled`, `disabled`, or `ignored`.",
			},
			"group": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "HA group identifier constraining placement, when set.",
			},
			"max_restart": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Maximal number of restart tries on a node after a failed start.",
			},
			"max_relocate": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Maximal number of relocate tries when a resource fails to start.",
			},
			"failback": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether the resource migrates back to a higher-priority node when it comes online.",
			},
			"auto_rebalance": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether the resource may be migrated during automatic rebalancing.",
			},
			"comment": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Description of the HA resource.",
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveHaResourceDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = haConfigureDataSource(req, resp)
}

// Read implements datasource.DataSource.
func (d *pveHaResourceDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveHaResourceDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Error reading pve_ha_resource", "provider client is not configured")
		return
	}

	res, err := d.client.GetHAResource(ctx, data.SID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_ha_resource",
			fmt.Sprintf("reading HA resource %s: %s", data.SID.ValueString(), err),
		)
		return
	}
	data.Type = nodeNetworkStringToTF(res.Type)
	data.State = nodeNetworkStringToTF(res.State)
	data.Group = nodeNetworkStringToTF(res.Group)
	data.MaxRestart = haInt64PtrToTF(res.MaxRestart)
	data.MaxRelocate = haInt64PtrToTF(res.MaxRelocate)
	data.Failback = nodeNetworkBoolPtrToTF(res.Failback)
	data.AutoRebalance = nodeNetworkBoolPtrToTF(res.AutoRebalance)
	data.Comment = nodeNetworkStringToTF(res.Comment)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
