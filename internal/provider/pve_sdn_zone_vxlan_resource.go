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
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ resource.Resource                = &pveSdnZoneVxlanResource{}
	_ resource.ResourceWithConfigure   = &pveSdnZoneVxlanResource{}
	_ resource.ResourceWithImportState = &pveSdnZoneVxlanResource{}
)

// NewPveSdnZoneVxlanResource returns the resource implementation.
func NewPveSdnZoneVxlanResource() resource.Resource {
	return &pveSdnZoneVxlanResource{}
}

// pveSdnZoneVxlanResource manages one VXLAN SDN zone (POST/GET/PUT/DELETE
// on /cluster/sdn/zones with type `vxlan`).
type pveSdnZoneVxlanResource struct {
	client *pveclient.Client
}

// pveSdnZoneVxlanResourceModel is the Terraform-facing shape.
type pveSdnZoneVxlanResourceModel struct {
	sdnZoneCommonModel
	Peers     types.String `tfsdk:"peers"`
	VxlanPort types.Int64  `tfsdk:"vxlan_port"`
	Fabric    types.String `tfsdk:"fabric"`
}

// Metadata implements resource.Resource.
func (r *pveSdnZoneVxlanResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveSdnZoneVxlan
}

// Schema implements resource.Resource.
func (r *pveSdnZoneVxlanResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := sdnZoneCommonAttributes()
	attrs["peers"] = schema.StringAttribute{
		Optional:            true,
		MarkdownDescription: "Comma-separated list of peer addresses (PVE `ip-list`) that are part of the VXLAN zone, usually the IPs of the participating nodes, for example `10.0.0.1,10.0.0.2`.",
	}
	attrs["vxlan_port"] = schema.Int64Attribute{
		Optional:            true,
		MarkdownDescription: "UDP port used for the VXLAN tunnel (PVE `vxlan-port`, default `4789`). Must be between 1 and 65536.",
		Validators: []validator.Int64{
			int64validator.Between(1, 65536),
		},
	}
	attrs["fabric"] = schema.StringAttribute{
		Optional:            true,
		MarkdownDescription: "SDN fabric (PVE `pve-sdn-fabric-id`) to use as the underlay for this VXLAN zone, for example a `pve_sdn_fabric_openfabric` fabric.",
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a VXLAN SDN zone (`/cluster/sdn/zones`, type `vxlan`) building an overlay between the peers' VTEPs. Creating, updating, or deleting a zone only changes the pending SDN configuration; run the `pve_sdn_apply` action to apply it to the running configuration. Changing `zone` forces recreation.",
		Attributes:          attrs,
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveSdnZoneVxlanResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = haConfigureResource(req, resp)
}

// Create implements resource.Resource.
func (r *pveSdnZoneVxlanResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveSdnZoneVxlanResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error creating pve_sdn_zone_vxlan", "provider client is not configured")
		return
	}
	if err := r.client.CreateSdnZone(ctx, sdnZoneVxlanFromModel(plan)); err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_sdn_zone_vxlan",
			fmt.Sprintf("creating zone %s: %s", plan.Zone.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_sdn_zone_vxlan after create",
			fmt.Sprintf("reading zone %s: %s", plan.Zone.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	tflog.Debug(ctx, "created PVE vxlan sdn zone", map[string]any{"zone": plan.Zone.ValueString()})
}

// Read implements resource.Resource.
func (r *pveSdnZoneVxlanResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveSdnZoneVxlanResourceModel
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
			"Error reading pve_sdn_zone_vxlan",
			fmt.Sprintf("reading zone %s: %s", state.Zone.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource. Changed fields travel in the PUT
// body; attributes cleared in the plan travel in the `delete` query
// parameter.
func (r *pveSdnZoneVxlanResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveSdnZoneVxlanResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state pveSdnZoneVxlanResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	deleteFields := append(
		sdnZoneCommonDeleteFields(plan.sdnZoneCommonModel, state.sdnZoneCommonModel),
		sdnZoneVxlanDeleteFields(plan, state)...,
	)
	if err := r.client.UpdateSdnZone(ctx, plan.Zone.ValueString(), sdnZoneVxlanFromModel(plan), deleteFields); err != nil {
		resp.Diagnostics.AddError(
			"Error updating pve_sdn_zone_vxlan",
			fmt.Sprintf("updating zone %s: %s", plan.Zone.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_sdn_zone_vxlan after update",
			fmt.Sprintf("reading zone %s: %s", plan.Zone.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	tflog.Debug(ctx, "updated PVE vxlan sdn zone", map[string]any{"zone": plan.Zone.ValueString()})
}

// Delete implements resource.Resource.
func (r *pveSdnZoneVxlanResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveSdnZoneVxlanResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	tflog.Debug(ctx, "deleting PVE vxlan sdn zone", map[string]any{"zone": state.Zone.ValueString()})
	if err := r.client.DeleteSdnZone(ctx, state.Zone.ValueString()); err != nil {
		if isPVEClientNotFound(err) {
			// Already absent counts as deleted.
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting pve_sdn_zone_vxlan",
			fmt.Sprintf("deleting zone %s: %s", state.Zone.ValueString(), err),
		)
	}
}

// ImportState parses an import ID of the form `<zone>`.
func (r *pveSdnZoneVxlanResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		resp.Diagnostics.AddError("Invalid pve_sdn_zone_vxlan import ID", "import ID must be the zone identifier, e.g. `zone1`")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("zone"), req.ID)...)
}

// readInto refreshes the model from PVE, erroring when the upstream type
// is no longer vxlan.
func (r *pveSdnZoneVxlanResource) readInto(ctx context.Context, m *pveSdnZoneVxlanResourceModel) error {
	z, err := sdnZoneGetChecked(ctx, r.client, m.Zone.ValueString(), "vxlan")
	if err != nil {
		return err
	}
	sdnZoneVxlanApply(z, m)
	return nil
}

// sdnZoneVxlanFromModel projects the Terraform model into the wire body;
// null values are omitted from the request.
func sdnZoneVxlanFromModel(m pveSdnZoneVxlanResourceModel) pveclient.SdnZone {
	body := sdnZoneCommonFromModel(m.sdnZoneCommonModel)
	body.Type = "vxlan"
	if !m.Peers.IsNull() && !m.Peers.IsUnknown() {
		body.Peers = m.Peers.ValueString()
	}
	if !m.VxlanPort.IsNull() && !m.VxlanPort.IsUnknown() {
		v := m.VxlanPort.ValueInt64()
		body.VxlanPort = &v
	}
	if !m.Fabric.IsNull() && !m.Fabric.IsUnknown() {
		body.Fabric = m.Fabric.ValueString()
	}
	return body
}

// sdnZoneVxlanApply writes a fetched configuration into the model; absent
// settings become null.
func sdnZoneVxlanApply(s *pveclient.SdnZone, m *pveSdnZoneVxlanResourceModel) {
	sdnZoneCommonApply(s, &m.sdnZoneCommonModel)
	m.Peers = nodeNetworkStringToTF(s.Peers)
	m.VxlanPort = haInt64PtrToTF(s.VxlanPort)
	m.Fabric = nodeNetworkStringToTF(s.Fabric)
}

// sdnZoneVxlanDeleteFields returns the PVE wire field names to clear on
// update: attributes present in state but null in the plan.
func sdnZoneVxlanDeleteFields(plan, state pveSdnZoneVxlanResourceModel) []string {
	var out []string
	if nodeNetworkStringCleared(plan.Peers, state.Peers) {
		out = append(out, "peers")
	}
	if nodeNetworkIntCleared(plan.VxlanPort, state.VxlanPort) {
		out = append(out, "vxlan-port")
	}
	if nodeNetworkStringCleared(plan.Fabric, state.Fabric) {
		out = append(out, "fabric")
	}
	return out
}
