# terraform-provider-pve — agent rules

## Implementation standard: terraform-plugin-framework

All provider code — resources, data sources, provider-defined functions, actions,
ephemeral resources, provider schema, and `gen/` emitted code — is implemented with
`github.com/hashicorp/terraform-plugin-framework` (pinned in `go.mod`).

Deviations — e.g. `terraform-plugin-sdk/v2`, muxed providers, or bypassing Framework
interfaces — require explicit user approval **before any further work occurs**.
Stop, ask, and wait for approval; do not continue the affected task without it.
