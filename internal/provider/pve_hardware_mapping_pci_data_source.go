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
	_ datasource.DataSource              = &pveHardwareMappingPciDataSource{}
	_ datasource.DataSourceWithConfigure = &pveHardwareMappingPciDataSource{}
)

// NewPveHardwareMappingPciDataSource returns the data source implementation.
func NewPveHardwareMappingPciDataSource() datasource.DataSource {
	return &pveHardwareMappingPciDataSource{}
}

// pveHardwareMappingPciDataSource reads a single logical PCI mapping
// (GET /cluster/mapping/pci/{id}).
type pveHardwareMappingPciDataSource struct {
	client *pveclient.Client
}

// pveHardwareMappingPciDataSourceModel is the Terraform-facing shape.
type pveHardwareMappingPciDataSourceModel struct {
	ID                   types.String                      `tfsdk:"id"`
	Description          types.String                      `tfsdk:"description"`
	Mdev                 types.Bool                        `tfsdk:"mdev"`
	LiveMigrationCapable types.Bool                        `tfsdk:"live_migration_capable"`
	Map                  []pveHardwareMappingPciEntryModel `tfsdk:"map"`
}

// Metadata implements datasource.DataSource.
func (d *pveHardwareMappingPciDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveHardwareMappingPci
}

// Schema implements datasource.DataSource.
func (d *pveHardwareMappingPciDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads a single logical PCI hardware mapping (`GET /cluster/mapping/pci/{id}`).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The ID of the logical PCI mapping to read.",
			},
			"description": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Description of the logical PCI device.",
			},
			"mdev": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether the device(s) are marked as capable of providing mediated devices.",
			},
			"live_migration_capable": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether the device(s) are marked as able to be live-migrated.",
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
						"iommugroup": schema.Int64Attribute{
							Computed:            true,
							MarkdownDescription: "The IOMMU group the device is expected in.",
						},
						"path": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "PCI path of the device.",
						},
						"subsystem_id": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Expected subsystem vendor and device ID in `vendor:device` form.",
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
func (d *pveHardwareMappingPciDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = haConfigureDataSource(req, resp)
}

// Read implements datasource.DataSource.
func (d *pveHardwareMappingPciDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state pveHardwareMappingPciDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Error reading pve_hardware_mapping_pci data source", "provider client is not configured")
		return
	}
	mapping, err := d.client.GetMappingPCI(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_hardware_mapping_pci data source",
			fmt.Sprintf("reading PCI mapping %s: %s", state.ID.ValueString(), err),
		)
		return
	}
	state.Description = nodeNetworkStringToTF(mapping.Description)
	state.Mdev = nodeNetworkBoolPtrToTF(mapping.Mdev)
	state.LiveMigrationCapable = nodeNetworkBoolPtrToTF(mapping.LiveMigrationCapable)
	state.Map = make([]pveHardwareMappingPciEntryModel, 0, len(mapping.Map))
	for _, e := range mapping.Map {
		state.Map = append(state.Map, pveHardwareMappingPciEntryModel{
			Node:        types.StringValue(e.Node),
			ID:          nodeNetworkStringToTF(e.ID),
			IommuGroup:  haInt64PtrToTF(e.IOMMUGroup),
			Path:        nodeNetworkStringToTF(e.Path),
			SubsystemID: nodeNetworkStringToTF(e.SubsystemID),
			Description: nodeNetworkStringToTF(e.Description),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
