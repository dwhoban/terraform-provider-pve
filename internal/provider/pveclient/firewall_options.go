// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"context"
	"encoding/json"
	"fmt"
)

// FirewallBoolPtr returns a pointer to b, for building firewall options
// request structs.
func FirewallBoolPtr(b bool) *bool { return &b }

// FirewallInt64Ptr returns a pointer to i, for building firewall options
// request structs.
func FirewallInt64Ptr(i int64) *int64 { return &i }

// FirewallStrPtr returns a pointer to s, for building firewall options
// request structs.
func FirewallStrPtr(s string) *string { return &s }

// ClusterFirewallOptions mirrors the firewall option set managed through
// GET/PUT /cluster/firewall/options. Every field is optional: nil means
// "not set". Per the pin, the cluster-wide enable is an integer level
// (unlike the boolean enable of the node, guest, and vnet scopes) and the
// forward policy lacks the REJECT value. LogRateLimit carries the
// `log_ratelimit` property string verbatim
// (`[enable=]<1|0> [,burst=<integer>] [,rate=<rate>]`).
type ClusterFirewallOptions struct {
	Ebitables     *bool
	Enable        *int64
	LogRateLimit  *string
	PolicyForward *string
	PolicyIn      *string
	PolicyOut     *string
}

// clusterFirewallOptionsRaw mirrors the wire shape with lenient fields as
// json.RawMessage: PVE emits the booleans as 0/1 integers on some
// versions.
type clusterFirewallOptionsRaw struct {
	Ebitables     json.RawMessage `json:"ebtables,omitempty"`
	Enable        json.RawMessage `json:"enable,omitempty"`
	LogRateLimit  *string         `json:"log_ratelimit,omitempty"`
	PolicyForward *string         `json:"policy_forward,omitempty"`
	PolicyIn      *string         `json:"policy_in,omitempty"`
	PolicyOut     *string         `json:"policy_out,omitempty"`
}

// UnmarshalJSON tolerates the 0/1 int encoding of the boolean fields and
// keeps absent settings nil.
func (o *ClusterFirewallOptions) UnmarshalJSON(data []byte) error {
	var raw clusterFirewallOptionsRaw
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	o.Ebitables = nodeNetworkBoolishPtr(raw.Ebitables)
	o.Enable = haInt64PtrFromRaw(raw.Enable)
	o.LogRateLimit = raw.LogRateLimit
	o.PolicyForward = raw.PolicyForward
	o.PolicyIn = raw.PolicyIn
	o.PolicyOut = raw.PolicyOut
	return nil
}

// FirewallOptions is the union of the firewall option sets of the node,
// guest (qemu and lxc), and SDN vnet scopes; every field is optional and
// callers set only the fields their scope supports. Decode tolerates the
// 0/1 int encoding PVE emits for the boolean fields. The wire keys are the
// pin's snake_case names (`nf_conntrack_max`, `tcpflags`, `macfilter`, ...).
type FirewallOptions struct {
	// Shared by all scopes that carry an enable (node, guest, vnet).
	Enable *bool
	// Shared by the guest and node scopes.
	Ndp         *bool
	LogLevelIn  *string
	LogLevelOut *string
	PolicyIn    *string
	PolicyOut   *string
	// Guest (qemu and lxc) scope.
	DHCP      *bool
	IPFilter  *bool
	MacFilter *bool
	Radv      *bool
	// Node scope.
	LogLevelForward                  *string
	LogNFConntrack                   *bool
	NFConntrackAllowInvalid          *bool
	NFConntrackHelpers               *string
	NFConntrackMax                   *int64
	NFConntrackTCPTimeoutEstablished *int64
	NFConntrackTCPTimeoutSynRecv     *int64
	Nftables                         *bool
	Nosmurfs                         *bool
	ProtectionSynflood               *bool
	ProtectionSynfloodBurst          *int64
	ProtectionSynfloodRate           *int64
	SmurfLogLevel                    *string
	TCPFlags                         *bool
	TCPFlagsLogLevel                 *string
	// SDN vnet scope.
	PolicyForward *string
}

// firewallOptionsRaw mirrors the wire shape of FirewallOptions with the
// lenient bool and integer fields as json.RawMessage.
type firewallOptionsRaw struct {
	Enable                           json.RawMessage `json:"enable,omitempty"`
	Ndp                              json.RawMessage `json:"ndp,omitempty"`
	LogLevelIn                       *string         `json:"log_level_in,omitempty"`
	LogLevelOut                      *string         `json:"log_level_out,omitempty"`
	PolicyIn                         *string         `json:"policy_in,omitempty"`
	PolicyOut                        *string         `json:"policy_out,omitempty"`
	DHCP                             json.RawMessage `json:"dhcp,omitempty"`
	IPFilter                         json.RawMessage `json:"ipfilter,omitempty"`
	MacFilter                        json.RawMessage `json:"macfilter,omitempty"`
	Radv                             json.RawMessage `json:"radv,omitempty"`
	LogLevelForward                  *string         `json:"log_level_forward,omitempty"`
	LogNFConntrack                   json.RawMessage `json:"log_nf_conntrack,omitempty"`
	NFConntrackAllowInvalid          json.RawMessage `json:"nf_conntrack_allow_invalid,omitempty"`
	NFConntrackHelpers               *string         `json:"nf_conntrack_helpers,omitempty"`
	NFConntrackMax                   json.RawMessage `json:"nf_conntrack_max,omitempty"`
	NFConntrackTCPTimeoutEstablished json.RawMessage `json:"nf_conntrack_tcp_timeout_established,omitempty"`
	NFConntrackTCPTimeoutSynRecv     json.RawMessage `json:"nf_conntrack_tcp_timeout_syn_recv,omitempty"`
	Nftables                         json.RawMessage `json:"nftables,omitempty"`
	Nosmurfs                         json.RawMessage `json:"nosmurfs,omitempty"`
	ProtectionSynflood               json.RawMessage `json:"protection_synflood,omitempty"`
	ProtectionSynfloodBurst          json.RawMessage `json:"protection_synflood_burst,omitempty"`
	ProtectionSynfloodRate           json.RawMessage `json:"protection_synflood_rate,omitempty"`
	SmurfLogLevel                    *string         `json:"smurf_log_level,omitempty"`
	TCPFlags                         json.RawMessage `json:"tcpflags,omitempty"`
	TCPFlagsLogLevel                 *string         `json:"tcp_flags_log_level,omitempty"`
	PolicyForward                    *string         `json:"policy_forward,omitempty"`
}

// UnmarshalJSON tolerates the 0/1 int encoding of the boolean fields and
// keeps absent settings nil.
func (o *FirewallOptions) UnmarshalJSON(data []byte) error {
	var raw firewallOptionsRaw
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	o.Enable = nodeNetworkBoolishPtr(raw.Enable)
	o.Ndp = nodeNetworkBoolishPtr(raw.Ndp)
	o.LogLevelIn = raw.LogLevelIn
	o.LogLevelOut = raw.LogLevelOut
	o.PolicyIn = raw.PolicyIn
	o.PolicyOut = raw.PolicyOut
	o.DHCP = nodeNetworkBoolishPtr(raw.DHCP)
	o.IPFilter = nodeNetworkBoolishPtr(raw.IPFilter)
	o.MacFilter = nodeNetworkBoolishPtr(raw.MacFilter)
	o.Radv = nodeNetworkBoolishPtr(raw.Radv)
	o.LogLevelForward = raw.LogLevelForward
	o.LogNFConntrack = nodeNetworkBoolishPtr(raw.LogNFConntrack)
	o.NFConntrackAllowInvalid = nodeNetworkBoolishPtr(raw.NFConntrackAllowInvalid)
	o.NFConntrackHelpers = raw.NFConntrackHelpers
	o.NFConntrackMax = haInt64PtrFromRaw(raw.NFConntrackMax)
	o.NFConntrackTCPTimeoutEstablished = haInt64PtrFromRaw(raw.NFConntrackTCPTimeoutEstablished)
	o.NFConntrackTCPTimeoutSynRecv = haInt64PtrFromRaw(raw.NFConntrackTCPTimeoutSynRecv)
	o.Nftables = nodeNetworkBoolishPtr(raw.Nftables)
	o.Nosmurfs = nodeNetworkBoolishPtr(raw.Nosmurfs)
	o.ProtectionSynflood = nodeNetworkBoolishPtr(raw.ProtectionSynflood)
	o.ProtectionSynfloodBurst = haInt64PtrFromRaw(raw.ProtectionSynfloodBurst)
	o.ProtectionSynfloodRate = haInt64PtrFromRaw(raw.ProtectionSynfloodRate)
	o.SmurfLogLevel = raw.SmurfLogLevel
	o.TCPFlags = nodeNetworkBoolishPtr(raw.TCPFlags)
	o.TCPFlagsLogLevel = raw.TCPFlagsLogLevel
	o.PolicyForward = raw.PolicyForward
	return nil
}

// firewallOptionsPutBody is the JSON body of the firewall options PUT
// requests: only fields the caller set travel on the wire.
type firewallOptionsPutBody map[string]any

// firewallOptionsBody projects the typed options into the PUT body,
// omitting nil fields entirely.
func firewallOptionsBody(o FirewallOptions) firewallOptionsPutBody {
	body := firewallOptionsPutBody{}
	if o.Enable != nil {
		body["enable"] = *o.Enable
	}
	if o.Ndp != nil {
		body["ndp"] = *o.Ndp
	}
	if o.LogLevelIn != nil {
		body["log_level_in"] = *o.LogLevelIn
	}
	if o.LogLevelOut != nil {
		body["log_level_out"] = *o.LogLevelOut
	}
	if o.PolicyIn != nil {
		body["policy_in"] = *o.PolicyIn
	}
	if o.PolicyOut != nil {
		body["policy_out"] = *o.PolicyOut
	}
	if o.DHCP != nil {
		body["dhcp"] = *o.DHCP
	}
	if o.IPFilter != nil {
		body["ipfilter"] = *o.IPFilter
	}
	if o.MacFilter != nil {
		body["macfilter"] = *o.MacFilter
	}
	if o.Radv != nil {
		body["radv"] = *o.Radv
	}
	if o.LogLevelForward != nil {
		body["log_level_forward"] = *o.LogLevelForward
	}
	if o.LogNFConntrack != nil {
		body["log_nf_conntrack"] = *o.LogNFConntrack
	}
	if o.NFConntrackAllowInvalid != nil {
		body["nf_conntrack_allow_invalid"] = *o.NFConntrackAllowInvalid
	}
	if o.NFConntrackHelpers != nil {
		body["nf_conntrack_helpers"] = *o.NFConntrackHelpers
	}
	if o.NFConntrackMax != nil {
		body["nf_conntrack_max"] = *o.NFConntrackMax
	}
	if o.NFConntrackTCPTimeoutEstablished != nil {
		body["nf_conntrack_tcp_timeout_established"] = *o.NFConntrackTCPTimeoutEstablished
	}
	if o.NFConntrackTCPTimeoutSynRecv != nil {
		body["nf_conntrack_tcp_timeout_syn_recv"] = *o.NFConntrackTCPTimeoutSynRecv
	}
	if o.Nftables != nil {
		body["nftables"] = *o.Nftables
	}
	if o.Nosmurfs != nil {
		body["nosmurfs"] = *o.Nosmurfs
	}
	if o.ProtectionSynflood != nil {
		body["protection_synflood"] = *o.ProtectionSynflood
	}
	if o.ProtectionSynfloodBurst != nil {
		body["protection_synflood_burst"] = *o.ProtectionSynfloodBurst
	}
	if o.ProtectionSynfloodRate != nil {
		body["protection_synflood_rate"] = *o.ProtectionSynfloodRate
	}
	if o.SmurfLogLevel != nil {
		body["smurf_log_level"] = *o.SmurfLogLevel
	}
	if o.TCPFlags != nil {
		body["tcpflags"] = *o.TCPFlags
	}
	if o.TCPFlagsLogLevel != nil {
		body["tcp_flags_log_level"] = *o.TCPFlagsLogLevel
	}
	if o.PolicyForward != nil {
		body["policy_forward"] = *o.PolicyForward
	}
	return body
}

// clusterFirewallOptionsBody projects the cluster-scope options into the
// PUT body, omitting nil fields entirely.
func clusterFirewallOptionsBody(o ClusterFirewallOptions) firewallOptionsPutBody {
	body := firewallOptionsPutBody{}
	if o.Ebitables != nil {
		body["ebtables"] = *o.Ebitables
	}
	if o.Enable != nil {
		body["enable"] = *o.Enable
	}
	if o.LogRateLimit != nil {
		body["log_ratelimit"] = *o.LogRateLimit
	}
	if o.PolicyForward != nil {
		body["policy_forward"] = *o.PolicyForward
	}
	if o.PolicyIn != nil {
		body["policy_in"] = *o.PolicyIn
	}
	if o.PolicyOut != nil {
		body["policy_out"] = *o.PolicyOut
	}
	return body
}

// GetClusterFirewallOptions fetches GET /cluster/firewall/options.
func (c *Client) GetClusterFirewallOptions(ctx context.Context) (*ClusterFirewallOptions, error) {
	var opts ClusterFirewallOptions
	if err := c.Do(ctx, "GET", "/cluster/firewall/options", nil, &opts); err != nil {
		return nil, err
	}
	return &opts, nil
}

// UpdateClusterFirewallOptions PUTs /cluster/firewall/options with the
// supplied options and translates deleteFields into PVE's `delete` query
// parameter (comma-separated settings to clear).
func (c *Client) UpdateClusterFirewallOptions(ctx context.Context, opts ClusterFirewallOptions, deleteFields []string) error {
	return c.Do(ctx, "PUT", haDeleteQuery("/cluster/firewall/options", deleteFields), clusterFirewallOptionsBody(opts), nil)
}

// GetNodeFirewallOptions fetches GET /nodes/{node}/firewall/options.
func (c *Client) GetNodeFirewallOptions(ctx context.Context, node string) (*FirewallOptions, error) {
	var opts FirewallOptions
	if err := c.Do(ctx, "GET", fmt.Sprintf("/nodes/%s/firewall/options", node), nil, &opts); err != nil {
		return nil, err
	}
	return &opts, nil
}

// UpdateNodeFirewallOptions PUTs /nodes/{node}/firewall/options. The pin
// lists `node` among the PUT parameters, so it travels in the body as
// well.
func (c *Client) UpdateNodeFirewallOptions(ctx context.Context, node string, opts FirewallOptions, deleteFields []string) error {
	body := firewallOptionsBody(opts)
	body["node"] = node
	return c.Do(ctx, "PUT", haDeleteQuery(fmt.Sprintf("/nodes/%s/firewall/options", node), deleteFields), body, nil)
}

// GetGuestFirewallOptions fetches GET
// /nodes/{node}/{qemu|lxc}/{vmid}/firewall/options. guestType is `qemu`
// for virtual machines and `lxc` for containers.
func (c *Client) GetGuestFirewallOptions(ctx context.Context, node, guestType string, vmid int64) (*FirewallOptions, error) {
	path := fmt.Sprintf("/nodes/%s/%s/%d/firewall/options", node, guestType, vmid)
	var opts FirewallOptions
	if err := c.Do(ctx, "GET", path, nil, &opts); err != nil {
		return nil, err
	}
	return &opts, nil
}

// UpdateGuestFirewallOptions PUTs
// /nodes/{node}/{qemu|lxc}/{vmid}/firewall/options. The pin lists `node`
// and `vmid` among the PUT parameters, so both travel in the body as well.
func (c *Client) UpdateGuestFirewallOptions(ctx context.Context, node, guestType string, vmid int64, opts FirewallOptions, deleteFields []string) error {
	path := fmt.Sprintf("/nodes/%s/%s/%d/firewall/options", node, guestType, vmid)
	body := firewallOptionsBody(opts)
	body["node"] = node
	body["vmid"] = vmid
	return c.Do(ctx, "PUT", haDeleteQuery(path, deleteFields), body, nil)
}

// GetVNetFirewallOptions fetches GET
// /cluster/sdn/vnets/{vnet}/firewall/options.
func (c *Client) GetVNetFirewallOptions(ctx context.Context, vnet string) (*FirewallOptions, error) {
	var opts FirewallOptions
	if err := c.Do(ctx, "GET", fmt.Sprintf("/cluster/sdn/vnets/%s/firewall/options", vnet), nil, &opts); err != nil {
		return nil, err
	}
	return &opts, nil
}

// UpdateVNetFirewallOptions PUTs
// /cluster/sdn/vnets/{vnet}/firewall/options. The pin lists `vnet` among
// the PUT parameters, so it travels in the body as well.
func (c *Client) UpdateVNetFirewallOptions(ctx context.Context, vnet string, opts FirewallOptions, deleteFields []string) error {
	body := firewallOptionsBody(opts)
	body["vnet"] = vnet
	return c.Do(ctx, "PUT", haDeleteQuery(fmt.Sprintf("/cluster/sdn/vnets/%s/firewall/options", vnet), deleteFields), body, nil)
}
