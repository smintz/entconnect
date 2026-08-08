---
phase: 01-mixinforproto-core
plan: 01
subsystem: mixinforproto
tags: [go, protoreflect, ent, entc, protovalidate, buf, mixin, walking-skeleton]

# Dependency graph
requires: []
provides:
  - "Two-module Go repo scaffold: go.work + root github.com/smintz/entconnect + github.com/smintz/entconnect/mixinforproto"
  - "MixinForProto[M proto.Message](opts ...Option) ent.Mixin and Validate[M proto.Message](opts ...Option) error"
  - "SourceMessage/SourceField annotation contract (ContractVersion=1) proven to survive a real entc schema-load subprocess"
  - "Pure derive[M]/mapField seam Plans 02-04 extend without touching the mixin adapter"
  - "proto/mixinforprototest/v1/tracer.proto test corpus (Tracer, Empty, MultiField, Unsupported) plus committed generated Go stub"
affects: [01-02, 01-03, 01-04, 01-05]

# Actuals (#2632)
actuals:
  tokens: 13338
  tasks: 3
  commits: 1

# Tech tracking
tech-stack:
  added:
    - "entgo.io/ent v0.14.6 (direct)"
    - "google.golang.org/protobuf v1.36.11 (direct)"
    - "buf.build/go/protovalidate v1.2.0 (direct)"
    - "buf CLI v1.72.0 (go install, not committed as a dependency)"
    - "protoc-gen-go v1.36.11 (go install, not committed as a dependency)"
  patterns:
    - "Pure derive[M] core + panicking ent.Mixin adapter split (D-06), so Validate[M] is free"
    - "protoreflect descriptor walk by index only, never via a Go map (D-24 determinism)"
    - "Annotations are the only channel across entc's schema-load JSON boundary; proven with a real entc.LoadGraph subprocess run, not a unit test"
    - "buf managed-mode go_package override (per-file, not prefix-only) to get a flat mixinforprototestv1 Go package directory from a dotted/versioned proto package"

key-files:
  created:
    - go.work
    - go.mod
    - mixinforproto/go.mod
    - mixinforproto/annotation.go
    - mixinforproto/option.go
    - mixinforproto/derive.go
    - mixinforproto/fieldmap.go
    - mixinforproto/mixin.go
    - mixinforproto/derive_test.go
    - proto/buf.yaml
    - proto/buf.gen.yaml
    - proto/mixinforprototest/v1/tracer.proto
    - mixinforproto/internal/gen/mixinforprototestv1/tracer.pb.go
    - mixinforproto/internal/boundarytest/ent/schema/tracer.go
    - mixinforproto/internal/boundarytest/boundary_test.go
  modified: []

key-decisions:
  - "Task 1 checkpoint (resolved by resolution block): module path github.com/smintz/entconnect/mixinforproto, nested module, mixinforproto/vX.Y.Z tag convention documented — no tag pushed."
  - "Task 2 checkpoint (resolved by resolution block): integer ContractVersion=1 on both annotation structs, additive-only, exact v1 struct layouts."
  - "SourceField's proto-field-name accessor is Go field FieldName (json:\"name\"), not Name — a struct cannot declare both a field and a method named Name, and schema.Annotation fixes Name() string. Wire/JSON shape is unchanged from the decision text; only the Go identifier differs."
  - "fieldmap.go calls protovalidate.ResolveFieldRules for the one StringKind case and handles its (nil, nil) constraint-free return without dereferencing, satisfying the ANNO-02 empty edge and giving buf.build/go/protovalidate a genuine direct-dependency usage (rather than an unused go.mod pin that `go mod tidy` would strip)."
  - "buf.gen.yaml uses a file-scoped managed-mode go_package override (not the bare go_package_prefix override) to land the generated stub at the exact flat path mixinforproto/internal/gen/mixinforprototestv1/tracer.pb.go named in the plan, since go_package_prefix alone mirrors the proto package's dotted path (mixinforprototest/v1) rather than flattening it."
  - "mixinforproto/go.mod's go directive is pinned to go 1.24.0 with an explicit toolchain go1.24.7 line, and github.com/rogpeppe/go-internal is held at v1.14.0 (rather than the v1.16.0 `go mod tidy` initially selected) — v1.16.0 requires go>=1.25 and is pulled in only transitively through entc/gen's own test-dependency graph (ariga.io/atlas -> hcl -> hclsyntax's test files), not by anything mixinforproto imports directly."

patterns-established:
  - "Pattern: pure derive[M ] core / panicking ent.Mixin wrapper split — every later mapping/translation path (Plans 02-04) extends derive.go's error-collecting loop and fieldmap.go's mapField switch, never touches mixin.go's panic isolation."
  - "Pattern: SourceMessage.Fields is the COMPLETE descriptor inventory, populated unconditionally before any Exclude/Override check — Plans 02/03 must not narrow this to the derived subset."

requirements-completed: [MIX-01, MIX-13, MIX-14, ANNO-01, ANNO-02, ANNO-04, PIPE-02, PIPE-04]

coverage:
  - id: D1
    description: "MixinForProto[*Tracer]() declared in a real ent schema materializes one derived ent.Field (string \"name\") at schema-load time, with no committed descriptor file and no string message name in the call"
    requirement: "MIX-01"
    verification:
      - kind: unit
        ref: "mixinforproto/derive_test.go#TestDerive_TracerMapsSingleStringField"
        status: pass
      - kind: integration
        ref: "mixinforproto/internal/boundarytest/boundary_test.go#TestAnnotationsCrossSchemaLoadBoundary"
        status: pass
    human_judgment: false
  - id: D2
    description: "Both SourceMessage and SourceField annotations survive a real entc schema-load subprocess and decode back out of a real gen.Graph via encoding/json"
    requirement: "ANNO-01"
    verification:
      - kind: integration
        ref: "mixinforproto/internal/boundarytest/boundary_test.go#TestAnnotationsCrossSchemaLoadBoundary"
        status: pass
    human_judgment: false
  - id: D3
    description: "SourceField carries exact proto field name/number, and empty (not absent) constraint-ID lists for a constraint-free field, with protovalidate.ResolveFieldRules's (nil, nil) return handled without dereferencing"
    requirement: "ANNO-02"
    verification:
      - kind: unit
        ref: "mixinforproto/derive_test.go#TestDerive_SourceFieldAnnotationEncoding"
        status: pass
      - kind: unit
        ref: "mixinforproto/derive_test.go#TestResolveFieldRules_NilForConstraintFreeField"
        status: pass
    human_judgment: false
  - id: D4
    description: "ContractVersion=1 integer version marker present and non-zero on both decoded annotation structs after crossing the schema-load boundary"
    requirement: "ANNO-04"
    verification:
      - kind: integration
        ref: "mixinforproto/internal/boundarytest/boundary_test.go#TestAnnotationsCrossSchemaLoadBoundary"
        status: pass
    human_judgment: false
  - id: D5
    description: "Derived field order matches proto declaration order and is stable across repeated runs and concurrent goroutines"
    requirement: "MIX-13"
    verification:
      - kind: unit
        ref: "mixinforproto/derive_test.go#TestDerive_FieldOrderMatchesDeclarationOrder"
        status: pass
      - kind: unit
        ref: "mixinforproto/derive_test.go#TestDerive_DeterministicAcrossRepeatedCalls"
        status: pass
      - kind: unit
        ref: "mixinforproto/derive_test.go#TestDerive_ConcurrentSafe (go test -race -count=5)"
        status: pass
    human_judgment: false
  - id: D6
    description: "mixinforproto builds/vets/tests standalone under GOWORK=off with exactly three direct non-test dependencies (ent, protobuf, protovalidate) and no cel-go/cel-expr module"
    requirement: "MIX-14"
    verification:
      - kind: other
        ref: "GOWORK=off go build ./... && GOWORK=off go vet ./... && GOWORK=off go test -race -count=5 ./... (mixinforproto/)"
        status: pass
      - kind: other
        ref: "go mod edit -json mixinforproto/go.mod — 3 direct Require entries"
        status: pass
    human_judgment: false
  - id: D7
    description: "go.work + GOWORK=off CI-equivalent job both resolve mixinforproto correctly; no replace directive in any committed go.mod"
    requirement: "PIPE-02"
    verification:
      - kind: other
        ref: "go build ./mixinforproto/... && go test ./mixinforproto/... from repo root (workspace mode), plus GOWORK=off from mixinforproto/"
        status: pass
    human_judgment: false
  - id: D8
    description: "Nested-module tag convention (mixinforproto/vX.Y.Z) documented in the checkpoint resolution and this summary; no tag pushed during execution"
    requirement: "PIPE-04"
    verification: []
    human_judgment: true
    rationale: "Tagging is a deliberate release-time human action per the Task 1 resolution, not something execution verifies automatically — documenting the convention is the artifact, not a test."

duration: 55min
completed: 2026-08-08
status: complete
---

# Phase 1 Plan 1: MixinForProto Walking Skeleton Summary

**One `.proto` string field survives the full contract-to-schema path — `buf generate` → `MixinForProto[*Tracer]()` in a real ent schema → a real `entc` schema-load subprocess → a derived `ent.Field` plus `SourceMessage`/`SourceField` provenance annotations, decoded back out of the resulting `gen.Graph`.**

## Performance

- **Duration:** 55 min
- **Started:** 2026-08-08T10:07:00Z (approx, first scaffold write)
- **Completed:** 2026-08-08T11:02:00Z
- **Tasks:** 3 (2 checkpoint:decision, both pre-resolved; 1 tracer)
- **Files modified:** 16 (all new)

## Accomplishments

- Two-module Go scaffold (`go.work`, root `go.mod`, `mixinforproto/go.mod`) building and testing green, `mixinforproto` standalone-buildable under `GOWORK=off` with exactly three direct non-test dependencies.
- `buf lint`/`buf generate` pipeline produces a committed Go stub (`mixinforprototestv1`) from a real `.proto` corpus covering the tracer field, the zero-field edge case, a multi-field ordering fixture, and an unsupported-kind fixture.
- `mixinforproto`'s pure `derive[M]`/`mapField` core, the `Option`/`options` seam, the `SourceMessage`/`SourceField` annotation contract, and the `MixinForProto[M]`/`Validate[M]` public API are all implemented and unit-tested (11 tests in `derive_test.go`, all covering the plan's must-have edge probes: empty, encoding, concurrency, determinism).
- `TestAnnotationsCrossSchemaLoadBoundary` runs a real `entc.LoadGraph` subprocess against a real `ent.Schema` declaring `MixinForProto[*Tracer]()` and decodes both annotations back out of the resulting `gen.Graph` via a stdlib `encoding/json` round-trip — the one risk this plan exists to retire, now proven, not assumed.

## Task Commits

Both checkpoint:decision tasks (Task 1: module path/tag convention, Task 2: annotation contract shape/versioning) carried pre-resolved `<resolution>` blocks and produced no artifacts of their own — their decisions are recorded above under `key-decisions` and applied directly in Task 3's code.

1. **Task 3: End-to-end "one proto field becomes one annotated ent field"** - `a4b9732` (feat)

**Plan metadata:** committed alongside this SUMMARY (see below).

## Files Created/Modified

- `go.work` - workspace listing `.` and `./mixinforproto`, `go 1.26`
- `go.mod` - root module `github.com/smintz/entconnect`, `go 1.26`, dependency-free (D-18)
- `mixinforproto/go.mod` / `go.sum` - independent module, `go 1.24.0` / `toolchain go1.24.7`, three direct requires
- `mixinforproto/annotation.go` - `ContractVersion`, `MixinForProtoMessage`/`MixinForProtoField` keys, `SourceMessage`, `SourceField`
- `mixinforproto/option.go` - `Option`/`options` seam with excluded/overridden/asJSON lookup helpers (no exported constructors yet)
- `mixinforproto/derive.go` - pure `derive[M proto.Message](opts...) (*derivation, error)`, error-collecting descriptor walk
- `mixinforproto/fieldmap.go` - `mapField` for `protoreflect.StringKind`, calling `protovalidate.ResolveFieldRules` and guarding its nil
- `mixinforproto/mixin.go` - `MixinForProto[M]` (panicking `ent.Mixin` adapter) and `Validate[M]` (MIX-12 debug entry point)
- `mixinforproto/derive_test.go` - 12 tests covering the tracer path, empty/encoding/determinism/concurrency edges, unsupported-kind failure, and the `ResolveFieldRules` nil-safety edge
- `proto/buf.yaml` / `proto/buf.gen.yaml` - buf v2 config; managed mode with a file-scoped `go_package` override
- `proto/mixinforprototest/v1/tracer.proto` - `Tracer`, `Empty`, `MultiField`, `Unsupported` test messages
- `mixinforproto/internal/gen/mixinforprototestv1/tracer.pb.go` - committed generated stub (D-22: `go test` needs no `buf` on the machine)
- `mixinforproto/internal/boundarytest/ent/schema/tracer.go` - real `ent.Schema` declaring `MixinForProto[*Tracer]()`
- `mixinforproto/internal/boundarytest/boundary_test.go` - `TestAnnotationsCrossSchemaLoadBoundary`, the real `entc.LoadGraph` subprocess proof

## Decisions Made

- **Checkpoint Task 1 (module path/tags):** `github.com/smintz/entconnect/mixinforproto`, nested module, `mixinforproto/vX.Y.Z` tag convention documented in this summary and in the mixin's own doc comments; no git tag pushed during execution (deliberate — tagging is a separate, human-owned release action).
- **Checkpoint Task 2 (annotation shape/versioning):** integer `ContractVersion` starting at 1 on both `SourceMessage` and `SourceField`, additive-only between bumps, exact v1 struct layouts as specified in the resolution block (modulo the `Name`→`FieldName` Go-identifier fix below).
- **Reconciliation R1 applied:** no `cel-go`/`cel-expr` dependency added; verified via `go list -m all` containing no `github.com/cel-expr/` module. Deferred note carried forward: Phase 3 must re-run `go get github.com/cel-expr/cel-go/cel@latest` before choosing an import path, since RESEARCH.md's finding was time-boxed to 7 days from 2026-08-08.
- **buf managed-mode approach:** rather than the plain `go_package_prefix` override (which mirrors the proto package's own dotted/versioned directory structure — `mixinforprototest/v1/tracer.pb.go`, not the flat path the plan names), used a file-scoped `override: file_option: go_package` entry with an explicit full import path, combined with `opt: module=<module>` on the plugin, to land the stub at the exact `mixinforproto/internal/gen/mixinforprototestv1/tracer.pb.go` path named in the plan's `files_modified`. This pattern will need to generalize (a per-path override, or a naming convention the override targets by directory) once Plan 05's real conformance corpus adds more proto files.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] `SourceField.Name` renamed to `SourceField.FieldName` (Go identifier only; JSON key unchanged)**
- **Found during:** Task 3 (writing `annotation.go`)
- **Issue:** The checkpoint-resolved struct shape names a field `Name` on `SourceField`, but `SourceField` must also implement `schema.Annotation`'s `Name() string` method. Go does not allow a struct to declare both a data field and a method with the same identifier — `go build` fails with `field and method with the same name Name`.
- **Fix:** Renamed the Go field to `FieldName` with `json:"name"` preserved, so the on-the-wire/JSON shape exactly matches the checkpoint decision; only the Go-level accessor identifier differs.
- **Files modified:** `mixinforproto/annotation.go`, `mixinforproto/fieldmap.go`, `mixinforproto/derive_test.go`, `mixinforproto/internal/boundarytest/boundary_test.go`
- **Verification:** `go build ./...` clean; `TestDerive_SourceFieldAnnotationEncoding` and `TestAnnotationsCrossSchemaLoadBoundary` both assert `FieldName == "name"` after decode.
- **Committed in:** `a4b9732` (Task 3 commit)

**2. [Rule 3 - Blocking] `buf.build/go/protovalidate` stripped by `go mod tidy` as an unused direct dependency**
- **Found during:** Task 3 (running `go mod tidy` after the initial implementation)
- **Issue:** `go mod tidy` removed `buf.build/go/protovalidate` from `go.mod` entirely because Plan 01's original code never called any of its functions (Tier 1 translation is Plan 04's job) — leaving only 2 of the 3 required direct dependencies, failing MIX-14's acceptance criterion.
- **Fix:** Wired a genuine, in-scope call in `fieldmap.go`'s `StringKind` case: `protovalidate.ResolveFieldRules(fd)`, explicitly handling its documented `(nil, nil)` return for a constraint-free field without dereferencing. This satisfies the plan's own ANNO-02 "empty" must-have (verified live against the real function) and keeps `protovalidate` a real, actually-imported direct dependency rather than an inert `go.mod` pin `go mod tidy` will keep stripping. No Tier 1 translation logic was added — `TranslatedIDs`/`ResidualIDs` stay empty as the plan specifies.
- **Files modified:** `mixinforproto/fieldmap.go`, `mixinforproto/derive_test.go` (added `TestResolveFieldRules_NilForConstraintFreeField`)
- **Verification:** `go mod edit -json mixinforproto/go.mod` lists exactly 3 direct requires after `go mod tidy`; all tests pass.
- **Committed in:** `a4b9732` (Task 3 commit)

**3. [Rule 3 - Blocking] `go mod tidy` bumped the `go` directive to 1.25 via a transitive test-dependency chain**
- **Found during:** Task 3 (running `go mod tidy` to resolve `entc/gen`'s dependency graph for the boundary test)
- **Issue:** `entc/gen` pulls `ariga.io/atlas`, which pulls `hcl/v2`, whose own `_test.go` files (not anything mixinforproto imports) pull `github.com/rogpeppe/go-internal@v1.16.0`, which requires `go >= 1.25` — `go mod tidy` silently bumped `mixinforproto/go.mod`'s `go` directive from 1.24 to 1.25, contradicting the plan's explicit `go 1.24` target (the highest floor of the three actual direct dependencies).
- **Fix:** Pinned `github.com/rogpeppe/go-internal` down to `v1.14.0` (satisfies the same callers, no `go >= 1.25` requirement), then re-ran `go mod edit -go=1.24` and `go mod tidy`, landing on `go 1.24.0` with an explicit `toolchain go1.24.7` line.
- **Files modified:** `mixinforproto/go.mod`, `mixinforproto/go.sum`
- **Verification:** `GOWORK=off go build/vet/test -race -count=5 ./...` all pass under the local go1.24.7 toolchain with this pin in place.
- **Committed in:** `a4b9732` (Task 3 commit)

---

**Total deviations:** 3 auto-fixed (all Rule 3 — blocking compile/tooling issues)
**Impact on plan:** All three were necessary to make the plan's own acceptance criteria (a compiling `SourceField`, a genuine 3-direct-dependency `go.mod`, a `go 1.24` floor) achievable at all. No scope creep — no Tier 1/2 translation logic, no new option constructors, no additional field-kind mappings were added.

## Issues Encountered

- **buf managed-mode go_package_prefix does not flatten dotted proto packages.** Initial attempts using `managed.override: file_option: go_package_prefix` (as the plan's action text literally suggests) produced a nested `mixinforprototest/v1/tracer.pb.go` output path (or, without `paths=source_relative`, a doubly-nested path under the full import path), never the flat `mixinforprototestv1/tracer.pb.go` the plan's `files_modified` names exactly. Resolved by using a file-scoped `file_option: go_package` override with the full desired import path plus `opt: module=<module>` on the plugin (see Decisions Made) — confirmed empirically against the real `buf` v1.72.0 binary rather than assumed from documentation.
- **`buf`/`protoc-gen-go` were not preinstalled**, as the plan's `<precondition>` anticipated. Installed via `go install github.com/bufbuild/buf/cmd/buf@v1.72.0` and `go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.11`, both succeeding on the first attempt (matches RESEARCH.md's verified installation path). Neither is a `go.mod` dependency; both must be installed the same way in CI.

## User Setup Required

None - no external service configuration required. Note for CI/environment setup: `buf` (`go install github.com/bufbuild/buf/cmd/buf@v1.72.0`) and `protoc-gen-go` (`go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.11`) must be installed to regenerate `tracer.pb.go`, but are not required to build or test `mixinforproto` — the generated stub is committed (D-22).

## Next Phase Readiness

- The `derive[M]`/`mapField`/`Option` seams are in place and unit-tested; Plan 02 (broader field mapping: scalars, enums, WKTs, presence, maps, oneof) extends `fieldmap.go`'s `switch` and `derive.go`'s classification without touching `mixin.go`.
- Plan 03 (`Exclude`/`Override`) extends `option.go`'s already-present lookup helpers with the first exported option constructors.
- Plan 04 (Tier 1 constraint translation) extends `fieldmap.go`'s already-wired `protovalidate.ResolveFieldRules` call to actually populate `TranslatedIDs`/`ResidualIDs` instead of leaving them empty.
- **Carry-forward flag (from the Task 2 resolution):** the annotation contract shape stays cheaply revisable until Phase 2 actually decodes it — flag `SourceMessage`/`SourceField`'s shape for a second look at the Phase 1→2 transition, before the entc extension starts consuming it. This includes the `FieldName` rename documented above, which Phase 2's decoder must use verbatim.
- **Carry-forward flag (from R1):** re-verify `github.com/cel-expr/cel-go`'s module-path migration status before Phase 3 planning; `github.com/google/cel-go` (transitively pinned at v0.28.0 via `buf.build/go/protovalidate`) is still the only working import as of this plan's execution.

---
*Phase: 01-mixinforproto-core*
*Completed: 2026-08-08*
