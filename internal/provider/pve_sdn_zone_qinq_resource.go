// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ resource.Resource                = &pveSdnZoneQinqResource{}
	_ resource.ResourceWithConfigure   = &pveSdnZoneQinqResource{}
	_ resource.ResourceWithImportState = &pveSdnZoneQinqResource{}
)

// NewPveSdnZoneQinqResource returns the resource implementation.
func NewPveSdnZoneQinqResource() resource.Resource {
	return &pveSdnZoneQinqResource{}
}

// pveSdnZoneQinqResource manages one QinQ SDN zone (POST/GET/PUT/DELETE on
// /cluster/sdn/zones with type `qinq`).
type pveSdnZoneQinqResource struct {
	client *pveclient.Client
}

// pveSdnZoneQinqResourceModel is the Terraform-facing shape.
type pveSdnZoneQinqResourceModel struct {
	sdnZoneCommonModel
	Bridge       types.String `tfsdk:"bridge"`
	Tag          types.Int64  `tfsdk:"tag"`
	VlanProtocol types.String `tfsdk:"vlan_protocol"`
}

// Metadata implements resource.Resource.
func (r *pveSdnZoneQinqResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveSdnZoneQinq
}

// Schema implements resource.Resource.
func (r *pveSdnZoneQinqResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := sdnZoneCommonAttributes()
	attrs["bridge"] = schema.StringAttribute{
		Optional:            true,
		MarkdownDescription: "The local bridge (PVE `bridge`) on each node where the service VLAN for this zone is managed, for example `vmbr0`.",
	}
	attrs["tag"] = schema.Int64Attribute{
		Optional:            true,
		MarkdownDescription: "Service-VLAN tag, i.e. the outer VLAN id of the QinQ zone (PVE `tag`). Must be 0 or greater.",
		Validators: []validator.Int64{
			int64validator.AtLeast(0),
		},
	}
	attrs["vlan_protocol"] = schema.StringAttribute{
		Optional:            true,
		MarkdownDescription: "VLAN protocol used when creating the QinQ zone (PVE `vlan-protocol`, default `802.1q`). Must be one of: `802.1q`, `802.1ad`.",
		Validators: []validator.String{
			stringvalidator.OneOf("802.1q", "802.1ad"),
		},
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a QinQ SDN zone (`/cluster/sdn/zones`, type `qinq`) stacking the vnets' inner VLAN tags below a service-VLAN tag on the zone. Creating, updating, or deleting a zone only changes the pending SDN configuration; run the `pve_sdn_apply` action to apply it to the running configuration. Changing `zone` forces recreation.",
		Attributes:          attrs,
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveSdnZoneQinqResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = haConfigureResource(req, resp)
}

// Create implements resource.Resource.
func (r *pveSdnZoneQinqResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveSdnZoneQinqResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error creating pve_sdn_zone_qinq", "provider client is not configured")
		return
	}
	if err := r.client.CreateSdnZone(ctx, sdnZoneQinqFromModel(plan)); err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_sdn_zone_qinq",
			fmt.Sprintf("creating zone %s: %s", plan.Zone.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_sdn_zone_qinq after create",
			fmt.Sprintf("reading zone %s: %s", plan.Zone.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	tflog.Debug(ctx, "created PVE qinq sdn zone", map[string]any{"zone": plan.Zone.ValueString()})
}

// Read implements resource.Resource.
func (r *pveSdnZoneQinqResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveSdnZoneQinqResourceModel
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
			"Error reading pve_sdn_zone_qinq",
			fmt.Sprintf("reading zone %s: %s", state.Zone.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource. Changed fields travel in the PUT
// body; attributes cleared in the plan travel in the `delete` query
// parameter.
func (r *pveSdnZoneQinqResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveSdnZoneQinqResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state pveSdnZoneQinqResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	deleteFields := append(
		sdnZoneCommonDeleteFields(plan.sdnZoneCommonModel, state.sdnZoneCommonModel),
		sdnZoneQinqDeleteFields(plan, state)...,
	)
	if err := r.client.UpdateSdnZone(ctx, plan.Zone.ValueString(), sdnZoneQinqFromModel(plan), deleteFields); err != nil {
		resp.Diagnostics.AddError(
			"Error updating pve_sdn_zone_qinq",
			fmt.Sprintf("updating zone %s: %s", plan.Zone.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_sdn_zone_qinq after update",
			fmt.Sprintf("reading zone %s: %s", plan.Zone.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	tflog.Debug(ctx, "updated PVE qinq sdn zone", map[string]any{"zone": plan.Zone.ValueString()})
}

// Delete implements resource.Resource.
func (r *pveSdnZoneQinqResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveSdnZoneQinqResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	tflog.Debug(ctx, "deleting PVE qinq sdn zone", map[string]any{"zone": state.Zone.ValueString()})
	if err := r.client.DeleteSdnZone(ctx, state.Zone.ValueString()); err != nil {
		if isPVEClientNotFound(err) {
			// Already absent counts as deleted.
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting pve_sdn_zone_qinq",
			fmt.Sprintf("deleting zone %s: %s", state.Zone.ValueString(), err),
		)
	}
}

// ImportState parses an import ID of the form `<zone>`.
func (r *pveSdnZoneQinqResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		resp.Diagnostics.AddError("Invalid pve_sdn_zone_qinq import ID", "import ID must be the zone identifier, e.g. `zone1`")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("zone"), req.ID)...)
}

// readInto refreshes the model from PVE, erroring when the upstream type
// is no longer qinq.
func (r *pveSdnZoneQinqResource) readInto(ctx context.Context, m *pveSdnZoneQinqResourceModel) error {
	z, err := sdnZoneGetChecked(ctx, r.client, m.Zone.ValueString(), "qinq")
	if err != nil {
		return err
	}
	sdnZoneQinqApply(z, m)
	return nil
}

// sdnZoneQinqFromModel projects the Terraform model into the wire body;
// null values are omitted from the request.
func sdnZoneQinqFromModel(m pveSdnZoneQinqResourceModel) pveclient.SdnZone {
	body := sdnZoneCommonFromModel(m.sdnZoneCommonModel)
	body.Type = "qinq"
	if !m.Bridge.IsNull() && !m.Bridge.IsUnknown() {
		body.Bridge = m.Bridge.ValueString()
	}
	if !m.Tag.IsNull() && !m.Tag.IsUnknown() {
		v := m.Tag.ValueInt64()
		body.Tag = &v
	}
	if !m.VlanProtocol.IsNull() && !m.VlanProtocol.IsUnknown() {
		body.VlanProtocol = m.VlanProtocol.ValueString()
	}
	return body
}

// sdnZoneQinqApply writes a fetched configuration into the model; absent
// settings become null.
func sdnZoneQinqApply(s *pveclient.SdnZone, m *pveSdnZoneQinqResourceModel) {
	sdnZoneCommonApply(s, &m.sdnZoneCommonModel)
	m.Bridge = nodeNetworkStringToTF(s.Bridge)
	m.Tag = haInt64PtrToTF(s.Tag)
	m.VlanProtocol = nodeNetworkStringToTF(s.VlanProtocol)
}

// sdnZoneQinqDeleteFields returns the PVE wire field names to clear on
// update: attributes present in state but null in the plan.
func sdnZoneQinqDeleteFields(plan, state pveSdnZoneQinqResourceModel) []string {
	var out []string
	if nodeNetworkStringCleared(plan.Bridge, state.Bridge) {
		out = append(out, "bridge")
	}
	if nodeNetworkIntCleared(plan.Tag, state.Tag) {
		out = append(out, "tag")
	}
	if nodeNetworkStringCleared(plan.VlanProtocol, state.VlanProtocol) {
		out = append(out, "vlan-protocol")
	}
	return out
}
