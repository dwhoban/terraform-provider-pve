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
	_ datasource.DataSource              = &pveAcmeCertificateDataSource{}
	_ datasource.DataSourceWithConfigure = &pveAcmeCertificateDataSource{}
)

// NewPveAcmeCertificateDataSource returns the data source implementation.
func NewPveAcmeCertificateDataSource() datasource.DataSource {
	return &pveAcmeCertificateDataSource{}
}

// pveAcmeCertificateDataSource reads a node's ACME registration and the
// certificate deployed for it.
type pveAcmeCertificateDataSource struct {
	client *pveclient.Client
}

// pveAcmeCertificateDataSourceModel is the Terraform-facing shape.
type pveAcmeCertificateDataSourceModel struct {
	ID            types.String `tfsdk:"id"`
	Node          types.String `tfsdk:"node"`
	Domains       types.List   `tfsdk:"domains"`
	Fingerprint   types.String `tfsdk:"fingerprint"`
	Issuer        types.String `tfsdk:"issuer"`
	Subject       types.String `tfsdk:"subject"`
	NotAfter      types.Int64  `tfsdk:"not_after"`
	NotBefore     types.Int64  `tfsdk:"not_before"`
	SAN           types.List   `tfsdk:"san"`
	PublicKeyType types.String `tfsdk:"public_key_type"`
	PublicKeyBits types.Int64  `tfsdk:"public_key_bits"`
}

// Metadata implements datasource.DataSource.
func (d *pveAcmeCertificateDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveAcmeCertificate
}

// Schema implements datasource.DataSource.
func (d *pveAcmeCertificateDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads a node's ACME certificate registration (the `acme` entry of `GET /nodes/{node}/config`) and the certificate deployed for it (`GET /nodes/{node}/certificates/info`). The certificate fields are null when no ACME certificate is currently deployed.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Identifier of the data source; equals the `node` name.",
			},
			"node": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The cluster node name.",
			},
			"domains": schema.ListAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Domains registered in the node's `acme` configuration entry. Empty when the node has no ACME registration.",
			},
			"fingerprint": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "SHA 256 fingerprint of the deployed certificate.",
			},
			"issuer": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Certificate issuer name.",
			},
			"subject": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Certificate subject name.",
			},
			"not_after": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Certificate notAfter timestamp (UNIX epoch).",
			},
			"not_before": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Certificate notBefore timestamp (UNIX epoch).",
			},
			"san": schema.ListAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "SubjectAlternativeName entries of the deployed certificate.",
			},
			"public_key_type": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Certificate public key algorithm (e.g. `rsa`).",
			},
			"public_key_bits": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Certificate public key size in bits.",
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveAcmeCertificateDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
func (d *pveAcmeCertificateDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state pveAcmeCertificateDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	node := state.Node.ValueString()
	cfg, err := d.client.GetNodeConfig(ctx, node)
	if err != nil {
		resp.Diagnostics.AddError("Error reading pve_acme_certificate data source", fmt.Sprintf("reading node config on %s: %s", node, err))
		return
	}
	state.Domains = listStringToTF(acmeCertificateDomainNames(cfg.ACMEDomains))
	rows, err := d.client.GetNodeCertificates(ctx, node)
	if err != nil {
		resp.Diagnostics.AddError("Error reading pve_acme_certificate data source", fmt.Sprintf("reading certificates on %s: %s", node, err))
		return
	}
	row := acmeCertificateFindRow(rows, "", acmeCertificatePrimaryDomain(acmeCertificateDomainNames(cfg.ACMEDomains)))
	if row != nil {
		state.Fingerprint = nodeCertificateStringOrNull(row.Fingerprint)
		state.Issuer = nodeCertificateStringOrNull(row.Issuer)
		state.Subject = nodeCertificateStringOrNull(row.Subject)
		state.PublicKeyType = nodeCertificateStringOrNull(row.PublicKeyType)
		state.NotAfter = nodeCertificateInt64OrNull(row.NotAfter)
		state.NotBefore = nodeCertificateInt64OrNull(row.NotBefore)
		state.PublicKeyBits = nodeCertificateInt64OrNull(row.PublicKeyBits)
		state.SAN = listStringToTF(row.SAN)
	}
	state.ID = state.Node
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// acmeCertificateDomainNames lists the domains registered in the node's
// ACME configuration entry.
func acmeCertificateDomainNames(rows []pveclient.NodeACMEDomain) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.Domain)
	}
	return out
}
