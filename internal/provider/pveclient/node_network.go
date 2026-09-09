// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// NetworkInterface mirrors a single entry in /nodes/{node}/network. PVE
// returns the same field set regardless of the interface type
// (eth | bond | bridge | vlan | ovsbond | ovsbridge | ovsintport | ovsport |
// unknown), so the consumer branches on the `type` field rather than
// picking between typed structs.
//
// Autostart and Active come back as 0/1 integers on most PVE versions but
// newer ones emit real booleans; the custom UnmarshalJSON accepts both.
//
// BridgePorts is a comma-separated string per the PVE schema; callers
// split it on demand for display.
type NetworkInterface struct {
	Iface              string            `json:"iface"`
	Type               string            `json:"type"`
	Autostart          *bool             `json:"autostart,omitempty"`
	Active             bool              `json:"-"`
	CIDR               string            `json:"cidr,omitempty"`
	Address            string            `json:"address,omitempty"`
	Gateway            string            `json:"gateway,omitempty"`
	MTU                int               `json:"mtu,omitempty"`
	BridgePorts        string            `json:"bridge_ports,omitempty"`
	BridgeSTP          *bool             `json:"bridge_stp,omitempty"`
	BridgeVIDs         string            `json:"bridge_vids,omitempty"`
	BridgeVLANAware    *bool             `json:"bridge_vlan_aware,omitempty"`
	BridgeFD           int               `json:"bridge_fd,omitempty"`
	BondXmitHashPolicy string            `json:"bond_xmit_hash_policy,omitempty"`
	VLANID             int               `json:"vlan-id,omitempty"`
	VLANRawDevice      string            `json:"vlan-raw-device,omitempty"`
	OVSBridge          string            `json:"ovs_bridge,omitempty"`
	OVSType            string            `json:"ovs_type,omitempty"`
	OVSOptions         map[string]string `json:"ovs_options,omitempty"`
	BondMode           string            `json:"bond_mode,omitempty"`
	BondPrimary        string            `json:"bond_primary,omitempty"`
	Slaves             []string          `json:"slaves,omitempty"`
	Comments           string            `json:"comments,omitempty"`
	Families           []string          `json:"families,omitempty"`
	Method             string            `json:"method,omitempty"`
	Digest             string            `json:"digest,omitempty"`
}

// networkInterfaceRaw mirrors the wire shape with the boolish fields as
// json.RawMessage so the custom unmarshaler can accept either bool or int.
type networkInterfaceRaw struct {
	Iface              string            `json:"iface"`
	Type               string            `json:"type"`
	Autostart          json.RawMessage   `json:"autostart"`
	Active             json.RawMessage   `json:"active"`
	CIDR               string            `json:"cidr,omitempty"`
	Address            string            `json:"address,omitempty"`
	Gateway            string            `json:"gateway,omitempty"`
	BridgePorts        string            `json:"bridge_ports,omitempty"`
	BridgeSTP          *bool             `json:"bridge_stp,omitempty"`
	BridgeVIDs         string            `json:"bridge_vids,omitempty"`
	BridgeVLANAware    json.RawMessage   `json:"bridge_vlan_aware,omitempty"`
	BridgeFD           int               `json:"bridge_fd,omitempty"`
	BondXmitHashPolicy string            `json:"bond_xmit_hash_policy,omitempty"`
	VLANID             int               `json:"vlan-id,omitempty"`
	VLANRawDevice      string            `json:"vlan-raw-device,omitempty"`
	OVSBridge          string            `json:"ovs_bridge,omitempty"`
	OVSType            string            `json:"ovs_type,omitempty"`
	OVSOptions         map[string]string `json:"ovs_options,omitempty"`
	BondMode           string            `json:"bond_mode,omitempty"`
	BondPrimary        string            `json:"bond_primary,omitempty"`
	Slaves             []string          `json:"slaves,omitempty"`
	Comments           string            `json:"comments,omitempty"`
	Families           []string          `json:"families,omitempty"`
	Method             string            `json:"method,omitempty"`
	Digest             string            `json:"digest,omitempty"`
}

// UnmarshalJSON tolerates the int 0/1 encoding of Autostart/Active that
// older PVE releases emit.
func (n *NetworkInterface) UnmarshalJSON(data []byte) error {
	var raw networkInterfaceRaw
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	n.Iface = raw.Iface
	n.Type = raw.Type
	n.Autostart = nodeNetworkBoolishPtr(raw.Autostart)
	n.Active = decodeBoolish(raw.Active)
	n.CIDR = raw.CIDR
	n.Address = raw.Address
	n.BridgeVIDs = raw.BridgeVIDs
	if v := nodeNetworkBoolishPtr(raw.BridgeVLANAware); v != nil {
		n.BridgeVLANAware = v
	}
	n.BridgeFD = raw.BridgeFD
	n.BondXmitHashPolicy = raw.BondXmitHashPolicy
	n.BridgePorts = raw.BridgePorts
	n.BridgeSTP = raw.BridgeSTP
	n.BridgeFD = raw.BridgeFD
	n.VLANID = raw.VLANID
	n.VLANRawDevice = raw.VLANRawDevice
	n.OVSBridge = raw.OVSBridge
	n.OVSType = raw.OVSType
	n.OVSOptions = raw.OVSOptions
	n.BondMode = raw.BondMode
	n.BondPrimary = raw.BondPrimary
	n.Slaves = raw.Slaves
	n.Comments = raw.Comments
	n.Families = raw.Families
	n.Method = raw.Method
	n.Digest = raw.Digest
	return nil
}

// decodeBoolish interprets PVE's lenient bool encoding (true|false|0|1).
// Used by NetworkInterface and Disk to round-trip values PVE emits as
// 0/1 integers on some endpoints and true/false on others.
func decodeBoolish(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return false
	}
	switch trimmed[0] {
	case 't', 'T', '1':
		return true

	case 'f', 'F', '0':
		return false
	}
	var b bool
	if err := json.Unmarshal(trimmed, &b); err == nil {
		return b
	}
	return false
}

// nodeNetworkBoolishPtr decodes a raw boolish value into a *bool, keeping
// nil for absent/null so callers can distinguish "unset" from "false".
func nodeNetworkBoolishPtr(raw json.RawMessage) *bool {
	if len(raw) == 0 {
		return nil
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil
	}
	v := decodeBoolish(raw)
	return &v
}

// ListNodeNetwork returns the array from GET /nodes/{node}/network.
func (c *Client) ListNodeNetwork(ctx context.Context, node string) ([]NetworkInterface, error) {
	var out []NetworkInterface
	path := fmt.Sprintf("/nodes/%s/network", node)
	if err := c.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// CreateNodeNetwork POSTs /nodes/{node}/network with the supplied interface.
// The caller supplies Iface; everything else is sent as-is.
func (c *Client) CreateNodeNetwork(ctx context.Context, node string, iface NetworkInterface) error {
	path := fmt.Sprintf("/nodes/%s/network", node)
	return c.Do(ctx, "POST", path, iface, nil)
}

// GetNodeNetwork reads /nodes/{node}/network/{iface}. PVE version skew
// returns either a single object or a one-element array; decode into a slice
// first and take [0].
func (c *Client) GetNodeNetwork(ctx context.Context, node, iface string) (*NetworkInterface, error) {
	path := fmt.Sprintf("/nodes/%s/network/%s", node, iface)
	var raw []NetworkInterface
	if err := c.Do(ctx, "GET", path, nil, &raw); err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		var single NetworkInterface
		if err := c.Do(ctx, "GET", path, nil, &single); err != nil {
			return nil, err
		}
		return &single, nil
	}
	return &raw[0], nil
}

// UpdateNodeNetwork PUTs /nodes/{node}/network/{iface} with the supplied
// body and translates the delete slice into the PVE `delete` query
// parameter (comma-separated field names to clear).
func (c *Client) UpdateNodeNetwork(ctx context.Context, node, iface string, body NetworkInterface, deleteFields []string) error {
	path := fmt.Sprintf("/nodes/%s/network/%s", node, iface)
	if len(deleteFields) > 0 {
		path += "?delete=" + url.QueryEscape(strings.Join(deleteFields, ","))
	}
	return c.Do(ctx, "PUT", path, body, nil)
}

// DeleteNodeNetwork deletes a single interface by name.
func (c *Client) DeleteNodeNetwork(ctx context.Context, node, iface string) error {
	path := fmt.Sprintf("/nodes/%s/network/%s", node, iface)
	return c.Do(ctx, "DELETE", path, nil, nil)
}

// ReloadNodeNetwork PUTs /nodes/{node}/network to apply the pending network
// configuration. Returns the upid for the spawned task.
func (c *Client) ReloadNodeNetwork(ctx context.Context, node string) (string, error) {
	var upid string
	path := fmt.Sprintf("/nodes/%s/network", node)
	if err := c.Do(ctx, "PUT", path, nil, &upid); err != nil {
		return "", err
	}
	return upid, nil
}

// RevertNodeNetwork DELETEs /nodes/{node}/network to roll back any pending
// (not yet applied) edits.
func (c *Client) RevertNodeNetwork(ctx context.Context, node string) error {
	path := fmt.Sprintf("/nodes/%s/network", node)
	return c.Do(ctx, "DELETE", path, nil, nil)
}
