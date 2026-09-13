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
	_ datasource.DataSource              = &pveRealmOpenidDataSource{}
	_ datasource.DataSourceWithConfigure = &pveRealmOpenidDataSource{}
)

// NewPveRealmOpenidDataSource returns the data source implementation.
func NewPveRealmOpenidDataSource() datasource.DataSource {
	return &pveRealmOpenidDataSource{}
}

// pveRealmOpenidDataSource reads one OpenID Connect authentication realm
// (type=openid) from /access/domains/{realm}.
type pveRealmOpenidDataSource struct {
	client *pveclient.Client
}

// pveRealmOpenidDataSourceModel is the Terraform-facing shape.
type pveRealmOpenidDataSourceModel struct {
	realmCommonModel
	realmOpenidModel
}

// Metadata implements datasource.DataSource.
func (d *pveRealmOpenidDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveRealmOpenid
}

// Schema implements datasource.DataSource.
func (d *pveRealmOpenidDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads an OpenID Connect authentication realm (`type=openid`) from Proxmox VE (`/access/domains/{realm}`).",
		Attributes:          realmDataSourceAttributes(realmOpenidAttributes(true, false)),
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveRealmOpenidDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
func (d *pveRealmOpenidDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config pveRealmOpenidDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	realm := config.Realm.ValueString()
	domain, err := realmGetChecked(ctx, d.client, realm, realmTypeOpenid)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_realm_openid",
			fmt.Sprintf("reading realm %s: %s", realm, err),
		)
		return
	}
	realmCommonIntoModel(&config.realmCommonModel, domain)
	realmOpenidIntoModel(&config.realmOpenidModel, domain)
	config.Realm = types.StringValue(realm)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
