# Mapping guide (gen/mapping)

Each roadmap phase is one Go file in this package declaring every provider
type of that phase. The generator derives complete attribute schemas from
the API schema (`internal/pveapidoc/schema/apidoc.js`), audits coverage, and
emits `internal/provider/registry.json`, the ROADMAP Appendix A table, and
usage examples. **No provider code is hand-written per type.**

Reference example: `phase2.go`.

## Feedback loop

```bash
gofmt -w gen/mapping/ && go test ./gen/...          # derive + structure audit
go run ./gen                                        # full audit + emit (single writer: integrator only)
```

`go test ./gen/...` must stay green at all times; the full-coverage audit
(`assigned to a phase but unused`) is enforced by `go run ./gen` once all
phases are mapped.

## Anatomy

```go
{
    Type: "pve_storage", Kind: "resource", Phase: 5,
    Summary: "Cluster storage pool configuration (any backend).",
    IDAttrs: []string{"storage"},
    Create: &Endpoint{Path: "/storage"},                 // method defaults by role
    Read:   &Endpoint{Path: "/storage/{storeid}"},
    Update: &Endpoint{Path: "/storage/{storeid}"},
    Delete: &Endpoint{Path: "/storage/{storeid}"},
    Attrs:  []AttrRule{{Name: "storage", ForceNew: true}},
},
```

- Endpoint paths must be **verbatim apidoc family paths**, including the
  real placeholder names (e.g. `{pci-id-or-mapping}`, not a guess). The
  audit rejects unknown paths; grep `internal/pveapidoc/schema/apidoc.js`
  or read `ROADMAP.md` Appendix A (normalized form).
- Methods default from the role: create→POST, read→GET, update→PUT,
  delete→DELETE. Override `Method` when the API differs.
- Data sources declare `Reads: []*Endpoint` (plural). Array/scalar
  responses need `ResultName` (the attribute that carries the list/object).
  Object responses without `ResultName` merge into top-level computed
  attributes.
- Actions declare `Invoke`; functions declare `Call` (type name
  `pve_nextid` → function `nextid()`).

## Derivation rules

- Request parameters → Required (per apidoc) or Optional+Default attrs.
- Path placeholders → Required force-new string attrs (`vmid`, `osdid`, …
  are int64 automatically); they also compose the default resource ID in
  read-path order (override with `IDAttrs`).
- Read-response fields → Computed attrs (a parameter that is also echoed
  becomes Optional+Computed).
- Response property `id` is renamed `api_id` (`id` is reserved).
- Names containing `-` map to `_` with `APIName` set automatically.
- Sensitive-by-name heuristic: password/secret/privkey/passphrase/apikey/
  token. Add `Sensitive` via AttrRule when the heuristic misses.
- `WaitTask` defaults true for mutations; the engine only waits when the
  response actually is a UPID, so it is safe to leave default.

## Behaviors

- `Behavior: "singleton"` — GET/PUT endpoints (`pve_node_dns`): create and
  update both PUT; delete only if the API has one.
- `Behavior: "acl"` — PUT-created entries read from a list: `Read` needs
  `Filter: [][2]string{{"path", "path"}, {"roleid", "role_id"}, …}`
  (response field, attribute).
- `Behavior: "state-only-delete"` — no API delete; destroy only removes
  state (`pve_cluster`). Pair with `AllForceNew: true` (audit demands it
  when there is no update endpoint).
- `Clone: &Clone{Path: "/nodes/{node}/qemu/{vmid}/clone", SourceAttr:
  "clone_source_vmid", SourceParam: "vmid", NewIDParam: "newid", …}` with
  the source attribute declared via `Attrs: …{Name: "clone_source_vmid",
  Type: "int64", Add: true, Optional: BoolPtr(true), ForceNew: true}`.
- Scoped firewall types: `Scopes: []string{"cluster","node","vm","container"}`
  and every endpoint uses `Paths: map[string]string{"cluster": "/cluster/firewall/rules", …}`
  instead of `Path`. A `scope` attribute is derived automatically.

## AttrRule essentials

```go
{Name: "disk", Required: BoolPtr(false), Optional: BoolPtr(true)}, // relax
{Name: "vmid", ForceNew: true},                                     // tighten
{Name: "template_vmid", Type: "int64", Add: true, Optional: BoolPtr(true)},
{Name: "secret_value", Sensitive: true},
{Name: "internal_only", Skip: true},                                // drop derived
```

Detail reads (run only when a parameter is set):

```go
{Path: "/nodes/{node}/disks/smart", ResultName: "smart", SkipIfMissing: []string{"disk"}},
```

`SkipIfMissingParam: true` does the same for the endpoint's path placeholders.

## Cross-phase families

When a type uses a family Appendix A assigns to a different phase (e.g.
`pve_backup_info` (2) reads `/cluster/backup-info` (9)), set
`CrossPhase: true` on that endpoint. Out-of-scope families must never be
referenced — the audit rejects them.

## Naming

Resources `pve_<noun>[_<sub>]`, data sources `pve_<noun>`, actions
`pve_<noun>_<verb>`, functions `pve_<verb>_<noun>`. Auxiliary endpoints of a
roadmap type (rollback, resize, apply, …) become actions with that naming.
