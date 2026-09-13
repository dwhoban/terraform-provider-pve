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
	_ datasource.DataSource              = &pveCephMonDataSource{}
	_ datasource.DataSourceWithConfigure = &pveCephMonDataSource{}
)

// NewPveCephMonDataSource returns the data source implementation.
func NewPveCephMonDataSource() datasource.DataSource {
	return &pveCephMonDataSource{}
}

// pveCephMonDataSource reads a single Ceph monitor from the monitor
// listing (GET /nodes/{node}/ceph/mon).
type pveCephMonDataSource struct {
	client *pveclient.Client
}

// pveCephMonDataSourceModel is the Terraform-facing shape.
type pveCephMonDataSourceModel struct {
	ID               types.String `tfsdk:"id"`
	Node             types.String `tfsdk:"node"`
	MonID            types.String `tfsdk:"monid"`
	Addr             types.String `tfsdk:"addr"`
	Host             types.String `tfsdk:"host"`
	State            types.String `tfsdk:"state"`
	Rank             types.Int64  `tfsdk:"rank"`
	InQuorum         types.Bool   `tfsdk:"in_quorum"`
	Service          types.Bool   `tfsdk:"service"`
	DirExists        types.Bool   `tfsdk:"dir_exists"`
	CephVersion      types.String `tfsdk:"ceph_version"`
	CephVersionShort types.String `tfsdk:"ceph_version_short"`
}

// Metadata implements datasource.DataSource.
func (d *pveCephMonDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveCephMon
}

// Schema implements datasource.DataSource.
func (d *pveCephMonDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads a single Ceph monitor from `GET /nodes/{node}/ceph/mon`.",
		Attributes: map[string]schema.Attribute{
			"node": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Name of the PVE node the listing request is issued against.",
			},
			"monid": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Monitor ID (typically the hostname).",
			},
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Identifier in the form `<node>:<monid>`.",
			},
			"addr": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Ceph-formatted monitor address as advertised by the monitor (typically `IP:PORT/NONCE`, possibly a messenger-v2 vector).",
			},
			"host": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Host the monitor runs on.",
			},
			"state": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Run state of the monitor: `running` (in quorum), `stopped` (systemd unit configured but daemon not visible to the cluster), or `unknown` (no rados access).",
			},
			"rank": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Rank of the monitor within the mon map.",
			},
			"in_quorum": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether the monitor is part of the current quorum.",
			},
			"service": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether a `ceph-mon@<id>` systemd unit is enabled on the hosting node.",
			},
			"dir_exists": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether the monitor's data directory exists on this node.",
			},
			"ceph_version": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Full Ceph version string of the monitor daemon.",
			},
			"ceph_version_short": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Short Ceph version string of the monitor daemon (e.g. `19.2.0`).",
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveCephMonDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = cephConfigureDataSource(req, resp)
}

// Read implements datasource.DataSource.
func (d *pveCephMonDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveCephMonDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Error reading pve_ceph_mon data source", "provider client is not configured")
		return
	}

	mon, err := d.client.GetCephMon(ctx, data.Node.ValueString(), data.MonID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading pve_ceph_mon data source", fmt.Sprintf("reading monitor %s via %s: %s", data.MonID.ValueString(), data.Node.ValueString(), err))
		return
	}

	data.ID = types.StringValue(fmt.Sprintf("%s:%s", data.Node.ValueString(), data.MonID.ValueString()))
	data.Addr = nodeNetworkStringToTF(mon.Addr)
	data.Host = nodeNetworkStringToTF(mon.Host)
	data.State = nodeNetworkStringToTF(mon.State)
	data.Rank = cephInt64PtrToTF(mon.Rank)
	data.InQuorum = nodeNetworkBoolPtrToTF(mon.Quorum)
	data.Service = nodeNetworkBoolPtrToTF(mon.Service)
	data.DirExists = nodeNetworkBoolPtrToTF(mon.DirExists)
	data.CephVersion = nodeNetworkStringToTF(mon.CephVersion)
	data.CephVersionShort = nodeNetworkStringToTF(mon.CephVersionShort)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
