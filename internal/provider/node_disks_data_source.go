// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ datasource.DataSource              = &pveNodeDisksDataSource{}
	_ datasource.DataSourceWithConfigure = &pveNodeDisksDataSource{}
)

// NewPveNodeDisksDataSource returns the data source implementation.
func NewPveNodeDisksDataSource() datasource.DataSource {
	return &pveNodeDisksDataSource{}
}

// pveNodeDisksDataSource enumerates physical disks attached to a node.
type pveNodeDisksDataSource struct {
	client *pveclient.Client
}

// pveNodeDisksDataSourceModel is the Terraform-facing shape.
type pveNodeDisksDataSourceModel struct {
	ID           types.String                      `tfsdk:"id"`
	Node         types.String                      `tfsdk:"node"`
	IncludeSMART types.Bool                        `tfsdk:"include_smart"`
	WWNFilter    types.String                      `tfsdk:"wwn"`
	Disks        []pveNodeDisksDataSourceDiskModel `tfsdk:"disks"`
}

// pveNodeDisksDataSourceDiskModel mirrors one row of /nodes/{node}/disks/list.
type pveNodeDisksDataSourceDiskModel struct {
	DevPath types.String `tfsdk:"devpath"`
	Size    types.Int64  `tfsdk:"size"`
	Used    types.String `tfsdk:"used"`
	GPT     types.Bool   `tfsdk:"gpt"`
	Mounted types.Bool   `tfsdk:"mounted"`
	OSDID   types.Int64  `tfsdk:"osdid"`
	Vendor  types.String `tfsdk:"vendor"`
	Model   types.String `tfsdk:"model"`
	Serial  types.String `tfsdk:"serial"`
	WWN     types.String `tfsdk:"wwn"`
	Health  types.String `tfsdk:"health"`
	Parent  types.String `tfsdk:"parent"`
	Type    types.String `tfsdk:"type"`
}

// Metadata implements datasource.DataSource.
func (d *pveNodeDisksDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveNodeDisks
}

// Schema implements datasource.DataSource.
func (d *pveNodeDisksDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists the physical disks attached to a Proxmox VE node (`GET /nodes/{node}/disks/list`).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Static identifier equal to the node name.",
			},
			"node": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Name of the PVE node to query.",
			},
			"include_smart": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "When `true`, fetch SMART data per disk and merge the `health` field onto the rows. Defaults to `false`.",
			},
			"wwn": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Optional substring filter on the disk WWN.",
			},
			"disks": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Disks matching the filter (or all disks when no filter is set).",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"devpath": schema.StringAttribute{Computed: true, MarkdownDescription: "Device path (e.g. `/dev/sda`)."},
						"used":    schema.StringAttribute{Computed: true, MarkdownDescription: "PVE usage label. One of `root`, `unused`, `zfs`, `lvm`, `lvmthin`, `ceph`, or `directory`."},
						"gpt":     schema.BoolAttribute{Computed: true, MarkdownDescription: "True when the disk carries a GPT partition table."},
						"mounted": schema.BoolAttribute{Computed: true, MarkdownDescription: "True when the disk has at least one mounted filesystem."},
						"osdid":   schema.Int64Attribute{Computed: true, MarkdownDescription: "OSD identifier if the disk participates in Ceph."},
						"vendor":  schema.StringAttribute{Computed: true, MarkdownDescription: "Vendor string."},
						"model":   schema.StringAttribute{Computed: true, MarkdownDescription: "Model string."},
						"serial":  schema.StringAttribute{Computed: true, MarkdownDescription: "Serial number."},
						"wwn":     schema.StringAttribute{Computed: true, MarkdownDescription: "World Wide Name."},
						"health":  schema.StringAttribute{Computed: true, MarkdownDescription: "SMART health (populated when `include_smart=true`); one of `PASSED`, `FAILED`, or `UNKNOWN`."},
						"parent":  schema.StringAttribute{Computed: true, MarkdownDescription: "Parent device path (for partitions)."},
						"type":    schema.StringAttribute{Computed: true, MarkdownDescription: "Disk type. One of `ssd`, `hdd`, or `nvme`."},
					},
				},
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveNodeDisksDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
func (d *pveNodeDisksDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveNodeDisksDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	node := data.Node.ValueString()
	disks, err := d.client.ListNodeDisks(ctx, node)
	if err != nil {
		resp.Diagnostics.AddError("Error reading pve_node_disks", fmt.Sprintf("listing disks on %s: %s", node, err))
		return
	}

	smartByName := map[string]string{}
	if data.IncludeSMART.ValueBool() {
		smart, err := d.client.ListNodeDiskSmart(ctx, node)
		if err != nil {
			resp.Diagnostics.AddError("Error reading pve_node_disks SMART data", fmt.Sprintf("listing SMART data on %s: %s", node, err))
			return
		}
		for _, s := range smart {
			smartByName[s.Name] = s.Health
		}
	}

	wwnFilter := strings.ToLower(data.WWNFilter.ValueString())
	rows := make([]pveNodeDisksDataSourceDiskModel, 0, len(disks))
	for _, disk := range disks {
		if wwnFilter != "" && !strings.Contains(strings.ToLower(disk.WWN), wwnFilter) {
			continue
		}
		health := disk.Health
		if h, ok := smartByName[disk.DevPath]; ok {
			health = h
		}
		rows = append(rows, pveNodeDisksDataSourceDiskModel{
			DevPath: types.StringValue(disk.DevPath),
			Size:    types.Int64Value(disk.Size),
			Used:    types.StringValue(disk.Used),
			GPT:     types.BoolValue(disk.GPT),
			Mounted: types.BoolValue(disk.Mounted),
			OSDID:   types.Int64Value(int64(disk.OSDID)),
			Vendor:  types.StringValue(disk.Vendor),
			Model:   types.StringValue(disk.Model),
			Serial:  types.StringValue(disk.Serial),
			WWN:     types.StringValue(disk.WWN),
			Health:  types.StringValue(health),
			Parent:  types.StringValue(disk.Parent),
			Type:    types.StringValue(disk.Type),
		})
	}
	data.ID = types.StringValue(node)
	data.Disks = rows
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
