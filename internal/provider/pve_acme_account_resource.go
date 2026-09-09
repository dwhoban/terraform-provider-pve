// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ resource.Resource                = &pveAcmeAccountResource{}
	_ resource.ResourceWithConfigure   = &pveAcmeAccountResource{}
	_ resource.ResourceWithImportState = &pveAcmeAccountResource{}
)

// NewPveAcmeAccountResource returns the resource implementation.
func NewPveAcmeAccountResource() resource.Resource {
	return &pveAcmeAccountResource{}
}

// pveAcmeAccountResource manages an ACME account registration
// (/cluster/acme/account). Register, refresh, and deactivate are async per
// the pin (all three return a task UPID), so every mutation waits for its
// task before re-reading.
type pveAcmeAccountResource struct {
	client *pveclient.Client
}

// pveAcmeAccountResourceModel is the Terraform-facing shape.
type pveAcmeAccountResourceModel struct {
	Name       types.String `tfsdk:"name"`
	Contact    types.List   `tfsdk:"contact"`
	Directory  types.String `tfsdk:"directory"`
	TosURL     types.String `tfsdk:"tos_url"`
	EabKid     types.String `tfsdk:"eab_kid"`
	EabHmacKey types.String `tfsdk:"eab_hmac_key"`
	AccountURL types.String `tfsdk:"account_url"`
	Tos        types.String `tfsdk:"tos"`
}

// Metadata implements resource.Resource.
func (r *pveAcmeAccountResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveAcmeAccount
}

// Schema implements resource.Resource.
func (r *pveAcmeAccountResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Registers an ACME account with a CA (`POST /cluster/acme/account` and `GET/PUT/DELETE /cluster/acme/account/{name}`). Registration, contact refresh, and deactivation run as async tasks; the provider waits for each to finish. Deleting this resource deactivates the account at the CA per the pin's DELETE verb.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "ACME account config file name (PVE `pve-configid` format; the API defaults to `default`). Changing this value forces recreation.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"contact": schema.ListAttribute{
				Required:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Contact email addresses, sent as PVE's comma-separated email-list; entries are usually written as `mailto:user@example.com` URIs, which is how PVE stores them.",
				PlanModifiers: []planmodifier.List{
					listplanmodifier.RequiresReplace(),
				},
			},
			"directory": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "URL of the ACME CA directory endpoint (PVE default: `https://acme-v02.api.letsencrypt.org/directory`). Refreshed from the CA response on read.",
			},
			"tos_url": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "URL of the CA Terms of Service; setting it indicates agreement. Write-only per the pin (the account read returns the CA-reported `tos` instead).",
			},
			"eab_kid": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Key identifier for External Account Binding. The pin requires it to be supplied together with `eab_hmac_key`.",
			},
			"eab_hmac_key": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "HMAC key for External Account Binding. The pin requires it to be supplied together with `eab_kid`.",
			},
			"account_url": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The CA account URL as reported by the `location` field of the account read.",
			},
			"tos": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The CA's Terms of Service URL as reported by the account read.",
			},
		},
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveAcmeAccountResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = haConfigureResource(req, resp)
}

// Create implements resource.Resource.
func (r *pveAcmeAccountResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveAcmeAccountResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error creating pve_acme_account", "provider client is not configured")
		return
	}
	upid, err := r.client.RegisterAcmeAccount(ctx, acmeAccountRegistrationFromModel(plan))
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_acme_account",
			fmt.Sprintf("registering ACME account %s: %s", plan.Name.ValueString(), err),
		)
		return
	}
	if err := r.waitAcmeTask(ctx, upid); err != nil {
		resp.Diagnostics.AddError(
			"Error waiting for pve_acme_account register task",
			fmt.Sprintf("registration of ACME account %s did not complete: %s", plan.Name.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_acme_account after create",
			fmt.Sprintf("reading ACME account %s: %s", plan.Name.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read implements resource.Resource.
func (r *pveAcmeAccountResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveAcmeAccountResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.readInto(ctx, &state); err != nil {
		if isPVEClientNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error reading pve_acme_account",
			fmt.Sprintf("reading ACME account %s: %s", state.Name.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource. The pin's update verb only accepts
// a new contact list (an empty body would trigger a CA refresh).
func (r *pveAcmeAccountResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveAcmeAccountResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error updating pve_acme_account", "provider client is not configured")
		return
	}
	upid, err := r.client.UpdateAcmeAccount(ctx, plan.Name.ValueString(), listStringFromTF(plan.Contact))
	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating pve_acme_account",
			fmt.Sprintf("updating ACME account %s: %s", plan.Name.ValueString(), err),
		)
		return
	}
	if err := r.waitAcmeTask(ctx, upid); err != nil {
		resp.Diagnostics.AddError(
			"Error waiting for pve_acme_account update task",
			fmt.Sprintf("contact refresh of ACME account %s did not complete: %s", plan.Name.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_acme_account after update",
			fmt.Sprintf("reading ACME account %s: %s", plan.Name.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete implements resource.Resource. Deactivation is async per the pin;
// an already-absent account counts as deleted.
func (r *pveAcmeAccountResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveAcmeAccountResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	upid, err := r.client.DeactivateAcmeAccount(ctx, state.Name.ValueString())
	if err != nil {
		if isPVEClientNotFound(err) {
			// Already absent counts as deleted.
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting pve_acme_account",
			fmt.Sprintf("deactivating ACME account %s: %s", state.Name.ValueString(), err),
		)
		return
	}
	if err := r.waitAcmeTask(ctx, upid); err != nil {
		resp.Diagnostics.AddError(
			"Error waiting for pve_acme_account deactivate task",
			fmt.Sprintf("deactivation of ACME account %s did not complete: %s", state.Name.ValueString(), err),
		)
	}
}

// ImportState parses an import ID of the form `<name>`.
func (r *pveAcmeAccountResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		resp.Diagnostics.AddError("Invalid pve_acme_account import ID", "import ID must be the ACME account name, e.g. `default`")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), req.ID)...)
}

// readInto refreshes the computed fields from PVE. The pin's account read
// does not echo the contact list or the write-only EAB and ToS inputs, so
// those keep their configured values.
func (r *pveAcmeAccountResource) readInto(ctx context.Context, m *pveAcmeAccountResourceModel) error {
	acct, err := r.client.GetAcmeAccount(ctx, m.Name.ValueString())
	if err != nil {
		return err
	}
	m.Directory = nodeNetworkStringToTF(acct.Directory)
	m.AccountURL = nodeNetworkStringToTF(acct.Location)
	m.Tos = nodeNetworkStringToTF(acct.Tos)
	return nil
}

// waitAcmeTask waits for an async account task, deriving the node from the
// UPID (ACME account calls are cluster-scoped and PVE picks the node).
func (r *pveAcmeAccountResource) waitAcmeTask(ctx context.Context, upid string) error {
	if !strings.HasPrefix(upid, "UPID:") {
		return nil
	}
	parts := strings.Split(upid, ":")
	if len(parts) < 2 || parts[1] == "" {
		return fmt.Errorf("cannot determine node from UPID %q", upid)
	}
	_, err := r.client.WaitForTask(ctx, parts[1], upid, pveclient.WaitForTaskOptions{})
	return err
}

// acmeAccountRegistrationFromModel projects the Terraform model into the
// registration request body.
func acmeAccountRegistrationFromModel(m pveAcmeAccountResourceModel) pveclient.AcmeAccountRegistration {
	reg := pveclient.AcmeAccountRegistration{
		Name:    m.Name.ValueString(),
		Contact: listStringFromTF(m.Contact),
	}
	if !m.Directory.IsNull() && !m.Directory.IsUnknown() {
		reg.Directory = m.Directory.ValueString()
	}
	if !m.TosURL.IsNull() && !m.TosURL.IsUnknown() {
		reg.TosURL = m.TosURL.ValueString()
	}
	if !m.EabKid.IsNull() && !m.EabKid.IsUnknown() {
		reg.EabKid = m.EabKid.ValueString()
	}
	if !m.EabHmacKey.IsNull() && !m.EabHmacKey.IsUnknown() {
		reg.EabHMACKey = m.EabHmacKey.ValueString()
	}
	return reg
}
