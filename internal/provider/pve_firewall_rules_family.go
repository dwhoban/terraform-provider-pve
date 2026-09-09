// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// firewallRulesRuleModel mirrors one rule object inside the ordered
// `rules` list shared by the five firewall rules resources.
type firewallRulesRuleModel struct {
	Pos       types.Int64  `tfsdk:"pos"`
	Enable    types.Bool   `tfsdk:"enable"`
	Type      types.String `tfsdk:"type"`
	Action    types.String `tfsdk:"action"`
	Macro     types.String `tfsdk:"macro"`
	Proto     types.String `tfsdk:"proto"`
	DPort     types.String `tfsdk:"dport"`
	SPort     types.String `tfsdk:"sport"`
	Source    types.String `tfsdk:"source"`
	Dest      types.String `tfsdk:"dest"`
	ICMPType  types.String `tfsdk:"icmp_type"`
	IFace     types.String `tfsdk:"iface"`
	Log       types.String `tfsdk:"log"`
	Comment   types.String `tfsdk:"comment"`
	IPVersion types.Int64  `tfsdk:"ipversion"`
}

// firewallRulesRuleAttributes renders the nested rule object schema,
// shared unchanged by all five firewall rules resources. Field sets and
// enums follow the pin's rule parameter block, which is identical for
// every scope.
func firewallRulesRuleAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"pos": schema.Int64Attribute{
			Computed:            true,
			MarkdownDescription: "Rule position in the ruleset (0-based). PVE assigns and renumbers positions, so the provider resyncs it from the ruleset after every apply; the list order in configuration is authoritative.",
		},
		"enable": schema.BoolAttribute{
			Optional:            true,
			MarkdownDescription: "Whether the rule is active. PVE stores this flag as 0/1.",
		},
		"type": schema.StringAttribute{
			Required: true,
			Validators: []validator.String{
				stringvalidator.OneOf("in", "out", "forward", "group"),
			},
			MarkdownDescription: "Rule direction. `group` entries reference a security group via `action`. Must be one of: `in`, `out`, `forward`, `group`.",
		},
		"action": schema.StringAttribute{
			Required:            true,
			MarkdownDescription: "Rule action (`ACCEPT`, `DROP`, `REJECT`) or, for `type = \"group\"`, the name of the referenced security group. PVE validates the value as an identifier: 2-20 characters, starting with a letter, then letters, digits, `-` or `_`.",
		},
		"macro": schema.StringAttribute{
			Optional:            true,
			MarkdownDescription: "Use a predefined standard macro (e.g. `SSH`, `HTTP`, `SMB`, `Ping`); PVE expands it into implicit port/protocol matches. PVE accepts the names returned by `GET /cluster/firewall/macros`, so the provider passes the value through without validation.",
		},
		"proto": schema.StringAttribute{
			Optional:            true,
			MarkdownDescription: "IP protocol: a protocol name (`tcp`, `udp`, `icmp`, `icmpv6`/`ipv6-icmp`, ...) or a number as defined in `/etc/protocols`. PVE accepts names and numbers without a fixed whitelist, so the provider passes the value through.",
		},
		"dport": schema.StringAttribute{
			Optional:            true,
			MarkdownDescription: "Restrict TCP/UDP destination port: a service name or number (0-65535), a range like `80:85`, or a comma-separated list of ports and ranges. Only meaningful together with a TCP/UDP `proto` or macro.",
		},
		"sport": schema.StringAttribute{
			Optional:            true,
			MarkdownDescription: "Restrict TCP/UDP source port: a service name or number (0-65535), a range like `80:85`, or a comma-separated list of ports and ranges. Only meaningful together with a TCP/UDP `proto` or macro.",
		},
		"source": schema.StringAttribute{
			Optional:            true,
			MarkdownDescription: "Restrict packet source address: a single IP address or CIDR network, an IP set (`+ipsetname`), an alias name, an address range like `10.0.0.1-10.0.0.10`, or a comma-separated list. Do not mix IPv4 and IPv6 entries in one list.",
		},
		"dest": schema.StringAttribute{
			Optional:            true,
			MarkdownDescription: "Restrict packet destination address: a single IP address or CIDR network, an IP set (`+ipsetname`), an alias name, an address range like `10.0.0.1-10.0.0.10`, or a comma-separated list. Do not mix IPv4 and IPv6 entries in one list.",
		},
		"icmp_type": schema.StringAttribute{
			Optional:            true,
			MarkdownDescription: "Restrict the ICMP type (e.g. `echo-request`). Only valid when `proto` is `icmp` or `icmpv6`/`ipv6-icmp`.",
		},
		"iface": schema.StringAttribute{
			Optional:            true,
			MarkdownDescription: "Network interface name. On guest rulesets use the guest configuration key names (`net0`, `net1`, ...); node and cluster rulesets accept arbitrary host interface names.",
		},
		"log": schema.StringAttribute{
			Optional: true,
			Validators: []validator.String{
				stringvalidator.OneOf("emerg", "alert", "crit", "err", "warning", "notice", "info", "debug", "nolog"),
			},
			MarkdownDescription: "Log level for the rule. Must be one of: `emerg`, `alert`, `crit`, `err`, `warning`, `notice`, `info`, `debug`, `nolog`.",
		},
		"comment": schema.StringAttribute{
			Optional:            true,
			MarkdownDescription: "Descriptive comment.",
		},
		"ipversion": schema.Int64Attribute{
			Computed:            true,
			MarkdownDescription: "IP version (4 or 6), determined by PVE from `source`/`dest`; null when the rule does not restrict addresses.",
		},
	}
}

// firewallRulesFromModel projects the planned rule models into wire
// rules; null attributes carry the zero value, which the wire marshal
// omits from the request body.
func firewallRulesFromModel(in []firewallRulesRuleModel) []pveclient.FirewallRule {
	out := make([]pveclient.FirewallRule, 0, len(in))
	for _, m := range in {
		rule := pveclient.FirewallRule{
			Type:     m.Type.ValueString(),
			Action:   m.Action.ValueString(),
			Macro:    m.Macro.ValueString(),
			Proto:    m.Proto.ValueString(),
			DPort:    m.DPort.ValueString(),
			SPort:    m.SPort.ValueString(),
			Source:   m.Source.ValueString(),
			Dest:     m.Dest.ValueString(),
			ICMPType: m.ICMPType.ValueString(),
			IFace:    m.IFace.ValueString(),
			Log:      m.Log.ValueString(),
			Comment:  m.Comment.ValueString(),
		}
		if !m.Enable.IsNull() && !m.Enable.IsUnknown() {
			rule.Enable = pveclient.FirewallRuleBoolPtr(m.Enable.ValueBool())
		}
		out = append(out, rule)
	}
	return out
}

// firewallRulesToModel projects wire rules into rule models with fresh
// positions and computed fields.
func firewallRulesToModel(in []pveclient.FirewallRule) []firewallRulesRuleModel {
	out := make([]firewallRulesRuleModel, 0, len(in))
	for _, r := range in {
		m := firewallRulesRuleModel{
			Pos:       firewallRulesInt64TF(r.Pos),
			IPVersion: firewallRulesInt64TF(r.IPVersion),
			Enable:    nodeNetworkBoolPtrToTF(r.Enable),
			Type:      types.StringValue(r.Type),
			Action:    types.StringValue(r.Action),
			Macro:     nodeNetworkStringToTF(r.Macro),
			Proto:     nodeNetworkStringToTF(r.Proto),
			DPort:     nodeNetworkStringToTF(r.DPort),
			SPort:     nodeNetworkStringToTF(r.SPort),
			Source:    nodeNetworkStringToTF(r.Source),
			Dest:      nodeNetworkStringToTF(r.Dest),
			ICMPType:  nodeNetworkStringToTF(r.ICMPType),
			IFace:     nodeNetworkStringToTF(r.IFace),
			Log:       nodeNetworkStringToTF(r.Log),
			Comment:   nodeNetworkStringToTF(r.Comment),
		}
		out = append(out, m)
	}
	return out
}

// firewallRulesInt64TF maps an optional wire integer into a Terraform
// int64, null when absent.
func firewallRulesInt64TF(v *int64) types.Int64 {
	if v == nil {
		return types.Int64Null()
	}
	return types.Int64Value(*v)
}

// firewallRulesSame reports whether two rules carry the same
// user-managed fields, ignoring the server-assigned pos, the computed
// ipversion, and the concurrency digest.
func firewallRulesSame(a, b pveclient.FirewallRule) bool {
	str := func(r pveclient.FirewallRule) string {
		return strings.Join([]string{r.Type, r.Action, r.Macro, r.Proto, r.DPort, r.SPort, r.Source, r.Dest, r.ICMPType, r.IFace, r.Log, r.Comment}, "\x00")
	}
	if str(a) != str(b) {
		return false
	}
	// PVE creates rules active when enable is omitted, so an unset
	// flag compares as active.
	enabled := func(b *bool) bool { return b == nil || *b }
	return enabled(a.Enable) == enabled(b.Enable)
}

// firewallRulesRulePos extracts a rule's upstream position, refusing
// entries the server returned without one.
func firewallRulesRulePos(r pveclient.FirewallRule) (int64, error) {
	if r.Pos == nil {
		return 0, fmt.Errorf("server returned a firewall rule without pos (action %q)", r.Action)
	}
	return *r.Pos, nil
}

// firewallRulesApplyDiff converges the upstream ruleset at basePath to
// the planned rules, in order. It never trusts a position across calls:
// surplus entries are deleted highest position first with a re-list
// after every delete, and each remaining plan entry is compared against
// a freshly listed ruleset before being written in place (PUT) or
// appended (POST), so an upstream that renumbers on any mutation stays
// consistent. A final re-list returns the rules with fresh positions.
func firewallRulesApplyDiff(ctx context.Context, client *pveclient.Client, basePath string, plan []pveclient.FirewallRule) ([]pveclient.FirewallRule, error) {
	current, err := client.ListFirewallRules(ctx, basePath)
	if err != nil {
		return nil, fmt.Errorf("listing firewall rules at %s: %w", basePath, err)
	}
	// 1. Remove surplus entries, highest position first.
	for len(current) > len(plan) {
		pos, err := firewallRulesRulePos(current[len(current)-1])
		if err != nil {
			return nil, err
		}
		if err := client.DeleteFirewallRule(ctx, basePath, pos); err != nil {
			return nil, fmt.Errorf("deleting firewall rule at pos %d: %w", pos, err)
		}
		if current, err = client.ListFirewallRules(ctx, basePath); err != nil {
			return nil, fmt.Errorf("re-listing firewall rules at %s after delete: %w", basePath, err)
		}
	}
	// 2. Write each plan entry, one at a time, against a fresh listing:
	// entries that already match are kept, entries below the current
	// length are modified in place, entries beyond it are appended.
	for i := range plan {
		if current, err = client.ListFirewallRules(ctx, basePath); err != nil {
			return nil, fmt.Errorf("re-listing firewall rules at %s: %w", basePath, err)
		}
		if i < len(current) {
			if firewallRulesSame(plan[i], current[i]) {
				continue
			}
			pos, err := firewallRulesRulePos(current[i])
			if err != nil {
				return nil, err
			}
			if err := client.UpdateFirewallRule(ctx, basePath, pos, plan[i], nil); err != nil {
				return nil, fmt.Errorf("updating firewall rule at pos %d: %w", pos, err)
			}
			continue
		}
		if err := client.CreateFirewallRule(ctx, basePath, plan[i]); err != nil {
			return nil, fmt.Errorf("creating firewall rule at index %d: %w", i, err)
		}
	}
	// 3. Resync positions into state.
	fresh, err := client.ListFirewallRules(ctx, basePath)
	if err != nil {
		return nil, fmt.Errorf("re-listing firewall rules at %s: %w", basePath, err)
	}
	return fresh, nil
}

// firewallRulesDeleteAll empties the ruleset, deleting from the highest
// position downward and re-listing between deletions so upstream
// renumbering never invalidates a position.
func firewallRulesDeleteAll(ctx context.Context, client *pveclient.Client, basePath string) error {
	for {
		current, err := client.ListFirewallRules(ctx, basePath)
		if err != nil {
			return fmt.Errorf("listing firewall rules at %s: %w", basePath, err)
		}
		if len(current) == 0 {
			return nil
		}
		pos, err := firewallRulesRulePos(current[len(current)-1])
		if err != nil {
			return err
		}
		if err := client.DeleteFirewallRule(ctx, basePath, pos); err != nil {
			return fmt.Errorf("deleting firewall rule at pos %d: %w", pos, err)
		}
	}
}

// firewallRulesReadInto lists the upstream ruleset and projects it into
// the model's rule slice, preserving upstream order and positions.
func firewallRulesReadInto(ctx context.Context, client *pveclient.Client, basePath string, rules *[]firewallRulesRuleModel) error {
	fresh, err := client.ListFirewallRules(ctx, basePath)
	if err != nil {
		return err
	}
	*rules = firewallRulesToModel(fresh)
	return nil
}

// firewallRulesSplitImportID splits a `:`-joined import ID into exactly
// want non-empty parts.
func firewallRulesSplitImportID(id string, want int) ([]string, error) {
	parts := strings.Split(id, ":")
	if len(parts) != want {
		return nil, fmt.Errorf("import ID must have %d `:`-separated parts, got %d in %q", want, len(parts), id)
	}
	for _, part := range parts {
		if part == "" {
			return nil, fmt.Errorf("import ID %q contains an empty part", id)
		}
	}
	return parts, nil
}

// firewallRulesConfigureResource extracts the shared client from provider
// data for the firewall rules resources. Nil provider data leaves the
// resource unconfigured (unit tests).
func firewallRulesConfigureResource(req resource.ConfigureRequest, resp *resource.ConfigureResponse) *pveclient.Client {
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
