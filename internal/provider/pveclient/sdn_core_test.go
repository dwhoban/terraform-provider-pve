// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// TestSdn_Vnets_CRUD covers the vnet wire set: create body with the fixed
// vnet type and hyphenated keys, list decode, read decode, update with the
// delete query, delete, and the per-node mac-vrf runtime read.
func TestSdn_Vnets_CRUD(t *testing.T) {
	var lastBody []byte
	var lastQuery string
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		lastBody, _ = io.ReadAll(r.Body)
		lastQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/cluster/sdn/vnets":
			_, _ = io.WriteString(w, `{"data":null}`)
		case r.Method == http.MethodGet && r.URL.Path == "/cluster/sdn/vnets":
			_, _ = io.WriteString(w, `{"data":[{"vnet":"vnet1","type":"vnet","zone":"zone1","tag":10,"vlanaware":true,"state":"new"}]}`)
		case r.Method == http.MethodGet && r.URL.Path == "/cluster/sdn/vnets/vnet1":
			_, _ = io.WriteString(w, `{"data":{"vnet":"vnet1","type":"vnet","zone":"zone1","tag":10,"vlanaware":true,"isolate-ports":false,"alias":"guest net","digest":"d1","state":"changed"}}`)
		case r.Method == http.MethodPut && r.URL.Path == "/cluster/sdn/vnets/vnet1":
			_, _ = io.WriteString(w, `{"data":null}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/cluster/sdn/vnets/vnet1":
			_, _ = io.WriteString(w, `{"data":null}`)
		case r.Method == http.MethodGet && r.URL.Path == "/nodes/pve1/sdn/vnets/vnet1/mac-vrf":
			_, _ = io.WriteString(w, `{"data":[{"ip":"10.0.0.5","mac":"BC:24:11:00:00:01","nexthop":"10.0.0.1"}]}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	ctx := context.Background()

	vnet := SdnVnet{
		Vnet:         "vnet1",
		Zone:         "zone1",
		Alias:        "guest net",
		Tag:          HAInt64Ptr(10),
		VlanAware:    HABoolPtr(true),
		IsolatePorts: HABoolPtr(false),
	}
	if err := c.CreateSdnVnet(ctx, vnet); err != nil {
		t.Fatalf("CreateSdnVnet: %v", err)
	}
	var sent map[string]any
	if err := json.Unmarshal(lastBody, &sent); err != nil {
		t.Fatalf("create body %q is not JSON: %v", lastBody, err)
	}
	if sent["vnet"] != "vnet1" || sent["zone"] != "zone1" || sent["type"] != "vnet" || sent["tag"] != float64(10) || sent["isolate-ports"] != false {
		t.Fatalf("create body = %v", sent)
	}

	items, err := c.ListSdnVnets(ctx)
	if err != nil {
		t.Fatalf("ListSdnVnets: %v", err)
	}
	if len(items) != 1 || items[0].Vnet != "vnet1" || items[0].Zone != "zone1" || items[0].Tag == nil || *items[0].Tag != 10 || items[0].VlanAware == nil || !*items[0].VlanAware || items[0].State != "new" {
		t.Fatalf("ListSdnVnets = %+v", items)
	}

	got, err := c.GetSdnVnet(ctx, "vnet1")
	if err != nil {
		t.Fatalf("GetSdnVnet: %v", err)
	}
	if got.Alias != "guest net" || got.Digest != "d1" || got.State != "changed" || got.IsolatePorts == nil || *got.IsolatePorts {
		t.Fatalf("GetSdnVnet = %+v", got)
	}

	vnet.Alias = "renamed"
	vnet.IsolatePorts = nil
	if err := c.UpdateSdnVnet(ctx, "vnet1", vnet, []string{"isolate-ports"}); err != nil {
		t.Fatalf("UpdateSdnVnet: %v", err)
	}
	if lastQuery != "delete=isolate-ports" {
		t.Fatalf("update query = %q", lastQuery)
	}
	if err := json.Unmarshal(lastBody, &sent); err != nil {
		t.Fatalf("update body %q is not JSON: %v", lastBody, err)
	}
	if sent["alias"] != "renamed" {
		t.Fatalf("update body = %v", sent)
	}

	if err := c.DeleteSdnVnet(ctx, "vnet1"); err != nil {
		t.Fatalf("DeleteSdnVnet: %v", err)
	}

	entries, err := c.GetSdnVnetMacVrf(ctx, "pve1", "vnet1")
	if err != nil {
		t.Fatalf("GetSdnVnetMacVrf: %v", err)
	}
	if len(entries) != 1 || entries[0].IP != "10.0.0.5" || entries[0].MAC != "BC:24:11:00:00:01" || entries[0].Nexthop != "10.0.0.1" {
		t.Fatalf("mac-vrf = %+v", entries)
	}
}

// TestSdn_Subnets_CRUD covers the subnet wire set nested under a vnet:
// create body with the fixed subnet type, list and read decode, update with
// the delete query, and delete.
func TestSdn_Subnets_CRUD(t *testing.T) {
	var lastBody []byte
	var lastQuery string
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		lastBody, _ = io.ReadAll(r.Body)
		lastQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/cluster/sdn/vnets/vnet1/subnets":
			_, _ = io.WriteString(w, `{"data":null}`)
		case r.Method == http.MethodGet && r.URL.Path == "/cluster/sdn/vnets/vnet1/subnets":
			_, _ = io.WriteString(w, `{"data":[{"subnet":"10.0.0.0-24","type":"subnet","gateway":"10.0.0.1","snat":true}]}`)
		case r.Method == http.MethodGet && r.URL.Path == "/cluster/sdn/vnets/vnet1/subnets/10.0.0.0-24":
			_, _ = io.WriteString(w, `{"data":{"subnet":"10.0.0.0-24","type":"subnet","gateway":"10.0.0.1","snat":true,"dhcp-range":["10.0.0.100-10.0.0.200"],"dhcp-dns-server":"10.0.0.53","dnszoneprefix":"adm"}}`)
		case r.Method == http.MethodPut && r.URL.Path == "/cluster/sdn/vnets/vnet1/subnets/10.0.0.0-24":
			_, _ = io.WriteString(w, `{"data":null}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/cluster/sdn/vnets/vnet1/subnets/10.0.0.0-24":
			_, _ = io.WriteString(w, `{"data":null}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	ctx := context.Background()

	subnet := SdnSubnet{
		Subnet:        "10.0.0.0-24",
		Vnet:          "vnet1",
		Gateway:       "10.0.0.1",
		Snat:          HABoolPtr(true),
		DhcpDnsServer: "10.0.0.53",
		DhcpRange:     []string{"10.0.0.100-10.0.0.200"},
		Dnszoneprefix: "adm",
	}
	if err := c.CreateSdnSubnet(ctx, "vnet1", subnet); err != nil {
		t.Fatalf("CreateSdnSubnet: %v", err)
	}
	var sent map[string]any
	if err := json.Unmarshal(lastBody, &sent); err != nil {
		t.Fatalf("create body %q is not JSON: %v", lastBody, err)
	}
	if sent["subnet"] != "10.0.0.0-24" || sent["vnet"] != "vnet1" || sent["type"] != "subnet" || sent["snat"] != true {
		t.Fatalf("create body = %v", sent)
	}
	ranges, ok := sent["dhcp-range"].([]any)
	if !ok || len(ranges) != 1 || ranges[0] != "10.0.0.100-10.0.0.200" {
		t.Fatalf("create dhcp-range = %v", sent["dhcp-range"])
	}

	items, err := c.ListSdnSubnets(ctx, "vnet1")
	if err != nil {
		t.Fatalf("ListSdnSubnets: %v", err)
	}
	if len(items) != 1 || items[0].Subnet != "10.0.0.0-24" || items[0].Gateway != "10.0.0.1" || items[0].Snat == nil || !*items[0].Snat {
		t.Fatalf("ListSdnSubnets = %+v", items)
	}

	got, err := c.GetSdnSubnet(ctx, "vnet1", "10.0.0.0-24")
	if err != nil {
		t.Fatalf("GetSdnSubnet: %v", err)
	}
	if got.DhcpDnsServer != "10.0.0.53" || got.Dnszoneprefix != "adm" || len(got.DhcpRange) != 1 {
		t.Fatalf("GetSdnSubnet = %+v", got)
	}

	subnet.Dnszoneprefix = ""
	if err := c.UpdateSdnSubnet(ctx, "vnet1", "10.0.0.0-24", subnet, []string{"dnszoneprefix"}); err != nil {
		t.Fatalf("UpdateSdnSubnet: %v", err)
	}
	if lastQuery != "delete=dnszoneprefix" {
		t.Fatalf("update query = %q", lastQuery)
	}

	if err := c.DeleteSdnSubnet(ctx, "vnet1", "10.0.0.0-24"); err != nil {
		t.Fatalf("DeleteSdnSubnet: %v", err)
	}
}

// TestSdn_Controllers_CRUD covers the controller wire set: create body with
// the bgp type, list and read decode including the pin's read-only key
// `bgp-multipath-as-relax`, update with the delete query, and delete.
func TestSdn_Controllers_CRUD(t *testing.T) {
	var lastBody []byte
	var lastQuery string
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		lastBody, _ = io.ReadAll(r.Body)
		lastQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/cluster/sdn/controllers":
			_, _ = io.WriteString(w, `{"data":null}`)
		case r.Method == http.MethodGet && r.URL.Path == "/cluster/sdn/controllers":
			_, _ = io.WriteString(w, `{"data":[{"controller":"bgp1","type":"bgp","asn":65000,"peers":"10.0.0.1,10.0.0.2","bgp-multipath-as-relax":true}]}`)
		case r.Method == http.MethodGet && r.URL.Path == "/cluster/sdn/controllers/bgp1":
			_, _ = io.WriteString(w, `{"data":{"controller":"bgp1","type":"bgp","asn":65000,"bgp-mode":"internal","peers":"10.0.0.1,10.0.0.2","ebgp-multihop":3,"digest":"d1","state":"new"}}`)
		case r.Method == http.MethodPut && r.URL.Path == "/cluster/sdn/controllers/bgp1":
			_, _ = io.WriteString(w, `{"data":null}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/cluster/sdn/controllers/bgp1":
			_, _ = io.WriteString(w, `{"data":null}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	ctx := context.Background()

	ctl := SdnController{
		Controller:              "bgp1",
		Type:                    "bgp",
		ASN:                     HAInt64Ptr(65000),
		BgpMode:                 "internal",
		BgpMultipathAsPathRelax: HABoolPtr(true),
		Peers:                   []string{"10.0.0.1", "10.0.0.2"},
	}
	if err := c.CreateSdnController(ctx, ctl); err != nil {
		t.Fatalf("CreateSdnController: %v", err)
	}
	var sent map[string]any
	if err := json.Unmarshal(lastBody, &sent); err != nil {
		t.Fatalf("create body %q is not JSON: %v", lastBody, err)
	}
	if sent["controller"] != "bgp1" || sent["type"] != "bgp" || sent["asn"] != float64(65000) {
		t.Fatalf("create body = %v", sent)
	}
	if sent["peers"] != "10.0.0.1,10.0.0.2" || sent["bgp-multipath-as-path-relax"] != true {
		t.Fatalf("create controller wire list/bool keys = %v", sent)
	}

	items, err := c.ListSdnControllers(ctx, "")
	if err != nil {
		t.Fatalf("ListSdnControllers: %v", err)
	}
	if len(items) != 1 || items[0].Controller != "bgp1" || items[0].ASN == nil || *items[0].ASN != 65000 {
		t.Fatalf("ListSdnControllers = %+v", items)
	}
	if items[0].BgpMultipathAsPathRelax == nil || !*items[0].BgpMultipathAsPathRelax {
		t.Fatalf("read key bgp-multipath-as-relax not decoded: %+v", items[0])
	}
	if len(items[0].Peers) != 2 || items[0].Peers[0] != "10.0.0.1" {
		t.Fatalf("peers split = %+v", items[0].Peers)
	}

	got, err := c.GetSdnController(ctx, "bgp1")
	if err != nil {
		t.Fatalf("GetSdnController: %v", err)
	}
	if got.BgpMode != "internal" || got.EbgpMultihop == nil || *got.EbgpMultihop != 3 || got.Digest != "d1" {
		t.Fatalf("GetSdnController = %+v", got)
	}

	ctl.BgpMode = "external"
	if err := c.UpdateSdnController(ctx, "bgp1", ctl, []string{"ebgp-multihop"}); err != nil {
		t.Fatalf("UpdateSdnController: %v", err)
	}
	if lastQuery != "delete=ebgp-multihop" {
		t.Fatalf("update query = %q", lastQuery)
	}

	if err := c.DeleteSdnController(ctx, "bgp1"); err != nil {
		t.Fatalf("DeleteSdnController: %v", err)
	}
}

// TestSdn_Dns_CRUD covers the DNS wire set: create body with the fixed
// powerdns type and required key/url, read decode, update with the delete
// query, and delete.
func TestSdn_Dns_CRUD(t *testing.T) {
	var lastBody []byte
	var lastQuery string
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		lastBody, _ = io.ReadAll(r.Body)
		lastQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/cluster/sdn/dns":
			_, _ = io.WriteString(w, `{"data":null}`)
		case r.Method == http.MethodGet && r.URL.Path == "/cluster/sdn/dns":
			_, _ = io.WriteString(w, `{"data":[{"dns":"pdns1","type":"powerdns","url":"https://pdns.example:8081"}]}`)
		case r.Method == http.MethodGet && r.URL.Path == "/cluster/sdn/dns/pdns1":
			_, _ = io.WriteString(w, `{"data":{"dns":"pdns1","type":"powerdns","url":"https://pdns.example:8081","key":"secret","reversemaskv6":64,"ttl":60}}`)
		case r.Method == http.MethodPut && r.URL.Path == "/cluster/sdn/dns/pdns1":
			_, _ = io.WriteString(w, `{"data":null}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/cluster/sdn/dns/pdns1":
			_, _ = io.WriteString(w, `{"data":null}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	ctx := context.Background()

	dns := SdnDns{
		Dns:           "pdns1",
		Key:           "secret",
		URL:           "https://pdns.example:8081",
		Reversemaskv6: HAInt64Ptr(64),
		TTL:           HAInt64Ptr(60),
	}
	if err := c.CreateSdnDns(ctx, dns); err != nil {
		t.Fatalf("CreateSdnDns: %v", err)
	}
	var sent map[string]any
	if err := json.Unmarshal(lastBody, &sent); err != nil {
		t.Fatalf("create body %q is not JSON: %v", lastBody, err)
	}
	if sent["dns"] != "pdns1" || sent["type"] != "powerdns" || sent["key"] != "secret" || sent["url"] != "https://pdns.example:8081" {
		t.Fatalf("create body = %v", sent)
	}

	items, err := c.ListSdnDns(ctx, "")
	if err != nil {
		t.Fatalf("ListSdnDns: %v", err)
	}
	if len(items) != 1 || items[0].Dns != "pdns1" || items[0].Type != "powerdns" {
		t.Fatalf("ListSdnDns = %+v", items)
	}

	got, err := c.GetSdnDns(ctx, "pdns1")
	if err != nil {
		t.Fatalf("GetSdnDns: %v", err)
	}
	if got.Key != "secret" || got.Reversemaskv6 == nil || *got.Reversemaskv6 != 64 || got.TTL == nil || *got.TTL != 60 {
		t.Fatalf("GetSdnDns = %+v", got)
	}

	dns.TTL = nil
	if err := c.UpdateSdnDns(ctx, "pdns1", dns, []string{"ttl"}); err != nil {
		t.Fatalf("UpdateSdnDns: %v", err)
	}
	if lastQuery != "delete=ttl" {
		t.Fatalf("update query = %q", lastQuery)
	}

	if err := c.DeleteSdnDns(ctx, "pdns1"); err != nil {
		t.Fatalf("DeleteSdnDns: %v", err)
	}
}

// TestSdn_Ipams_CRUD covers the IPAM wire set: create body with the netbox
// type, read decode, update with the delete query, delete, and the opaque
// status listing decode.
func TestSdn_Ipams_CRUD(t *testing.T) {
	var lastBody []byte
	var lastQuery string
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		lastBody, _ = io.ReadAll(r.Body)
		lastQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/cluster/sdn/ipams":
			_, _ = io.WriteString(w, `{"data":null}`)
		case r.Method == http.MethodGet && r.URL.Path == "/cluster/sdn/ipams":
			_, _ = io.WriteString(w, `{"data":[{"ipam":"netbox1","type":"netbox","url":"https://netbox.example"}]}`)
		case r.Method == http.MethodGet && r.URL.Path == "/cluster/sdn/ipams/netbox1":
			_, _ = io.WriteString(w, `{"data":{"ipam":"netbox1","type":"netbox","url":"https://netbox.example","token":"tok","section":1}}`)
		case r.Method == http.MethodPut && r.URL.Path == "/cluster/sdn/ipams/netbox1":
			_, _ = io.WriteString(w, `{"data":null}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/cluster/sdn/ipams/netbox1":
			_, _ = io.WriteString(w, `{"data":null}`)
		case r.Method == http.MethodGet && r.URL.Path == "/cluster/sdn/ipams/netbox1/status":
			_, _ = io.WriteString(w, `{"data":[{"zone":"zone1","subnet":"10.0.0.0-24","info":["10.0.0.5"]}]}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	ctx := context.Background()

	ipam := SdnIpam{
		Ipam:        "netbox1",
		Type:        "netbox",
		URL:         "https://netbox.example",
		Token:       "tok",
		Fingerprint: "AA:BB",
		Section:     HAInt64Ptr(1),
	}
	if err := c.CreateSdnIpam(ctx, ipam); err != nil {
		t.Fatalf("CreateSdnIpam: %v", err)
	}
	var sent map[string]any
	if err := json.Unmarshal(lastBody, &sent); err != nil {
		t.Fatalf("create body %q is not JSON: %v", lastBody, err)
	}
	if sent["ipam"] != "netbox1" || sent["type"] != "netbox" || sent["token"] != "tok" || sent["section"] != float64(1) {
		t.Fatalf("create body = %v", sent)
	}

	items, err := c.ListSdnIpams(ctx, "netbox")
	if err != nil {
		t.Fatalf("ListSdnIpams: %v", err)
	}
	if len(items) != 1 || items[0].Ipam != "netbox1" || items[0].Type != "netbox" {
		t.Fatalf("ListSdnIpams = %+v", items)
	}

	got, err := c.GetSdnIpam(ctx, "netbox1")
	if err != nil {
		t.Fatalf("GetSdnIpam: %v", err)
	}
	if got.Token != "tok" || got.Section == nil || *got.Section != 1 {
		t.Fatalf("GetSdnIpam = %+v", got)
	}

	ipam.Token = ""
	if err := c.UpdateSdnIpam(ctx, "netbox1", ipam, []string{"token"}); err != nil {
		t.Fatalf("UpdateSdnIpam: %v", err)
	}
	if lastQuery != "delete=token" {
		t.Fatalf("update query = %q", lastQuery)
	}

	if err := c.DeleteSdnIpam(ctx, "netbox1"); err != nil {
		t.Fatalf("DeleteSdnIpam: %v", err)
	}

	status, err := c.ListSdnIpamStatus(ctx, "netbox1")
	if err != nil {
		t.Fatalf("ListSdnIpamStatus: %v", err)
	}
	if len(status) != 1 || !strings.Contains(status[0], `"zone":"zone1"`) || !strings.Contains(status[0], `"subnet":"10.0.0.0-24"`) {
		t.Fatalf("status = %v", status)
	}
}
