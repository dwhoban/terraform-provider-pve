# Repository Guidelines

## Project Overview

This repository is a HashiCorp Terraform Plugin Framework provider scaffold, not a completed Proxmox provider. It demonstrates one resource, data source, ephemeral resource, action, and function under `internal/provider/`, plus Terraform examples and generated Registry documentation. Replace the example implementations when adding real provider behavior.

Requirements are Go `1.25.8` from `go.mod` and Terraform `>= 1.0` (CI exercises Terraform `1.13.*` and `1.14.*`).

## Architecture & Data Flow

- `main.go` parses `-debug`, configures protocol 6 serving, and calls `providerserver.Serve(..., provider.New(version), ...)`.
- `internal/provider/provider.go` implements `ScaffoldingProvider`, declares provider schema, configures provider data, and registers framework capabilities.
- Provider `Configure` decodes `ScaffoldingProviderModel`, appends diagnostics, and currently passes `http.DefaultClient` through `ResourceData` and `DataSourceData`.
- Resources, data sources, and actions receive provider data in `Configure`, assert its concrete type, and retain the client on their implementation struct.
- Operations decode Terraform config, plan, or state into framework models, perform provider work, then append diagnostics while writing state or results.
- Current examples are intentionally local: resource/data-source IDs and ephemeral values are hardcoded; no API client, authentication, retries, waiters, not-found handling, or Proxmox domain package exists.
- Framework functions use `Definition` plus `Run`; actions use `Invoke` and may emit progress events; the example ephemeral resource only implements `Open`.

## Key Directories

- `internal/provider/`: provider registration, schemas, CRUD/read/Invoke/Open implementations, and tests.
- `examples/`: Terraform configurations used by documentation generation and manual CLI checks. Documentation discovery expects `provider/provider.tf`, `data-sources/<full name>/data-source.tf`, `resources/<full name>/resource.tf`, plus matching action and ephemeral paths.
- `docs/`: generated `tfplugindocs` pages; edit source schemas/examples, then regenerate.
- `tools/`: separate Go module containing `go:generate` dependencies and directives.
- `.github/workflows/`: build, lint, generation-diff, acceptance-test, and release automation.

## Development Commands

Run from the repository root:

```sh
make build       # go build -v ./...
make install     # build and go install
make fmt         # gofmt -s -w -e .
make lint        # golangci-lint run
make test        # unit/in-process tests with coverage
make generate    # headers, Terraform example formatting, and docs
make testacc     # TF_ACC=1 acceptance suite; use intentionally
```

A focused unit run is useful while editing: `go test -v ./internal/provider -run '^TestExampleFunction_'`. After changing schemas or examples, run `make generate`; CI rejects generated diffs. `tools/tools.go` owns the generation commands. Do not hand-edit generated files in `docs/`.

## Code Conventions & Common Patterns

- Use Terraform Plugin Framework APIs, not SDKv2. `.golangci.yml` explicitly blocks SDKv2 packages and enables `staticcheck`, `errcheck`, `unused`, `gofmt`, and related checks.
- Keep compile-time interface assertions: `var _ resource.Resource = &ExampleResource{}`.
- Provide `New<Type>()` constructors returning framework interfaces and register them in the corresponding provider method (`Resources`, `DataSources`, `EphemeralResources`, `Functions`, or `Actions`).
- Define Terraform models with framework `types.*` values and `tfsdk` tags. Keep schema descriptions in source; they feed generated docs.
- After `req.Config.Get`, `req.Plan.Get`, `req.State.Get`, or result/state `Set`, append diagnostics and return early when `HasError()` is true.
- In component `Configure` methods, handle nil provider data before type assertions and report a diagnostic on a wrong type; do not panic.
- Resource lifecycle methods are `Create`, `Read`, `Update`, and `Delete`; implement import with `resource.ImportStatePassthroughID` when IDs support passthrough. Remove state on an upstream not-found response in real resources.
- Use `tflog` for provider-operation logging and keep secrets out of logs. Add retries/waiters only for APIs that are actually eventually consistent.
- Preserve Terraform state semantics: map API responses into model values, distinguish null/unknown/known values, and avoid hardcoded example values in real implementations.

## Important Files

- `main.go`: provider executable and registry address.
- `internal/provider/provider.go`: provider schema, configuration, and capability registration.
- `internal/provider/example_resource.go`: CRUD, import, schema, and provider-data pattern.
- `internal/provider/example_data_source.go`: data-source configuration and read pattern.
- `internal/provider/example_ephemeral_resource.go`: ephemeral `Open` and result pattern.
- `internal/provider/example_action.go` and `example_function.go`: action/function framework patterns.
- `GNUmakefile`: authoritative local commands.
- `tools/tools.go`: generation source of truth.
- `.github/workflows/test.yml`: CI build, lint, generation, and Terraform-version matrix.
- `.golangci.yml`: enabled linters and SDKv2 restrictions.
- `.goreleaser.yml`: cross-platform release builds, checksums, and signing.

## Runtime/Tooling Preferences

Use Go modules and the versions declared in `go.mod` and `tools/go.mod`; do not introduce a second package manager. Use the `GNUmakefile` targets instead of duplicating command recipes. `make generate` requires Terraform on `PATH` and uses `tfplugindocs`; the tools module pins `copywrite` and `terraform-plugin-docs`. Release builds set `CGO_ENABLED=0`, use `-trimpath`, and inject version metadata through GoReleaser.

## Testing & QA

Tests use Go `testing` plus `terraform-plugin-testing`. Acceptance-style tests live beside implementations as `internal/provider/*_test.go`; protocol 6 factories are defined in `provider_test.go`. State assertions use `statecheck`, `knownvalue`, and `tfjsonpath`; the ephemeral test also uses the testing `echoprovider`.

- `make test` runs `go test -v -cover -timeout=120s -parallel=10 ./...`.
- `make testacc` runs `TF_ACC=1 go test -v -cover -timeout 120m ./...`; acceptance tests may create real resources and incur cost in a real provider.
- CI builds and lints before testing, then tests Terraform `1.13.*` and `1.14.*` with `TF_ACC=1`.
- Add or update behavior-focused state checks for schema and lifecycle changes. Preserve import, update, delete, and not-found coverage for real resources.
- When changing generated output, verify `make generate` leaves no diff; this is a CI gate.
