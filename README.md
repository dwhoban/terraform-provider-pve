# Terraform Provider for Proxmox VE (PVE)

A [Terraform](https://www.terraform.io) provider for [Proxmox Virtual Environment](https://www.proxmox.com/en/proxmox-virtual-environment), built on the [Terraform Plugin Framework](https://github.com/hashicorp/terraform-plugin-framework). It manages a PVE cluster through the REST API: access control, cluster and HA configuration, firewall, backup and replication, metrics and notifications, storage, nodes, guests (QEMU and LXC), certificates, and SDN.

The API surface is partitioned per [ADR 0001](docs/adr/0001-proxmox-api-endpoint-treatment.md): every sanctioned component maps to exactly one Terraform treatment (resource, data source, action, or function), pinned to the vendored API spec in `api-spec/apidoc.js`. `TestProvider_RegisteredSurface` asserts the registered inventory so the surface cannot drift silently.

## Requirements

- [Terraform](https://developer.hashicorp.com/terraform/downloads) >= 1.0 (>= 1.14 for actions)
- [Go](https://golang.org/doc/install) >= 1.25.8 (to build the provider)

## Using the Provider

Configure the provider with an endpoint plus either an API token or a username/password (environment variables `PROXMOX_VE_ENDPOINT`, `PROXMOX_VE_API_TOKEN`, `PROXMOX_VE_USERNAME`, `PROXMOX_VE_PASSWORD`, and friends are also honored):

```hcl
provider "pve" {
  endpoint = "https://pve.example.com:8006/"
  api_token = "root@pam!terraform=uuid"
}
```

For the upstream API operations and schemas this provider targets, see the [Proxmox VE API Viewer](https://pve.proxmox.com/pve-docs/api-viewer/apidoc.js).

## Building the Provider

```shell
go install
```

## Developing the Provider

- `make build` — compile the provider.
- `make test` — unit and schema tests (no cluster required).
- `make lint` — pinned golangci-lint plus the custom anti-slop plugin (builds `bin/custom-gcl`; requires `golangci-lint` on `PATH`).
- `make generate` — regenerate `docs/` via tfplugindocs and format examples (requires `terraform` on `PATH`).
- `make validate-docs` — validate generated documentation against the provider schema.

Adding or changing a component? Register it in `internal/provider/provider.go` and add its type name to `TestProvider_RegisteredSurface` in the same package; derive attribute sets from `api-spec/apidoc.js` (grep, never whole-file read).

To add a dependency:

```shell
go get github.com/author/dependency
go mod tidy
```

Then commit the changes to `go.mod` and `go.sum`.

In order to run the acceptance tests, point the provider at a disposable cluster via the `PROXMOX_VE_*` environment variables and run `make testacc`.

*Note:* Acceptance tests create real resources on a live Proxmox VE cluster.

```shell
make testacc
```
