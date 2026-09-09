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

	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ datasource.DataSource              = &pveNodeCapabilitiesDataSource{}
	_ datasource.DataSourceWithConfigure = &pveNodeCapabilitiesDataSource{}
)

// NewPveNodeCapabilitiesDataSource returns the data source implementation.
func NewPveNodeCapabilitiesDataSource() datasource.DataSource {
	return &pveNodeCapabilitiesDataSource{}
}

// pveNodeCapabilitiesDataSource merges the QEMU capability reads of one
// node (qemu/cpu, qemu/machines, qemu/migration). The cluster-wide
// cpu-flags endpoint is not part of this data source.
type pveNodeCapabilitiesDataSource struct {
	client *pveclient.Client
}

// pveNodeCapabilitiesDataSourceModel is the Terraform-facing shape.
type pveNodeCapabilitiesDataSourceModel struct {
	ID                types.String                                         `tfsdk:"id"`
	Node              types.String                                         `tfsdk:"node"`
	CPUModels         []pveNodeCapabilitiesDataSourceCPUModelModel         `tfsdk:"cpu_models"`
	Machines          []pveNodeCapabilitiesDataSourceMachineModel          `tfsdk:"machines"`
	MigrationFeatures []pveNodeCapabilitiesDataSourceMigrationFeatureModel `tfsdk:"migration_features"`
}

// pveNodeCapabilitiesDataSourceCPUModelModel mirrors one CPU model row.
type pveNodeCapabilitiesDataSourceCPUModelModel struct {
	Name     types.String `tfsdk:"name"`
	Vendor   types.String `tfsdk:"vendor"`
	Custom   types.Bool   `tfsdk:"custom"`
	Abstract types.Bool   `tfsdk:"abstract"`
}

// pveNodeCapabilitiesDataSourceMachineModel mirrors one machine type row.
type pveNodeCapabilitiesDataSourceMachineModel struct {
	ID      types.String `tfsdk:"id"`
	Version types.String `tfsdk:"version"`
	Type    types.String `tfsdk:"type"`
	Changes types.String `tfsdk:"changes"`
}

// pveNodeCapabilitiesDataSourceMigrationFeatureModel carries one migration
// capability flag.
type pveNodeCapabilitiesDataSourceMigrationFeatureModel struct {
	HasDbusVMState types.Bool `tfsdk:"has_dbus_vmstate"`
}

// Metadata implements datasource.DataSource.
func (d *pveNodeCapabilitiesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveNodeCapabilities
}

// Schema implements datasource.DataSource.
func (d *pveNodeCapabilitiesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Merges the QEMU capabilities of one node: available CPU models (`GET /nodes/{node}/capabilities/qemu/cpu`), machine types (`GET /nodes/{node}/capabilities/qemu/machines`), and migration capabilities (`GET /nodes/{node}/capabilities/qemu/migration`).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Identifier in `node` form (same as the `node` attribute).",
			},
			"node": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The cluster node name whose capabilities to read.",
			},
			"cpu_models": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "All default and custom CPU models usable by VMs on this node.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The CPU model name, prefixed with `custom-` for custom models.",
						},
						"vendor": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The CPU vendor visible to the guest when this model is selected.",
						},
						"custom": schema.BoolAttribute{
							Computed:            true,
							MarkdownDescription: "Whether this is a custom CPU model.",
						},
						"abstract": schema.BoolAttribute{
							Computed:            true,
							MarkdownDescription: "Whether this is a PVE-internal abstract profile (e.g. `x86-64-v2`) that cannot be used as a custom model's `reported-model`; null when not reported.",
						},
					},
				},
			},
			"machines": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Available QEMU/KVM machine types.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Full name of machine type and version (e.g. `pc-q35-8.0`).",
						},
						"version": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The machine version (e.g. `8.0`).",
						},
						"type": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The machine type: `q35` or `i440fx`.",
						},
						"changes": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Notable changes of the version, currently only set for `+pveX` versions; null otherwise.",
						},
					},
				},
			},
			"migration_features": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Single-element list carrying the node's QEMU live-migration capabilities.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"has_dbus_vmstate": schema.BoolAttribute{
							Computed:            true,
							MarkdownDescription: "Whether the host supports live-migrating additional VM state via the dbus-vmstate helper.",
						},
					},
				},
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveNodeCapabilitiesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
func (d *pveNodeCapabilitiesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config pveNodeCapabilitiesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	node := config.Node.ValueString()
	caps, err := d.client.GetNodeCapabilities(ctx, node)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_node_capabilities",
			fmt.Sprintf("reading QEMU capabilities for node %s: %s", node, err),
		)
		return
	}
	config.CPUModels = make([]pveNodeCapabilitiesDataSourceCPUModelModel, 0, len(caps.CPUModels))
	for _, model := range caps.CPUModels {
		config.CPUModels = append(config.CPUModels, pveNodeCapabilitiesDataSourceCPUModelModel{
			Name:     types.StringValue(model.Name),
			Vendor:   types.StringValue(model.Vendor),
			Custom:   types.BoolValue(model.Custom),
			Abstract: types.BoolPointerValue(model.Abstract),
		})
	}
	config.Machines = make([]pveNodeCapabilitiesDataSourceMachineModel, 0, len(caps.Machines))
	for _, machine := range caps.Machines {
		config.Machines = append(config.Machines, pveNodeCapabilitiesDataSourceMachineModel{
			ID:      types.StringValue(machine.ID),
			Version: types.StringValue(machine.Version),
			Type:    types.StringPointerValue(machine.Type),
			Changes: types.StringPointerValue(machine.Changes),
		})
	}
	config.MigrationFeatures = []pveNodeCapabilitiesDataSourceMigrationFeatureModel{{
		HasDbusVMState: types.BoolValue(caps.MigrationFeatures.HasDbusVMState),
	}}
	config.ID = types.StringValue(node)
	tflog.Debug(ctx, "read node capabilities", map[string]any{"node": node, "cpu_models": len(config.CPUModels), "machines": len(config.Machines)})
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
