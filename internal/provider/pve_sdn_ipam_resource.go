// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// sdnIpamTypes enumerates the pin's closed IPAM plugin type enum.
var sdnIpamTypes = []string{"netbox", "phpipam", "pve"}

// Ensure framework interfaces are satisfied.
var (
	_ resource.Resource                = &pveSdnIpamResource{}
	_ resource.ResourceWithConfigure   = &pveSdnIpamResource{}
	_ resource.ResourceWithImportState = &pveSdnIpamResource{}
)

// NewPveSdnIpamResource returns the resource implementation.
func NewPveSdnIpamResource() resource.Resource {
	return &pveSdnIpamResource{}
}

// pveSdnIpamResource manages an SDN IPAM plugin object
// (/cluster/sdn/ipams). Every mutation is synchronous per the pin.
type pveSdnIpamResource struct {
	client *pveclient.Client
}

// pveSdnIpamResourceModel is the Terraform-facing shape.
type pveSdnIpamResourceModel struct {
	Ipam        types.String `tfsdk:"ipam"`
	Type        types.String `tfsdk:"type"`
	URL         types.String `tfsdk:"url"`
	Token       types.String `tfsdk:"token"`
	Fingerprint types.String `tfsdk:"fingerprint"`
	Section     types.Int64  `tfsdk:"section"`
}

// Metadata implements resource.Resource.
func (r *pveSdnIpamResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveSdnIpam
}

// Schema implements resource.Resource.
func (r *pveSdnIpamResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages an SDN IPAM plugin object (`/cluster/sdn/ipams`). The IPAM tracks IP address allocation for SDN zones and vnets. Changes stay pending until the `pve_sdn_apply` action runs.",
		Attributes: map[string]schema.Attribute{
			"ipam": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The SDN IPAM object identifier (2 or more characters, starting with a letter). Changing this value forces recreation.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"type": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "IPAM plugin type. Must be one of: `netbox`, `phpipam`, `pve`. Changing this value forces recreation.",
				Validators: []validator.String{
					stringvalidator.OneOf(sdnIpamTypes...),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"url": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "URL of the IPAM server (netbox and phpipam only).",
			},
			"token": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "API token for the IPAM server (netbox and phpipam only).",
			},
			"fingerprint": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Certificate SHA 256 fingerprint of the IPAM server, as 32 colon-separated hex pairs.",
			},
			"section": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "phpIPAM section identifier (phpipam only).",
			},
		},
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveSdnIpamResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = haConfigureResource(req, resp)
}

// Create implements resource.Resource.
func (r *pveSdnIpamResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveSdnIpamResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error creating pve_sdn_ipam", "provider client is not configured")
		return
	}
	if err := r.client.CreateSdnIpam(ctx, sdnIpamFromModel(plan)); err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_sdn_ipam",
			fmt.Sprintf("creating SDN ipam %s: %s", plan.Ipam.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_sdn_ipam after create",
			fmt.Sprintf("reading SDN ipam %s: %s", plan.Ipam.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read implements resource.Resource.
func (r *pveSdnIpamResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveSdnIpamResourceModel
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
			"Error reading pve_sdn_ipam",
			fmt.Sprintf("reading SDN ipam %s: %s", state.Ipam.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource.
func (r *pveSdnIpamResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveSdnIpamResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state pveSdnIpamResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error updating pve_sdn_ipam", "provider client is not configured")
		return
	}
	deleteFields := sdnIpamDeleteFields(plan, state)
	if err := r.client.UpdateSdnIpam(ctx, plan.Ipam.ValueString(), sdnIpamFromModel(plan), deleteFields); err != nil {
		resp.Diagnostics.AddError(
			"Error updating pve_sdn_ipam",
			fmt.Sprintf("updating SDN ipam %s: %s", plan.Ipam.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_sdn_ipam after update",
			fmt.Sprintf("reading SDN ipam %s: %s", plan.Ipam.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete implements resource.Resource.
func (r *pveSdnIpamResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveSdnIpamResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error deleting pve_sdn_ipam", "provider client is not configured")
		return
	}
	if err := r.client.DeleteSdnIpam(ctx, state.Ipam.ValueString()); err != nil {
		if isPVEClientNotFound(err) {
			// Already absent counts as deleted.
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting pve_sdn_ipam",
			fmt.Sprintf("deleting SDN ipam %s: %s", state.Ipam.ValueString(), err),
		)
	}
}

// ImportState parses an import ID of the form `<ipam>`.
func (r *pveSdnIpamResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		resp.Diagnostics.AddError("Invalid pve_sdn_ipam import ID", "import ID must be the IPAM plugin identifier, e.g. `netbox1`")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("ipam"), req.ID)...)
}

// readInto refreshes the model from PVE.
func (r *pveSdnIpamResource) readInto(ctx context.Context, m *pveSdnIpamResourceModel) error {
	ipam, err := r.client.GetSdnIpam(ctx, m.Ipam.ValueString())
	if err != nil {
		return err
	}
	m.Type = nodeNetworkStringToTF(ipam.Type)
	m.URL = nodeNetworkStringToTF(ipam.URL)
	m.Token = nodeNetworkStringToTF(ipam.Token)
	m.Fingerprint = nodeNetworkStringToTF(ipam.Fingerprint)
	m.Section = haInt64PtrToTF(ipam.Section)
	return nil
}

// sdnIpamFromModel projects the Terraform model into the wire body.
func sdnIpamFromModel(m pveSdnIpamResourceModel) pveclient.SdnIpam {
	body := pveclient.SdnIpam{
		Ipam: m.Ipam.ValueString(),
		Type: m.Type.ValueString(),
	}
	if !m.URL.IsNull() && !m.URL.IsUnknown() {
		body.URL = m.URL.ValueString()
	}
	if !m.Token.IsNull() && !m.Token.IsUnknown() {
		body.Token = m.Token.ValueString()
	}
	if !m.Fingerprint.IsNull() && !m.Fingerprint.IsUnknown() {
		body.Fingerprint = m.Fingerprint.ValueString()
	}
	if !m.Section.IsNull() && !m.Section.IsUnknown() {
		body.Section = pveclient.HAInt64Ptr(m.Section.ValueInt64())
	}
	return body
}

// sdnIpamDeleteFields returns the PVE field names to clear on update:
// optional attributes present in state but null in plan.
func sdnIpamDeleteFields(plan, state pveSdnIpamResourceModel) []string {
	var out []string
	if plan.URL.IsNull() && !state.URL.IsNull() {
		out = append(out, "url")
	}
	if plan.Token.IsNull() && !state.Token.IsNull() {
		out = append(out, "token")
	}
	if plan.Fingerprint.IsNull() && !state.Fingerprint.IsNull() {
		out = append(out, "fingerprint")
	}
	if plan.Section.IsNull() && !state.Section.IsNull() {
		out = append(out, "section")
	}
	return out
}
