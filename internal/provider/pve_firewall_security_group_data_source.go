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
	_ datasource.DataSource              = &pveFirewallSecurityGroupDataSource{}
	_ datasource.DataSourceWithConfigure = &pveFirewallSecurityGroupDataSource{}
)

// NewPveFirewallSecurityGroupDataSource returns the data source
// implementation.
func NewPveFirewallSecurityGroupDataSource() datasource.DataSource {
	return &pveFirewallSecurityGroupDataSource{}
}

// pveFirewallSecurityGroupDataSource reads a single security group's
// metadata (GET /cluster/firewall/groups, filtered by group name).
type pveFirewallSecurityGroupDataSource struct {
	client *pveclient.Client
}

// pveFirewallSecurityGroupDataSourceModel is the Terraform-facing shape.
type pveFirewallSecurityGroupDataSourceModel struct {
	Group   types.String `tfsdk:"group"`
	Comment types.String `tfsdk:"comment"`
}

// Metadata implements datasource.DataSource.
func (d *pveFirewallSecurityGroupDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveFirewallSecurityGroup
}

// Schema implements datasource.DataSource.
func (d *pveFirewallSecurityGroupDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads a single security group's metadata from `GET /cluster/firewall/groups`. The group's rules are exposed by `pve_security_group_firewall_rules`.",
		Attributes: map[string]schema.Attribute{
			"group": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Security Group name to look up.",
			},
			"comment": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Optional comment or description, or null when the group has none.",
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveFirewallSecurityGroupDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = firewallEntityConfigureDataSource(req, resp)
}

// Read implements datasource.DataSource.
func (d *pveFirewallSecurityGroupDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveFirewallSecurityGroupDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Error reading pve_firewall_security_group", "provider client is not configured")
		return
	}

	// The pin's GET /cluster/firewall/groups/{group} returns the group's
	// rules rather than its metadata, so read the listing and filter.
	groups, err := d.client.ListFirewallSecurityGroups(ctx)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_firewall_security_group",
			fmt.Sprintf("listing firewall security groups for %s: %s", data.Group.ValueString(), err),
		)
		return
	}
	found := false
	for _, group := range groups {
		if group.Group == data.Group.ValueString() {
			data.Comment = nodeNetworkStringToTF(group.Comment)
			found = true
			break
		}
	}
	if !found {
		resp.Diagnostics.AddError(
			"Error reading pve_firewall_security_group",
			fmt.Sprintf("firewall security group %s not found", data.Group.ValueString()),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
