// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ resource.Resource                = &pveVmResource{}
	_ resource.ResourceWithConfigure   = &pveVmResource{}
	_ resource.ResourceWithImportState = &pveVmResource{}
)

// NewPveVmResource returns the QEMU guest resource implementation.
func NewPveVmResource() resource.Resource {
	return &pveVmResource{}
}

// pveVmResource manages one QEMU guest (POST/GET/PUT/DELETE
// /nodes/{node}/qemu/{vmid}). Changing `node` migrates the guest instead
// of recreating it; the modeled config keys are applied through the
// config PUT while unmodeled PVE config keys are never sent and never
// removed.
type pveVmResource struct {
	client *pveclient.Client
}

// pveVmResourceModel is the Terraform-facing shape.
type pveVmResourceModel struct {
	ID                    types.String         `tfsdk:"id"`
	VMID                  types.Int64          `tfsdk:"vmid"`
	Node                  types.String         `tfsdk:"node"`
	Name                  types.String         `tfsdk:"name"`
	Description           types.String         `tfsdk:"description"`
	Tags                  types.List           `tfsdk:"tags"`
	Started               types.Bool           `tfsdk:"started"`
	StopOnDestroy         types.Bool           `tfsdk:"stop_on_destroy"`
	Onboot                types.Bool           `tfsdk:"onboot"`
	Protection            types.Bool           `tfsdk:"protection"`
	Template              types.Bool           `tfsdk:"template"`
	Agent                 types.Bool           `tfsdk:"agent"`
	BIOS                  types.String         `tfsdk:"bios"`
	Machine               types.String         `tfsdk:"machine"`
	OSType                types.String         `tfsdk:"ostype"`
	Cores                 types.Int64          `tfsdk:"cores"`
	Sockets               types.Int64          `tfsdk:"sockets"`
	Memory                types.Int64          `tfsdk:"memory"`
	CPUType               types.String         `tfsdk:"cpu_type"`
	SCSIHW                types.String         `tfsdk:"scsihw"`
	BootOrder             types.List           `tfsdk:"boot_order"`
	Clone                 *pveVmCloneModel     `tfsdk:"clone"`
	Disks                 []pveVmDiskModel     `tfsdk:"disks"`
	NetworkInterfaces     []pveVmNetModel      `tfsdk:"network_interfaces"`
	CloudInit             *pveVmCloudInitModel `tfsdk:"cloud_init"`
	MigrateWithLocalDisks types.Bool           `tfsdk:"migrate_with_local_disks"`
	TargetStorage         types.String         `tfsdk:"target_storage"`
	Status                types.String         `tfsdk:"status"`
	PendingChanges        types.List           `tfsdk:"pending_changes"`
	Uptime                types.Int64          `tfsdk:"uptime"`
	MaxMem                types.Int64          `tfsdk:"maxmem"`
	MaxCPU                types.Int64          `tfsdk:"maxcpu"`
}

// Metadata implements resource.Resource.
func (r *pveVmResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveVm
}

// Schema implements resource.Resource.
func (r *pveVmResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	unmodeledNote := "Config keys outside this schema are part of the guest, but this resource never sends or deletes them."
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages one QEMU guest (KVM virtual machine) on a PVE node " +
			"(`POST/GET/PUT/DELETE /nodes/{node}/qemu/{vmid}`). Changing `node` performs a live " +
			"migration instead of recreation. " + unmodeledNote,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Resource identifier in `<node>/<vmid>` form.",
			},
			"vmid": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "The (unique) VM ID. Pin range 100-999999999. When null at create, the next free ID is allocated via `GET /cluster/nextid` and stored. Changing the VM ID forces replacement.",
				Validators: []validator.Int64{
					int64validator.Between(100, 999999999),
				},
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"node": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The cluster node name the guest runs on. Changing the node migrates the guest (`POST .../migrate` + task wait) instead of recreating it.",
			},
			"name": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Set a name for the VM (pin `name`, DNS-name format; only used on the web interface).",
			},
			"description": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Description for the VM, shown in the web interface summary (pin `description`).",
			},
			"tags": schema.ListAttribute{
				ElementType:         types.StringType,
				Optional:            true,
				MarkdownDescription: "Tags of the VM (pin `tags`, a `;`-separated tag list rendered from this ordered list). This is only meta information.",
			},
			"started": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
				MarkdownDescription: "Whether the guest should be running. Create waits for the start task when true; updates issue start/shutdown tasks on change. Templates never start. Reflects the live power state on read.",
			},
			"stop_on_destroy": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "When true, destroy issues an ACPI shutdown (then a hard stop if still running) before deleting the guest. When false the destroy call itself stops a running guest's process.",
			},
			"onboot": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Specifies whether the VM will be started during system bootup (pin `onboot`, default 0).",
			},
			"protection": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Sets the protection flag of the VM, disabling remove-VM and remove-disk operations (pin `protection`, default 0).",
			},
			"template": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Whether the guest is a template. Template conversion is create-only per the pin; changing this value forces replacement.",
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.RequiresReplace(),
				},
			},
			"agent": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Enable/disable communication with the QEMU Guest Agent (pin `agent`, rendered as `enabled=<1|0>`; default 0).",
			},
			"bios": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Select BIOS implementation. Must be one of: `seabios`, `ovmf`.",
				Validators: []validator.String{
					stringvalidator.OneOf("seabios", "ovmf"),
				},
			},
			"machine": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Specify the QEMU machine (pin `machine`), e.g. `q35`, `pc`, or a versioned type like `pc-q35-9.0`.",
			},
			"ostype": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Specify guest operating system. Must be one of: `other`, `wxp`, `w2k`, `w2k3`, `w2k8`, `wvista`, `win7`, `win8`, `win10`, `win11`, `l24`, `l26`, `solaris`.",
				Validators: []validator.String{
					stringvalidator.OneOf("other", "wxp", "w2k", "w2k3", "w2k8", "wvista", "win7", "win8", "win10", "win11", "l24", "l26", "solaris"),
				},
			},
			"cores": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "The number of cores per socket (pin `cores`, default 1). Must be at least 1.",
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
				},
			},
			"sockets": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "The number of CPU sockets (pin `sockets`, default 1). Must be at least 1.",
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
				},
			},
			"memory": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Amount of online RAM for the VM in MiB (pin `memory` `current`, default 512). Must be at least 16.",
				Validators: []validator.Int64{
					int64validator.AtLeast(16),
				},
			},
			"cpu_type": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Emulated CPU type (pin `cpu` parameter, rendered as `cputype=<value>`), e.g. `kvm64`, `x86-64-v2-AES`, `host`, or a custom model.",
			},
			"scsihw": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "SCSI controller model. Must be one of: `lsi`, `lsi53c810`, `virtio-scsi-pci`, `virtio-scsi-single`, `megasas`, `pvscsi`.",
				Validators: []validator.String{
					stringvalidator.OneOf("lsi", "lsi53c810", "virtio-scsi-pci", "virtio-scsi-single", "megasas", "pvscsi"),
				},
			},
			"boot_order": schema.ListAttribute{
				ElementType:         types.StringType,
				Optional:            true,
				MarkdownDescription: "Guest boot order as an ordered device list (pin `boot`, rendered as `order=<dev>;<dev>`), e.g. `[\"scsi0\", \"net0\"]`.",
			},
			"migrate_with_local_disks": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Enable live storage migration for local disks when the guest is migrated to another node (pin `with-local-disks`).",
			},
			"target_storage": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Migration target storage mapping (pin `targetstorage`): a single storage ID maps all source storages to it; the special value `1` maps each source storage to itself.",
			},
			"status": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "QEMU process status, `running` or `stopped`.",
			},
			"pending_changes": schema.ListAttribute{
				ElementType:         types.StringType,
				Computed:            true,
				MarkdownDescription: "Config keys with staged (not yet applied) changes or pending deletes from `GET .../pending`. Pending values are merged into this resource's computed view so drift surfaces before the next apply.",
			},
			"uptime": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Uptime of the guest in seconds (0 when stopped).",
			},
			"maxmem": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Maximum memory in bytes, from the status read.",
			},
			"maxcpu": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Maximum usable CPUs (pin status field `cpus`).",
			},
			"clone": schema.SingleNestedAttribute{
				Optional:            true,
				MarkdownDescription: "Create the guest by cloning an existing VM or template instead of fresh creation (`POST .../clone`). Create-only: the block cannot be added or changed after creation. All non-clone attributes are applied to the clone via a config PUT after the clone task finishes; the `disks` list is ignored in clone mode because the clone copies the source disks.",
				Attributes: map[string]schema.Attribute{
					"source_vmid": schema.Int64Attribute{
						Required:            true,
						MarkdownDescription: "VMID of the source VM or template to clone (pin `vmid` of the clone endpoint).",
					},
					"name": schema.StringAttribute{
						Optional:            true,
						MarkdownDescription: "Set a name for the new VM (pin clone `name`).",
					},
					"full": schema.BoolAttribute{
						Optional:            true,
						Computed:            true,
						Default:             booldefault.StaticBool(true),
						MarkdownDescription: "Create a full copy of all disks (pin `full`). PVE defaults to linked clones for templates when false; disk format below only applies to full clones.",
					},
					"storage": schema.StringAttribute{
						Optional:            true,
						MarkdownDescription: "Target storage for a full clone (pin clone `storage`).",
					},
					"format": schema.StringAttribute{
						Optional:            true,
						MarkdownDescription: "Target disk format for a full clone. Must be one of: `raw`, `qcow2`, `vmdk`.",
						Validators: []validator.String{
							stringvalidator.OneOf("raw", "qcow2", "vmdk"),
						},
					},
					"pool": schema.StringAttribute{
						Optional:            true,
						MarkdownDescription: "Add the new VM to the specified pool (pin clone `pool`).",
					},
				},
			},
			"disks": schema.ListNestedAttribute{
				Optional:            true,
				MarkdownDescription: "Ordered virtual disk drives (pin `ide[n]`/`sata[n]`/`scsi[n]`/`virtio[n]`/`efidisk0`). New drives allocate `storage:size` volumes; size changes only grow (`PUT .../resize`); storage moves issue `POST .../move_disk`; removals delete the config entry (the volume becomes an `unusedN` disk). Sizes are allocated in whole GiB.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "Drive key, one of `ide0`-`ide3`, `sata0`-`sata5`, `scsi0`-`scsi30`, `virtio0`-`virtio15`, or `efidisk0`.",
							Validators: []validator.String{
								stringvalidator.RegexMatches(vmDiskKeyRe, "must be a valid drive id (ide0-3, sata0-5, scsi0-30, virtio0-15, or efidisk0)"),
							},
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
							},
						},
						"storage": schema.StringAttribute{
							Optional:            true,
							MarkdownDescription: "Storage ID backing the drive. Required when the drive allocates a new volume.",
						},
						"size": schema.StringAttribute{
							Optional:            true,
							MarkdownDescription: "Drive size in PVE disk-size syntax (e.g. `32G`, `512M`). Growth-only: shrinking issues an error. New volumes are allocated in whole GiB.",
						},
						"format": schema.StringAttribute{
							Optional:            true,
							MarkdownDescription: "The drive's backing file data format. Must be one of: `raw`, `qcow`, `qed`, `qcow2`, `vmdk`, `cloop`.",
							Validators: []validator.String{
								stringvalidator.OneOf("raw", "qcow", "qed", "qcow2", "vmdk", "cloop"),
							},
						},
						"cache": schema.StringAttribute{
							Optional:            true,
							MarkdownDescription: "The drive's cache mode. Must be one of: `none`, `writethrough`, `writeback`, `unsafe`, `directsync`.",
							Validators: []validator.String{
								stringvalidator.OneOf("none", "writethrough", "writeback", "unsafe", "directsync"),
							},
						},
						"discard": schema.StringAttribute{
							Optional:            true,
							MarkdownDescription: "Whether discard/trim requests pass to the underlying storage. Must be one of: `ignore`, `on`.",
							Validators: []validator.String{
								stringvalidator.OneOf("ignore", "on"),
							},
						},
						"iothread": schema.BoolAttribute{
							Optional:            true,
							MarkdownDescription: "Whether to use iothreads for this drive.",
						},
						"ssd": schema.BoolAttribute{
							Optional:            true,
							MarkdownDescription: "Whether to expose this drive as an SSD (only supported on SCSI and SATA drives).",
						},
					},
				},
			},
			"network_interfaces": schema.ListNestedAttribute{
				Optional:            true,
				MarkdownDescription: "Ordered network devices (pin `net[n]`). Removals delete the config entry; changes re-render the device value. A null `macaddr` lets PVE generate one at create.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "Device key `net0`-`net31`.",
							Validators: []validator.String{
								stringvalidator.RegexMatches(vmNetKeyRe, "must be a valid network device id (net0 through net31)"),
							},
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
							},
						},
						"model": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "Network card model. Must be one of: `e1000`, `e1000-82540em`, `e1000-82544gc`, `e1000-82545em`, `e1000e`, `i82551`, `i82557b`, `i82559er`, `ne2k_isa`, `ne2k_pci`, `pcnet`, `rtl8139`, `virtio`, `vmxnet3`.",
							Validators: []validator.String{
								stringvalidator.OneOf("e1000", "e1000-82540em", "e1000-82544gc", "e1000-82545em", "e1000e", "i82551", "i82557b", "i82559er", "ne2k_isa", "ne2k_pci", "pcnet", "rtl8139", "virtio", "vmxnet3"),
							},
						},
						"bridge": schema.StringAttribute{
							Optional:            true,
							MarkdownDescription: "Bridge to attach the device to (e.g. `vmbr0`). Omitting it creates a NATed user-mode network device.",
						},
						"vlan_tag": schema.Int64Attribute{
							Optional:            true,
							MarkdownDescription: "VLAN tag to apply to packets on this interface (pin `tag`). Must be between 1 and 4094.",
							Validators: []validator.Int64{
								int64validator.Between(1, 4094),
							},
						},
						"firewall": schema.BoolAttribute{
							Optional:            true,
							MarkdownDescription: "Whether this interface should be protected by the firewall.",
						},
						"macaddr": schema.StringAttribute{
							Optional:            true,
							Computed:            true,
							MarkdownDescription: "MAC address (pin `macaddr`), auto-generated by PVE when unset. Must be unique within the network.",
						},
						"queues": schema.Int64Attribute{
							Optional:            true,
							MarkdownDescription: "Number of packet queues used on the device. Must be between 0 and 64.",
							Validators: []validator.Int64{
								int64validator.Between(0, 64),
							},
						},
					},
				},
			},
			"cloud_init": schema.SingleNestedAttribute{
				Optional:            true,
				MarkdownDescription: "Cloud-init parameters rendered to the pin's `ciuser`/`cipassword`/`searchdomain`/`nameserver`/`sshkeys`/`ipconfig[n]` config keys. Requires a cloud-init enabled image.",
				Attributes: map[string]schema.Attribute{
					"user": schema.StringAttribute{
						Optional:            true,
						MarkdownDescription: "Cloud-init user name to change ssh keys and password for instead of the image's default user (pin `ciuser`).",
					},
					"password": schema.StringAttribute{
						Optional:            true,
						Sensitive:           true,
						MarkdownDescription: "Cloud-init password (pin `cipassword`). Write-only: PVE never returns it, so it is applied when set and never removed.",
					},
					"searchdomain": schema.StringAttribute{
						Optional:            true,
						MarkdownDescription: "Cloud-init DNS search domains.",
					},
					"nameserver": schema.StringAttribute{
						Optional:            true,
						MarkdownDescription: "Cloud-init DNS server IP address.",
					},
					"sshkeys": schema.StringAttribute{
						Optional:            true,
						MarkdownDescription: "Public SSH keys, one per line (OpenSSH format). Percent-encoded on the wire per the pin's `urlencoded` format.",
					},
					"ipconfig": schema.MapAttribute{
						ElementType:         types.StringType,
						Optional:            true,
						MarkdownDescription: "IP configurations keyed by interface (`ipconfig0`-`ipconfig31`), e.g. `ipconfig0 = \"ip=10.0.0.5/24,gw=10.0.0.1\"`. Use `dhcp` for DHCP.",
					},
				},
			},
		},
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveVmResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = vmConfigureResource(req, resp)
}

// Create implements resource.Resource.
func (r *pveVmResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveVmResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Provider not configured", "The PVE client is not configured; Configure must run before Create.")
		return
	}

	if err := r.allocateVMID(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to allocate a VM ID", err.Error())
		return
	}
	vmid := plan.VMID.ValueInt64()
	node := plan.Node.ValueString()
	tflog.Info(ctx, "creating pve_vm", map[string]any{"node": node, "vmid": vmid})

	if plan.Clone != nil {
		if !r.cloneInto(ctx, node, vmid, &plan, &resp.Diagnostics) {
			return
		}
	} else {
		input, diags := vmResourceConfigInput(&plan, plan.Disks)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		upid, err := r.client.CreateQemuVM(ctx, node, vmid, input)
		if err != nil {
			resp.Diagnostics.AddError("Unable to create QEMU VM", err.Error())
			return
		}
		if _, err := r.client.WaitForTask(ctx, node, upid, pveclient.WaitForTaskOptions{}); err != nil {
			resp.Diagnostics.AddError("QEMU VM create task failed", err.Error())
			return
		}
	}

	plan.ID = types.StringValue(fmt.Sprintf("%s/%d", node, vmid))
	if !isPveVmTemplate(plan.Template) {
		if err := vmStartIfNeeded(ctx, r.client, node, vmid, plan.Started.ValueBool()); err != nil {
			resp.Diagnostics.AddError("Unable to start QEMU VM", err.Error())
			return
		}
	}

	if err := r.readInto(ctx, node, vmid, &plan); err != nil {
		if isPVEClientNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read QEMU VM after create", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read implements resource.Resource.
func (r *pveVmResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveVmResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	node := state.Node.ValueString()
	vmid := state.VMID.ValueInt64()
	if err := r.readInto(ctx, node, vmid, &state); err != nil {
		if isPVEClientNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read QEMU VM", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource: migration on node change, config
// PUT for key changes, move_disk/resize for storage and size changes, and
// power actions for started changes.
func (r *pveVmResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var state, plan pveVmResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Provider not configured", "The PVE client is not configured; Configure must run before Update.")
		return
	}

	if vmCloneChanged(state.Clone, plan.Clone) {
		resp.Diagnostics.AddError("Clone block is immutable", "The clone block is create-only; changing or removing it forces replacement of the guest.")
		return
	}

	vmid := plan.VMID.ValueInt64()
	targetNode := plan.Node.ValueString()
	sourceNode := state.Node.ValueString()

	if targetNode != sourceNode {
		if err := r.migrateInto(ctx, sourceNode, targetNode, vmid, &plan); err != nil {
			resp.Diagnostics.AddError("Unable to migrate QEMU VM", err.Error())
			return
		}
	}

	if err := r.applyConfigDiff(ctx, targetNode, vmid, &state, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to update QEMU VM configuration", err.Error())
		return
	}

	if !plan.Started.Equal(state.Started) {
		if err := vmStartIfNeeded(ctx, r.client, targetNode, vmid, plan.Started.ValueBool()); err != nil {
			resp.Diagnostics.AddError("Unable to change QEMU VM power state", err.Error())
			return
		}
	}

	if err := r.readInto(ctx, targetNode, vmid, &plan); err != nil {
		if isPVEClientNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read QEMU VM after update", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete implements resource.Resource. An optional ACPI shutdown runs
// first when stop_on_destroy is set, then the destroy task removes the
// guest. Deleting an already-absent guest succeeds.
func (r *pveVmResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveVmResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Provider not configured", "The PVE client is not configured; Configure must run before Delete.")
		return
	}
	node := state.Node.ValueString()
	vmid := state.VMID.ValueInt64()

	if state.StopOnDestroy.ValueBool() && !isPveVmTemplate(state.Template) {
		timeout := int64(60)
		if err := vmEnsureStopped(ctx, r.client, node, vmid, &timeout); err != nil {
			resp.Diagnostics.AddError("Unable to stop QEMU VM before destroy", err.Error())
			return
		}
	}

	upid, err := r.client.DeleteQemuVM(ctx, node, vmid, pveclient.QemuVMDeleteOptions{})
	if isPVEClientNotFound(err) {
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to delete QEMU VM", err.Error())
		return
	}
	if _, err := r.client.WaitForTask(ctx, node, upid, pveclient.WaitForTaskOptions{}); err != nil {
		resp.Diagnostics.AddError("QEMU VM destroy task failed", err.Error())
		return
	}
}

// ImportState implements resource.ResourceWithImportState. The import ID
// is `<node>/<vmid>`.
func (r *pveVmResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	node, vmid, err := vmParseImportID(req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Unable to import QEMU VM", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("node"), node)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("vmid"), vmid)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), fmt.Sprintf("%s/%d", node, vmid))...)
}

// cloneInto runs the clone-then-configure create path. It returns false
// when diagnostics carry the failure.
func (r *pveVmResource) cloneInto(ctx context.Context, node string, vmid int64, plan *pveVmResourceModel, diags *diag.Diagnostics) bool {
	full := true
	if !plan.Clone.Full.IsNull() {
		full = plan.Clone.Full.ValueBool()
	}
	opts := pveclient.QemuVMCloneOptions{
		Name:    plan.Clone.Name.ValueString(),
		Full:    &full,
		Storage: plan.Clone.Storage.ValueString(),
		Format:  plan.Clone.Format.ValueString(),
		Pool:    plan.Clone.Pool.ValueString(),
	}
	upid, err := r.client.CloneQemuVM(ctx, node, plan.Clone.SourceVmid.ValueInt64(), vmid, opts)
	if err != nil {
		diags.AddError("Unable to clone QEMU VM", err.Error())
	}
	if _, err := r.client.WaitForTask(ctx, node, upid, pveclient.WaitForTaskOptions{}); err != nil {
		diags.AddError("QEMU VM clone task failed", err.Error())
		return false
	}
	// Clone mode copies the source disks, so apply everything except the
	// disks list.
	input, cloneDiags := vmResourceConfigInput(plan, nil)
	diags.Append(cloneDiags...)
	if cloneDiags.HasError() {
		return false
	}
	if err := r.client.UpdateQemuVMConfig(ctx, node, vmid, input, nil); err != nil {
		diags.AddError("Unable to configure cloned QEMU VM", err.Error())
		return false
	}
	return true
}

// migrateInto migrates the guest from sourceNode to targetNode, choosing
// online migration when the guest is running.
func (r *pveVmResource) migrateInto(ctx context.Context, sourceNode, targetNode string, vmid int64, plan *pveVmResourceModel) error {
	online := false
	if status, err := r.client.GetQemuVMStatusCurrentMinimal(ctx, sourceNode, vmid); err == nil {
		online = status.Status == "running"
	}
	opts := pveclient.QemuVMMigrateOptions{
		Target: targetNode,
		Online: &online,
	}
	if !plan.MigrateWithLocalDisks.IsNull() {
		withLocal := plan.MigrateWithLocalDisks.ValueBool()
		opts.WithLocalDisks = &withLocal
	}
	if !plan.TargetStorage.IsNull() {
		opts.TargetStorage = plan.TargetStorage.ValueString()
	}
	upid, err := r.client.MigrateQemuVM(ctx, sourceNode, vmid, opts)
	if err != nil {
		return fmt.Errorf("migrate from %s to %s: %w", sourceNode, targetNode, err)
	}
	if _, err := r.client.WaitForTask(ctx, sourceNode, upid, pveclient.WaitForTaskOptions{}); err != nil {
		return fmt.Errorf("migration task from %s to %s: %w", sourceNode, targetNode, err)
	}
	return nil
}

// applyConfigDiff computes and applies the difference between state and
// plan: one config PUT for simple keys, cloud-init, new/changed drives,
// and network devices, plus move_disk and resize calls for storage moves
// and size growth.
func (r *pveVmResource) applyConfigDiff(ctx context.Context, node string, vmid int64, state, plan *pveVmResourceModel) error {
	live, err := r.client.GetQemuVMConfig(ctx, node, vmid)
	if err != nil {
		return fmt.Errorf("read live config of QEMU VM %d on node %s: %w", vmid, node, err)
	}

	input, deletes := vmDiffModel(state, plan, live)
	moves, resizes, driveDiags := vmDiffDriveOps(state.Disks, plan.Disks, live)
	if driveDiags.HasError() {
		return fmt.Errorf("%s", vmDiagsText(driveDiags))
	}
	if vmConfigInputNeedsPut(input, deletes) {
		if err := r.client.UpdateQemuVMConfig(ctx, node, vmid, input, deletes); err != nil {
			return err
		}
	}

	for _, op := range moves {
		upid, err := r.client.MoveQemuDisk(ctx, node, vmid, pveclient.QemuVMMoveDiskOptions{
			Disk:    op.Disk,
			Storage: op.Storage,
			Format:  op.Format,
		})
		if err != nil {
			return fmt.Errorf("move disk %s of QEMU VM %d to storage %s: %w", op.Disk, vmid, op.Storage, err)
		}
		if _, err := r.client.WaitForTask(ctx, node, upid, pveclient.WaitForTaskOptions{}); err != nil {
			return fmt.Errorf("move disk %s task: %w", op.Disk, err)
		}
	}
	for _, op := range resizes {
		upid, err := r.client.ResizeQemuDisk(ctx, node, vmid, op.Disk, op.Size, "")
		if err != nil {
			return fmt.Errorf("resize disk %s of QEMU VM %d to %s: %w", op.Disk, vmid, op.Size, err)
		}
		if _, err := r.client.WaitForTask(ctx, node, upid, pveclient.WaitForTaskOptions{}); err != nil {
			return fmt.Errorf("resize disk %s task: %w", op.Disk, err)
		}
	}
	return nil
}

// readInto refreshes the model from the node's config, status, and
// pending reads.
func (r *pveVmResource) readInto(ctx context.Context, node string, vmid int64, m *pveVmResourceModel) error {
	config, err := r.client.GetQemuVMConfig(ctx, node, vmid)
	if err != nil {
		return fmt.Errorf("read QEMU VM %d config on node %s: %w", vmid, node, err)
	}
	status, err := r.client.GetQemuVMStatusCurrent(ctx, node, vmid)
	if err != nil {
		return fmt.Errorf("read QEMU VM %d status on node %s: %w", vmid, node, err)
	}
	pending, err := r.client.GetQemuVMPending(ctx, node, vmid)
	if err != nil {
		return fmt.Errorf("read QEMU VM %d pending changes on node %s: %w", vmid, node, err)
	}
	pendingKeys := vmMergePending(config, pending)

	vmConfigIntoResourceModel(config, status, m)
	m.ID = types.StringValue(fmt.Sprintf("%s/%d", node, vmid))
	m.Node = types.StringValue(node)
	m.VMID = types.Int64Value(vmid)
	m.PendingChanges = listStringToTF(pendingKeys)
	return nil
}

// isPveVmTemplate reports whether the model marks the guest a template.
func isPveVmTemplate(template types.Bool) bool {
	return !template.IsNull() && template.ValueBool()
}

// vmStartIfNeeded starts or stops the guest to match desired.
func vmStartIfNeeded(ctx context.Context, client *pveclient.Client, node string, vmid int64, desired bool) error {
	if desired {
		upid, err := client.QemuVMStart(ctx, node, vmid, nil)
		if err != nil {
			return fmt.Errorf("start QEMU VM %d on node %s: %w", vmid, node, err)
		}
		if _, err := client.WaitForTask(ctx, node, upid, pveclient.WaitForTaskOptions{}); err != nil {
			return fmt.Errorf("start task for QEMU VM %d: %w", vmid, err)
		}
		return nil
	}
	return vmEnsureStopped(ctx, client, node, vmid, nil)
}

// vmConfigInputNeedsPut reports whether the diff produced anything to
// send: any set key or delete key warrants a config PUT.
func vmConfigInputNeedsPut(input pveclient.QemuVMConfigInput, deletes []string) bool {
	return len(deletes) > 0 || input.Name != nil || input.Description != nil || input.Tags != nil ||
		input.BootOrder != nil || input.Onboot != nil || input.Protection != nil || input.Template != nil ||
		input.Agent != nil || input.BIOS != nil || input.Machine != nil || input.OSType != nil ||
		input.CPUType != nil || input.SCSIHW != nil || input.Cores != nil || input.Sockets != nil ||
		input.MemoryMiB != nil || input.CIUser != nil || input.CIPassword != nil || input.CISearchDomain != nil ||
		input.CINameserver != nil || input.CISSHKeys != nil || len(input.CIIpconfigs) > 0 ||
		len(input.Drives) > 0 || len(input.Networks) > 0
}

// vmConfigureResource extracts the configured PVE client (shared by the
// vm resource and data source). Nil provider data leaves the resource
// unconfigured (unit tests).
func vmConfigureResource(req resource.ConfigureRequest, resp *resource.ConfigureResponse) *pveclient.Client {
	if req.ProviderData == nil {
		return nil
	}
	client, ok := req.ProviderData.(*pveclient.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *pveclient.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return nil
	}
	return client
}

// vmCloneChanged reports whether the create-only clone block differs
// between state and plan.
func vmCloneChanged(state, plan *pveVmCloneModel) bool {
	if state == nil && plan == nil {
		return false
	}
	if state == nil || plan == nil {
		return true
	}
	return !state.SourceVmid.Equal(plan.SourceVmid) || !state.Name.Equal(plan.Name) ||
		!state.Full.Equal(plan.Full) || !state.Storage.Equal(plan.Storage) ||
		!state.Format.Equal(plan.Format) || !state.Pool.Equal(plan.Pool)
}

// vmResourceConfigInput renders the modeled config into a client input.
// drives supplies the disk models to render (nil in clone mode, where the
// clone already carries the source disks).
func vmResourceConfigInput(m *pveVmResourceModel, drives []pveVmDiskModel) (pveclient.QemuVMConfigInput, diag.Diagnostics) {
	var diags diag.Diagnostics
	input := vmSimpleKeysInput(m)
	input.CIUser, input.CIPassword, input.CISearchDomain, input.CINameserver, input.CISSHKeys, input.CIIpconfigs = vmCloudInitInput(m.CloudInit)

	if len(drives) > 0 {
		input.Drives = map[string]string{}
		for _, disk := range drives {
			rendered, err := vmRenderNewDrive(disk)
			if err != nil {
				diags.AddError("Invalid disk configuration", err.Error())
				continue
			}
			input.Drives[disk.ID.ValueString()] = rendered
		}
	}
	if len(m.NetworkInterfaces) > 0 {
		input.Networks = map[string]string{}
		for _, net := range m.NetworkInterfaces {
			input.Networks[net.ID.ValueString()] = vmRenderNet(net)
		}
	}
	return input, diags
}

// vmSimpleKeysInput renders the scalar config keys from the model.
func vmSimpleKeysInput(m *pveVmResourceModel) pveclient.QemuVMConfigInput {
	input := pveclient.QemuVMConfigInput{}
	setString := func(target **string, value types.String) {
		if !value.IsNull() {
			v := value.ValueString()
			*target = &v
		}
	}
	setString(&input.Name, m.Name)
	setString(&input.Description, m.Description)
	setString(&input.BIOS, m.BIOS)
	setString(&input.Machine, m.Machine)
	setString(&input.OSType, m.OSType)
	setString(&input.CPUType, m.CPUType)
	setString(&input.SCSIHW, m.SCSIHW)
	if !m.Cores.IsNull() {
		input.Cores = m.Cores.ValueInt64Pointer()
	}
	if !m.Sockets.IsNull() {
		input.Sockets = m.Sockets.ValueInt64Pointer()
	}
	if !m.Memory.IsNull() {
		input.MemoryMiB = m.Memory.ValueInt64Pointer()
	}
	if !m.Onboot.IsNull() {
		input.Onboot = m.Onboot.ValueBoolPointer()
	}
	if !m.Protection.IsNull() {
		input.Protection = m.Protection.ValueBoolPointer()
	}
	if !m.Template.IsNull() {
		input.Template = m.Template.ValueBoolPointer()
	}
	if !m.Agent.IsNull() {
		input.Agent = m.Agent.ValueBoolPointer()
	}
	if !m.Tags.IsNull() {
		input.Tags = listStringFromTF(m.Tags)
	}
	if !m.BootOrder.IsNull() {
		input.BootOrder = listStringFromTF(m.BootOrder)
	}
	return input
}

// vmCloudInitInput renders the cloud_init block into the client input
// fields. A nil block yields no cloud-init keys.
func vmCloudInitInput(block *pveVmCloudInitModel) (user, password, searchDomain, nameserver, sshKeys *string, ipconfigs map[string]string) {
	if block == nil {
		return nil, nil, nil, nil, nil, nil
	}
	if !block.User.IsNull() {
		v := block.User.ValueString()
		user = &v
	}
	if !block.Password.IsNull() {
		v := block.Password.ValueString()
		password = &v
	}
	if !block.SearchDomain.IsNull() {
		v := block.SearchDomain.ValueString()
		searchDomain = &v
	}
	if !block.Nameserver.IsNull() {
		v := block.Nameserver.ValueString()
		nameserver = &v
	}
	if !block.SSHKeys.IsNull() {
		v := block.SSHKeys.ValueString()
		sshKeys = &v
	}
	if len(block.Ipconfig) > 0 {
		ipconfigs = block.Ipconfig
	}
	return user, password, searchDomain, nameserver, sshKeys, ipconfigs
}

// vmDiffModel computes the config PUT between state and plan: simple
// keys, cloud-init, drives, and network devices. Removed keys land in
// deletes; null plan values for previously set keys delete them.
func vmDiffModel(state, plan *pveVmResourceModel, live map[string]json.RawMessage) (pveclient.QemuVMConfigInput, []string) {
	input, deletes := vmDiffSimpleKeys(state, plan)
	ciUser, ciPassword, ciSearchDomain, ciNameserver, ciSSHKeys, ciIpconfigs, ciDeletes := vmDiffCloudInit(state.CloudInit, plan.CloudInit, live)
	input.CIUser = ciUser
	input.CIPassword = ciPassword
	input.CISearchDomain = ciSearchDomain
	input.CINameserver = ciNameserver
	input.CISSHKeys = ciSSHKeys
	deletes = append(deletes, ciDeletes...)

	planDrives, driveDiags := vmRenderPlanDrives(plan.Disks, live)
	if !driveDiags.HasError() && len(planDrives) > 0 {
		input.Drives = planDrives
	}
	for _, disk := range state.Disks {
		id := disk.ID.ValueString()
		if !vmDriveInList(plan.Disks, id) {
			deletes = append(deletes, id)
		}
	}

	nets, netDeletes := vmDiffNetworks(state.NetworkInterfaces, plan.NetworkInterfaces)
	deletes = append(deletes, netDeletes...)
	if len(nets) > 0 {
		input.Networks = nets
	}
	if len(ciIpconfigs) > 0 {
		input.CIIpconfigs = ciIpconfigs
	}
	return input, deletes
}

// vmDiffSimpleKeys diffs the scalar modeled keys, filling the client
// input and the delete list.
func vmDiffSimpleKeys(state, plan *pveVmResourceModel) (pveclient.QemuVMConfigInput, []string) {
	input := pveclient.QemuVMConfigInput{}
	deletes := []string{}
	diffString := func(key string, stateVal, planVal types.String, target **string) {
		if stateVal.Equal(planVal) {
			return
		}
		if planVal.IsNull() {
			deletes = append(deletes, key)
			return
		}
		v := planVal.ValueString()
		*target = &v
	}
	diffBool := func(key string, stateVal, planVal types.Bool, target **bool) {
		if stateVal.Equal(planVal) {
			return
		}
		if planVal.IsNull() {
			deletes = append(deletes, key)
			return
		}
		v := planVal.ValueBool()
		*target = &v
	}
	diffInt := func(key string, stateVal, planVal types.Int64, target **int64) {
		if stateVal.Equal(planVal) {
			return
		}
		if planVal.IsNull() {
			deletes = append(deletes, key)
			return
		}
		v := planVal.ValueInt64()
		*target = &v
	}
	diffString("name", state.Name, plan.Name, &input.Name)
	diffString("description", state.Description, plan.Description, &input.Description)
	diffString("bios", state.BIOS, plan.BIOS, &input.BIOS)
	diffString("machine", state.Machine, plan.Machine, &input.Machine)
	diffString("ostype", state.OSType, plan.OSType, &input.OSType)
	diffString("cpu", state.CPUType, plan.CPUType, &input.CPUType)
	diffString("scsihw", state.SCSIHW, plan.SCSIHW, &input.SCSIHW)
	diffBool("onboot", state.Onboot, plan.Onboot, &input.Onboot)
	diffBool("protection", state.Protection, plan.Protection, &input.Protection)
	diffBool("agent", state.Agent, plan.Agent, &input.Agent)
	diffInt("cores", state.Cores, plan.Cores, &input.Cores)
	diffInt("sockets", state.Sockets, plan.Sockets, &input.Sockets)
	diffInt("memory", state.Memory, plan.Memory, &input.MemoryMiB)
	if !state.Tags.Equal(plan.Tags) {
		if plan.Tags.IsNull() {
			deletes = append(deletes, "tags")
		} else {
			input.Tags = listStringFromTF(plan.Tags)
		}
	}
	if !state.BootOrder.Equal(plan.BootOrder) {
		if plan.BootOrder.IsNull() {
			deletes = append(deletes, "boot")
		} else {
			input.BootOrder = listStringFromTF(plan.BootOrder)
		}
	}
	return input, deletes
}

// vmDiffCloudInit diffs the cloud_init block. When the block is removed
// entirely, every live cloud-init key (including ipconfig entries) is
// deleted. The password is only ever set, never deleted.
func vmDiffCloudInit(state, plan *pveVmCloudInitModel, live map[string]json.RawMessage) (user, password, searchDomain, nameserver, sshKeys *string, ipconfigs map[string]string, deletes []string) {
	switch {
	case state == nil && plan == nil:
		return nil, nil, nil, nil, nil, nil, nil
	case state == nil && plan != nil:
		u, p, sd, ns, sk, ipc := vmCloudInitInput(plan)
		return u, p, sd, ns, sk, ipc, nil
	case state != nil && plan == nil:
		for key := range live {
			if key == "ciuser" || key == "cipassword" || key == "searchdomain" || key == "nameserver" || key == "sshkeys" || vmIpconfigKeyRe.MatchString(key) {
				if key != "cipassword" {
					deletes = append(deletes, key)
				}
			}
		}
		return nil, nil, nil, nil, nil, nil, deletes
	}
	if !state.User.Equal(plan.User) {
		if plan.User.IsNull() {
			deletes = append(deletes, "ciuser")
		} else {
			v := plan.User.ValueString()
			user = &v
		}
	}
	if !plan.Password.IsNull() && !state.Password.Equal(plan.Password) {
		v := plan.Password.ValueString()
		password = &v
	}
	if !state.SearchDomain.Equal(plan.SearchDomain) {
		if plan.SearchDomain.IsNull() {
			deletes = append(deletes, "searchdomain")
		} else {
			v := plan.SearchDomain.ValueString()
			searchDomain = &v
		}
	}
	if !state.Nameserver.Equal(plan.Nameserver) {
		if plan.Nameserver.IsNull() {
			deletes = append(deletes, "nameserver")
		} else {
			v := plan.Nameserver.ValueString()
			nameserver = &v
		}
	}
	if !state.SSHKeys.Equal(plan.SSHKeys) {
		if plan.SSHKeys.IsNull() {
			deletes = append(deletes, "sshkeys")
		} else {
			v := plan.SSHKeys.ValueString()
			sshKeys = &v
		}
	}
	ipconfigs = map[string]string{}
	for key, value := range plan.Ipconfig {
		if stateValue, ok := state.Ipconfig[key]; !ok || stateValue != value {
			ipconfigs[key] = value
		}
	}
	for key := range state.Ipconfig {
		if _, ok := plan.Ipconfig[key]; !ok {
			deletes = append(deletes, key)
		}
	}
	if len(ipconfigs) == 0 {
		ipconfigs = nil
	}
	return user, password, searchDomain, nameserver, sshKeys, ipconfigs, deletes
}

// vmDriveInList reports whether a drive with the given id is present.
func vmDriveInList(disks []pveVmDiskModel, id string) bool {
	for _, disk := range disks {
		if disk.ID.ValueString() == id {
			return true
		}
	}
	return false
}

// vmDiffNetworks renders changed, new, and unchanged-but-present network
// devices, and returns delete keys for removed ones.
func vmDiffNetworks(stateNets, planNets []pveVmNetModel) (map[string]string, []string) {
	deletes := []string{}
	sets := map[string]string{}
	stateByID := map[string]pveVmNetModel{}
	for _, net := range stateNets {
		stateByID[net.ID.ValueString()] = net
	}
	planByID := map[string]pveVmNetModel{}
	for _, net := range planNets {
		planByID[net.ID.ValueString()] = net
	}
	for _, net := range planNets {
		id := net.ID.ValueString()
		stateNet, exists := stateByID[id]
		if !exists || vmNetChanged(stateNet, net) {
			sets[id] = vmRenderNet(net)
		}
	}
	for _, net := range stateNets {
		if _, exists := planByID[net.ID.ValueString()]; !exists {
			deletes = append(deletes, net.ID.ValueString())
		}
	}
	if len(sets) == 0 {
		sets = nil
	}
	return sets, deletes
}

// vmNetChanged reports whether any modeled field of a network device
// differs.
func vmNetChanged(state, plan pveVmNetModel) bool {
	return !state.Model.Equal(plan.Model) || !state.Bridge.Equal(plan.Bridge) ||
		!state.VlanTag.Equal(plan.VlanTag) || !state.Firewall.Equal(plan.Firewall) ||
		!state.MACAddr.Equal(plan.MACAddr) || !state.Queues.Equal(plan.Queues)
}

// vmResizeOp is one pending disk size growth.
type vmResizeOp struct {
	Disk string
	Size string
}

// vmMoveOp is one pending disk storage move.
type vmMoveOp struct {
	Disk    string
	Storage string
	Format  string
}

// vmDiffDriveOps compares planned drives against state: size growth
// becomes a resize, storage changes become a move, and shrinking is an
// error the pin does not support.
func vmDiffDriveOps(stateDisks, planDisks []pveVmDiskModel, live map[string]json.RawMessage) ([]vmMoveOp, []vmResizeOp, diag.Diagnostics) {
	var diags diag.Diagnostics
	moves := []vmMoveOp{}
	resizes := []vmResizeOp{}
	stateByID := map[string]pveVmDiskModel{}
	for _, disk := range stateDisks {
		stateByID[disk.ID.ValueString()] = disk
	}
	for _, disk := range planDisks {
		id := disk.ID.ValueString()
		stateDisk, exists := stateByID[id]
		if !exists {
			continue
		}
		liveDrive := vmDriveLive{}
		if raw, ok := live[id]; ok {
			if text := vmRawString(raw); text != nil {
				liveDrive = vmParseDriveValue(*text)
			}
		}
		if !disk.Storage.IsNull() && !stateDisk.Storage.IsNull() && disk.Storage.ValueString() != stateDisk.Storage.ValueString() {
			format := ""
			if !disk.Format.IsNull() {
				format = disk.Format.ValueString()
			}
			moves = append(moves, vmMoveOp{Disk: id, Storage: disk.Storage.ValueString(), Format: format})
		}
		if disk.Size.IsNull() || stateDisk.Size.IsNull() {
			continue
		}
		planBytes, err := vmDiskSizeBytes(disk.Size.ValueString())
		if err != nil {
			diags.AddError("Invalid disk size", fmt.Sprintf("disk %s: %s", id, err.Error()))
			continue
		}
		stateBytes := liveDrive.Size
		if stateBytes == "" {
			stateBytes = stateDisk.Size.ValueString()
		}
		currentBytes, err := vmDiskSizeBytes(stateBytes)
		if err != nil {
			diags.AddError("Invalid disk size", fmt.Sprintf("disk %s: %s", id, err.Error()))
			continue
		}
		switch {
		case planBytes > currentBytes:
			resizes = append(resizes, vmResizeOp{Disk: id, Size: vmFormatSizeBytes(planBytes)})
		case planBytes < currentBytes:
			diags.AddError(
				"Shrinking disks is not supported",
				fmt.Sprintf("disk %s: shrinking from %s to %s is rejected by PVE; resize to a larger size or recreate the drive", id, stateBytes, disk.Size.ValueString()),
			)
		}
	}
	if len(moves) == 0 {
		moves = nil
	}
	if len(resizes) == 0 {
		resizes = nil
	}
	return moves, resizes, diags
}

// allocateVMID fills plan.VMID from GET /cluster/nextid when it is null;
// an explicit VMID is kept.
func (r *pveVmResource) allocateVMID(ctx context.Context, plan *pveVmResourceModel) error {
	if !plan.VMID.IsNull() {
		return nil
	}
	nextID, err := r.client.GetNextID(ctx)
	if err != nil {
		return fmt.Errorf("GET /cluster/nextid: %w", err)
	}
	plan.VMID = types.Int64Value(nextID)
	return nil
}
