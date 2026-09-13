// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/float64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
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

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ resource.Resource                = &pveCephOSDResource{}
	_ resource.ResourceWithConfigure   = &pveCephOSDResource{}
	_ resource.ResourceWithImportState = &pveCephOSDResource{}
)

// NewPveCephOSDResource returns the resource implementation.
func NewPveCephOSDResource() resource.Resource {
	return &pveCephOSDResource{}
}

// pveCephOSDResource manages one Ceph OSD via
// POST/DELETE /nodes/{node}/ceph/osd. Every non-key attribute forces
// replacement: the pin defines no OSD update verb.
type pveCephOSDResource struct {
	client *pveclient.Client
}

// pveCephOSDResourceModel is the Terraform-facing shape.
type pveCephOSDResourceModel struct {
	Node             types.String  `tfsdk:"node"`
	Device           types.String  `tfsdk:"device"`
	DBDevice         types.String  `tfsdk:"db_device"`
	DBDeviceSizeGiB  types.Float64 `tfsdk:"db_device_size_gib"`
	WALDevice        types.String  `tfsdk:"wal_device"`
	WALDeviceSizeGiB types.Float64 `tfsdk:"wal_device_size_gib"`
	CrushDeviceClass types.String  `tfsdk:"crush_device_class"`
	Encrypted        types.Bool    `tfsdk:"encrypted"`
	OSDsPerDevice    types.Int64   `tfsdk:"osds_per_device"`
	CleanupOnDestroy types.Bool    `tfsdk:"cleanup_on_destroy"`
	OSDID            types.Int64   `tfsdk:"osd_id"`
	Hostname         types.String  `tfsdk:"hostname"`
	OSDData          types.String  `tfsdk:"osd_data"`
	OSDObjectStore   types.String  `tfsdk:"osd_objectstore"`
	Version          types.String  `tfsdk:"version"`
}

// Metadata implements resource.Resource.
func (r *pveCephOSDResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveCephOsd
}

// Schema implements resource.Resource.
func (r *pveCephOSDResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Creates and destroys one Ceph OSD on a node (`POST/DELETE /nodes/{node}/ceph/osd`). All creation attributes force replacement because the pin defines no OSD update verb; the OSD id is assigned by Ceph during the create task.",
		Attributes: map[string]schema.Attribute{
			"node": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Name of the PVE node the OSD is created on.",
			},
			"device": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Block device name backing the OSD (pin `dev`), e.g. `/dev/sdb`.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"db_device": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Block device name for `block.db` (pin `db_dev`). Mutually exclusive with `osds_per_device`.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"db_device_size_gib": schema.Float64Attribute{
				Optional:            true,
				MarkdownDescription: "Size in GiB for `block.db` (pin `db_dev_size`). Must be at least 1; requires `db_device`.",
				Validators: []validator.Float64{
					float64validator.AtLeast(1),
				},
				PlanModifiers: []planmodifier.Float64{
					float64planmodifier.RequiresReplace(),
				},
			},
			"wal_device": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Block device name for `block.wal` (pin `wal_dev`). Mutually exclusive with `osds_per_device`.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"wal_device_size_gib": schema.Float64Attribute{
				Optional:            true,
				MarkdownDescription: "Size in GiB for `block.wal` (pin `wal_dev_size`). Must be at least 0.5; requires `wal_device`.",
				Validators: []validator.Float64{
					float64validator.AtLeast(0.5),
				},
				PlanModifiers: []planmodifier.Float64{
					float64planmodifier.RequiresReplace(),
				},
			},
			"crush_device_class": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "CRUSH device class to assign the OSD in the CRUSH map (pin `crush-device-class`), e.g. `ssd` or `hdd`.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"encrypted": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Enable LUKS/dm-crypt encryption of the OSD (pin `encrypted`, default `false`).",
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.RequiresReplace(),
				},
			},
			"osds_per_device": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "OSD services per physical device; only useful for fast NVMe devices. Must be at least 1 and is mutually exclusive with `db_device` and `wal_device`.",
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
				},
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"cleanup_on_destroy": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "Also destroy the underlying logical volumes via `ceph-volume lvm zap --destroy`, remove the volume group's physical volume with `pvremove`, and wipe leftover journal/block.db/block.wal partitions on destroy (pin `cleanup`, default `false`). When `false` the LVs and partitions are left intact for inspection.",
			},
			"osd_id": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "OSD id assigned by Ceph during the create task.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"hostname": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Name of the host containing the OSD (from the OSD metadata read).",
			},
			"osd_data": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Path to the OSD's data directory, e.g. `/var/lib/ceph/osd/pve1-ceph-0`.",
			},
			"osd_objectstore": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The object store type used by the OSD (typically `bluestore`).",
			},
			"version": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Ceph version of the OSD service.",
			},
		},
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveCephOSDResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = cephConfigureResource(req, resp)
}

// Create implements resource.Resource.
func (r *pveCephOSDResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveCephOSDResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error creating pve_ceph_osd", "provider client is not configured")
		return
	}
	node := plan.Node.ValueString()

	// The create task assigns the OSD id, so snapshot the OSD ids before
	// and after and diff to identify the new one.
	before, err := r.client.ListCephOSDTree(ctx, node)
	if err != nil {
		resp.Diagnostics.AddError("Error creating pve_ceph_osd", fmt.Sprintf("listing ceph OSDs on %s before create: %s", node, err))
		return
	}

	in := pveclient.CreateCephOSDInput{
		Dev:              plan.Device.ValueString(),
		DBDev:            plan.DBDevice.ValueString(),
		WALDev:           plan.WALDevice.ValueString(),
		CrushDeviceClass: plan.CrushDeviceClass.ValueString(),
		Encrypted:        plan.Encrypted.ValueBool(),
	}
	if !plan.DBDeviceSizeGiB.IsNull() && !plan.DBDeviceSizeGiB.IsUnknown() {
		in.DBDevSizeGiB = plan.DBDeviceSizeGiB.ValueFloat64()
	}
	if !plan.WALDeviceSizeGiB.IsNull() && !plan.WALDeviceSizeGiB.IsUnknown() {
		in.WALDevSizeGiB = plan.WALDeviceSizeGiB.ValueFloat64()
	}
	if !plan.OSDsPerDevice.IsNull() && !plan.OSDsPerDevice.IsUnknown() {
		in.OSDsPerDevice = int(plan.OSDsPerDevice.ValueInt64())
	}
	upid, err := r.client.CreateCephOSD(ctx, node, in)
	if err != nil {
		resp.Diagnostics.AddError("Error creating pve_ceph_osd", fmt.Sprintf("creating OSD on %s from %s: %s", node, plan.Device.ValueString(), err))
		return
	}
	if _, err := r.client.WaitForTask(ctx, node, upid, defaultWaitOptions()); err != nil {
		resp.Diagnostics.AddError("Error waiting for pve_ceph_osd create", fmt.Sprintf("waiting for OSD create on %s from %s: %s", node, plan.Device.ValueString(), err))
		return
	}
	after, err := r.client.ListCephOSDTree(ctx, node)
	if err != nil {
		resp.Diagnostics.AddError("Error creating pve_ceph_osd", fmt.Sprintf("listing ceph OSDs on %s after create: %s", node, err))
		return
	}
	newIDs := cephOSDNewIDs(before, after)
	if len(newIDs) != 1 {
		resp.Diagnostics.AddError(
			"Error creating pve_ceph_osd",
			fmt.Sprintf("cannot determine the OSD id created on %s from %s: %d new OSD ids appeared (%v); the OSD may exist unmanaged", node, plan.Device.ValueString(), len(newIDs), newIDs),
		)
		return
	}
	plan.OSDID = types.Int64Value(int64(newIDs[0]))

	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Error reading pve_ceph_osd after create", fmt.Sprintf("reading OSD %d on %s: %s", newIDs[0], node, err))
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read implements resource.Resource.
func (r *pveCephOSDResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveCephOSDResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.readInto(ctx, &state); err != nil {
		if isPVEClientNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading pve_ceph_osd", fmt.Sprintf("reading OSD %d on %s: %s", state.OSDID.ValueInt64(), state.Node.ValueString(), err))
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is a no-op at the API level: the pin defines no OSD update verb.
// Plan modifiers force replacement for every non-key attribute, so this
// method should never run in practice.
func (r *pveCephOSDResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveCephOSDResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.AddWarning(
		"pve_ceph_osd update ignored",
		"PVE offers no in-place OSD reconfiguration; the plan should have forced replacement.",
	)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete implements resource.Resource.
func (r *pveCephOSDResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveCephOSDResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error deleting pve_ceph_osd", "provider client is not configured")
		return
	}
	node := state.Node.ValueString()
	osdID := int(state.OSDID.ValueInt64())

	// Mark the OSD out first so data rebalances away from it, per the pin's
	// destroy flow; the verb returns no upid. A 404 means the OSD is
	// already gone and destroy is a no-op.
	if err := r.client.CephOSDOut(ctx, node, osdID); err != nil {
		if !isPVEClientNotFound(err) {
			resp.Diagnostics.AddError("Error deleting pve_ceph_osd", fmt.Sprintf("marking OSD %d out on %s: %s", osdID, node, err))
			return
		}
		return
	}
	upid, err := r.client.DestroyCephOSD(ctx, node, osdID, state.CleanupOnDestroy.ValueBool())
	if err != nil {
		if isPVEClientNotFound(err) {
			// Already absent counts as deleted.
			return
		}
		resp.Diagnostics.AddError("Error deleting pve_ceph_osd", fmt.Sprintf("destroying OSD %d on %s: %s", osdID, node, err))
		return
	}
	if _, err := r.client.WaitForTask(ctx, node, upid, defaultWaitOptions()); err != nil {
		resp.Diagnostics.AddError("Error waiting for pve_ceph_osd delete", fmt.Sprintf("waiting for OSD destroy %d on %s: %s", osdID, node, err))
		return
	}
}

// ImportState parses an import ID of the form `<node>:osd.<id>` (the plain
// `<node>:<id>` form is also accepted).
func (r *pveCephOSDResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"Invalid pve_ceph_osd import ID",
			fmt.Sprintf("Import ID must be in the form `<node>:osd.<id>`, got %q.", req.ID),
		)
		return
	}
	rawID := strings.TrimPrefix(parts[1], "osd.")
	osdID, err := strconv.Atoi(rawID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid pve_ceph_osd import ID",
			fmt.Sprintf("Import ID must be in the form `<node>:osd.<id>`, got %q: %s.", req.ID, err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("node"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("osd_id"), int64(osdID))...)
}

// readInto refreshes the computed metadata from the OSD metadata read; 404
// removes the resource from state.
func (r *pveCephOSDResource) readInto(ctx context.Context, m *pveCephOSDResourceModel) error {
	meta, err := r.client.GetCephOSDMetadata(ctx, m.Node.ValueString(), int(m.OSDID.ValueInt64()))
	if err != nil {
		return err
	}
	m.Hostname = nodeNetworkStringToTF(meta.Hostname)
	m.OSDData = nodeNetworkStringToTF(meta.OSDData)
	m.OSDObjectStore = nodeNetworkStringToTF(meta.OSDObjectStore)
	m.Version = nodeNetworkStringToTF(meta.Version)
	return nil
}

// cephOSDNewIDs returns the OSD ids present in after but not in before.
func cephOSDNewIDs(before, after []pveclient.CephOSDTreeEntry) []int {
	known := make(map[int]bool, len(before))
	for _, e := range before {
		known[e.ID] = true
	}
	var out []int
	for _, e := range after {
		if !known[e.ID] {
			out = append(out, e.ID)
		}
	}
	return out
}
