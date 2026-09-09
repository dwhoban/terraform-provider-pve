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
	_ datasource.DataSource              = &pveAcmeAccountDataSource{}
	_ datasource.DataSourceWithConfigure = &pveAcmeAccountDataSource{}
)

// NewPveAcmeAccountDataSource returns the data source implementation.
func NewPveAcmeAccountDataSource() datasource.DataSource {
	return &pveAcmeAccountDataSource{}
}

// pveAcmeAccountDataSource reads a single ACME account
// (GET /cluster/acme/account/{name}).
type pveAcmeAccountDataSource struct {
	client *pveclient.Client
}

// pveAcmeAccountDataSourceModel is the Terraform-facing shape.
type pveAcmeAccountDataSourceModel struct {
	Name       types.String `tfsdk:"name"`
	Directory  types.String `tfsdk:"directory"`
	AccountURL types.String `tfsdk:"account_url"`
	Tos        types.String `tfsdk:"tos"`
}

// Metadata implements datasource.DataSource.
func (d *pveAcmeAccountDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveAcmeAccount
}

// Schema implements datasource.DataSource.
func (d *pveAcmeAccountDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads an ACME account registered on this cluster (`GET /cluster/acme/account/{name}`), exposing the CA-reported directory URL, account URL, and Terms of Service.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "ACME account config file name to read (the API defaults to `default`).",
			},
			"directory": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "URL of the ACME CA directory endpoint.",
			},
			"account_url": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The CA account URL as reported by the `location` field of the account read.",
			},
			"tos": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The CA's Terms of Service URL.",
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveAcmeAccountDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = haConfigureDataSource(req, resp)
}

// Read implements datasource.DataSource.
func (d *pveAcmeAccountDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state pveAcmeAccountDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Error reading pve_acme_account data source", "provider client is not configured")
		return
	}
	acct, err := d.client.GetAcmeAccount(ctx, state.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_acme_account data source",
			fmt.Sprintf("reading ACME account %s: %s", state.Name.ValueString(), err),
		)
		return
	}
	state.Directory = nodeNetworkStringToTF(acct.Directory)
	state.AccountURL = nodeNetworkStringToTF(acct.Location)
	state.Tos = nodeNetworkStringToTF(acct.Tos)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
