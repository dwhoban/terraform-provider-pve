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
	_ datasource.DataSource              = &pveAppliancesDataSource{}
	_ datasource.DataSourceWithConfigure = &pveAppliancesDataSource{}
)

// NewPveAppliancesDataSource returns the data source implementation.
func NewPveAppliancesDataSource() datasource.DataSource {
	return &pveAppliancesDataSource{}
}

// pveAppliancesDataSource lists the appliance templates available on a
// node (GET /nodes/{node}/aplinfo).
type pveAppliancesDataSource struct {
	client *pveclient.Client
}

// pveAppliancesDataSourceModel is the Terraform-facing shape.
type pveAppliancesDataSourceModel struct {
	ID         types.String                            `tfsdk:"id"`
	Node       types.String                            `tfsdk:"node"`
	Appliances []pveAppliancesDataSourceApplianceModel `tfsdk:"appliances"`
}

// pveAppliancesDataSourceApplianceModel mirrors the per-row schema of the
// appliance template list. Field names line up with the pveclient struct.
type pveAppliancesDataSourceApplianceModel struct {
	Template       types.String `tfsdk:"template"`
	Source         types.String `tfsdk:"source"`
	Infrastructure types.String `tfsdk:"infrastructure"`
	Description    types.String `tfsdk:"description"`
	Package        types.String `tfsdk:"package"`
	Section        types.String `tfsdk:"section"`
	Version        types.String `tfsdk:"version"`
	OS             types.String `tfsdk:"os"`
	Info           types.String `tfsdk:"info"`
	ManageURL      types.String `tfsdk:"manage_url"`
	SHA512Sum      types.String `tfsdk:"sha512sum"`
	Architecture   types.String `tfsdk:"architecture"`
}

// Metadata implements datasource.DataSource.
func (d *pveAppliancesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveAppliances
}

// Schema implements datasource.DataSource.
func (d *pveAppliancesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists the appliance templates (container templates and virtual appliance " +
			"images) available on a node as reported by `GET /nodes/{node}/aplinfo`. " +
			"Templates are downloaded to a storage via `POST /nodes/{node}/aplinfo`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Identifier of this data source (the node name).",
			},
			"node": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The cluster node whose appliance list is read.",
			},
			"appliances": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Appliance templates available on the node.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"template": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "File name of the template archive, e.g. `ubuntu-24.04-standard_24.04-2_amd64.tar.zst`.",
						},
						"source": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Source the template was retrieved from.",
						},
						"infrastructure": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Infrastructure repository the template belongs to, e.g. `pve`.",
						},
						"description": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Human-readable description of the appliance.",
						},
						"package": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Package name of the appliance.",
						},
						"section": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Category of the appliance, e.g. `system` or `mail`.",
						},
						"version": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Version of the appliance package.",
						},
						"os": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Operating system identifier of the appliance, e.g. `ubuntu-24.04`.",
						},
						"info": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "URL with more information about the appliance.",
						},
						"manage_url": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Management URL to open after deployment (`<ip>` is replaced at runtime).",
						},
						"sha512sum": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "SHA-512 checksum of the template archive.",
						},
						"architecture": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Target architecture of the template, e.g. `amd64`.",
						},
					},
				},
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveAppliancesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
func (d *pveAppliancesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveAppliancesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Unconfigured pve_appliances",
			fmt.Sprintf("The provider client was not configured; cannot list appliances on node %s.", data.Node.ValueString()),
		)
		return
	}
	node := data.Node.ValueString()
	appliances, err := d.client.ListNodeAppliances(ctx, node)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_appliances",
			fmt.Sprintf("listing appliance templates on node %s: %s", node, err),
		)
		return
	}
	rows := make([]pveAppliancesDataSourceApplianceModel, 0, len(appliances))
	for _, a := range appliances {
		rows = append(rows, pveAppliancesDataSourceApplianceModel{
			Template:       types.StringValue(a.Template),
			Source:         types.StringValue(a.Source),
			Infrastructure: types.StringValue(a.Infrastructure),
			Description:    types.StringValue(a.Description),
			Package:        types.StringValue(a.Package),
			Section:        types.StringValue(a.Section),
			Version:        types.StringValue(a.Version),
			OS:             types.StringValue(a.OS),
			Info:           types.StringValue(a.Info),
			ManageURL:      types.StringValue(a.ManageURL),
			SHA512Sum:      types.StringValue(a.SHA512Sum),
			Architecture:   types.StringValue(a.Architecture),
		})
	}
	data.ID = types.StringValue(node)
	data.Appliances = rows
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
