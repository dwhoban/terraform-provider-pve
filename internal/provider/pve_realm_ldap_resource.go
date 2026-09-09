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
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ resource.Resource                = &pveRealmLdapResource{}
	_ resource.ResourceWithConfigure   = &pveRealmLdapResource{}
	_ resource.ResourceWithImportState = &pveRealmLdapResource{}
)

// NewPveRealmLdapResource returns the resource implementation.
func NewPveRealmLdapResource() resource.Resource {
	return &pveRealmLdapResource{}
}

// pveRealmLdapResource manages an LDAP authentication realm
// (type=ldap) via /access/domains.
type pveRealmLdapResource struct {
	client *pveclient.Client
}

// pveRealmLdapResourceModel is the Terraform-facing shape.
type pveRealmLdapResourceModel struct {
	realmCommonModel
	realmTransportModel
	realmLdapModel
}

// Metadata implements resource.Resource.
func (r *pveRealmLdapResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveRealmLdap
}

// Schema implements resource.Resource.
func (r *pveRealmLdapResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages an LDAP authentication realm (`type=ldap`) in Proxmox VE (`/access/domains`). The upstream server validates that the supplied options fit the realm type.",
		Attributes:          realmLDAPAttributes(false, true),
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveRealmLdapResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = realmConfigureResource(req, resp)
}

// Create implements resource.Resource.
func (r *pveRealmLdapResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveRealmLdapResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := pveclient.Domain{Realm: plan.Realm.ValueString(), Type: realmTypeLDAP}
	realmCommonApply(plan.realmCommonModel, realmCommonModel{}, &body)
	realmTransportApply(plan.realmTransportModel, realmTransportModel{}, &body)
	realmLDAPApply(plan.realmLdapModel, realmLdapModel{}, &body)
	tflog.Debug(ctx, "creating LDAP realm", map[string]any{"realm": plan.Realm.ValueString()})
	if err := r.client.CreateDomain(ctx, body); err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_realm_ldap",
			fmt.Sprintf("creating realm %s: %s", plan.Realm.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_realm_ldap after create",
			fmt.Sprintf("reading realm %s: %s", plan.Realm.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read implements resource.Resource.
func (r *pveRealmLdapResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveRealmLdapResourceModel
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
			"Error reading pve_realm_ldap",
			fmt.Sprintf("reading realm %s: %s", state.Realm.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource.
func (r *pveRealmLdapResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveRealmLdapResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state pveRealmLdapResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := pveclient.Domain{Realm: plan.Realm.ValueString(), Type: realmTypeLDAP}
	deleteFields := realmCommonApply(plan.realmCommonModel, state.realmCommonModel, &body)
	deleteFields = append(deleteFields, realmTransportApply(plan.realmTransportModel, state.realmTransportModel, &body)...)
	deleteFields = append(deleteFields, realmLDAPApply(plan.realmLdapModel, state.realmLdapModel, &body)...)
	body.Delete = strings.Join(deleteFields, ",")
	tflog.Debug(ctx, "updating LDAP realm", map[string]any{"realm": plan.Realm.ValueString()})
	if err := r.client.UpdateDomain(ctx, plan.Realm.ValueString(), body); err != nil {
		resp.Diagnostics.AddError(
			"Error updating pve_realm_ldap",
			fmt.Sprintf("updating realm %s: %s", plan.Realm.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_realm_ldap after update",
			fmt.Sprintf("reading realm %s: %s", plan.Realm.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete implements resource.Resource.
func (r *pveRealmLdapResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveRealmLdapResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	tflog.Debug(ctx, "deleting LDAP realm", map[string]any{"realm": state.Realm.ValueString()})
	if err := r.client.DeleteDomain(ctx, state.Realm.ValueString()); err != nil {
		if isPVEClientNotFound(err) {
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting pve_realm_ldap",
			fmt.Sprintf("deleting realm %s: %s", state.Realm.ValueString(), err),
		)
		return
	}
}

// ImportState parses an import ID of the form `<realm>`.
func (r *pveRealmLdapResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("realm"), req.ID)...)
}

// readInto populates the model from PVE; an upstream type mismatch is an
// error rather than a silent adoption.
func (r *pveRealmLdapResource) readInto(ctx context.Context, m *pveRealmLdapResourceModel) error {
	domain, err := realmGetChecked(ctx, r.client, m.Realm.ValueString(), realmTypeLDAP)
	if err != nil {
		return err
	}
	realmCommonIntoModel(&m.realmCommonModel, domain)
	realmTransportIntoModel(&m.realmTransportModel, domain)
	realmLDAPIntoModel(&m.realmLdapModel, domain)
	return nil
}
