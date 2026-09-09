// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// SdnPrefixListEntry is one entry of a Proxmox VE SDN prefix list. Seq
// is the server-assigned position handle (the pin's entry link key); Ge
// and Le bound the matched prefix length. On the wire the entry travels
// as a PVE property string when embedded in a request's entries array,
// and as a JSON object in responses; UnmarshalJSON accepts both forms.
type SdnPrefixListEntry struct {
	Seq    *int64
	Action string
	Prefix string
	Ge     *int64
	Le     *int64
}

// MarshalJSON emits the pin's property-string form:
// action=<permit|deny>,prefix=<cidr>[,ge=<n>][,le=<n>][,seq=<n>].
func (e SdnPrefixListEntry) MarshalJSON() ([]byte, error) {
	var sb strings.Builder
	sb.WriteString("action=" + e.Action)
	sb.WriteString(",prefix=" + e.Prefix)
	if e.Ge != nil {
		sb.WriteString(",ge=" + strconv.FormatInt(*e.Ge, 10))
	}
	if e.Le != nil {
		sb.WriteString(",le=" + strconv.FormatInt(*e.Le, 10))
	}
	if e.Seq != nil {
		sb.WriteString(",seq=" + strconv.FormatInt(*e.Seq, 10))
	}
	return json.Marshal(sb.String())
}

// UnmarshalJSON decodes both the response-side JSON object form and the
// property-string form.
func (e *SdnPrefixListEntry) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		return nil
	}
	if data[0] == '{' {
		var wire struct {
			Seq    *int64 `json:"seq"`
			Action string `json:"action"`
			Prefix string `json:"prefix"`
			Ge     *int64 `json:"ge"`
			Le     *int64 `json:"le"`
		}
		if err := json.Unmarshal(data, &wire); err != nil {
			return fmt.Errorf("sdn prefix list entry object: %w", err)
		}
		e.Seq, e.Action, e.Prefix, e.Ge, e.Le = wire.Seq, wire.Action, wire.Prefix, wire.Ge, wire.Le
		return nil
	}
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return fmt.Errorf("sdn prefix list entry: %w", err)
	}
	parsed, err := parseSdnPrefixListPropertyString(text)
	if err != nil {
		return err
	}
	*e = parsed
	return nil
}

// parseSdnPrefixListPropertyString parses the pin's entry property
// string form.
func parseSdnPrefixListPropertyString(text string) (SdnPrefixListEntry, error) {
	entry := SdnPrefixListEntry{}
	if text == "" {
		return entry, fmt.Errorf("sdn prefix list entry: empty property string")
	}
	for _, part := range strings.Split(text, ",") {
		key, value, found := strings.Cut(part, "=")
		if !found {
			return entry, fmt.Errorf("sdn prefix list entry: malformed component %q in %q", part, text)
		}
		switch key {
		case "action":
			entry.Action = value
		case "prefix":
			entry.Prefix = value
		case "ge":
			v, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return entry, fmt.Errorf("sdn prefix list entry: ge %q: %w", value, err)
			}
			entry.Ge = &v
		case "le":
			v, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return entry, fmt.Errorf("sdn prefix list entry: le %q: %w", value, err)
			}
			entry.Le = &v
		case "seq":
			v, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return entry, fmt.Errorf("sdn prefix list entry: seq %q: %w", value, err)
			}
			entry.Seq = &v
		default:
			return entry, fmt.Errorf("sdn prefix list entry: unknown key %q in %q", key, text)
		}
	}
	return entry, nil
}

// SdnPrefixList is one Proxmox VE SDN prefix list with its entries.
type SdnPrefixList struct {
	ID      string
	Entries []SdnPrefixListEntry
}

// SdnRouteMapKV is one key/value clause of a route map match or set
// condition. On the wire it is the pin's property string
// key=<name>[,value=<key-dependent>]; UnmarshalJSON also accepts the
// JSON object form.
type SdnRouteMapKV struct {
	Key   string
	Value string
}

// MarshalJSON emits the pin's property-string form.
func (kv SdnRouteMapKV) MarshalJSON() ([]byte, error) {
	if kv.Value == "" {
		return json.Marshal("key=" + kv.Key)
	}
	return json.Marshal("key=" + kv.Key + ",value=" + kv.Value)
}

// UnmarshalJSON decodes both the property-string and the object form.
func (kv *SdnRouteMapKV) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		return nil
	}
	if data[0] == '{' {
		var wire struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		}
		if err := json.Unmarshal(data, &wire); err != nil {
			return fmt.Errorf("sdn route map clause object: %w", err)
		}
		kv.Key, kv.Value = wire.Key, wire.Value
		return nil
	}
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return fmt.Errorf("sdn route map clause: %w", err)
	}
	key, value, found := strings.Cut(text, ",value=")
	key, foundKey := strings.CutPrefix(key, "key=")
	if !foundKey || (strings.Contains(text, ",") && !found) {
		return fmt.Errorf("sdn route map clause: malformed property string %q", text)
	}
	kv.Key, kv.Value = key, strings.TrimPrefix(value, text[:0])
	if !found {
		kv.Value = ""
	}
	return nil
}

// SdnRouteMapExitAction is a route map entry's exit action: the pin's
// property string key=<on-match-goto|on-match-next|continue>
// [,value=<integer>]. UnmarshalJSON also accepts the object form.
type SdnRouteMapExitAction struct {
	Key   string
	Value *int64
}

// MarshalJSON emits the pin's property-string form.
func (x SdnRouteMapExitAction) MarshalJSON() ([]byte, error) {
	if x.Value == nil {
		return json.Marshal("key=" + x.Key)
	}
	return json.Marshal("key=" + x.Key + ",value=" + strconv.FormatInt(*x.Value, 10))
}

// UnmarshalJSON decodes both the property-string and the object form.
func (x *SdnRouteMapExitAction) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		return nil
	}
	if data[0] == '{' {
		var wire struct {
			Key   string `json:"key"`
			Value *int64 `json:"value"`
		}
		if err := json.Unmarshal(data, &wire); err != nil {
			return fmt.Errorf("sdn route map exit action object: %w", err)
		}
		x.Key, x.Value = wire.Key, wire.Value
		return nil
	}
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return fmt.Errorf("sdn route map exit action: %w", err)
	}
	parsed, err := parseSdnRouteMapExitAction(text)
	if err != nil {
		return err
	}
	*x = parsed
	return nil
}

// parseSdnRouteMapExitAction parses the pin's exit-action property
// string form.
func parseSdnRouteMapExitAction(text string) (SdnRouteMapExitAction, error) {
	action := SdnRouteMapExitAction{}
	head, valuePart, hasValue := strings.Cut(text, ",")
	key, found := strings.CutPrefix(head, "key=")
	if !found {
		return action, fmt.Errorf("sdn route map exit action: malformed property string %q", text)
	}
	action.Key = key
	if hasValue {
		raw, foundValue := strings.CutPrefix(valuePart, "value=")
		if !foundValue {
			return action, fmt.Errorf("sdn route map exit action: malformed component %q in %q", valuePart, text)
		}
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return action, fmt.Errorf("sdn route map exit action: value %q: %w", raw, err)
		}
		action.Value = &v
	}
	return action, nil
}

// SdnRouteMapEntry is one entry of a Proxmox VE SDN route map. Order is
// the server-assigned position handle; Match and Set carry the entry's
// match and set clauses; Digest is response-only concurrency metadata.
type SdnRouteMapEntry struct {
	RouteMapID string
	Order      *int64
	Action     string
	Call       string
	ExitAction *SdnRouteMapExitAction
	Match      []SdnRouteMapKV
	Set        []SdnRouteMapKV
	Digest     string
}

// SdnRouteMapOrderPtr returns a pointer to order, for building
// SdnRouteMapEntry request structs.
func SdnRouteMapOrderPtr(order int64) *int64 { return &order }

// sdnRouteMapEntryWire mirrors the wire shape of SdnRouteMapEntry: the
// hyphenated route-map-id and exit-action keys.
type sdnRouteMapEntryWire struct {
	RouteMapID string                 `json:"route-map-id,omitempty"`
	Order      *int64                 `json:"order,omitempty"`
	Action     string                 `json:"action,omitempty"`
	Call       string                 `json:"call,omitempty"`
	ExitAction *SdnRouteMapExitAction `json:"exit-action,omitempty"`
	Match      []SdnRouteMapKV        `json:"match,omitempty"`
	Set        []SdnRouteMapKV        `json:"set,omitempty"`
	Digest     string                 `json:"digest,omitempty"`
}

// MarshalJSON emits the wire shape.
func (e SdnRouteMapEntry) MarshalJSON() ([]byte, error) {
	// The wire mirror differs only in JSON tags; the conversion keeps the
	// fields lockstep without a field-by-field copy.
	return json.Marshal(sdnRouteMapEntryWire(e))
}

// UnmarshalJSON decodes the wire shape.
func (e *SdnRouteMapEntry) UnmarshalJSON(data []byte) error {
	var wire sdnRouteMapEntryWire
	if err := json.Unmarshal(data, &wire); err != nil {
		return fmt.Errorf("sdn route map entry: %w", err)
	}
	e.RouteMapID, e.Order, e.Action, e.Call = wire.RouteMapID, wire.Order, wire.Action, wire.Call
	e.ExitAction, e.Match, e.Set, e.Digest = wire.ExitAction, wire.Match, wire.Set, wire.Digest
	return nil
}

// SdnPrefixListsPath returns the prefix-list collection path.
func SdnPrefixListsPath() string { return "/cluster/sdn/prefix-lists" }

// sdnPrefixListPath returns the path of one prefix list.
func sdnPrefixListPath(id string) string { return SdnPrefixListsPath() + "/" + id }

// sdnPrefixListEntriesPath returns the entry collection path of one
// prefix list.
func sdnPrefixListEntriesPath(id string) string { return sdnPrefixListPath(id) + "/entries" }

// sdnPrefixListEntryPath returns the path of one prefix list entry.
func sdnPrefixListEntryPath(id string, seq int64) string {
	return sdnPrefixListEntriesPath(id) + "/" + strconv.FormatInt(seq, 10)
}

// SdnRouteMapEntriesPath returns the route-map entry collection path.
func SdnRouteMapEntriesPath() string { return "/cluster/sdn/route-maps/entries" }

// sdnRouteMapEntriesPath returns the entry collection path of one route
// map.
func sdnRouteMapEntriesPath(routeMapID string) string {
	return SdnRouteMapEntriesPath() + "/" + routeMapID
}

// sdnRouteMapEntryPath returns the path of one route map entry.
func sdnRouteMapEntryPath(routeMapID string, order int64) string {
	return sdnRouteMapEntriesPath(routeMapID) + "/entry/" + strconv.FormatInt(order, 10)
}

// sdnPrefixListCreateBody is the POST /cluster/sdn/prefix-lists request
// body; entries travel as property strings via SdnPrefixListEntry's
// MarshalJSON.
type sdnPrefixListCreateBody struct {
	ID      string               `json:"id"`
	Entries []SdnPrefixListEntry `json:"entries,omitempty"`
}

// sdnPrefixListEntryBody is the flat request body of the single-entry
// POST and PUT verbs, whose parameters the pin declares top level.
type sdnPrefixListEntryBody struct {
	Action string `json:"action,omitempty"`
	Prefix string `json:"prefix,omitempty"`
	Ge     *int64 `json:"ge,omitempty"`
	Le     *int64 `json:"le,omitempty"`
	Seq    *int64 `json:"seq,omitempty"`
}

// GetSdnPrefixList reads GET /cluster/sdn/prefix-lists/{id}.
func (c *Client) GetSdnPrefixList(ctx context.Context, id string) (*SdnPrefixList, error) {
	var wire struct {
		ID      string               `json:"id"`
		Entries []SdnPrefixListEntry `json:"entries"`
	}
	if err := c.Do(ctx, "GET", sdnPrefixListPath(id), nil, &wire); err != nil {
		return nil, err
	}
	return &SdnPrefixList{ID: wire.ID, Entries: wire.Entries}, nil
}

// CreateSdnPrefixList POSTs /cluster/sdn/prefix-lists with the list
// identifier and its initial entries.
func (c *Client) CreateSdnPrefixList(ctx context.Context, id string, entries []SdnPrefixListEntry) error {
	return c.Do(ctx, "POST", SdnPrefixListsPath(), sdnPrefixListCreateBody{ID: id, Entries: entries}, nil)
}

// DeleteSdnPrefixList DELETEs /cluster/sdn/prefix-lists/{id}.
func (c *Client) DeleteSdnPrefixList(ctx context.Context, id string) error {
	return c.Do(ctx, "DELETE", sdnPrefixListPath(id), nil, nil)
}

// ListSdnPrefixListEntries returns the entries of one prefix list from
// GET /cluster/sdn/prefix-lists/{id}/entries, in upstream order.
func (c *Client) ListSdnPrefixListEntries(ctx context.Context, id string) ([]SdnPrefixListEntry, error) {
	var entries []SdnPrefixListEntry
	if err := c.Do(ctx, "GET", sdnPrefixListEntriesPath(id), nil, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

// CreateSdnPrefixListEntry POSTs /cluster/sdn/prefix-lists/{id}/entries
// with the flat entry parameters. Seq is omitted when unset, letting
// PVE assign the position.
func (c *Client) CreateSdnPrefixListEntry(ctx context.Context, id string, entry SdnPrefixListEntry) error {
	return c.Do(ctx, "POST", sdnPrefixListEntriesPath(id), sdnPrefixListEntryBody{
		Action: entry.Action,
		Prefix: entry.Prefix,
		Ge:     entry.Ge,
		Le:     entry.Le,
		Seq:    entry.Seq,
	}, nil)
}

// UpdateSdnPrefixListEntry PUTs /cluster/sdn/prefix-lists/{id}/entries/
// {seq} with the flat entry parameters and translates deleteFields into
// PVE's `delete` query parameter.
func (c *Client) UpdateSdnPrefixListEntry(ctx context.Context, id string, seq int64, entry SdnPrefixListEntry, deleteFields []string) error {
	return c.Do(ctx, "PUT", firewallRulesDeleteQuery(sdnPrefixListEntryPath(id, seq), deleteFields), sdnPrefixListEntryBody{
		Action: entry.Action,
		Prefix: entry.Prefix,
		Ge:     entry.Ge,
		Le:     entry.Le,
	}, nil)
}

// DeleteSdnPrefixListEntry DELETEs /cluster/sdn/prefix-lists/{id}/
// entries/{seq}.
func (c *Client) DeleteSdnPrefixListEntry(ctx context.Context, id string, seq int64) error {
	return c.Do(ctx, "DELETE", sdnPrefixListEntryPath(id, seq), nil, nil)
}

// ListSdnRouteMapEntries returns the entries of one route map from GET
// /cluster/sdn/route-maps/entries/{route-map-id}, in upstream order.
func (c *Client) ListSdnRouteMapEntries(ctx context.Context, routeMapID string) ([]SdnRouteMapEntry, error) {
	var entries []SdnRouteMapEntry
	if err := c.Do(ctx, "GET", sdnRouteMapEntriesPath(routeMapID), nil, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

// CreateSdnRouteMapEntry POSTs /cluster/sdn/route-maps/entries with the
// entry; PVE materializes the route map through its first entry (the
// pin defines no route-map creation verb of its own).
func (c *Client) CreateSdnRouteMapEntry(ctx context.Context, entry SdnRouteMapEntry) error {
	return c.Do(ctx, "POST", SdnRouteMapEntriesPath(), entry, nil)
}

// UpdateSdnRouteMapEntry PUTs /cluster/sdn/route-maps/entries/
// {route-map-id}/entry/{order} with the entry and translates
// deleteFields into PVE's `delete` query parameter.
func (c *Client) UpdateSdnRouteMapEntry(ctx context.Context, routeMapID string, order int64, entry SdnRouteMapEntry, deleteFields []string) error {
	return c.Do(ctx, "PUT", firewallRulesDeleteQuery(sdnRouteMapEntryPath(routeMapID, order), deleteFields), entry, nil)
}

// DeleteSdnRouteMapEntry DELETEs /cluster/sdn/route-maps/entries/
// {route-map-id}/entry/{order}.
func (c *Client) DeleteSdnRouteMapEntry(ctx context.Context, routeMapID string, order int64) error {
	return c.Do(ctx, "DELETE", sdnRouteMapEntryPath(routeMapID, order), nil, nil)
}
