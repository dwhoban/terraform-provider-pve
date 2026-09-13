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

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ resource.Resource                = &pveRealmAdResource{}
	_ resource.ResourceWithConfigure   = &pveRealmAdResource{}
	_ resource.ResourceWithImportState = &pveRealmAdResource{}
)

// NewPveRealmAdResource returns the resource implementation.
func NewPveRealmAdResource() resource.Resource {
	return &pveRealmAdResource{}
}

// pveRealmAdResource manages an Active Directory authentication realm
// (type=ad) via /access/domains.
type pveRealmAdResource struct {
	client *pveclient.Client
}

// pveRealmAdResourceModel is the Terraform-facing shape.
type pveRealmAdResourceModel struct {
	realmCommonModel
	realmTransportModel
	realmAdModel
}

// Metadata implements resource.Resource.
func (r *pveRealmAdResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveRealmAd
}

// Schema implements resource.Resource.
func (r *pveRealmAdResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages an Active Directory authentication realm (`type=ad`) in Proxmox VE (`/access/domains`). The upstream server validates that the supplied options fit the realm type.",
		Attributes:          realmADAttributes(false, true),
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveRealmAdResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = realmConfigureResource(req, resp)
}

// Create implements resource.Resource.
func (r *pveRealmAdResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveRealmAdResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := pveclient.Domain{Realm: plan.Realm.ValueString(), Type: realmTypeAD}
	realmCommonApply(plan.realmCommonModel, realmCommonModel{}, &body)
	realmTransportApply(plan.realmTransportModel, realmTransportModel{}, &body)
	realmADApply(plan.realmAdModel, realmAdModel{}, &body)
	tflog.Debug(ctx, "creating AD realm", map[string]any{"realm": plan.Realm.ValueString()})
	if err := r.client.CreateDomain(ctx, body); err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_realm_ad",
			fmt.Sprintf("creating realm %s: %s", plan.Realm.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_realm_ad after create",
			fmt.Sprintf("reading realm %s: %s", plan.Realm.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read implements resource.Resource.
func (r *pveRealmAdResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveRealmAdResourceModel
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
			"Error reading pve_realm_ad",
			fmt.Sprintf("reading realm %s: %s", state.Realm.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource.
func (r *pveRealmAdResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveRealmAdResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state pveRealmAdResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := pveclient.Domain{Realm: plan.Realm.ValueString(), Type: realmTypeAD}
	deleteFields := realmCommonApply(plan.realmCommonModel, state.realmCommonModel, &body)
	deleteFields = append(deleteFields, realmTransportApply(plan.realmTransportModel, state.realmTransportModel, &body)...)
	deleteFields = append(deleteFields, realmADApply(plan.realmAdModel, state.realmAdModel, &body)...)
	body.Delete = strings.Join(deleteFields, ",")
	tflog.Debug(ctx, "updating AD realm", map[string]any{"realm": plan.Realm.ValueString()})
	if err := r.client.UpdateDomain(ctx, plan.Realm.ValueString(), body); err != nil {
		resp.Diagnostics.AddError(
			"Error updating pve_realm_ad",
			fmt.Sprintf("updating realm %s: %s", plan.Realm.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_realm_ad after update",
			fmt.Sprintf("reading realm %s: %s", plan.Realm.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete implements resource.Resource.
func (r *pveRealmAdResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveRealmAdResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	tflog.Debug(ctx, "deleting AD realm", map[string]any{"realm": state.Realm.ValueString()})
	if err := r.client.DeleteDomain(ctx, state.Realm.ValueString()); err != nil {
		if isPVEClientNotFound(err) {
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting pve_realm_ad",
			fmt.Sprintf("deleting realm %s: %s", state.Realm.ValueString(), err),
		)
		return
	}
}

// ImportState parses an import ID of the form `<realm>`.
func (r *pveRealmAdResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("realm"), req.ID)...)
}

// readInto populates the model from PVE; an upstream type mismatch is an
// error rather than a silent adoption.
func (r *pveRealmAdResource) readInto(ctx context.Context, m *pveRealmAdResourceModel) error {
	domain, err := realmGetChecked(ctx, r.client, m.Realm.ValueString(), realmTypeAD)
	if err != nil {
		return err
	}
	realmCommonIntoModel(&m.realmCommonModel, domain)
	realmTransportIntoModel(&m.realmTransportModel, domain)
	realmADIntoModel(&m.realmAdModel, domain)
	return nil
}
