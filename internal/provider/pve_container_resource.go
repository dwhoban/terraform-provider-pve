// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	int64validator "github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
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
	_ resource.Resource                   = &pveContainerResource{}
	_ resource.ResourceWithConfigure      = &pveContainerResource{}
	_ resource.ResourceWithImportState    = &pveContainerResource{}
	_ resource.ResourceWithValidateConfig = &pveContainerResource{}
)

// NewPveContainerResource returns the resource implementation.
func NewPveContainerResource() resource.Resource {
	return &pveContainerResource{}
}

// pveContainerResource manages one LXC container via
// /nodes/{node}/lxc/{vmid}. Unmodeled PVE configuration keys (netN,
// features, devN, startup, tty, ...) are never sent and never removed by
// this resource.
type pveContainerResource struct {
	client *pveclient.Client
}

// pveContainerCloneModel is the create-only clone block; its presence
// switches Create to POST .../clone of an existing container.
type pveContainerCloneModel struct {
	SourceVMID types.Int64  `tfsdk:"source_vmid"`
	Hostname   types.String `tfsdk:"hostname"`
	Full       types.Bool   `tfsdk:"full"`
	Storage    types.String `tfsdk:"storage"`
	Pool       types.String `tfsdk:"pool"`
}

// pveContainerResourceModel is the Terraform-facing shape.
type pveContainerResourceModel struct {
	ID             types.String               `tfsdk:"id"`
	VMID           types.Int64                `tfsdk:"vmid"`
	Node           types.String               `tfsdk:"node"`
	OSTemplate     types.String               `tfsdk:"ostemplate"`
	Hostname       types.String               `tfsdk:"hostname"`
	Description    types.String               `tfsdk:"description"`
	Tags           types.String               `tfsdk:"tags"`
	Started        types.Bool                 `tfsdk:"started"`
	StopOnDestroy  types.Bool                 `tfsdk:"stop_on_destroy"`
	Onboot         types.Bool                 `tfsdk:"onboot"`
	Protection     types.Bool                 `tfsdk:"protection"`
	Template       types.Bool                 `tfsdk:"template"`
	Unprivileged   types.Bool                 `tfsdk:"unprivileged"`
	Cores          types.Int64                `tfsdk:"cores"`
	Memory         types.Int64                `tfsdk:"memory"`
	Swap           types.Int64                `tfsdk:"swap"`
	Password       types.String               `tfsdk:"password"`
	SSHPublicKeys  types.String               `tfsdk:"ssh_public_keys"`
	Nameserver     types.String               `tfsdk:"nameserver"`
	Searchdomain   types.String               `tfsdk:"searchdomain"`
	Clone          *pveContainerCloneModel    `tfsdk:"clone"`
	MountPoints    []containerMountPointModel `tfsdk:"mount_points"`
	Status         types.String               `tfsdk:"status"`
	PendingChanges types.List                 `tfsdk:"pending_changes"`
	Interfaces     []containerInterfaceModel  `tfsdk:"interfaces"`
}

// containerID renders the resource id `<node>/<vmid>`.
func containerID(node string, vmid int64) string {
	return fmt.Sprintf("%s/%d", node, vmid)
}

// Metadata implements resource.Resource.
func (r *pveContainerResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveContainer
}

// Schema implements resource.Resource.
func (r *pveContainerResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages one LXC container, either from an `ostemplate` or by cloning an existing container via the create-only `clone` block. " +
			"The resource models the pinned subset of the container config: PVE configuration keys it does not model (for example `netN`, `features`, `devN`, `startup`, `tty`) are never sent and never removed. " +
			"Import ID is `<node>/<vmid>`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Resource identifier in the form `<node>/<vmid>`.",
			},
			"vmid": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Container ID (100 - 999999999). When omitted at create time the next free ID is allocated via `GET /cluster/nextid`.",
				Validators:          []validator.Int64{int64validator.Between(100, 999999999)},
				PlanModifiers:       []planmodifier.Int64{int64PlanModifierRequiresReplace{}},
			},
			"node": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The cluster node the container runs on. Changing the node migrates the container via `POST .../migrate` (task-waiting) instead of recreating it.",
			},
			"ostemplate": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The OS template or backup file the container is created from (a volume string such as `local:vztmpl/debian-12-standard_12.7-1_amd64.tar.zst`). Create-only: exactly one of `ostemplate` or `clone` must be set; it is never sent on update.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"hostname": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Set a host name for the container.",
			},
			"description": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Description for the container; saved as comment inside the configuration file.",
			},
			"tags": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Tags of the container (a `;`-separated list). This is only meta information.",
			},
			"started": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Whether the container is started after creation (default `true`) and kept running. Toggling it starts or cleanly shuts the container down.",
			},
			"stop_on_destroy": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Gracefully shut the container down (60s timeout, falling back to an abrupt stop) before destroying it. Defaults to `false`.",
			},
			"onboot": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Specifies whether the container will be started during system bootup.",
			},
			"protection": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Sets the protection flag of the container. This prevents the container or its disks from removal or update operations.",
			},
			"template": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Enable/disable the container template flag. Changing it forces replacement.",
				PlanModifiers:       []planmodifier.Bool{boolPlanModifierRequiresReplace{}},
			},
			"unprivileged": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Makes the container run as an unprivileged user (creation default upstream is `true`). The pin marks this as should-not-be-modified-manually, so changing it forces replacement.",
				PlanModifiers:       []planmodifier.Bool{boolPlanModifierRequiresReplace{}},
			},
			"cores": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "The number of cores assigned to the container. Must be between 1 and 8192.",
				Validators:          []validator.Int64{int64validator.Between(1, 8192)},
			},
			"memory": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Amount of RAM for the container in MB. Must be at least 16.",
				Validators:          []validator.Int64{int64validator.AtLeast(16)},
			},
			"swap": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Amount of SWAP for the container in MB. Must be at least 0.",
				Validators:          []validator.Int64{int64validator.AtLeast(0)},
			},
			"password": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "Sets the root password inside the container at create time. PVE never returns it; the value is write-only after create.",
			},
			"ssh_public_keys": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "Setup public SSH keys (one key per line, OpenSSH format) at create time. PVE never returns it; the value is write-only after create.",
			},
			"nameserver": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Sets DNS server IP addresses for the container. Create automatically uses the setting from the host if neither `nameserver` nor `searchdomain` is set.",
			},
			"searchdomain": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Sets DNS search domains for the container. Create automatically uses the setting from the host if neither `nameserver` nor `searchdomain` is set.",
			},
			"clone": schema.SingleNestedAttribute{
				Optional:            true,
				MarkdownDescription: "Create the container by cloning an existing container instead of restoring an `ostemplate`. Mutually exclusive with `ostemplate`; changing the block forces replacement.",
				PlanModifiers:       []planmodifier.Object{objectPlanModifierRequiresReplace{}},
				Attributes: map[string]schema.Attribute{
					"source_vmid": schema.Int64Attribute{
						Required:            true,
						MarkdownDescription: "VMID of the container to clone (100 - 999999999).",
						Validators:          []validator.Int64{int64validator.Between(100, 999999999)},
					},
					"hostname": schema.StringAttribute{
						Optional:            true,
						MarkdownDescription: "Host name for the new container. Conflicts with setting `hostname` on the resource.",
					},
					"full": schema.BoolAttribute{
						Optional:            true,
						MarkdownDescription: "Create a full copy of all disks. For container templates a linked clone is attempted by default when unset.",
					},
					"storage": schema.StringAttribute{
						Optional:            true,
						MarkdownDescription: "Target storage for a full clone.",
					},
					"pool": schema.StringAttribute{
						Optional:            true,
						MarkdownDescription: "Add the new container to the specified pool.",
					},
				},
			},
			"mount_points": schema.ListNestedAttribute{
				Optional:            true,
				MarkdownDescription: "Ordered mount points including the root volume. `rootfs` must appear at most once; `mp0`..`mp255` follow. Updating re-renders changed entries via `PUT .../config` and applies size growth through the resize verb. Bind mounts (a `volume` host path without `storage`) cannot be resized.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: containerMountPointNestedAttributes(false),
				},
			},
			"status": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Container status as reported by `GET .../status/current`: `stopped` or `running`.",
			},
			"pending_changes": schema.ListAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Configuration keys with a pending (not yet applied) value or a pending delete, from `GET .../pending`.",
			},
			"interfaces": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Network interfaces with discovered IP addresses from `GET .../interfaces`; null when the container is stopped (the endpoint requires a running container).",
				NestedObject: schema.NestedAttributeObject{
					Attributes: containerInterfaceNestedAttributes(),
				},
			},
		},
	}
}

// int64PlanModifierRequiresReplace forces replacement unconditionally.
type int64PlanModifierRequiresReplace struct{}

// Description implements planmodifier.Int64.
func (m int64PlanModifierRequiresReplace) Description(_ context.Context) string {
	return "Changing this value forces replacement."
}

// MarkdownDescription implements planmodifier.Int64.
func (m int64PlanModifierRequiresReplace) MarkdownDescription(_ context.Context) string {
	return "Changing this value forces replacement."
}

// PlanModifyInt64 implements planmodifier.Int64.
func (m int64PlanModifierRequiresReplace) PlanModifyInt64(_ context.Context, _ planmodifier.Int64Request, resp *planmodifier.Int64Response) {
	resp.RequiresReplace = true
}

// boolPlanModifierRequiresReplace forces replacement unconditionally.
type boolPlanModifierRequiresReplace struct{}

// Description implements planmodifier.Bool.
func (m boolPlanModifierRequiresReplace) Description(_ context.Context) string {
	return "Changing this value forces replacement."
}

// MarkdownDescription implements planmodifier.Bool.
func (m boolPlanModifierRequiresReplace) MarkdownDescription(_ context.Context) string {
	return "Changing this value forces replacement."
}

// PlanModifyBool implements planmodifier.Bool.
func (m boolPlanModifierRequiresReplace) PlanModifyBool(_ context.Context, _ planmodifier.BoolRequest, resp *planmodifier.BoolResponse) {
	resp.RequiresReplace = true
}

// objectPlanModifierRequiresReplace forces replacement unconditionally for
// whole nested attributes.
type objectPlanModifierRequiresReplace struct{}

// Description implements planmodifier.Object.
func (m objectPlanModifierRequiresReplace) Description(_ context.Context) string {
	return "Changing this value forces replacement."
}

// MarkdownDescription implements planmodifier.Object.
func (m objectPlanModifierRequiresReplace) MarkdownDescription(_ context.Context) string {
	return "Changing this value forces replacement."
}

// PlanModifyObject implements planmodifier.Object.
func (m objectPlanModifierRequiresReplace) PlanModifyObject(_ context.Context, _ planmodifier.ObjectRequest, resp *planmodifier.ObjectResponse) {
	resp.RequiresReplace = true
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveContainerResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = containerConfigureResource(req, resp)
}

// ValidateConfig enforces the exactly-one create-source rule and the mount
// point key/allocation rules.
func (r *pveContainerResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var plan pveContainerResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	hasTemplate := !plan.OSTemplate.IsNull()
	hasClone := plan.Clone != nil
	if hasTemplate == hasClone {
		resp.Diagnostics.AddError(
			"Invalid pve_container configuration",
			"Exactly one of `ostemplate` or `clone` must be set: a container is created from a template or by cloning an existing container.",
		)
		return
	}
	if hasClone {
		if !plan.Clone.Hostname.IsNull() && !plan.Hostname.IsNull() {
			resp.Diagnostics.AddError(
				"Invalid pve_container configuration",
				"`hostname` is set both on the resource and in the `clone` block; set it in exactly one place.",
			)
			return
		}
		if !plan.VMID.IsNull() && !plan.Clone.SourceVMID.IsNull() && plan.VMID.ValueInt64() == plan.Clone.SourceVMID.ValueInt64() {
			resp.Diagnostics.AddError(
				"Invalid pve_container configuration",
				fmt.Sprintf("`clone.source_vmid` %d equals `vmid`; a container cannot clone itself.", plan.VMID.ValueInt64()),
			)
			return
		}
	}
	pveContainerValidateMountPoints(plan.MountPoints, &resp.Diagnostics)
}

// containerMountPointKeyPattern matches rootfs and mp0..mp999.
var containerMountPointKeyPattern = regexp.MustCompile(`^(rootfs|mp[0-9]{1,3})$`)

// pveContainerValidateMountPoints checks key syntax, uniqueness, rootfs
// placement and the storage/volume/size allocation combinations.
func pveContainerValidateMountPoints(mountPoints []containerMountPointModel, diags interface{ AddError(summary, detail string) }) {
	seen := map[string]bool{}
	rootfsSeen := false
	for _, mp := range mountPoints {
		id := mp.ID.ValueString()
		if !containerMountPointKeyPattern.MatchString(id) {
			diags.AddError(
				"Invalid pve_container mount point id",
				fmt.Sprintf("Mount point id %q must be `rootfs` or `mp0`..`mp255`.", id),
			)
			continue
		}
		if seen[id] {
			diags.AddError(
				"Invalid pve_container mount point id",
				fmt.Sprintf("Mount point id %q appears more than once.", id),
			)
			continue
		}
		seen[id] = true
		if id == "rootfs" {
			if rootfsSeen {
				diags.AddError(
					"Invalid pve_container mount point id",
					"`rootfs` appears more than once in `mount_points`.",
				)
				continue
			}
			rootfsSeen = true
			if !mp.Mountpoint.IsNull() {
				diags.AddError(
					"Invalid pve_container mount point",
					"`mountpoint` is not valid for the `rootfs` entry; the pin defines no `mp=` option for the root volume.",
				)
			}
		} else if mp.Mountpoint.IsNull() {
			diags.AddError(
				"Invalid pve_container mount point",
				fmt.Sprintf("`mountpoint` is required for mount point %q.", id),
			)
		}
		hasStorage := !mp.Storage.IsNull()
		hasVolume := !mp.Volume.IsNull()
		hasSize := !mp.Size.IsNull()
		switch {
		case hasStorage && hasVolume && hasSize:
			diags.AddError(
				"Invalid pve_container mount point",
				fmt.Sprintf("Mount point %q sets `storage`, `volume`, and `size`; reference an existing volume with `storage` + `volume`, or allocate one with `storage` + `size`.", id),
			)
		case hasStorage && !hasVolume && !hasSize:
			diags.AddError(
				"Invalid pve_container mount point",
				fmt.Sprintf("Mount point %q sets `storage` without `volume`; `size` is required to allocate a new volume.", id),
			)
		case !hasStorage && hasVolume && hasSize:
			diags.AddError(
				"Invalid pve_container mount point",
				fmt.Sprintf("Mount point %q is a bind mount (`volume` without `storage`) and cannot have a `size`.", id),
			)
		}
	}
}

// Create implements resource.Resource.
func (r *pveContainerResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveContainerResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error creating pve_container", "provider client is not configured")
		return
	}
	node := plan.Node.ValueString()

	vmid := plan.VMID.ValueInt64()
	if plan.VMID.IsNull() || plan.VMID.IsUnknown() {
		allocated, err := r.client.GetNextID(ctx)
		if err != nil {
			resp.Diagnostics.AddError("Error creating pve_container", fmt.Sprintf("allocating the next free VMID: %s", err))
			return
		}
		vmid = allocated
	}

	if plan.Clone != nil {
		r.createByClone(ctx, node, vmid, &plan, &resp.Diagnostics)
	} else {
		r.createByTemplate(ctx, node, vmid, &plan, &resp.Diagnostics)
	}
	if resp.Diagnostics.HasError() {
		return
	}
	plan.VMID = types.Int64Value(vmid)
	plan.ID = types.StringValue(containerID(node, vmid))
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Error reading pve_container after create", fmt.Sprintf("reading container %d on %s: %s", vmid, node, err))
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// createByTemplate restores an ostemplate via POST /nodes/{node}/lxc and
// waits for the create task.
func (r *pveContainerResource) createByTemplate(ctx context.Context, node string, vmid int64, plan *pveContainerResourceModel, diags interface{ AddError(summary, detail string) }) {
	started := plan.Started.IsNull() || plan.Started.ValueBool()
	params := pveclient.CreateLxcContainerParams{
		VMID:          vmid,
		OSTemplate:    plan.OSTemplate.ValueString(),
		Hostname:      plan.Hostname.ValueString(),
		Description:   plan.Description.ValueString(),
		Tags:          plan.Tags.ValueString(),
		Onboot:        containerBoolPtr(plan.Onboot),
		Protection:    containerBoolPtr(plan.Protection),
		Template:      containerBoolPtr(plan.Template),
		Unprivileged:  containerBoolPtr(plan.Unprivileged),
		Password:      plan.Password.ValueString(),
		SSHPublicKeys: plan.SSHPublicKeys.ValueString(),
		Nameserver:    plan.Nameserver.ValueString(),
		Searchdomain:  plan.Searchdomain.ValueString(),
		Start:         &started,
	}
	if !plan.Cores.IsNull() {
		cores := plan.Cores.ValueInt64()
		params.Cores = &cores
	}
	if !plan.Memory.IsNull() {
		memory := plan.Memory.ValueInt64()
		params.Memory = &memory
	}
	if !plan.Swap.IsNull() {
		swap := plan.Swap.ValueInt64()
		params.Swap = &swap
	}
	r.applyMountPointsToCreate(plan, &params)
	upid, err := r.client.CreateLxcContainer(ctx, node, params)
	if err != nil {
		diags.AddError("Error creating pve_container", fmt.Sprintf("creating container %d on %s from %s: %s", vmid, node, plan.OSTemplate.ValueString(), err))
		return
	}
	if _, err := r.client.WaitForTask(ctx, node, upid, defaultWaitOptions()); err != nil {
		diags.AddError("Error waiting for pve_container create", fmt.Sprintf("waiting for container %d create on %s: %s", vmid, node, err))
	}
}

// applyMountPointsToCreate renders rootfs/mpN strings into the create body.
func (r *pveContainerResource) applyMountPointsToCreate(plan *pveContainerResourceModel, params *pveclient.CreateLxcContainerParams) {
	if len(plan.MountPoints) == 0 {
		return
	}
	indexed := containerMountPointModelsByIndex(plan.MountPoints)
	if rootfs, ok := indexed["rootfs"]; ok {
		params.Rootfs = containerMountPointToWire(rootfs).Render()
	}
	mountPoints := make(map[string]string, len(plan.MountPoints))
	for id, mp := range indexed {
		if id == "rootfs" {
			continue
		}
		mountPoints[id] = containerMountPointToWire(mp).Render()
	}
	if len(mountPoints) > 0 {
		params.MountPoints = mountPoints
	}
}

// createByClone clones the source container, applies the modeled config on
// top of the clone, and optionally starts the new container.
func (r *pveContainerResource) createByClone(ctx context.Context, node string, vmid int64, plan *pveContainerResourceModel, diags interface{ AddError(summary, detail string) }) {
	clone := plan.Clone
	params := pveclient.CloneLxcContainerParams{NewID: vmid}
	if !clone.Hostname.IsNull() {
		params.Hostname = clone.Hostname.ValueString()
	}
	if !clone.Full.IsNull() {
		full := clone.Full.ValueBool()
		params.Full = &full
	}
	if !clone.Storage.IsNull() {
		params.Storage = clone.Storage.ValueString()
	}
	if !clone.Pool.IsNull() {
		params.Pool = clone.Pool.ValueString()
	}
	upid, err := r.client.CloneLxcContainer(ctx, node, clone.SourceVMID.ValueInt64(), params)
	if err != nil {
		diags.AddError("Error creating pve_container", fmt.Sprintf("cloning container %d to %d on %s: %s", clone.SourceVMID.ValueInt64(), vmid, node, err))
		return
	}
	if _, err := r.client.WaitForTask(ctx, node, upid, defaultWaitOptions()); err != nil {
		diags.AddError("Error waiting for pve_container clone", fmt.Sprintf("waiting for clone %d to %d on %s: %s", clone.SourceVMID.ValueInt64(), vmid, node, err))
		return
	}
	// Overlay the modeled config onto the fresh clone; the clone inherits
	// the source's settings, so only explicitly set attributes are sent.
	update := pveContainerConfigUpdate(plan, &pveContainerResourceModel{})
	if update != nil {
		if err := r.client.UpdateLxcConfig(ctx, node, vmid, *update); err != nil {
			diags.AddError("Error creating pve_container", fmt.Sprintf("applying configuration to clone %d on %s: %s", vmid, node, err))
			return
		}
	}
	started := plan.Started.IsNull() || plan.Started.ValueBool()
	if started {
		startUPID, err := r.client.LxcStart(ctx, node, vmid)
		if err != nil {
			diags.AddError("Error creating pve_container", fmt.Sprintf("starting cloned container %d on %s: %s", vmid, node, err))
			return
		}
		if _, err := r.client.WaitForTask(ctx, node, startUPID, defaultWaitOptions()); err != nil {
			diags.AddError("Error waiting for pve_container start", fmt.Sprintf("waiting for clone %d start on %s: %s", vmid, node, err))
		}
	}
}

// Read implements resource.Resource.
func (r *pveContainerResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveContainerResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.readInto(ctx, &state); err != nil {
		if isPVEClientNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading pve_container", fmt.Sprintf("reading container %d on %s: %s", state.VMID.ValueInt64(), state.Node.ValueString(), err))
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource: node changes migrate the container,
// config changes go through PUT .../config, size growth through the resize
// verb, and started toggles power-state tasks.
func (r *pveContainerResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state pveContainerResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error updating pve_container", "provider client is not configured")
		return
	}
	vmid := state.VMID.ValueInt64()
	node := plan.Node.ValueString()

	if plan.Node.ValueString() != state.Node.ValueString() {
		sourceNode := state.Node.ValueString()
		upid, err := r.client.MigrateLxcContainer(ctx, sourceNode, vmid, pveclient.LxcMigrateParams{Target: node})
		if err != nil {
			resp.Diagnostics.AddError("Error updating pve_container", fmt.Sprintf("migrating container %d from %s to %s: %s", vmid, sourceNode, node, err))
			return
		}
		if _, err := r.client.WaitForTask(ctx, sourceNode, upid, defaultWaitOptions()); err != nil {
			resp.Diagnostics.AddError("Error waiting for pve_container migration", fmt.Sprintf("waiting for container %d migration from %s to %s: %s", vmid, sourceNode, node, err))
			return
		}
		plan.ID = types.StringValue(containerID(node, vmid))
	}

	update := pveContainerConfigUpdate(&plan, &state)
	r.applyMountPointDiff(ctx, vmid, node, &plan, &state, &update, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	if update != nil {
		if err := r.client.UpdateLxcConfig(ctx, node, vmid, *update); err != nil {
			resp.Diagnostics.AddError("Error updating pve_container", fmt.Sprintf("updating configuration of container %d on %s: %s", vmid, node, err))
			return
		}
	}

	if !plan.Started.Equal(state.Started) {
		started := plan.Started.IsNull() || plan.Started.ValueBool()
		if started {
			upid, err := r.client.LxcStart(ctx, node, vmid)
			if err != nil {
				resp.Diagnostics.AddError("Error updating pve_container", fmt.Sprintf("starting container %d on %s: %s", vmid, node, err))
				return
			}
			if _, err := r.client.WaitForTask(ctx, node, upid, defaultWaitOptions()); err != nil {
				resp.Diagnostics.AddError("Error waiting for pve_container start", fmt.Sprintf("waiting for container %d start on %s: %s", vmid, node, err))
				return
			}
		} else {
			timeout := int64(60)
			upid, err := r.client.LxcShutdown(ctx, node, vmid, pveclient.LxcShutdownParams{Timeout: &timeout})
			if err != nil {
				resp.Diagnostics.AddError("Error updating pve_container", fmt.Sprintf("shutting down container %d on %s: %s", vmid, node, err))
				return
			}
			if _, err := r.client.WaitForTask(ctx, node, upid, defaultWaitOptions()); err != nil {
				resp.Diagnostics.AddError("Error waiting for pve_container shutdown", fmt.Sprintf("waiting for container %d shutdown on %s: %s", vmid, node, err))
				return
			}
		}
	}

	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Error reading pve_container after update", fmt.Sprintf("reading container %d on %s: %s", vmid, node, err))
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// applyMountPointDiff re-renders new or changed mount points into the
// config PUT, collects removed ids into the delete list, and applies size
// growth through the resize verb.
func (r *pveContainerResource) applyMountPointDiff(ctx context.Context, vmid int64, node string, plan, state *pveContainerResourceModel, update **pveclient.UpdateLxcConfigParams, diags interface {
	AddError(summary, detail string)
}) {
	if plan.MountPoints == nil && state.MountPoints == nil {
		return
	}
	planMPs := containerMountPointModelsByIndex(plan.MountPoints)
	stateMPs := containerMountPointModelsByIndex(state.MountPoints)
	removed := make([]string, 0)
	for id := range stateMPs {
		if _, ok := planMPs[id]; !ok {
			removed = append(removed, id)
		}
	}
	sort.Strings(removed)
	if len(removed) > 0 {
		if *update == nil {
			*update = &pveclient.UpdateLxcConfigParams{}
		}
		(*update).Delete = strings.Join(removed, ",")
	}
	mountPoints := map[string]string{}
	changed := false
	for id, mp := range planMPs {
		prior, exists := stateMPs[id]
		if !exists {
			mountPoints[id] = containerMountPointToWire(mp).Render()
			changed = true
			continue
		}
		if !containerMountPointEntryEqual(mp, prior) {
			mountPoints[id] = containerMountPointToWire(mp).Render()
			changed = true
		}
		r.resizeMountPointGrowth(ctx, vmid, node, id, mp, prior, diags)
	}
	if changed {
		if *update == nil {
			*update = &pveclient.UpdateLxcConfigParams{}
		}
		(*update).MountPoints = mountPoints
	}
}

// resizeMountPointGrowth applies a growth-only size change through the
// resize verb; shrinking, no-op changes and bind mounts are rejected.
func (r *pveContainerResource) resizeMountPointGrowth(ctx context.Context, vmid int64, node, id string, mp, prior containerMountPointModel, diags interface {
	AddError(summary, detail string)
}) {
	if mp.Size.Equal(prior.Size) || mp.Size.IsNull() || mp.Size.IsUnknown() {
		return
	}
	newBytes, newOK := containerDiskSizeBytes(mp.Size.ValueString())
	if !newOK {
		diags.AddError("Error resizing pve_container mount point", fmt.Sprintf("mount point %q size %q is not a valid PVE disk size (for example `8G`).", id, mp.Size.ValueString()))
		return
	}
	oldRaw := prior.Size.ValueString()
	oldBytes, oldOK := containerDiskSizeBytes(oldRaw)
	if !oldOK {
		diags.AddError("Error resizing pve_container mount point", fmt.Sprintf("mount point %q has no resizable size (current %q, likely a bind mount); bind mounts cannot be resized.", id, oldRaw))
		return
	}
	if newBytes <= oldBytes {
		diags.AddError("Error resizing pve_container mount point", fmt.Sprintf("mount point %q size %q does not grow the current size %q; shrinking is not supported by PVE.", id, mp.Size.ValueString(), oldRaw))
		return
	}
	var upid string
	var err error
	if id == "rootfs" {
		upid, err = r.client.ResizeLxcRootfs(ctx, node, vmid, mp.Size.ValueString())
	} else {
		upid, err = r.client.ResizeLxcMountpoint(ctx, node, vmid, id, mp.Size.ValueString())
	}
	if err != nil {
		diags.AddError("Error resizing pve_container mount point", fmt.Sprintf("resizing mount point %q of container %d on %s to %s: %s", id, vmid, node, mp.Size.ValueString(), err))
		return
	}
	if _, err := r.client.WaitForTask(ctx, node, upid, defaultWaitOptions()); err != nil {
		diags.AddError("Error waiting for pve_container resize", fmt.Sprintf("waiting for mount point %q resize of container %d on %s: %s", id, vmid, node, err))
	}
}

// containerMountPointEntryEqual compares the managed fields of two mount
// point models.
func containerMountPointEntryEqual(a, b containerMountPointModel) bool {
	return a.ID.Equal(b.ID) &&
		a.Mountpoint.Equal(b.Mountpoint) &&
		a.Storage.Equal(b.Storage) &&
		a.Volume.Equal(b.Volume) &&
		a.ACL.Equal(b.ACL) &&
		a.Backup.Equal(b.Backup) &&
		a.ReadOnly.Equal(b.ReadOnly)
}

// pveContainerConfigUpdate builds the PUT .../config body for the modeled
// scalar attributes that differ between plan and prior (null = nothing to
// update). Mount point handling is layered on by the caller.
func pveContainerConfigUpdate(plan, prior *pveContainerResourceModel) *pveclient.UpdateLxcConfigParams {
	update := &pveclient.UpdateLxcConfigParams{}
	changed := false
	setString := func(p, q types.String, dst *string) {
		if p.IsNull() || p.Equal(q) {
			return
		}
		*dst = p.ValueString()
		changed = true
	}
	setBool := func(p, q types.Bool, dst **bool) {
		if p.IsNull() || p.Equal(q) {
			return
		}
		v := p.ValueBool()
		*dst = &v
		changed = true
	}
	setString(plan.Hostname, prior.Hostname, &update.Hostname)
	setString(plan.Description, prior.Description, &update.Description)
	setString(plan.Tags, prior.Tags, &update.Tags)
	setBool(plan.Onboot, prior.Onboot, &update.Onboot)
	setBool(plan.Protection, prior.Protection, &update.Protection)
	setBool(plan.Template, prior.Template, &update.Template)
	setBool(plan.Unprivileged, prior.Unprivileged, &update.Unprivileged)
	if !plan.Cores.IsNull() && !plan.Cores.Equal(prior.Cores) {
		v := plan.Cores.ValueInt64()
		update.Cores = &v
		changed = true
	}
	if !plan.Memory.IsNull() && !plan.Memory.Equal(prior.Memory) {
		v := plan.Memory.ValueInt64()
		update.Memory = &v
		changed = true
	}
	if !plan.Swap.IsNull() && !plan.Swap.Equal(prior.Swap) {
		v := plan.Swap.ValueInt64()
		update.Swap = &v
		changed = true
	}
	setString(plan.Nameserver, prior.Nameserver, &update.Nameserver)
	setString(plan.Searchdomain, prior.Searchdomain, &update.Searchdomain)
	if !changed {
		return nil
	}
	return update
}

// Delete implements resource.Resource. With `stop_on_destroy` a running
// container is shut down gracefully (60s timeout) with an abrupt stop as
// the fallback before the destroy task runs.
func (r *pveContainerResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveContainerResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error deleting pve_container", "provider client is not configured")
		return
	}
	node := state.Node.ValueString()
	vmid := state.VMID.ValueInt64()

	if state.StopOnDestroy.ValueBool() {
		if status, err := r.client.GetLxcStatus(ctx, node, vmid); err == nil && status.Status == "running" {
			timeout := int64(60)
			upid, err := r.client.LxcShutdown(ctx, node, vmid, pveclient.LxcShutdownParams{Timeout: &timeout})
			if err != nil {
				resp.Diagnostics.AddError("Error deleting pve_container", fmt.Sprintf("shutting down container %d on %s: %s", vmid, node, err))
				return
			}
			if _, err := r.client.WaitForTask(ctx, node, upid, defaultWaitOptions()); err != nil {
				resp.Diagnostics.AddError("Error waiting for pve_container shutdown", fmt.Sprintf("waiting for container %d shutdown on %s: %s", vmid, node, err))
				return
			}
			if status, err := r.client.GetLxcStatus(ctx, node, vmid); err == nil && status.Status == "running" {
				stopUPID, err := r.client.LxcStop(ctx, node, vmid)
				if err != nil {
					resp.Diagnostics.AddError("Error deleting pve_container", fmt.Sprintf("stopping container %d on %s after failed shutdown: %s", vmid, node, err))
					return
				}
				if _, err := r.client.WaitForTask(ctx, node, stopUPID, defaultWaitOptions()); err != nil {
					resp.Diagnostics.AddError("Error waiting for pve_container stop", fmt.Sprintf("waiting for container %d stop on %s: %s", vmid, node, err))
					return
				}
			}
		}
	}

	upid, err := r.client.DeleteLxcContainer(ctx, node, vmid, pveclient.DeleteLxcContainerParams{})
	if err != nil {
		if isPVEClientNotFound(err) {
			// Already absent counts as deleted.
			return
		}
		resp.Diagnostics.AddError("Error deleting pve_container", fmt.Sprintf("destroying container %d on %s: %s", vmid, node, err))
		return
	}
	if _, err := r.client.WaitForTask(ctx, node, upid, defaultWaitOptions()); err != nil {
		resp.Diagnostics.AddError("Error waiting for pve_container delete", fmt.Sprintf("waiting for container %d destroy on %s: %s", vmid, node, err))
	}
}

// ImportState parses an import ID of the form `<node>/<vmid>`.
func (r *pveContainerResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	node, vmid, err := containerSplitImportID(req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Invalid pve_container import ID", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("node"), node)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("vmid"), vmid)...)
}

// containerSplitImportID splits `<node>/<vmid>`, validating the vmid.
func containerSplitImportID(id string) (string, int64, error) {
	node, raw, found := strings.Cut(id, "/")
	if !found || node == "" || raw == "" {
		return "", 0, fmt.Errorf("import ID must be in the form `<node>/<vmid>`, got %q", id)
	}
	vmid, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return "", 0, fmt.Errorf("import ID must be in the form `<node>/<vmid>`, got %q: %s", id, err)
	}
	return node, vmid, nil
}

// readInto refreshes the model from config, status, pending, and (when
// running) the interface IP discovery. Write-only attributes
// (ostemplate, password, ssh_public_keys, stop_on_destroy, clone) are
// never touched.
func (r *pveContainerResource) readInto(ctx context.Context, m *pveContainerResourceModel) error {
	node := m.Node.ValueString()
	vmid := m.VMID.ValueInt64()
	cfg, err := r.client.GetLxcConfig(ctx, node, vmid)
	if err != nil {
		return err
	}
	status, err := r.client.GetLxcStatus(ctx, node, vmid)
	if err != nil {
		return err
	}
	m.Hostname = nodeNetworkStringToTF(cfg.Hostname)
	m.Description = nodeNetworkStringToTF(cfg.Description)
	m.Tags = nodeNetworkStringToTF(cfg.Tags)
	m.Onboot = nodeNetworkBoolPtrToTF(cfg.Onboot)
	m.Protection = nodeNetworkBoolPtrToTF(cfg.Protection)
	m.Template = nodeNetworkBoolPtrToTF(cfg.Template)
	m.Unprivileged = nodeNetworkBoolPtrToTF(cfg.Unprivileged)
	m.Cores = containerInt64PtrToTF(cfg.Cores)
	m.Memory = containerInt64PtrToTF(cfg.Memory)
	m.Swap = containerInt64PtrToTF(cfg.Swap)
	m.Nameserver = nodeNetworkStringToTF(cfg.Nameserver)
	m.Searchdomain = nodeNetworkStringToTF(cfg.Searchdomain)
	m.MountPoints = containerMountPointsFromConfig(cfg)
	m.Status = types.StringValue(status.Status)
	m.Started = types.BoolValue(status.Status == "running")
	pending, err := r.client.GetLxcPending(ctx, node, vmid)
	if err != nil {
		return err
	}
	m.PendingChanges = listStringToTF(containerPendingKeys(pending))
	if status.Status == "running" {
		ifaces, err := r.client.GetLxcInterfaces(ctx, node, vmid)
		if err != nil {
			return err
		}
		m.Interfaces = containerInterfacesFromWire(ifaces)
	} else {
		m.Interfaces = nil
	}
	return nil
}

// containerInt64PtrToTF maps a nil pointer to a null integer.
func containerInt64PtrToTF(v *int64) types.Int64 {
	if v == nil {
		return types.Int64Null()
	}
	return types.Int64Value(*v)
}
