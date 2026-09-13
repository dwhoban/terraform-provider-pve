// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	int64validator "github.com/hashicorp/terraform-plugin-framework-validators/int64validator"

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ datasource.DataSource              = &pveContainerDataSource{}
	_ datasource.DataSourceWithConfigure = &pveContainerDataSource{}
)

// NewPveContainerDataSource returns the data source implementation.
func NewPveContainerDataSource() datasource.DataSource {
	return &pveContainerDataSource{}
}

// pveContainerDataSource exposes one LXC container's configuration, status,
// mount points and (when running) discovered interface IPs.
type pveContainerDataSource struct {
	client *pveclient.Client
}

// pveContainerDataSourceModel is the data source shape.
type pveContainerDataSourceModel struct {
	ID           types.String               `tfsdk:"id"`
	Node         types.String               `tfsdk:"node"`
	VMID         types.Int64                `tfsdk:"vmid"`
	Hostname     types.String               `tfsdk:"hostname"`
	Description  types.String               `tfsdk:"description"`
	Tags         types.String               `tfsdk:"tags"`
	Onboot       types.Bool                 `tfsdk:"onboot"`
	Protection   types.Bool                 `tfsdk:"protection"`
	Template     types.Bool                 `tfsdk:"template"`
	Unprivileged types.Bool                 `tfsdk:"unprivileged"`
	Cores        types.Int64                `tfsdk:"cores"`
	Memory       types.Int64                `tfsdk:"memory"`
	Swap         types.Int64                `tfsdk:"swap"`
	Nameserver   types.String               `tfsdk:"nameserver"`
	Searchdomain types.String               `tfsdk:"searchdomain"`
	Status       types.String               `tfsdk:"status"`
	Started      types.Bool                 `tfsdk:"started"`
	MountPoints  []containerMountPointModel `tfsdk:"mount_points"`
	Interfaces   []containerInterfaceModel  `tfsdk:"interfaces"`
}

// Metadata implements datasource.DataSource.
func (d *pveContainerDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveContainer
}

// Schema implements datasource.DataSource.
func (d *pveContainerDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads one LXC container's configuration and runtime state. The interface list is only populated while the container is running (`GET .../interfaces` requires a running container).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Data source identifier in the form `<node>/<vmid>`.",
			},
			"node": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The cluster node the container runs on.",
			},
			"vmid": schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "Container ID (100 - 999999999).",
				Validators:          []validator.Int64{int64validator.Between(100, 999999999)},
			},
			"hostname": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Host name of the container.",
			},
			"description": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Description of the container.",
			},
			"tags": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Tags of the container (a `;`-separated list).",
			},
			"onboot": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether the container is started during system bootup.",
			},
			"protection": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether the protection flag is set.",
			},
			"template": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether the container is a template.",
			},
			"unprivileged": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether the container runs as an unprivileged user.",
			},
			"cores": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Number of cores assigned to the container.",
			},
			"memory": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "RAM for the container in MB.",
			},
			"swap": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "SWAP for the container in MB.",
			},
			"nameserver": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "DNS server IP addresses of the container.",
			},
			"searchdomain": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "DNS search domains of the container.",
			},
			"status": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Container status: `stopped` or `running`.",
			},
			"started": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether the container is currently running.",
			},
			"mount_points": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Ordered mount points including the root volume, as parsed from the config's `rootfs`/`mpN` entries. Bind mounts surface as `volume` host paths without `storage`.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: containerMountPointDSNestedAttributes(),
				},
			},
			"interfaces": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Network interfaces with discovered IP addresses from `GET .../interfaces`; null when the container is stopped.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: containerInterfaceDSNestedAttributes(),
				},
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveContainerDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = containerConfigureDataSource(req, resp)
}

// Read implements datasource.DataSource.
func (d *pveContainerDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state pveContainerDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Error reading pve_container data source", "provider client is not configured")
		return
	}
	node := state.Node.ValueString()
	vmid := state.VMID.ValueInt64()
	cfg, err := d.client.GetLxcConfig(ctx, node, vmid)
	if err != nil {
		resp.Diagnostics.AddError("Error reading pve_container data source", fmt.Sprintf("reading config of container %d on %s: %s", vmid, node, err))
		return
	}
	status, err := d.client.GetLxcStatus(ctx, node, vmid)
	if err != nil {
		resp.Diagnostics.AddError("Error reading pve_container data source", fmt.Sprintf("reading status of container %d on %s: %s", vmid, node, err))
		return
	}
	state.Hostname = nodeNetworkStringToTF(cfg.Hostname)
	state.Description = nodeNetworkStringToTF(cfg.Description)
	state.Tags = nodeNetworkStringToTF(cfg.Tags)
	state.Onboot = nodeNetworkBoolPtrToTF(cfg.Onboot)
	state.Protection = nodeNetworkBoolPtrToTF(cfg.Protection)
	state.Template = nodeNetworkBoolPtrToTF(cfg.Template)
	state.Unprivileged = nodeNetworkBoolPtrToTF(cfg.Unprivileged)
	state.Cores = containerInt64PtrToTF(cfg.Cores)
	state.Memory = containerInt64PtrToTF(cfg.Memory)
	state.Swap = containerInt64PtrToTF(cfg.Swap)
	state.Nameserver = nodeNetworkStringToTF(cfg.Nameserver)
	state.Searchdomain = nodeNetworkStringToTF(cfg.Searchdomain)
	state.MountPoints = containerMountPointsFromConfig(cfg)
	state.Status = types.StringValue(status.Status)
	state.Started = types.BoolValue(status.Status == "running")
	state.ID = types.StringValue(containerID(node, vmid))
	if status.Status == "running" {
		ifaces, err := d.client.GetLxcInterfaces(ctx, node, vmid)
		if err != nil {
			resp.Diagnostics.AddError("Error reading pve_container data source", fmt.Sprintf("reading interfaces of container %d on %s: %s", vmid, node, err))
			return
		}
		state.Interfaces = containerInterfacesFromWire(ifaces)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
