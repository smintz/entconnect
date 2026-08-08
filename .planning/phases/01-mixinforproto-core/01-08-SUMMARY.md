---
phase: 01-mixinforproto-core
plan: 08
subsystem: mixinforproto
tags: [ent, protobuf, protovalidate, fieldmap, validate, gap-closure]

# Dependency graph
requires:
  - phase: 01-mixinforproto-core (plans 01-05, 01-06, 01-07)
    provides: derive[M]/mapField/classify core, VAL-01/02/03 Tier 1 translation, constraints.proto corpus, WINDOWS.md ledger, scripts/generate-stubs.sh (single canonical buf generate invocation)
provides:
  - "classifyRequired's presence-first branch order, closing 01-VERIFICATION.md gap 3 / 01-REVIEW.md CR-03 (VAL-03): optional string/bytes + required now derives non-optional construction with no NotEmpty(), matching protovalidate's presence semantics"
  - "delegatingFormatValidator's per-field violation filtering, closing 01-VERIFICATION.md gap 2 / 01-REVIEW.md CR-02 (VAL-01): a delegated format field on a multi-field message now accepts a valid value AND still rejects an invalid one"
  - "RequiredOptionalString/RequiredOptionalBytes/StringFormatWithSibling corpus fixtures that make both defect shapes observable going forward, with goldens"
  - "proto/mixinforprototest.binpb regenerated to match the canonical buf build invocation; Broken Window 4 closed"
  - "README.md field-mapping table distinguishes presence-tracking vs implicit-presence required semantics; WINDOWS.md entry 2's stale description corrected"
affects: ["Phase 2 (entc extension) - Tier 1 translation this phase's foundation now behaves correctly on realistic multi-field contracts", "Phase 3 (differential harness) - both fixed divergences would otherwise have been invisible to it"]

actuals:
  tokens: 14100
  tasks: 3
  commits: 6

tech-stack:
  added: []
  patterns:
    - "classifyRequired's branch order is now presence-first for every kind: optional && required always wins ahead of required && hasNotEmpty, so string/bytes stop being special-cased relative to the numeric builders."
    - "delegatingFormatValidator evaluates the whole synthetic dynamicpb message but extracts a per-field verdict by comparing protovalidate.Violation.FieldDescriptor.FullName() against the candidate field's FullName() — never string-parsing the violation's field path."
    - "Broken Window ledger corrections (description-only, status untouched) are a distinct action from windows fixed <id> — the former is a hand edit to WINDOWS.md, the latter is exclusively the gsd-tools CLI verb."

key-files:
  created:
    - mixinforproto/testdata/constraints_required_optional_string.golden
    - mixinforproto/testdata/constraints_required_optional_bytes.golden
    - mixinforproto/testdata/constraints_string_format_with_sibling.golden
  modified:
    - proto/mixinforprototest/v1/constraints.proto
    - mixinforproto/internal/gen/mixinforprototestv1/constraints.pb.go
    - mixinforproto/fieldmap.go
    - mixinforproto/validate.go
    - mixinforproto/validate_test.go
    - mixinforproto/README.md
    - proto/mixinforprototest.binpb
    - .planning/WINDOWS.md

key-decisions:
  - "Presence wins for every kind in classifyRequired, not just non-string/bytes kinds — required on a presence-tracking field is always a presence assertion, never a non-emptiness one, closing the asymmetry that made string/bytes the two kinds where the bug was reachable."
  - "delegatingFormatValidator's errors.As branch treats a non-*ValidationError verdict as a genuine evaluator failure and surfaces it wrapped, rather than silently treating it as 'no rejection' or masquerading it as a format violation — this keeps the anti-bypass guarantee honest under an unexpected error shape."
  - "WINDOWS.md entry 2 was corrected in place (description only, status stays open) per the plan's explicit scope fence; entry 4 was closed exclusively via `gsd-tools windows fixed 4`, never by hand-editing its status/resolved_at fields."
  - "The regenerated proto/mixinforprototest.binpb was independently re-verified byte-identical against a second, separate buf build invocation after commit, proving the regeneration is stable and not an artifact of a stale buf cache."

requirements-completed: [VAL-01, VAL-03]

coverage:
  - id: D1
    description: "A delegated format validator (hostname/uri/ip/uuid) on a multi-field message accepts a valid value for the candidate field and still rejects an invalid one, ignoring unrelated violations on sibling fields"
    requirement: "VAL-01"
    verification:
      - kind: unit
        ref: "mixinforproto/validate_test.go#TestDelegatedFormatIgnoresUnrelatedViolations"
        status: pass
      - kind: unit
        ref: "mixinforproto/validate_test.go#TestStringFormatValidators (regression: all five single-field formats unchanged)"
        status: pass
    human_judgment: false
  - id: D2
    description: "optional string/optional bytes carrying required derives non-optional construction with no NotEmpty(), no Nillable().Optional(), no Default — matching protovalidate's presence ('must be set') semantics rather than non-emptiness"
    requirement: "VAL-03"
    verification:
      - kind: unit
        ref: "mixinforproto/validate_test.go#TestRequiredTranslation/optional_string:_yields_presence,_not_NotEmpty"
        status: pass
      - kind: unit
        ref: "mixinforproto/validate_test.go#TestRequiredTranslation/optional_bytes:_yields_presence,_not_NotEmpty"
        status: pass
      - kind: unit
        ref: "mixinforproto/validate_test.go#TestRequiredTranslation (regression: RequiredString/RequiredOptionalNonString/RequiredPlainNonString unchanged)"
        status: pass
    human_judgment: false
  - id: D3
    description: "proto/mixinforprototest.binpb matches a byte-identical fresh run of the canonical buf build proto -o ... --as-file-descriptor-set --exclude-source-info invocation; Broken Window 4 closed"
    verification:
      - kind: other
        ref: "make check-stubs (pass); cmp against a second independent buf build re-run (byte-identical); gsd-tools windows fixed 4"
        status: pass
    human_judgment: false
  - id: D4
    description: "Three new corpus fixtures (RequiredOptionalString, RequiredOptionalBytes, StringFormatWithSibling) with golden fixtures; zero pre-existing golden modified"
    verification:
      - kind: unit
        ref: "mixinforproto/validate_test.go#TestGoldenConstraints (three new subtests); git status --porcelain mixinforproto/testdata shows exactly 3 untracked, 0 modified"
        status: pass
    human_judgment: false

duration: ~25min
completed: 2026-08-08
status: complete
---

# Phase 1 Plan 08: Gap Closure — Delegated Format Verdict Filtering and Presence-Correct Required Summary

**Fixed two silent data-loss defects: a delegated format validator that rejected ~100% of inputs on any multi-field message, and a presence-tracking `optional string`/`bytes` + `required` that wrongly derived `NotEmpty()` instead of presence semantics — both closed with RED-before-GREEN evidence and an anti-bypass guard.**

## Performance

- **Duration:** ~25 min (git commit span 13:41–13:52 UTC; reading/verification time not fully captured in that span)
- **Started:** 2026-08-08T13:41:06Z (baseline commit before this plan)
- **Completed:** 2026-08-08T13:52:12Z
- **Tasks:** 3 (all completed)
- **Files modified:** 8 modified, 3 created (goldens)

## Accomplishments

- Closed 01-VERIFICATION.md gap 3 / 01-REVIEW.md CR-03 (VAL-03): `classifyRequired` now resolves `optional && required` (presence) ahead of `required && hasNotEmpty` (implicit-presence NotEmpty) for every kind, so `buildStringField`/`buildBytesField` derive non-optional, non-NotEmpty construction for a presence-tracking field carrying `required` — matching protovalidate's "must be set" semantics exactly.
- Closed 01-VERIFICATION.md gap 2 / 01-REVIEW.md CR-02 (VAL-01): `delegatingFormatValidator` now filters protovalidate's whole-message `*ValidationError.Violations` down to the one attributed to the candidate field (`FieldDescriptor.FullName()` comparison), so an unrelated rule on a sibling field never poisons the format verdict — while an actually-invalid value on the candidate field is still rejected (the mandatory anti-bypass assertion).
- Added three corpus fixtures (`RequiredOptionalString`, `RequiredOptionalBytes`, `StringFormatWithSibling`) that make both defect shapes permanently observable, with golden fixtures; corrected `constraints.proto`'s header comment, which had presented the now-fixed single-field precondition as a design rationale rather than the silently-depended-upon property it was.
- Regenerated `proto/mixinforprototest.binpb` (stale since 01-06 added `repeated.proto`; Broken Window 4) via the canonical `buf build proto -o proto/mixinforprototest.binpb --as-file-descriptor-set --exclude-source-info` invocation, verified byte-identical against a second independent regeneration, and closed the window via `gsd-tools windows fixed 4`.
- Corrected three stale doc comments: the `requiredResult` block and both builders' comments (no longer claiming NotEmpty is exact "in either presence state"), and `validate.go`'s file header (WR-06's false "exactly three direct dependencies" invariant — `go.mod`'s first require block already lists five, with the protovalidate protocolbuffers package as a genuine direct, non-indirect dependency).
- Corrected `WINDOWS.md` entry 2's stale "no residual record" clause (description only, status stays open — `recordBytesResidual` does record `bytes.*` constraint IDs as residual).

## Task Commits

Each task was committed atomically, with RED-before-GREEN pairs for the two TDD tasks:

1. **Task 1: Add corpus fixtures and regenerate the stub** - `e282a25` (feat)
2. **Task 2: Presence semantics for required on optional string/bytes**
   - `18d6097` (test — RED)
   - `249270f` (fix — GREEN)
3. **Task 3: Filter delegated format verdict to the field under validation**
   - `57132b1` (test — RED)
   - `f0e8a46` (fix — GREEN)
4. **Additional in-scope fix: regenerate `proto/mixinforprototest.binpb`, close Broken Window 4** - `54badbc` (fix)

**Plan metadata:** committed as part of this SUMMARY's own commit.

## Files Created/Modified

- `proto/mixinforprototest/v1/constraints.proto` - Added `RequiredOptionalString`, `RequiredOptionalBytes`, `StringFormatWithSibling`; corrected header comment
- `mixinforproto/internal/gen/mixinforprototestv1/constraints.pb.go` - Regenerated for the three new messages
- `mixinforproto/fieldmap.go` - `classifyRequired` branch reorder; `requiredExactPresence` arm in `buildStringField`/`buildBytesField`; doc-comment corrections
- `mixinforproto/validate.go` - `delegatingFormatValidator` violation filtering by `FieldDescriptor`; new `errors` import; doc-comment corrections (delegatingFormatValidator, file header WR-06)
- `mixinforproto/validate_test.go` - New `TestRequiredTranslation` presence subtests (string/bytes); new `TestDelegatedFormatIgnoresUnrelatedViolations`; three new `TestGoldenConstraints` subtests
- `mixinforproto/testdata/constraints_required_optional_string.golden` - New golden
- `mixinforproto/testdata/constraints_required_optional_bytes.golden` - New golden
- `mixinforproto/testdata/constraints_string_format_with_sibling.golden` - New golden
- `mixinforproto/README.md` - Split the `required` row into presence-tracking vs implicit-presence rows; added a sentence on delegated-format-validator field scoping
- `proto/mixinforprototest.binpb` - Regenerated to match the canonical `buf build` invocation
- `.planning/WINDOWS.md` - Entry 2 description corrected (stays open); entry 4 marked fixed via `gsd-tools windows fixed 4`

## Decisions Made

- Presence wins for every kind in `classifyRequired`'s branch order, not conditionally for string/bytes — this removes the asymmetry that made string/bytes the two kinds where the defect was reachable, and keeps the switch's semantics uniform with the non-string builders' existing `requiredExactPresence` arm.
- `delegatingFormatValidator`'s `errors.As` failure branch treats a non-`*ValidationError` as a genuine evaluator failure and wraps/surfaces it, rather than defaulting to "no rejection" — an unfiltered fallback there would have been a second, subtler bypass path.
- Window ledger entry corrections and closures are two distinct actions with two distinct mechanisms: entry 2's description was hand-edited (status untouched, per the plan's explicit scope fence), while entry 4's closure went exclusively through `gsd-tools windows fixed 4`, never a hand edit of its status/resolved_at fields.
- The regenerated `proto/mixinforprototest.binpb` was independently re-verified byte-identical against a second, separate `buf build` invocation run after the commit, to rule out a stale-cache artifact rather than a true fixed point.

## Red evidence (gap 3)

Command: `go test ./... -run 'TestRequiredTranslation' -count=1 -v` (mixinforproto), run against `mixinforproto/fieldmap.go` BEFORE the `classifyRequired` reorder (commit `18d6097`, before `249270f`):

```
=== RUN   TestRequiredTranslation
=== RUN   TestRequiredTranslation/string:_yields_NotEmpty
=== RUN   TestRequiredTranslation/optional_non-string:_yields_non-optional_construction
=== RUN   TestRequiredTranslation/plain_non-string:_recorded_residual,_not_approximated
=== RUN   TestRequiredTranslation/optional_string:_yields_presence,_not_NotEmpty
    validate_test.go:368: want zero validators (presence, not NotEmpty), got 1
=== RUN   TestRequiredTranslation/optional_bytes:_yields_presence,_not_NotEmpty
    validate_test.go:396: want zero validators (presence, not NotEmpty), got 1
--- FAIL: TestRequiredTranslation (0.00s)
    --- PASS: TestRequiredTranslation/string:_yields_NotEmpty (0.00s)
    --- PASS: TestRequiredTranslation/optional_non-string:_yields_non-optional_construction (0.00s)
    --- PASS: TestRequiredTranslation/plain_non-string:_recorded_residual,_not_approximated (0.00s)
    --- FAIL: TestRequiredTranslation/optional_string:_yields_presence,_not_NotEmpty (0.00s)
    --- FAIL: TestRequiredTranslation/optional_bytes:_yields_presence,_not_NotEmpty (0.00s)
FAIL
FAIL	github.com/smintz/entconnect/mixinforproto	0.010s
```

`RequiredOptionalString`/`RequiredOptionalBytes` derived a field carrying exactly 1 validator (the wrongly-attached `NotEmpty()`) instead of the wanted zero — confirming the presence branch was unreachable before the fix. After `249270f` (branch reorder + `requiredExactPresence` arm for string/bytes), the identical subtests pass with `ValidatorCount == 0`.

## Red evidence (gap 2)

Command: `go test ./... -run 'TestDelegatedFormatIgnoresUnrelatedViolations' -count=1 -v` (mixinforproto), run against `mixinforproto/validate.go` BEFORE the violation-filtering fix (commit `57132b1`, before `f0e8a46`):

```
=== RUN   TestDelegatedFormatIgnoresUnrelatedViolations
    validate_test.go:838: want a valid URI accepted despite the unrelated required violation on owner, got value does not satisfy the uri format constraint
--- FAIL: TestDelegatedFormatIgnoresUnrelatedViolations (0.01s)
FAIL
FAIL	github.com/smintz/entconnect/mixinforproto	0.012s
```

A syntactically valid URI (`https://example.com/foo`) on `StringFormatWithSibling.endpoint` was rejected purely because the sibling `owner` field (unset, zero value, carrying `required`) produced its own violation on the same whole-message `Validate()` call, and the pre-fix code treated ANY non-nil error as `endpoint`'s own rejection. After `f0e8a46` (violation filtering by `FieldDescriptor.FullName()`), the same test passes both halves: the valid URI is accepted, AND a genuinely invalid URI (`"not a uri"`) on the same multi-field message is still rejected (the mandatory anti-bypass assertion, run in the same test).

## Deviations from Plan

None - plan executed exactly as written, including the additional in-scope descriptor-set regeneration and Broken Window 4 closure the plan explicitly folded in.

## Issues Encountered

None. `buf`/`protoc-gen-go` were confirmed on `$(go env GOPATH)/bin` (not `PATH`) per the plan's documented environment facts, and every verification command in the plan's `<verification>` block and each task's `<acceptance_criteria>` passed on first execution after the corresponding fix.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- All 25 phase-1 requirement IDs are now satisfied (VAL-01 and VAL-03 close the last two BLOCKED items from 01-VERIFICATION.md); MIX-02 was already closed by 01-06.
- Broken Windows 1 (unsigned-integer intervals), 2 (bytes.* length/pattern translation, description corrected but status open), and 3 (real list-typed mapping) remain open and documented — none are silent gaps.
- `proto/mixinforprototest.binpb` is current with the full corpus (including this plan's three new fixtures); `make check-stubs` and `make check-goversion` both pass; the CI staleness gate closed by 01-07 continues to guard this going forward.
- No new dependency was added (`mixinforproto/go.mod`/`go.sum` untouched); the `go 1.24.0` pin remains intact per `make check-goversion`.

---
*Phase: 01-mixinforproto-core*
*Completed: 2026-08-08*
