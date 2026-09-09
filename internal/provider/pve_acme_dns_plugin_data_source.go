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
	_ datasource.DataSource              = &pveAcmeDnsPluginDataSource{}
	_ datasource.DataSourceWithConfigure = &pveAcmeDnsPluginDataSource{}
)

// NewPveAcmeDnsPluginDataSource returns the data source implementation.
func NewPveAcmeDnsPluginDataSource() datasource.DataSource {
	return &pveAcmeDnsPluginDataSource{}
}

// pveAcmeDnsPluginDataSource reads a single ACME DNS challenge plugin
// (GET /cluster/acme/plugins/{id}).
type pveAcmeDnsPluginDataSource struct {
	client *pveclient.Client
}

// pveAcmeDnsPluginDataSourceModel is the Terraform-facing shape.
type pveAcmeDnsPluginDataSourceModel struct {
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
func (d *pveAcmeDnsPluginDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveAcmeDnsPlugin
}

// Schema implements datasource.DataSource.
func (d *pveAcmeDnsPluginDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads a single ACME DNS challenge plugin (`GET /cluster/acme/plugins/{id}`).",
		Attributes: map[string]schema.Attribute{
			"plugin": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Unique identifier of the ACME plugin instance to read.",
			},
			"type": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "ACME challenge type of the plugin.",
			},
			"api": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "DNS API plugin name.",
			},
			"data": schema.StringAttribute{
				Computed:            true,
				Sensitive:           true,
				MarkdownDescription: "DNS plugin data (base64-encoded credentials).",
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
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveAcmeDnsPluginDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = haConfigureDataSource(req, resp)
}

// Read implements datasource.DataSource.
func (d *pveAcmeDnsPluginDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state pveAcmeDnsPluginDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Error reading pve_acme_dns_plugin data source", "provider client is not configured")
		return
	}
	plugin, err := d.client.GetAcmePlugin(ctx, state.Plugin.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_acme_dns_plugin data source",
			fmt.Sprintf("reading ACME dns plugin %s: %s", state.Plugin.ValueString(), err),
		)
		return
	}
	state.Type = types.StringValue(plugin.Type)
	state.API = nodeNetworkStringToTF(plugin.API)
	state.Data = nodeNetworkStringToTF(plugin.Data)
	state.Disable = nodeNetworkBoolPtrToTF(plugin.Disable)
	state.Nodes = listStringToTF(plugin.Nodes)
	state.ValidationDelay = haInt64PtrToTF(plugin.ValidationDelay)
	state.Digest = nodeNetworkStringToTF(plugin.Digest)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
