// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ datasource.DataSource              = &pveAcmePluginsDataSource{}
	_ datasource.DataSourceWithConfigure = &pveAcmePluginsDataSource{}
)

// NewPveAcmePluginsDataSource returns the data source implementation.
func NewPveAcmePluginsDataSource() datasource.DataSource {
	return &pveAcmePluginsDataSource{}
}

// pveAcmePluginsDataSource lists the ACME challenge plugins configured on
// the cluster (GET /cluster/acme/plugins), including the built-in
// `standalone` plugin.
type pveAcmePluginsDataSource struct {
	client *pveclient.Client
}

// pveAcmePluginsDataSourceModel is the Terraform-facing shape.
type pveAcmePluginsDataSourceModel struct {
	Type    types.String             `tfsdk:"type"`
	Plugins []pveAcmePluginItemModel `tfsdk:"plugins"`
}

// pveAcmePluginItemModel is one plugin of the listing.
type pveAcmePluginItemModel struct {
	Plugin          types.String `tfsdk:"plugin"`
	Type            types.String `tfsdk:"type"`
	API             types.String `tfsdk:"api"`
	Data            types.String `tfsdk:"data"`
	Disable         types.Bool   `tfsdk:"disable"`
	Nodes           types.List   `tfsdk:"nodes"`
	ValidationDelay types.Int64  `tfsdk:"validation_delay"`
	Digest          types.String `tfsdk:"digest"`
}

// Metadata implements datasource.DataSource.
func (d *pveAcmePluginsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveAcmePlugins
}

// Schema implements datasource.DataSource.
func (d *pveAcmePluginsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists the ACME challenge plugins configured on the cluster (`GET /cluster/acme/plugins`), including the built-in `standalone` plugin. DNS plugin instances can be managed with `pve_acme_dns_plugin`.",
		Attributes: map[string]schema.Attribute{
			"type": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Only list plugins of this challenge type. Must be one of: `dns`, `standalone`.",
				Validators: []validator.String{
					stringvalidator.OneOf("dns", "standalone"),
				},
			},
			"plugins": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Every visible ACME plugin.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"plugin": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Unique identifier of the plugin instance.",
						},
						"type": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "ACME challenge type of the plugin.",
						},
						"api": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "DNS API plugin name (dns plugins only).",
						},
						"data": schema.StringAttribute{
							Computed:            true,
							Sensitive:           true,
							MarkdownDescription: "DNS plugin data (base64-encoded credentials); dns plugins only.",
						},
						"disable": schema.BoolAttribute{
							Computed:            true,
							MarkdownDescription: "Whether the plugin config is disabled.",
						},
						"nodes": schema.ListAttribute{
							Computed:            true,
							ElementType:         types.StringType,
							MarkdownDescription: "Cluster nodes the plugin is limited to; empty means all nodes.",
						},
						"validation_delay": schema.Int64Attribute{
							Computed:            true,
							MarkdownDescription: "Extra delay in seconds before requesting validation.",
						},
						"digest": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Digest of the plugin configuration file.",
						},
					},
				},
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveAcmePluginsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = haConfigureDataSource(req, resp)
}

// Read implements datasource.DataSource.
func (d *pveAcmePluginsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state pveAcmePluginsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Error reading pve_acme_plugins data source", "provider client is not configured")
		return
	}
	plugins, err := d.client.ListAcmePlugins(ctx, state.Type.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_acme_plugins data source",
			fmt.Sprintf("listing ACME plugins: %s", err),
		)
		return
	}
	state.Plugins = make([]pveAcmePluginItemModel, 0, len(plugins))
	for _, p := range plugins {
		state.Plugins = append(state.Plugins, pveAcmePluginItemModel{
			Plugin:          types.StringValue(p.Plugin),
			Type:            types.StringValue(p.Type),
			ValidationDelay: haInt64PtrToTF(p.ValidationDelay),
			API:             nodeNetworkStringToTF(p.API),
			Data:            nodeNetworkStringToTF(p.Data),
			Disable:         nodeNetworkBoolPtrToTF(p.Disable),
			Nodes:           listStringToTF(p.Nodes),
			Digest:          nodeNetworkStringToTF(p.Digest),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
