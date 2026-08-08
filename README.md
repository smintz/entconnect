# entconnect

entconnect makes protobuf contracts the source of truth for [ent](https://entgo.io)
applications. It inverts entproto's direction of generation: instead of deriving
`.proto` files from ent schemas, entconnect derives ent fields *from* the contract and
generates the transport layer that binds RPCs to the application.

It ships as two independently adoptable pieces: `mixinforproto` (a runtime ent mixin —
message type in, `[]ent.Field` + validation out, zero codegen) and the `entconnect` entc
extension (handler generation, drift checking, multi-transport emission). It is the
contract half of a two-project split; the workflow half is **entflow**, a separate
project that entconnect touches only through a small metadata interface.

See [`entconnect.md`](./entconnect.md) and [`mixinforproto.md`](./mixinforproto.md) for
the full design.

## Repository layout

This is a **two-module Go repository**:

```
entconnect/
├── go.mod              # root module: github.com/smintz/entconnect
├── go.work              # workspace listing both modules — dev-only, never required
│                         #   for either module to build for an external consumer
├── mixinforproto/        # independent go.mod: message type -> ent fields (+ validation)
│   └── README.md          # start here for mixinforproto specifically
└── proto/                 # buf module: the synthetic conformance corpus (Phase 1)
```

`mixinforproto` is independently taggable and independently adoptable — see
[`CONTRIBUTING.md`](./CONTRIBUTING.md) for the dual-tagging release convention this
implies, and read it *before* your first `git tag` in this repo.

## Building and testing

Never run a bare `go build ./...` / `go test ./...` from the repo root and assume it
covers `mixinforproto` — Go does not descend into a nested `go.mod`. Use `make`, which
enumerates both modules explicitly:

```sh
make build vet test     # both modules, from the checked-in MODULES list
make test-standalone     # mixinforproto alone, with GOWORK=off
make check-modules        # fails if either go.mod declares a local-path substitution
make pipeline               # scripts/pipeline.sh — the canonical five-step build pipeline
```

`scripts/pipeline.sh` runs, in this fixed order: `buf lint`, `buf generate`, a separate
`buf build --as-file-descriptor-set` invocation, `go generate ./...` per module, and
`atlas migrate diff`. In Phase 1 the last two steps have nothing real to act on yet and
print a visible skip line rather than silently doing nothing — see the script's own
header comment for the full rationale, including why it is serial-only.

## Where to go next

- **Using `mixinforproto` on its own?** Start with
  [`mixinforproto/README.md`](./mixinforproto/README.md) — in particular its
  "Proto3 presence and the zero-collapse" section, which every adopter should read
  before writing an Update handler.
- **Contributing to this repo?** Start with [`CONTRIBUTING.md`](./CONTRIBUTING.md) for
  the module-substitution review rule and the release-tag convention.
