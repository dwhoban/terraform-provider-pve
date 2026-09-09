// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ resource.Resource                = &pveNodeNetworkInterfaceResource{}
	_ resource.ResourceWithConfigure   = &pveNodeNetworkInterfaceResource{}
	_ resource.ResourceWithImportState = &pveNodeNetworkInterfaceResource{}
)

// NewPveNodeNetworkInterfaceResource returns the resource implementation.
func NewPveNodeNetworkInterfaceResource() resource.Resource {
	return &pveNodeNetworkInterfaceResource{}
}

// pveNodeNetworkInterfaceResource manages a single network interface (eth,
// bond, bridge, vlan, OVS bridge, ...) on a PVE node.
type pveNodeNetworkInterfaceResource struct {
	client *pveclient.Client
}

// pveNodeNetworkInterfaceResourceModel is the Terraform-facing shape.
type pveNodeNetworkInterfaceResourceModel struct {
	Node        types.String `tfsdk:"node"`
	Iface       types.String `tfsdk:"iface"`
	Type        types.String `tfsdk:"type"`
	Autostart   types.Bool   `tfsdk:"autostart"`
	CIDR        types.String `tfsdk:"cidr"`
	Address     types.String `tfsdk:"address"`
	Gateway     types.String `tfsdk:"gateway"`
	MTU         types.Int64  `tfsdk:"mtu"`
	Comments    types.String `tfsdk:"comments"`
	Method      types.String `tfsdk:"method"`
	BridgePorts types.String `tfsdk:"bridge_ports"`
	BridgeSTP   types.Bool   `tfsdk:"bridge_stp"`
	BridgeFD    types.Int64  `tfsdk:"bridge_fd"`
	VLANID      types.Int64  `tfsdk:"vlan_id"`
	VLANRawDev  types.String `tfsdk:"vlan_raw_device"`
	OVSBridge   types.String `tfsdk:"ovs_bridge"`
	OVSType     types.String `tfsdk:"ovs_type"`
	OVSOptions  types.Map    `tfsdk:"ovs_options"`
	BondMode    types.String `tfsdk:"bond_mode"`
	BondPrimary types.String `tfsdk:"bond_primary"`
	Slaves      types.List   `tfsdk:"slaves"`
	Delete      types.List   `tfsdk:"delete"`
	Active      types.Bool   `tfsdk:"active"`
	Digest      types.String `tfsdk:"digest"`
}

// ifaceTypes enumerates the values PVE accepts on /nodes/{node}/network.
var ifaceTypes = []string{
	"eth", "bond", "bridge", "vlan",
	"ovsbond", "ovsbridge", "ovsintport", "ovsport", "unknown",
}

// ifaceMethods enumerates the values PVE accepts for the `method` attribute.
var ifaceMethods = []string{"static", "dhcp", "manual", "loopback"}

// bondModes enumerates the values PVE accepts for the `bond_mode` attribute.
// These are the Linux kernel bonding driver modes; PVE forwards them to
// `ip link add type bond mode <value>`.
var bondModes = []string{
	"balance-rr", "active-backup", "balance-xor", "broadcast",
	"802.3ad", "balance-tlb", "balance-alb",
}

// ovsPortTypes enumerates the values PVE accepts for the `ovs_type` attribute.
var ovsPortTypes = []string{"internal", "native", "trunk", "access", "patch"}

// Metadata implements resource.Resource.
func (r *pveNodeNetworkInterfaceResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveNodeNetworkInterface
}

// Schema implements resource.Resource.
func (r *pveNodeNetworkInterfaceResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a single Proxmox VE host network interface (`/nodes/{node}/network`).",
		Attributes: map[string]schema.Attribute{
			"node": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Name of the PVE node.",
			},
			"iface": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Interface name (Linux device name or alias).",
			},
			"type": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Interface type. Must be one of: `eth`, `bond`, `bridge`, `vlan`, `ovsbond`, `ovsbridge`, `ovsintport`, `ovsport`, `unknown`. Changing the type forces replacement because PVE rewrites the underlying device on type change.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.OneOf(ifaceTypes...),
				},
			},
			"autostart": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
				MarkdownDescription: "Bring the interface up automatically on boot. Defaults to `true`.",
			},
			"cidr": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "CIDR-notation address for the interface (alias form).",
			},
			"address": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Plain IP address (with mask as a separate field on the wire).",
			},
			"gateway": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Default gateway for IPv4 traffic.",
			},
			"mtu": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Maximum transmission unit in bytes. Must be between 576 and 65535 inclusive (Linux kernel networking range).",
				Validators: []validator.Int64{
					int64validator.Between(576, 65535),
				},
			},
			"comments": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Free-form text shown in the UI.",
			},
			"method": schema.StringAttribute{
				Optional:            true,
				Validators: []validator.String{
					stringvalidator.OneOf(ifaceMethods...),
				},
			},
			"bridge_ports": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Comma-separated list of member interfaces (bridge type).",
			},
			"bridge_stp": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Enable spanning tree protocol on the bridge.",
			},
			"bridge_fd": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Bridge forward delay in seconds. Must be between 0 and 15 inclusive.",
				Validators: []validator.Int64{
					int64validator.Between(0, 15),
				},
			},
			"vlan_id": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "VLAN tag (vlan type only). Must be between 0 and 4094 inclusive (IEEE 802.1Q range).",
				Validators: []validator.Int64{
					int64validator.Between(0, 4094),
				},
			},
			"vlan_raw_device": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Underlying device the VLAN sits on (vlan type only).",
			},
			"ovs_bridge": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "OVS bridge this interface belongs to.",
			},
			"ovs_type": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "OVS port type. Must be one of: `internal`, `native`, `trunk`, `access`, `patch`.",
				Validators: []validator.String{
					stringvalidator.OneOf(ovsPortTypes...),
				},
			},
			"ovs_options": schema.MapAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Extra OVS options (e.g. `tag=10`).",
			},
			"bond_mode": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Linux kernel bonding driver mode. Must be one of: `balance-rr`, `active-backup`, `balance-xor`, `broadcast`, `802.3ad`, `balance-tlb`, `balance-alb`.",
				Validators: []validator.String{
					stringvalidator.OneOf(bondModes...),
				},
			},
			"bond_primary": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Primary bond slave.",
			},
			"slaves": schema.ListAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Member interfaces of a bond.",
			},
			"delete": schema.ListAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Field names to clear from PVE on the next update (translated to the `delete` query parameter).",
			},
			"active": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether the interface is currently active in the kernel.",
			},
			"digest": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Opaque PVE digest (not used for network updates; populated for parity with other resources).",
			},
		},
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveNodeNetworkInterfaceResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*pveclient.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *pveclient.Client, got: %T.", req.ProviderData),
		)
		return
	}
	r.client = client
}

// Create implements resource.Resource.
func (r *pveNodeNetworkInterfaceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveNodeNetworkInterfaceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := ifaceFromModel(plan)
	if err := r.client.CreateNodeNetwork(ctx, plan.Node.ValueString(), body); err != nil {
		resp.Diagnostics.AddError("Error creating pve_node_network_interface", fmt.Sprintf("creating %s on %s: %s", plan.Iface.ValueString(), plan.Node.ValueString(), err))
		return
	}
	resp.Diagnostics.AddWarning(
		"Network changes pending",
		"The new interface is queued in PVE but not yet applied. Reload the network (`PUT /nodes/{node}/network` or `ifreload -a`) to apply.",
	)
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Error reading pve_node_network_interface after create", fmt.Sprintf("reading %s on %s: %s", plan.Iface.ValueString(), plan.Node.ValueString(), err))
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read implements resource.Resource.
func (r *pveNodeNetworkInterfaceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveNodeNetworkInterfaceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.readInto(ctx, &state); err != nil {
		if isPVEClientNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading pve_node_network_interface", fmt.Sprintf("reading %s on %s: %s", state.Iface.ValueString(), state.Node.ValueString(), err))
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource.
func (r *pveNodeNetworkInterfaceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveNodeNetworkInterfaceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := ifaceFromModel(plan)
	var deleteFields []string
	if !plan.Delete.IsNull() && !plan.Delete.IsUnknown() {
		for _, e := range plan.Delete.Elements() {
			if s, ok := e.(types.String); ok {
				deleteFields = append(deleteFields, s.ValueString())
			}
		}
	}
	if err := r.client.UpdateNodeNetwork(ctx, plan.Node.ValueString(), plan.Iface.ValueString(), body, deleteFields); err != nil {
		resp.Diagnostics.AddError("Error updating pve_node_network_interface", fmt.Sprintf("updating %s on %s: %s", plan.Iface.ValueString(), plan.Node.ValueString(), err))
		return
	}
	resp.Diagnostics.AddWarning(
		"Network changes pending",
		"The interface update is queued in PVE but not yet applied. Reload the network to apply.",
	)
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Error reading pve_node_network_interface after update", fmt.Sprintf("reading %s on %s: %s", plan.Iface.ValueString(), plan.Node.ValueString(), err))
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete implements resource.Resource.
func (r *pveNodeNetworkInterfaceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pveNodeNetworkInterfaceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteNodeNetwork(ctx, state.Node.ValueString(), state.Iface.ValueString()); err != nil {
		if isPVEClientNotFound(err) {
			return
		}
		resp.Diagnostics.AddError("Error deleting pve_node_network_interface", fmt.Sprintf("deleting %s on %s: %s", state.Iface.ValueString(), state.Node.ValueString(), err))
		return
	}
	resp.Diagnostics.AddWarning(
		"Network changes pending",
		"The interface has been deleted from PVE's pending config. Reload the network to apply.",
	)
}

// ImportState parses an import ID of the form `<node>:<iface>` and sets the
// two attributes so subsequent Read can populate the rest.
func (r *pveNodeNetworkInterfaceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"Invalid pve_node_network_interface import ID",
			fmt.Sprintf("Import ID must be in the form `<node>:<iface>`, got %q.", req.ID),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("node"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("iface"), parts[1])...)
}

// ifaceFromModel projects the Terraform model into the pveclient body. We
// deliberately omit null/unknown fields so the wire JSON stays minimal; PVE
// rejects requests that include empty arrays/maps for some attributes.
func ifaceFromModel(m pveNodeNetworkInterfaceResourceModel) pveclient.NetworkInterface {
	body := pveclient.NetworkInterface{
		Iface:     m.Iface.ValueString(),
		Type:      m.Type.ValueString(),
		Autostart: m.Autostart.ValueBool(),
	}
	if !m.CIDR.IsNull() && !m.CIDR.IsUnknown() {
		body.CIDR = m.CIDR.ValueString()
	}
	if !m.Address.IsNull() && !m.Address.IsUnknown() {
		body.Address = m.Address.ValueString()
	}
	if !m.Gateway.IsNull() && !m.Gateway.IsUnknown() {
		body.Gateway = m.Gateway.ValueString()
	}
	if !m.MTU.IsNull() && !m.MTU.IsUnknown() {
		body.MTU = int(m.MTU.ValueInt64())
	}
	if !m.Comments.IsNull() && !m.Comments.IsUnknown() {
		body.Comments = m.Comments.ValueString()
	}
	if !m.Method.IsNull() && !m.Method.IsUnknown() {
		body.Method = m.Method.ValueString()
	}
	if !m.BridgePorts.IsNull() && !m.BridgePorts.IsUnknown() {
		body.BridgePorts = m.BridgePorts.ValueString()
	}
	if !m.BridgeSTP.IsNull() && !m.BridgeSTP.IsUnknown() {
		v := m.BridgeSTP.ValueBool()
		body.BridgeSTP = &v
	}
	if !m.BridgeFD.IsNull() && !m.BridgeFD.IsUnknown() {
		body.BridgeFD = int(m.BridgeFD.ValueInt64())
	}
	if !m.VLANID.IsNull() && !m.VLANID.IsUnknown() {
		body.VLANID = int(m.VLANID.ValueInt64())
	}
	if !m.VLANRawDev.IsNull() && !m.VLANRawDev.IsUnknown() {
		body.VLANRawDevice = m.VLANRawDev.ValueString()
	}
	if !m.OVSBridge.IsNull() && !m.OVSBridge.IsUnknown() {
		body.OVSBridge = m.OVSBridge.ValueString()
	}
	if !m.OVSType.IsNull() && !m.OVSType.IsUnknown() {
		body.OVSType = m.OVSType.ValueString()
	}
	if !m.OVSOptions.IsNull() && !m.OVSOptions.IsUnknown() {
		body.OVSOptions = mapStringFromTF(m.OVSOptions)
	}
	if !m.BondMode.IsNull() && !m.BondMode.IsUnknown() {
		body.BondMode = m.BondMode.ValueString()
	}
	if !m.BondPrimary.IsNull() && !m.BondPrimary.IsUnknown() {
		body.BondPrimary = m.BondPrimary.ValueString()
	}
	if !m.Slaves.IsNull() && !m.Slaves.IsUnknown() {
		body.Slaves = listStringFromTF(m.Slaves)
	}
	return body
}

// readInto populates model from PVE; 404 removes the resource from state.
func (r *pveNodeNetworkInterfaceResource) readInto(ctx context.Context, m *pveNodeNetworkInterfaceResourceModel) error {
	iface, err := r.client.GetNodeNetwork(ctx, m.Node.ValueString(), m.Iface.ValueString())
	if err != nil {
		return err
	}
	m.Type = types.StringValue(iface.Type)
	m.Autostart = types.BoolValue(iface.Autostart)
	m.CIDR = types.StringValue(iface.CIDR)
	m.Address = types.StringValue(iface.Address)
	m.Gateway = types.StringValue(iface.Gateway)
	m.MTU = types.Int64Value(int64(iface.MTU))
	m.Comments = types.StringValue(iface.Comments)
	m.Method = types.StringValue(iface.Method)
	m.BridgePorts = types.StringValue(iface.BridgePorts)
	if iface.BridgeSTP != nil {
		m.BridgeSTP = types.BoolValue(*iface.BridgeSTP)
	} else {
		m.BridgeSTP = types.BoolNull()
	}
	m.BridgeFD = types.Int64Value(int64(iface.BridgeFD))
	m.VLANID = types.Int64Value(int64(iface.VLANID))
	m.VLANRawDev = types.StringValue(iface.VLANRawDevice)
	m.OVSBridge = types.StringValue(iface.OVSBridge)
	m.OVSType = types.StringValue(iface.OVSType)
	if len(iface.OVSOptions) > 0 {
		m.OVSOptions = mapStringToTF(iface.OVSOptions)
	} else {
		m.OVSOptions = types.MapNull(types.StringType)
	}
	m.BondMode = types.StringValue(iface.BondMode)
	m.BondPrimary = types.StringValue(iface.BondPrimary)
	if len(iface.Slaves) > 0 {
		m.Slaves = listStringToTF(iface.Slaves)
	} else {
		m.Slaves = types.ListNull(types.StringType)
	}
	m.Active = types.BoolValue(iface.Active)
	m.Digest = types.StringValue(iface.Digest)
	return nil
}

// mapStringFromTF projects a types.Map of strings into a Go map, skipping
// nulls and unknowns.
func mapStringFromTF(in types.Map) map[string]string {
	out := make(map[string]string, len(in.Elements()))
	for k, v := range in.Elements() {
		s, ok := v.(types.String)
		if !ok || s.IsNull() || s.IsUnknown() {
			continue
		}
		out[k] = s.ValueString()
	}
	return out
}

// mapStringToTF builds a types.Map from a Go map[string]string.
func mapStringToTF(in map[string]string) types.Map {
	elems := make(map[string]attr.Value, len(in))
	for k, v := range in {
		elems[k] = types.StringValue(v)
	}
	m, _ := types.MapValue(types.StringType, elems)
	return m
}

// listStringFromTF flattens a types.List of strings into a []string.
func listStringFromTF(in types.List) []string {
	out := make([]string, 0, len(in.Elements()))
	for _, e := range in.Elements() {
		s, ok := e.(types.String)
		if !ok || s.IsNull() || s.IsUnknown() {
			continue
		}
		out = append(out, s.ValueString())
	}
	return out
}

// listStringToTF builds a types.List from a Go []string.
func listStringToTF(in []string) types.List {
	elems := make([]attr.Value, 0, len(in))
	for _, s := range in {
		elems = append(elems, types.StringValue(s))
	}
	l, _ := types.ListValue(types.StringType, elems)
	return l
}
