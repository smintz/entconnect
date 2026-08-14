# Roadmap: entconnect

## Overview

entconnect inverts entproto's generation direction: a protobuf contract drives ent field derivation (`mixinforproto`) and ConnectRPC transport generation (`entc` extension), so contract/schema/service drift becomes a build failure instead of a runtime surprise. The journey starts with the piece everything else depends on — a standalone, codegen-free mixin that turns a message type into ent fields with enough provenance for later drift checking — then builds the CRUD transport layer on top of it (handlers, partial updates, the interceptor chain), deepens validation fidelity for the long tail protovalidate constraints Tier 1 can't translate, binds RPCs to entflow once that project has real running code, and closes with the full bidirectional drift check plus a reference application proving the whole canonical pipeline holds together in CI.

## Phases

**Phase Numbering:**

- Integer phases (1, 2, 3): Planned milestone work
- Decimal phases (2.1, 2.2): Urgent insertions (marked with INSERTED)

Decimal phases appear between their surrounding integers in numeric order.

- [x] **Phase 1: MixinForProto Core** - Contract-derived ent fields, provenance annotations, Tier 1 validation, and the two-module CI scaffold
- [x] **Phase 2: CRUD Handlers & Interceptor Chain** - Generated Get/List/Create/Update/Delete Connect handlers with FieldMask-gated updates and a fixed, privacy-aware interceptor chain
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

**Plans**: 9/9 plans executed
Plans:
**Wave 1**

- [x] 01-01-PLAN.md — Walking-skeleton tracer: one proto field becomes one annotated ent field, proven through a real entc schema load

**Wave 2** *(blocked on Wave 1 completion)*

- [x] 01-02-PLAN.md — Field-mapping expansion: scalars, enums, WKTs, presence, maps and AsJSON, golden-asserted against a synthetic corpus
- [x] 01-03-PLAN.md — Canonical pipeline script, two-module CI, release-tag convention, and adopter documentation

**Wave 3** *(blocked on Wave 2 completion)*

- [x] 01-04-PLAN.md — Exclude/Override options, reserved-identifier and oneof gates, collected failures, and in-process reproduction
- [x] 01-05-PLAN.md — Tier 1 validation relay: string, presence and numeric translation with residual provenance recording

**Gap Closure Wave 1** *(closes `01-VERIFICATION.md` gaps; runs after the five original plans)*

- [x] 01-06-PLAN.md — Gap 1 (MIX-02/SC1): `classify()` gains the missing `IsList()` branch so repeated scalars/enums fail at schema load instead of silently deriving singular fields
- [x] 01-07-PLAN.md — Gap 4 (PIPE-03): one shared stub-generation script, an orphan-aware staleness gate runnable via `make check-stubs`, and three executed detection proofs

**Gap Closure Wave 2** *(blocked on Gap Closure Wave 1)*

- [x] 01-08-PLAN.md — Gaps 2 and 3 (VAL-01/VAL-03): delegated format validators judge only their own field, and `required` on a presence-tracking string/bytes means presence, not non-emptiness

**Gap Closure Wave 3** *(blocked on Gap Closure Wave 2)*

- [x] 01-09-PLAN.md — Root cause: corpus-adequacy guards making "the corpus avoids this shape" a detectable condition rather than a silent one

### Phase 2: CRUD Handlers & Interceptor Chain

**Goal**: Developers get generated ConnectRPC CRUD handlers wired directly to the ent client, with safe partial updates and a fixed, privacy-aware interceptor chain — no hand-written handler code, no ent client leakage
**Mode:** mvp
**Depends on**: Phase 1
**Requirements**: CRUD-01, CRUD-02, CRUD-03, CRUD-04, CRUD-05, CRUD-06, CRUD-07, INT-01, INT-02, INT-03, INT-04, INT-05
**Success Criteria** (what must be TRUE):

  1. Developer runs `go generate` and gets working Get/Create/Delete Connect handlers over a MixinForProto-backed entity, calling the ent client directly
  2. Developer gets a List handler that pages via AIP-158 `page_token`/`next_page_token` on hand-emitted keyset predicates over ent's own comparison operators — not offset paging, and with no dependency on the ent contrib GraphQL extension
  3. Developer gets an Update handler that requires a `FieldMask` and only calls `Set*` for paths present in the mask (untouched fields never zeroed); an unknown mask path fails the build against the message descriptor
  4. Every request flows through a fixed interceptor chain (authn → viewer injection → protovalidate → otel → handler) built once per process; privacy policy denials surface to the client as Connect `PermissionDenied`, and generated server wiring is the only place an `*ent.Client` is used to serve requests — it is never handed back to application code (corrected 2026-08-09; the original "only place ... is constructed" wording is unachievable, since generated code cannot know a deployment's connection string — see REQUIREMENTS.md CRUD-07 and 02-VERIFICATION.md)
  5. Developer can hand-write one handler via `entconnect.Manual("rpc")`, and it still runs inside the generated interceptor chain and shows up in drift-check output; all generated code is byte-stable across runs and covered by golden-file tests

> **Planning note (2026-08-08):** Success Criterion 2's original wording attributed List paging to
> a keyset method native to core `entgo.io/ent`. Phase 2 research disproved this by direct source
> inspection: that method is generated exclusively by the ent contrib GraphQL extension, not core
> ent. Corrected above to describe the hand-emitted keyset predicate mechanism this phase actually
> builds. See `.planning/phases/02-crud-handlers-interceptor-chain/02-RESEARCH.md` §Summary and
> Pitfall 1.

**Plans**: 5/5 plans executed
Plans:
**Wave 1**

- [x] 02-01-PLAN.md — Tracer: one entity, one Get RPC, contract to descriptor to emitted handler to a real Connect response through the fixed chain

**Wave 2** *(blocked on Wave 1 completion; three parallel plans, disjoint files)*

- [x] 02-02-PLAN.md — Create and Delete handlers against the ent client, with mutation-shaped error mapping
- [x] 02-03-PLAN.md — List with hand-emitted keyset paging and the fingerprinted page token, plus the CRUD-03 requirement-text correction
- [x] 02-04-PLAN.md — FieldMask-gated Update, validated at build time against descriptor and Phase 1 provenance and at request time against the live message

**Wave 3** *(blocked on Wave 2 completion)*

- [x] 02-05-PLAN.md — Manual escape hatch inside the generated chain, the deterministic claims report and duplicate-claim gate, and golden/byte-stability CI hardening

### Phase 3: Validation Fidelity

**Goal**: Residual and message-level validation rules that Tier 1 can't translate get executed with byte-identical results at both the RPC boundary and the storage layer, proven by an automated differential harness
**Mode:** mvp
**Depends on**: Phase 2
**Requirements**: VAL-04, VAL-05, VAL-06, VAL-07, VAL-08, VAL-09, VAL-10, VAL-11, PIPE-05, PIPE-06
**Success Criteria** (what must be TRUE):

  1. The mixin hook evaluates the full protovalidate field-rule set for every in-scope field — translated and residual alike, not residual-only — compiled once at schema load; a Tier 1 translated rule failing at the storage layer must still carry protovalidate's own constraint ID, which a residual-only hook cannot produce (Amended 2026-08-14, D-02 — see 03-CONTEXT.md: this scope is the minimum needed for VAL-07's identity guarantee to hold)
  2. A schema-layer violation carries the same protovalidate constraint ID and message as the boundary interceptor would produce, exposed as a structured error consumable outside any RPC context — so a caller cannot tell which layer caught it
  3. Message-level (cross-field) rules stay boundary-only unless a developer opts in with `WithMessageRules(OnCreate)`; the mixin hook's ordering relative to schema-declared hooks/policies is documented and covered by a test that fails if ent changes that order; the boundary validator is built once per process, not per request
  4. CI fails when `mixinforproto`'s and `entconnect`'s resolved protovalidate/cel-go versions diverge
  5. A conformance corpus golden-asserts every field-mapping rule and protovalidate constraint class against derived fields, and a differential harness feeds random values through every corpus message asserting `protovalidate verdict == ent mutation verdict` for field-scoped rules

**Plans**: 5/5 plans executed
Plans:
**Wave 1**

- [x] 03-01-PLAN.md — Tracer: one residual protovalidate CEL rule rejects a real ent Create end to end, driverless; `runtime.MapError` learns the new error type; the ROADMAP SC1 / VAL-05 / CLAUDE.md corrections land first

**Wave 2** *(blocked on Wave 1)*

- [x] 03-02-PLAN.md — The reverse conversion table for every derivation kind, the JSON-to-`dynamicpb` leg as its own risk-isolated task, and D-09's boundary-only rule provenance on `SourceMessage`

**Wave 3** *(blocked on Wave 2; two parallel plans, disjoint files)*

- [x] 03-03-PLAN.md — Hybrid completion: protovalidate's own evaluator via `WithFilter`, the mixed-rule convergence settled by test, D-06's operation-dependent scope, and the schema-load panic for uncompilable CEL
- [x] 03-04-PLAN.md — Root-module proofs: real-client wiring, relative hook ordering across policy-bearing and policy-free schemas, once-per-process validator construction, and the VAL-11 dependency-parity gate

**Wave 4** *(blocked on Wave 3)*

- [x] 03-05-PLAN.md — `WithMessageRules(OnCreate)` with the excluded-field-reference gate, constraint-class corpus coverage, and the deterministically seeded differential sweep

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
| 1. MixinForProto Core | 9/9 | In Progress|  |
| 2. CRUD Handlers & Interceptor Chain | 0/5 | Planned | - |
| 3. Validation Fidelity | 5/5 | In Progress|  |
| 4. Flow Binding | 0/TBD | Not started | - |
| 5. Full Drift Check & Reference App | 0/TBD | Not started | - |
