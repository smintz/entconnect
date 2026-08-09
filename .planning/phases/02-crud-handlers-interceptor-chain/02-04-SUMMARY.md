---
phase: 02-crud-handlers-interceptor-chain
plan: 04
subsystem: api
tags: [entc, connect-rpc, fieldmask, protobuf, ent, codegen]

# Dependency graph
requires:
  - phase: 02-crud-handlers-interceptor-chain
    provides: "02-01's entc.Extension skeleton (RegisterGenerator registry, gen.Hook emission, Bindings annotations, resolve.go's procedure resolution), runtime.Chain/Server/MapError, and mixinforproto's SourceMessage/SourceField provenance annotations"
provides:
  - "runtime/fieldmask.go — request-time FieldMask validation (ValidateMask): empty/absent rejection, explicit nested/wildcard rejection layered on top of fieldmaskpb.IsValid, per-path descriptor validity + allowed-set membership, sorted deduplicated output"
  - "entc/maskcheck.go — build-time mask-path validation (ValidateMaskPaths) cross-checking the Update entity's descriptor against Phase 1's SourceMessage provenance: satisfiable / excluded / unknown / nested-or-wildcard classification, collected in one pass"
  - "entc/crud_update.go + entc/templates/update.tmpl — the OpUpdate generator: one sorted switch case per derived, non-excluded, SourceField-annotated field, an erroring default, D-18 error classification, LengthUnitDivergentIDs recorded in a doc comment"
  - "proto/entconnecttest/v1/update.proto — Patch entity, UpdatePatchRequest/Response, PatchUpdateService, plus PatchUpdateAltService (a maskcheck-test-only second service)"
  - "internal/entconnecttest/update/ — real generated fixture proving the zero-collapse case, request-time and build-time mask rejections, idempotent repeat application, and a privacy denial on Update"
affects: [02-05-manual-drift-report, phase-3-validation, phase-5-drift-check]

# Actuals (#2632)
actuals:
  tokens: 55500
  tasks: 3
  commits: 3

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Build-time descriptor+provenance cross-check (entc/maskcheck.go) called from inside a per-verb Generator (entc/crud_update.go), before any template rendering — mirrors entc/errors.go's collected-failures discipline rather than returning on first offense"
    - "Request-time mask validation checks each FieldMask path individually against fieldmaskpb.FieldMask.IsValid (a single-path FieldMask per check) rather than the whole slice in one call, specifically so a rejection can name the offending path verbatim"
    - "A generated Update handler's allowed field set is embedded as a literal []string{...} in the emitted source (entc/crud_update.go's allowedLiteral), never recomputed at runtime — runtime.ValidateMask receives it as a plain argument"

key-files:
  created:
    - runtime/fieldmask.go
    - runtime/fieldmask_test.go
    - entc/maskcheck.go
    - entc/maskcheck_test.go
    - entc/crud_update.go
    - entc/templates/update.tmpl
    - proto/entconnecttest/v1/update.proto
    - internal/entconnecttest/update/ent/schema/patch.go
    - internal/entconnecttest/update/update_test.go
    - internal/entconnecttest/update/mask_edges_test.go
    - internal/entconnecttest/update/badmask/ent/schema/badpatch.go
    - internal/entconnecttest/update/badmask/ent/schema/excludedpatch.go
  modified:
    - internal/gen/entconnecttestv1/update.pb.go
    - internal/gen/entconnecttestv1/entconnecttestv1connect/update.connect.go
    - proto/descriptorset.binpb

key-decisions:
  - "D-17's build-time exclusion check is UNCONDITIONAL: any field present in a schema's Exclude() set is a build failure once ValidateMaskPaths runs, with no carve-out for a field the developer deliberately never wants updatable. This means internal_note's own exclusion in the real, committed Patch fixture (Task 1) becomes a build failure if that fixture is ever regenerated after Task 2 wires ValidateMaskPaths into crud_update.go — confirmed live: `go generate ./internal/entconnecttest/update/...` now fails deterministically (identical error text on repeated runs) naming internal_note. This is read as the plan's own intent, not a bug: Task 1's own comment on the Exclude call reads 'so task 2 has a real excluded field to reject at build time.' The already-committed, already-tested internal/entconnecttest/update/entconnect/patch_update_service.entconnect.go is never regenerated again within this plan — Tasks 2 and 3's own <verify> blocks never re-run `go generate` on it, only `go test`, `make build`, and `make vet` against the existing generated code."
  - "The entity's own reserved structural identifier field ('id') is excluded from ValidateMaskPaths' candidate walk entirely (skipped before classification) rather than being treated as an excluded-classification failure like any other Exclude()d field — it is always excluded by construction (mixinforproto/reserved.go's reservedStructural) and is never itself a mask target."
  - "The negative-build fixtures (badpatch.go, excludedpatch.go) bind to two different procedures (PatchUpdateService/UpdatePatch and a maskcheck-test-only PatchUpdateAltService/UpdatePatchAlt) so they can be loaded in the SAME entc.LoadGraph call without tripping D-05's duplicate-claim check, which would otherwise short-circuit codegen before either fixture's mask failure is ever discovered."
  - "The 'two offenders collected in one returned error' acceptance criterion is proven as a direct ValidateMaskPaths pure-function call (one SourceMessage with both an excluded field and an unknown field simultaneously), not via the full entc.LoadGraph + Generate() pipeline across two schemas — entc/extension.go's Generate() checks failures per-service after each service's inner loop, so two DIFFERENT services' failures cannot both surface in one error without editing entc/extension.go, which sits outside this plan's files_modified and is shared with sibling wave agents. TestMaskCheck_NegativeBuildFixtures separately proves the real Generate() call path does fail on the fixtures (a distinct, narrower claim)."
  - "proto/entconnecttest/v1/update.proto's Patch message carries no buf.validate constraints at all, so SourceField.LengthUnitDivergentIDs is empty for every derived field — Task 3's encoding case (mask_edges_test.go) asserts this absence directly, per the plan's own sanctioned fallback, rather than inventing a divergent constraint the corpus does not have."

patterns-established:
  - "Build-time mask-path validation (entc/maskcheck.go) and request-time mask validation (runtime/fieldmask.go) are deliberately separate, non-overlapping layers: the build-time check can reject a schema before any handler exists; the request-time check protects every live call regardless of what a (possibly stale, hand-edited) generated file happens to allow."

requirements-completed: [CRUD-04, CRUD-05]

coverage:
  - id: D1
    description: "A generated Update handler requires an explicit google.protobuf.FieldMask and applies a Set call for exactly the paths present in it; a field absent from the mask keeps its stored value even though the request message carries its Go zero value, across repeated identical calls"
    requirement: "CRUD-04"
    verification:
      - kind: e2e
        ref: "internal/entconnecttest/update/update_test.go#TestUpdate_MaskedFieldChangesUnmaskedFieldsSurvive"
        status: pass
      - kind: e2e
        ref: "internal/entconnecttest/update/mask_edges_test.go#TestMaskEdges_RepeatedApplicationIsANoOp"
        status: pass
    human_judgment: false
  - id: D2
    description: "An empty, absent, nested, wildcard, unknown, or excluded-field mask path is rejected with CodeInvalidArgument naming the offending path verbatim, having mutated nothing"
    requirement: "CRUD-04"
    verification:
      - kind: e2e
        ref: "internal/entconnecttest/update/mask_edges_test.go#TestMaskEdges_AbsentMask,TestMaskEdges_ZeroLengthPaths,TestMaskEdges_EmptyStringPath,TestMaskEdges_NestedPath,TestMaskEdges_WildcardPath,TestMaskEdges_UnknownPath,TestMaskEdges_ExcludedFieldPath,TestMaskEdges_CaseDifferingPathRejected"
        status: pass
      - kind: unit
        ref: "runtime/fieldmask_test.go#TestValidateMask"
        status: pass
    human_judgment: false
  - id: D3
    description: "Duplicate mask paths collapse to a single Set call, and path order in the mask never changes the resulting row"
    requirement: "CRUD-04"
    verification:
      - kind: e2e
        ref: "internal/entconnecttest/update/mask_edges_test.go#TestMaskEdges_DuplicatePathAppliesOnce,TestMaskEdges_PathOrderIsIrrelevant"
        status: pass
    human_judgment: false
  - id: D4
    description: "Codegen fails, collected and deterministically sorted, when an Update binding's mask surface names a field the schema excluded, a field with no corresponding settable ent field, or leaves nothing settable at all — each failure's first line alone names the message, the path, and a remedy"
    requirement: "CRUD-05"
    verification:
      - kind: unit
        ref: "entc/maskcheck_test.go#TestMaskCheck_ValidateMaskPaths"
        status: pass
      - kind: integration
        ref: "entc/maskcheck_test.go#TestMaskCheck_NegativeBuildFixtures"
        status: pass
      - kind: unit
        ref: "entc/maskcheck_test.go#TestMaskCheck_Determinism"
        status: pass
    human_judgment: false
  - id: D5
    description: "The committed emitted Update handler never generates a Set case for a field the schema excluded from derivation"
    requirement: "CRUD-05"
    verification:
      - kind: other
        ref: "grep -n internal_note internal/entconnecttest/update/entconnect/patch_update_service.entconnect.go — no matches"
        status: pass
    human_judgment: false
  - id: D6
    description: "The string-length unit divergence Phase 1 recorded (SourceField.LengthUnitDivergentIDs) has an asserted answer for this corpus"
    requirement: "CRUD-05"
    verification:
      - kind: unit
        ref: "internal/entconnecttest/update/mask_edges_test.go#TestMaskEdges_LengthUnitDivergence"
        status: pass
    human_judgment: false
  - id: D7
    description: "An ent privacy denial on the Update mutation surfaces as Connect's PermissionDenied"
    verification:
      - kind: e2e
        ref: "internal/entconnecttest/update/update_test.go#TestUpdate_PrivacyDenyIsPermissionDenied"
        status: pass
    human_judgment: false

duration: 3h40min
completed: 2026-08-09
status: complete
---

# Phase 2 Plan 4: FieldMask-Gated Update Summary

**A generated Connect Update handler requires an explicit `google.protobuf.FieldMask`, applies exactly one sorted `Set*` call per masked, non-excluded field, and is validated twice — once at build time against the message descriptor and Phase 1's `SourceMessage` provenance, once at request time against the live message — closing entconnect.md §9's Open Question 5 in favor of explicit masks over the proto3 zero-collapse trap.**

## Performance

- **Duration:** ~3h40min
- **Started:** 2026-08-09T00:55:00Z (approx)
- **Completed:** 2026-08-09T04:35:00Z
- **Tasks:** 3 (all `type="auto"`)
- **Files modified:** 15 hand-written new/modified + 24 ent-generated (update fixture) + 3 proto/gen additions

## Accomplishments

- `runtime/fieldmask.go`'s `ValidateMask` enforces D-14 through D-16 at request time: a nil or empty mask is `InvalidArgument` (never "update everything"), nested/wildcard paths are rejected explicitly (`fieldmaskpb.IsValid` alone would accept them), every remaining path is checked individually against the live descriptor (so a rejection can name the specific offending path) and against the generator's own `allowed` set, and the surviving paths come back deduplicated and sorted.
- `entc/maskcheck.go`'s `ValidateMaskPaths` enforces D-17 at build time: it walks the Update entity's complete field descriptor and classifies every field as satisfiable, excluded (deliberately, via `Exclude()`), unknown (no corresponding settable ent field — e.g. an `Override()`d field with no `SourceField` provenance), or nested/wildcard, collecting every finding in one pass rather than stopping at the first.
- `entc/crud_update.go` + `entc/templates/update.tmpl` render the actual handler: a sorted `switch` with one case per allowed field and an erroring `default`, `NotFoundError`/`ConstraintError`/`ValidationError` classified against the local `ent` package, and a doc comment recording `SourceField.LengthUnitDivergentIDs` (empty for this corpus).
- A complete, generated, tested fixture (`internal/entconnecttest/update/`) proves the zero-collapse case end to end — a field left at its Go zero value but absent from the mask survives an update untouched, across repeated identical requests — plus every edge from the empty/nested/wildcard/unknown/duplicate/ordering/encoding/idempotency matrix, against a real ent client and a real HTTP round trip.
- Two negative-build fixtures (`badmask/ent/schema/{badpatch,excludedpatch}.go`) prove `entconnect.Generate` genuinely fails, with a self-sufficient first line, when a schema's Update binding cannot be satisfied.

## Task Commits

Each task was committed atomically:

1. **Task 1: Update end to end — masked Set calls persist, unmasked fields survive** — `e78a9cd` (feat)
2. **Task 2: Build-time mask-path validation against the descriptor and Phase 1 provenance (CRUD-05)** — `5e39973` (feat)
3. **Task 3: Request-time mask edges — empty, nested, wildcard, unknown, duplicate, and order independence** — `55490e2` (test)

**Plan metadata:** *(this commit, docs: complete plan)*

## Files Created/Modified

- `proto/entconnecttest/v1/update.proto` — `Patch`, `UpdatePatchRequest/Response`, `PatchUpdateService`, `PatchUpdateAltService` (maskcheck-test-only)
- `runtime/fieldmask.go` + `runtime/fieldmask_test.go` — `ValidateMask`, `ErrMaskEmpty`/`ErrMaskNested`/`ErrMaskUnknown`
- `entc/maskcheck.go` + `entc/maskcheck_test.go` — `ValidateMaskPaths`
- `entc/crud_update.go` + `entc/templates/update.tmpl` — the `OpUpdate` generator
- `internal/entconnecttest/update/ent/schema/patch.go` — the `Patch` schema (Exclude `id`/`internal_note`, a `Policy` denying `DeniedSubject` on mutation), plus the full generated `ent/` and `entconnect/` packages
- `internal/entconnecttest/update/update_test.go` — the real round trip and privacy-deny proof
- `internal/entconnecttest/update/mask_edges_test.go` — every request-time edge case
- `internal/entconnecttest/update/badmask/ent/schema/{badpatch,excludedpatch}.go` — negative-build fixtures

## Decisions Made

See `key-decisions` in frontmatter above.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] `entconnectruntime.ValidateMask`'s call in the generated Update body originally passed the whole request message, not the entity submessage**
- **Found during:** Task 1, first `go test` run against the fixture
- **Issue:** `entc/templates/update.tmpl` called `runtime.ValidateMask(req.Msg.GetUpdateMask(), req.Msg, allowed)` — `req.Msg` is `UpdatePatchRequest`, whose only top-level fields are `patch` and `update_mask`. Mask paths like `"title"` are never valid against that descriptor, so `mask.IsValid(msg)` failed for every legitimate request.
- **Fix:** Added a `PatchFieldGoName` template field so the call reads `runtime.ValidateMask(req.Msg.GetUpdateMask(), req.Msg.GetPatch(), allowed)` — validating paths against the entity submessage they actually describe.
- **Files modified:** `entc/templates/update.tmpl`, `entc/crud_update.go`
- **Verification:** `TestUpdate_MaskedFieldChangesUnmaskedFieldsSurvive` passes
- **Committed in:** `e78a9cd`

**2. [Rule 1 - Bug] `ValidateMask`'s descriptor-validity check could not name the specific offending path**
- **Found during:** Task 3, `TestMaskEdges_UnknownPath`
- **Issue:** `runtime/fieldmask.go` called `mask.IsValid(msg)` once over the whole `Paths` slice — `IsValid` reports only whether ALL paths are valid, never which one failed, so the rejection message could only name the message descriptor, not the offending path CRUD-05 requires.
- **Fix:** Check each path individually via a single-path `&fieldmaskpb.FieldMask{Paths: []string{p}}).IsValid(msg)`, so the first invalid path is named verbatim in the returned error.
- **Files modified:** `runtime/fieldmask.go`
- **Verification:** `TestMaskEdges_UnknownPath`, `TestMaskEdges_NestedPath`, `TestMaskEdges_WildcardPath` all assert the offending path appears verbatim; `go test -race -count=5` passes
- **Committed in:** `55490e2`

**3. [Rule 3 - Blocking] `UpdatePatchAlt` could not be a second RPC on the real `PatchUpdateService`**
- **Found during:** Task 1 (initial proto design), before any negative-build test was written
- **Issue:** A first draft added `UpdatePatchAlt` directly to `PatchUpdateService` so the future negative-build fixtures could bind a second procedure. This broke the real fixture's build: Connect's generated `<Service>Handler` interface requires every RPC on a service to have an implementing method, and `UpdatePatchAlt` is never bound by any real schema, so `patchUpdateServiceServer` failed to satisfy the interface.
- **Fix:** Moved `UpdatePatchAlt` to a standalone `PatchUpdateAltService`, used only by `entc/maskcheck_test.go`'s negative-build fixtures.
- **Files modified:** `proto/entconnecttest/v1/update.proto`
- **Verification:** `go build ./...` passes; `internal/entconnecttest/update/entconnect/` compiles
- **Committed in:** `e78a9cd`

---

**Total deviations:** 3 auto-fixed (2 bug, 1 blocking)
**Impact on plan:** All three were necessary for the generated code to compile and behave correctly, or for the negative-build test design to actually work. None expanded scope beyond FieldMask-gated Update.

## Issues Encountered

- **D-17's build-time exclusion check, once wired in during Task 2, makes the main fixture's own `internal_note` exclusion a build failure if `go generate` is ever re-run against it.** This is documented as an intentional design decision (see `key-decisions`), not a bug — Task 1's own comment on the `Exclude("internal_note")` call anticipated exactly this. Confirmed live: re-running `go generate ./internal/entconnecttest/update/...` after Task 2 landed fails deterministically (byte-identical error text across repeated runs), and `git status` shows no unintended file changes (the generator errors out before writing anything). The already-committed, already-tested generated file is untouched and is not regenerated again within this plan.

## User Setup Required

None — no external service configuration required.

## Next Phase Readiness

- CRUD-04 and CRUD-05 are complete; the Update generator is registered via `entc/crud_update.go`'s own `init()` call to `RegisterGenerator(OpUpdate, ...)`, with no edit to the shared `entc/crud.go` registry.
- `entc/extension.go`'s per-service failure short-circuiting (checked once per service, immediately after that service's generator-dispatch loop) means a future plan wanting to prove cross-service failure aggregation will need to either restructure that check or accept the same pure-function-level testing approach this plan used for `ValidateMaskPaths`'s "collected in one pass" claim.
- Deferred, as scoped: Tier 2/3 CEL passthrough (Phase 3), flow binding (Phase 4), the drift check itself (Phase 5). This plan's protovalidate interceptor stage remains the only enforcement for untranslated constraints, per the phase's own boundary note.

---
*Phase: 02-crud-handlers-interceptor-chain*
*Completed: 2026-08-09*

## Self-Check: PASSED

All 12 key created files verified present on disk; all 3 task commit hashes (`e78a9cd`, `5e39973`, `55490e2`) verified present in `git log --oneline --all`. No missing items.
