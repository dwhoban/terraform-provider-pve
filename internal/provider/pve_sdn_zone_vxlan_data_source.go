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
	_ datasource.DataSource              = &pveSdnZoneVxlanDataSource{}
	_ datasource.DataSourceWithConfigure = &pveSdnZoneVxlanDataSource{}
)

// NewPveSdnZoneVxlanDataSource returns the data source implementation.
func NewPveSdnZoneVxlanDataSource() datasource.DataSource {
	return &pveSdnZoneVxlanDataSource{}
}

// pveSdnZoneVxlanDataSource reads a single VXLAN SDN zone (GET
// /cluster/sdn/zones/{zone}), erroring when the upstream type is not vxlan.
type pveSdnZoneVxlanDataSource struct {
	client *pveclient.Client
}

// pveSdnZoneVxlanDataSourceModel is the Terraform-facing shape.
type pveSdnZoneVxlanDataSourceModel struct {
	sdnZoneCommonModel
	Peers     types.String `tfsdk:"peers"`
	VxlanPort types.Int64  `tfsdk:"vxlan_port"`
	Fabric    types.String `tfsdk:"fabric"`
}

// Metadata implements datasource.DataSource.
func (d *pveSdnZoneVxlanDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveSdnZoneVxlan
}

// Schema implements datasource.DataSource.
func (d *pveSdnZoneVxlanDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	attrs := sdnZoneDataSourceAttributes(sdnZoneCommonComputedAttributes())
	attrs["peers"] = schema.StringAttribute{
		Computed:            true,
		MarkdownDescription: "Comma-separated list of peer addresses (PVE `ip-list`) that are part of the VXLAN zone, usually the IPs of the participating nodes.",
	}
	attrs["vxlan_port"] = schema.Int64Attribute{
		Computed:            true,
		MarkdownDescription: "UDP port used for the VXLAN tunnel (PVE `vxlan-port`, default `4789`).",
	}
	attrs["fabric"] = schema.StringAttribute{
		Computed:            true,
		MarkdownDescription: "SDN fabric (PVE `pve-sdn-fabric-id`) used as the underlay for this VXLAN zone.",
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads a VXLAN SDN zone from `GET /cluster/sdn/zones/{zone}`. Errors when the zone's upstream type is not `vxlan`.",
		Attributes:          attrs,
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveSdnZoneVxlanDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = haConfigureDataSource(req, resp)
}

// Read implements datasource.DataSource.
func (d *pveSdnZoneVxlanDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveSdnZoneVxlanDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Error reading pve_sdn_zone_vxlan", "provider client is not configured")
		return
	}
	z, err := sdnZoneGetChecked(ctx, d.client, data.Zone.ValueString(), "vxlan")
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_sdn_zone_vxlan",
			fmt.Sprintf("reading zone %s: %s", data.Zone.ValueString(), err),
		)
		return
	}
	sdnZoneCommonApply(z, &data.sdnZoneCommonModel)
	data.Peers = nodeNetworkStringToTF(z.Peers)
	data.VxlanPort = haInt64PtrToTF(z.VxlanPort)
	data.Fabric = nodeNetworkStringToTF(z.Fabric)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
