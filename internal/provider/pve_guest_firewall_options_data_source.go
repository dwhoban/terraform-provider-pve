// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ datasource.DataSource              = &pveGuestFirewallOptionsDataSource{}
	_ datasource.DataSourceWithConfigure = &pveGuestFirewallOptionsDataSource{}
)

// NewPveGuestFirewallOptionsDataSource returns the data source
// implementation.
func NewPveGuestFirewallOptionsDataSource() datasource.DataSource {
	return &pveGuestFirewallOptionsDataSource{}
}

// pveGuestFirewallOptionsDataSource reads the firewall options singleton of
// one guest (GET /nodes/{node}/{qemu|lxc}/{vmid}/firewall/options).
type pveGuestFirewallOptionsDataSource struct {
	client *pveclient.Client
}

// pveGuestFirewallOptionsDataSourceModel is the Terraform-facing shape.
type pveGuestFirewallOptionsDataSourceModel struct {
	Node      types.String `tfsdk:"node"`
	GuestType types.String `tfsdk:"guest_type"`
	VMID      types.Int64  `tfsdk:"vmid"`
	pveGuestFirewallOptionsOptionSet
	ID types.String `tfsdk:"id"`
}

// guestFirewallOptionsDataSourceAttributes renders the full attribute set
// with the required lookup keys.
func guestFirewallOptionsDataSourceAttributes() map[string]datasourceschema.Attribute {
	attrs := make(map[string]datasourceschema.Attribute, len(guestFirewallOptionsFieldSpecs)+4)
	for _, f := range guestFirewallOptionsFieldSpecs {
		attrs[f.Name] = clusterOptionsDataSourceLeaf(f)
	}
	attrs["node"] = datasourceschema.StringAttribute{
		Required:            true,
		MarkdownDescription: "The cluster node name the guest runs on.",
	}
	attrs["guest_type"] = datasourceschema.StringAttribute{
		Required:            true,
		MarkdownDescription: "The guest type: `qemu` for virtual machines, `lxc` for containers. " + firewallOptionsEnumText(guestFirewallOptionsTypeValues),
		Validators:          []validator.String{stringvalidator.OneOf(guestFirewallOptionsTypeValues...)},
	}
	attrs["vmid"] = datasourceschema.Int64Attribute{
		Required:            true,
		MarkdownDescription: "The (unique) ID of the guest. Must be between 100 and 999999999.",
		Validators:          []validator.Int64{int64validator.Between(100, 999999999)},
	}
	attrs["id"] = datasourceschema.StringAttribute{
		Computed:            true,
		MarkdownDescription: "Identifier of the guest firewall options singleton; formatted `<node>:<guest_type>:<vmid>`.",
	}
	return attrs
}

// Metadata implements datasource.DataSource.
func (d *pveGuestFirewallOptionsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveGuestFirewallOptions
}

// Schema implements datasource.DataSource.
func (d *pveGuestFirewallOptionsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Reads the firewall options singleton of one guest from `GET /nodes/{node}/{qemu|lxc}/{vmid}/firewall/options`.",
		Attributes:          guestFirewallOptionsDataSourceAttributes(),
	}
}

// Configure implements datasource.DataSourceWithConfigure.
func (d *pveGuestFirewallOptionsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
func (d *pveGuestFirewallOptionsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data pveGuestFirewallOptionsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Error reading pve_guest_firewall_options", "provider client is not configured")
		return
	}
	err := guestFirewallOptionsReadInto(ctx, d.client, data.Node.ValueString(), data.GuestType.ValueString(), data.VMID.ValueInt64(), &data.pveGuestFirewallOptionsOptionSet)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_guest_firewall_options",
			fmt.Sprintf("reading firewall options of guest %d on node %s: %s", data.VMID.ValueInt64(), data.Node.ValueString(), err),
		)
		return
	}
	data.ID = types.StringValue(guestFirewallOptionsID(data.Node.ValueString(), data.GuestType.ValueString(), data.VMID.ValueInt64()))
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
