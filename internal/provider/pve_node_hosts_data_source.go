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
	_ datasource.DataSource              = &pveNodeHostsDataSource{}
	_ datasource.DataSourceWithConfigure = &pveNodeHostsDataSource{}
)

// NewPveNodeHostsDataSource returns the /etc/hosts data source
// implementation.
func NewPveNodeHostsDataSource() datasource.DataSource {
	return &pveNodeHostsDataSource{}
}

// pveNodeHostsDataSource exposes the current /etc/hosts content of one
// node (GET /nodes/{node}/hosts) as parsed entries.
type pveNodeHostsDataSource struct {
	client *pveclient.Client
}

// pveNodeHostsDataSourceModel is the Terraform-facing shape.
type pveNodeHostsDataSourceModel struct {
	Node    types.String             `tfsdk:"node"`
	Entries []pveNodeHostsEntryModel `tfsdk:"entries"`
	Digest  types.String             `tfsdk:"digest"`
	ID      types.String             `tfsdk:"id"`
}

// Metadata implements datasource.DataSource.
func (d *pveNodeHostsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveNodeHosts
}

// Schema implements datasource.DataSource.
func (d *pveNodeHostsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads the `/etc/hosts` file of one node (`GET /nodes/{node}/hosts`) as ordered entries. Blank lines and `#` comments are skipped.",
		Attributes: map[string]schema.Attribute{
			"node": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The cluster node name whose `/etc/hosts` is read.",
			},
			"entries": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Ordered entries of the hosts file.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"address": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "IP address of the entry.",
						},
						"hostnames": schema.ListAttribute{
							Computed:            true,
							ElementType:         types.StringType,
							MarkdownDescription: "Hostnames of the entry; the first is the canonical name.",
						},
					},
				},
			},
			"digest": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Digest of the current file as reported by the node.",
			},
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Identifier of the data source; equals `node`.",
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveNodeHostsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
func (d *pveNodeHostsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveNodeHostsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Error reading pve_node_hosts", "provider client is not configured")
		return
	}
	node := data.Node.ValueString()
	hosts, err := d.client.GetNodeHosts(ctx, node)
	if err != nil {
		resp.Diagnostics.AddError("Error reading pve_node_hosts", fmt.Sprintf("reading hosts (GET /nodes/%s/hosts) for node %s: %s", node, node, err))
		return
	}
	data.Entries = nodeHostsParse(hosts.Data)
	data.Digest = nodeNetworkStringToTF(hosts.Digest)
	data.ID = types.StringValue(node)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
