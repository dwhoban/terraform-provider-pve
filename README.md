# Terraform Provider PVE

A [Terraform](https://www.terraform.io) provider for [Proxmox Virtual Environment](https://pve.proxmox.com) built on the [Terraform Plugin Framework](https://developer.hashicorp.com/terraform/plugin/framework). Goal: every declarative aspect of a Proxmox node and cluster configurable via Terraform — see [ROADMAP.md](ROADMAP.md) for the phased plan and current status.

**Status:** Phase 1 (foundation) — provider configuration, API client, task engine, and the `pve_version` data source.

## Requirements

- [Terraform](https://developer.hashicorp.com/terraform/downloads) >= 1.0
- [Go](https://golang.org/doc/install) >= 1.24 (development only)
- Proxmox VE >= 8.0 with an API token or user credentials

## Using the Provider

Configure the endpoint and credentials explicitly, or via environment variables:

```hcl
provider "pve" {
  api_url   = "https://pve.example.com:8006"
  api_token = "root@pam!terraform=00000000-0000-0000-0000-000000000000"
}
```

| Attribute | Environment variable | Notes |
|---|---|---|
| `api_url` | `PVE_API_URL` | Required; `https://`, port defaults to 8006 |
| `api_token` | `PVE_API_TOKEN` | Recommended auth; `user@realm!tokenid=uuid` |
| `username` / `password` (+ `totp`) | `PVE_USER` / `PVE_PASSWORD` / `PVE_TOTP` | Session-ticket auth; token takes precedence |
| `insecure` | `PVE_INSECURE` | Skip TLS verification (self-signed certs) |
| `ca_cert` | `PVE_CA_CERT` | PEM bundle to verify the endpoint |
| `request_timeout` | `PVE_REQUEST_TIMEOUT` | Per-request timeout, seconds (default 30) |
| `skip_credentials_validation` | — | Skip the plan-time auth check |

Credentials are validated when the provider configures, so invalid credentials fail the plan with an actionable error instead of surfacing mid-apply.

Data sources available so far:

```hcl
data "pve_version" "current" {}

output "pve_version" {
  value = data.pve_version.current.version
}
```

## Developing the Provider

```shell
make build      # go build ./...
make test       # unit tests
make generate   # regenerate docs/ from schemas + examples/
```

Acceptance tests run against a live Proxmox VE endpoint:

```shell
export PVE_API_URL=https://pve.example.com:8006
export PVE_API_TOKEN='root@pam!terraform=uuid'
export PVE_INSECURE=true   # if the endpoint uses a self-signed certificate
make testacc
```

## License

[MPL-2.0](LICENSE)
