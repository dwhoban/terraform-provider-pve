// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ datasource.DataSource              = &pveVmDataSource{}
	_ datasource.DataSourceWithConfigure = &pveVmDataSource{}
)

// NewPveVmDataSource returns the QEMU guest data source implementation.
func NewPveVmDataSource() datasource.DataSource {
	return &pveVmDataSource{}
}

// pveVmDataSource reads one QEMU guest's effective configuration and
// runtime status from GET /nodes/{node}/qemu/{vmid}/config and
// .../status/current.
type pveVmDataSource struct {
	client *pveclient.Client
}

// pveVmDataSourceModel is the Terraform-facing shape.
type pveVmDataSourceModel struct {
	ID                types.String           `tfsdk:"id"`
	VMID              types.Int64            `tfsdk:"vmid"`
	Node              types.String           `tfsdk:"node"`
	Name              types.String           `tfsdk:"name"`
	Description       types.String           `tfsdk:"description"`
	Tags              types.List             `tfsdk:"tags"`
	Onboot            types.Bool             `tfsdk:"onboot"`
	Protection        types.Bool             `tfsdk:"protection"`
	Template          types.Bool             `tfsdk:"template"`
	Agent             types.Bool             `tfsdk:"agent"`
	BIOS              types.String           `tfsdk:"bios"`
	Machine           types.String           `tfsdk:"machine"`
	OSType            types.String           `tfsdk:"ostype"`
	CPUType           types.String           `tfsdk:"cpu_type"`
	SCSIHW            types.String           `tfsdk:"scsihw"`
	Cores             types.Int64            `tfsdk:"cores"`
	Sockets           types.Int64            `tfsdk:"sockets"`
	Memory            types.Int64            `tfsdk:"memory"`
	BootOrder         types.List             `tfsdk:"boot_order"`
	Disks             []pveVmDiskModel       `tfsdk:"disks"`
	NetworkInterfaces []pveVmNetModel        `tfsdk:"network_interfaces"`
	CloudInit         *pveVmDsCloudInitModel `tfsdk:"cloud_init"`
	Status            types.String           `tfsdk:"status"`
	QMPStatus         types.String           `tfsdk:"qmpstatus"`
	Uptime            types.Int64            `tfsdk:"uptime"`
	MaxMem            types.Int64            `tfsdk:"maxmem"`
	MaxCPU            types.Int64            `tfsdk:"maxcpu"`
}

// pveVmDsCloudInitModel is the read-only cloud_init view; the password is
// write-only in PVE and never readable.
type pveVmDsCloudInitModel struct {
	User         types.String      `tfsdk:"user"`
	SearchDomain types.String      `tfsdk:"searchdomain"`
	Nameserver   types.String      `tfsdk:"nameserver"`
	SSHKeys      types.String      `tfsdk:"sshkeys"`
	Ipconfig     map[string]string `tfsdk:"ipconfig"`
}

// Metadata implements datasource.DataSource.
func (d *pveVmDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveVm
}

// Schema implements datasource.DataSource.
func (d *pveVmDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads one QEMU guest's effective configuration and runtime status " +
			"(`GET /nodes/{node}/qemu/{vmid}/config` plus `.../status/current`). Attribute names mirror " +
			"the `pve_vm` resource; unmodeled PVE config keys are not exposed.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Data-source identifier in `<node>/<vmid>` form.",
			},
			"node": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The cluster node name the guest runs on.",
			},
			"vmid": schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "The (unique) VM ID to read.",
			},
			"name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "VM name (pin `name`).",
			},
			"description": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "VM description (pin `description`).",
			},
			"tags": schema.ListAttribute{
				ElementType:         types.StringType,
				Computed:            true,
				MarkdownDescription: "Tags of the VM, split from the pin's `;`-separated `tags` string.",
			},
			"onboot": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether the VM starts during system bootup.",
			},
			"protection": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether the protection flag is set.",
			},
			"template": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether the guest is a template.",
			},
			"agent": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether the QEMU Guest Agent is enabled.",
			},
			"bios": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "BIOS implementation (`seabios` or `ovmf`).",
			},
			"machine": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "QEMU machine type.",
			},
			"ostype": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Guest operating system type.",
			},
			"cpu_type": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Emulated CPU type extracted from the pin `cpu` parameter.",
			},
			"scsihw": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "SCSI controller model.",
			},
			"cores": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Number of cores per socket.",
			},
			"sockets": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Number of CPU sockets.",
			},
			"memory": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Amount of online RAM in MiB.",
			},
			"boot_order": schema.ListAttribute{
				ElementType:         types.StringType,
				Computed:            true,
				MarkdownDescription: "Boot order devices from the pin `boot` parameter (`order=a;b`).",
			},
			"disks": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Virtual disk drives parsed from the config values.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Drive key (`scsi0`, `virtio1`, `efidisk0`, ...).",
						},
						"storage": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Storage ID backing the drive.",
						},
						"size": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Drive size in PVE disk-size syntax.",
						},
						"format": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Backing file data format.",
						},
						"cache": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Drive cache mode.",
						},
						"discard": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Discard/trim passthrough setting.",
						},
						"iothread": schema.BoolAttribute{
							Computed:            true,
							MarkdownDescription: "Whether iothreads are used.",
						},
						"ssd": schema.BoolAttribute{
							Computed:            true,
							MarkdownDescription: "Whether the drive is exposed as an SSD.",
						},
					},
				},
			},
			"network_interfaces": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Network devices parsed from the config values.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Device key (`net0`, ...).",
						},
						"model": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Network card model.",
						},
						"bridge": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Bridge the device attaches to.",
						},
						"vlan_tag": schema.Int64Attribute{
							Computed:            true,
							MarkdownDescription: "VLAN tag applied to packets.",
						},
						"firewall": schema.BoolAttribute{
							Computed:            true,
							MarkdownDescription: "Whether the firewall protects this interface.",
						},
						"macaddr": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "MAC address of the device.",
						},
						"queues": schema.Int64Attribute{
							Computed:            true,
							MarkdownDescription: "Number of packet queues.",
						},
					},
				},
			},
			"cloud_init": schema.SingleNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Cloud-init settings from the config keys. The password is write-only in PVE and never exposed.",
				Attributes: map[string]schema.Attribute{
					"user": schema.StringAttribute{
						Computed:            true,
						MarkdownDescription: "Cloud-init user name (pin `ciuser`).",
					},
					"searchdomain": schema.StringAttribute{
						Computed:            true,
						MarkdownDescription: "Cloud-init DNS search domains.",
					},
					"nameserver": schema.StringAttribute{
						Computed:            true,
						MarkdownDescription: "Cloud-init DNS server.",
					},
					"sshkeys": schema.StringAttribute{
						Computed:            true,
						MarkdownDescription: "Public SSH keys, one per line (decoded from the percent-encoded config value).",
					},
					"ipconfig": schema.MapAttribute{
						ElementType:         types.StringType,
						Computed:            true,
						MarkdownDescription: "IP configurations keyed by interface (`ipconfig0`-`ipconfig31`).",
					},
				},
			},
			"status": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "QEMU process status, `running` or `stopped`.",
			},
			"qmpstatus": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "VM run state from the QMP monitor (when running).",
			},
			"uptime": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Uptime in seconds.",
			},
			"maxmem": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Maximum memory in bytes.",
			},
			"maxcpu": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Maximum usable CPUs.",
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveVmDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*pveclient.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *pveclient.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}
	d.client = client
}

// Read implements datasource.DataSource.
func (d *pveVmDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state pveVmDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Provider not configured", "The PVE client is not configured; Configure must run before Read.")
		return
	}
	node := state.Node.ValueString()
	vmid := state.VMID.ValueInt64()

	config, err := d.client.GetQemuVMConfig(ctx, node, vmid)
	if err != nil {
		if isPVEClientNotFound(err) {
			resp.Diagnostics.AddError("QEMU VM not found", fmt.Sprintf("QEMU VM %d does not exist on node %s", vmid, node))
			return
		}
		resp.Diagnostics.AddError("Unable to read QEMU VM config", err.Error())
		return
	}
	status, err := d.client.GetQemuVMStatusCurrent(ctx, node, vmid)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read QEMU VM status", err.Error())
		return
	}

	vmConfigIntoDataSourceModel(config, status, &state)
	state.ID = types.StringValue(fmt.Sprintf("%s/%d", node, vmid))
	state.Node = types.StringValue(node)
	state.VMID = types.Int64Value(vmid)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// vmConfigIntoDataSourceModel fills the data-source model from the raw
// config and status reads.
func vmConfigIntoDataSourceModel(config map[string]json.RawMessage, status *pveclient.QemuVMStatus, m *pveVmDataSourceModel) {
	m.Name = vmTFStringFromRaw(config["name"])
	m.Description = vmTFStringFromRaw(config["description"])
	m.Tags = listStringToTF(vmTagsFromRaw(config["tags"]))
	m.Onboot = vmTFBoolFromRaw(config["onboot"])
	m.Protection = vmTFBoolFromRaw(config["protection"])
	m.Template = vmTFBoolFromRaw(config["template"])
	m.Agent = vmAgentBoolToTF(config["agent"])
	m.BIOS = vmTFStringFromRaw(config["bios"])
	m.Machine = vmTFStringFromRaw(config["machine"])
	m.OSType = vmTFStringFromRaw(config["ostype"])
	m.CPUType = vmCPUTypeToTF(config["cpu"])
	m.SCSIHW = vmTFStringFromRaw(config["scsihw"])
	m.Cores = vmTFIntFromRaw(config["cores"])
	m.Sockets = vmTFIntFromRaw(config["sockets"])
	m.Memory = vmTFIntFromRaw(config["memory"])
	m.BootOrder = listStringToTF(vmBootOrderFromRaw(config["boot"]))
	m.Disks = vmDisksFromConfig(config)
	m.NetworkInterfaces = vmNetsFromConfig(config)
	if block := vmCloudInitFromConfig(config); block != nil {
		m.CloudInit = &pveVmDsCloudInitModel{
			User:         block.User,
			SearchDomain: block.SearchDomain,
			Nameserver:   block.Nameserver,
			SSHKeys:      block.SSHKeys,
			Ipconfig:     block.Ipconfig,
		}
	}
	if status != nil {
		m.Status = types.StringValue(status.Status)
		m.QMPStatus = types.StringValue(status.QMPStatus)
		m.Uptime = types.Int64Value(status.Uptime)
		m.MaxMem = types.Int64Value(status.MaxMem)
		m.MaxCPU = types.Int64Value(status.MaxCPU)
		if m.Template.IsNull() {
			m.Template = types.BoolValue(status.Template)
		}
	}
}
