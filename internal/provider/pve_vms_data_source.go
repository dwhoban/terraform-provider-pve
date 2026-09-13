// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// Ensure the framework interfaces are satisfied.
var (
	_ datasource.DataSource              = &pveVmsDataSource{}
	_ datasource.DataSourceWithConfigure = &pveVmsDataSource{}
)

// NewPveVmsDataSource returns the data source implementation.
func NewPveVmsDataSource() datasource.DataSource {
	return &pveVmsDataSource{}
}

// pveVmsDataSource reads the per-node virtual machine index
// (GET /nodes/{node}/qemu).
type pveVmsDataSource struct {
	client *pveclient.Client
}

// pveVmsDataSourceModel is the Terraform-facing shape.
type pveVmsDataSourceModel struct {
	ID   types.String              `tfsdk:"id"`
	Node types.String              `tfsdk:"node"`
	Full types.Bool                `tfsdk:"full"`
	VMs  []pveVmsDataSourceModelVM `tfsdk:"vms"`
}

// pveVmsDataSourceModelVM mirrors one row of the /nodes/{node}/qemu
// response. Every documented field is exposed; fields PVE omits for a
// given VM (or when the full flag is not set) decode as empty values.
type pveVmsDataSourceModelVM struct {
	CPU                types.Float64 `tfsdk:"cpu"`
	CPUs               types.Float64 `tfsdk:"cpus"`
	DiskRead           types.Int64   `tfsdk:"diskread"`
	DiskWrite          types.Int64   `tfsdk:"diskwrite"`
	Lock               types.String  `tfsdk:"lock"`
	MaxDisk            types.Int64   `tfsdk:"maxdisk"`
	MaxMem             types.Int64   `tfsdk:"maxmem"`
	Mem                types.Int64   `tfsdk:"mem"`
	MemHost            types.Int64   `tfsdk:"memhost"`
	Name               types.String  `tfsdk:"name"`
	NetIn              types.Int64   `tfsdk:"netin"`
	NetOut             types.Int64   `tfsdk:"netout"`
	PID                types.Int64   `tfsdk:"pid"`
	PressureCPUFull    types.Float64 `tfsdk:"pressurecpufull"`
	PressureCPUSome    types.Float64 `tfsdk:"pressurecpusome"`
	PressureIOFull     types.Float64 `tfsdk:"pressureiofull"`
	PressureIOSome     types.Float64 `tfsdk:"pressureiosome"`
	PressureMemoryFull types.Float64 `tfsdk:"pressurememoryfull"`
	PressureMemorySome types.Float64 `tfsdk:"pressurememorysome"`
	QMPStatus          types.String  `tfsdk:"qmpstatus"`
	RunningMachine     types.String  `tfsdk:"running_machine"`
	RunningQEMU        types.String  `tfsdk:"running_qemu"`
	Serial             types.Bool    `tfsdk:"serial"`
	Status             types.String  `tfsdk:"status"`
	Tags               types.String  `tfsdk:"tags"`
	Template           types.Bool    `tfsdk:"template"`
	Uptime             types.Int64   `tfsdk:"uptime"`
	VMID               types.Int64   `tfsdk:"vmid"`
}

// Metadata implements datasource.DataSource.
func (d *pveVmsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveVms
}

// Schema implements datasource.DataSource.
func (d *pveVmsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists the virtual machines on one Proxmox VE node as reported by `GET /nodes/{node}/qemu`. " +
			"Configuration options not modeled by this provider (disk, memory, network settings, and so on) are never sent " +
			"to the API by this data source and remain untouched.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The node name, mirroring the `node` attribute.",
			},
			"node": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The cluster node name whose virtual machines to list.",
			},
			"full": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Request the full status of running VMs. Per the API, `full=1` adds the fields `memhost`, `pid`, `qmpstatus`, `running_machine`, `running_qemu`, and the `pressure*` gauges for active VMs; those fields are empty without it.",
			},
			"vms": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Virtual machines on the node, ordered as returned by the API. Fields that PVE omits for a given VM are empty.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"cpu":                schema.Float64Attribute{Computed: true, MarkdownDescription: "Current CPU usage."},
						"cpus":               schema.Float64Attribute{Computed: true, MarkdownDescription: "Maximum usable CPUs."},
						"diskread":           schema.Int64Attribute{Computed: true, MarkdownDescription: "Bytes the guest read from its block devices since start (not available for all storage types)."},
						"diskwrite":          schema.Int64Attribute{Computed: true, MarkdownDescription: "Bytes the guest wrote to its block devices since start (not available for all storage types)."},
						"lock":               schema.StringAttribute{Computed: true, MarkdownDescription: "The current config lock, if any."},
						"maxdisk":            schema.Int64Attribute{Computed: true, MarkdownDescription: "Root disk size in bytes."},
						"maxmem":             schema.Int64Attribute{Computed: true, MarkdownDescription: "Maximum memory in bytes."},
						"mem":                schema.Int64Attribute{Computed: true, MarkdownDescription: "Currently used memory in bytes, from ballooning when available; does not include KSM effects."},
						"memhost":            schema.Int64Attribute{Computed: true, MarkdownDescription: "Current memory usage on the host, in bytes (requires `full`)."},
						"name":               schema.StringAttribute{Computed: true, MarkdownDescription: "VM (host)name."},
						"netin":              schema.Int64Attribute{Computed: true, MarkdownDescription: "Traffic in bytes sent to the guest over the network since start."},
						"netout":             schema.Int64Attribute{Computed: true, MarkdownDescription: "Traffic in bytes sent from the guest over the network since start."},
						"pid":                schema.Int64Attribute{Computed: true, MarkdownDescription: "PID of the QEMU process, if the VM is running (requires `full`)."},
						"pressurecpufull":    schema.Float64Attribute{Computed: true, MarkdownDescription: "CPU Full pressure stall average over the last 10 seconds (requires `full`)."},
						"pressurecpusome":    schema.Float64Attribute{Computed: true, MarkdownDescription: "CPU Some pressure stall average over the last 10 seconds (requires `full`)."},
						"pressureiofull":     schema.Float64Attribute{Computed: true, MarkdownDescription: "IO Full pressure stall average over the last 10 seconds (requires `full`)."},
						"pressureiosome":     schema.Float64Attribute{Computed: true, MarkdownDescription: "IO Some pressure stall average over the last 10 seconds (requires `full`)."},
						"pressurememoryfull": schema.Float64Attribute{Computed: true, MarkdownDescription: "Memory Full pressure stall average over the last 10 seconds (requires `full`)."},
						"pressurememorysome": schema.Float64Attribute{Computed: true, MarkdownDescription: "Memory Some pressure stall average over the last 10 seconds (requires `full`)."},
						"qmpstatus":          schema.StringAttribute{Computed: true, MarkdownDescription: "VM run state from the `query-status` QMP monitor command (requires `full`)."},
						"running_machine":    schema.StringAttribute{Computed: true, MarkdownDescription: "The currently running machine type, if running (requires `full`)."},
						"running_qemu":       schema.StringAttribute{Computed: true, MarkdownDescription: "The QEMU version the VM is currently using, if running (requires `full`)."},
						"serial":             schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the guest has a serial device configured."},
						"status":             schema.StringAttribute{Computed: true, MarkdownDescription: "QEMU process status. Must be one of: `stopped`, `running`."},
						"tags":               schema.StringAttribute{Computed: true, MarkdownDescription: "The currently configured tags, if any."},
						"template":           schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the guest is a template."},
						"uptime":             schema.Int64Attribute{Computed: true, MarkdownDescription: "Uptime in seconds."},
						"vmid":               schema.Int64Attribute{Computed: true, MarkdownDescription: "The unique VM ID (100 to 999999999)."},
					},
				},
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveVmsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*pveclient.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *pveclient.Client, got: %T.", req.ProviderData),
		)
		return
	}
	d.client = client
}

// Read implements datasource.DataSource.
func (d *pveVmsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveVmsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	vms, err := d.client.ListQemuVMs(ctx, data.Node.ValueString(), data.Full.ValueBool())
	if err != nil {
		resp.Diagnostics.AddError("Error reading pve_vms", fmt.Sprintf("listing virtual machines on node %s: %s", data.Node.ValueString(), err))
		return
	}
	rows := make([]pveVmsDataSourceModelVM, 0, len(vms))
	for _, vm := range vms {
		rows = append(rows, pveVmsDataSourceModelVM{
			CPU:                types.Float64Value(vm.CPU),
			CPUs:               types.Float64Value(vm.CPUs),
			DiskRead:           types.Int64Value(vm.DiskRead),
			DiskWrite:          types.Int64Value(vm.DiskWrite),
			Lock:               types.StringValue(vm.Lock),
			MaxDisk:            types.Int64Value(vm.MaxDisk),
			MaxMem:             types.Int64Value(vm.MaxMem),
			Mem:                types.Int64Value(vm.Mem),
			MemHost:            types.Int64Value(vm.MemHost),
			Name:               types.StringValue(vm.Name),
			NetIn:              types.Int64Value(vm.NetIn),
			NetOut:             types.Int64Value(vm.NetOut),
			PID:                types.Int64Value(vm.PID),
			PressureCPUFull:    types.Float64Value(vm.PressureCPUFull),
			PressureCPUSome:    types.Float64Value(vm.PressureCPUSome),
			PressureIOFull:     types.Float64Value(vm.PressureIOFull),
			PressureIOSome:     types.Float64Value(vm.PressureIOSome),
			PressureMemoryFull: types.Float64Value(vm.PressureMemoryFull),
			PressureMemorySome: types.Float64Value(vm.PressureMemorySome),
			QMPStatus:          types.StringValue(vm.QMPStatus),
			RunningMachine:     types.StringValue(vm.RunningMachine),
			RunningQEMU:        types.StringValue(vm.RunningQEMU),
			Serial:             types.BoolValue(vm.Serial),
			Status:             types.StringValue(vm.Status),
			Tags:               types.StringValue(vm.Tags),
			Template:           types.BoolValue(vm.Template),
			Uptime:             types.Int64Value(vm.Uptime),
			VMID:               types.Int64Value(vm.VMID),
		})
	}
	data.ID = data.Node
	data.VMs = rows
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
