// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ resource.Resource                = &pveSdnSubnetResource{}
	_ resource.ResourceWithConfigure   = &pveSdnSubnetResource{}
	_ resource.ResourceWithImportState = &pveSdnSubnetResource{}
)

// NewPveSdnSubnetResource returns the resource implementation.
func NewPveSdnSubnetResource() resource.Resource {
	return &pveSdnSubnetResource{}
}

// pveSdnSubnetResource manages an SDN subnet object nested under a vnet
// (/cluster/sdn/vnets/{vnet}/subnets). Every mutation is synchronous per
// the pin.
type pveSdnSubnetResource struct {
	client *pveclient.Client
}

// pveSdnSubnetResourceModel is the Terraform-facing shape.
type pveSdnSubnetResourceModel struct {
	Vnet          types.String `tfsdk:"vnet"`
	Subnet        types.String `tfsdk:"subnet"`
	Gateway       types.String `tfsdk:"gateway"`
	Snat          types.Bool   `tfsdk:"snat"`
	DhcpDnsServer types.String `tfsdk:"dhcp_dns_server"`
	DhcpRange     types.List   `tfsdk:"dhcp_range"`
	Dnszoneprefix types.String `tfsdk:"dnszoneprefix"`
}

// Metadata implements resource.Resource.
func (r *pveSdnSubnetResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveSdnSubnet
}

// Schema implements resource.Resource.
func (r *pveSdnSubnetResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages an SDN subnet object nested under a vnet (`/cluster/sdn/vnets/{vnet}/subnets`). Changes stay pending until the `pve_sdn_apply` action runs.",
		Attributes: map[string]schema.Attribute{
			"vnet": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The vnet this subnet belongs to. Changing this value forces recreation.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"subnet": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The subnet identifier in CIDR form; PVE writes the mask separator as a hyphen (for example `10.0.0.0-24` for `10.0.0.0/24`). Changing this value forces recreation.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"gateway": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Subnet gateway, assigned on the vnet for layer3 zones.",
			},
			"snat": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Enable masquerade for this subnet when the pve-firewall is used.",
			},
			"dhcp_dns_server": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "IP address of the DHCP DNS server.",
			},
			"dhcp_range": schema.ListAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "DHCP ranges for this subnet, each written as `start-end` (for example `10.0.0.100-10.0.0.200`).",
			},
			"dnszoneprefix": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "DNS domain zone prefix; with prefix `adm`, hostnames register as `<hostname>.adm.mydomain.com`.",
			},
		},
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveSdnSubnetResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = haConfigureResource(req, resp)
}

// Create implements resource.Resource.
func (r *pveSdnSubnetResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveSdnSubnetResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error creating pve_sdn_subnet", "provider client is not configured")
		return
	}
	if err := r.client.CreateSdnSubnet(ctx, plan.Vnet.ValueString(), sdnSubnetFromModel(plan)); err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_sdn_subnet",
			fmt.Sprintf("creating SDN subnet %s on vnet %s: %s", plan.Subnet.ValueString(), plan.Vnet.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_sdn_subnet after create",
			fmt.Sprintf("reading SDN subnet %s on vnet %s: %s", plan.Subnet.ValueString(), plan.Vnet.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read implements resource.Resource.
func (r *pveSdnSubnetResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveSdnSubnetResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.readInto(ctx, &state); err != nil {
		if isPVEClientNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error reading pve_sdn_subnet",
			fmt.Sprintf("reading SDN subnet %s on vnet %s: %s", state.Subnet.ValueString(), state.Vnet.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource.
func (r *pveSdnSubnetResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveSdnSubnetResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state pveSdnSubnetResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error updating pve_sdn_subnet", "provider client is not configured")
		return
	}
	deleteFields := sdnSubnetDeleteFields(plan, state)
	if err := r.client.UpdateSdnSubnet(ctx, plan.Vnet.ValueString(), plan.Subnet.ValueString(), sdnSubnetFromModel(plan), deleteFields); err != nil {
		resp.Diagnostics.AddError(
			"Error updating pve_sdn_subnet",
			fmt.Sprintf("updating SDN subnet %s on vnet %s: %s", plan.Subnet.ValueString(), plan.Vnet.ValueString(), err),
		)
		return
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_sdn_subnet after update",
			fmt.Sprintf("reading SDN subnet %s on vnet %s: %s", plan.Subnet.ValueString(), plan.Vnet.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete implements resource.Resource.
func (r *pveSdnSubnetResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveSdnSubnetResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error deleting pve_sdn_subnet", "provider client is not configured")
		return
	}
	if err := r.client.DeleteSdnSubnet(ctx, state.Vnet.ValueString(), state.Subnet.ValueString()); err != nil {
		if isPVEClientNotFound(err) {
			// Already absent counts as deleted.
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting pve_sdn_subnet",
			fmt.Sprintf("deleting SDN subnet %s on vnet %s: %s", state.Subnet.ValueString(), state.Vnet.ValueString(), err),
		)
	}
}

// ImportState parses an import ID of the form `<vnet>:<subnet>`.
func (r *pveSdnSubnetResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, ":")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError("Invalid pve_sdn_subnet import ID", "import ID must be `<vnet>:<subnet>`, e.g. `vnet1:10.0.0.0-24`")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("vnet"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("subnet"), parts[1])...)
}

// readInto refreshes the model from PVE.
func (r *pveSdnSubnetResource) readInto(ctx context.Context, m *pveSdnSubnetResourceModel) error {
	subnet, err := r.client.GetSdnSubnet(ctx, m.Vnet.ValueString(), m.Subnet.ValueString())
	if err != nil {
		return err
	}
	m.Gateway = nodeNetworkStringToTF(subnet.Gateway)
	m.Snat = nodeNetworkBoolPtrToTF(subnet.Snat)
	m.DhcpDnsServer = nodeNetworkStringToTF(subnet.DhcpDnsServer)
	m.DhcpRange = listStringToTF(subnet.DhcpRange)
	m.Dnszoneprefix = nodeNetworkStringToTF(subnet.Dnszoneprefix)
	return nil
}

// sdnSubnetFromModel projects the Terraform model into the wire body.
func sdnSubnetFromModel(m pveSdnSubnetResourceModel) pveclient.SdnSubnet {
	body := pveclient.SdnSubnet{
		Subnet:  m.Subnet.ValueString(),
		Gateway: m.Gateway.ValueString(),
	}
	if !m.Snat.IsNull() && !m.Snat.IsUnknown() {
		body.Snat = pveclient.HABoolPtr(m.Snat.ValueBool())
	}
	if !m.DhcpDnsServer.IsNull() && !m.DhcpDnsServer.IsUnknown() {
		body.DhcpDnsServer = m.DhcpDnsServer.ValueString()
	}
	body.DhcpRange = listStringFromTF(m.DhcpRange)
	if !m.Dnszoneprefix.IsNull() && !m.Dnszoneprefix.IsUnknown() {
		body.Dnszoneprefix = m.Dnszoneprefix.ValueString()
	}
	return body
}

// sdnSubnetDeleteFields returns the PVE field names to clear on update:
// optional attributes present in state but null in plan.
func sdnSubnetDeleteFields(plan, state pveSdnSubnetResourceModel) []string {
	var out []string
	if plan.Gateway.IsNull() && !state.Gateway.IsNull() {
		out = append(out, "gateway")
	}
	if plan.Snat.IsNull() && !state.Snat.IsNull() {
		out = append(out, "snat")
	}
	if plan.DhcpDnsServer.IsNull() && !state.DhcpDnsServer.IsNull() {
		out = append(out, "dhcp-dns-server")
	}
	if plan.DhcpRange.IsNull() && !state.DhcpRange.IsNull() {
		out = append(out, "dhcp-range")
	}
	if plan.Dnszoneprefix.IsNull() && !state.Dnszoneprefix.IsNull() {
		out = append(out, "dnszoneprefix")
	}
	return out
}
