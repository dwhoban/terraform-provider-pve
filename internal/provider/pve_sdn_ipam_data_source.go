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
	_ datasource.DataSource              = &pveSdnIpamDataSource{}
	_ datasource.DataSourceWithConfigure = &pveSdnIpamDataSource{}
)

// NewPveSdnIpamDataSource returns the data source implementation.
func NewPveSdnIpamDataSource() datasource.DataSource {
	return &pveSdnIpamDataSource{}
}

// pveSdnIpamDataSource reads a single SDN IPAM plugin object and its
// allocation status (GET /cluster/sdn/ipams/{ipam} and
// GET /cluster/sdn/ipams/{ipam}/status).
type pveSdnIpamDataSource struct {
	client *pveclient.Client
}

// pveSdnIpamDataSourceModel is the Terraform-facing shape.
type pveSdnIpamDataSourceModel struct {
	Ipam        types.String `tfsdk:"ipam"`
	Type        types.String `tfsdk:"type"`
	URL         types.String `tfsdk:"url"`
	Token       types.String `tfsdk:"token"`
	Fingerprint types.String `tfsdk:"fingerprint"`
	Section     types.Int64  `tfsdk:"section"`
	Status      types.List   `tfsdk:"status"`
}

// Metadata implements datasource.DataSource.
func (d *pveSdnIpamDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveSdnIpam
}

// Schema implements datasource.DataSource.
func (d *pveSdnIpamDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads a single SDN IPAM plugin object (`GET /cluster/sdn/ipams/{ipam}`) plus its allocation status (`GET /cluster/sdn/ipams/{ipam}/status`).",
		Attributes: map[string]schema.Attribute{
			"ipam": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The SDN IPAM object identifier to read.",
			},
			"type": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "IPAM plugin type; one of `netbox`, `phpipam`, `pve`.",
			},
			"url": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "URL of the IPAM server.",
			},
			"token": schema.StringAttribute{
				Computed:            true,
				Sensitive:           true,
				MarkdownDescription: "API token for the IPAM server.",
			},
			"fingerprint": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Certificate SHA 256 fingerprint of the IPAM server.",
			},
			"section": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "phpIPAM section identifier.",
			},
			"status": schema.ListAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "IPAM entry index as returned by `GET /cluster/sdn/ipams/{ipam}/status`; the pin declares the entries as untyped objects, so each element carries one entry verbatim as a JSON object string.",
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveSdnIpamDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = haConfigureDataSource(req, resp)
}

// Read implements datasource.DataSource.
func (d *pveSdnIpamDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state pveSdnIpamDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Error reading pve_sdn_ipam data source", "provider client is not configured")
		return
	}
	ipam, err := d.client.GetSdnIpam(ctx, state.Ipam.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_sdn_ipam data source",
			fmt.Sprintf("reading SDN ipam %s: %s", state.Ipam.ValueString(), err),
		)
		return
	}
	state.Type = nodeNetworkStringToTF(ipam.Type)
	state.URL = nodeNetworkStringToTF(ipam.URL)
	state.Token = nodeNetworkStringToTF(ipam.Token)
	state.Fingerprint = nodeNetworkStringToTF(ipam.Fingerprint)
	state.Section = haInt64PtrToTF(ipam.Section)
	status, err := d.client.ListSdnIpamStatus(ctx, state.Ipam.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_sdn_ipam data source",
			fmt.Sprintf("reading status of SDN ipam %s: %s", state.Ipam.ValueString(), err),
		)
		return
	}
	state.Status = listStringToTF(status)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
