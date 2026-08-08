---
phase: 01-mixinforproto-core
plan: 06
subsystem: mixinforproto
tags: [ent, protobuf, protoreflect, fieldmap, gap-closure]

# Dependency graph
requires:
  - phase: 01-mixinforproto-core (plans 01-05)
    provides: derive[M]/mapField/classify core, corpus, error surface (failure/derivationError), README field-mapping table, WINDOWS.md ledger
provides:
  - "classify()'s missing fd.IsList() branch, closing 01-VERIFICATION.md gap 1 / CR-01 (MIX-02)"
  - "classRepeated fieldClass + mapRepeated: repeated message fields keep MIX-09/MIX-10 behavior; every other repeated kind fails loudly at schema load with the Exclude/Override remedy"
  - "proto/mixinforprototest/v1/repeated.proto corpus fixture (RepeatedScalar/RepeatedEnum/RepeatedItemsFormat/RepeatedMessage) that makes this defect shape observable going forward"
  - "README.md and WINDOWS.md record the repeated-cardinality boundary as a deliberate, documented v0.1 limitation, not a silent gap"
affects: ["01-07 (CI staleness gate, unrelated gap)", "01-09 (corpus-adequacy guard cites fieldClass numeric values)", "Phase 3 (a real list-typed ent mapping is the follow-up this plan defers)"]

actuals:
  tokens: 6900
  tasks: 3
  commits: 5

tech-stack:
  added: []
  patterns:
    - "classify()'s cardinality-before-oneof-before-optional branch order is now the canonical shape: IsMap() -> IsList() -> real-oneof -> optional-keyword -> Kind switch, each guarded by a threat-model-cited ordering hazard in the doc comment."
    - "mapField's error-returning branches route through derive.go's existing failure{rule:\"fieldmap\"} wrapper rather than constructing a new failure inline — mapRepeated follows the same pattern as buildScalarField's default case."

key-files:
  created:
    - proto/mixinforprototest/v1/repeated.proto
    - mixinforproto/internal/gen/mixinforprototestv1/repeated.pb.go
  modified:
    - mixinforproto/fieldmap.go
    - mixinforproto/fieldmap_test.go
    - mixinforproto/README.md
    - .planning/WINDOWS.md

key-decisions:
  - "IsMap() stays strictly before IsList() in classify() — a map field also reports IsList()==true in protoreflect, so this ordering is load-bearing (T-01G-01), not stylistic."
  - "Repeated message-typed fields are the one repeated shape NOT routed to the loud-failure path — mapRepeated delegates them to the existing mapMessageField, preserving MIX-09/MIX-10 unchanged."
  - "No real list-typed ent mapping (field.JSON([]T{}), field.Strings()) is attempted in this plan — loud failure is the D-10-consistent posture; the real mapping is recorded as a new open Broken Window (id 3), not built here."
  - "repeated.items.string.uri (not a bare string.* rule on the repeated field) is the only lint-clean way to express a constrained-repeated fixture — buf lint rejects string.* on repeated fields directly, confirmed live."

requirements-completed: [MIX-02]

coverage:
  - id: D1
    description: "A repeated scalar or repeated enum field fails at schema load (self-sufficient first line naming message/field/cardinality, Exclude/Override remedy) instead of silently deriving a singular ent field"
    requirement: "MIX-02"
    verification:
      - kind: unit
        ref: "mixinforproto/fieldmap_test.go#TestRepeatedCardinality/repeated_scalar_fails_at_schema_load"
        status: pass
      - kind: unit
        ref: "mixinforproto/fieldmap_test.go#TestRepeatedCardinality/repeated_enum_fails_at_schema_load"
        status: pass
    human_judgment: false
  - id: D2
    description: "A repeated field carrying protovalidate repeated.items constraints also fails loudly rather than deriving with constraints silently discarded"
    requirement: "MIX-02"
    verification:
      - kind: unit
        ref: "mixinforproto/fieldmap_test.go#TestRepeatedCardinality/repeated_field_with_constraints_fails_loudly,_not_silently_discarded"
        status: pass
    human_judgment: false
  - id: D3
    description: "Repeated message-typed fields keep their pre-existing MIX-09/MIX-10 skip-by-default and AsJSON opt-in behavior unchanged"
    verification:
      - kind: unit
        ref: "mixinforproto/fieldmap_test.go#TestRepeatedCardinality/repeated_message_field_keeps_skip-by-default_behavior"
        status: pass
      - kind: unit
        ref: "mixinforproto/fieldmap_test.go#TestRepeatedCardinality/repeated_message_field_honors_AsJSON_opt-in"
        status: pass
      - kind: unit
        ref: "mixinforproto/fieldmap_test.go#TestGolden (Messages subtest, unchanged golden)"
        status: pass
    human_judgment: false
  - id: D4
    description: "The boundary is discoverable: README field-mapping table gains three rows and a prose sentence; WINDOWS.md records a new open entry"
    verification:
      - kind: other
        ref: "mixinforproto/README.md field-mapping table + .planning/WINDOWS.md entry id 3 (grep-verified)"
        status: pass
    human_judgment: false

duration: 30min
completed: 2026-08-08
status: complete
---

# Phase 1 Plan 06: Repeated-Cardinality Gap Closure Summary

**`classify()` in `mixinforproto/fieldmap.go` now tests `fd.IsList()`: a repeated scalar or enum fails loudly at schema load with an Exclude/Override remedy instead of silently deriving as a singular field, and the corpus now carries the fixture that would have caught this five waves ago.**

## Performance

- **Duration:** ~30 min
- **Started:** 2026-08-08T13:20:00Z
- **Completed:** 2026-08-08T13:30:19Z
- **Tasks:** 3
- **Files modified:** 6 (2 created, 4 modified)

## Accomplishments

- Closed `01-VERIFICATION.md` gap 1 (MIX-02 / roadmap SC1 / CR-01): `classify()` is now total over cardinality — `IsList()` is checked immediately after `IsMap()` (map-before-list ordering is load-bearing, since a map field also reports `IsList()==true`).
- Added `proto/mixinforprototest/v1/repeated.proto` (`RepeatedScalar`, `RepeatedEnum`, `RepeatedItemsFormat`, `RepeatedMessage`), the corpus fixture whose absence made the defect invisible to five prior green waves.
- Captured and recorded genuine RED evidence for `TestRepeatedCardinality` before touching `fieldmap.go` (see "Red evidence" below) — the fixture-fails-before/passes-after ordering this plan required.
- The class of mutation-time panic the original code review reproduced (a validator closure calling `msg.Set` against a list-cardinality descriptor) is now structurally unreachable: a repeated field never reaches a scalar/enum builder.
- Recorded the boundary in `mixinforproto/README.md`'s field-mapping table (three new rows + a prose sentence) and in `.planning/WINDOWS.md` (new open entry, id 3) — a documented v0.1 limitation, not a silent gap.

## Red evidence

`TestRepeatedCardinality`, run with `go test ./... -run 'TestRepeatedCardinality' -count=1 -v` **before** any edit to `fieldmap.go` (fixture-only state, commit `7fbe513`):

```
=== RUN   TestRepeatedCardinality
=== RUN   TestRepeatedCardinality/repeated_scalar_fails_at_schema_load
    fieldmap_test.go:278: want an error for a repeated scalar field, got nil (repeated cardinality silently dropped): derived field &{Name:tags TypeKind:string Nillable:false Optional:false Immutable:false Default:"" ValidatorCount:0 Enums:[] StorageKey: Comment: SourceField:0xbc37dd5c630}
=== RUN   TestRepeatedCardinality/repeated_enum_fails_at_schema_load
    fieldmap_test.go:293: want an error for a repeated enum field, got nil (repeated cardinality silently dropped)
=== RUN   TestRepeatedCardinality/repeated_field_with_constraints_fails_loudly,_not_silently_discarded
    fieldmap_test.go:308: want an error for a repeated field carrying repeated.items rules, got nil
=== RUN   TestRepeatedCardinality/repeated_message_field_keeps_skip-by-default_behavior
=== RUN   TestRepeatedCardinality/repeated_message_field_honors_AsJSON_opt-in
=== RUN   TestRepeatedCardinality/Validate_parity_with_derive_(MIX-12)
    fieldmap_test.go:345: want both derive and Validate to error; derive=<nil> validate=<nil>
--- FAIL: TestRepeatedCardinality (0.00s)
    --- FAIL: TestRepeatedCardinality/repeated_scalar_fails_at_schema_load (0.00s)
    --- FAIL: TestRepeatedCardinality/repeated_enum_fails_at_schema_load (0.00s)
    --- FAIL: TestRepeatedCardinality/repeated_field_with_constraints_fails_loudly,_not_silently_discarded (0.00s)
    --- PASS: TestRepeatedCardinality/repeated_message_field_keeps_skip-by-default_behavior (0.00s)
    --- PASS: TestRepeatedCardinality/repeated_message_field_honors_AsJSON_opt-in (0.00s)
    --- FAIL: TestRepeatedCardinality/Validate_parity_with_derive_(MIX-12) (0.00s)
FAIL
FAIL	github.com/smintz/entconnect/mixinforproto	0.008s
```

The failure for `repeated_scalar_fails_at_schema_load` explicitly names the derived field: `Name:tags TypeKind:string` — a **singular** `string` field, proving `RepeatedScalar.tags` (a `repeated string`) silently collapsed to a scalar exactly as `01-VERIFICATION.md` gap 1 described. The regression-guard sub-tests (repeated message skip / AsJSON opt-in) already passed pre-fix, as expected — they assert behavior this plan does not change.

After the `fieldmap.go` fix (commit `643b5b6`), the identical run is fully green:

```
--- PASS: TestRepeatedCardinality (0.00s)
    --- PASS: TestRepeatedCardinality/repeated_scalar_fails_at_schema_load (0.00s)
    --- PASS: TestRepeatedCardinality/repeated_enum_fails_at_schema_load (0.00s)
    --- PASS: TestRepeatedCardinality/repeated_field_with_constraints_fails_loudly,_not_silently_discarded (0.00s)
    --- PASS: TestRepeatedCardinality/repeated_message_field_keeps_skip-by-default_behavior (0.00s)
    --- PASS: TestRepeatedCardinality/repeated_message_field_honors_AsJSON_opt-in (0.00s)
    --- PASS: TestRepeatedCardinality/Validate_parity_with_derive_(MIX-12) (0.00s)
```

## Task Commits

Each task was committed atomically:

1. **Task 1: Add the repeated-cardinality corpus fixture and regenerate its committed stub** - `7fbe513` (feat)
2. **Task 2, RED: add failing TestRepeatedCardinality** - `07881d5` (test)
2. **Task 2, GREEN: make classify() total over cardinality** - `643b5b6` (feat)
3. **Task 3: Record the repeated boundary in README + WINDOWS.md** - `1df16ea` (docs)

**Plan metadata:** commit pending below (docs: complete plan)

_Task 2 is a `tdd="true"` task and produced two commits (RED then GREEN); no REFACTOR commit was needed — the implementation was minimal and required no cleanup pass._

## Files Created/Modified

- `proto/mixinforprototest/v1/repeated.proto` - New corpus fixture: `RepeatedScalar`, `RepeatedEnum`, `RepeatedItemsFormat` (via `repeated.items.string.uri`, the only lint-clean spelling), `RepeatedMessage`
- `mixinforproto/internal/gen/mixinforprototestv1/repeated.pb.go` - Committed generated stub for the above, produced via the scoped `buf generate --path mixinforprototest`
- `mixinforproto/fieldmap.go` - `classRepeated` fieldClass (appended, preserving prior numeric values); `classify()` gains an `fd.IsList()` branch immediately after `fd.IsMap()`; `mapField` gains a `classRepeated` case routing to the new `mapRepeated` helper
- `mixinforproto/fieldmap_test.go` - `TestRepeatedCardinality`: pins the loud-failure behavior for repeated scalar/enum/constrained-repeated fields, the unchanged skip/AsJSON behavior for repeated message fields, and MIX-12 `Validate`/`derive` parity
- `mixinforproto/README.md` - Field-mapping table: three new rows (repeated scalar/enum loud failure, repeated message skip/AsJSON, repeated-with-constraints) plus a prose sentence ahead of the table
- `.planning/WINDOWS.md` - New open entry (id 3) recording the repeated-cardinality boundary as a deliberate v0.1 limitation

## Decisions Made

- **`IsMap()` before `IsList()`, no exceptions.** In protoreflect a map field also reports `IsList()==true`; inverting the check order would silently reclassify every map field as `classRepeated`. This is threat T-01G-01 in the plan's threat model, mitigated by an explicit acceptance criterion (line-number ordering check) plus `TestGolden/Maps` staying green.
- **Repeated message fields are the sole exception to the loud-failure rule.** `mapRepeated` checks `fd.Kind() == protoreflect.MessageKind` first and delegates to the existing `mapMessageField`, so MIX-09 (`AsJSON` opt-in) and MIX-10 (skip-by-default) are completely unchanged for this one shape — verified by both `TestRepeatedCardinality`'s regression-guard sub-tests and the pre-existing `TestGolden/Messages` fixture staying green with no golden-file diff.
- **No real list-typed ent mapping was attempted.** A `field.JSON(name, []T{})` or similar is a new mapping-table feature with its own storage semantics, not a gap fix — recorded as a new open Broken Window (id 3) for a future plan, consistent with the plan's explicit scope fence.
- **`repeated.items.string.uri`, not a bare `string.uri`, for the constrained-repeated fixture.** `buf lint` rejects `(buf.validate.field).string.*` rules directly on a repeated field (`Field "urls" is of type repeated string but has (buf.validate.field).string rules` — confirmed live). The legal, lint-clean spelling for a per-item constraint is `repeated.items.string.uri`, which is what `RepeatedItemsFormat` uses.

## Deviations from Plan

None - plan executed exactly as written. The only wording adjustment was strengthening `TestRepeatedCardinality`'s failure message to print the derived field's `Name`/`TypeKind` explicitly (`&{Name:tags TypeKind:string ...}`), so the captured RED evidence self-evidently shows the singular-field mis-derivation the plan's acceptance criteria required, rather than relying on the reader to infer it from "got nil" alone. This is a test-quality improvement within Task 2's own scope, not a deviation from any plan instruction.

## Issues Encountered

None. `buf`/`protoc-gen-go` were present at `$(go env GOPATH)/bin` as the plan's "Executor environment facts" section predicted; the scoped `buf generate --path mixinforprototest` invocation worked on the first attempt and did not touch `proto/buf/validate`.

## User Setup Required

None - no external service configuration required.

## Verification (plan-level, run from repo root)

All six steps from the plan's `<verification>` block passed:

1. `export PATH="$PATH:$(go env GOPATH)/bin" && (cd proto && buf lint)` — exit 0, no output.
2. `(cd mixinforproto && go test ./... -count=1)` — full suite green, no golden fixture changed (`git status --porcelain mixinforproto/testdata` empty).
3. `(cd mixinforproto && go test -race -count=5 ./...)` — green (MIX-13 determinism + race safety).
4. `make test` — both modules pass (root module reports its documented `SKIP: . has no Go packages yet`).
5. `make test-standalone` — `GOWORK=off` standalone build+test pass.
6. `git status --porcelain mixinforproto/go.mod mixinforproto/go.sum go.mod` — empty; the `go 1.24.0` / `toolchain go1.24.7` / `github.com/rogpeppe/go-internal v1.14.0` pins survive unchanged (no `go mod tidy` was run).

Threat-model acceptance criterion (T-01G-SC) also holds: no package was installed, `go.mod`/`go.sum` diff is empty.

## Next Phase Readiness

- MIX-02 is now fully satisfied (previously `✗ BLOCKED` per `01-VERIFICATION.md`); Phase 1's remaining open gaps from that report (VAL-01/CR-02, VAL-03/CR-03, the CI staleness gate/CR-04) are out of this plan's scope and belong to separate gap-closure plans.
- The two pre-existing Broken Windows (unsigned-integer intervals; `bytes.*` translation) remain open and untouched; a third is now open (repeated-cardinality list mapping) as this plan's own recorded follow-up.
- `01-09`'s corpus-adequacy guard (referenced in this plan's frontmatter) can now assert against `classRepeated`'s final position in the `fieldClass` iota block.

---
*Phase: 01-mixinforproto-core*
*Completed: 2026-08-08*
