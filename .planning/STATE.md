---
gsd_state_version: 1.0
milestone: v1.0
milestone_name: milestone
current_phase: 03
current_phase_name: validation-fidelity
status: executing
stopped_at: Completed 03-05-PLAN.md (final plan of phase 03-validation-fidelity)
last_updated: "2026-08-15T08:59:41.447Z"
last_activity: 2026-08-14
last_activity_desc: Phase 02 marked complete
progress:
  total_phases: 3
  completed_phases: 3
  total_plans: 22
  completed_plans: 19
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-08-08)

**Core value:** API-visible fields and their validation are defined exactly once — in the protobuf contract — and enforced identically at the transport boundary and at the storage layer, so contract/schema drift is a build failure rather than a runtime surprise.
**Current focus:** Phase 03 — validation-fidelity

## Current Position

Phase: 03 (validation-fidelity) — EXECUTING
Plan: 5 of 8
Status: Ready to execute
Last activity: 2026-08-15 — Phase 03 gap-closure plans 03-06..03-08 created

Progress: [████████▒▒] 86%

## Performance Metrics

**Velocity:**

- Total plans completed: 0
- Average duration: - min
- Total execution time: 0 hours

**By Phase:**

| Phase | Plans | Total | Avg/Plan |
|-------|-------|-------|----------|
| - | - | - | - |

**Recent Trend:**

- Last 5 plans: -
- Trend: -

*Updated after each plan completion*
**Per-Plan Metrics:**

| Plan | Duration | Tasks | Files |
|------|----------|-------|-------|
| Phase 01 P01 | 55min | 3 tasks | 16 files |
| Phase 01 P02 | 16min | 2 tasks | 32 files |
| Phase 01 P03 | 10min | 2 tasks | 8 files |
| Phase 01 P04 | 45min | 3 tasks | 12 files |
| Phase 01 P05 | 44min | 3 tasks | 42 files |
| Phase 01 P06 | 30min | 3 tasks | 6 files |
| Phase 01 P07 | 55min | 3 tasks | 5 files |
| Phase 01 P08 | 25min | 3 tasks | 12 files |
| Phase 01 P09 | 15min | 3 tasks | 2 files |
| Phase 03 P01 | 55min | 3 tasks | 33 files |
| Phase 03 P02 | 50min | 3 tasks | 22 files |
| Phase 03 P03 | 62min | 3 tasks | 22 files |
| Phase 03-validation-fidelity P04 | 34min | 3 tasks | 48 files |
| Phase 03 P05 | 51min | 3 tasks | 186 files |

## Accumulated Context

### Decisions

Decisions are logged in PROJECT.md Key Decisions table.
Recent decisions affecting current work:

- Roadmap: Annotation contract (ANNO-*) ships in Phase 1 alongside mixin field derivation — closures do not survive entc's schema-load JSON boundary, so provenance must be written as annotations from the start
- Roadmap: FieldMask-gated Update (CRUD-04/05) ships in the same phase as Update RPC codegen (Phase 2) — the proto3 zero-collapse pitfall bites exactly there
- Roadmap: Flow binding (Phase 4) sequenced after CRUD and validation phases since it depends on the external, not-yet-built entflow project
- Roadmap: Full drift check (Phase 5) is a pure function of Phases 1-4 outputs, sequenced last
- [Phase ?]: Task 1 checkpoint: module path github.com/smintz/entconnect/mixinforproto, nested module, mixinforproto/vX.Y.Z tag convention (no tag pushed)
- [Phase ?]: Task 2 checkpoint: integer ContractVersion=1 on SourceMessage/SourceField, additive-only versioning
- [Phase ?]: SourceField.Name renamed to FieldName (Go identifier only, JSON key stays 'name') to resolve a field/method name collision with schema.Annotation.Name()
- [Phase ?]: AsJSON validates against full message field set post-loop (validateAsJSON), not per-field inside mapField
- [Phase ?]: google.protobuf.Value maps to field.JSON(json.RawMessage) for lossless round-tripping; Struct maps to field.JSON(map[string]any)
- [Phase ?]: SourceField.Kind now carries derivation-class strings (scalar/optionalScalar/enum/wkt/scalarMap/asJSON), superseding Plan 01's raw protoreflect.Kind string
- [Phase ?]: Phase 01 Plan 03: Descriptor-set output path proto/mixinforprototest.binpb, sibling to proto/buf.yaml
- [Phase ?]: Phase 01 Plan 03: Makefile vet/test targets probe go list ./... first and skip visibly on zero-package modules, since go vet/go test (unlike go build) exit 1 on an empty module
- [Phase ?]: Phase 01 Plan 03: mixinforproto/README.md usage example omits Exclude/Override (not shipped until 01-04) despite mixinforproto.md's own example using them
- [Phase ?]: Phase 01 Plan 03: CI's pipeline-script invocation added to the modules job (Rule 2 follow-up) so CI actually runs scripts/pipeline.sh, not just its individual steps
- [Phase ?]: Phase 01 Plan 04: failure struct + derivationError aggregate replaces ad-hoc []string error collection; every failure path (option validation, fieldmap errors, reserved collisions, oneof gate) now produces the same self-sufficient-first-line, deterministically-sorted shape
- [Phase ?]: Phase 01 Plan 04: reservedStructural ships as exactly {id, label} (R3) — type/edge/where deliberately not added, unverified against ent's source; documented as an open follow-up
- [Phase ?]: Phase 01 Plan 04: reserved-identifier check runs only on fields that survive mapField; Override installs verbatim before mapField/classify runs, remaining the sole escape hatch for both reserved collisions and unresolved oneof members
- [Phase ?]: Phase 01 Plan 05: Task 1 checkpoint resolved by user — string.min_len/max_len/len map directly onto ent's byte-comparing MinLen/MaxLen (option-c); non-ASCII divergence documented in README and recorded machine-visibly via SourceField.LengthUnitDivergentIDs
- [Phase ?]: Phase 01 Plan 05: VAL-03 required/presence matrix — string/bytes always NotEmpty(); optional-keyword non-string -> non-optional construction (exact); plain non-string -> residual (no exact translation exists)
- [Phase ?]: Phase 01 Plan 05: unsigned integer intervals and bytes.* length/pattern constraints recorded residual, not translated — documented scope boundary, logged to WINDOWS.md
- [Phase ?]: Phase 01 Plan 05: buf/validate/validate.proto vendored locally at proto/buf/validate/ (BSR unreachable from this sandboxed environment); buf.gen.yaml override scoped to mixinforprototest/** to avoid colliding with the vendored file's own published Go package
- [Phase ?]: Phase 01 Plan 06 (gap closure): classify() now checks fd.IsList() immediately after fd.IsMap() (map-before-list ordering is load-bearing); repeated message fields alone are exempted via mapRepeated delegating to mapMessageField, preserving MIX-09/MIX-10; a real list-typed ent mapping is deferred and recorded as a new open Broken Window rather than built
- [Phase ?]: Rebuilt the D-22 staleness gate on an empty-output-tree design (copy working tree proto/ into a fresh temp root, no pre-seeded committed stubs) instead of git archive HEAD, closing WR-03's orphaned-file blind spot and making the gate rehearsable locally against uncommitted proto/ edits.
- [Phase ?]: Collapsed the two independently-drifting buf generate spellings (pipeline.sh's scoped call, CI's unscoped call) into scripts/generate-stubs.sh, the single canonical invocation both the pipeline and the CI staleness gate now delegate to.
- [Phase ?]: Phase 01 Plan 08 (gap closure): classifyRequired's presence-first branch order now applies to every kind including string/bytes — required on a presence-tracking (optional) field is always a presence assertion, never a non-emptiness one.
- [Phase ?]: Phase 01 Plan 08 (gap closure): delegatingFormatValidator filters protovalidate's whole-message violations down to the candidate field via FieldDescriptor.FullName comparison, closing the multi-field-message false-rejection defect while keeping the anti-bypass guarantee (an actually-invalid value on the candidate field is still rejected).
- [Phase ?]: Phase 01 Plan 08 (gap closure): regenerated the stale proto/mixinforprototest.binpb descriptor set and closed Broken Window 4 via gsd-tools windows fixed 4.
- [Phase ?]: TestCorpusExercisesEveryRequiredResult additionally requires requiredExactPresence to be witnessed separately by a hasNotEmpty=true (string/bytes) and hasNotEmpty=false field — plain per-outcome existence stays green under gap 3's original branch order
- [Phase ?]: corpusMessages() filters IsMapEntry() synthetic messages; Nested/Inner (reference-only proto types never derived directly) get test:TestGolden coverage claims rather than a standalone golden
- [Phase ?]: Phase 03 Plan 01: mixinforproto's Hooks() implements only the residual-CEL half of D-07's hybrid evaluator this plan; the standard-rule half (D-02) is deferred to a later plan in the phase, per the tracer's thinnest-possible-slice mandate
- [Phase ?]: Phase 03 Plan 01: Open Question 1 resolved empirically — ent's generated defaults() calls a Default()-bearing field's setter before any hook runs on Create, so mutation.Fields() already contains it; D-06's Create-side scope needs no Op() branch
- [Phase ?]: Phase 03 Plan 01: cel-go's correct import path/version is github.com/google/cel-go v0.28.0 (not cel-expr/cel-go@v0.31.0) — CLAUDE.md, REQUIREMENTS.md VAL-05, and ROADMAP.md SC1 amended accordingly (D-16/D-06/D-02)
- [Phase ?]: Phase 03 Plan 02: google.protobuf.Value's protojson round trip works against the bare Value descriptor with no containing-message context — RESEARCH Assumption A3 resolved TRUE, closing the phase's second named risk concentration
- [Phase ?]: Phase 03 Plan 02: an absent/unset ent value for optionalScalar, asJSON, wkt-Value, and wkt-Struct reports as an invalid protoreflect.Value (nil error), never a fabricated proto3 zero or empty message (D-03 phantom-violation class closed at the reverse-conversion layer)
- [Phase ?]: Phase 03 Plan 02: scalarMap wire-level determinism requires proto.MarshalOptions{Deterministic: true} at marshal time — protobuf-go's generic Map deliberately randomizes Range/marshal iteration order (internal/detrand) regardless of insertion order
- [Phase ?]: Phase 03 Plan 02: SourceMessage.BoundaryOnly (D-09's second half) records every protovalidate rule mixinforproto cannot enforce at the storage layer with a closed reason set (excluded/overridden/no-ent-field/unbindable), message-scoped so no pre-existing per-field golden fixture is touched
- [Phase ?]: Phase 03 Plan 03: 03-RESEARCH.md Pitfall 1 (D-07/D-08 hybrid-evaluator convergence) resolved as resolution (a) — accept double evaluation of a mixed field's custom CEL rule, deduplicate by (RuleId, FieldPath) at violation.go's newValidationError — not resolution (b) (field-exclusive routing), since (a) needed no new structural-rule code path and (b) would have required a separate byte-identical-to-protovalidate proof this plan did not need
- [Phase ?]: Phase 03 Plan 03: D-06's operation-dependent scope mechanism collapsed to a single m.Fields() read with no ent.Op() branch, per 03-01's recorded empirical evidence (Create's defaults() runs before any hook); proven correct on Update too via a real ent.Client (Pitfall 4's no-safety-net case)
- [Phase ?]: Phase 03 Plan 03: hooks.go's standard-rule evaluator deliberately stays scoped to classScalar/classOptionalScalar fields (not extended to enum/wkt/scalarMap/asJSON despite reverse.go already supporting them) — a documented, tested-absent scope boundary for a later plan to close
- [Phase ?]: Phase 03 Plan 04: VAL-09's ordering test asserts a relative marker sequence, never an index into ent's Hooks[] slice — Policed/Unpoliced fixture pair exists precisely to cover both privacy-wrapper-present and privacy-wrapper-absent configurations
- [Phase ?]: Phase 03 Plan 04: check-dep-parity's module set is exactly D-15's five names, resolved independently per module via GOWORK=off go list -m; both pass and fail directions rehearsed in scratch copies, never the real tree
- [Phase ?]: D-10's schema-load field-reference walk lives in buildHookState (hooks.go), not derive.go — reuses the existing failures/newDerivationError collected pattern
- [Phase ?]: PIPE-05's constraint-class guard ground truth is scoped to rule categories actually populated in the live corpus (reflectively computed), not protovalidate's full abstract surface
- [Phase ?]: PIPE-06's differential sweep drives a real ent.Client generically via reflection across 21 corpus messages (36 more covered-with-zero-cases), since internal/difftest cannot reach mixinforproto's unexported hookState.evaluate()

### Pending Todos

None yet.

### Blockers/Concerns

- Phase 4 (Flow Binding) is gated on entflow having real, running sync-flow code — not just a design doc. Do not plan past interface sketching until that exists.
- Annotation struct versioning/forward-compatibility strategy (ANNO-04) needs concrete design during Phase 1 planning — no direct precedent.
- Sensitive-field marking (`field.Sensitive()` inference) is an unresolved open question with security implications, deferred to v2 (MIX2-02) but worth a decision checkpoint by end of Phase 1.
- mixinforproto.md §4.1's Tier 1 table needs correcting for string length units (code points vs bytes) — deferred to Phase 1->2 transition per Task 1's resolution
- mixinforproto v0.1.0 (root go.mod's pinned dependency) predates Hooks() — make test-standalone-root fails on the new hookwiring fixture under GOWORK=off; D-19 follow-up needed: push a new mixinforproto tag and bump root go.mod in a separate commit

## Deferred Items

Items acknowledged and carried forward from previous milestone close:

| Category | Item | Status | Deferred At |
|----------|------|--------|-------------|
| *(none)* | | | |

## Session Continuity

Last session: 2026-08-14T18:05:34.237Z
Stopped at: Completed 03-05-PLAN.md (final plan of phase 03-validation-fidelity)
Resume file: None
