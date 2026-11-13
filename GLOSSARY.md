# Glossary

Module vocabulary, named from the code's own usage.

- **Registry** — the generated registry document (`registry.json`, embedded in the provider, decoded once by `GeneratedRegistry`) listing every `TypeDescriptor`.
- **Mapping** — the generator input (`gen/mapping`): one Go file per phase declaring each provider type's endpoints and attribute overrides; the generator derives full schemas from it and audits coverage.
- **Attr** — one attribute declaration of a generated type (`descriptor.go`): name, `AttrType`, wire `AttrFormat`, and optional `BlockSpec`.
- **Op** — one provider operation wired to a PVE endpoint (`descriptor.go`): method, path template, parameter set, task-wait policy.
- **TypeDescriptor** — one generated provider type (`descriptor.go`): attrs, ops, ID composition; pure data that parameterizes an engine.
- **Engine** — a generic runtime (`genericResource`, `genericDataSource`, `genericAction`, `genericFunction`) instantiated per `TypeDescriptor`; there are no per-type hand-written implementations.
- **Codec** — the wire⇄Terraform translation layers: `attr_codec.go` for plain attributes (decode, encode, wire equality) and `block_codec.go` for slot-family blocks (encode/decode/diff/merge).
- **Property string** — PVE's `key=value,...` scalar encoding (keys sorted); parsed/rendered by the property-string trio in `attr_codec.go`.
- **Slot family** — an indexed PVE parameter family (`scsi0`, `net1`, `mp3`, optionally with a secondary family `net1` ⇄ `ipconfig1` sharing the element index).
- **Block** — a Terraform nested block (`BlockSpec`) whose elements map onto slot-family slots, each slot's value a property string.
- **Reconcile mode** — the merge policy `applyReadResponse`/`mergeObject`/`mergeBlock` fold a read response under: outside import mode (after-apply and managed refresh) null plain-Optional values stay null; import (and data-source reads) adopt server values wholesale.
- **Transport** — the one `pveclient` request path (`do`/`doRaw` → `doRequest` → `handleResponse`, with single-flight `ensureTicket` for password auth) that every HTTP call goes through.
