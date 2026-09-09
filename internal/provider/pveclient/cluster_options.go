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

// ClusterOptions mirrors the datacenter-wide option set managed through
// GET/PUT /cluster/options. Every field is optional: nil means "not set".
//
// PVE stores the grouped options (bwlimit, crs, ha, location, migration,
// next-id, notify, replication, tag-style, u2f, user-tag-access, webauthn)
// as property strings (`key=value,key2=value2`) in datacenter.cfg and
// returns them verbatim on GET; the typed sub-structs translate both ways.
// Writing a property string replaces the whole option, so removing a single
// key is done by rewriting the string without it; clearing the whole option
// travels in the `delete` query parameter of UpdateClusterOptions.
type ClusterOptions struct {
	BWLimit           *ClusterOptionsBWLimit
	ConsentText       *string
	Console           *string
	CRS               *ClusterOptionsCRS
	Description       *string
	EmailFrom         *string
	Fencing           *string
	HA                *ClusterOptionsHA
	HTTPProxy         *string
	Keyboard          *string
	Language          *string
	Location          *ClusterOptionsLocation
	MACPrefix         *string
	MaxWorkers        *int
	Migration         *ClusterOptionsMigration
	MigrationUnsecure *bool
	NextID            *ClusterOptionsNextID
	Notify            *ClusterOptionsNotify
	RegisteredTags    *string
	Replication       *ClusterOptionsReplication
	TagStyle          *ClusterOptionsTagStyle
	U2F               *ClusterOptionsU2F
	UserTagAccess     *ClusterOptionsUserTagAccess
	WebAuthn          *ClusterOptionsWebAuthn
}

// ClusterOptionsBWLimit holds the bwlimit property string fields (KiB/s).
type ClusterOptionsBWLimit struct {
	Clone     *float64
	Default   *float64
	Migration *float64
	Move      *float64
	Restore   *float64
}

// ClusterOptionsCRS holds the crs (cluster resource scheduling) fields.
type ClusterOptionsCRS struct {
	Ha                          *string
	HaAutoRebalance             *bool
	HaAutoRebalanceHoldDuration *float64
	HaAutoRebalanceMargin       *float64
	HaAutoRebalanceMethod       *string
	HaAutoRebalanceThreshold    *float64
	HaRebalanceOnStart          *bool
}

// ClusterOptionsHA holds the ha property string fields.
type ClusterOptionsHA struct {
	ShutdownPolicy *string
}

// ClusterOptionsLocation holds the location property string fields.
type ClusterOptionsLocation struct {
	Latitude  *float64
	Longitude *float64
	Name      *string
}

// ClusterOptionsMigration holds the migration property string fields.
type ClusterOptionsMigration struct {
	Type    *string
	Network *string
}

// ClusterOptionsNextID holds the next-id (VMID range) fields.
type ClusterOptionsNextID struct {
	Lower *int
	Upper *int
}

// ClusterOptionsNotify holds the notify property string fields.
type ClusterOptionsNotify struct {
	Fencing              *string
	PackageUpdates       *string
	Replication          *string
	TargetFencing        *string
	TargetPackageUpdates *string
	TargetReplication    *string
}

// ClusterOptionsReplication holds the replication property string fields.
type ClusterOptionsReplication struct {
	Type    *string
	Network *string
}

// ClusterOptionsTagStyle holds the tag-style property string fields.
type ClusterOptionsTagStyle struct {
	CaseSensitive *bool
	ColorMap      *string
	Ordering      *string
	Shape         *string
}

// ClusterOptionsU2F holds the u2f property string fields.
type ClusterOptionsU2F struct {
	AppID  *string
	Origin *string
}

// ClusterOptionsUserTagAccess holds the user-tag-access fields.
type ClusterOptionsUserTagAccess struct {
	UserAllow     *string
	UserAllowList *string
}

// ClusterOptionsWebAuthn holds the webauthn property string fields.
type ClusterOptionsWebAuthn struct {
	AllowSubdomains *bool
	ID              *string
	Origin          *string
	RP              *string
}

// clusterOptionsWire mirrors the raw GET /cluster/options response. Property
// string options arrive as strings; MigrationUnsecure arrives boolish.
type clusterOptionsWire struct {
	BWLimit           *string         `json:"bwlimit,omitempty"`
	ConsentText       *string         `json:"consent-text,omitempty"`
	Console           *string         `json:"console,omitempty"`
	CRS               *string         `json:"crs,omitempty"`
	Description       *string         `json:"description,omitempty"`
	EmailFrom         *string         `json:"email_from,omitempty"`
	Fencing           *string         `json:"fencing,omitempty"`
	HA                *string         `json:"ha,omitempty"`
	HTTPProxy         *string         `json:"http_proxy,omitempty"`
	Keyboard          *string         `json:"keyboard,omitempty"`
	Language          *string         `json:"language,omitempty"`
	Location          *string         `json:"location,omitempty"`
	MACPrefix         *string         `json:"mac_prefix,omitempty"`
	MaxWorkers        *int            `json:"max_workers,omitempty"`
	Migration         *string         `json:"migration,omitempty"`
	MigrationUnsecure json.RawMessage `json:"migration_unsecure,omitempty"`
	NextID            *string         `json:"next-id,omitempty"`
	Notify            *string         `json:"notify,omitempty"`
	RegisteredTags    *string         `json:"registered-tags,omitempty"`
	Replication       *string         `json:"replication,omitempty"`
	TagStyle          *string         `json:"tag-style,omitempty"`
	U2F               *string         `json:"u2f,omitempty"`
	UserTagAccess     *string         `json:"user-tag-access,omitempty"`
	WebAuthn          *string         `json:"webauthn,omitempty"`
}

// GetClusterOptions fetches GET /cluster/options.
func (c *Client) GetClusterOptions(ctx context.Context) (*ClusterOptions, error) {
	var raw clusterOptionsWire
	if err := c.Do(ctx, "GET", "/cluster/options", nil, &raw); err != nil {
		return nil, err
	}
	return clusterOptionsFromWire(raw)
}

// UpdateClusterOptions PUTs /cluster/options with the supplied options and
// translates deleteFields into the PVE `delete` query parameter
// (comma-separated names of options to clear).
func (c *Client) UpdateClusterOptions(ctx context.Context, opts ClusterOptions, deleteFields []string) error {
	path := "/cluster/options"
	if len(deleteFields) > 0 {
		path += "?delete=" + url.QueryEscape(strings.Join(deleteFields, ","))
	}
	return c.Do(ctx, "PUT", path, clusterOptionsBody(opts), nil)
}

// clusterOptionsFromWire converts the raw response into typed options,
// parsing each property string and naming the option in any parse error.
func clusterOptionsFromWire(raw clusterOptionsWire) (*ClusterOptions, error) {
	out := &ClusterOptions{
		BWLimit:        &ClusterOptionsBWLimit{},
		ConsentText:    raw.ConsentText,
		Console:        raw.Console,
		CRS:            &ClusterOptionsCRS{},
		Description:    raw.Description,
		EmailFrom:      raw.EmailFrom,
		Fencing:        raw.Fencing,
		HA:             &ClusterOptionsHA{},
		HTTPProxy:      raw.HTTPProxy,
		Keyboard:       raw.Keyboard,
		Language:       raw.Language,
		Location:       &ClusterOptionsLocation{},
		MACPrefix:      raw.MACPrefix,
		MaxWorkers:     raw.MaxWorkers,
		Migration:      &ClusterOptionsMigration{},
		NextID:         &ClusterOptionsNextID{},
		Notify:         &ClusterOptionsNotify{},
		RegisteredTags: raw.RegisteredTags,
		Replication:    &ClusterOptionsReplication{},
		TagStyle:       &ClusterOptionsTagStyle{},
		U2F:            &ClusterOptionsU2F{},
		UserTagAccess:  &ClusterOptionsUserTagAccess{},
		WebAuthn:       &ClusterOptionsWebAuthn{},
	}
	if raw.MigrationUnsecure != nil {
		out.MigrationUnsecure = nodeNetworkBoolishPtr(raw.MigrationUnsecure)
	}
	var err error
	if out.BWLimit, err = parseClusterOptionsBWLimit(clusterOptionsDeref(raw.BWLimit)); err != nil {
		return nil, err
	}
	if out.CRS, err = parseClusterOptionsCRS(clusterOptionsDeref(raw.CRS)); err != nil {
		return nil, err
	}
	if out.HA, err = parseClusterOptionsHA(clusterOptionsDeref(raw.HA)); err != nil {
		return nil, err
	}
	if out.Location, err = parseClusterOptionsLocation(clusterOptionsDeref(raw.Location)); err != nil {
		return nil, err
	}
	if out.Migration, err = parseClusterOptionsMigration(clusterOptionsDeref(raw.Migration)); err != nil {
		return nil, err
	}
	if out.NextID, err = parseClusterOptionsNextID(clusterOptionsDeref(raw.NextID)); err != nil {
		return nil, err
	}
	if out.Notify, err = parseClusterOptionsNotify(clusterOptionsDeref(raw.Notify)); err != nil {
		return nil, err
	}
	if out.Replication, err = parseClusterOptionsReplication(clusterOptionsDeref(raw.Replication)); err != nil {
		return nil, err
	}
	if out.TagStyle, err = parseClusterOptionsTagStyle(clusterOptionsDeref(raw.TagStyle)); err != nil {
		return nil, err
	}
	if out.U2F, err = parseClusterOptionsU2F(clusterOptionsDeref(raw.U2F)); err != nil {
		return nil, err
	}
	if out.UserTagAccess, err = parseClusterOptionsUserTagAccess(clusterOptionsDeref(raw.UserTagAccess)); err != nil {
		return nil, err
	}
	if out.WebAuthn, err = parseClusterOptionsWebAuthn(clusterOptionsDeref(raw.WebAuthn)); err != nil {
		return nil, err
	}
	return out, nil
}

// clusterOptionsDeref nils out absent property strings so absent options
// stay nil.
func clusterOptionsDeref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// clusterOptionsParseKV splits a PVE property string into key=value pairs.
// Values in the /cluster/options formats never contain commas, so a plain
// split is safe.
func clusterOptionsParseKV(field, raw string) (map[string]string, error) {
	out := make(map[string]string)
	if raw == "" {
		return out, nil
	}
	for _, part := range strings.Split(raw, ",") {
		key, value, found := strings.Cut(part, "=")
		if !found || key == "" {
			return nil, fmt.Errorf("cluster option %s: segment %q is not key=value", field, part)
		}
		out[key] = value
	}
	return out, nil
}

// clusterOptionsKVStr returns key's value or nil when the key is absent.
func clusterOptionsKVStr(kv map[string]string, key string) *string {
	if v, ok := kv[key]; ok {
		return &v
	}
	return nil
}

// clusterOptionsKVFloat parses key as a float, naming field and key on error.
func clusterOptionsKVFloat(field, key string, kv map[string]string) (*float64, error) {
	v, ok := kv[key]
	if !ok {
		return nil, nil
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return nil, fmt.Errorf("cluster option %s: key %q: %w", field, key, err)
	}
	return &f, nil
}

// clusterOptionsKVInt parses key as an integer, naming field and key on error.
func clusterOptionsKVInt(field, key string, kv map[string]string) (*int, error) {
	v, ok := kv[key]
	if !ok {
		return nil, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return nil, fmt.Errorf("cluster option %s: key %q: %w", field, key, err)
	}
	return &n, nil
}

// clusterOptionsKVBool parses key as a PVE bool (1/0, true/false tolerated),
// naming field and key on error.
func clusterOptionsKVBool(field, key string, kv map[string]string) (*bool, error) {
	v, ok := kv[key]
	if !ok {
		return nil, nil
	}
	switch v {
	case "1", "true":
		out := true
		return &out, nil
	case "0", "false":
		out := false
		return &out, nil
	default:
		return nil, fmt.Errorf("cluster option %s: key %q: not a boolean: %q", field, key, v)
	}
}

// parseClusterOptionsBWLimit decodes the bwlimit property string.
func parseClusterOptionsBWLimit(raw string) (*ClusterOptionsBWLimit, error) {
	kv, err := clusterOptionsParseKV("bwlimit", raw)
	if err != nil {
		return nil, err
	}
	if len(kv) == 0 {
		return nil, nil
	}
	out := &ClusterOptionsBWLimit{}
	if out.Clone, err = clusterOptionsKVFloat("bwlimit", "clone", kv); err != nil {
		return nil, err
	}
	if out.Default, err = clusterOptionsKVFloat("bwlimit", "default", kv); err != nil {
		return nil, err
	}
	if out.Migration, err = clusterOptionsKVFloat("bwlimit", "migration", kv); err != nil {
		return nil, err
	}
	if out.Move, err = clusterOptionsKVFloat("bwlimit", "move", kv); err != nil {
		return nil, err
	}
	if out.Restore, err = clusterOptionsKVFloat("bwlimit", "restore", kv); err != nil {
		return nil, err
	}
	return out, nil
}

// parseClusterOptionsCRS decodes the crs property string.
func parseClusterOptionsCRS(raw string) (*ClusterOptionsCRS, error) {
	kv, err := clusterOptionsParseKV("crs", raw)
	if err != nil {
		return nil, err
	}
	if len(kv) == 0 {
		return nil, nil
	}
	out := &ClusterOptionsCRS{
		Ha:                    clusterOptionsKVStr(kv, "ha"),
		HaAutoRebalanceMethod: clusterOptionsKVStr(kv, "ha-auto-rebalance-method"),
	}
	if out.HaAutoRebalance, err = clusterOptionsKVBool("crs", "ha-auto-rebalance", kv); err != nil {
		return nil, err
	}
	if out.HaAutoRebalanceHoldDuration, err = clusterOptionsKVFloat("crs", "ha-auto-rebalance-hold-duration", kv); err != nil {
		return nil, err
	}
	if out.HaAutoRebalanceMargin, err = clusterOptionsKVFloat("crs", "ha-auto-rebalance-margin", kv); err != nil {
		return nil, err
	}
	if out.HaAutoRebalanceThreshold, err = clusterOptionsKVFloat("crs", "ha-auto-rebalance-threshold", kv); err != nil {
		return nil, err
	}
	if out.HaRebalanceOnStart, err = clusterOptionsKVBool("crs", "ha-rebalance-on-start", kv); err != nil {
		return nil, err
	}
	return out, nil
}

// parseClusterOptionsHA decodes the ha property string.
func parseClusterOptionsHA(raw string) (*ClusterOptionsHA, error) {
	kv, err := clusterOptionsParseKV("ha", raw)
	if err != nil {
		return nil, err
	}
	if len(kv) == 0 {
		return nil, nil
	}
	return &ClusterOptionsHA{ShutdownPolicy: clusterOptionsKVStr(kv, "shutdown_policy")}, nil
}

// parseClusterOptionsLocation decodes the location property string.
func parseClusterOptionsLocation(raw string) (*ClusterOptionsLocation, error) {
	kv, err := clusterOptionsParseKV("location", raw)
	if err != nil {
		return nil, err
	}
	if len(kv) == 0 {
		return nil, nil
	}
	out := &ClusterOptionsLocation{Name: clusterOptionsKVStr(kv, "name")}
	if out.Latitude, err = clusterOptionsKVFloat("location", "latitude", kv); err != nil {
		return nil, err
	}
	if out.Longitude, err = clusterOptionsKVFloat("location", "longitude", kv); err != nil {
		return nil, err
	}
	return out, nil
}

// parseClusterOptionsMigration decodes the migration property string.
func parseClusterOptionsMigration(raw string) (*ClusterOptionsMigration, error) {
	kv, err := clusterOptionsParseKV("migration", raw)
	if err != nil {
		return nil, err
	}
	if len(kv) == 0 {
		return nil, nil
	}
	return &ClusterOptionsMigration{
		Type:    clusterOptionsKVStr(kv, "type"),
		Network: clusterOptionsKVStr(kv, "network"),
	}, nil
}

// parseClusterOptionsNextID decodes the next-id property string.
func parseClusterOptionsNextID(raw string) (*ClusterOptionsNextID, error) {
	kv, err := clusterOptionsParseKV("next-id", raw)
	if err != nil {
		return nil, err
	}
	if len(kv) == 0 {
		return nil, nil
	}
	out := &ClusterOptionsNextID{}
	if out.Lower, err = clusterOptionsKVInt("next-id", "lower", kv); err != nil {
		return nil, err
	}
	if out.Upper, err = clusterOptionsKVInt("next-id", "upper", kv); err != nil {
		return nil, err
	}
	return out, nil
}

// parseClusterOptionsNotify decodes the notify property string.
func parseClusterOptionsNotify(raw string) (*ClusterOptionsNotify, error) {
	kv, err := clusterOptionsParseKV("notify", raw)
	if err != nil {
		return nil, err
	}
	if len(kv) == 0 {
		return nil, nil
	}
	return &ClusterOptionsNotify{
		Fencing:              clusterOptionsKVStr(kv, "fencing"),
		PackageUpdates:       clusterOptionsKVStr(kv, "package-updates"),
		Replication:          clusterOptionsKVStr(kv, "replication"),
		TargetFencing:        clusterOptionsKVStr(kv, "target-fencing"),
		TargetPackageUpdates: clusterOptionsKVStr(kv, "target-package-updates"),
		TargetReplication:    clusterOptionsKVStr(kv, "target-replication"),
	}, nil
}

// parseClusterOptionsReplication decodes the replication property string.
func parseClusterOptionsReplication(raw string) (*ClusterOptionsReplication, error) {
	kv, err := clusterOptionsParseKV("replication", raw)
	if err != nil {
		return nil, err
	}
	if len(kv) == 0 {
		return nil, nil
	}
	return &ClusterOptionsReplication{
		Type:    clusterOptionsKVStr(kv, "type"),
		Network: clusterOptionsKVStr(kv, "network"),
	}, nil
}

// parseClusterOptionsTagStyle decodes the tag-style property string.
func parseClusterOptionsTagStyle(raw string) (*ClusterOptionsTagStyle, error) {
	kv, err := clusterOptionsParseKV("tag-style", raw)
	if err != nil {
		return nil, err
	}
	if len(kv) == 0 {
		return nil, nil
	}
	out := &ClusterOptionsTagStyle{
		ColorMap: clusterOptionsKVStr(kv, "color-map"),
		Ordering: clusterOptionsKVStr(kv, "ordering"),
		Shape:    clusterOptionsKVStr(kv, "shape"),
	}
	if out.CaseSensitive, err = clusterOptionsKVBool("tag-style", "case-sensitive", kv); err != nil {
		return nil, err
	}
	return out, nil
}

// parseClusterOptionsU2F decodes the u2f property string.
func parseClusterOptionsU2F(raw string) (*ClusterOptionsU2F, error) {
	kv, err := clusterOptionsParseKV("u2f", raw)
	if err != nil {
		return nil, err
	}
	if len(kv) == 0 {
		return nil, nil
	}
	return &ClusterOptionsU2F{
		AppID:  clusterOptionsKVStr(kv, "appid"),
		Origin: clusterOptionsKVStr(kv, "origin"),
	}, nil
}

// parseClusterOptionsUserTagAccess decodes the user-tag-access string.
func parseClusterOptionsUserTagAccess(raw string) (*ClusterOptionsUserTagAccess, error) {
	kv, err := clusterOptionsParseKV("user-tag-access", raw)
	if err != nil {
		return nil, err
	}
	if len(kv) == 0 {
		return nil, nil
	}
	return &ClusterOptionsUserTagAccess{
		UserAllow:     clusterOptionsKVStr(kv, "user-allow"),
		UserAllowList: clusterOptionsKVStr(kv, "user-allow-list"),
	}, nil
}

// parseClusterOptionsWebAuthn decodes the webauthn property string.
func parseClusterOptionsWebAuthn(raw string) (*ClusterOptionsWebAuthn, error) {
	kv, err := clusterOptionsParseKV("webauthn", raw)
	if err != nil {
		return nil, err
	}
	if len(kv) == 0 {
		return nil, nil
	}
	out := &ClusterOptionsWebAuthn{
		ID:     clusterOptionsKVStr(kv, "id"),
		Origin: clusterOptionsKVStr(kv, "origin"),
		RP:     clusterOptionsKVStr(kv, "rp"),
	}
	if out.AllowSubdomains, err = clusterOptionsKVBool("webauthn", "allow-subdomains", kv); err != nil {
		return nil, err
	}
	return out, nil
}

// clusterOptionsBuilder renders typed fields into a PVE property string,
// skipping nil fields and joining present ones as key=value pairs.
type clusterOptionsBuilder struct {
	parts []string
}

func (b *clusterOptionsBuilder) str(key string, v *string) {
	if v != nil {
		b.parts = append(b.parts, key+"="+*v)
	}
}

func (b *clusterOptionsBuilder) boolean(key string, v *bool) {
	if v != nil {
		rendered := "0"
		if *v {
			rendered = "1"
		}
		b.parts = append(b.parts, key+"="+rendered)
	}
}

func (b *clusterOptionsBuilder) float(key string, v *float64) {
	if v != nil {
		b.parts = append(b.parts, key+"="+strconv.FormatFloat(*v, 'f', -1, 64))
	}
}

func (b *clusterOptionsBuilder) integer(key string, v *int) {
	if v != nil {
		b.parts = append(b.parts, key+"="+strconv.Itoa(*v))
	}
}

// join renders the accumulated pairs, or "" when nothing was added.
func (b *clusterOptionsBuilder) join() string {
	return strings.Join(b.parts, ",")
}

// clusterOptionsPutBody is the JSON body of PUT /cluster/options: scalar
// values keyed by their wire names and grouped options rendered as
// property strings.
type clusterOptionsPutBody map[string]any

// clusterOptionsBody projects typed options into the JSON body of
// PUT /cluster/options: scalars as typed values, grouped options as
// property strings, nil options omitted entirely.
func clusterOptionsBody(o ClusterOptions) clusterOptionsPutBody {
	body := make(clusterOptionsPutBody)
	clusterOptionsBodyStr(body, "consent-text", o.ConsentText)
	clusterOptionsBodyStr(body, "console", o.Console)
	clusterOptionsBodyStr(body, "description", o.Description)
	clusterOptionsBodyStr(body, "email_from", o.EmailFrom)
	clusterOptionsBodyStr(body, "fencing", o.Fencing)
	clusterOptionsBodyStr(body, "http_proxy", o.HTTPProxy)
	clusterOptionsBodyStr(body, "keyboard", o.Keyboard)
	clusterOptionsBodyStr(body, "language", o.Language)
	clusterOptionsBodyStr(body, "mac_prefix", o.MACPrefix)
	clusterOptionsBodyStr(body, "registered-tags", o.RegisteredTags)
	if o.MaxWorkers != nil {
		body["max_workers"] = *o.MaxWorkers
	}
	if o.MigrationUnsecure != nil {
		body["migration_unsecure"] = *o.MigrationUnsecure
	}
	if o.BWLimit != nil {
		var b clusterOptionsBuilder
		b.float("clone", o.BWLimit.Clone)
		b.float("default", o.BWLimit.Default)
		b.float("migration", o.BWLimit.Migration)
		b.float("move", o.BWLimit.Move)
		b.float("restore", o.BWLimit.Restore)
		clusterOptionsBodyProp(body, "bwlimit", b.join())
	}
	if o.CRS != nil {
		var b clusterOptionsBuilder
		b.str("ha", o.CRS.Ha)
		b.boolean("ha-auto-rebalance", o.CRS.HaAutoRebalance)
		b.float("ha-auto-rebalance-hold-duration", o.CRS.HaAutoRebalanceHoldDuration)
		b.float("ha-auto-rebalance-margin", o.CRS.HaAutoRebalanceMargin)
		b.str("ha-auto-rebalance-method", o.CRS.HaAutoRebalanceMethod)
		b.float("ha-auto-rebalance-threshold", o.CRS.HaAutoRebalanceThreshold)
		b.boolean("ha-rebalance-on-start", o.CRS.HaRebalanceOnStart)
		clusterOptionsBodyProp(body, "crs", b.join())
	}
	if o.HA != nil {
		var b clusterOptionsBuilder
		b.str("shutdown_policy", o.HA.ShutdownPolicy)
		clusterOptionsBodyProp(body, "ha", b.join())
	}
	if o.Location != nil {
		var b clusterOptionsBuilder
		b.float("latitude", o.Location.Latitude)
		b.float("longitude", o.Location.Longitude)
		b.str("name", o.Location.Name)
		clusterOptionsBodyProp(body, "location", b.join())
	}
	if o.Migration != nil {
		var b clusterOptionsBuilder
		b.str("type", o.Migration.Type)
		b.str("network", o.Migration.Network)
		clusterOptionsBodyProp(body, "migration", b.join())
	}
	if o.NextID != nil {
		var b clusterOptionsBuilder
		b.integer("lower", o.NextID.Lower)
		b.integer("upper", o.NextID.Upper)
		clusterOptionsBodyProp(body, "next-id", b.join())
	}
	if o.Notify != nil {
		var b clusterOptionsBuilder
		b.str("fencing", o.Notify.Fencing)
		b.str("package-updates", o.Notify.PackageUpdates)
		b.str("replication", o.Notify.Replication)
		b.str("target-fencing", o.Notify.TargetFencing)
		b.str("target-package-updates", o.Notify.TargetPackageUpdates)
		b.str("target-replication", o.Notify.TargetReplication)
		clusterOptionsBodyProp(body, "notify", b.join())
	}
	if o.Replication != nil {
		var b clusterOptionsBuilder
		b.str("type", o.Replication.Type)
		b.str("network", o.Replication.Network)
		clusterOptionsBodyProp(body, "replication", b.join())
	}
	if o.TagStyle != nil {
		var b clusterOptionsBuilder
		b.boolean("case-sensitive", o.TagStyle.CaseSensitive)
		b.str("color-map", o.TagStyle.ColorMap)
		b.str("ordering", o.TagStyle.Ordering)
		b.str("shape", o.TagStyle.Shape)
		clusterOptionsBodyProp(body, "tag-style", b.join())
	}
	if o.U2F != nil {
		var b clusterOptionsBuilder
		b.str("appid", o.U2F.AppID)
		b.str("origin", o.U2F.Origin)
		clusterOptionsBodyProp(body, "u2f", b.join())
	}
	if o.UserTagAccess != nil {
		var b clusterOptionsBuilder
		b.str("user-allow", o.UserTagAccess.UserAllow)
		b.str("user-allow-list", o.UserTagAccess.UserAllowList)
		clusterOptionsBodyProp(body, "user-tag-access", b.join())
	}
	if o.WebAuthn != nil {
		var b clusterOptionsBuilder
		b.boolean("allow-subdomains", o.WebAuthn.AllowSubdomains)
		b.str("id", o.WebAuthn.ID)
		b.str("origin", o.WebAuthn.Origin)
		b.str("rp", o.WebAuthn.RP)
		clusterOptionsBodyProp(body, "webauthn", b.join())
	}
	return body
}

// clusterOptionsBodyStr adds a scalar string when set.
func clusterOptionsBodyStr(body clusterOptionsPutBody, key string, v *string) {
	if v != nil {
		body[key] = *v
	}
}

// clusterOptionsBodyProp adds a rendered property string when non-empty.
func clusterOptionsBodyProp(body clusterOptionsPutBody, key, rendered string) {
	if rendered != "" {
		body[key] = rendered
	}
}
