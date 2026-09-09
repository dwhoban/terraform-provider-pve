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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ resource.Resource                = &pveNodeDiskLvmthinResource{}
	_ resource.ResourceWithConfigure   = &pveNodeDiskLvmthinResource{}
	_ resource.ResourceWithImportState = &pveNodeDiskLvmthinResource{}
)

// NewPveNodeDiskLvmthinResource returns the resource implementation.
func NewPveNodeDiskLvmthinResource() resource.Resource {
	return &pveNodeDiskLvmthinResource{}
}

// pveNodeDiskLvmthinResource manages an LVM thinpool on a PVE node. PVE
// offers no in-place reconfigure; every non-key attribute forces
// replacement.
type pveNodeDiskLvmthinResource struct {
	client *pveclient.Client
}

// pveNodeDiskLvmthinResourceModel is the Terraform-facing shape.
type pveNodeDiskLvmthinResourceModel struct {
	Node          types.String `tfsdk:"node"`
	Name          types.String `tfsdk:"name"`
	Device        types.String `tfsdk:"device"`
	AddStorage    types.Bool   `tfsdk:"add_storage"`
	CleanupConfig types.Bool   `tfsdk:"cleanup_config"`
	CleanupDisks  types.Bool   `tfsdk:"cleanup_disks"`
}

// Metadata implements resource.Resource.
func (r *pveNodeDiskLvmthinResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveNodeDiskLvmthin
}

// Schema implements resource.Resource.
func (r *pveNodeDiskLvmthinResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages an LVM thinpool on a Proxmox VE node. PVE does not support in-place thinpool reconfigure, so any non-key attribute change forces replacement.",
		Attributes: map[string]schema.Attribute{
			"node": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Name of the PVE node.",
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Thinpool name. Also the storage identifier PVE uses when `add_storage` is enabled.",
			},
			"device": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The block device to create the thinpool on (e.g. `/dev/sdb`).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"add_storage": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "Also create a storage entry backed by the thinpool in PVE.",
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.RequiresReplace(),
				},
			},
			"cleanup_config": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
				MarkdownDescription: "When deleting the thinpool, also remove the storage entry PVE created when `add_storage=true` was set. Defaults to `true`.",
			},
			"cleanup_disks": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
				MarkdownDescription: "When deleting the thinpool, also wipe the underlying disk so it can be repurposed. Defaults to `true`; set to `false` to keep the disk for inspection.",
			},
		},
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveNodeDiskLvmthinResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*pveclient.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *pveclient.Client, got: %T.", req.ProviderData),
		)
		return
	}
	r.client = client
}

// Create implements resource.Resource.
func (r *pveNodeDiskLvmthinResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveNodeDiskLvmthinResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	in := pveclient.CreateLVMThinpoolInput{
		Name:       plan.Name.ValueString(),
		Device:     plan.Device.ValueString(),
		AddStorage: plan.AddStorage.ValueBool(),
	}
	upid, err := r.client.CreateLVMThinpool(ctx, plan.Node.ValueString(), in)
	if err != nil {
		resp.Diagnostics.AddError("Error creating pve_node_disk_lvmthin", fmt.Sprintf("creating thinpool %s on %s: %s", plan.Name.ValueString(), plan.Node.ValueString(), err))
		return
	}
	if _, err := r.client.WaitForTask(ctx, plan.Node.ValueString(), upid, defaultWaitOptions()); err != nil {
		resp.Diagnostics.AddError("Error waiting for pve_node_disk_lvmthin create", fmt.Sprintf("waiting for thinpool create %s on %s: %s", plan.Name.ValueString(), plan.Node.ValueString(), err))
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read implements resource.Resource.
func (r *pveNodeDiskLvmthinResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveNodeDiskLvmthinResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	found := false
	thinpools, err := r.client.ListLVMThinpools(ctx, state.Node.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading pve_node_disk_lvmthin", fmt.Sprintf("listing thinpools on %s: %s", state.Node.ValueString(), err))
		return
	}
	for _, tp := range thinpools {
		if tp.LV == state.Name.ValueString() {
			found = true
			break
		}
	}
	if !found {
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is a no-op at the API level: PVE has no PUT on /disks/lvmthin.
// Plan modifiers force replacement for every non-key attribute.
func (r *pveNodeDiskLvmthinResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveNodeDiskLvmthinResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.AddWarning(
		"pve_node_disk_lvmthin update ignored",
		"PVE does not support in-place LVM thinpool reconfiguration; the plan should have forced replacement.",
	)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete implements resource.Resource.
func (r *pveNodeDiskLvmthinResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveNodeDiskLvmthinResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	upid, err := r.client.DeleteLVMThinpool(ctx, state.Node.ValueString(), state.Name.ValueString(), state.CleanupConfig.ValueBool(), state.CleanupDisks.ValueBool())
	if err != nil {
		if isPVEClientNotFound(err) {
			return
		}
		resp.Diagnostics.AddError("Error deleting pve_node_disk_lvmthin", fmt.Sprintf("deleting thinpool %s on %s: %s", state.Name.ValueString(), state.Node.ValueString(), err))
		return
	}
	if _, err := r.client.WaitForTask(ctx, state.Node.ValueString(), upid, defaultWaitOptions()); err != nil {
		resp.Diagnostics.AddError("Error waiting for pve_node_disk_lvmthin delete", fmt.Sprintf("waiting for thinpool delete %s on %s: %s", state.Name.ValueString(), state.Node.ValueString(), err))
		return
	}
}

// ImportState parses an import ID of the form `<node>:<thinpool>`.
func (r *pveNodeDiskLvmthinResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"Invalid pve_node_disk_lvmthin import ID",
			fmt.Sprintf("Import ID must be in the form `<node>:<thinpool>`, got %q.", req.ID),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("node"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), parts[1])...)
}
