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
	_ datasource.DataSource              = &pvePoolDataSource{}
	_ datasource.DataSourceWithConfigure = &pvePoolDataSource{}
)

// NewPvePoolDataSource returns the data source implementation.
func NewPvePoolDataSource() datasource.DataSource {
	return &pvePoolDataSource{}
}

// pvePoolDataSource reads a single resource pool (GET /pools/{poolid}).
type pvePoolDataSource struct {
	client *pveclient.Client
}

// pvePoolDataSourceModel is the Terraform-facing shape.
type pvePoolDataSourceModel struct {
	PoolID  types.String         `tfsdk:"poolid"`
	Comment types.String         `tfsdk:"comment"`
	Members []pvePoolMemberModel `tfsdk:"members"`
}

// Metadata implements datasource.DataSource.
func (d *pvePoolDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePvePool
}

// Schema implements datasource.DataSource.
func (d *pvePoolDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads a Proxmox VE resource pool from `GET /pools/{poolid}`.",
		Attributes: map[string]schema.Attribute{
			"poolid": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The pool identifier to look up.",
			},
			"comment": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Description of the pool, empty when unset.",
			},
			"members": poolMemberDataSourceNestedList(),
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pvePoolDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = haConfigureDataSource(req, resp)
}

// Read implements datasource.DataSource.
func (d *pvePoolDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pvePoolDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Error reading pve_pool", "provider client is not configured")
		return
	}

	pool, err := d.client.GetPool(ctx, data.PoolID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_pool",
			fmt.Sprintf("reading pool %s: %s", data.PoolID.ValueString(), err),
		)
		return
	}
	if pool == nil {
		resp.Diagnostics.AddError(
			"Error reading pve_pool",
			fmt.Sprintf("pool %s not found", data.PoolID.ValueString()),
		)
		return
	}
	data.Comment = types.StringValue(pool.Comment)
	data.Members = poolMemberModels(pool.Members)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// poolMemberDataSourceNestedList returns the computed members list with the
// datasource-schema attribute types.
func poolMemberDataSourceNestedList() schema.ListNestedAttribute {
	return schema.ListNestedAttribute{
		Computed: true,
		NestedObject: schema.NestedAttributeObject{
			Attributes: map[string]schema.Attribute{
				"id": schema.StringAttribute{
					Computed:            true,
					MarkdownDescription: "Member object identifier, e.g. `qemu/100` or `storage/local`.",
				},
				"node": schema.StringAttribute{
					Computed:            true,
					MarkdownDescription: "Node hosting the member object.",
				},
				"storage": schema.StringAttribute{
					Computed:            true,
					MarkdownDescription: "Storage identifier, set only for storage members.",
				},
				"type": schema.StringAttribute{
					Computed:            true,
					MarkdownDescription: "Member type. Must be one of: `qemu`, `lxc`, `openvz`, `storage`.",
				},
				"vmid": schema.Int64Attribute{
					Computed:            true,
					MarkdownDescription: "Guest VMID, set only for guest members.",
				},
			},
		},
		MarkdownDescription: "Current pool members.",
	}
}
