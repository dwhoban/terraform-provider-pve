// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"context"
	"encoding/json"
	"fmt"
)

// NotificationMatcher is a notification matcher (GET/POST
// /cluster/notifications/matchers, GET/PUT/DELETE
// /cluster/notifications/matchers/{name}). The match lists carry their
// pin-declared entries verbatim:
//
//   - MatchField entries have the form `(regex|exact):<field>=<value>`.
//   - MatchSeverity entries are one of info, notice, warning, error, unknown.
//   - MatchCalendar entries are PVE calendar-event strings (e.g.
//     `sat..sun 02:30`).
//
// Target, MatchField, MatchSeverity, and MatchCalendar travel as JSON
// arrays per the pin. InvertMatch and Disable tolerate the 0/1 int
// encoding PVE emits on some versions; Origin (user-created | builtin |
// modified-builtin) and Digest are read-only.
type NotificationMatcher struct {
	Name          string   `json:"name"`
	Target        []string `json:"target,omitempty"`
	MatchField    []string `json:"match-field,omitempty"`
	MatchSeverity []string `json:"match-severity,omitempty"`
	MatchCalendar []string `json:"match-calendar,omitempty"`
	Mode          string   `json:"mode,omitempty"`
	InvertMatch   *bool    `json:"invert-match,omitempty"`
	Disable       *bool    `json:"disable,omitempty"`
	Comment       string   `json:"comment,omitempty"`
	Origin        string   `json:"origin,omitempty"`
	Digest        string   `json:"digest,omitempty"`
}

// notificationMatcherRaw mirrors the wire shape with the boolish fields as
// json.RawMessage and the hyphenated keys PVE uses on the wire.
type notificationMatcherRaw struct {
	Name          string          `json:"name"`
	Target        []string        `json:"target,omitempty"`
	MatchField    []string        `json:"match-field,omitempty"`
	MatchSeverity []string        `json:"match-severity,omitempty"`
	MatchCalendar []string        `json:"match-calendar,omitempty"`
	Mode          string          `json:"mode,omitempty"`
	InvertMatch   json.RawMessage `json:"invert-match,omitempty"`
	Disable       json.RawMessage `json:"disable,omitempty"`
	Comment       string          `json:"comment,omitempty"`
	Origin        string          `json:"origin,omitempty"`
	Digest        string          `json:"digest,omitempty"`
}

// UnmarshalJSON tolerates the 0/1 int encoding of the boolean fields that
// PVE emits on some versions.
func (m *NotificationMatcher) UnmarshalJSON(data []byte) error {
	var raw notificationMatcherRaw
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*m = NotificationMatcher{
		Name:          raw.Name,
		Target:        raw.Target,
		MatchField:    raw.MatchField,
		MatchSeverity: raw.MatchSeverity,
		MatchCalendar: raw.MatchCalendar,
		Mode:          raw.Mode,
		InvertMatch:   decodeBoolishPtr(raw.InvertMatch),
		Disable:       decodeBoolishPtr(raw.Disable),
		Comment:       raw.Comment,
		Origin:        raw.Origin,
		Digest:        raw.Digest,
	}
	return nil
}

// NotificationTarget is one entry of GET /cluster/notifications/targets,
// the flat list of everything that can receive notifications — user-created
// endpoints and built-ins (e.g. mail-to-root) alike. Disable tolerates the
// 0/1 int encoding on decode.
type NotificationTarget struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Comment string `json:"comment,omitempty"`
	Disable *bool  `json:"disable,omitempty"`
	Origin  string `json:"origin,omitempty"`
}

// notificationTargetRaw mirrors the wire shape of NotificationTarget with
// the boolish field as a raw value.
type notificationTargetRaw struct {
	Name    string          `json:"name"`
	Type    string          `json:"type"`
	Comment string          `json:"comment,omitempty"`
	Disable json.RawMessage `json:"disable,omitempty"`
	Origin  string          `json:"origin,omitempty"`
}

// UnmarshalJSON tolerates the 0/1 int encoding of the boolean field that
// PVE emits on some versions.
func (t *NotificationTarget) UnmarshalJSON(data []byte) error {
	var raw notificationTargetRaw
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*t = NotificationTarget{
		Name:    raw.Name,
		Type:    raw.Type,
		Comment: raw.Comment,
		Disable: decodeBoolishPtr(raw.Disable),
		Origin:  raw.Origin,
	}
	return nil
}

// ListNotificationMatchers returns the matchers from GET
// /cluster/notifications/matchers.
func (c *Client) ListNotificationMatchers(ctx context.Context) ([]NotificationMatcher, error) {
	var out []NotificationMatcher
	if err := c.Do(ctx, "GET", "/cluster/notifications/matchers", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// CreateNotificationMatcher POSTs /cluster/notifications/matchers with the
// supplied matcher. The call is synchronous per the pin (returns null).
func (c *Client) CreateNotificationMatcher(ctx context.Context, matcher NotificationMatcher) error {
	return c.Do(ctx, "POST", "/cluster/notifications/matchers", matcher, nil)
}

// GetNotificationMatcher reads /cluster/notifications/matchers/{name}.
func (c *Client) GetNotificationMatcher(ctx context.Context, name string) (*NotificationMatcher, error) {
	var out NotificationMatcher
	path := fmt.Sprintf("/cluster/notifications/matchers/%s", name)
	if err := c.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateNotificationMatcher PUTs /cluster/notifications/matchers/{name}
// with the supplied body and translates the delete slice into PVE's
// `delete` query parameter.
func (c *Client) UpdateNotificationMatcher(ctx context.Context, name string, body NotificationMatcher, deleteFields []string) error {
	path := haDeleteQuery(fmt.Sprintf("/cluster/notifications/matchers/%s", name), deleteFields)
	return c.Do(ctx, "PUT", path, body, nil)
}

// DeleteNotificationMatcher DELETEs /cluster/notifications/matchers/{name}.
func (c *Client) DeleteNotificationMatcher(ctx context.Context, name string) error {
	return c.Do(ctx, "DELETE", fmt.Sprintf("/cluster/notifications/matchers/%s", name), nil, nil)
}

// ListNotificationTargets returns the entries from GET
// /cluster/notifications/targets, including built-in targets.
func (c *Client) ListNotificationTargets(ctx context.Context) ([]NotificationTarget, error) {
	var out []NotificationTarget
	if err := c.Do(ctx, "GET", "/cluster/notifications/targets", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// TestNotificationTarget POSTs /cluster/notifications/targets/{name}/test,
// sending a test notification through the named target or matcher. The
// call is synchronous per the pin (returns null).
func (c *Client) TestNotificationTarget(ctx context.Context, name string) error {
	return c.Do(ctx, "POST", fmt.Sprintf("/cluster/notifications/targets/%s/test", name), nil, nil)
}
