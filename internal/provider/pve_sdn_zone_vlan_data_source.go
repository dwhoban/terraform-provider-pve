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
	_ datasource.DataSource              = &pveSdnZoneVlanDataSource{}
	_ datasource.DataSourceWithConfigure = &pveSdnZoneVlanDataSource{}
)

// NewPveSdnZoneVlanDataSource returns the data source implementation.
func NewPveSdnZoneVlanDataSource() datasource.DataSource {
	return &pveSdnZoneVlanDataSource{}
}

// pveSdnZoneVlanDataSource reads a single VLAN SDN zone (GET
// /cluster/sdn/zones/{zone}), erroring when the upstream type is not vlan.
type pveSdnZoneVlanDataSource struct {
	client *pveclient.Client
}

// pveSdnZoneVlanDataSourceModel is the Terraform-facing shape.
type pveSdnZoneVlanDataSourceModel struct {
	sdnZoneCommonModel
	Bridge                   types.String `tfsdk:"bridge"`
	BridgeDisableMacLearning types.Bool   `tfsdk:"bridge_disable_mac_learning"`
}

// Metadata implements datasource.DataSource.
func (d *pveSdnZoneVlanDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveSdnZoneVlan
}

// Schema implements datasource.DataSource.
func (d *pveSdnZoneVlanDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	attrs := sdnZoneDataSourceAttributes(sdnZoneCommonComputedAttributes())
	attrs["bridge"] = schema.StringAttribute{
		Computed:            true,
		MarkdownDescription: "The local bridge (PVE `bridge`) on each node where the VLANs for this zone are managed.",
	}
	attrs["bridge_disable_mac_learning"] = schema.BoolAttribute{
		Computed:            true,
		MarkdownDescription: "Whether auto MAC learning (PVE `bridge-disable-mac-learning`) is disabled on the zone bridge.",
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads a VLAN SDN zone from `GET /cluster/sdn/zones/{zone}`. Errors when the zone's upstream type is not `vlan`.",
		Attributes:          attrs,
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveSdnZoneVlanDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = haConfigureDataSource(req, resp)
}

// Read implements datasource.DataSource.
func (d *pveSdnZoneVlanDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveSdnZoneVlanDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Error reading pve_sdn_zone_vlan", "provider client is not configured")
		return
	}
	z, err := sdnZoneGetChecked(ctx, d.client, data.Zone.ValueString(), "vlan")
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_sdn_zone_vlan",
			fmt.Sprintf("reading zone %s: %s", data.Zone.ValueString(), err),
		)
		return
	}
	sdnZoneCommonApply(z, &data.sdnZoneCommonModel)
	data.Bridge = nodeNetworkStringToTF(z.Bridge)
	data.BridgeDisableMacLearning = nodeNetworkBoolPtrToTF(z.BridgeDisableMacLearning)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
