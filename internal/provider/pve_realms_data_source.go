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
	_ datasource.DataSource              = &pveRealmsDataSource{}
	_ datasource.DataSourceWithConfigure = &pveRealmsDataSource{}
)

// NewPveRealmsDataSource returns the data source implementation.
func NewPveRealmsDataSource() datasource.DataSource {
	return &pveRealmsDataSource{}
}

// pveRealmsDataSource lists the authentication realms in the cluster,
// including the built-ins pam and pve (GET /access/domains).
type pveRealmsDataSource struct {
	client *pveclient.Client
}

// pveRealmsDataSourceModel is the Terraform-facing shape.
type pveRealmsDataSourceModel struct {
	ID     types.String                  `tfsdk:"id"`
	Realms []pveRealmsDataSourceRowModel `tfsdk:"realms"`
}

// pveRealmsDataSourceRowModel mirrors the per-row schema of the realm
// index. Field names line up with the pveclient struct.
type pveRealmsDataSourceRowModel struct {
	Realm   types.String `tfsdk:"realm"`
	Type    types.String `tfsdk:"type"`
	Comment types.String `tfsdk:"comment"`
	TFA     types.String `tfsdk:"tfa"`
}

// Metadata implements datasource.DataSource.
func (d *pveRealmsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveRealms
}

// Schema implements datasource.DataSource.
func (d *pveRealmsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists every authentication realm in the Proxmox VE cluster, including the built-ins `pam` and `pve`, as reported by `GET /access/domains`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Static identifier for the realm index.",
			},
			"realms": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Authentication realms configured in the cluster.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"realm": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Authentication domain ID.",
						},
						"type": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Realm type: one of `ad`, `ldap`, `openid`, `pam`, or `pve`.",
						},
						"comment": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Description shown in the PVE login window; null when unset.",
						},
						"tfa": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Two-factor authentication provider; one of `yubico` or `oath` when enabled, null otherwise.",
						},
					},
				},
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveRealmsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
func (d *pveRealmsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config pveRealmsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	domains, err := d.client.ListDomains(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading pve_realms", fmt.Sprintf("listing authentication realms: %s", err))
		return
	}
	rows := make([]pveRealmsDataSourceRowModel, 0, len(domains))
	for _, domain := range domains {
		rows = append(rows, pveRealmsDataSourceRowModel{
			Realm:   types.StringValue(domain.Realm),
			Type:    types.StringValue(domain.Type),
			Comment: realmStringToTF(domain.Comment),
			TFA:     realmStringToTF(domain.TFA),
		})
	}
	config.ID = types.StringValue("pve_realms")
	config.Realms = rows
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
