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
	_ datasource.DataSource              = &pveVmAgentInfoDataSource{}
	_ datasource.DataSourceWithConfigure = &pveVmAgentInfoDataSource{}
)

// NewPveVmAgentInfoDataSource returns the data source implementation.
func NewPveVmAgentInfoDataSource() datasource.DataSource {
	return &pveVmAgentInfoDataSource{}
}

// pveVmAgentInfoDataSource merges the pin's GET QEMU guest agent commands
// for one VM into a single set of computed facts.
type pveVmAgentInfoDataSource struct {
	client *pveclient.Client
}

// pveVmAgentInfoDataSourceModel is the Terraform-facing shape.
type pveVmAgentInfoDataSourceModel struct {
	ID              types.String                             `tfsdk:"id"`
	Node            types.String                             `tfsdk:"node"`
	VMID            types.Int64                              `tfsdk:"vmid"`
	Version         types.String                             `tfsdk:"version"`
	HostName        types.String                             `tfsdk:"hostname"`
	OSID            types.String                             `tfsdk:"os_id"`
	OSIDLike        types.String                             `tfsdk:"os_id_like"`
	OSMachine       types.String                             `tfsdk:"os_machine"`
	OSKernelName    types.String                             `tfsdk:"os_kernel_name"`
	OSKernelRelease types.String                             `tfsdk:"os_kernel_release"`
	OSKernelVersion types.String                             `tfsdk:"os_kernel_version"`
	OSName          types.String                             `tfsdk:"os_name"`
	OSPrettyName    types.String                             `tfsdk:"os_pretty_name"`
	OSVariant       types.String                             `tfsdk:"os_variant"`
	OSVariantID     types.String                             `tfsdk:"os_variant_id"`
	OSVersion       types.String                             `tfsdk:"os_version"`
	OSVersionID     types.String                             `tfsdk:"os_version_id"`
	OSHomeURL       types.String                             `tfsdk:"os_home_url"`
	Time            types.Int64                              `tfsdk:"time"`
	TimeZone        types.String                             `tfsdk:"timezone"`
	VCPUs           types.Int64                              `tfsdk:"vcpus"`
	Users           types.List                               `tfsdk:"users"`
	Interfaces      []pveVmAgentInfoDataSourceInterfaceModel `tfsdk:"interfaces"`
}

// pveVmAgentInfoDataSourceInterfaceModel mirrors one row of the
// network-get-interfaces result.
type pveVmAgentInfoDataSourceInterfaceModel struct {
	Name            types.String                             `tfsdk:"name"`
	HardwareAddress types.String                             `tfsdk:"hardware_address"`
	IPAddresses     []pveVmAgentInfoDataSourceIPAddressModel `tfsdk:"ip_addresses"`
}

// pveVmAgentInfoDataSourceIPAddressModel mirrors one address row.
type pveVmAgentInfoDataSourceIPAddressModel struct {
	Address types.String `tfsdk:"address"`
	Type    types.String `tfsdk:"type"`
	Prefix  types.Int64  `tfsdk:"prefix"`
}

// Metadata implements datasource.DataSource.
func (d *pveVmAgentInfoDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveVmAgentInfo
}

// Schema implements datasource.DataSource.
func (d *pveVmAgentInfoDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Merged runtime facts of a QEMU guest as reported by its guest agent " +
			"(`GET /nodes/{node}/qemu/{vmid}/agent/{info,get-host-name,get-osinfo,get-time,get-timezone,get-vcpus,get-users,network-get-interfaces}`). " +
			"The QEMU guest agent must be installed and running inside the guest; facts whose command the " +
			"installed agent does not answer are null. Only the pin's GET agent commands are used.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Identifier of this data source (`node:vmid`).",
			},
			"node": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The cluster node running the guest.",
			},
			"vmid": schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "The ID of the guest (VM) to query.",
			},
			"version": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Guest agent version from the `info` command.",
			},
			"hostname": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Host name reported by the guest (`get-host-name`).",
			},
			"os_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Operating system identifier from `get-osinfo`, e.g. `ubuntu`.",
			},
			"os_id_like": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Operating systems this one resembles (`get-osinfo` `id_like`), e.g. `debian`.",
			},
			"os_machine": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Machine architecture from `get-osinfo`, e.g. `x86_64`.",
			},
			"os_kernel_name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Kernel name from `get-osinfo`, e.g. `Linux`.",
			},
			"os_kernel_release": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Kernel release from `get-osinfo`, e.g. `5.15.0-91-generic`.",
			},
			"os_kernel_version": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Kernel build version from `get-osinfo`.",
			},
			"os_name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Operating system name from `get-osinfo`, e.g. `Ubuntu`.",
			},
			"os_pretty_name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Pretty operating system name from `get-osinfo`, e.g. `Ubuntu 22.04.5 LTS`.",
			},
			"os_variant": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Operating system variant from `get-osinfo`, e.g. `server`.",
			},
			"os_variant_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Variant identifier from `get-osinfo`, e.g. `server`.",
			},
			"os_version": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Operating system version from `get-osinfo`.",
			},
			"os_version_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Operating system version identifier from `get-osinfo`, e.g. `22.04`.",
			},
			"os_home_url": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Operating system home page from `get-osinfo`.",
			},
			"time": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Guest clock from `get-time` (agent-reported timestamp; the raw value is nanoseconds since the epoch on modern agents).",
			},
			"timezone": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Guest time zone from `get-timezone`, e.g. `Europe/Berlin`.",
			},
			"vcpus": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Number of vCPU entries reported by `get-vcpus`.",
			},
			"users": schema.ListAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "User names of logged-in users reported by `get-users`.",
			},
			"interfaces": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Guest network interfaces from `network-get-interfaces`.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Interface name inside the guest, e.g. `eth0`.",
						},
						"hardware_address": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "MAC address of the interface, e.g. `52:54:00:aa:bb:cc`.",
						},
						"ip_addresses": schema.ListNestedAttribute{
							Computed:            true,
							MarkdownDescription: "Addresses assigned to this interface.",
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"address": schema.StringAttribute{
										Computed:            true,
										MarkdownDescription: "The IP address, e.g. `192.168.1.10`.",
									},
									"type": schema.StringAttribute{
										Computed:            true,
										MarkdownDescription: "Address family reported by the agent: `ipv4` or `ipv6`.",
									},
									"prefix": schema.Int64Attribute{
										Computed:            true,
										MarkdownDescription: "Network prefix length of the address.",
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveVmAgentInfoDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
func (d *pveVmAgentInfoDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveVmAgentInfoDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Unconfigured pve_vm_agent_info",
			fmt.Sprintf("The provider client was not configured; cannot read agent facts of vm %d.", data.VMID.ValueInt64()),
		)
		return
	}
	node := data.Node.ValueString()
	vmid := data.VMID.ValueInt64()
	facts, err := d.client.GetGuestAgentFacts(ctx, node, vmid)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_vm_agent_info",
			fmt.Sprintf("reading guest agent facts of vm %d on node %s: %s", vmid, node, err),
		)
		return
	}
	data.ID = types.StringValue(fmt.Sprintf("%s:%d", node, vmid))
	data.Version = types.StringValue(facts.Version)
	data.HostName = types.StringValue(facts.HostName)
	data.OSID = types.StringValue(facts.OSID)
	data.OSIDLike = types.StringValue(facts.OSIDLike)
	data.OSMachine = types.StringValue(facts.OSMachine)
	data.OSKernelName = types.StringValue(facts.OSKernelName)
	data.OSKernelRelease = types.StringValue(facts.OSKernelRelease)
	data.OSKernelVersion = types.StringValue(facts.OSKernelVersion)
	data.OSName = types.StringValue(facts.OSName)
	data.OSPrettyName = types.StringValue(facts.OSPrettyName)
	data.OSVariant = types.StringValue(facts.OSVariant)
	data.OSVariantID = types.StringValue(facts.OSVariantID)
	data.OSVersion = types.StringValue(facts.OSVersion)
	data.OSVersionID = types.StringValue(facts.OSVersionID)
	data.OSHomeURL = types.StringValue(facts.OSHomeURL)
	if facts.Time != nil {
		data.Time = types.Int64Value(*facts.Time)
	} else {
		data.Time = types.Int64Null()
	}
	data.TimeZone = types.StringValue(facts.TimeZone)
	if facts.VCPUs != nil {
		data.VCPUs = types.Int64Value(*facts.VCPUs)
	} else {
		data.VCPUs = types.Int64Null()
	}
	data.Users = listStringToTF(facts.Users)
	interfaces := make([]pveVmAgentInfoDataSourceInterfaceModel, 0, len(facts.Interfaces))
	for _, iface := range facts.Interfaces {
		addresses := make([]pveVmAgentInfoDataSourceIPAddressModel, 0, len(iface.IPAddresses))
		for _, addr := range iface.IPAddresses {
			addresses = append(addresses, pveVmAgentInfoDataSourceIPAddressModel{
				Address: types.StringValue(addr.Address),
				Type:    types.StringValue(addr.Type),
				Prefix:  types.Int64Value(addr.Prefix),
			})
		}
		interfaces = append(interfaces, pveVmAgentInfoDataSourceInterfaceModel{
			Name:            types.StringValue(iface.Name),
			HardwareAddress: types.StringValue(iface.HardwareAddress),
			IPAddresses:     addresses,
		})
	}
	data.Interfaces = interfaces
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
