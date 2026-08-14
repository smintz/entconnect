---
phase: 02-crud-handlers-interceptor-chain
plan: 01
subsystem: api
tags: [entc, connect-rpc, protobuf, ent, protovalidate, otel, go-workspaces, codegen]

# Dependency graph
requires:
  - phase: 01-mixinforproto-core
    provides: "mixinforproto.MixinForProto[M]/SourceMessage/SourceField provenance annotations, the errors.go failure/derivationError discipline, the committed FileDescriptorSet pipeline (scripts/pipeline.sh, scripts/generate-stubs.sh, scripts/check-stubs.sh)"
provides:
  - "entc/ — the entconnect entc.Extension: RPC-binding annotations (GetRPC/ListRPC/CreateRPC/UpdateRPC/DeleteRPC/Manual), procedure-string-to-descriptor resolution with no Go import edge to any generated Connect package, a per-verb generator registry, and a working OpGet generator emitting gofmt-clean Go via a gen.Hook"
  - "runtime/ — shipped (non-generated) framework code: Authenticator, the fixed authn->viewer->protovalidate->otel interceptor chain (Chain), Server/Route (no *ent.Client accessor), MapError, and runtime/viewer's first-party Viewer type"
  - "The root module's first dependency edge on github.com/smintz/entconnect/mixinforproto v0.1.0 (published tag), plus a root-module GOWORK=off CI job proving standalone resolution"
  - "proto/entconnecttest/v1/read.proto — the root module's first service-bearing synthetic corpus, with committed Connect stubs"
  - "internal/entconnecttest/read/ — a real, generated, tested fixture app proving the whole GetOrder path end to end over a real ent client and real HTTP"
affects: [02-02-write-handlers, 02-03-list-paging, 02-04-update-fieldmask, 02-05-manual-drift-report, phase-3-validation, phase-5-drift-check]

# Actuals (#2632)
actuals:
  tokens: 62700
  tasks: 3
  commits: 3

# Tech tracking
tech-stack:
  added:
    - "connectrpc.com/connect v1.20.0 (direct)"
    - "connectrpc.com/validate v0.6.0 (protovalidate interceptor)"
    - "connectrpc.com/otelconnect v0.9.0 (otel interceptor)"
    - "entgo.io/ent v0.14.6 (direct, root module's first use)"
    - "golang.org/x/tools v0.48.0 (imports.Process for hook-written file formatting)"
    - "modernc.org/sqlite v1.56.0 (CGO-free test driver)"
  patterns:
    - "entc.Extension skeleton (entproto precedent): Extension{entc.DefaultExtension}, NewExtension(opts...), Hooks() []gen.Hook, top-level Generate(g, e)"
    - "Procedure-string -> protoreflect.MethodDescriptor resolution against a committed FileDescriptorSet, zero Go import edge to any generated service package"
    - "gen.Hook writing files directly via os.WriteFile + imports.Process (not Extension.Templates(), which cannot target a sibling output directory)"
    - "Per-verb generator registry (RegisterGenerator/GenRequest/MethodImpl) so later CRUD plans add verbs without editing entc/extension.go"
    - "Explicit `option go_package` in proto source (not managed-mode-only) wherever entc/ code needs to read a Go import path off the descriptor set — buf build never applies managed-mode overrides"
    - "Aliased import (entconnectruntime) for the runtime package in all generated code, avoiding the runtime/runtime stdlib name collision"

key-files:
  created:
    - entc/binder.go
    - entc/resolve.go
    - entc/extension.go
    - entc/crud.go
    - entc/crud_get.go
    - entc/templates/service.tmpl
    - entc/templates/get.tmpl
    - runtime/interceptor.go
    - runtime/server.go
    - runtime/errormap.go
    - runtime/viewer/viewer.go
    - internal/entconnecttest/read/ent/schema/order.go
    - internal/entconnecttest/read/tracer_test.go
    - internal/entconnecttest/read/entconnect/order_read_service.entconnect.go
    - proto/entconnecttest/v1/read.proto
  modified:
    - go.mod
    - proto/buf.gen.yaml
    - scripts/generate-stubs.sh
    - scripts/check-stubs.sh
    - scripts/pipeline.sh
    - .github/workflows/ci.yml
    - Makefile

key-decisions:
  - "Task 1 checkpoint resolved v0-1-0 (publish mixinforproto/v0.1.0 now): tag pushed by the user from a machine with write credentials after this session's own credential was confirmed to lack tag-push/ref-delete permission (real 403 from GitHub, not a proxy policy block); root go.mod's mixinforproto require landed in its own commit referencing the pushed tag, per D-21"
  - "Order.id excluded from MixinForProto derivation (mixinforproto/reserved.go blocks 'id' as a reserved structural identifier); the entity uses ent's own default auto-increment int ID, with GetOrder converting between the wire's string id and ent's int ID at the boundary"
  - "runtime.MapError intentionally does NOT classify ent's *NotFoundError/*ConstraintError/*ValidationError: those types are generated per-application (entc/gen/template/base.tmpl), so a shared runtime package has no cross-app Go type to assert against without breaking its own 'shipped once, imported by every app' design. Generated handler bodies (get.tmpl) classify those three cases directly against the LOCAL app's own already-imported ent package, falling through to runtime.MapError only for privacy.Deny and the generic default"
  - "GetOrderRequest.id (not Order.customer) carries the tracer-exercised string.min_len=1 rule: GetOrderRequest never carries a customer field, so Order.customer's own rule is structurally unreachable by a GetOrder call — Order.customer's rule remains real and will be exercised directly by later Create/Update plans' own tracer tests"
  - "read.proto sets an explicit `option go_package` (identical to buf.gen.yaml's managed-mode override value) because the committed FileDescriptorSet is built via a separate `buf build` invocation that never applies managed-mode overrides — entc/'s Go-import-path derivation for emitted code reads this option directly off the descriptor set"

patterns-established:
  - "Pattern: entc/ extension code never imports a generated Connect/proto package (verified via negative grep in acceptance criteria) — descriptor resolution and Go-import-path derivation both go through protoreflect against the committed FileDescriptorSet"
  - "Pattern: every gen.Hook-written file is run through golang.org/x/tools/imports.Process before os.WriteFile, and any project-owned package whose name collides with a stdlib package (here: runtime) must be referenced via an explicit aliased import in template source, never left to goimports' own resolution"

requirements-completed: [CRUD-01, CRUD-06, CRUD-07, INT-01, INT-02, INT-03]

coverage:
  - id: D1
    description: "An ent-schema annotation naming a generated procedure constant (entconnect.GetRPC(...GetOrderProcedure)) resolves to a protoreflect.MethodDescriptor against the committed FileDescriptorSet, with no Go import edge from entc/ to any generated Connect package"
    requirement: "CRUD-01"
    verification:
      - kind: unit
        ref: "entc/resolve_test.go#TestResolveMethod (resolvable/malformed/unknown-service/unknown-method subtests)"
        status: pass
      - kind: other
        ref: "grep -rn 'orderv1connect|entconnecttestv1connect' entc/*.go — no matches"
        status: pass
    human_judgment: false
  - id: D2
    description: "A gofmt-clean, header-marked Connect GetOrder handler is emitted by a gen.Hook into a sibling package (internal/entconnecttest/read/entconnect/), and regenerating it twice is byte-identical"
    requirement: "CRUD-06"
    verification:
      - kind: integration
        ref: "go generate ./internal/entconnecttest/read/... run twice; sha256sum byte-identical; gofmt -l reports nothing"
        status: pass
    human_judgment: false
  - id: D3
    description: "Application code (the fixture) receives a Connect path + http.Handler via entconnect.NewServer/runtime.Server and has no exported route to the *ent.Client the generated wiring constructed"
    requirement: "CRUD-07"
    verification:
      - kind: other
        ref: "grep -rn 'func.*Client()' internal/entconnecttest/read/entconnect runtime/server.go — no matches"
        status: pass
      - kind: e2e
        ref: "internal/entconnecttest/read/tracer_test.go#TestGetOrder_RealRoundTrip"
        status: pass
    human_judgment: false
  - id: D4
    description: "Every request passes authn -> viewer injection -> protovalidate -> otel, in that fixed order, before reaching the handler body (connect.WithInterceptors argument order)"
    requirement: "INT-01"
    verification:
      - kind: other
        ref: "grep -n 'connect.WithInterceptors(' -A6 runtime/interceptor.go — literal order AuthnInterceptor, ViewerInterceptor, validateInterceptor, cfg.otel"
        status: pass
    human_judgment: false
  - id: D5
    description: "A request whose authenticator returns a context carrying no viewer is refused (CodeUnauthenticated) before the handler body runs — never treated as an anonymous allow"
    requirement: "INT-02"
    verification:
      - kind: e2e
        ref: "internal/entconnecttest/read/tracer_test.go#TestGetOrder_MissingViewerIsUnauthenticated"
        status: pass
    human_judgment: false
  - id: D6
    description: "An ent privacy denial (privacy.Deny sentinel, matched via errors.Is) reaches the Connect client as CodePermissionDenied"
    requirement: "INT-03"
    verification:
      - kind: e2e
        ref: "internal/entconnecttest/read/tracer_test.go#TestGetOrder_PrivacyDenyIsPermissionDenied"
        status: pass
      - kind: other
        ref: "grep -n 'errors.Is(err, privacy.Deny)' runtime/errormap.go — present; no `case *privacy.` type-switch form"
        status: pass
    human_judgment: false
  - id: D7
    description: "The root module's first dependency edge (mixinforproto v0.1.0) resolves as a genuine standalone Go module dependency, not only via go.work workspace visibility"
    verification:
      - kind: other
        ref: "make test-standalone-root (GOWORK=off go build/vet/test ./...) — pass; GOWORK=off go list -m github.com/smintz/entconnect/mixinforproto@v0.1.0 resolves through the module proxy"
        status: pass
    human_judgment: false

duration: 5h10min
completed: 2026-08-09
status: complete
---

# Phase 2 Plan 1: CRUD Handlers & Interceptor Chain — Walking Slice Summary

**A schema annotation naming a generated Connect procedure constant resolves against a committed FileDescriptorSet with zero Go import edge, an entc `gen.Hook` emits a gofmt-clean `GetOrder` handler into a sibling package, and a real Connect client gets a real row back from a real ent client through the fixed authn→viewer→protovalidate→otel chain — proven with `-race` tests over an in-memory SQLite database and a real HTTP round trip.**

## Performance

- **Duration:** ~5h10min (includes recovering a working Go toolchain, `buf`, and `protoc-gen-connect-go` in a sandbox with no pre-installed Go and a blocked `go.dev`, plus a mid-plan checkpoint pause/resume for the `mixinforproto/v0.1.0` tag push)
- **Started:** 2026-08-08T21:22:00Z (approx, session start)
- **Completed:** 2026-08-09T00:52:28Z
- **Tasks:** 3 (Task 1 checkpoint:decision — resolved `v0-1-0`; Task 2 auto; Task 3 tracer)
- **Files modified:** 54 (12 hand-written new + 27 ent-generated + 1 entconnect-generated + proto/scripts/CI/go.mod changes)

## Accomplishments

- The entc extension core (`entc/`) — annotation binders, procedure resolution, per-verb generator registry, and the `gen.Hook`-based emission mechanism — resolves RPC bindings purely against the committed descriptor set, never importing a generated Connect package itself.
- The `runtime/` framework package — `Authenticator`, the fixed `authn → viewer → protovalidate → otel` interceptor chain, `Server`/`Route` (no `*ent.Client` accessor), and `MapError` — is real, shipped, non-generated code an application imports once.
- A complete, generated, tested fixture app (`internal/entconnecttest/read/`) proves the whole path end to end: a real `ent.Client` backed by `modernc.org/sqlite`, a real HTTP server, and a real Connect client, asserting the happy path plus three negative paths (`CodeInvalidArgument` via protovalidate, `CodeUnauthenticated` via a missing viewer, `CodePermissionDenied` via a real ent privacy denial) — all under `-race`.
- The root module's first real dependency edge (`github.com/smintz/entconnect/mixinforproto v0.1.0`) is live and standalone-resolvable, proven by a new `make test-standalone-root` / CI `standalone` job step running `GOWORK=off`.

## Task Commits

Each task was committed atomically:

1. **Task 1: Decide the mixinforproto release tag (checkpoint:decision, resolved `v0-1-0`)** — tag `mixinforproto/v0.1.0` created locally on commit `fec3ca0` and pushed by the user (this session's credential lacks tag-push permission — confirmed via a real `403` from GitHub, not a proxy policy block)
2. **Task 2: Service-bearing proto corpus, Connect stub generation, root module's first dependencies** — `cf1e70e` (feat), `322c453` (feat — the mixinforproto dependency edge, its own commit per D-21)
3. **Task 3: End-to-end GetOrder tracer** — `becc571` (feat)

**Plan metadata:** *(this commit, docs: complete plan)*

## Files Created/Modified

- `entc/binder.go` — `GetRPC`/`ListRPC`/`CreateRPC`/`UpdateRPC`/`DeleteRPC`/`Manual`, `Bindings`/`Binding`/`Op`, `Bindings.Merge` (schema.Merger)
- `entc/decode.go` — error-returning `gen.Annotations` JSON round-trip
- `entc/resolve.go` — `SplitProcedure`/`LoadDescriptorSet`/`ResolveMethod`/`AllProcedures`
- `entc/resolve_test.go` — resolvable/malformed/unknown-service/unknown-method coverage
- `entc/errors.go` — `failure`/`generateError` aggregate (mixinforproto/errors.go discipline)
- `entc/extension.go` — `Extension`/`NewExtension`/`Hooks()`/`Generate(g, e)`, per-service file emission via `imports.Process`
- `entc/crud.go` — `GenRequest`/`MethodImpl`/`Generator`/`RegisterGenerator`, Go-import-path derivation off the descriptor set, `goCamelCase`
- `entc/crud_get.go` + `entc/templates/{service,get}.tmpl` — the `OpGet` generator
- `runtime/interceptor.go`, `runtime/server.go`, `runtime/errormap.go`, `runtime/viewer/viewer.go`, `runtime/doc.go`
- `proto/entconnecttest/v1/read.proto` — the root module's service-bearing corpus (`Order`/`GetOrderRequest`/`GetOrderResponse`/`OrderReadService`)
- `internal/entconnecttest/read/ent/schema/order.go`, `ent/entc.go`, `ent/generate.go`, plus the full generated `ent/` package and `entconnect/order_read_service.entconnect.go`
- `internal/entconnecttest/read/tracer_test.go` — the four end-to-end tests
- `proto/buf.gen.yaml`, `scripts/{generate-stubs,check-stubs,pipeline}.sh`, `.github/workflows/ci.yml`, `Makefile`, `go.mod`/`go.sum`/`go.work.sum`

## Decisions Made

See `key-decisions` in frontmatter above.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing Critical] Explicit `option go_package` needed in proto source, not managed-mode-only**
- **Found during:** Task 3, while designing `entc/`'s Go-import-path derivation for emitted code
- **Issue:** `proto/descriptorset.binpb` is built via a separate `buf build` invocation (pipeline step 3), which — unlike `buf generate` — never applies `buf.gen.yaml`'s managed-mode overrides. Without an explicit `option go_package`, the committed descriptor set carries an empty `go_package`, and `entc/`'s code generation (which must write real Go import paths into emitted handler source, per RESEARCH.md's own Q4 finding) had no way to determine them.
- **Fix:** Added `option go_package = "github.com/smintz/entconnect/internal/gen/entconnecttestv1";` to `read.proto`, identical to the managed-mode override value, so `buf build` and `buf generate` never disagree. `entc/crud.go`'s `goImportPath` reads it directly off `FileDescriptorProto.Options.GoPackage`.
- **Files modified:** `proto/entconnecttest/v1/read.proto`
- **Verification:** `go generate` succeeds, emitted file imports resolve correctly, `make build`/`vet`/`test` all pass
- **Committed in:** `becc571`

**2. [Rule 1 - Bug] `runtime` package name collides with the Go stdlib `runtime` package**
- **Found during:** Task 3, designing generated code that references the shared `runtime` package
- **Issue:** `github.com/smintz/entconnect/runtime`'s package name is `runtime`, identical to Go's standard library `runtime` package. Relying on `goimports`/`imports.Process` to auto-resolve a bare `runtime.X` reference in generated code risks silently inserting the stdlib import instead of the intended one.
- **Fix:** Every generated-code reference to the shared package uses an explicit aliased import, `entconnectruntime "github.com/smintz/entconnect/runtime"`, written directly into `entc/templates/service.tmpl` and `entc/templates/get.tmpl` rather than left to auto-resolution.
- **Files modified:** `entc/templates/service.tmpl`, `entc/templates/get.tmpl`, `runtime/errormap.go` (doc comment)
- **Verification:** generated file's import block correctly resolves to `github.com/smintz/entconnect/runtime`; `make build`/`vet` pass
- **Committed in:** `becc571`

**3. [Rule 1 - Bug] `runtime.MapError` cannot classify `ent.IsNotFound`/`IsConstraintError`/`IsValidationError` as a shared package**
- **Found during:** Task 3, implementing D-18's error mapping table
- **Issue:** `*ent.NotFoundError`/`*ent.ConstraintError`/`*ent.ValidationError` are generated PER APPLICATION (`entc/gen/template/base.tmpl`), not exported from core `entgo.io/ent`. A shared `runtime` package imported by every app has no single Go type to `errors.As` against without breaking its own "shipped once" design.
- **Fix:** `runtime.MapError` classifies only what is genuinely shareable (`privacy.Deny` → `PermissionDenied`, already-typed `*connect.Error` passthrough, default → `Internal`). The three ent-specific cases are classified directly inside generated handler code (`get.tmpl`), which already imports the app's own local `ent` package.
- **Files modified:** `runtime/errormap.go`, `entc/templates/get.tmpl`
- **Verification:** `runtime/errormap.go` contains `errors.Is(err, privacy.Deny)` and no `case *privacy.` type switch (acceptance criterion); generated handler correctly maps all four cases in the tracer test
- **Committed in:** `becc571`

**4. [Rule 3 - Blocking] `go:generate` directive's `-mod=mod` flag incompatible with workspace mode**
- **Found during:** Task 3, first `go generate` run against the fixture
- **Issue:** `go run -mod=mod entc.go` (the literal form PLAN.md's action text names) fails with `-mod may only be set to readonly or vendor when in workspace mode` since `go.work` is checked in.
- **Fix:** Dropped `-mod=mod` from the directive (`go run entc.go`); unnecessary in workspace mode since the workspace already resolves all module versions.
- **Files modified:** `internal/entconnecttest/read/ent/generate.go`
- **Verification:** `go generate ./internal/entconnecttest/read/...` succeeds
- **Committed in:** `becc571`

**5. [Rule 3 - Blocking] `scripts/pipeline.sh` step 5's atlas-schema-dir detection false-positived on the new test fixture**
- **Found during:** Task 3, first full pipeline run after adding `internal/entconnecttest/read/ent/schema/`
- **Issue:** Step 5's `find . -type d -name schema -path '*/ent/schema' -not -path './mixinforproto/internal/*'` matched the new fixture's schema directory, demanding `atlas` be installed for a test fixture that was never meant to need migration.
- **Fix:** Extended the exclusion to `-not -path './internal/entconnecttest/*'`, matching the existing `mixinforproto/internal/*` exclusion's intent (test fixtures, not application schemas).
- **Files modified:** `scripts/pipeline.sh`
- **Verification:** `bash scripts/pipeline.sh` step 5 correctly skips with the existing "no application ent/schema package exists yet" message
- **Committed in:** `becc571`

**6. [Rule 2 - Missing Critical] `GetOrderRequest.id` needed its own `string.min_len` rule for the tracer test to exercise protovalidate on GetOrder**
- **Found during:** Task 3, writing the tracer test's negative assertions
- **Issue:** PLAN.md's action text places the tracer-exercised `string.min_len` rule on `Order.customer`, but `GetOrderRequest` (the actual message protovalidate validates on a `GetOrder` call) never carries a `customer` field — that rule can never reject a `GetOrder` request.
- **Fix:** Added an identical `string.min_len = 1` rule to `GetOrderRequest.id`, which the tracer test's `TestGetOrder_ProtovalidateRejectsEmptyID` now exercises directly. `Order.customer`'s rule remains in the corpus for later Create/Update plans' own tracer tests.
- **Files modified:** `proto/entconnecttest/v1/read.proto`
- **Verification:** `TestGetOrder_ProtovalidateRejectsEmptyID` passes, asserting `CodeInvalidArgument`
- **Committed in:** `becc571`

**7. [Rule 1 - Bug] Root module's `mixinforproto` dependency edge could not be published within this sandbox's credentials**
- **Found during:** Task 1's checkpoint execution (pushing the `mixinforproto/v0.1.0` tag)
- **Issue:** This session's git credential to `origin` can create/update branches but returns a real `403 Forbidden` from GitHub itself (not the egress proxy) on tag pushes and ref deletions — confirmed via a probe branch push (succeeded) versus a tag push (failed) and a branch delete (also failed).
- **Fix:** The plan was halted mid-execution with a `checkpoint:human-action` report; the user pushed the already-created local tag from a machine with sufficient permissions. Execution resumed once the orchestrator confirmed `refs/tags/mixinforproto/v0.1.0` existed on `origin` and dereferenced to the exact commit tagged locally (`fec3ca0`).
- **Files modified:** none (infrastructure/credentials, not code)
- **Verification:** `GOWORK=off go list -m github.com/smintz/entconnect/mixinforproto@v0.1.0` resolves through the module proxy; `make test-standalone-root` passes
- **Committed in:** N/A (git tag push, not a repo commit)

---

**Total deviations:** 7 auto-fixed (2 missing-critical, 4 bug, 1 blocking) plus 1 externally-resolved credential blocker
**Impact on plan:** All auto-fixes were necessary for the generated code to actually compile/run correctly or for the pipeline to remain honest about what it covers; none expanded scope beyond the plan's own stated GetOrder-only slice. The credential blocker was resolved by the user outside this session, exactly as the plan's own checkpoint anticipated tag-push being "the one irreversible action... explicitly authorized."

## Issues Encountered

- **No Go toolchain, `buf`, or `protoc-gen-connect-go` pre-installed in this sandbox, and `go.dev` (Go's default toolchain-auto-switch fallback) blocked by egress policy.** Resolved by fetching a real `go1.26.0` toolchain through `proxy.golang.org` (allowlisted, unlike `go.dev`) via the `golang.org/toolchain` pseudo-module's proxy-redirect-to-`storage.googleapis.com` path, symlinking it to `/usr/local/go` (already on `PATH`), and `go install`-ing `buf@v1.72.0`/`protoc-gen-go@v1.36.11`/`protoc-gen-connect-go@v1.20.0` into `$HOME/go/bin`. No workaround of any policy denial was needed — `proxy.golang.org` was already explicitly allowlisted.
- **`mixinforproto/v0.1.0` tag could not be pushed by this session's credential (403 from GitHub).** See Deviation 7 above. A stray disposable probe branch (`test-push-probe-DELETE-ME`) pushed while diagnosing this remains on `origin` and could not be deleted by this session (same permission gap) — a human with delete permission should remove it.

## User Setup Required

None — no external service configuration required. (The `mixinforproto/v0.1.0` tag push was a one-time, already-completed exception documented above, not a recurring setup step.)

## Next Phase Readiness

- The architectural slice (annotation → resolution → hook-based emission → fixed interceptor chain → real ent client) is proven end to end and ready for 02-02 (Create/Delete) to widen without revisiting the resolution or emission mechanisms.
- `entc/crud.go`'s `RegisterGenerator` registry means 02-02/02-03/02-04 add verbs without touching `entc/extension.go`.
- Outstanding, non-blocking: the stray `test-push-probe-DELETE-ME` branch on `origin` needs manual deletion by someone with sufficient GitHub permissions.
- D-07's sorted claims report (`crud:<op>`/`manual`/`unclaimed`) remains explicitly deferred to 02-05, per the plan's own artifact table — not started here.

---
*Phase: 02-crud-handlers-interceptor-chain*
*Completed: 2026-08-09*

## Self-Check: PASSED

All 15 key created files verified present on disk; all 4 commit hashes (`cf1e70e`, `322c453`, `becc571`, `0523426`) verified present in `git log --oneline --all`. No missing items.
