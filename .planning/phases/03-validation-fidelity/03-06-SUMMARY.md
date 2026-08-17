---
phase: 03-validation-fidelity
plan: 06
subsystem: database
tags: [protovalidate, cel-go, ent, mixinforproto, validation]

requires:
  - phase: 03-validation-fidelity
    provides: "03-01/03-02/03-03/03-05's D-07 hybrid evaluator (buildHookState/evaluate), reverse.go's reverse-conversion table, WithMessageRules(OnCreate)'s D-10 field-reference gate, and 03-VERIFICATION.md's CR-02/CR-03 findings this plan closes"
provides:
  - "(buf.validate.field).ignore honored identically by the local CEL half and protovalidate's own boundary evaluator, for both IGNORE_ALWAYS (compile-time skip) and IGNORE_IF_ZERO_VALUE (mutation-time skip)"
  - "Exclude(...)/Override(...) fields produce no storage-layer violations and no evaluators entry, closing the hooks.go -> option.go key link 03-VERIFICATION.md recorded as NOT WIRED"
  - "A type-changing Override no longer produces a D-12 fault that runtime.MapError maps to CodeInternal"
affects: [03-08, phase-3-ship-gate]

actuals:
  tokens: 113575
  tasks: 3
  commits: 3

tech-stack:
  added: []
  patterns:
    - "Compile-time vs mutation-time ignore gating: IGNORE_ALWAYS is resolved once in buildHookState (fieldEvaluator.ignore, no CEL programs compiled); IGNORE_IF_ZERO_VALUE is resolved every evaluate() call against the actual reverse-converted value via isZeroForKind, since it is value-dependent and cannot be decided at schema load"
    - "Exclude/Override skip placed BEFORE protovalidate.ResolveFieldRules in buildHookState's per-field loop, not after — placement itself is the fix, pinned by a structural test (TestHooksGo_ExcludedOverriddenCheckPrecedesResolveFieldRules)"

key-files:
  created:
    - proto/mixinforprototest/v1/ignore.proto
    - mixinforproto/internal/gen/mixinforprototestv1/ignore.pb.go
    - mixinforproto/internal/difftest/ent/schema/ignorerules.go
    - mixinforproto/internal/difftest/ent/schema/overriddenrules.go
    - mixinforproto/internal/difftest/ignore_test.go
    - mixinforproto/internal/difftest/optionsuppression_test.go
  modified:
    - mixinforproto/hooks.go
    - mixinforproto/hooks_test.go
    - mixinforproto/corpus_test.go
    - proto/buf.yaml
    - proto/descriptorset.binpb

key-decisions:
  - "IGNORE_ALWAYS is a schema-load-time (compile-time) skip inside buildHookState's per-field loop: it never compiles CEL programs for the field, but keeps the field's evaluators entry so the standard-rule Filter scope and message-rule reverse-conversion both stay correct."
  - "IGNORE_IF_ZERO_VALUE is a mutation-time (value-dependent) skip inside evaluate()'s celFields loop via a new isZeroForKind(fd, v) helper, scoped to exactly the scalar/optionalScalar kinds hookFieldClass already routes."
  - "Exclude(...)/Override(...) skip placed in buildHookState BEFORE protovalidate.ResolveFieldRules — not after — so a type-changing Override never reaches reverseValue with a value of the replacement field's Go type."
  - "proto/buf.yaml gained a narrowly-scoped lint ignore_only for the PROTOVALIDATE check on ignore.proto only, since buf's built-in lint flags ignore=IGNORE_ALWAYS combined with a sibling rule as dead weight at the boundary — exactly the combination CR-02's fixture needs to prove the LOCAL cel.Env also honors it."

requirements-completed: [VAL-04, VAL-05, VAL-06, VAL-07, PIPE-05, PIPE-06]

coverage:
  - id: D1
    description: "IGNORE_ALWAYS honored identically by the local CEL half and the boundary evaluator"
    requirement: VAL-06
    verification:
      - kind: unit
        ref: "mixinforproto/hooks_test.go (TestBuildHookState_* / evaluators struct field), covered indirectly by fieldEvaluator.ignore"
        status: pass
      - kind: integration
        ref: "mixinforproto/internal/difftest/ignore_test.go#TestIgnoreAlways_SuppressedFieldProducesZeroViolations"
        status: pass
      - kind: integration
        ref: "mixinforproto/internal/difftest/ignore_test.go#TestIgnoreAlways_NonIgnoredSiblingStillRejects"
        status: pass
    human_judgment: false
  - id: D2
    description: "IGNORE_IF_ZERO_VALUE suppresses the local CEL half exactly at the type's zero, for both string and int32 kinds"
    requirement: VAL-06
    verification:
      - kind: unit
        ref: "mixinforproto/hooks_test.go#TestEvaluate_IgnoreIfZeroValue_ZeroStringProducesNoViolation"
        status: pass
      - kind: unit
        ref: "mixinforproto/hooks_test.go#TestEvaluate_IgnoreIfZeroValue_NonZeroFailingStringProducesOneViolation"
        status: pass
      - kind: unit
        ref: "mixinforproto/hooks_test.go#TestEvaluate_IgnoreIfZeroValue_ZeroInt32ProducesNoViolation"
        status: pass
      - kind: unit
        ref: "mixinforproto/hooks_test.go#TestEvaluate_IgnoreIfZeroValue_NonZeroFailingInt32ProducesOneViolation"
        status: pass
      - kind: unit
        ref: "mixinforproto/hooks_test.go#TestIsZeroForKind_CoversMultipleKinds"
        status: pass
      - kind: integration
        ref: "mixinforproto/internal/difftest/ignore_test.go#TestIgnoreIfZeroValue_BothFieldsAtZeroProduceZeroViolations"
        status: pass
      - kind: integration
        ref: "mixinforproto/internal/difftest/ignore_test.go#TestIgnoreIfZeroValue_NonZeroFailingValuesRejectedIdentically"
        status: pass
    human_judgment: false
  - id: D3
    description: "Exclude(...)/Override(...) suppress storage-layer validation relay entirely, including a type-changing Override that previously produced a D-12 CodeInternal fault"
    requirement: VAL-07
    verification:
      - kind: unit
        ref: "mixinforproto/hooks_test.go#TestBuildHookState_ExcludedFieldHasNoEvaluatorEntry"
        status: pass
      - kind: unit
        ref: "mixinforproto/hooks_test.go#TestEvaluate_ExcludedFieldProducesZeroViolations"
        status: pass
      - kind: unit
        ref: "mixinforproto/hooks_test.go#TestEvaluate_OverriddenFieldProducesZeroViolations"
        status: pass
      - kind: unit
        ref: "mixinforproto/hooks_test.go#TestEvaluate_TypeChangingOverrideReturnsNilErrorAndNoViolations"
        status: pass
      - kind: unit
        ref: "mixinforproto/hooks_test.go#TestBuildHookState_AllRuleBearingFieldsExcludedProducesNoEvaluators"
        status: pass
      - kind: unit
        ref: "mixinforproto/hooks_test.go#TestHooksGo_ExcludedOverriddenCheckPrecedesResolveFieldRules"
        status: pass
      - kind: integration
        ref: "mixinforproto/internal/difftest/optionsuppression_test.go#TestOptionSuppression_OverriddenFieldAcceptsRuleViolatingValue"
        status: pass
      - kind: integration
        ref: "mixinforproto/internal/difftest/optionsuppression_test.go#TestOptionSuppression_NonOverriddenSiblingStillRejects"
        status: pass
    human_judgment: false
  - id: D4
    description: "New ignore corpus fixture is registered with a coverage claim and exercised by the differential harness, and no new protovalidate.ValidationError construction site was introduced"
    requirement: PIPE-06
    verification:
      - kind: unit
        ref: "mixinforproto/corpus_test.go#TestCorpusMessagesHaveRecordedCoverage"
        status: pass
      - kind: unit
        ref: "mixinforproto/corpus_test.go#TestCorpusExercisesEveryProtovalidateConstraintClass"
        status: pass
      - kind: other
        ref: "make check-single-validationerror-site"
        status: pass
    human_judgment: false

duration: 36min
completed: 2026-08-15
status: complete
---

# Phase 3 Plan 06: Ignore-aware, Option-aware buildHookState Summary

**Closed CR-02 and CR-03 from 03-VERIFICATION.md: `(buf.validate.field).ignore` and `Exclude`/`Override` now suppress storage-layer validation exactly as the boundary and `option.go`'s docs promise, including a fix for a type-changing `Override` that previously produced a permanent HTTP 500.**

## Performance

- **Duration:** 36 min
- **Started:** 2026-08-15T11:29:46Z
- **Completed:** 2026-08-15T12:01:15Z
- **Tasks:** 3
- **Files modified:** 42 (across 3 commits)

## Accomplishments

- `buildHookState` now resolves each field's `(buf.validate.field).ignore` mode once at schema load; `IGNORE_ALWAYS` skips local CEL compilation entirely while keeping the field's `evaluators` entry (needed for both the standard-rule Filter scope and message-rule reverse-conversion).
- `evaluate()`'s CEL loop gates `IGNORE_IF_ZERO_VALUE` at mutation time via a new `isZeroForKind` helper, scoped to the scalar/optionalScalar kinds `hookFieldClass` already routes (string, bytes, bool, every integer/float kind, enum).
- `buildHookState`'s per-field loop now skips `Exclude(...)`/`Override(...)` fields BEFORE calling `protovalidate.ResolveFieldRules` — the placement itself (not just the presence of a check) is what prevents a type-changing `Override` from ever reaching `reverseValue` with a value of the replacement field's Go type, closing the D-12 `CodeInternal` fault the verifier reproduced.
- New `ignore.proto` corpus fixture (`IgnoreAlwaysWithCel`, `IgnoreIfZeroWithCel`) and real `ent.Client` differential proofs (`ignore_test.go`, `optionsuppression_test.go`) prove both gaps closed through ent's actual `withHooks` pipeline, not just hand-built `ent.Mutation` doubles.
- Every pre-fix failure documented below was independently rehearsed in a scratch copy (never the committed tree) and reproduces the exact symptom the verifier/plan predicted.

## Task Commits

Each task was committed atomically:

1. **Task 1: End-to-end "IGNORE_ALWAYS is honored at storage"** — `8bb1520` (feat, tracer)
2. **Task 2: IGNORE_IF_ZERO_VALUE at mutation time + corpus/coverage registration** — `8ecf13d` (feat)
3. **Task 3: Exclude(...)/Override(...) suppress storage-layer relay** — `11391ad` (feat)

_Note: Task 2's commit was amended once, in-session, immediately after creation and before Task 3 existed: staging the shared generated `internal/difftest/ent/*` files via a plain `git add <path>` accidentally captured working-tree state that already included Task 3's `OverriddenMixedFieldRules` regeneration (schema/overriddenrules.go existed on disk mid-session before its own commit). This was caught by checking out the Task 2 commit into an isolated `git worktree` and running `go test ./...` there — it failed to build. Fixed by temporarily removing the Task 3 schema file, regenerating the ent client to the correct Task-2-only state, and amending. Re-verified buildable in isolation via the same worktree check before proceeding to Task 3._

## Files Created/Modified

- `proto/mixinforprototest/v1/ignore.proto` - CR-02's corpus fixture: `IgnoreAlwaysWithCel` (Task 1) and `IgnoreIfZeroWithCel` (Task 2)
- `mixinforproto/internal/gen/mixinforprototestv1/ignore.pb.go` - generated stub for the above
- `proto/descriptorset.binpb` - regenerated descriptor set covering the new fixture
- `proto/buf.yaml` - scoped `ignore_only` for the `PROTOVALIDATE` lint check on `ignore.proto` (the fixture deliberately combines `ignore=IGNORE_ALWAYS` with a sibling rule, which buf's built-in check otherwise flags as dead weight)
- `mixinforproto/hooks.go` - `fieldEvaluator.ignore` field; ignore-aware CEL-compilation gate in `buildHookState`; `isZeroForKind`; mutation-time `IGNORE_IF_ZERO_VALUE` gate in `evaluate()`; `Exclude`/`Override` skip before `ResolveFieldRules`; updated doc comments
- `mixinforproto/hooks_test.go` - unit tests for all three gap-closure behaviors, plus a structural placement pin
- `mixinforproto/corpus_test.go` - coverage claims for `IgnoreAlwaysWithCel`/`IgnoreIfZeroWithCel`
- `mixinforproto/internal/difftest/ent/schema/ignorerules.go` - real ent schemas for both new corpus messages
- `mixinforproto/internal/difftest/ent/schema/overriddenrules.go` - real ent schema for `MixedFieldRules` with `Override("both", field.String("both"))` applied
- `mixinforproto/internal/difftest/ignore_test.go` - real `ent.Client` differential proofs for both ignore modes
- `mixinforproto/internal/difftest/optionsuppression_test.go` - real `ent.Client` differential proofs for `Override`
- `mixinforproto/internal/difftest/ent/*` (generated) - regenerated client covering `IgnoreAlwaysWithCel`, `IgnoreIfZeroWithCel`, `OverriddenMixedFieldRules`

## Decisions Made

- IGNORE_ALWAYS resolved once at schema load (compile-time skip); IGNORE_IF_ZERO_VALUE resolved every `evaluate()` call against the real reverse-converted value (mutation-time, value-dependent skip) — the two ignore modes cannot share one gating mechanism because only one of them is knowable before a mutation exists.
- The `Exclude`/`Override` skip's placement (before `ResolveFieldRules`, not after) is the actual fix, not merely "a check exists somewhere" — pinned with a dedicated structural test (`TestHooksGo_ExcludedOverriddenCheckPrecedesResolveFieldRules`) asserting the source-text ordering directly, since a correctness property this specific deserves a test that can't pass by accident.
- `proto/buf.yaml` gained a narrowly path-scoped lint exception (`ignore_only`) rather than disabling the `PROTOVALIDATE` check repo-wide, matching the file's existing precedent of scoping exceptions to exactly the file that needs one.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] proto/buf.yaml lint exception for the IgnoreAlwaysWithCel fixture**
- **Found during:** Task 1
- **Issue:** `buf lint` rejects a field carrying `ignore=IGNORE_ALWAYS` alongside another rule (its own built-in `PROTOVALIDATE` check calls the combination dead weight at the boundary) — but that combination is exactly what CR-02's fixture needs to prove the LOCAL cel.Env also honors `ignore`.
- **Fix:** Added a narrowly path-scoped `lint.ignore_only` entry for `PROTOVALIDATE` on `mixinforprototest/v1/ignore.proto` only, with a doc comment explaining why.
- **Files modified:** `proto/buf.yaml`
- **Verification:** `buf lint proto` exits 0; every other file still gets the full `PROTOVALIDATE` check.
- **Committed in:** `8bb1520` (Task 1 commit)

**2. [Rule 3 - Blocking] Task 1's own `make test` required a corpus coverage claim for `IgnoreAlwaysWithCel`**
- **Found during:** Task 1
- **Issue:** The plan's Task 2 action step says to register both new corpus messages' coverage claims, but Task 1's own `<verify>` list includes `make test`, which fails immediately once any new message exists in the registry without a `corpusCoverage` entry — `IgnoreAlwaysWithCel` exists after Task 1 alone.
- **Fix:** Added `IgnoreAlwaysWithCel`'s coverage claim in Task 1's commit; Task 2 added `IgnoreIfZeroWithCel`'s claim as originally planned.
- **Files modified:** `mixinforproto/corpus_test.go`
- **Verification:** `go test . -run TestCorpusMessagesHaveRecordedCoverage` exits 0 after Task 1.
- **Committed in:** `8bb1520` (Task 1 commit)

**3. [Rule 1 - Bug] Direct `hs.evaluate` calls on a cel-only ignore field double-count violations before dedup**
- **Found during:** Task 2
- **Issue:** A field with only a custom cel rule (no standard rule) still gets evaluated by BOTH halves of the hybrid once it is in scope for the standard-rule Filter (Pitfall 1's known, accepted double-evaluation) — an initial test asserting `len(violations) == 1` directly off `hs.evaluate`'s raw output failed with 2, exactly mirroring the repo's own precedent (`violation_test.go`'s `dedupeViolations` pattern for `MixedFieldRules`).
- **Fix:** Used `dedupeViolations` (the same helper `violation_test.go` already uses) before asserting violation count, matching the codebase's own established pattern rather than inventing a new one.
- **Files modified:** `mixinforproto/hooks_test.go`
- **Verification:** Tests pass; the underlying double-evaluation is documented, accepted, existing behavior (Pitfall 1), not a regression this plan introduced.
- **Committed in:** `8ecf13d` (Task 2 commit)

**4. [Rule 1 - Bug] Task 2 commit did not build in isolation after a plain `git add <path>` staged mid-session Task 3 working-tree state**
- **Found during:** Task 3, before its own commit
- **Issue:** Staging the shared generated `internal/difftest/ent/*` files for Task 2's commit via `git add <path>` captured their CURRENT working-tree content, which by that point already included Task 3's `OverriddenMixedFieldRules` regeneration (the schema file existed on disk, uncommitted, from work already in progress). This left Task 2's commit referencing a type (`OverriddenMixedFieldRules`) whose defining files were not yet committed — a build failure if that commit were checked out alone.
- **Fix:** Caught by checking out Task 2's HEAD into an isolated `git worktree` and running `go test ./...` there (failed to build). Fixed by moving Task 3's schema/test files aside, regenerating the ent client to the correct Task-2-only state, staging just those regenerated files, and amending Task 2's commit (the immediately-preceding, same-session, unpushed commit — not a rewrite of shared history). Re-verified the amended commit builds in isolation via the same worktree check, then restored Task 3's files and regenerated again for Task 3's own commit.
- **Files modified:** `mixinforproto/internal/difftest/ent/{client.go,ent.go,hook/hook.go,migrate/schema.go,mutation.go,predicate/predicate.go,runtime/runtime.go,tx.go}`
- **Verification:** `git worktree add --detach <tmp> 8ecf13d && cd <tmp>/mixinforproto && go build ./... && go test ./...` — green, after the amend.
- **Committed in:** `8ecf13d` (amended)

**5. [Rule 1 - Bug] Disk space exhaustion caused a spurious `entc` package test failure unrelated to this plan's changes**
- **Found during:** Task 3, final `make test` run
- **Issue:** `/root/.cache/go-build` had grown to 28GB (from repeated toolchain-switching builds across this session), filling the sandbox's disk and causing `go test`'s linker step to fail with "no space left on device" for `entc` package tests — files this plan never touched.
- **Fix:** `go clean -cache` freed ~19GB; confirmed the same `make build`/`make vet`/`make test` sequence then passes cleanly, isolating the failure to disk pressure, not a regression.
- **Files modified:** None (environment-only fix)
- **Verification:** `df -h /` before/after; `make test` green afterward.
- **Committed in:** N/A (no source change)

---

**Total deviations:** 5 auto-fixed (2 blocking, 2 bugs, 1 blocking/coverage). No architectural changes, no scope creep — every deviation is either a correctness requirement of the plan's own `<verify>` gates or a mechanical build-integrity fix.
**Impact on plan:** All auto-fixes necessary for the plan's own commits to be individually buildable and for `make test`/`make check-stubs`/`buf lint` to pass as required by every task's `<verify>` block.

## Issues Encountered

- `go`, `buf`, `protoc-gen-go`, `protoc-gen-connect-go` were not directly on `$PATH` in this sandbox — `go` lives under `/usr/local/go1.24.7/bin` (matching `mixinforproto/go.mod`'s pin) and the buf toolchain under `/root/go/bin`. Every shell invocation this session explicitly prepended both to `PATH` and set `GOROOT`, since the harness does not persist shell state between Bash calls. Documented here so a future executor in the same sandbox does not have to rediscover it.
- Pre-fix rehearsals were run in throwaway scratch copies under the session scratchpad directory (never the committed tree), one per task, each removed immediately after recording the observed failure output below.

### Pre-fix failure output (rehearsed, scratch copies only)

**Task 1 (IGNORE_ALWAYS reverted — the `if ignoreMode != validate.Ignore_IGNORE_ALWAYS` gate replaced with `if true`):**
```
--- FAIL: TestIgnoreAlways_SuppressedFieldProducesZeroViolations
    ignore_test.go:46: want zero storage-layer violations (always_ignored is IGNORE_ALWAYS), got map[ignore.ignore_always_with_cel.always_ignored.starts_with_x always_ignored:true]
--- FAIL: TestIgnoreAlways_NonIgnoredSiblingStillRejects
    ignore_test.go:96: storage/boundary identity mismatch: storage=map[...both keys...] boundary=map[...only enforced...]
```

**Task 3 (`isExcluded`/`isOverridden` skip removed from `buildHookState`):**
```
--- FAIL: TestBuildHookState_ExcludedFieldHasNoEvaluatorEntry
    hooks_test.go:702: want no evaluators entry for excluded field "both", got entries: map[both:true cel_only:true standard_only:true]
--- FAIL: TestEvaluate_ExcludedFieldProducesZeroViolations
    hooks_test.go:725: want zero violations naming excluded field "both", got: [... 2 violations naming "both" ...]
--- FAIL: TestEvaluate_ExcludedSiblingsStillViolateWithUnchangedRuleIDs
    hooks_test.go:752: want RuleIds [...2...], got [...3, including "both"...]
--- FAIL: TestEvaluate_OverriddenFieldProducesZeroViolations / TestEvaluate_OverriddenSiblingsStillViolateWithUnchangedRuleIDs
    (identical shape, under Override)
--- FAIL: TestEvaluate_TypeChangingOverrideReturnsNilErrorAndNoViolations
    hooks_test.go:829: want a nil error for a type-changing Override, got: mixinforproto: reverse-converting field "both" (kind string): expected a Go string, got bool
```

This last line is the exact D-12 `CodeInternal` fault 03-VERIFICATION.md's CR-03 describes, reproduced verbatim.

### Ignore enum constant names used

Confirmed against the vendored `proto/buf/validate/validate.proto` before writing code, per the plan's `<interface_context>`: `validate.Ignore_IGNORE_UNSPECIFIED` (0), `validate.Ignore_IGNORE_IF_ZERO_VALUE` (1), `validate.Ignore_IGNORE_ALWAYS` (3). No value 2 exists and none was invented.

### Part A sanity list

`TestCorpusExercisesEveryProtovalidateConstraintClass`'s Part A hand-maintained `want` list did NOT need editing — as the plan predicted, `"ignore"` is already in `boundaryOnlyNonConstraintFields` (fieldmap.go), so `fieldRuleClasses` filters it out of the ground truth entirely; the new fixtures' `string`/`int32`/`cel` classes were already present in the existing sanity list.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- CR-02 and CR-03 are closed; the `hooks.go -> option.go` key link 03-VERIFICATION.md recorded as NOT WIRED is now wired, and the differential harness (PIPE-06) is no longer blind to either rule shape.
- 03-08-PLAN.md (the phase's remaining gap-closure plan, covering CR-01's message-rule `cel_expression`/`oneof` carrier gap and VAL-07's single-construction-site CI gate — the latter already closed by the prior wave's 03-07) is unblocked to run.
- No new module requirement landed in `mixinforproto/go.mod` (verified via `git diff mixinforproto/go.mod` — empty).

## Self-Check: PASSED

All created/modified files verified present on disk; all three task commit hashes (`8bb1520`, `8ecf13d`, `11391ad`) verified present in `git log --oneline --all`.

---
*Phase: 03-validation-fidelity*
*Completed: 2026-08-15*
