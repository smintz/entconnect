# Walking Skeleton — entconnect / mixinforproto

**Phase:** 1
**Generated:** 2026-08-08

> **Template adaptation note.** The stock skeleton template is written for web applications
> (routing, UI interaction, dev deployment). This project ships a **Go library**, so the
> template's *intent* is translated rather than its literal checklist: sections with no
> analog (routing, UI interaction) are dropped rather than faked. See
> "Stack Touched in Phase 1" for the honest end-to-end slice.

## Capability Proven End-to-End

> One real `.proto` file → `buf generate` → a generated Go message type →
> `MixinForProto[*T]()` declared in a real `ent/schema` package → a real `entc generate`
> run → a derived ent field **and** its provenance annotation readable from the resulting
> `gen.Graph`.

That slice crosses every boundary this phase's risk actually lives at. The load-bearing one
is the **entc schema-load JSON boundary**: `entc generate` compiles and runs the schema
package in a subprocess and serializes the result to JSON, where Go closures do not survive
and `load.Schema.Field.Validators` is an `int` count, not a function list. Annotations are
the *only* channel across it. No unit test can prove that boundary holds — only a real
`entc generate` run can, which is why the tracer ends at `gen.Graph`, not at `[]ent.Field`.

## Architectural Decisions

| Decision | Choice | Rationale |
|---|---|---|
| Root module path | `github.com/smintz/entconnect` | Matches the `origin` remote; the import path must equal the module path or the proxy cannot resolve it. |
| Nested module path | `github.com/smintz/entconnect/mixinforproto` | Independent `go.mod` nested in the same repo (design doc §7). Release-time extraction to a standalone repo stays possible; the import path does not change if it happens under the same org. |
| Module isolation | `mixinforproto` requires **only** `entgo.io/ent`, `google.golang.org/protobuf`, `buf.build/go/protovalidate` (+ `github.com/sebdah/goldie/v2`, test-only) | The constraint that makes it adoptable standalone. **No `cel-go` dependency in Phase 1** — see Reconciliation R1. |
| Go directive | `mixinforproto`: `go 1.24` · root: `go 1.26` | `go 1.24` is the highest floor of the three direct deps (verified against the proxy). The root anticipates `connectrpc.com/connect@v1.20.0` (Go ≥1.25) in Phase 2. Nested modules need not share a directive. |
| Cross-boundary contract | Two annotation structs in `mixinforproto/annotation.go`: `SourceMessage` (schema-level), `SourceField` (field-level), both plain serializable structs carrying `ContractVersion int` | Annotations are the only channel over the schema-load/codegen boundary. Struct types live in `mixinforproto` because that is the one part of its API another module decodes (D-01/D-02). |
| Error model | Pure `derive[M](opts...) (*derivation, error)` core; the `ent.Mixin` adapter is the only place that panics | Makes the in-process debug entry point (`Validate[M]`) nearly free instead of a parallel code path (D-06/D-07). |
| Determinism | Iterate `protoreflect` descriptor lists by index in declaration order; never range a Go `map` into ordered output | Map iteration order is randomized; golden tests would be flaky exactly where this design is most novel (D-24). |
| Toolchain bootstrap | `buf` installed via `go install github.com/bufbuild/buf/cmd/buf@v1.72.0` | Verified working. No system package manager needed; keeps CI images simple. |
| Directory layout | `mixinforproto/{mixin,derive,fieldmap,validate,annotation,option,errors,reserved}.go` + `testdata/` | `annotation.go` is called out separately because it is the one file another module imports. |

## Stack Touched in Phase 1

- [ ] Two-module scaffold (`go.work`, root `go.mod`, `mixinforproto/go.mod`) building green
- [ ] Contract layer — at least one real `.proto` compiled by `buf generate` into a committed Go stub
- [ ] Derivation layer — at least one real proto field → one real `ent.Field`
- [ ] Provenance layer — `SourceMessage` + `SourceField` annotations written by the mixin
- [ ] **Schema-load boundary** — a real `entc generate` run whose `gen.Graph` yields back both annotations
- [ ] Standalone-consumption proof — `GOWORK=off` build/test of `mixinforproto` alone

*(Dropped as having no analog in a Go library: "Routing", "UI interaction", "Deployment to a dev
environment". The `GOWORK=off` job is the honest substitute for "proves it runs outside the
developer's own machine".)*

## Out of Scope (Deferred to Later Slices)

- Tier 2 CEL passthrough and the mutation-time `Hooks()` entry — Phase 3 (VAL-04…VAL-11)
- Any entc **extension**, handler generation, interceptor chain — Phase 2
- Any drift checking — Phase 5, and a permanent non-goal of `mixinforproto` itself
- Flow binding — Phase 4
- The Order/Inventory reference application — Phase 5
- `WithMessageRules(OnCreate)` — the option symbol is **not** declared in Phase 1 (omitting is cleaner than a lying no-op)
- `StrictPresence` (MIX2-01), `debug_redact` → `field.Sensitive()` (MIX2-02), `Duration` mapping (MIX2-04) — all v2
- The root `entconnect` module importing `mixinforproto` — deliberately absent until Phase 2 (D-18)

## Subsequent Slice Plan

Each later phase adds one vertical slice on top of this skeleton without altering its
architectural decisions:

- **Phase 2:** an RPC over a `MixinForProto`-backed entity answers a real Connect request (CRUD handlers + interceptor chain). First consumer of the annotation contract.
- **Phase 3:** a residual CEL constraint recorded in Phase 1 as `SourceField.ResidualIDs` is actually *executed* at mutation time, with a verdict identical to the boundary's.
- **Phase 4:** an RPC is answered by an entflow flow matched purely by request type.
- **Phase 5:** the build fails when contract, schema, and service surface disagree — consuming `SourceMessage.Fields`/`Excluded`/`Overridden` written in Phase 1.

## Contract Notes for Later Phases

1. **`ContractVersion` starts at `1`.** Additive-only between bumps: new fields may be added,
   existing fields are never renamed or repurposed, decoding tolerates unknown keys. A consumer
   reading a version higher than it was compiled against must fail with a named mismatch error,
   never silently decode a partial struct (D-03/ANNO-04).
2. **`SourceMessage.Fields` is the complete descriptor inventory**, not the derived subset.
   Without it, Phase 5 cannot distinguish "deliberately excluded" from "silently forgotten" —
   both look identical as absence from `gen.Graph`.
3. **Residual constraints are recorded, not enforced, in Phase 1.** `SourceField.ResidualIDs`
   plus a stable fingerprint of the CEL expression *strings* is the handoff to Phase 3. The
   Phase 1 storage layer is explicitly not a complete guarantee, and the docs say so.
4. **A mixin's `Hooks()` run before any schema-declared hook or privacy policy.** Phase 1
   ships no hook, but the docs state the ordering now so Phase 3 is not the first time an
   adopter meets it (D-27).
