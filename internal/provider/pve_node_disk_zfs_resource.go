// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ resource.Resource                = &pveNodeDiskZFSResource{}
	_ resource.ResourceWithConfigure   = &pveNodeDiskZFSResource{}
	_ resource.ResourceWithImportState = &pveNodeDiskZFSResource{}
)

// NewPveNodeDiskZFSResource returns the resource implementation.
func NewPveNodeDiskZFSResource() resource.Resource {
	return &pveNodeDiskZFSResource{}
}

// pveNodeDiskZFSResource manages a ZFS pool on a PVE node. PVE offers no
// in-place reconfigure; every non-key attribute forces replacement.
type pveNodeDiskZFSResource struct {
	client *pveclient.Client
}

// pveNodeDiskZFSResourceModel is the Terraform-facing shape.
type pveNodeDiskZFSResourceModel struct {
	Node        types.String   `tfsdk:"node"`
	Name        types.String   `tfsdk:"name"`
	RaidLevel   types.String   `tfsdk:"raidlevel"`
	Devices     []types.String `tfsdk:"devices"`
	Ashift      types.Int64    `tfsdk:"ashift"`
	Compression types.String   `tfsdk:"compression"`
	DRAIDConfig types.String   `tfsdk:"draid_config"`
	AddStorage  types.Bool     `tfsdk:"add_storage"`
	State       types.String   `tfsdk:"state"`
}

// zfsRaidLevels enumerates the values PVE accepts.
var zfsRaidLevels = []string{
	"single", "mirror", "raid10", "raidz", "raidz2", "raidz3",
	"draid", "draid2", "draid3",
}

// Metadata implements resource.Resource.
func (r *pveNodeDiskZFSResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveNodeDiskZFS
}

// Schema implements resource.Resource.
func (r *pveNodeDiskZFSResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"node": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Name of the PVE node.",
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Name of the ZFS pool.",
			},
			"raidlevel": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Pool topology. Must be one of: `single`, `mirror`, `raid10`, `raidz`, `raidz2`, `raidz3`, `draid`, `draid2`, `draid3`.",
				Validators: []validator.String{
					stringvalidator.OneOf(zfsRaidLevels...),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"devices": schema.ListAttribute{
				Required:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Devices backing the pool (e.g. `/dev/sdb`).",
				Validators: []validator.List{
					listvalidator.SizeAtLeast(1),
				},
				PlanModifiers: []planmodifier.List{
					listplanmodifier.RequiresReplace(),
				},
			},
			"ashift": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Pool sector size exponent (e.g. 12 for 4 KiB, 13 for 8 KiB). Must be between 9 and 16 inclusive (ZFS supports 512 B through 64 KiB physical blocks).",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
				Validators: []validator.Int64{
					int64validator.Between(9, 16),
				},
			},
			"compression": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Default compression algorithm. PVE forwards this to `zfs create -o compression=<value>`, which accepts `on`, `off`, `lzjb`, `lz4`, `zle`, `gzip`, `gzip-N` (1-9), `zstd`, and `zstd-fast` (plus PVE-specific `zstd-fast-N`). The provider does not validate the value; PVE and ZFS reject unknown algorithms.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"draid_config": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "dRAID configuration string (`dataspares|spares` per PVE). Required for `draid*` topologies.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"add_storage": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "Also create a storage entry for the pool in PVE. When `true`, the provider does not model the resulting storage; manage it via a future `pve_storage` resource.",
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.RequiresReplace(),
				},
			},
			"state": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Latest observed pool state. One of `online`, `degraded`, `faulted`, `offline`, `removed`, or `unavail`.",
			},
		},
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveNodeDiskZFSResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
func (r *pveNodeDiskZFSResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveNodeDiskZFSResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	in := pveclient.CreateZFSPoolInput{
		Name:        plan.Name.ValueString(),
		RaidLevel:   plan.RaidLevel.ValueString(),
		Devices:     stringsFromTF(plan.Devices),
		Ashift:      int(plan.Ashift.ValueInt64()),
		Compression: plan.Compression.ValueString(),
		DRAIDConfig: plan.DRAIDConfig.ValueString(),
		AddStorage:  plan.AddStorage.ValueBool(),
	}
	upid, err := r.client.CreateZFSPool(ctx, plan.Node.ValueString(), in)
	if err != nil {
		resp.Diagnostics.AddError("Error creating pve_node_disk_zfs", fmt.Sprintf("creating pool %s on %s: %s", plan.Name.ValueString(), plan.Node.ValueString(), err))
		return
	}
	if _, err := r.client.WaitForTask(ctx, plan.Node.ValueString(), upid, defaultWaitOptions()); err != nil {
		resp.Diagnostics.AddError("Error waiting for pve_node_disk_zfs create", fmt.Sprintf("waiting for pool create %s on %s: %s", plan.Name.ValueString(), plan.Node.ValueString(), err))
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Error reading pve_node_disk_zfs after create", fmt.Sprintf("reading pool %s on %s: %s", plan.Name.ValueString(), plan.Node.ValueString(), err))
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read implements resource.Resource.
func (r *pveNodeDiskZFSResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveNodeDiskZFSResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.readInto(ctx, &state); err != nil {
		if isPVEClientNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading pve_node_disk_zfs", fmt.Sprintf("reading pool %s on %s: %s", state.Name.ValueString(), state.Node.ValueString(), err))
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is a no-op at the API level: PVE has no PUT on /disks/zfs/{name}.
// Plan modifiers force replacement for every non-key attribute, so this
// method should never run in practice.
func (r *pveNodeDiskZFSResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveNodeDiskZFSResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.AddWarning(
		"pve_node_disk_zfs update ignored",
		"PVE does not support in-place ZFS reconfiguration; the plan should have forced replacement.",
	)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete implements resource.Resource.
func (r *pveNodeDiskZFSResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveNodeDiskZFSResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	upid, err := r.client.DeleteZFSPool(ctx, state.Node.ValueString(), state.Name.ValueString())
	if err != nil {
		if isPVEClientNotFound(err) {
			return
		}
		resp.Diagnostics.AddError("Error deleting pve_node_disk_zfs", fmt.Sprintf("deleting pool %s on %s: %s", state.Name.ValueString(), state.Node.ValueString(), err))
		return
	}
	if _, err := r.client.WaitForTask(ctx, state.Node.ValueString(), upid, defaultWaitOptions()); err != nil {
		resp.Diagnostics.AddError("Error waiting for pve_node_disk_zfs delete", fmt.Sprintf("waiting for pool delete %s on %s: %s", state.Name.ValueString(), state.Node.ValueString(), err))
		return
	}
}

// ImportState parses an import ID of the form `<node>:<pool>`.
func (r *pveNodeDiskZFSResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"Invalid pve_node_disk_zfs import ID",
			fmt.Sprintf("Import ID must be in the form `<node>:<pool>`, got %q.", req.ID),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("node"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), parts[1])...)
}

// readInto populates state from PVE; 404 removes the resource from state.
func (r *pveNodeDiskZFSResource) readInto(ctx context.Context, m *pveNodeDiskZFSResourceModel) error {
	pool, err := r.client.GetZFSPool(ctx, m.Node.ValueString(), m.Name.ValueString())
	if err != nil {
		return err
	}
	m.State = types.StringValue(pool.State)
	return nil
}

// stringsFromTF flattens a []types.String into a []string, preserving nil
// for empty/null inputs.
func stringsFromTF(in []types.String) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	for _, e := range in {
		if e.IsNull() || e.IsUnknown() {
			continue
		}
		out = append(out, e.ValueString())
	}
	return out
}

// defaultWaitOptions returns a WaitForTaskOptions tuned for disk
// operations (PVE /disks/* calls can run for tens of seconds).
func defaultWaitOptions() pveclient.WaitForTaskOptions {
	return pveclient.WaitForTaskOptions{
		Interval: pveclient.DefaultWaitInterval,
		Timeout:  pveclient.DefaultWaitTimeout,
	}
}
