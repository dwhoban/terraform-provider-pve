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
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// This file carries the plumbing shared by the pve_node_network_linux_bridge,
// pve_node_network_linux_bond, and pve_node_network_vlan resources (ADR 0001
// B16: one resource per interface type, apply folded into every mutation).

// nodeNetworkMethods enumerates the values PVE accepts for `method`.
var nodeNetworkMethods = []string{"static", "dhcp", "manual", "loopback"}

// nodeNetworkBondModes enumerates the Linux kernel bonding driver modes PVE
// accepts for `bond_mode`.
var nodeNetworkBondModes = []string{
	"balance-rr", "active-backup", "balance-xor", "broadcast",
	"802.3ad", "balance-tlb", "balance-alb",
}

// nodeNetworkBondXmitHashPolicies enumerates the transmit hash policies PVE
// accepts for `bond_xmit_hash_policy` (balance-xor / 802.3ad modes).
var nodeNetworkBondXmitHashPolicies = []string{"layer2", "layer2+3", "layer3+4"}

// nodeNetworkCommonModel holds the attributes shared by every member of the
// node network family. Family resources embed it in their models.
type nodeNetworkCommonModel struct {
	Node      types.String `tfsdk:"node"`
	Iface     types.String `tfsdk:"iface"`
	Autostart types.Bool   `tfsdk:"autostart"`
	CIDR      types.String `tfsdk:"cidr"`
	Gateway   types.String `tfsdk:"gateway"`
	Method    types.String `tfsdk:"method"`
	MTU       types.Int64  `tfsdk:"mtu"`
	Comments  types.String `tfsdk:"comments"`
	Active    types.Bool   `tfsdk:"active"`
	Digest    types.String `tfsdk:"digest"`
}

// nodeNetworkCommonAttributes returns the schema attributes common to the
// whole family. Type-specific attributes are merged on top by each resource.
func nodeNetworkCommonAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"node": schema.StringAttribute{
			Required:            true,
			MarkdownDescription: "Name of the PVE node. Changing this forces replacement.",
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.RequiresReplace(),
			},
		},
		"iface": schema.StringAttribute{
			Required:            true,
			MarkdownDescription: "Interface name (Linux device name or alias). Changing this forces replacement.",
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.RequiresReplace(),
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
			MarkdownDescription: "IPv4 or IPv6 address in CIDR notation for the interface.",
		},
		"gateway": schema.StringAttribute{
			Optional:            true,
			MarkdownDescription: "Default gateway.",
		},
		"method": schema.StringAttribute{
			Optional:            true,
			MarkdownDescription: "Address assignment method. Must be one of: `static`, `dhcp`, `manual`, `loopback`.",
			Validators: []validator.String{
				stringvalidator.OneOf(nodeNetworkMethods...),
			},
		},
		"mtu": schema.Int64Attribute{
			Optional:            true,
			MarkdownDescription: "Maximum transmission unit in bytes. Must be between 576 and 65535 inclusive.",
			Validators: []validator.Int64{
				int64validator.Between(576, 65535),
			},
		},
		"comments": schema.StringAttribute{
			Optional:            true,
			MarkdownDescription: "Free-form text shown in the UI.",
		},
		"active": schema.BoolAttribute{
			Computed:            true,
			MarkdownDescription: "Whether the interface is currently active in the running kernel.",
		},
		"digest": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Opaque PVE configuration digest.",
		},
	}
}

// nodeNetworkCommonFromModel projects the shared attributes into a wire body.
// Null/unknown fields are omitted so the request JSON stays minimal; PVE
// rejects requests carrying empty values for some attributes.
func nodeNetworkCommonFromModel(m nodeNetworkCommonModel, ifaceType string) pveclient.NetworkInterface {
	body := pveclient.NetworkInterface{
		Iface: m.Iface.ValueString(),
		Type:  ifaceType,
	}
	if !m.Autostart.IsNull() && !m.Autostart.IsUnknown() {
		v := m.Autostart.ValueBool()
		body.Autostart = &v
	}
	if !m.CIDR.IsNull() && !m.CIDR.IsUnknown() {
		body.CIDR = m.CIDR.ValueString()
	}
	if !m.Gateway.IsNull() && !m.Gateway.IsUnknown() {
		body.Gateway = m.Gateway.ValueString()
	}
	if !m.Method.IsNull() && !m.Method.IsUnknown() {
		body.Method = m.Method.ValueString()
	}
	if !m.MTU.IsNull() && !m.MTU.IsUnknown() {
		body.MTU = int(m.MTU.ValueInt64())
	}
	if !m.Comments.IsNull() && !m.Comments.IsUnknown() {
		body.Comments = m.Comments.ValueString()
	}
	return body
}

// nodeNetworkCommonIntoModel populates the shared attributes from a wire
// read. Absent upstream values become Terraform nulls (never zero values) so
// optional attributes do not churn between reads and plans.
func nodeNetworkCommonIntoModel(m *nodeNetworkCommonModel, iface *pveclient.NetworkInterface) {
	if iface.Autostart != nil {
		m.Autostart = types.BoolValue(*iface.Autostart)
	}
	m.CIDR = nodeNetworkStringToTF(iface.CIDR)
	m.Gateway = nodeNetworkStringToTF(iface.Gateway)
	m.Method = nodeNetworkStringToTF(iface.Method)
	m.MTU = nodeNetworkIntToTF(iface.MTU)
	m.Comments = nodeNetworkStringToTF(iface.Comments)
	m.Active = types.BoolValue(iface.Active)
	m.Digest = types.StringValue(iface.Digest)
}

// nodeNetworkGetChecked reads one interface and errors when the upstream
// type does not match the resource's fixed type, so a resource never
// silently adopts an interface of another kind.
func nodeNetworkGetChecked(ctx context.Context, client *pveclient.Client, node, iface, wantType string) (*pveclient.NetworkInterface, error) {
	got, err := client.GetNodeNetwork(ctx, node, iface)
	if err != nil {
		return nil, err
	}
	if got.Type != wantType {
		return nil, &nodeNetworkTypeError{Node: node, Iface: iface, Got: got.Type, Want: wantType}
	}
	return got, nil
}

// nodeNetworkTypeError reports an upstream interface whose type differs from
// the resource's fixed type.
type nodeNetworkTypeError struct {
	Node  string
	Iface string
	Got   string
	Want  string
}

func (e *nodeNetworkTypeError) Error() string {
	return fmt.Sprintf("interface %q on node %q has upstream type %q but this resource manages type %q", e.Iface, e.Node, e.Got, e.Want)
}

// nodeNetworkApply applies pending network configuration changes
// (`PUT /nodes/{node}/network`) and waits for the resulting task. ADR 0001
// folds apply into every network mutation.
func nodeNetworkApply(ctx context.Context, client *pveclient.Client, node string) error {
	upid, err := client.ReloadNodeNetwork(ctx, node)
	if err != nil {
		return fmt.Errorf("applying network configuration on node %s: %w", node, err)
	}
	if _, err := client.WaitForTask(ctx, node, upid, pveclient.WaitForTaskOptions{}); err != nil {
		return fmt.Errorf("network apply task on node %s: %w", node, err)
	}
	return nil
}

// nodeNetworkParseImportID splits an import ID of the form `<node>:<iface>`.
func nodeNetworkParseImportID(id string) (string, string, error) {
	parts := strings.SplitN(id, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("import ID must be in the form `<node>:<iface>`, got %q", id)
	}
	return parts[0], parts[1], nil
}

// nodeNetworkConfigureResource extracts the shared client from provider
// data. Nil provider data leaves the resource unconfigured (unit tests).
func nodeNetworkConfigureResource(req resource.ConfigureRequest, resp *resource.ConfigureResponse) *pveclient.Client {
	if req.ProviderData == nil {
		return nil
	}
	client, ok := req.ProviderData.(*pveclient.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *pveclient.Client, got: %T.", req.ProviderData),
		)
		return nil
	}
	return client
}

// nodeNetworkCommonDeleteFields returns the PVE field names to clear on
// update: optional shared attributes present in state but null in plan.
func nodeNetworkCommonDeleteFields(plan, state nodeNetworkCommonModel) []string {
	var out []string
	if nodeNetworkStringCleared(plan.CIDR, state.CIDR) {
		out = append(out, "cidr")
	}
	if nodeNetworkStringCleared(plan.Gateway, state.Gateway) {
		out = append(out, "gateway")
	}
	if nodeNetworkStringCleared(plan.Method, state.Method) {
		out = append(out, "method")
	}
	if nodeNetworkStringCleared(plan.Comments, state.Comments) {
		out = append(out, "comments")
	}
	if nodeNetworkIntCleared(plan.MTU, state.MTU) {
		out = append(out, "mtu")
	}
	return out
}

// nodeNetworkStringCleared reports a string attr null in plan but set in state.
func nodeNetworkStringCleared(plan, state types.String) bool {
	return plan.IsNull() && !state.IsNull()
}

// nodeNetworkIntCleared reports an int attr null in plan but set in state.
func nodeNetworkIntCleared(plan, state types.Int64) bool {
	return plan.IsNull() && !state.IsNull()
}

// nodeNetworkBoolPtrToTF converts an optional wire bool to Terraform,
// mapping a nil pointer to null.
func nodeNetworkBoolPtrToTF(v *bool) types.Bool {
	if v == nil {
		return types.BoolNull()
	}
	return types.BoolValue(*v)
}

// nodeNetworkStringToTF maps the empty string to null.
func nodeNetworkStringToTF(s string) types.String {
	if s == "" {
		return types.StringNull()
	}
	return types.StringValue(s)
}

// nodeNetworkIntToTF maps zero to null.
func nodeNetworkIntToTF(i int) types.Int64 {
	if i == 0 {
		return types.Int64Null()
	}
	return types.Int64Value(int64(i))
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
