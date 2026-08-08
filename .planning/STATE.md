---
gsd_state_version: 1.0
milestone: v1.0
milestone_name: milestone
current_phase: 1
current_phase_name: MixinForProto Core
status: executing
stopped_at: Completed 01-03-PLAN.md
last_updated: "2026-08-08T10:53:56.547Z"
last_activity: 2026-08-08
last_activity_desc: Roadmap created from 62 v1 requirements across 5 phases
progress:
  total_phases: 1
  completed_phases: 0
  total_plans: 5
  completed_plans: 3
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-08-08)

**Core value:** API-visible fields and their validation are defined exactly once — in the protobuf contract — and enforced identically at the transport boundary and at the storage layer, so contract/schema drift is a build failure rather than a runtime surprise.
**Current focus:** Phase 1 — MixinForProto Core

## Current Position

Phase: 1 (MixinForProto Core) — EXECUTING
Plan: 4 of 5
Status: Ready to execute
Last activity: 2026-08-08 — Phase 1 execution started

Progress: [██████░░░░] 60%

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

### Pending Todos

None yet.

### Blockers/Concerns

- Phase 4 (Flow Binding) is gated on entflow having real, running sync-flow code — not just a design doc. Do not plan past interface sketching until that exists.
- Annotation struct versioning/forward-compatibility strategy (ANNO-04) needs concrete design during Phase 1 planning — no direct precedent.
- Sensitive-field marking (`field.Sensitive()` inference) is an unresolved open question with security implications, deferred to v2 (MIX2-02) but worth a decision checkpoint by end of Phase 1.

## Deferred Items

Items acknowledged and carried forward from previous milestone close:

| Category | Item | Status | Deferred At |
|----------|------|--------|-------------|
| *(none)* | | | |

## Session Continuity

Last session: 2026-08-08T10:53:56.535Z
Stopped at: Completed 01-03-PLAN.md
Resume file: None
