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
	_ datasource.DataSource              = &pveRealmLdapDataSource{}
	_ datasource.DataSourceWithConfigure = &pveRealmLdapDataSource{}
)

// NewPveRealmLdapDataSource returns the data source implementation.
func NewPveRealmLdapDataSource() datasource.DataSource {
	return &pveRealmLdapDataSource{}
}

// pveRealmLdapDataSource reads one LDAP authentication realm
// (type=ldap) from /access/domains/{realm}.
type pveRealmLdapDataSource struct {
	client *pveclient.Client
}

// pveRealmLdapDataSourceModel is the Terraform-facing shape.
type pveRealmLdapDataSourceModel struct {
	realmCommonModel
	realmTransportModel
	realmLdapModel
}

// Metadata implements datasource.DataSource.
func (d *pveRealmLdapDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveRealmLdap
}

// Schema implements datasource.DataSource.
func (d *pveRealmLdapDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads an LDAP authentication realm (`type=ldap`) from Proxmox VE (`/access/domains/{realm}`).",
		Attributes:          realmDataSourceAttributes(realmLDAPAttributes(true, false)),
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveRealmLdapDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
func (d *pveRealmLdapDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config pveRealmLdapDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	realm := config.Realm.ValueString()
	domain, err := realmGetChecked(ctx, d.client, realm, realmTypeLDAP)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_realm_ldap",
			fmt.Sprintf("reading realm %s: %s", realm, err),
		)
		return
	}
	realmCommonIntoModel(&config.realmCommonModel, domain)
	realmTransportIntoModel(&config.realmTransportModel, domain)
	realmLDAPIntoModel(&config.realmLdapModel, domain)
	config.Realm = types.StringValue(realm)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
