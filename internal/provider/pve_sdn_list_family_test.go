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

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/dwhoban/terraform-provider-pve/internal/provider/pveclient"
)

// sdnListTestClient spins up a fake PVE API served by h and returns a
// client pointed at it. Token auth avoids the /access/ticket exchange.
func sdnListTestClient(t *testing.T, h http.HandlerFunc) *pveclient.Client {
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

// sdnPrefixListFakeServer serves one prefix list's entries and shifts
// every sequence number after each mutation, emulating an upstream that
// renumbers aggressively: clients must re-list between calls instead of
// trusting stale positions.
type sdnPrefixListFakeServer struct {
	t         *testing.T
	entries   []pveclient.SdnPrefixListEntry
	posShift  int64
	posGrowth int64
	calls     []string
}

// handler serves the fake prefix list on the HTTP mux.
func (s *sdnPrefixListFakeServer) handler(w http.ResponseWriter, r *http.Request) {
	const base = "/cluster/sdn/prefix-lists/pl1/entries"
	w.Header().Set("Content-Type", "application/json")
	if !strings.HasPrefix(r.URL.Path, base) {
		s.t.Fatalf("unexpected path %q", r.URL.Path)
	}
	switch r.Method {
	case http.MethodGet:
		var sb strings.Builder
		_, _ = sb.WriteString(`{"data":[`)
		for i, e := range s.entries {
			if i > 0 {
				_, _ = sb.WriteString(",")
			}
			_, _ = sb.WriteString(`{"seq":` + strconv.FormatInt(int64(i)+s.posShift, 10) +
				`,"action":` + strconv.Quote(e.Action) +
				`,"prefix":` + strconv.Quote(e.Prefix))
			if e.Ge != nil {
				_, _ = sb.WriteString(`,"ge":` + strconv.FormatInt(*e.Ge, 10))
			}
			if e.Le != nil {
				_, _ = sb.WriteString(`,"le":` + strconv.FormatInt(*e.Le, 10))
			}
			_, _ = sb.WriteString(`}`)
		}
		_, _ = sb.WriteString(`]}`)
		_, _ = w.Write([]byte(sb.String()))
		return
	case http.MethodPost:
		s.entries = append(s.entries, decodeSdnPrefixListEntryBody(s.t, r))
		s.calls = append(s.calls, "POST")
	case http.MethodPut:
		seq, _ := strconv.ParseInt(strings.TrimPrefix(r.URL.Path, base+"/"), 10, 64)
		idx := int(seq - s.posShift)
		if idx < 0 || idx >= len(s.entries) {
			s.t.Fatalf("PUT at out-of-range seq %d", seq)
		}
		s.entries[idx] = decodeSdnPrefixListEntryBody(s.t, r)
		s.calls = append(s.calls, "PUT:"+strconv.FormatInt(seq, 10))
	case http.MethodDelete:
		seq, _ := strconv.ParseInt(strings.TrimPrefix(r.URL.Path, base+"/"), 10, 64)
		idx := int(seq - s.posShift)
		if idx < 0 || idx >= len(s.entries) {
			s.t.Fatalf("DELETE at out-of-range seq %d", seq)
		}
		s.entries = append(s.entries[:idx], s.entries[idx+1:]...)
		s.calls = append(s.calls, "DEL:"+strconv.FormatInt(seq, 10))
	}
	// Renumber after every mutation (never on reads): the next list
	// carries fresh positions.
	s.posShift += s.posGrowth
	_, _ = w.Write([]byte(`{"data":null}`))
}

// decodeSdnPrefixListEntryBody reads the flat request body of the
// single-entry POST and PUT verbs.
func decodeSdnPrefixListEntryBody(t *testing.T, r *http.Request) pveclient.SdnPrefixListEntry {
	t.Helper()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("reading entry body: %v", err)
	}
	var wire struct {
		Action string `json:"action"`
		Prefix string `json:"prefix"`
		Ge     *int64 `json:"ge"`
		Le     *int64 `json:"le"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		t.Fatalf("bad entry body %q: %v", body, err)
	}
	return pveclient.SdnPrefixListEntry{Action: wire.Action, Prefix: wire.Prefix, Ge: wire.Ge, Le: wire.Le}
}

// TestSdnListOrderedDiff_PrefixList exercises the shared ordered-diff
// engine through the prefix-list adapter against a renumbering fake:
// phase one removes and modifies entries (deletes must hit the fresh
// highest seq and updates the fresh in-place seq); phase two reorders
// content in place and appends one entry.
func TestSdnListOrderedDiff_PrefixList(t *testing.T) {
	srv := &sdnPrefixListFakeServer{
		t:         t,
		posShift:  1000,
		posGrowth: 1000,
		entries: []pveclient.SdnPrefixListEntry{
			{Action: "permit", Prefix: "10.0.0.0/8"},
			{Action: "deny", Prefix: "10.2.0.0/16"},
			{Action: "permit", Prefix: "10.3.0.0/16"},
			{Action: "deny", Prefix: "10.4.0.0/16"},
			{Action: "permit", Prefix: "10.5.0.0/16"},
		},
	}
	client := sdnListTestClient(t, srv.handler)
	ops := sdnPrefixListOps{client: client, id: "pl1"}
	ctx := context.Background()

	// Phase 1: shrink and modify.
	plan1 := []pveclient.SdnPrefixListEntry{
		{Action: "permit", Prefix: "10.0.0.0/8"},
		{Action: "deny", Prefix: "10.9.0.0/16"},
		{Action: "permit", Prefix: "10.3.0.0/16"},
	}
	fresh, err := sdnListApplyDiff(ctx, ops, plan1)
	if err != nil {
		t.Fatalf("sdnListApplyDiff (phase 1): %v", err)
	}
	wantCalls := []string{"DEL:1004", "DEL:2003", "PUT:3001"}
	assertSdnListCalls(t, srv.calls, wantCalls)
	for i := range plan1 {
		if fresh[i].Prefix != plan1[i].Prefix || fresh[i].Action != plan1[i].Action {
			t.Fatalf("fresh[%d] = %+v, want %+v", i, fresh[i], plan1[i])
		}
	}

	// Phase 2: reorder content in place and append.
	plan2 := append(append([]pveclient.SdnPrefixListEntry{}, plan1[0], plan1[2], plan1[1]),
		pveclient.SdnPrefixListEntry{Action: "permit", Prefix: "192.168.0.0/16"})
	fresh, err = sdnListApplyDiff(ctx, ops, plan2)
	if err != nil {
		t.Fatalf("sdnListApplyDiff (phase 2): %v", err)
	}
	wantCalls = append(wantCalls, "PUT:4001", "PUT:5002", "POST")
	assertSdnListCalls(t, srv.calls, wantCalls)
	for i := range plan2 {
		if fresh[i].Prefix != plan2[i].Prefix {
			t.Fatalf("fresh[%d] = %+v, want %+v (order not converged)", i, fresh[i], plan2[i])
		}
		if fresh[i].Seq == nil || *fresh[i].Seq != int64(i)+7000 {
			t.Fatalf("fresh[%d].seq = %v, want resynced %d", i, fresh[i].Seq, i+7000)
		}
	}
}

// sdnRouteMapFakeServer serves one route map's entries and shifts every
// order index after each mutation, like the prefix-list fake.
type sdnRouteMapFakeServer struct {
	t         *testing.T
	entries   []pveclient.SdnRouteMapEntry
	posShift  int64
	posGrowth int64
	calls     []string
	// lastCreateOrder records the order index of the last created
	// entry, to assert the adapter's append placement.
	lastCreateOrder int64
}

// handler serves the fake route map on the HTTP mux.
func (s *sdnRouteMapFakeServer) handler(w http.ResponseWriter, r *http.Request) {
	const base = "/cluster/sdn/route-maps/entries/rm1"
	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.Method == http.MethodGet && r.URL.Path == base:
		var sb strings.Builder
		_, _ = sb.WriteString(`{"data":[`)
		for i, e := range s.entries {
			if i > 0 {
				_, _ = sb.WriteString(",")
			}
			_, _ = sb.WriteString(`{"route-map-id":"rm1","order":` + strconv.FormatInt(int64(i)+s.posShift, 10) +
				`,"action":` + strconv.Quote(e.Action))
			if e.Call != "" {
				_, _ = sb.WriteString(`,"call":` + strconv.Quote(e.Call))
			}
			_, _ = sb.WriteString(`}`)
		}
		_, _ = sb.WriteString(`]}`)
		_, _ = w.Write([]byte(sb.String()))
		return
	case r.Method == http.MethodPost && r.URL.Path == "/cluster/sdn/route-maps/entries":
		entry := decodeSdnRouteMapEntryBody(s.t, r)
		s.lastCreateOrder = *entry.Order
		s.entries = append(s.entries, entry)
		s.calls = append(s.calls, "POST")
	case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, base+"/entry/"):
		order, _ := strconv.ParseInt(strings.TrimPrefix(r.URL.Path, base+"/entry/"), 10, 64)
		idx := int(order - s.posShift)
		if idx < 0 || idx >= len(s.entries) {
			s.t.Fatalf("PUT at out-of-range order %d", order)
		}
		s.entries[idx] = decodeSdnRouteMapEntryBody(s.t, r)
		s.calls = append(s.calls, "PUT:"+strconv.FormatInt(order, 10))
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, base+"/entry/"):
		order, _ := strconv.ParseInt(strings.TrimPrefix(r.URL.Path, base+"/entry/"), 10, 64)
		idx := int(order - s.posShift)
		if idx < 0 || idx >= len(s.entries) {
			s.t.Fatalf("DELETE at out-of-range order %d", order)
		}
		s.entries = append(s.entries[:idx], s.entries[idx+1:]...)
		s.calls = append(s.calls, "DEL:"+strconv.FormatInt(order, 10))
	default:
		s.t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
	}
	// Renumber after every mutation (never on reads).
	s.posShift += s.posGrowth
	_, _ = w.Write([]byte(`{"data":null}`))
}

// decodeSdnRouteMapEntryBody reads a route map entry request body in
// the wire format the client emits.
func decodeSdnRouteMapEntryBody(t *testing.T, r *http.Request) pveclient.SdnRouteMapEntry {
	t.Helper()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("reading entry body: %v", err)
	}
	var entry pveclient.SdnRouteMapEntry
	if err := json.Unmarshal(body, &entry); err != nil {
		t.Fatalf("bad entry body %q: %v", body, err)
	}
	return entry
}

// TestSdnListOrderedDiff_RouteMap exercises the shared engine through
// the route-map adapter, including the adapter's append placement after
// the highest existing order.
func TestSdnListOrderedDiff_RouteMap(t *testing.T) {
	srv := &sdnRouteMapFakeServer{
		t:         t,
		posShift:  1000,
		posGrowth: 1000,
		entries: []pveclient.SdnRouteMapEntry{
			{Action: "permit"},
			{Action: "deny"},
			{Action: "permit"},
			{Action: "deny"},
			{Action: "permit"},
		},
	}
	client := sdnListTestClient(t, srv.handler)
	ops := sdnRouteMapOps{client: client, routeMapID: "rm1"}
	ctx := context.Background()

	// Phase 1: shrink and modify.
	plan1 := []pveclient.SdnRouteMapEntry{
		{Action: "permit"},
		{Action: "deny", Call: "rm2"},
		{Action: "permit"},
	}
	fresh, err := sdnListApplyDiff(ctx, ops, plan1)
	if err != nil {
		t.Fatalf("sdnListApplyDiff (phase 1): %v", err)
	}
	wantCalls := []string{"DEL:1004", "DEL:2003", "PUT:3001"}
	assertSdnListCalls(t, srv.calls, wantCalls)
	if fresh[1].Call != "rm2" {
		t.Fatalf("fresh[1] = %+v, want call rm2", fresh[1])
	}

	// Phase 2: append one entry; the in-place entries already
	// converged, so only the append hits the upstream.
	plan2 := append(append([]pveclient.SdnRouteMapEntry{}, plan1...),
		pveclient.SdnRouteMapEntry{
			Action: "permit",
			Match:  []pveclient.SdnRouteMapKV{{Key: "ip-address-prefix-list", Value: "pl1"}},
		})
	fresh, err = sdnListApplyDiff(ctx, ops, plan2)
	if err != nil {
		t.Fatalf("sdnListApplyDiff (phase 2): %v", err)
	}
	wantCalls = append(wantCalls, "POST")
	assertSdnListCalls(t, srv.calls, wantCalls)
	if len(fresh) != len(plan2) {
		t.Fatalf("fresh = %+v, want %d entries", fresh, len(plan2))
	}
	// The adapter placed the appended entry after the highest order
	// it listed at create time (4002) in steps of 10.
	if srv.lastCreateOrder != 4012 {
		t.Fatalf("created order = %d, want 4012", srv.lastCreateOrder)
	}
	for i := range plan2 {
		if fresh[i].Action != plan2[i].Action {
			t.Fatalf("fresh[%d] = %+v, want %+v", i, fresh[i], plan2[i])
		}
	}
	if fresh[3].Order == nil || *fresh[3].Order != 5003 {
		t.Fatalf("fresh[3].order = %v, want resynced 5003", fresh[3].Order)
	}
}

// TestSdnListDeleteAll_RouteMap verifies the tear-down deletes from the
// highest order downward, re-listing between deletions.
func TestSdnListDeleteAll_RouteMap(t *testing.T) {
	srv := &sdnRouteMapFakeServer{
		t:         t,
		posShift:  1000,
		posGrowth: 1000,
		entries: []pveclient.SdnRouteMapEntry{
			{Action: "permit"},
			{Action: "deny"},
			{Action: "permit"},
		},
	}
	client := sdnListTestClient(t, srv.handler)
	if err := sdnListDeleteAll(context.Background(), sdnRouteMapOps{client: client, routeMapID: "rm1"}); err != nil {
		t.Fatalf("sdnListDeleteAll: %v", err)
	}
	want := []string{"DEL:1002", "DEL:2001", "DEL:3000"}
	assertSdnListCalls(t, srv.calls, want)
	if len(srv.entries) != 0 {
		t.Fatalf("entries = %+v, want empty", srv.entries)
	}
}

// assertSdnListCalls fails when the recorded calls differ from want.
func assertSdnListCalls(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("calls = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("calls = %v, want %v (stale position trusted at call %d)", got, want, i)
		}
	}
}

// TestSdnListsFamilyMetadataAndSchema asserts the type names and schema
// shapes of all four SDN list components.
func TestSdnListsFamilyMetadataAndSchema(t *testing.T) {
	ctx := context.Background()
	resourceCases := []struct {
		name     string
		ctor     func() resource.Resource
		typeName string
		keys     []string
	}{
		{"prefix-list", NewPveSdnPrefixListResource, "pve_" + TypeNamePveSdnPrefixList, []string{"id", "entries"}},
		{"route-map", NewPveSdnRouteMapResource, "pve_" + TypeNamePveSdnRouteMap, []string{"id", "route_map_id", "entries"}},
	}
	for _, tc := range resourceCases {
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
		})
	}
	dataSourceCases := []struct {
		name     string
		ctor     func() datasource.DataSource
		typeName string
		keys     []string
	}{
		{"prefix-list", NewPveSdnPrefixListDataSource, "pve_" + TypeNamePveSdnPrefixList, []string{"id", "entries"}},
		{"route-map", NewPveSdnRouteMapDataSource, "pve_" + TypeNamePveSdnRouteMap, []string{"id", "route_map_id", "entries"}},
	}
	for _, tc := range dataSourceCases {
		t.Run(tc.name, func(t *testing.T) {
			d := tc.ctor()
			metaResp := &datasource.MetadataResponse{}
			d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "pve"}, metaResp)
			if metaResp.TypeName != tc.typeName {
				t.Fatalf("TypeName = %q, want %q", metaResp.TypeName, tc.typeName)
			}
			schemaResp := &datasource.SchemaResponse{}
			d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
			for _, key := range tc.keys {
				if schemaResp.Schema.Attributes[key] == nil {
					t.Fatalf("schema missing %s attribute", key)
				}
			}
		})
	}
}

// TestPveSdnPrefixListDataSource_Read verifies the data source decode,
// including an entry served in the property-string form.
func TestPveSdnPrefixListDataSource_Read(t *testing.T) {
	d := NewPveSdnPrefixListDataSource()
	// safetyassert: the constructor in this package always returns this concrete implementation type.
	impl, ok := d.(*pveSdnPrefixListDataSource)
	if !ok {
		t.Fatalf("constructor returned %T", d)
	}
	impl.client = sdnListTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/cluster/sdn/prefix-lists/pl1":
			_, _ = w.Write([]byte(`{"data":{"id":"pl1"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/cluster/sdn/prefix-lists/pl1/entries":
			_, _ = w.Write([]byte(`{"data":[{"seq":1,"action":"permit","prefix":"10.0.0.0/8"},"action=deny,prefix=10.2.0.0/16,ge=100,le=120,seq=5"]}`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	ctx := context.Background()
	cfg := haDSConfig(t, d, ctx, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "pl1"),
	})
	resp := &datasource.ReadResponse{State: haDSNullState(t, d, ctx)}
	impl.Read(ctx, datasource.ReadRequest{Config: cfg}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %s", diagnosticsError(resp.Diagnostics))
	}
	var got pveSdnPrefixListDataSourceModel
	if err := resp.State.Get(ctx, &got); err != nil {
		t.Fatalf("State.Get: %v", err)
	}
	if got.ID.ValueString() != "pl1" || len(got.Entries) != 2 {
		t.Fatalf("prefix list = %+v", got)
	}
	if got.Entries[0].Seq.ValueInt64() != 1 || got.Entries[0].Action.ValueString() != "permit" {
		t.Fatalf("entries[0] = %+v", got.Entries[0])
	}
	if got.Entries[1].Seq.ValueInt64() != 5 || got.Entries[1].Ge.ValueInt64() != 100 || got.Entries[1].Le.ValueInt64() != 120 {
		t.Fatalf("entries[1] = %+v (property-string decode)", got.Entries[1])
	}
}
