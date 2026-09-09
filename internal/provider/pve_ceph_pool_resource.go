// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/float64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ resource.Resource                = &pveCephPoolResource{}
	_ resource.ResourceWithConfigure   = &pveCephPoolResource{}
	_ resource.ResourceWithImportState = &pveCephPoolResource{}
)

// NewPveCephPoolResource returns the resource implementation.
func NewPveCephPoolResource() resource.Resource {
	return &pveCephPoolResource{}
}

// cephPoolTargetSizePattern is the pin's target-size format
// `^(\d+(\.\d+)?)([KMGT])?$` (a byte count with optional SI suffix).
var cephPoolTargetSizePattern = regexp.MustCompile(`^(\d+(\.\d+)?)([KMGT])?$`)

// pveCephPoolResource manages a Ceph pool via /nodes/{node}/ceph/pool.
// Pools are cluster-scoped in Ceph but managed through any node endpoint;
// `node` is the management node only.
type pveCephPoolResource struct {
	client *pveclient.Client
}

// pveCephPoolResourceModel is the Terraform-facing shape.
type pveCephPoolResourceModel struct {
	Node                    types.String  `tfsdk:"node"`
	Name                    types.String  `tfsdk:"name"`
	Size                    types.Int64   `tfsdk:"size"`
	MinSize                 types.Int64   `tfsdk:"min_size"`
	PGNum                   types.Int64   `tfsdk:"pg_num"`
	PGNumMin                types.Int64   `tfsdk:"pg_num_min"`
	PGAutoscaleMode         types.String  `tfsdk:"pg_autoscale_mode"`
	Application             types.String  `tfsdk:"application"`
	CrushRule               types.String  `tfsdk:"crush_rule"`
	TargetSize              types.String  `tfsdk:"target_size"`
	TargetSizeRatio         types.Float64 `tfsdk:"target_size_ratio"`
	AddStorages             types.Bool    `tfsdk:"add_storages"`
	ErasureCoding           types.String  `tfsdk:"erasure_coding"`
	ForceDestroy            types.Bool    `tfsdk:"force_destroy"`
	RemoveStoragesOnDestroy types.Bool    `tfsdk:"remove_storages_on_destroy"`
	PoolID                  types.Int64   `tfsdk:"pool_id"`
	PoolType                types.String  `tfsdk:"pool_type"`
}

// Metadata implements resource.Resource.
func (r *pveCephPoolResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveCephPool
}

// Schema implements resource.Resource.
func (r *pveCephPoolResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Creates and manages a Ceph pool through `POST/PUT/DELETE /nodes/{node}/ceph/pool`. The pool is cluster-scoped in Ceph; `node` only selects the management endpoint the requests go through.",
		Attributes: map[string]schema.Attribute{
			"node": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Management node the pool requests are issued against. PVE applies the change cluster-wide; any Ceph-configured node works.",
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Name of the pool. It must be unique and must not contain `:`, `/`, or whitespace.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"size": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Number of replicas per object. Must be between 1 and 7 (PVE default 3).",
				Validators: []validator.Int64{
					int64validator.Between(1, 7),
				},
			},
			"min_size": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Minimum number of replicas required to accept writes. Must be between 1 and 7 (PVE default 2).",
				Validators: []validator.Int64{
					int64validator.Between(1, 7),
				},
			},
			"pg_num": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Number of placement groups. Must be between 1 and 32768 (PVE default 128).",
				Validators: []validator.Int64{
					int64validator.Between(1, 32768),
				},
			},
			"pg_num_min": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Minimal number of placement groups the autoscaler may choose. Must be between 1 and 32768.",
				Validators: []validator.Int64{
					int64validator.Between(1, 32768),
				},
			},
			"pg_autoscale_mode": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "The automatic PG scaling mode of the pool. Must be one of: `on`, `off`, `warn` (PVE default `warn`).",
				Validators: []validator.String{
					stringvalidator.OneOf("on", "off", "warn"),
				},
			},
			"application": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The application of the pool. Must be one of: `rbd`, `cephfs`, `rgw` (PVE default `rbd`).",
				Validators: []validator.String{
					stringvalidator.OneOf("rbd", "cephfs", "rgw"),
				},
			},
			"crush_rule": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Name of the CRUSH rule used for mapping object placement in the cluster. Read back from the pool listing (`crush_rule_name`).",
			},
			"target_size": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Estimated target size of the pool for the PG autoscaler, e.g. `10G`. Pattern: digits with optional decimal part and optional `K`, `M`, `G`, or `T` suffix.",
				Validators: []validator.String{
					stringvalidator.RegexMatches(cephPoolTargetSizePattern, "value must match digits with optional decimal part and optional K, M, G, or T suffix (e.g. 10G)"),
				},
			},
			"target_size_ratio": schema.Float64Attribute{
				Optional:            true,
				MarkdownDescription: "Estimated target ratio of total pool capacity for the PG autoscaler.",
				PlanModifiers: []planmodifier.Float64{
					float64planmodifier.UseStateForUnknown(),
				},
			},
			"add_storages": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Configure VM and CT storage entries using the new pool (create-time only; PVE defaults to false for replicated pools). The created storage entries are not managed by this resource.",
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.RequiresReplace(),
				},
			},
			"erasure_coding": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Create an erasure coded pool with an accompanying replicated metadata pool, as the pin's `erasure-coding` property string: `k=<int>, m=<int> [,device-class=<class>] [,failure-domain=<domain>] [,profile=<profile>]`. When set, `size`, `min_size`, and `crush_rule` apply to the metadata pool. Create-time only.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"force_destroy": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "Destroy the pool even when it is still in use (pin `force`, default `false`).",
			},
			"remove_storages_on_destroy": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "Remove all pveceph-managed storage entries configured for this pool on destroy (pin `remove_storages`, default `false`).",
			},
			"pool_id": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Numeric pool id assigned by Ceph.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"pool_type": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Pool type reported by PVE: `replicated`, `erasure`, or `unknown`.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveCephPoolResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = cephConfigureResource(req, resp)
}

// Create implements resource.Resource.
func (r *pveCephPoolResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveCephPoolResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error creating pve_ceph_pool", "provider client is not configured")
		return
	}

	in := pveclient.CreateCephPoolInput{
		Name:            plan.Name.ValueString(),
		PGAutoscaleMode: plan.PGAutoscaleMode.ValueString(),
		Application:     plan.Application.ValueString(),
		CrushRule:       plan.CrushRule.ValueString(),
		TargetSize:      plan.TargetSize.ValueString(),
		ErasureCoding:   plan.ErasureCoding.ValueString(),
	}
	if !plan.Size.IsNull() && !plan.Size.IsUnknown() {
		in.Size = cephIntPtr(int(plan.Size.ValueInt64()))
	}
	if !plan.MinSize.IsNull() && !plan.MinSize.IsUnknown() {
		in.MinSize = cephIntPtr(int(plan.MinSize.ValueInt64()))
	}
	if !plan.PGNum.IsNull() && !plan.PGNum.IsUnknown() {
		in.PGNum = cephIntPtr(int(plan.PGNum.ValueInt64()))
	}
	if !plan.PGNumMin.IsNull() && !plan.PGNumMin.IsUnknown() {
		in.PGNumMin = cephIntPtr(int(plan.PGNumMin.ValueInt64()))
	}
	if !plan.TargetSizeRatio.IsNull() && !plan.TargetSizeRatio.IsUnknown() {
		in.TargetSizeRatio = cephFloat64Ptr(plan.TargetSizeRatio.ValueFloat64())
	}
	if !plan.AddStorages.IsNull() && !plan.AddStorages.IsUnknown() {
		in.AddStorages = cephBoolPtr(plan.AddStorages.ValueBool())
	}

	upid, err := r.client.CreateCephPool(ctx, plan.Node.ValueString(), in)
	if err != nil {
		resp.Diagnostics.AddError("Error creating pve_ceph_pool", fmt.Sprintf("creating pool %s via %s: %s", plan.Name.ValueString(), plan.Node.ValueString(), err))
		return
	}
	if _, err := r.client.WaitForTask(ctx, plan.Node.ValueString(), upid, defaultWaitOptions()); err != nil {
		resp.Diagnostics.AddError("Error waiting for pve_ceph_pool create", fmt.Sprintf("waiting for pool create %s via %s: %s", plan.Name.ValueString(), plan.Node.ValueString(), err))
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Error reading pve_ceph_pool after create", fmt.Sprintf("reading pool %s via %s: %s", plan.Name.ValueString(), plan.Node.ValueString(), err))
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read implements resource.Resource.
func (r *pveCephPoolResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveCephPoolResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.readInto(ctx, &state); err != nil {
		if isPVEClientNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading pve_ceph_pool", fmt.Sprintf("reading pool %s via %s: %s", state.Name.ValueString(), state.Node.ValueString(), err))
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource.
func (r *pveCephPoolResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveCephPoolResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state pveCephPoolResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error updating pve_ceph_pool", "provider client is not configured")
		return
	}

	in := cephPoolUpdateInput(plan, state)
	upid, err := r.client.UpdateCephPool(ctx, plan.Node.ValueString(), plan.Name.ValueString(), in)
	if err != nil {
		resp.Diagnostics.AddError("Error updating pve_ceph_pool", fmt.Sprintf("updating pool %s via %s: %s", plan.Name.ValueString(), plan.Node.ValueString(), err))
		return
	}
	if _, err := r.client.WaitForTask(ctx, plan.Node.ValueString(), upid, defaultWaitOptions()); err != nil {
		resp.Diagnostics.AddError("Error waiting for pve_ceph_pool update", fmt.Sprintf("waiting for pool update %s via %s: %s", plan.Name.ValueString(), plan.Node.ValueString(), err))
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Error reading pve_ceph_pool after update", fmt.Sprintf("reading pool %s via %s: %s", plan.Name.ValueString(), plan.Node.ValueString(), err))
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete implements resource.Resource.
func (r *pveCephPoolResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveCephPoolResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error deleting pve_ceph_pool", "provider client is not configured")
		return
	}
	force := state.ForceDestroy.ValueBool()
	removeStorages := state.RemoveStoragesOnDestroy.ValueBool()
	upid, err := r.client.DeleteCephPool(ctx, state.Node.ValueString(), state.Name.ValueString(), force, true, removeStorages)
	if err != nil {
		if isPVEClientNotFound(err) {
			// Already absent counts as deleted.
			return
		}
		resp.Diagnostics.AddError("Error deleting pve_ceph_pool", fmt.Sprintf("deleting pool %s via %s: %s", state.Name.ValueString(), state.Node.ValueString(), err))
		return
	}
	if _, err := r.client.WaitForTask(ctx, state.Node.ValueString(), upid, defaultWaitOptions()); err != nil {
		resp.Diagnostics.AddError("Error waiting for pve_ceph_pool delete", fmt.Sprintf("waiting for pool delete %s via %s: %s", state.Name.ValueString(), state.Node.ValueString(), err))
		return
	}
}

// ImportState parses an import ID of the form `<node>:<name>`.
func (r *pveCephPoolResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"Invalid pve_ceph_pool import ID",
			fmt.Sprintf("Import ID must be in the form `<node>:<name>`, got %q.", req.ID),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("node"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), parts[1])...)
}

// readInto refreshes the model from the PVE pool listing; the synthesized
// 404 from GetCephPool propagates so Read can remove the resource.
func (r *pveCephPoolResource) readInto(ctx context.Context, m *pveCephPoolResourceModel) error {
	pool, err := r.client.GetCephPool(ctx, m.Node.ValueString(), m.Name.ValueString())
	if err != nil {
		return err
	}
	m.PoolID = types.Int64Value(int64(pool.Pool))
	m.PoolType = nodeNetworkStringToTF(pool.Type)
	m.Size = types.Int64Value(int64(pool.Size))
	m.MinSize = types.Int64Value(int64(pool.MinSize))
	m.PGNum = types.Int64Value(int64(pool.PGNum))
	m.PGNumMin = cephInt64OptToTF(pool.PGNumMin)
	m.PGAutoscaleMode = nodeNetworkStringToTF(pool.PGAutoscaleMode)
	m.CrushRule = nodeNetworkStringToTF(pool.CrushRuleName)
	return nil
}

// cephPoolUpdateInput diffs plan against state and returns only the
// settable fields that actually changed, so unchanged PG counts are never
// re-sent.
func cephPoolUpdateInput(plan, state pveCephPoolResourceModel) pveclient.UpdateCephPoolInput {
	in := pveclient.UpdateCephPoolInput{}
	if !plan.Size.Equal(state.Size) && !plan.Size.IsNull() && !plan.Size.IsUnknown() {
		in.Size = cephIntPtr(int(plan.Size.ValueInt64()))
	}
	if !plan.MinSize.Equal(state.MinSize) && !plan.MinSize.IsNull() && !plan.MinSize.IsUnknown() {
		in.MinSize = cephIntPtr(int(plan.MinSize.ValueInt64()))
	}
	if !plan.PGNum.Equal(state.PGNum) && !plan.PGNum.IsNull() && !plan.PGNum.IsUnknown() {
		in.PGNum = cephIntPtr(int(plan.PGNum.ValueInt64()))
	}
	if !plan.PGNumMin.Equal(state.PGNumMin) && !plan.PGNumMin.IsNull() && !plan.PGNumMin.IsUnknown() {
		in.PGNumMin = cephIntPtr(int(plan.PGNumMin.ValueInt64()))
	}
	if !plan.PGAutoscaleMode.Equal(state.PGAutoscaleMode) && !plan.PGAutoscaleMode.IsNull() && !plan.PGAutoscaleMode.IsUnknown() {
		in.PGAutoscaleMode = plan.PGAutoscaleMode.ValueString()
	}
	if !plan.Application.Equal(state.Application) && !plan.Application.IsNull() && !plan.Application.IsUnknown() {
		in.Application = plan.Application.ValueString()
	}
	if !plan.CrushRule.Equal(state.CrushRule) && !plan.CrushRule.IsNull() && !plan.CrushRule.IsUnknown() {
		in.CrushRule = plan.CrushRule.ValueString()
	}
	if !plan.TargetSize.Equal(state.TargetSize) && !plan.TargetSize.IsNull() && !plan.TargetSize.IsUnknown() {
		in.TargetSize = plan.TargetSize.ValueString()
	}
	if !plan.TargetSizeRatio.Equal(state.TargetSizeRatio) && !plan.TargetSizeRatio.IsNull() && !plan.TargetSizeRatio.IsUnknown() {
		in.TargetSizeRatio = cephFloat64Ptr(plan.TargetSizeRatio.ValueFloat64())
	}
	return in
}

// cephIntPtr maps an int to a *int for the client input structs.
func cephIntPtr(i int) *int { return &i }

// cephBoolPtr maps a bool to a *bool for the client input structs.
func cephBoolPtr(b bool) *bool { return &b }

// cephFloat64Ptr maps a float64 to a *float64 for the client input structs.
func cephFloat64Ptr(f float64) *float64 { return &f }

// cephInt64OptToTF maps a *int (an optional listing field) to Int64.
func cephInt64OptToTF(v *int) types.Int64 {
	if v == nil {
		return types.Int64Null()
	}
	return types.Int64Value(int64(*v))
}
