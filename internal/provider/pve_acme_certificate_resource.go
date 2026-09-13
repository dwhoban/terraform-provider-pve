// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ resource.Resource                = &pveAcmeCertificateResource{}
	_ resource.ResourceWithConfigure   = &pveAcmeCertificateResource{}
	_ resource.ResourceWithImportState = &pveAcmeCertificateResource{}
)

// NewPveAcmeCertificateResource returns the resource implementation.
func NewPveAcmeCertificateResource() resource.Resource {
	return &pveAcmeCertificateResource{}
}

// pveAcmeCertificateResource manages a node's ACME-ordered certificate
// (POST/PUT/DELETE /nodes/{node}/certificates/acme/certificate). The order
// verbs return task UPIDs, so every mutation waits on the ACME task.
type pveAcmeCertificateResource struct {
	client *pveclient.Client
}

// pveAcmeCertificateResourceModel is the Terraform-facing shape.
type pveAcmeCertificateResourceModel struct {
	ID            types.String   `tfsdk:"id"`
	Node          types.String   `tfsdk:"node"`
	Domains       []types.String `tfsdk:"domains"`
	Aliases       types.Map      `tfsdk:"aliases"`
	Force         types.Bool     `tfsdk:"force"`
	Fingerprint   types.String   `tfsdk:"fingerprint"`
	Issuer        types.String   `tfsdk:"issuer"`
	Subject       types.String   `tfsdk:"subject"`
	NotAfter      types.Int64    `tfsdk:"not_after"`
	NotBefore     types.Int64    `tfsdk:"not_before"`
	SAN           types.List     `tfsdk:"san"`
	PublicKeyType types.String   `tfsdk:"public_key_type"`
	PublicKeyBits types.Int64    `tfsdk:"public_key_bits"`
}

// Metadata implements resource.Resource.
func (r *pveAcmeCertificateResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveAcmeCertificate
}

// Schema implements resource.Resource.
func (r *pveAcmeCertificateResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a node's ACME-ordered TLS certificate (`POST`/`PUT`/`DELETE /nodes/{node}/certificates/acme/certificate`). Create writes the domains into the node's `acme` configuration entry (`PUT /nodes/{node}/config`), orders the certificate, and waits for the ACME task to finish. Updates rewrite the configuration and force a renewal so domain changes take effect immediately. Delete revokes the certificate and then clears the node's `acme` entry. Ordering requires an ACME account (see `pve_acme_account`) and, for DNS challenges, a DNS plugin (see `pve_acme_dns_plugin`) in the cluster ACME configuration. This resource owns the node's `acme` entry — do not manage the same fields through the node configuration concurrently. PVE renews the certificate automatically near expiry; force an early renewal by tainting the resource or changing any attribute. Import ID: the node name (one ACME certificate per node).",
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
			"domains": schema.ListAttribute{
				Required:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Domains to order the certificate for. The first entry becomes the certificate subject, the remaining entries are SubjectAlternativeNames. Written to the node's `acme` configuration before ordering.",
				Validators: []validator.List{
					listvalidator.SizeAtLeast(1),
				},
			},
			"aliases": schema.MapAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Map of domain (a `domains` entry) to the DNS alias used for that domain's ACME challenge. Stored as the `alias` of the domain's node configuration entry.",
			},
			"force": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Overwrite an existing custom certificate when ordering. PVE default: `false`.",
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
func (r *pveAcmeCertificateResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
func (r *pveAcmeCertificateResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveAcmeCertificateResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	node := plan.Node.ValueString()
	if err := acmeCertificateSyncNodeConfig(ctx, r.client, node, stringsFromTF(plan.Domains), acmeCertificateAliasesFromTF(plan.Aliases)); err != nil {
		resp.Diagnostics.AddError("Error creating pve_acme_certificate", fmt.Sprintf("writing acme node config on %s: %s", node, err))
		return
	}
	upid, err := r.client.OrderAcmeCertificate(ctx, node, plan.Force.ValueBool())
	if err != nil {
		resp.Diagnostics.AddError("Error creating pve_acme_certificate", fmt.Sprintf("ordering certificate on %s: %s", node, err))
		return
	}
	if _, err := r.client.WaitForTask(ctx, node, upid, defaultWaitOptions()); err != nil {
		resp.Diagnostics.AddError("Error waiting for pve_acme_certificate order", fmt.Sprintf("waiting for certificate order on %s: %s", node, err))
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Error reading pve_acme_certificate after create", fmt.Sprintf("reading certificate on %s: %s", node, err))
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read implements resource.Resource.
func (r *pveAcmeCertificateResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveAcmeCertificateResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.readInto(ctx, &state); err != nil {
		if isPVEClientNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading pve_acme_certificate", fmt.Sprintf("reading certificate on %s: %s", state.Node.ValueString(), err))
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource. Renewal (PUT) re-issues against the
// freshly written node configuration; force is set so domain changes are
// applied even when the current certificate is still young.
func (r *pveAcmeCertificateResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveAcmeCertificateResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	node := plan.Node.ValueString()
	if err := acmeCertificateSyncNodeConfig(ctx, r.client, node, stringsFromTF(plan.Domains), acmeCertificateAliasesFromTF(plan.Aliases)); err != nil {
		resp.Diagnostics.AddError("Error updating pve_acme_certificate", fmt.Sprintf("writing acme node config on %s: %s", node, err))
		return
	}
	upid, err := r.client.RenewAcmeCertificate(ctx, node, true)
	if err != nil {
		resp.Diagnostics.AddError("Error updating pve_acme_certificate", fmt.Sprintf("renewing certificate on %s: %s", node, err))
		return
	}
	if _, err := r.client.WaitForTask(ctx, node, upid, defaultWaitOptions()); err != nil {
		resp.Diagnostics.AddError("Error waiting for pve_acme_certificate renewal", fmt.Sprintf("waiting for certificate renewal on %s: %s", node, err))
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Error reading pve_acme_certificate after update", fmt.Sprintf("reading certificate on %s: %s", node, err))
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete implements resource.Resource. The pin defines a revoke verb, so
// delete revokes the certificate (best effort) and clears the node's `acme`
// entry.
func (r *pveAcmeCertificateResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveAcmeCertificateResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	node := state.Node.ValueString()
	upid, err := r.client.RevokeAcmeCertificate(ctx, node)
	switch {
	case isPVEClientNotFound(err):
		// Nothing deployed; fall through to the config cleanup.
	case err != nil:
		resp.Diagnostics.AddError("Error deleting pve_acme_certificate", fmt.Sprintf("revoking certificate on %s: %s", node, err))
		return
	default:
		if _, err := r.client.WaitForTask(ctx, node, upid, defaultWaitOptions()); err != nil {
			resp.Diagnostics.AddError("Error waiting for pve_acme_certificate revocation", fmt.Sprintf("waiting for certificate revocation on %s: %s", node, err))
			return
		}
	}
	// Clearing the node config is best-effort cleanup: the certificate is
	// already revoked at this point, so a config failure only warns.
	if err := acmeCertificateSyncNodeConfig(ctx, r.client, node, nil, nil); err != nil {
		resp.Diagnostics.AddWarning(
			"pve_acme_certificate node config cleanup failed",
			fmt.Sprintf("clearing the acme entry from the node config on %s: %s", node, err),
		)
	}
}

// ImportState parses an import ID of the form `<node>` (a node carries at
// most one ACME certificate).
func (r *pveAcmeCertificateResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		resp.Diagnostics.AddError(
			"Invalid pve_acme_certificate import ID",
			fmt.Sprintf("Import ID must be the node name, got %q.", req.ID),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("node"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

// readInto refreshes the computed certificate fields from
// /nodes/{node}/certificates/info. The deployed row is located by stored
// fingerprint, then by SAN/domain, then by the pveproxy-ssl.pem entry; a
// missing row is reported as a 404 so callers drop the resource from state.
func (r *pveAcmeCertificateResource) readInto(ctx context.Context, m *pveAcmeCertificateResourceModel) error {
	node := m.Node.ValueString()
	rows, err := r.client.GetNodeCertificates(ctx, node)
	if err != nil {
		return err
	}
	row := acmeCertificateFindRow(rows, m.Fingerprint.ValueString(), acmeCertificatePrimaryDomain(stringsFromTF(m.Domains)))
	if row == nil {
		return &pveclient.APIError{
			StatusCode: 404,
			Method:     "GET",
			Path:       fmt.Sprintf("/nodes/%s/certificates/info", node),
		}
	}
	m.Fingerprint = nodeCertificateStringOrNull(row.Fingerprint)
	m.Issuer = nodeCertificateStringOrNull(row.Issuer)
	m.Subject = nodeCertificateStringOrNull(row.Subject)
	m.PublicKeyType = nodeCertificateStringOrNull(row.PublicKeyType)
	m.NotAfter = nodeCertificateInt64OrNull(row.NotAfter)
	m.NotBefore = nodeCertificateInt64OrNull(row.NotBefore)
	m.PublicKeyBits = nodeCertificateInt64OrNull(row.PublicKeyBits)
	m.SAN = listStringToTF(row.SAN)
	m.ID = m.Node
	return nil
}

// acmeCertificateSyncNodeConfig rewrites the node's `acme` entry with the
// supplied domains and aliases (nil clears the entry). The configuration is
// read first so its digest and unrelated fields round-trip untouched.
func acmeCertificateSyncNodeConfig(ctx context.Context, client *pveclient.Client, node string, domains []string, aliases map[string]string) error {
	cfg, err := client.GetNodeConfig(ctx, node)
	if err != nil {
		return fmt.Errorf("reading node config: %w", err)
	}
	cfg.ACMEDomains = acmeCertificateBuildDomains(domains, aliases)
	if err := client.UpdateNodeConfig(ctx, node, *cfg); err != nil {
		return fmt.Errorf("writing node config: %w", err)
	}
	return nil
}

// acmeCertificateBuildDomains maps domains and their optional aliases onto
// the node config's ACME registration rows.
func acmeCertificateBuildDomains(domains []string, aliases map[string]string) []pveclient.NodeACMEDomain {
	if len(domains) == 0 {
		return nil
	}
	out := make([]pveclient.NodeACMEDomain, 0, len(domains))
	for _, d := range domains {
		row := pveclient.NodeACMEDomain{Domain: d}
		if alias, ok := aliases[d]; ok {
			row.Alias = []string{alias}
		}
		out = append(out, row)
	}
	return out
}

// acmeCertificateAliasesFromTF flattens the aliases map; null and empty
// maps both yield nil.
func acmeCertificateAliasesFromTF(in types.Map) map[string]string {
	if in.IsNull() || in.IsUnknown() {
		return nil
	}
	out := make(map[string]string, len(in.Elements()))
	for k, v := range in.Elements() {
		sv, ok := v.(types.String)
		if !ok || sv.IsNull() || sv.IsUnknown() {
			continue
		}
		out[k] = sv.ValueString()
	}
	return out
}

// acmeCertificatePrimaryDomain returns the first configured domain (the
// certificate subject) or an empty string.
func acmeCertificatePrimaryDomain(domains []string) string {
	if len(domains) == 0 {
		return ""
	}
	return domains[0]
}

// acmeCertificateFindRow locates the deployed ACME certificate: first by
// stored fingerprint, then by a SAN match on the primary domain, then by
// the pveproxy-ssl.pem entry.
func acmeCertificateFindRow(rows []pveclient.NodeCertificateInfo, fingerprint, domain string) *pveclient.NodeCertificateInfo {
	for i := range rows {
		if fingerprint != "" && rows[i].Fingerprint == fingerprint {
			return &rows[i]
		}
	}
	if fingerprint != "" {
		return nil
	}
	for i := range rows {
		for _, san := range rows[i].SAN {
			if domain != "" && san == domain {
				return &rows[i]
			}
		}
	}
	for i := range rows {
		if rows[i].Filename == nodeCertificateCustomFilename {
			return &rows[i]
		}
	}
	return nil
}
