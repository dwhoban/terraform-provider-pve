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
	_ datasource.DataSource              = &pveGroupDataSource{}
	_ datasource.DataSourceWithConfigure = &pveGroupDataSource{}
)

// NewPveGroupDataSource returns the data source implementation.
func NewPveGroupDataSource() datasource.DataSource {
	return &pveGroupDataSource{}
}

// pveGroupDataSource reads a single PVE user group via
// /access/groups/{groupid}.
type pveGroupDataSource struct {
	client *pveclient.Client
}

// pveGroupDataSourceModel is the Terraform-facing shape.
type pveGroupDataSourceModel struct {
	ID      types.String `tfsdk:"id"`
	GroupID types.String `tfsdk:"groupid"`
	Comment types.String `tfsdk:"comment"`
	Members types.Set    `tfsdk:"members"`
}

// Metadata implements datasource.DataSource.
func (d *pveGroupDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveGroup
}

// Schema implements datasource.DataSource.
func (d *pveGroupDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads a single user group in Proxmox VE (`GET /access/groups/{groupid}`), including the user IDs currently in it.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Identifier of the group (same value as `groupid`).",
			},
			"groupid": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Group identifier (PVE `pve-groupid` format).",
			},
			"comment": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Comment describing the group; null when the group has none.",
			},
			"members": schema.SetAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Full user IDs (`name@realm`) that are members of this group.",
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveGroupDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
func (d *pveGroupDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveGroupDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	detail, err := d.client.GetAccessGroup(ctx, data.GroupID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_group data source",
			fmt.Sprintf("reading group %s: %s", data.GroupID.ValueString(), err),
		)
		return
	}
	data.ID = types.StringValue(data.GroupID.ValueString())
	data.Comment = accessGroupCommentToTF(detail.Comment)
	members, err := accessGroupMembersToTF(detail.Members)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_group data source",
			fmt.Sprintf("converting members of group %s: %s", data.GroupID.ValueString(), err),
		)
		return
	}
	data.Members = members
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
