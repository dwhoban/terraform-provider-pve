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
	_ datasource.DataSource              = &pveRealmAdDataSource{}
	_ datasource.DataSourceWithConfigure = &pveRealmAdDataSource{}
)

// NewPveRealmAdDataSource returns the data source implementation.
func NewPveRealmAdDataSource() datasource.DataSource {
	return &pveRealmAdDataSource{}
}

// pveRealmAdDataSource reads one Active Directory authentication realm
// (type=ad) from /access/domains/{realm}.
type pveRealmAdDataSource struct {
	client *pveclient.Client
}

// pveRealmAdDataSourceModel is the Terraform-facing shape.
type pveRealmAdDataSourceModel struct {
	realmCommonModel
	realmTransportModel
	realmAdModel
}

// Metadata implements datasource.DataSource.
func (d *pveRealmAdDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveRealmAd
}

// Schema implements datasource.DataSource.
func (d *pveRealmAdDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads an Active Directory authentication realm (`type=ad`) from Proxmox VE (`/access/domains/{realm}`).",
		Attributes:          realmDataSourceAttributes(realmADAttributes(true, false)),
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveRealmAdDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
func (d *pveRealmAdDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config pveRealmAdDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	realm := config.Realm.ValueString()
	domain, err := realmGetChecked(ctx, d.client, realm, realmTypeAD)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_realm_ad",
			fmt.Sprintf("reading realm %s: %s", realm, err),
		)
		return
	}
	realmCommonIntoModel(&config.realmCommonModel, domain)
	realmTransportIntoModel(&config.realmTransportModel, domain)
	realmADIntoModel(&config.realmAdModel, domain)
	config.Realm = types.StringValue(realm)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
