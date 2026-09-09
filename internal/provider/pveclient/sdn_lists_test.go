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

// TestSdnListPrefixListWireCRUD covers the prefix-list wire set: create
// body with property-string entries, list decode of object-form
// entries, single-entry create, update with delete query, per-entry
// delete, and list delete.
func TestSdnListPrefixListWireCRUD(t *testing.T) {
	var lastBody []byte
	var lastQuery string
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		lastBody, _ = io.ReadAll(r.Body)
		lastQuery = r.URL.RawQuery
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/cluster/sdn/prefix-lists":
			_, _ = w.Write([]byte(`{"data":null}`))
		case r.Method == http.MethodGet && r.URL.Path == "/cluster/sdn/prefix-lists/pl1":
			_, _ = w.Write([]byte(`{"data":{"id":"pl1","entries":[{"seq":1,"action":"permit","prefix":"10.0.0.0/8"},{"seq":5,"action":"deny","prefix":"10.2.0.0/16","ge":100,"le":120}]}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/cluster/sdn/prefix-lists/pl1/entries":
			_, _ = w.Write([]byte(`{"data":[{"seq":1,"action":"permit","prefix":"10.0.0.0/8"},{"seq":5,"action":"deny","prefix":"10.2.0.0/16","ge":100,"le":120}]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/cluster/sdn/prefix-lists/pl1/entries":
			_, _ = w.Write([]byte(`{"data":null}`))
		case r.Method == http.MethodPut && r.URL.Path == "/cluster/sdn/prefix-lists/pl1/entries/1":
			_, _ = w.Write([]byte(`{"data":null}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/cluster/sdn/prefix-lists/pl1/entries/5":
			_, _ = w.Write([]byte(`{"data":null}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/cluster/sdn/prefix-lists/pl1":
			_, _ = w.Write([]byte(`{"data":null}`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	ctx := context.Background()

	// Create the list with two entries; the entries must travel as the
	// pin's property-string array.
	create := []SdnPrefixListEntry{
		{Action: "permit", Prefix: "10.0.0.0/8"},
		{Action: "deny", Prefix: "10.2.0.0/16", Ge: i64Ptr(100), Le: i64Ptr(120), Seq: i64Ptr(5)},
	}
	if err := c.CreateSdnPrefixList(ctx, "pl1", create); err != nil {
		t.Fatalf("CreateSdnPrefixList: %v", err)
	}
	var sent struct {
		ID      string   `json:"id"`
		Entries []string `json:"entries"`
	}
	if err := json.Unmarshal(lastBody, &sent); err != nil {
		t.Fatalf("create body %q is not JSON: %v", lastBody, err)
	}
	if sent.ID != "pl1" || len(sent.Entries) != 2 {
		t.Fatalf("create body = %v", sent)
	}
	if sent.Entries[0] != "action=permit,prefix=10.0.0.0/8" {
		t.Fatalf("entries[0] = %q", sent.Entries[0])
	}
	if sent.Entries[1] != "action=deny,prefix=10.2.0.0/16,ge=100,le=120,seq=5" {
		t.Fatalf("entries[1] = %q", sent.Entries[1])
	}

	// Get the list and decode its entries.
	list, err := c.GetSdnPrefixList(ctx, "pl1")
	if err != nil {
		t.Fatalf("GetSdnPrefixList: %v", err)
	}
	if list.ID != "pl1" || len(list.Entries) != 2 || list.Entries[1].Seq == nil || *list.Entries[1].Seq != 5 {
		t.Fatalf("list = %+v", list)
	}
	if list.Entries[0].Action != "permit" || list.Entries[0].Prefix != "10.0.0.0/8" {
		t.Fatalf("entries[0] = %+v", list.Entries[0])
	}

	entries, err := c.ListSdnPrefixListEntries(ctx, "pl1")
	if err != nil {
		t.Fatalf("ListSdnPrefixListEntries: %v", err)
	}
	if len(entries) != 2 || entries[1].Ge == nil || *entries[1].Ge != 100 || *entries[1].Le != 120 {
		t.Fatalf("entries = %+v", entries)
	}

	if err := c.CreateSdnPrefixListEntry(ctx, "pl1", SdnPrefixListEntry{Action: "permit", Prefix: "192.168.0.0/16"}); err != nil {
		t.Fatalf("CreateSdnPrefixListEntry: %v", err)
	}
	var entryBody map[string]any
	if err := json.Unmarshal(lastBody, &entryBody); err != nil {
		t.Fatalf("entry body %q is not JSON: %v", lastBody, err)
	}
	if entryBody["action"] != "permit" || entryBody["prefix"] != "192.168.0.0/16" {
		t.Fatalf("entry body = %v (single-entry create uses flat parameters)", entryBody)
	}

	update := SdnPrefixListEntry{Action: "deny", Prefix: "10.9.0.0/16"}
	if err := c.UpdateSdnPrefixListEntry(ctx, "pl1", 1, update, []string{"ge", "le"}); err != nil {
		t.Fatalf("UpdateSdnPrefixListEntry: %v", err)
	}
	if !strings.Contains(lastQuery, "delete=ge%2Cle") {
		t.Fatalf("update query = %q", lastQuery)
	}

	if err := c.DeleteSdnPrefixListEntry(ctx, "pl1", 5); err != nil {
		t.Fatalf("DeleteSdnPrefixListEntry: %v", err)
	}
	if err := c.DeleteSdnPrefixList(ctx, "pl1"); err != nil {
		t.Fatalf("DeleteSdnPrefixList: %v", err)
	}
}

// TestSdnListPrefixListPropertyString covers the property-string
// round trip and the object decode path, including garbage rejection.
func TestSdnListPrefixListPropertyString(t *testing.T) {
	entry := SdnPrefixListEntry{Seq: i64Ptr(7), Action: "deny", Prefix: "fd00::/8", Ge: i64Ptr(64)}
	raw, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	if string(raw) != `"action=deny,prefix=fd00::/8,ge=64,seq=7"` {
		t.Fatalf("marshaled = %s", raw)
	}
	var back SdnPrefixListEntry
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("UnmarshalJSON (string form): %v", err)
	}
	if back.Action != "deny" || back.Prefix != "fd00::/8" || back.Ge == nil || *back.Ge != 64 || back.Seq == nil || *back.Seq != 7 || back.Le != nil {
		t.Fatalf("round trip = %+v", back)
	}
	var fromObject SdnPrefixListEntry
	if err := json.Unmarshal([]byte(`{"seq":2,"action":"permit","prefix":"10.0.0.0/8","le":120}`), &fromObject); err != nil {
		t.Fatalf("UnmarshalJSON (object form): %v", err)
	}
	if fromObject.Action != "permit" || fromObject.Le == nil || *fromObject.Le != 120 {
		t.Fatalf("object decode = %+v", fromObject)
	}
	if err := json.Unmarshal([]byte(`"nonsense-without-equals"`), &back); err == nil {
		t.Fatal("expected error for malformed property string")
	}
}

// TestSdnListRouteMapWireCRUD covers the route-map wire set: create with
// route-map-id and property-string match/set/exit-action, list decode,
// update with delete query, and per-entry delete.
func TestSdnListRouteMapWireCRUD(t *testing.T) {
	var lastBody []byte
	var lastQuery string
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		lastBody, _ = io.ReadAll(r.Body)
		lastQuery = r.URL.RawQuery
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/cluster/sdn/route-maps/entries":
			_, _ = w.Write([]byte(`{"data":null}`))
		case r.Method == http.MethodGet && r.URL.Path == "/cluster/sdn/route-maps/entries/rm1":
			_, _ = w.Write([]byte(`{"data":[{"route-map-id":"rm1","order":0,"action":"permit","digest":"abc","match":["key=ip-address-prefix-list,value=pl1"],"set":["key=local-preference,value=200"]},{"route-map-id":"rm1","order":10,"action":"deny","exit-action":"key=on-match-goto,value=20"}]}`))
		case r.Method == http.MethodPut && r.URL.Path == "/cluster/sdn/route-maps/entries/rm1/entry/10":
			_, _ = w.Write([]byte(`{"data":null}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/cluster/sdn/route-maps/entries/rm1/entry/10":
			_, _ = w.Write([]byte(`{"data":null}`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	ctx := context.Background()

	create := SdnRouteMapEntry{
		RouteMapID: "rm1",
		Order:      SdnRouteMapOrderPtr(0),
		Action:     "permit",
		Match:      []SdnRouteMapKV{{Key: "ip-address-prefix-list", Value: "pl1"}},
		Set:        []SdnRouteMapKV{{Key: "local-preference", Value: "200"}},
		ExitAction: &SdnRouteMapExitAction{Key: "on-match-goto", Value: i64Ptr(20)},
	}
	if err := c.CreateSdnRouteMapEntry(ctx, create); err != nil {
		t.Fatalf("CreateSdnRouteMapEntry: %v", err)
	}
	var sent map[string]any
	if err := json.Unmarshal(lastBody, &sent); err != nil {
		t.Fatalf("create body %q is not JSON: %v", lastBody, err)
	}
	if sent["route-map-id"] != "rm1" || sent["order"] != float64(0) || sent["action"] != "permit" {
		t.Fatalf("create body = %v", sent)
	}
	if sent["exit-action"] != "key=on-match-goto,value=20" {
		t.Fatalf("exit-action = %v (must be the pin's property string)", sent["exit-action"])
	}
	match, ok := sent["match"].([]any)
	if !ok || len(match) != 1 || match[0] != "key=ip-address-prefix-list,value=pl1" {
		t.Fatalf("match = %v (must be the pin's property-string array)", sent["match"])
	}

	entries, err := c.ListSdnRouteMapEntries(ctx, "rm1")
	if err != nil {
		t.Fatalf("ListSdnRouteMapEntries: %v", err)
	}
	if len(entries) != 2 || entries[0].Order == nil || *entries[0].Order != 0 {
		t.Fatalf("entries = %+v", entries)
	}
	if len(entries[0].Match) != 1 || entries[0].Match[0].Key != "ip-address-prefix-list" || entries[0].Match[0].Value != "pl1" {
		t.Fatalf("match decode = %+v", entries[0].Match)
	}
	if entries[0].Digest != "abc" {
		t.Fatalf("digest decode = %+v", entries[0])
	}
	if entries[1].ExitAction == nil || entries[1].ExitAction.Key != "on-match-goto" || entries[1].ExitAction.Value == nil || *entries[1].ExitAction.Value != 20 {
		t.Fatalf("exit-action decode = %+v", entries[1].ExitAction)
	}

	update := SdnRouteMapEntry{RouteMapID: "rm1", Order: SdnRouteMapOrderPtr(10), Action: "deny"}
	if err := c.UpdateSdnRouteMapEntry(ctx, "rm1", 10, update, []string{"call", "exit-action"}); err != nil {
		t.Fatalf("UpdateSdnRouteMapEntry: %v", err)
	}
	if !strings.Contains(lastQuery, "delete=call%2Cexit-action") {
		t.Fatalf("update query = %q", lastQuery)
	}

	if err := c.DeleteSdnRouteMapEntry(ctx, "rm1", 10); err != nil {
		t.Fatalf("DeleteSdnRouteMapEntry: %v", err)
	}
}

// TestSdnListRouteMapPropertyStrings covers the KV and exit-action property
// string round trips in both directions, including empty values.
func TestSdnListRouteMapPropertyStrings(t *testing.T) {
	kv := SdnRouteMapKV{Key: "tag", Value: "42"}
	raw, err := json.Marshal(kv)
	if err != nil {
		t.Fatalf("KV MarshalJSON: %v", err)
	}
	if string(raw) != `"key=tag,value=42"` {
		t.Fatalf("KV marshaled = %s", raw)
	}
	var kvBack SdnRouteMapKV
	if err := json.Unmarshal(raw, &kvBack); err != nil {
		t.Fatalf("KV UnmarshalJSON (string form): %v", err)
	}
	if kvBack.Key != "tag" || kvBack.Value != "42" {
		t.Fatalf("KV round trip = %+v", kvBack)
	}
	var kvBare SdnRouteMapKV
	if err := json.Unmarshal([]byte(`{"key":"vni","value":"100"}`), &kvBare); err != nil {
		t.Fatalf("KV UnmarshalJSON (object form): %v", err)
	}
	if kvBare.Key != "vni" || kvBare.Value != "100" {
		t.Fatalf("KV object decode = %+v", kvBare)
	}

	exit := SdnRouteMapExitAction{Key: "on-match-next"}
	raw, err = json.Marshal(exit)
	if err != nil {
		t.Fatalf("exit-action MarshalJSON: %v", err)
	}
	if string(raw) != `"key=on-match-next"` {
		t.Fatalf("exit-action marshaled = %s", raw)
	}
	var exitBack SdnRouteMapExitAction
	if err := json.Unmarshal(raw, &exitBack); err != nil {
		t.Fatalf("exit-action UnmarshalJSON (string form): %v", err)
	}
	if exitBack.Key != "on-match-next" || exitBack.Value != nil {
		t.Fatalf("exit-action round trip = %+v", exitBack)
	}
}

// TestSdnLists_Missing404IsAPIError confirms missing prefix lists and
// route maps surface as 404 *APIError so the resource layer can react.
func TestSdnLists_Missing404IsAPIError(t *testing.T) {
	c := newFakePVETokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method %s", r.Method)
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"errors":["no such list"]}`))
	})
	ctx := context.Background()
	if _, err := c.GetSdnPrefixList(ctx, "ghost"); err == nil {
		t.Fatal("GetSdnPrefixList: expected error")
	} else if !isTestNotFound(err) {
		t.Fatalf("GetSdnPrefixList error = %v, want 404 APIError", err)
	}
	if _, err := c.ListSdnRouteMapEntries(ctx, "ghost"); err == nil {
		t.Fatal("ListSdnRouteMapEntries: expected error")
	} else if !isTestNotFound(err) {
		t.Fatalf("ListSdnRouteMapEntries error = %v, want 404 APIError", err)
	}
}

// i64Ptr returns a pointer to v, for building test fixtures.
func i64Ptr(v int64) *int64 { return &v }
