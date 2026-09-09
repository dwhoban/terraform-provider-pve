// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// AccessGroup mirrors a single entry in the GET /access/groups index. The
// index carries `users` as one comma-separated userid string; use
// GetAccessGroup for the authoritative member array.
type AccessGroup struct {
	GroupID string  `json:"groupid"`
	Comment *string `json:"comment,omitempty"`
	Users   string  `json:"users,omitempty"`
}

// AccessGroupDetail mirrors GET /access/groups/{groupid}. Members are full
// user IDs in the `name@realm` format. Membership itself is managed through
// the user endpoints (PUT /access/users/{userid} `groups` parameter), never
// through the group endpoint.
type AccessGroupDetail struct {
	Comment *string  `json:"comment,omitempty"`
	Members []string `json:"members,omitempty"`
}

// accessGroupCreateBody is the POST /access/groups request shape. A nil
// Comment is omitted; a pointer to "" clears the comment upstream.
type accessGroupCreateBody struct {
	GroupID string  `json:"groupid"`
	Comment *string `json:"comment,omitempty"`
}

// accessGroupUpdateBody is the PUT /access/groups/{groupid} request shape.
// The PVE endpoint has no `delete` parameter, so clearing a comment is done
// by sending an empty string.
type accessGroupUpdateBody struct {
	Comment *string `json:"comment,omitempty"`
}

// AccessRole mirrors a single entry in the GET /access/roles index. Privs is
// the raw `pve-priv-list` wire string (comma-separated privilege names);
// Special marks the built-in roles shipped with PVE.
type AccessRole struct {
	RoleID  string `json:"roleid"`
	Privs   string `json:"privs,omitempty"`
	Special *bool  `json:"special,omitempty"`
}

// accessRoleRaw mirrors the wire shape of the role index with Special as a
// raw value, because PVE emits the flag as 0/1 on some versions and as a
// real boolean on others.
type accessRoleRaw struct {
	RoleID  string          `json:"roleid"`
	Privs   string          `json:"privs,omitempty"`
	Special json.RawMessage `json:"special,omitempty"`
}

// accessRoleCreateBody is the POST /access/roles request shape. Privs is the
// comma-joined `pve-priv-list`; an empty slice sends an empty string (an
// empty role is legal per the PVE schema).
type accessRoleCreateBody struct {
	RoleID string `json:"roleid"`
	Privs  string `json:"privs,omitempty"`
}

// accessRoleUpdateBody is the PUT /access/roles/{roleid} request shape. The
// provider always replaces the full privilege set, so the pin's `append`
// flag (which requires `privs` and merges into the existing set) is
// deliberately never sent.
type accessRoleUpdateBody struct {
	Privs string `json:"privs,omitempty"`
}

// ListAccessGroups returns the array from GET /access/groups.
func (c *Client) ListAccessGroups(ctx context.Context) ([]AccessGroup, error) {
	var out []AccessGroup
	if err := c.Do(ctx, "GET", "/access/groups", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetAccessGroup reads a single group via GET /access/groups/{groupid}.
func (c *Client) GetAccessGroup(ctx context.Context, groupID string) (*AccessGroupDetail, error) {
	var out AccessGroupDetail
	path := fmt.Sprintf("/access/groups/%s", groupID)
	if err := c.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateAccessGroup POSTs /access/groups. A nil comment creates the group
// without one.
func (c *Client) CreateAccessGroup(ctx context.Context, groupID string, comment *string) error {
	body := accessGroupCreateBody{GroupID: groupID, Comment: comment}
	return c.Do(ctx, "POST", "/access/groups", body, nil)
}

// UpdateAccessGroup PUTs /access/groups/{groupid}. The endpoint accepts only
// the comment: pass nil to leave it unchanged or a pointer to "" to clear
// it. Group membership is not settable here (user-side attribute).
func (c *Client) UpdateAccessGroup(ctx context.Context, groupID string, comment *string) error {
	body := accessGroupUpdateBody{Comment: comment}
	path := fmt.Sprintf("/access/groups/%s", groupID)
	return c.Do(ctx, "PUT", path, body, nil)
}

// DeleteAccessGroup deletes a single group.
func (c *Client) DeleteAccessGroup(ctx context.Context, groupID string) error {
	path := fmt.Sprintf("/access/groups/%s", groupID)
	return c.Do(ctx, "DELETE", path, nil, nil)
}

// ListAccessRoles returns the array from GET /access/roles.
func (c *Client) ListAccessRoles(ctx context.Context) ([]AccessRole, error) {
	var raw []accessRoleRaw
	if err := c.Do(ctx, "GET", "/access/roles", nil, &raw); err != nil {
		return nil, err
	}
	out := make([]AccessRole, 0, len(raw))
	for _, entry := range raw {
		role := AccessRole{RoleID: entry.RoleID, Privs: entry.Privs}
		if len(entry.Special) > 0 && string(entry.Special) != "null" {
			special := decodeBoolish(entry.Special)
			role.Special = &special
		}
		out = append(out, role)
	}
	return out, nil
}

// GetAccessRole reads GET /access/roles/{roleid}, whose response object maps
// each PVE privilege name to a boolean. Callers grant a privilege when the
// value is true.
func (c *Client) GetAccessRole(ctx context.Context, roleID string) (map[string]bool, error) {
	path := fmt.Sprintf("/access/roles/%s", roleID)
	var out map[string]bool
	if err := c.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// accessRolePrivList joins privilege names into the PVE `pve-priv-list`
// wire format: one comma-separated string.
func accessRolePrivList(privs []string) string {
	return strings.Join(privs, ",")
}

// CreateAccessRole POSTs /access/roles with the full privilege list.
func (c *Client) CreateAccessRole(ctx context.Context, roleID string, privs []string) error {
	body := accessRoleCreateBody{RoleID: roleID, Privs: accessRolePrivList(privs)}
	return c.Do(ctx, "POST", "/access/roles", body, nil)
}

// UpdateAccessRole PUTs /access/roles/{roleid}, replacing the role's full
// privilege set (the pin's `append` merge flag is never sent).
func (c *Client) UpdateAccessRole(ctx context.Context, roleID string, privs []string) error {
	body := accessRoleUpdateBody{Privs: accessRolePrivList(privs)}
	path := fmt.Sprintf("/access/roles/%s", roleID)
	return c.Do(ctx, "PUT", path, body, nil)
}

// DeleteAccessRole deletes a single role. PVE rejects deleting built-in
// (special) roles upstream; that error surfaces unchanged.
func (c *Client) DeleteAccessRole(ctx context.Context, roleID string) error {
	path := fmt.Sprintf("/access/roles/%s", roleID)
	return c.Do(ctx, "DELETE", path, nil, nil)
}
