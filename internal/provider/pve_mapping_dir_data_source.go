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
	_ datasource.DataSource              = &pveMappingDirDataSource{}
	_ datasource.DataSourceWithConfigure = &pveMappingDirDataSource{}
)

// NewPveMappingDirDataSource returns the data source implementation.
func NewPveMappingDirDataSource() datasource.DataSource {
	return &pveMappingDirDataSource{}
}

// pveMappingDirDataSource reads a single directory mapping
// (GET /cluster/mapping/dir/{id}).
type pveMappingDirDataSource struct {
	client *pveclient.Client
}

// pveMappingDirDataSourceModel is the Terraform-facing shape.
type pveMappingDirDataSourceModel struct {
	ID          types.String              `tfsdk:"id"`
	Description types.String              `tfsdk:"description"`
	Map         []pveMappingDirEntryModel `tfsdk:"map"`
}

// Metadata implements datasource.DataSource.
func (d *pveMappingDirDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveMappingDir
}

// Schema implements datasource.DataSource.
func (d *pveMappingDirDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads a single directory hardware mapping (`GET /cluster/mapping/dir/{id}`).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The ID of the directory mapping to read.",
			},
			"description": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Description of the directory mapping.",
			},
			"map": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Per-node directory entries of the mapping.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"node": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The cluster node name.",
						},
						"path": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Absolute directory path on the node.",
						},
					},
				},
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveMappingDirDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = haConfigureDataSource(req, resp)
}

// Read implements datasource.DataSource.
func (d *pveMappingDirDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state pveMappingDirDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Error reading pve_mapping_dir data source", "provider client is not configured")
		return
	}
	mapping, err := d.client.GetMappingDir(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_mapping_dir data source",
			fmt.Sprintf("reading directory mapping %s: %s", state.ID.ValueString(), err),
		)
		return
	}
	state.Description = nodeNetworkStringToTF(mapping.Description)
	state.Map = make([]pveMappingDirEntryModel, 0, len(mapping.Map))
	for _, e := range mapping.Map {
		state.Map = append(state.Map, pveMappingDirEntryModel{
			Node: types.StringValue(e.Node),
			Path: types.StringValue(e.Path),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
