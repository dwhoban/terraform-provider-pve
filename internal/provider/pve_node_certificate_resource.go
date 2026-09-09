// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ resource.Resource                = &pveNodeCertificateResource{}
	_ resource.ResourceWithConfigure   = &pveNodeCertificateResource{}
	_ resource.ResourceWithImportState = &pveNodeCertificateResource{}
)

// NewPveNodeCertificateResource returns the resource implementation.
func NewPveNodeCertificateResource() resource.Resource {
	return &pveNodeCertificateResource{}
}

// pveNodeCertificateResource manages the node's custom TLS certificate
// chain (POST/DELETE /nodes/{node}/certificates/custom). Both verbs are
// synchronous per the pin — no task UPID is returned.
type pveNodeCertificateResource struct {
	client *pveclient.Client
}

// pveNodeCertificateResourceModel is the Terraform-facing shape.
type pveNodeCertificateResourceModel struct {
	ID            types.String `tfsdk:"id"`
	Node          types.String `tfsdk:"node"`
	CertificatesP types.String `tfsdk:"certificates_pem"`
	PrivateKey    types.String `tfsdk:"private_key"`
	Force         types.Bool   `tfsdk:"force"`
	Restart       types.Bool   `tfsdk:"restart"`
	Fingerprint   types.String `tfsdk:"fingerprint"`
	Issuer        types.String `tfsdk:"issuer"`
	Subject       types.String `tfsdk:"subject"`
	NotAfter      types.Int64  `tfsdk:"not_after"`
	NotBefore     types.Int64  `tfsdk:"not_before"`
	SAN           types.List   `tfsdk:"san"`
	PublicKeyType types.String `tfsdk:"public_key_type"`
	PublicKeyBits types.Int64  `tfsdk:"public_key_bits"`
}

// nodeCertificateCustomFilename is the info row PVE reports for the custom
// chain it installs as /etc/pve/local/pveproxy-ssl.pem.
const nodeCertificateCustomFilename = "pveproxy-ssl.pem"

// Metadata implements resource.Resource.
func (r *pveNodeCertificateResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveNodeCertificate
}

// Schema implements resource.Resource.
func (r *pveNodeCertificateResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the node's custom TLS certificate chain (`POST`/`DELETE /nodes/{node}/certificates/custom`). PVE stores the chain in `/etc/pve/local/pveproxy-ssl.pem` and serves it for the web UI and API. Upload and removal are synchronous; pair with `restart = true` to make `pveproxy` pick the chain up immediately. Read matches the deployed chain by stored fingerprint, falling back to the `pveproxy-ssl.pem` info row; when the chain is no longer deployed the resource is recreated on the next plan. Import ID: the node name (one custom certificate per node).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Identifier of the certificate resource; equals the `node` name.",
			},
			"node": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The cluster node name.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"certificates_pem": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "PEM encoded certificate (chain) to deploy as the node web certificate. Changing this value re-uploads the chain.",
			},
			"private_key": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "PEM encoded private key matching `certificates_pem`, stored as `/etc/pve/local/pveproxy-ssl.key`. Omit only when refreshing a chain whose key is already deployed.",
			},
			"force": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Overwrite an existing custom or ACME certificate on upload. PVE default: `false`.",
			},
			"restart": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Restart `pveproxy` after the upload or removal so the new certificate is served immediately. PVE default: `false`.",
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

// Configure implements resource.ResourceWithConfigure.
func (r *pveNodeCertificateResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*pveclient.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *pveclient.Client, got: %T.", req.ProviderData),
		)
		return
	}
	r.client = client
}

// Create implements resource.Resource.
func (r *pveNodeCertificateResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveNodeCertificateResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.upload(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Error creating pve_node_certificate", fmt.Sprintf("uploading custom certificate on %s: %s", plan.Node.ValueString(), err))
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Error reading pve_node_certificate after create", fmt.Sprintf("reading custom certificate on %s: %s", plan.Node.ValueString(), err))
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read implements resource.Resource.
func (r *pveNodeCertificateResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveNodeCertificateResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.readInto(ctx, &state); err != nil {
		if isPVEClientNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading pve_node_certificate", fmt.Sprintf("reading custom certificate on %s: %s", state.Node.ValueString(), err))
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource. The upload endpoint is documented as
// "upload or update", so a changed chain or key re-POSTs the same payload.
func (r *pveNodeCertificateResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveNodeCertificateResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.upload(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Error updating pve_node_certificate", fmt.Sprintf("uploading custom certificate on %s: %s", plan.Node.ValueString(), err))
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Error reading pve_node_certificate after update", fmt.Sprintf("reading custom certificate on %s: %s", plan.Node.ValueString(), err))
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete implements resource.Resource. Removing an already-absent custom
// certificate is treated as success.
func (r *pveNodeCertificateResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveNodeCertificateResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.DeleteNodeCustomCertificate(ctx, state.Node.ValueString(), state.Restart.ValueBool())
	if err != nil && !isPVEClientNotFound(err) {
		resp.Diagnostics.AddError("Error deleting pve_node_certificate", fmt.Sprintf("deleting custom certificate on %s: %s", state.Node.ValueString(), err))
		return
	}
}

// ImportState parses an import ID of the form `<node>` (a node carries at
// most one custom certificate).
func (r *pveNodeCertificateResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		resp.Diagnostics.AddError(
			"Invalid pve_node_certificate import ID",
			fmt.Sprintf("Import ID must be the node name, got %q.", req.ID),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("node"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

// upload POSTs the configured chain and key. The endpoint is synchronous
// per the pin, so no WaitForTask round-trip follows.
func (r *pveNodeCertificateResource) upload(ctx context.Context, m *pveNodeCertificateResourceModel) error {
	in := pveclient.UploadNodeCustomCertificateInput{
		Certificates: m.CertificatesP.ValueString(),
		Key:          m.PrivateKey.ValueString(),
		Force:        m.Force.ValueBool(),
		Restart:      m.Restart.ValueBool(),
	}
	_, err := r.client.UploadNodeCustomCertificate(ctx, m.Node.ValueString(), in)
	return err
}

// readInto refreshes the computed certificate fields from
// /nodes/{node}/certificates/info. The deployed row is located by stored
// fingerprint, falling back to the pveproxy-ssl.pem entry; a missing row is
// reported as a 404 so callers drop the resource from state.
func (r *pveNodeCertificateResource) readInto(ctx context.Context, m *pveNodeCertificateResourceModel) error {
	rows, err := r.client.GetNodeCertificates(ctx, m.Node.ValueString())
	if err != nil {
		return err
	}
	row := nodeCertificateFindRow(rows, m.Fingerprint.ValueString())
	if row == nil {
		return &pveclient.APIError{
			StatusCode: 404,
			Method:     "GET",
			Path:       fmt.Sprintf("/nodes/%s/certificates/info", m.Node.ValueString()),
		}
	}
	nodeCertificateApplyRow(m, row)
	m.SAN = listStringToTF(row.SAN)
	m.ID = m.Node
	return nil
}

// nodeCertificateFindRow locates the custom certificate row by fingerprint,
// or by filename when no fingerprint is known yet (import path).
func nodeCertificateFindRow(rows []pveclient.NodeCertificateInfo, fingerprint string) *pveclient.NodeCertificateInfo {
	for i := range rows {
		if fingerprint != "" && rows[i].Fingerprint == fingerprint {
			return &rows[i]
		}
	}
	if fingerprint != "" {
		return nil
	}
	for i := range rows {
		if rows[i].Filename == nodeCertificateCustomFilename {
			return &rows[i]
		}
	}
	return nil
}

// nodeCertificateApplyRow copies an info row onto the model's computed
// fields, keeping absent fields null.
func nodeCertificateApplyRow(m *pveNodeCertificateResourceModel, row *pveclient.NodeCertificateInfo) {
	m.Fingerprint = nodeCertificateStringOrNull(row.Fingerprint)
	m.Issuer = nodeCertificateStringOrNull(row.Issuer)
	m.Subject = nodeCertificateStringOrNull(row.Subject)
	m.PublicKeyType = nodeCertificateStringOrNull(row.PublicKeyType)
	m.NotAfter = nodeCertificateInt64OrNull(row.NotAfter)
	m.NotBefore = nodeCertificateInt64OrNull(row.NotBefore)
	m.PublicKeyBits = nodeCertificateInt64OrNull(row.PublicKeyBits)
}

// nodeCertificateStringOrNull maps an absent info field to a null string.
func nodeCertificateStringOrNull(v string) types.String {
	if v == "" {
		return types.StringNull()
	}
	return types.StringValue(v)
}

// nodeCertificateInt64OrNull maps an absent info field to a null integer.
func nodeCertificateInt64OrNull(v *int64) types.Int64 {
	if v == nil {
		return types.Int64Null()
	}
	return types.Int64Value(*v)
}
