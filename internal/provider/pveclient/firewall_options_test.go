// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

// TestFirewallOptions_ClusterCRUD covers the cluster scope wire set:
// GET decode with boolish ebtables and integer enable, PUT body keys, and
// the delete query parameter for cleared settings.
func TestFirewallOptions_ClusterCRUD(t *testing.T) {
	var lastBody []byte
	var lastQuery string
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		lastBody, _ = io.ReadAll(r.Body)
		lastQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/cluster/firewall/options":
			_, _ = io.WriteString(w, `{"data":{"ebtables":1,"enable":1,"log_ratelimit":"enable=1,burst=5,rate=1/second","policy_forward":"DROP","policy_in":"ACCEPT","policy_out":"ACCEPT"}}`)
		case r.Method == http.MethodPut && r.URL.Path == "/cluster/firewall/options":
			_, _ = io.WriteString(w, `{"data":null}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	ctx := context.Background()

	opts, err := c.GetClusterFirewallOptions(ctx)
	if err != nil {
		t.Fatalf("GetClusterFirewallOptions: %v", err)
	}
	if opts.Ebitables == nil || !*opts.Ebitables || opts.Enable == nil || *opts.Enable != 1 {
		t.Fatalf("cluster options decode = %+v", opts)
	}
	if opts.LogRateLimit == nil || !strings.Contains(*opts.LogRateLimit, "burst=5") {
		t.Fatalf("log_ratelimit = %+v", opts.LogRateLimit)
	}
	if opts.PolicyForward == nil || *opts.PolicyForward != "DROP" || opts.PolicyIn == nil || *opts.PolicyIn != "ACCEPT" {
		t.Fatalf("policies = %+v", opts)
	}

	update := ClusterFirewallOptions{
		Ebitables:     FirewallBoolPtr(false),
		Enable:        FirewallInt64Ptr(0),
		PolicyForward: FirewallStrPtr("ACCEPT"),
	}
	if err := c.UpdateClusterFirewallOptions(ctx, update, []string{"log_ratelimit", "policy_in"}); err != nil {
		t.Fatalf("UpdateClusterFirewallOptions: %v", err)
	}
	if !strings.HasPrefix(lastQuery, "delete=") || !strings.Contains(lastQuery, "log_ratelimit") || !strings.Contains(lastQuery, "policy_in") {
		t.Fatalf("update query = %q", lastQuery)
	}
	var sent map[string]any
	if err := json.Unmarshal(lastBody, &sent); err != nil {
		t.Fatalf("update body %q is not JSON: %v", lastBody, err)
	}
	if sent["ebtables"] != false || sent["enable"] != float64(0) || sent["policy_forward"] != "ACCEPT" {
		t.Fatalf("update body = %v", sent)
	}
	if _, ok := sent["log_ratelimit"]; ok {
		t.Fatalf("cleared option must not travel in the body: %v", sent)
	}
}

// TestFirewallOptions_NodeCRUD covers the host scope wire set under
// /nodes/{node}/firewall/options: boolish and integer decode of the
// conntrack keys and the node key traveling in the PUT body.
func TestFirewallOptions_NodeCRUD(t *testing.T) {
	var lastBody []byte
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		lastBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/nodes/pve1/firewall/options":
			_, _ = io.WriteString(w, `{"data":{"enable":1,"log_level_in":"info","log_level_out":"nolog","log_nf_conntrack":0,"ndp":true,"nf_conntrack_max":262144,"nf_conntrack_tcp_timeout_syn_recv":45,"nftables":0,"nosmurfs":1,"protection_synflood":0,"tcpflags":1}}`)
		case r.Method == http.MethodPut && r.URL.Path == "/nodes/pve1/firewall/options":
			_, _ = io.WriteString(w, `{"data":null}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	ctx := context.Background()

	opts, err := c.GetNodeFirewallOptions(ctx, "pve1")
	if err != nil {
		t.Fatalf("GetNodeFirewallOptions: %v", err)
	}
	if opts.Enable == nil || !*opts.Enable || opts.LogLevelIn == nil || *opts.LogLevelIn != "info" {
		t.Fatalf("node options decode = %+v", opts)
	}
	if opts.LogNFConntrack == nil || *opts.LogNFConntrack || opts.Nftables == nil || *opts.Nftables {
		t.Fatalf("boolish flags = %+v", opts)
	}
	if opts.NFConntrackMax == nil || *opts.NFConntrackMax != 262144 {
		t.Fatalf("nf_conntrack_max = %+v", opts.NFConntrackMax)
	}
	if opts.NFConntrackTCPTimeoutSynRecv == nil || *opts.NFConntrackTCPTimeoutSynRecv != 45 {
		t.Fatalf("syn recv timeout = %+v", opts.NFConntrackTCPTimeoutSynRecv)
	}
	if !*opts.Nosmurfs || !*opts.TCPFlags || *opts.ProtectionSynflood {
		t.Fatalf("nosmurfs/smurf flags = %+v", opts)
	}

	update := FirewallOptions{
		Enable:     FirewallBoolPtr(false),
		LogLevelIn: FirewallStrPtr("debug"),
	}
	if err := c.UpdateNodeFirewallOptions(ctx, "pve1", update, []string{"log_level_out"}); err != nil {
		t.Fatalf("UpdateNodeFirewallOptions: %v", err)
	}
	var sent map[string]any
	if err := json.Unmarshal(lastBody, &sent); err != nil {
		t.Fatalf("update body %q is not JSON: %v", lastBody, err)
	}
	if sent["enable"] != false || sent["log_level_in"] != "debug" || sent["node"] != "pve1" {
		t.Fatalf("update body = %v", sent)
	}
}

// TestFirewallOptions_GuestCRUD covers both guest scopes (qemu and lxc)
// under /nodes/{node}/{qemu,lxc}/{vmid}/firewall/options and the node/vmid
// keys traveling in the PUT body.
func TestFirewallOptions_GuestCRUD(t *testing.T) {
	var lastPath, lastBody string
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		lastPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		lastBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && (r.URL.Path == "/nodes/pve1/qemu/100/firewall/options" || r.URL.Path == "/nodes/pve1/lxc/101/firewall/options"):
			_, _ = io.WriteString(w, `{"data":{"enable":1,"dhcp":0,"ipfilter":1,"macfilter":1,"ndp":1,"radv":0,"log_level_in":"emerg","policy_in":"DROP","policy_out":"REJECT"}}`)
		case r.Method == http.MethodPut && (r.URL.Path == "/nodes/pve1/qemu/100/firewall/options" || r.URL.Path == "/nodes/pve1/lxc/101/firewall/options"):
			_, _ = io.WriteString(w, `{"data":null}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	ctx := context.Background()

	vm, err := c.GetGuestFirewallOptions(ctx, "pve1", "qemu", 100)
	if err != nil {
		t.Fatalf("GetGuestFirewallOptions qemu: %v", err)
	}
	if vm.Enable == nil || !*vm.Enable || vm.DHCP == nil || *vm.DHCP || vm.MacFilter == nil || !*vm.MacFilter {
		t.Fatalf("qemu options decode = %+v", vm)
	}
	if vm.PolicyOut == nil || *vm.PolicyOut != "REJECT" {
		t.Fatalf("policy_out = %+v", vm.PolicyOut)
	}

	ct, err := c.GetGuestFirewallOptions(ctx, "pve1", "lxc", 101)
	if err != nil {
		t.Fatalf("GetGuestFirewallOptions lxc: %v", err)
	}
	if ct.IPFilter == nil || !*ct.IPFilter || ct.Radv == nil || *ct.Radv {
		t.Fatalf("lxc options decode = %+v", ct)
	}

	if err := c.UpdateGuestFirewallOptions(ctx, "pve1", "qemu", 100, FirewallOptions{
		Enable:     FirewallBoolPtr(true),
		LogLevelIn: FirewallStrPtr("debug"),
	}, []string{"dhcp"}); err != nil {
		t.Fatalf("UpdateGuestFirewallOptions: %v", err)
	}
	if lastPath != "/nodes/pve1/qemu/100/firewall/options" {
		t.Fatalf("update path = %q", lastPath)
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte(lastBody), &sent); err != nil {
		t.Fatalf("update body %q is not JSON: %v", lastBody, err)
	}
	if sent["enable"] != true || sent["node"] != "pve1" || sent["vmid"] != float64(100) {
		t.Fatalf("update body = %v", sent)
	}
}

// TestFirewallOptions_VNetCRUD covers the SDN vnet scope under
// /cluster/sdn/vnets/{vnet}/firewall/options.
func TestFirewallOptions_VNetCRUD(t *testing.T) {
	var lastBody []byte
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		lastBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/cluster/sdn/vnets/vnet1/firewall/options":
			_, _ = io.WriteString(w, `{"data":{"enable":1,"log_level_forward":"debug","policy_forward":"DROP"}}`)
		case r.Method == http.MethodPut && r.URL.Path == "/cluster/sdn/vnets/vnet1/firewall/options":
			_, _ = io.WriteString(w, `{"data":null}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	ctx := context.Background()

	opts, err := c.GetVNetFirewallOptions(ctx, "vnet1")
	if err != nil {
		t.Fatalf("GetVNetFirewallOptions: %v", err)
	}
	if opts.Enable == nil || !*opts.Enable || opts.LogLevelForward == nil || *opts.LogLevelForward != "debug" || opts.PolicyForward == nil || *opts.PolicyForward != "DROP" {
		t.Fatalf("vnet options decode = %+v", opts)
	}

	if err := c.UpdateVNetFirewallOptions(ctx, "vnet1", FirewallOptions{
		Enable: FirewallBoolPtr(false),
	}, []string{"policy_forward"}); err != nil {
		t.Fatalf("UpdateVNetFirewallOptions: %v", err)
	}
	var sent map[string]any
	if err := json.Unmarshal(lastBody, &sent); err != nil {
		t.Fatalf("update body %q is not JSON: %v", lastBody, err)
	}
	if sent["enable"] != false || sent["vnet"] != "vnet1" {
		t.Fatalf("update body = %v", sent)
	}
}

// TestFirewallOptions_GetNode_404IsAPIError confirms a missing node or
// guest surfaces as a 404 *APIError so the resource layer can drop the
// singleton from state.
func TestFirewallOptions_GetNode_404IsAPIError(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"errors":"no such node"}`)
	})
	_, err := c.GetNodeFirewallOptions(context.Background(), "gone")
	if err == nil {
		t.Fatal("expected an error")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 APIError, got %v", err)
	}
}
