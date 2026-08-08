---
phase: 01-mixinforproto-core
plan: 09
subsystem: testing
tags: [ent, protobuf, protoreflect, corpus-coverage, gap-closure, test-infrastructure]

# Dependency graph
requires:
  - phase: 01-mixinforproto-core (plans 01-06, 01-08)
    provides: "classify()'s IsList()/classRepeated branch (01-06), classifyRequired's presence-first branch order and delegatingFormatValidator's per-field violation filtering (01-08) — the three code paths this plan's guards structurally protect"
provides:
  - "mixinforproto/corpus_test.go: corpusMessages() registry walk (IsMapEntry-filtered, sorted, D-24) shared by all three guards"
  - "TestCorpusExercisesEveryFieldClass: exhaustiveness of fieldClassNames against classify()'s fieldClass constant block, plus per-class corpus coverage"
  - "TestCorpusExercisesEveryRequiredResult: exhaustiveness of requiredResultNames against classifyRequired()'s requiredResult constant block, plus per-outcome corpus coverage — with requiredExactPresence additionally split by hasNotEmpty(string/bytes vs other), the exact axis gap 3 hid in"
  - "TestCorpusMessagesHaveRecordedCoverage + corpusCoverage: every mixinforprototest.v1 message has a golden:/test: coverage claim, checked bidirectionally, golden claims verified to exist on disk"
  - "CONTRIBUTING.md '## Test corpus coverage' section documenting the convention and the add-coverage-never-relax rule"
affects: ["Phase 2 (entc extension) - any future corpus message addition is now guarded by these three checks", "Phase 3 - the same corpusMessages()/corpusCoverage pattern is reusable for Tier 2 CEL and Tier 3 map-constraint corpus additions"]

actuals:
  tokens: 6800
  tasks: 3
  commits: 3

tech-stack:
  added: []
  patterns:
    - "corpusMessages(t): a single, package-shared registry walk (protoregistry.GlobalFiles.RangeFilesByPackage, recursive over nested messages, IsMapEntry-filtered, FullName()-sorted) that every corpus-adequacy guard in this file builds on, rather than each guard re-deriving its own message set."
    - "Exhaustiveness-then-coverage, two-step guard shape: an explicit name table asserted exhaustive against the source constant block's own length, THEN a corpus-coverage assertion over that table — applied identically to fieldClass and requiredResult, so a newly added enum value is caught structurally (missing table entry) independently of whether a fixture exists for it."
    - "Guards must witness the SAME axis a real defect used, not just outcome existence: TestCorpusExercisesEveryRequiredResult's requiredExactPresence check specifically requires a hasNotEmpty=true (string/bytes) witness AND a hasNotEmpty=false witness, because a plain per-outcome check stays green under gap 3's original defect (a non-string field alone satisfies presence existence regardless of branch order) — see Deviations below."

key-files:
  created:
    - mixinforproto/corpus_test.go
  modified:
    - CONTRIBUTING.md

key-decisions:
  - "requiredExactPresence coverage is split by hasNotEmpty (string/bytes vs. other), not just asserted as a single existence check across the whole corpus — this is a strengthening beyond the plan's literal Part-B text, made necessary by the plan's own mandatory counterfactual acceptance criterion (see Deviations)."
  - "Nested (maps.proto) and Inner (messages.proto) are reference-only proto message types — never derived directly on their own, only referenced as a map-value type and a message-field type respectively. Their corpusCoverage claim is 'test:TestGolden', naming the top-level test whose Maps/Messages subtests exercise their being skipped, rather than inventing a golden fixture that derives them standalone (which nothing in the production code path does)."
  - "corpusCoverage's golden claims store the fixture's base name (e.g. 'wkt'), and the guard appends '.golden' + resolves against mixinforproto/testdata/ itself, rather than storing the full path — keeps entries short and matches assertGolden's own name argument shape."

requirements-completed: [MIX-02, VAL-01, VAL-03]

coverage:
  - id: D1
    description: "Every value classify() can return is produced by at least one corpus field, and the guard's own name table is proven exhaustive against the fieldClass constant block — a newly added fieldClass with no table entry or no covering fixture fails the suite"
    requirement: "MIX-02"
    verification:
      - kind: unit
        ref: "mixinforproto/corpus_test.go#TestCorpusExercisesEveryFieldClass"
        status: pass
    human_judgment: false
  - id: D2
    description: "Every value classifyRequired() can return is produced by a real corpus fixture under the real (optional,required,hasNotEmpty) triple the builders pass; requiredExactPresence is additionally witnessed separately for hasNotEmpty=true and hasNotEmpty=false, proven via the mandatory counterfactual to catch gap 3's original branch order"
    requirement: "VAL-03"
    verification:
      - kind: unit
        ref: "mixinforproto/corpus_test.go#TestCorpusExercisesEveryRequiredResult"
        status: pass
    human_judgment: false
  - id: D3
    description: "Every message declared in mixinforprototest.v1 has a recorded, bidirectionally-checked coverage claim (golden fixture existence verified on disk, or a named test); an unlisted message, a stale entry, and a missing golden file are each independently detectable"
    requirement: "VAL-01"
    verification:
      - kind: unit
        ref: "mixinforproto/corpus_test.go#TestCorpusMessagesHaveRecordedCoverage"
        status: pass
    human_judgment: false
  - id: D4
    description: "The corpus-coverage convention (what the three guards check, why, how to add a corpus message, and the add-coverage-never-relax rule) is documented in CONTRIBUTING.md where a contributor adding a corpus message will meet it"
    verification:
      - kind: other
        ref: "CONTRIBUTING.md '## Test corpus coverage' section (grep-verified: corpusCoverage, scripts/generate-stubs.sh, make check-stubs all present)"
        status: pass
    human_judgment: false

duration: ~15min
completed: 2026-08-08
status: complete
---

# Phase 1 Plan 09: Corpus-Adequacy Guards — Gap Closure Root-Cause Summary

**Three structural test guards (`mixinforproto/corpus_test.go`) turn "the corpus does not exercise this shape" from a silent condition into a build failure, with all three (plus the mandatory gap-3 counterfactual) observed failing and passing on live edits — not merely asserted to work.**

## Performance

- **Duration:** ~15 min (commit span 14:01:58Z–14:09:12Z; reading/verification time not fully captured in that span)
- **Started:** 2026-08-08T13:55:18Z (baseline commit before this plan)
- **Completed:** 2026-08-08T14:10:08Z
- **Tasks:** 3 (all completed)
- **Files modified:** 2 (1 created, 1 modified)

## Accomplishments

- `mixinforproto/corpus_test.go`'s `corpusMessages()` walks `protoregistry.GlobalFiles.RangeFilesByPackage("mixinforprototest.v1")`, recurses nested messages, filters `IsMapEntry()` synthetic messages, and returns a `FullName()`-sorted, deterministic message set shared by all three guards.
- `TestCorpusExercisesEveryFieldClass` closes the root cause behind gap 1 (repeated-cardinality, 01-VERIFICATION.md): every `fieldClass` value `classify()` can return is now proven both named (exhaustiveness table) and reachable (real corpus fixture) — never calling `derive` (several corpus messages deliberately fail full derivation).
- `TestCorpusExercisesEveryRequiredResult` closes the root cause behind gap 3 (VAL-03/CR-03): every `classifyRequired()` outcome is proven reachable through the real `(optional, required, hasNotEmpty)` triple the builders pass — with `requiredExactPresence` specifically required to be witnessed by both a `hasNotEmpty=true` (string/bytes) and a `hasNotEmpty=false` field, the exact split gap 3 lived on (see Deviations for why plain existence-checking was insufficient).
- `TestCorpusMessagesHaveRecordedCoverage` + `corpusCoverage` close the root cause behind gap 2 (VAL-01/CR-02): all 43 `mixinforprototest.v1` messages carry a recorded, bidirectionally-checked coverage claim (`golden:<name>` — verified to exist on disk — or `test:<TestName>`), so a future single-field-by-convention corpus property can never again silently poison a guard's assumptions.
- `CONTRIBUTING.md`'s new `## Test corpus coverage` section documents the three guards, the Phase 1 finding that motivated them, the exact steps to add a corpus message, the "add coverage, never relax the assertion" rule, and a pointer to `make check-stubs` (the companion staleness gate from 01-07).
- Every detection and counterfactual proof named in the plan's acceptance criteria was executed against the real guards on live, temporary edits and recorded verbatim below — not merely asserted.

## Guard detection proofs

All scenarios below were executed from the repo root against the real guards (`cd mixinforproto && go test ./... -run '<test>' -count=1 -v`), then reverted; `git status --porcelain fieldmap.go` / `corpus_test.go` was confirmed clean after each revert.

### Guard 1 — TestCorpusExercisesEveryFieldClass

**Proof A — new class + table entry, no fixture (must FAIL naming the uncovered class):**
Appended `classDetectionProbe` to `fieldmap.go`'s `fieldClass` block and a matching `corpus_test.go` table entry (no corpus fixture exercises it).

```
$ go test ./... -run 'TestCorpusExercisesEveryFieldClass' -count=1 -v
=== RUN   TestCorpusExercisesEveryFieldClass/every_fieldClass_value_is_produced_by_at_least_one_corpus_field
    corpus_test.go:156: no corpus field classifies as classDetectionProbe (fieldClass=8) — add a proto/mixinforprototest/v1/*.proto fixture exercising this shape and record its coverage; do NOT delete or weaken this assertion
--- FAIL: TestCorpusExercisesEveryFieldClass (0.00s)
```
Exit status: `1`. Reverted both edits; `git status --porcelain fieldmap.go` confirmed empty; test passes again.

**Proof B — table entry deleted, constants untouched (must FAIL naming the missing registration):**
Deleted `classEnum`'s entry from `fieldClassNames` only.

```
$ go test ./... -run 'TestCorpusExercisesEveryFieldClass' -count=1 -v
=== RUN   TestCorpusExercisesEveryFieldClass/table_is_exhaustive_against_the_fieldClass_constant_block
    corpus_test.go:131: fieldClassNames has 7 entries, want 8 (== classRepeated+1) — a new classification was added to fieldmap.go's fieldClass constant block without registering it here; add the missing entry, do not delete this assertion
--- FAIL: TestCorpusExercisesEveryFieldClass (0.00s)
```
Exit status: `1`. Reverted; test passes again.

### Guard 2 — TestCorpusExercisesEveryRequiredResult

**Proof A — sixth outcome + table entry, no fixture (must FAIL naming the uncovered outcome):**
Appended `requiredDetectionProbe` to `fieldmap.go`'s `requiredResult` block and a matching table entry.

```
$ go test ./... -run 'TestCorpusExercisesEveryRequiredResult' -count=1 -v
=== RUN   TestCorpusExercisesEveryRequiredResult/table_is_exhaustive_against_the_requiredResult_constant_block
    corpus_test.go:200: requiredResultNames has 6 entries, want 5 (== requiredOptionalPlain+1) — a new outcome was added to fieldmap.go's requiredResult constant block without registering it here; add the missing entry, do not delete this assertion
=== RUN   TestCorpusExercisesEveryRequiredResult/every_requiredResult_outcome_is_reachable_through_a_real_corpus_fixture
    corpus_test.go:249: no corpus field produces classifyRequired outcome requiredDetectionProbe (requiredResult=5) — add a fixture with the (optional,required,kind) combination that reaches it, then record it in this test's coverage comment; do NOT delete or weaken this assertion
--- FAIL: TestCorpusExercisesEveryRequiredResult (0.00s)
```
Exit status: `1`. Reverted; `git status --porcelain fieldmap.go` confirmed empty; test passes again.

**Counterfactual proof — gap 3's original branch order (highest-value proof in this plan):**
Restored `classifyRequired`'s pre-01-08 branch order (`required && hasNotEmpty` checked BEFORE `optional && required`):

```
$ go test ./... -run 'TestCorpusExercisesEveryRequiredResult|TestRequiredTranslation' -count=1 -v
=== RUN   TestCorpusExercisesEveryRequiredResult/every_requiredResult_outcome_is_reachable_through_a_real_corpus_fixture
    corpus_test.go:272: requiredResult requiredExactPresence covered by mixinforprototest.v1.RequiredOptionalNonString.value
    corpus_test.go:272: requiredResult requiredExactNotEmpty covered by mixinforprototest.v1.RequiredOptionalBytes.value
    corpus_test.go:277: requiredExactPresence is never witnessed by a field with hasNotEmpty=true — this is exactly the axis gap 3 hid in (01-VERIFICATION.md/CR-03): presence must win for EVERY kind, not just the ones without a hasNotEmpty=true builder. Add an `optional <kind> ... [(buf.validate.field).required = true]` fixture for this hasNotEmpty value.
--- FAIL: TestCorpusExercisesEveryRequiredResult (0.00s)
=== RUN   TestRequiredTranslation/optional_string:_yields_presence,_not_NotEmpty
    validate_test.go:368: want zero validators (presence, not NotEmpty), got 1
=== RUN   TestRequiredTranslation/optional_bytes:_yields_presence,_not_NotEmpty
    validate_test.go:396: want zero validators (presence, not NotEmpty), got 1
--- FAIL: TestRequiredTranslation (0.00s)
FAIL
```
Exit status: `1`. **Both** the corpus guard and plan 01-08's `TestRequiredTranslation` presence subtests failed, exactly as the plan's mandatory counterfactual requires — this demonstrates the guard would have caught gap 3 before it shipped. Reverted `classifyRequired` to its correct branch order; `git status --porcelain fieldmap.go` confirmed empty; both tests pass again.

**Why the guard needed strengthening to make this proof succeed:** a plain "is `requiredExactPresence` produced by ANY corpus field" check (the literal minimum the plan's action text describes) stays green under gap 3's original branch order — `RequiredOptionalNonString.value` (an `optional int32`, `hasNotEmpty=false` since it is not string/bytes) still reaches `requiredExactPresence` regardless of branch order, because the reordering only changes behavior when `hasNotEmpty=true`. A guard built only on outcome existence would have passed even under the exact defective code the plan requires it to catch. `TestCorpusExercisesEveryRequiredResult` therefore additionally tracks `producedPresenceByHasNotEmpty`, requiring a witness for both `hasNotEmpty=true` and `hasNotEmpty=false` — this is what makes the counterfactual proof above actually fail, and is the one place this plan's implementation goes beyond the plan's literal Part-B text (documented here per Rule 2: the mandatory counterfactual acceptance criterion cannot be satisfied without it).

### Guard 3 — TestCorpusMessagesHaveRecordedCoverage

**Proof A — unlisted message (must FAIL naming the message and its .proto file):**
Deleted the `"mixinforprototest.v1.Wkt"` entry from `corpusCoverage`.

```
$ go test ./... -run 'TestCorpusMessagesHaveRecordedCoverage' -count=1 -v
=== RUN   TestCorpusMessagesHaveRecordedCoverage/every_corpus_message_has_a_recorded_coverage_claim
    corpus_test.go:397: corpus message(s) with no recorded coverage claim in corpusCoverage — "no coverage" is a real answer only if it is written down. Add a golden fixture or a named test, then record the claim (golden:<name> or test:<TestName>):
          mixinforprototest.v1.Wkt (declared in mixinforprototest/v1/wkt.proto)
--- FAIL: TestCorpusMessagesHaveRecordedCoverage (0.00s)
```
Exit status: `1`. Reverted; test passes again.

**Proof B — stale entry (must FAIL reporting the orphan):**
Added `"mixinforprototest.v1.DoesNotExist": "golden:does_not_exist"` to `corpusCoverage`.

```
$ go test ./... -run 'TestCorpusMessagesHaveRecordedCoverage' -count=1 -v
=== RUN   TestCorpusMessagesHaveRecordedCoverage/every_corpusCoverage_entry_names_a_message_that_still_exists
    corpus_test.go:412: corpusCoverage entry(ies) name a message that is no longer in the registry — renamed or removed; remove or update the stale entry:
          mixinforprototest.v1.DoesNotExist
--- FAIL: TestCorpusMessagesHaveRecordedCoverage (0.00s)
```
Exit status: `1`. Reverted; test passes again.

**Proof C — missing golden (must FAIL, isolated from Proof A/B):**
Pointed the existing `Wkt` entry at a nonexistent golden name (`golden:wkt_detection_probe`).

```
$ go test ./... -run 'TestCorpusMessagesHaveRecordedCoverage' -count=1 -v
=== RUN   TestCorpusMessagesHaveRecordedCoverage/every_golden:_claim_resolves_to_an_existing_testdata_fixture
    corpus_test.go:429: corpusCoverage golden claim(s) point at a nonexistent testdata fixture:
          mixinforprototest.v1.Wkt -> golden:wkt_detection_probe (missing testdata/wkt_detection_probe.golden)
--- FAIL: TestCorpusMessagesHaveRecordedCoverage (0.00s)
```
Exit status: `1`. Reverted; test passes again. (Confirmed this failure mode is independent of Proof A/B: renaming an existing entry's golden value fails only the third subtest, not the first two.)

## Task Commits

Each task was committed atomically:

1. **Task 1: Build the corpus registry walk and guard every classify() branch** - `0bf8e5a` (test)
2. **Task 2: Guard every classifyRequired() outcome — the branch VAL-03's defect lived in** - `484c87b` (test)
3. **Task 3: Require every corpus message to carry a named coverage claim, and write the convention down** - `7e71f54` (test)

**Plan metadata:** commit pending below (docs: complete plan)

## Files Created/Modified

- `mixinforproto/corpus_test.go` - NEW. `corpusMessages()` helper; `fieldClassNames`/`TestCorpusExercisesEveryFieldClass`; `requiredResultNames`/`TestCorpusExercisesEveryRequiredResult`; `corpusCoverage`/`TestCorpusMessagesHaveRecordedCoverage`.
- `CONTRIBUTING.md` - New `## Test corpus coverage` section: what the guards check and why, steps to add a corpus message, the add-coverage-never-relax rule, and a pointer to `make check-stubs`.

## Decisions Made

- **`requiredExactPresence` coverage split by `hasNotEmpty`.** The plan's literal Part-B text ("assert all five outcomes are produced by the corpus") describes a guard that would have stayed green under gap 3's own defect, because a non-string `optional`+`required` field satisfies plain existence regardless of branch order. Since the plan's own acceptance criteria mandate the counterfactual proof actually fail the guard, `TestCorpusExercisesEveryRequiredResult` tracks a second, finer-grained witness map (`producedPresenceByHasNotEmpty`) requiring both a string/bytes and a non-string/bytes witness for `requiredExactPresence` specifically — the one outcome sensitive to the exact branch-order bug. This is documented in the "Why the guard needed strengthening" note above rather than silently added.
- **`Nested`/`Inner` get `test:TestGolden` coverage claims, not a golden of their own.** Both are reference-only proto types (a map-value type, a message-field type) that no production code path ever derives standalone — inventing a golden fixture that calls `derive[Nested]` directly would test something the real code never does. Their being skipped-or-mapped-correctly is already exercised through `TestGolden`'s `Maps`/`Messages` subtests on their containing message, so the honest claim names that test.
- **`corpusMessages()` filters `IsMapEntry()` messages, never demands coverage for them.** Proto map fields synthesize their own nested map-entry `MessageDescriptor`s (confirmed live via `Maps.string_map`/`int_map`/`message_map`'s three synthetic entries); no human wrote them and no contributor could satisfy a coverage claim for them, so they are excluded from `corpusMessages()`'s output entirely rather than being an always-failing edge case.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing Critical] Strengthened TestCorpusExercisesEveryRequiredResult beyond literal Part-B text to satisfy the plan's own mandatory counterfactual**
- **Found during:** Task 2, running the counterfactual proof against the guard as first written (plain per-outcome existence check)
- **Issue:** The plan's Part-B text, read literally, produces a guard that checks only "is each `requiredResult` value produced by ANY corpus field." Running the mandatory counterfactual (gap 3's original branch order) against that literal design showed the guard staying GREEN — `RequiredOptionalNonString.value` (non-string) still satisfies `requiredExactPresence` regardless of branch order, since the bug only manifests when `hasNotEmpty=true`. This contradicted the plan's own acceptance criterion: "the guard plus plan 01-08's presence subtests must both fail."
- **Fix:** Added a second, `hasNotEmpty`-keyed witness map for `requiredExactPresence` specifically (the one outcome the branch-order bug affects), requiring both a string/bytes and a non-string/bytes witness. Re-ran the counterfactual: the guard now fails as required, alongside `TestRequiredTranslation`.
- **Files modified:** `mixinforproto/corpus_test.go`
- **Verification:** Detection proof and counterfactual proof both recorded verbatim above; full suite, `-race -count=5`, and standalone build all still pass with the strengthened guard on the correct (unmodified) `fieldmap.go`.
- **Committed in:** `484c87b` (Task 2 commit)

---

**Total deviations:** 1 auto-fixed (Rule 2 — missing critical functionality: the guard as literally specified could not satisfy the plan's own mandatory acceptance criterion)
**Impact on plan:** No scope creep — the fix is entirely within `corpus_test.go`, adds no new file, and is required specifically to make the plan's own counterfactual proof pass. `git diff mixinforproto/*.go` excluding `corpus_test.go` remains empty.

## Known Stubs

None — this plan adds only test infrastructure and documentation; no production code path, UI, or data flow was touched.

## Issues Encountered

None beyond the Rule 2 deviation above. `buf`/`protoc-gen-go` were not needed (no `.proto` file was added or changed by this plan). `make check-stubs` and `make check-goversion` both passed on the first run against the unmodified corpus and pinned `go.mod`.

## User Setup Required

None - no external service configuration required.

## Verification (plan-level, run from repo root)

All eight steps from the plan's `<verification>` block passed:

1. `(cd mixinforproto && go test ./... -count=1 -v -run 'TestCorpus')` — all three guards PASS, `-v` output enumerates coverage for every fieldClass, every requiredResult (including both hasNotEmpty witnesses), and all three coverage subtests.
2. `(cd mixinforproto && go test ./... -count=1)` — full suite green.
3. `(cd mixinforproto && go test -race -count=5 ./...)` — deterministic, race-clean.
4. `(cd mixinforproto && GOWORK=off go build ./... && GOWORK=off go test ./...)` — standalone consumption unaffected.
5. `make test && make test-standalone && make check-modules` — all pass.
6. `make check-stubs` — pass (no `.proto` file touched by this plan).
7. `git status --porcelain go.mod mixinforproto/go.mod mixinforproto/go.sum mixinforproto/internal/gen mixinforproto/testdata` — empty.
8. `git diff --stat mixinforproto/` — `corpus_test.go` only (against the pre-plan baseline); no production `.go` file modified (`git diff --stat -- 'mixinforproto/*.go' ':!mixinforproto/corpus_test.go'` is empty).

`make check-goversion` also passed (pin `go 1.24.0` unchanged) — the plan's pins-you-must-not-undo requirement.

Broken Windows 1 (unsigned-integer intervals), 2 (`bytes.*` translation), and 3 (repeated-cardinality list mapping) remain **open** and untouched; Window 4 (descriptor-set staleness, closed by 01-08) remains **fixed** and untouched — confirmed via `.planning/WINDOWS.md`, not edited by this plan.

## Next Phase Readiness

- All 25 Phase 1 requirement IDs remain satisfied (unchanged by this plan — it adds no new requirement coverage beyond documenting MIX-02/VAL-01/VAL-03's structural regression protection).
- Phase 2 (entc extension) and Phase 3 (Tier 2 CEL, Tier 3 map constraints) inherit a proven pattern: `corpusMessages()` + exhaustiveness-table-then-coverage is directly reusable for any future proto-corpus expansion, and `corpusCoverage`'s bidirectional check means a forgotten coverage claim on a new corpus message is now a build failure, not a silent gap.
- This is the last plan in Phase 1's gap-closure set (01-06 through 01-09); all four verification gaps from `01-VERIFICATION.md` are now closed, and three of the four have a structural regression guard specifically because their root cause (corpus-shape avoidance) was common to all three.

---
*Phase: 01-mixinforproto-core*
*Completed: 2026-08-08*

## Self-Check: PASSED

Both created/modified files (`mixinforproto/corpus_test.go`, `CONTRIBUTING.md`) and this
SUMMARY confirmed present on disk. All three task commit hashes (`0bf8e5a`, `484c87b`,
`7e71f54`) confirmed present in `git log --oneline --all`.
