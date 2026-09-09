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

// TestFirewallRules_ClusterCRUD covers the cluster ruleset wire set:
// list decode with boolish enable and the hyphenated icmp-type key,
// create body with the pin's 0/1 enable encoding, single-rule read,
// update with delete query, and delete.
func TestFirewallRules_ClusterCRUD(t *testing.T) {
	var lastBody []byte
	var lastQuery string
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		lastBody, _ = io.ReadAll(r.Body)
		lastQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/cluster/firewall/rules":
			_, _ = io.WriteString(w, `{"data":[`+
				`{"pos":0,"type":"in","action":"ACCEPT","enable":1,"proto":"tcp","dport":"22","source":"10.0.0.0/8","icmp-type":"any","ipversion":4,"comment":"ssh"},`+
				`{"pos":1,"type":"out","action":"DROP","enable":true,"log":"debug","macro":"SMTP"}]}`)
		case r.Method == http.MethodPost && r.URL.Path == "/cluster/firewall/rules":
			_, _ = io.WriteString(w, `{"data":null}`)
		case r.Method == http.MethodGet && r.URL.Path == "/cluster/firewall/rules/0":
			_, _ = io.WriteString(w, `{"data":{"pos":0,"type":"in","action":"ACCEPT","enable":0,"iface":"vmbr0"}}`)
		case r.Method == http.MethodPut && r.URL.Path == "/cluster/firewall/rules/1":
			_, _ = io.WriteString(w, `{"data":null}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/cluster/firewall/rules/1":
			_, _ = io.WriteString(w, `{"data":null}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	ctx := context.Background()

	rules, err := c.ListFirewallRules(ctx, FirewallRulesPathCluster())
	if err != nil {
		t.Fatalf("ListFirewallRules: %v", err)
	}
	if len(rules) != 2 {
		t.Fatalf("rules = %+v, want 2 entries", rules)
	}
	first, second := rules[0], rules[1]
	if first.Pos == nil || *first.Pos != 0 || second.Pos == nil || *second.Pos != 1 {
		t.Fatalf("positions = %+v", rules)
	}
	if first.Enable == nil || !*first.Enable || second.Enable == nil || !*second.Enable {
		t.Fatalf("boolish enable decode = %+v", rules)
	}
	if first.ICMPType != "any" || first.IPVersion == nil || *first.IPVersion != 4 {
		t.Fatalf("hyphenated/lenient fields = %+v", first)
	}
	if second.Log != "debug" || second.Macro != "SMTP" || second.Type != "out" {
		t.Fatalf("second rule = %+v", second)
	}

	create := FirewallRule{
		Type: "in", Action: "ACCEPT",
		Enable: FirewallRuleBoolPtr(true),
		Proto:  "tcp", DPort: "22", Source: "10.0.0.0/8",
		Comment: "ssh",
	}
	if err := c.CreateFirewallRule(ctx, FirewallRulesPathCluster(), create); err != nil {
		t.Fatalf("CreateFirewallRule: %v", err)
	}
	var sent map[string]any
	if err := json.Unmarshal(lastBody, &sent); err != nil {
		t.Fatalf("create body %q is not JSON: %v", lastBody, err)
	}
	if sent["enable"] != float64(1) || sent["type"] != "in" || sent["action"] != "ACCEPT" || sent["dport"] != "22" {
		t.Fatalf("create body = %v (enable must be the pin's 0/1 integer)", sent)
	}
	if _, ok := sent["pos"]; ok {
		t.Fatalf("create body must not carry pos when unset: %v", sent)
	}

	read, err := c.GetFirewallRule(ctx, FirewallRulesPathCluster(), 0)
	if err != nil {
		t.Fatalf("GetFirewallRule: %v", err)
	}
	if read.Enable == nil || *read.Enable || read.IFace != "vmbr0" {
		t.Fatalf("read rule = %+v", read)
	}

	update := FirewallRule{Type: "out", Action: "DROP", Log: "nolog"}
	if err := c.UpdateFirewallRule(ctx, FirewallRulesPathCluster(), 1, update, []string{"macro", "comment"}); err != nil {
		t.Fatalf("UpdateFirewallRule: %v", err)
	}
	if !strings.Contains(lastQuery, "delete=macro%2Ccomment") {
		t.Fatalf("update query = %q", lastQuery)
	}
	if err := c.DeleteFirewallRule(ctx, FirewallRulesPathCluster(), 1); err != nil {
		t.Fatalf("DeleteFirewallRule: %v", err)
	}
}

// TestFirewallRules_ScopePaths parametrizes the five ruleset scope shapes
// against the exact paths the pin defines.
func TestFirewallRules_ScopePaths(t *testing.T) {
	cases := []struct {
		name     string
		basePath string
		list     func(*Client, context.Context) ([]FirewallRule, error)
	}{
		{"cluster", "/cluster/firewall/rules", func(c *Client, ctx context.Context) ([]FirewallRule, error) {
			return c.ListFirewallRules(ctx, FirewallRulesPathCluster())
		}},
		{"node", "/nodes/pve1/firewall/rules", func(c *Client, ctx context.Context) ([]FirewallRule, error) {
			return c.ListFirewallRules(ctx, FirewallRulesPathNode("pve1"))
		}},
		{"guest-qemu", "/nodes/pve1/qemu/100/firewall/rules", func(c *Client, ctx context.Context) ([]FirewallRule, error) {
			return c.ListFirewallRules(ctx, FirewallRulesPathGuest("pve1", "qemu", 100))
		}},
		{"guest-lxc", "/nodes/pve1/lxc/101/firewall/rules", func(c *Client, ctx context.Context) ([]FirewallRule, error) {
			return c.ListFirewallRules(ctx, FirewallRulesPathGuest("pve1", "lxc", 101))
		}},
		{"security-group", "/cluster/firewall/groups/web/rules", func(c *Client, ctx context.Context) ([]FirewallRule, error) {
			return c.ListFirewallRules(ctx, FirewallRulesPathSecurityGroup("web"))
		}},
		{"vnet", "/cluster/sdn/vnets/vnet0/firewall/rules", func(c *Client, ctx context.Context) ([]FirewallRule, error) {
			return c.ListFirewallRules(ctx, FirewallRulesPathVnet("vnet0"))
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var seen string
			c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
				seen = r.Method + " " + r.URL.Path
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"data":[]}`)
			})
			if _, err := tc.list(c, context.Background()); err != nil {
				t.Fatalf("list: %v", err)
			}
			if seen != "GET "+tc.basePath {
				t.Fatalf("wire request = %q, want GET %s", seen, tc.basePath)
			}
		})
	}
}

// TestFirewallRules_GetMissing404IsAPIError confirms a missing rule
// surfaces as a 404 *APIError so the resource layer can react.
func TestFirewallRules_GetMissing404IsAPIError(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/nodes/pve1/firewall/rules/9" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"errors":"no rule at position 9"}`)
	})
	_, err := c.GetFirewallRule(context.Background(), FirewallRulesPathNode("pve1"), 9)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 APIError, got %v", err)
	}
}
