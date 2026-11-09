# Changelog

## 0.1.0 (unreleased)

Phase 1 — foundation ([ROADMAP.md](ROADMAP.md)):

- Provider `pve` replaces the HashiCorp scaffold: module `github.com/dwhoban/terraform-provider-pve`, address `registry.terraform.io/dwhoban/pve`.
- Provider configuration with credential resolution: static config > environment variables; API token (`PVE_API_TOKEN`) or username/password + optional TOTP (`PVE_USER`/`PVE_PASSWORD`/`PVE_TOTP`), `api_url`, `insecure`, `ca_cert`, `request_timeout`, and plan-time credential validation (skippable).
- PVE REST client: `/api2/json` envelope handling, typed API errors, TLS options, retry with backoff on idempotent GETs, session-ticket auth with automatic re-authentication.
- Task engine: UPID parsing and polling with timeout, exit-status errors carrying the task log tail.
- New data source: `pve_version`.
