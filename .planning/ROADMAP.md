# Roadmap: entconnect

## Overview

entconnect inverts entproto's generation direction: a protobuf contract drives ent field derivation (`mixinforproto`) and ConnectRPC transport generation (`entc` extension), so contract/schema/service drift becomes a build failure instead of a runtime surprise. The journey starts with the piece everything else depends on — a standalone, codegen-free mixin that turns a message type into ent fields with enough provenance for later drift checking — then builds the CRUD transport layer on top of it (handlers, partial updates, the interceptor chain), deepens validation fidelity for the long tail protovalidate constraints Tier 1 can't translate, binds RPCs to entflow once that project has real running code, and closes with the full bidirectional drift check plus a reference application proving the whole canonical pipeline holds together in CI.

## Phases

**Phase Numbering:**

- Integer phases (1, 2, 3): Planned milestone work
- Decimal phases (2.1, 2.2): Urgent insertions (marked with INSERTED)

Decimal phases appear between their surrounding integers in numeric order.

- [ ] **Phase 1: MixinForProto Core** - Contract-derived ent fields, provenance annotations, Tier 1 validation, and the two-module CI scaffold
- [ ] **Phase 2: CRUD Handlers & Interceptor Chain** - Generated Get/List/Create/Update/Delete Connect handlers with FieldMask-gated updates and a fixed, privacy-aware interceptor chain
- [ ] **Phase 3: Validation Fidelity** - Tier 2/3 CEL passthrough, structured schema-layer errors, and a differential validation harness
- [ ] **Phase 4: Flow Binding** - RPC-to-flow matching, generated flow-bound handlers, and long-running run status
- [ ] **Phase 5: Full Drift Check & Reference App** - Bidirectional drift checking and an end-to-end reference application proving the canonical pipeline

## Phase Details

### Phase 1: MixinForProto Core

**Goal**: Developers can derive a complete, validated ent schema directly from a protobuf message type, with zero codegen and full provenance for later drift checking
**Mode:** mvp
**Depends on**: Nothing (first phase)
**Requirements**: MIX-01, MIX-02, MIX-03, MIX-04, MIX-05, MIX-06, MIX-07, MIX-08, MIX-09, MIX-10, MIX-11, MIX-12, MIX-13, MIX-14, ANNO-01, ANNO-02, ANNO-03, ANNO-04, VAL-01, VAL-02, VAL-03, PIPE-01, PIPE-02, PIPE-03, PIPE-04, PIPE-08
**Success Criteria** (what must be TRUE):

  1. Developer declares `MixinForProto[*orderv1.Order]()` in a schema's `Mixin()` and sees ent fields (scalars, enums, well-known types, presence-correct optionals, scalar maps) materialized at schema load — no committed descriptor file, no string message names, deterministic field order across runs
  2. Developer excludes/overrides fields via `Exclude()`/`Override()`/`AsJSON()`; naming an unknown field or leaving an unresolved `oneof` fails schema load with a message naming the offending message/field/option and the fix — and the developer can reproduce that same failure in-process for debugging, without going through `go generate`
  3. Developer inspects `gen.Graph` and finds per-schema and per-field provenance annotations (source message, source field, derivation kind, exclude/override record, version marker) that survive entc's JSON schema-load boundary
  4. Protovalidate string/numeric/presence constraints on the message (min_len/max_len/pattern, gt/gte/lt/lte, required) show up as native ent builder calls (`MaxLen`, `Match`, `Min`/`Max`/`Range`/`Positive`, `NotEmpty`), introspectable by other ecosystem tools
  5. `mixinforproto` ships as an independently buildable, independently tagged module (`mixinforproto/vX.Y.Z`, no `replace` directives) with its own `go.mod` (ent + protobuf + protovalidate only — see note); a documented pipeline script and CI enumerate both modules explicitly, run a `GOWORK=off` job proving standalone consumption, and its docs prominently cover proto3 presence/zero-collapse semantics before adopters hit them

> **Planning note (2026-08-08):** Phase 1 research verified that `mixinforproto` needs **no cel-go
> dependency at all** — Tier 1 uses only protovalidate's constraint enumeration, never CEL
> compilation. cel-go becomes a direct dependency in Phase 3. Separately, the announced
> `github.com/cel-expr/cel-go` module path was verified **non-functional** (its own `go.mod` still
> declares the old path); Phase 3 must re-verify before choosing an import path.

**Plans**: 2/5 plans executed
Plans:
**Wave 1**

- [x] 01-01-PLAN.md — Walking-skeleton tracer: one proto field becomes one annotated ent field, proven through a real entc schema load

**Wave 2** *(blocked on Wave 1 completion)*

- [x] 01-02-PLAN.md — Field-mapping expansion: scalars, enums, WKTs, presence, maps and AsJSON, golden-asserted against a synthetic corpus
- [ ] 01-03-PLAN.md — Canonical pipeline script, two-module CI, release-tag convention, and adopter documentation

**Wave 3** *(blocked on Wave 2 completion)*

- [ ] 01-04-PLAN.md — Exclude/Override options, reserved-identifier and oneof gates, collected failures, and in-process reproduction
- [ ] 01-05-PLAN.md — Tier 1 validation relay: string, presence and numeric translation with residual provenance recording

### Phase 2: CRUD Handlers & Interceptor Chain

**Goal**: Developers get generated ConnectRPC CRUD handlers wired directly to the ent client, with safe partial updates and a fixed, privacy-aware interceptor chain — no hand-written handler code, no ent client leakage
**Mode:** mvp
**Depends on**: Phase 1
**Requirements**: CRUD-01, CRUD-02, CRUD-03, CRUD-04, CRUD-05, CRUD-06, CRUD-07, INT-01, INT-02, INT-03, INT-04, INT-05
**Success Criteria** (what must be TRUE):

  1. Developer runs `go generate` and gets working Get/Create/Delete Connect handlers over a MixinForProto-backed entity, calling the ent client directly
  2. Developer gets a List handler that pages via AIP-158 `page_token`/`next_page_token` on ent's native keyset `Paginate()` — not offset paging
  3. Developer gets an Update handler that requires a `FieldMask` and only calls `Set*` for paths present in the mask (untouched fields never zeroed); an unknown mask path fails the build against the message descriptor
  4. Every request flows through a fixed interceptor chain (authn → viewer injection → protovalidate → otel → handler) built once per process; privacy policy denials surface to the client as Connect `PermissionDenied`, and generated server wiring is the only place an `*ent.Client` is constructed
  5. Developer can hand-write one handler via `entconnect.Manual("rpc")`, and it still runs inside the generated interceptor chain and shows up in drift-check output; all generated code is byte-stable across runs and covered by golden-file tests

**Plans**: TBD

### Phase 3: Validation Fidelity

**Goal**: Residual and message-level validation rules that Tier 1 can't translate get executed with byte-identical results at both the RPC boundary and the storage layer, proven by an automated differential harness
**Mode:** mvp
**Depends on**: Phase 2
**Requirements**: VAL-04, VAL-05, VAL-06, VAL-07, VAL-08, VAL-09, VAL-10, VAL-11, PIPE-05, PIPE-06
**Success Criteria** (what must be TRUE):

  1. Residual field-scoped protovalidate CEL rules (the long tail Tier 1 can't translate) are compiled once at schema load and evaluated by a single mixin-declared hook that only checks fields the mutation actually changed
  2. A schema-layer violation carries the same protovalidate constraint ID and message as the boundary interceptor would produce, exposed as a structured error consumable outside any RPC context — so a caller cannot tell which layer caught it
  3. Message-level (cross-field) rules stay boundary-only unless a developer opts in with `WithMessageRules(OnCreate)`; the mixin hook's ordering relative to schema-declared hooks/policies is documented and covered by a test that fails if ent changes that order; the boundary validator is built once per process, not per request
  4. CI fails when `mixinforproto`'s and `entconnect`'s resolved protovalidate/cel-go versions diverge
  5. A conformance corpus golden-asserts every field-mapping rule and protovalidate constraint class against derived fields, and a differential harness feeds random values through every corpus message asserting `protovalidate verdict == ent mutation verdict` for field-scoped rules

**Plans**: TBD

### Phase 4: Flow Binding

**Goal**: Developers can bind an RPC to an entflow flow by request type alone, with synchronous and long-running flows both answered correctly, and entconnect remains fully buildable and testable with zero real flows
**Mode:** mvp
**Depends on**: Phase 2 (interceptor chain, viewer context, CRUD client wiring)
**Requirements**: FLOW-01, FLOW-02, FLOW-03, FLOW-04, FLOW-05, FLOW-06
**Success Criteria** (what must be TRUE):

  1. Developer's RPC is matched to a flow purely by request type via entflow's metadata surface, without entconnect importing entflow's runtime
  2. The generated flow-bound handler (never hand-editable) decodes the request, protovalidates it, extracts the viewer, calls `flow.Start`, and shapes the response
  3. A synchronous flow answers its RPC directly; a long-running flow answers with a run reference, retrievable via a generated per-service `GetRunStatus` RPC
  4. `codec/proto` adapts `proto.Message` request/response types to entflow's codec interface
  5. entconnect's own test suite exercises flow binding end-to-end against a fake metadata implementation, proving entconnect builds and is useful with zero real flows

**Plans**: TBD

### Phase 5: Full Drift Check & Reference App

**Goal**: The build fails whenever the contract, schema, and service surface disagree — every RPC, field, and constraint is provably accounted for — proven end-to-end by a reference application running the canonical pipeline in CI
**Mode:** mvp
**Depends on**: Phase 4
**Requirements**: DRIFT-01, DRIFT-02, DRIFT-03, DRIFT-04, DRIFT-05, DRIFT-06, DRIFT-07, PIPE-07
**Success Criteria** (what must be TRUE):

  1. The build fails when a service RPC is claimed by neither a flow, a CRUD binding, nor an explicit `Manual`, and warns (silenceable) when a proto-input flow has no claiming RPC
  2. The build fails when a skipped message-typed field has no matching declared edge and no explicit exclude, and warns when a `google.api.resource_reference` id field has no corresponding edge
  3. The build fails when a schema hand-declares a validator that conflicts with a mirrored protovalidate constraint (by fingerprint), or when the committed `FileDescriptorSet` is stale relative to `.proto` sources
  4. Drift-check output is deterministic and sorted, so CI diffs stay meaningful across runs
  5. A reference Order/Inventory application builds end-to-end through the full canonical pipeline in CI, and connect-go client tests against its generated handlers assert privacy denials surface as `PermissionDenied`

**Plans**: TBD

## Progress

**Execution Order:**
Phases execute in numeric order: 1 → 2 → 3 → 4 → 5

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 1. MixinForProto Core | 2/5 | In Progress|  |
| 2. CRUD Handlers & Interceptor Chain | 0/TBD | Not started | - |
| 3. Validation Fidelity | 0/TBD | Not started | - |
| 4. Flow Binding | 0/TBD | Not started | - |
| 5. Full Drift Check & Reference App | 0/TBD | Not started | - |
