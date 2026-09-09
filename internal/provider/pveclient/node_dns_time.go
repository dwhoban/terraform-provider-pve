// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"context"
	"fmt"
)

// NodeDNS mirrors GET /nodes/{node}/dns. The PVE endpoint accepts and
// returns the same four fields (search domain plus three name servers);
// unset servers serialize as empty strings.
type NodeDNS struct {
	Search string `json:"search,omitempty"`
	DNS1   string `json:"dns1,omitempty"`
	DNS2   string `json:"dns2,omitempty"`
	DNS3   string `json:"dns3,omitempty"`
}

// GetNodeDNS fetches /nodes/{node}/dns.
func (c *Client) GetNodeDNS(ctx context.Context, node string) (*NodeDNS, error) {
	var out NodeDNS
	path := fmt.Sprintf("/nodes/%s/dns", node)
	if err := c.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// SetNodeDNS PUTs /nodes/{node}/dns.
func (c *Client) SetNodeDNS(ctx context.Context, node string, dns NodeDNS) error {
	path := fmt.Sprintf("/nodes/%s/dns", node)
	return c.Do(ctx, "PUT", path, dns, nil)
}

// NodeTime mirrors GET/PUT /nodes/{node}/time. PVE accepts any IANA
// timezone name; we don't validate the field on the client side because
// PVE's own validator rejects bogus values and surfaces them as APIError.
type NodeTime struct {
	Timezone string `json:"timezone,omitempty"`
}

// GetNodeTime fetches /nodes/{node}/time.
func (c *Client) GetNodeTime(ctx context.Context, node string) (*NodeTime, error) {
	var out NodeTime
	path := fmt.Sprintf("/nodes/%s/time", node)
	if err := c.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// SetNodeTime PUTs /nodes/{node}/time.
func (c *Client) SetNodeTime(ctx context.Context, node string, t NodeTime) error {
	path := fmt.Sprintf("/nodes/%s/time", node)
	return c.Do(ctx, "PUT", path, t, nil)
}
