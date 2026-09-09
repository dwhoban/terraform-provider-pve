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

// HABoolPtr returns a pointer to b, for building HA request structs.
func HABoolPtr(b bool) *bool { return &b }

// HAInt64Ptr returns a pointer to i, for building HA request structs.
func HAInt64Ptr(i int64) *int64 { return &i }

// HAStatusEntry mirrors one entry of the array returned by
// GET /cluster/ha/status/current. Entry shape depends on Type
// (quorum | master | lrm | service | fencing); fields not relevant to a
// type stay at their zero value. The boolish fields tolerate the 0/1 int
// encoding PVE emits on some versions.
type HAStatusEntry struct {
	ID            string `json:"id"`
	Type          string `json:"type"`
	Node          string `json:"node,omitempty"`
	Status        string `json:"status,omitempty"`
	State         string `json:"state,omitempty"`
	CRMState      string `json:"crm_state,omitempty"`
	RequestState  string `json:"request_state,omitempty"`
	SID           string `json:"sid,omitempty"`
	ArmedState    string `json:"armed_state,omitempty"`
	ResourceMode  string `json:"resource_mode,omitempty"`
	Quorate       *bool  `json:"-"`
	Timestamp     int64  `json:"-"`
	MaxRestart    *int64 `json:"-"`
	MaxRelocate   *int64 `json:"-"`
	Failback      *bool  `json:"-"`
	AutoRebalance *bool  `json:"-"`
}

// haStatusEntryRaw mirrors the wire shape with the lenient fields as
// json.RawMessage and the hyphenated keys PVE uses on the wire.
type haStatusEntryRaw struct {
	ID            string          `json:"id"`
	Type          string          `json:"type"`
	Node          string          `json:"node,omitempty"`
	Status        string          `json:"status,omitempty"`
	State         string          `json:"state,omitempty"`
	CRMState      string          `json:"crm_state,omitempty"`
	RequestState  string          `json:"request_state,omitempty"`
	SID           string          `json:"sid,omitempty"`
	ArmedState    string          `json:"armed-state,omitempty"`
	ResourceMode  string          `json:"resource-mode,omitempty"`
	Quorate       json.RawMessage `json:"quorate,omitempty"`
	Timestamp     int64           `json:"timestamp,omitempty"`
	MaxRestart    json.RawMessage `json:"max_restart,omitempty"`
	MaxRelocate   json.RawMessage `json:"max_relocate,omitempty"`
	Failback      json.RawMessage `json:"failback,omitempty"`
	AutoRebalance json.RawMessage `json:"auto-rebalance,omitempty"`
}

// UnmarshalJSON tolerates the 0/1 int encoding of the boolean fields that
// PVE emits on some versions.
func (e *HAStatusEntry) UnmarshalJSON(data []byte) error {
	var raw haStatusEntryRaw
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	e.ID = raw.ID
	e.Type = raw.Type
	e.Node = raw.Node
	e.Status = raw.Status
	e.State = raw.State
	e.CRMState = raw.CRMState
	e.RequestState = raw.RequestState
	e.SID = raw.SID
	e.ArmedState = raw.ArmedState
	e.ResourceMode = raw.ResourceMode
	e.Quorate = nodeNetworkBoolishPtr(raw.Quorate)
	e.Timestamp = raw.Timestamp
	e.MaxRestart = haInt64PtrFromRaw(raw.MaxRestart)
	e.MaxRelocate = haInt64PtrFromRaw(raw.MaxRelocate)
	e.Failback = nodeNetworkBoolishPtr(raw.Failback)
	e.AutoRebalance = nodeNetworkBoolishPtr(raw.AutoRebalance)
	return nil
}

// haInt64PtrFromRaw decodes a raw JSON number into an *int64, keeping nil
// for absent or null values.
func haInt64PtrFromRaw(raw json.RawMessage) *int64 {
	if len(raw) == 0 {
		return nil
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil
	}
	v, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil {
		return nil
	}
	return &v
}

// HAGroup is an HA group (GET/POST/PUT/DELETE /cluster/ha/groups).
// Nodes carries `<node>[:<priority>]` entries; the wire encodes them as a
// single comma-separated string. Restricted and NoFailback tolerate the
// 0/1 int encoding on decode.
type HAGroup struct {
	Group      string   `json:"group"`
	Nodes      []string `json:"-"`
	Restricted *bool    `json:"-"`
	NoFailback *bool    `json:"-"`
	Comment    string   `json:"-"`
	Digest     string   `json:"digest,omitempty"`
}

// haGroupWire mirrors the wire shape of HAGroup.
type haGroupWire struct {
	Group      string          `json:"group,omitempty"`
	Nodes      string          `json:"nodes,omitempty"`
	Restricted json.RawMessage `json:"restricted,omitempty"`
	NoFailback json.RawMessage `json:"nofailback,omitempty"`
	Comment    string          `json:"comment,omitempty"`
	Digest     string          `json:"digest,omitempty"`
}

// MarshalJSON encodes the node list as PVE's comma-separated wire string.
func (g HAGroup) MarshalJSON() ([]byte, error) {
	w := haGroupWire{
		Group:   g.Group,
		Nodes:   strings.Join(g.Nodes, ","),
		Comment: g.Comment,
		Digest:  g.Digest,
	}
	if g.Restricted != nil {
		w.Restricted = haBoolRaw(*g.Restricted)
	}
	if g.NoFailback != nil {
		w.NoFailback = haBoolRaw(*g.NoFailback)
	}
	return json.Marshal(w)
}

// UnmarshalJSON splits the wire node list and decodes the boolish flags.
func (g *HAGroup) UnmarshalJSON(data []byte) error {
	var w haGroupWire
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	g.Group = w.Group
	g.Nodes = haSplitList(w.Nodes)
	g.Restricted = nodeNetworkBoolishPtr(w.Restricted)
	g.NoFailback = nodeNetworkBoolishPtr(w.NoFailback)
	g.Comment = w.Comment
	g.Digest = w.Digest
	return nil
}

// HAResource is an HA resource (GET/POST/PUT/DELETE
// /cluster/ha/resources). SID is `<type>:<name>` (e.g. vm:100, ct:101).
// AutoRebalance maps to the hyphenated wire key `auto-rebalance`; the
// boolish fields tolerate the 0/1 int encoding on decode.
type HAResource struct {
	SID           string `json:"sid"`
	Type          string `json:"type,omitempty"`
	State         string `json:"state,omitempty"`
	Group         string `json:"group,omitempty"`
	Comment       string `json:"comment,omitempty"`
	Digest        string `json:"digest,omitempty"`
	MaxRestart    *int64 `json:"-"`
	MaxRelocate   *int64 `json:"-"`
	Failback      *bool  `json:"-"`
	AutoRebalance *bool  `json:"-"`
}

// haResourceWire mirrors the wire shape of HAResource.
type haResourceWire struct {
	SID           string          `json:"sid,omitempty"`
	Type          string          `json:"type,omitempty"`
	State         string          `json:"state,omitempty"`
	Group         string          `json:"group,omitempty"`
	Comment       string          `json:"comment,omitempty"`
	Digest        string          `json:"digest,omitempty"`
	MaxRestart    json.RawMessage `json:"max_restart,omitempty"`
	MaxRelocate   json.RawMessage `json:"max_relocate,omitempty"`
	Failback      json.RawMessage `json:"failback,omitempty"`
	AutoRebalance json.RawMessage `json:"auto-rebalance,omitempty"`
}

// MarshalJSON emits the hyphenated auto-rebalance key.
func (r HAResource) MarshalJSON() ([]byte, error) {
	w := haResourceWire{
		SID:         r.SID,
		Type:        r.Type,
		State:       r.State,
		Group:       r.Group,
		Comment:     r.Comment,
		Digest:      r.Digest,
		MaxRestart:  haInt64Raw(r.MaxRestart),
		MaxRelocate: haInt64Raw(r.MaxRelocate),
		Failback:    haBoolRawPtr(r.Failback),
	}
	if r.AutoRebalance != nil {
		w.AutoRebalance = haBoolRaw(*r.AutoRebalance)
	}
	return json.Marshal(w)
}

// UnmarshalJSON decodes the boolish and lenient numeric fields.
func (r *HAResource) UnmarshalJSON(data []byte) error {
	var w haResourceWire
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	r.SID = w.SID
	r.Type = w.Type
	r.State = w.State
	r.Group = w.Group
	r.Comment = w.Comment
	r.Digest = w.Digest
	r.MaxRestart = haInt64PtrFromRaw(w.MaxRestart)
	r.MaxRelocate = haInt64PtrFromRaw(w.MaxRelocate)
	r.Failback = nodeNetworkBoolishPtr(w.Failback)
	r.AutoRebalance = nodeNetworkBoolishPtr(w.AutoRebalance)
	return nil
}

// HARule is an HA rule (GET/POST/PUT/DELETE /cluster/ha/rules). Type is
// the pin's rule type (node-affinity | resource-affinity); Nodes and
// Resources carry their list entries and are encoded as PVE's
// comma-separated wire strings. Strict and Disable tolerate the 0/1 int
// encoding on decode.
type HARule struct {
	Rule      string   `json:"rule"`
	Type      string   `json:"type,omitempty"`
	Affinity  string   `json:"affinity,omitempty"`
	Comment   string   `json:"-"`
	Digest    string   `json:"digest,omitempty"`
	Nodes     []string `json:"-"`
	Resources []string `json:"-"`
	Strict    *bool    `json:"-"`
	Disable   *bool    `json:"-"`
}

// haRuleWire mirrors the wire shape of HARule.
type haRuleWire struct {
	Rule      string          `json:"rule,omitempty"`
	Type      string          `json:"type,omitempty"`
	Affinity  string          `json:"affinity,omitempty"`
	Comment   string          `json:"comment,omitempty"`
	Digest    string          `json:"digest,omitempty"`
	Nodes     string          `json:"nodes,omitempty"`
	Resources string          `json:"resources,omitempty"`
	Strict    json.RawMessage `json:"strict,omitempty"`
	Disable   json.RawMessage `json:"disable,omitempty"`
}

// MarshalJSON encodes the node and resource lists as PVE's
// comma-separated wire strings.
func (r HARule) MarshalJSON() ([]byte, error) {
	w := haRuleWire{
		Rule:      r.Rule,
		Type:      r.Type,
		Affinity:  r.Affinity,
		Comment:   r.Comment,
		Digest:    r.Digest,
		Nodes:     strings.Join(r.Nodes, ","),
		Resources: strings.Join(r.Resources, ","),
	}
	if r.Strict != nil {
		w.Strict = haBoolRaw(*r.Strict)
	}
	if r.Disable != nil {
		w.Disable = haBoolRaw(*r.Disable)
	}
	return json.Marshal(w)
}

// UnmarshalJSON splits the wire lists and decodes the boolish flags.
func (r *HARule) UnmarshalJSON(data []byte) error {
	var w haRuleWire
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	r.Rule = w.Rule
	r.Type = w.Type
	r.Affinity = w.Affinity
	r.Comment = w.Comment
	r.Digest = w.Digest
	r.Nodes = haSplitList(w.Nodes)
	r.Resources = haSplitList(w.Resources)
	r.Strict = nodeNetworkBoolishPtr(w.Strict)
	r.Disable = nodeNetworkBoolishPtr(w.Disable)
	return nil
}

// haBoolRaw encodes a bool as a raw JSON value.
func haBoolRaw(b bool) json.RawMessage {
	if b {
		return json.RawMessage("true")
	}
	return json.RawMessage("false")
}

// haBoolRawPtr encodes an optional bool, returning nil for a nil pointer.
func haBoolRawPtr(b *bool) json.RawMessage {
	if b == nil {
		return nil
	}
	return haBoolRaw(*b)
}

// haInt64Raw encodes an optional int64, returning nil for a nil pointer.
func haInt64Raw(i *int64) json.RawMessage {
	if i == nil {
		return nil
	}
	return json.RawMessage(strconv.FormatInt(*i, 10))
}

// haSplitList splits PVE's comma-separated list encoding into entries,
// dropping empty segments.
func haSplitList(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// haDeleteQuery appends PVE's `delete` query parameter (comma-separated
// field names to clear) to a path when fields are supplied.
func haDeleteQuery(path string, deleteFields []string) string {
	if len(deleteFields) == 0 {
		return path
	}
	return path + "?delete=" + url.QueryEscape(strings.Join(deleteFields, ","))
}

// GetHAStatus returns the entry array from GET /cluster/ha/status/current.
func (c *Client) GetHAStatus(ctx context.Context) ([]HAStatusEntry, error) {
	var out []HAStatusEntry
	if err := c.Do(ctx, "GET", "/cluster/ha/status/current", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetHAManagerStatus returns the raw document from GET
// /cluster/ha/status/manager_status. The pin declares the response only as
// "object" (the CRM's internal status document), so it is surfaced
// verbatim as raw JSON instead of being projected onto guessed fields.
func (c *Client) GetHAManagerStatus(ctx context.Context) (json.RawMessage, error) {
	var out json.RawMessage
	if err := c.Do(ctx, "GET", "/cluster/ha/status/manager_status", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ArmHA POSTs /cluster/ha/status/arm-ha to re-arm the HA stack after it
// was disarmed. The call is synchronous (returns null).
func (c *Client) ArmHA(ctx context.Context) error {
	return c.Do(ctx, "POST", "/cluster/ha/status/arm-ha", nil, nil)
}

// DisarmHA POSTs /cluster/ha/status/disarm-ha to disarm the HA stack and
// release all watchdogs cluster-wide. resourceMode optionally controls how
// HA-managed resources are handled while disarmed (freeze | ignore).
func (c *Client) DisarmHA(ctx context.Context, resourceMode string) error {
	var body any
	if resourceMode != "" {
		body = map[string]string{"resource-mode": resourceMode}
	}
	return c.Do(ctx, "POST", "/cluster/ha/status/disarm-ha", body, nil)
}

// ListHAGroups returns the groups from GET /cluster/ha/groups.
func (c *Client) ListHAGroups(ctx context.Context) ([]HAGroup, error) {
	var out []HAGroup
	if err := c.Do(ctx, "GET", "/cluster/ha/groups", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// CreateHAGroup POSTs /cluster/ha/groups with the supplied group.
func (c *Client) CreateHAGroup(ctx context.Context, group HAGroup) error {
	return c.Do(ctx, "POST", "/cluster/ha/groups", group, nil)
}

// GetHAGroup reads /cluster/ha/groups/{group}.
func (c *Client) GetHAGroup(ctx context.Context, group string) (*HAGroup, error) {
	var out HAGroup
	path := fmt.Sprintf("/cluster/ha/groups/%s", group)
	if err := c.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateHAGroup PUTs /cluster/ha/groups/{group} with the supplied body and
// translates the delete slice into PVE's `delete` query parameter.
func (c *Client) UpdateHAGroup(ctx context.Context, group string, body HAGroup, deleteFields []string) error {
	path := haDeleteQuery(fmt.Sprintf("/cluster/ha/groups/%s", group), deleteFields)
	return c.Do(ctx, "PUT", path, body, nil)
}

// DeleteHAGroup DELETEs /cluster/ha/groups/{group}.
func (c *Client) DeleteHAGroup(ctx context.Context, group string) error {
	return c.Do(ctx, "DELETE", fmt.Sprintf("/cluster/ha/groups/%s", group), nil, nil)
}

// ListHAResources returns the resources from GET /cluster/ha/resources.
// resourceType optionally filters the list (ct | vm).
func (c *Client) ListHAResources(ctx context.Context, resourceType string) ([]HAResource, error) {
	path := "/cluster/ha/resources"
	if resourceType != "" {
		path += "?type=" + url.QueryEscape(resourceType)
	}
	var out []HAResource
	if err := c.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// CreateHAResource POSTs /cluster/ha/resources with the supplied resource.
func (c *Client) CreateHAResource(ctx context.Context, res HAResource) error {
	return c.Do(ctx, "POST", "/cluster/ha/resources", res, nil)
}

// GetHAResource reads /cluster/ha/resources/{sid}.
func (c *Client) GetHAResource(ctx context.Context, sid string) (*HAResource, error) {
	var out HAResource
	path := fmt.Sprintf("/cluster/ha/resources/%s", sid)
	if err := c.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateHAResource PUTs /cluster/ha/resources/{sid} with the supplied body
// and translates the delete slice into PVE's `delete` query parameter.
func (c *Client) UpdateHAResource(ctx context.Context, sid string, body HAResource, deleteFields []string) error {
	path := haDeleteQuery(fmt.Sprintf("/cluster/ha/resources/%s", sid), deleteFields)
	return c.Do(ctx, "PUT", path, body, nil)
}

// DeleteHAResource DELETEs /cluster/ha/resources/{sid}. purge controls
// whether the resource is removed from rules referencing it (PVE default:
// true, deleting rules where it was the only member).
func (c *Client) DeleteHAResource(ctx context.Context, sid string, purge bool) error {
	path := fmt.Sprintf("/cluster/ha/resources/%s?purge=%t", sid, purge)
	return c.Do(ctx, "DELETE", path, nil, nil)
}

// ListHARules returns the rules from GET /cluster/ha/rules. ruleType
// optionally filters the list (node-affinity | resource-affinity).
func (c *Client) ListHARules(ctx context.Context, ruleType string) ([]HARule, error) {
	path := "/cluster/ha/rules"
	if ruleType != "" {
		path += "?type=" + url.QueryEscape(ruleType)
	}
	var out []HARule
	if err := c.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// CreateHARule POSTs /cluster/ha/rules with the supplied rule.
func (c *Client) CreateHARule(ctx context.Context, rule HARule) error {
	return c.Do(ctx, "POST", "/cluster/ha/rules", rule, nil)
}

// GetHARule reads /cluster/ha/rules/{rule}.
func (c *Client) GetHARule(ctx context.Context, rule string) (*HARule, error) {
	var out HARule
	path := fmt.Sprintf("/cluster/ha/rules/%s", rule)
	if err := c.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateHARule PUTs /cluster/ha/rules/{rule} with the supplied body and
// translates the delete slice into PVE's `delete` query parameter.
func (c *Client) UpdateHARule(ctx context.Context, rule string, body HARule, deleteFields []string) error {
	path := haDeleteQuery(fmt.Sprintf("/cluster/ha/rules/%s", rule), deleteFields)
	return c.Do(ctx, "PUT", path, body, nil)
}

// DeleteHARule DELETEs /cluster/ha/rules/{rule}.
func (c *Client) DeleteHARule(ctx context.Context, rule string) error {
	return c.Do(ctx, "DELETE", fmt.Sprintf("/cluster/ha/rules/%s", rule), nil, nil)
}
