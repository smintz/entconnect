---
phase: 02-crud-handlers-interceptor-chain
plan: 05
subsystem: api
tags: [entc, connect-rpc, codegen, golden-tests, ci, escape-hatch, audit]

# Dependency graph
requires:
  - phase: 02-crud-handlers-interceptor-chain
    provides: "02-01's entc.Extension skeleton (RegisterGenerator registry, gen.Hook emission, resolve.go's procedure resolution, errors.go's collected-failures discipline) and runtime/ framework (Chain, Server, MapError, viewer.Viewer); 02-02's combined server.tmpl/NewServer mechanism; 02-04's build-time/request-time mask validation as the most recent precedent for a two-layer enforcement split"
provides:
  - "entc/crud_manual.go + entc/templates/manual.tmpl — the OpManual generator: an app-supplied func field on the per-service struct, a method that calls it, CodeUnimplemented naming the procedure when nil"
  - "entc/extension.go's synthesized compile-time obligation for an unclaimed method on a partly-claimed service (Task 1's ManualField/renderEntry mechanism) — never a silent auto-generated no-op"
  - "entc/claims.go — BuildClaims/WriteClaimsReport (INT-05's deterministic sorted claims report) and checkConflicts (D-05's collected duplicate-claim / duplicate-op conflict gate)"
  - "internal/entconnecttest/manual/ — a real, generated, tested fixture proving a hand-written handler runs inside the identical fixed chain, plus the concrete claims.txt proof (manual/unclaimed rows)"
  - "internal/entconnecttest/conflict/ — the D-05 negative-build fixture (DupeA/DupeB)"
  - "internal/entconnecttest/adjacency/, internal/entconnecttest/empty/ — in-test-only CRUD-06 edge fixtures (never go:generate'd for real)"
  - "entc/extension_test.go + entc/testdata/ — golden coverage of every emitted file for every fixture this phase ships, plus the adjacency/empty/ordering/repeat-stability CRUD-06 edge suite"
  - "Makefile's test-determinism target + CI's modules job wiring it in (D-20's -count=5 discipline)"
affects: [phase-3-validation, phase-4-flow-binding, phase-5-drift-check]

# Actuals (#2632)
actuals:
  tokens: 58000
  tasks: 3
  commits: 3

# Tech tracking
tech-stack:
  added:
    - "github.com/sebdah/goldie/v2 v2.8.0 (root module's first test-only direct dependency, already pinned in .claude/CLAUDE.md's stack)"
  patterns:
    - "Manual escape hatch: an app-supplied func field on the generated per-service struct (FieldName/ParamName/FuncType threaded through MethodImpl.ManualField), a method calling it, CodeUnimplemented on nil — never a separate registration path, so the method inherits the same chain (INT-04)"
    - "An unclaimed method on a partly-claimed proto service gets the identical Manual-shaped synthetic treatment (entc/extension.go's post-claimed-loop diff against svcDesc.Methods()) so a generated <Service>Handler interface is always fully satisfied, never silently auto-stubbed"
    - "Binding-collection pass split from resolution: entc/extension.go decodes every schema's raw Bindings first, runs D-05's conflict gate over that snapshot, writes the claims report, THEN resolves procedures against the descriptor set — so the claims report and the conflict gate never depend on resolution succeeding"
    - "Golden tests generate into t.TempDir() (outside the real module tree) via entc.LoadGraph + entconnect.Generate called directly, in-process — goldie.New(t).Assert on raw bytes, one golden per emitted file plus the claims report"
  key-files:
    created:
      - entc/crud_manual.go
      - entc/templates/manual.tmpl
      - entc/claims.go
      - entc/claims_test.go
      - entc/extension_test.go
      - entc/testdata/*.golden (16 files)
      - internal/entconnecttest/manual/**
      - internal/entconnecttest/conflict/ent/schema/{dupe_a,dupe_b}.go
      - internal/entconnecttest/adjacency/ent/schema/{adja,adjb}.go
      - internal/entconnecttest/empty/ent/schema/nothing.go
      - proto/entconnecttest/v1/admin.proto
    modified:
      - entc/crud.go
      - entc/extension.go
      - entc/templates/service.tmpl
      - entc/templates/server.tmpl
      - Makefile
      - .github/workflows/ci.yml
      - go.mod
      - go.sum
      - proto/descriptorset.binpb

key-decisions:
  - "An unclaimed method on a partly-claimed service is treated identically to a real Manual binding for CODE GENERATION purposes (forced app-supplied func field), but is NEVER counted as 'manual' in the claims report — only genuine schema-declared bindings feed BuildClaims. This keeps INT-05's audit trail honest: a method the generator had to force a slot for because nothing claimed it still reads 'unclaimed', not 'manual'."
  - "D-05's conflict gate was refactored out of the old inline single-map dedup (entc/extension.go's original 'claimed' map, checked mid-resolution) into a dedicated pre-resolution pass (entc/claims.go's checkConflicts) over the raw, unresolved binding snapshot — this both fixed the 'same Op twice on one schema' gap the original inline check never covered, and let the claims report and conflict gate share one collection pass without either depending on descriptor resolution succeeding."
  - "Golden fixtures are generated into t.TempDir(), outside the real module tree, so imports.Process cannot resolve project-local unqualified identifiers (ent, entconnecttestv1, entconnecttestv1connect) the way it does for real, committed output — some goldens capture an import block that would not itself compile. This is expected and orthogonal to what the suite proves (byte-stability, per-file coverage); real compilability is separately proven by every fixture's own go generate + go test round trip, unaffected by this test-only artifact."
  - "The adjacency/empty CRUD-06 edge fixtures are in-test-only schema packages (internal/entconnecttest/adjacency, internal/entconnecttest/empty) with no go:generate directive and no committed generated output — loaded only via entc.LoadGraph inside entc/extension_test.go, per the plan's own sanctioned fallback (every real fixture owns a single, distinct proto service and would need editing to overlap)."

patterns-established:
  - "Pattern: entc/extension.go's per-service render loop collects claimed AND synthetic-unclaimed method output into one renderEntry slice, sorted by procedure before splitting into methodBodies/manualFields — the mechanism any future generator needing per-service ordering guarantees should reuse."

requirements-completed: [CRUD-06, INT-04, INT-05]

coverage:
  - id: D1
    description: "A developer annotates an ent schema with entconnect.Manual(<generated Procedure constant>), supplies only a handler func, and the generated wiring registers it so it runs inside the same fixed authn -> viewer -> protovalidate -> otel chain the CRUD handlers use"
    requirement: "INT-04"
    verification:
      - kind: e2e
        ref: "internal/entconnecttest/manual/manual_test.go#TestManual_HandwrittenBodyRuns, #TestManual_MissingViewerIsUnauthenticated, #TestManual_ProtovalidateRejectsEmptyID, #TestManual_ViewerReadableInsideHandler"
        status: pass
      - kind: other
        ref: "grep -c 'h.client\\|s.client' internal/entconnecttest/manual/entconnect/admin_service.entconnect.go — 0"
        status: pass
    human_judgment: false
  - id: D2
    description: "An unclaimed method on a partly-claimed service (Ping on AdminService) is a compile-time obligation on the application (a forced app-supplied func field defaulting to CodeUnimplemented), never a silently auto-generated no-op"
    requirement: "INT-04"
    verification:
      - kind: other
        ref: "internal/entconnecttest/manual/entconnect/admin_service.entconnect.go declares pingFn as a struct field and NewServer requires adminServicePing as a constructor parameter"
        status: pass
    human_judgment: false
  - id: D3
    description: "Codegen emits a deterministic, sorted claims report listing every RPC in the descriptor set with its claimant (crud:<op> / manual / unclaimed), even when zero bindings exist anywhere"
    requirement: "INT-05"
    verification:
      - kind: unit
        ref: "entc/claims_test.go#TestBuildClaims_AllUnclaimed, #TestBuildClaims_Mixed, #TestBuildClaims_Empty, #TestBuildClaims_Stability"
        status: pass
      - kind: e2e
        ref: "internal/entconnecttest/manual/claims_test.go#TestClaims_ManualIsAuditableUnclaimedIsReported"
        status: pass
    human_judgment: false
  - id: D4
    description: "An unclaimed RPC appears in the claims report and does NOT fail the build in this phase"
    requirement: "INT-05"
    verification:
      - kind: other
        ref: "go generate ./internal/entconnecttest/manual/... exits 0 despite the unclaimed Ping row"
        status: pass
    human_judgment: false
  - id: D5
    description: "A procedure claimed twice, or the same Op claimed twice on one schema, is a collected codegen error reporting every offender in one pass"
    requirement: "CRUD-06"
    verification:
      - kind: integration
        ref: "entc/claims_test.go#TestConflict_DuplicateClaimAndDuplicateOp, #TestConflict_Determinism"
        status: pass
    human_judgment: false
  - id: D6
    description: "Two schemas whose bindings target methods on the same service merge into one emitted file with a deterministic method order"
    requirement: "CRUD-06"
    verification:
      - kind: integration
        ref: "entc/extension_test.go#TestGolden_Adjacency"
        status: pass
    human_judgment: false
  - id: D7
    description: "A graph with zero entconnect bindings emits zero handler files and no empty stub file, while the claims report is still written; a service with exactly one claimed method emits exactly one file with one method"
    requirement: "CRUD-06"
    verification:
      - kind: integration
        ref: "entc/extension_test.go#TestGolden_Empty"
        status: pass
    human_judgment: false
  - id: D8
    description: "Methods and imports inside every emitted file appear in a stable sorted order that does not depend on graph or map iteration order, and repeated generation is byte-identical"
    requirement: "CRUD-06"
    verification:
      - kind: integration
        ref: "entc/extension_test.go#TestGolden_Ordering, #TestGolden_RepeatStability"
        status: pass
      - kind: other
        ref: "go test -race -count=5 ./entc/... — pass (413s)"
        status: pass
    human_judgment: false
  - id: D9
    description: "Every emitted handler file for every CRUD verb is covered by a golden-file test, and regenerating every fixture from a clean commit is byte-identical (idempotent)"
    requirement: "CRUD-06"
    verification:
      - kind: integration
        ref: "entc/extension_test.go#TestGolden_EmittedFiles (read/write/list/update/manual subtests)"
        status: pass
      - kind: other
        ref: "go test ./entc/... -update && git diff --exit-code -- entc/testdata — no output; go generate over every fixture app run twice — git diff --exit-code — clean; bash scripts/pipeline.sh — all 5 steps green, git status clean afterward"
        status: pass
    human_judgment: false

duration: ~3h
completed: 2026-08-14
status: complete
---

# Phase 2 Plan 5: Manual Escape Hatch, Claims Report, Golden Coverage & Byte-Stability Summary

**`entconnect.Manual(...)` lets a developer hand-write exactly one RPC and it still runs inside the identical fixed interceptor chain; a deterministic sorted claims report names every descriptor-set RPC's claimant (`crud:<op>`/`manual`/`unclaimed`) without ever failing the build on an unclaimed row; a procedure claimed twice is a collected, self-sufficient codegen error; and every emitted file for every CRUD verb across five fixtures is golden-covered and proven byte-identical across shuffled input order, five repeated runs, and a full clean-commit pipeline re-run.**

## Performance

- **Duration:** ~3h
- **Tasks:** 3 (all `type="auto"`)
- **Files modified:** ~70 (12 hand-written new + 3 hand-written modified + ~40 ent-generated manual fixture files + 16 golden fixtures + proto/gen/CI/Makefile/go.mod changes)

## Accomplishments

- `entc/crud_manual.go` + `entc/templates/manual.tmpl` implement the `OpManual` generator (INT-04/D-06): an app-supplied func field on the per-service struct, a method that calls it, `CodeUnimplemented` naming the procedure when the field is nil (T-02-29). The application supplies only the handler body — no ent client call anywhere in the emitted code (`grep -c 'client'` on the generated file reports 0).
- `entc/extension.go` now diffs a service's claimed methods against every method the proto service actually declares, and synthesizes the identical Manual-shaped compile-time obligation for any unclaimed method on a partly-claimed service — proven live by `AdminService`'s `Ping` RPC, which `internal/entconnecttest/manual/`'s fixture must supply a func for even though no schema binds it, while `entc/claims.go`'s report still names it `unclaimed`, not `manual`. This required threading a new `ManualField` (struct field / constructor param / func type) through `entc/crud.go`'s `MethodImpl`, `entc/templates/service.tmpl`'s struct declaration, and `entc/templates/server.tmpl`'s `NewServer` signature and struct literal — both template edits are additive-and-empty-safe, confirmed live by regenerating the four pre-existing fixtures (read/write/list/update) with zero byte drift.
- `internal/entconnecttest/manual/` proves the whole INT-04 claim end to end: a real HTTP round trip through a real Connect client into a hand-written `ArchiveAdmin` body, with three chain-order assertions specific to the manual procedure — a missing viewer refuses with `CodeUnauthenticated` before the body runs, a `buf.validate` violation refuses with `CodeInvalidArgument` before the body runs, and `viewer.FromContext` succeeds inside the body once it does run.
- `entc/claims.go`'s `BuildClaims`/`WriteClaimsReport` (INT-05) label every RPC in the descriptor set `crud:<op>`/`manual`/`unclaimed`, sorted by procedure, written to `<outputDir>/claims.txt` even when zero bindings exist anywhere — proven stable across five different binding input orders. `checkConflicts` (D-05) detects a procedure claimed by two or more schemas and a schema declaring the same Op twice, both collected in one pass and proven against a real negative-build fixture (`internal/entconnecttest/conflict/`) with byte-identical error text across `-count=5`.
- `entc/extension_test.go` + `entc/testdata/` golden-cover every file `Generate` emits for every fixture this phase ships (read/write/list/update/manual, 16 goldens total), plus four dedicated CRUD-06 edge cases: adjacency (two schemas, one service, ascending method order — proven via an in-test-only fixture since every real fixture owns a single, distinct service), empty (zero bindings against a literal zero-procedure descriptor set emits zero `.entconnect.go` files and a header-only claims report; a single-method service emits exactly one file with exactly one method), ordering (five independent generations produce byte-identical output despite Go's randomized map iteration, and the emitted import block is sorted within each gofmt-produced group), and repeat-stability (five generations of a two-service fixture are byte-identical file-for-file).
- `Makefile`'s new `test-determinism` target runs `go test -count=5 ./...` across both modules (reusing the existing skip-guard idiom); CI's `modules` job now runs it after `make test`. The `standalone` job's root-module `GOWORK=off` step already existed from 02-01's resolved checkpoint, so no further CI change was needed there.
- Full-pipeline proof: `bash scripts/pipeline.sh` runs all five steps green (step 5 correctly `SKIP`s — no application `ent/schema` exists yet, only test fixtures), and `git status --short` is clean afterward — regeneration from a clean commit is a true no-op across every fixture, not just the ones this plan directly touched.

## Task Commits

Each task was committed atomically:

1. **Task 1: Manual end to end — a hand-written handler runs inside the generated chain** — `caea770` (feat)
2. **Task 2: Deterministic claims report and the duplicate-claim conflict gate** — `7495d98` (feat)
3. **Task 3: Golden coverage for every emitted verb, byte-stability, and the CI determinism job** — `a3a5ad3` (test)

**Plan metadata:** *(this commit, docs: complete plan)*

## Files Created/Modified

- `proto/entconnecttest/v1/admin.proto` — `Admin`, `ArchiveAdminRequest`/`Response`, `PingRequest`/`Response`, `AdminService` (`ArchiveAdmin` claimed manually, `Ping` deliberately unclaimed)
- `entc/crud_manual.go`, `entc/templates/manual.tmpl` — the `OpManual` generator
- `entc/crud.go` — `ManualField` type, `MethodImpl.ManualField`
- `entc/extension.go` — binding-collection pass split from resolution, `checkConflicts`/`BuildClaims`/`WriteClaimsReport` wiring, per-service unclaimed-method synthesis, `renderEntry`-based sort-then-split
- `entc/templates/service.tmpl`, `entc/templates/server.tmpl` — additive `ManualFields`/`ManualParams` support
- `entc/claims.go`, `entc/claims_test.go` — `Claim`, `BuildClaims`, `WriteClaimsReport`, `checkConflicts`
- `entc/extension_test.go`, `entc/testdata/*.golden` (16 files) — the golden suite and its fixtures
- `internal/entconnecttest/manual/` — the real, generated, tested Manual fixture (`ent/schema/admin.go`, generated `ent/`/`entconnect/`, `manual_test.go`, `claims_test.go`)
- `internal/entconnecttest/conflict/ent/schema/{dupe_a,dupe_b}.go` — D-05 negative-build fixture
- `internal/entconnecttest/adjacency/ent/schema/{adja,adjb}.go`, `internal/entconnecttest/empty/ent/schema/nothing.go` — in-test-only CRUD-06 edge fixtures
- `internal/entconnecttest/{read,write,list,update,manual}/entconnect/claims.txt` — regenerated with every fixture's own claims report (new file type this plan introduces)
- `Makefile`, `.github/workflows/ci.yml` — `test-determinism` target and its CI wiring
- `go.mod`, `go.sum` — `github.com/sebdah/goldie/v2` as a direct dependency

## Decisions Made

See `key-decisions` in frontmatter above.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Admin message's second field could not be named "label"**
- **Found during:** Task 1, first `go generate` run against the fixture
- **Issue:** `mixinforproto/reserved.go`'s `reservedStructural` set blocks `"label"` as a per-type identifier ent itself reserves (a per-entity `Label` constant, `ent/ent#280`) — `MixinForProto[*entconnecttestv1.Admin]` panicked at schema-load naming the collision.
- **Fix:** Renamed the field to `"name"` in `admin.proto` (both the proto message and the regenerated stubs/descriptor set).
- **Files modified:** `proto/entconnecttest/v1/admin.proto`, `internal/gen/entconnecttestv1/admin.pb.go`, `internal/gen/entconnecttestv1/entconnecttestv1connect/admin.connect.go`, `proto/descriptorset.binpb`
- **Verification:** `go generate ./internal/entconnecttest/manual/...` succeeds
- **Committed in:** `caea770`

**2. [Rule 2 - Missing Critical] Unclaimed methods on a partly-claimed service needed the same compile-time-obligation mechanism as a real Manual binding**
- **Found during:** Task 1, designing the `AdminService` fixture (`ArchiveAdmin` claimed, `Ping` deliberately not)
- **Issue:** `service.tmpl`'s `var _ {{ConnectPkgAlias}}.{{ServiceName}}Handler = (*{{StructName}})(nil)` compile-time assertion requires the generated struct to satisfy the FULL `<Service>Handler` interface. With only claimed methods rendered, `Ping` would have no method at all and the assertion (and therefore the whole generated package) would fail to compile the moment any service had an unclaimed RPC alongside a claimed one — a design gap the plan's own task text anticipated and required closing, not an incidental discovery.
- **Fix:** `entc/extension.go`'s per-service loop now diffs claimed method names against `svcDesc.Methods()` and synthesizes an `OpManual`-generator call for every unclaimed method, giving it the identical forced-func-field treatment. Required extending `MethodImpl`/`serverEntry`/the two templates with `ManualField`/`ManualParams`.
- **Files modified:** `entc/crud.go`, `entc/extension.go`, `entc/templates/service.tmpl`, `entc/templates/server.tmpl`
- **Verification:** `internal/entconnecttest/manual/entconnect/admin_service.entconnect.go` declares both `archiveAdminFn` and `pingFn`; `go build ./...` passes; regenerating read/write/list/update produced zero byte drift
- **Committed in:** `caea770`

**3. [Rule 1 - Bug] `TestGolden_Ordering`'s import-sortedness check assumed one flat, globally-sorted import list**
- **Found during:** Task 3, first run of the ordering test
- **Issue:** `gofmt`/`goimports` groups imports into blank-line-separated blocks (stdlib, then third-party) and sorts WITHIN each block, never merges them into one global sort. The initial check flattened the whole import block into one list and demanded global alphabetical order, which real goimports output never satisfies (`"connectrpc.com/connect"` sorts before `"context"` globally but goimports correctly keeps them in separate, independently-sorted groups).
- **Fix:** Rewrote the check (`importGroups`) to split on blank lines and verify sortedness within each group only, matching gofmt/goimports' actual, documented behavior.
- **Files modified:** `entc/extension_test.go`
- **Verification:** `TestGolden_Ordering` passes against the real read fixture's multi-group import block
- **Committed in:** `a3a5ad3`

---

**Total deviations:** 3 auto-fixed (1 blocking, 1 missing-critical, 1 bug)
**Impact on plan:** All three were necessary for the fixture to compile at all (deviation 1), for the plan's own explicit "unclaimed method is a compile-time obligation, never an auto-stub" requirement to be achievable (deviation 2), or for a test assertion to check real behavior instead of a mistaken assumption about it (deviation 3). None expanded scope beyond the plan's own stated deliverables.

## Issues Encountered

- **Golden fixtures generated into `t.TempDir()` capture an import block that would not itself compile** (missing `ent`/`entconnecttestv1`/`entconnecttestv1connect` imports `imports.Process` cannot resolve outside the real module tree). Documented as an expected, non-blocking property of the golden-test harness in `key-decisions` above — not a stub, not a defect in production output. Real compilability is proven separately and independently by every fixture's own `go generate` + `go test` round trip (all passing, all committed).
- **`go test -race -count=5 ./entc/...` takes ~7 minutes** (413s measured) — well within a normal CI budget but worth noting for anyone iterating locally; `make test-determinism` (no `-race`) is faster (~170s for the root module) and is what CI actually runs.

## User Setup Required

None — no external service configuration required.

## Next Phase Readiness

- Phase 2's full scope (CRUD-01…CRUD-07, INT-01…INT-05) is now complete across all five plans. The entc extension's registry (`OpGet`/`OpList`/`OpCreate`/`OpUpdate`/`OpDelete`/`OpManual`), the fixed interceptor chain, the claims report, and golden/determinism coverage are all in place for Phase 3 (Tier 2/3 CEL passthrough) to build on without revisiting codegen mechanics.
- Phase 5's DRIFT-01 (failing the build on an unclaimed RPC) has exactly the report it needs: `entc/claims.go`'s `BuildClaims`/`WriteClaimsReport`, whose `<procedure> <claimant>` line shape is deliberately trivially parseable (documented in the plan's own flagged assumption) but not yet negotiated with Phase 5's specific consumer needs — worth a first look when that phase starts.
- `entc/extension.go`'s `renderEntry`-based sort-then-split mechanism (claimed + synthetic-unclaimed merged, sorted by procedure, then split into methodBodies/manualFields) is a reusable pattern for any future generator needing per-service ordering guarantees.
- Nothing outstanding or blocking.

---
*Phase: 02-crud-handlers-interceptor-chain*
*Completed: 2026-08-14*

## Self-Check: PASSED

All 14 key created files verified present on disk; all 4 commit hashes (`caea770`, `7495d98`, `a3a5ad3`, `6aa1bc7`) verified present in `git log --oneline --all`. No missing items.
