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

// Ensure the framework interfaces are satisfied.
var (
	_ datasource.DataSource              = &pveVersionDataSource{}
	_ datasource.DataSourceWithConfigure = &pveVersionDataSource{}
)

// NewPveVersionDataSource returns the data source implementation.
func NewPveVersionDataSource() datasource.DataSource {
	return &pveVersionDataSource{}
}

// pveVersionDataSource reads the API version details (GET /version).
type pveVersionDataSource struct {
	client *pveclient.Client
}

// pveVersionDataSourceModel is the Terraform-facing shape.
type pveVersionDataSourceModel struct {
	ID      types.String `tfsdk:"id"`
	Release types.String `tfsdk:"release"`
	RepoID  types.String `tfsdk:"repoid"`
	Version types.String `tfsdk:"version"`
	Console types.String `tfsdk:"console"`
}

// Metadata implements datasource.DataSource.
func (d *pveVersionDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveVersion
}

// Schema implements datasource.DataSource.
func (d *pveVersionDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Proxmox VE API version details as reported by `GET /version`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Static identifier for the version data source.",
			},
			"release": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The current Proxmox VE point release in `x.y` format.",
			},
			"repoid": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The short git revision from which this version was built.",
			},
			"version": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The full pve-manager package version of this node.",
			},
			"console": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The default console viewer. One of: `applet`, `vv`, `html5`, `xtermjs`.",
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveVersionDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

// Read implements datasource.DataSource.
func (d *pveVersionDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveVersionDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	version, err := d.client.GetVersion(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading pve_version", fmt.Sprintf("reading API version: %s", err))
		return
	}
	data.ID = types.StringValue("pve_version")
	data.Release = types.StringValue(version.Release)
	data.RepoID = types.StringValue(version.RepoID)
	data.Version = types.StringValue(version.Version)
	data.Console = types.StringValue(version.Console)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
