# terraform-provider-pve — agent rules

## Implementation standard: terraform-plugin-framework

All provider code — resources, data sources, provider-defined functions, actions,
ephemeral resources, provider schema, and `gen/` emitted code — is implemented with
`github.com/hashicorp/terraform-plugin-framework` (pinned in `go.mod`).

Deviations — e.g. `terraform-plugin-sdk/v2`, muxed providers, or bypassing Framework
interfaces — require explicit user approval **before any further work occurs**, except
where a HashiCorp-provided Terraform skill recommends or suggests them (e.g. the
`retry.StateChangeConf` helper from `terraform-plugin-sdk/v2`). Otherwise stop, ask, and
wait for approval; do not continue the affected task without it.
