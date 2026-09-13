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
	_ datasource.DataSource              = &pveHaRuleDataSource{}
	_ datasource.DataSourceWithConfigure = &pveHaRuleDataSource{}
)

// NewPveHaRuleDataSource returns the data source implementation.
func NewPveHaRuleDataSource() datasource.DataSource {
	return &pveHaRuleDataSource{}
}

// pveHaRuleDataSource reads a single HA rule
// (GET /cluster/ha/rules/{rule}).
type pveHaRuleDataSource struct {
	client *pveclient.Client
}

// pveHaRuleDataSourceModel is the Terraform-facing shape.
type pveHaRuleDataSourceModel struct {
	Rule      types.String `tfsdk:"rule"`
	Type      types.String `tfsdk:"type"`
	Affinity  types.String `tfsdk:"affinity"`
	Nodes     types.List   `tfsdk:"nodes"`
	Resources types.List   `tfsdk:"resources"`
	Strict    types.Bool   `tfsdk:"strict"`
	Disable   types.Bool   `tfsdk:"disable"`
	Comment   types.String `tfsdk:"comment"`
}

// Metadata implements datasource.DataSource.
func (d *pveHaRuleDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveHaRule
}

// Schema implements datasource.DataSource.
func (d *pveHaRuleDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads a single HA rule from `GET /cluster/ha/rules/{rule}`.",
		Attributes: map[string]schema.Attribute{
			"rule": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "HA rule identifier to look up.",
			},
			"type": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "HA rule type: `node-affinity` or `resource-affinity`.",
			},
			"affinity": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Rule affinity: `positive` or `negative`.",
			},
			"nodes": schema.ListAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "For `node-affinity` rules: cluster node members as `<node>` or `<node>:<priority>` entries.",
			},
			"resources": schema.ListAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "HA resource IDs the rule applies to (e.g. `vm:100`, `ct:101`).",
			},
			"strict": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "For `node-affinity` rules: whether the rule is strict.",
			},
			"disable": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether the HA rule is disabled.",
			},
			"comment": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "HA rule description.",
			},
		},
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveHaRuleDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = haConfigureDataSource(req, resp)
}

// Read implements datasource.DataSource.
func (d *pveHaRuleDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveHaRuleDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Error reading pve_ha_rule", "provider client is not configured")
		return
	}

	rule, err := d.client.GetHARule(ctx, data.Rule.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_ha_rule",
			fmt.Sprintf("reading HA rule %s: %s", data.Rule.ValueString(), err),
		)
		return
	}
	data.Type = nodeNetworkStringToTF(rule.Type)
	data.Affinity = nodeNetworkStringToTF(rule.Affinity)
	data.Nodes = listStringToTF(rule.Nodes)
	data.Resources = listStringToTF(rule.Resources)
	data.Strict = nodeNetworkBoolPtrToTF(rule.Strict)
	data.Disable = nodeNetworkBoolPtrToTF(rule.Disable)
	data.Comment = nodeNetworkStringToTF(rule.Comment)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
