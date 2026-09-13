// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ resource.Resource                = &pveNodeDiskDirectoryResource{}
	_ resource.ResourceWithConfigure   = &pveNodeDiskDirectoryResource{}
	_ resource.ResourceWithImportState = &pveNodeDiskDirectoryResource{}
)

// NewPveNodeDiskDirectoryResource returns the resource implementation.
func NewPveNodeDiskDirectoryResource() resource.Resource {
	return &pveNodeDiskDirectoryResource{}
}

// pveNodeDiskDirectoryResource manages a PVE-managed directory storage on a
// node: a filesystem created on an unused disk and mounted under
// /mnt/pve/<name>. PVE offers no in-place reconfigure; every non-key
// attribute forces replacement.
type pveNodeDiskDirectoryResource struct {
	client *pveclient.Client
}

// pveNodeDiskDirectoryResourceModel is the Terraform-facing shape.
type pveNodeDiskDirectoryResourceModel struct {
	Node          types.String `tfsdk:"node"`
	Name          types.String `tfsdk:"name"`
	Device        types.String `tfsdk:"device"`
	Filesystem    types.String `tfsdk:"filesystem"`
	AddStorage    types.Bool   `tfsdk:"add_storage"`
	CleanupConfig types.Bool   `tfsdk:"cleanup_config"`
	CleanupDisks  types.Bool   `tfsdk:"cleanup_disks"`
}

// directoryFilesystems enumerates the values the pinned API accepts.
var directoryFilesystems = []string{"ext4", "xfs"}

// Metadata implements resource.Resource.
func (r *pveNodeDiskDirectoryResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveNodeDiskDirectory
}

// Schema implements resource.Resource.
func (r *pveNodeDiskDirectoryResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages directory storage on a Proxmox VE node: a filesystem created on an unused disk, mounted under `/mnt/pve/<name>`. PVE does not support in-place directory reconfigure, so any non-key attribute change forces replacement.",
		Attributes: map[string]schema.Attribute{
			"node": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Name of the PVE node.",
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Storage identifier. The filesystem is mounted under `/mnt/pve/<name>`.",
			},
			"device": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The block device to create the filesystem on (e.g. `/dev/sdb`).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"filesystem": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The desired filesystem. Must be one of: `ext4`, `xfs`. PVE defaults to `ext4` when unset.",
				Validators: []validator.String{
					stringvalidator.OneOf(directoryFilesystems...),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"add_storage": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "Also create a storage entry pointing at the directory in PVE.",
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.RequiresReplace(),
				},
			},
			"cleanup_config": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
				MarkdownDescription: "When deleting the directory, also remove the storage entry PVE created when `add_storage=true` was set. Defaults to `true`.",
			},
			"cleanup_disks": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
				MarkdownDescription: "When deleting the directory, also wipe the underlying disk so it can be repurposed. Defaults to `true`; set to `false` to keep the disk for inspection.",
			},
		},
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveNodeDiskDirectoryResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
func (r *pveNodeDiskDirectoryResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveNodeDiskDirectoryResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	in := pveclient.CreateNodeDirectoryInput{
		Name:       plan.Name.ValueString(),
		Device:     plan.Device.ValueString(),
		Filesystem: plan.Filesystem.ValueString(),
		AddStorage: plan.AddStorage.ValueBool(),
	}
	upid, err := r.client.CreateNodeDirectory(ctx, plan.Node.ValueString(), in)
	if err != nil {
		resp.Diagnostics.AddError("Error creating pve_node_disk_directory", fmt.Sprintf("creating directory %s on %s: %s", plan.Name.ValueString(), plan.Node.ValueString(), err))
		return
	}
	if _, err := r.client.WaitForTask(ctx, plan.Node.ValueString(), upid, defaultWaitOptions()); err != nil {
		resp.Diagnostics.AddError("Error waiting for pve_node_disk_directory create", fmt.Sprintf("waiting for directory create %s on %s: %s", plan.Name.ValueString(), plan.Node.ValueString(), err))
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read implements resource.Resource.
func (r *pveNodeDiskDirectoryResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveNodeDiskDirectoryResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// The listing carries no name column; PVE mounts the filesystem under
	// /mnt/pve/<name>, so entries are matched on their mount path.
	found := false
	dirs, err := r.client.ListNodeDirectories(ctx, state.Node.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading pve_node_disk_directory", fmt.Sprintf("listing directories on %s: %s", state.Node.ValueString(), err))
		return
	}
	want := "/mnt/pve/" + state.Name.ValueString()
	for _, dir := range dirs {
		if dir.Path == want {
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

// Update is a no-op at the API level: PVE has no PUT on /disks/directory.
// Plan modifiers force replacement for every non-key attribute.
func (r *pveNodeDiskDirectoryResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveNodeDiskDirectoryResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.AddWarning(
		"pve_node_disk_directory update ignored",
		"PVE does not support in-place directory reconfiguration; the plan should have forced replacement.",
	)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete implements resource.Resource.
func (r *pveNodeDiskDirectoryResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveNodeDiskDirectoryResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	upid, err := r.client.DeleteNodeDirectory(ctx, state.Node.ValueString(), state.Name.ValueString(), state.CleanupConfig.ValueBool(), state.CleanupDisks.ValueBool())
	if err != nil {
		if isPVEClientNotFound(err) {
			return
		}
		resp.Diagnostics.AddError("Error deleting pve_node_disk_directory", fmt.Sprintf("deleting directory %s on %s: %s", state.Name.ValueString(), state.Node.ValueString(), err))
		return
	}
	if _, err := r.client.WaitForTask(ctx, state.Node.ValueString(), upid, defaultWaitOptions()); err != nil {
		resp.Diagnostics.AddError("Error waiting for pve_node_disk_directory delete", fmt.Sprintf("waiting for directory delete %s on %s: %s", state.Name.ValueString(), state.Node.ValueString(), err))
		return
	}
}

// ImportState parses an import ID of the form `<node>:<name>`.
func (r *pveNodeDiskDirectoryResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"Invalid pve_node_disk_directory import ID",
			fmt.Sprintf("Import ID must be in the form `<node>:<name>`, got %q.", req.ID),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("node"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), parts[1])...)
}
