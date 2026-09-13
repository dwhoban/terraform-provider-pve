// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ datasource.DataSource              = &pveSdnZoneSimpleDataSource{}
	_ datasource.DataSourceWithConfigure = &pveSdnZoneSimpleDataSource{}
)

// NewPveSdnZoneSimpleDataSource returns the data source implementation.
func NewPveSdnZoneSimpleDataSource() datasource.DataSource {
	return &pveSdnZoneSimpleDataSource{}
}

// pveSdnZoneSimpleDataSource reads a single simple SDN zone (GET
// /cluster/sdn/zones/{zone}), erroring when the upstream type is not
// simple.
type pveSdnZoneSimpleDataSource struct {
	client *pveclient.Client
}

// pveSdnZoneSimpleDataSourceModel is the Terraform-facing shape.
type pveSdnZoneSimpleDataSourceModel struct {
	sdnZoneCommonModel
}

// Metadata implements datasource.DataSource.
func (d *pveSdnZoneSimpleDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveSdnZoneSimple
}

// Schema implements datasource.DataSource.
func (d *pveSdnZoneSimpleDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads a simple SDN zone from `GET /cluster/sdn/zones/{zone}`. Errors when the zone's upstream type is not `simple`.",
		Attributes:          sdnZoneDataSourceAttributes(sdnZoneCommonComputedAttributes()),
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveSdnZoneSimpleDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = haConfigureDataSource(req, resp)
}

// Read implements datasource.DataSource.
func (d *pveSdnZoneSimpleDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveSdnZoneSimpleDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Error reading pve_sdn_zone_simple", "provider client is not configured")
		return
	}
	z, err := sdnZoneGetChecked(ctx, d.client, data.Zone.ValueString(), "simple")
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_sdn_zone_simple",
			fmt.Sprintf("reading zone %s: %s", data.Zone.ValueString(), err),
		)
		return
	}
	sdnZoneCommonApply(z, &data.sdnZoneCommonModel)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
