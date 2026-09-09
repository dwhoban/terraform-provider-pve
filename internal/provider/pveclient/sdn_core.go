// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// SdnVnet is an SDN vnet object (GET/POST /cluster/sdn/vnets,
// GET/PUT/DELETE /cluster/sdn/vnets/{vnet}). Tag is the VLAN tag (VLAN and
// QinQ zones) or VXLAN VNI (VXLAN and EVPN zones). State carries the pin's
// pending-config marker (new | changed | deleted).
type SdnVnet struct {
	Vnet         string `json:"vnet"`
	Zone         string `json:"zone,omitempty"`
	Alias        string `json:"alias,omitempty"`
	Tag          *int64 `json:"tag,omitempty"`
	VlanAware    *bool  `json:"vlanaware,omitempty"`
	IsolatePorts *bool  `json:"isolate-ports,omitempty"`
	Digest       string `json:"digest,omitempty"`
	State        string `json:"state,omitempty"`
	Type         string `json:"type,omitempty"`
}

// SdnSubnet is an SDN subnet object nested under a vnet
// GET/PUT/DELETE /cluster/sdn/vnets/{vnet}/subnets/{subnet}). Subnet is the
// PVE `pve-sdn-subnet-id` CIDR identifier with the mask separator written as
// a hyphen (for example `10.0.0.0-24`).
type SdnSubnet struct {
	Subnet        string   `json:"subnet"`
	Type          string   `json:"type,omitempty"`
	Vnet          string   `json:"vnet,omitempty"`
	Gateway       string   `json:"gateway,omitempty"`
	Snat          *bool    `json:"snat,omitempty"`
	DhcpDnsServer string   `json:"dhcp-dns-server,omitempty"`
	DhcpRange     []string `json:"dhcp-range,omitempty"`
	Dnszoneprefix string   `json:"dnszoneprefix,omitempty"`
}

// SdnController is an SDN controller object (GET/POST
// /cluster/sdn/controllers, GET/PUT/DELETE
// /cluster/sdn/controllers/{controller}). Type is one of the pin's plugin
// types: bgp | evpn | faucet | isis. Nodes, Peers, and IsisIfaces carry the
// pin's comma-separated wire strings split into Go slices; the write path
// joins them again. The pin names the read-side multipath flag
// `bgp-multipath-as-relax` and the write-side parameter
// `bgp-multipath-as-path-relax`; both decode into
// BgpMultipathAsPathRelax.
type SdnController struct {
	Controller              string   `json:"controller"`
	Type                    string   `json:"type,omitempty"`
	ASN                     *int64   `json:"asn,omitempty"`
	BgpMode                 string   `json:"bgp-mode,omitempty"`
	BgpMultipathAsPathRelax *bool    `json:"-"`
	Ebgp                    *bool    `json:"-"`
	EbgpMultihop            *int64   `json:"ebgp-multihop,omitempty"`
	Fabric                  string   `json:"fabric,omitempty"`
	IsisDomain              string   `json:"isis-domain,omitempty"`
	IsisIfaces              []string `json:"-"`
	IsisNet                 string   `json:"isis-net,omitempty"`
	Loopback                string   `json:"loopback,omitempty"`
	Node                    string   `json:"node,omitempty"`
	Nodes                   []string `json:"-"`
	PeerGroupName           string   `json:"peer-group-name,omitempty"`
	Peers                   []string `json:"-"`
	RouteMapIn              string   `json:"route-map-in,omitempty"`
	RouteMapOut             string   `json:"route-map-out,omitempty"`
	Digest                  string   `json:"digest,omitempty"`
	State                   string   `json:"state,omitempty"`
}

// sdnControllerWire mirrors the wire shape of SdnController: the list
// fields as comma-separated strings and the bool fields tolerating either
// encoding.
type sdnControllerWire struct {
	Controller              string          `json:"controller"`
	Type                    string          `json:"type,omitempty"`
	ASN                     *int64          `json:"asn,omitempty"`
	BgpMode                 string          `json:"bgp-mode,omitempty"`
	BgpMultipathAsPathRelax json.RawMessage `json:"bgp-multipath-as-path-relax,omitempty"`
	BgpMultipathAsRelax     json.RawMessage `json:"bgp-multipath-as-relax,omitempty"`
	Ebgp                    json.RawMessage `json:"ebgp,omitempty"`
	EbgpMultihop            *int64          `json:"ebgp-multihop,omitempty"`
	Fabric                  string          `json:"fabric,omitempty"`
	IsisDomain              string          `json:"isis-domain,omitempty"`
	IsisIfaces              string          `json:"isis-ifaces,omitempty"`
	IsisNet                 string          `json:"isis-net,omitempty"`
	Loopback                string          `json:"loopback,omitempty"`
	Node                    string          `json:"node,omitempty"`
	Nodes                   string          `json:"nodes,omitempty"`
	PeerGroupName           string          `json:"peer-group-name,omitempty"`
	Peers                   string          `json:"peers,omitempty"`
	RouteMapIn              string          `json:"route-map-in,omitempty"`
	RouteMapOut             string          `json:"route-map-out,omitempty"`
	Digest                  string          `json:"digest,omitempty"`
	State                   string          `json:"state,omitempty"`
}

// MarshalJSON encodes the controller for create and update, joining the
// list fields into PVE's comma-separated wire strings.
func (s SdnController) MarshalJSON() ([]byte, error) {
	wire := sdnControllerWire{
		Controller:    s.Controller,
		Type:          s.Type,
		ASN:           s.ASN,
		BgpMode:       s.BgpMode,
		EbgpMultihop:  s.EbgpMultihop,
		Fabric:        s.Fabric,
		IsisDomain:    s.IsisDomain,
		IsisIfaces:    strings.Join(s.IsisIfaces, ","),
		IsisNet:       s.IsisNet,
		Loopback:      s.Loopback,
		Node:          s.Node,
		Nodes:         strings.Join(s.Nodes, ","),
		PeerGroupName: s.PeerGroupName,
		Peers:         strings.Join(s.Peers, ","),
		RouteMapIn:    s.RouteMapIn,
		RouteMapOut:   s.RouteMapOut,
		Digest:        s.Digest,
		State:         s.State,
	}
	if s.BgpMultipathAsPathRelax != nil {
		wire.BgpMultipathAsPathRelax = json.RawMessage(sdnBoolJSON(*s.BgpMultipathAsPathRelax))
	}
	if s.Ebgp != nil {
		wire.Ebgp = json.RawMessage(sdnBoolJSON(*s.Ebgp))
	}
	return json.Marshal(wire)
}

// UnmarshalJSON decodes the controller response, splitting the wire
// comma-separated lists and accepting both spellings of the multipath flag.
func (s *SdnController) UnmarshalJSON(data []byte) error {
	var wire sdnControllerWire
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	*s = SdnController{
		Controller:    wire.Controller,
		Type:          wire.Type,
		ASN:           wire.ASN,
		BgpMode:       wire.BgpMode,
		EbgpMultihop:  wire.EbgpMultihop,
		Fabric:        wire.Fabric,
		IsisDomain:    wire.IsisDomain,
		IsisIfaces:    sdnSplitCommaList(wire.IsisIfaces),
		IsisNet:       wire.IsisNet,
		Loopback:      wire.Loopback,
		Node:          wire.Node,
		Nodes:         sdnSplitCommaList(wire.Nodes),
		PeerGroupName: wire.PeerGroupName,
		Peers:         sdnSplitCommaList(wire.Peers),
		RouteMapIn:    wire.RouteMapIn,
		RouteMapOut:   wire.RouteMapOut,
		Digest:        wire.Digest,
		State:         wire.State,
	}
	s.BgpMultipathAsPathRelax = decodeBoolishPtr(wire.BgpMultipathAsPathRelax)
	if s.BgpMultipathAsPathRelax == nil {
		s.BgpMultipathAsPathRelax = decodeBoolishPtr(wire.BgpMultipathAsRelax)
	}
	s.Ebgp = decodeBoolishPtr(wire.Ebgp)
	return nil
}

// sdnBoolJSON encodes a bool as a JSON literal.
func sdnBoolJSON(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// sdnSplitCommaList splits PVE's comma-separated wire strings, treating an
// empty string as an empty list.
func sdnSplitCommaList(in string) []string {
	if in == "" {
		return nil
	}
	return strings.Split(in, ",")
}

// SdnDns is an SDN reverse DNS plugin object (GET/POST /cluster/sdn/dns,
// GET/PUT/DELETE /cluster/sdn/dns/{dns}). The pin's only plugin type is
// powerdns.
type SdnDns struct {
	Dns           string `json:"dns"`
	Type          string `json:"type,omitempty"`
	Key           string `json:"key,omitempty"`
	URL           string `json:"url,omitempty"`
	Fingerprint   string `json:"fingerprint,omitempty"`
	Reversemaskv6 *int64 `json:"reversemaskv6,omitempty"`
	TTL           *int64 `json:"ttl,omitempty"`
}

// SdnIpam is an SDN IPAM plugin object (GET/POST /cluster/sdn/ipams,
// GET/PUT/DELETE /cluster/sdn/ipams/{ipam}). Type is one of the pin's
// plugin types: netbox | phpipam | pve.
type SdnIpam struct {
	Ipam        string `json:"ipam"`
	Type        string `json:"type,omitempty"`
	URL         string `json:"url,omitempty"`
	Token       string `json:"token,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
	Section     *int64 `json:"section,omitempty"`
}

// SdnMacVrfEntry is one route from a vnet's MAC VRF
// (GET /nodes/{node}/sdn/vnets/{vnet}/mac-vrf, EVPN zones).
type SdnMacVrfEntry struct {
	IP      string `json:"ip"`
	MAC     string `json:"mac"`
	Nexthop string `json:"nexthop"`
}

// ListSdnVnets returns the vnets from GET /cluster/sdn/vnets.
func (c *Client) ListSdnVnets(ctx context.Context) ([]SdnVnet, error) {
	var out []SdnVnet
	if err := c.Do(ctx, "GET", "/cluster/sdn/vnets", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// CreateSdnVnet POSTs /cluster/sdn/vnets with the supplied vnet, fixing the
// pin's `type` to `vnet`.
func (c *Client) CreateSdnVnet(ctx context.Context, vnet SdnVnet) error {
	vnet.Type = "vnet"
	return c.Do(ctx, "POST", "/cluster/sdn/vnets", vnet, nil)
}

// GetSdnVnet reads /cluster/sdn/vnets/{vnet}.
func (c *Client) GetSdnVnet(ctx context.Context, vnet string) (*SdnVnet, error) {
	var out SdnVnet
	if err := c.Do(ctx, "GET", fmt.Sprintf("/cluster/sdn/vnets/%s", vnet), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateSdnVnet PUTs /cluster/sdn/vnets/{vnet} with the supplied body and
// translates the delete slice into PVE's `delete` query parameter.
func (c *Client) UpdateSdnVnet(ctx context.Context, vnet string, body SdnVnet, deleteFields []string) error {
	path := haDeleteQuery(fmt.Sprintf("/cluster/sdn/vnets/%s", vnet), deleteFields)
	return c.Do(ctx, "PUT", path, body, nil)
}

// DeleteSdnVnet DELETEs /cluster/sdn/vnets/{vnet}.
func (c *Client) DeleteSdnVnet(ctx context.Context, vnet string) error {
	return c.Do(ctx, "DELETE", fmt.Sprintf("/cluster/sdn/vnets/%s", vnet), nil, nil)
}

// GetSdnVnetMacVrf reads /nodes/{node}/sdn/vnets/{vnet}/mac-vrf, the
// per-node MAC VRF routes of an EVPN-attached vnet.
func (c *Client) GetSdnVnetMacVrf(ctx context.Context, node, vnet string) ([]SdnMacVrfEntry, error) {
	var out []SdnMacVrfEntry
	path := fmt.Sprintf("/nodes/%s/sdn/vnets/%s/mac-vrf", node, vnet)
	if err := c.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ListSdnSubnets returns the subnets of a vnet from
// GET /cluster/sdn/vnets/{vnet}/subnets.
func (c *Client) ListSdnSubnets(ctx context.Context, vnet string) ([]SdnSubnet, error) {
	var out []SdnSubnet
	if err := c.Do(ctx, "GET", fmt.Sprintf("/cluster/sdn/vnets/%s/subnets", vnet), nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// CreateSdnSubnet POSTs /cluster/sdn/vnets/{vnet}/subnets with the supplied
// subnet, fixing the pin's `type` to `subnet`.
func (c *Client) CreateSdnSubnet(ctx context.Context, vnet string, subnet SdnSubnet) error {
	subnet.Type = "subnet"
	subnet.Vnet = vnet
	return c.Do(ctx, "POST", fmt.Sprintf("/cluster/sdn/vnets/%s/subnets", vnet), subnet, nil)
}

// GetSdnSubnet reads /cluster/sdn/vnets/{vnet}/subnets/{subnet}.
func (c *Client) GetSdnSubnet(ctx context.Context, vnet, subnet string) (*SdnSubnet, error) {
	var out SdnSubnet
	path := fmt.Sprintf("/cluster/sdn/vnets/%s/subnets/%s", vnet, subnet)
	if err := c.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateSdnSubnet PUTs /cluster/sdn/vnets/{vnet}/subnets/{subnet} with the
// supplied body and translates the delete slice into PVE's `delete` query
// parameter.
func (c *Client) UpdateSdnSubnet(ctx context.Context, vnet, subnet string, body SdnSubnet, deleteFields []string) error {
	path := haDeleteQuery(fmt.Sprintf("/cluster/sdn/vnets/%s/subnets/%s", vnet, subnet), deleteFields)
	return c.Do(ctx, "PUT", path, body, nil)
}

// DeleteSdnSubnet DELETEs /cluster/sdn/vnets/{vnet}/subnets/{subnet}.
func (c *Client) DeleteSdnSubnet(ctx context.Context, vnet, subnet string) error {
	return c.Do(ctx, "DELETE", fmt.Sprintf("/cluster/sdn/vnets/%s/subnets/%s", vnet, subnet), nil, nil)
}

// ListSdnControllers returns the controllers from
// GET /cluster/sdn/controllers. controllerType optionally filters the list
// (bgp | evpn | faucet | isis).
func (c *Client) ListSdnControllers(ctx context.Context, controllerType string) ([]SdnController, error) {
	var out []SdnController
	path := "/cluster/sdn/controllers"
	if controllerType != "" {
		path += "?type=" + controllerType
	}
	if err := c.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// CreateSdnController POSTs /cluster/sdn/controllers with the supplied
// controller.
func (c *Client) CreateSdnController(ctx context.Context, controller SdnController) error {
	return c.Do(ctx, "POST", "/cluster/sdn/controllers", controller, nil)
}

// GetSdnController reads /cluster/sdn/controllers/{controller}.
func (c *Client) GetSdnController(ctx context.Context, controller string) (*SdnController, error) {
	var out SdnController
	if err := c.Do(ctx, "GET", fmt.Sprintf("/cluster/sdn/controllers/%s", controller), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateSdnController PUTs /cluster/sdn/controllers/{controller} with the
// supplied body and translates the delete slice into PVE's `delete` query
// parameter.
func (c *Client) UpdateSdnController(ctx context.Context, controller string, body SdnController, deleteFields []string) error {
	path := haDeleteQuery(fmt.Sprintf("/cluster/sdn/controllers/%s", controller), deleteFields)
	return c.Do(ctx, "PUT", path, body, nil)
}

// DeleteSdnController DELETEs /cluster/sdn/controllers/{controller}.
func (c *Client) DeleteSdnController(ctx context.Context, controller string) error {
	return c.Do(ctx, "DELETE", fmt.Sprintf("/cluster/sdn/controllers/%s", controller), nil, nil)
}

// ListSdnDns returns the DNS plugins from GET /cluster/sdn/dns. dnsType
// optionally filters the list (the pin's only type is powerdns).
func (c *Client) ListSdnDns(ctx context.Context, dnsType string) ([]SdnDns, error) {
	var out []SdnDns
	path := "/cluster/sdn/dns"
	if dnsType != "" {
		path += "?type=" + dnsType
	}
	if err := c.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// CreateSdnDns POSTs /cluster/sdn/dns with the supplied plugin, fixing the
// pin's `type` to `powerdns`.
func (c *Client) CreateSdnDns(ctx context.Context, dns SdnDns) error {
	if dns.Type == "" {
		dns.Type = "powerdns"
	}
	return c.Do(ctx, "POST", "/cluster/sdn/dns", dns, nil)
}

// GetSdnDns reads /cluster/sdn/dns/{dns}.
func (c *Client) GetSdnDns(ctx context.Context, dns string) (*SdnDns, error) {
	var out SdnDns
	if err := c.Do(ctx, "GET", fmt.Sprintf("/cluster/sdn/dns/%s", dns), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateSdnDns PUTs /cluster/sdn/dns/{dns} with the supplied body and
// translates the delete slice into PVE's `delete` query parameter.
func (c *Client) UpdateSdnDns(ctx context.Context, dns string, body SdnDns, deleteFields []string) error {
	path := haDeleteQuery(fmt.Sprintf("/cluster/sdn/dns/%s", dns), deleteFields)
	return c.Do(ctx, "PUT", path, body, nil)
}

// DeleteSdnDns DELETEs /cluster/sdn/dns/{dns}.
func (c *Client) DeleteSdnDns(ctx context.Context, dns string) error {
	return c.Do(ctx, "DELETE", fmt.Sprintf("/cluster/sdn/dns/%s", dns), nil, nil)
}

// ListSdnIpams returns the IPAM plugins from GET /cluster/sdn/ipams.
// ipamType optionally filters the list (netbox | phpipam | pve).
func (c *Client) ListSdnIpams(ctx context.Context, ipamType string) ([]SdnIpam, error) {
	var out []SdnIpam
	path := "/cluster/sdn/ipams"
	if ipamType != "" {
		path += "?type=" + ipamType
	}
	if err := c.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// CreateSdnIpam POSTs /cluster/sdn/ipams with the supplied plugin.
func (c *Client) CreateSdnIpam(ctx context.Context, ipam SdnIpam) error {
	return c.Do(ctx, "POST", "/cluster/sdn/ipams", ipam, nil)
}

// GetSdnIpam reads /cluster/sdn/ipams/{ipam}.
func (c *Client) GetSdnIpam(ctx context.Context, ipam string) (*SdnIpam, error) {
	var out SdnIpam
	if err := c.Do(ctx, "GET", fmt.Sprintf("/cluster/sdn/ipams/%s", ipam), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateSdnIpam PUTs /cluster/sdn/ipams/{ipam} with the supplied body and
// translates the delete slice into PVE's `delete` query parameter.
func (c *Client) UpdateSdnIpam(ctx context.Context, ipam string, body SdnIpam, deleteFields []string) error {
	path := haDeleteQuery(fmt.Sprintf("/cluster/sdn/ipams/%s", ipam), deleteFields)
	return c.Do(ctx, "PUT", path, body, nil)
}

// DeleteSdnIpam DELETEs /cluster/sdn/ipams/{ipam}.
func (c *Client) DeleteSdnIpam(ctx context.Context, ipam string) error {
	return c.Do(ctx, "DELETE", fmt.Sprintf("/cluster/sdn/ipams/%s", ipam), nil, nil)
}

// ListSdnIpamStatus reads /cluster/sdn/ipams/{ipam}/status, the IPAM entry
// index. The pin declares the response items as untyped objects, so each
// entry is returned as its raw JSON string.
func (c *Client) ListSdnIpamStatus(ctx context.Context, ipam string) ([]string, error) {
	var out []json.RawMessage
	if err := c.Do(ctx, "GET", fmt.Sprintf("/cluster/sdn/ipams/%s/status", ipam), nil, &out); err != nil {
		return nil, err
	}
	entries := make([]string, 0, len(out))
	for _, raw := range out {
		entries = append(entries, string(raw))
	}
	return entries, nil
}
