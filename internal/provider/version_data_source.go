// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/dwhoban/terraform-provider-pve/internal/pveclient"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure the data source satisfies the interfaces it implements.
var (
	_ datasource.DataSource              = &versionDataSource{}
	_ datasource.DataSourceWithConfigure = &versionDataSource{}
)

// NewVersionDataSource returns a new pve_version data source.
func NewVersionDataSource() datasource.DataSource {
	return &versionDataSource{}
}

// versionDataSource reads GET /version from the configured PVE endpoint.
type versionDataSource struct {
	client *pveclient.Client
}

type versionDataSourceModel struct {
	Version types.String `tfsdk:"version"`
	Release types.String `tfsdk:"release"`
	Repoid  types.String `tfsdk:"repoid"`
}

func (d *versionDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_version"
}

func (d *versionDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Returns the Proxmox VE version of the configured API endpoint.",
		Attributes: map[string]schema.Attribute{
			"version": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "PVE version, e.g. `9.2.2`.",
			},
			"release": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "PVE major.minor release, e.g. `9.2`.",
			},
			"repoid": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "PVE repository commit id.",
			},
		},
	}
}

func (d *versionDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*pveclient.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *pveclient.Client, got: %T.", req.ProviderData),
		)
		return
	}
	d.client = client
}

func (d *versionDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data versionDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if d.client == nil {
		resp.Diagnostics.AddError(
			"Unconfigured Proxmox VE Client",
			"The Proxmox VE client is not configured, usually because provider credentials are missing, invalid, or rejected by the endpoint. Check api_url and api_token (or username and password) in the provider block, or the PVE_* environment variables.",
		)
		return
	}

	version, err := d.client.GetVersion(ctx)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading PVE Version",
			fmt.Sprintf("reading version from the Proxmox VE API: %s", err),
		)
		return
	}

	data.Version = types.StringValue(version.Version)
	data.Release = types.StringValue(version.Release)
	data.Repoid = types.StringValue(version.Repoid)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
