# Repository Guidelines

## Project Overview

This repository is a Terraform Plugin Framework provider targeting Proxmox Virtual Environment (PVE). The current implementation is still HashiCorp scaffold code: it serves the placeholder provider address `registry.terraform.io/hashicorp/scaffolding`, uses hardcoded example values, and has no Proxmox API client, authentication chain, or PVE resources yet.

Requirements are Go `1.25.8` from `go.mod` and Terraform `>= 1.0`. The provider uses Terraform protocol 6 and the Plugin Framework, not a completed SDKv2 provider.

## Architecture & Data Flow

- `main.go` parses `-debug` and serves `provider.New(version)` through `providerserver.Serve`.
- `internal/provider/provider.go` defines provider metadata/schema, `Configure`, and registration for resources, data sources, ephemeral resources, functions, and actions.
- `Configure` decodes provider config, appends diagnostics, and currently passes `http.DefaultClient` through `ResourceData` and `DataSourceData`; it is a placeholder for a real PVE client and credential chain.
- Components receive provider data in `Configure`, reject nil or unexpected types with diagnostics, and retain the client on their implementation struct.
- Operations decode config, plan, or state into framework models, perform provider work, append diagnostics, and write Terraform state or results. Current examples return hardcoded IDs/tokens and do not call an upstream API.

For real PVE work, keep provider configuration and API-client construction in `Configure`; resources and data sources should consume that configured client rather than create clients independently.

## Key Directories

- `internal/provider/`: provider registration, schemas, CRUD/read implementations, action/function/ephemeral implementations, and tests.
- `examples/`: Terraform configurations used by `tfplugindocs`; discovery expects `provider/provider.tf`, `data-sources/<full-name>/data-source.tf`, `resources/<full-name>/resource.tf`, and matching action/ephemeral paths.
- `docs/`: generated Registry documentation. Edit schemas/templates/examples, then regenerate; do not hand-edit generated pages.
- `tools/`: separate Go module containing generation dependencies and `go:generate` directives.
- `.github/workflows/`: build/lint, generation-diff, acceptance-test, and release automation.

## Development Commands

Run from the repository root:

```sh
make build       # go build -v ./...
make install     # build, then go install -v ./...
make fmt         # gofmt -s -w -e .
make lint        # build bin/custom-gcl, then run the configured linters
make test        # go test -v -cover -timeout=120s -parallel=10 ./...
make testacc     # TF_ACC=1 go test -v -cover -timeout 120m ./...
make generate    # headers, Terraform example formatting, and tfplugindocs
```

The provider server can be started directly with `go run . -debug`; Terraform normally launches the compiled provider using its registry address. There is no separate application server or `make run` target.

After changing schemas or documentation examples, run `make generate`. CI rejects generated-file differences. The authoritative generation directives are in `tools/tools.go`.

## Code Conventions & Common Patterns

### Framework and registration

- Use Terraform Plugin Framework APIs for new code; do not add SDKv2 imports. `.golangci.yml` blocks SDKv2 packages through `depguard`.
- Keep compile-time interface assertions such as `var _ resource.Resource = &ExampleResource{}`.
- Provide `New<Type>()` constructors returning framework interfaces and register them from the provider's `Resources`, `DataSources`, `EphemeralResources`, `Functions`, or `Actions` methods.
- Define Terraform models with `types.*` values and `tfsdk` tags. Give every schema attribute a `MarkdownDescription`; descriptions feed generated Registry docs.

### Diagnostics and state

- After `req.Config.Get`, `req.Plan.Get`, `req.State.Get`, or result/state `Set`, append diagnostics and return immediately when `HasError()` is true.
- In component `Configure` methods, handle nil provider data before type assertions and report a diagnostic for the wrong concrete type; do not panic.
- Preserve Terraform null/unknown/known semantics. Use plan modifiers and validators for schema behavior, and mark secrets `Sensitive`.
- Real resource `Read` methods must remove state when the upstream object is not found; `Delete` should treat an already-absent object as success. Import uses `resource.ImportStatePassthroughID` only when the API identifier supports passthrough.

### API and operation behavior

- Wrap underlying errors with `%w`, match typed API errors rather than error strings, and make diagnostics name the operation, object type, identifier, and cause.
- Use `tflog` for provider-operation logging; never log credentials or other secret values.
- Add retries/waiters only for APIs that are actually eventually consistent. Keep finder, not-found, status, and waiter behavior reusable across CRUD methods.
- Actions use `Invoke`, report meaningful progress for long operations, and are exercised through Terraform `action_trigger` blocks. Ephemeral resources implement `Open`; add `Renew`/`Close` only when the upstream value has a real lease lifecycle and never persist secret results.

## Important Files

- `main.go`: provider executable, debug flag, registry address, and version wiring.
- `internal/provider/provider.go`: provider schema/configuration and capability registration.
- `internal/provider/example_resource.go`, `example_data_source.go`, `example_action.go`, `example_function.go`, and `example_ephemeral_resource.go`: current Framework patterns to replace with PVE behavior.
- `GNUmakefile`, `.golangci.yml`, and `.custom-gcl.yml`: authoritative local build, test, generation, formatter, and custom anti-slop lint wiring.
- `tools/tools.go`, `examples/`, and `docs/`: documentation-generation source, Terraform examples, and generated output.

## Runtime/Tooling Preferences

- Use Go modules only: the provider uses root `go.mod`; generation tools use the separate `tools/go.mod`. Do not introduce another package manager.
- `make generate` requires Terraform on `PATH`; it runs `terraform fmt`, Copywrite, and `tfplugindocs`. Generated docs use provider name `scaffolding` until the provider address is renamed.
- `make lint` builds `bin/custom-gcl` with golangci-lint `v2.10.1` and anti-slop-go `v1.4.0`; the `bin/` directory is ignored. CI uses the same pinned custom-linter build.
- Release builds use GoReleaser from `.goreleaser.yml`, set `CGO_ENABLED=0`, use `-trimpath`, and inject version/commit metadata. Releases are triggered by `v*` tags.
- Load the relevant local guidance before implementation: `.agents/skills/new-terraform-provider/SKILL.md`, `provider-configuration/SKILL.md`, `provider-resources/SKILL.md`, `provider-actions/SKILL.md`, `provider-ephemeral-resources/SKILL.md`, `provider-test-patterns/SKILL.md`, `provider-docs/SKILL.md`, `run-acceptance-tests/SKILL.md`, `terraform-style-guide/SKILL.md`, and `terraform-test/SKILL.md`.

## Testing & QA

- Tests live beside implementations in `internal/provider/*_test.go` and use Go `testing` plus `terraform-plugin-testing`.
- Protocol 6 factories are defined in `provider_test.go`. Prefer `ConfigStateChecks` with `statecheck`, `knownvalue`, and `tfjsonpath`; use `resource.ParallelTest` unless tests share state.
- Current `TestAcc*` tests exercise scaffold resources, data sources, actions, and ephemeral/function examples. They do not contact Proxmox and do not validate real PVE behavior. Version gates currently cover Terraform `>= 1.10` for ephemeral resources, `>= 1.14` for actions, and `>= 1.8` for functions.
- Use focused unit tests for pure provider logic. For real acceptance coverage, set `TF_ACC=1`, pass an explicit timeout, verify the target PVE account first, and add cleanup/sweeper coverage before running against live infrastructure.
- CI tests Terraform `1.13.*` and `1.14.*`; the generation job runs `make generate` and fails if generated output changes. The repository has no native `.tftest.hcl` suite currently.
