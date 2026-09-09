// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package pveclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
)

// Domain mirrors one authentication realm (auth server) in PVE. The pin
// (apidoc.js /access/domains) defines one flat parameter set for every
// realm type; PVE validates the per-type subset server-side, so consumers
// project only the fields relevant to their fixed `type` (ldap | ad |
// openid | pam | pve).
//
// PVE stores realm options in a section config, so GET responses may
// encode booleans as quoted "1"/"0" strings (or bare 1/0 integers on
// newer releases) and the port as a string; the custom UnmarshalJSON
// accepts every encoding. Password and client-key are write-only-ish:
// PVE either never returns them or returns the stored value, so callers
// decide how to merge them back into state.
type Domain struct {
	Realm   string `json:"realm,omitempty"`
	Type    string `json:"type,omitempty"`
	Comment string `json:"comment,omitempty"`
	Default *bool  `json:"default,omitempty"`
	TFA     string `json:"tfa,omitempty"`
	Digest  string `json:"digest,omitempty"`

	// LDAP and AD.
	Server1             string `json:"server1,omitempty"`
	Server2             string `json:"server2,omitempty"`
	Port                *int   `json:"port,omitempty"`
	Mode                string `json:"mode,omitempty"`
	Secure              *bool  `json:"secure,omitempty"`
	Verify              *bool  `json:"verify,omitempty"`
	Capath              string `json:"capath,omitempty"`
	Cert                string `json:"cert,omitempty"`
	CertKey             string `json:"certkey,omitempty"`
	SSLVersion          string `json:"sslversion,omitempty"`
	BaseDN              string `json:"base_dn,omitempty"`
	BindDN              string `json:"bind_dn,omitempty"`
	Password            string `json:"password,omitempty"`
	UserAttr            string `json:"user_attr,omitempty"`
	UserClasses         string `json:"user_classes,omitempty"`
	GroupClasses        string `json:"group_classes,omitempty"`
	GroupDN             string `json:"group_dn,omitempty"`
	GroupFilter         string `json:"group_filter,omitempty"`
	GroupNameAttr       string `json:"group_name_attr,omitempty"`
	Filter              string `json:"filter,omitempty"`
	SyncAttributes      string `json:"sync_attributes,omitempty"`
	SyncDefaultsOptions string `json:"sync-defaults-options,omitempty"`
	CaseSensitive       *bool  `json:"case-sensitive,omitempty"`
	CheckConnection     *bool  `json:"check-connection,omitempty"`
	Domain              string `json:"domain,omitempty"`

	// OpenID.
	IssuerURL        string `json:"issuer-url,omitempty"`
	ClientID         string `json:"client-id,omitempty"`
	ClientKey        string `json:"client-key,omitempty"`
	UsernameClaim    string `json:"username-claim,omitempty"`
	GroupsClaim      string `json:"groups-claim,omitempty"`
	GroupsAutocreate *bool  `json:"groups-autocreate,omitempty"`
	GroupsOverwrite  *bool  `json:"groups-overwrite,omitempty"`
	Autocreate       *bool  `json:"autocreate,omitempty"`
	QueryUserinfo    *bool  `json:"query-userinfo,omitempty"`
	Scopes           string `json:"scopes,omitempty"`
	Prompt           string `json:"prompt,omitempty"`
	ACRValues        string `json:"acr-values,omitempty"`
	Audiences        string `json:"audiences,omitempty"`

	// Delete carries the PUT-only list of settings to clear
	// (comma-separated PVE field names).
	Delete string `json:"delete,omitempty"`
}

// domainRaw mirrors Domain with the lenient fields as json.RawMessage.
type domainRaw struct {
	Realm            string          `json:"realm"`
	Type             string          `json:"type"`
	Comment          string          `json:"comment"`
	Default          json.RawMessage `json:"default"`
	TFA              string          `json:"tfa"`
	Digest           string          `json:"digest"`
	Server1          string          `json:"server1"`
	Server2          string          `json:"server2"`
	Port             json.RawMessage `json:"port"`
	Mode             string          `json:"mode"`
	Secure           json.RawMessage `json:"secure"`
	Verify           json.RawMessage `json:"verify"`
	Capath           string          `json:"capath"`
	Cert             string          `json:"cert"`
	CertKey          string          `json:"certkey"`
	SSLVersion       string          `json:"sslversion"`
	BaseDN           string          `json:"base_dn"`
	BindDN           string          `json:"bind_dn"`
	Password         string          `json:"password"`
	UserAttr         string          `json:"user_attr"`
	UserClasses      string          `json:"user_classes"`
	GroupClasses     string          `json:"group_classes"`
	GroupDN          string          `json:"group_dn"`
	GroupFilter      string          `json:"group_filter"`
	GroupNameAttr    string          `json:"group_name_attr"`
	Filter           string          `json:"filter"`
	SyncAttributes   string          `json:"sync_attributes"`
	SyncDefaults     string          `json:"sync-defaults-options"`
	CaseSensitive    json.RawMessage `json:"case-sensitive"`
	CheckConnection  json.RawMessage `json:"check-connection"`
	Domain           string          `json:"domain"`
	IssuerURL        string          `json:"issuer-url"`
	ClientID         string          `json:"client-id"`
	ClientKey        string          `json:"client-key"`
	UsernameClaim    string          `json:"username-claim"`
	GroupsClaim      string          `json:"groups-claim"`
	GroupsAutocreate json.RawMessage `json:"groups-autocreate"`
	GroupsOverwrite  json.RawMessage `json:"groups-overwrite"`
	Autocreate       json.RawMessage `json:"autocreate"`
	QueryUserinfo    json.RawMessage `json:"query-userinfo"`
	Scopes           string          `json:"scopes"`
	Prompt           string          `json:"prompt"`
	ACRValues        string          `json:"acr-values"`
	Audiences        string          `json:"audiences"`
	Delete           string          `json:"delete"`
}

// UnmarshalJSON tolerates the string/bool/int encodings PVE section configs
// emit for realm options.
func (d *Domain) UnmarshalJSON(data []byte) error {
	var raw domainRaw
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	d.Realm = raw.Realm
	d.Type = raw.Type
	d.Comment = raw.Comment
	d.Default = accessDomainsBoolPtr(raw.Default)
	d.TFA = raw.TFA
	d.Digest = raw.Digest
	d.Server1 = raw.Server1
	d.Server2 = raw.Server2
	d.Port = accessDomainsIntPtr(raw.Port)
	d.Mode = raw.Mode
	d.Secure = accessDomainsBoolPtr(raw.Secure)
	d.Verify = accessDomainsBoolPtr(raw.Verify)
	d.Capath = raw.Capath
	d.Cert = raw.Cert
	d.CertKey = raw.CertKey
	d.SSLVersion = raw.SSLVersion
	d.BaseDN = raw.BaseDN
	d.BindDN = raw.BindDN
	d.Password = raw.Password
	d.UserAttr = raw.UserAttr
	d.UserClasses = raw.UserClasses
	d.GroupClasses = raw.GroupClasses
	d.GroupDN = raw.GroupDN
	d.GroupFilter = raw.GroupFilter
	d.GroupNameAttr = raw.GroupNameAttr
	d.Filter = raw.Filter
	d.SyncAttributes = raw.SyncAttributes
	d.SyncDefaultsOptions = raw.SyncDefaults
	d.CaseSensitive = accessDomainsBoolPtr(raw.CaseSensitive)
	d.CheckConnection = accessDomainsBoolPtr(raw.CheckConnection)
	d.Domain = raw.Domain
	d.IssuerURL = raw.IssuerURL
	d.ClientID = raw.ClientID
	d.ClientKey = raw.ClientKey
	d.UsernameClaim = raw.UsernameClaim
	d.GroupsClaim = raw.GroupsClaim
	d.GroupsAutocreate = accessDomainsBoolPtr(raw.GroupsAutocreate)
	d.GroupsOverwrite = accessDomainsBoolPtr(raw.GroupsOverwrite)
	d.Autocreate = accessDomainsBoolPtr(raw.Autocreate)
	d.QueryUserinfo = accessDomainsBoolPtr(raw.QueryUserinfo)
	d.Scopes = raw.Scopes
	d.Prompt = raw.Prompt
	d.ACRValues = raw.ACRValues
	d.Audiences = raw.Audiences
	d.Delete = raw.Delete
	return nil
}

// accessDomainsBoolPtr decodes a raw boolish value into a *bool, keeping
// nil for absent/null. PVE section configs store booleans as quoted
// "1"/"0" strings, so quotes are stripped before the boolish decode.
func accessDomainsBoolPtr(raw json.RawMessage) *bool {
	trimmed := bytes.Trim(bytes.TrimSpace(raw), `"`)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil
	}
	v := decodeBoolish(trimmed)
	return &v
}

// accessDomainsIntPtr decodes a raw port value into a *int, accepting the
// int and quoted-string encodings PVE emits.
func accessDomainsIntPtr(raw json.RawMessage) *int {
	trimmed := bytes.Trim(bytes.TrimSpace(raw), `"`)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil
	}
	v, err := strconv.Atoi(string(trimmed))
	if err != nil {
		return nil
	}
	return &v
}

// ListDomains returns the array from GET /access/domains. Built-in realms
// (pam, pve) are included; the index carries only realm, type, comment,
// and tfa per entry.
func (c *Client) ListDomains(ctx context.Context) ([]Domain, error) {
	var out []Domain
	if err := c.Do(ctx, "GET", "/access/domains", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// CreateDomain POSTs /access/domains with the supplied realm definition.
// The caller supplies Realm and Type; everything else is sent as-is.
func (c *Client) CreateDomain(ctx context.Context, domain Domain) error {
	return c.Do(ctx, "POST", "/access/domains", domain, nil)
}

// GetDomain reads GET /access/domains/{realm} (the stored section config,
// including digest).
func (c *Client) GetDomain(ctx context.Context, realm string) (*Domain, error) {
	var out Domain
	path := fmt.Sprintf("/access/domains/%s", realm)
	if err := c.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateDomain PUTs /access/domains/{realm}. Delete carries the
// comma-separated PVE field names to clear; empty string omits it.
func (c *Client) UpdateDomain(ctx context.Context, realm string, body Domain) error {
	path := fmt.Sprintf("/access/domains/%s", realm)
	return c.Do(ctx, "PUT", path, body, nil)
}

// DeleteDomain deletes an authentication realm by ID.
func (c *Client) DeleteDomain(ctx context.Context, realm string) error {
	path := fmt.Sprintf("/access/domains/%s", realm)
	return c.Do(ctx, "DELETE", path, nil, nil)
}

// SyncDomainOptions carries the parameters of POST /access/domains/{realm}/sync.
// Full and Purge are deprecated upstream (use RemoveVanished) but remain in
// the pin, so they stay available.
type SyncDomainOptions struct {
	Scope          string `json:"scope,omitempty"`
	DryRun         *bool  `json:"dry-run,omitempty"`
	EnableNew      *bool  `json:"enable-new,omitempty"`
	RemoveVanished string `json:"remove-vanished,omitempty"`
	Full           *bool  `json:"full,omitempty"`
	Purge          *bool  `json:"purge,omitempty"`
}

// SyncDomain POSTs /access/domains/{realm}/sync and returns the worker
// task UPID the sync runs under.
func (c *Client) SyncDomain(ctx context.Context, realm string, opts SyncDomainOptions) (string, error) {
	var upid string
	path := fmt.Sprintf("/access/domains/%s/sync", realm)
	if err := c.Do(ctx, "POST", path, opts, &upid); err != nil {
		return "", err
	}
	return upid, nil
}

// RealmSyncJob mirrors one realm-sync job definition
// (/cluster/jobs/realm-sync). Run timestamps may arrive as integers or
// quoted strings and the boolean flags in either bool or "1"/"0" form,
// depending on the PVE release and config encoding.
type RealmSyncJob struct {
	ID             string `json:"id,omitempty"`
	Realm          string `json:"realm,omitempty"`
	Schedule       string `json:"schedule,omitempty"`
	Scope          string `json:"scope,omitempty"`
	RemoveVanished string `json:"remove-vanished,omitempty"`
	EnableNew      *bool  `json:"enable-new,omitempty"`
	Enabled        *bool  `json:"enabled,omitempty"`
	Comment        string `json:"comment,omitempty"`
	LastRun        *int64 `json:"last-run,omitempty"`
	NextRun        *int64 `json:"next-run,omitempty"`

	// Delete carries the PUT-only list of settings to clear
	// (comma-separated PVE field names).
	Delete string `json:"delete,omitempty"`
}

// realmSyncJobRaw mirrors RealmSyncJob with the lenient fields as
// json.RawMessage.
type realmSyncJobRaw struct {
	ID             string          `json:"id"`
	Realm          string          `json:"realm"`
	Schedule       string          `json:"schedule"`
	Scope          string          `json:"scope"`
	RemoveVanished string          `json:"remove-vanished"`
	EnableNew      json.RawMessage `json:"enable-new"`
	Enabled        json.RawMessage `json:"enabled"`
	Comment        string          `json:"comment"`
	LastRun        json.RawMessage `json:"last-run"`
	NextRun        json.RawMessage `json:"next-run"`
	Delete         string          `json:"delete"`
}

// UnmarshalJSON tolerates the "1"/0 encodings PVE emits for job flags and
// the int/string encodings of the run timestamps.
func (j *RealmSyncJob) UnmarshalJSON(data []byte) error {
	var raw realmSyncJobRaw
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	j.ID = raw.ID
	j.Realm = raw.Realm
	j.Schedule = raw.Schedule
	j.Scope = raw.Scope
	j.RemoveVanished = raw.RemoveVanished
	j.EnableNew = accessDomainsBoolPtr(raw.EnableNew)
	j.Enabled = accessDomainsBoolPtr(raw.Enabled)
	j.Comment = raw.Comment
	j.LastRun = accessDomainsInt64Ptr(raw.LastRun)
	j.NextRun = accessDomainsInt64Ptr(raw.NextRun)
	j.Delete = raw.Delete
	return nil
}

// accessDomainsInt64Ptr decodes a raw integer-or-string timestamp into a
// *int64, keeping nil for absent/null.
func accessDomainsInt64Ptr(raw json.RawMessage) *int64 {
	trimmed := bytes.Trim(bytes.TrimSpace(raw), `"`)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil
	}
	v, err := strconv.ParseInt(string(trimmed), 10, 64)
	if err != nil {
		return nil
	}
	return &v
}

// ListRealmSyncJobs returns the array from GET /cluster/jobs/realm-sync.
func (c *Client) ListRealmSyncJobs(ctx context.Context) ([]RealmSyncJob, error) {
	var out []RealmSyncJob
	if err := c.Do(ctx, "GET", "/cluster/jobs/realm-sync", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetRealmSyncJob reads GET /cluster/jobs/realm-sync/{id}.
func (c *Client) GetRealmSyncJob(ctx context.Context, id string) (*RealmSyncJob, error) {
	var out RealmSyncJob
	path := fmt.Sprintf("/cluster/jobs/realm-sync/%s", id)
	if err := c.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateRealmSyncJob POSTs /cluster/jobs/realm-sync with the supplied job
// definition. The caller supplies ID, Realm, and Schedule.
func (c *Client) CreateRealmSyncJob(ctx context.Context, job RealmSyncJob) error {
	return c.Do(ctx, "POST", "/cluster/jobs/realm-sync", job, nil)
}

// UpdateRealmSyncJob PUTs /cluster/jobs/realm-sync/{id}. Delete carries the
// comma-separated PVE field names to clear; empty string omits it.
func (c *Client) UpdateRealmSyncJob(ctx context.Context, id string, body RealmSyncJob) error {
	path := fmt.Sprintf("/cluster/jobs/realm-sync/%s", id)
	return c.Do(ctx, "PUT", path, body, nil)
}

// DeleteRealmSyncJob deletes a realm-sync job definition by ID.
func (c *Client) DeleteRealmSyncJob(ctx context.Context, id string) error {
	path := fmt.Sprintf("/cluster/jobs/realm-sync/%s", id)
	return c.Do(ctx, "DELETE", path, nil, nil)
}

// ScheduleRun is one future run returned by GET /cluster/jobs/schedule-analyze.
type ScheduleRun struct {
	Timestamp int64  `json:"timestamp"`
	UTC       string `json:"utc"`
}

// AnalyzeJobSchedule calls GET /cluster/jobs/schedule-analyze to validate a
// systemd calendar-event schedule and preview its next runs. Iterations and
// starttime of 0 use the server defaults.
func (c *Client) AnalyzeJobSchedule(ctx context.Context, schedule string, iterations int, starttime int64) ([]ScheduleRun, error) {
	q := url.Values{}
	q.Set("schedule", schedule)
	if iterations > 0 {
		q.Set("iterations", strconv.Itoa(iterations))
	}
	if starttime > 0 {
		q.Set("starttime", strconv.FormatInt(starttime, 10))
	}
	var out []ScheduleRun
	path := "/cluster/jobs/schedule-analyze?" + q.Encode()
	if err := c.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}
