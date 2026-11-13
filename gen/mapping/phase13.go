// Copyright dwhoban 2026
// SPDX-License-Identifier: MPL-2.0

package mapping

// Phase13 — Integrations: notification endpoints & matchers (plus the
// target test action and matcher-field metadata data sources), external
// metric servers, ACME accounts/plugins/directories, and node certificates
// (ACME-ordered or custom-uploaded).
var Phase13 = []Type{
	//
	// Notification endpoints (cluster-wide config under
	// /cluster/notifications/endpoints/<kind>).
	//
	{
		Type: "pve_notification_endpoint_gotify", Kind: "resource", Phase: 13,
		Summary: "Gotify push notification endpoint.",
		Create:  &Endpoint{Path: "/cluster/notifications/endpoints/gotify"},
		Read:    &Endpoint{Path: "/cluster/notifications/endpoints/gotify/{name}"},
		Update:  &Endpoint{Path: "/cluster/notifications/endpoints/gotify/{name}"},
		Delete:  &Endpoint{Path: "/cluster/notifications/endpoints/gotify/{name}"},
		Attrs:   []AttrRule{{Name: "name", ForceNew: true}},
	},
	{
		Type: "pve_notification_endpoint_sendmail", Kind: "resource", Phase: 13,
		Summary: "Sendmail (email) notification endpoint.",
		Create:  &Endpoint{Path: "/cluster/notifications/endpoints/sendmail"},
		Read:    &Endpoint{Path: "/cluster/notifications/endpoints/sendmail/{name}"},
		Update:  &Endpoint{Path: "/cluster/notifications/endpoints/sendmail/{name}"},
		Delete:  &Endpoint{Path: "/cluster/notifications/endpoints/sendmail/{name}"},
		Attrs:   []AttrRule{{Name: "name", ForceNew: true}},
	},
	{
		Type: "pve_notification_endpoint_smtp", Kind: "resource", Phase: 13,
		Summary: "SMTP notification endpoint.",
		Create:  &Endpoint{Path: "/cluster/notifications/endpoints/smtp"},
		Read:    &Endpoint{Path: "/cluster/notifications/endpoints/smtp/{name}"},
		Update:  &Endpoint{Path: "/cluster/notifications/endpoints/smtp/{name}"},
		Delete:  &Endpoint{Path: "/cluster/notifications/endpoints/smtp/{name}"},
		Attrs:   []AttrRule{{Name: "name", ForceNew: true}},
	},
	{
		Type: "pve_notification_endpoint_webhook", Kind: "resource", Phase: 13,
		Summary: "Webhook notification endpoint.",
		Create:  &Endpoint{Path: "/cluster/notifications/endpoints/webhook"},
		Read:    &Endpoint{Path: "/cluster/notifications/endpoints/webhook/{name}"},
		Update:  &Endpoint{Path: "/cluster/notifications/endpoints/webhook/{name}"},
		Delete:  &Endpoint{Path: "/cluster/notifications/endpoints/webhook/{name}"},
		Attrs:   []AttrRule{{Name: "name", ForceNew: true}},
	},
	{
		Type: "pve_notification_matcher", Kind: "resource", Phase: 13,
		Summary: "Notification matcher routing notifications to targets by metadata, severity, and calendar.",
		Create:  &Endpoint{Path: "/cluster/notifications/matchers"},
		Read:    &Endpoint{Path: "/cluster/notifications/matchers/{name}"},
		Update:  &Endpoint{Path: "/cluster/notifications/matchers/{name}"},
		Delete:  &Endpoint{Path: "/cluster/notifications/matchers/{name}"},
		Attrs:   []AttrRule{{Name: "name", ForceNew: true}},
	},

	// One-shot target test notification.
	{
		Type: "pve_notification_target_test", Kind: "action", Phase: 13,
		Summary: "Send a test notification to a notification target.",
		Invoke:  &Endpoint{Path: "/cluster/notifications/targets/{name}/test"},
	},

	//
	// External metric servers. Create is POST on the item path (the
	// collection is read-only).
	//
	{
		Type: "pve_metric_server", Kind: "resource", Phase: 13,
		Summary: "External metric server (graphite, influxdb, or opentelemetry).",
		Create:  &Endpoint{Path: "/cluster/metrics/server/{id}"},
		Read:    &Endpoint{Path: "/cluster/metrics/server/{id}"},
		Update:  &Endpoint{Path: "/cluster/metrics/server/{id}"},
		Delete:  &Endpoint{Path: "/cluster/metrics/server/{id}"},
		IDAttrs: []string{"item_id"},
		Attrs:   []AttrRule{{Name: "item_id", PathParam: "id", ForceNew: true}},
	},
	{
		Type: "pve_metric_servers", Kind: "data_source", Phase: 13,
		Summary: "Configured external metric servers.",
		Reads:   []*Endpoint{{Path: "/cluster/metrics/server", ResultName: "servers"}},
	},

	//
	// ACME accounts and challenge plugins.
	//
	{
		Type: "pve_acme_account", Kind: "resource", Phase: 13,
		Summary: "ACME account registered with a certificate authority.",
		Create:  &Endpoint{Path: "/cluster/acme/account"},
		Read:    &Endpoint{Path: "/cluster/acme/account/{name}"},
		Update:  &Endpoint{Path: "/cluster/acme/account/{name}"},
		Delete:  &Endpoint{Path: "/cluster/acme/account/{name}"},
		Attrs: []AttrRule{
			// The API default ("default") only applies to Optional+Computed.
			{Name: "name", Optional: BoolPtr(true), Computed: BoolPtr(true), ForceNew: true},
			{Name: "eab_hmac_key", Sensitive: true},
		},
	},
	{
		Type: "pve_acme_plugin", Kind: "resource", Phase: 13,
		Summary: "ACME challenge plugin (standalone http-01 or a DNS API).",
		Create:  &Endpoint{Path: "/cluster/acme/plugins"},
		Read:    &Endpoint{Path: "/cluster/acme/plugins/{id}"},
		Update:  &Endpoint{Path: "/cluster/acme/plugins/{id}"},
		Delete:  &Endpoint{Path: "/cluster/acme/plugins/{id}"},
		IDAttrs: []string{"item_id"},
		Attrs: []AttrRule{
			{Name: "item_id", PathParam: "id", ForceNew: true},
			{Name: "data", Sensitive: true}, // base64 DNS API credentials
		},
	},

	//
	// Node certificates. Neither item family has a GET; both flows deploy
	// to /etc/pve/local/pveproxy-ssl.pem, so existence is read from the
	// node's certificate info list filtered by filename.
	//
	{
		Type: "pve_node_certificate", Kind: "resource", Phase: 13,
		Summary: "Node certificate ordered from an ACME CA (create orders, update renews, destroy revokes).",
		Create:  &Endpoint{Path: "/nodes/{node}/certificates/acme/certificate"},
		Read: &Endpoint{
			Path:   "/nodes/{node}/certificates/info",
			Filter: [][2]string{{"filename", "filename"}},
		},
		Update:  &Endpoint{Path: "/nodes/{node}/certificates/acme/certificate"},
		Delete:  &Endpoint{Path: "/nodes/{node}/certificates/acme/certificate"},
		IDAttrs: []string{"node", "filename"},
		Attrs: []AttrRule{
			{Name: "node", ForceNew: true},
			{Name: "filename", Type: "string", Add: true, Optional: BoolPtr(true), Computed: BoolPtr(true), Default: "pveproxy-ssl.pem"},
		},
	},
	{
		Type: "pve_node_certificate_custom", Kind: "resource", Phase: 13,
		Summary: "Custom certificate chain and key uploaded to a node's pveproxy.",
		Create:  &Endpoint{Path: "/nodes/{node}/certificates/custom"},
		Read: &Endpoint{
			Path:   "/nodes/{node}/certificates/info",
			Filter: [][2]string{{"filename", "filename"}},
		},
		Delete:  &Endpoint{Path: "/nodes/{node}/certificates/custom"},
		IDAttrs: []string{"node", "filename"},
		Attrs: []AttrRule{
			{Name: "node", ForceNew: true},
			{Name: "filename", Type: "string", Add: true, Optional: BoolPtr(true), Computed: BoolPtr(true), Default: "pveproxy-ssl.pem"},
			{Name: "key", Sensitive: true}, // PEM private key
		},
		AllForceNew: true, // no update endpoint: re-upload replaces
	},

	//
	// ACME metadata data sources.
	//
	{
		Type: "pve_acme_directories", Kind: "data_source", Phase: 13,
		Summary: "Known ACME directory endpoints (Let's Encrypt, staging, custom).",
		Reads:   []*Endpoint{{Path: "/cluster/acme/directories", ResultName: "directories"}},
	},
	{
		Type: "pve_acme_meta", Kind: "data_source", Phase: 13,
		Summary: "ACME CA metadata: directory meta, terms of service, and challenge type schemas.",
		Reads: []*Endpoint{
			{Path: "/cluster/acme/meta", ResultName: "meta"},
			{Path: "/cluster/acme/tos", ResultName: "tos"},
			{Path: "/cluster/acme/challenge-schema", ResultName: "challenge_schemas"},
			{Path: "/cluster/acme", ResultName: "index"},
		},
	},

	// Node certificate inventory.
	{
		Type: "pve_certificate_info", Kind: "data_source", Phase: 13,
		Summary: "Certificates installed on a node: full info list plus the certificate/ACME indexes.",
		Reads: []*Endpoint{
			{Path: "/nodes/{node}/certificates/info", ResultName: "certificates"},
			{Path: "/nodes/{node}/certificates", ResultName: "index"},
			{Path: "/nodes/{node}/certificates/acme", ResultName: "acme_index"},
		},
	},

	//
	// Notification inventory data sources.
	//
	{
		Type: "pve_notification_targets", Kind: "data_source", Phase: 13,
		Summary: "All usable notification targets (endpoints, matchers, built-ins).",
		Reads: []*Endpoint{
			{Path: "/cluster/notifications/targets", ResultName: "targets"},
			{Path: "/cluster/notifications/endpoints", ResultName: "endpoint_types"},
			{Path: "/cluster/notifications", ResultName: "index"},
		},
	},
	{
		Type: "pve_notification_matcher_fields", Kind: "data_source", Phase: 13,
		Summary: "Known notification metadata fields and their known values.",
		Reads: []*Endpoint{
			{Path: "/cluster/notifications/matcher-fields", ResultName: "fields"},
			{Path: "/cluster/notifications/matcher-field-values", ResultName: "field_values"},
		},
	},
}
