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
	_ datasource.DataSource              = &pveSdnZoneQinqDataSource{}
	_ datasource.DataSourceWithConfigure = &pveSdnZoneQinqDataSource{}
)

// NewPveSdnZoneQinqDataSource returns the data source implementation.
func NewPveSdnZoneQinqDataSource() datasource.DataSource {
	return &pveSdnZoneQinqDataSource{}
}

// pveSdnZoneQinqDataSource reads a single QinQ SDN zone (GET
// /cluster/sdn/zones/{zone}), erroring when the upstream type is not qinq.
type pveSdnZoneQinqDataSource struct {
	client *pveclient.Client
}

// pveSdnZoneQinqDataSourceModel is the Terraform-facing shape.
type pveSdnZoneQinqDataSourceModel struct {
	sdnZoneCommonModel
	Bridge       types.String `tfsdk:"bridge"`
	Tag          types.Int64  `tfsdk:"tag"`
	VlanProtocol types.String `tfsdk:"vlan_protocol"`
}

// Metadata implements datasource.DataSource.
func (d *pveSdnZoneQinqDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveSdnZoneQinq
}

// Schema implements datasource.DataSource.
func (d *pveSdnZoneQinqDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	attrs := sdnZoneDataSourceAttributes(sdnZoneCommonComputedAttributes())
	attrs["bridge"] = schema.StringAttribute{
		Computed:            true,
		MarkdownDescription: "The local bridge (PVE `bridge`) on each node where the service VLAN for this zone is managed.",
	}
	attrs["tag"] = schema.Int64Attribute{
		Computed:            true,
		MarkdownDescription: "Service-VLAN tag, i.e. the outer VLAN id of the QinQ zone (PVE `tag`).",
	}
	attrs["vlan_protocol"] = schema.StringAttribute{
		Computed:            true,
		MarkdownDescription: "VLAN protocol used for the QinQ zone (PVE `vlan-protocol`). Must be one of: `802.1q`, `802.1ad`.",
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads a QinQ SDN zone from `GET /cluster/sdn/zones/{zone}`. Errors when the zone's upstream type is not `qinq`.",
		Attributes:          attrs,
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveSdnZoneQinqDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = haConfigureDataSource(req, resp)
}

// Read implements datasource.DataSource.
func (d *pveSdnZoneQinqDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveSdnZoneQinqDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Error reading pve_sdn_zone_qinq", "provider client is not configured")
		return
	}
	z, err := sdnZoneGetChecked(ctx, d.client, data.Zone.ValueString(), "qinq")
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_sdn_zone_qinq",
			fmt.Sprintf("reading zone %s: %s", data.Zone.ValueString(), err),
		)
		return
	}
	sdnZoneCommonApply(z, &data.sdnZoneCommonModel)
	data.Bridge = nodeNetworkStringToTF(z.Bridge)
	data.Tag = haInt64PtrToTF(z.Tag)
	data.VlanProtocol = nodeNetworkStringToTF(z.VlanProtocol)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
