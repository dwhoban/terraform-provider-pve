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
	_ datasource.DataSource              = &pveNodePciDevicesDataSource{}
	_ datasource.DataSourceWithConfigure = &pveNodePciDevicesDataSource{}
)

// NewPveNodePciDevicesDataSource returns the data source implementation.
func NewPveNodePciDevicesDataSource() datasource.DataSource {
	return &pveNodePciDevicesDataSource{}
}

// pveNodePciDevicesDataSource lists the local PCI devices of one node
// (GET /nodes/{node}/hardware/pci).
type pveNodePciDevicesDataSource struct {
	client *pveclient.Client
}

// pveNodePciDevicesDataSourceModel is the Terraform-facing shape.
type pveNodePciDevicesDataSourceModel struct {
	ID      types.String                             `tfsdk:"id"`
	Node    types.String                             `tfsdk:"node"`
	Devices []pveNodePciDevicesDataSourceDeviceModel `tfsdk:"devices"`
}

// pveNodePciDevicesDataSourceDeviceModel mirrors one PCI device row.
type pveNodePciDevicesDataSourceDeviceModel struct {
	ID                  types.String `tfsdk:"id"`
	ClassName           types.String `tfsdk:"class"`
	DeviceID            types.String `tfsdk:"device"`
	DeviceName          types.String `tfsdk:"device_name"`
	VendorID            types.String `tfsdk:"vendor"`
	VendorName          types.String `tfsdk:"vendor_name"`
	SubsystemDeviceID   types.String `tfsdk:"subsystem_device"`
	SubsystemDeviceName types.String `tfsdk:"subsystem_device_name"`
	SubsystemVendorID   types.String `tfsdk:"subsystem_vendor"`
	SubsystemVendorName types.String `tfsdk:"subsystem_vendor_name"`
	IOMMUGroup          types.Int64  `tfsdk:"iommugroup"`
	MDev                types.Bool   `tfsdk:"mdev"`
}

// Metadata implements datasource.DataSource.
func (d *pveNodePciDevicesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveNodePciDevices
}

// Schema implements datasource.DataSource.
func (d *pveNodePciDevicesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists the local PCI devices of one node (`GET /nodes/{node}/hardware/pci`).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Identifier in `node` form (same as the `node` attribute).",
			},
			"node": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The cluster node name whose PCI devices to list.",
			},
			"devices": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "PCI devices, sorted by PCI address upstream.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The PCI address (e.g. `0000:01:00.0`).",
						},
						"class": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The PCI class of the device (e.g. `0x0300`).",
						},
						"device": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The device ID.",
						},
						"device_name": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The human readable device name; null when unknown.",
						},
						"vendor": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The vendor ID.",
						},
						"vendor_name": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The human readable vendor name; null when unknown.",
						},
						"subsystem_device": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The subsystem device ID; null when the device has none.",
						},
						"subsystem_device_name": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The human readable subsystem device name; null when unknown.",
						},
						"subsystem_vendor": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The subsystem vendor ID; null when the device has none.",
						},
						"subsystem_vendor_name": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The human readable subsystem vendor name; null when unknown.",
						},
						"iommugroup": schema.Int64Attribute{
							Computed:            true,
							MarkdownDescription: "The IOMMU group of the device; `-1` when no IOMMU group was detected.",
						},
						"mdev": schema.BoolAttribute{
							Computed:            true,
							MarkdownDescription: "Whether the device is capable of creating mediated devices; null when the host does not report it.",
						},
					},
				},
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveNodePciDevicesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
func (d *pveNodePciDevicesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config pveNodePciDevicesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	node := config.Node.ValueString()
	devices, err := d.client.ListNodePciDevices(ctx, node, pveclient.ListNodePciDevicesOptions{})
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_node_pci_devices",
			fmt.Sprintf("listing PCI devices for node %s: %s", node, err),
		)
		return
	}
	config.Devices = make([]pveNodePciDevicesDataSourceDeviceModel, 0, len(devices))
	for _, device := range devices {
		config.Devices = append(config.Devices, pveNodePciDevicesDataSourceDeviceModel{
			ID:                  types.StringValue(device.ID),
			ClassName:           types.StringValue(device.ClassName),
			DeviceID:            types.StringValue(device.DeviceID),
			DeviceName:          types.StringPointerValue(device.DeviceName),
			VendorID:            types.StringValue(device.VendorID),
			VendorName:          types.StringPointerValue(device.VendorName),
			SubsystemDeviceID:   types.StringPointerValue(device.SubsystemDeviceID),
			SubsystemDeviceName: types.StringPointerValue(device.SubsystemDeviceNam),
			SubsystemVendorID:   types.StringPointerValue(device.SubsystemVendorID),
			SubsystemVendorName: types.StringPointerValue(device.SubsystemVendorNam),
			IOMMUGroup:          types.Int64Value(device.IOMMUGroup),
			MDev:                types.BoolPointerValue(device.MDev),
		})
	}
	config.ID = types.StringValue(node)
	tflog.Debug(ctx, "read node PCI devices", map[string]any{"node": node, "count": len(config.Devices)})
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
