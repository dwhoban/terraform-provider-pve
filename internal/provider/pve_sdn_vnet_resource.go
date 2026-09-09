// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ resource.Resource                = &pveSdnVnetResource{}
	_ resource.ResourceWithConfigure   = &pveSdnVnetResource{}
	_ resource.ResourceWithImportState = &pveSdnVnetResource{}
)

// NewPveSdnVnetResource returns the resource implementation.
func NewPveSdnVnetResource() resource.Resource {
	return &pveSdnVnetResource{}
}

// pveSdnVnetResource manages an SDN vnet object (/cluster/sdn/vnets).
// Every mutation is synchronous per the pin; applying the pending SDN
// configuration is the separate pve_sdn_apply action's job.
type pveSdnVnetResource struct {
	client *pveclient.Client
}

// pveSdnVnetResourceModel is the Terraform-facing shape.
type pveSdnVnetResourceModel struct {
	Vnet         types.String `tfsdk:"vnet"`
	Zone         types.String `tfsdk:"zone"`
	Alias        types.String `tfsdk:"alias"`
	Tag          types.Int64  `tfsdk:"tag"`
	VlanAware    types.Bool   `tfsdk:"vlanaware"`
	IsolatePorts types.Bool   `tfsdk:"isolate_ports"`
	State        types.String `tfsdk:"state"`
	Digest       types.String `tfsdk:"digest"`
}

// Metadata implements resource.Resource.
func (r *pveSdnVnetResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveSdnVnet
}

// Schema implements resource.Resource.
func (r *pveSdnVnetResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages an SDN vnet object (`/cluster/sdn/vnets`). A vnet is a virtual network bridged into a zone; changes stay pending until the `pve_sdn_apply` action runs.",
		Attributes: map[string]schema.Attribute{
			"vnet": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The SDN vnet object identifier (2 to 8 characters, starting with a letter, letters and digits only). Changing this value forces recreation.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"zone": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Name of the zone this vnet belongs to. Changing this value forces recreation.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"alias": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Alias name of the vnet (up to 256 characters).",
			},
			"tag": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "VLAN tag (for VLAN or QinQ zones) or VXLAN VNI (for VXLAN or EVPN zones). Must be between 1 and 16777215.",
				Validators: []validator.Int64{
					int64validator.Between(1, 16777215),
				},
			},
			"vlanaware": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Allow VLANs to pass through this vnet.",
			},
			"isolate_ports": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "If true, sets the isolated property for all interfaces on the bridge of this vnet.",
			},
			"state": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "State of the SDN configuration object; one of `new`, `changed`, or `deleted` when the configuration has not been applied yet.",
			},
			"digest": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Digest of the vnet section.",
			},
		},
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveSdnVnetResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = haConfigureResource(req, resp)
}

// Create implements resource.Resource.
func (r *pveSdnVnetResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveSdnVnetResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error creating pve_sdn_vnet", "provider client is not configured")
		return
	}
	if err := r.client.CreateSdnVnet(ctx, sdnVnetFromModel(plan)); err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_sdn_vnet",
			fmt.Sprintf("creating SDN vnet %s: %s", plan.Vnet.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_sdn_vnet after create",
			fmt.Sprintf("reading SDN vnet %s: %s", plan.Vnet.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read implements resource.Resource.
func (r *pveSdnVnetResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveSdnVnetResourceModel
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
			"Error reading pve_sdn_vnet",
			fmt.Sprintf("reading SDN vnet %s: %s", state.Vnet.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource.
func (r *pveSdnVnetResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveSdnVnetResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state pveSdnVnetResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error updating pve_sdn_vnet", "provider client is not configured")
		return
	}
	deleteFields := sdnVnetDeleteFields(plan, state)
	if err := r.client.UpdateSdnVnet(ctx, plan.Vnet.ValueString(), sdnVnetFromModel(plan), deleteFields); err != nil {
		resp.Diagnostics.AddError(
			"Error updating pve_sdn_vnet",
			fmt.Sprintf("updating SDN vnet %s: %s", plan.Vnet.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_sdn_vnet after update",
			fmt.Sprintf("reading SDN vnet %s: %s", plan.Vnet.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete implements resource.Resource.
func (r *pveSdnVnetResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveSdnVnetResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error deleting pve_sdn_vnet", "provider client is not configured")
		return
	}
	if err := r.client.DeleteSdnVnet(ctx, state.Vnet.ValueString()); err != nil {
		if isPVEClientNotFound(err) {
			// Already absent counts as deleted.
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting pve_sdn_vnet",
			fmt.Sprintf("deleting SDN vnet %s: %s", state.Vnet.ValueString(), err),
		)
	}
}

// ImportState parses an import ID of the form `<vnet>`.
func (r *pveSdnVnetResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		resp.Diagnostics.AddError("Invalid pve_sdn_vnet import ID", "import ID must be the vnet identifier, e.g. `vnet1`")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("vnet"), req.ID)...)
}

// readInto refreshes the model from PVE.
func (r *pveSdnVnetResource) readInto(ctx context.Context, m *pveSdnVnetResourceModel) error {
	vnet, err := r.client.GetSdnVnet(ctx, m.Vnet.ValueString())
	if err != nil {
		return err
	}
	m.Zone = nodeNetworkStringToTF(vnet.Zone)
	m.Alias = nodeNetworkStringToTF(vnet.Alias)
	m.Tag = haInt64PtrToTF(vnet.Tag)
	m.VlanAware = nodeNetworkBoolPtrToTF(vnet.VlanAware)
	m.IsolatePorts = nodeNetworkBoolPtrToTF(vnet.IsolatePorts)
	m.State = nodeNetworkStringToTF(vnet.State)
	m.Digest = nodeNetworkStringToTF(vnet.Digest)
	return nil
}

// sdnVnetFromModel projects the Terraform model into the wire body.
func sdnVnetFromModel(m pveSdnVnetResourceModel) pveclient.SdnVnet {
	body := pveclient.SdnVnet{
		Vnet: m.Vnet.ValueString(),
		Zone: m.Zone.ValueString(),
	}
	if !m.Alias.IsNull() && !m.Alias.IsUnknown() {
		body.Alias = m.Alias.ValueString()
	}
	if !m.Tag.IsNull() && !m.Tag.IsUnknown() {
		body.Tag = pveclient.HAInt64Ptr(m.Tag.ValueInt64())
	}
	if !m.VlanAware.IsNull() && !m.VlanAware.IsUnknown() {
		body.VlanAware = pveclient.HABoolPtr(m.VlanAware.ValueBool())
	}
	if !m.IsolatePorts.IsNull() && !m.IsolatePorts.IsUnknown() {
		body.IsolatePorts = pveclient.HABoolPtr(m.IsolatePorts.ValueBool())
	}
	return body
}

// sdnVnetDeleteFields returns the PVE field names to clear on update:
// optional attributes present in state but null in plan.
func sdnVnetDeleteFields(plan, state pveSdnVnetResourceModel) []string {
	var out []string
	if plan.Alias.IsNull() && !state.Alias.IsNull() {
		out = append(out, "alias")
	}
	if plan.Tag.IsNull() && !state.Tag.IsNull() {
		out = append(out, "tag")
	}
	if plan.VlanAware.IsNull() && !state.VlanAware.IsNull() {
		out = append(out, "vlanaware")
	}
	if plan.IsolatePorts.IsNull() && !state.IsolatePorts.IsNull() {
		out = append(out, "isolate-ports")
	}
	return out
}
