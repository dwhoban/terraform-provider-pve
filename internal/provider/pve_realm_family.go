// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

// This file carries the plumbing shared by the pve_realm_ldap,
// pve_realm_ad, and pve_realm_openid resources and data sources (ADR 0001
// B04: one component per realm type, wire type fixed per component).

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hashicorp/terraform-provider-scaffolding-framework/internal/provider/pveclient"
)

// PVE realm type constants fixed by each member of the realm family.
const (
	realmTypeLDAP   = "ldap"
	realmTypeAD     = "ad"
	realmTypeOpenid = "openid"
)

// realmCommonModel holds the attributes shared by every realm component:
// the realm ID, the login-window description, and the PVE-wide flags.
type realmCommonModel struct {
	Realm   types.String `tfsdk:"realm"`
	Comment types.String `tfsdk:"comment"`
	Default types.Bool   `tfsdk:"default"`
	TFA     types.String `tfsdk:"tfa"`
	Digest  types.String `tfsdk:"digest"`
}

// realmTransportModel holds the LDAP/AD server transport attributes shared
// by the ldap and ad realm types.
type realmTransportModel struct {
	Server1         types.String `tfsdk:"server1"`
	Server2         types.String `tfsdk:"server2"`
	Port            types.Int64  `tfsdk:"port"`
	Mode            types.String `tfsdk:"mode"`
	Secure          types.Bool   `tfsdk:"secure"`
	Verify          types.Bool   `tfsdk:"verify"`
	Capath          types.String `tfsdk:"capath"`
	Cert            types.String `tfsdk:"cert"`
	CertKey         types.String `tfsdk:"certkey"`
	SSLVersion      types.String `tfsdk:"sslversion"`
	CaseSensitive   types.Bool   `tfsdk:"case_sensitive"`
	CheckConnection types.Bool   `tfsdk:"check_connection"`
}

// realmLdapModel holds the LDAP-specific directory attributes.
type realmLdapModel struct {
	BaseDN              types.String `tfsdk:"base_dn"`
	BindDN              types.String `tfsdk:"bind_dn"`
	Password            types.String `tfsdk:"password"`
	UserAttr            types.String `tfsdk:"user_attr"`
	UserClasses         types.String `tfsdk:"user_classes"`
	GroupClasses        types.String `tfsdk:"group_classes"`
	GroupDN             types.String `tfsdk:"group_dn"`
	GroupFilter         types.String `tfsdk:"group_filter"`
	GroupNameAttr       types.String `tfsdk:"group_name_attr"`
	Filter              types.String `tfsdk:"filter"`
	SyncAttributes      types.String `tfsdk:"sync_attributes"`
	SyncDefaultsOptions types.String `tfsdk:"sync_defaults_options"`
}

// realmAdModel holds the AD-specific attributes on top of the shared
// transport set.
type realmAdModel struct {
	Domain   types.String `tfsdk:"domain"`
	BindDN   types.String `tfsdk:"bind_dn"`
	Password types.String `tfsdk:"password"`
}

// realmOpenidModel holds the OpenID Connect attributes.
type realmOpenidModel struct {
	IssuerURL        types.String `tfsdk:"issuer_url"`
	ClientID         types.String `tfsdk:"client_id"`
	ClientKey        types.String `tfsdk:"client_key"`
	UsernameClaim    types.String `tfsdk:"username_claim"`
	GroupsClaim      types.String `tfsdk:"groups_claim"`
	GroupsAutocreate types.Bool   `tfsdk:"groups_autocreate"`
	GroupsOverwrite  types.Bool   `tfsdk:"groups_overwrite"`
	Autocreate       types.Bool   `tfsdk:"autocreate"`
	QueryUserinfo    types.Bool   `tfsdk:"query_userinfo"`
	Scopes           types.String `tfsdk:"scopes"`
	Prompt           types.String `tfsdk:"prompt"`
	ACRValues        types.String `tfsdk:"acr_values"`
	Audiences        types.String `tfsdk:"audiences"`
}

// realmCommonAttributes returns the shared realm attributes. computed
// marks the data-source form (everything but realm is read-only);
// replaceable adds the RequiresReplace plan modifier for the resource form.
func realmCommonAttributes(computed, replaceable bool) map[string]schema.Attribute {
	attrs := map[string]schema.Attribute{
		"realm": schema.StringAttribute{
			Required:            true,
			MarkdownDescription: "Authentication domain ID (PVE realm name, up to 32 characters).",
			Validators: []validator.String{
				stringvalidator.LengthAtMost(32),
			},
		},
		"comment": schema.StringAttribute{
			Optional:            !computed,
			Computed:            computed,
			MarkdownDescription: "Description shown in the PVE login window when the realm is selected.",
		},
		"default": schema.BoolAttribute{
			Optional:            !computed,
			Computed:            computed,
			MarkdownDescription: "Use this realm as the default realm.",
		},
		"tfa": schema.StringAttribute{
			Optional:            !computed,
			Computed:            computed,
			MarkdownDescription: "Two-factor authentication configuration in PVE `pve-tfa-config` format (e.g. `type=oath`). The realm index reports the active provider instead (`yubico` or `oath`).",
		},
		"digest": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Realm configuration digest used by PVE to prevent concurrent modifications (read-only).",
		},
	}
	if replaceable {
		if realm, ok := attrs["realm"].(schema.StringAttribute); ok {
			realm.PlanModifiers = []planmodifier.String{
				stringplanmodifier.RequiresReplace(),
			}
			attrs["realm"] = realm
		}
	}
	return attrs
}

// realmTransportAttributes returns the LDAP/AD server transport attributes.
// See realmCommonAttributes for the flag semantics.
func realmTransportAttributes(computed bool) map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"server1": schema.StringAttribute{
			Optional:            !computed,
			Computed:            computed,
			MarkdownDescription: "Server IP address (or DNS name), up to 256 characters.",
		},
		"server2": schema.StringAttribute{
			Optional:            !computed,
			Computed:            computed,
			MarkdownDescription: "Fallback server IP address (or DNS name).",
		},
		"port": schema.Int64Attribute{
			Optional:            !computed,
			Computed:            computed,
			MarkdownDescription: "Server port. Must be between 1 and 65535.",
			Validators: []validator.Int64{
				int64validator.Between(1, 65535),
			},
		},
		"mode": schema.StringAttribute{
			Optional:            !computed,
			Computed:            computed,
			MarkdownDescription: "LDAP protocol mode. Must be one of `ldap`, `ldaps`, `ldap+starttls`. Defaults to `ldap`.",
			Validators: []validator.String{
				stringvalidator.OneOf("ldap", "ldaps", "ldap+starttls"),
			},
		},
		"secure": schema.BoolAttribute{
			Optional:            !computed,
			Computed:            computed,
			MarkdownDescription: "Use secure LDAPS protocol. Deprecated upstream: use `mode` instead.",
		},
		"verify": schema.BoolAttribute{
			Optional:            !computed,
			Computed:            computed,
			MarkdownDescription: "Verify the server's SSL certificate. Defaults to `false`.",
		},
		"capath": schema.StringAttribute{
			Optional:            !computed,
			Computed:            computed,
			MarkdownDescription: "Path to the CA certificate store. Defaults to `/etc/ssl/certs`.",
		},
		"cert": schema.StringAttribute{
			Optional:            !computed,
			Computed:            computed,
			MarkdownDescription: "Path to the client certificate.",
		},
		"certkey": schema.StringAttribute{
			Optional:            !computed,
			Computed:            computed,
			MarkdownDescription: "Path to the client certificate key.",
		},
		"sslversion": schema.StringAttribute{
			Optional:            !computed,
			Computed:            computed,
			MarkdownDescription: "LDAPS TLS/SSL version. Must be one of `tlsv1`, `tlsv1_1`, `tlsv1_2`, `tlsv1_3`. Upstream recommends 1.2 or newer.",
			Validators: []validator.String{
				stringvalidator.OneOf("tlsv1", "tlsv1_1", "tlsv1_2", "tlsv1_3"),
			},
		},
		"case_sensitive": schema.BoolAttribute{
			Optional:            !computed,
			Computed:            computed,
			MarkdownDescription: "Username is case-sensitive. Defaults to `true`.",
		},
		"check_connection": schema.BoolAttribute{
			Optional:            !computed,
			Computed:            computed,
			MarkdownDescription: "Check the bind connection to the server when saving. Defaults to `false`.",
		},
	}
}

// realmLDAPAttributes returns the full attribute set of the ldap realm
// type: common + transport + LDAP directory fields.
func realmLDAPAttributes(computed, replaceable bool) map[string]schema.Attribute {
	attrs := realmCommonAttributes(computed, replaceable)
	for name, attr := range realmTransportAttributes(computed) {
		attrs[name] = attr
	}
	for name, attr := range map[string]schema.Attribute{
		"base_dn": schema.StringAttribute{
			Optional:            !computed,
			Computed:            computed,
			MarkdownDescription: "LDAP base domain name (up to 256 characters).",
		},
		"bind_dn": schema.StringAttribute{
			Optional:            !computed,
			Computed:            computed,
			MarkdownDescription: "LDAP bind domain name (up to 256 characters).",
		},
		"password": schema.StringAttribute{
			Optional:            !computed,
			Computed:            computed,
			Sensitive:           true,
			MarkdownDescription: "LDAP bind password. PVE stores it in `/etc/pve/priv/realm/<realm>.pw` and never returns it; the provider only sends it when you set it.",
		},
		"user_attr": schema.StringAttribute{
			Optional:            !computed,
			Computed:            computed,
			MarkdownDescription: "LDAP user attribute name.",
		},
		"user_classes": schema.StringAttribute{
			Optional:            !computed,
			Computed:            computed,
			MarkdownDescription: "The objectclasses for users, comma-separated. Defaults to `inetorgperson, posixaccount, person, user`.",
		},
		"group_classes": schema.StringAttribute{
			Optional:            !computed,
			Computed:            computed,
			MarkdownDescription: "The objectclasses for groups, comma-separated. Defaults to `groupOfNames, group, univentionGroup, ipausergroup`.",
		},
		"group_dn": schema.StringAttribute{
			Optional:            !computed,
			Computed:            computed,
			MarkdownDescription: "LDAP base domain name for group sync. Defaults to `base_dn`.",
		},
		"group_filter": schema.StringAttribute{
			Optional:            !computed,
			Computed:            computed,
			MarkdownDescription: "LDAP filter for group sync (up to 2048 characters).",
		},
		"group_name_attr": schema.StringAttribute{
			Optional:            !computed,
			Computed:            computed,
			MarkdownDescription: "LDAP attribute representing a group's name. Defaults to the first value of the DN.",
		},
		"filter": schema.StringAttribute{
			Optional:            !computed,
			Computed:            computed,
			MarkdownDescription: "LDAP filter for user sync (up to 2048 characters).",
		},
		"sync_attributes": schema.StringAttribute{
			Optional:            !computed,
			Computed:            computed,
			MarkdownDescription: "Comma-separated `key=value` pairs mapping LDAP attributes to PVE user fields, e.g. `email=mail`. Defaults to the same-named LDAP attribute per PVE user field.",
		},
		"sync_defaults_options": schema.StringAttribute{
			Optional:            !computed,
			Computed:            computed,
			MarkdownDescription: "Default options for realm synchronizations in PVE `realm-sync-options` format, e.g. `enable-new=1,scope=both`.",
		},
	} {
		attrs[name] = attr
	}
	return attrs
}

// realmADAttributes returns the full attribute set of the ad realm type.
func realmADAttributes(computed, replaceable bool) map[string]schema.Attribute {
	attrs := realmCommonAttributes(computed, replaceable)
	for name, attr := range realmTransportAttributes(computed) {
		attrs[name] = attr
	}
	for name, attr := range map[string]schema.Attribute{
		"domain": schema.StringAttribute{
			Optional:            !computed,
			Computed:            computed,
			MarkdownDescription: "AD domain name (e.g. `ad.example.com`).",
		},
		"bind_dn": schema.StringAttribute{
			Optional:            !computed,
			Computed:            computed,
			MarkdownDescription: "LDAP bind domain name for authenticated binds against the AD domain (up to 256 characters).",
		},
		"password": schema.StringAttribute{
			Optional:            !computed,
			Computed:            computed,
			Sensitive:           true,
			MarkdownDescription: "Bind password. PVE stores it in `/etc/pve/priv/realm/<realm>.pw` and never returns it; the provider only sends it when you set it.",
		},
	} {
		attrs[name] = attr
	}
	return attrs
}

// realmOpenidAttributes returns the full attribute set of the openid realm
// type: common + OpenID Connect fields.
func realmOpenidAttributes(computed, replaceable bool) map[string]schema.Attribute {
	attrs := realmCommonAttributes(computed, replaceable)
	for name, attr := range map[string]schema.Attribute{
		"issuer_url": schema.StringAttribute{
			Optional:            !computed,
			Computed:            computed,
			MarkdownDescription: "OpenID issuer URL (up to 256 characters).",
		},
		"client_id": schema.StringAttribute{
			Optional:            !computed,
			Computed:            computed,
			MarkdownDescription: "OpenID client ID (up to 256 characters).",
		},
		"client_key": schema.StringAttribute{
			Optional:            !computed,
			Computed:            computed,
			Sensitive:           true,
			MarkdownDescription: "OpenID client key. PVE may not return it, so the provider keeps your configured value and only sends it when you set it.",
		},
		"username_claim": schema.StringAttribute{
			Optional:            !computed,
			Computed:            computed,
			MarkdownDescription: "OpenID claim used to generate the unique username (e.g. `sub` or `email`).",
		},
		"groups_claim": schema.StringAttribute{
			Optional:            !computed,
			Computed:            computed,
			MarkdownDescription: "OpenID claim used to retrieve groups with.",
		},
		"groups_autocreate": schema.BoolAttribute{
			Optional:            !computed,
			Computed:            computed,
			MarkdownDescription: "Automatically create groups if they do not exist. Defaults to `false`.",
		},
		"groups_overwrite": schema.BoolAttribute{
			Optional:            !computed,
			Computed:            computed,
			MarkdownDescription: "Overwrite all groups of the user on login. Defaults to `false`.",
		},
		"autocreate": schema.BoolAttribute{
			Optional:            !computed,
			Computed:            computed,
			MarkdownDescription: "Automatically create users if they do not exist. Defaults to `false`.",
		},
		"query_userinfo": schema.BoolAttribute{
			Optional:            !computed,
			Computed:            computed,
			MarkdownDescription: "Query the userinfo endpoint for claims values. Defaults to `true`.",
		},
		"scopes": schema.StringAttribute{
			Optional:            !computed,
			Computed:            computed,
			MarkdownDescription: "Scopes to authorize and return, e.g. `email profile`. Defaults to `email profile`.",
		},
		"prompt": schema.StringAttribute{
			Optional:            !computed,
			Computed:            computed,
			MarkdownDescription: "Whether the Authorization Server prompts the End-User for reauthentication and consent, e.g. `none`, `login`, `consent`, or `select_account`.",
		},
		"acr_values": schema.StringAttribute{
			Optional:            !computed,
			Computed:            computed,
			MarkdownDescription: "Authentication Context Class Reference values the Authorization Server is requested to use for the auth request.",
		},
		"audiences": schema.StringAttribute{
			Optional:            !computed,
			Computed:            computed,
			MarkdownDescription: "Additional audiences the OpenID Issuer may include beyond `client_id`.",
		},
	} {
		attrs[name] = attr
	}
	return attrs
}

// realmCommonApply copies the common attributes into the wire body and
// returns the PVE field names to clear (set in state, null in plan).
func realmCommonApply(plan, state realmCommonModel, body *pveclient.Domain) []string {
	if !plan.Comment.IsNull() && !plan.Comment.IsUnknown() {
		body.Comment = plan.Comment.ValueString()
	}
	if !plan.Default.IsNull() && !plan.Default.IsUnknown() {
		v := plan.Default.ValueBool()
		body.Default = &v
	}
	if !plan.TFA.IsNull() && !plan.TFA.IsUnknown() {
		body.TFA = plan.TFA.ValueString()
	}
	var del []string
	if realmStringCleared(plan.Comment, state.Comment) {
		del = append(del, "comment")
	}
	if plan.Default.IsNull() && !state.Default.IsNull() && !state.Default.IsUnknown() {
		del = append(del, "default")
	}
	if realmStringCleared(plan.TFA, state.TFA) {
		del = append(del, "tfa")
	}
	return del
}

// realmTransportApply copies the transport attributes into the wire body
// and returns the PVE field names to clear.
func realmTransportApply(plan, state realmTransportModel, body *pveclient.Domain) []string {
	if !plan.Server1.IsNull() && !plan.Server1.IsUnknown() {
		body.Server1 = plan.Server1.ValueString()
	}
	if !plan.Server2.IsNull() && !plan.Server2.IsUnknown() {
		body.Server2 = plan.Server2.ValueString()
	}
	if !plan.Port.IsNull() && !plan.Port.IsUnknown() {
		pv := int(plan.Port.ValueInt64())
		body.Port = &pv
	}
	if !plan.Mode.IsNull() && !plan.Mode.IsUnknown() {
		body.Mode = plan.Mode.ValueString()
	}
	if !plan.Secure.IsNull() && !plan.Secure.IsUnknown() {
		v := plan.Secure.ValueBool()
		body.Secure = &v
	}
	if !plan.Verify.IsNull() && !plan.Verify.IsUnknown() {
		v := plan.Verify.ValueBool()
		body.Verify = &v
	}
	if !plan.Capath.IsNull() && !plan.Capath.IsUnknown() {
		body.Capath = plan.Capath.ValueString()
	}
	if !plan.Cert.IsNull() && !plan.Cert.IsUnknown() {
		body.Cert = plan.Cert.ValueString()
	}
	if !plan.CertKey.IsNull() && !plan.CertKey.IsUnknown() {
		body.CertKey = plan.CertKey.ValueString()
	}
	if !plan.SSLVersion.IsNull() && !plan.SSLVersion.IsUnknown() {
		body.SSLVersion = plan.SSLVersion.ValueString()
	}
	if !plan.CaseSensitive.IsNull() && !plan.CaseSensitive.IsUnknown() {
		v := plan.CaseSensitive.ValueBool()
		body.CaseSensitive = &v
	}
	if !plan.CheckConnection.IsNull() && !plan.CheckConnection.IsUnknown() {
		v := plan.CheckConnection.ValueBool()
		body.CheckConnection = &v
	}
	var del []string
	if realmStringCleared(plan.Server1, state.Server1) {
		del = append(del, "server1")
	}
	if realmStringCleared(plan.Server2, state.Server2) {
		del = append(del, "server2")
	}
	if realmIntCleared(plan.Port, state.Port) {
		del = append(del, "port")
	}
	if realmStringCleared(plan.Mode, state.Mode) {
		del = append(del, "mode")
	}
	if realmBoolCleared(plan.Secure, state.Secure) {
		del = append(del, "secure")
	}
	if realmBoolCleared(plan.Verify, state.Verify) {
		del = append(del, "verify")
	}
	if realmStringCleared(plan.Capath, state.Capath) {
		del = append(del, "capath")
	}
	if realmStringCleared(plan.Cert, state.Cert) {
		del = append(del, "cert")
	}
	if realmStringCleared(plan.CertKey, state.CertKey) {
		del = append(del, "certkey")
	}
	if realmStringCleared(plan.SSLVersion, state.SSLVersion) {
		del = append(del, "sslversion")
	}
	if realmBoolCleared(plan.CaseSensitive, state.CaseSensitive) {
		del = append(del, "case-sensitive")
	}
	if realmBoolCleared(plan.CheckConnection, state.CheckConnection) {
		del = append(del, "check-connection")
	}
	return del
}

// realmLDAPApply copies the LDAP directory attributes into the wire body
// and returns the PVE field names to clear. Password is only sent when set
// (it is write-only upstream) and never cleared.
func realmLDAPApply(plan, state realmLdapModel, body *pveclient.Domain) []string {
	var del []string
	for _, f := range []struct {
		plan, state types.String
		apply       func(string)
		wire        string
	}{
		{plan.BaseDN, state.BaseDN, func(v string) { body.BaseDN = v }, "base_dn"},
		{plan.BindDN, state.BindDN, func(v string) { body.BindDN = v }, "bind_dn"},
		{plan.UserAttr, state.UserAttr, func(v string) { body.UserAttr = v }, "user_attr"},
		{plan.UserClasses, state.UserClasses, func(v string) { body.UserClasses = v }, "user_classes"},
		{plan.GroupClasses, state.GroupClasses, func(v string) { body.GroupClasses = v }, "group_classes"},
		{plan.GroupDN, state.GroupDN, func(v string) { body.GroupDN = v }, "group_dn"},
		{plan.GroupFilter, state.GroupFilter, func(v string) { body.GroupFilter = v }, "group_filter"},
		{plan.GroupNameAttr, state.GroupNameAttr, func(v string) { body.GroupNameAttr = v }, "group_name_attr"},
		{plan.Filter, state.Filter, func(v string) { body.Filter = v }, "filter"},
		{plan.SyncAttributes, state.SyncAttributes, func(v string) { body.SyncAttributes = v }, "sync_attributes"},
		{plan.SyncDefaultsOptions, state.SyncDefaultsOptions, func(v string) { body.SyncDefaultsOptions = v }, "sync-defaults-options"},
	} {
		if !f.plan.IsNull() && !f.plan.IsUnknown() {
			f.apply(f.plan.ValueString())
		}
		if realmStringCleared(f.plan, f.state) {
			del = append(del, f.wire)
		}
	}
	if !plan.Password.IsNull() && !plan.Password.IsUnknown() {
		body.Password = plan.Password.ValueString()
	}
	return del
}

// realmADApply copies the AD attributes into the wire body and returns the
// PVE field names to clear. Password is only sent when set (it is
// write-only upstream) and never cleared.
func realmADApply(plan, state realmAdModel, body *pveclient.Domain) []string {
	if !plan.Domain.IsNull() && !plan.Domain.IsUnknown() {
		body.Domain = plan.Domain.ValueString()
	}
	if !plan.BindDN.IsNull() && !plan.BindDN.IsUnknown() {
		body.BindDN = plan.BindDN.ValueString()
	}
	if !plan.Password.IsNull() && !plan.Password.IsUnknown() {
		body.Password = plan.Password.ValueString()
	}
	var del []string
	if realmStringCleared(plan.Domain, state.Domain) {
		del = append(del, "domain")
	}
	if realmStringCleared(plan.BindDN, state.BindDN) {
		del = append(del, "bind_dn")
	}
	return del
}

// realmOpenidApply copies the OpenID Connect attributes into the wire body
// and returns the PVE field names to clear. ClientKey is only sent when
// set and never cleared automatically.
func realmOpenidApply(plan, state realmOpenidModel, body *pveclient.Domain) []string {
	setStr := func(p types.String, apply func(string)) {
		if !p.IsNull() && !p.IsUnknown() {
			apply(p.ValueString())
		}
	}
	setBool := func(p types.Bool, apply func(bool)) {
		if !p.IsNull() && !p.IsUnknown() {
			apply(p.ValueBool())
		}
	}
	setStr(plan.IssuerURL, func(v string) { body.IssuerURL = v })
	setStr(plan.ClientID, func(v string) { body.ClientID = v })
	setStr(plan.ClientKey, func(v string) { body.ClientKey = v })
	setStr(plan.UsernameClaim, func(v string) { body.UsernameClaim = v })
	setStr(plan.GroupsClaim, func(v string) { body.GroupsClaim = v })
	setBool(plan.GroupsAutocreate, func(v bool) { body.GroupsAutocreate = &v })
	setBool(plan.GroupsOverwrite, func(v bool) { body.GroupsOverwrite = &v })
	setBool(plan.Autocreate, func(v bool) { body.Autocreate = &v })
	setBool(plan.QueryUserinfo, func(v bool) { body.QueryUserinfo = &v })
	setStr(plan.Scopes, func(v string) { body.Scopes = v })
	setStr(plan.Prompt, func(v string) { body.Prompt = v })
	setStr(plan.ACRValues, func(v string) { body.ACRValues = v })
	setStr(plan.Audiences, func(v string) { body.Audiences = v })
	var del []string
	if realmStringCleared(plan.IssuerURL, state.IssuerURL) {
		del = append(del, "issuer-url")
	}
	if realmStringCleared(plan.ClientID, state.ClientID) {
		del = append(del, "client-id")
	}
	if realmStringCleared(plan.ClientKey, state.ClientKey) {
		del = append(del, "client-key")
	}
	if realmStringCleared(plan.UsernameClaim, state.UsernameClaim) {
		del = append(del, "username-claim")
	}
	if realmStringCleared(plan.GroupsClaim, state.GroupsClaim) {
		del = append(del, "groups-claim")
	}
	if realmBoolCleared(plan.GroupsAutocreate, state.GroupsAutocreate) {
		del = append(del, "groups-autocreate")
	}
	if realmBoolCleared(plan.GroupsOverwrite, state.GroupsOverwrite) {
		del = append(del, "groups-overwrite")
	}
	if realmBoolCleared(plan.Autocreate, state.Autocreate) {
		del = append(del, "autocreate")
	}
	if realmBoolCleared(plan.QueryUserinfo, state.QueryUserinfo) {
		del = append(del, "query-userinfo")
	}
	if realmStringCleared(plan.Scopes, state.Scopes) {
		del = append(del, "scopes")
	}
	if realmStringCleared(plan.Prompt, state.Prompt) {
		del = append(del, "prompt")
	}
	if realmStringCleared(plan.ACRValues, state.ACRValues) {
		del = append(del, "acr-values")
	}
	if realmStringCleared(plan.Audiences, state.Audiences) {
		del = append(del, "audiences")
	}
	return del
}

// realmCommonIntoModel populates the common attributes from a wire read.
// Absent upstream values become Terraform nulls (never zero values) so
// optional attributes do not churn between reads and plans.
func realmCommonIntoModel(m *realmCommonModel, d *pveclient.Domain) {
	m.Realm = types.StringValue(d.Realm)
	m.Comment = realmStringToTF(d.Comment)
	m.Default = realmBoolPtrToTF(d.Default)
	m.TFA = realmStringToTF(d.TFA)
	m.Digest = realmStringToTF(d.Digest)
}

// realmTransportIntoModel populates the transport attributes from a wire
// read.
func realmTransportIntoModel(m *realmTransportModel, d *pveclient.Domain) {
	m.Server1 = realmStringToTF(d.Server1)
	m.Server2 = realmStringToTF(d.Server2)
	m.Port = realmIntPtrToTF(d.Port)
	m.Mode = realmStringToTF(d.Mode)
	m.Secure = realmBoolPtrToTF(d.Secure)
	m.Verify = realmBoolPtrToTF(d.Verify)
	m.Capath = realmStringToTF(d.Capath)
	m.Cert = realmStringToTF(d.Cert)
	m.CertKey = realmStringToTF(d.CertKey)
	m.SSLVersion = realmStringToTF(d.SSLVersion)
	m.CaseSensitive = realmBoolPtrToTF(d.CaseSensitive)
	m.CheckConnection = realmBoolPtrToTF(d.CheckConnection)
}

// realmLDAPIntoModel populates the LDAP directory attributes from a wire
// read. Password is write-only upstream and keeps its configured value.
func realmLDAPIntoModel(m *realmLdapModel, d *pveclient.Domain) {
	m.BaseDN = realmStringToTF(d.BaseDN)
	m.BindDN = realmStringToTF(d.BindDN)
	m.UserAttr = realmStringToTF(d.UserAttr)
	m.UserClasses = realmStringToTF(d.UserClasses)
	m.GroupClasses = realmStringToTF(d.GroupClasses)
	m.GroupDN = realmStringToTF(d.GroupDN)
	m.GroupFilter = realmStringToTF(d.GroupFilter)
	m.GroupNameAttr = realmStringToTF(d.GroupNameAttr)
	m.Filter = realmStringToTF(d.Filter)
	m.SyncAttributes = realmStringToTF(d.SyncAttributes)
	m.SyncDefaultsOptions = realmStringToTF(d.SyncDefaultsOptions)
}

// realmADIntoModel populates the AD attributes from a wire read. Password
// is write-only upstream and keeps its configured value.
func realmADIntoModel(m *realmAdModel, d *pveclient.Domain) {
	m.Domain = realmStringToTF(d.Domain)
	m.BindDN = realmStringToTF(d.BindDN)
}

// realmOpenidIntoModel populates the OpenID Connect attributes from a wire
// read. ClientKey may be withheld upstream and keeps its configured value.
func realmOpenidIntoModel(m *realmOpenidModel, d *pveclient.Domain) {
	m.IssuerURL = realmStringToTF(d.IssuerURL)
	m.ClientID = realmStringToTF(d.ClientID)
	m.UsernameClaim = realmStringToTF(d.UsernameClaim)
	m.GroupsClaim = realmStringToTF(d.GroupsClaim)
	m.GroupsAutocreate = realmBoolPtrToTF(d.GroupsAutocreate)
	m.GroupsOverwrite = realmBoolPtrToTF(d.GroupsOverwrite)
	m.Autocreate = realmBoolPtrToTF(d.Autocreate)
	m.QueryUserinfo = realmBoolPtrToTF(d.QueryUserinfo)
	m.Scopes = realmStringToTF(d.Scopes)
	m.Prompt = realmStringToTF(d.Prompt)
	m.ACRValues = realmStringToTF(d.ACRValues)
	m.Audiences = realmStringToTF(d.Audiences)
}

// realmGetChecked reads one realm and errors when the upstream type does
// not match the component's fixed type, so a component never silently
// adopts a realm of another kind.
func realmGetChecked(ctx context.Context, client *pveclient.Client, realm, wantType string) (*pveclient.Domain, error) {
	domain, err := client.GetDomain(ctx, realm)
	if err != nil {
		return nil, err
	}
	if domain.Type != wantType {
		return nil, &realmTypeError{Realm: realm, Got: domain.Type, Want: wantType}
	}
	return domain, nil
}

// realmTypeError reports an upstream realm whose type differs from the
// component's fixed type.
type realmTypeError struct {
	Realm string
	Got   string
	Want  string
}

func (e *realmTypeError) Error() string {
	return fmt.Sprintf("realm %q has upstream type %q but this component manages type %q", e.Realm, e.Got, e.Want)
}

// realmConfigureResource extracts the shared client from provider data.
// Nil provider data leaves the resource unconfigured (unit tests).
func realmConfigureResource(req resource.ConfigureRequest, resp *resource.ConfigureResponse) *pveclient.Client {
	if req.ProviderData == nil {
		return nil
	}
	client, ok := req.ProviderData.(*pveclient.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *pveclient.Client, got: %T.", req.ProviderData),
		)
		return nil
	}
	return client
}

// realmStringCleared reports a string attr null in plan but set in state.
func realmStringCleared(plan, state types.String) bool {
	return plan.IsNull() && !state.IsNull()
}

// realmBoolCleared reports a bool attr null in plan but set in state.
func realmBoolCleared(plan, state types.Bool) bool {
	return plan.IsNull() && !state.IsNull() && !state.IsUnknown()
}

// realmIntCleared reports an int attr null in plan but set in state.
func realmIntCleared(plan, state types.Int64) bool {
	return plan.IsNull() && !state.IsNull()
}

// realmStringToTF maps the empty string to null.
func realmStringToTF(s string) types.String {
	if s == "" {
		return types.StringNull()
	}
	return types.StringValue(s)
}

// realmBoolPtrToTF converts an optional wire bool to Terraform, mapping a
// nil pointer to null.
func realmBoolPtrToTF(v *bool) types.Bool {
	if v == nil {
		return types.BoolNull()
	}
	return types.BoolValue(*v)
}

// realmIntPtrToTF converts an optional wire int to Terraform, mapping a
// nil pointer to null.
func realmIntPtrToTF(v *int) types.Int64 {
	if v == nil {
		return types.Int64Null()
	}
	return types.Int64Value(int64(*v))
}

// realmDataSourceAttributes converts a resource-side realm attribute set
// (leaf String/Bool/Int64 attributes only) into its data-source
// equivalent. RequiresReplace plan modifiers are intentionally dropped:
// data sources have no plan. Any unsupported attribute type is a schema
// bug and panics so the schema tests fail loudly.
func realmDataSourceAttributes(in map[string]schema.Attribute) map[string]datasourceschema.Attribute {
	out := make(map[string]datasourceschema.Attribute, len(in))
	for name, attr := range in {
		switch a := attr.(type) {
		case schema.StringAttribute:
			out[name] = datasourceschema.StringAttribute{
				Required:            a.Required,
				Optional:            a.Optional,
				Computed:            a.Computed,
				Sensitive:           a.Sensitive,
				MarkdownDescription: a.MarkdownDescription,
				Validators:          a.Validators,
			}
		case schema.BoolAttribute:
			out[name] = datasourceschema.BoolAttribute{
				Required:            a.Required,
				Optional:            a.Optional,
				Computed:            a.Computed,
				Sensitive:           a.Sensitive,
				MarkdownDescription: a.MarkdownDescription,
				Validators:          a.Validators,
			}
		case schema.Int64Attribute:
			out[name] = datasourceschema.Int64Attribute{
				Required:            a.Required,
				Optional:            a.Optional,
				Computed:            a.Computed,
				Sensitive:           a.Sensitive,
				MarkdownDescription: a.MarkdownDescription,
				Validators:          a.Validators,
			}
		default:
			panic(fmt.Sprintf("realmDataSourceAttributes: attribute %q has unsupported type %T", name, attr))
		}
	}
	return out
}
