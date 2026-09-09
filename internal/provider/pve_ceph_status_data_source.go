// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ datasource.DataSource              = &pveCephStatusDataSource{}
	_ datasource.DataSourceWithConfigure = &pveCephStatusDataSource{}
)

// NewPveCephStatusDataSource returns the data source implementation.
func NewPveCephStatusDataSource() datasource.DataSource {
	return &pveCephStatusDataSource{}
}

// pveCephStatusDataSource reads the Ceph cluster status as seen from one
// node (GET /nodes/{node}/ceph/status), plus the monitor roster and the
// cluster-wide metadata payload.
type pveCephStatusDataSource struct {
	client *pveclient.Client
}

// pveCephStatusDataSourceModel is the Terraform-facing shape.
type pveCephStatusDataSourceModel struct {
	ID           types.String                `tfsdk:"id"`
	Node         types.String                `tfsdk:"node"`
	Health       types.String                `tfsdk:"health"`
	QuorumNames  types.List                  `tfsdk:"quorum_names"`
	Versions     map[string]types.Int64      `tfsdk:"versions"`
	Monitors     []pveCephStatusMonitorModel `tfsdk:"monitors"`
	StatusJSON   types.String                `tfsdk:"status_json"`
	MetadataJSON types.String                `tfsdk:"metadata_json"`
}

// pveCephStatusMonitorModel mirrors one row of the computed monitors list.
type pveCephStatusMonitorModel struct {
	Name        types.String `tfsdk:"name"`
	Addr        types.String `tfsdk:"addr"`
	Host        types.String `tfsdk:"host"`
	State       types.String `tfsdk:"state"`
	Rank        types.Int64  `tfsdk:"rank"`
	InQuorum    types.Bool   `tfsdk:"in_quorum"`
	Service     types.Bool   `tfsdk:"service"`
	CephVersion types.String `tfsdk:"ceph_version"`
}

// Metadata implements datasource.DataSource.
func (d *pveCephStatusDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveCephStatus
}

// Schema implements datasource.DataSource.
func (d *pveCephStatusDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Ceph cluster status as observed from one node: the raw `ceph status` payload from `GET /nodes/{node}/ceph/status`, derived health and quorum summaries, the monitor roster, and the cluster-wide metadata from `GET /cluster/ceph/metadata`.",
		Attributes: map[string]schema.Attribute{
			"node": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Name of the PVE node through which the Ceph status is queried.",
			},
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Static identifier equal to the queried node name.",
			},
			"health": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Aggregate Ceph health summary derived from the raw status payload: `HEALTH_OK`, `HEALTH_WARN`, or `HEALTH_ERR`.",
			},
			"quorum_names": schema.ListAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Names of the monitors currently forming the Ceph quorum.",
			},
			"versions": schema.MapAttribute{
				Computed:            true,
				ElementType:         types.Int64Type,
				MarkdownDescription: "Map of Ceph daemon version string to the number of daemons reporting that version.",
			},
			"monitors": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Ceph monitors known to the queried node (from `GET /nodes/{node}/ceph/mon`).",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name":         schema.StringAttribute{Computed: true, MarkdownDescription: "Monitor ID (typically the hostname)."},
						"addr":         schema.StringAttribute{Computed: true, MarkdownDescription: "Ceph-formatted monitor address (typically `IP:PORT/NONCE`)."},
						"host":         schema.StringAttribute{Computed: true, MarkdownDescription: "Host the monitor runs on."},
						"state":        schema.StringAttribute{Computed: true, MarkdownDescription: "Run state of the monitor: `running`, `stopped`, or `unknown`."},
						"rank":         schema.Int64Attribute{Computed: true, MarkdownDescription: "Rank of the monitor within the mon map."},
						"in_quorum":    schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the monitor is part of the current quorum."},
						"service":      schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether a `ceph-mon@<id>` systemd unit is enabled on the hosting node."},
						"ceph_version": schema.StringAttribute{Computed: true, MarkdownDescription: "Full Ceph version string of the monitor daemon."},
					},
				},
			},
			"status_json": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Raw `ceph status` JSON exactly as returned by `GET /nodes/{node}/ceph/status`; the pin types this payload as an unstructured object.",
			},
			"metadata_json": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Raw cluster metadata JSON exactly as returned by `GET /cluster/ceph/metadata`; the pin types this payload as an unstructured object.",
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveCephStatusDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = cephConfigureDataSource(req, resp)
}

// Read implements datasource.DataSource.
func (d *pveCephStatusDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveCephStatusDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Error reading pve_ceph_status", "provider client is not configured")
		return
	}
	node := data.Node.ValueString()

	status, err := d.client.GetCephStatus(ctx, node)
	if err != nil {
		resp.Diagnostics.AddError("Error reading pve_ceph_status", fmt.Sprintf("reading ceph status via %s: %s", node, err))
		return
	}
	mons, err := d.client.ListCephMons(ctx, node)
	if err != nil {
		resp.Diagnostics.AddError("Error reading pve_ceph_status", fmt.Sprintf("listing ceph monitors via %s: %s", node, err))
		return
	}
	metadata, err := d.client.GetCephMetadata(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading pve_ceph_status", fmt.Sprintf("reading ceph metadata: %s", err))
		return
	}

	data.ID = types.StringValue(node)
	data.Health = nodeNetworkStringToTF(status.Health)
	data.QuorumNames = listStringToTF(status.QuorumNames)
	versions := make(map[string]types.Int64, len(status.Versions))
	for version, count := range status.Versions {
		versions[version] = types.Int64Value(count)
	}
	data.Versions = versions
	monitors := make([]pveCephStatusMonitorModel, 0, len(mons))
	for _, mon := range mons {
		monitors = append(monitors, cephStatusMonitorFromClient(mon))
	}
	data.Monitors = monitors
	data.StatusJSON = types.StringValue(status.RawJSON)
	data.MetadataJSON = nodeNetworkStringToTF(metadata)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// cephStatusMonitorFromClient projects a client monitor entry into the
// Terraform row model.
func cephStatusMonitorFromClient(mon pveclient.CephMon) pveCephStatusMonitorModel {
	return pveCephStatusMonitorModel{
		Name:        types.StringValue(mon.Name),
		Addr:        nodeNetworkStringToTF(mon.Addr),
		Host:        nodeNetworkStringToTF(mon.Host),
		State:       nodeNetworkStringToTF(mon.State),
		Rank:        cephInt64PtrToTF(mon.Rank),
		InQuorum:    nodeNetworkBoolPtrToTF(mon.Quorum),
		Service:     nodeNetworkBoolPtrToTF(mon.Service),
		CephVersion: nodeNetworkStringToTF(mon.CephVersion),
	}
}

// cephConfigureDataSource extracts the shared client from provider data for
// Ceph data sources. Nil provider data leaves the data source unconfigured
// (unit tests).
func cephConfigureDataSource(req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) *pveclient.Client {
	if req.ProviderData == nil {
		return nil
	}
	client, ok := req.ProviderData.(*pveclient.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *pveclient.Client, got: %T.", req.ProviderData),
		)
		return nil
	}
	return client
}

// cephConfigureResource extracts the shared client from provider data for
// Ceph resources. Nil provider data leaves the resource unconfigured
// (unit tests).
func cephConfigureResource(req resource.ConfigureRequest, resp *resource.ConfigureResponse) *pveclient.Client {
	if req.ProviderData == nil {
		return nil
	}
	client, ok := req.ProviderData.(*pveclient.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *pveclient.Client, got: %T.", req.ProviderData),
		)
		return nil
	}
	return client
}

// cephInt64PtrToTF maps a nil pointer to a null Int64 for computed rows.
func cephInt64PtrToTF(v *int) types.Int64 {
	if v == nil {
		return types.Int64Null()
	}
	return types.Int64Value(int64(*v))
}
