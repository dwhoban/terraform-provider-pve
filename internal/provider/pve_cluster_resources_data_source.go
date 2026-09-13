// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// Ensure the framework interfaces are satisfied.
var (
	_ datasource.DataSource              = &pveClusterResourcesDataSource{}
	_ datasource.DataSourceWithConfigure = &pveClusterResourcesDataSource{}
)

// NewPveClusterResourcesDataSource returns the data source implementation.
func NewPveClusterResourcesDataSource() datasource.DataSource {
	return &pveClusterResourcesDataSource{}
}

// pveClusterResourcesDataSource reads the cluster-wide resource index
// (GET /cluster/resources).
type pveClusterResourcesDataSource struct {
	client *pveclient.Client
}

// pveClusterResourcesDataSourceModel is the Terraform-facing shape.
type pveClusterResourcesDataSourceModel struct {
	ID        types.String                            `tfsdk:"id"`
	Type      types.String                            `tfsdk:"type"`
	Resources []pveClusterResourcesDataSourceRowModel `tfsdk:"resources"`
}

// pveClusterResourcesDataSourceRowModel mirrors one row of the
// /cluster/resources response. Every documented field is exposed; fields
// PVE omits for a given resource type decode as empty values.
type pveClusterResourcesDataSourceRowModel struct {
	CgroupMode  types.Int64   `tfsdk:"cgroup_mode"`
	Content     types.String  `tfsdk:"content"`
	CPU         types.Float64 `tfsdk:"cpu"`
	Disk        types.Int64   `tfsdk:"disk"`
	DiskRead    types.Int64   `tfsdk:"diskread"`
	DiskWrite   types.Int64   `tfsdk:"diskwrite"`
	HAState     types.String  `tfsdk:"hastate"`
	HostArch    types.String  `tfsdk:"host_arch"`
	ID          types.String  `tfsdk:"id"`
	Level       types.String  `tfsdk:"level"`
	Lock        types.String  `tfsdk:"lock"`
	MaxCPU      types.Float64 `tfsdk:"maxcpu"`
	MaxDisk     types.Int64   `tfsdk:"maxdisk"`
	MaxMem      types.Int64   `tfsdk:"maxmem"`
	Mem         types.Int64   `tfsdk:"mem"`
	MemHost     types.Int64   `tfsdk:"memhost"`
	Name        types.String  `tfsdk:"name"`
	NetIn       types.Int64   `tfsdk:"netin"`
	NetOut      types.Int64   `tfsdk:"netout"`
	Network     types.String  `tfsdk:"network"`
	NetworkType types.String  `tfsdk:"network_type"`
	Node        types.String  `tfsdk:"node"`
	PluginType  types.String  `tfsdk:"plugintype"`
	Pool        types.String  `tfsdk:"pool"`
	Protocol    types.String  `tfsdk:"protocol"`
	SDN         types.String  `tfsdk:"sdn"`
	Shared      types.Bool    `tfsdk:"shared"`
	Status      types.String  `tfsdk:"status"`
	Storage     types.String  `tfsdk:"storage"`
	Tags        types.String  `tfsdk:"tags"`
	Template    types.Bool    `tfsdk:"template"`
	Type        types.String  `tfsdk:"type"`
	Uptime      types.Int64   `tfsdk:"uptime"`
	VMID        types.Int64   `tfsdk:"vmid"`
	ZoneType    types.String  `tfsdk:"zone_type"`
}

// Metadata implements datasource.DataSource.
func (d *pveClusterResourcesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveClusterResources
}

// Schema implements datasource.DataSource.
func (d *pveClusterResourcesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists every resource in the Proxmox VE cluster (nodes, storages, pools, guests, and SDN entities) as reported by `GET /cluster/resources`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Static identifier for the cluster resource index.",
			},
			"type": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Optional server-side filter by resource type. Must be one of: `vm`, `storage`, `node`, `sdn`.",
				Validators: []validator.String{
					stringvalidator.OneOf("vm", "storage", "node", "sdn"),
				},
			},
			"resources": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Cluster resources matching the filter (or all resources when no filter is set). Fields that PVE omits for a given resource type are empty.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"cgroup_mode":  schema.Int64Attribute{Computed: true, MarkdownDescription: "The cgroup mode the node operates under (for type `node`)."},
						"content":      schema.StringAttribute{Computed: true, MarkdownDescription: "Allowed storage content types (for type `storage`)."},
						"cpu":          schema.Float64Attribute{Computed: true, MarkdownDescription: "CPU utilization (for types `node`, `qemu`, and `lxc`)."},
						"disk":         schema.Int64Attribute{Computed: true, MarkdownDescription: "Used disk space in bytes (for type `storage`), used root image space for guests (for types `qemu` and `lxc`)."},
						"diskread":     schema.Int64Attribute{Computed: true, MarkdownDescription: "Bytes the guest read from its block devices since start (for types `qemu` and `lxc`)."},
						"diskwrite":    schema.Int64Attribute{Computed: true, MarkdownDescription: "Bytes the guest wrote to its block devices since start (for types `qemu` and `lxc`)."},
						"hastate":      schema.StringAttribute{Computed: true, MarkdownDescription: "HA service status (for HA-managed guests)."},
						"host_arch":    schema.StringAttribute{Computed: true, MarkdownDescription: "The node's CPU architecture: `x86_64` or `aarch64` (for type `node`)."},
						"id":           schema.StringAttribute{Computed: true, MarkdownDescription: "Resource ID."},
						"level":        schema.StringAttribute{Computed: true, MarkdownDescription: "Support level (for type `node`)."},
						"lock":         schema.StringAttribute{Computed: true, MarkdownDescription: "The guest's current config lock (for types `qemu` and `lxc`)."},
						"maxcpu":       schema.Float64Attribute{Computed: true, MarkdownDescription: "Number of available CPUs (for types `node`, `qemu`, and `lxc`)."},
						"maxdisk":      schema.Int64Attribute{Computed: true, MarkdownDescription: "Storage size in bytes (for type `storage`), root image size for guests (for types `qemu` and `lxc`)."},
						"maxmem":       schema.Int64Attribute{Computed: true, MarkdownDescription: "Available memory in bytes (for types `node`, `qemu`, and `lxc`)."},
						"mem":          schema.Int64Attribute{Computed: true, MarkdownDescription: "Used memory in bytes (for types `node`, `qemu`, and `lxc`)."},
						"memhost":      schema.Int64Attribute{Computed: true, MarkdownDescription: "Used memory in bytes from the host's point of view (for type `qemu`)."},
						"name":         schema.StringAttribute{Computed: true, MarkdownDescription: "Name of the resource."},
						"netin":        schema.Int64Attribute{Computed: true, MarkdownDescription: "Traffic in bytes sent to the guest over the network since start (for types `qemu` and `lxc`)."},
						"netout":       schema.Int64Attribute{Computed: true, MarkdownDescription: "Traffic in bytes sent from the guest over the network since start (for types `qemu` and `lxc`)."},
						"network":      schema.StringAttribute{Computed: true, MarkdownDescription: "Name of a network entity (for type `network`)."},
						"network_type": schema.StringAttribute{Computed: true, MarkdownDescription: "Type of network resource: `fabric` or `zone` (for type `network`)."},
						"node":         schema.StringAttribute{Computed: true, MarkdownDescription: "The cluster node name (for types `node`, `storage`, `qemu`, and `lxc`)."},
						"plugintype":   schema.StringAttribute{Computed: true, MarkdownDescription: "More specific type, if available."},
						"pool":         schema.StringAttribute{Computed: true, MarkdownDescription: "The pool name (for types `pool`, `qemu`, and `lxc`)."},
						"protocol":     schema.StringAttribute{Computed: true, MarkdownDescription: "Protocol of a fabric (for type `network`, network_type `fabric`)."},
						"sdn":          schema.StringAttribute{Computed: true, MarkdownDescription: "Name of an SDN entity (for type `sdn`)."},
						"shared":       schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the storage is shared (for type `storage`)."},
						"status":       schema.StringAttribute{Computed: true, MarkdownDescription: "Resource type dependent status."},
						"storage":      schema.StringAttribute{Computed: true, MarkdownDescription: "The storage identifier (for type `storage`)."},
						"tags":         schema.StringAttribute{Computed: true, MarkdownDescription: "The guest's tags (for types `qemu` and `lxc`)."},
						"template":     schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the guest is a template (for types `qemu` and `lxc`)."},
						"type":         schema.StringAttribute{Computed: true, MarkdownDescription: "Resource type: `node`, `storage`, `pool`, `qemu`, `lxc`, `openvz`, `sdn`, or `network`."},
						"uptime":       schema.Int64Attribute{Computed: true, MarkdownDescription: "Uptime of node or guest in seconds (for types `node`, `qemu`, and `lxc`)."},
						"vmid":         schema.Int64Attribute{Computed: true, MarkdownDescription: "The numerical VM ID (for types `qemu` and `lxc`)."},
						"zone_type":    schema.StringAttribute{Computed: true, MarkdownDescription: "The type of an SDN zone (for type `sdn`)."},
					},
				},
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveClusterResourcesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
func (d *pveClusterResourcesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveClusterResourcesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	rows, err := d.client.GetClusterResources(ctx, data.Type.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading pve_cluster_resources", fmt.Sprintf("listing cluster resources: %s", err))
		return
	}
	resources := make([]pveClusterResourcesDataSourceRowModel, 0, len(rows))
	for _, row := range rows {
		resources = append(resources, pveClusterResourcesDataSourceRowModel{
			CgroupMode:  types.Int64Value(int64(row.CgroupMode)),
			Content:     types.StringValue(row.Content),
			CPU:         types.Float64Value(row.CPU),
			Disk:        types.Int64Value(row.Disk),
			DiskRead:    types.Int64Value(row.DiskRead),
			DiskWrite:   types.Int64Value(row.DiskWrite),
			HAState:     types.StringValue(row.HAState),
			HostArch:    types.StringValue(row.HostArch),
			ID:          types.StringValue(row.ID),
			Level:       types.StringValue(row.Level),
			Lock:        types.StringValue(row.Lock),
			MaxCPU:      types.Float64Value(row.MaxCPU),
			MaxDisk:     types.Int64Value(row.MaxDisk),
			MaxMem:      types.Int64Value(row.MaxMem),
			Mem:         types.Int64Value(row.Mem),
			MemHost:     types.Int64Value(row.MemHost),
			Name:        types.StringValue(row.Name),
			NetIn:       types.Int64Value(row.NetIn),
			NetOut:      types.Int64Value(row.NetOut),
			Network:     types.StringValue(row.Network),
			NetworkType: types.StringValue(row.NetworkType),
			Node:        types.StringValue(row.Node),
			PluginType:  types.StringValue(row.PluginType),
			Pool:        types.StringValue(row.Pool),
			Protocol:    types.StringValue(row.Protocol),
			SDN:         types.StringValue(row.SDN),
			Shared:      types.BoolValue(row.Shared),
			Status:      types.StringValue(row.Status),
			Storage:     types.StringValue(row.Storage),
			Tags:        types.StringValue(row.Tags),
			Template:    types.BoolValue(row.Template),
			Type:        types.StringValue(row.Type),
			Uptime:      types.Int64Value(row.Uptime),
			VMID:        types.Int64Value(row.VMID),
			ZoneType:    types.StringValue(row.ZoneType),
		})
	}
	data.ID = types.StringValue("pve_cluster_resources")
	data.Resources = resources
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
