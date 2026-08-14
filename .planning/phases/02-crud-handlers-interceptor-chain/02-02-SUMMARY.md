---
phase: 02-crud-handlers-interceptor-chain
plan: 02
subsystem: api
tags: [entc, connect-rpc, protobuf, ent, codegen, mutations]

# Dependency graph
requires:
  - phase: 02-crud-handlers-interceptor-chain (plan 01)
    provides: "entc/ extension skeleton (RPC-binding annotations, procedure resolution, generator registry, gen.Hook emission), runtime/ (fixed interceptor chain, MapError, viewer), internal/entconnecttest/read/ working GetOrder fixture"
provides:
  - "entc/crud_create.go + entc/templates/create.tmpl — the OpCreate generator: emits one Set<Field> call per surviving contract field, Save(ctx) with the request context, ConstraintError->AlreadyExists/ValidationError->InvalidArgument mapped directly, else runtime.MapError"
  - "entc/crud_delete.go + entc/templates/delete.tmpl — the OpDelete generator: by-primary-key-only DeleteOneID(id).Exec(ctx), never a predicate-free/bulk delete, NotFound mapped directly, else runtime.MapError"
  - "entc/templates/server.tmpl + extension.go fix — a single combined NewServer per graph (not per proto service), fixing a duplicate-declaration bug the two-service Item fixture exposed"
  - "proto/entconnecttest/v1/write.proto — Item entity, ItemCreateService, ItemDeleteService fixture contract"
  - "internal/entconnecttest/write/ — a real, generated, tested fixture app proving Create and Delete end to end over a real ent client and real HTTP, with both RPCs bound on one schema (Bindings.Merge exercised)"
affects: [02-03-list-paging, 02-04-update-fieldmask, 02-05-manual-drift-report, phase-3-validation, phase-5-drift-check]

# Actuals (#2632)
actuals:
  tokens: 47500
  tasks: 2
  commits: 3

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Generator iterates mixinforproto.SourceMessage.Fields (descriptor declaration order, D-20) rather than gen.Type.Fields directly, skipping names in Excluded and the ent-generated primary key (matched via gen.Type.ID.Name, not string heuristics)"
    - "Combined server.entconnect.go: one NewServer(client, authenticator, opts...) per graph, wiring every bound proto service's chain-wrapped handler into a single entconnectruntime.Server via its variadic NewServer(routes ...Route) — required the moment a graph binds RPCs across 2+ proto services"
    - "Mutation ent.Policy uses privacy.MutationPolicy (not QueryPolicy) with the same ContextQueryMutationRule shape the read fixture's Query policy used — both interfaces are satisfied by the same QueryMutationRule value"

key-files:
  created:
    - entc/crud_create.go
    - entc/crud_delete.go
    - entc/templates/create.tmpl
    - entc/templates/delete.tmpl
    - entc/templates/server.tmpl
    - proto/entconnecttest/v1/write.proto
    - internal/entconnecttest/write/ent/schema/item.go
    - internal/entconnecttest/write/create_test.go
    - internal/entconnecttest/write/delete_test.go
    - internal/entconnecttest/write/entconnect/item_create_service.entconnect.go
    - internal/entconnecttest/write/entconnect/item_delete_service.entconnect.go
    - internal/entconnecttest/write/entconnect/server.entconnect.go
  modified:
    - entc/extension.go
    - entc/templates/service.tmpl
    - internal/entconnecttest/read/entconnect/order_read_service.entconnect.go
    - internal/entconnecttest/read/entconnect/server.entconnect.go
    - internal/gen/entconnecttestv1/write.pb.go
    - internal/gen/entconnecttestv1/entconnecttestv1connect/write.connect.go
    - proto/descriptorset.binpb

key-decisions:
  - "NewServer moved out of the per-service template into its own entc/templates/server.tmpl, rendered once per Generate() pass into a combined server.entconnect.go — see Deviations for why this was necessary, not optional"
  - "Create's Set<Field> iteration order follows mixinforproto.SourceMessage.Fields (the complete descriptor field inventory in declaration order), cross-referenced against Excluded and gen.Type.Fields by name — never gen.Type.Fields' own order directly — so the generator's iteration order is traceable to the same provenance annotation the drift check (Phase 5) will read"
  - "Item's ent ID uses the default auto-incrementing int (mirrors Order from 02-01): \"id\" is excluded from MixinForProto derivation for the same structural-reserved-identifier reason, and Create/Delete both convert between the wire's string id and ent's int ID at the boundary"

patterns-established:
  - "Pattern: a proto message field with the entity's own full name (found via a linear scan of Fields(), matching Kind()==MessageKind && Message().FullName()==entity) is how a Create generator locates both the request's \"carry the entity\" field and the response's \"hold the result\" field — reusable by the Update generator (02-04)"
  - "Pattern: mutation privacy policies use privacy.MutationPolicy (not QueryPolicy); the same ContextQueryMutationRule/AlwaysAllowRule values satisfy both interfaces, so an app schema author writes one rule shape for both Query and Mutation policies"

requirements-completed: [CRUD-02, INT-03]

coverage:
  - id: D1
    description: "A developer annotates an ent schema with entconnect.CreateRPC(...) and a generated Connect Create handler persists a new row through the ent client, inside the fixed interceptor chain"
    requirement: "CRUD-02"
    verification:
      - kind: e2e
        ref: "internal/entconnecttest/write/create_test.go#TestCreateItem_RealRoundTrip"
        status: pass
      - kind: other
        ref: "grep -n '.Save(ctx)' internal/entconnecttest/write/entconnect/item_create_service.entconnect.go — present; no context.Background()/context.TODO() in the file"
        status: pass
    human_judgment: false
  - id: D2
    description: "Creating the same uniquely-constrained sku twice returns CodeAlreadyExists on the second call, not a duplicate row and not CodeInternal"
    requirement: "CRUD-02"
    verification:
      - kind: e2e
        ref: "internal/entconnecttest/write/create_test.go#TestCreateItem_DuplicateSkuIsAlreadyExists"
        status: pass
    human_judgment: false
  - id: D3
    description: "A developer annotates an ent schema with entconnect.DeleteRPC(...) and a generated Connect Delete handler removes exactly the identified row through the ent client, by primary key only — never a predicate-free or bulk delete"
    requirement: "CRUD-02"
    verification:
      - kind: e2e
        ref: "internal/entconnecttest/write/delete_test.go#TestDeleteItem_RealRoundTrip, #TestDeleteItem_UnrelatedRowSurvives"
        status: pass
      - kind: other
        ref: "grep -n '.Exec(ctx)' + negative grep 'client.Item.Delete()' (no bulk delete call) in item_delete_service.entconnect.go"
        status: pass
    human_judgment: false
  - id: D4
    description: "Deleting the same identifier twice returns CodeNotFound on the second call"
    requirement: "CRUD-02"
    verification:
      - kind: e2e
        ref: "internal/entconnecttest/write/delete_test.go#TestDeleteItem_RepeatIsNotFound"
        status: pass
    human_judgment: false
  - id: D5
    description: "An ent privacy denial on a Create or Delete mutation reaches the Connect client as CodePermissionDenied, and the denied mutation does not persist/remove anything"
    requirement: "INT-03"
    verification:
      - kind: e2e
        ref: "create_test.go#TestCreateItem_PrivacyDenyIsPermissionDenied, delete_test.go#TestDeleteItem_PrivacyDenyIsPermissionDenied"
        status: pass
    human_judgment: false
  - id: D6
    description: "Both generators register themselves via init()/RegisterGenerator without any edit to entc/crud.go"
    verification:
      - kind: other
        ref: "git diff --exit-code -- entc/crud.go (relative to 02-01's committed version) — empty"
        status: pass
    human_judgment: false

duration: ~1h20min
completed: 2026-08-09
status: complete
---

# Phase 2 Plan 2: Create/Delete Handlers Summary

**Generated Connect Create and Delete handlers that persist and remove real rows through a real ent client inside the same fixed interceptor chain the Get slice proved — plus a fix to the shared server-wiring emission mechanism that the plan's own two-service fixture was the first thing in this repo to actually exercise.**

## Performance

- **Duration:** ~1h20min
- **Tasks:** 2 (both `type="auto"`)
- **Files modified:** 39 (per `git diff --shortstat` against the 02-01 baseline)

## Accomplishments

- `entc/crud_create.go`/`entc/templates/create.tmpl` (`OpCreate`): iterates the mixin's `SourceMessage.Fields` provenance in descriptor declaration order, skips `Excluded` names and the ent-generated primary key, and emits one `Set<Field>(req.Msg.Get<Entity>().Get<Field>())` call per surviving field, `Save(ctx)` with the request context so privacy sees the injected viewer, `ConstraintError`→`AlreadyExists`/`ValidationError`→`InvalidArgument` mapped directly, everything else through `runtime.MapError`.
- `entc/crud_delete.go`/`entc/templates/delete.tmpl` (`OpDelete`): parses the request's `id` field, deletes **by primary key only** via `DeleteOneID(id).Exec(ctx)` — never a predicate-free or bulk `Delete()` — `NotFound` mapped directly, else `runtime.MapError`.
- `proto/entconnecttest/v1/write.proto` + generated stubs: `Item` entity with a `sku` protovalidate rule, `ItemCreateService`/`ItemDeleteService` as two independent services on one entity.
- `internal/entconnecttest/write/`: a complete, generated, tested fixture proving both verbs end to end — real in-memory SQLite ent client, real HTTP, real Connect client, `-race` tested, 8 tests covering the happy path, idempotency edges, protovalidate rejection, and privacy denial for both verbs.
- Fixed a genuine bug in Wave 1's per-service emission mechanism (`entc/extension.go`/`entc/templates/service.tmpl`): a hardcoded `func NewServer` per emitted service file collides the moment a graph binds RPCs across 2+ proto services on one schema — exactly this plan's own `ItemCreateService` + `ItemDeleteService` fixture. Fixed by moving `NewServer` into a new `entc/templates/server.tmpl`, rendered once per `Generate()` pass into a combined `server.entconnect.go` that wires every bound service's handler into a single `entconnectruntime.Server` (the runtime layer's `NewServer(routes ...Route)` was already variadic, built for exactly this in 02-01, just never exercised).

## Task Commits

Each task was committed atomically:

1. **write.proto corpus + generated stubs** — `438dddd` (feat)
2. **Task 1: Create end to end** — `6db3d04` (feat)
3. **Task 2: Delete end to end** (+ the `NewServer` collision fix) — `bb1a20a` (feat)

**Plan metadata:** *(this commit, docs: complete plan)*

## Files Created/Modified

- `entc/crud_create.go`, `entc/templates/create.tmpl` — the `OpCreate` generator
- `entc/crud_delete.go`, `entc/templates/delete.tmpl` — the `OpDelete` generator
- `entc/templates/server.tmpl` (new), `entc/extension.go`, `entc/templates/service.tmpl` (modified) — the combined-`NewServer` fix
- `proto/entconnecttest/v1/write.proto`, `internal/gen/entconnecttestv1/write.pb.go`, `internal/gen/entconnecttestv1/entconnecttestv1connect/write.connect.go`, `proto/descriptorset.binpb`
- `internal/entconnecttest/write/ent/schema/item.go` — `Item` entity, unique `sku` index, `CreateRPC`+`DeleteRPC` bindings, mutation privacy policy
- `internal/entconnecttest/write/ent/entc.go`, `ent/generate.go`, full generated `ent/` package
- `internal/entconnecttest/write/entconnect/{item_create_service,item_delete_service,server}.entconnect.go` — generated handlers
- `internal/entconnecttest/write/create_test.go`, `delete_test.go` — 8 end-to-end tests
- `internal/entconnecttest/read/entconnect/order_read_service.entconnect.go`, `server.entconnect.go` — regenerated (Wave 1's fixture, mechanically updated by the `NewServer` fix; tests unchanged and still passing)

## Decisions Made

See `key-decisions` in frontmatter above.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1/3 - Bug/Blocking] `NewServer` redeclared across two per-service files**

- **Found during:** Task 2, immediately after adding `DeleteRPC` to the same schema as `CreateRPC` and running `go generate` — `go build` failed with `NewServer redeclared in this block` between `item_create_service.entconnect.go` and `item_delete_service.entconnect.go`.
- **Issue:** `entc/templates/service.tmpl` hardcoded a literal, non-templated `func NewServer(...)` inside every per-service output file. `entc/extension.go`'s `Generate` emits one file per proto service into the same `entconnect` package — this was never a problem for 02-01's single-service `OrderReadService` fixture, but this plan's own task text explicitly requires **two separate services** (`ItemCreateService`, `ItemDeleteService`) bound on **one schema**, which is exactly the condition that exposes the collision. Confirmed live via a real `go build` failure before any fix was applied.
- **Fix:** Moved `NewServer` out of `service.tmpl` into a new `entc/templates/server.tmpl`, and extended `entc/extension.go`'s `Generate` to accumulate one `serverEntry` per bound proto service (in the same deterministic `svcNames` order, D-20) and render **one** combined `server.entconnect.go` after the per-service loop, wiring every bound service's chain-wrapped handler into a single `entconnectruntime.Server` via its already-variadic `NewServer(routes ...Route)` (built in 02-01, never previously exercised with >1 route). This matches D-11/D-12's explicit intent — "one `NewServer(client, authenticator, opts...)` for the whole generated wiring" — which the original per-file implementation had silently diverged from.
- **Files modified:** `entc/extension.go`, `entc/templates/service.tmpl`, `entc/templates/server.tmpl` (new). Neither file is in this plan's declared `files_modified` list, and neither is touched by either sibling wave plan (02-03, 02-04) per their own `files_modified` lists (verified by direct inspection before editing) — no parallel-agent collision risk.
- **Verification:** Regenerated `internal/entconnecttest/read/` (Wave 1's own fixture, not owned by this plan) and confirmed its existing tests (`TestGetOrder_*`, 4 cases) still pass unchanged with `-race` — the only diff is `NewServer` moving out of `order_read_service.entconnect.go` into a new `server.entconnect.go`. `go generate ./internal/entconnecttest/write/...` run twice leaves a clean diff (idempotent). `make build && make vet && make test && make check-stubs` all pass.
- **Committed in:** `bb1a20a` (Task 2 commit, documented inline in the commit message)

---

**Total deviations:** 1 auto-fixed (Rule 1/3 — bug discovered exactly where this plan's own two-service design first exercised the affected code path)
**Impact on plan:** Necessary for Task 2's stated acceptance criteria (`make build` must pass with both `CreateRPC` and `DeleteRPC` bound on one schema) to be achievable at all. No scope creep beyond fixing the exposed defect and confirming backward compatibility with Wave 1's fixture.

## Issues Encountered

None beyond the deviation documented above.

## User Setup Required

None — no external service configuration required.

## Next Phase Readiness

- `entc/crud.go`'s registry gained two more verbs (`OpCreate`, `OpDelete`) without any edit to the registry file itself, confirming the `RegisterGenerator` pattern scales as 02-01 intended.
- The combined `NewServer`/`server.entconnect.go` mechanism is now proven with 2 services on 1 schema; 02-03 (List) and 02-04 (Update) each add their own single-service fixture and will not re-trigger this class of bug, but any later phase combining 3+ verbs on one schema inherits the fix for free.
- `mixinforproto.SourceMessage.Fields`-driven iteration (skip `Excluded`, cross-reference `gen.Type.Fields` by name) is now a proven pattern 02-04's Update generator can reuse directly for its FieldMask-gated `Set*` emission.
- D-07's sorted claims report (`crud:<op>`/`manual`/`unclaimed`) remains explicitly deferred to 02-05, unaffected by this plan.

---
*Phase: 02-crud-handlers-interceptor-chain*
*Completed: 2026-08-09*

## Self-Check: PASSED

All key created files verified present on disk (`entc/crud_create.go`, `entc/crud_delete.go`, `entc/templates/create.tmpl`, `entc/templates/delete.tmpl`, `entc/templates/server.tmpl`, `proto/entconnecttest/v1/write.proto`, `internal/entconnecttest/write/ent/schema/item.go`, `internal/entconnecttest/write/create_test.go`, `internal/entconnecttest/write/delete_test.go`, `internal/entconnecttest/write/entconnect/{item_create_service,item_delete_service,server}.entconnect.go`); all 3 commit hashes (`438dddd`, `6db3d04`, `bb1a20a`) verified present in `git log --oneline`. No missing items.
