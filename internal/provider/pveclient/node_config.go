// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
)

// NodeACMEDomain represents a single ACME registration row on a node. PVE
// stores these as a map keyed by domain, but the natural shape for callers
// is an ordered slice, so we accept a slice in Go and translate on the
// wire.
type NodeACMEDomain struct {
	Domain string   `json:"domain"`
	Alias  []string `json:"alias,omitempty"`
}

// NodeConfig is the resource payload for GET/PUT /nodes/{node}/config. The
// PVE endpoint persists `/etc/pve/nodes/<node>/config` and tracks node-level
// metadata (description, wake-on-LAN MAC, ACME registrations, the
// start-all-on-boot delay, and an opaque digest used for optimistic
// concurrency).
type NodeConfig struct {
	Description         string           `json:"description,omitempty"`
	Wakeonlan           string           `json:"wakeonlan,omitempty"`
	ACMEDomains         []NodeACMEDomain `json:"-"`
	StartAllOnBootDelay int              `json:"startall_onboot_delay,omitempty"`
	Digest              string           `json:"digest,omitempty"`
}

// MarshalJSON translates ACMEDomains into the PVE shape: a map keyed by
// domain with a nested `alias` array. Domains must be unique; the last
// duplicate wins on the server side.
func (c NodeConfig) MarshalJSON() ([]byte, error) {
	type alias struct {
		Alias []string `json:"alias,omitempty"`
	}
	type wire struct {
		Description         string           `json:"description,omitempty"`
		Wakeonlan           string           `json:"wakeonlan,omitempty"`
		ACME                map[string]alias `json:"acme,omitempty"`
		StartAllOnBootDelay int              `json:"startall_onboot_delay,omitempty"`
		Digest              string           `json:"digest,omitempty"`
	}
	out := wire{
		Description:         c.Description,
		Wakeonlan:           c.Wakeonlan,
		StartAllOnBootDelay: c.StartAllOnBootDelay,
		Digest:              c.Digest,
	}
	if len(c.ACMEDomains) > 0 {
		out.ACME = make(map[string]alias, len(c.ACMEDomains))
		for _, d := range c.ACMEDomains {
			out.ACME[d.Domain] = alias{Alias: d.Alias}
		}
	}
	return json.Marshal(out)
}

// UnmarshalJSON translates PVE's ACME map shape into the Go slice form.
// Domains are sorted by name so two reads of the same config produce equal
// structs.
func (c *NodeConfig) UnmarshalJSON(data []byte) error {
	type alias struct {
		Alias []string `json:"alias,omitempty"`
	}
	type wire struct {
		Description         string           `json:"description,omitempty"`
		Wakeonlan           string           `json:"wakeonlan,omitempty"`
		ACME                map[string]alias `json:"acme,omitempty"`
		StartAllOnBootDelay int              `json:"startall_onboot_delay,omitempty"`
		Digest              string           `json:"digest,omitempty"`
	}
	var w wire
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	c.Description = w.Description
	c.Wakeonlan = w.Wakeonlan
	c.StartAllOnBootDelay = w.StartAllOnBootDelay
	c.Digest = w.Digest
	if len(w.ACME) > 0 {
		domains := make([]string, 0, len(w.ACME))
		for d := range w.ACME {
			domains = append(domains, d)
		}
		sort.Strings(domains)
		c.ACMEDomains = make([]NodeACMEDomain, 0, len(domains))
		for _, d := range domains {
			c.ACMEDomains = append(c.ACMEDomains, NodeACMEDomain{Domain: d, Alias: w.ACME[d].Alias})
		}
	} else {
		c.ACMEDomains = nil
	}
	return nil
}

// GetNodeConfig fetches the persistent node configuration.
func (c *Client) GetNodeConfig(ctx context.Context, node string) (*NodeConfig, error) {
	var cfg NodeConfig
	path := fmt.Sprintf("/nodes/%s/config", node)
	if err := c.Do(ctx, "GET", path, nil, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// UpdateNodeConfig writes the supplied configuration. The supplied Digest is
// passed through unchanged so callers can opt into PVE's optimistic
// concurrency check; pass an empty string to skip the check.
func (c *Client) UpdateNodeConfig(ctx context.Context, node string, cfg NodeConfig) error {
	path := fmt.Sprintf("/nodes/%s/config", node)
	// Encode cfg through its MarshalJSON so the acme map shape is emitted
	// correctly; passing cfg directly to Do would call the default
	// reflection encoder which would emit the `-` tagged field as
	// "ACMEDomains".
	buf, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("pveclient: marshal NodeConfig: %w", err)
	}
	return c.Do(ctx, "PUT", path, json.RawMessage(bytes.TrimSpace(buf)), nil)
}
