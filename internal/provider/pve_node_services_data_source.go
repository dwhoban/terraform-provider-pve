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
	_ datasource.DataSource = &pveNodeServicesDataSource{}
)

// NewPveNodeServicesDataSource returns the data source implementation.
func NewPveNodeServicesDataSource() datasource.DataSource {
	return &pveNodeServicesDataSource{}
}

// nodeSvcAptConfigureDataSource extracts the configured *pveclient.Client
// from a datasource ConfigureRequest for the node services and APT data
// sources. Nil provider data leaves the data source unconfigured (unit
// tests).
func nodeSvcAptConfigureDataSource(req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) *pveclient.Client {
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

// pveNodeServicesDataSource lists the systemd services PVE manages on one
// node (GET /nodes/{node}/services).
type pveNodeServicesDataSource struct {
	client *pveclient.Client
}

// pveNodeServicesDataSourceModel is the Terraform-facing shape.
type pveNodeServicesDataSourceModel struct {
	ID       types.String                     `tfsdk:"id"`
	Node     types.String                     `tfsdk:"node"`
	Services []pveNodeServicesDataSourceEntry `tfsdk:"services"`
}

// pveNodeServicesDataSourceEntry mirrors one service row.
type pveNodeServicesDataSourceEntry struct {
	Name        types.String `tfsdk:"name"`
	Service     types.String `tfsdk:"service"`
	Description types.String `tfsdk:"description"`
	State       types.String `tfsdk:"state"`
	ActiveState types.String `tfsdk:"active_state"`
	UnitState   types.String `tfsdk:"unit_state"`
}

// Metadata implements datasource.DataSource.
func (d *pveNodeServicesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveNodeServices
}

// Schema implements datasource.DataSource.
func (d *pveNodeServicesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists the systemd services PVE manages on one node (`GET /nodes/{node}/services`), with their current systemd states. Fields a service does not report are empty.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Identifier of the data source (the node name).",
			},
			"node": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The cluster node whose services to list.",
			},
			"services": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Services managed by PVE on the node.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Short identifier for the service (e.g. `pveproxy`).",
						},
						"service": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Systemd unit name (e.g. `pveproxy`).",
						},
						"description": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Description of the service.",
						},
						"state": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Execution status of the service process (systemd SubState), e.g. `running`, `dead`, `exited`, `failed`.",
						},
						"active_state": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Current state of the service process (systemd ActiveState). One of: `active`, `inactive`, `failed`, `activating`, `deactivating`, `maintenance`, `reloading`, `refreshing`, `unknown`.",
						},
						"unit_state": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Whether the service is enabled (systemd UnitFileState), e.g. `enabled`, `disabled`, `static`, `masked`.",
						},
					},
				},
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveNodeServicesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = nodeSvcAptConfigureDataSource(req, resp)
}

// Read implements datasource.DataSource.
func (d *pveNodeServicesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveNodeServicesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Error reading pve_node_services", "provider client is not configured")
		return
	}
	node := data.Node.ValueString()
	services, err := d.client.ListNodeServices(ctx, node)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_node_services",
			fmt.Sprintf("listing services on node %s: %s", node, err),
		)
		return
	}
	rows := make([]pveNodeServicesDataSourceEntry, 0, len(services))
	for _, svc := range services {
		rows = append(rows, pveNodeServicesDataSourceEntry{
			Name:        types.StringValue(svc.Name),
			Service:     types.StringValue(svc.Service),
			Description: types.StringValue(svc.Description),
			State:       types.StringValue(svc.State),
			ActiveState: types.StringValue(svc.ActiveState),
			UnitState:   types.StringValue(svc.UnitState),
		})
	}
	data.ID = types.StringValue(node)
	data.Services = rows
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
