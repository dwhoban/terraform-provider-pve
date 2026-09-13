// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ datasource.DataSource              = &pveNodeUsbDevicesDataSource{}
	_ datasource.DataSourceWithConfigure = &pveNodeUsbDevicesDataSource{}
)

// NewPveNodeUsbDevicesDataSource returns the data source implementation.
func NewPveNodeUsbDevicesDataSource() datasource.DataSource {
	return &pveNodeUsbDevicesDataSource{}
}

// pveNodeUsbDevicesDataSource lists the local USB devices of one node
// (GET /nodes/{node}/hardware/usb).
type pveNodeUsbDevicesDataSource struct {
	client *pveclient.Client
}

// pveNodeUsbDevicesDataSourceModel is the Terraform-facing shape.
type pveNodeUsbDevicesDataSourceModel struct {
	ID      types.String                             `tfsdk:"id"`
	Node    types.String                             `tfsdk:"node"`
	Devices []pveNodeUsbDevicesDataSourceDeviceModel `tfsdk:"devices"`
}

// pveNodeUsbDevicesDataSourceDeviceModel mirrors one USB device row.
type pveNodeUsbDevicesDataSourceDeviceModel struct {
	BusNum       types.Int64  `tfsdk:"busnum"`
	Class        types.Int64  `tfsdk:"class"`
	DevNum       types.Int64  `tfsdk:"devnum"`
	Level        types.Int64  `tfsdk:"level"`
	Port         types.Int64  `tfsdk:"port"`
	VendorID     types.String `tfsdk:"vendid"`
	ProductID    types.String `tfsdk:"prodid"`
	Manufacturer types.String `tfsdk:"manufacturer"`
	Product      types.String `tfsdk:"product"`
	Serial       types.String `tfsdk:"serial"`
	Speed        types.String `tfsdk:"speed"`
	USBPath      types.String `tfsdk:"usbpath"`
}

// Metadata implements datasource.DataSource.
func (d *pveNodeUsbDevicesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveNodeUsbDevices
}

// Schema implements datasource.DataSource.
func (d *pveNodeUsbDevicesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists the local USB devices of one node (`GET /nodes/{node}/hardware/usb`).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Identifier in `node` form (same as the `node` attribute).",
			},
			"node": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The cluster node name whose USB devices to list.",
			},
			"devices": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "USB devices currently attached to the node.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"busnum": schema.Int64Attribute{
							Computed:            true,
							MarkdownDescription: "The USB bus number.",
						},
						"class": schema.Int64Attribute{
							Computed:            true,
							MarkdownDescription: "The USB class code.",
						},
						"devnum": schema.Int64Attribute{
							Computed:            true,
							MarkdownDescription: "The device number on the bus.",
						},
						"level": schema.Int64Attribute{
							Computed:            true,
							MarkdownDescription: "The hub depth of the device.",
						},
						"port": schema.Int64Attribute{
							Computed:            true,
							MarkdownDescription: "The physical port the device is plugged into.",
						},
						"vendid": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The vendor ID (e.g. `0x8087`).",
						},
						"prodid": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The product ID (e.g. `0x1234`).",
						},
						"manufacturer": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The manufacturer string reported by the device; null when not reported.",
						},
						"product": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The product string reported by the device; null when not reported.",
						},
						"serial": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The serial number reported by the device; null when not reported.",
						},
						"speed": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The negotiated link speed (e.g. `480`).",
						},
						"usbpath": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The USB path (e.g. `1-2`); null when not reported.",
						},
					},
				},
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveNodeUsbDevicesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
func (d *pveNodeUsbDevicesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config pveNodeUsbDevicesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	node := config.Node.ValueString()
	devices, err := d.client.ListNodeUsbDevices(ctx, node)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_node_usb_devices",
			fmt.Sprintf("listing USB devices for node %s: %s", node, err),
		)
		return
	}
	config.Devices = make([]pveNodeUsbDevicesDataSourceDeviceModel, 0, len(devices))
	for _, device := range devices {
		config.Devices = append(config.Devices, pveNodeUsbDevicesDataSourceDeviceModel{
			BusNum:       types.Int64Value(device.BusNum),
			Class:        types.Int64Value(device.Class),
			DevNum:       types.Int64Value(device.DevNum),
			Level:        types.Int64Value(device.Level),
			Port:         types.Int64Value(device.Port),
			VendorID:     types.StringValue(device.VendorID),
			ProductID:    types.StringValue(device.ProductID),
			Manufacturer: types.StringPointerValue(device.Manufacturer),
			Product:      types.StringPointerValue(device.Product),
			Serial:       types.StringPointerValue(device.Serial),
			Speed:        types.StringValue(device.Speed),
			USBPath:      types.StringPointerValue(device.USBPath),
		})
	}
	config.ID = types.StringValue(node)
	tflog.Debug(ctx, "read node USB devices", map[string]any{"node": node, "count": len(config.Devices)})
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
