// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ resource.Resource                = &pvePoolResource{}
	_ resource.ResourceWithConfigure   = &pvePoolResource{}
	_ resource.ResourceWithImportState = &pvePoolResource{}
)

// NewPvePoolResource returns the resource implementation.
func NewPvePoolResource() resource.Resource {
	return &pvePoolResource{}
}

// pvePoolResource manages a resource pool via /pools.
type pvePoolResource struct {
	client *pveclient.Client
}

// pvePoolResourceModel is the Terraform-facing shape.
type pvePoolResourceModel struct {
	PoolID    types.String         `tfsdk:"poolid"`
	Comment   types.String         `tfsdk:"comment"`
	VMs       types.Set            `tfsdk:"vms"`
	Storages  types.Set            `tfsdk:"storages"`
	AllowMove types.Bool           `tfsdk:"allow_move"`
	Members   []pvePoolMemberModel `tfsdk:"members"`
}

// pvePoolMemberModel is one computed pool member entry.
type pvePoolMemberModel struct {
	ID      types.String `tfsdk:"id"`
	Node    types.String `tfsdk:"node"`
	Storage types.String `tfsdk:"storage"`
	Type    types.String `tfsdk:"type"`
	VMID    types.Int64  `tfsdk:"vmid"`
}

// Metadata implements resource.Resource.
func (r *pvePoolResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePvePool
}

// Schema implements resource.Resource.
func (r *pvePoolResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Proxmox VE resource pool (`/pools`). PVE's create endpoint does not accept members; guest and storage members declared in `vms`/`storages` are attached (and detached) through `PUT /pools/{poolid}` after creation. Members may also join a pool at guest or storage create time; the `members` attribute is a computed view of the pool's current contents.",
		Attributes: map[string]schema.Attribute{
			"poolid": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The pool identifier (PVE `pve-poolid` format). Changing this value forces recreation.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"comment": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Description of the pool.",
			},
			"vms": schema.SetAttribute{
				ElementType:         types.Int64Type,
				Optional:            true,
				MarkdownDescription: "Set of guest VMIDs that belong to the pool. Guests must exist; a guest already in another pool needs `allow_move = true` to be moved into this pool. Removal from the set detaches the guest from the pool (the guest itself is untouched).",
			},
			"storages": schema.SetAttribute{
				ElementType:         types.StringType,
				Optional:            true,
				MarkdownDescription: "Set of storage IDs that belong to the pool. Storages must exist. Removal from the set detaches the storage from the pool.",
			},
			"allow_move": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Allow adding a guest even if it is already in another pool; the guest is removed from its current pool and added to this one. Defaults to `false`. Also requires permission to modify the guest's current pool.",
			},
			"members": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: poolMemberNestedAttributes(),
				},
				MarkdownDescription: "Current pool members, in upstream order.",
			},
		},
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pvePoolResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = haConfigureResource(req, resp)
}

// Create implements resource.Resource.
func (r *pvePoolResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pvePoolResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error creating pve_pool", "provider client is not configured")
		return
	}
	comment := poolOptionalString(plan.Comment)
	if err := r.client.CreatePool(ctx, plan.PoolID.ValueString(), comment); err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_pool",
			fmt.Sprintf("creating pool %s: %s", plan.PoolID.ValueString(), err),
		)
		return
	}
	vms := poolInt64SetValues(plan.VMs)
	storages := poolStringSetValues(plan.Storages)
	if len(vms) > 0 || len(storages) > 0 {
		update := pveclient.PoolUpdate{VMs: vms, Storages: storages}
		if plan.AllowMove.ValueBool() {
			update.AllowMove = pveclient.PoolBoolPtr(true)
		}
		if err := r.client.UpdatePool(ctx, plan.PoolID.ValueString(), update); err != nil {
			resp.Diagnostics.AddError(
				"Error creating pve_pool",
				fmt.Sprintf("attaching members to pool %s: %s", plan.PoolID.ValueString(), err),
			)
			return
		}
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_pool after create",
			fmt.Sprintf("reading pool %s: %s", plan.PoolID.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read implements resource.Resource.
func (r *pvePoolResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pvePoolResourceModel
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
			"Error reading pve_pool",
			fmt.Sprintf("reading pool %s: %s", state.PoolID.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource.
func (r *pvePoolResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pvePoolResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state pvePoolResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Detach removed members first so the add pass sees the final state.
	removedVMs := poolInt64SetRemoved(state.VMs, plan.VMs)
	removedStorages := poolStringSetRemoved(state.Storages, plan.Storages)
	if len(removedVMs) > 0 || len(removedStorages) > 0 {
		update := pveclient.PoolUpdate{VMs: removedVMs, Storages: removedStorages, Delete: true}
		if err := r.client.UpdatePool(ctx, plan.PoolID.ValueString(), update); err != nil {
			resp.Diagnostics.AddError(
				"Error updating pve_pool",
				fmt.Sprintf("detaching members from pool %s: %s", plan.PoolID.ValueString(), err),
			)
			return
		}
	}
	addedVMs := poolInt64SetRemoved(plan.VMs, state.VMs)
	addedStorages := poolStringSetRemoved(plan.Storages, state.Storages)
	commentChanged := poolOptionalString(plan.Comment) != poolOptionalString(state.Comment)
	if len(addedVMs) > 0 || len(addedStorages) > 0 || commentChanged {
		update := pveclient.PoolUpdate{
			Comment:  poolOptionalString(plan.Comment),
			VMs:      addedVMs,
			Storages: addedStorages,
		}
		if plan.AllowMove.ValueBool() {
			update.AllowMove = pveclient.PoolBoolPtr(true)
		}
		if err := r.client.UpdatePool(ctx, plan.PoolID.ValueString(), update); err != nil {
			resp.Diagnostics.AddError(
				"Error updating pve_pool",
				fmt.Sprintf("updating pool %s: %s", plan.PoolID.ValueString(), err),
			)
			return
		}
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_pool after update",
			fmt.Sprintf("reading pool %s: %s", plan.PoolID.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete implements resource.Resource.
func (r *pvePoolResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pvePoolResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeletePool(ctx, state.PoolID.ValueString()); err != nil {
		if isPVEClientNotFound(err) {
			// Already absent counts as deleted.
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting pve_pool",
			fmt.Sprintf("deleting pool %s: %s", state.PoolID.ValueString(), err),
		)
	}
}

// ImportState parses an import ID of the form `<poolid>`.
func (r *pvePoolResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		resp.Diagnostics.AddError("Invalid pve_pool import ID", "import ID must be the pool identifier, e.g. `prod`")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("poolid"), req.ID)...)
}

// readInto refreshes the computed fields of the model from PVE.
func (r *pvePoolResource) readInto(ctx context.Context, m *pvePoolResourceModel) error {
	pool, err := r.client.GetPool(ctx, m.PoolID.ValueString())
	if err != nil {
		return err
	}
	if pool == nil {
		return fmt.Errorf("pool %s not found", m.PoolID.ValueString())
	}
	m.Members = poolMemberModels(pool.Members)
	return nil
}

// poolMemberModels projects wire members into the Terraform member model,
// ordered by member ID for stable state.
func poolMemberModels(members []pveclient.PoolMember) []pvePoolMemberModel {
	out := make([]pvePoolMemberModel, 0, len(members))
	for _, m := range members {
		entry := pvePoolMemberModel{
			ID:      types.StringValue(m.ID),
			Node:    types.StringValue(m.Node),
			Storage: types.StringNull(),
			Type:    types.StringValue(m.Type),
			VMID:    types.Int64Null(),
		}
		if m.Storage != "" {
			entry.Storage = types.StringValue(m.Storage)
		}
		if m.VMID != nil {
			entry.VMID = types.Int64Value(*m.VMID)
		}
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID.ValueString() < out[j].ID.ValueString() })
	return out
}

// poolOptionalString returns the string value, or "" when null or unknown.
func poolOptionalString(v types.String) string {
	if v.IsNull() || v.IsUnknown() {
		return ""
	}
	return v.ValueString()
}

// poolInt64SetValues returns the set's int64 values in deterministic order;
// null and unknown sets yield nil.
func poolInt64SetValues(set types.Set) []int64 {
	if set.IsNull() || set.IsUnknown() {
		return nil
	}
	var out []int64
	_ = set.ElementsAs(context.Background(), &out, false)
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// poolStringSetValues returns the set's string values in deterministic
// order; null and unknown sets yield nil.
func poolStringSetValues(set types.Set) []string {
	if set.IsNull() || set.IsUnknown() {
		return nil
	}
	var out []string
	_ = set.ElementsAs(context.Background(), &out, false)
	sort.Strings(out)
	return out
}

// poolInt64SetRemoved returns the values present in `from` but not in
// `to`, both decoded sets.
func poolInt64SetRemoved(from, to types.Set) []int64 {
	fromVals := poolInt64SetValues(from)
	if len(fromVals) == 0 {
		return nil
	}
	toVals := map[int64]bool{}
	for _, v := range poolInt64SetValues(to) {
		toVals[v] = true
	}
	var out []int64
	for _, v := range fromVals {
		if !toVals[v] {
			out = append(out, v)
		}
	}
	return out
}

// poolStringSetRemoved returns the values present in `from` but not in
// `to`, both decoded sets.
func poolStringSetRemoved(from, to types.Set) []string {
	fromVals := poolStringSetValues(from)
	if len(fromVals) == 0 {
		return nil
	}
	toVals := map[string]bool{}
	for _, v := range poolStringSetValues(to) {
		toVals[v] = true
	}
	var out []string
	for _, v := range fromVals {
		if !toVals[v] {
			out = append(out, v)
		}
	}
	return out
}

// poolMemberNestedAttributes returns the shared member object attribute set
// for the pool resource and data source schemas.
func poolMemberNestedAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Member object identifier, e.g. `qemu/100` or `storage/local`.",
		},
		"node": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Node hosting the member object.",
		},
		"storage": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Storage identifier, set only for storage members.",
		},
		"type": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Member type. Must be one of: `qemu`, `lxc`, `openvz`, `storage`.",
		},
		"vmid": schema.Int64Attribute{
			Computed:            true,
			MarkdownDescription: "Guest VMID, set only for guest members.",
		},
	}
}
