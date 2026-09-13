// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

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
	_ resource.Resource                = &pveNodeFirewallOptionsResource{}
	_ resource.ResourceWithConfigure   = &pveNodeFirewallOptionsResource{}
	_ resource.ResourceWithImportState = &pveNodeFirewallOptionsResource{}
)

// NewPveNodeFirewallOptionsResource returns the resource implementation.
func NewPveNodeFirewallOptionsResource() resource.Resource {
	return &pveNodeFirewallOptionsResource{}
}

// pveNodeFirewallOptionsResource manages the host firewall options
// singleton of one node via GET/PUT /nodes/{node}/firewall/options.
type pveNodeFirewallOptionsResource struct {
	client *pveclient.Client
}

// pveNodeFirewallOptionsOptionSet carries the option fields shared by the
// resource and data source models; both embed it with its tfsdk tags.
type pveNodeFirewallOptionsOptionSet struct {
	Enable                           types.Bool   `tfsdk:"enable"`
	LogLevelIn                       types.String `tfsdk:"log_level_in"`
	LogLevelOut                      types.String `tfsdk:"log_level_out"`
	LogLevelForward                  types.String `tfsdk:"log_level_forward"`
	LogNFConntrack                   types.Bool   `tfsdk:"log_nf_conntrack"`
	Ndp                              types.Bool   `tfsdk:"ndp"`
	NFConntrackAllowInvalid          types.Bool   `tfsdk:"nf_conntrack_allow_invalid"`
	NFConntrackHelpers               types.String `tfsdk:"nf_conntrack_helpers"`
	NFConntrackMax                   types.Int64  `tfsdk:"nf_conntrack_max"`
	NFConntrackTCPTimeoutEstablished types.Int64  `tfsdk:"nf_conntrack_tcp_timeout_established"`
	NFConntrackTCPTimeoutSynRecv     types.Int64  `tfsdk:"nf_conntrack_tcp_timeout_syn_recv"`
	Nftables                         types.Bool   `tfsdk:"nftables"`
	Nosmurfs                         types.Bool   `tfsdk:"nosmurfs"`
	ProtectionSynflood               types.Bool   `tfsdk:"protection_synflood"`
	ProtectionSynfloodBurst          types.Int64  `tfsdk:"protection_synflood_burst"`
	ProtectionSynfloodRate           types.Int64  `tfsdk:"protection_synflood_rate"`
	SmurfLogLevel                    types.String `tfsdk:"smurf_log_level"`
	TCPFlags                         types.Bool   `tfsdk:"tcpflags"`
	TCPFlagsLogLevel                 types.String `tfsdk:"tcp_flags_log_level"`
}

// pveNodeFirewallOptionsResourceModel is the Terraform-facing shape of the
// resource.
type pveNodeFirewallOptionsResourceModel struct {
	Node types.String `tfsdk:"node"`
	pveNodeFirewallOptionsOptionSet
	ID types.String `tfsdk:"id"`
}

// nodeFirewallOptionsFieldSpecs is the /nodes/{node}/firewall/options
// attribute set, in wire order, transcribed from the api-spec pin.
var nodeFirewallOptionsFieldSpecs = []clusterOptionsField{
	{Name: "enable", Wire: "enable", Kind: clusterOptionsKindBool, Description: "Enable host firewall rules."},
	{Name: "log_level_in", Wire: "log_level_in", Kind: clusterOptionsKindString, Enum: firewallOptionsLogLevelValues, Description: "Log level for incoming traffic."},
	{Name: "log_level_out", Wire: "log_level_out", Kind: clusterOptionsKindString, Enum: firewallOptionsLogLevelValues, Description: "Log level for outgoing traffic."},
	{Name: "log_level_forward", Wire: "log_level_forward", Kind: clusterOptionsKindString, Enum: firewallOptionsLogLevelValues, Description: "Log level for forwarded traffic."},
	{Name: "log_nf_conntrack", Wire: "log_nf_conntrack", Kind: clusterOptionsKindBool, Description: "Enable logging of conntrack information."},
	{Name: "ndp", Wire: "ndp", Kind: clusterOptionsKindBool, Description: "Enable NDP (Neighbor Discovery Protocol)."},
	{Name: "nf_conntrack_allow_invalid", Wire: "nf_conntrack_allow_invalid", Kind: clusterOptionsKindBool, Description: "Allow invalid packets on connection tracking."},
	{Name: "nf_conntrack_helpers", Wire: "nf_conntrack_helpers", Kind: clusterOptionsKindString, Description: "Enable conntrack helpers for specific protocols. Supported protocols: amanda, ftp, irc, netbios-ns, pptp, sane, sip, snmp, tftp."},
	{Name: "nf_conntrack_max", Wire: "nf_conntrack_max", Kind: clusterOptionsKindInt64, Description: "Maximum number of tracked connections.", Min: clusterOptionsF64(32768)},
	{Name: "nf_conntrack_tcp_timeout_established", Wire: "nf_conntrack_tcp_timeout_established", Kind: clusterOptionsKindInt64, Description: "Conntrack established timeout in seconds.", Min: clusterOptionsF64(7875)},
	{Name: "nf_conntrack_tcp_timeout_syn_recv", Wire: "nf_conntrack_tcp_timeout_syn_recv", Kind: clusterOptionsKindInt64, Description: "Conntrack syn recv timeout in seconds.", Min: clusterOptionsF64(30), Max: clusterOptionsF64(60)},
	{Name: "nftables", Wire: "nftables", Kind: clusterOptionsKindBool, Description: "Enable nftables based firewall (tech preview)."},
	{Name: "nosmurfs", Wire: "nosmurfs", Kind: clusterOptionsKindBool, Description: "Enable SMURFS filter."},
	{Name: "protection_synflood", Wire: "protection_synflood", Kind: clusterOptionsKindBool, Description: "Enable synflood protection."},
	{Name: "protection_synflood_burst", Wire: "protection_synflood_burst", Kind: clusterOptionsKindInt64, Description: "Synflood protection rate burst by ip src."},
	{Name: "protection_synflood_rate", Wire: "protection_synflood_rate", Kind: clusterOptionsKindInt64, Description: "Synflood protection rate syn/sec by ip src."},
	{Name: "smurf_log_level", Wire: "smurf_log_level", Kind: clusterOptionsKindString, Enum: firewallOptionsLogLevelValues, Description: "Log level for SMURFS filter."},
	{Name: "tcpflags", Wire: "tcpflags", Kind: clusterOptionsKindBool, Description: "Filter illegal combinations of TCP flags."},
	{Name: "tcp_flags_log_level", Wire: "tcp_flags_log_level", Kind: clusterOptionsKindString, Enum: firewallOptionsLogLevelValues, Description: "Log level for illegal tcp flags filter."},
}

// nodeFirewallOptionsResourceAttributes renders the full resource attribute
// set.
func nodeFirewallOptionsResourceAttributes() map[string]schema.Attribute {
	attrs := make(map[string]schema.Attribute, len(nodeFirewallOptionsFieldSpecs)+2)
	for _, f := range nodeFirewallOptionsFieldSpecs {
		attrs[f.Name] = clusterOptionsResourceLeaf(f)
	}
	attrs["node"] = schema.StringAttribute{
		Required:            true,
		MarkdownDescription: "The cluster node name. Changing this value forces recreation.",
		PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
	}
	attrs["id"] = schema.StringAttribute{
		Computed:            true,
		MarkdownDescription: "Identifier of the host firewall options singleton; equals the `node` name.",
	}
	return attrs
}

// Metadata implements resource.Resource.
func (r *pveNodeFirewallOptionsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveNodeFirewallOptions
}

// Schema implements resource.Resource.
func (r *pveNodeFirewallOptionsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the host firewall options singleton of one node (`GET/PUT /nodes/{node}/firewall/options`). Every listed attribute is managed: removing an attribute from configuration clears the option via the `delete` parameter. The pin defines no delete verb for this scope, so destroy only forgets the state. Requires `Sys.Modify` on `/nodes/{node}`.",
		Attributes:          nodeFirewallOptionsResourceAttributes(),
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveNodeFirewallOptionsResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// Create implements resource.Resource. A singleton has no upstream create
// verb; writing the planned options is the whole operation.
func (r *pveNodeFirewallOptionsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveNodeFirewallOptionsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error creating pve_node_firewall_options", "provider client is not configured")
		return
	}
	if err := r.client.UpdateNodeFirewallOptions(ctx, plan.Node.ValueString(), nodeFirewallOptionsFromModel(&plan.pveNodeFirewallOptionsOptionSet), nil); err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_node_firewall_options",
			fmt.Sprintf("writing firewall options of node %s: %s", plan.Node.ValueString(), err),
		)
		return
	}
	if err := nodeFirewallOptionsReadInto(ctx, r.client, plan.Node.ValueString(), &plan.pveNodeFirewallOptionsOptionSet); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_node_firewall_options after create",
			fmt.Sprintf("reading firewall options of node %s: %s", plan.Node.ValueString(), err),
		)
		return
	}
	plan.ID = plan.Node
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read implements resource.Resource.
func (r *pveNodeFirewallOptionsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveNodeFirewallOptionsResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := nodeFirewallOptionsReadInto(ctx, r.client, state.Node.ValueString(), &state.pveNodeFirewallOptionsOptionSet); err != nil {
		if isPVEClientNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error reading pve_node_firewall_options",
			fmt.Sprintf("reading firewall options of node %s: %s", state.Node.ValueString(), err),
		)
		return
	}
	state.ID = state.Node
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource. Set fields travel in the PUT body;
// fields cleared in the plan travel in the `delete` query parameter.
func (r *pveNodeFirewallOptionsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveNodeFirewallOptionsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state pveNodeFirewallOptionsResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	deleteFields := nodeFirewallOptionsDeleteFields(&plan.pveNodeFirewallOptionsOptionSet, &state.pveNodeFirewallOptionsOptionSet)
	if err := r.client.UpdateNodeFirewallOptions(ctx, plan.Node.ValueString(), nodeFirewallOptionsFromModel(&plan.pveNodeFirewallOptionsOptionSet), deleteFields); err != nil {
		resp.Diagnostics.AddError(
			"Error updating pve_node_firewall_options",
			fmt.Sprintf("updating firewall options of node %s: %s", plan.Node.ValueString(), err),
		)
		return
	}
	if err := nodeFirewallOptionsReadInto(ctx, r.client, plan.Node.ValueString(), &plan.pveNodeFirewallOptionsOptionSet); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_node_firewall_options after update",
			fmt.Sprintf("reading firewall options of node %s: %s", plan.Node.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete implements resource.Resource. The pin defines no delete verb for
// the host firewall options, so destroy only forgets the state.
func (r *pveNodeFirewallOptionsResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

// ImportState parses an import ID of the form `<node>`.
func (r *pveNodeFirewallOptionsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		resp.Diagnostics.AddError("Invalid pve_node_firewall_options import ID", "import ID must be the node name, e.g. `pve1`")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("node"), req.ID)...)
}

// nodeFirewallOptionsReadInto refreshes the option set from the node.
func nodeFirewallOptionsReadInto(ctx context.Context, client *pveclient.Client, node string, m *pveNodeFirewallOptionsOptionSet) error {
	opts, err := client.GetNodeFirewallOptions(ctx, node)
	if err != nil {
		return err
	}
	nodeFirewallOptionsApply(m, opts)
	return nil
}

// nodeFirewallOptionsFromModel projects the Terraform model into the wire
// struct; null values become nil pointers and are omitted from the PUT body.
func nodeFirewallOptionsFromModel(m *pveNodeFirewallOptionsOptionSet) pveclient.FirewallOptions {
	return pveclient.FirewallOptions{
		Enable:                           firewallOptionsBoolPtr(m.Enable),
		LogLevelIn:                       firewallOptionsStrPtr(m.LogLevelIn),
		LogLevelOut:                      firewallOptionsStrPtr(m.LogLevelOut),
		LogLevelForward:                  firewallOptionsStrPtr(m.LogLevelForward),
		LogNFConntrack:                   firewallOptionsBoolPtr(m.LogNFConntrack),
		Ndp:                              firewallOptionsBoolPtr(m.Ndp),
		NFConntrackAllowInvalid:          firewallOptionsBoolPtr(m.NFConntrackAllowInvalid),
		NFConntrackHelpers:               firewallOptionsStrPtr(m.NFConntrackHelpers),
		NFConntrackMax:                   firewallOptionsInt64Ptr(m.NFConntrackMax),
		NFConntrackTCPTimeoutEstablished: firewallOptionsInt64Ptr(m.NFConntrackTCPTimeoutEstablished),
		NFConntrackTCPTimeoutSynRecv:     firewallOptionsInt64Ptr(m.NFConntrackTCPTimeoutSynRecv),
		Nftables:                         firewallOptionsBoolPtr(m.Nftables),
		Nosmurfs:                         firewallOptionsBoolPtr(m.Nosmurfs),
		ProtectionSynflood:               firewallOptionsBoolPtr(m.ProtectionSynflood),
		ProtectionSynfloodBurst:          firewallOptionsInt64Ptr(m.ProtectionSynfloodBurst),
		ProtectionSynfloodRate:           firewallOptionsInt64Ptr(m.ProtectionSynfloodRate),
		SmurfLogLevel:                    firewallOptionsStrPtr(m.SmurfLogLevel),
		TCPFlags:                         firewallOptionsBoolPtr(m.TCPFlags),
		TCPFlagsLogLevel:                 firewallOptionsStrPtr(m.TCPFlagsLogLevel),
	}
}

// nodeFirewallOptionsApply writes fetched options into the model; absent
// options become null.
func nodeFirewallOptionsApply(m *pveNodeFirewallOptionsOptionSet, o *pveclient.FirewallOptions) {
	m.Enable = nodeNetworkBoolPtrToTF(o.Enable)
	m.LogLevelIn = firewallOptionsStringValue(o.LogLevelIn)
	m.LogLevelOut = firewallOptionsStringValue(o.LogLevelOut)
	m.LogLevelForward = firewallOptionsStringValue(o.LogLevelForward)
	m.LogNFConntrack = nodeNetworkBoolPtrToTF(o.LogNFConntrack)
	m.Ndp = nodeNetworkBoolPtrToTF(o.Ndp)
	m.NFConntrackAllowInvalid = nodeNetworkBoolPtrToTF(o.NFConntrackAllowInvalid)
	m.NFConntrackHelpers = firewallOptionsStringValue(o.NFConntrackHelpers)
	m.NFConntrackMax = firewallOptionsInt64Value(o.NFConntrackMax)
	m.NFConntrackTCPTimeoutEstablished = firewallOptionsInt64Value(o.NFConntrackTCPTimeoutEstablished)
	m.NFConntrackTCPTimeoutSynRecv = firewallOptionsInt64Value(o.NFConntrackTCPTimeoutSynRecv)
	m.Nftables = nodeNetworkBoolPtrToTF(o.Nftables)
	m.Nosmurfs = nodeNetworkBoolPtrToTF(o.Nosmurfs)
	m.ProtectionSynflood = nodeNetworkBoolPtrToTF(o.ProtectionSynflood)
	m.ProtectionSynfloodBurst = firewallOptionsInt64Value(o.ProtectionSynfloodBurst)
	m.ProtectionSynfloodRate = firewallOptionsInt64Value(o.ProtectionSynfloodRate)
	m.SmurfLogLevel = firewallOptionsStringValue(o.SmurfLogLevel)
	m.TCPFlags = nodeNetworkBoolPtrToTF(o.TCPFlags)
	m.TCPFlagsLogLevel = firewallOptionsStringValue(o.TCPFlagsLogLevel)
}

// nodeFirewallOptionsDeleteFields returns the wire names of options present
// in state but cleared in the plan.
func nodeFirewallOptionsDeleteFields(plan, state *pveNodeFirewallOptionsOptionSet) []string {
	var out []string
	if plan.Enable.IsNull() && !state.Enable.IsNull() {
		out = append(out, "enable")
	}
	if plan.LogLevelIn.IsNull() && !state.LogLevelIn.IsNull() {
		out = append(out, "log_level_in")
	}
	if plan.LogLevelOut.IsNull() && !state.LogLevelOut.IsNull() {
		out = append(out, "log_level_out")
	}
	if plan.LogLevelForward.IsNull() && !state.LogLevelForward.IsNull() {
		out = append(out, "log_level_forward")
	}
	if plan.LogNFConntrack.IsNull() && !state.LogNFConntrack.IsNull() {
		out = append(out, "log_nf_conntrack")
	}
	if plan.Ndp.IsNull() && !state.Ndp.IsNull() {
		out = append(out, "ndp")
	}
	if plan.NFConntrackAllowInvalid.IsNull() && !state.NFConntrackAllowInvalid.IsNull() {
		out = append(out, "nf_conntrack_allow_invalid")
	}
	if plan.NFConntrackHelpers.IsNull() && !state.NFConntrackHelpers.IsNull() {
		out = append(out, "nf_conntrack_helpers")
	}
	if plan.NFConntrackMax.IsNull() && !state.NFConntrackMax.IsNull() {
		out = append(out, "nf_conntrack_max")
	}
	if plan.NFConntrackTCPTimeoutEstablished.IsNull() && !state.NFConntrackTCPTimeoutEstablished.IsNull() {
		out = append(out, "nf_conntrack_tcp_timeout_established")
	}
	if plan.NFConntrackTCPTimeoutSynRecv.IsNull() && !state.NFConntrackTCPTimeoutSynRecv.IsNull() {
		out = append(out, "nf_conntrack_tcp_timeout_syn_recv")
	}
	if plan.Nftables.IsNull() && !state.Nftables.IsNull() {
		out = append(out, "nftables")
	}
	if plan.Nosmurfs.IsNull() && !state.Nosmurfs.IsNull() {
		out = append(out, "nosmurfs")
	}
	if plan.ProtectionSynflood.IsNull() && !state.ProtectionSynflood.IsNull() {
		out = append(out, "protection_synflood")
	}
	if plan.ProtectionSynfloodBurst.IsNull() && !state.ProtectionSynfloodBurst.IsNull() {
		out = append(out, "protection_synflood_burst")
	}
	if plan.ProtectionSynfloodRate.IsNull() && !state.ProtectionSynfloodRate.IsNull() {
		out = append(out, "protection_synflood_rate")
	}
	if plan.SmurfLogLevel.IsNull() && !state.SmurfLogLevel.IsNull() {
		out = append(out, "smurf_log_level")
	}
	if plan.TCPFlags.IsNull() && !state.TCPFlags.IsNull() {
		out = append(out, "tcpflags")
	}
	if plan.TCPFlagsLogLevel.IsNull() && !state.TCPFlagsLogLevel.IsNull() {
		out = append(out, "tcp_flags_log_level")
	}
	return out
}
