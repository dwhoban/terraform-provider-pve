// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// AclEntry is one row of GET /access/acl. Propagate comes back as a real
// boolean on some PVE versions and as 0/1 integers on others (or is absent
// entirely, meaning the server default of propagate=1 applies), so it is
// decoded leniently into a *bool that stays nil when absent.
type AclEntry struct {
	Path      string `json:"path"`
	RoleID    string `json:"roleid"`
	Type      string `json:"type"`
	Ugid      string `json:"ugid,omitempty"`
	Propagate *bool  `json:"propagate,omitempty"`
}

// aclEntryRaw mirrors the wire shape with Propagate as json.RawMessage so
// the custom unmarshaler can accept either bool or int encodings.
type aclEntryRaw struct {
	Path      string          `json:"path"`
	RoleID    string          `json:"roleid"`
	Type      string          `json:"type"`
	Ugid      string          `json:"ugid"`
	Propagate json.RawMessage `json:"propagate"`
}

// UnmarshalJSON tolerates the 0/1 int encoding of Propagate that many PVE
// releases emit.
func (e *AclEntry) UnmarshalJSON(data []byte) error {
	var raw aclEntryRaw
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	e.Path = raw.Path
	e.RoleID = raw.RoleID
	e.Type = raw.Type
	e.Ugid = raw.Ugid
	e.Propagate = nodeNetworkBoolishPtr(raw.Propagate)
	return nil
}

// AclUpdate is the body of PUT /access/acl (update_acl). The pin defines no
// DELETE verb on this path: removal is a PUT with Delete set to true. Roles
// is required by the API; exactly one of Users, Groups, or Tokens carries
// the entry identity for a single-entry update.
type AclUpdate struct {
	Delete    bool   `json:"delete,omitempty"`
	Path      string `json:"path"`
	Roles     string `json:"roles"`
	Propagate *bool  `json:"propagate,omitempty"`
	Users     string `json:"users,omitempty"`
	Groups    string `json:"groups,omitempty"`
	Tokens    string `json:"tokens,omitempty"`
}

// AclPermissions mirrors GET /access/permissions: a map of path to a map of
// privilege to propagate flag. Values are decoded leniently because PVE
// emits both 0/1 integers and real booleans.
type AclPermissions map[string]map[string]bool

// UnmarshalJSON tolerates the 0/1 int encoding of the propagate flag.
func (p *AclPermissions) UnmarshalJSON(data []byte) error {
	var raw map[string]map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	out := make(AclPermissions, len(raw))
	for path, privs := range raw {
		row := make(map[string]bool, len(privs))
		for priv, v := range privs {
			row[priv] = decodeBoolish(v)
		}
		out[path] = row
	}
	*p = out
	return nil
}

// GetAcl returns the full access control list from GET /access/acl. The list
// is restricted by the server to entries the caller may modify.
func (c *Client) GetAcl(ctx context.Context) ([]AclEntry, error) {
	var out []AclEntry
	if err := c.Do(ctx, "GET", "/access/acl", nil, &out); err != nil {
		return nil, fmt.Errorf("pveclient: get /access/acl: %w", err)
	}
	return out, nil
}

// PutAcl adds or replaces an ACL entry via PUT /access/acl. The operation is
// synchronous (the pin returns null, no task UPID).
func (c *Client) PutAcl(ctx context.Context, update AclUpdate) error {
	return c.Do(ctx, "PUT", "/access/acl", update, nil)
}

// DeleteAcl removes one ACL entry. The pin defines no DELETE verb on
// /access/acl, so removal is PUT update_acl with delete=true and the
// entry's identity parameters.
func (c *Client) DeleteAcl(ctx context.Context, entry AclEntry) error {
	update := AclUpdate{Delete: true, Path: entry.Path, Roles: entry.RoleID}
	switch entry.Type {
	case "group":
		update.Groups = entry.Ugid
	case "token":
		update.Tokens = entry.Ugid
	default:
		update.Users = entry.Ugid
	}
	return c.PutAcl(ctx, update)
}

// GetPermissions retrieves effective permissions from GET /access/permissions.
// Both path and userid are optional server-side filters; empty strings are
// omitted from the query.
func (c *Client) GetPermissions(ctx context.Context, path, userid string) (AclPermissions, error) {
	query := url.Values{}
	if path != "" {
		query.Set("path", path)
	}
	if userid != "" {
		query.Set("userid", userid)
	}
	permsPath := "/access/permissions"
	if encoded := query.Encode(); encoded != "" {
		permsPath += "?" + encoded
	}
	var out AclPermissions
	if err := c.Do(ctx, "GET", permsPath, nil, &out); err != nil {
		return nil, fmt.Errorf("pveclient: get %s: %w", permsPath, err)
	}
	return out, nil
}
