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

// Ensure framework interfaces are satisfied.
var (
	_ datasource.DataSource              = &pveContainersDataSource{}
	_ datasource.DataSourceWithConfigure = &pveContainersDataSource{}
)

// NewPveContainersDataSource returns the data source implementation.
func NewPveContainersDataSource() datasource.DataSource {
	return &pveContainersDataSource{}
}

// pveContainersDataSource lists the LXC containers of one node
// (GET /nodes/{node}/lxc).
type pveContainersDataSource struct {
	client *pveclient.Client
}

// pveContainersDataSourceModel is the Terraform-facing shape.
type pveContainersDataSourceModel struct {
	ID         types.String                            `tfsdk:"id"`
	Node       types.String                            `tfsdk:"node"`
	Containers []pveContainersDataSourceContainerModel `tfsdk:"containers"`
}

// pveContainersDataSourceContainerModel mirrors one row of the
// /nodes/{node}/lxc index. Optional upstream fields surface as null when
// the node does not report them (e.g. stopped containers report no
// runtime counters).
type pveContainersDataSourceContainerModel struct {
	VMID               types.Int64   `tfsdk:"vmid"`
	Status             types.String  `tfsdk:"status"`
	Name               types.String  `tfsdk:"name"`
	Template           types.Bool    `tfsdk:"template"`
	CPU                types.Float64 `tfsdk:"cpu"`
	CPUs               types.Int64   `tfsdk:"cpus"`
	Mem                types.Int64   `tfsdk:"mem"`
	MaxMem             types.Int64   `tfsdk:"maxmem"`
	MaxSwap            types.Int64   `tfsdk:"maxswap"`
	Disk               types.Int64   `tfsdk:"disk"`
	MaxDisk            types.Int64   `tfsdk:"maxdisk"`
	DiskRead           types.Int64   `tfsdk:"diskread"`
	DiskWrite          types.Int64   `tfsdk:"diskwrite"`
	NetIn              types.Int64   `tfsdk:"netin"`
	NetOut             types.Int64   `tfsdk:"netout"`
	Uptime             types.Int64   `tfsdk:"uptime"`
	Lock               types.String  `tfsdk:"lock"`
	Tags               types.String  `tfsdk:"tags"`
	PressureCPUSome    types.Float64 `tfsdk:"pressure_cpu_some"`
	PressureIOSome     types.Float64 `tfsdk:"pressure_io_some"`
	PressureIOFull     types.Float64 `tfsdk:"pressure_io_full"`
	PressureMemorySome types.Float64 `tfsdk:"pressure_memory_some"`
	PressureMemoryFull types.Float64 `tfsdk:"pressure_memory_full"`
}

// Metadata implements datasource.DataSource.
func (d *pveContainersDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveContainers
}

// Schema implements datasource.DataSource.
func (d *pveContainersDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists the LXC containers on one node as reported by `GET /nodes/{node}/lxc`. Only containers the credentials may audit (`VM.Audit` on `/vms/<vmid>`) are returned. Configuration keys the provider models elsewhere are not part of this index; unmodeled upstream fields are not surfaced.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Identifier of the listing; equals the queried `node`.",
			},
			"node": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The cluster node whose containers to list.",
			},
			"containers": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Containers on the node, ordered as PVE reports them.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"vmid":                 schema.Int64Attribute{Computed: true, MarkdownDescription: "The (unique) ID of the container."},
						"status":               schema.StringAttribute{Computed: true, MarkdownDescription: "LXC container status: `stopped` or `running`."},
						"name":                 schema.StringAttribute{Computed: true, MarkdownDescription: "Container name; null when the node does not report it."},
						"template":             schema.BoolAttribute{Computed: true, MarkdownDescription: "Determines if the guest is a template; false when absent."},
						"cpu":                  schema.Float64Attribute{Computed: true, MarkdownDescription: "Current CPU usage (fraction); null for stopped containers."},
						"cpus":                 schema.Int64Attribute{Computed: true, MarkdownDescription: "Maximum usable CPUs."},
						"mem":                  schema.Int64Attribute{Computed: true, MarkdownDescription: "Currently used memory in bytes."},
						"maxmem":               schema.Int64Attribute{Computed: true, MarkdownDescription: "Maximum memory in bytes."},
						"maxswap":              schema.Int64Attribute{Computed: true, MarkdownDescription: "Maximum SWAP memory in bytes."},
						"disk":                 schema.Int64Attribute{Computed: true, MarkdownDescription: "Root disk image space usage in bytes."},
						"maxdisk":              schema.Int64Attribute{Computed: true, MarkdownDescription: "Root disk image size in bytes."},
						"diskread":             schema.Int64Attribute{Computed: true, MarkdownDescription: "Bytes the guest read from its block devices since start; not available for all storage types."},
						"diskwrite":            schema.Int64Attribute{Computed: true, MarkdownDescription: "Bytes the guest wrote to its block devices since start; not available for all storage types."},
						"netin":                schema.Int64Attribute{Computed: true, MarkdownDescription: "Traffic in bytes sent to the guest over the network since start."},
						"netout":               schema.Int64Attribute{Computed: true, MarkdownDescription: "Traffic in bytes sent from the guest over the network since start."},
						"uptime":               schema.Int64Attribute{Computed: true, MarkdownDescription: "Uptime in seconds."},
						"lock":                 schema.StringAttribute{Computed: true, MarkdownDescription: "Current config lock, if any; null when unlocked."},
						"tags":                 schema.StringAttribute{Computed: true, MarkdownDescription: "Current configured tags, if any."},
						"pressure_cpu_some":    schema.Float64Attribute{Computed: true, MarkdownDescription: "CPU Some pressure stall average over the last 10 seconds."},
						"pressure_io_some":     schema.Float64Attribute{Computed: true, MarkdownDescription: "IO Some pressure stall average over the last 10 seconds."},
						"pressure_io_full":     schema.Float64Attribute{Computed: true, MarkdownDescription: "IO Full pressure stall average over the last 10 seconds."},
						"pressure_memory_some": schema.Float64Attribute{Computed: true, MarkdownDescription: "Memory Some pressure stall average over the last 10 seconds."},
						"pressure_memory_full": schema.Float64Attribute{Computed: true, MarkdownDescription: "Memory Full pressure stall average over the last 10 seconds."},
					},
				},
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveContainersDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
func (d *pveContainersDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveContainersDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Error reading pve_containers", "provider client is not configured")
		return
	}
	node := data.Node.ValueString()
	containers, err := d.client.ListLxcContainers(ctx, node)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_containers",
			fmt.Sprintf("listing LXC containers on node %s: %s", node, err),
		)
		return
	}
	data.Containers = make([]pveContainersDataSourceContainerModel, 0, len(containers))
	for _, ct := range containers {
		data.Containers = append(data.Containers, pveContainersContainerFromWire(ct))
	}
	data.ID = data.Node
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// pveContainersContainerFromWire projects a wire container row onto the
// nested list row model.
func pveContainersContainerFromWire(ct pveclient.LxcContainer) pveContainersDataSourceContainerModel {
	return pveContainersDataSourceContainerModel{
		VMID:               types.Int64Value(ct.VMID),
		Status:             types.StringValue(ct.Status),
		Name:               containersStringPtrToTF(ct.Name),
		Template:           nodeNetworkBoolPtrToTF(ct.Template),
		CPU:                containersFloat64PtrToTF(ct.CPU),
		CPUs:               containersFloat64PtrToInt64(ct.CPUs),
		Mem:                haInt64PtrToTF(ct.Mem),
		MaxMem:             haInt64PtrToTF(ct.MaxMem),
		MaxSwap:            haInt64PtrToTF(ct.MaxSwap),
		Disk:               haInt64PtrToTF(ct.Disk),
		MaxDisk:            haInt64PtrToTF(ct.MaxDisk),
		DiskRead:           haInt64PtrToTF(ct.DiskRead),
		DiskWrite:          haInt64PtrToTF(ct.DiskWrite),
		NetIn:              haInt64PtrToTF(ct.NetIn),
		NetOut:             haInt64PtrToTF(ct.NetOut),
		Uptime:             haInt64PtrToTF(ct.Uptime),
		Lock:               containersStringPtrToTF(ct.Lock),
		Tags:               containersStringPtrToTF(ct.Tags),
		PressureCPUSome:    containersFloat64PtrToTF(ct.PressureCPUSome),
		PressureIOSome:     containersFloat64PtrToTF(ct.PressureIOSome),
		PressureIOFull:     containersFloat64PtrToTF(ct.PressureIOFull),
		PressureMemorySome: containersFloat64PtrToTF(ct.PressureMemorySome),
		PressureMemoryFull: containersFloat64PtrToTF(ct.PressureMemoryFull),
	}
}

// containersFloat64PtrToTF maps a nil optional float to null.
func containersFloat64PtrToTF(v *float64) types.Float64 {
	if v == nil {
		return types.Float64Null()
	}
	return types.Float64Value(*v)
}

// containersFloat64PtrToInt64 maps a nil optional float to null and
// truncates whole-number values such as core counts.
func containersFloat64PtrToInt64(v *float64) types.Int64 {
	if v == nil {
		return types.Int64Null()
	}
	return types.Int64Value(int64(*v))
}

// containersStringPtrToTF maps a nil optional string to null.
func containersStringPtrToTF(v *string) types.String {
	if v == nil {
		return types.StringNull()
	}
	return types.StringValue(*v)
}
