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
	_ datasource.DataSource              = &pveNodeStoragesDataSource{}
	_ datasource.DataSourceWithConfigure = &pveNodeStoragesDataSource{}
)

// NewPveNodeStoragesDataSource returns the data source implementation.
func NewPveNodeStoragesDataSource() datasource.DataSource {
	return &pveNodeStoragesDataSource{}
}

// pveNodeStoragesDataSource lists the storages visible on one node with
// their runtime status (GET /nodes/{node}/storage).
type pveNodeStoragesDataSource struct {
	client *pveclient.Client
}

// pveNodeStoragesDataSourceModel is the Terraform-facing shape.
type pveNodeStoragesDataSourceModel struct {
	ID       types.String                            `tfsdk:"id"`
	Node     types.String                            `tfsdk:"node"`
	Content  types.String                            `tfsdk:"content"`
	Storages []pveNodeStoragesDataSourceStorageModel `tfsdk:"storages"`
}

// pveNodeStoragesDataSourceStorageModel mirrors one row of the storage
// index. Field names line up with the pveclient struct.
type pveNodeStoragesDataSourceStorageModel struct {
	Storage      types.String  `tfsdk:"storage"`
	Type         types.String  `tfsdk:"type"`
	Content      types.List    `tfsdk:"content"`
	Shared       types.Bool    `tfsdk:"shared"`
	Enabled      types.Bool    `tfsdk:"enabled"`
	Active       types.Bool    `tfsdk:"active"`
	Used         types.Int64   `tfsdk:"used"`
	Total        types.Int64   `tfsdk:"total"`
	Avail        types.Int64   `tfsdk:"avail"`
	UsedFraction types.Float64 `tfsdk:"used_fraction"`
}

// Metadata implements datasource.DataSource.
func (d *pveNodeStoragesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveNodeStorages
}

// Schema implements datasource.DataSource.
func (d *pveNodeStoragesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists every storage visible on a node with its runtime status, as reported by `GET /nodes/{node}/storage`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Static identifier for the node storage index.",
			},
			"node": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The cluster node name whose storages to list.",
			},
			"content": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Only list stores which support this content type. One of the PVE content types: `images`, `rootdir`, `vztmpl`, `iso`, `backup`, `snippets`, `import`. PVE filters the list upstream.",
			},
			"storages": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Storages on the node with their runtime status.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"storage": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The storage identifier.",
						},
						"type": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Storage type (the plugin name, e.g. `dir`, `nfs`, `zfspool`).",
						},
						"content": schema.ListAttribute{
							Computed:            true,
							ElementType:         types.StringType,
							MarkdownDescription: "Allowed storage content types.",
						},
						"shared": schema.BoolAttribute{
							Computed:            true,
							MarkdownDescription: "Shared flag from the storage configuration; null when the API did not report it.",
						},
						"enabled": schema.BoolAttribute{
							Computed:            true,
							MarkdownDescription: "Whether the storage is enabled (not disabled); null when the API did not report it.",
						},
						"active": schema.BoolAttribute{
							Computed:            true,
							MarkdownDescription: "Whether the storage is accessible; null when the API did not report it.",
						},
						"used": schema.Int64Attribute{
							Computed:            true,
							MarkdownDescription: "Used storage space in bytes; null when the API did not report it.",
						},
						"total": schema.Int64Attribute{
							Computed:            true,
							MarkdownDescription: "Total storage space in bytes; null when the API did not report it.",
						},
						"avail": schema.Int64Attribute{
							Computed:            true,
							MarkdownDescription: "Available storage space in bytes; null when the API did not report it.",
						},
						"used_fraction": schema.Float64Attribute{
							Computed:            true,
							MarkdownDescription: "Used fraction (used/total); null when the API did not report it.",
						},
					},
				},
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveNodeStoragesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
func (d *pveNodeStoragesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveNodeStoragesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	storages, err := d.client.ListNodeStorages(ctx, data.Node.ValueString(), data.Content.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading pve_node_storages", fmt.Sprintf("listing storages on node %q: %s", data.Node.ValueString(), err))
		return
	}
	rows := make([]pveNodeStoragesDataSourceStorageModel, 0, len(storages))
	for _, s := range storages {
		rows = append(rows, pveNodeStoragesDataSourceStorageModel{
			Storage:      types.StringValue(s.Storage),
			Type:         types.StringValue(s.Type),
			Content:      listStringToTF(s.Content),
			Shared:       nodeStoragesBoolToTF(s.Shared),
			Enabled:      nodeStoragesBoolToTF(s.Enabled),
			Active:       nodeStoragesBoolToTF(s.Active),
			Used:         nodeStoragesInt64ToTF(s.Used),
			Total:        nodeStoragesInt64ToTF(s.Total),
			Avail:        nodeStoragesInt64ToTF(s.Avail),
			UsedFraction: nodeStoragesFloat64ToTF(s.UsedFraction),
		})
	}
	data.ID = types.StringValue("pve_node_storages")
	data.Storages = rows
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// nodeStoragesBoolToTF maps an optional API boolean onto a nullable TF bool.
func nodeStoragesBoolToTF(v *bool) types.Bool {
	if v == nil {
		return types.BoolNull()
	}
	return types.BoolValue(*v)
}

// nodeStoragesInt64ToTF maps an optional API integer onto a nullable TF int.
func nodeStoragesInt64ToTF(v *int64) types.Int64 {
	if v == nil {
		return types.Int64Null()
	}
	return types.Int64Value(*v)
}

// nodeStoragesFloat64ToTF maps an optional API number onto a nullable TF
// float.
func nodeStoragesFloat64ToTF(v *float64) types.Float64 {
	if v == nil {
		return types.Float64Null()
	}
	return types.Float64Value(*v)
}
