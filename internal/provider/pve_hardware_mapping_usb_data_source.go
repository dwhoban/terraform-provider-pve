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
	_ datasource.DataSource              = &pveHardwareMappingUsbDataSource{}
	_ datasource.DataSourceWithConfigure = &pveHardwareMappingUsbDataSource{}
)

// NewPveHardwareMappingUsbDataSource returns the data source implementation.
func NewPveHardwareMappingUsbDataSource() datasource.DataSource {
	return &pveHardwareMappingUsbDataSource{}
}

// pveHardwareMappingUsbDataSource reads a single logical USB mapping
// (GET /cluster/mapping/usb/{id}).
type pveHardwareMappingUsbDataSource struct {
	client *pveclient.Client
}

// pveHardwareMappingUsbDataSourceModel is the Terraform-facing shape.
type pveHardwareMappingUsbDataSourceModel struct {
	ID          types.String                      `tfsdk:"id"`
	Description types.String                      `tfsdk:"description"`
	Map         []pveHardwareMappingUsbEntryModel `tfsdk:"map"`
}

// Metadata implements datasource.DataSource.
func (d *pveHardwareMappingUsbDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveHardwareMappingUsb
}

// Schema implements datasource.DataSource.
func (d *pveHardwareMappingUsbDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads a single logical USB hardware mapping (`GET /cluster/mapping/usb/{id}`).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The ID of the logical USB mapping to read.",
			},
			"description": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Description of the logical USB device.",
			},
			"map": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Per-node device entries of the mapping.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"node": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The cluster node name.",
						},
						"id": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Expected vendor and device ID in `vendor:device` form.",
						},
						"path": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "USB port path of the device.",
						},
						"description": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Description of the node-specific device.",
						},
					},
				},
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveHardwareMappingUsbDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = haConfigureDataSource(req, resp)
}

// Read implements datasource.DataSource.
func (d *pveHardwareMappingUsbDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state pveHardwareMappingUsbDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Error reading pve_hardware_mapping_usb data source", "provider client is not configured")
		return
	}
	mapping, err := d.client.GetMappingUSB(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_hardware_mapping_usb data source",
			fmt.Sprintf("reading USB mapping %s: %s", state.ID.ValueString(), err),
		)
		return
	}
	state.Description = nodeNetworkStringToTF(mapping.Description)
	state.Map = make([]pveHardwareMappingUsbEntryModel, 0, len(mapping.Map))
	for _, e := range mapping.Map {
		state.Map = append(state.Map, pveHardwareMappingUsbEntryModel{
			Node:        types.StringValue(e.Node),
			ID:          nodeNetworkStringToTF(e.ID),
			Path:        nodeNetworkStringToTF(e.Path),
			Description: nodeNetworkStringToTF(e.Description),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
