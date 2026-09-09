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

// TestFirewall_Aliases_CRUD covers the alias wire set: create body, list and
// single-read decode, update body carrying the clear-to-empty comment, and
// delete.
func TestFirewall_Aliases_CRUD(t *testing.T) {
	var lastBody []byte
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		lastBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/cluster/firewall/aliases":
			_, _ = io.WriteString(w, `{"data":null}`)
		case r.Method == http.MethodGet && r.URL.Path == "/cluster/firewall/aliases":
			_, _ = io.WriteString(w, `{"data":[{"name":"office","cidr":"203.0.113.0/24","comment":"HQ"},{"name":"lab","cidr":"198.51.100.0/24","digest":"d1"}]}`)
		case r.Method == http.MethodGet && r.URL.Path == "/cluster/firewall/aliases/office":
			_, _ = io.WriteString(w, `{"data":{"name":"office","cidr":"203.0.113.0/24","comment":"HQ","digest":"d2"}}`)
		case r.Method == http.MethodPut && r.URL.Path == "/cluster/firewall/aliases/office":
			_, _ = io.WriteString(w, `{"data":null}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/cluster/firewall/aliases/office":
			_, _ = io.WriteString(w, `{"data":null}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	ctx := context.Background()

	if err := c.CreateFirewallAlias(ctx, FirewallAlias{Name: "office", Cidr: "203.0.113.0/24", Comment: "HQ"}); err != nil {
		t.Fatalf("CreateFirewallAlias: %v", err)
	}
	var sent map[string]any
	if err := json.Unmarshal(lastBody, &sent); err != nil {
		t.Fatalf("create body %q is not JSON: %v", lastBody, err)
	}
	if sent["name"] != "office" || sent["cidr"] != "203.0.113.0/24" || sent["comment"] != "HQ" {
		t.Fatalf("create body = %v", sent)
	}

	list, err := c.ListFirewallAliases(ctx)
	if err != nil {
		t.Fatalf("ListFirewallAliases: %v", err)
	}
	if len(list) != 2 || list[0].Name != "office" || list[0].Cidr != "203.0.113.0/24" || list[0].Comment != "HQ" {
		t.Fatalf("list = %+v", list)
	}
	if list[1].Comment != "" {
		t.Fatalf("absent comment must decode empty, got %q", list[1].Comment)
	}

	alias, err := c.GetFirewallAlias(ctx, "office")
	if err != nil {
		t.Fatalf("GetFirewallAlias: %v", err)
	}
	if alias.Cidr != "203.0.113.0/24" || alias.Comment != "HQ" || alias.Digest != "d2" {
		t.Fatalf("read alias = %+v", alias)
	}

	// Update always emits comment so an empty string clears it upstream.
	if err := c.UpdateFirewallAlias(ctx, "office", FirewallAlias{Name: "office", Cidr: "203.0.113.0/24", Comment: ""}); err != nil {
		t.Fatalf("UpdateFirewallAlias: %v", err)
	}
	if err := json.Unmarshal(lastBody, &sent); err != nil {
		t.Fatalf("update body %q is not JSON: %v", lastBody, err)
	}
	raw, err := json.Marshal(sent)
	if err != nil {
		t.Fatalf("re-marshal: %v", err)
	}
	comment, ok := sent["comment"]
	if !ok || comment != "" {
		t.Fatalf("update body must carry explicit empty comment, got %s", raw)
	}

	if err := c.DeleteFirewallAlias(ctx, "office"); err != nil {
		t.Fatalf("DeleteFirewallAlias: %v", err)
	}
}

// TestFirewall_Alias_404IsAPIError confirms a missing alias surfaces as a
// 404 *APIError so the resource layer can treat already-absent as success.
func TestFirewall_Alias_404IsAPIError(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/cluster/firewall/aliases/gone" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"errors":"no such alias"}`)
	})
	_, err := c.GetFirewallAlias(context.Background(), "gone")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 APIError, got %v", err)
	}
}

// TestFirewall_Ipsets_CRUD covers the ipset wire set: create body, the
// update-by-create rename form, list decode, member add/list/remove with the
// slash-escaped CIDR path, and forced delete.
func TestFirewall_Ipsets_CRUD(t *testing.T) {
	var lastBody []byte
	var lastURI string
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		lastBody, _ = io.ReadAll(r.Body)
		lastURI = r.RequestURI
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/cluster/firewall/ipset":
			_, _ = io.WriteString(w, `{"data":null}`)
		case r.Method == http.MethodGet && r.URL.Path == "/cluster/firewall/ipset":
			_, _ = io.WriteString(w, `{"data":[{"name":"mgmt","comment":"Admin hosts","digest":"d1"}]}`)
		case r.Method == http.MethodGet && r.URL.Path == "/cluster/firewall/ipset/mgmt":
			_, _ = io.WriteString(w, `{"data":[{"cidr":"10.0.0.1"},{"cidr":"192.168.1.0/24","comment":"lan","nomatch":1}]}`)
		case r.Method == http.MethodPost && r.URL.Path == "/cluster/firewall/ipset/mgmt":
			_, _ = io.WriteString(w, `{"data":null}`)
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/cluster/firewall/ipset/"):
			_, _ = io.WriteString(w, `{"data":null}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	ctx := context.Background()

	if err := c.CreateFirewallIpset(ctx, FirewallIpset{Name: "mgmt", Comment: "Admin hosts"}); err != nil {
		t.Fatalf("CreateFirewallIpset: %v", err)
	}
	var sent map[string]any
	if err := json.Unmarshal(lastBody, &sent); err != nil {
		t.Fatalf("create body %q is not JSON: %v", lastBody, err)
	}
	if sent["name"] != "mgmt" || sent["comment"] != "Admin hosts" {
		t.Fatalf("create body = %v", sent)
	}

	sets, err := c.ListFirewallIpsets(ctx)
	if err != nil {
		t.Fatalf("ListFirewallIpsets: %v", err)
	}
	if len(sets) != 1 || sets[0].Name != "mgmt" || sets[0].Comment != "Admin hosts" {
		t.Fatalf("list = %+v", sets)
	}

	members, err := c.ListFirewallIpsetMembers(ctx, "mgmt")
	if err != nil {
		t.Fatalf("ListFirewallIpsetMembers: %v", err)
	}
	if len(members) != 2 || members[0].Cidr != "10.0.0.1" || members[1].Cidr != "192.168.1.0/24" {
		t.Fatalf("members = %+v", members)
	}

	if err := c.AddFirewallIpsetMember(ctx, "mgmt", "10.0.0.1"); err != nil {
		t.Fatalf("AddFirewallIpsetMember: %v", err)
	}
	sent = nil
	if err := json.Unmarshal(lastBody, &sent); err != nil {
		t.Fatalf("add-member body %q is not JSON: %v", lastBody, err)
	}
	if sent["cidr"] != "10.0.0.1" {
		t.Fatalf("add-member body = %v", sent)
	}

	if err := c.RemoveFirewallIpsetMember(ctx, "mgmt", "192.168.1.0/24"); err != nil {
		t.Fatalf("RemoveFirewallIpsetMember: %v", err)
	}
	// The CIDR's slash must travel escaped so PVE still sees one path segment.
	if !strings.Contains(lastURI, "192.168.1.0%2F24") {
		t.Fatalf("remove URI = %q, want escaped slash", lastURI)
	}

	if err := c.UpdateFirewallIpset(ctx, "mgmt", ""); err != nil {
		t.Fatalf("UpdateFirewallIpset: %v", err)
	}
	sent = nil
	if err := json.Unmarshal(lastBody, &sent); err != nil {
		t.Fatalf("update body %q is not JSON: %v", lastBody, err)
	}
	if sent["rename"] != "mgmt" || sent["name"] != "mgmt" || sent["comment"] != "" {
		t.Fatalf("update body = %v (rename form must carry explicit empty comment)", sent)
	}

	if err := c.DeleteFirewallIpset(ctx, "mgmt"); err != nil {
		t.Fatalf("DeleteFirewallIpset: %v", err)
	}
	if !strings.Contains(lastURI, "force=true") {
		t.Fatalf("delete URI = %q, want force=true", lastURI)
	}
}

// TestFirewall_SecurityGroups_CRUD covers the security-group wire set:
// create body, list decode, the update-by-create rename form, and delete.
func TestFirewall_SecurityGroups_CRUD(t *testing.T) {
	var lastBody []byte
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		lastBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/cluster/firewall/groups":
			_, _ = io.WriteString(w, `{"data":null}`)
		case r.Method == http.MethodGet && r.URL.Path == "/cluster/firewall/groups":
			_, _ = io.WriteString(w, `{"data":[{"group":"web","comment":"Web rules","digest":"d1"}]}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/cluster/firewall/groups/web":
			_, _ = io.WriteString(w, `{"data":null}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	ctx := context.Background()

	if err := c.CreateFirewallSecurityGroup(ctx, FirewallSecurityGroup{Group: "web", Comment: "Web rules"}); err != nil {
		t.Fatalf("CreateFirewallSecurityGroup: %v", err)
	}
	var sent map[string]any
	if err := json.Unmarshal(lastBody, &sent); err != nil {
		t.Fatalf("create body %q is not JSON: %v", lastBody, err)
	}
	if sent["group"] != "web" || sent["comment"] != "Web rules" {
		t.Fatalf("create body = %v", sent)
	}

	groups, err := c.ListFirewallSecurityGroups(ctx)
	if err != nil {
		t.Fatalf("ListFirewallSecurityGroups: %v", err)
	}
	if len(groups) != 1 || groups[0].Group != "web" || groups[0].Comment != "Web rules" {
		t.Fatalf("list = %+v", groups)
	}

	if err := c.UpdateFirewallSecurityGroup(ctx, FirewallSecurityGroup{Group: "web", Rename: "web", Comment: ""}); err != nil {
		t.Fatalf("UpdateFirewallSecurityGroup: %v", err)
	}
	sent = nil
	if err := json.Unmarshal(lastBody, &sent); err != nil {
		t.Fatalf("update body %q is not JSON: %v", lastBody, err)
	}
	if sent["group"] != "web" || sent["rename"] != "web" || sent["comment"] != "" {
		t.Fatalf("update body = %v (rename form must carry explicit empty comment)", sent)
	}

	if err := c.DeleteFirewallSecurityGroup(ctx, "web"); err != nil {
		t.Fatalf("DeleteFirewallSecurityGroup: %v", err)
	}
}
