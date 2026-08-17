---
phase: 03-validation-fidelity
plan: 03
subsystem: database
tags: [ent, protobuf, protovalidate, cel-go, mixinforproto, hybrid-evaluator]

requires:
  - phase: 03-validation-fidelity
    provides: "03-01's residual-CEL half of the hook (hooks.go/hookState/celProgram), the single shared newValidationError constructor, and the empirical answer to Open Question 1 (mutation.Fields() on Create already includes an unset Default()-bearing field); 03-02's complete reverse.go conversion table and the MixedFieldRules corpus fixture (constraints.proto)"
provides:
  - "hooks.go's complete D-07 hybrid: a schema-load-precompiled protovalidate.Validator (WithMessages + WithDisableLazy) evaluating every standard rule on every in-scope field via a WithFilter scope, alongside the unchanged local cel.Env for custom (buf.validate.field).cel rules"
  - "violation.go's newValidationError gains deterministic (field-descriptor-index, then RuleId) ordering and (RuleId, FieldPath) deduplication — the settled resolution to 03-RESEARCH.md Pitfall 1's mixed-field double-evaluation hazard"
  - "D-06's operation-dependent scope implemented as a single m.Fields() read with no ent.Op() branch (inScopeFieldNames), proven correct on both Create (03-01) and Update (this plan's difftest tests) via a real generated ent.Client"
  - "A real, generated MixedFieldRules-backed ent schema and regenerated ent package under mixinforproto/internal/difftest, proving Pitfall 1's dedup resolution through ent's actual withHooks pipeline (PIPE-06's first instance)"
  - "fakeDriver.Exec extended to populate a *entsql.Result, unblocking real Update().Save() flows through the driverless harness for the first time"
affects: [03-04, 03-05]

actuals:
  tokens: 41000
  tasks: 3
  commits: 4

tech-stack:
  added: []
  patterns:
    - "protovalidate.WithMessageDescriptors-equivalent construction via protovalidate.WithMessages(dynamicpb.NewMessage(md)) — no caller-supplied M instance needed inside buildHookState, which only ever received a MessageDescriptor"
    - "A protoreflect.FilterFunc gating on `d == msg.Descriptor()` (message-level, VAL-08) and field-number membership in a per-mutation computed set (D-03/D-06), reused verbatim from 03-RESEARCH.md Pattern 2"
    - "protodesc.NewFile-constructed test descriptors for failure modes buf's own build pipeline makes unreachable through a real corpus .proto (message-level rules, uncompilable custom CEL) — the same 'hand-build past what buf would allow' technique validate_test.go's TestPatternCompiles established for string.pattern"
    - "A fakeMutation ent.Mutation double built by embedding the (nil) interface and overriding only Op/Type/Fields/Field, so any unimplemented method call panics loudly rather than silently returning a zero value — used to prove 'evaluate() calls no evaluator' empirically by nulling hs.validator"

key-files:
  created:
    - mixinforproto/hooks_test.go
    - mixinforproto/violation_test.go
    - mixinforproto/internal/difftest/ent/schema/mixedrules.go
    - mixinforproto/internal/difftest/hybrid_test.go
    - mixinforproto/internal/difftest/ent/mixedfieldrules.go
    - mixinforproto/internal/difftest/ent/mixedfieldrules/mixedfieldrules.go
    - mixinforproto/internal/difftest/ent/mixedfieldrules/where.go
    - mixinforproto/internal/difftest/ent/mixedfieldrules_create.go
    - mixinforproto/internal/difftest/ent/mixedfieldrules_delete.go
    - mixinforproto/internal/difftest/ent/mixedfieldrules_query.go
    - mixinforproto/internal/difftest/ent/mixedfieldrules_update.go
  modified:
    - mixinforproto/hooks.go
    - mixinforproto/violation.go
    - mixinforproto/internal/difftest/fakedriver.go
    - mixinforproto/internal/difftest/ent/client.go
    - mixinforproto/internal/difftest/ent/ent.go
    - mixinforproto/internal/difftest/ent/hook/hook.go
    - mixinforproto/internal/difftest/ent/migrate/schema.go
    - mixinforproto/internal/difftest/ent/mutation.go
    - mixinforproto/internal/difftest/ent/predicate/predicate.go
    - mixinforproto/internal/difftest/ent/runtime/runtime.go
    - mixinforproto/internal/difftest/ent/tx.go

key-decisions:
  - "Pitfall 1 resolved as resolution (a) — accept double evaluation, deduplicate by (RuleId, FieldPath) at violation.go's newValidationError — not resolution (b) (excluding a mixed field from the standard-rule Filter and routing its whole rule set through the local env). Evidence: (a) needed zero new structural-rule code path (it reuses protovalidate's own evaluator unconditionally for every rule-bearing field, per D-08) and passed every mixed-field test on the first attempt; (b) would additionally have required proving a structural rule (string.min_len) evaluated exclusively through the local pvcel-constructed env is byte-identical to protovalidate's own evaluator's — a real, separate proof this plan did not need to build."
  - "D-06's mechanism collapsed to a single m.Fields() read with no ent.Op() branch, per 03-01's recorded empirical evidence (TestUnsetDefaultFieldAppearsInMutationFieldsOnCreate: mutation.Fields() on Create already includes an unset Default()-bearing field because ent's own defaults() runs before any hook). This plan's Task 3 precondition required reading that recorded answer before writing inScopeFieldNames, and did so — the two-path (Op()-branching) design was NOT built."
  - "hookFieldClass deliberately stays scoped to classScalar/classOptionalScalar, not extended to enum/wkt/scalarMap/asJSON even though reverse.go (03-02) can already reverse-convert all of them. None of this plan's acceptance criteria or corpus fixtures exercise those classes carrying a protovalidate rule, and extending the routing table without dedicated tests risked silent correctness gaps. Recorded as a known, deliberate scope boundary — fields of those classes remain boundary-only via 03-02's recordBoundaryOnly until a future plan (03-05's PIPE-06 differential sweep is the natural place this would first surface) adds the coverage."
  - "fakeDriver.Exec (internal/difftest/fakedriver.go, outside this plan's declared file scope) extended to populate a *entsql.Result when called — Rule 3 (auto-fix blocking issue). Task 3's real-Update() tests panicked on a nil-interface RowsAffected() call inside dialect/sql/sqlgraph's updateTable, since 03-01/03-02's Create-only harness never exercised the Exec/Result path (Create's insertLastID goes through Query on SQLite). Not owned by plan 03-04's declared scope."
  - "The message-level-rule test fixture (TestEvaluate_MessageLevelRulesStayBoundaryOnlyByDefault) and the uncompilable-CEL aggregation fixture are built via protodesc.NewFile at test time rather than a new corpus .proto: buf's own lint would accept an arbitrary (unchecked) custom CEL expression string, but adding either fixture to proto/mixinforprototest/v1/*.proto would require regenerating mixinforproto/internal/gen — a path outside this plan's declared scope and inside no other plan's either, so the protodesc route avoided both false scope expansion and an unregenerated-stub staleness risk."

patterns-established:
  - "In-scope field computation (inScopeFieldNames) and the standard-rule Filter (built inline in evaluate()) both key off protoreflect.FieldNumber, not field name, once inside the reverse-converted/reconstructed dynamicpb message — field NAME is only used for the m.Field(name) lookup against the ent.Mutation itself."

requirements-completed: [VAL-04, VAL-05, VAL-07, VAL-08, PIPE-06]

coverage:
  - id: D1
    description: "Every standard protovalidate rule on an in-scope field (translated and residual alike) is evaluated by protovalidate's own evaluator through a once-built Validator and a per-call field-scoped Filter; message-level rules stay boundary-only by default"
    requirement: "VAL-04, VAL-07, VAL-08"
    verification:
      - kind: unit
        ref: "mixinforproto/hooks_test.go#TestEvaluate_StandardRule_StringMaxLenCarriesProtovalidateRuleID"
        status: pass
      - kind: unit
        ref: "mixinforproto/hooks_test.go#TestEvaluate_StandardRule_FloatGtMatchesDirectProtovalidateAtBoundary"
        status: pass
      - kind: unit
        ref: "mixinforproto/hooks_test.go#TestEvaluate_StandardRule_RequiredOnNonPresenceMatchesDirectProtovalidate"
        status: pass
      - kind: unit
        ref: "mixinforproto/hooks_test.go#TestEvaluate_MessageLevelRulesStayBoundaryOnlyByDefault"
        status: pass
      - kind: unit
        ref: "mixinforproto/hooks_test.go#TestEvaluate_FieldMaskUpdate_UntouchedRequiredSiblingProducesNoViolation"
        status: pass
      - kind: unit
        ref: "mixinforproto/hooks_test.go#TestBuildHookState_StandardValidatorBuiltOnceAtConstruction"
        status: pass
      - kind: unit
        ref: "mixinforproto/hooks_test.go#TestEvaluate_EmptyInScopeCallsNoEvaluator"
        status: pass
    human_judgment: false
  - id: D2
    description: "A field carrying both a standard rule and a custom CEL rule yields exactly the boundary's violation set — no doubles, no divergent message text — with the resolution and evidence recorded"
    requirement: "VAL-07, PIPE-06"
    verification:
      - kind: unit
        ref: "mixinforproto/violation_test.go#TestMixedField_ViolatesOnlyCEL"
        status: pass
      - kind: unit
        ref: "mixinforproto/violation_test.go#TestMixedField_ViolatesOnlyStandard"
        status: pass
      - kind: unit
        ref: "mixinforproto/violation_test.go#TestMixedField_ViolatesBoth"
        status: pass
      - kind: unit
        ref: "mixinforproto/violation_test.go#TestMixedField_MatchesDirectProtovalidateOnEntity"
        status: pass
      - kind: unit
        ref: "mixinforproto/violation_test.go#TestMixedField_StableOrderAcrossRuns"
        status: pass
      - kind: unit
        ref: "mixinforproto/violation_test.go#TestMixedField_SiblingsUnaffected"
        status: pass
      - kind: unit
        ref: "mixinforproto/internal/difftest/hybrid_test.go#TestMixedFieldRules_ViolatesOnlyCEL_RealCreate"
        status: pass
      - kind: unit
        ref: "mixinforproto/internal/difftest/hybrid_test.go#TestMixedFieldRules_ViolatesOnlyStandard_RealCreate"
        status: pass
      - kind: unit
        ref: "mixinforproto/internal/difftest/hybrid_test.go#TestMixedFieldRules_ViolatesBoth_RealCreate"
        status: pass
      - kind: unit
        ref: "mixinforproto/internal/difftest/hybrid_test.go#TestMixedFieldRules_SiblingsUnaffected_RealCreate"
        status: pass
    human_judgment: false
  - id: D3
    description: "D-06's scope matches specification on both operations (Create: all derived fields; Update: changed-only), with Update's no-ent-side-safety-net case independently tested against a real ent.Client"
    requirement: "VAL-05"
    verification:
      - kind: unit
        ref: "mixinforproto/internal/difftest/hybrid_test.go#TestUpdate_UntouchedDerivedFieldIsNotEvaluated"
        status: pass
      - kind: unit
        ref: "mixinforproto/internal/difftest/hybrid_test.go#TestUpdate_TouchedDerivedFieldIsEvaluated"
        status: pass
      - kind: unit
        ref: "mixinforproto/internal/difftest/hybrid_test.go#TestUpdate_TouchedDerivedFieldAcceptsValidValue"
        status: pass
    human_judgment: false
  - id: D4
    description: "An uncompilable CEL expression fails at schema load through the existing collected-failure machinery; multiple offenders reported in one pass, first line self-sufficient and asserted verbatim"
    requirement: "VAL-04"
    verification:
      - kind: unit
        ref: "mixinforproto/hooks_test.go#TestCompileCELRule_UncompilableExpressionIsAnError"
        status: pass
      - kind: unit
        ref: "mixinforproto/hooks_test.go#TestBuildHookState_UncompilableCELExpressionsAreCollectedInOnePass"
        status: pass
    human_judgment: false
  - id: D5
    description: "go test ./... -count=5 -race, GOWORK=off go test ./..., make build/vet/test-determinism/check-modules/check-goversion all pass for both modules; violation.go is the sole *protovalidate.ValidationError{ construction site"
    verification:
      - kind: other
        ref: "cd mixinforproto && go test ./... -count=5 -race; GOWORK=off go test ./...; make build && make vet && make test-determinism && make check-modules && make check-goversion (repo root); grep -rn 'protovalidate.ValidationError{' mixinforproto --include=*.go | grep -v _test.go returns exactly mixinforproto/violation.go"
        status: pass
    human_judgment: false

duration: 62min
completed: 2026-08-14
status: complete
---

# Phase 3 Plan 3: The Complete D-07 Hybrid Summary

**mixinforproto's `Hooks()` completes D-07's hybrid — protovalidate's own precompiled evaluator now enforces every standard rule on every in-scope field (translated and residual alike), a `(RuleId, FieldPath)` dedup at `newValidationError` settles 03-RESEARCH.md Pitfall 1's mixed-field double-evaluation hazard, and D-06's operation-dependent scope collapses to a single `m.Fields()` read proven correct on both Create and Update through a real generated `ent.Client`.**

## Performance

- **Duration:** 62 min
- **Started:** 2026-08-14T16:03:59Z
- **Completed:** 2026-08-14T16:35:39Z (checkpoint policy: autonomous, no checkpoints hit)
- **Tasks:** 3 (all `type="auto" tdd="true"`)
- **Files modified:** 22 (11 created, 11 modified)

## Accomplishments

- `hooks.go`'s `buildHookState` now precompiles one `protovalidate.Validator` per message (`protovalidate.New(WithMessages(dynamicpb.NewMessage(md)), WithDisableLazy())`, built once, outside the returned hook closure) whenever at least one field carries any protovalidate rule — standard or custom CEL alike. `evaluate()` reverse-converts every in-scope, rule-bearing field once, hands the reconstructed `dynamicpb.Message` to that Validator through a `WithFilter` scope restricted to exactly the in-scope field set (never the message descriptor itself, keeping message-level rules boundary-only per VAL-08), and — for fields carrying a custom `(buf.validate.field).cel` rule — also evaluates the unchanged local `cel.Env` half from 03-01.
- Settled 03-RESEARCH.md Pitfall 1 (the D-07/D-08 hybrid-evaluator convergence risk) as resolution (a): a mixed field's custom CEL rule is deliberately evaluated by both engines, and `violation.go`'s `newValidationError` deduplicates by `(RuleId, FieldPath)` — the same identity key both engines independently produce from the contract's own rule `id` and the field descriptor — before imposing a deterministic (field-descriptor-index, then RuleId) order and returning the merged `*protovalidate.ValidationError`.
- `celResultToViolation`'s bool-false fallback message now quotes the CEL **expression** (`"<expression>" returned false`), matching `buf.build/go/protovalidate@v1.2.0/program.go`'s own fallback text exactly — the prior 03-01 implementation quoted the rule ID instead, which would have produced divergent message text for a mixed field's doubled-then-deduped violation.
- D-06's operation-dependent scope (`inScopeFieldNames`) is a single `m.Fields()` read with no `ent.Op()` branch, per 03-01's recorded empirical evidence — this plan's Task 3 `<precondition>` required reading that evidence before implementing, and did so; the two-path design was not built.
- Added a `MixedFieldRules`-backed ent schema to `mixinforproto/internal/difftest/ent/schema` and regenerated the fixture's committed `ent` package, then drove all three mixed-field cases (CEL-only, standard-only, both) through a real `Create().Save(ctx)` — PIPE-06's first instance, proving the Pitfall 1 resolution through ent's actual `withHooks` pipeline, not just a hand-built `ent.Mutation` double.
- Independently proved D-06's Update-side "no ent-side safety net" (Pitfall 4) against a real `ent.Client`: an untouched derived field on `ResidualCel.Update()` is never evaluated (even though it carries a live CEL rule), while a field the caller does set is evaluated exactly as on Create.
- Both schema-load CEL failure modes from D-09 — a single uncompilable custom CEL expression, and two uncompilable expressions on two different fields reported in one collected-failure pass — are now directly tested, using `protodesc`-built descriptors for the fixtures no real corpus `.proto` can express (buf's lint does not statically check custom CEL syntax, so an "uncompilable CEL" `.proto` fixture would in fact compile fine at `buf build` time and only fail later at `cel.Env.Compile` — exactly the schema-load moment D-09 targets, which is why the test must construct the descriptor directly rather than relying on buf to reject it).

## Task Commits

Each task was committed atomically, though hooks.go/violation.go's core implementation is a single unit spanning all three tasks (see Deviations):

1. **Tasks 1+2+3 core (production code)** — `c37f8de` (feat): `hooks.go` gains the standard-rule evaluator, WithFilter scope, and D-06 single-read mechanism; `violation.go` gains dedup + deterministic ordering.
2. **Task 1+3 (unit tests)** — `bb79616` (test): `hooks_test.go` — standard-rule evaluator behavior, scope, validator-build-once, and both uncompilable-CEL schema-load tests.
3. **Task 2 (unit tests)** — `0a47efc` (test): `violation_test.go` — mixed-field violation identity/ordering.
4. **Tasks 2+3 (end-to-end)** — `d9e3f0f` (feat): `difftest/ent/schema/mixedrules.go` + regenerated `ent` package, `fakedriver.go`'s Exec/Result extension, and `hybrid_test.go`'s mixed-field and Create/Update scope proofs.

**Plan metadata:** _(recorded in this commit's own final metadata commit)_

_Note: this plan's tasks all carried `tdd="true"`, but the RED/GREEN split is not visible in the commit list above — see Deviations for why the implementation was built and committed as fewer, larger units than one commit per task._

## Files Created/Modified

- `mixinforproto/hooks.go` — the complete D-07 hybrid: `buildHookState` (Validator + CEL program compilation), `evaluate()` (reverse-conversion, dynamicpb reconstruction, WithFilter scope, both evaluation halves), `inScopeFieldNames` (D-06's single-read scope), `celResultToViolation` (fixed message-text fallback)
- `mixinforproto/violation.go` — `newValidationError` gains `dedupeViolations`/`sortViolations`/`violationKey`/`violationFieldIndex`
- `mixinforproto/hooks_test.go` — Task 1 + Task 3 unit tests, including the `fakeMutation` `ent.Mutation` double and `protodesc`-based descriptor builders
- `mixinforproto/violation_test.go` — Task 2 mixed-field identity/ordering tests
- `mixinforproto/internal/difftest/ent/schema/mixedrules.go` — the `MixedFieldRules` ent schema
- `mixinforproto/internal/difftest/ent/mixedfieldrules*.go`, `mixedfieldrules/` — regenerated committed `ent` package (entc codegen output)
- `mixinforproto/internal/difftest/fakedriver.go` — `Exec` now populates `*entsql.Result` (`fakeResult`)
- `mixinforproto/internal/difftest/hybrid_test.go` — mixed-field Create proofs + Create/Update scope proofs
- Regenerated shared difftest `ent` package files (`client.go`, `ent.go`, `hook/hook.go`, `migrate/schema.go`, `mutation.go`, `predicate/predicate.go`, `runtime/runtime.go`, `tx.go`) — entc's whole-schema-directory regeneration, not hand-edited

## Decisions Made

See `key-decisions` in frontmatter for the full list. Highlights:
- Pitfall 1 resolved as resolution (a) — dedup, not field-exclusive routing — with the reasoning and the rejected alternative recorded.
- D-06's mechanism is a single `m.Fields()` read, per 03-01's recorded evidence, not a new two-path design.
- `hookFieldClass` deliberately NOT extended beyond scalar/optionalScalar in this plan — recorded as a known, tested-absence scope boundary rather than silently expanded without dedicated coverage.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking issue] `fakeDriver.Exec` needed to populate a `*entsql.Result` for real `Update()` flows to work at all**
- **Found during:** Task 3's `TestUpdate_TouchedDerivedFieldAcceptsValidValue`, first run
- **Issue:** `dialect/sql/sqlgraph`'s `updateTable` unconditionally calls `res.RowsAffected()` on whatever `Exec` populated; 03-01/03-02's harness only ever exercised Create (which reaches the driver through `Query`, not `Exec`, on a non-MySQL dialect), so `Exec` had never been asked to populate a real result and its prior no-op implementation left `res` as a nil `sql.Result` interface — `.RowsAffected()` on that panics (nil-interface method call), not merely returns a wrong count.
- **Fix:** `Exec` now type-asserts `v` to `*entsql.Result` and, when present, sets it to a `fakeResult{rowsAffected: 1}` (every mutation this harness drives targets exactly one entity by primary key).
- **Files modified:** `mixinforproto/internal/difftest/fakedriver.go` (not in this plan's declared file scope, but not owned by plan 03-04's declared scope either)
- **Committed in:** `d9e3f0f` (Tasks 2+3 end-to-end commit)

### Scope/process notes (not corrections, documented for traceability)

**2. `hooks.go`/`violation.go`'s core implementation spans all three tasks in a single commit, not three**

Task 1 (standard-rule evaluator + WithFilter), Task 2 (mixed-field dedup resolution), and Task 3 (D-06's scope mechanism + D-09's CEL panic — the latter already implemented by 03-01) all modify the same handful of functions in the same two files, and `hooks.go`'s `hook()` closure could not compile against an intermediate `violation.go` signature that only some of the three tasks' work would produce. Splitting the implementation into three genuinely separable, independently-buildable commits would have meant writing and then partially reverting/re-adding functionality purely for commit-history granularity, with real risk of introducing bugs across the extra edit passes. The four commits that were made instead split cleanly along file-ownership and buildability lines (core implementation; Task 1+3 unit tests; Task 2 unit tests; Tasks 2+3 end-to-end proof), each commit message stating which task(s) it serves. This is a process deviation from "one commit per task," not a functional one — every task's acceptance criteria are independently verified by tests in the commit history above.

**3. `mixinforproto/internal/gen/mixinforprototestv1`'s `MixedFieldRules` type was already generated by 03-02** — no proto regeneration was needed in this plan; only the `mixinforproto/internal/difftest/ent` package (a separate, unrelated generated artifact — the driverless harness's own `ent.Client`) was regenerated, via its existing `//go:generate go run entc.go` entry point, exactly as the plan's action text specifies.

**Total deviations:** 1 auto-fixed blocking issue (outside declared scope, not owned by 03-04, necessary for Task 3's own acceptance criteria), plus 2 documented process/traceability notes. No scope creep beyond what Task 3's own acceptance criteria required.

## Issues Encountered

- `mixinforproto/hooks_test.go`'s `TestEvaluate_StandardRule_FloatGtMatchesDirectProtovalidateAtBoundary` initially failed: comparing the hook's per-field verdict against `protovalidate.Validate` on an entity with only `GtField` set produced a false mismatch, because `FloatComparators`' sibling `GteLteField` field's zero value independently violates its own `gte: 1.5` bound — a test-construction bug (isolating the comparison requires setting every OTHER field to a value valid under its own rules), not a hooks.go defect. Fixed by setting `LtField`/`GteLteField` to values that satisfy their own constraints in the direct-comparison entity.
- `buf` is not on `PATH` in this session's environment (unlike 03-02's session, which installed it via `go install`). This plan made no `.proto` changes and touched no `proto/` or `mixinforproto/internal/gen` files (confirmed via `git status --short`), so `make check-stubs` was not run — it has nothing to regenerate against this plan's diff. Recorded here rather than silently skipped.
- `make test-determinism` (both modules, `go test -count=5 ./...`) took roughly 3 minutes wall-clock in this sandbox due to `internal/boundarytest`'s repeated real `entc.LoadGraph` subprocess invocations (a pre-existing cost, not something this plan changed) — ran to completion in the background rather than under the default 2-minute foreground timeout.

## User Setup Required

None — no external service configuration required.

## Next Phase Readiness

- Plan 03-04 (Wave 3, parallel to this plan) owns `proto/entconnecttest/v1/hookwiring.proto`, `internal/gen/entconnecttestv1/**`, `internal/entconnecttest/hookwiring/**`, `runtime/interceptor_test.go`, `mixinforproto/README.md`, `Makefile`, and `.github/workflows/ci.yml` — none of those paths were touched by this plan.
- `mixinforproto/hooks.go`'s standard-rule evaluator is scoped to `classScalar`/`classOptionalScalar` fields only; extending `hookFieldClass` to `enum`/`wkt`/`scalarMap`/`asJSON` (all reverse-convertible since 03-02) is explicitly deferred, with the current behavior (those classes stay boundary-only) tested-as-absent rather than silently assumed. A future plan — most naturally 03-05's PIPE-06 differential sweep, which is designed to walk the whole corpus — is where this gap would first become visible and is the right place to close it.
- The differential-harness pattern this plan established (a real generated `ent.Schema` + regenerated committed `ent` package under `internal/difftest`, driven by `fakeDriver`) is now proven for both Create and Update; `fakeDriver`'s `Exec`/`Result` support is general (not `MixedFieldRules`- or `ResidualCel`-specific), so a future plan adding more schemas to this harness inherits Update coverage for free.
- `mixinforproto` still declares `go 1.24.0` (`make check-goversion` verified) and still has no DB driver in its `go.mod` (`make check-modules` verified, and `internal/difftest` gained no new imports beyond what was already there).

---
*Phase: 03-validation-fidelity*
*Completed: 2026-08-14*

## Self-Check: PASSED

All created/modified files (mixinforproto/hooks.go, mixinforproto/violation.go,
mixinforproto/hooks_test.go, mixinforproto/violation_test.go,
mixinforproto/internal/difftest/ent/schema/mixedrules.go,
mixinforproto/internal/difftest/hybrid_test.go,
mixinforproto/internal/difftest/fakedriver.go, this SUMMARY.md) and all four
task commits (c37f8de, bb79616, 0a47efc, d9e3f0f) verified present on disk /
in git log.
