// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ datasource.DataSource              = &pveCephOSDDataSource{}
	_ datasource.DataSourceWithConfigure = &pveCephOSDDataSource{}
)

// NewPveCephOSDDataSource returns the data source implementation.
func NewPveCephOSDDataSource() datasource.DataSource {
	return &pveCephOSDDataSource{}
}

// pveCephOSDDataSource reads one OSD's metadata
// (GET /nodes/{node}/ceph/osd/{osdid}/metadata).
type pveCephOSDDataSource struct {
	client *pveclient.Client
}

// pveCephOSDDataSourceModel is the Terraform-facing shape.
type pveCephOSDDataSourceModel struct {
	ID             types.String                  `tfsdk:"id"`
	Node           types.String                  `tfsdk:"node"`
	OSDID          types.Int64                   `tfsdk:"osd_id"`
	Hostname       types.String                  `tfsdk:"hostname"`
	OSDData        types.String                  `tfsdk:"osd_data"`
	OSDObjectStore types.String                  `tfsdk:"osd_objectstore"`
	Encrypted      types.Bool                    `tfsdk:"encrypted"`
	Version        types.String                  `tfsdk:"version"`
	PID            types.Int64                   `tfsdk:"pid"`
	FrontAddr      types.String                  `tfsdk:"front_addr"`
	BackAddr       types.String                  `tfsdk:"back_addr"`
	HBFrontAddr    types.String                  `tfsdk:"hb_front_addr"`
	HBBackAddr     types.String                  `tfsdk:"hb_back_addr"`
	MemUsage       types.Int64                   `tfsdk:"mem_usage"`
	Devices        []pveCephOSDDeviceDeviceModel `tfsdk:"devices"`
}

// pveCephOSDDeviceDeviceModel mirrors one row of the computed devices list.
type pveCephOSDDeviceDeviceModel struct {
	DevNode        types.String `tfsdk:"dev_node"`
	Device         types.String `tfsdk:"device"`
	PhysicalDevice types.String `tfsdk:"physical_device"`
	Size           types.Int64  `tfsdk:"size"`
	SupportDiscard types.Bool   `tfsdk:"support_discard"`
	Type           types.String `tfsdk:"type"`
}

// Metadata implements datasource.DataSource.
func (d *pveCephOSDDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveCephOsd
}

// Schema implements datasource.DataSource.
func (d *pveCephOSDDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads the metadata of one Ceph OSD from `GET /nodes/{node}/ceph/osd/{osdid}/metadata`.",
		Attributes: map[string]schema.Attribute{
			"node": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Name of the PVE node hosting the OSD.",
			},
			"osd_id": schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "OSD id (the numeric part of `osd.<id>`).",
			},
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Identifier in the form `<node>:osd.<id>`.",
			},
			"hostname": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Name of the host containing the OSD.",
			},
			"osd_data": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Path to the OSD's data directory.",
			},
			"osd_objectstore": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The object store type used by the OSD (typically `bluestore`).",
			},
			"encrypted": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether the OSD is encrypted with LUKS via dm-crypt.",
			},
			"version": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Ceph version of the OSD service.",
			},
			"pid": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "OSD process id; null when the systemd unit for this OSD is not currently running.",
			},
			"front_addr": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Address and port used to talk to clients and monitors.",
			},
			"back_addr": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Address and port used to talk to other OSDs.",
			},
			"hb_front_addr": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Heartbeat address and port for other OSDs.",
			},
			"hb_back_addr": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Heartbeat address and port for clients and monitors.",
			},
			"mem_usage": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Proportional set size (PSS) memory usage of the OSD daemon process in bytes; 0 when the process is not running.",
			},
			"devices": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Devices backing the OSD (block, db, and wal devices).",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"dev_node":        schema.StringAttribute{Computed: true, MarkdownDescription: "Device node name."},
						"device":          schema.StringAttribute{Computed: true, MarkdownDescription: "Kind of OSD device: `block`, `db`, or `wal`."},
						"physical_device": schema.StringAttribute{Computed: true, MarkdownDescription: "Underlying physical device(s) used by this OSD device."},
						"size":            schema.Int64Attribute{Computed: true, MarkdownDescription: "Size of the OSD device in bytes."},
						"support_discard": schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the underlying physical device supports discard/TRIM."},
						"type":            schema.StringAttribute{Computed: true, MarkdownDescription: "Type of device, for example `hdd` or `ssd`."},
					},
				},
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveCephOSDDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = cephConfigureDataSource(req, resp)
}

// Read implements datasource.DataSource.
func (d *pveCephOSDDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveCephOSDDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Error reading pve_ceph_osd data source", "provider client is not configured")
		return
	}

	meta, err := d.client.GetCephOSDMetadata(ctx, data.Node.ValueString(), int(data.OSDID.ValueInt64()))
	if err != nil {
		resp.Diagnostics.AddError("Error reading pve_ceph_osd data source", fmt.Sprintf("reading OSD %d on %s: %s", data.OSDID.ValueInt64(), data.Node.ValueString(), err))
		return
	}

	data.ID = types.StringValue(fmt.Sprintf("%s:osd.%d", data.Node.ValueString(), data.OSDID.ValueInt64()))
	data.Hostname = nodeNetworkStringToTF(meta.Hostname)
	data.OSDData = nodeNetworkStringToTF(meta.OSDData)
	data.OSDObjectStore = nodeNetworkStringToTF(meta.OSDObjectStore)
	data.Encrypted = types.BoolValue(meta.Encrypted)
	data.Version = nodeNetworkStringToTF(meta.Version)
	data.PID = cephInt64OptToTF(meta.PID)
	data.FrontAddr = nodeNetworkStringToTF(meta.FrontAddr)
	data.BackAddr = nodeNetworkStringToTF(meta.BackAddr)
	data.HBFrontAddr = nodeNetworkStringToTF(meta.HBFrontAddr)
	data.HBBackAddr = nodeNetworkStringToTF(meta.HBBackAddr)
	data.MemUsage = types.Int64Value(meta.MemUsage)
	devices := make([]pveCephOSDDeviceDeviceModel, 0, len(meta.Devices))
	for _, dev := range meta.Devices {
		devices = append(devices, pveCephOSDDeviceDeviceModel{
			DevNode:        types.StringValue(dev.DevNode),
			Device:         types.StringValue(dev.Device),
			PhysicalDevice: types.StringValue(dev.PhysicalDevice),
			Size:           types.Int64Value(dev.Size),
			SupportDiscard: types.BoolValue(dev.SupportDiscard),
			Type:           types.StringValue(dev.Type),
		})
	}
	data.Devices = devices

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
