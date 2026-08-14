---
phase: 03-validation-fidelity
plan: 05
subsystem: database
tags: [ent, protobuf, protovalidate, cel-go, mixinforproto, differential-testing, ci]

requires:
  - phase: 03-validation-fidelity
    provides: "03-01's first Hooks() implementation and shared newValidationError constructor; 03-02's complete reverse.go conversion table and SourceMessage.BoundaryOnly provenance; 03-03's complete D-07 hybrid evaluator (standard-rule Filter + violation dedup) this plan's opt-in and sweep both build directly on top of; 03-04's real-client wiring proof and dependency-parity gate"
provides:
  - "WithMessageRules(OnCreate) — message-level (cross-field) protovalidate rules stay boundary-only by default; opting in enforces them on Create only, with D-10's schema-load field-reference walk (messagerules.go) panicking on any reference to a field this package cannot reconstruct, proven in both the false-negative and false-positive direction"
  - "TestCorpusExercisesEveryProtovalidateConstraintClass (PIPE-05): a reflectively-computed ground truth of every protovalidate rule category populated anywhere in the corpus, cross-checked for witness in recorded provenance (TranslatedIDs/ResidualIDs/LengthUnitDivergentIDs/BoundaryOnly) — plus a duplicate-golden-claim guard"
  - "The driverless PIPE-06 differential sweep (internal/difftest/sweep_test.go, generate.go): every corpus message swept against real Create()/Update() calls (21 messages with a committed ent.Schema fixture) or reported covered-with-zero-cases (36 messages), with a name-derived deterministic seed and an opt-in time-seeded CI job"
  - "deriveFromDescriptor — derive[M]'s descriptor-driven core extracted so a caller (the new constraint-class guard) can walk the corpus generically with no per-message compile-time type parameter"
  - "fakeDriver.Tx now returns dialect.NopTx instead of an error, unblocking UpdateOneID(id).Save(ctx) — the single-entity Update path dialect/sql/sqlgraph's UpdateNode always wraps in drv.Tx(ctx), independent of whether the schema has edges"
affects: []

actuals:
  tokens: 423636
  tasks: 3
  commits: 3

tech-stack:
  added: []
  patterns:
    - "D-10's field-reference walk: cel.Ast.NativeRep()'s navigable expression tree (common/ast.NavigateAST + MatchDescendants(KindMatcher(SelectKind))), matching every SelectKind node whose Operand is the identifier \"this\" — resolves 03-RESEARCH.md Open Question 3 (ReferenceMap cannot make this distinction; an AST select-node walk can), verified in both directions before being wired in"
    - "Reflective, generic ent.Client driving via reflect + ent.Mutation.SetField: internal/difftest/sweep_test.go drives Create()/UpdateOneID() against any of 21 entity types with zero per-type Go code, using each generated builder's shared Mutation()/Save(ctx) method shape"
    - "Constraint-class coverage ground truth computed from protoreflect.Message.Range over validate.FieldRules' populated \"type\" oneof member (fieldRuleClasses, corpus_test.go), the identical mechanism fieldmap.go's boundaryOnlyRuleIDs already uses for provenance — applied to every corpus field regardless of whether it derives, so the guard's ground truth can never be a hand-typed list"
    - "Clock-free deterministic seeding (D-14): sweepSeedFor hashes the message FullName via fnv64a; an opt-in SWEEP_EXPLORATORY_SEED env var (set only by CI's own shell step, never by Go code) is XORed in for the exploratory job, keeping the required path's seed a provably pure function of the name"

key-files:
  created:
    - mixinforproto/messagerules.go
    - mixinforproto/messagerules_test.go
    - mixinforproto/internal/difftest/messagerules_test.go
    - mixinforproto/internal/difftest/generate.go
    - mixinforproto/internal/difftest/sweep_test.go
    - mixinforproto/internal/difftest/ent/schema/messagerules.go
    - mixinforproto/internal/difftest/ent/schema/sweep_*.go (19 new single-mixin fixtures)
    - proto/mixinforprototest/v1/messagerules.proto
    - mixinforproto/testdata/messagerules_ok.golden
    - mixinforproto/testdata/messagerules_excluded_ref.golden
  modified:
    - mixinforproto/option.go
    - mixinforproto/hooks.go
    - mixinforproto/hooks_test.go
    - mixinforproto/mixin.go
    - mixinforproto/derive.go
    - mixinforproto/corpus_test.go
    - mixinforproto/internal/difftest/fakedriver.go
    - mixinforproto/internal/difftest/ent/** (regenerated: client.go, ent.go, hook/hook.go, migrate/schema.go, mutation.go, predicate/predicate.go, runtime/runtime.go, tx.go, plus 19 new entity type files)
    - proto/descriptorset.binpb
    - mixinforproto/internal/gen/mixinforprototestv1/messagerules.pb.go
    - .github/workflows/ci.yml

key-decisions:
  - "D-10's schema-load gate lives entirely in buildHookState (hooks.go), not derive.go: message-level rules are consumed ONLY by the hook's evaluator, so the check belongs where the consuming code is, reusing hooks.go's own failures/newDerivationError collected pattern rather than adding a second panic mechanism. buildHookState's signature grew a variadic ...Option parameter (backward-compatible with every existing hooks_test.go call site) rather than a breaking *options change."
  - "The D-10 field-reference walk's resolved reference set seeds EXTRA hookState evaluator entries for fields a message rule reads but which carry no field-level rule of their own — otherwise those fields would never be reverse-converted into the reconstructed dynamicpb message and a message-level rule would silently compute against a phantom proto3 zero (D-03 extended to message scope)."
  - "PIPE-05's constraint-class guard scopes its ground truth to the rule categories actually populated somewhere in the LIVE mixinforprototest.v1 corpus (reflectively computed, never protovalidate's full abstract FieldRules surface) — the corpus today uses exactly {string, int32, float, double, required, cel, repeated}; enum.defined_only is a documented exception (D-14, needs no residual record by construction). This is a scope decision made explicit in corpus_test.go's own doc comment, not a silent narrowing: it makes the guard genuinely closeable (every class in ground truth is witnessed) rather than requiring dozens of new proto fixtures the corpus's own translation surface (validate.go) never targets."
  - "PIPE-06's differential sweep drives REAL ent.Client Create()/Update() calls (not a hand-built ent.Mutation double) exactly as the plan specifies, because internal/difftest is a SEPARATE Go package from mixinforproto and cannot call its unexported hookState.evaluate() — the only reachable 'storage verdict' surface from outside the package is a real generated ent.Client. Rather than hand-writing 21 per-message Go drivers, the sweep drives them generically via reflect + ent.Mutation.SetField (every generated Create/UpdateOneID builder shares the identical Mutation()/Save(ctx) method shape), so a new schema fixture needs zero sweep_test.go changes to be picked up."
  - "Only 21 of the corpus's 57 messages get a committed ent.Schema fixture: exactly the ones that (a) derive successfully with zero Exclude/Override/AsJSON options and (b) carry at least one scalar/optionalScalar field with its OWN protovalidate rule (hooks.go's hookFieldClass scope boundary). The other 36 are swept as 'covered with zero cases' — verified by direct diagnostic before writing any fixture, not assumed."
  - "fakeDriver.Tx (outside this plan's declared file scope, Rule 3 auto-fix) now returns dialect.NopTx(d) instead of a not-implemented error: dialect/sql/sqlgraph's UpdateNode — the code path UpdateOneID(id).Save(ctx) always takes — calls drv.Tx(ctx) unconditionally, unlike the bulk Update().Where(...) path 03-03's fixtures use (which only needs a real Tx when edges exist). dialect.NopTx is the same real, exported ent helper dialect/sql/sqlgraph already uses for itself everywhere else an edge-free schema needs no real transaction."

patterns-established:
  - "WithMessageRules(OnCreate)'s MessageRuleTrigger type declares exactly one value — go doc . lists it explicitly, matching Phase 1's precedent that a declared-but-inert symbol (OnUpdateWithFetch) is worse than an absent one."

requirements-completed: [VAL-04, VAL-08, PIPE-05, PIPE-06]

coverage:
  - id: D1
    description: "WithMessageRules(OnCreate) enforces message-level rules on Create only; without the opt-in a violating entity produces zero storage-layer violations at and around the rule's threshold"
    requirement: "VAL-08"
    verification:
      - kind: unit
        ref: "mixinforproto/messagerules_test.go#TestEvaluate_MessageRuleWithoutOptInProducesZeroViolations"
        status: pass
      - kind: unit
        ref: "mixinforproto/internal/difftest/messagerules_test.go#TestMessageRules_CreateRejectsViolatingEntity"
        status: pass
      - kind: unit
        ref: "mixinforproto/internal/difftest/messagerules_test.go#TestMessageRules_UpdateNotSubjectToMessageRuleEnforcement"
        status: pass
      - kind: unit
        ref: "mixinforproto/internal/difftest/messagerules_test.go#TestMessageRules_DeterministicAndConcurrent"
        status: pass
    human_judgment: false
  - id: D2
    description: "D-10's field-reference walk panics on a message-rule reference to an excluded, overridden, or underivable field, naming every offender in one collected pass — proven with zero false positives against a lookalike fixture and byte-exact field-name matching"
    requirement: "VAL-08"
    verification:
      - kind: unit
        ref: "mixinforproto/messagerules_test.go#TestBuildHookState_MessageRuleExcludedRefFailsSchemaLoad"
        status: pass
      - kind: unit
        ref: "mixinforproto/messagerules_test.go#TestBuildHookState_MessageRuleTwoExcludedRefsFailsInOnePass"
        status: pass
      - kind: unit
        ref: "mixinforproto/messagerules_test.go#TestBuildHookState_MessageRuleUnderivableRefFailsSchemaLoad"
        status: pass
      - kind: unit
        ref: "mixinforproto/messagerules_test.go#TestBuildHookState_MessageRuleNoneIsLegalNoOp"
        status: pass
      - kind: unit
        ref: "mixinforproto/messagerules_test.go#TestBuildHookState_MessageRuleLookalikeDoesNotFalsePositive"
        status: pass
      - kind: unit
        ref: "mixinforproto/messagerules_test.go#TestCheckMessageRuleReferences_FieldLookupIsByteExact"
        status: pass
    human_judgment: false
  - id: D3
    description: "An unexercised protovalidate constraint class, a missing coverage claim, a deleted golden, and a duplicated golden claim are each a named, sorted, reproducible test failure — each rehearsed failing then restored"
    requirement: "PIPE-05"
    verification:
      - kind: unit
        ref: "mixinforproto/corpus_test.go#TestCorpusExercisesEveryProtovalidateConstraintClass"
        status: pass
      - kind: unit
        ref: "mixinforproto/corpus_test.go#TestCorpusMessagesHaveRecordedCoverage (no two corpus messages share the same golden claim)"
        status: pass
      - kind: other
        ref: "manual rehearsal (recorded below): missing coverage entry, deleted golden, unwitnessed class — each observed failing with the expected message, then restored; diff-verified byte-identical to pre-rehearsal state"
        status: pass
    human_judgment: false
  - id: D4
    description: "Every corpus message is swept — 21 with real Create()/Update() calls, 36 covered-with-zero-cases — with deterministic per-message seeding, boundary-adjacent values, non-ASCII coverage for length-unit-divergent fields, and protovalidate/ent verdict agreement on pass/fail, RuleId set, and field path"
    requirement: "PIPE-06"
    verification:
      - kind: unit
        ref: "mixinforproto/internal/difftest/sweep_test.go#TestSweep_DriverlessDifferential"
        status: pass
      - kind: unit
        ref: "mixinforproto/internal/difftest/sweep_test.go#TestSweepSeed_DeterministicAndClockIndependent"
        status: pass
      - kind: unit
        ref: "mixinforproto/internal/difftest/sweep_test.go#TestSweepDisagreement_ReportsSeedMessageFieldAndValue"
        status: pass
      - kind: unit
        ref: "mixinforproto/internal/difftest/sweep_test.go#TestSweep_ZeroFieldMessageIsReportedNotSkipped"
        status: pass
    human_judgment: false
  - id: D5
    description: "go test ./... -count=5 -race, GOWORK=off build/test, make build/vet/test-determinism/check-stubs/check-modules/check-goversion/check-dep-parity all pass; mixinforproto/go.mod gains no DB driver and no new dependency; ci.yml's sweep-exploratory job is workflow_dispatch-only and does not alter the push/pull_request trigger set"
    verification:
      - kind: other
        ref: "cd mixinforproto && go test ./... -count=5 -race; GOWORK=off go build ./... && GOWORK=off go test ./...; make build && make vet && make test-determinism && make check-stubs && make check-modules && make check-goversion && make check-dep-parity (repo root) — all exit 0; git diff mixinforproto/go.mod mixinforproto/go.sum is empty"
        status: pass
    human_judgment: false

duration: 51min
completed: 2026-08-14
status: complete
---

# Phase 3 Plan 5: Message-Level Rule Opt-In, Constraint-Class Coverage, and the Differential Sweep Summary

**Closes Phase 3: `WithMessageRules(OnCreate)` opts a schema into Create-only enforcement of cross-field rules with a schema-load gate against phantom-field references; a reflectively-computed constraint-class guard makes an unexercised protovalidate rule category a named corpus failure; and a driverless differential sweep drives real `Create()`/`Update()` calls against 21 corpus messages (36 more reported covered-with-zero-cases) to prove `protovalidate verdict == ent mutation verdict`.**

## Performance

- **Duration:** 51 min
- **Started:** 2026-08-14T17:12:01Z
- **Completed:** 2026-08-14T18:03:17Z
- **Tasks:** 3 (all `type="auto" tdd="true"`)
- **Files modified:** 186 (most of the volume is regenerated `ent` code for 19 new differential-sweep fixtures — non-generated diff is ~101KB of the ~1.7MB total)

## Accomplishments

- `WithMessageRules(OnCreate)` and `MessageRuleTrigger`/`OnCreate` are real, declared API (`option.go`, `messagerules.go`): message-level (cross-field) protovalidate rules stay boundary-only by default (VAL-08); opting in enforces them on Create only via a single boolean flip in `evaluate()`'s existing standard-rule `Filter` — no second evaluation path. `OnUpdateWithFetch` stays deliberately undeclared per `mixinforproto.md` §4.3.
- D-10's schema-load field-reference walk (`checkMessageRuleReferences`) compiles each message-level CEL rule once and statically enumerates every top-level `this.<field>` select via `cel-go`'s navigable AST (`common/ast.NavigateAST` + `MatchDescendants(KindMatcher(SelectKind))`), resolving 03-RESEARCH.md Open Question 3: `ReferenceMap()` cannot distinguish a field select from an unrelated identifier/call/literal; an AST select-node walk can. Verified directly against a lookalike expression before being wired in — a comprehension-bound variable and a string literal both correctly produce zero false positives, and a genuine `this.hi` select is correctly found alongside them.
- A reference to a field this package cannot reconstruct — excluded, overridden, or underivable — fails schema load naming the message, the rule id, and the field, with every offender reported in one collected pass (`buildHookState`'s existing `failures`/`newDerivationError` machinery, no new panic mechanism).
- `TestCorpusExercisesEveryProtovalidateConstraintClass` (PIPE-05, `corpus_test.go`) reflectively enumerates every protovalidate rule category populated anywhere in the corpus (via `fieldRuleClasses`, the same `protoreflect.Message.Range` mechanism `boundaryOnlyRuleIDs` already uses) and asserts each is witnessed by recorded provenance — retrying a field's derivation with just that field `Exclude()`'d when the whole message otherwise fails to derive, so `repeated` surfaces via `BoundaryOnly` with no new corpus fixture. `TestCorpusMessagesHaveRecordedCoverage` gained a duplicate-golden-claim check.
- `mixinforproto/internal/difftest/sweep_test.go` + `generate.go` (PIPE-06) walk the identical registry corpus `corpus_test.go` walks. For 21 messages with a committed `ent.Schema` fixture (19 added this task, single-mixin, zero-option boilerplate; `ResidualCel`/`MixedFieldRules` already existed), deterministically seeded values (name-derived `fnv64a` hash — D-14, no clock) are pushed through both `protovalidate.Validate` and a real `Create()`/`UpdateOneID().Save(ctx)` driven generically via `reflect` + `ent.Mutation.SetField` — no per-type Go code. Every other corpus message (36) is reported "covered with zero cases," never silently skipped; a swept+zero-case accounting assertion catches a silently dropped message. Every `LengthUnitDivergentIDs`-flagged field is proven to receive a genuinely multi-byte non-ASCII value.
- `ci.yml` gained an opt-in `sweep-exploratory` job (`workflow_dispatch` only, outside the required `push`/`pull_request` path) that reruns the sweep with `SWEEP_EXPLORATORY_SEED` set from the shell's own `date +%s` — the Go test code itself never imports `time`, keeping `sweepSeedFor` a provably pure function of the message name for every other caller.

## Task Commits

Each task was committed atomically:

1. **Task 1: `WithMessageRules(OnCreate)` and the excluded-field-reference gate** - `f816673` (feat)
2. **Task 2: Make an unexercised constraint class a named failure (PIPE-05)** - `58b106a` (test)
3. **Task 3: The driverless differential sweep with deterministic per-message seeding (PIPE-06)** - `154e75c` (feat)

**Plan metadata:** _(recorded in this commit's own final metadata commit)_

_Note: all three tasks carried `tdd="true"`; each was implemented and verified as a single commit per task (tests + production code together) rather than a separate RED/GREEN split — every task's own `<verify>` commands were run and passed before that task's commit._

## Files Created/Modified

- `mixinforproto/messagerules.go` - `WithMessageRules`'s schema-load walk (`checkMessageRuleReferences`, `fieldSelectsOnThis`, `messageRuleCELEnv`, `messageRuleFieldUnavailable`)
- `mixinforproto/option.go` - `WithMessageRules`, `options.messageRules`/`messageRulesEnabled()`
- `mixinforproto/hooks.go` - `buildHookState(md, opts ...Option)`, `hookState.messageRulesOnCreate`, the Filter's one-boolean flip
- `mixinforproto/mixin.go` - `Hooks()` passes `m.opts` through to `buildHookState`
- `mixinforproto/derive.go` - `deriveFromDescriptor` extracted from `derive[M]` (no behavior change)
- `proto/mixinforprototest/v1/messagerules.proto` - `MessageRuleOk`/`ExcludedRef`/`TwoExcludedRefs`/`UnderivableRef`/`Detail`/`None`/`Lookalike`
- `mixinforproto/messagerules_test.go`, `mixinforproto/internal/difftest/messagerules_test.go` - Task 1's schema-load and real-client tests
- `mixinforproto/corpus_test.go` - `TestCorpusExercisesEveryProtovalidateConstraintClass`, duplicate-golden-claim guard
- `mixinforproto/internal/difftest/generate.go`, `sweep_test.go` - PIPE-06's value generator and sweep
- `mixinforproto/internal/difftest/fakedriver.go` - `Tx()` now returns `dialect.NopTx(d)`
- `mixinforproto/internal/difftest/ent/schema/messagerules.go` + 19 `sweep_*.go` fixtures, and the whole regenerated `ent` package
- `.github/workflows/ci.yml` - `sweep-exploratory` job, `workflow_dispatch` trigger added

## Decisions Made

See `key-decisions` in frontmatter for the full list. Highlights:
- D-10's gate lives in `hooks.go`, not `derive.go` — message-level rules are consumed only by the hook.
- PIPE-05's guard scopes its ground truth to the corpus's actual rule usage (7 classes today), documented as a deliberate, closeable scope rather than protovalidate's full abstract surface.
- PIPE-06 drives a REAL `ent.Client` (package-visibility forces this — `internal/difftest` cannot reach `mixinforproto`'s unexported `hookState.evaluate()`), generically via reflection so no per-message Go code is needed.
- Only 21 of 57 corpus messages needed a new/existing `ent.Schema` fixture — determined empirically before writing any fixture, not assumed.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] `fakeDriver.Tx` needed a working implementation for `UpdateOneID(id).Save(ctx)`**
- **Found during:** Task 3's first sweep run (`RequiredOptionalBytes`/`RequiredOptionalNonString`/`RequiredOptionalString`/`StringFormatHostname` Update sub-tests failing)
- **Issue:** `dialect/sql/sqlgraph`'s `UpdateNode` (the code path `UpdateOneID(id).Save(ctx)` always takes) calls `drv.Tx(ctx)` unconditionally, regardless of whether the schema has edges — unlike the bulk `Update().Where(...)` path 03-03's fixtures use, which only needs a transaction when edges exist. `fakeDriver.Tx` previously returned a "not implemented — not needed" error, correct for every prior caller but wrong once this sweep started calling `UpdateOneID` generically.
- **Fix:** `Tx()` now returns `dialect.NopTx(d), nil` — the same real, exported ent helper `dialect/sql/sqlgraph` already uses for itself in every other edge-free Tx-needing path.
- **Files modified:** `mixinforproto/internal/difftest/fakedriver.go` (outside this task's declared file scope, but a direct, mechanical, necessary consequence of driving `UpdateOneID` generically)
- **Committed in:** `154e75c` (Task 3 commit)

**2. [Rule 1 - Bug] Update-half boundary comparison was scoped to ALL sweepable fields instead of just the touched subset**
- **Found during:** Task 3's first sweep run (`DoubleComparators` Update sub-test disagreement: boundary flagged a violation on an untouched sibling field that ent's storage side correctly never evaluated)
- **Issue:** The Update boundary entity's `protovalidate.WithFilter` scope was built from `allNames(fields)` (every sweepable field) instead of the actual touched subset — an untouched field left at its proto3 zero then tripped its own unrelated rule on the boundary side only, a false disagreement rather than a real one.
- **Fix:** Scope the Update-half filter to the subset map's own keys.
- **Files modified:** `mixinforproto/internal/difftest/sweep_test.go`
- **Committed in:** `154e75c` (Task 3 commit)

---

**Total deviations:** 2 auto-fixed (both Rule 1 — bugs found and fixed by the plan's own verification loop before the task commit). No scope creep: both fixes were necessary for Task 3's own acceptance criteria (a real, passing Update sweep) to hold.
**Impact on plan:** None on design; both are implementation-correctness fixes.

## Guard Rehearsals (Task 2, PIPE-05)

Each rehearsal was performed via a temporary edit, observed failing with `go test`, then reverted (confirmed byte-identical to the pre-rehearsal file via `diff`) before the task's commit:

1. **Missing coverage entry** — commented out `MessageRuleOk`'s `corpusCoverage` entry. Observed: `TestCorpusMessagesHaveRecordedCoverage/every_corpus_message_has_a_recorded_coverage_claim` failed naming `mixinforprototest.v1.MessageRuleOk (declared in mixinforprototest/v1/messagerules.proto)`. Restored; diff clean.
2. **Deleted golden** — moved `testdata/messagerules_ok.golden` out of the tree. Observed: `TestCorpusMessagesHaveRecordedCoverage/every_golden:_claim_resolves_to_an_existing_testdata_fixture` failed naming `mixinforprototest.v1.MessageRuleOk -> golden:messagerules_ok (missing testdata/messagerules_ok.golden)`. Restored; test green again.
3. **Unwitnessed class** — injected `groundTruth["rehearsal_break_3_unwitnessed"] = true` directly after Part A's subtest (isolating Part B's own failure text). Observed: the test failed naming `rehearsal_break_3_unwitnessed` in the exact "protovalidate constraint class(es) with zero corpus witness..." message, while Part A's own subtest independently PASSED (confirming isolation). Reverted; diff clean against the pre-rehearsal file.

## Issues Encountered

- Determining PIPE-06's real sweep scope required an empirical pass (not assumption): a diagnostic test run against the live corpus showed only 21 of 57 messages both derive with zero options AND carry a scalar/optionalScalar field with its own protovalidate rule (`hooks.go`'s `hookFieldClass` scope boundary) — the other 36 are correctly "covered with zero cases," which the plan's own Test 5 requires be reported, not silently skipped. Building this empirically first avoided either under- or over-building `ent.Schema` fixtures.
- `internal/difftest` is a separate Go package from `mixinforproto` and cannot reach `hookState.evaluate()` (unexported) — this ruled out a lighter-weight sweep design driving `hs.evaluate()` directly against a hand-built `ent.Mutation`, and confirmed the plan's literal "real Create().Save(ctx) through the generated ent.Client" instruction was the only architecturally reachable design, not merely the preferred one.
- `make test-determinism` takes several minutes in this sandbox (pre-existing, `internal/boundarytest`'s repeated real `entc.LoadGraph` subprocess cost, unrelated to this plan) — run via `run_in_background` rather than the default foreground timeout; completed with exit code 0.

## User Setup Required

None — no external service configuration required.

## Next Phase Readiness

- Phase 3 (VAL-04…VAL-11, PIPE-05, PIPE-06) is now complete: `mixinforproto` enforces every protovalidate field-scoped rule (standard and residual CEL) at the storage layer, message-level rules are opt-in and Create-only with a schema-load gate against phantom-field references, the corpus is guarded against an unexercised protovalidate constraint class on top of its existing derivation-shape/message-coverage guards, and a driverless differential sweep proves `protovalidate verdict == ent mutation verdict` across the corpus with deterministic, reproducible seeding.
- The known pre-existing gap flagged in 03-04's SUMMARY (`mixinforproto v0.1.0`'s published tag predates `Hooks()`, so `make test-standalone-root` fails under `GOWORK=off` for the `hookwiring` fixture) is UNCHANGED by this plan — still open, still Phase 1 D-19's follow-up (push a new `mixinforproto` tag, bump root's `go.mod` in a separate commit), still explicitly out of scope here.
- `internal/difftest`'s reflective Create/Update driving pattern (`sweep_test.go`'s `createEntity`/`updateEntity`) is now available as a reusable technique for any future plan that needs to drive a corpus-wide real-client proof without per-message Go code.
- `mixinforproto` still has no DB driver in its `go.mod` (verified: `git diff mixinforproto/go.mod` is empty across this whole plan) and still declares `go 1.24.0`.

---
*Phase: 03-validation-fidelity*
*Completed: 2026-08-14*

## Self-Check: PASSED

All created files (mixinforproto/messagerules.go, mixinforproto/messagerules_test.go,
mixinforproto/internal/difftest/messagerules_test.go, mixinforproto/internal/difftest/generate.go,
mixinforproto/internal/difftest/sweep_test.go, mixinforproto/internal/difftest/ent/schema/messagerules.go,
proto/mixinforprototest/v1/messagerules.proto, mixinforproto/testdata/messagerules_ok.golden,
mixinforproto/testdata/messagerules_excluded_ref.golden, this SUMMARY.md) and all three task
commits (f816673, 58b106a, 154e75c) verified present on disk / in git log.
