# Terraform Provider PVE

A [Terraform](https://www.terraform.io) provider for [Proxmox Virtual Environment](https://pve.proxmox.com) built on the [Terraform Plugin Framework](https://developer.hashicorp.com/terraform/plugin/framework). Goal: every declarative aspect of a Proxmox node and cluster configurable via Terraform — see [ROADMAP.md](ROADMAP.md) for the phased plan and current status.

**Status:** all roadmap phases implemented. The provider surface — **223 types** — 222 generated (72 resources, 73 data sources, 75 actions, 2 functions) plus the hand-written `pve_version` data source covering all 388 in-scope API path families — is **generated from the official API schema** (`apidoc.js`), not hand-written. Live-cluster acceptance tests (`TF_ACC`) are pending a lab; development verification is `go build`, `go vet`, `golangci-lint`, and unit tests against an in-process PVE stub.

## Requirements

- [Terraform](https://developer.hashicorp.com/terraform/downloads) >= 1.14 (actions; >= 1.8 for functions)
- [Go](https://golang.org/doc/install) >= 1.25.8 (development only)
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
| `api_token` | `PVE_API_TOKEN` | Recommended auth; `user@realm!tokenid=uuid`; alternative: split `api_token_id` + `api_secret` below |
| `api_token_id` / `api_secret` | `PVE_API_TOKEN_ID` / `PVE_API_SECRET` | Alternative to `api_token`: paste the two dashboard values (Token ID + UUID secret) as-is; mutually exclusive with `api_token` |
| `username` / `password` (+ `totp`) | `PVE_USER` / `PVE_PASSWORD` / `PVE_TOTP` | Session-ticket auth; token takes precedence |
| `insecure` | `PVE_INSECURE` | Skip TLS verification (self-signed certs) |
| `ca_cert` | `PVE_CA_CERT` | PEM bundle to verify the endpoint |
| `request_timeout` | `PVE_REQUEST_TIMEOUT` | Per-request timeout, seconds (default 30) |
| `skip_credentials_validation` | — | Skip the plan-time auth check |

### SSH (optional)

The provider is API-first; **most configurations never need SSH** and, without an `ssh` block, no SSH connection is ever made. SSH is used only where the Proxmox VE API cannot do the job: installing Ceph (`pve_ceph_install` action → `pveceph install`), QDevice setup (`pve_cluster_qdevice` resource → `pvecm qdevice`) and, opt-in, `ssh.root_ticket` for `root@pam`-only API calls (node calls, device passthrough, cluster create/join) made with an API token. Host keys are verified against `known_hosts`; non-root users run through `sudo -n` and need a strict sudoers rule. Details: [When is SSH required?](docs/guides/ssh.md).

```hcl
provider "pve" {
  api_url   = "https://pve.example.com:8006"
  api_token = "root@pam!terraform=00000000-0000-0000-0000-000000000000"

  ssh {
    username = "root"
    agent    = true
  }
}
```

Credentials are validated when the provider configures, so invalid credentials fail the plan with an actionable error instead of surfacing mid-apply.

## What's included

Everything from inventory to Ceph: guests (`pve_vm`, `pve_container` + snapshots, clone-at-create, start/stop/migrate actions), storage/pools/files, node system management (networks with PVE's staged-apply semantics, DNS/time, disks, services, APT), RBAC (users, roles, API tokens, ACLs, realms), cluster lifecycle and mappings, backup/replication/HA, scoped firewall (cluster/node/vm/container), SDN, Ceph, and integrations (notifications, metrics, ACME certificates). `nextid()` and `schedule_analyze()` are provider functions.

Browse [`docs/`](docs) for the full type reference (generated) and [`examples/`](examples) for minimal configurations. A flavor of the surface:

```hcl
data "pve_nodes" "all" {}
data "pve_version" "current" {}

resource "pve_vm" "app" {
  node = "pve1"
  vmid = 101
  name = "app-01"
}
```

Async PVE operations (create, migrate, backup, …) are transparent: the provider polls the returned task UPID to completion and surfaces the task-log tail on failure.

## How it is built (generated provider)

The type surface is a build artifact, regenerated and audited on demand:

```text
apidoc.js ──internal/pveapidoc──▶ typed API tree (449 families)
                                      │
ROADMAP.md phase tables ──gen/mapping/phase*.go──▶ mappings
                                      │
                          go run ./gen (audit: every family used exactly
                          once, OOS never referenced, phases consistent)
                                      │
                                      ▼
             internal/provider/registry.json ──embedded──▶ generic engines
             (resource / data source / action / function)
```

- **No per-type provider code.** Four generic engines interpret registry descriptors; attributes derive from the API schema (parameters → inputs, response schemas → computed attributes, path placeholders → force-new identifiers).
- **Coverage guarantee.** The generator exits non-zero on any unassigned, missing, or out-of-scope-referenced API family; ROADMAP Appendix A is its output.
- **Import** works for every resource (`terraform import pve_vm.x <node>/<vmid>` — the ID format is documented per type).

## Development

```bash
make build        # go build ./...
make test         # unit tests (no cluster needed)
make lint         # golangci-lint
make gen          # regenerate registry + Appendix A + examples (audited)
make generate     # gen + tfplugindocs (docs/) + fmt
make testacc      # acceptance tests against a live lab (TF_ACC=1)
```

The mapping guide for adding or adjusting types lives in [`gen/MAPPING.md`](gen/MAPPING.md).
