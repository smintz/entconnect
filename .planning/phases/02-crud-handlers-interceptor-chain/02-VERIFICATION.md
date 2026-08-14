---
phase: 02-crud-handlers-interceptor-chain
verified: 2026-08-14T10:47:07Z
status: gaps_found
score: 11/12 must-haves verified
behavior_unverified: 0
overrides_applied: 0
gaps:
  - truth: "CRUD-07: Generated server wiring is the only place an ent client is constructed and never hands a privileged client to application code"
    status: partial
    reason: >
      The "never hands the client back out" half is genuinely true and structurally enforced
      (runtime.Server/Route expose no *ent.Client accessor; every generated per-service struct
      field holding the client is unexported; no accessor method exists anywhere in runtime/ or
      the five generated `entconnect` packages). But the "is the only place an ent client is
      constructed" half is false as written: every generated `NewServer(client *ent.Client,
      authenticator ..., opts...)` takes the client as an input parameter. All five fixture tests
      construct the *ent.Client themselves in application-level code
      (`client := ent.NewClient(ent.Driver(drv))`) and pass it in — the client is constructed
      OUTSIDE generated wiring, by the application, which therefore always holds a raw, fully
      privileged, unwrapped *ent.Client in scope at (and after) the exact call site where
      entconnect.NewServer is invoked. Nothing in the generated code or runtime/ package prevents
      that same app code from using client directly, bypassing the interceptor chain and privacy
      policies entirely. This directly contradicts 02-CONTEXT.md's own D-12 text ("the *ent.Client
      is constructed inside generated wiring in an internal package") and the 02-01-SUMMARY.md's
      D3 coverage claim ("has no exported route to the *ent.Client the generated wiring
      constructed" — the wiring does not construct it). 02-01-PLAN.md's own concrete design
      (`New<Service>Server(client *ent.Client) *<Service>Server`) already specified client as a
      constructor parameter, so this is a real, load-bearing design tension between the plan's own
      two documents (CONTEXT.md's stated intent vs. PLAN.md's actual mechanism), not a one-off
      coding slip — and it was carried through all five plans unchanged.
    artifacts:
      - path: "internal/entconnecttest/read/entconnect/server.entconnect.go:20"
        issue: "func NewServer(client *ent.Client, ...) — client constructed by caller, not by generated wiring"
      - path: "internal/entconnecttest/read/tracer_test.go:72"
        issue: "client := ent.NewClient(ent.Driver(drv)) — application code constructs the ent.Client and holds it in scope alongside the call to entconnect.NewServer(client, authenticator)"
    missing:
      - "Either: correct CRUD-07's wording (similar to the CRUD-03 correction already made in this phase) to state the achievable property — 'generated wiring never returns/exposes the *ent.Client it is given, and no application code path reaches it through the generated API' — and drop 'is the only place a client is constructed', which is architecturally unachievable without generated code owning DB connection config; OR"
      - "Provide an accepted override in this VERIFICATION.md's frontmatter if the maintainer judges constructor-injection (client passed in once, never returned) as the intended reading all along."
requirements_bookkeeping_gap: >
  REQUIREMENTS.md checkboxes for CRUD-01, CRUD-02, CRUD-04, CRUD-05, CRUD-06, CRUD-07, INT-01
  through INT-05 are still unchecked (`[ ]`); only CRUD-03 is checked. Code evidence below shows
  all twelve requirements have real, tested implementations except the CRUD-07 nuance above — this
  is orchestrator bookkeeping not yet performed, not a delivery gap, and does not by itself block
  the phase.
---

# Phase 2: CRUD Handlers & Interceptor Chain — Verification Report

**Phase Goal:** Developers get generated ConnectRPC CRUD handlers wired directly to the ent client, with safe partial updates and a fixed, privacy-aware interceptor chain — no hand-written handler code, no ent client leakage
**Verified:** 2026-08-14T10:47:07Z
**Status:** gaps_found
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths / Requirement Verdicts

| # | Requirement | Truth | Status | Evidence |
|---|---|---|---|---|
| 1 | CRUD-01 | `go generate` yields a working Get handler over a MixinForProto entity, calling the ent client directly, no Go import edge from `entc/` to any generated Connect package | ✓ VERIFIED | `entc/resolve.go`/`resolve_test.go` (resolvable/malformed/unknown-service/unknown-method); `grep -rn 'orderv1connect\|entconnecttestv1connect' entc/*.go` → no import matches (only doc-comment string literals in struct field comments, confirmed by direct read); `internal/entconnecttest/read/entconnect/order_read_service.entconnect.go:35` calls `s.client.Order.Get(ctx, id)` directly; `go test ./internal/entconnecttest/read/...` → `ok` |
| 2 | CRUD-02 | Create and Delete handlers operate directly against the ent client | ✓ VERIFIED | `entc/templates/create.tmpl`/`delete.tmpl`; `entc/templates/delete.tmpl:3` uses `DeleteOneID(id).Exec(ctx)` — never a predicate-free/bulk delete (confirmed by direct read, no `.Delete()` call anywhere in the template); `internal/entconnecttest/write/{create,delete}_test.go` — 8 e2e tests incl. duplicate-sku→AlreadyExists, repeat-delete→NotFound, unrelated-row-survives, privacy-deny; `go test ./internal/entconnecttest/write/...` → `ok` |
| 3 | CRUD-03 | List pages via hand-emitted keyset predicates, not offset paging, no entgql dependency | ✓ VERIFIED | `go list -m all \| grep entgo.io/contrib` → no output (confirmed live); `grep -rniE 'offset' internal/entconnecttest/list/entconnect entc/crud_list.go entc/templates/list.tmpl` → no matches; `internal/entconnecttest/list/paging_edges_test.go#TestPagingEdges_TiesAtBoundary` genuinely seeds two byte-identical `created_at` values at a page boundary at two different page sizes and asserts every row appears exactly once — read directly, this is a real tie-case test, not a token assertion; `go test ./internal/entconnecttest/list/...` → `ok`. REQUIREMENTS.md/ROADMAP.md corrected in-place with dated evidence pointing at 02-RESEARCH.md, exactly as the phase's own `<known_state>` describes |
| 4 | CRUD-04 | Update requires FieldMask, gates every `Set*` on the mask, untouched fields never zeroed | ✓ VERIFIED | `runtime/fieldmask.go`'s `ValidateMask` — nil/empty mask → `ErrMaskEmpty`, checked before anything else; `entc/templates/update.tmpl` — `switch p { case "field": ... default: return InvalidArgument }`, only masked paths get a `Set*` call; `internal/entconnecttest/update/update_test.go#TestUpdate_MaskedFieldChangesUnmaskedFieldsSurvive` and `mask_edges_test.go`'s 8+ edge tests (absent/nested/wildcard/unknown/excluded/case-differing/duplicate/order-independence) — read directly, all present and passing |
| 5 | CRUD-05 | Codegen validates every mask path against the descriptor and fails the build on an unknown path | ✓ VERIFIED | `entc/maskcheck.go`'s `ValidateMaskPaths` — build-time classification (satisfiable/excluded/unknown/nested-or-wildcard), collected in one pass, sorted; `go test ./entc/... -run TestMaskCheck -v` → all 3 subtests pass live, including `TestMaskCheck_NegativeBuildFixtures` against real negative-build schemas (`badmask/ent/schema/{badpatch,excludedpatch}.go`). D-17's resolved reading (an `Exclude()`d field is skipped, not a build failure; an *unknown* field still fails the build) is implemented exactly as `02-CONTEXT.md`'s known_state describes, confirmed by direct source read of `entc/maskcheck.go`'s `excludedSet[name]: continue` branch vs. the `default:` unknown-failure branch |
| 6 | CRUD-06 | Byte-stable generated code, golden-file covered | ✓ VERIFIED | `bash scripts/pipeline.sh` run live end-to-end (steps 1–4 green, step 5 correctly `SKIP`s per the phase's documented known_state); `git status --porcelain` empty immediately after — genuine byte-stability from the current commit, not a pre-dirty-tree check. `entc/extension_test.go` + 16 golden fixtures in `entc/testdata/` cover every emitted file for every fixture (`TestGolden_EmittedFiles`, `_Adjacency`, `_Empty`, `_Ordering`, `_RepeatStability`); `go test ./entc/...` → `ok` (32s) |
| 7 | CRUD-07 | Generated server wiring is the only place an ent client is constructed; never hands a privileged client to application code | ✗ PARTIAL / GAP | See `gaps` in frontmatter. The no-accessor / no-return-path half is real (`grep -rn 'func.*Client()' runtime/ entc/templates/` → no matches; `Server` type exposes only `Routes()`/`Register()`). The "only place constructed" half is **false**: `func NewServer(client *ent.Client, ...)` in all five generated `server.entconnect.go` files takes the client as an input parameter, and every fixture's own test constructs it in application code (`ent.NewClient(ent.Driver(drv))`) before passing it in — confirmed by direct grep across all 5 fixtures. |
| 8 | INT-01 | Fixed interceptor chain order authn → viewer → protovalidate → otel → handler, non-negotiable | ✓ VERIFIED | `runtime/interceptor.go:90-95` — literal `connect.WithInterceptors(AuthnInterceptor(a), ViewerInterceptor(), validateInterceptor, cfg.otel)`; `connect.WithInterceptors` composes first-listed-outermost (doc-commented in source, matches connect-go's documented behavior); `Option` type (`runtime/interceptor.go:110`) can only set the validator/otel instances via unexported `config` struct fields — no way to add/remove/reorder a stage from application code, confirmed by direct read of the `Option`/`config` types |
| 9 | INT-02 | Viewer-scoped context reaches ent privacy; missing viewer refused as Unauthenticated, never anonymous-allow | ✓ VERIFIED | `runtime/interceptor.go:51-60` `ViewerInterceptor` — `if _, ok := viewer.FromContext(ctx); !ok { return CodeUnauthenticated }`; e2e: `TestGetOrder_MissingViewerIsUnauthenticated`, `TestManual_MissingViewerIsUnauthenticated` pass live |
| 10 | INT-03 | Privacy denials surface as Connect PermissionDenied, mapped via `errors.Is` (sentinel), not a type switch | ✓ VERIFIED | `runtime/errormap.go:44` — `case errors.Is(err, privacy.Deny):` — confirmed by direct read, no `case *privacy.` type-switch form anywhere in the file; e2e privacy-deny tests pass for Get/Create/Delete/Update, all read directly |
| 11 | INT-04 | `entconnect.Manual("rpc")` runs inside the full generated chain | ✓ VERIFIED | `entc/templates/manual.tmpl` — nil handler → `CodeUnimplemented`, else calls the app func inline in the same generated method (no separate registration/bypass path); `internal/entconnecttest/manual/manual_test.go` — `TestManual_MissingViewerIsUnauthenticated`, `TestManual_ProtovalidateRejectsEmptyID`, `TestManual_ViewerReadableInsideHandler` all pass, proving authn/viewer/protovalidate stages run before a hand-written Manual body; `go test ./internal/entconnecttest/manual/...` → `ok` |
| 12 | INT-05 | Deterministic sorted claims report; Phase 2 does not fail the build on an unclaimed RPC | ✓ VERIFIED | `entc/claims.go`'s `BuildClaims`/`WriteClaimsReport` — pure report generation, no error return path tied to "unclaimed"; live `claims.txt` for the manual fixture shows `Ping unclaimed` alongside `ArchiveAdmin manual` with `go generate`/`go test` exiting 0 — confirmed live. `checkConflicts` (D-05) is the only build-fatal path, and it fires only on genuine duplicate claims, confirmed via `entc/claims_test.go` |

**Score:** 11/12 requirement-level truths verified (CRUD-07 partial — see gap)

### Required Artifacts

| Artifact | Expected | Status | Details |
|---|---|---|---|
| `entc/extension.go`, `entc/crud*.go`, `entc/resolve.go`, `entc/claims.go`, `entc/maskcheck.go` | entc.Extension skeleton, per-verb generator registry, procedure resolution, claims report, mask validation | ✓ VERIFIED | All present, compile, tested (`go build ./...`, `go vet ./...` both exit 0; `go test ./entc/...` passes in 32s) |
| `runtime/interceptor.go`, `runtime/server.go`, `runtime/errormap.go`, `runtime/fieldmask.go`, `runtime/cursor.go`, `runtime/viewer/viewer.go` | shipped, non-generated framework code | ✓ VERIFIED | All present, no accessor to `*ent.Client` anywhere in this package (confirmed by direct read + grep) |
| `internal/entconnecttest/{read,write,list,update,manual}/` | five real, generated, tested fixture apps | ✓ VERIFIED | All five compile, all five test suites pass (`go test ./internal/entconnecttest/...` → 5/5 `ok`) |
| `internal/entconnecttest/conflict/`, `adjacency/`, `empty/` | negative-build / CRUD-06 edge fixtures | ✓ VERIFIED | Exercised via `entc/claims_test.go`/`entc/extension_test.go`, all passing |
| `proto/entconnecttest/v1/*.proto` + committed stubs/descriptor set | service-bearing synthetic corpus | ✓ VERIFIED | `buf lint`/`buf build` both pass live via `scripts/pipeline.sh` |

### Key Link Verification

| From | To | Via | Status | Details |
|---|---|---|---|---|
| ent schema annotation (`entconnect.GetRPC(...)`) | `protoreflect.MethodDescriptor` | `entc/resolve.go` procedure-string split + descriptor-set lookup | ✓ WIRED | `entc/resolve_test.go` passes; zero Go import edge to generated Connect packages (grep-confirmed) |
| Generated handler | ent client mutation/query | `s.client.<Entity>.<Verb>(ctx, ...)` | ✓ WIRED | Direct reads of `get.tmpl`, `create.tmpl`, `delete.tmpl`, `update.tmpl`, `list.tmpl` all confirm real ent client calls, not static returns |
| `runtime.Chain` | `connect.WithInterceptors` | literal 4-arg call in fixed order | ✓ WIRED | `runtime/interceptor.go:90-95`, confirmed by direct read |
| Application code | `*ent.Client` | `NewServer(client, authenticator, ...)` constructor parameter | ⚠️ INVERTED | The client flows INTO generated wiring, not out of it — but this means it originates in, and remains available to, application code (see CRUD-07 gap) |

### Behavioral Spot-Checks / Test Runs

| Behavior | Command | Result | Status |
|---|---|---|---|
| Full build | `go build ./...` | exit 0 | ✓ PASS |
| Full vet | `go vet ./...` | exit 0 | ✓ PASS |
| entc package tests | `go test ./entc/...` | `ok` (32.2s) | ✓ PASS |
| All 5 fixture app tests | `go test ./internal/entconnecttest/...` | 5/5 `ok` | ✓ PASS |
| Mask-check build-time validation | `go test ./entc/... -run TestMaskCheck -v` | 3/3 subtests pass incl. real negative-build fixtures | ✓ PASS |
| Full pipeline, byte-stability | `bash scripts/pipeline.sh` then `git status --porcelain` | pipeline green (step 5 correctly SKIP), working tree clean after | ✓ PASS |
| `go list -m all \| grep entgo.io/contrib` | — | no output | ✓ PASS (no entgql/entproto runtime dependency) |
| `grep -rn 'func.*Client()' runtime/ entc/templates/` | — | no matches | ✓ PASS (no client accessor) |
| `client := ent.NewClient(...)` present in every fixture test | — | present in all 5 fixtures | ⚠️ Confirms CRUD-07 gap — app constructs the client |

### Requirements Coverage

| Requirement | Source Plan(s) | Status | Evidence |
|---|---|---|---|
| CRUD-01 | 02-01 | ✓ SATISFIED | See truth #1 |
| CRUD-02 | 02-02 | ✓ SATISFIED | See truth #2 |
| CRUD-03 | 02-03 | ✓ SATISFIED (already `[x]` in REQUIREMENTS.md) | See truth #3 |
| CRUD-04 | 02-04 | ✓ SATISFIED | See truth #4 |
| CRUD-05 | 02-04 | ✓ SATISFIED | See truth #5 |
| CRUD-06 | 02-05 | ✓ SATISFIED | See truth #6 |
| CRUD-07 | 02-01 | ⚠️ PARTIAL — see gap | See truth #7 |
| INT-01 | 02-01 | ✓ SATISFIED | See truth #8 |
| INT-02 | 02-01 | ✓ SATISFIED | See truth #9 |
| INT-03 | 02-01/02-02 | ✓ SATISFIED | See truth #10 |
| INT-04 | 02-05 | ✓ SATISFIED | See truth #11 |
| INT-05 | 02-05 | ✓ SATISFIED | See truth #12 |

No orphaned requirements found — all 12 requirement IDs mapped to this phase in REQUIREMENTS.md appear in at least one plan's `requirements`/`requirements-completed` field.

### Anti-Patterns Found

None. `TBD`/`FIXME`/`XXX`/`TODO`/`HACK`/`PLACEHOLDER`/"not yet implemented"/"coming soon" scans over `entc/*.go` and `runtime/*.go` (hand-written, non-generated source) returned zero matches.

### Scope Leakage Check (item 8 of the verification brief)

| Concern | Check | Result |
|---|---|---|
| Tier 2/3 CEL passthrough (Phase 3) | `grep 'cel-expr\|protovalidate/cel'` in go.mod; search for CEL env construction outside protovalidate's own boundary validator | Not present — `cel-go` appears only as an `// indirect` transitive dependency (via `buf.build/go/protovalidate`), not a direct entconnect usage |
| Flow binding / `codec/proto` (Phase 4) | `grep -rln 'entflow\|flow.Start\|GetRunStatus'` across `entc/`, `runtime/` | No matches |
| Drift-check build enforcement (Phase 5) | `entc/claims.go` — does `BuildClaims`/`checkConflicts` ever fail the build on `unclaimed`? | No — confirmed by direct read; only genuine duplicate claims (`checkConflicts`) are build-fatal |
| `grpc-gateway`/`genproto` HTTP-annotation deps | `grep -i 'grpc-gateway\|genproto'` in go.mod | `genproto/googleapis/{api,rpc}` present only as `// indirect` (transitive, likely via connect/otel/protovalidate tooling) — no direct dependency added |
| `entgo.io/contrib` (entgql/entproto) as a runtime dependency | `go list -m all` | Absent |

No scope leakage found in either direction.

### Known-State Items (context, not findings)

- `pipeline.sh` step 5 (`atlas migrate diff`) correctly `SKIP`s — confirmed live, exit code 0, matches the phase's documented known_state (no application ent schema exists yet, only test fixtures).
- REQUIREMENTS.md checkboxes remain unchecked for 11 of 12 IDs (only CRUD-03 is `[x]`) — orchestrator bookkeeping gap, not a delivery gap; see frontmatter `requirements_bookkeeping_gap`.
- `mixinforproto/v0.1.0` dependency edge resolves correctly (`go.mod` pins `github.com/smintz/entconnect/mixinforproto v0.1.0`, no `replace` directive present in `go.mod`).

### Human Verification Required

None triggered by this pass — every must-have resolved to either VERIFIED or a concretely evidenced gap; nothing was left ambiguous or behavior-unverifiable.

### Gaps Summary

One substantive gap: **CRUD-07's literal wording is not fully satisfied.** The generated wiring never hands the client *back* to application code (that half is real, structurally enforced, and correctly tested), but application code must itself construct the `*ent.Client` to call `NewServer`, meaning it always holds a live, unrestricted, unwrapped client in scope at the composition-root call site — nothing in the generated code prevents that same client from being used directly, bypassing the interceptor chain and ent privacy policies entirely. This is a real discrepancy between the phase's own design intent as stated in `02-CONTEXT.md`'s D-12 ("the *ent.Client is constructed inside generated wiring") and the actual, uniformly-applied implementation across all five generated `server.entconnect.go` files (`func NewServer(client *ent.Client, ...)`, client as constructor parameter). Given that constructing a DB client requires driver/DSN information only application code can supply, the stricter literal reading may simply be unachievable, and the weaker reading ("wiring never exposes the client it receives") is fully met — but that determination is a project judgment call, not something this verification pass can resolve unilaterally. Recommend either correcting CRUD-07's wording (the same treatment CRUD-03 already received in this phase) or recording an explicit override.

All eleven other requirement-level truths (CRUD-01…06, INT-01…05) are verified against real, passing code — not SUMMARY.md claims — including live test runs, live grep checks, and a live full-pipeline byte-stability proof from the current commit.

---

*Verified: 2026-08-14T10:47:07Z*
*Verifier: Claude (gsd-verifier)*
