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
	_ datasource.DataSource              = &pveSdnDnsDataSource{}
	_ datasource.DataSourceWithConfigure = &pveSdnDnsDataSource{}
)

// NewPveSdnDnsDataSource returns the data source implementation.
func NewPveSdnDnsDataSource() datasource.DataSource {
	return &pveSdnDnsDataSource{}
}

// pveSdnDnsDataSource reads a single SDN reverse-DNS plugin object
// (GET /cluster/sdn/dns/{dns}).
type pveSdnDnsDataSource struct {
	client *pveclient.Client
}

// pveSdnDnsDataSourceModel is the Terraform-facing shape.
type pveSdnDnsDataSourceModel struct {
	Dns           types.String `tfsdk:"dns"`
	Type          types.String `tfsdk:"type"`
	Key           types.String `tfsdk:"key"`
	URL           types.String `tfsdk:"url"`
	Fingerprint   types.String `tfsdk:"fingerprint"`
	Reversemaskv6 types.Int64  `tfsdk:"reversemaskv6"`
	TTL           types.Int64  `tfsdk:"ttl"`
}

// Metadata implements datasource.DataSource.
func (d *pveSdnDnsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveSdnDns
}

// Schema implements datasource.DataSource.
func (d *pveSdnDnsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads a single SDN reverse-DNS plugin object (`GET /cluster/sdn/dns/{dns}`).",
		Attributes: map[string]schema.Attribute{
			"dns": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The SDN DNS object identifier to read.",
			},
			"type": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "DNS plugin type; `powerdns` per the pin.",
			},
			"key": schema.StringAttribute{
				Computed:            true,
				Sensitive:           true,
				MarkdownDescription: "API authentication key for the PowerDNS server.",
			},
			"url": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "URL of the PowerDNS API server.",
			},
			"fingerprint": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Certificate SHA 256 fingerprint of the PowerDNS server.",
			},
			"reversemaskv6": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Mask for IPv6 reverse lookups.",
			},
			"ttl": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Record TTL for registered entries.",
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveSdnDnsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = haConfigureDataSource(req, resp)
}

// Read implements datasource.DataSource.
func (d *pveSdnDnsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state pveSdnDnsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Error reading pve_sdn_dns data source", "provider client is not configured")
		return
	}
	dns, err := d.client.GetSdnDns(ctx, state.Dns.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_sdn_dns data source",
			fmt.Sprintf("reading SDN dns %s: %s", state.Dns.ValueString(), err),
		)
		return
	}
	state.Type = nodeNetworkStringToTF(dns.Type)
	state.Key = nodeNetworkStringToTF(dns.Key)
	state.URL = nodeNetworkStringToTF(dns.URL)
	state.Fingerprint = nodeNetworkStringToTF(dns.Fingerprint)
	state.Reversemaskv6 = haInt64PtrToTF(dns.Reversemaskv6)
	state.TTL = haInt64PtrToTF(dns.TTL)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
