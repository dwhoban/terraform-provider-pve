// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Notification endpoint type path segments, as the pin defines them under
// /cluster/notifications/endpoints.
const (
	NotificationEndpointTypeSendmail = "sendmail"
	NotificationEndpointTypeGotify   = "gotify"
	NotificationEndpointTypeSMTP     = "smtp"
	NotificationEndpointTypeWebhook  = "webhook"
)

// NotificationEndpoint is a notification endpoint of any supported type
// (sendmail | gotify | smtp | webhook); which fields carry data depends on
// Type, the others stay at their zero value. Token and Password are
// write-only (PVE never returns them); the webhook Body and the header and
// secret values travel base64-encoded on the wire and are surfaced decoded.
type NotificationEndpoint struct {
	Type string
	Name string
	// Common optional fields.
	Comment string
	Disable *bool
	// sendmail / smtp mail fields.
	Author      string
	FromAddress string
	MailTo      []string
	MailToUser  []string
	// gotify / smtp server fields.
	Server string
	// gotify-only secret (write-only).
	Token string
	// smtp-only fields.
	Username string
	Password string // write-only
	Mode     string
	Port     *int64
	// webhook-only fields.
	URL     string
	Method  string
	Body    string
	Headers map[string]string
	Secrets map[string]string
	// Digest is decode-only (concurrency token from the single GET).
	Digest string
}

// NotificationEndpoints holds the four typed endpoint collections served by
// GET /cluster/notifications/endpoints/<type>.
type NotificationEndpoints struct {
	Sendmail []NotificationEndpoint
	Gotify   []NotificationEndpoint
	SMTP     []NotificationEndpoint
	Webhook  []NotificationEndpoint
}

// notificationEndpointWire mirrors the union wire shape of all endpoint
// types with the hyphenated keys PVE uses; lenient fields stay raw so the
// boolish/int decoders apply.
type notificationEndpointWire struct {
	Name        string          `json:"name"`
	Comment     string          `json:"comment,omitempty"`
	Disable     json.RawMessage `json:"disable,omitempty"`
	Digest      string          `json:"digest,omitempty"`
	Author      string          `json:"author,omitempty"`
	FromAddress string          `json:"from-address,omitempty"`
	MailTo      []string        `json:"mailto,omitempty"`
	MailToUser  []string        `json:"mailto-user,omitempty"`
	Server      string          `json:"server,omitempty"`
	Token       string          `json:"token,omitempty"`
	Username    string          `json:"username,omitempty"`
	Password    string          `json:"password,omitempty"`
	Mode        string          `json:"mode,omitempty"`
	Port        json.RawMessage `json:"port,omitempty"`
	URL         string          `json:"url,omitempty"`
	Method      string          `json:"method,omitempty"`
	Body        string          `json:"body,omitempty"`
	Header      []string        `json:"header,omitempty"`
	Secret      []string        `json:"secret,omitempty"`
}

// notificationEndpointCollectionPath resolves the pin's per-type collection
// path, rejecting unsupported endpoint types before a request is issued.
func notificationEndpointCollectionPath(endpointType string) (string, error) {
	switch endpointType {
	case NotificationEndpointTypeSendmail:
		return "/cluster/notifications/endpoints/sendmail", nil
	case NotificationEndpointTypeGotify:
		return "/cluster/notifications/endpoints/gotify", nil
	case NotificationEndpointTypeSMTP:
		return "/cluster/notifications/endpoints/smtp", nil
	case NotificationEndpointTypeWebhook:
		return "/cluster/notifications/endpoints/webhook", nil
	}
	return "", fmt.Errorf("notification endpoint type %q is not one of sendmail, gotify, smtp, webhook", endpointType)
}

// project converts the decoded wire shape into the typed endpoint. The
// union fields never collide across types, so no type knowledge is needed
// on decode; endpointType only labels the result.
func (w notificationEndpointWire) project(endpointType string) (NotificationEndpoint, error) {
	ep := NotificationEndpoint{
		Type:        endpointType,
		Name:        w.Name,
		Comment:     w.Comment,
		Disable:     nodeNetworkBoolishPtr(w.Disable),
		Digest:      w.Digest,
		Author:      w.Author,
		FromAddress: w.FromAddress,
		MailTo:      w.MailTo,
		MailToUser:  w.MailToUser,
		Server:      w.Server,
		Token:       w.Token,
		Username:    w.Username,
		Password:    w.Password,
		Mode:        w.Mode,
		Port:        haInt64PtrFromRaw(w.Port),
		URL:         w.URL,
		Method:      w.Method,
	}
	if w.Body != "" {
		raw, err := base64.StdEncoding.DecodeString(w.Body)
		if err != nil {
			return ep, fmt.Errorf("decoding webhook endpoint %s body: %w", w.Name, err)
		}
		ep.Body = string(raw)
	}
	headers, err := notificationEndpointPropertyStringsToMap(w.Header)
	if err != nil {
		return ep, fmt.Errorf("webhook endpoint %s: %w", w.Name, err)
	}
	ep.Headers = headers
	secrets, err := notificationEndpointPropertyStringsToMap(w.Secret)
	if err != nil {
		return ep, fmt.Errorf("webhook endpoint %s: %w", w.Name, err)
	}
	ep.Secrets = secrets
	return ep, nil
}

// MarshalJSON encodes the endpoint for its Type. PVE rejects unknown
// parameters (additionalProperties: 0), so only the type's own fields are
// emitted; write-only fields are omitted when empty.
func (e NotificationEndpoint) MarshalJSON() ([]byte, error) {
	common := notificationEndpointWire{
		Name:    e.Name,
		Comment: e.Comment,
		Disable: haBoolRawPtr(e.Disable),
		Digest:  e.Digest,
	}
	switch e.Type {
	case NotificationEndpointTypeSendmail:
		common.Author = e.Author
		common.FromAddress = e.FromAddress
		common.MailTo = e.MailTo
		common.MailToUser = e.MailToUser
		return json.Marshal(common)
	case NotificationEndpointTypeGotify:
		common.Server = e.Server
		common.Token = e.Token
		return json.Marshal(common)
	case NotificationEndpointTypeSMTP:
		common.Server = e.Server
		common.FromAddress = e.FromAddress
		common.Author = e.Author
		common.MailTo = e.MailTo
		common.MailToUser = e.MailToUser
		common.Username = e.Username
		common.Password = e.Password
		common.Mode = e.Mode
		common.Port = haInt64Raw(e.Port)
		return json.Marshal(common)
	case NotificationEndpointTypeWebhook:
		common.URL = e.URL
		common.Method = e.Method
		if e.Body != "" {
			common.Body = base64.StdEncoding.EncodeToString([]byte(e.Body))
		}
		common.Header = notificationEndpointPropertyStringsFromMap(e.Headers)
		common.Secret = notificationEndpointPropertyStringsFromMap(e.Secrets)
		return json.Marshal(common)
	}
	return nil, fmt.Errorf("notification endpoint type %q is not one of sendmail, gotify, smtp, webhook", e.Type)
}

// ListNotificationEndpoints returns all four typed endpoint collections.
func (c *Client) ListNotificationEndpoints(ctx context.Context) (*NotificationEndpoints, error) {
	sendmail, err := c.listNotificationEndpointsOfType(ctx, NotificationEndpointTypeSendmail)
	if err != nil {
		return nil, fmt.Errorf("listing sendmail endpoints: %w", err)
	}
	gotify, err := c.listNotificationEndpointsOfType(ctx, NotificationEndpointTypeGotify)
	if err != nil {
		return nil, fmt.Errorf("listing gotify endpoints: %w", err)
	}
	smtp, err := c.listNotificationEndpointsOfType(ctx, NotificationEndpointTypeSMTP)
	if err != nil {
		return nil, fmt.Errorf("listing smtp endpoints: %w", err)
	}
	webhook, err := c.listNotificationEndpointsOfType(ctx, NotificationEndpointTypeWebhook)
	if err != nil {
		return nil, fmt.Errorf("listing webhook endpoints: %w", err)
	}
	return &NotificationEndpoints{
		Sendmail: sendmail,
		Gotify:   gotify,
		SMTP:     smtp,
		Webhook:  webhook,
	}, nil
}

// listNotificationEndpointsOfType decodes one per-type collection.
func (c *Client) listNotificationEndpointsOfType(ctx context.Context, endpointType string) ([]NotificationEndpoint, error) {
	base, err := notificationEndpointCollectionPath(endpointType)
	if err != nil {
		return nil, err
	}
	var wire []notificationEndpointWire
	if err := c.Do(ctx, "GET", base, nil, &wire); err != nil {
		return nil, err
	}
	out := make([]NotificationEndpoint, 0, len(wire))
	for _, w := range wire {
		ep, err := w.project(endpointType)
		if err != nil {
			return nil, err
		}
		out = append(out, ep)
	}
	return out, nil
}

// GetNotificationEndpoint reads /cluster/notifications/endpoints/{type}/{name}.
func (c *Client) GetNotificationEndpoint(ctx context.Context, endpointType, name string) (*NotificationEndpoint, error) {
	base, err := notificationEndpointCollectionPath(endpointType)
	if err != nil {
		return nil, err
	}
	var wire notificationEndpointWire
	if err := c.Do(ctx, "GET", base+"/"+name, nil, &wire); err != nil {
		return nil, err
	}
	ep, err := wire.project(endpointType)
	if err != nil {
		return nil, err
	}
	return &ep, nil
}

// CreateNotificationEndpoint POSTs /cluster/notifications/endpoints/{type}.
func (c *Client) CreateNotificationEndpoint(ctx context.Context, ep NotificationEndpoint) error {
	base, err := notificationEndpointCollectionPath(ep.Type)
	if err != nil {
		return err
	}
	return c.Do(ctx, "POST", base, ep, nil)
}

// UpdateNotificationEndpoint PUTs
// /cluster/notifications/endpoints/{type}/{name}; deleteFields names the
// settings PVE should clear (its `delete` parameter).
func (c *Client) UpdateNotificationEndpoint(ctx context.Context, ep NotificationEndpoint, deleteFields []string) error {
	base, err := notificationEndpointCollectionPath(ep.Type)
	if err != nil {
		return err
	}
	return c.Do(ctx, "PUT", haDeleteQuery(base+"/"+ep.Name, deleteFields), ep, nil)
}

// DeleteNotificationEndpoint DELETEs
// /cluster/notifications/endpoints/{type}/{name}.
func (c *Client) DeleteNotificationEndpoint(ctx context.Context, endpointType, name string) error {
	base, err := notificationEndpointCollectionPath(endpointType)
	if err != nil {
		return err
	}
	return c.Do(ctx, "DELETE", base+"/"+name, nil, nil)
}

// notificationEndpointPropertyStringsToMap decodes PVE property strings of
// the form `name=<name>,value=<base64 of value>` into a plain map.
func notificationEndpointPropertyStringsToMap(items []string) (map[string]string, error) {
	if len(items) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(items))
	for _, item := range items {
		name, encoded, found := strings.Cut(item, ",value=")
		if !strings.HasPrefix(item, "name=") || !found {
			return nil, fmt.Errorf("entry %q is not a name=<name>,value=<base64> property string", item)
		}
		key := strings.TrimPrefix(name, "name=")
		raw, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, fmt.Errorf("decoding property string %q: %w", item, err)
		}
		out[key] = string(raw)
	}
	return out, nil
}

// notificationEndpointPropertyStringsFromMap encodes a plain map into PVE
// `name=<name>,value=<base64 of value>` property strings, sorted by name
// for deterministic request bodies.
func notificationEndpointPropertyStringsFromMap(m map[string]string) []string {
	if len(m) == 0 {
		return nil
	}
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]string, 0, len(names))
	for _, name := range names {
		out = append(out, "name="+name+",value="+base64.StdEncoding.EncodeToString([]byte(m[name])))
	}
	return out
}
