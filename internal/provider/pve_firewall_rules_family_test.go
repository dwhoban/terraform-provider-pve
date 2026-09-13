// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// firewallRulesTestClient spins up a fake PVE API served by h and returns
// a client pointed at it. Token auth avoids the /access/ticket exchange.
func firewallRulesTestClient(t *testing.T, h http.HandlerFunc) *pveclient.Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	client, err := pveclient.NewClient(pveclient.Credentials{
		Endpoint: srv.URL,
		Token:    "root@pam!test=00000000-0000-0000-0000-000000000000",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return client
}

type firewallRulesFakeServer struct {
	t *testing.T
	// basePath is the ruleset path requests must target.
	basePath string
	rules    []pveclient.FirewallRule
	// posShift is the offset added to index-derived positions; it
	// grows by posGrowth after every mutation, emulating an upstream
	// that renumbers aggressively. Clients must therefore re-list
	// between calls instead of trusting stale positions.
	posShift  int64
	posGrowth int64
	calls     []string
}

// handler serves the fake ruleset on the HTTP mux.
func (s *firewallRulesFakeServer) handler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.URL.Path != s.basePath && !strings.HasPrefix(r.URL.Path, s.basePath+"/") {
		s.t.Fatalf("unexpected path %q, want prefix %q", r.URL.Path, s.basePath)
	}
	switch r.Method {
	case http.MethodGet:
		var sb strings.Builder
		_, _ = sb.WriteString(`{"data":[`)
		for i, rule := range s.rules {
			if i > 0 {
				_, _ = sb.WriteString(",")
			}
			_, _ = sb.WriteString(`{"pos":` + strconv.FormatInt(int64(i)+s.posShift, 10) +
				`,"type":` + strconv.Quote(rule.Type) +
				`,"action":` + strconv.Quote(rule.Action) +
				`,"enable":1` +
				`,"comment":` + strconv.Quote(rule.Comment) + `}`)
		}
		_, _ = sb.WriteString(`]}`)
		_, _ = w.Write([]byte(sb.String()))
	case http.MethodPost:
		s.rules = append(s.rules, decodeFirewallRuleBody(s.t, r))
		s.calls = append(s.calls, "POST")
		_, _ = w.Write([]byte(`{"data":null}`))
	case http.MethodPut:
		pos, _ := strconv.ParseInt(strings.TrimPrefix(r.URL.Path, s.basePath+"/"), 10, 64)
		idx := int(pos - s.posShift)
		if idx < 0 || idx >= len(s.rules) {
			s.t.Fatalf("PUT at out-of-range pos %d", pos)
		}
		s.rules[idx] = decodeFirewallRuleBody(s.t, r)
		s.calls = append(s.calls, "PUT:"+strconv.FormatInt(pos, 10))
		_, _ = w.Write([]byte(`{"data":null}`))
	case http.MethodDelete:
		pos, _ := strconv.ParseInt(strings.TrimPrefix(r.URL.Path, s.basePath+"/"), 10, 64)
		idx := int(pos - s.posShift)
		if idx < 0 || idx >= len(s.rules) {
			s.t.Fatalf("DELETE at out-of-range pos %d", pos)
		}
		s.rules = append(s.rules[:idx], s.rules[idx+1:]...)
		s.calls = append(s.calls, "DEL:"+strconv.FormatInt(pos, 10))
		_, _ = w.Write([]byte(`{"data":null}`))
	}
	// Renumber after every mutation (never on reads): the next list
	// carries fresh positions.
	if r.Method != http.MethodGet {
		s.posShift += s.posGrowth
	}
}

// decodeFirewallRuleBody reads the request body into a wire rule,
// keeping only the fields the fake server models.
func decodeFirewallRuleBody(t *testing.T, r *http.Request) pveclient.FirewallRule {
	t.Helper()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("reading rule body: %v", err)
	}
	var wire struct {
		Type    string `json:"type"`
		Action  string `json:"action"`
		Enable  *int   `json:"enable"`
		Comment string `json:"comment"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		t.Fatalf("bad rule body %q: %v", body, err)
	}
	rule := pveclient.FirewallRule{Type: wire.Type, Action: wire.Action, Comment: wire.Comment}
	if wire.Enable != nil {
		rule.Enable = pveclient.FirewallRuleBoolPtr(*wire.Enable != 0)
	}
	return rule
}

// TestFirewallRulesOrderedDiff exercises the ordered-diff engine against
// a fake server that renumbers positions after every mutation. Phase
// one removes and modifies rules (the engine must delete at the fresh
// highest position and never reuse a stale one); phase two appends two
// rules onto the renumbered ruleset.
func TestFirewallRulesOrderedDiff(t *testing.T) {
	base := "/cluster/firewall/rules"
	srv := &firewallRulesFakeServer{
		t:         t,
		basePath:  base,
		posShift:  1000,
		posGrowth: 1000,
		rules: []pveclient.FirewallRule{
			{Type: "in", Action: "ACCEPT", Comment: "A"},
			{Type: "in", Action: "DROP", Comment: "B"},
			{Type: "out", Action: "ACCEPT", Comment: "C"},
			{Type: "out", Action: "DROP", Comment: "D"},
			{Type: "in", Action: "REJECT", Comment: "E"},
		},
	}
	client := firewallRulesTestClient(t, srv.handler)
	ctx := context.Background()

	// Phase 1: shrink and modify.
	shrunk := []pveclient.FirewallRule{
		{Type: "in", Action: "ACCEPT", Comment: "A"},
		{Type: "in", Action: "REJECT", Comment: "B2"},
		{Type: "out", Action: "REJECT", Comment: "C2"},
	}
	fresh, err := firewallRulesApplyDiff(ctx, client, base, shrunk)
	if err != nil {
		t.Fatalf("firewallRulesApplyDiff (phase 1): %v", err)
	}
	wantCalls := []string{"DEL:1004", "DEL:2003", "PUT:3001", "PUT:4002"}
	if len(srv.calls) != len(wantCalls) {
		t.Fatalf("calls = %v, want %v", srv.calls, wantCalls)
	}
	for i, want := range wantCalls {
		if srv.calls[i] != want {
			t.Fatalf("calls = %v, want %v (stale position trusted at call %d)", srv.calls, wantCalls, i)
		}
	}
	for i := range shrunk {
		if fresh[i].Comment != shrunk[i].Comment {
			t.Fatalf("fresh[%d] = %+v, want %+v", i, fresh[i], shrunk[i])
		}
	}

	// Phase 2: append two rules onto the renumbered ruleset.
	grown := append(append([]pveclient.FirewallRule{}, shrunk...),
		pveclient.FirewallRule{Type: "out", Action: "ACCEPT", Comment: "D"},
		pveclient.FirewallRule{Type: "in", Action: "ACCEPT", Comment: "N"},
	)
	fresh, err = firewallRulesApplyDiff(ctx, client, base, grown)
	if err != nil {
		t.Fatalf("firewallRulesApplyDiff (phase 2): %v", err)
	}
	wantCalls = append(wantCalls, "POST", "POST")
	if len(srv.calls) != len(wantCalls) {
		t.Fatalf("calls = %v, want %v", srv.calls, wantCalls)
	}
	for i, want := range wantCalls {
		if srv.calls[i] != want {
			t.Fatalf("calls = %v, want %v (stale position trusted at call %d)", srv.calls, wantCalls, i)
		}
	}
	if len(fresh) != len(grown) {
		t.Fatalf("fresh rules = %+v, want %d entries", fresh, len(grown))
	}
	for i := range grown {
		if fresh[i].Comment != grown[i].Comment {
			t.Fatalf("fresh[%d] = %+v, want %+v", i, fresh[i], grown[i])
		}
		if fresh[i].Pos == nil || *fresh[i].Pos != int64(i)+7000 {
			t.Fatalf("fresh[%d].pos = %v, want resynced %d", i, fresh[i].Pos, i+7000)
		}
	}
}

// TestFirewallRulesOrderedDiff_NoChanges verifies an already-converged
// ruleset produces no upstream calls.
func TestFirewallRulesOrderedDiff_NoChanges(t *testing.T) {
	base := "/cluster/firewall/rules"
	srv := &firewallRulesFakeServer{
		t:        t,
		basePath: base,
		rules: []pveclient.FirewallRule{
			{Type: "in", Action: "ACCEPT"},
			{Type: "in", Action: "DROP"},
		},
	}
	client := firewallRulesTestClient(t, srv.handler)
	plan := []pveclient.FirewallRule{
		{Type: "in", Action: "ACCEPT", Enable: pveclient.FirewallRuleBoolPtr(true)},
		{Type: "in", Action: "DROP", Enable: pveclient.FirewallRuleBoolPtr(true)},
	}
	if _, err := firewallRulesApplyDiff(context.Background(), client, base, plan); err != nil {
		t.Fatalf("firewallRulesApplyDiff: %v", err)
	}
	if len(srv.calls) != 0 {
		t.Fatalf("converged ruleset should produce no calls, got %v", srv.calls)
	}
}

// TestFirewallRulesDeleteAll_Renumbers verifies the tear-down deletes
// from the highest position downward, re-listing between deletions so a
// renumbering upstream never sees a stale position.
func TestFirewallRulesDeleteAll_Renumbers(t *testing.T) {
	base := "/nodes/pve1/firewall/rules"
	srv := &firewallRulesFakeServer{
		t:         t,
		basePath:  base,
		posShift:  1000,
		posGrowth: 1000,
		rules: []pveclient.FirewallRule{
			{Type: "in", Action: "ACCEPT"},
			{Type: "in", Action: "DROP"},
			{Type: "out", Action: "ACCEPT"},
		},
	}
	client := firewallRulesTestClient(t, srv.handler)
	if err := firewallRulesDeleteAll(context.Background(), client, base); err != nil {
		t.Fatalf("firewallRulesDeleteAll: %v", err)
	}
	want := []string{"DEL:1002", "DEL:2001", "DEL:3000"}
	if len(srv.calls) != len(want) {
		t.Fatalf("calls = %v, want %v", srv.calls, want)
	}
	for i := range want {
		if srv.calls[i] != want[i] {
			t.Fatalf("calls = %v, want %v", srv.calls, want)
		}
	}
}

// TestFirewallRulesSplitImportID covers the `:`-joined import contract.
func TestFirewallRulesSplitImportID(t *testing.T) {
	parts, err := firewallRulesSplitImportID("pve1:qemu:100", 3)
	if err != nil || parts[0] != "pve1" || parts[1] != "qemu" || parts[2] != "100" {
		t.Fatalf("split 3 = %v, %v", parts, err)
	}
	parts, err = firewallRulesSplitImportID("web", 1)
	if err != nil || parts[0] != "web" {
		t.Fatalf("split 1 = %v, %v", parts, err)
	}
	for _, id := range []string{"pve1:qemu", "pve1:qemu:100:extra", "pve1::100", ""} {
		if _, err := firewallRulesSplitImportID(id, 3); err == nil {
			t.Fatalf("id %q: expected error", id)
		}
	}
}

// TestFirewallRulesFamilyMetadataAndSchema asserts the type name and
// schema shape of all five family members.
func TestFirewallRulesFamilyMetadataAndSchema(t *testing.T) {
	cases := []struct {
		name     string
		ctor     func() resource.Resource
		typeName string
		keys     []string
	}{
		{"cluster", NewPveClusterFirewallRulesResource, "pve_" + TypeNamePveClusterFirewallRules, []string{"id", "rules"}},
		{"node", NewPveNodeFirewallRulesResource, "pve_" + TypeNamePveNodeFirewallRules, []string{"id", "node", "rules"}},
		{"guest", NewPveGuestFirewallRulesResource, "pve_" + TypeNamePveGuestFirewallRules, []string{"id", "node", "guest_type", "vmid", "rules"}},
		{"security-group", NewPveSecurityGroupFirewallRulesResource, "pve_" + TypeNamePveSecurityGroupFirewallRules, []string{"id", "group", "rules"}},
		{"vnet", NewPveVnetFirewallRulesResource, "pve_" + TypeNamePveVnetFirewallRules, []string{"id", "vnet", "rules"}},
	}
	ctx := context.Background()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := tc.ctor()
			metaResp := &resource.MetadataResponse{}
			r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
			if metaResp.TypeName != tc.typeName {
				t.Fatalf("TypeName = %q, want %q", metaResp.TypeName, tc.typeName)
			}
			schemaResp := &resource.SchemaResponse{}
			r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
			for _, key := range tc.keys {
				if schemaResp.Schema.Attributes[key] == nil {
					t.Fatalf("schema missing %s attribute", key)
				}
			}
			rules, ok := schemaResp.Schema.Attributes["rules"].(schema.ListNestedAttribute)
			if !ok {
				t.Fatal("rules attribute is not a ListNestedAttribute")
			}
			if !rules.Required {
				t.Fatal("rules attribute should be Required")
			}
			for _, ruleKey := range []string{"pos", "enable", "type", "action", "macro", "proto", "dport", "sport", "source", "dest", "icmp_type", "iface", "log", "comment", "ipversion"} {
				if rules.NestedObject.Attributes[ruleKey] == nil {
					t.Fatalf("rule object missing %s attribute", ruleKey)
				}
			}
		})
	}
}
