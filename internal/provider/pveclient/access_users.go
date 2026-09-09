// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// AccessUser mirrors a PVE user account as read from /access/users or
// written by POST /access/users and PUT /access/users/{userid}.
//
// Two wire quirks are normalized here:
//
//   - PVE's update_user endpoint has no `delete` parameter (verified against
//     the vendored apidoc pin and upstream pve-access-control source), so a
//     field is cleared by writing an empty value. MarshalJSON therefore emits
//     the writable string fields and groups unconditionally; the empty string
//     is the deliberate "clear" signal. On create the empty values are
//     ignored by PVE (falsy in the handler), so the same shape is safe.
//   - Groups travels as PVE's `pve-groupid-list` wire format: a single
//     comma-separated string on write and on the collection listing, an array
//     of strings on the per-user read. MarshalJSON/UnmarshalJSON translate
//     between []string and both forms; Enable tolerates the 0/1 integer
//     encoding older PVE versions emit.
type AccessUser struct {
	UserID    string   `json:"userid,omitempty"`
	Comment   string   `json:"comment,omitempty"`
	Email     string   `json:"email,omitempty"`
	Firstname string   `json:"firstname,omitempty"`
	Lastname  string   `json:"lastname,omitempty"`
	Keys      string   `json:"keys,omitempty"`
	Enable    *bool    `json:"enable,omitempty"`
	Expire    *int64   `json:"expire,omitempty"`
	Groups    []string `json:"-"`
	Password  string   `json:"password,omitempty"`
}

// accessUserRaw mirrors the wire shape of a user with the polymorphic fields
// (enable boolish, groups string-or-array) left as raw JSON.
type accessUserRaw struct {
	UserID    string          `json:"userid"`
	Comment   string          `json:"comment"`
	Email     string          `json:"email"`
	Firstname string          `json:"firstname"`
	Lastname  string          `json:"lastname"`
	Keys      string          `json:"keys"`
	Enable    json.RawMessage `json:"enable"`
	Expire    *int64          `json:"expire"`
	Groups    json.RawMessage `json:"groups"`
	Password  string          `json:"password"`
}

// MarshalJSON emits the writable string fields and groups unconditionally so
// updates carry explicit clear-to-empty values (see AccessUser), and encodes
// groups as the comma-separated list PVE expects on write.
func (u AccessUser) MarshalJSON() ([]byte, error) {
	body := struct {
		UserID    string `json:"userid,omitempty"`
		Comment   string `json:"comment"`
		Email     string `json:"email"`
		Firstname string `json:"firstname"`
		Lastname  string `json:"lastname"`
		Keys      string `json:"keys"`
		Enable    *bool  `json:"enable,omitempty"`
		Expire    *int64 `json:"expire,omitempty"`
		Groups    string `json:"groups"`
		Password  string `json:"password,omitempty"`
	}{
		UserID:    u.UserID,
		Comment:   u.Comment,
		Email:     u.Email,
		Firstname: u.Firstname,
		Lastname:  u.Lastname,
		Keys:      u.Keys,
		Enable:    u.Enable,
		Expire:    u.Expire,
		Groups:    strings.Join(u.Groups, ","),
		Password:  u.Password,
	}
	return json.Marshal(body)
}

// UnmarshalJSON accepts groups as either the array form (per-user read) or
// the comma-separated string form (collection listing), and enable as either
// bool or 0/1.
func (u *AccessUser) UnmarshalJSON(data []byte) error {
	var raw accessUserRaw
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	u.UserID = raw.UserID
	u.Comment = raw.Comment
	u.Email = raw.Email
	u.Firstname = raw.Firstname
	u.Lastname = raw.Lastname
	u.Keys = raw.Keys
	u.Enable = accessBoolishPtr(raw.Enable)
	u.Expire = raw.Expire
	if len(raw.Groups) > 0 && string(raw.Groups) != "null" {
		var list []string
		if err := json.Unmarshal(raw.Groups, &list); err == nil {
			u.Groups = list
		} else {
			var joined string
			if jerr := json.Unmarshal(raw.Groups, &joined); jerr != nil {
				return fmt.Errorf("pveclient: decode user groups: %w (array: %v)", jerr, err)
			}
			for _, g := range strings.Split(joined, ",") {
				if g = strings.TrimSpace(g); g != "" {
					u.Groups = append(u.Groups, g)
				}
			}
		}
	}
	u.Password = raw.Password
	return nil
}

// accessBoolishPtr decodes a raw boolish value (true/false/0/1) into a *bool,
// keeping nil for absent/null so callers can distinguish "unset" from
// "false". PVE emits enable and privsep as 0/1 integers from user.cfg on
// most versions and as real booleans on others.
func accessBoolishPtr(raw json.RawMessage) *bool {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	v := decodeBoolish(raw)
	return &v
}

// AccessToken mirrors a PVE API token as read from
// /access/users/{userid}/token[/{tokenid}] or written by POST/PUT
// /access/users/{userid}/token/{tokenid}.
//
// Value and FullTokenID are populated only by create (and by regenerate on
// update): PVE never returns the secret on reads.
type AccessToken struct {
	TokenID     string `json:"tokenid,omitempty"`
	Comment     string `json:"comment,omitempty"`
	Expire      *int64 `json:"expire,omitempty"`
	Privsep     *bool  `json:"privsep,omitempty"`
	Value       string `json:"value,omitempty"`
	FullTokenID string `json:"full-tokenid,omitempty"`
}

// accessTokenRaw mirrors the wire shape with privsep as raw JSON so the
// 0/1 encoding decodes.
type accessTokenRaw struct {
	TokenID     string          `json:"tokenid"`
	Comment     string          `json:"comment"`
	Expire      *int64          `json:"expire"`
	Privsep     json.RawMessage `json:"privsep"`
	Value       string          `json:"value"`
	FullTokenID string          `json:"full-tokenid"`
}

// UnmarshalJSON tolerates the int 0/1 encoding of Privsep that PVE emits
// from user.cfg.
func (t *AccessToken) UnmarshalJSON(data []byte) error {
	var raw accessTokenRaw
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	t.TokenID = raw.TokenID
	t.Comment = raw.Comment
	t.Expire = raw.Expire
	t.Privsep = accessBoolishPtr(raw.Privsep)
	t.Value = raw.Value
	t.FullTokenID = raw.FullTokenID
	return nil
}

// AccessTokenInfo mirrors the nested info block of the token generate
// response; GET and PUT return the same fields flat instead.
type AccessTokenInfo struct {
	Comment string `json:"comment"`
	Expire  *int64 `json:"expire"`
	Privsep *bool  `json:"privsep"`
}

// accessTokenInfoRaw mirrors the info block with privsep as raw JSON.
type accessTokenInfoRaw struct {
	Comment string          `json:"comment"`
	Expire  *int64          `json:"expire"`
	Privsep json.RawMessage `json:"privsep"`
}

// UnmarshalJSON tolerates the int 0/1 encoding of Privsep.
func (i *AccessTokenInfo) UnmarshalJSON(data []byte) error {
	var raw accessTokenInfoRaw
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	i.Comment = raw.Comment
	i.Expire = raw.Expire
	i.Privsep = accessBoolishPtr(raw.Privsep)
	return nil
}

// accessUserTokenGenerateResponse is the POST
// /access/users/{userid}/token/{tokenid} return shape: the one-time secret
// value plus a nested info block.
type accessUserTokenGenerateResponse struct {
	FullTokenID string          `json:"full-tokenid"`
	Info        AccessTokenInfo `json:"info"`
	Value       string          `json:"value"`
}

// ListAccessUsers returns the array from GET /access/users.
func (c *Client) ListAccessUsers(ctx context.Context) ([]AccessUser, error) {
	var users []AccessUser
	if err := c.Do(ctx, "GET", "/access/users", nil, &users); err != nil {
		return nil, fmt.Errorf("pveclient: list users: %w", err)
	}
	return users, nil
}

// CreateAccessUser POSTs /access/users. UserID is required; Password is only
// honored at create time.
func (c *Client) CreateAccessUser(ctx context.Context, user AccessUser) error {
	if user.UserID == "" {
		return fmt.Errorf("pveclient: create user: userid is required")
	}
	if err := c.Do(ctx, "POST", "/access/users", user, nil); err != nil {
		return fmt.Errorf("pveclient: create user %s: %w", user.UserID, err)
	}
	return nil
}

// GetAccessUser reads /access/users/{userid}.
func (c *Client) GetAccessUser(ctx context.Context, userid string) (*AccessUser, error) {
	var user AccessUser
	if err := c.Do(ctx, "GET", "/access/users/"+userid, nil, &user); err != nil {
		return nil, fmt.Errorf("pveclient: read user %s: %w", userid, err)
	}
	return &user, nil
}

// UpdateAccessUser PUTs /access/users/{userid} with the full field set;
// cleared fields travel as empty values because PVE's update_user has no
// `delete` parameter.
func (c *Client) UpdateAccessUser(ctx context.Context, userid string, user AccessUser) error {
	if err := c.Do(ctx, "PUT", "/access/users/"+userid, user, nil); err != nil {
		return fmt.Errorf("pveclient: update user %s: %w", userid, err)
	}
	return nil
}

// DeleteAccessUser deletes a user account.
func (c *Client) DeleteAccessUser(ctx context.Context, userid string) error {
	if err := c.Do(ctx, "DELETE", "/access/users/"+userid, nil, nil); err != nil {
		return fmt.Errorf("pveclient: delete user %s: %w", userid, err)
	}
	return nil
}

// ListAccessUserTokens returns the array from GET
// /access/users/{userid}/token.
func (c *Client) ListAccessUserTokens(ctx context.Context, userid string) ([]AccessToken, error) {
	var tokens []AccessToken
	if err := c.Do(ctx, "GET", "/access/users/"+userid+"/token", nil, &tokens); err != nil {
		return nil, fmt.Errorf("pveclient: list tokens of user %s: %w", userid, err)
	}
	return tokens, nil
}

// CreateAccessUserToken POSTs /access/users/{userid}/token/{tokenid} — the
// pin places token create on the tokenid leaf, not on the token collection —
// and returns the decoded response. Value carries the full secret
// `userid!tokenid=…` which PVE never returns again.
func (c *Client) CreateAccessUserToken(ctx context.Context, userid, tokenid string, tok AccessToken) (*AccessToken, error) {
	var wire accessUserTokenGenerateResponse
	if err := c.Do(ctx, "POST", "/access/users/"+userid+"/token/"+tokenid, tok, &wire); err != nil {
		return nil, fmt.Errorf("pveclient: create token %s for user %s: %w", tokenid, userid, err)
	}
	return &AccessToken{
		TokenID:     tokenid,
		Comment:     wire.Info.Comment,
		Expire:      wire.Info.Expire,
		Privsep:     wire.Info.Privsep,
		Value:       wire.Value,
		FullTokenID: wire.FullTokenID,
	}, nil
}

// GetAccessUserToken reads /access/users/{userid}/token/{tokenid}. Value is
// never populated by reads; PVE only returns it at create/regenerate.
func (c *Client) GetAccessUserToken(ctx context.Context, userid, tokenid string) (*AccessToken, error) {
	var tok AccessToken
	if err := c.Do(ctx, "GET", "/access/users/"+userid+"/token/"+tokenid, nil, &tok); err != nil {
		return nil, fmt.Errorf("pveclient: read token %s of user %s: %w", tokenid, userid, err)
	}
	tok.TokenID = tokenid
	return &tok, nil
}

// UpdateAccessUserToken PUTs /access/users/{userid}/token/{tokenid} and
// translates the delete slice into the PVE `delete` query parameter
// (comma-separated field names to clear — the token endpoint does have one,
// unlike the user endpoint). The PUT may return a new Value when PVE
// regenerates the secret.
func (c *Client) UpdateAccessUserToken(ctx context.Context, userid, tokenid string, tok AccessToken, deleteFields []string) (*AccessToken, error) {
	path := "/access/users/" + userid + "/token/" + tokenid
	if len(deleteFields) > 0 {
		path += "?delete=" + url.QueryEscape(strings.Join(deleteFields, ","))
	}
	var out AccessToken
	if err := c.Do(ctx, "PUT", path, tok, &out); err != nil {
		return nil, fmt.Errorf("pveclient: update token %s of user %s: %w", tokenid, userid, err)
	}
	return &out, nil
}

// DeleteAccessUserToken deletes an API token.
func (c *Client) DeleteAccessUserToken(ctx context.Context, userid, tokenid string) error {
	if err := c.Do(ctx, "DELETE", "/access/users/"+userid+"/token/"+tokenid, nil, nil); err != nil {
		return fmt.Errorf("pveclient: delete token %s of user %s: %w", tokenid, userid, err)
	}
	return nil
}
