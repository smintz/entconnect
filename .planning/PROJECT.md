# entconnect

## What This Is

entconnect makes protobuf contracts the source of truth for [ent](https://entgo.io) applications. It inverts entproto's direction of generation: instead of deriving `.proto` files from ent schemas, entconnect derives ent fields *from* the contract and generates the transport layer that binds RPCs to the application. It is a Go library for teams building contract-first, LLM-assisted backends where design → proto → schema is the only human-writable surface and everything downstream is generated.

It ships as two independently adoptable pieces: `mixinforproto` (a runtime ent mixin — message type in, `[]ent.Field` + validation out, zero codegen) and the `entconnect` entc extension (handler generation, drift checking, multi-transport emission). It is the contract half of a two-project split; the workflow half is **entflow**, a separate project that entconnect touches only through a small metadata interface.

## Core Value

API-visible fields and their validation are defined exactly once — in the protobuf contract — and enforced identically at the transport boundary and at the storage layer, so contract/schema drift is a build failure rather than a runtime surprise.

## Requirements

### Validated

(None yet — ship to validate)

### Active

- [ ] `MixinForProto[M proto.Message](opts ...Option) ent.Mixin` derives ent fields from a generated message type via protoreflect — no committed `FileDescriptorSet`, no string message names, compile-time-checked reference
- [ ] Field mapping covers proto scalars, enums, well-known types, presence semantics, `map<K,V>`, and `oneof` (panic-on-ambiguity, never silent guessing)
- [ ] `Exclude(names...)` and `Override(name, field)` options, both panicking at schema load on unknown field names
- [ ] Tier 1 validation translation: protovalidate constraints with native ent equivalents become real builder calls (`MaxLen`, `Match`, `Min`/`Max`/`Range`, `Positive`, `NotEmpty`) so the rest of the ecosystem can introspect them
- [ ] Tier 2 validation passthrough: residual field-scoped protovalidate CEL compiled once at schema load, executed by a single mixin-declared hook over changed mutation fields
- [ ] Tier 3 message-level rules: boundary-only by default, opt-in `WithMessageRules(OnCreate)` for complete-mutation enforcement
- [ ] Schema-level violations carry the protovalidate constraint ID and message, so they map to the identical wire error the boundary interceptor would produce
- [ ] `AsJSON("field")` opt-in for genuinely embedded (non-entity) message values
- [ ] `mixinforproto` ships at its own module path with an independent `go.mod` and depends only on `ent`, `google.golang.org/protobuf`, and `protovalidate-go`/`cel-go`
- [ ] entc extension generates ConnectRPC handlers for standard CRUD RPCs (get/list/create/update/delete) directly against the ent client
- [ ] List RPCs implement AIP-158-style page-token paging and filtering; Update implements documented partial-update semantics
- [ ] Generated interceptor chain runs in fixed order: authn → viewer injection → protovalidate → otel → handler
- [ ] Generated server wiring is the only place an ent client is constructed, and never exposes a privileged client to application code
- [ ] Flow-bound handlers: RPCs whose request type matches an entflow flow input decode, protovalidate, extract the viewer, call `flow.Start`, and shape the response
- [ ] Long-running flows answer with a run reference plus a generated `GetRunStatus` RPC per service
- [ ] Drift check errors when an RPC is unclaimed by a flow, a CRUD binding, or an explicit `entconnect.Manual("rpc")`
- [ ] Drift check warns when a proto-input flow has no claiming RPC (silenceable by marking the flow internal)
- [ ] Drift check errors when a skipped message-typed field has no matching declared edge or explicit exclude; warns on `google.api.resource_reference` id fields with no corresponding edge
- [ ] Drift check fingerprints mirrored protovalidate constraints and errors when a schema hand-declares a conflicting validator
- [ ] Golden-file tests for all generated code, plus a conformance corpus covering every mapping rule and constraint class
- [ ] Differential validation harness: for every corpus message, random values assert `protovalidate verdict == ent mutation verdict` for field-scoped rules
- [ ] Reference application (Order/Inventory) built end-to-end through the canonical pipeline in CI, with connect-go client tests asserting privacy denials surface as `PermissionDenied`

### Out of Scope

- **Generating proto from ent schemas** — that is entproto's direction and the exact inversion this project exists to reject
- **Edge generation from message-typed fields** — protos express shape, edges carry relational semantics (direction, ownership, cascade, uniqueness) the contract does not state; edges stay human-declared and machine-verified
- **Inventing a REST mapping convention** — `google.api.http` is the mapping language; entconnect only reads it
- **Update-time message-level validation via fetch-then-merge** — hidden query cost and unclear semantics under concurrent writes; deferred to an explicit `OnUpdateWithFetch` option only if demanded
- **A silent hand-written-handler escape hatch** — `entconnect.Manual("rpc")` exists and is deliberately made to feel like the smell it is
- **entflow itself** — separate project, separate repo, separate design doc; entconnect consumes only its metadata surface
- **Drift checking inside `mixinforproto`** — a runtime mixin cannot see the service surface or other schemas' edges; cross-artifact validation is codegen's job
- **Custom proto field options as an annotation channel** (e.g. `(entconnect.field).immutable`) — migrates schema semantics into the proto; `Override` is the answer until real demand appears

## Context

**The pipeline this serves.** Contract-first and LLM-assisted: design (Figma / Claude Design) → protos written from the design (ConnectRPC services, protovalidate constraints) → ent schemas materializing contract fields plus storage-only fields, transitions, hooks, privacy, flows → everything else generated (handlers, validation, Atlas migrations, connect-es TS client). Exactly three human/LLM-writable artifact classes, each declarative, each downstream layer generated and cross-checked against the one above.

**Proto-first is forced, not chosen.** Ent schema files reference contract types directly (flow inputs like `*orderv1.CancelOrderRequest`, handler response shapes), so the schema imports the contract and cannot compile before `buf generate` runs. You cannot generate a contract from a schema that will not compile without the contract. entconnect leans into this topology instead of fighting it.

**Canonical build pipeline (strictly ordered):**

```
1. buf lint && buf generate   # → orderv1 Go pkg, FileDescriptorSet (.binpb, committed),
                              #   connect stubs, connect-es TS client
2. go generate ./...          # entc + entconnect + entflow extensions
3. atlas migrate diff         # schema → versioned migration
```

The descriptor set is committed so step 2 is hermetic and schema-loading never shells out to buf.

**Repository layout:**

```
entconnect/
├── mixinforproto/   # independent go.mod: message type → ent fields (+ validation relay)
├── entc/            # extension: handler gen, drift check, transports
├── codec/proto/     # entflow.Codec adapter for proto.Message inputs (~10 lines)
└── runtime/         # interceptors: viewer, protovalidate, otel
```

**Why validation is relayed into the schema at all:** mutations do not only originate from RPC. Flows, workers, seeds, tests, and CLIs all mutate through the ent client. The interceptor at the boundary is UX (fast, well-shaped errors before any DB work); the schema is the guarantee. Both are generated from the same protovalidate source, so they cannot disagree.

**Status quo being replaced:** entproto (wrong generation direction, classic gRPC not Connect), dual schema definition (resource message vs `Fields()` drifting silently), dual validation (protovalidate vs ent validators disagreeing on error surfaces), and entrest/entgql/entproto each re-deriving an API from the schema — three sources of truth for "the API," none of them the designed contract.

**Current repo state:** greenfield. Two design documents (`entconnect.md`, `mixinforproto.md`, both Draft v1, dated 2026-08-05) and no code.

## Constraints

- **Dependency direction**: `entflow → ent`; `entconnect → ent + protobuf/descriptor machinery (+ entflow metadata interface)`; `app → both`. The application is the only meeting point — Either project must be releasable, versionable, and adoptable without the other.
- **Module isolation**: `mixinforproto` depends only on `ent`, `google.golang.org/protobuf`, and `protovalidate-go`/`cel-go` — this is what makes it shippable as a standalone micro-module to people who want proto-first ent and none of the rest of the stack.
- **entflow coupling**: confined to a small metadata interface (flow name, input/output types, sync/async nature), ideally duck-typed or a tiny shared `entflow/meta` package, so entconnect builds without entflow's runtime.
- **Zero-flow and zero-contract usability**: entconnect must be fully useful with zero flows (pure CRUD apps); entflow must be fully useful with zero contract.
- **Codegen-free mixin**: `MixinForProto` is plain Go executing at schema-load time — no entc extension, no code generation, no committed descriptor files. Schema load *is* the check phase, so failures panic there rather than at first mutation.
- **Generated handlers are not editable files**: flow-bound handlers are 100% generated and "dumb by construction," not by discipline.
- **Ecosystem precedent**: golden-file tests for generated code follow the entgql/entproto precedent.

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Proto-first, not ent-first (invert entproto) | Schema imports contract types for flow inputs, so it cannot compile before `buf generate`; the dependency arrow is physical, not stylistic | — Pending |
| Runtime mixin via protoreflect type parameter, not codegen | Compile-time-checked reference; no committed `FileDescriptorSet`, no string message names, no load-order coupling to buf | — Pending |
| Explicit `Override()` builder, not shadowing | Implicit shadow-merge semantics were unspecifiable; an override suppresses validation relay for that field so ownership is total | — Pending |
| Three-tier validation (translate / execute CEL / boundary-only) | Tier 1 keeps constraints introspectable to the ecosystem; Tier 2 gives 100% fidelity with no mapping-table maintenance for the long tail | — Pending |
| Tier 2 as a single mixin-declared hook, not per-field validators | ent exposes arbitrary `Validate(fn)` on strings only; one hook covers all field types with a uniform error shape | — Pending |
| Message-typed fields skipped; edges human-declared, machine-verified | Inferring edges means inventing conventions, and a generic mixin cannot reference app schema types without worse locality than `Edges()` | — Pending |
| proto3 non-optional scalars → `Default(zero)` non-optional ent fields | "Unset" and "zero" collapse exactly as they do on the wire; contracts needing the distinction must say `optional` | ⚠️ Revisit — sharpest edge of the design; needs a prominent doc section and possibly a `StrictPresence` option |
| Async flows answer with `run_id` + generated `GetRunStatus` RPC | Simplest correct MVP; Connect server streaming of run-state transitions designed but deferred | — Pending |
| Keep the name `entconnect` despite gateway/REST scope | Connect is the flagship transport and the name is legible and available; objection recorded (alternatives: `entcontract`, `entproto2`) | ⚠️ Revisit |
| Descriptor set committed to the repo | Makes `go generate` hermetic; schema loading never shells out to buf | — Pending |

## Evolution

This document evolves at phase transitions and milestone boundaries.

**After each phase transition** (via `/gsd-transition`):
1. Requirements invalidated? → Move to Out of Scope with reason
2. Requirements validated? → Move to Validated with phase reference
3. New requirements emerged? → Add to Active
4. Decisions to log? → Add to Key Decisions
5. "What This Is" still accurate? → Update if drifted

**After each milestone** (via `/gsd-complete-milestone`):
1. Full review of all sections
2. Core Value check — still the right priority?
3. Audit Out of Scope — reasons still valid?
4. Update Context with current state

---
*Last updated: 2026-08-08 after initialization*
