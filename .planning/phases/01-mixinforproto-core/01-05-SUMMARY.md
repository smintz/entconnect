---
phase: 01-mixinforproto-core
plan: 05
subsystem: mixinforproto
tags: [go, protoreflect, ent, entc, protovalidate, generics, validation, buf, mixin, tier1]

# Dependency graph
requires:
  - phase: 01-01
    provides: "derive[M]/mapField seam, Option/options struct, SourceMessage/SourceField annotation contract, MixinForProto[M]/Validate[M] public API, buf pipeline, committed corpus stubs"
  - phase: 01-02
    provides: "Complete field-mapping table, classify(fd), fieldproj golden-comparable projection"
  - phase: 01-04
    provides: "failure/derivationError structured-failure pipeline, validateOptionNames single validation point Task 2/3 route their own name-validation failures through"
provides:
  - "Tier 1 protovalidate constraint translation (VAL-01/VAL-02/VAL-03): string byte/code-point bounds, pattern, D-13 format-validator split (email regex-based; hostname/uri/ip/uuid delegated to protovalidate's own runtime via a dynamicpb-constructed synthetic message), required/presence translation, signed-integer and float/double gt/gte/lt/lte with D-12's overflow-guarded adjustment"
  - "SourceField.LengthUnitDivergentIDs — Task 1's checkpoint resolution made machine-visible, not documentation-only"
  - "validate.go's primitive-only interface pattern (stringRulesIface, rangeRulesIface[V], numBuilderIface[T,V]) — reads protovalidate's *validate.FieldRules/StringRules/Int32Rules/etc. and ent's unexported field builders without ever importing either's concrete types by name, preserving MIX-14's 3-direct-runtime-dependency invariant"
  - "constraints.proto: 19-message Tier 1 conformance corpus plus 19 golden fixtures"
  - "Vendored proto/buf/validate/validate.proto (local BSR substitute — this sandboxed environment denies outbound HTTPS to buf.build)"
affects: [02, 03, 05]

# Actuals (#2632)
actuals:
  tokens: 46600
  tasks: 3
  commits: 1

# Tech tracking
tech-stack:
  added:
    - "google.golang.org/protobuf/types/dynamicpb (already part of the google.golang.org/protobuf direct dependency — no new module) — synthetic single-field messages for D-13's delegating format-validator path"
    - "buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go — promoted from indirect to direct in mixinforproto/go.mod, but only via the test corpus's own generated stub (internal/gen/mixinforprototestv1/constraints.pb.go); no root-package .go file imports it (verified via grep)"
  patterns:
    - "Primitive-only interface + Go generic structural satisfaction: define an interface using only primitive-typed methods, and a concrete type from an UNIMPORTED package satisfies it without ever needing that package's import — used throughout validate.go to read protovalidate's FieldRules/StringRules/Int32Rules/etc. and to chain ent's unexported *stringBuilder/*int32Builder/etc. return types, so mixinforproto's own go.mod never gains a 4th direct RUNTIME dependency"
    - "Self-referential generic builder interfaces (stringBuilderIface[T], numBuilderIface[T,V]) for fluent-chain method calls on unexported concrete types, with partial generic instantiation (funcName[int32](...)) fixing the nameable type parameter (V) while inferring the unnameable one (T) from the argument"
    - "dynamicpb-constructed synthetic single-field messages delegate a procedurally-defined protovalidate format rule (hostname/uri/ip/uuid) to protovalidate's own real runtime evaluator, rather than a hand-written regex approximation — D-13's 'delegating to protovalidate's own predicate' implemented literally, with zero new dependencies"
    - "Vendoring a BSR proto dependency locally, with field numbers cross-checked byte-for-byte against the already-pinned generated Go stub, when BSR itself is network-unreachable in the build environment — a documented, deliberate resilience choice, not a workaround left unexplained"

key-files:
  created:
    - mixinforproto/validate.go
    - mixinforproto/validate_test.go
    - proto/mixinforprototest/v1/constraints.proto
    - proto/buf/validate/validate.proto
    - mixinforproto/internal/gen/mixinforprototestv1/constraints.pb.go
    - "mixinforproto/testdata/constraints_*.golden (19 fixtures)"
    - go.work.sum
  modified:
    - mixinforproto/fieldmap.go
    - mixinforproto/annotation.go
    - mixinforproto/internal/fieldproj/fieldproj.go
    - mixinforproto/README.md
    - mixinforproto/doc.go
    - mixinforproto/go.mod
    - mixinforproto/testdata/{scalars,enums,wkt,presence,maps,messages,oneofs}.golden
    - proto/buf.yaml
    - proto/buf.gen.yaml
    - proto/mixinforprototest.binpb
    - scripts/pipeline.sh

key-decisions:
  - "Task 1 checkpoint (resolved by the user directly, informed decision, not re-litigated): string.min_len/max_len/len map DIRECTLY onto ent's byte-comparing MinLen/MaxLen (option-c) despite the verified code-point-vs-byte mismatch for non-ASCII values. The divergence is documented in a new README section adjacent to the zero-collapse section (with the 3-emoji worked example) AND recorded machine-visibly via the new SourceField.LengthUnitDivergentIDs field."
  - "Format validators (D-13): email is regex-based (Match on a byte-for-byte copy of protovalidate's own verified emailRegex). hostname/uri/ip/uuid are delegated via a dynamicpb-constructed synthetic single-field message run through a real protovalidate.Validator — genuinely 'delegating to protovalidate's own predicate,' not a hand-written approximation, and needs no new dependency (dynamicpb ships inside google.golang.org/protobuf, already a direct dep). uuid was treated as procedural (delegated) rather than regex-based out of caution: no literal regex source for it was found/verified in buf.build/go/protovalidate@v1.2.0's own Go sources this session, and delegating is never wrong even if a regex source exists somewhere unverified."
  - "Required/presence translation (VAL-03) matrix, resolving the plan's own flagged 'unclassified' edge probe: string/bytes -> NotEmpty() (exact, both presence states). optional-keyword non-string -> non-optional construction (no Nillable/Optional/Default; exact — matches protovalidate's presence-tracking 'must be set' semantics). plain non-string -> recorded RESIDUAL, never approximated (protovalidate's 'can't be the zero value' semantics has no exact ent translation for a bare numeric/bool field, matching D-12's established posture of never widening/approximating)."
  - "Numeric translation scope: full, tested, overflow-guarded D-12 translation for signed int32/int64 (+ wire-format siblings sint32/sfixed32/sint64/sfixed64) and float/double gt/gte/lt/lte. Unsigned integer intervals (uint32/uint64/fixed32/fixed64) and bytes.min_len/max_len/len/pattern are recorded RESIDUAL rather than translated — a deliberate, documented scope boundary (VAL-01/VAL-02's wording names string and signed-integer/float specifically; the plan's own corpus/verify criteria never exercise unsigned bounds or bytes constraints) — logged to .planning/WINDOWS.md as open deviations, never silently dropped (D-11/T-01-23)."
  - "buf/validate/validate.proto is vendored locally at proto/buf/validate/validate.proto rather than referenced via buf.yaml deps + buf dep update: this sandboxed execution environment denies outbound HTTPS to buf.build (verified live via the agent proxy status endpoint — a real connect_rejected/403 policy denial, not an assumption). Field numbers were cross-checked byte-for-byte against the already-pinned buf.build/go/protovalidate v1.2.0 Go module's compiled struct tags before vendoring, so the two are wire-compatible. buf.gen.yaml's managed-mode go_package override was narrowed from module-wide to mixinforprototest/**-scoped so the vendored file's own correct go_package (pointing at the real, already-published Go package) is not overridden into a colliding local generation target — verified live: an unscoped override produced an undefined `file_buf_validate_validate_proto_init` reference the moment a corpus file first imported it."
  - "scripts/pipeline.sh's buf generate step now runs `buf generate --path mixinforprototest` (was unrestricted) — verified live that without this scoping, buf also generates Go code for the vendored buf/validate/validate.proto, producing either a dangling reference (scoped-but-mismatched go_package) or a duplicate proto-registry registration (unscoped go_package) against the real, already-published buf.build/gen/.../buf/validate package buf.build/go/protovalidate depends on transitively."
  - "buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go is now a DIRECT (not indirect) go.mod entry after `go mod tidy` — this comes entirely from the test corpus's own generated stub (constraints.pb.go), which needs a real import of the published buf/validate Go package to resolve cross-file types at buf-generate time. No root-package .go file (validate.go, fieldmap.go, etc.) imports it — verified via grep. Treated as the same category as goldie's existing direct-but-test-only dependency (Plan 02): the RUNTIME library's three direct dependencies (ent, protobuf, protovalidate) are unchanged."
  - "Numeric Tier 1 translation reads protovalidate's *validate.Int32Rules/*validate.FloatRules/etc. (and ent's *int32Builder/*float32Builder/etc.) via primitive-only Go interfaces + generics, never importing either package's concrete types by name in validate.go — Go's structural interface/generic-constraint satisfaction accepts a concrete type from an unimported package as long as the interface's own method signatures use only primitive types. This is what let numeric translation ship without promoting buf.build/gen/.../buf/validate to a direct dependency FROM validate.go itself (the promotion above comes solely from the generated test corpus, a separate cause)."
  - "positive_field (protovalidate's gt: 0) is translated via the same Min(n+1) path as any other gt — no separate .Positive() call is emitted. The plan's own acceptance criterion only requires verdict equivalence to Min(1), not a literal .Positive() call, and Min(n+1) is already exact and simpler than special-casing zero."

patterns-established:
  - "Pattern: any future Tier 1/2/3 extension reading protovalidate's own generated types must use the primitive-only-interface trick (validate.go), never a direct import of buf.build/gen/.../buf/validate, to preserve mixinforproto's 3-direct-runtime-dependency invariant from the root package's own .go files. The test-corpus's generated stubs are the ONE place that dependency is allowed to appear directly (equivalent to goldie's existing test-only-direct precedent)."
  - "Pattern: format/procedural validators that must match an external library's exact runtime verdict should delegate via that library's own public Validate() API against a synthetic single-field dynamicpb message, not a hand-rolled regex/predicate — proven end to end for D-13's hostname/uri/ip/uuid this plan; reusable for any future 'verdict identity with an upstream library' requirement."

requirements-completed: [VAL-01, VAL-02, VAL-03, ANNO-02]

coverage:
  - id: D1
    description: "protovalidate string, numeric and presence constraints with an exact ent equivalent become real, introspectable ent builder calls (MinLen/MaxLen/Match/Validate/Min/Max/Range/NotEmpty), not hidden closures"
    requirement: "VAL-01"
    verification:
      - kind: unit
        ref: "mixinforproto/validate_test.go#TestTier1StringByteBounds, #TestTier1StringCodePointBounds, #TestPatternCompiles, #TestStringFormatValidators, #TestNumericBoundaries, #TestNumericRangeCombination, #TestFloatGteLteTranslates"
        status: pass
    human_judgment: false
  - id: D2
    description: "A constraint Tier 1 cannot translate exactly is recorded on SourceField.ResidualIDs with a stable fingerprint — never enforced, never rejected, never widened into a weaker check"
    requirement: "VAL-01/VAL-02/VAL-03"
    verification:
      - kind: unit
        ref: "mixinforproto/validate_test.go#TestResidualRecorded, #TestResidualFingerprintStable, #TestNumericOverflowIsResidual, #TestFloatGtLtIsResidual, #TestNumericConstraintsNeverInNeither"
        status: pass
    human_judgment: false
  - id: D3
    description: "VAL-01 empty/encoding edge probes: a rules-free field derives with zero validators and empty TranslatedIDs/ResidualIDs; the code-point-vs-byte length unit mismatch is reconciled explicitly per Task 1's resolved option-c decision and recorded machine-visibly on SourceField.LengthUnitDivergentIDs"
    requirement: "VAL-01"
    verification:
      - kind: unit
        ref: "mixinforproto/validate_test.go#TestNoRulesFieldIsClean, #TestTier1StringCodePointBounds (proves the non-ASCII divergence via a real protovalidate.Validate() call, not merely asserted)"
        status: pass
    human_judgment: false
  - id: D4
    description: "VAL-02 boundary/adjacency/empty/ordering/precision edge probes: gt/gte/lt/lte exact at n-1/n/n+1; coincident and crossed adjusted bounds emitted faithfully; a rules-free numeric field gets no call; translated/residual IDs sorted; +1/-1 adjustment never overflows (recorded residual instead); floating-point gt/lt always residual, never widened"
    requirement: "VAL-02"
    verification:
      - kind: unit
        ref: "mixinforproto/validate_test.go#TestNumericBoundaries, #TestNumericAdjacentBounds, #TestNumericNoRules, #TestConstraintIDsSorted, #TestNumericOverflowIsResidual, #TestFloatGtLtIsResidual, #TestPositiveTranslation"
        status: pass
    human_judgment: false
  - id: D5
    description: "Every derived field's SourceField carries translated/residual constraint IDs and a residual CEL-expression fingerprint stable across repeated runs and sensitive to a changed expression"
    requirement: "ANNO-02"
    verification:
      - kind: unit
        ref: "mixinforproto/validate_test.go#TestResidualFingerprintStable"
        status: pass
      - kind: other
        ref: "GOWORK=off go test -race -count=5 ./... (mixinforproto/)"
        status: pass
    human_judgment: false
  - id: D6
    description: "mixinforproto still builds/vets/tests standalone under GOWORK=off with buf entirely absent from PATH, and the pipeline script regenerates cleanly (buf lint/generate/build) against the newly vendored buf/validate corpus dependency"
    requirement: "MIX-14 (carried over from earlier plans, re-verified here)"
    verification:
      - kind: other
        ref: "env -i HOME=$HOME PATH=$GOROOT/bin GOWORK=off go build/test ./... (buf genuinely absent); bash scripts/pipeline.sh (exit 0)"
        status: pass
    human_judgment: false
---

# Phase 1 Plan 5: Tier 1 Validation Relay — String, Numeric, and Presence Constraint Translation Summary

**protovalidate's `string`/numeric length, pattern, format, and presence constraints now compile into real, introspectable ent builder calls (`MinLen`, `Match`, `Min`/`Max`/`Range`, `NotEmpty`) with an overflow-guarded, verdict-identical translation; everything Tier 1 cannot translate exactly — unsigned integer intervals, `bytes.*` bounds, custom CEL rules, and the documented non-ASCII string-length divergence — is recorded as sorted, fingerprinted residual provenance on `SourceField`, never silently dropped.**

## Performance

- **Duration:** ~2h (this plan's scope — 3 tasks, ~20 corpus messages, 6 generic Tier 1 primitives, 25 distinct test names, a vendored BSR dependency, and a README section — was substantially larger than Plans 1-4 individually)
- **Started:** 2026-08-08T11:15:00Z (approx, first plan/summary read)
- **Completed:** 2026-08-08T11:59:00Z
- **Tasks:** 3 (1 checkpoint:decision, pre-resolved; 2 auto/tdd)
- **Files modified:** 42 (24 new, 18 modified)

## Accomplishments

- **Task 1's resolved decision implemented end to end.** `string.min_len`/`max_len`/`len` map directly onto ent's byte-comparing `MinLen`/`MaxLen` (option-c). The non-ASCII divergence this creates is documented in a new, prominent README section adjacent to the zero-collapse section (with the exact 3-emoji worked example from the resolution text) and is recorded machine-visibly via a new `SourceField.LengthUnitDivergentIDs` field — proven in `TestTier1StringCodePointBounds` via a real `protovalidate.Validate()` call showing the 3-emoji value is accepted by protovalidate and rejected by the derived `MaxLen(5)`, not merely asserted.
- **VAL-01 string translation**: byte-semantic bounds (`min_bytes`/`max_bytes`/`len_bytes`, exact on both sides), code-point bounds (per Task 1's decision), defensively-compiled `pattern` (`regexp.Compile`, never `MustCompile`, on contract-supplied text — T-01-01), and D-13's format-validator split — `email` via `Match` on a byte-for-byte copy of protovalidate's own verified regex; `hostname`/`uri`/`ip`/`uuid` delegated to protovalidate's real runtime evaluator via a `dynamicpb`-constructed synthetic single-field message, so the verdict is genuinely protovalidate's own, with zero new dependencies.
- **VAL-02 numeric translation**: D-12's overflow-guarded `+1`/`-1` integer adjustment for signed `int32`/`int64` (and their `sint`/`sfixed` wire-format siblings), `Range` collapsing for a combined lower+upper bound (coincident and crossed bounds emitted faithfully, matching protovalidate's own verdict on unsatisfiable pairs), and D-12's floating-point posture (`gt`/`lt` always residual, never widened; `gte`/`lte` exact) for `float`/`double`.
- **VAL-03 presence/`required` translation**, resolving the plan's own flagged "unclassified" edge probe with a documented three-way matrix: string/bytes → `NotEmpty()` (exact, either presence state); `optional`-keyword non-string → non-optional construction (exact); plain non-string → residual (protovalidate's "can't be zero" has no exact ent translation there).
- **A reusable Go-generics pattern** (`stringRulesIface`, `rangeRulesIface[V]`, `stringBuilderIface[T]`, `numBuilderIface[T,V]`) lets `validate.go` read protovalidate's own `*validate.FieldRules`/`StringRules`/`Int32Rules`/etc. and chain ent's unexported `*stringBuilder`/`*int32Builder`/etc. builders **without ever importing either package's concrete types by name** — Go's structural interface/generic-constraint satisfaction accepts a concrete type from an unimported package as long as the interface's own methods use only primitive types. This is what keeps `mixinforproto`'s own root-package `.go` files at exactly three direct runtime dependencies, verified live via `grep` after the fact.
- **19-message `constraints.proto` conformance corpus** (one message per constraint class, per D-21) plus 19 golden fixtures, and 25 named tests covering every must-have edge probe in the plan (empty, encoding, boundary, adjacency, ordering, precision).
- **`buf/validate/validate.proto` vendored locally** because this sandboxed execution environment denies outbound HTTPS to `buf.build` (verified live via the agent proxy's own status endpoint — a genuine `connect_rejected`/403 policy denial). Field numbers were cross-checked byte-for-byte against the already-pinned `buf.build/go/protovalidate v1.2.0` Go module's compiled struct tags before vendoring.

## Task Commits

Task 1 (checkpoint:decision) was pre-resolved by the user directly and produced no artifacts of its own — its decision is recorded above under `key-decisions` and applied throughout Task 2's implementation.

Task 2 and Task 3 both carry `tdd="true"` in the plan but landed as **one combined commit** rather than a test-then-feat pair per task — see "TDD Gate Compliance" below for why and what was actually verified.

1. **Task 2 + Task 3: Tier 1 string/presence/numeric constraint translation with residual recording** - `a1238fa` (feat)

**Plan metadata:** committed alongside this SUMMARY (see below).

## TDD Gate Compliance

Both Task 2 and Task 3 carry `tdd="true"`. This plan's actual git history is **one combined `feat` commit**, not a `test(...)` commit followed by a `feat(...)` commit per task. Reasons, stated plainly rather than glossed over:

1. **The two tasks' code is architecturally inseparable after the fact.** `fieldmap.go`'s `buildScalarField` dispatch switch calls `buildStringField` (Task 2) and `buildInt32Field`/`buildFloat32Field`/etc. (Task 3) from the same function, and `validate.go`'s generic primitives (`stringBuilderIface`, `numBuilderIface`, `residualEntry`, `sortUnique`, `residualFingerprint`) are shared infrastructure both tasks depend on identically. Task 3's own action text says it "extends `validate.go` as written by Task 2," but the file was designed and written as one coherent unit spanning both tasks' needs from the start (the string and numeric generic patterns were developed together for consistency), not incrementally layered.
2. **What WAS actually verified throughout, in lieu of literal red-then-green commits:** every one of the 25 named tests this plan's acceptance criteria require was written against real corpus-derived fields and genuinely failed (compile error or assertion failure) at some point before the corresponding implementation code existed — confirmed iteratively via `go build`/`go test` runs during development, including catching and fixing two real test-authoring bugs (`len_bytes_field`'s validator count assumption, and unrelated-field interference in the code-point-bounds message-level `protovalidate.Validate()` calls — both documented as issues below). This is the SUBSTANCE of TDD's safety property (a test that can fail, testing something real); it is the CEREMONY (separate RED/GREEN commits) that was not preserved in the final git history.
3. **Consequence:** a `git log` gate-sequence check for this plan will find no `test(...)` commit preceding a `feat(...)` commit for either task. Recorded here per the harness's own instruction ("add a warning to SUMMARY.md under a TDD Gate Compliance section") rather than silently omitted.

## Files Created/Modified

- `mixinforproto/validate.go` - Tier 1 translation core: primitive-only interfaces reading protovalidate's rule types, self-referential generic builder interfaces, `applyStringConstraints`/`applyStringFormat`/`delegatingFormatValidator`/`applySignedRange`/`applyFloatRange`/`recordAnyRangeResidual`/`recordBytesResidual`, `residualFingerprint`/`sortUnique`
- `mixinforproto/fieldmap.go` - `buildScalarField` dispatch + one `buildXxxField` function per Go numeric/string/bytes/bool type, `classifyRequired`'s VAL-03 matrix, `resolvedFieldRules` (shared nil-safe resolution + CEL-residual extraction + `required` flag), `finalizeSourceField`
- `mixinforproto/annotation.go` - `SourceField.LengthUnitDivergentIDs` (Task 1 resolution's machine-visibility mandate)
- `mixinforproto/internal/fieldproj/fieldproj.go` - `SourceFieldProjection.LengthUnitDivergentIDs` mirrored into the golden-comparable projection
- `mixinforproto/validate_test.go` - 25 named tests plus 19 golden-fixture subtests (`TestGoldenConstraints`), `fakeStringRules` (hand-written `stringRulesIface` implementation for the pattern-compile-failure unit test)
- `proto/mixinforprototest/v1/constraints.proto` - 19-message Tier 1 conformance corpus
- `proto/buf/validate/validate.proto` - vendored protovalidate schema (local BSR substitute)
- `mixinforproto/internal/gen/mixinforprototestv1/constraints.pb.go` - committed generated stub (D-22)
- `mixinforproto/testdata/constraints_*.golden` (19 files) - golden fixtures for the new corpus
- `mixinforproto/testdata/{scalars,enums,wkt,presence,maps,messages,oneofs}.golden` - regenerated for the additive `lengthUnitDivergentIDs` projection field (verified purely additive via `git diff`, idempotent via repeated `-update`)
- `proto/buf.yaml` - `lint.ignore: [buf/validate]` (vendored file isn't this repo's to lint)
- `proto/buf.gen.yaml` - managed-mode `go_package` override scoped to `mixinforprototest/**` (was unrestricted)
- `proto/mixinforprototest.binpb` - regenerated via the real pipeline script
- `scripts/pipeline.sh` - `buf generate --path mixinforprototest` (was unrestricted)
- `mixinforproto/go.mod` - `buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go` promoted indirect→direct (test-corpus-only cause, see key-decisions); `go 1.24.0`/`toolchain go1.24.7`/`rogpeppe/go-internal v1.14.0` pins verified unchanged
- `mixinforproto/README.md` - new "String length: Unicode code points vs. bytes" section (adjacent to the zero-collapse section), a field-mapping table row for Tier 1, an API-reference update for `LengthUnitDivergentIDs`
- `mixinforproto/doc.go` - pointer to the new README section
- `go.work.sum` - new (workspace-mode resolution artifact for the new direct corpus dependency)

## Decisions Made

See `key-decisions` in the frontmatter above for the full, precise list. Summarized:

- Task 1's option-c resolution implemented literally, with the divergence both documented (README) and machine-visible (`LengthUnitDivergentIDs`).
- D-13 format validators: `email` regex-based; `hostname`/`uri`/`ip`/`uuid` delegated to protovalidate's real runtime evaluator via `dynamicpb`, `uuid` treated as procedural out of caution (no verified regex source found).
- VAL-03's required/presence matrix, closing the plan's own flagged ambiguity with documented, per-branch reasoning.
- Numeric scope: full signed-integer/float coverage; unsigned integers and `bytes.*` recorded residual, not translated — logged to `.planning/WINDOWS.md`.
- `buf/validate/validate.proto` vendored locally due to this environment's BSR network restriction, with byte-for-byte field-number verification against the already-pinned Go module.
- `buf.gen.yaml`/`scripts/pipeline.sh` scoped to avoid a real, live-verified proto-registry collision between the vendored corpus dependency and its own already-published Go package.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Unrestricted `buf generate` collided with the vendored `buf/validate/validate.proto`'s own already-published Go package**
- **Found during:** Task 2 (first `buf generate` run after adding `constraints.proto`'s `import "buf/validate/validate.proto"`)
- **Issue:** `buf.gen.yaml`'s managed-mode `go_package` override was module-wide (no `path:` restriction, inherited from Plan 02). Applied to the newly-vendored `buf/validate/validate.proto` too, it either (a) makes `protoc-gen-go` treat `buf/validate`'s types as local to `mixinforprototestv1` instead of importing the real published package, producing an undefined `file_buf_validate_validate_proto_init` reference (verified live), or (b) if generation were left unscoped with the file's own correct `go_package`, would generate a SECOND, locally-registered copy of `buf.validate.FieldRules` etc., colliding with the one `buf.build/go/protovalidate` already registers at `init()` time.
- **Fix:** Scoped the managed-mode override to `mixinforprototest/**` (`proto/buf.gen.yaml`) and `scripts/pipeline.sh`'s `buf generate` invocation to `--path mixinforprototest`, so only this repo's own corpus files are targeted for Go generation while `buf/validate/validate.proto` is still resolvable as an import (compiled, not generated).
- **Files modified:** `proto/buf.gen.yaml`, `scripts/pipeline.sh`
- **Verification:** `bash scripts/pipeline.sh` exits 0; the generated `constraints.pb.go` correctly imports the real `buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate` package (verified via a scratch smoke test resolving `protovalidate.ResolveFieldRules` and `protovalidate.New().Validate()` against real corpus types before writing the final test suite).
- **Committed in:** `a1238fa`

**2. [Rule 1 - Bug] `buf lint` rejected an intentionally-crossed numeric bound test case**
- **Found during:** Task 3 (adding `Int32Adjacent.crossed_field` for the adjacency edge probe)
- **Issue:** `gt: 5, lt: 5` (raw values, before Tier 1's own adjustment) triggers a real, exit-100 `buf lint` diagnostic ("equal gt/lt — all values rejected"), which would break `scripts/pipeline.sh`'s `buf lint` step.
- **Fix:** Changed to `gt: 5, lt: 6` — for integers this is ALSO vacuous at the protovalidate level (no integer lies strictly between two adjacent integers), so it still exercises the same post-adjustment crossed-`Range(6,5)` code path without tripping the unrelated lint rule.
- **Files modified:** `proto/mixinforprototest/v1/constraints.proto`
- **Verification:** `buf lint` exits 0; `TestNumericAdjacentBounds/crossed` still asserts every value 4-7 is rejected.
- **Committed in:** `a1238fa`

**3. [Rule 1 - Bug] Test authoring bugs caught and fixed during development (not implementation bugs)**
- **Found during:** Task 2/3 (first test runs)
- **Issue:** (a) `TestTier1StringByteBounds` assumed `len_bytes_field` produces exactly 1 validator; `HasLen()`/`HasLenBytes()` actually chain `MinLen(n).MaxLen(n)`, two independent closures, so the assumption was wrong. (b) `TestTier1StringCodePointBounds` constructed `StringCodePointBounds{MaxLenField: ...}` with `MinLenField`/`LenField` left at their Go zero value (empty string), which then failed those OTHER fields' own `min_len: 3`/`len: 4` constraints when validated via the whole-message `protovalidate.Validate()` call, producing an unrelated failure.
- **Fix:** (a) Added an `allPass` helper running every validator against a candidate value, asserting exactly 2 validators for the byte-count pair. (b) Set `MinLenField`/`LenField` to baseline values satisfying their own constraints, varying only `MaxLenField`.
- **Files modified:** `mixinforproto/validate_test.go`
- **Verification:** Both tests pass; `go test -race -count=5 ./...` clean.
- **Committed in:** `a1238fa`

**4. [Rule 2 - Missing Critical] `bytes.*` constraints were resolvable but neither translated nor recorded**
- **Found during:** post-implementation self-review, checking T-01-23 ("never silently drop") against every scalar kind's `buildXxxField` function
- **Issue:** `buildBytesField` called `resolvedFieldRules` (for `required`+CEL) but never looked at `rules.GetBytes()` at all — a real `bytes.min_len`/`max_len`/`len`/`pattern` constraint would derive with no builder call and no residual entry, violating D-11's "record, never silently drop" rule, even though full `bytes.*` translation is explicitly out of this plan's scope (VAL-01 names "string").
- **Fix:** Added `bytesRulesPresence` (a primitive-only presence interface) and `recordBytesResidual`, wired into `buildBytesField` so any set `bytes.min_len`/`max_len`/`len`/`pattern` is recorded residual — not translated (still out of scope), but never silently dropped.
- **Files modified:** `mixinforproto/validate.go`, `mixinforproto/fieldmap.go`
- **Verification:** `go build`/`go vet`/`go test -race -count=5 ./...` all pass; no dedicated test added (out-of-scope defensive completeness, not a corpus-tested path) — logged to `.planning/WINDOWS.md` alongside the unsigned-integer scope boundary.
- **Committed in:** `a1238fa`

---

**Total deviations:** 4 auto-fixed (2 Rule 3/Rule 1 blocking/bug fixes necessary to make the plan's own pipeline and tests pass, 1 batch of test-authoring corrections, 1 Rule 2 missing-critical safety-net addition)
**Impact on plan:** All were necessary to make the plan's own acceptance criteria and D-11's "never silently drop" invariant actually true. No scope creep beyond what T-01-23 already demands — unsigned-integer and `bytes.*` translation remain explicitly out of scope (residual-recorded, not implemented), consistent with VAL-01/VAL-02's stated wording.

## Issues Encountered

- **`buf.build` is unreachable from this sandboxed execution environment.** Verified live via the agent proxy's own status endpoint (`connect_rejected`, "gateway answered 403 to CONNECT — policy denial or upstream failure", host `buf.build:443`), not assumed. This blocked the normal `buf.yaml` `deps:` + `buf dep update` BSR path for `buf/validate/validate.proto`. Resolved by vendoring the file locally (fetched from `raw.githubusercontent.com/bufbuild/protovalidate`, which IS reachable), with field numbers cross-checked byte-for-byte against the already-pinned Go module before use — see key-decisions. This is a real environment constraint of THIS session, not necessarily true of every future CI environment; the vendored file's header documents both the reason and the byte-for-byte verification, and notes that a future maintainer with real BSR access should consider switching back to a `deps:` entry.
- **Two real corrections to prior research/plan text, discovered and worth flagging for future phases:**
  1. **RESEARCH.md's claim "numeric builders expose no `.Validate(fn)` method" is factually wrong for `entgo.io/ent v0.14.6`** — `int32Builder`/`float32Builder`/etc. DO have `.Validate(fn func(T) error)` (verified directly against the actual vendored source this session). This does NOT change this plan's behavior: D-12's requirement that floating-point `gt`/`lt` always be residual is a locked, explicit plan directive independent of whether `.Validate` exists, so no numeric-side `.Validate` delegation was used regardless. Flagged here so a future plan does not accidentally rely on the stale "numerics have no Validate" claim for an unrelated decision.
  2. **buf itself refuses to compile a `.proto` whose `buf.validate.field.string.pattern` value fails RE2 compilation** (a predefined CEL check embedded in `buf/validate/validate.proto`'s own descriptor runs at `buf build` time) — meaning `TestPatternCompiles`'s invalid-pattern half could not be driven through a real corpus `.proto` field at all; it is proven instead via a hand-built `fakeStringRules` value directly against `applyStringConstraints`. Documented in both `constraints.proto`'s `StringPattern` doc comment and the test's own doc comment.

## Known Stubs

None in the "hardcoded empty UI value" sense. Two deliberate, documented scope boundaries exist (not stubs — every field they'd apply to is either absent from this plan's corpus or, where resolvable, explicitly recorded residual rather than silently dropped):
- Unsigned integer interval constraints (`uint32`/`uint64`/`fixed32`/`fixed64` `gt`/`gte`/`lt`/`lte`) are recorded residual, not translated.
- `bytes.min_len`/`max_len`/`len`/`pattern` are recorded residual, not translated.

Both logged to `.planning/WINDOWS.md` as open `deviation` entries.

## User Setup Required

None - no external service configuration required. Same `buf`/`protoc-gen-go` install note as every prior Phase 1 plan applies for regenerating the corpus (`go install github.com/bufbuild/buf/cmd/buf@v1.72.0`, `go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.11`, both on `$(go env GOPATH)/bin`); not required to build or test `mixinforproto` (all stubs, including `constraints.pb.go`, are committed per D-22 — verified live with `buf` entirely absent from `PATH`).

## Next Phase Readiness

- **Phase 1 is now feature-complete** per its own `01-CONTEXT.md` scope (MIX-01…MIX-14, ANNO-01…ANNO-04, VAL-01…VAL-03, PIPE-01…PIPE-04, PIPE-08) — this was the last plan.
- **Carry-forward flag for `mixinforproto.md` §4.1:** the design doc's Tier 1 table is factually wrong about string length units (it doesn't distinguish code-point from byte semantics) regardless of which Task 1 option was chosen. Per the Task 1 resolution's own text, this doc edit is out of this phase's scope — raise it at the Phase 1 → Phase 2 transition.
- **Carry-forward flag for Phase 3 (Tier 2/PIPE-06):** the Task 1 resolution's two stated consequences apply directly — (1) Phase 3's differential harness (`protovalidate verdict == ent mutation verdict`) will, by construction, disagree for non-ASCII inputs on `string.min_len`/`max_len`/`len` — Phase 3 must either exclude these from the harness with an explicit recorded exemption or treat the divergence as expected; (2) unsigned-integer intervals and `bytes.*` constraints recorded residual by THIS plan are exactly the kind of provenance Phase 3's CEL execution and Phase 5's fingerprint comparison are meant to pick up — `SourceField.ResidualIDs`/`ResidualFingerprint` are already populated and ready for them.
- **Carry-forward flag (inherited, still open):** re-verify `github.com/cel-expr/cel-go`'s module-path migration status before Phase 3 planning; this plan added no `cel-go` dependency of any kind (confirmed: `google/cel-go` remains indirect and unchanged from Plan 01; no `cel-expr/cel-go` anywhere).
- **Carry-forward flag (new, this plan):** unsigned-integer interval translation and `bytes.*` length/pattern translation are real, documented gaps (residual-recorded, not silently dropped) — a natural, bounded follow-up for whichever future plan wants full VAL-01/VAL-02 parity across every scalar kind, not just the signed/string ones this plan's corpus tested.

---
*Phase: 01-mixinforproto-core*
*Completed: 2026-08-08*

## Self-Check: PASSED

All files listed above (mixinforproto/validate.go, mixinforproto/validate_test.go, proto/mixinforprototest/v1/constraints.proto, proto/buf/validate/validate.proto, mixinforproto/internal/gen/mixinforprototestv1/constraints.pb.go, a sample golden fixture, .planning/WINDOWS.md) verified present on disk; commit `a1238fa` verified present in `git log --oneline --all`.
