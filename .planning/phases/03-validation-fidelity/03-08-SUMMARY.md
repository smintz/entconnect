---
phase: 03-validation-fidelity
plan: 08
subsystem: database
tags: [protovalidate, cel-go, ent, mixinforproto, validation, gap-closure]

# Dependency graph
requires:
  - phase: 03-validation-fidelity
    provides: "03-05's WithMessageRules(OnCreate)/D-10 schema-load reference gate, 03-06's ignore-aware/option-aware buildHookState and PIPE-06 differential harness, 03-07's check-single-validationerror-site gate, and 03-VERIFICATION.md's CR-01/WR-04 findings this plan closes"
provides:
  - "checkMessageRuleReferences enumerates all three buf.validate.MessageRules carriers (cel, cel_expression, oneof) through one normalization helper (messageRuleReferences), closing CR-01's phantom-verdict gap for the two previously-unread carriers"
  - "WithMessageRules(trigger) stores and validates trigger instead of discarding it; an undeclared MessageRuleTrigger fails schema load naming the option and the value (WR-04)"
  - "Two declaration-surface exhaustiveness guards (Guard A: MessageRules, Guard B: FieldRules) that fail a named test when protovalidate grows a member this package neither handles nor exempts"
  - "PIPE-06 differential harness extended to drive both new carriers through a real ent.Client, comparing violation identity against protovalidate.Validate's own boundary verdict"
affects: [phase-3-ship-gate]

# Actuals (#2632)
actuals:
  tokens: 59858
  tasks: 3
  commits: 4

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Three-carrier normalization: messageRuleReferences(md, msgRules, env) converts cel/cel_expression/oneof into one deterministic []messageRuleRef slice so checkMessageRuleReferences' downstream loop (messageRuleFieldUnavailable + refs/failures accumulation) runs identically regardless of carrier — the single convergence point CR-01's root cause was missing."
    - "Presence-vs-value option storage: messageRulesSet/messageRulesTrigger replace a bare bool, mirroring overridden's own presence-vs-value split, so an undeclared trigger value is distinguishable from 'never called' and can be validated rather than silently coerced."
    - "Reflective declaration-surface guards: both new exhaustiveness tests enumerate protobuf descriptor fields (and, for FieldRules' `type` oneof, the oneof's own member list) rather than hand-typing a member list, so a future protovalidate release adding a member is caught by descriptor inspection, not by someone remembering to update a list."

key-files:
  created:
    - mixinforproto/internal/difftest/ent/messagerulecelexpression.go (generated)
    - mixinforproto/internal/difftest/ent/messageruleoneof.go (generated)
  modified:
    - proto/mixinforprototest/v1/messagerules.proto
    - mixinforproto/messagerules.go
    - mixinforproto/messagerules_test.go
    - mixinforproto/option.go
    - mixinforproto/hooks_test.go
    - mixinforproto/corpus_test.go
    - mixinforproto/internal/difftest/messagerules_test.go
    - mixinforproto/internal/difftest/ent/schema/messagerules.go
    - proto/descriptorset.binpb (generated)
    - mixinforproto/internal/gen/mixinforprototestv1/messagerules.pb.go (generated)

key-decisions:
  - "messageRuleReferences(md, msgRules, env) accepts an explicit *validate.MessageRules parameter rather than resolving one from md's own descriptor — this is what let the oneof-unknown-field-name edge case be unit-tested directly (a hand-built MessageOneofRule against MessageRuleOk's descriptor) with no new proto fixture needed."
  - "oneof failures use protovalidate's own fixed 'message.oneof' RuleId (verified against buf.build/go/protovalidate@v1.2.0/message_oneof.go) rather than inventing an identifier; cel_expression failures reuse fieldmap.go's celRuleResidual('', expr) expression-fingerprint fallback for schema-load failure text — but the RUNTIME RuleId protovalidate itself assigns a cel_expression rule is the raw expression string (expressionsToRules), a documented, expected divergence since the fingerprint is only ever used before any protovalidate output exists."
  - "A message-level oneof rule's constraint CLASS is excepted from corpus_test.go's Part A ground truth (constraintClassExceptions['oneof']) rather than given a Part B witness fixture: derive.go never consults message-level rules at all by design (messagerules.go's own doc comment), so no code path can write SourceField/SourceMessage provenance for it regardless of which fields a fixture excludes. Real coverage exists via messagerules_test.go/corpusCoverage instead — just not through the provenance channel this specific guard inspects."
  - "Added a 5th proto fixture (MessageRuleMultiCarrierExcludedRef) beyond the plan's literal 4, because the plan's own acceptance criteria requires a 'two different carriers, one pass' test that no single-carrier fixture can produce (Rule 2: missing critical test coverage the plan's own acceptance criteria demands)."

patterns-established:
  - "Edge-case tests that need a shape no committed .proto fixture provides can call a normalization helper directly with a hand-built protovalidate rule struct, rather than adding a proto fixture purely to exercise one internal code path — the messageRuleReferences signature was designed for exactly this."

requirements-completed: [VAL-04, VAL-07, VAL-08, PIPE-05, PIPE-06]

coverage:
  - id: D1
    description: "CR-01 closed: cel_expression and oneof MessageRules carriers are enumerated by D-10's schema-load reference gate and seed extraFields identically to cel"
    requirement: VAL-08
    verification:
      - kind: unit
        ref: "mixinforproto/messagerules_test.go#TestBuildHookState_MessageRuleCelExpressionExcludedRefFailsSchemaLoad"
        status: pass
      - kind: unit
        ref: "mixinforproto/messagerules_test.go#TestBuildHookState_MessageRuleOneofExcludedRefFailsSchemaLoad"
        status: pass
      - kind: unit
        ref: "mixinforproto/messagerules_test.go#TestBuildHookState_MessageRuleCelExpressionOverriddenRefFailsSchemaLoad"
        status: pass
      - kind: unit
        ref: "mixinforproto/messagerules_test.go#TestBuildHookState_MessageRuleOneofOverriddenRefFailsSchemaLoad"
        status: pass
      - kind: unit
        ref: "mixinforproto/messagerules_test.go#TestBuildHookState_MessageRuleMultiCarrierExcludedRefFailsInOnePass"
        status: pass
      - kind: unit
        ref: "mixinforproto/messagerules_test.go#TestMessageRuleReferences_OneofUnknownFieldNameIsCollectedFailure"
        status: pass
      - kind: unit
        ref: "mixinforproto/messagerules_test.go#TestBuildHookState_MessageRuleCelExpressionNoFieldRefIsLegalNoOp"
        status: pass
    human_judgment: false
  - id: D2
    description: "WR-04 closed: WithMessageRules no longer discards its trigger — an undeclared value fails schema load naming the option and value"
    requirement: VAL-08
    verification:
      - kind: unit
        ref: "mixinforproto/messagerules_test.go#TestWithMessageRules_UndeclaredTriggerFailsSchemaLoad"
        status: pass
      - kind: unit
        ref: "mixinforproto/messagerules_test.go#TestWithMessageRules_UndeclaredTriggerFailsEvenWithRealRules"
        status: pass
      - kind: unit
        ref: "mixinforproto/messagerules_test.go#TestWithMessageRules_OnCreateStillWorks"
        status: pass
    human_judgment: false
  - id: D3
    description: "PIPE-06 differential harness no longer blind to either alternate carrier — real ent.Client Create/Update against both, violation identity compared to protovalidate.Validate's own boundary verdict"
    requirement: PIPE-06
    verification:
      - kind: integration
        ref: "mixinforproto/internal/difftest/messagerules_test.go#TestMessageRuleCelExpression_CreateRejectsViolatingEntity"
        status: pass
      - kind: integration
        ref: "mixinforproto/internal/difftest/messagerules_test.go#TestMessageRuleCelExpression_UpdateNotSubjectToMessageRuleEnforcement"
        status: pass
      - kind: integration
        ref: "mixinforproto/internal/difftest/messagerules_test.go#TestMessageRuleOneof_CreateRejectsViolatingEntity"
        status: pass
      - kind: integration
        ref: "mixinforproto/internal/difftest/messagerules_test.go#TestMessageRuleOneof_UpdateNotSubjectToMessageRuleEnforcement"
        status: pass
    human_judgment: false
  - id: D4
    description: "The root cause behind CR-01/CR-02/CR-03 is structurally guarded: an unhandled, unexempted member of either declaration surface fails a named test with the member's name in the output"
    requirement: PIPE-05
    verification:
      - kind: unit
        ref: "mixinforproto/messagerules_test.go#TestMessageRulesDeclarationSurfaceIsFullyHandled"
        status: pass
      - kind: unit
        ref: "mixinforproto/hooks_test.go#TestFieldRulesDeclarationSurfaceIsFullyHandled"
        status: pass
      - kind: other
        ref: "both guards rehearsed in a scratch copy (removing oneof / removing ignore) — see 'Verification Rehearsal' section below"
        status: pass
    human_judgment: false

duration: 32min
completed: 2026-08-15
status: complete
---

# Phase 3 Plan 08: cel_expression/oneof MessageRules Carriers + WithMessageRules Trigger Summary

**Closed CR-01 (checkMessageRuleReferences now enumerates all three protovalidate MessageRules carriers, not just `cel`) and WR-04 (WithMessageRules(trigger) validates its argument instead of discarding it), plus two reflective declaration-surface exhaustiveness guards that fail by name when protovalidate grows a member this package doesn't handle.**

## Performance

- **Duration:** 32 min
- **Started:** 2026-08-15T12:07:59Z
- **Completed:** 2026-08-15T12:39:00Z
- **Tasks:** 3
- **Files modified:** 34 (across 4 commits — 3 task commits + 1 gofmt-only fix commit)

## Accomplishments

- `checkMessageRuleReferences` (D-10's schema-load reference gate) now walks all three `buf.validate.MessageRules` carriers — `cel`, `cel_expression`, `oneof` — through a new `messageRuleReferences` normalization helper that converges every carrier onto ONE downstream loop. A message rule declared via `cel_expression` or `oneof` referencing an `Exclude`d or `Override`n field now fails schema load exactly like the pre-existing `cel` carrier did; before this plan it loaded silently and evaluated against a fabricated proto3-zero value (a phantom verdict).
- `oneof`'s field-name list has no CEL compiler validating it, unlike the two CEL carriers — an unresolvable name is now a collected schema-load failure, never a silent skip.
- `WithMessageRules(trigger)` stores and validates the exact trigger value passed (`messageRulesSet`/`messageRulesTrigger` in `option.go`, mirroring `overridden`'s presence-vs-value discipline). An undeclared `MessageRuleTrigger` fails schema load naming `WithMessageRules` and the offending numeric value — closing WR-04.
- Two new declaration-surface exhaustiveness guards (`TestMessageRulesDeclarationSurfaceIsFullyHandled`, `TestFieldRulesDeclarationSurfaceIsFullyHandled`) reflectively enumerate the protobuf descriptors of `MessageRules`/`FieldRules` and fail, naming the member, if anything is neither handled nor exempted with a written design reason. Both rehearsed live in a scratch copy: removing `oneof`/`ignore` from their respective handled sets produces the expected named failure.
- Real `ent.Client` differential fixtures for both new carriers (`MessageRuleCelExpression`, `MessageRuleOneof` schemas) extend PIPE-06's coverage, comparing violation identity against `protovalidate.Validate`'s own boundary verdict rather than a hard-coded expectation.
- Every pre-fix failure documented below was independently rehearsed in a scratch copy (never the committed tree) before the fix landed.

## Task Commits

Each task was committed atomically:

1. **Task 1: Enumerate the cel_expression and oneof carriers in D-10's schema-load reference gate** — `bb81f07` (feat)
2. **Task 2: WithMessageRules honors its trigger, and the corpus and differential harness see both new carriers** — `6d562cc` (feat)
3. **Task 3: Declaration-surface exhaustiveness guards** — `25dc4fd` (test)

**Follow-up fix:** `e945df2` (chore) — `gofmt` import-ordering fix on `internal/difftest/messagerules_test.go`, discovered by a repo-wide `gofmt -l` sweep after Task 3. Purely mechanical, no behavior change.

## Files Created/Modified

- `proto/mixinforprototest/v1/messagerules.proto` — 6 new corpus fixtures: `MessageRuleCelExpressionOk`/`MessageRuleCelExpressionExcludedRef` (cel_expression carrier), `MessageRuleOneofOk`/`MessageRuleOneofExcludedRef` (oneof carrier), `MessageRuleMultiCarrierExcludedRef` (two-carriers-one-pass acceptance test), `MessageRuleCelExpressionNoFieldRef` (VAL-08/empty edge probe)
- `mixinforproto/messagerules.go` — `messageRuleReferences`/`messageRuleRef` (the three-carrier convergence point), trigger validation at the top of `checkMessageRuleReferences`, updated doc comments
- `mixinforproto/messagerules_test.go` — 20 new tests: carrier gap-closure (Task 1), WR-04 (Task 2), Guard A (Task 3)
- `mixinforproto/option.go` — `messageRulesSet`/`messageRulesTrigger` replace the bare `messageRules bool`; `messageRulesTriggerValue()` accessor; `WithMessageRules` stores instead of discarding
- `mixinforproto/hooks_test.go` — Guard B (`TestFieldRulesDeclarationSurfaceIsFullyHandled`)
- `mixinforproto/corpus_test.go` — coverage claims for the 6 new fixtures; Part A message-level scan extended for `cel_expression`/`oneof`; `oneof` added to `constraintClassExceptions` with a written design reason
- `mixinforproto/internal/difftest/ent/schema/messagerules.go` — `MessageRuleCelExpression`/`MessageRuleOneof` ent schemas, both `WithMessageRules(OnCreate)`
- `mixinforproto/internal/difftest/messagerules_test.go` — real-client differential proofs for both new carriers
- `proto/descriptorset.binpb`, `mixinforproto/internal/gen/mixinforprototestv1/messagerules.pb.go`, `mixinforproto/internal/difftest/ent/*` — regenerated

## Decisions Made

- `messageRuleReferences(md, msgRules, env)` takes an explicit `*validate.MessageRules` rather than resolving one itself from `md` — this let the oneof-unknown-field-name edge case be unit-tested directly against a hand-built `MessageOneofRule`, with no new proto fixture needed just to exercise that one path.
- `oneof` failures use protovalidate's own fixed `"message.oneof"` RuleId (verified against `buf.build/go/protovalidate@v1.2.0/message_oneof.go`'s source this session), matching the discipline of never inventing an identifier protovalidate itself owns.
- A message-level `oneof` rule's constraint class is excepted from `corpus_test.go`'s Part A ground truth rather than chased with a Part B witness fixture — see "Part A sanity list" below for the full reasoning; this was the one place the plan's own "let the test tell you" instruction genuinely produced a design decision rather than a mechanical fixture addition.
- Added a 5th proto fixture, `MessageRuleMultiCarrierExcludedRef`, beyond the plan's literal 4, to satisfy the plan's own acceptance criterion requiring a "two different carriers, one pass" test — see Deviations below.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing Critical] Added a 5th proto fixture (MessageRuleMultiCarrierExcludedRef) for the "two carriers, one pass" acceptance criterion**
- **Found during:** Task 1
- **Issue:** Task 1's action text lists exactly 4 new fixtures (2 per single carrier), but its own acceptance criteria requires "A named test asserts offenders across two different carriers on one message are reported in one pass" — no single-carrier fixture can produce that shape.
- **Fix:** Added `MessageRuleMultiCarrierExcludedRef` (a `cel_expression` rule and a `oneof` rule both referencing the same excluded field "hi"), plus `TestBuildHookState_MessageRuleMultiCarrierExcludedRefFailsInOnePass` asserting 2 failures in one `derivationError`, sorted deterministically.
- **Files modified:** `proto/mixinforprototest/v1/messagerules.proto`, `mixinforproto/messagerules_test.go`, generated stubs
- **Verification:** Test passes; failure text names both carriers' rule identifiers in deterministic order.
- **Committed in:** `bb81f07` (Task 1 commit)

**2. [Rule 2 - Missing Critical] Corpus coverage claims added in Task 1's own commit**
- **Found during:** Task 1
- **Issue:** Task 1's own `<verify>` includes `make test`, which fails immediately once the 6 new corpus messages exist without `corpusCoverage` entries — the plan's Task 2 formally owns "corpus registration," but that's too late for Task 1's own verify gate (same shape as 03-06's own precedented deviation).
- **Fix:** Added `corpusCoverage` entries for all 6 new fixtures in Task 1's commit.
- **Files modified:** `mixinforproto/corpus_test.go`
- **Verification:** `TestCorpusMessagesHaveRecordedCoverage` passes after Task 1's commit alone.
- **Committed in:** `bb81f07` (Task 1 commit)

**3. [Rule 1 - Bug] gofmt import-ordering fix**
- **Found during:** post-Task-3 repo-wide `gofmt -l` sweep
- **Issue:** `mixinforproto/internal/difftest/messagerules_test.go`'s import block (added in Task 2) was not gofmt-ordered.
- **Fix:** `gofmt -w` on the one file.
- **Files modified:** `mixinforproto/internal/difftest/messagerules_test.go`
- **Verification:** `gofmt -l` clean on every file this plan touched (two pre-existing, untouched-by-this-plan findings elsewhere in the repo — `entc/maskcheck.go`, `runtime/errormap_test.go` — are out of scope per the deviation rules' scope boundary).
- **Committed in:** `e945df2` (separate chore commit, since it landed after Task 3's own commit)

---

**Total deviations:** 3 auto-fixed (2 missing-critical, 1 bug). No architectural changes, no scope creep — every deviation is either a correctness requirement of the plan's own acceptance criteria or a mechanical build-integrity fix.
**Impact on plan:** All auto-fixes necessary for the plan's own `<verify>`/`<acceptance_criteria>` gates to pass exactly as the plan specifies them.

## Issues Encountered

- `go`/`buf`/`protoc-gen-go`/`protoc-gen-connect-go` were not on `$PATH` by default in this sandbox; resolved by prepending `/usr/local/go1.24.7/bin` (matching `mixinforproto/go.mod`'s pin) and `/root/go/bin` (the buf toolchain) to every shell invocation, matching 03-06/03-07's own documented workaround for this sandbox.
- Pre-fix rehearsals (Task 1's CR-01 reproduction, Task 3's two guard-failure rehearsals) were run in throwaway scratch copies under the session scratchpad directory, never the committed tree, each removed immediately after recording the observed output below.

### Pre-fix failure output (rehearsed, scratch copies only)

**Task 1 (messagerules.go reverted to pre-fix, cel-only carrier walk):**
```
PRE-FIX BUG REPRODUCED: schema load silently succeeded (hs=&{msgName:mixinforprototest.v1.MessageRuleCelExpressionExcludedRef ... evaluators:[] messageRulesOnCreate:true}) even though the cel_expression rule references excluded field "hi"
PRE-FIX BUG REPRODUCED: schema load silently succeeded (hs=&{msgName:mixinforprototest.v1.MessageRuleOneofExcludedRef ... evaluators:[] messageRulesOnCreate:true}) even though the oneof rule references excluded field "hi"
```
This is CR-01's phantom-verdict class reproduced verbatim: `hs.evaluators` is empty, meaning neither excluded field would ever be reverse-converted — a message-level rule referencing it would be evaluated against a fabricated proto3 zero, with no schema-load error to catch it first.

**Task 3, Guard A (removing "oneof" from `messageRulesHandledCarriers`):**
```
messagerules_test.go:516: buf.validate.MessageRules declares member(s) [oneof] that checkMessageRuleReferences neither handles nor exempts — handle the carrier in messagerules.go's messageRuleReferences, or record an exemption in messageRulesExemptedMembers with a design reason. Never delete this assertion.
--- FAIL: TestMessageRulesDeclarationSurfaceIsFullyHandled (0.00s)
```

**Task 3, Guard B (removing "ignore" from `fieldRulesConsultedModifier`):**
```
hooks_test.go:985: buf.validate.FieldRules declares member(s) [ignore] not accounted for in any disposition (locally compiled, consulted modifier, delegated wholesale) — handle it in hooks.go's buildHookState/evaluate, or record its disposition in one of this test's three named sets. Never delete this assertion.
--- FAIL: TestFieldRulesDeclarationSurfaceIsFullyHandled (0.00s)
```

### Part A sanity list

The hand-maintained `want` list in `TestCorpusExercisesEveryProtovalidateConstraintClass` did **NOT** need editing, but only because of a design decision rather than a mechanical outcome: extending Part A's message-level scan to count `cel_expression` toward the `"cel"` ground truth added nothing new (field-level `fieldRuleClasses` already folds the two). Extending it to count `oneof` toward its own `"oneof"` class DID initially produce a Part A failure (`got 8 classes, want 7`) and a Part B failure (`zero corpus witness: oneof`) — but the correct fix, per this guard's own discipline ("closed by adding coverage... or a design reason as real as enum's"), was to except `"oneof"` from ground truth with a written reason: a message-level `oneof` rule is **structurally invisible** to Part B's witness mechanism (`SourceField`/`SourceMessage` provenance), because `derive.go` never consults message-level rules at all, by design (`messagerules.go`'s own doc comment: "never from derive.go"). No fixture or `Exclude(...)` path can create provenance that no code path writes. Real, machine-checked coverage for the `oneof` carrier exists via `messagerules_test.go`'s own tests and `corpusCoverage`'s entries — just not through this specific guard's provenance channel. After the exception, the `want` list stayed at its original 7 classes, unchanged.

### cel_expression RuleId divergence (flagged assumption resolution)

Confirmed live via the real `ent.Client` differential test: protovalidate's own RuleId for a `cel_expression`-declared message rule with no explicit id is the **raw expression string itself** (`this.lo <= this.hi`) — `buf.build/go/protovalidate@v1.2.0/builder.go`'s `expressionsToRules` sets `Id: proto.String(expr)`. This **differs** from `celRuleResidual("", expr)`'s schema-load-time fingerprint (`"cel." + expr`), which `messageRuleReferences` uses only for the *schema-load failure text*, where no protovalidate output exists yet. This is the expected, plan-anticipated divergence, not a bug: `TestMessageRuleCelExpression_CreateRejectsViolatingEntity` reads the expected RuleId from `protovalidate.Validate`'s own output rather than hard-coding either string, so the test is correct regardless of which identifier scheme is "right."

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- CR-01 and WR-04 are closed; 03-VERIFICATION.md's outstanding gap-closure items for this phase are now all addressed (03-06 closed CR-02/CR-03, 03-07 closed WR-07/VAL-07's enforcement gap, this plan closes CR-01/WR-04).
- The declaration-surface exhaustiveness guards (Guard A/B) are the phase's structural backstop: the NEXT protovalidate release adding a `MessageRules` or `FieldRules` member will fail a named test in this repo before it can silently reproduce the CR-01/CR-02/CR-03 class.
- No new `mixinforproto/go.mod` requirement landed (verified via `git diff mixinforproto/go.mod mixinforproto/go.sum` — empty; `make test-standalone` and `make check-dep-parity` both green).
- Phase 03 (validation-fidelity) has no further plans in its roadmap wave sequence per `03-06-SUMMARY.md`'s "Next Phase Readiness" — this was the last remaining gap-closure plan.

## Self-Check: PASSED

All created/modified files verified present on disk; all four commit hashes (`bb81f07`, `6d562cc`, `25dc4fd`, `e945df2`) verified present in `git log --oneline --all`.

---
*Phase: 03-validation-fidelity*
*Completed: 2026-08-15*
