// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// FirewallRule is one Proxmox VE firewall rule, shared by every rules
// scope: /cluster/firewall/rules, /nodes/{node}/firewall/rules,
// /nodes/{node}/{qemu,lxc}/{vmid}/firewall/rules,
// /cluster/firewall/groups/{group}/rules, and
// /cluster/sdn/vnets/{vnet}/firewall/rules. Enable tolerates the 0/1
// integer encoding the pin declares (and the true/false spelling some
// versions emit); IPVersion is response-only; Pos is set by PVE.
type FirewallRule struct {
	Pos       *int64
	Enable    *bool
	Type      string
	Action    string
	Macro     string
	Proto     string
	DPort     string
	SPort     string
	Source    string
	Dest      string
	ICMPType  string
	IFace     string
	Log       string
	Comment   string
	IPVersion *int64
	Digest    string
}

// FirewallRuleBoolPtr returns a pointer to b, for building FirewallRule
// request structs.
func FirewallRuleBoolPtr(b bool) *bool { return &b }

// firewallRuleWire mirrors the wire shape of FirewallRule: the
// hyphenated icmp-type key and the lenient enable/ipversion fields as
// raw JSON.
type firewallRuleWire struct {
	Pos       *int64          `json:"pos,omitempty"`
	Enable    json.RawMessage `json:"enable,omitempty"`
	Type      string          `json:"type,omitempty"`
	Action    string          `json:"action,omitempty"`
	Macro     string          `json:"macro,omitempty"`
	Proto     string          `json:"proto,omitempty"`
	DPort     string          `json:"dport,omitempty"`
	SPort     string          `json:"sport,omitempty"`
	Source    string          `json:"source,omitempty"`
	Dest      string          `json:"dest,omitempty"`
	ICMPType  string          `json:"icmp-type,omitempty"`
	IFace     string          `json:"iface,omitempty"`
	Log       string          `json:"log,omitempty"`
	Comment   string          `json:"comment,omitempty"`
	IPVersion json.RawMessage `json:"ipversion,omitempty"`
	Digest    string          `json:"digest,omitempty"`
}

// firewallRuleEnableRaw encodes enable as the pin's 0/1 integer, nil
// when unset.
func firewallRuleEnableRaw(b *bool) json.RawMessage {
	if b == nil {
		return nil
	}
	if *b {
		return json.RawMessage("1")
	}
	return json.RawMessage("0")
}

// firewallInt64PtrFromRaw decodes a raw JSON number into an *int64,
// keeping nil for absent or null values.
func firewallInt64PtrFromRaw(raw json.RawMessage) *int64 {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	i, err := strconv.ParseInt(string(raw), 10, 64)
	if err != nil {
		return nil
	}
	return &i
}

// MarshalJSON emits the wire shape, including the hyphenated icmp-type
// key and the 0/1 enable encoding.
func (r FirewallRule) MarshalJSON() ([]byte, error) {
	return json.Marshal(firewallRuleWire{
		Pos:       r.Pos,
		Enable:    firewallRuleEnableRaw(r.Enable),
		Type:      r.Type,
		Action:    r.Action,
		Macro:     r.Macro,
		Proto:     r.Proto,
		DPort:     r.DPort,
		SPort:     r.SPort,
		Source:    r.Source,
		Dest:      r.Dest,
		ICMPType:  r.ICMPType,
		IFace:     r.IFace,
		Log:       r.Log,
		Comment:   r.Comment,
		IPVersion: nil,
		Digest:    r.Digest,
	})
}

// UnmarshalJSON decodes the wire shape, tolerating the 0/1 integer and
// true/false encodings of enable and the hyphenated icmp-type key.
func (r *FirewallRule) UnmarshalJSON(data []byte) error {
	var wire firewallRuleWire
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	*r = FirewallRule{
		Pos:       wire.Pos,
		Enable:    decodeBoolishPtr(wire.Enable),
		Type:      wire.Type,
		Action:    wire.Action,
		Macro:     wire.Macro,
		Proto:     wire.Proto,
		DPort:     wire.DPort,
		SPort:     wire.SPort,
		Source:    wire.Source,
		Dest:      wire.Dest,
		ICMPType:  wire.ICMPType,
		IFace:     wire.IFace,
		Log:       wire.Log,
		Comment:   wire.Comment,
		IPVersion: firewallInt64PtrFromRaw(wire.IPVersion),
		Digest:    wire.Digest,
	}
	return nil
}

// decodeBoolishPtr decodes a raw JSON bool-or-0/1 value into an *bool,
// keeping nil for absent values.
func decodeBoolishPtr(raw json.RawMessage) *bool {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var b bool
	if err := json.Unmarshal(raw, &b); err == nil {
		return &b
	}
	if decodeBoolish(raw) {
		return FirewallRuleBoolPtr(true)
	}
	return FirewallRuleBoolPtr(false)
}

// FirewallRulesPathCluster returns the cluster-wide ruleset base path.
func FirewallRulesPathCluster() string { return "/cluster/firewall/rules" }

// FirewallRulesPathNode returns the firewall ruleset base path of one node.
func FirewallRulesPathNode(node string) string {
	return fmt.Sprintf("/nodes/%s/firewall/rules", node)
}

// FirewallRulesPathGuest returns the firewall ruleset base path of one
// guest (guestType `qemu` or `lxc`).
func FirewallRulesPathGuest(node, guestType string, vmid int64) string {
	return fmt.Sprintf("/nodes/%s/%s/%d/firewall/rules", node, guestType, vmid)
}

// FirewallRulesPathSecurityGroup returns the ruleset base path of one
// security group.
func FirewallRulesPathSecurityGroup(group string) string {
	return fmt.Sprintf("/cluster/firewall/groups/%s/rules", group)
}

// FirewallRulesPathVnet returns the firewall ruleset base path of one
// SDN vnet.
func FirewallRulesPathVnet(vnet string) string {
	return fmt.Sprintf("/cluster/sdn/vnets/%s/firewall/rules", vnet)
}

// firewallRulesRulePath appends the position to a ruleset base path.
func firewallRulesRulePath(basePath string, pos int64) string {
	return basePath + "/" + strconv.FormatInt(pos, 10)
}

// firewallRulesDeleteQuery appends PVE's `delete` query parameter
// (comma-separated field names to clear) when fields are supplied.
func firewallRulesDeleteQuery(basePath string, deleteFields []string) string {
	if len(deleteFields) == 0 {
		return basePath
	}
	return basePath + "?delete=" + url.QueryEscape(strings.Join(deleteFields, ","))
}

// ListFirewallRules returns the rules from GET <basePath>, in upstream
// order.
func (c *Client) ListFirewallRules(ctx context.Context, basePath string) ([]FirewallRule, error) {
	var out []FirewallRule
	if err := c.Do(ctx, "GET", basePath, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetFirewallRule reads GET <basePath>/{pos}.
func (c *Client) GetFirewallRule(ctx context.Context, basePath string, pos int64) (*FirewallRule, error) {
	var out FirewallRule
	if err := c.Do(ctx, "GET", firewallRulesRulePath(basePath, pos), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateFirewallRule POSTs <basePath> with the supplied rule. The pin
// declares the response null, so there is nothing to decode; PVE appends
// the rule when the rule carries no Pos.
func (c *Client) CreateFirewallRule(ctx context.Context, basePath string, rule FirewallRule) error {
	return c.Do(ctx, "POST", basePath, rule, nil)
}

// UpdateFirewallRule PUTs <basePath>/{pos} with the supplied rule and
// translates deleteFields into PVE's `delete` query parameter.
func (c *Client) UpdateFirewallRule(ctx context.Context, basePath string, pos int64, rule FirewallRule, deleteFields []string) error {
	return c.Do(ctx, "PUT", firewallRulesDeleteQuery(firewallRulesRulePath(basePath, pos), deleteFields), rule, nil)
}

// DeleteFirewallRule DELETEs <basePath>/{pos}.
func (c *Client) DeleteFirewallRule(ctx context.Context, basePath string, pos int64) error {
	return c.Do(ctx, "DELETE", firewallRulesRulePath(basePath, pos), nil, nil)
}
