// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// Ensure framework interfaces are satisfied.
var (
	_ resource.Resource                = &pveGuestFirewallOptionsResource{}
	_ resource.ResourceWithConfigure   = &pveGuestFirewallOptionsResource{}
	_ resource.ResourceWithImportState = &pveGuestFirewallOptionsResource{}
)

// guestFirewallOptionsTypeValues is the closed guest type set of the pin.
var guestFirewallOptionsTypeValues = []string{"qemu", "lxc"}

// NewPveGuestFirewallOptionsResource returns the resource implementation.
func NewPveGuestFirewallOptionsResource() resource.Resource {
	return &pveGuestFirewallOptionsResource{}
}

// pveGuestFirewallOptionsResource manages the firewall options singleton of
// one guest (qemu VM or lxc container) via
// GET/PUT /nodes/{node}/{qemu|lxc}/{vmid}/firewall/options.
type pveGuestFirewallOptionsResource struct {
	client *pveclient.Client
}

// pveGuestFirewallOptionsOptionSet carries the option fields shared by the
// resource and data source models; both embed it with its tfsdk tags.
type pveGuestFirewallOptionsOptionSet struct {
	Enable      types.Bool   `tfsdk:"enable"`
	DHCP        types.Bool   `tfsdk:"dhcp"`
	IPFilter    types.Bool   `tfsdk:"ipfilter"`
	MacFilter   types.Bool   `tfsdk:"macfilter"`
	Ndp         types.Bool   `tfsdk:"ndp"`
	Radv        types.Bool   `tfsdk:"radv"`
	LogLevelIn  types.String `tfsdk:"log_level_in"`
	LogLevelOut types.String `tfsdk:"log_level_out"`
	PolicyIn    types.String `tfsdk:"policy_in"`
	PolicyOut   types.String `tfsdk:"policy_out"`
}

// pveGuestFirewallOptionsResourceModel is the Terraform-facing shape of the
// resource.
type pveGuestFirewallOptionsResourceModel struct {
	Node      types.String `tfsdk:"node"`
	GuestType types.String `tfsdk:"guest_type"`
	VMID      types.Int64  `tfsdk:"vmid"`
	pveGuestFirewallOptionsOptionSet
	ID types.String `tfsdk:"id"`
}

// guestFirewallOptionsFieldSpecs is the guest firewall options attribute
// set, in wire order, transcribed from the api-spec pin
// (GET/PUT /nodes/{node}/{qemu|lxc}/{vmid}/firewall/options; the pin
// declares an identical field set for both guest types).
var guestFirewallOptionsFieldSpecs = []clusterOptionsField{
	{Name: "enable", Wire: "enable", Kind: clusterOptionsKindBool, Description: "Enable/disable firewall rules."},
	{Name: "dhcp", Wire: "dhcp", Kind: clusterOptionsKindBool, Description: "Enable DHCP."},
	{Name: "ipfilter", Wire: "ipfilter", Kind: clusterOptionsKindBool, Description: "Enable default IP filters. This is equivalent to adding an empty ipfilter-net<id> ipset for every interface; such ipsets implicitly contain sane default restrictions such as restricting IPv6 link local addresses to the one derived from the interface's MAC address, and for containers the configured IP addresses will be implicitly added."},
	{Name: "log_level_in", Wire: "log_level_in", Kind: clusterOptionsKindString, Enum: firewallOptionsLogLevelValues, Description: "Log level for incoming traffic."},
	{Name: "log_level_out", Wire: "log_level_out", Kind: clusterOptionsKindString, Enum: firewallOptionsLogLevelValues, Description: "Log level for outgoing traffic."},
	{Name: "macfilter", Wire: "macfilter", Kind: clusterOptionsKindBool, Description: "Enable/disable MAC address filter."},
	{Name: "ndp", Wire: "ndp", Kind: clusterOptionsKindBool, Description: "Enable NDP (Neighbor Discovery Protocol)."},
	{Name: "policy_in", Wire: "policy_in", Kind: clusterOptionsKindString, Enum: firewallOptionsPolicyInValues, Description: "Input policy."},
	{Name: "policy_out", Wire: "policy_out", Kind: clusterOptionsKindString, Enum: firewallOptionsPolicyInValues, Description: "Output policy."},
	{Name: "radv", Wire: "radv", Kind: clusterOptionsKindBool, Description: "Allow sending Router Advertisement."},
}

// guestFirewallOptionsResourceAttributes renders the full resource attribute
// set.
func guestFirewallOptionsResourceAttributes() map[string]schema.Attribute {
	attrs := make(map[string]schema.Attribute, len(guestFirewallOptionsFieldSpecs)+4)
	for _, f := range guestFirewallOptionsFieldSpecs {
		attrs[f.Name] = clusterOptionsResourceLeaf(f)
	}
	attrs["node"] = schema.StringAttribute{
		Required:            true,
		MarkdownDescription: "The cluster node name the guest runs on. Changing this value forces recreation.",
		PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
	}
	attrs["guest_type"] = schema.StringAttribute{
		Required:            true,
		MarkdownDescription: "The guest type: `qemu` for virtual machines, `lxc` for containers. " + firewallOptionsEnumText(guestFirewallOptionsTypeValues),
		Validators:          []validator.String{stringvalidator.OneOf(guestFirewallOptionsTypeValues...)},
		PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
	}
	attrs["vmid"] = schema.Int64Attribute{
		Required:            true,
		MarkdownDescription: "The (unique) ID of the guest. Must be between 100 and 999999999. Changing this value forces recreation.",
		Validators:          []validator.Int64{int64validator.Between(100, 999999999)},
		PlanModifiers:       []planmodifier.Int64{int64planmodifier.RequiresReplace()},
	}
	attrs["id"] = schema.StringAttribute{
		Computed:            true,
		MarkdownDescription: "Identifier of the guest firewall options singleton; formatted `<node>:<guest_type>:<vmid>`.",
	}
	return attrs
}

// Metadata implements resource.Resource.
func (r *pveGuestFirewallOptionsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + TypeNamePveGuestFirewallOptions
}

// Schema implements resource.Resource.
func (r *pveGuestFirewallOptionsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the firewall options singleton of one guest (`GET/PUT /nodes/{node}/{qemu|lxc}/{vmid}/firewall/options`). Every listed attribute is managed: removing an attribute from configuration clears the option via the `delete` parameter. The pin defines no delete verb for this scope, so destroy only forgets the state. Requires `VM.Config.Network` on `/vms/{vmid}`.",
		Attributes:          guestFirewallOptionsResourceAttributes(),
	}
}

// Configure implements resource.ResourceWithConfigure.
func (r *pveGuestFirewallOptionsResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
func (r *pveGuestFirewallOptionsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pveGuestFirewallOptionsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Error creating pve_guest_firewall_options", "provider client is not configured")
		return
	}
	err := r.client.UpdateGuestFirewallOptions(ctx, plan.Node.ValueString(), plan.GuestType.ValueString(), plan.VMID.ValueInt64(), guestFirewallOptionsFromModel(&plan.pveGuestFirewallOptionsOptionSet), nil)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating pve_guest_firewall_options",
			fmt.Sprintf("writing firewall options of guest %d on node %s: %s", plan.VMID.ValueInt64(), plan.Node.ValueString(), err),
		)
		return
	}
	if err := guestFirewallOptionsReadInto(ctx, r.client, plan.Node.ValueString(), plan.GuestType.ValueString(), plan.VMID.ValueInt64(), &plan.pveGuestFirewallOptionsOptionSet); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_guest_firewall_options after create",
			fmt.Sprintf("reading firewall options of guest %d on node %s: %s", plan.VMID.ValueInt64(), plan.Node.ValueString(), err),
		)
		return
	}
	plan.ID = types.StringValue(guestFirewallOptionsID(plan.Node.ValueString(), plan.GuestType.ValueString(), plan.VMID.ValueInt64()))
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read implements resource.Resource.
func (r *pveGuestFirewallOptionsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pveGuestFirewallOptionsResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := guestFirewallOptionsReadInto(ctx, r.client, state.Node.ValueString(), state.GuestType.ValueString(), state.VMID.ValueInt64(), &state.pveGuestFirewallOptionsOptionSet)
	if err != nil {
		if isPVEClientNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error reading pve_guest_firewall_options",
			fmt.Sprintf("reading firewall options of guest %d on node %s: %s", state.VMID.ValueInt64(), state.Node.ValueString(), err),
		)
		return
	}
	state.ID = types.StringValue(guestFirewallOptionsID(state.Node.ValueString(), state.GuestType.ValueString(), state.VMID.ValueInt64()))
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource. Set fields travel in the PUT body;
// fields cleared in the plan travel in the `delete` query parameter.
func (r *pveGuestFirewallOptionsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pveGuestFirewallOptionsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state pveGuestFirewallOptionsResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	deleteFields := guestFirewallOptionsDeleteFields(&plan.pveGuestFirewallOptionsOptionSet, &state.pveGuestFirewallOptionsOptionSet)
	err := r.client.UpdateGuestFirewallOptions(ctx, plan.Node.ValueString(), plan.GuestType.ValueString(), plan.VMID.ValueInt64(), guestFirewallOptionsFromModel(&plan.pveGuestFirewallOptionsOptionSet), deleteFields)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating pve_guest_firewall_options",
			fmt.Sprintf("updating firewall options of guest %d on node %s: %s", plan.VMID.ValueInt64(), plan.Node.ValueString(), err),
		)
		return
	}
	if err := guestFirewallOptionsReadInto(ctx, r.client, plan.Node.ValueString(), plan.GuestType.ValueString(), plan.VMID.ValueInt64(), &plan.pveGuestFirewallOptionsOptionSet); err != nil {
		resp.Diagnostics.AddError(
			"Error reading pve_guest_firewall_options after update",
			fmt.Sprintf("reading firewall options of guest %d on node %s: %s", plan.VMID.ValueInt64(), plan.Node.ValueString(), err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete implements resource.Resource. The pin defines no delete verb for
// the guest firewall options, so destroy only forgets the state.
func (r *pveGuestFirewallOptionsResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

// ImportState parses an import ID of the form `<node>:<guest_type>:<vmid>`.
func (r *pveGuestFirewallOptionsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	node, guestType, vmid, err := guestFirewallOptionsParseImportID(req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Invalid pve_guest_firewall_options import ID", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("node"), node)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("guest_type"), guestType)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("vmid"), vmid)...)
}

// guestFirewallOptionsID renders the singleton identifier.
func guestFirewallOptionsID(node, guestType string, vmid int64) string {
	return fmt.Sprintf("%s:%s:%d", node, guestType, vmid)
}

// guestFirewallOptionsParseImportID splits an import ID of the form
// `<node>:<guest_type>:<vmid>`.
func guestFirewallOptionsParseImportID(id string) (string, string, int64, error) {
	parts := strings.Split(id, ":")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return "", "", 0, fmt.Errorf("import ID must be formatted `<node>:<guest_type>:<vmid>`, e.g. `pve1:qemu:100`, got %q", id)
	}
	vmid, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return "", "", 0, fmt.Errorf("import ID %q: vmid is not a number: %s", id, err)
	}
	return parts[0], parts[1], vmid, nil
}

// guestFirewallOptionsReadInto refreshes the option set from the guest.
func guestFirewallOptionsReadInto(ctx context.Context, client *pveclient.Client, node, guestType string, vmid int64, m *pveGuestFirewallOptionsOptionSet) error {
	opts, err := client.GetGuestFirewallOptions(ctx, node, guestType, vmid)
	if err != nil {
		return err
	}
	guestFirewallOptionsApply(m, opts)
	return nil
}

// guestFirewallOptionsFromModel projects the Terraform model into the wire
// struct; null values become nil pointers and are omitted from the PUT body.
func guestFirewallOptionsFromModel(m *pveGuestFirewallOptionsOptionSet) pveclient.FirewallOptions {
	return pveclient.FirewallOptions{
		Enable:      firewallOptionsBoolPtr(m.Enable),
		DHCP:        firewallOptionsBoolPtr(m.DHCP),
		IPFilter:    firewallOptionsBoolPtr(m.IPFilter),
		MacFilter:   firewallOptionsBoolPtr(m.MacFilter),
		Ndp:         firewallOptionsBoolPtr(m.Ndp),
		Radv:        firewallOptionsBoolPtr(m.Radv),
		LogLevelIn:  firewallOptionsStrPtr(m.LogLevelIn),
		LogLevelOut: firewallOptionsStrPtr(m.LogLevelOut),
		PolicyIn:    firewallOptionsStrPtr(m.PolicyIn),
		PolicyOut:   firewallOptionsStrPtr(m.PolicyOut),
	}
}

// guestFirewallOptionsApply writes fetched options into the model; absent
// options become null.
func guestFirewallOptionsApply(m *pveGuestFirewallOptionsOptionSet, o *pveclient.FirewallOptions) {
	m.Enable = nodeNetworkBoolPtrToTF(o.Enable)
	m.DHCP = nodeNetworkBoolPtrToTF(o.DHCP)
	m.IPFilter = nodeNetworkBoolPtrToTF(o.IPFilter)
	m.MacFilter = nodeNetworkBoolPtrToTF(o.MacFilter)
	m.Ndp = nodeNetworkBoolPtrToTF(o.Ndp)
	m.Radv = nodeNetworkBoolPtrToTF(o.Radv)
	m.LogLevelIn = firewallOptionsStringValue(o.LogLevelIn)
	m.LogLevelOut = firewallOptionsStringValue(o.LogLevelOut)
	m.PolicyIn = firewallOptionsStringValue(o.PolicyIn)
	m.PolicyOut = firewallOptionsStringValue(o.PolicyOut)
}

// guestFirewallOptionsDeleteFields returns the wire names of options present
// in state but cleared in the plan.
func guestFirewallOptionsDeleteFields(plan, state *pveGuestFirewallOptionsOptionSet) []string {
	var out []string
	if plan.Enable.IsNull() && !state.Enable.IsNull() {
		out = append(out, "enable")
	}
	if plan.DHCP.IsNull() && !state.DHCP.IsNull() {
		out = append(out, "dhcp")
	}
	if plan.IPFilter.IsNull() && !state.IPFilter.IsNull() {
		out = append(out, "ipfilter")
	}
	if plan.MacFilter.IsNull() && !state.MacFilter.IsNull() {
		out = append(out, "macfilter")
	}
	if plan.Ndp.IsNull() && !state.Ndp.IsNull() {
		out = append(out, "ndp")
	}
	if plan.Radv.IsNull() && !state.Radv.IsNull() {
		out = append(out, "radv")
	}
	if plan.LogLevelIn.IsNull() && !state.LogLevelIn.IsNull() {
		out = append(out, "log_level_in")
	}
	if plan.LogLevelOut.IsNull() && !state.LogLevelOut.IsNull() {
		out = append(out, "log_level_out")
	}
	if plan.PolicyIn.IsNull() && !state.PolicyIn.IsNull() {
		out = append(out, "policy_in")
	}
	if plan.PolicyOut.IsNull() && !state.PolicyOut.IsNull() {
		out = append(out, "policy_out")
	}
	return out
}
