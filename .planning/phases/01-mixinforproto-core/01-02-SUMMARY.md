---
phase: 01-mixinforproto-core
plan: 02
subsystem: mixinforproto
tags: [go, protoreflect, ent, entc, protovalidate, buf, mixin, field-mapping, golden-tests]

# Dependency graph
requires:
  - phase: 01-01
    provides: "derive[M]/mapField seam, Option/options struct, SourceMessage/SourceField annotation contract, MixinForProto[M]/Validate[M] public API, buf pipeline, committed corpus stubs"
provides:
  - "Complete Phase 1 field-mapping table: all 15 proto scalar kinds, enums, three well-known types (Timestamp/Struct/Value; FieldMask/Duration skipped), proto3 presence (optional vs plain), scalar-valued maps (message-valued maps skipped), and opt-in AsJSON for message fields"
  - "classify(fd) — the single, RESEARCH.md-verified-order classification seam separating MIX-05 (optional scalar) from MIX-10 (real oneof member) via ContainingOneof().IsSynthetic()"
  - "AsJSON(name string) Option plus validateAsJSON's three-case failure surface (empty name, unknown name, non-message-typed field)"
  - "mixinforproto/internal/fieldproj — the R4 golden-comparable projection struct/Project/ProjectAll, decoupled from mixinforproto via a JSON round-trip to avoid an import cycle"
  - "Seven corpus .proto files (scalars, enums, wkt, presence, maps, messages, oneofs) plus committed generated stubs, and eight golden fixtures under mixinforproto/testdata/"
affects: [01-03, 01-04, 01-05]

# Actuals (#2632)
actuals:
  tokens: 30251
  tasks: 2
  commits: 2

# Tech tracking
tech-stack:
  added:
    - "github.com/sebdah/goldie/v2 v2.8.0 (direct, test-only) — golden-file assertions per the entgql/entproto precedent"
  patterns:
    - "classify(fd) branch order is load-bearing: IsMap() -> real-oneof (ContainingOneof()!=nil && !IsSynthetic()) -> HasOptionalKeyword() -> MessageKind -> EnumKind -> plain scalar"
    - "Golden-comparable projection struct (fieldproj.Field), never json.Marshal on the raw *field.Descriptor — its Validators []any and Err fields are not JSON-safe"
    - "stableDefault renders via '%#v', not fmt.Sprint, so a zero-value string Default(\"\") is distinguishable from 'no default set' (both would otherwise render as the same empty string)"
    - "Post-loop validation pass (validateAsJSON) for option-name checks that need the full message field set, layered onto derive.go's existing per-field loop rather than duplicating context inside mapField"

key-files:
  created:
    - mixinforproto/internal/fieldproj/fieldproj.go
    - mixinforproto/internal/fieldproj/fieldproj_test.go
    - mixinforproto/fieldmap_test.go
    - mixinforproto/testdata/scalars.golden
    - mixinforproto/testdata/enums.golden
    - mixinforproto/testdata/wkt.golden
    - mixinforproto/testdata/presence.golden
    - mixinforproto/testdata/maps.golden
    - mixinforproto/testdata/messages.golden
    - mixinforproto/testdata/oneofs.golden
    - mixinforproto/testdata/empty.golden
    - proto/mixinforprototest/v1/scalars.proto
    - proto/mixinforprototest/v1/enums.proto
    - proto/mixinforprototest/v1/wkt.proto
    - proto/mixinforprototest/v1/presence.proto
    - proto/mixinforprototest/v1/maps.proto
    - proto/mixinforprototest/v1/messages.proto
    - proto/mixinforprototest/v1/oneofs.proto
    - mixinforproto/internal/gen/mixinforprototestv1/{scalars,enums,wkt,presence,maps,messages,oneofs}.pb.go
  modified:
    - mixinforproto/fieldmap.go
    - mixinforproto/option.go
    - mixinforproto/derive.go
    - mixinforproto/derive_test.go
    - proto/buf.gen.yaml
    - mixinforproto/go.mod
    - mixinforproto/go.sum

key-decisions:
  - "google.protobuf.Value maps to field.JSON typed json.RawMessage (not a concrete struct type), because it round-trips any JSON value losslessly — closes RESEARCH.md Open Question 2."
  - "google.protobuf.Struct maps to field.JSON typed map[string]any, matching its own natural JSON shape."
  - "Scalar-valued maps (MIX-06) map to field.JSON typed as a reflect-constructed map[K]V of the mapped key/value Go types (built via reflect.MapOf, not a fixed set of hardcoded cases), so the mapping generalizes beyond the two map shapes the corpus happens to test."
  - "SourceField.Kind is set to the derivation-class name (\"scalar\" / \"optionalScalar\" / \"enum\" / \"wkt\" / \"scalarMap\" / \"asJSON\"), not the raw protoreflect.Kind string Plan 01 used — matching D-02's documented intent (\"derivation kind (scalar / enum / WKT / map-as-JSON / message-as-JSON / override)\")."
  - "buf.gen.yaml's managed-mode override was generalized from Plan 01's single per-file entry (path: tracer.proto) to a module-wide entry with no path restriction, since every corpus file needs the identical flat go_package — verified empirically against the real buf v1.72.0 binary that this covers all files, including tracer.proto, without regenerating a byte-different tracer.pb.go."

patterns-established:
  - "Pattern: classify() is the single seam a later plan (03) extends to add the panic-unless-resolved gate for real-oneof members — it must not duplicate or reorder the branch logic here."
  - "Pattern: any Option whose validation needs full-message context (not just the current field) gets a post-loop validate*(msgName, md, o) function called once from derive.go, rather than being threaded into every per-kind mapping function."

requirements-completed: [MIX-02, MIX-03, MIX-04, MIX-05, MIX-06, MIX-09, MIX-13]

coverage:
  - id: D1
    description: "All 15 proto scalar kinds derive to their same-width ent builder with no widening (int32 stays int32, not int64; sint32/sfixed32 also land on int32; etc.), golden-asserted over the Scalars corpus message"
    requirement: "MIX-02"
    verification:
      - kind: unit
        ref: "mixinforproto/fieldmap_test.go#TestScalarsCoverAllKinds"
        status: pass
      - kind: unit
        ref: "mixinforproto/fieldmap_test.go#TestGolden/Scalars"
        status: pass
    human_judgment: false
  - id: D2
    description: "Enum fields derive to field.Enum(name).Values(...) carrying the declared value names verbatim, built by index-order iteration of EnumDescriptor.Values()"
    requirement: "MIX-03"
    verification:
      - kind: unit
        ref: "mixinforproto/fieldmap_test.go#TestGolden/Enums"
        status: pass
    human_judgment: false
  - id: D3
    description: "Timestamp derives to field.Time; Struct and Value derive to JSON fields; FieldMask and Duration are skipped entirely — asserted by exact field count and by-name absence checks"
    requirement: "MIX-04"
    verification:
      - kind: unit
        ref: "mixinforproto/fieldmap_test.go#TestGolden/Wkt"
        status: pass
    human_judgment: false
  - id: D4
    description: "A proto3 optional scalar derives Nillable().Optional() with no default; a plain proto3 scalar derives Default(zero) and stays non-optional — both branches asserted side by side on the same message"
    requirement: "MIX-05"
    verification:
      - kind: unit
        ref: "mixinforproto/fieldmap_test.go#TestGolden/Presence"
        status: pass
    human_judgment: false
  - id: D5
    description: "Scalar-valued maps derive one JSON field each; a message-valued map derives no field at all"
    requirement: "MIX-06"
    verification:
      - kind: unit
        ref: "mixinforproto/fieldmap_test.go#TestGolden/Maps"
        status: pass
    human_judgment: false
  - id: D6
    description: "Message-typed fields are skipped by default; AsJSON(name) opts one in as a JSON field; AsJSON(\"\"), an unknown name, or a non-message-typed name each fail at schema load naming the message, the field, and \"AsJSON\""
    requirement: "MIX-09"
    verification:
      - kind: unit
        ref: "mixinforproto/fieldmap_test.go#TestGolden/Messages"
        status: pass
      - kind: unit
        ref: "mixinforproto/fieldmap_test.go#TestAsJSON"
        status: pass
    human_judgment: false
  - id: D7
    description: "Derived field order is stable and byte-identical across repeated runs (go test -count=5 -race), and the Oneofs golden proves the synthetic one-member oneof around an optional scalar is never conflated with a real oneof's members"
    requirement: "MIX-13"
    verification:
      - kind: unit
        ref: "mixinforproto/fieldmap_test.go#TestGolden/Oneofs"
        status: pass
      - kind: other
        ref: "GOWORK=off go test -race -count=5 ./... (mixinforproto/)"
        status: pass
    human_judgment: false

duration: 16min
completed: 2026-08-08
status: complete
---

# Phase 1 Plan 2: Complete Field-Mapping Table Summary

**Full Phase 1 mapping table implemented — all 15 proto scalar kinds, enums, three well-known types, proto3 presence semantics, scalar maps, and opt-in `AsJSON` for message fields — golden-asserted against a seven-file synthetic corpus, stable under `go test -race -count=5`.**

## Performance

- **Duration:** 16 min
- **Started:** 2026-08-08T10:23:00Z (approx, first proto file write)
- **Completed:** 2026-08-08T10:38:00Z
- **Tasks:** 2
- **Files modified:** 32 (25 new, 7 modified)

## Accomplishments

- `fieldmap.go`'s `classify(fd)` implements the exact RESEARCH.md-verified branch order (`IsMap()` → real-oneof membership via `ContainingOneof() != nil && !IsSynthetic()` → `HasOptionalKeyword()` → `MessageKind` → `EnumKind` → plain scalar), the single seam that keeps MIX-05 (`optional` scalar) and MIX-10 (real oneof member) from being conflated.
- Every proto scalar kind (15 of them, including the `sint`/`fixed`/`sfixed` width-matched variants) maps to its same-width ent builder with no widening, verified field-by-field via `TestScalarsCoverAllKinds`.
- Well-known types: `Timestamp` → `field.Time`; `Struct` → `field.JSON(map[string]any{})`; `Value` → `field.JSON(json.RawMessage(nil))` (chosen for lossless round-tripping — closes RESEARCH.md Open Question 2); `FieldMask`/`Duration` deliberately skipped, proven by exact-count and by-name-absence assertions.
- Proto3 presence: `optional` scalars derive `.Nillable().Optional()` with no default; plain scalars derive `.Default(<Go zero>)` and stay non-optional — both branches asserted side by side on `Presence`.
- Scalar-valued maps derive one JSON field each (Go map type built generically via `reflect.MapOf`, not hardcoded per shape); message-valued maps derive no field.
- `AsJSON(name string) Option` added to `option.go`; message fields are skipped by default and only materialize as a JSON field when named. `validateAsJSON` rejects an empty name, an unknown field name, and a non-message-typed name, each error naming the message, the field, and `"AsJSON"`.
- `mixinforproto/internal/fieldproj` — the R4 golden-comparable projection (`Project`/`ProjectAll`), which never marshals the raw `*field.Descriptor` (its `Validators []any` closures and `Err` field are not JSON-safe) and is decoupled from `mixinforproto` via a JSON round-trip to avoid an import cycle with the test files that import it.
- Seven corpus `.proto` files plus committed generated stubs, and eight golden fixtures under `mixinforproto/testdata/`, all stable under `go test -race -count=5` and idempotent under `-update` (verified via a before/after SHA-256 diff of every `.golden` file).

## Task Commits

1. **Task 1: Build the synthetic conformance corpus and the golden projection harness** - `cc78ffa` (feat)
2. **Task 2: Implement the complete Phase 1 mapping table with golden assertions** - `3081485` (feat)

**Plan metadata:** committed alongside this SUMMARY (see below).

## Files Created/Modified

- `proto/mixinforprototest/v1/{scalars,enums,wkt,presence,maps,messages,oneofs}.proto` - the seven mapping-rule corpus files
- `proto/buf.gen.yaml` - generalized the managed-mode `go_package` override from a single `tracer.proto`-scoped entry to a module-wide entry covering every corpus file
- `mixinforproto/internal/gen/mixinforprototestv1/{scalars,enums,wkt,presence,maps,messages,oneofs}.pb.go` - committed generated stubs (D-22); `tracer.pb.go` regenerated byte-identical (verified via `git diff`)
- `mixinforproto/internal/fieldproj/fieldproj.go` / `fieldproj_test.go` - the R4 golden-comparable projection struct, `Project`/`ProjectAll`, and its regression-guard tests
- `mixinforproto/fieldmap.go` - `classify`, `mapScalar`, `mapOptionalScalar`, `mapEnum`, `mapScalarMap`, `mapWellKnownType`, `mapAsJSON`, `mapMessageField`, `validateAsJSON`, `sourceFieldFor`
- `mixinforproto/option.go` - `AsJSON(name string) Option`, `asJSONNames()`
- `mixinforproto/derive.go` - `mapField` call now threads `o` through; one post-loop `validateAsJSON(msgName, md, o)` call added
- `mixinforproto/derive_test.go` - three Plan 01 tests superseded (see Deviations)
- `mixinforproto/fieldmap_test.go` - `TestGolden` (8 subtests), `TestScalarsCoverAllKinds`, `TestAsJSON`
- `mixinforproto/testdata/{scalars,enums,wkt,presence,maps,messages,oneofs,empty}.golden` - golden fixtures
- `mixinforproto/go.mod` / `go.sum` - added `github.com/sebdah/goldie/v2 v2.8.0` (direct, test-only); `go 1.24.0`/`toolchain go1.24.7`/`rogpeppe/go-internal v1.14.0` pins from Plan 01 preserved through `go mod tidy` (verified, not assumed)

## Decisions Made

- **`google.protobuf.Value` → `field.JSON(json.RawMessage(nil))`**, closing RESEARCH.md Open Question 2: chosen over a concrete struct type because it round-trips any JSON value losslessly.
- **`google.protobuf.Struct` → `field.JSON(map[string]any{})`**, matching its own natural JSON shape.
- **Scalar map Go types built generically via `reflect.MapOf`** rather than a hardcoded switch per observed map shape, so the mapping covers any scalar key/value combination, not just the two the corpus happens to test.
- **`SourceField.Kind` now carries the derivation-class name** (`"scalar"`, `"optionalScalar"`, `"enum"`, `"wkt"`, `"scalarMap"`, `"asJSON"`) rather than the raw `protoreflect.Kind` string Plan 01's walking skeleton used — this matches D-02's documented annotation contract ("derivation kind (scalar / enum / WKT / map-as-JSON / message-as-JSON / override)") more precisely than the string Plan 01 shipped for its one-field slice.
- **`buf.gen.yaml`'s override generalized to module-wide** (no `path:` restriction) rather than adding seven more per-file entries — verified empirically against the real `buf` v1.72.0 binary that `tracer.pb.go` regenerates byte-identical under the new, simpler override.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] `mapField`'s signature gained an `o *options` parameter, requiring a minimal `derive.go` change the plan's `files_modified` list did not name**
- **Found during:** Task 2 (wiring `AsJSON`)
- **Issue:** `AsJSON`'s opt-in check (`o.isAsJSON(name)`) and its "unknown field name" / "empty name" validation both need the effective `*options` for the current derivation, but Plan 01's `mapField(msgName, fd)` signature had no way to receive it, and the plan's own `files_modified` list for this task omits `derive.go`.
- **Fix:** Changed `mapField`'s signature to `mapField(msgName string, fd protoreflect.FieldDescriptor, o *options)` and updated derive.go's single call site to pass `o` (already in scope there). Also added one post-loop call, `errs = append(errs, validateAsJSON(msgName, md, o)...)`, immediately before the existing `if len(errs) > 0` check — this is where "AsJSON names a field that does not exist" can be detected at all, since that requires the message's full field set (`md`), not just the current field being walked. Both changes are mechanical (threading an already-in-scope variable, one new call), preserve every existing test and the loop's structure/ordering, and were necessary to satisfy MIX-09's three-case failure surface (empty name, unknown name, non-message field) — none of these can be validated without full-message context that only exists after the per-field walk completes.
- **Files modified:** `mixinforproto/derive.go`
- **Verification:** `TestAsJSON`'s three subtests all pass; all of Plan 01's `derive_test.go` tests continue to pass unchanged.
- **Committed in:** `3081485` (Task 2 commit)

**2. [Rule 1 - Bug] Three Plan 01 tests asserting "Unsupported" fails were now asserting a false negative, since Plan 02 closed the gap they tested**
- **Found during:** Task 2 (running the full suite after implementing `mapScalar`)
- **Issue:** Plan 01's `Unsupported` corpus message (one `int32` field) existed specifically to prove an un-mapped scalar kind fails loudly at derivation time. Once `mapScalar`'s exhaustive kind switch covers `Int32Kind` (this plan's whole point), `Unsupported.count` derives successfully — `TestDerive_UnsupportedKindFailsLoudly`, `TestMixinForProto_PanicsOnUnsupportedKind`, and `TestValidate_MirrorsDeriveWithoutSubprocess`'s failure-path assertion all failed because they asserted the pre-Plan-02 behavior. No proto3-expressible scalar kind remains unmapped after this plan (only `GroupKind`, a proto2-only construct with no proto3 keyword, could still trigger the `default:` branch), so a still-"unsupported" fixture cannot be constructed from valid proto3 syntax.
- **Fix:** Renamed and repurposed the two derive/mixin-adapter tests to `TestDerive_FormerlyUnsupportedKindNowMaps` / `TestMixinForProto_FormerlyUnsupportedKindNowMaps`, asserting `Unsupported.count` now derives one `int32` field. Replaced `TestValidate_MirrorsDeriveWithoutSubprocess`'s failure-path fixture with `AsJSON("does_not_exist")` against `Messages` (Plan 02's own failure surface), preserving the test's original purpose (proving `Validate[M]` mirrors `derive`'s failure verdict without the `entc.LoadGraph` subprocess) without relying on retired behavior.
- **Files modified:** `mixinforproto/derive_test.go`
- **Verification:** `GOWORK=off go test -race -count=5 ./...` passes; `go vet ./...` clean.
- **Committed in:** `3081485` (Task 2 commit)

**3. [Rule 1 - Bug] `fieldproj.stableDefault`'s original `fmt.Sprint`-based rendering made a zero-value string default indistinguishable from "no default set"**
- **Found during:** Task 2 (writing the `Presence` golden subtest, which the plan's own acceptance criteria require to show "the plain fields with both false and a non-empty Default")
- **Issue:** `fmt.Sprint("")` and `fmt.Sprint(nil)`-guarded-to-`""` both render as the empty string, so `plain_string`'s `Default("")` (a real, set default — the Go zero value for `string`) was indistinguishable from `optional_string`'s absent default, failing the plan's own "non-empty Default" acceptance check for the plain-scalar branch.
- **Fix:** Changed `stableDefault` to render non-nil values via the `"%#v"` Go-syntax verb instead of `fmt.Sprint`, so `Default("")` renders as the 2-character string `""` (visibly present) while an actually-nil `Default` still renders as the empty string (the explicit `nil` guard is unchanged). This preserves R4's core guarantee (never marshal the raw descriptor or its closures) while making "a default is present" always detectable from the rendered string.
- **Files modified:** `mixinforproto/internal/fieldproj/fieldproj.go`
- **Verification:** `TestGolden/Presence` passes; `fieldproj`'s own `TestProject_NilDefaultRendersStableEmptyString` (unaffected — still checks the actual-nil path) continues to pass.
- **Committed in:** `3081485` (Task 2 commit)

**4. [Rule 2 - Missing Critical] The `Empty` fixture this plan's acceptance criteria require was not re-declared — the identically-named message already exists from Plan 01**
- **Found during:** Task 1 (writing the corpus proto files)
- **Issue:** The plan's action text says to "add an `Empty` message with zero fields (to `scalars.proto`)". `Empty` already exists as `message Empty {}` in `tracer.proto`, in the same proto package (`mixinforprototest.v1`) — a second declaration would be a duplicate-symbol error at `buf generate` time (proto packages share one namespace across files).
- **Fix:** Did not re-declare `Empty`. The `TestGolden/Empty` subtest reuses `mixinforprototestv1.Empty` (Plan 01's generated type) directly, producing an `empty.golden` fixture that satisfies the plan's acceptance criterion ("A golden fixture exists under `mixinforproto/testdata/` for each of: ... Empty") without a proto-level collision.
- **Files modified:** none (test-only decision, documented in `fieldmap_test.go`'s `Empty` subtest comment)
- **Verification:** `buf lint`/`buf generate` both exit 0 against the full `proto/` tree; `TestGolden/Empty` passes.
- **Committed in:** `cc78ffa` (Task 1, corpus decision) / `3081485` (Task 2, test using it)

---

**Total deviations:** 4 auto-fixed (1 blocking signature change, 2 bug fixes to stale/ambiguous test assertions, 1 missing-functionality/collision avoidance)
**Impact on plan:** All four were necessary to make the plan's own acceptance criteria achievable or to keep the test suite honest after this plan's changes made prior assertions false. No scope creep — no Tier 1/2 translation logic, no Exclude/Override implementation (Plan 03), no oneof-resolution panic gate (also Plan 03) were added.

## Issues Encountered

- **`buf`/`protoc-gen-go` were installed under `$(go env GOPATH)/bin` but not on `PATH`** in this session, same as Plan 01 flagged. Resolved by exporting `PATH="$PATH:$(go env GOPATH)/bin"` for the `buf lint`/`buf generate` invocations; standalone `go build`/`go test` under `GOWORK=off` were verified to still succeed with `buf` genuinely absent from `PATH` (D-22's property), not just untested.
- **`go mod tidy` promoted `goldie` from indirect to direct** (expected — it's a real, used test dependency) and pulled in its two transitive diff-library dependencies (`pmezard/go-difflib`, `sergi/go-diff`); the `go 1.24.0`/`toolchain go1.24.7`/`github.com/rogpeppe/go-internal v1.14.0` pins Plan 01 fought for were verified unchanged by `tidy` (diffed `go.mod` before/after) — no repeat of Plan 01's deviation 3.

## User Setup Required

None - no external service configuration required. Same `buf`/`protoc-gen-go` install note as Plan 01 applies for regenerating stubs; not required to build or test `mixinforproto` (stubs are committed).

## Next Phase Readiness

- `classify(fd)` and `mapField` are the seam Plan 03 extends: real-oneof members already classify correctly (`classRealOneofMember`) and produce no field/no error in this plan by design — Plan 03 adds the panic-unless-all-resolved gate over a oneof's members, plus `Exclude`/`Override`'s exported constructors (the `options` struct's `isExcluded`/`isOverridden` lookups already exist and are already wired into `derive.go`'s loop from Plan 01).
- Plan 04 (Tier 1 constraint translation) extends the `protovalidate.ResolveFieldRules` call already present in `mapScalar`'s `StringKind` case to actually populate `TranslatedIDs`/`ResidualIDs` instead of leaving them empty; the same pattern should extend to numeric kinds' `Min`/`Max`/`Range` translation.
- **Carry-forward flag:** `SourceField.Kind`'s value set changed in this plan (derivation-class strings, not raw `protoreflect.Kind` strings) — any downstream consumer decoding `SourceField.Kind` (Phase 2's entc extension, Phase 5's drift check) should read the class strings this plan establishes (`"scalar"`, `"optionalScalar"`, `"enum"`, `"wkt"`, `"scalarMap"`, `"asJSON"`), not Plan 01's single `fd.Kind().String()` value (`"string"`).
- **Carry-forward flag (inherited from Plan 01, still open):** re-verify `github.com/cel-expr/cel-go`'s module-path migration status before Phase 3 planning; this plan added no `cel-go` dependency of any kind.

---
*Phase: 01-mixinforproto-core*
*Completed: 2026-08-08*

## Self-Check: PASSED

All 28 files listed above verified present on disk; all three commits (`cc78ffa`, `3081485`, `e96792b`) verified present in `git log --oneline --all`.
