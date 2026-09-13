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
	_ resource.Resource                = &pveRealmOpenidResource{}
	_ resource.ResourceWithConfigure   = &pveRealmOpenidResource{}
	_ resource.ResourceWithImportState = &pveRealmOpenidResource{}
)

// NewPveRealmOpenidResource returns the resource implementation.
func NewPveRealmOpenidResource() resource.Resource {
	return &pveRealmOpenidResource{}
}

// pveRealmOpenidResource manages an OpenID Connect authentication realm
// (type=openid) via /access/domains.
type pveRealmOpenidResource struct {
	client *pveclient.Client
}

// pveRealmOpenidResourceModel is the Terraform-facing shape.
type pveRealmOpenidResourceModel struct {
	realmCommonModel
	realmOpenidModel
}

// Metadata implements resource.Resource.
func (r *pveRealmOpenidResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveRealmOpenid
}

// Schema implements resource.Resource.
func (r *pveRealmOpenidResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages an OpenID Connect authentication realm (`type=openid`) in Proxmox VE (`/access/domains`). The upstream server validates that the supplied options fit the realm type.",
		Attributes:          realmOpenidAttributes(false, true),
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveRealmOpenidResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = realmConfigureResource(req, resp)
}

// Create implements resource.Resource.
func (r *pveRealmOpenidResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveRealmOpenidResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := pveclient.Domain{Realm: plan.Realm.ValueString(), Type: realmTypeOpenid}
	realmCommonApply(plan.realmCommonModel, realmCommonModel{}, &body)
	realmOpenidApply(plan.realmOpenidModel, realmOpenidModel{}, &body)
	tflog.Debug(ctx, "creating OpenID realm", map[string]any{"realm": plan.Realm.ValueString()})
	if err := r.client.CreateDomain(ctx, body); err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_realm_openid",
			fmt.Sprintf("creating realm %s: %s", plan.Realm.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_realm_openid after create",
			fmt.Sprintf("reading realm %s: %s", plan.Realm.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read implements resource.Resource.
func (r *pveRealmOpenidResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveRealmOpenidResourceModel
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
			"Error reading pve_realm_openid",
			fmt.Sprintf("reading realm %s: %s", state.Realm.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource.
func (r *pveRealmOpenidResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveRealmOpenidResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state pveRealmOpenidResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := pveclient.Domain{Realm: plan.Realm.ValueString(), Type: realmTypeOpenid}
	deleteFields := realmCommonApply(plan.realmCommonModel, state.realmCommonModel, &body)
	deleteFields = append(deleteFields, realmOpenidApply(plan.realmOpenidModel, state.realmOpenidModel, &body)...)
	body.Delete = strings.Join(deleteFields, ",")
	tflog.Debug(ctx, "updating OpenID realm", map[string]any{"realm": plan.Realm.ValueString()})
	if err := r.client.UpdateDomain(ctx, plan.Realm.ValueString(), body); err != nil {
		resp.Diagnostics.AddError(
			"Error updating pve_realm_openid",
			fmt.Sprintf("updating realm %s: %s", plan.Realm.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_realm_openid after update",
			fmt.Sprintf("reading realm %s: %s", plan.Realm.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete implements resource.Resource.
func (r *pveRealmOpenidResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveRealmOpenidResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	tflog.Debug(ctx, "deleting OpenID realm", map[string]any{"realm": state.Realm.ValueString()})
	if err := r.client.DeleteDomain(ctx, state.Realm.ValueString()); err != nil {
		if isPVEClientNotFound(err) {
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting pve_realm_openid",
			fmt.Sprintf("deleting realm %s: %s", state.Realm.ValueString(), err),
		)
		return
	}
}

// ImportState parses an import ID of the form `<realm>`.
func (r *pveRealmOpenidResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("realm"), req.ID)...)
}

// readInto populates the model from PVE; an upstream type mismatch is an
// error rather than a silent adoption.
func (r *pveRealmOpenidResource) readInto(ctx context.Context, m *pveRealmOpenidResourceModel) error {
	domain, err := realmGetChecked(ctx, r.client, m.Realm.ValueString(), realmTypeOpenid)
	if err != nil {
		return err
	}
	realmCommonIntoModel(&m.realmCommonModel, domain)
	realmOpenidIntoModel(&m.realmOpenidModel, domain)
	return nil
}
