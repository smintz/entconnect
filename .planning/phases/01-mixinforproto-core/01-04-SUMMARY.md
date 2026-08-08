---
phase: 01-mixinforproto-core
plan: 04
subsystem: mixinforproto
tags: [go, protoreflect, ent, entc, mixin, failure-surface, oneof, reserved-identifiers]

# Dependency graph
requires:
  - phase: 01-01
    provides: "derive[M]/mapField seam, Option/options struct, SourceMessage/SourceField annotation contract, MixinForProto[M]/Validate[M] public API, buf pipeline, committed corpus stubs"
  - phase: 01-02
    provides: "Complete field-mapping table, classify(fd) real-oneof/synthetic-oneof distinction, AsJSON option and its validateAsJSON failure surface"
provides:
  - "Exclude(names ...string) and Override(name string, f ent.Field) exported option constructors"
  - "errors.go: failure record type and derivationError aggregate implementing D-08's self-sufficient first-line format and D-24's deterministic sort"
  - "reserved.go: reservedStatic (41 unique, hand-copied from entc/gen/type.go) and reservedStructural ({id, label}) collision catalogs, isReserved(name)"
  - "checkOneofResolution: MIX-10's unresolved-oneof gate over every real (non-synthetic) oneof"
  - "Validate[M] parity proven end-to-end for every failure path via failure_test.go's TestSchemaLoadFailures, without entc.LoadGraph/entc.Generate"
  - "reserved.proto corpus (ReservedStatic, ReservedStructural, NotReserved, PartialOneof) plus committed stub and regenerated proto/mixinforprototest.binpb"
affects: [01-05]

# Actuals (#2632)
actuals:
  tokens: 18300
  tasks: 3
  commits: 4

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "failure{message, field, fieldIndex, rule, description, remedy} struct feeding a single derivationError aggregate, replacing Plan 01/02's ad-hoc []string error collection — every failure path (option validation, fieldmap errors, reserved collisions, oneof gate) now produces the same shape"
    - "Override installs the caller's ent.Field verbatim before mapField/classify ever runs for that field name — this is also what lets Override resolve a real-oneof member without going through classRealOneofMember at all"
    - "Reserved-identifier check runs only on fields that actually derive through mapField (never on excluded or overridden fields) — Override is the documented escape hatch, not a second check to satisfy"
    - "checkOneofResolution and validateOptionNames are both post-context checks called once per derive[M], not threaded into the per-field walk, because both need information (the full oneof member set; the full option name set) that outruns a single field's scope"

key-files:
  created:
    - mixinforproto/errors.go
    - mixinforproto/errors_test.go
    - mixinforproto/reserved.go
    - mixinforproto/reserved_test.go
    - mixinforproto/failure_test.go
    - proto/mixinforprototest/v1/reserved.proto
    - mixinforproto/internal/gen/mixinforprototestv1/reserved.pb.go
  modified:
    - mixinforproto/option.go
    - mixinforproto/derive.go
    - mixinforproto/fieldmap.go
    - mixinforproto/fieldmap_test.go
    - proto/mixinforprototest.binpb

key-decisions:
  - "failure.line() renders 'mixinforproto: <Msg>.<field>: <description> — <remedy>' with no separate '(via rule)' clause — every description string names its triggering option/condition explicitly (e.g. 'Exclude(%q) names a field that does not exist'), so the D-08 first-line format stays exactly as specified while still satisfying the acceptance criterion that the first line name the offending option."
  - "Empty-name (Exclude(\"\"), AsJSON(\"\")) and unknown-name failures share one detection path: md.Fields().ByName(\"\") returns nil the same as any other nonexistent name, so no special-cased empty check was needed — the %q-quoted description text ('Exclude(\"\") names a field that does not exist...') already names the empty string explicitly, satisfying MIX-07's empty edge probe without extra code."
  - "Reserved-identifier check (isReserved) runs only for fields that survive mapField with f != nil — excluded fields never reach it (continue'd earlier), and overridden fields install verbatim before mapField/classify runs at all, so Override remains the uncontested escape hatch D-10 documents, not a second gate."
  - "checkOneofResolution treats 'resolved' as isExcluded(name) || isOverridden(name), independent of whether an Override's replacement field is nil — a nil replacement is already a separate collected failure (validateOptionNames), so the oneof gate does not double-flag it as also unresolved."
  - "fieldIndexUnnamed (math.MaxInt32-equivalent sentinel) orders unknown-name failures (no real descriptor position) after every real field in the deterministic sort; fieldIndexMessageScoped (-1) is defined for a future truly message-scoped failure but unused in this plan — every failure produced here names at least one field or oneof."
  - "reservedStructural ships as exactly {id, label} per this plan's R3 reconciliation — type/edge/where are named in CONTEXT.md D-10 but were not verified against ent's source in 01-RESEARCH.md, and speculatively rejecting them would force adopters into an unnecessary Override for legitimate field names."

patterns-established:
  - "Pattern: every new failure path in mixinforproto builds a failure{} struct and appends it to the shared failures slice threaded through derive[M] — no function anywhere in the package constructs a bare error string for a user-facing schema-load failure."
  - "Pattern: option-name validation (Exclude/Override/AsJSON unknown names, nil Override, Exclude/Override conflicts) all live in one function, validateOptionNames, called once before the per-field walk — Plan 05 extending the option set should add its name-validation branch there, not scatter a new pre-walk check elsewhere."

requirements-completed: [MIX-07, MIX-08, MIX-10, MIX-11, MIX-12, ANNO-03]

coverage:
  - id: D1
    description: "Naming an unknown field in Exclude, Override, or AsJSON fails at schema load with a message naming the offending message, field, and option, plus the fix — never a bare Go panic"
    requirement: "MIX-07/MIX-08"
    verification:
      - kind: unit
        ref: "mixinforproto/errors_test.go#TestOptionUnknownNamesAreCollected, #TestOverrideUnknownName"
        status: pass
      - kind: unit
        ref: "mixinforproto/failure_test.go#TestSchemaLoadFailures/unknown_Exclude_name, /unknown_Override_name, /unknown_AsJSON_name"
        status: pass
    human_judgment: false
  - id: D2
    description: "A message containing a real oneof fails at schema load unless every member is excluded or overridden; no member is ever silently guessed"
    requirement: "MIX-10"
    verification:
      - kind: unit
        ref: "mixinforproto/reserved_test.go#TestOneofUnresolvedFails, #TestOneofFullyExcludedSucceeds, #TestOneofFullyOverriddenSucceeds, #TestOneofPartialResolutionFails, #TestSyntheticOneofNotGated"
        status: pass
      - kind: unit
        ref: "mixinforproto/failure_test.go#TestSchemaLoadFailures/unresolved_oneof, /partially_resolved_oneof"
        status: pass
    human_judgment: false
  - id: D3
    description: "A derived field name colliding with an ent reserved identifier fails at schema load with an Exclude/Override remedy; no derived field is ever silently renamed"
    requirement: "MIX-11 (via D-10)"
    verification:
      - kind: unit
        ref: "mixinforproto/reserved_test.go#TestReservedStaticCatalogSize, #TestReservedStructuralCatalog, #TestReservedCollisionFails, #TestNotReservedDerivesCleanly, #TestNoAutoRename"
        status: pass
    human_judgment: false
  - id: D4
    description: "Exclude and Override decisions are recorded on the SourceMessage annotation, so a later drift check can distinguish deliberate omission from accidental drift"
    requirement: "ANNO-03"
    verification:
      - kind: unit
        ref: "mixinforproto/errors_test.go#TestAnnotationRecordsExcludeOverride"
        status: pass
    human_judgment: false
  - id: D5
    description: "Every schema-load failure is reproducible in-process via Validate[M] with a byte-identical message, without invoking go generate or entc's schema loader"
    requirement: "MIX-12"
    verification:
      - kind: unit
        ref: "mixinforproto/failure_test.go#TestSchemaLoadFailures (11 subtests, each asserting err.Error() == recovered panic message)"
        status: pass
      - kind: other
        ref: "git grep -nE 'LoadGraph|entc\\.Generate' -- mixinforproto/failure_test.go (no match)"
        status: pass
    human_judgment: false
  - id: D6
    description: "Every failure message's first line is self-sufficient and contains no embedded newline; collected failures render in a deterministic order (field descriptor index, then rule) so repeated runs are byte-identical"
    requirement: "MIX-11"
    verification:
      - kind: unit
        ref: "mixinforproto/errors_test.go#TestFailureFirstLineIsSelfSufficient, #TestFailureOrderingIsDeterministic"
        status: pass
      - kind: unit
        ref: "mixinforproto/failure_test.go#assertFirstLineSelfSufficient (shared helper applied to all 11 rows)"
        status: pass
      - kind: other
        ref: "Negative-verification: temporarily replaced the panic value with a bare \"boom\" string; all 11 TestSchemaLoadFailures subtests went red, confirming assertions are load-bearing; reverted before commit"
        status: pass
    human_judgment: false

duration: 45min
completed: 2026-08-08
status: complete
---

# Phase 1 Plan 4: Failure Surface — Exclude/Override, Reserved Collisions, Unresolved Oneofs, and In-Process Reproduction Summary

**Every failure this mixin can produce now arrives at schema load, names itself precisely (self-sufficient first line, deterministic ordering), and reproduces byte-identically in-process via `Validate[M]` — proven by an 11-row table-driven suite that goes red the moment the message text degrades to a bare Go panic.**

## Performance

- **Duration:** ~45 min
- **Started:** 2026-08-08T11:03:00Z (approx, first file read)
- **Completed:** 2026-08-08T11:48:00Z
- **Tasks:** 3
- **Files modified:** 12 (7 new, 5 modified)

## Accomplishments

- `Exclude(names ...string)` and `Override(name string, f ent.Field)` are now exported (MIX-07/MIX-08). Both validate their names against the descriptor's real field inventory before the per-field walk starts; unknown names, a nil `Override` replacement, and a name passed to both options are each collected as a distinct, self-sufficient failure rather than the first one winning silently.
- `errors.go` replaces Plan 01/02's ad-hoc `[]string` error collection with a `failure{message, field, fieldIndex, rule, description, remedy}` struct and a `derivationError` aggregate implementing D-08's exact first-line format (self-sufficient even when entc's subprocess truncates output to one line) and D-24's deterministic sort (field descriptor index, then rule, then description).
- `reserved.go` carries the hand-copied, case-sensitive union of `entc/gen/type.go`'s `globalIdent`/`privateField` (41 unique identifiers after deduplicating `config`) plus the explicitly incomplete structural supplement `{id, label}` — never `type`/`edge`/`where`, per this plan's R3 reconciliation (unverified against ent's source; adding them speculatively would reject legitimate contract field names). A collision fails at schema load naming both `Exclude` and `Override` as remedies; no field is ever auto-renamed (`TestNoAutoRename`).
- `checkOneofResolution` implements MIX-10's unresolved-oneof gate over every real (non-synthetic) oneof: every member must be excluded or overridden, or the failure names the oneof and lists only its unresolved members, in declaration order. The `IsSynthetic()` guard (already established by Plan 02's `classify`) keeps every proto3 `optional` scalar from ever triggering it — verified directly against the `Presence` and `Oneofs` corpus fixtures.
- `failure_test.go`'s `TestSchemaLoadFailures` is a single table-driven suite covering 11 distinct failure paths. Every row proves three things: `Fields()` panics at schema load (not first mutation), the panic message's first line is self-sufficient (shared `assertFirstLineSelfSufficient` helper), and `Validate[M]` returns an error byte-identical to the panic message without ever invoking `entc.LoadGraph`/`entc.Generate` (MIX-12 parity, `git grep` confirms). A negative-verification run — temporarily replacing the panic value with a bare `"boom"` string — made all 11 subtests fail, proving the assertions are load-bearing, not vacuous; reverted before commit.
- Added `reserved.proto` (`ReservedStatic`, `ReservedStructural`, `NotReserved`, `PartialOneof`) to the corpus, committed its generated stub, and re-ran `scripts/pipeline.sh` so `proto/mixinforprototest.binpb` stays in sync (D-22) — `buf lint`/`buf generate` produced zero drift in any previously-committed stub.

## Task Commits

1. **Task 1: Exclude/Override options, collected structured failures, D-08 message format** - `fce46ea` (feat)
2. **Task 2: Reserved-identifier collision check and the unresolved-oneof gate** - `6267562` (feat)
3. **Task 3: Load-time failure suite proving every panic path fires at schema load and reproduces in-process** - `6ffa6f6` (test)
4. **Follow-up: regenerate proto/mixinforprototest.binpb** - `987ef66` (fix) — see Deviations

**Plan metadata:** committed alongside this SUMMARY (see below).

## Files Created/Modified

- `mixinforproto/errors.go` - `failure` struct, `derivationError` aggregate, D-08 first-line rendering, D-24 deterministic sort
- `mixinforproto/errors_test.go` - MIX-07/MIX-08 empty/encoding edge probes, ANNO-03 annotation recording, D-05 relay-suppression proof
- `mixinforproto/reserved.go` - `reservedStatic` (41 unique), `reservedStructural` ({id, label}), `isReserved`
- `mixinforproto/reserved_test.go` - catalog-size/content proofs, no-over-reject proof, no-auto-rename proof, full oneof-gate coverage
- `mixinforproto/failure_test.go` - `TestSchemaLoadFailures` (11 rows), shared `recoverPanic`/`assertFirstLineSelfSufficient` helpers, success-path row
- `mixinforproto/option.go` - `Exclude`, `Override` exported; `overridden` changed from `map[string]bool` to `map[string]ent.Field` so a nil replacement is distinguishable from "never overridden"
- `mixinforproto/derive.go` - `validateOptionNames` (pre-walk name validation), Override-before-mapField installation, reserved check post-mapField, `checkOneofResolution` post-walk
- `mixinforproto/fieldmap.go` - `validateAsJSON` now returns `[]failure`; unsupported-kind error text no longer double-prefixes `"mixinforproto: msg.field:"` now that `errors.go` owns that formatting
- `mixinforproto/fieldmap_test.go` - `TestGolden/Oneofs` now passes `Exclude("a", "b")` (Rule 1 fix, see Deviations)
- `proto/mixinforprototest/v1/reserved.proto` / `mixinforproto/internal/gen/mixinforprototestv1/reserved.pb.go` - corpus + committed stub
- `proto/mixinforprototest.binpb` - regenerated to include `reserved.proto`

## Decisions Made

- **`failure.line()` format:** `mixinforproto: <Msg>.<field>: <description> — <remedy>`, with no separate `(via rule)` clause — every `description` string names its triggering option/condition explicitly (e.g. `Exclude(%q) names a field that does not exist`). This keeps the D-08 format exactly as specified while still satisfying "the first line names the offending option" as an emergent property of description text, not a formatting addition.
- **Empty-name and unknown-name detection share one code path**: `md.Fields().ByName("")` returns `nil` the same as any nonexistent name, so `Exclude("")`/`AsJSON("")` need no special-cased branch — the `%q`-quoted description already names the empty string explicitly.
- **Reserved-identifier check placement:** runs only for fields that survive `mapField` with `f != nil`. Excluded fields `continue` earlier; overridden fields install verbatim *before* `mapField`/`classify` ever run for that name — so `Override` remains the sole, uncontested escape hatch D-10 documents, never a second gate to also satisfy.
- **`checkOneofResolution`'s "resolved" definition** is `isExcluded(name) || isOverridden(name)`, independent of whether an `Override`'s replacement is itself nil — a nil replacement is already a separate collected failure from `validateOptionNames`, so the oneof gate does not double-flag the same field.
- **`reservedStructural` stays `{id, label}` only** — `type`/`edge`/`where` are named in CONTEXT.md D-10 but unverified against ent's source per 01-RESEARCH.md's Open Question 1; this plan's R3 reconciliation explicitly ships without them and documents the Go-compiler-error fallback for anything outside both catalogs.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] `fieldmap.go` needed modification even though it is not in this plan's `files_modified` list**
- **Found during:** Task 1 (implementing `errors.go`'s structured `failure` type)
- **Issue:** `errors.go`'s `failure.line()` renders `"mixinforproto: <Msg>.<field>: <description> — <remedy>"`. Plan 01/02's `mapField`/`validateAsJSON` error strings already embedded that exact `"mixinforproto: %s.%s: ..."` prefix and their own trailing remedy text — wiring them into the new structured-failure format unchanged would have produced a double-prefixed, doubly-remedied rendered message (e.g. `mixinforproto: X.Y: mixinforproto: X.Y: unsupported field kind ... — use Exclude(...)... — use Exclude(...)`).
- **Fix:** Stripped the `"mixinforproto: %s.%s: "` prefix and the inline remedy clause from `mapScalar`/`mapOptionalScalar`'s unsupported-kind errors and from the `ResolveFieldRules` error wrap, leaving plain description text; `derive.go` now supplies the location and remedy uniformly via the `failure` struct. `validateAsJSON` was converted from `[]string` to `[]failure` for the same reason — it needed to become a first-class contributor to the same collected-failures/derivationError pipeline Task 1 introduced, not a separate string-returning helper bolted on afterward.
- **Files modified:** `mixinforproto/fieldmap.go`
- **Verification:** All of Plan 01/02's existing tests (`derive_test.go`, `fieldmap_test.go`) continued to pass unchanged after the rewiring; `go vet`/`go test -race -count=5` clean.
- **Committed in:** `fce46ea` (Task 1 commit)

**2. [Rule 1 - Bug] `fieldmap_test.go`'s `TestGolden/Oneofs` subtest asserted now-stale behavior once Task 2's oneof gate shipped**
- **Found during:** Task 2 (running the full suite after adding `checkOneofResolution`)
- **Issue:** Plan 02's `Oneofs` golden subtest called `derive[*mixinforprototestv1.Oneofs]` with zero options, relying on the pre-Task-2 behavior where an unresolved real-oneof member (`choice`'s `a`/`b`) silently produced no field. Task 2's whole purpose is to make that fail loudly instead (MIX-10) — so the existing zero-option call now correctly returns an error, and the subtest's assertions on `d.fields` became unreachable/wrong.
- **Fix:** Added `Exclude("a", "b")` to the subtest's `derive` call, preserving its original intent (proving `classify()`'s MIX-05/MIX-10 adjacency — that `maybe`'s synthetic oneof is never conflated with `choice`'s real one) while satisfying the new gate. The golden fixture's JSON content is unchanged (still just the `maybe` field), confirmed by the unmodified `testdata/oneofs.golden` passing without `-update`.
- **Files modified:** `mixinforproto/fieldmap_test.go`
- **Verification:** `TestGolden/Oneofs` passes; `testdata/oneofs.golden` required no regeneration (verified via `git status` showing no `testdata/*.golden` changes).
- **Committed in:** `6267562` (Task 2 commit)

**3. [Rule 2 - Missing Critical] `proto/mixinforprototest.binpb` was not regenerated after adding `reserved.proto` to the corpus**
- **Found during:** post-Task-3 self-review, per this plan's own `<environment_notes>` instruction ("If you add corpus protos, commit their stubs and re-run scripts/pipeline.sh so proto/mixinforprototest.binpb stays in sync")
- **Issue:** Task 2 added `reserved.proto` and its generated Go stub, but the committed `FileDescriptorSet` (`proto/mixinforprototest.binpb`, D-22) was not regenerated to include it — leaving the descriptor set and the live corpus out of sync, exactly the "two independent sources of truth" hazard Pitfall 9 warns about.
- **Fix:** Ran `bash scripts/pipeline.sh` end-to-end (all 5 steps; steps 4-5 correctly report their Phase 1 no-op skip lines). `buf lint`/`buf generate` produced zero drift in any previously-committed `.pb.go` stub; only `proto/mixinforprototest.binpb` changed.
- **Files modified:** `proto/mixinforprototest.binpb`
- **Verification:** `git status --short proto/ mixinforproto/internal/gen/` showed only the `.binpb` as modified before this commit; `bash scripts/pipeline.sh` exits 0.
- **Committed in:** `987ef66` (follow-up commit, after all three task commits)

---

**Total deviations:** 3 auto-fixed (1 Rule 3 blocking, 1 Rule 1 bug fix to a now-stale assertion, 1 Rule 2 missing-critical sync step)
**Impact on plan:** All three were necessary to make the plan's own acceptance criteria and D-22's committed-descriptor discipline actually true rather than apparently true. No scope creep — no Tier 2/3 validation logic, no new option constructors beyond `Exclude`/`Override`, no `WithMessageRules` declaration (confirmed absent via `git grep -n 'func WithMessageRules' mixinforproto/option.go`, no match).

## Issues Encountered

- **`buf`/`protoc-gen-go` were not on `PATH`** in this session, same as every prior plan. `buf@v1.72.0` required bumping the local toolchain via `go install`'s automatic `GOTOOLCHAIN=auto` fetch (it declares `go >= 1.25.10`); this only affects the standalone `buf` binary's own build, not `mixinforproto/go.mod`'s pinned `go 1.24.0`/`toolchain go1.24.7`, which were verified unchanged before and after every commit in this plan.
- **R3 residual carried forward (from this plan's own "Reconciliation applied in this plan" section, restated here per the plan's `<output>` instruction):** whether `type`, `edge`, and `where` collide with ent-generated identifiers the same per-type-generated way `id`/`label` do was **not** determined this plan. `reservedStructural` ships as `{id, label}` only. Closing this properly requires reading `entc/gen/type.go`'s field-name-*generation* logic (not just its reserved-word maps) — a worthwhile follow-up, deliberately not budgeted here. Until then, a contract field literally named `type`, `edge`, or `where` will either derive successfully (if no actual per-type collision exists) or surface as a Go compiler `redeclared in this block` error at codegen time — never a mixinforproto schema-load panic, and never silently auto-renamed.

## Known Stubs

None. `TranslatedIDs`/`ResidualIDs` on `SourceField` remain empty `[]string{}` (not stubs — they are Plan 05's explicit Tier 1 translation scope, already documented as such by Plan 01/02's summaries).

## User Setup Required

None - no external service configuration required. Same `buf`/`protoc-gen-go` install note as every prior Phase 1 plan applies for regenerating the corpus; not required to build or test `mixinforproto` (all stubs, including `reserved.pb.go`, are committed per D-22).

## Next Phase Readiness

- Every requirement this plan claims (MIX-07, MIX-08, MIX-10, MIX-11, MIX-12, ANNO-03) is covered by a test asserting specific message content, not merely "it panicked" — verified live via the negative-verification run documented above.
- `errors.go`'s `failure`/`derivationError` types and `validateOptionNames`'s single pre-walk validation point are the seam Plan 05 (Tier 1 constraint translation) should extend if it needs a new option-name failure class — do not scatter a second ad-hoc validation function elsewhere.
- **Carry-forward flag (R3 residual, restated per this plan's own `<output>` instruction):** re-verify whether `type`/`edge`/`where` structurally collide with per-type ent identifiers before any future plan considers extending `reservedStructural` — read `entc/gen/type.go`'s field-name-generation logic directly, not just its static reserved-word maps.
- **Carry-forward flag (inherited from Plan 01, still open):** re-verify `github.com/cel-expr/cel-go`'s module-path migration status before Phase 3 planning; this plan added no `cel-go` dependency of any kind.
- Phase 1's five plans are now complete pending this SUMMARY and STATE.md update; Plan 05 (per ROADMAP.md, if scoped) or a phase transition is the next step.

---
*Phase: 01-mixinforproto-core*
*Completed: 2026-08-08*

## Self-Check: PASSED

All 12 files listed above (7 created, 5 modified) verified present on disk; all four commits (`fce46ea`, `6267562`, `6ffa6f6`, `987ef66`) verified present in `git log --oneline --all`.
