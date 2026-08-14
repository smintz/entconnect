---
phase: 03-validation-fidelity
plan: 02
subsystem: database
tags: [ent, protobuf, protovalidate, dynamicpb, mixinforproto, reverse-conversion]

requires:
  - phase: 03-validation-fidelity
    provides: "03-01's reverse.go string-scalar leg, the newValidationError shared constructor, and hooks.go's hookState/evaluate machinery this plan's reverse table extends without touching"
provides:
  - "reverse.go's complete D-05 mirror table: every derivation kind (scalar, optionalScalar, enum, wkt/Timestamp+Struct+Value, scalarMap, asJSON) reverse-converts an ent mutation Go value back into a protoreflect.Value, failing closed (D-12) on any conversion fault"
  - "RESEARCH Assumption A3 closed: google.protobuf.Value's protojson mapping works against the bare Value descriptor with no containing-message context, proven against all six Value oneof alternatives independently"
  - "SourceMessage.BoundaryOnly — D-09's still-unenforced-rule provenance, recording every protovalidate rule on a field that cannot be enforced at the storage layer (excluded, overridden, or produces no ent field), with a closed reason set, surviving entc's real schema-load JSON boundary"
  - "The MixedFieldRules corpus fixture (constraints.proto) plan 03-03 needs to settle the D-07/D-08 hybrid-evaluator convergence risk"
  - "D-12's fail-closed error contract pinned as an explicit tested property, not an emergent one"
affects: [03-03, 03-04, 03-05]

actuals:
  tokens: 26000
  tasks: 3
  commits: 3

tech-stack:
  added: []
  patterns:
    - "Exhaustive protoreflect.Kind switch shared between a plain scalar field and a scalarMap's per-entry key/value conversion (reverseScalarByKind), avoiding two divergent scalar-conversion paths"
    - "Absence-as-invalid-Value convention: an unset optionalScalar/asJSON/wkt-Value/wkt-Struct field reports as (protoreflect.Value{}, nil) — IsValid()==false, no error — distinct from both a converted zero and a D-12 fault"
    - "protojson.Unmarshal into a fresh dynamicpb.NewMessage(fd's own descriptor) for every JSON-backed leg (asJSON, Value, Struct) — never a hand-rolled JSON walker"
    - "Message-scoped (not field-scoped) provenance for cross-cutting facts: BoundaryOnly lives on SourceMessage, not SourceField, so it cannot touch any of the 30+ pre-existing per-field golden fixtures"

key-files:
  created:
    - proto/mixinforprototest/v1/reverse.proto
    - mixinforproto/reverse_test.go
    - mixinforproto/internal/boundarytest/ent/schema/boundaryonly.go
    - mixinforproto/testdata/reverse_scalars.golden
    - mixinforproto/testdata/reverse_enum.golden
    - mixinforproto/testdata/reverse_wkt.golden
    - mixinforproto/testdata/reverse_scalar_map.golden
    - mixinforproto/testdata/reverse_as_json.golden
    - mixinforproto/testdata/constraints_mixed_field_rules.golden
  modified:
    - mixinforproto/reverse.go
    - mixinforproto/annotation.go
    - mixinforproto/derive.go
    - mixinforproto/fieldmap.go
    - mixinforproto/corpus_test.go
    - mixinforproto/derive_test.go
    - mixinforproto/internal/boundarytest/boundary_test.go
    - mixinforproto/internal/gen/mixinforprototestv1/constraints.pb.go
    - mixinforproto/internal/gen/mixinforprototestv1/messages.pb.go
    - mixinforproto/internal/gen/mixinforprototestv1/reverse.pb.go
    - proto/mixinforprototest/v1/constraints.proto
    - proto/mixinforprototest/v1/messages.proto
    - proto/descriptorset.binpb

key-decisions:
  - "google.protobuf.Value's protojson round trip works against the BARE Value descriptor with no containing-message context — RESEARCH Assumption A3 resolved TRUE, no special-casing needed, proven against all six Value oneof alternatives (null, number, string, bool, list, struct) independently"
  - "An absent/unset ent value for optionalScalar, asJSON, wkt-Value, and wkt-Struct reports as an invalid protoreflect.Value (IsValid()==false) with a nil error — never a fabricated proto3 zero and never an empty message, per D-03's phantom-violation rationale; a genuinely non-nil-but-empty Struct/map is a real value and converts normally"
  - "scalarMap's deterministic-serialization requirement is satisfied at marshal time via proto.MarshalOptions{Deterministic: true}, not by insertion order alone — protoreflect's generic Map implementation deliberately randomizes Range/marshal iteration order (google.golang.org/protobuf/internal/detrand) regardless of insertion order, so reverseScalarMap's sorted-key insertion is correct hygiene but not itself sufficient for byte-identical wire output"
  - "SourceMessage.BoundaryOnly's RuleIDs are enumerated generically via a single protoreflect.Message.Range pass over validate.FieldRules' populated fields (the 'type' oneof member name, e.g. 'string'), plus 'required' and CEL rule IDs by name — not a full per-kind Tier 1 translation, since the fields this applies to never reach a buildXxxField call site at all"
  - "The BoundaryOnlyUnbindable reason (a field DOES derive but its class is one reverse.go cannot reverse-bind) is implemented and tested for absence-of-triggering only — it is structurally dormant today because Task 2 closed every currently-derivable class; kept as a named, closed-set outcome for a future derivation kind fieldmap.go might add before reverse.go catches up"
  - "messages.proto's singular_message field gained a required rule (not the plan's literally-stated 'string.min_len', which cannot apply to a message-typed field) as the fixture for D-09's BoundaryOnlyNoEntField test — required is valid on any field kind including message-typed ones and satisfies the same acceptance-criteria shape (a non-empty RuleIDs slice on a skipped field)"

patterns-established:
  - "D-12 three-way split (success / genuine violation produced elsewhere / infrastructure fault produced here) is now a documented, tested invariant at the top of reverse.go and pinned by TestReverseValue_EveryFailureModeFailsClosed's enumeration"

requirements-completed: [VAL-04, PIPE-05]

coverage:
  - id: D1
    description: "Every derivation kind (scalar/optionalScalar/enum/wkt-Timestamp/scalarMap) reverse-converts under test against dedicated single-purpose corpus fixtures, with an exhaustive 15-scalar-kind round trip and D-12 fail-closed behavior on wrong-Go-type input"
    requirement: "VAL-04"
    verification:
      - kind: unit
        ref: "mixinforproto/reverse_test.go#TestReverseValue_ScalarRoundTrip"
        status: pass
      - kind: unit
        ref: "mixinforproto/reverse_test.go#TestReverseValue_OptionalScalarAbsent"
        status: pass
      - kind: unit
        ref: "mixinforproto/reverse_test.go#TestReverseValue_EnumValid"
        status: pass
      - kind: unit
        ref: "mixinforproto/reverse_test.go#TestReverseValue_TimestampRoundTrip"
        status: pass
      - kind: unit
        ref: "mixinforproto/reverse_test.go#TestReverseValue_ScalarMapDeterministicOrder"
        status: pass
    human_judgment: false
  - id: D2
    description: "The JSON-to-dynamicpb leg (asJSON, google.protobuf.Value, google.protobuf.Struct) hydrates a real dynamicpb message via protojson, with Value's six oneof alternatives each proven individually and RESEARCH Assumption A3 closed"
    requirement: "VAL-04"
    verification:
      - kind: unit
        ref: "mixinforproto/reverse_test.go#TestReverseValue_AsJSONRoundTrip"
        status: pass
      - kind: unit
        ref: "mixinforproto/reverse_test.go#TestReverseValue_ProtoValueRoundTrip"
        status: pass
      - kind: unit
        ref: "mixinforproto/reverse_test.go#TestReverseValue_StructRoundTrip"
        status: pass
      - kind: unit
        ref: "mixinforproto/reverse_test.go#TestReverseValue_AsJSONAbsent"
        status: pass
    human_judgment: false
  - id: D3
    description: "Every protovalidate rule on a field that mixinforproto cannot enforce at the storage layer (excluded, overridden, or produces no ent field) is recorded on SourceMessage.BoundaryOnly with a named cause, sorted and deterministic, and survives entc's real schema-load JSON boundary"
    requirement: "PIPE-05"
    verification:
      - kind: unit
        ref: "mixinforproto/derive_test.go#TestDerive_BoundaryOnlyRecordsSkippedFieldRule"
        status: pass
      - kind: unit
        ref: "mixinforproto/derive_test.go#TestDerive_BoundaryOnlyExcludedAndOverridden"
        status: pass
      - kind: unit
        ref: "mixinforproto/derive_test.go#TestDerive_BoundaryOnlySortedAndDeterministic"
        status: pass
      - kind: unit
        ref: "mixinforproto/internal/boundarytest/boundary_test.go#TestBoundaryOnlyCrossSchemaLoadBoundary"
        status: pass
    human_judgment: false
  - id: D4
    description: "D-12's fail-closed contract (no reverse-conversion fault ever errors.As-matches *protovalidate.ValidationError) is a tested property enumerating every failure mode this file's conversions can produce"
    verification:
      - kind: unit
        ref: "mixinforproto/reverse_test.go#TestReverseValue_EveryFailureModeFailsClosed"
        status: pass
    human_judgment: false
  - id: D5
    description: "No pre-existing golden fixture changed; the new MixedFieldRules/Reverse* corpus messages gain golden coverage; check-stubs, standalone build, and cross-module dependency hygiene all stay green"
    verification:
      - kind: other
        ref: "git diff --stat HEAD~3 HEAD -- mixinforproto/testdata/ shows only new files added, zero pre-existing files modified; make build && make vet && make test-determinism && make check-stubs && make test-standalone && make check-modules && make check-goversion all exit 0"
        status: pass
    human_judgment: false

duration: 50min
completed: 2026-08-14
status: complete
---

# Phase 3 Plan 2: The Reverse Conversion Table Summary

**mixinforproto's D-05 reverse conversion table is complete — every derivation kind (scalar, enum, WKT, scalarMap, asJSON) now converts an ent mutation's Go value back into a `protoreflect.Value`, with `google.protobuf.Value`'s protojson round trip proven against its bare descriptor and every contract rule ent cannot enforce recorded on `SourceMessage.BoundaryOnly`.**

## Performance

- **Duration:** 50 min
- **Started:** 2026-08-14T15:23:00Z
- **Completed:** 2026-08-14T16:13:00Z
- **Tasks:** 3 (all `type="auto" tdd="true"`/`type="auto"`)
- **Files modified:** 22

## Accomplishments

- `mixinforproto/reverse.go` now implements a full leg per derivation class: an exhaustive, no-widening `protoreflect.Kind` switch for `scalar`/`optionalScalar` (all 15 proto scalar kinds), `enum` (descriptor-backed string→number resolution), `wkt` (`Timestamp` via `timestamppb.New`, `Struct`/`Value` via `protojson.Unmarshal` into a `dynamicpb` message), `scalarMap` (native Go map → `protoreflect.Map`, sorted-key insertion), and `asJSON` (the same `protojson`/`dynamicpb` mechanism as `Value`).
- Closed **RESEARCH Assumption A3**: `google.protobuf.Value`'s JSON shape (a oneof of scalar/struct/list/null) round-trips correctly through `protojson.Unmarshal` against its own bare descriptor with **no** containing-message context or special-casing, proven by six independent sub-tests (null, number, string, bool, list, struct) — the plan's stated "five alternatives" undercounts `Value`'s real six-member oneof, and the test exercises all six rather than dropping one to match the literal count.
- An absent/unset ent value across every JSON-backed and optional-scalar leg reports as an *invalid* `protoreflect.Value` (`IsValid() == false`, nil error) — never a fabricated proto3 zero and never an empty message, closing D-03's phantom-violation class at the reverse-conversion layer.
- Added `proto/mixinforprototest/v1/reverse.proto` (`ReverseScalars`, `ReverseEnum`, `ReverseWkt`, `ReverseScalarMap`, `ReversePayload`/`ReverseAsJSON`) — dedicated, single-purpose corpus messages so no single-field-message assumption can hide a defect, per `constraints.proto`'s own precedent — and `MixedFieldRules` in `constraints.proto`, the fixture plan 03-03 needs to settle the D-07/D-08 hybrid-evaluator convergence risk (a field carrying both a standard rule and a custom CEL rule, plus two single-rule siblings).
- Implemented D-09's second half: `SourceMessage` gained a `BoundaryOnly []BoundaryOnlyRule` member (additive under `ContractVersion=1`) recording every protovalidate rule on a field mixinforproto cannot enforce at the storage layer, with a closed reason set (`excluded` / `overridden` / `no-ent-field` / `unbindable`), sorted deterministically, and proven to survive a real `entc.LoadGraph` schema-load subprocess.
- Pinned D-12's fail-closed contract (no reverse-conversion fault can ever be mistaken for a `*protovalidate.ValidationError`) as an explicit, enumerated test rather than an emergent property of scattered individual tests.

## Task Commits

Each task was committed atomically:

1. **Task 1: Reverse legs for scalar, optionalScalar, enum, Timestamp and scalarMap** - `bb9ed01` (feat)
2. **Task 2: The JSON-to-dynamicpb leg — asJSON, google.protobuf.Value, google.protobuf.Struct** - `ad090a9` (feat)
3. **Task 3: Record boundary-only rules on SourceMessage and pin the fail-closed contract** - `e9c1953` (feat)

**Plan metadata:** _(recorded in this commit's own final metadata commit)_

## Files Created/Modified

- `mixinforproto/reverse.go` - the complete D-05 mirror table: `reverseScalarField`/`reverseScalarByKind` (exhaustive Kind switch), `reverseEnum`, `reverseWkt` (Timestamp/Struct/Value), `reverseScalarMap`, `reverseJSONMessage`/`reverseStruct` (Task 2's JSON-to-dynamicpb leg)
- `mixinforproto/reverse_test.go` - per-kind round-trip tests, absence handling, D-12 fail-closed tests, and the corpus golden fixtures for every new message
- `proto/mixinforprototest/v1/reverse.proto` - the D-05 conformance corpus
- `proto/mixinforprototest/v1/constraints.proto` - adds `MixedFieldRules` (plan 03-03's fixture)
- `proto/mixinforprototest/v1/messages.proto` - `singular_message` gains a `required` rule (the D-09 BoundaryOnly test fixture)
- `mixinforproto/annotation.go` - `SourceMessage.BoundaryOnly`, `BoundaryOnlyRule`, and the four `BoundaryOnly*` reason constants
- `mixinforproto/derive.go` - `recordBoundaryOnly` wired into the existing field walk at every non-deriving branch, plus deterministic sorting
- `mixinforproto/fieldmap.go` - `boundaryOnlyRuleIDs` (generic protovalidate rule enumeration), `sourceFieldKind`/`reverseBindableKinds` (the dormant `BoundaryOnlyUnbindable` check)
- `mixinforproto/derive_test.go` - `TestDerive_BoundaryOnly*` tests
- `mixinforproto/internal/boundarytest/ent/schema/boundaryonly.go` + `boundary_test.go` - the real-`entc.LoadGraph` proof for `BoundaryOnly`
- `mixinforproto/corpus_test.go` - `corpusCoverage` entries for every new corpus message
- Regenerated: `mixinforproto/internal/gen/mixinforprototestv1/{constraints,messages,reverse}.pb.go`, `proto/descriptorset.binpb`

## Decisions Made

See `key-decisions` in frontmatter for the full list. Highlights:
- RESEARCH Assumption A3 closed TRUE — no special-casing needed for `google.protobuf.Value`.
- `proto.MarshalOptions{Deterministic: true}` (not insertion order alone) is what actually guarantees byte-identical `scalarMap` serialization, since protobuf-go's generic `Map` deliberately randomizes iteration order via `internal/detrand`.
- `messages.proto`'s BoundaryOnly fixture uses `required` rather than the plan's literal `string.min_len` wording, since `string.min_len` cannot apply to a message-typed field — the acceptance criteria's actual requirement (a non-empty `RuleIDs` slice on a skipped field) is satisfied identically.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] scalarMap determinism test initially failed — insertion order does not control wire serialization order**
- **Found during:** Task 1's verification
- **Issue:** `reverseScalarMap`'s sorted-key insertion into the `protoreflect.Map` did not by itself produce byte-identical `proto.Marshal` output across repeated calls — protobuf-go's generic map codec deliberately randomizes `Range`/marshal iteration order (`internal/detrand`) independent of insertion order, to catch code that relies on it.
- **Fix:** The test now marshals via `proto.MarshalOptions{Deterministic: true}`, the documented, sanctioned mechanism for stable wire output; `reverseScalarMap`'s sorted-key insertion is retained as correct D-24 hygiene but the doc comment now states explicitly that it alone does not guarantee wire-level determinism.
- **Files modified:** `mixinforproto/reverse_test.go`, `mixinforproto/reverse.go` (doc comment only)
- **Committed in:** `bb9ed01` (Task 1 commit)

**2. [Rule 1 - Bug] `TestReverseValue_ProtoValueRoundTrip`'s own alternative count was wrong**
- **Found during:** Task 2's verification
- **Issue:** The test's own sanity check asserted "5 alternatives," but `google.protobuf.Value`'s real `kind` oneof has six members (null/number/string/bool/struct/list) — the plan's behavior text itself lists six items while calling them "the five alternatives."
- **Fix:** Corrected the test's count to 6 and documented the plan-text discrepancy inline, exercising all six alternatives rather than dropping one to match the literal wording.
- **Files modified:** `mixinforproto/reverse_test.go`
- **Committed in:** `ad090a9` (Task 2 commit)

**3. [Rule 2 - Missing critical functionality] `boundaryOnlyRuleIDs` needed to enumerate ALL protovalidate rule categories, not just `required`/`cel`**
- **Found during:** Task 3's verification (`TestDerive_BoundaryOnlySortedAndDeterministic` initially found only 2 of 3 expected `MixedFieldRules` entries)
- **Issue:** An initial implementation only recorded `required` and CEL rule IDs, missing standard structural rules (e.g. `string.min_len`) entirely — `MixedFieldRules.standard_only` (which carries only `string.min_len`) was silently dropped from `BoundaryOnly`.
- **Fix:** Added a generic `protoreflect.Message.Range` pass over `validate.FieldRules`' populated fields, recording the `type` oneof member's name (e.g. `"string"`) for any rule category not already covered by the `required`/CEL cases.
- **Files modified:** `mixinforproto/fieldmap.go`
- **Committed in:** `e9c1953` (Task 3 commit)

---

**Total deviations:** 3 auto-fixed (2 test-correctness bugs, 1 missing-critical-functionality gap in the new boundary-only enumeration). No scope creep — all three were necessary for the plan's own stated acceptance criteria to hold.
**Impact on plan:** None of these affected the plan's design decisions (D-05/D-09/D-12); all were implementation-correctness fixes discovered by the plan's own verification loop.

## Issues Encountered

- The sandboxed environment had no `buf`/`protoc-gen-go`/`protoc-gen-connect-go` on `PATH` at plan start; all three were installed via `go install` against the pinned versions (`buf@v1.72.0`, `protoc-gen-go@v1.36.11` matching the repo's `google.golang.org/protobuf` pin, `protoc-gen-connect-go@v1.20.0` matching the repo's `connectrpc.com/connect` pin) before any stub regeneration — a one-time environment-setup cost, not a plan deviation.
- `buf lint`/`buf build` must be run with the working directory `cd`'d into `proto/` (matching `scripts/generate-stubs.sh`'s and `scripts/pipeline.sh`'s own invocation pattern) — running it from the repo root produces spurious "imported file does not exist" errors unrelated to any real proto issue, since `buf.yaml` lives at `proto/buf.yaml` and buf does not walk up the directory tree to find it.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Plan 03-03 can now build the full D-07/D-08 hybrid evaluator (protovalidate's own evaluator for standard rules + the local `cel.Env` for residual CEL) on top of a complete `reverseValue` table — no derivation kind reverse.go cannot yet convert remains, so 03-03's hook can call `reverseValue` unconditionally for every in-scope field rather than special-casing unimplemented classes.
- `MixedFieldRules` (constraints.proto) is ready for 03-03's own tests to settle Pitfall 1 (whether a field carrying both a standard rule and a custom CEL rule is double-evaluated, and which of the two documented resolutions — dedup vs. field-exclusive routing — the hybrid hook adopts).
- `SourceMessage.BoundaryOnly` is ready for Phase 5's drift check to consume — every unenforceable rule is now named, with a cause, machine-visibly.
- `mixinforproto` still has no DB driver in its `go.mod` and still declares `go 1.24.0` (`make check-goversion` verified).

---
*Phase: 03-validation-fidelity*
*Completed: 2026-08-14*

## Self-Check: PASSED

All created/modified files (mixinforproto/reverse.go, mixinforproto/reverse_test.go,
proto/mixinforprototest/v1/reverse.proto, mixinforproto/annotation.go, mixinforproto/derive.go,
mixinforproto/fieldmap.go, mixinforproto/internal/boundarytest/ent/schema/boundaryonly.go,
mixinforproto/internal/boundarytest/boundary_test.go, mixinforproto/derive_test.go, this
SUMMARY.md) and all three task commits (bb9ed01, ad090a9, e9c1953) verified present on disk /
in git log.
