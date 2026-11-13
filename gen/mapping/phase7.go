// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package mapping

// Phase7 — access control (RBAC). See gen/MAPPING.md.
var Phase7 = []Type{
	{
		Type: "pve_user", Kind: "resource", Phase: 7,
		Summary: "API user with realm, groups, TFA settings, and account expiry.",
		Create:  &Endpoint{Path: "/access/users"},
		Read:    &Endpoint{Path: "/access/users/{userid}"},
		Update:  &Endpoint{Path: "/access/users/{userid}"},
		Delete:  &Endpoint{Path: "/access/users/{userid}"},
		Attrs: []AttrRule{
			{Name: "userid", ForceNew: true},
			{Name: "password", Sensitive: true},
		},
	},
	{
		Type: "pve_user", Kind: "data_source", Phase: 7,
		Summary: "One API user's configuration.",
		Reads: []*Endpoint{
			{Path: "/access/users/{userid}", ResultName: "user"},
			{Path: "/access/users/{userid}/tfa", ResultName: "tfa_entries"},
			{Path: "/access/users/{userid}/token", ResultName: "api_tokens"},
		},
	},
	{
		Type: "pve_user_unlock_tfa", Kind: "action", Phase: 7,
		Summary: "Unlock a user's TFA after too many failed attempts.",
		Invoke:  &Endpoint{Path: "/access/users/{userid}/unlock-tfa", Method: "PUT"},
	},
	{
		Type: "pve_group", Kind: "resource", Phase: 7,
		Summary: "Access control group.",
		Create:  &Endpoint{Path: "/access/groups"},
		Read:    &Endpoint{Path: "/access/groups/{groupid}"},
		Update:  &Endpoint{Path: "/access/groups/{groupid}"},
		Delete:  &Endpoint{Path: "/access/groups/{groupid}"},
		Attrs:   []AttrRule{{Name: "groupid", ForceNew: true}},
	},
	{
		Type: "pve_role", Kind: "resource", Phase: 7,
		Summary: "Privilege role.",
		Create:  &Endpoint{Path: "/access/roles"},
		Read:    &Endpoint{Path: "/access/roles/{roleid}"},
		Update:  &Endpoint{Path: "/access/roles/{roleid}"},
		Delete:  &Endpoint{Path: "/access/roles/{roleid}"},
		Attrs:   []AttrRule{{Name: "roleid", ForceNew: true}},
	},
	{
		Type: "pve_api_token", Kind: "resource", Phase: 7,
		Summary: "API token of a user. The full token value is captured into token_value (sensitive) at creation and whenever regenerate = true invalidates and reissues the secret — PVE returns it exactly once and it can never be read back afterward.",
		Create:  &Endpoint{Path: "/access/users/{userid}/token/{tokenid}", ResultAttr: "token_value"},
		Read:    &Endpoint{Path: "/access/users/{userid}/token/{tokenid}"},
		Update:  &Endpoint{Path: "/access/users/{userid}/token/{tokenid}", Method: "PUT", ResultAttr: "token_value", ResultField: "value"},
		Delete:  &Endpoint{Path: "/access/users/{userid}/token/{tokenid}", Method: "DELETE"},
		IDAttrs: []string{"userid", "tokenid"},
		Attrs: []AttrRule{
			{Name: "userid", ForceNew: true},
			{Name: "tokenid", ForceNew: true},
			{Name: "token_value", Type: "string", Add: true, Computed: BoolPtr(true), Sensitive: true},
		},
	},
	{
		Type: "pve_acl", Kind: "resource", Phase: 7,
		Summary:  "ACL grant of one role on one path to users and/or groups. Declare one resource per (path, role); matching entries are looked up in the ACL list.",
		Behavior: "acl",
		Create:   &Endpoint{Path: "/access/acl", Method: "PUT"},
		Read: &Endpoint{
			Path:   "/access/acl",
			Method: "GET",
			Filter: [][2]string{{"path", "path"}, {"roleid", "roles"}},
		},
		Update: &Endpoint{Path: "/access/acl", Method: "PUT"},
		// The API has no DELETE; removal is PUT with delete=1.
		Delete:  &Endpoint{Path: "/access/acl", Method: "PUT", ConstParams: map[string]string{"delete": "1"}},
		IDAttrs: []string{"path", "roles"},
		IDSep:   ":",
		Attrs: []AttrRule{
			{Name: "path", ForceNew: true},
			{Name: "roles", ForceNew: true},
		},
	},
	{
		Type: "pve_realm", Kind: "resource", Phase: 7,
		Summary: "Authentication realm (PAM, PVE, LDAP, OpenID, …).",
		Create:  &Endpoint{Path: "/access/domains"},
		Read:    &Endpoint{Path: "/access/domains/{realm}"},
		Update:  &Endpoint{Path: "/access/domains/{realm}"},
		Delete:  &Endpoint{Path: "/access/domains/{realm}"},
		Attrs: []AttrRule{
			{Name: "realm", ForceNew: true},
			{Name: "password", Sensitive: true},
		},
	},
	{
		Type: "pve_realm_sync", Kind: "action", Phase: 7,
		Summary: "Run a realm sync (optionally dry-run or with a specific preview).",
		Invoke:  &Endpoint{Path: "/access/domains/{realm}/sync"},
	},
	{
		Type: "pve_permissions", Kind: "data_source", Phase: 7,
		Summary: "Effective permission tree of the authenticated user plus the realm/domain inventory.",
		Reads: []*Endpoint{
			{Path: "/access/permissions", ResultName: "permissions"},
			{Path: "/access", ResultName: "access_index"},
		},
	},
}
