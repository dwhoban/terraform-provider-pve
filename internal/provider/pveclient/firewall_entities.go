// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"context"
	"fmt"
	"net/url"
)

// firewallPathSeg escapes a single URL path segment. IP set members are
// CIDRs whose embedded slash (e.g. 192.168.0.0/24) must travel as %2F so
// PVE's dispatcher still sees one path segment.
func firewallPathSeg(s string) string {
	return url.PathEscape(s)
}

// FirewallAlias is an IP or network alias in the cluster firewall
// configuration (POST/GET /cluster/firewall/aliases,
// GET/PUT/DELETE /cluster/firewall/aliases/{name}).
//
// Comment marshals unconditionally (no omitempty) so updates carry an
// explicit clear-to-empty value: PVE's firewall update handlers set a field
// when present and delete it when absent, so the empty string is the
// deliberate "clear" signal. On create the empty value is harmless. See
// AccessUser for the same convention.
type FirewallAlias struct {
	Name    string `json:"name"`
	Cidr    string `json:"cidr,omitempty"`
	Comment string `json:"comment"`
	Digest  string `json:"digest,omitempty"`
}

// FirewallIpset is an IP set in the cluster firewall configuration
// (GET/POST /cluster/firewall/ipset). Rename carries the pin's
// update-by-create form: posting with rename set to an existing set name
// updates that set's comment instead of creating a new set.
type FirewallIpset struct {
	Name    string `json:"name"`
	Comment string `json:"comment"`
	Rename  string `json:"rename,omitempty"`
	Digest  string `json:"digest,omitempty"`
}

// FirewallIpsetMember is one member entry of an IP set as returned by
// GET /cluster/firewall/ipset/{name}. Per-member comment and nomatch exist
// upstream but are not modeled; only the CIDR is consumed.
type FirewallIpsetMember struct {
	Cidr string `json:"cidr"`
}

// FirewallSecurityGroup is a security group in the cluster firewall
// configuration (GET/POST /cluster/firewall/groups, DELETE
// /cluster/firewall/groups/{group}). Rename carries the pin's
// update-by-create form: posting with rename set to an existing group name
// updates that group's comment instead of creating a new group.
type FirewallSecurityGroup struct {
	Group   string `json:"group"`
	Comment string `json:"comment"`
	Rename  string `json:"rename,omitempty"`
	Digest  string `json:"digest,omitempty"`
}

// ListFirewallAliases returns the aliases from GET /cluster/firewall/aliases.
func (c *Client) ListFirewallAliases(ctx context.Context) ([]FirewallAlias, error) {
	var out []FirewallAlias
	if err := c.Do(ctx, "GET", "/cluster/firewall/aliases", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetFirewallAlias reads GET /cluster/firewall/aliases/{name}.
func (c *Client) GetFirewallAlias(ctx context.Context, name string) (*FirewallAlias, error) {
	var out FirewallAlias
	path := fmt.Sprintf("/cluster/firewall/aliases/%s", firewallPathSeg(name))
	if err := c.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateFirewallAlias POSTs /cluster/firewall/aliases with the supplied alias.
func (c *Client) CreateFirewallAlias(ctx context.Context, alias FirewallAlias) error {
	return c.Do(ctx, "POST", "/cluster/firewall/aliases", alias, nil)
}

// UpdateFirewallAlias PUTs /cluster/firewall/aliases/{name}. The comment is
// always emitted (see FirewallAlias): an empty string clears it upstream.
func (c *Client) UpdateFirewallAlias(ctx context.Context, name string, alias FirewallAlias) error {
	path := fmt.Sprintf("/cluster/firewall/aliases/%s", firewallPathSeg(name))
	return c.Do(ctx, "PUT", path, alias, nil)
}

// DeleteFirewallAlias DELETEs /cluster/firewall/aliases/{name}.
func (c *Client) DeleteFirewallAlias(ctx context.Context, name string) error {
	return c.Do(ctx, "DELETE", fmt.Sprintf("/cluster/firewall/aliases/%s", firewallPathSeg(name)), nil, nil)
}

// ListFirewallIpsets returns the IP sets from GET /cluster/firewall/ipset.
// The set's own comment lives only on this listing.
func (c *Client) ListFirewallIpsets(ctx context.Context) ([]FirewallIpset, error) {
	var out []FirewallIpset
	if err := c.Do(ctx, "GET", "/cluster/firewall/ipset", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// CreateFirewallIpset POSTs /cluster/firewall/ipset. With Rename set to an
// existing set name the call updates that set's comment instead (the pin's
// update-by-create form) — see UpdateFirewallIpset.
func (c *Client) CreateFirewallIpset(ctx context.Context, ipset FirewallIpset) error {
	return c.Do(ctx, "POST", "/cluster/firewall/ipset", ipset, nil)
}

// UpdateFirewallIpset updates an existing set's comment via the pin's
// update-by-create form: POST /cluster/firewall/ipset with rename set to
// the set's own name. An empty comment clears it.
func (c *Client) UpdateFirewallIpset(ctx context.Context, name, comment string) error {
	return c.CreateFirewallIpset(ctx, FirewallIpset{Name: name, Rename: name, Comment: comment})
}

// ListFirewallIpsetMembers returns the member entries from
// GET /cluster/firewall/ipset/{name}.
func (c *Client) ListFirewallIpsetMembers(ctx context.Context, name string) ([]FirewallIpsetMember, error) {
	var out []FirewallIpsetMember
	path := fmt.Sprintf("/cluster/firewall/ipset/%s", firewallPathSeg(name))
	if err := c.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// AddFirewallIpsetMember POSTs /cluster/firewall/ipset/{name} to add one
// CIDR, single IP, or alias reference to the set.
func (c *Client) AddFirewallIpsetMember(ctx context.Context, name, cidr string) error {
	path := fmt.Sprintf("/cluster/firewall/ipset/%s", firewallPathSeg(name))
	return c.Do(ctx, "POST", path, FirewallIpsetMember{Cidr: cidr}, nil)
}

// RemoveFirewallIpsetMember DELETEs /cluster/firewall/ipset/{name}/{cidr}.
// The CIDR is path-escaped so embedded slashes travel as %2F.
func (c *Client) RemoveFirewallIpsetMember(ctx context.Context, name, cidr string) error {
	path := fmt.Sprintf("/cluster/firewall/ipset/%s/%s", firewallPathSeg(name), firewallPathSeg(cidr))
	return c.Do(ctx, "DELETE", path, nil, nil)
}

// DeleteFirewallIpset DELETEs /cluster/firewall/ipset/{name} with
// force=true so any remaining members are removed together with the set.
func (c *Client) DeleteFirewallIpset(ctx context.Context, name string) error {
	path := fmt.Sprintf("/cluster/firewall/ipset/%s?force=true", firewallPathSeg(name))
	return c.Do(ctx, "DELETE", path, nil, nil)
}

// ListFirewallSecurityGroups returns the security groups from
// GET /cluster/firewall/groups.
func (c *Client) ListFirewallSecurityGroups(ctx context.Context) ([]FirewallSecurityGroup, error) {
	var out []FirewallSecurityGroup
	if err := c.Do(ctx, "GET", "/cluster/firewall/groups", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// CreateFirewallSecurityGroup POSTs /cluster/firewall/groups. With Rename
// set to an existing group name the call updates that group's comment
// instead (the pin's update-by-create form) — see
// UpdateFirewallSecurityGroup.
func (c *Client) CreateFirewallSecurityGroup(ctx context.Context, group FirewallSecurityGroup) error {
	return c.Do(ctx, "POST", "/cluster/firewall/groups", group, nil)
}

// UpdateFirewallSecurityGroup updates an existing group's comment via the
// pin's update-by-create form: POST /cluster/firewall/groups with rename
// set to the group's own name. An empty comment clears it.
func (c *Client) UpdateFirewallSecurityGroup(ctx context.Context, group FirewallSecurityGroup) error {
	return c.CreateFirewallSecurityGroup(ctx, group)
}

// DeleteFirewallSecurityGroup DELETEs /cluster/firewall/groups/{group}.
func (c *Client) DeleteFirewallSecurityGroup(ctx context.Context, group string) error {
	return c.Do(ctx, "DELETE", fmt.Sprintf("/cluster/firewall/groups/%s", firewallPathSeg(group)), nil, nil)
}
