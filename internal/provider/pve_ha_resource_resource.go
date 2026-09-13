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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ resource.Resource                = &pveHaResourceResource{}
	_ resource.ResourceWithConfigure   = &pveHaResourceResource{}
	_ resource.ResourceWithImportState = &pveHaResourceResource{}
)

// NewPveHaResourceResource returns the resource implementation.
func NewPveHaResourceResource() resource.Resource {
	return &pveHaResourceResource{}
}

// pveHaResourceResource manages an HA resource via /cluster/ha/resources.
type pveHaResourceResource struct {
	client *pveclient.Client
}

// pveHaResourceResourceModel is the Terraform-facing shape.
type pveHaResourceResourceModel struct {
	SID           types.String `tfsdk:"sid"`
	Type          types.String `tfsdk:"type"`
	State         types.String `tfsdk:"state"`
	Group         types.String `tfsdk:"group"`
	MaxRestart    types.Int64  `tfsdk:"max_restart"`
	MaxRelocate   types.Int64  `tfsdk:"max_relocate"`
	Failback      types.Bool   `tfsdk:"failback"`
	AutoRebalance types.Bool   `tfsdk:"auto_rebalance"`
	Comment       types.String `tfsdk:"comment"`
}

// Metadata implements resource.Resource.
func (r *pveHaResourceResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveHaResource
}

// Schema implements resource.Resource.
func (r *pveHaResourceResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages an HA resource (`/cluster/ha/resources`), binding a guest (`vm:<vmid>` or `ct:<vmid>`) to the HA stack. Every mutation is synchronous per the pin (no task is spawned).",
		Attributes: map[string]schema.Attribute{
			"sid": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "HA resource ID in the form `<type>:<name>`, e.g. `vm:100` or `ct:101` (PVE `pve-ha-resource-or-vm-id`). Changing this value forces recreation.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"type": schema.StringAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "Resource type. Must be one of: `vm`, `ct`. " +
					"Only needed when `sid` is a bare VM/CT ID shortcut (e.g. `100`); otherwise it is derived from `sid`. Changes force recreation.",
				Validators: []validator.String{
					stringvalidator.OneOf("ct", "vm"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"state": schema.StringAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "Requested resource state. Must be one of: `started`, `stopped`, `enabled`, `disabled`, `ignored`. " +
					"`started` starts the resource and recovers it on node failures; `stopped` keeps it stopped but still relocates on node failures; " +
					"`enabled` is an alias for `started`; `disabled` stops it without relocating (error recovery); `ignored` removes it from HA management. Defaults to `started` upstream.",
				Validators: []validator.String{
					stringvalidator.OneOf("started", "stopped", "enabled", "disabled", "ignored"),
				},
			},
			"group": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "HA group identifier (PVE `pve-configid`) constraining placement. PVE marks groups as deprecated in favor of HA rules (`pve_ha_rule`).",
			},
			"max_restart": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Maximal number of tries to restart the resource on a node after its start failed. Must be 0 or greater. Defaults to 1 upstream.",
				Validators: []validator.Int64{
					int64validator.AtLeast(0),
				},
			},
			"max_relocate": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Maximal number of resource relocate tries when a resource fails to start. Must be 0 or greater. Defaults to 1 upstream.",
				Validators: []validator.Int64{
					int64validator.AtLeast(0),
				},
			},
			"failback": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Automatically migrate the HA resource to the node with the highest priority according to the node affinity rules when a higher-priority node comes online. Defaults to true upstream.",
			},
			"auto_rebalance": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Whether the HA resource may be migrated during automatic rebalancing (wire key `auto-rebalance`). Defaults to true upstream.",
			},
			"comment": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Description of the HA resource.",
			},
		},
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveHaResourceResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = haConfigureResource(req, resp)
}

// Create implements resource.Resource.
func (r *pveHaResourceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveHaResourceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error creating pve_ha_resource", "provider client is not configured")
		return
	}
	if err := r.client.CreateHAResource(ctx, haResourceFromModel(plan)); err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_ha_resource",
			fmt.Sprintf("creating HA resource %s: %s", plan.SID.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_ha_resource after create",
			fmt.Sprintf("reading HA resource %s: %s", plan.SID.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read implements resource.Resource.
func (r *pveHaResourceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveHaResourceResourceModel
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
			"Error reading pve_ha_resource",
			fmt.Sprintf("reading HA resource %s: %s", state.SID.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource.
func (r *pveHaResourceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveHaResourceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state pveHaResourceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	deleteFields := haResourceDeleteFields(plan, state)
	if err := r.client.UpdateHAResource(ctx, plan.SID.ValueString(), haResourceFromModel(plan), deleteFields); err != nil {
		resp.Diagnostics.AddError(
			"Error updating pve_ha_resource",
			fmt.Sprintf("updating HA resource %s: %s", plan.SID.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_ha_resource after update",
			fmt.Sprintf("reading HA resource %s: %s", plan.SID.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete implements resource.Resource.
func (r *pveHaResourceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveHaResourceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// Purge (the PVE default) also removes the resource from HA rules,
	// deleting a rule where it was the only member.
	if err := r.client.DeleteHAResource(ctx, state.SID.ValueString(), true); err != nil {
		if isPVEClientNotFound(err) {
			// Already absent counts as deleted.
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting pve_ha_resource",
			fmt.Sprintf("deleting HA resource %s: %s", state.SID.ValueString(), err),
		)
	}
}

// ImportState parses an import ID of the form `<sid>` (e.g. `vm:100`).
func (r *pveHaResourceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		resp.Diagnostics.AddError("Invalid pve_ha_resource import ID", "import ID must be the HA resource ID, e.g. `vm:100` or `ct:101`")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("sid"), req.ID)...)
}

// readInto refreshes the model from PVE.
func (r *pveHaResourceResource) readInto(ctx context.Context, m *pveHaResourceResourceModel) error {
	res, err := r.client.GetHAResource(ctx, m.SID.ValueString())
	if err != nil {
		return err
	}
	m.Type = nodeNetworkStringToTF(res.Type)
	m.State = nodeNetworkStringToTF(res.State)
	m.Group = nodeNetworkStringToTF(res.Group)
	m.MaxRestart = haInt64PtrToTF(res.MaxRestart)
	m.MaxRelocate = haInt64PtrToTF(res.MaxRelocate)
	m.Failback = nodeNetworkBoolPtrToTF(res.Failback)
	m.AutoRebalance = nodeNetworkBoolPtrToTF(res.AutoRebalance)
	m.Comment = nodeNetworkStringToTF(res.Comment)
	return nil
}

// haResourceFromModel projects the Terraform model into the wire body.
func haResourceFromModel(m pveHaResourceResourceModel) pveclient.HAResource {
	body := pveclient.HAResource{
		SID: m.SID.ValueString(),
	}
	if !m.Type.IsNull() && !m.Type.IsUnknown() {
		body.Type = m.Type.ValueString()
	}
	if !m.State.IsNull() && !m.State.IsUnknown() {
		body.State = m.State.ValueString()
	}
	if !m.Group.IsNull() && !m.Group.IsUnknown() {
		body.Group = m.Group.ValueString()
	}
	if !m.MaxRestart.IsNull() && !m.MaxRestart.IsUnknown() {
		body.MaxRestart = pveclient.HAInt64Ptr(m.MaxRestart.ValueInt64())
	}
	if !m.MaxRelocate.IsNull() && !m.MaxRelocate.IsUnknown() {
		body.MaxRelocate = pveclient.HAInt64Ptr(m.MaxRelocate.ValueInt64())
	}
	if !m.Failback.IsNull() && !m.Failback.IsUnknown() {
		body.Failback = pveclient.HABoolPtr(m.Failback.ValueBool())
	}
	if !m.AutoRebalance.IsNull() && !m.AutoRebalance.IsUnknown() {
		body.AutoRebalance = pveclient.HABoolPtr(m.AutoRebalance.ValueBool())
	}
	if !m.Comment.IsNull() && !m.Comment.IsUnknown() {
		body.Comment = m.Comment.ValueString()
	}
	return body
}

// haResourceDeleteFields returns the PVE field names to clear on update:
// optional attributes present in state but null in plan. `type` is
// create-only (the pin's PUT has no type parameter) and is never cleared.
func haResourceDeleteFields(plan, state pveHaResourceResourceModel) []string {
	var out []string
	if plan.State.IsNull() && !state.State.IsNull() {
		out = append(out, "state")
	}
	if plan.Group.IsNull() && !state.Group.IsNull() {
		out = append(out, "group")
	}
	if plan.MaxRestart.IsNull() && !state.MaxRestart.IsNull() {
		out = append(out, "max_restart")
	}
	if plan.MaxRelocate.IsNull() && !state.MaxRelocate.IsNull() {
		out = append(out, "max_relocate")
	}
	if plan.Failback.IsNull() && !state.Failback.IsNull() {
		out = append(out, "failback")
	}
	if plan.AutoRebalance.IsNull() && !state.AutoRebalance.IsNull() {
		out = append(out, "auto-rebalance")
	}
	if plan.Comment.IsNull() && !state.Comment.IsNull() {
		out = append(out, "comment")
	}
	return out
}
