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

// SdnZone mirrors one SDN zone object as read from
// /cluster/sdn/zones[/{zone}] or written by POST/PUT /cluster/sdn/zones.
// PVE stores zones in sdn.cfg as a section config: zone and type are
// required; every other field is optional (nil/"" = not set). The pin
// defines one union parameter set per verb, with each field scoped to one
// or more zone types in its description ("VLAN zone only", ...) — the
// provider layer validates the per-type projections.
//
// Wire quirks normalized here (same rationale as MetricsServer):
//
//   - Booleans (bridge-disable-mac-learning, advertise-subnets,
//     disable-arp-nd-suppression, exitnodes-local-routing) arrive as 0/1
//     integers from sdn.cfg on most versions and as real booleans on others.
//   - Numeric settings (mtu, tag, vxlan-port, vrf-vxlan) may arrive as JSON
//     numbers or as strings, because the section config stores text on disk.
type SdnZone struct {
	Zone string `json:"zone,omitempty"`
	Type string `json:"type,omitempty"`

	// Settings common to every zone type.
	IPAM       string `json:"ipam,omitempty"`
	DNS        string `json:"dns,omitempty"`
	DNSZone    string `json:"dnszone,omitempty"`
	ReverseDNS string `json:"reversedns,omitempty"`
	DHCP       string `json:"dhcp,omitempty"`
	MTU        *int64 `json:"mtu,omitempty"`
	Nodes      string `json:"nodes,omitempty"`

	// VLAN and QinQ zone types.
	Bridge                   string `json:"bridge,omitempty"`
	BridgeDisableMacLearning *bool  `json:"bridge-disable-mac-learning,omitempty"`

	// QinQ zone type.
	Tag          *int64 `json:"tag,omitempty"`
	VlanProtocol string `json:"vlan-protocol,omitempty"`

	// VXLAN zone type.
	Peers     string `json:"peers,omitempty"`
	VxlanPort *int64 `json:"vxlan-port,omitempty"`
	Fabric    string `json:"fabric,omitempty"`

	// EVPN zone type.
	Controller              string   `json:"controller,omitempty"`
	SecondaryControllers    []string `json:"secondary-controllers,omitempty"`
	AdvertiseSubnets        *bool    `json:"advertise-subnets,omitempty"`
	DisableArpNdSuppression *bool    `json:"disable-arp-nd-suppression,omitempty"`
	ExitNodes               string   `json:"exitnodes,omitempty"`
	ExitNodesLocalRouting   *bool    `json:"exitnodes-local-routing,omitempty"`
	ExitNodesPrimary        string   `json:"exitnodes-primary,omitempty"`
	Mac                     string   `json:"mac,omitempty"`
	RtImport                string   `json:"rt-import,omitempty"`
	VrfVxlan                *int64   `json:"vrf-vxlan,omitempty"`

	// Digest is the read-only section config revision.
	Digest string `json:"digest,omitempty"`
}

// sdnZoneRaw mirrors the wire shape with the lenient boolean and integer
// fields left as raw JSON.
type sdnZoneRaw struct {
	Zone string `json:"zone"`
	Type string `json:"type"`
	IPAM string `json:"ipam"`
	DNS  string `json:"dns"`

	DNSZone    string          `json:"dnszone"`
	ReverseDNS string          `json:"reversedns"`
	DHCP       string          `json:"dhcp"`
	MTU        json.RawMessage `json:"mtu"`
	Nodes      string          `json:"nodes"`

	Bridge                   string          `json:"bridge"`
	BridgeDisableMacLearning json.RawMessage `json:"bridge-disable-mac-learning"`

	Tag          json.RawMessage `json:"tag"`
	VlanProtocol string          `json:"vlan-protocol"`

	Peers     string          `json:"peers"`
	VxlanPort json.RawMessage `json:"vxlan-port"`
	Fabric    string          `json:"fabric"`

	Controller              string          `json:"controller"`
	SecondaryControllers    []string        `json:"secondary-controllers"`
	AdvertiseSubnets        json.RawMessage `json:"advertise-subnets"`
	DisableArpNdSuppression json.RawMessage `json:"disable-arp-nd-suppression"`
	ExitNodes               string          `json:"exitnodes"`
	ExitNodesLocalRouting   json.RawMessage `json:"exitnodes-local-routing"`
	ExitNodesPrimary        string          `json:"exitnodes-primary"`
	Mac                     string          `json:"mac"`
	RtImport                string          `json:"rt-import"`
	VrfVxlan                json.RawMessage `json:"vrf-vxlan"`

	Digest string `json:"digest"`
}

// UnmarshalJSON tolerates the 0/1 encoding of the boolean fields and the
// string encoding of numeric fields (see SdnZone), keeping absent settings
// nil.
func (z *SdnZone) UnmarshalJSON(data []byte) error {
	var raw sdnZoneRaw
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	z.Zone = raw.Zone
	z.Type = raw.Type
	z.IPAM = raw.IPAM
	z.DNS = raw.DNS
	z.DNSZone = raw.DNSZone
	z.ReverseDNS = raw.ReverseDNS
	z.DHCP = raw.DHCP
	z.MTU = sdnZoneInt64FromRaw(raw.MTU)
	z.Nodes = raw.Nodes
	z.Bridge = raw.Bridge
	z.BridgeDisableMacLearning = nodeNetworkBoolishPtr(raw.BridgeDisableMacLearning)
	z.Tag = sdnZoneInt64FromRaw(raw.Tag)
	z.VlanProtocol = raw.VlanProtocol
	z.Peers = raw.Peers
	z.VxlanPort = sdnZoneInt64FromRaw(raw.VxlanPort)
	z.Fabric = raw.Fabric
	z.Controller = raw.Controller
	z.SecondaryControllers = raw.SecondaryControllers
	z.AdvertiseSubnets = nodeNetworkBoolishPtr(raw.AdvertiseSubnets)
	z.DisableArpNdSuppression = nodeNetworkBoolishPtr(raw.DisableArpNdSuppression)
	z.ExitNodes = raw.ExitNodes
	z.ExitNodesLocalRouting = nodeNetworkBoolishPtr(raw.ExitNodesLocalRouting)
	z.ExitNodesPrimary = raw.ExitNodesPrimary
	z.Mac = raw.Mac
	z.RtImport = raw.RtImport
	z.VrfVxlan = sdnZoneInt64FromRaw(raw.VrfVxlan)
	z.Digest = raw.Digest
	return nil
}

// sdnZoneInt64FromRaw decodes a lenient integer setting: PVE stores section
// config values as text on disk, so numeric zone fields may arrive as JSON
// numbers or as JSON strings. Absent and null decode to nil, and a
// non-numeric value decodes to nil rather than failing the whole read.
func sdnZoneInt64FromRaw(raw json.RawMessage) *int64 {
	if len(raw) == 0 {
		return nil
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil
	}
	trimmed = strings.Trim(trimmed, `"`)
	v, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil {
		return nil
	}
	return &v
}

// ListSdnZones returns the array from GET /cluster/sdn/zones. zoneType
// optionally narrows the listing to one zone type per the pin's `type`
// filter; an empty string lists every type.
func (c *Client) ListSdnZones(ctx context.Context, zoneType string) ([]SdnZone, error) {
	path := "/cluster/sdn/zones"
	if zoneType != "" {
		path += "?type=" + url.QueryEscape(zoneType)
	}
	var zones []SdnZone
	if err := c.Do(ctx, "GET", path, nil, &zones); err != nil {
		return nil, fmt.Errorf("pveclient: list sdn zones: %w", err)
	}
	return zones, nil
}

// GetSdnZone reads /cluster/sdn/zones/{zone}.
func (c *Client) GetSdnZone(ctx context.Context, zone string) (*SdnZone, error) {
	var z SdnZone
	if err := c.Do(ctx, "GET", "/cluster/sdn/zones/"+url.PathEscape(zone), nil, &z); err != nil {
		return nil, fmt.Errorf("pveclient: read sdn zone %s: %w", zone, err)
	}
	z.Zone = zone
	return &z, nil
}

// CreateSdnZone POSTs /cluster/sdn/zones. Zone and Type are required per
// the pin.
func (c *Client) CreateSdnZone(ctx context.Context, z SdnZone) error {
	if z.Zone == "" {
		return fmt.Errorf("pveclient: create sdn zone: zone is required")
	}
	if z.Type == "" {
		return fmt.Errorf("pveclient: create sdn zone %s: type is required", z.Zone)
	}
	if err := c.Do(ctx, "POST", "/cluster/sdn/zones", z, nil); err != nil {
		return fmt.Errorf("pveclient: create sdn zone %s: %w", z.Zone, err)
	}
	return nil
}

// UpdateSdnZone PUTs /cluster/sdn/zones/{zone} with the supplied fields and
// translates deleteFields into the PVE `delete` query parameter
// (comma-separated field names to clear). The pin's update verb carries no
// `type` parameter — the zone type is immutable — so Type is never sent;
// `zone` is a required PUT parameter and is forced to the path zone.
func (c *Client) UpdateSdnZone(ctx context.Context, zone string, z SdnZone, deleteFields []string) error {
	z.Zone = zone
	z.Type = ""
	path := "/cluster/sdn/zones/" + url.PathEscape(zone)
	if len(deleteFields) > 0 {
		path += "?delete=" + url.QueryEscape(strings.Join(deleteFields, ","))
	}
	if err := c.Do(ctx, "PUT", path, z, nil); err != nil {
		return fmt.Errorf("pveclient: update sdn zone %s: %w", zone, err)
	}
	return nil
}

// DeleteSdnZone deletes an SDN zone object. The mutation is synchronous per
// the pin (no task is spawned); applying it to the running configuration is
// a separate PUT /cluster/sdn step.
func (c *Client) DeleteSdnZone(ctx context.Context, zone string) error {
	if err := c.Do(ctx, "DELETE", "/cluster/sdn/zones/"+url.PathEscape(zone), nil, nil); err != nil {
		return fmt.Errorf("pveclient: delete sdn zone %s: %w", zone, err)
	}
	return nil
}
