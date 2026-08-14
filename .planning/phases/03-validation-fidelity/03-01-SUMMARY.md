---
phase: 03-validation-fidelity
plan: 01
subsystem: database
tags: [ent, protobuf, protovalidate, cel-go, mixinforproto, connectrpc]

requires:
  - phase: 02-crud-handlers-interceptor-chain
    provides: runtime.MapError's error-mapping table (D-18) and the boundary protovalidate interceptor (runtime/interceptor.go) this plan's storage-layer hook now agrees with
  - phase: 01-mixinforproto-core
    provides: SourceField.ResidualIDs/ResidualFingerprint provenance (D-11), fieldmap.go's ResolveFieldRules usage pattern, the failure/derivationError schema-load-panic machinery
provides:
  - mixinforproto's first ent.Mixin.Hooks() implementation (VAL-04), compiling residual (buf.validate.field).cel rules once at schema load and evaluating them at mutation time
  - reverse.go — the string-scalar leg of the ent-value -> protoreflect.Value mirror table (D-05)
  - violation.go — the single shared *protovalidate.ValidationError constructor (D-07 consequence 3)
  - mixinforproto/internal/difftest — a driverless differential harness (D-13) proving ent's real withHooks pipeline invokes the new hook, with zero DB driver in mixinforproto's require block
  - runtime.MapError's *protovalidate.ValidationError -> CodeInvalidArgument case, matching connectrpc.com/validate's own boundary mapping (VAL-07's identity guarantee made concrete)
  - Empirical answer to 03-RESEARCH.md Open Question 1: mutation.Fields() on Create already includes an unset Default("")-bearing field, because ent's generated defaults() calls the setter before any hook runs
  - Corrected CLAUDE.md cel-go stack entry (github.com/google/cel-go v0.28.0, not cel-expr/cel-go) and amended REQUIREMENTS.md VAL-05 / ROADMAP.md Phase 3 SC1 wording (D-06/D-02)
affects: [03-02, 03-03, 03-04, 03-05]

actuals:
  tokens: 43241
  tasks: 3
  commits: 4

tech-stack:
  added: [github.com/google/cel-go@v0.28.0 (promoted from indirect to direct in mixinforproto/go.mod)]
  patterns:
    - "Schema-load-compiled hookState + mutation-time ent.Hook closure, mirroring the failure/derivationError collected-failure discipline for uncompilable CEL"
    - "Driverless differential harness: a real generated ent.Client driven by a hand-written dialect.Driver fake, proving ent's real hook pipeline fires with zero DB driver dependency"
    - "Package-level atomic test-seam counter (CELCompileCount) to prove compile-once-at-construction empirically from an external test package"

key-files:
  created:
    - mixinforproto/hooks.go
    - mixinforproto/violation.go
    - mixinforproto/reverse.go
    - mixinforproto/internal/difftest/fakedriver.go
    - mixinforproto/internal/difftest/tracer_test.go
    - mixinforproto/internal/difftest/ent/schema/residualcel.go
    - mixinforproto/internal/difftest/ent/entc.go
    - mixinforproto/internal/difftest/ent/generate.go
    - runtime/errormap_test.go
  modified:
    - mixinforproto/mixin.go
    - mixinforproto/go.mod
    - runtime/errormap.go
    - runtime/doc.go
    - mixinforproto/doc.go
    - mixinforproto/README.md
    - .claude/CLAUDE.md
    - .planning/REQUIREMENTS.md
    - .planning/ROADMAP.md

key-decisions:
  - "Task 1 checkpoint (D-01, gate=blocking, not blocking-human): auto-selected option-a — the hook returns protovalidate's own *protovalidate.ValidationError as mixinforproto's public error type, matching the plan's recorded expected answer"
  - "This plan compiles ONLY the residual custom-CEL half of D-07's hybrid design (D-08's routing line); the standard-rule half (protovalidate's own evaluator over every in-scope field) is deliberately deferred to a later plan in this phase, per the tracer's thinnest-possible-slice mandate"
  - "hookFieldClass reuses fieldmap.go's existing classify() rather than re-deriving field shape a second way, keeping D-05's forward/reverse tables provably in sync"
  - "The difftest fakeDriver's Query always returns a synthetic single-row int64 result regardless of query text (D-13) — sufficient for this edge-free schema's auto-increment ID scan path; Exec and Tx are unreachable no-ops for this schema shape, documented as such rather than left unexplained"

patterns-established:
  - "Test-only package-level instrumentation (celCompileCount/CELCompileCount) as a sanctioned seam for asserting compile-once behavior from outside the package, per the plan's own acceptance criteria wording"

requirements-completed: [VAL-04, VAL-05, VAL-06, VAL-07, PIPE-06]

coverage:
  - id: D1
    description: "A residual (buf.validate.field).cel rule rejects a real ent Create at the storage layer with a *protovalidate.ValidationError carrying the contract's RuleId and field path"
    requirement: "VAL-04"
    verification:
      - kind: unit
        ref: "mixinforproto/internal/difftest/tracer_test.go#TestResidualCELRejectsRealCreate"
        status: pass
      - kind: unit
        ref: "mixinforproto/internal/difftest/tracer_test.go#TestResidualCELAcceptsRealCreate"
        status: pass
    human_judgment: false
  - id: D2
    description: "mutation.Fields() on Create already includes an unset Default(\"\")-bearing derived field (D-06's Create-side scope mechanism), proven against a real generated fixture"
    requirement: "VAL-05"
    verification:
      - kind: unit
        ref: "mixinforproto/internal/difftest/tracer_test.go#TestUnsetDefaultFieldAppearsInMutationFieldsOnCreate"
        status: pass
    human_judgment: false
  - id: D3
    description: "Every cel.Program is compiled at Hooks() construction time, never inside the returned hook closure (VAL-04's 'compiled once', T-03-04's DoS mitigation)"
    requirement: "VAL-04"
    verification:
      - kind: unit
        ref: "mixinforproto/internal/difftest/tracer_test.go#TestHooksCompilesOnlyAtConstruction"
        status: pass
    human_judgment: false
  - id: D4
    description: "runtime.MapError maps *protovalidate.ValidationError (direct and errors.As-wrapped) to CodeInvalidArgument, without breaking privacy.Deny -> CodePermissionDenied or *connect.Error passthrough"
    requirement: "VAL-07"
    verification:
      - kind: unit
        ref: "runtime/errormap_test.go#TestMapErrorValidationError"
        status: pass
      - kind: unit
        ref: "runtime/errormap_test.go#TestMapErrorValidationErrorWrapped"
        status: pass
      - kind: unit
        ref: "runtime/errormap_test.go#TestMapErrorPrivacyDenyStillWins"
        status: pass
      - kind: unit
        ref: "runtime/errormap_test.go#TestMapErrorConnectErrorPassthrough"
        status: pass
    human_judgment: false
  - id: D5
    description: "mixinforproto/go.mod promotes github.com/google/cel-go v0.28.0 to a direct dependency with no DB driver added anywhere in the module, and CLAUDE.md/REQUIREMENTS.md/ROADMAP.md corrections land"
    verification:
      - kind: other
        ref: "go mod edit -json mixinforproto/go.mod (Indirect omitted/false for cel-go); make check-goversion; make check-modules; grep -c 'cel-expr/cel-go@v0.31.0' .claude/CLAUDE.md == 0"
        status: pass
    human_judgment: false

duration: 55min
completed: 2026-08-14
status: complete
---

# Phase 3 Plan 1: Residual CEL Storage-Layer Enforcement Summary

**mixinforproto's first ent.Mixin.Hooks() implementation enforces a residual `(buf.validate.field).cel` rule at storage-layer mutation time, proven end-to-end against a real generated ent.Client with zero DB driver, and `runtime.MapError` learns the resulting `*protovalidate.ValidationError` type.**

## Performance

- **Duration:** 55 min
- **Started:** 2026-08-14T14:26:20Z
- **Completed:** 2026-08-14T15:20:15Z
- **Tasks:** 3 (Task 1 checkpoint auto-approved, Task 2 tracer, Task 3 auto/tdd)
- **Files modified:** 33

## Accomplishments

- `mixinforproto` gained its first `Hooks()` implementation: schema-load-time compilation of one `cel.Env`/`cel.Program` per residual `(buf.validate.field).cel` rule, and a mutation-time `ent.Hook` that reverse-converts in-scope field values, evaluates the compiled programs, and rejects with the shared `newValidationError` constructor's `*protovalidate.ValidationError` — or returns a literal `nil` error when there are no violations.
- `mixinforproto/go.mod` promotes `github.com/google/cel-go v0.28.0` from indirect to direct (D-16), with no new external package name and no DB driver anywhere in the module's require block (D-13 preserved).
- `mixinforproto/internal/difftest` is a new driverless differential harness sibling to `internal/boundarytest`: a real generated `ent.Client` (no `entconnect` extension — `mixinforproto` still never imports the root module) driven by a hand-written `dialect.Driver` fake, proving ent's real `withHooks` pipeline invokes the new hook on a genuine `Save(ctx)` call.
- Empirically resolved 03-RESEARCH.md Open Question 1 against a real generated fixture (not a template read): on Create, `mutation.Fields()` already includes an unset, `Default("")`-collapsed field, because ent's own generated `defaults()` calls the field's setter before any hook runs. This means plan 03-03's D-06 mechanism can be a single `mutation.Fields()` call with no `Op()` branch.
- `runtime.MapError` gained the `*protovalidate.ValidationError -> CodeInvalidArgument` case (TDD RED/GREEN), ordered after `privacy.Deny` and ahead of the generic `*connect.Error` passthrough, using `errors.As` so a wrapped `ValidationError` still maps correctly.
- Every stale "boundary-only until Phase 3" claim is corrected: `runtime/doc.go`, `mixinforproto/doc.go`, `mixinforproto/README.md`. `.claude/CLAUDE.md`'s cel-go stack entry, "What NOT to Use" row, and Version Compatibility row are corrected to `github.com/google/cel-go v0.28.0` per D-16. `.planning/REQUIREMENTS.md` VAL-05 and `.planning/ROADMAP.md` Phase 3 SC1 are amended per D-06/D-02, with dated notes.

## Task Commits

Each task was committed atomically:

1. **Task 1: Confirm D-01 checkpoint** — auto-approved (option-a, `gate="blocking"`, matches the plan's recorded expected answer); no code change, no separate commit.
2. **Task 2: End-to-end — one residual CEL rule rejects a real ent Create, driverless** - `674119c` (feat)
3. **Task 3: Make the wire error and every stale "until Phase 3" claim true** - `6323e8d` (test, RED) → `df60beb` (feat, GREEN) → `0bb529f` (docs)

**Plan metadata:** _(recorded in this commit's own final metadata commit)_

_Note: Task 3 carried `tdd="true"`; the test → feat sequence above is the RED/GREEN pair. No REFACTOR commit was needed._

## Files Created/Modified

- `mixinforproto/hooks.go` - schema-load-compiled `hookState` + mutation-time `ent.Hook`; the residual-CEL half of D-07's hybrid evaluator
- `mixinforproto/violation.go` - the single shared `*protovalidate.ValidationError` constructor (D-07 consequence 3)
- `mixinforproto/reverse.go` - the string-scalar leg of the ent-value → `protoreflect.Value` reverse table (D-05), failing closed (D-12) on any unimplemented class/kind
- `mixinforproto/mixin.go` - `protoMixin[M].Hooks()` override
- `mixinforproto/go.mod` - `github.com/google/cel-go v0.28.0` promoted to direct
- `mixinforproto/internal/difftest/fakedriver.go` - hand-written `dialect.Driver`, zero new dependencies
- `mixinforproto/internal/difftest/tracer_test.go` - the four proof tests (reject, accept, Open Question 1, compile-once)
- `mixinforproto/internal/difftest/ent/schema/residualcel.go`, `entc.go`, `generate.go` + committed generated `ent` package - the driverless fixture
- `runtime/errormap.go` - `MapError`'s new `*protovalidate.ValidationError` case
- `runtime/errormap_test.go` - the five `MapError` test cases (new + regression)
- `runtime/doc.go`, `mixinforproto/doc.go`, `mixinforproto/README.md` - stale "boundary-only" language corrected
- `.claude/CLAUDE.md` - cel-go stack table, "What NOT to Use", and Version Compatibility corrections (D-16)
- `.planning/REQUIREMENTS.md`, `.planning/ROADMAP.md` - VAL-05/SC1 amendments (D-06/D-02)

## Decisions Made

- Task 1's `D-01` checkpoint auto-approved to option-a per the auto-mode checkpoint policy (`gate="blocking"`, not `blocking-human`), matching the plan's own recorded expected answer — no deviation.
- Scoped `hooks.go` to the residual-CEL evaluation path only, deliberately deferring D-02's full standard-rule evaluator to a later plan in this phase, per the tracer task's explicit "thinnest possible path" mandate and D-08's rule-kind routing line.
- Built the difftest `fakeDriver`'s `Query` to always return a synthetic single-row `int64` regardless of query text — correct and sufficient for this edge-free schema's auto-increment-ID scan path; `Exec`/`Tx` are documented, deliberate no-ops/errors since this schema shape never reaches them.

## Deviations from Plan

None — plan executed as written. The one open question the plan explicitly flagged (03-RESEARCH.md Open Question 1) was resolved empirically as anticipated, with the observed answer recorded verbatim in the test failure message and here.

## Issues Encountered

- Building the ColumnScanner-satisfying fake driver required reading `entgo.io/ent/dialect/sql/sqlgraph`'s `creator.insertLastID` source directly to determine the exact scan-target shape (`*entsql.Rows` wrapping a `ColumnScanner`) — not obvious from the public API surface alone. Resolved by tracing the real dependency source, consistent with 03-RESEARCH.md's own verification discipline.
- An initial test helper closure used the wrong `ent.Hook` inner-function signature (`func(ent.Hook) ent.Hook` instead of `func(ent.Mutator) ent.Mutator`) — caught immediately by the compiler, fixed before any commit.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Plans 03-02 through 03-05 (Wave 2+) can now build on a real, tested `Hooks()` entry point, the shared `newValidationError` constructor, and the `reverseValue`/D-05 mirror-table convention established here.
- D-07's full hybrid (protovalidate's own evaluator for standard rules, alongside this plan's local CEL env for residual rules) and D-06's Update-side changed-only scope are both explicitly out of scope for this plan and remain for the next wave.
- `mixinforproto/internal/difftest` and its committed generated `ent` package are now the established pattern for PIPE-06's differential sweep to extend, corpus-message by corpus-message.

---
*Phase: 03-validation-fidelity*
*Completed: 2026-08-14*

## Self-Check: PASSED

All created files (mixinforproto/hooks.go, mixinforproto/violation.go, mixinforproto/reverse.go,
mixinforproto/internal/difftest/fakedriver.go, mixinforproto/internal/difftest/tracer_test.go,
runtime/errormap.go, runtime/errormap_test.go, this SUMMARY.md) and all four task commits
(674119c, 6323e8d, df60beb, 0bb529f) verified present on disk / in git log.
