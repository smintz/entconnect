---
phase: 03-validation-fidelity
plan: 04
subsystem: database
tags: [ent, connectrpc, protovalidate, mixinforproto, ci, dependency-parity]

requires:
  - phase: 03-validation-fidelity
    provides: "03-01's residual-CEL hook half and runtime.MapError's *protovalidate.ValidationError case; 03-02's complete reverse.go table; 03-03's complete D-07 hybrid (standard-rule evaluator + dedup), which this plan proves is genuinely wired into a real ent mutation path rather than merely correct in isolation"
provides:
  - "internal/entconnecttest/hookwiring — the root module's real-client wiring fixture: Policed/Unpoliced ent schemas (byte-identical apart from Policy()), proving VAL-09's relative hook order (privacy -> mixin hook -> schema hooks) by recorded marker sequence, never by hooks-slice index"
  - "D-13's real-client wiring proof (wiring_test.go): a real Connect Create request through a real ent.Client, sqlite-backed, proves ent genuinely invokes the mixin hook; the storage layer's violation identities (RuleId, FieldPath) are asserted equal to protovalidate.Validate's on the bare entity (D-04, entity-relative)"
  - "runtime/interceptor_test.go — VAL-10's once-per-process protovalidate.Validator construction pinned as an explicit test: GlobalValidator pointer identity, one construction across 4 sequential requests, one construction across 8 concurrent Chain builds + requests, race-clean"
  - "Makefile's check-dep-parity — VAL-11/D-15's explicit five-module version-parity gate between root and mixinforproto, GOWORK=off, hosted as a new step in CI's existing standalone job"
  - "mixinforproto/README.md's ordering section now states the full verified order (defaults -> policy -> mixin hooks -> schema hooks -> sqlSave -> check) and the no-constraint-existence-leak guarantee, backed by a test"
affects: [03-05]

actuals:
  tokens: 64340
  tasks: 3
  commits: 3

tech-stack:
  added: []
  patterns:
    - "Marker-recording via a *schema.Recorder carried on the mutation context (recorder.go), appended to by both a Policy() rule and a schema-declared Hooks() entry — the mechanism VAL-09's relative-sequence assertion observes, deliberately never an index into ent's own generated Hooks[] slice (03-RESEARCH.md Pitfall 3: that slice's shape depends on whether ANY Policy() exists anywhere on the schema)"
    - "A countingValidator distinguishing CONSTRUCTION count from Validate CALL count (runtime/interceptor_test.go) — the only way to empirically prove 'never constructed on a request path' without reaching into protovalidate's own unexported sync.OnceValues memoization"
    - "Entity-relative violation-identity comparison via protovalidate's own exported (RuleId, FieldPathString) pair — the same identity key mixinforproto/violation.go's own newValidationError already dedups on — reused from outside the package via protovalidate.Violation.Proto's public fields, with no need to import mixinforproto internals"

key-files:
  created:
    - proto/entconnecttest/v1/hookwiring.proto
    - internal/entconnecttest/hookwiring/ent/schema/policed.go
    - internal/entconnecttest/hookwiring/ent/schema/unpoliced.go
    - internal/entconnecttest/hookwiring/ent/schema/recorder.go
    - internal/entconnecttest/hookwiring/ent/entc.go
    - internal/entconnecttest/hookwiring/ent/generate.go
    - internal/entconnecttest/hookwiring/ordering_test.go
    - internal/entconnecttest/hookwiring/wiring_test.go
    - runtime/interceptor_test.go
  modified:
    - mixinforproto/README.md
    - Makefile
    - .github/workflows/ci.yml
    - proto/descriptorset.binpb
    - entc/testdata/{read,write,list,update,manual}_claims.golden

key-decisions:
  - "VAL-09's ordering test asserts a RELATIVE marker sequence recorded on a shared *schema.Recorder, never an index into ent's generated Hooks[] slice — Policed and Unpoliced are byte-identical apart from Policy() precisely so both configurations (privacy wrapper present vs. absent at Hooks[0]) are covered without a hard-coded position"
  - "D-13's wiring proof separates three independently meaningful assertions rather than one combined test: (1) the full Connect stack rejects an invalid payload with CodeInvalidArgument, (2) the ent client driven DIRECTLY (no HTTP) rejects the identical input with violation identities matching protovalidate.Validate(entity) exactly, (3) the boundary validator alone (no ent.Client reached) still rejects the identical input — together proving defense-in-depth without conflating 'the whole stack works' with 'the storage layer independently enforces it'"
  - "VAL-10's test uses a countingValidator with a CONSTRUCTION counter separate from its CALL counter, supplied via WithValidator — this is the only way to empirically observe 'never reconstructed on a request path' from outside protovalidate's own unexported memoization; protovalidate.GlobalValidator itself is tested separately (pointer-identity across reads) since it is a fixed package-level value, not something this package's tests can intercept construction of"
  - "check-dep-parity's module set is exactly D-15's five names, each resolved independently per module via GOWORK=off `go list -m -f '{{.Version}}'` — never a bare grep of go.mod text, which would not resolve transitive/indirect requirements the same way the real build graph does"
  - "Rule 1 auto-fix (out of this plan's declared scope, necessary for Task 2's own `make test-standalone-root`/`go test ./...` run): entc/testdata/*_claims.golden fixtures for read/write/list/update/manual were stale the moment Task 1's hookwiring.proto added HookWiringCreateService to the shared descriptor set — every other fixture's own claims.txt golden now legitimately lists it 'unclaimed'. Regenerated via `go test ./entc/... -update`; not a behavior change, purely a descriptor-set-driven fixture refresh."

patterns-established:
  - "Real-client wiring proof harness (newTestClient/newTestServer, sqlite-backed, real httptest.Server, real generated Connect client) reused verbatim from internal/entconnecttest/{update,write}'s established shape — the third fixture in this repo to follow it exactly."

requirements-completed: [VAL-07, VAL-09, VAL-10, VAL-11]

coverage:
  - id: D1
    description: "ent genuinely invokes the mixin hook in the real mutation path: a real Connect Create request violating a residual protovalidate rule is answered CodeInvalidArgument, and the storage-layer violation's (RuleId, FieldPath) identities equal protovalidate.Validate's on the bare entity message (D-04, entity-relative, field path included) — proven via the ent client driven directly, no HTTP"
    requirement: "VAL-07"
    verification:
      - kind: unit
        ref: "internal/entconnecttest/hookwiring/wiring_test.go#TestWiring_ConnectCreateRejectsInvalidPayload"
        status: pass
      - kind: unit
        ref: "internal/entconnecttest/hookwiring/wiring_test.go#TestWiring_StorageLayerRejectsIndependentlyAndMatchesEntityValidation"
        status: pass
      - kind: unit
        ref: "internal/entconnecttest/hookwiring/wiring_test.go#TestWiring_BoundaryAloneStillRejects"
        status: pass
      - kind: unit
        ref: "internal/entconnecttest/hookwiring/wiring_test.go#TestWiring_ValidPayloadPersistsAndReadsBack"
        status: pass
    human_judgment: false
  - id: D2
    description: "The mixin hook's position is asserted relatively (privacy policy when declared -> mixin hook -> schema hooks) in both a policy-bearing and a policy-free schema, and a denying policy on an invalid mutation yields privacy.Deny never a *protovalidate.ValidationError — no constraint-existence leak"
    requirement: "VAL-09"
    verification:
      - kind: unit
        ref: "internal/entconnecttest/hookwiring/ordering_test.go#TestOrdering_PolicyBearing_InvalidValue"
        status: pass
      - kind: unit
        ref: "internal/entconnecttest/hookwiring/ordering_test.go#TestOrdering_PolicyBearing_ValidValue"
        status: pass
      - kind: unit
        ref: "internal/entconnecttest/hookwiring/ordering_test.go#TestOrdering_PolicyFree_InvalidValue"
        status: pass
      - kind: unit
        ref: "internal/entconnecttest/hookwiring/ordering_test.go#TestOrdering_PolicyFree_ValidValue"
        status: pass
      - kind: unit
        ref: "internal/entconnecttest/hookwiring/ordering_test.go#TestOrdering_DenyingPolicy_InvalidValue"
        status: pass
    human_judgment: false
  - id: D3
    description: "The boundary protovalidate interceptor stays independently armed — storage-layer enforcement is never a reason to weaken it"
    requirement: "VAL-07"
    verification:
      - kind: unit
        ref: "internal/entconnecttest/hookwiring/wiring_test.go#TestWiring_BoundaryAloneStillRejects"
        status: pass
    human_judgment: false
  - id: D4
    description: "A runtime.Chain built once uses one protovalidate.Validator instance for its whole lifetime — never constructed on a request path, sequentially or under 8-goroutine concurrency, race-clean"
    requirement: "VAL-10"
    verification:
      - kind: unit
        ref: "runtime/interceptor_test.go#TestGlobalValidator_PointerIdentityAcrossCalls"
        status: pass
      - kind: unit
        ref: "runtime/interceptor_test.go#TestChain_ValidatorBuiltOnceAcrossSequentialRequests"
        status: pass
      - kind: unit
        ref: "runtime/interceptor_test.go#TestChain_ValidatorBuiltOnceUnderConcurrency"
        status: pass
    human_judgment: false
  - id: D5
    description: "CI fails when the root module and mixinforproto independently resolve different versions of any of D-15's five named modules, or when a named module is absent from either go.mod — both directions rehearsed locally in scratch copies"
    requirement: "VAL-11"
    verification:
      - kind: other
        ref: "make check-dep-parity (passes against the real tree); scratch-copy rehearsal: skewed entgo.io/ent version -> FAIL naming module + both versions; module absent from both go.mod files -> FAIL distinguishing 'not found' from 'versions differ'"
        status: pass
    human_judgment: false

duration: 34min
completed: 2026-08-14
status: complete
---

# Phase 3 Plan 4: Real-Client Hook Wiring, Ordering, and Dependency Parity Summary

**A real Connect request through a real ent.Client proves mixinforproto's storage-layer hook is genuinely wired into ent's mutation path (not just correct in isolation), VAL-09's relative hook ordering is pinned by recorded marker sequence across policy-bearing and policy-free schemas, VAL-10's once-per-process validator construction is an explicit race-tested assertion, and a new `check-dep-parity` Makefile target fails CI when mixinforproto and the root module drift onto different versions of any of D-15's five named modules.**

## Performance

- **Duration:** 34 min
- **Started:** 2026-08-14T16:38:00Z
- **Completed:** 2026-08-14T17:12:00Z
- **Tasks:** 3 (all `type="auto" tdd="true"`/`type="auto"`)
- **Files modified:** 48 (created: hookwiring fixture — proto, ent schema, ordering_test.go, wiring_test.go; runtime/interceptor_test.go; modified: mixinforproto/README.md, Makefile, .github/workflows/ci.yml, proto/descriptorset.binpb, 5 entc golden fixtures)

## Accomplishments

- `internal/entconnecttest/hookwiring` is a new real-ent-client fixture pair, `Policed` and `Unpoliced`, byte-identical apart from `Policed` declaring a `Policy()` — both derive from a single `HookWiring` proto message carrying one residual `(buf.validate.field).cel` rule and one standard `string.min_len` rule, exercising both halves of D-07's hybrid evaluator through one real mutation.
- `ordering_test.go` proves, against a real generated `ent.Client`, VAL-09's relative sequence "privacy policy (if any) -> mixin hook -> schema hooks" in both configurations, by recorded marker sequence on a shared `*schema.Recorder` — never by indexing into ent's own generated `Hooks[]` slice, so an application merely adding or removing a `Policy()` can never turn into a false regression (03-RESEARCH.md Pitfall 3). A fifth test proves T-03-17: a denying policy on an invalid mutation yields `privacy.Deny`, never a `*protovalidate.ValidationError` — no constraint-existence leak to an unauthorized caller.
- `wiring_test.go` is D-13's real-client wiring proof: a real Connect `CreateHookWiring` request through a real HTTP server backed by a real sqlite `ent.Client` is rejected `CodeInvalidArgument`; the ent client driven DIRECTLY (bypassing HTTP and the boundary interceptor entirely) is independently rejected with a `*protovalidate.ValidationError` whose violation `(RuleId, FieldPath)` identities exactly match `protovalidate.Validate` on the bare entity message (D-04's entity-relative comparison, field path included, never excluded); the boundary validator alone (no ent client reached) still rejects the identical input; and a valid payload persists and reads back.
- `runtime/interceptor_test.go` pins VAL-10 as an explicit test: `protovalidate.GlobalValidator` is pointer-identical across reads; a `runtime.Chain` built once and driven through 4 sequential requests never reconstructs its validator (a `countingValidator`'s CONSTRUCTION counter, distinct from its CALL counter, stays at exactly 1); and 8 goroutines concurrently building Chains and issuing requests through a shared validator instance still observe exactly one construction, verified race-clean (`go test -race`).
- `Makefile` gains `check-dep-parity`: an explicit `DEP_PARITY_MODULES` list of D-15's five named modules, each resolved independently via `GOWORK=off go list -m`, compared between the root module and `mixinforproto`. A module absent from either module's resolved build list fails loudly, with a message distinguishing "not found" from "versions differ" — absence is never silently treated as agreement. `.github/workflows/ci.yml`'s existing `standalone` job (already `GOWORK=off`, already in a checkout containing `go.work`) gains one new named step invoking it; no new job was added.
- `mixinforproto/README.md`'s "Mixin hook and policy ordering" section now states the complete verified order for both Create and Update (`defaults() -> privacy Policy (when declared) -> mixin hooks -> schema hooks -> sqlSave() -> check()`) and the no-constraint-existence-leak guarantee, backed by this plan's own test.

## Task Commits

Each task was committed atomically:

1. **Task 1: The hookwiring fixture and VAL-09's relative hook-ordering assertion** - `f1ad6be` (feat)
2. **Task 2: Real-client wiring proof and once-per-process validator construction** - `75ed678` (feat)
3. **Task 3: VAL-11's dependency-parity gate in the GOWORK=off standalone job** - `136e8a9` (feat)

**Plan metadata:** _(recorded in this commit's own final metadata commit)_

_Note: all three tasks carried `tdd="true"` or `type="auto"`; each was implemented and verified in a single commit per task (tests + production wiring committed together) rather than a separate RED/GREEN split — every task's own `<acceptance_criteria>` and `<verify>` commands were run and passed before that task's commit, satisfying the plan's verification loop without an artificially split commit history._

## Files Created/Modified

- `proto/entconnecttest/v1/hookwiring.proto` - `HookWiring` (residual CEL + standard rule) and its Create RPC
- `proto/descriptorset.binpb`, `internal/gen/entconnecttestv1/{hookwiring.pb.go,entconnecttestv1connect/hookwiring.connect.go}` - regenerated stubs
- `internal/entconnecttest/hookwiring/ent/schema/{policed,unpoliced,recorder}.go` - the two ordering fixtures and the shared marker-recording mechanism
- `internal/entconnecttest/hookwiring/ent/entc.go`, `ent/generate.go` + regenerated `ent`/`entconnect` packages - the fixture's own real, committed generated client and Connect server wiring
- `internal/entconnecttest/hookwiring/ordering_test.go` - VAL-09's five relative-ordering tests
- `internal/entconnecttest/hookwiring/wiring_test.go` - D-13's real-client wiring proof (4 tests)
- `runtime/interceptor_test.go` - VAL-10's three once-per-process validator tests
- `mixinforproto/README.md` - ordering section rewritten with the full verified order and no-leak guarantee
- `Makefile` - `check-dep-parity` target + `DEP_PARITY_MODULES`
- `.github/workflows/ci.yml` - one new named step in the existing `standalone` job
- `entc/testdata/{read,write,list,update,manual}_claims.golden` - regenerated (Rule 1 deviation, see below)

## Decisions Made

See `key-decisions` in frontmatter for the full list. Highlights:
- VAL-09's ordering assertion is relative-sequence, never index-based — the entire reason both `Policed` and `Unpoliced` exist.
- D-13's wiring proof is deliberately three separate assertions (full stack / direct ent client / boundary alone) rather than one combined test, so each layer's independent arming is individually provable.
- VAL-10's construction-vs-call counter split is the only way to empirically prove "never reconstructed on a request path" from outside protovalidate's own unexported memoization.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Stale `entc/testdata/*_claims.golden` fixtures after Task 1's proto addition**
- **Found during:** Task 2's `<precondition>` run (`make test-standalone-root`, then `go test ./...` in workspace mode)
- **Issue:** Task 1's `hookwiring.proto` added `HookWiringCreateService` to the shared descriptor set. `entc`'s `TestGolden_EmittedFiles` golden-asserts every emitted `claims.txt` per fixture, and every OTHER fixture's own claims report now legitimately lists the new procedure as `unclaimed` — the golden fixtures for `read`, `write`, `list`, `update`, and `manual` were stale the moment Task 1 landed. This slipped through Task 1's own verification because Task 1 only ran `go build`/`go vet`/`make check-stubs`, not the full `go test ./...`.
- **Fix:** Regenerated via `go test ./entc/... -run TestGolden_EmittedFiles -update`. Purely a descriptor-set-driven fixture refresh — no behavior change, no hand-edited golden content.
- **Files modified:** `entc/testdata/{read,write,list,update,manual}_claims.golden`
- **Verification:** `go test ./entc/...` and `go test ./...` (workspace mode) both green afterward.
- **Committed in:** `75ed678` (Task 2 commit)

---

**Total deviations:** 1 auto-fixed (Rule 1, test-fixture staleness — out of this plan's declared file scope but a direct, mechanical, necessary consequence of Task 1's own change).
**Impact on plan:** None on design; a required correctness fix so `go test ./...` stays green. No scope creep beyond what Task 1's own change required.

## Issues Encountered

- **`make test-standalone-root` fails, both before and after this plan's Task 2 changes, with an identical root cause** — this is expected and explicitly anticipated by Task 2's own `<precondition>`, not a new regression:
  - Root's `go.mod` pins `github.com/smintz/entconnect/mixinforproto v0.1.0`, published BEFORE mixinforproto's `Hooks()` implementation existed (03-01). Under `GOWORK=off`, the root module resolves that OLD published version instead of the working tree.
  - `internal/entconnecttest/hookwiring`'s generated `ent/runtime/runtime.go` was produced by entc codegen running AGAINST THE CURRENT WORKING-TREE mixinforproto (via `go.work`), which observes `Policed`'s mixin returning exactly one `ent.Hook` (since `HookWiring` carries protovalidate rules) and hard-codes `policed.Hooks[1] = policedMixinHooks0[0]` accordingly. Under `GOWORK=off`, the OLD `v0.1.0` mixin's `Hooks()` returns an EMPTY slice (it predates any `Hooks()` implementation at all), so that hard-coded `[0]` index panics: `panic: runtime error: index out of range [0] with length 0` at `hookwiring/ent/runtime/runtime.go:33`.
  - **Precondition record:** `make test-standalone-root` was run BEFORE any Task 2 change (immediately after Task 1's commit) and FAILED with this exact panic. It was run AGAIN after Task 2's changes and FAILED IDENTICALLY — same panic, same line, same root cause. Task 2 introduced no new `GOWORK=off`-specific failure; `runtime`'s own tests (this plan's new `interceptor_test.go` included) pass cleanly under `GOWORK=off` — only the `hookwiring` package (which did not exist before this plan) is affected, and only because it is the FIRST fixture in this repo whose generated code was produced after `Hooks()` shipped (03-01/03-02/03-03's other fixtures — `update`, `write` — still carry generated code from BEFORE `Hooks()` existed and were never regenerated since, so they coincidentally still assume zero mixin hooks and do not hit this).
  - **Follow-up (Phase 1 D-19, out of this plan's scope):** a new `mixinforproto` tag must be pushed (containing 03-01/03-02/03-03's `Hooks()` implementation) and root's `go.mod` bumped to reference it, in a SEPARATE commit from the tag push — never a same-commit self-reference. Until that lands, `make test-standalone-root` (and CI's `standalone` job's corresponding step) will fail on this one fixture. This is a pre-existing, now-surfaced gap this plan's own `<precondition>` explicitly anticipated and instructed be reported rather than absorbed — not something Task 2 is scoped to fix.
  - `check-dep-parity` (Task 3) itself is NOT affected by this — it only compares module VERSIONS via `go list -m`, which succeeds fine under `GOWORK=off` for both modules (all five named modules resolve identically today); it does not build or run any test code.
- No other issues.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Plan 03-05 (if any) inherits a repo where: the storage-layer hook is proven genuinely wired (not just correct in isolation), hook ordering is pinned against an ent upgrade, validator construction cost is proven paid once per process, and CI will fail loudly on protovalidate/cel-go/protobuf/ent version skew between the two modules.
- The `mixinforproto v0.1.0` / root `go.mod` staleness surfaced in this plan's Issues Encountered is a real, now-documented gap — the D-19 follow-up (push a new `mixinforproto` tag reflecting Phase 3's `Hooks()` work, bump root's `go.mod` in a separate commit) is unblocked reading material for whoever picks it up next; it is NOT blocking for this plan's own success criteria, all of which are about the WORKSPACE-mode (not `GOWORK=off`) behavior of the new hook.
- `internal/entconnecttest/{update,write}`'s generated `ent` packages are now known to be stale relative to the current `mixinforproto`'s `Hooks()` implementation (their own mixin hooks were never regenerated since 03-01 shipped `Hooks()`) — flagged here for visibility; not fixed in this plan since those fixtures are owned by other, already-landed plans and regenerating them is outside this plan's declared scope.

---
*Phase: 03-validation-fidelity*
*Completed: 2026-08-14*

## Self-Check: PASSED

All created files (proto/entconnecttest/v1/hookwiring.proto,
internal/entconnecttest/hookwiring/ent/schema/{policed,unpoliced,recorder}.go,
internal/entconnecttest/hookwiring/ordering_test.go,
internal/entconnecttest/hookwiring/wiring_test.go,
runtime/interceptor_test.go) and modified files
(mixinforproto/README.md, Makefile, .github/workflows/ci.yml) verified
present on disk. All three task commits (f1ad6be, 75ed678, 136e8a9)
verified present in git log. All plan-level `<verification>` commands
(`make build && make vet && make test-determinism`, `make check-stubs`,
`make check-modules && make check-goversion && make check-dep-parity`)
re-run and green.
