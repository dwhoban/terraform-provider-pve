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
	_ datasource.DataSource              = &pveNodeCertificateDataSource{}
	_ datasource.DataSourceWithConfigure = &pveNodeCertificateDataSource{}
)

// NewPveNodeCertificateDataSource returns the data source implementation.
func NewPveNodeCertificateDataSource() datasource.DataSource {
	return &pveNodeCertificateDataSource{}
}

// pveNodeCertificateDataSource reads every certificate deployed on a node
// (GET /nodes/{node}/certificates/info).
type pveNodeCertificateDataSource struct {
	client *pveclient.Client
}

// pveNodeCertificateDataSourceModel is the Terraform-facing shape.
type pveNodeCertificateDataSourceModel struct {
	ID           types.String                           `tfsdk:"id"`
	Node         types.String                           `tfsdk:"node"`
	Certificates []pveNodeCertificateDataSourceRowModel `tfsdk:"certificates"`
}

// pveNodeCertificateDataSourceRowModel is one certificates/info row.
type pveNodeCertificateDataSourceRowModel struct {
	Filename      types.String `tfsdk:"filename"`
	Fingerprint   types.String `tfsdk:"fingerprint"`
	Issuer        types.String `tfsdk:"issuer"`
	NotAfter      types.Int64  `tfsdk:"not_after"`
	NotBefore     types.Int64  `tfsdk:"not_before"`
	PEM           types.String `tfsdk:"pem"`
	PublicKeyBits types.Int64  `tfsdk:"public_key_bits"`
	PublicKeyType types.String `tfsdk:"public_key_type"`
	SAN           types.List   `tfsdk:"san"`
	Subject       types.String `tfsdk:"subject"`
}

// Metadata implements datasource.DataSource.
func (d *pveNodeCertificateDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveNodeCertificate
}

// Schema implements datasource.DataSource.
func (d *pveNodeCertificateDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads the certificates deployed on a node (`GET /nodes/{node}/certificates/info`). Rows cover the cluster CA material, the default web certificate, and any custom or ACME chain.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Identifier of the data source; equals the `node` name.",
			},
			"node": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The cluster node name.",
			},
			"certificates": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Certificates installed on the node, one entry per info row.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"filename": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Base name of the installed certificate file (e.g. `pveproxy-ssl.pem`).",
						},
						"fingerprint": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Certificate SHA 256 fingerprint.",
						},
						"issuer": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Certificate issuer name.",
						},
						"not_after": schema.Int64Attribute{
							Computed:            true,
							MarkdownDescription: "Certificate notAfter timestamp (UNIX epoch).",
						},
						"not_before": schema.Int64Attribute{
							Computed:            true,
							MarkdownDescription: "Certificate notBefore timestamp (UNIX epoch).",
						},
						"pem": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Certificate in PEM format.",
						},
						"public_key_bits": schema.Int64Attribute{
							Computed:            true,
							MarkdownDescription: "Certificate public key size in bits.",
						},
						"public_key_type": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Certificate public key algorithm (e.g. `rsa`).",
						},
						"san": schema.ListAttribute{
							Computed:            true,
							ElementType:         types.StringType,
							MarkdownDescription: "SubjectAlternativeName entries of the certificate.",
						},
						"subject": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Certificate subject name.",
						},
					},
				},
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveNodeCertificateDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
func (d *pveNodeCertificateDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state pveNodeCertificateDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	rows, err := d.client.GetNodeCertificates(ctx, state.Node.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading pve_node_certificate data source", fmt.Sprintf("reading certificates on %s: %s", state.Node.ValueString(), err))
		return
	}
	state.Certificates = make([]pveNodeCertificateDataSourceRowModel, 0, len(rows))
	for i := range rows {
		state.Certificates = append(state.Certificates, nodeCertificateRowModelFromTF(&rows[i]))
	}
	state.ID = state.Node
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// nodeCertificateRowModelFromTF converts one wire row to its Terraform
// model, keeping absent fields null.
func nodeCertificateRowModelFromTF(row *pveclient.NodeCertificateInfo) pveNodeCertificateDataSourceRowModel {
	m := pveNodeCertificateDataSourceRowModel{
		Filename:      nodeCertificateStringOrNull(row.Filename),
		Fingerprint:   nodeCertificateStringOrNull(row.Fingerprint),
		Issuer:        nodeCertificateStringOrNull(row.Issuer),
		NotAfter:      nodeCertificateInt64OrNull(row.NotAfter),
		NotBefore:     nodeCertificateInt64OrNull(row.NotBefore),
		PEM:           nodeCertificateStringOrNull(row.PEM),
		PublicKeyBits: nodeCertificateInt64OrNull(row.PublicKeyBits),
		PublicKeyType: nodeCertificateStringOrNull(row.PublicKeyType),
		SAN:           listStringToTF(row.SAN),
		Subject:       nodeCertificateStringOrNull(row.Subject),
	}
	return m
}
