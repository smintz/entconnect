---
phase: 01-mixinforproto-core
verified: 2026-08-08T14:17:41Z
status: passed
score: 5/5 roadmap success criteria verified (26/26 requirement IDs satisfied)
behavior_unverified: 0
overrides_applied: 0
re_verification:
  previous_status: gaps_found
  previous_score: "3/5 roadmap success criteria (22/25 requirement IDs, corrected count 23/26 — see note below); 4 gaps"
  gaps_closed:
    - "Gap 1 (MIX-02/CR-01): repeated scalar/enum fields silently derived as singular fields"
    - "Gap 2 (VAL-01/CR-02): delegatingFormatValidator rejected ~100% of inputs on multi-field messages"
    - "Gap 3 (VAL-03/CR-03): optional string/bytes + required wrongly derived NotEmpty() instead of presence"
    - "Gap 4 (PIPE-03/CR-04): CI stubs job used an unscoped buf generate, contradicting pipeline.sh's own scoping requirement"
  gaps_remaining: []
  regressions: []
deferred: []
human_verification: []
---

# Phase 1: MixinForProto Core Verification Report

**Phase Goal:** Developers can derive a complete, validated ent schema directly from a protobuf message type, with zero codegen and full provenance for later drift checking
**Verified:** 2026-08-08T14:17:41Z
**Status:** passed
**Re-verification:** Yes — after gap closure (plans 01-06 through 01-09)

## Goal Achievement

All four gaps from the initial `01-VERIFICATION.md` were independently re-executed against the
live codebase in this session — not re-read from the gap-closure SUMMARYs. `buf@v1.72.0` and
`protoc-gen-go@v1.36.11` were available at `$(go env GOPATH)/bin` (exported onto `PATH` for this
session), which let this pass execute Gap 4's CI staleness gate directly, something the initial
verification could only reason about indirectly.

**Note on the initial verification's own score:** `01-VERIFICATION.md`'s frontmatter stated
"22/25 requirement IDs satisfied," but its own Requirements Coverage table lists 26 rows (14
MIX + 4 ANNO + 3 VAL + 5 PIPE) with 3 BLOCKED, i.e. 23 satisfied — the "22/25" figure was an
arithmetic slip in that report, not a real discrepancy in the codebase. This re-verification
uses the correct denominator of 26.

### Independent Reproductions (executed fresh in this session, not inferred from SUMMARYs)

**Gap 1 — repeated cardinality now fails loudly, not silently:**
```
$ go test ./... -run 'TestRepeatedCardinality' -count=1 -v
--- PASS: TestRepeatedCardinality (all 6 subtests)
```
Read `classify()`/`mapField()`/`mapRepeated()` directly: `IsList()` is checked immediately after
`IsMap()` (map-before-list ordering confirmed load-bearing and correct — a map field also reports
`IsList()==true` in protoreflect). Every repeated scalar/enum kind returns an error naming the
field and its repeated cardinality; only repeated message-typed fields delegate to the
pre-existing `mapMessageField` (MIX-09/MIX-10 unchanged).

**Gap 2 — delegated format validator, both directions independently exercised:**
Wrote and ran a standalone test (not part of the plan's own test file) that re-derives
`StringFormatWithSibling` and calls the derived `endpoint` validator directly:
```
$ go test -run 'TestIndependentGap2Verifier' -count=1 -v
--- PASS: TestIndependentGap2Verifier
```
- A valid URI (`https://valid.example/x`) on a message whose sibling field (`owner`, unset,
  carrying `required`) independently violates protovalidate is **accepted** — confirms the fix.
- An invalid URI (`totally not a uri!!`) on the same multi-field message is **still rejected** —
  confirms the fix did not become an over-aggressive filter / validation bypass (the exact
  failure mode the phase brief warned to check for).
Read `delegatingFormatValidator` directly: violations are filtered by `FieldDescriptor.FullName()
== fieldName`; a non-`*ValidationError` result is surfaced as a genuine evaluator error rather
than silently treated as "no rejection."

**Gap 3 — presence-first branch order, verified with a standalone unit call and a counterfactual:**
```
$ go test -run 'TestIndependentGap3Verifier' -count=1 -v
--- PASS: TestIndependentGap3Verifier
```
Direct call: `classifyRequired(optional=true, required=true, hasNotEmpty=true)` now returns
`requiredExactPresence`, not `requiredExactNotEmpty`. `buildStringField`/`buildBytesField` have a
`requiredExactPresence` arm emitting no `NotEmpty()`.

**Adversarial check of Plan 09's own guard-strengthening claim (counterfactual, executed
independently):** restored `classifyRequired`'s pre-01-08 branch order (`required && hasNotEmpty`
checked before `optional && required`) via a scratch edit, then re-ran the corpus-adequacy guard:
```
$ go test -run 'TestCorpusExercisesEveryRequiredResult|TestRequiredTranslation' -count=1 -v
--- FAIL: TestCorpusExercisesEveryRequiredResult
    corpus_test.go:281: requiredExactPresence is never witnessed by a field with
    hasNotEmpty=true — this is exactly the axis gap 3 hid in ...
--- FAIL: TestRequiredTranslation
    (optional_string / optional_bytes subtests both fail)
```
Confirms the claim in `01-09-SUMMARY.md`'s Deviations section: a plain per-outcome-existence
guard would have stayed green under gap 3's original defect (a non-string `optional`+`required`
field alone satisfies `requiredExactPresence` regardless of branch order), and the guard as
actually shipped is strengthened specifically to close that blind spot. File was restored
byte-identical afterward (`git status --porcelain` empty, full suite re-run green).

**Gap 4 — CI staleness gate, both destructive scenarios re-run independently:**
```
$ make check-stubs                      # clean tree
[check-stubs] OK: committed generated stubs match a fresh regeneration from proto/ sources.

$ echo "// verifier-drift-probe" >> mixinforproto/internal/gen/mixinforprototestv1/tracer.pb.go
$ make check-stubs; echo $?
Files .../tracer.pb.go and /tmp/.../tracer.pb.go differ
::error::Committed generated stubs ... are stale ...
2
$ git checkout -- mixinforproto/internal/gen/mixinforprototestv1/tracer.pb.go   # restored

$ cp .../tracer.pb.go .../zz_verifier_orphan_probe.pb.go
$ make check-stubs; echo $?
Only in mixinforproto/internal/gen/mixinforprototestv1: zz_verifier_orphan_probe.pb.go
2
$ rm -f .../zz_verifier_orphan_probe.pb.go                                      # restored
```
`git status --porcelain` confirmed empty after each scenario. Read `.github/workflows/ci.yml`
directly: the `stubs` job now runs `make check-stubs` (line ~117), which invokes
`scripts/check-stubs.sh`, which in turn calls `scripts/generate-stubs.sh` — the single canonical
`buf generate --path mixinforprototest` invocation. `grep -rln -e 'buf generate' scripts .github
Makefile` returns exactly `scripts/generate-stubs.sh`. The key link is now genuinely wired.

**Full-suite health, run fresh in this session:**
```
$ go test -race -count=5 ./...                              # all packages, green
$ cd mixinforproto && GOWORK=off go build ./... && GOWORK=off go test ./...   # green
$ make check-modules                                         # both go.mod, no Replace
$ make check-goversion                                       # go 1.24.0 pin intact
$ make build && make vet                                     # both modules clean
```

### Observable Truths (Roadmap Success Criteria)

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | Developer declares `MixinForProto[*orderv1.Order]()` and sees ent fields (scalars, enums, WKTs, presence-correct optionals, scalar maps) materialized at schema load — no descriptor file, no string names, deterministic order | ✓ VERIFIED (with a recorded, documented boundary) | All non-repeated scalars/enums/WKTs/optionals/maps verified correct. Repeated scalars/enums do NOT materialize as fields — they now fail loudly at schema load with a self-sufficient message and an Exclude/Override remedy (Broken Window #3, deliberate v0.1 boundary per D-10's no-silent-approximation posture). **Judgment call, stated plainly:** this closes CR-01's silent-corruption/mutation-time-panic defect completely, and is VERIFIED under the "materializes correctly or fails loudly, never corrupts silently" reading that MIX-11/D-10 establish as this project's actual contract. It is NOT verified under a stricter "every listed category, including repeated, produces a mapped field" reading — that reading remains unmet by design, tracked as an open Broken Window, not a silent gap. |
| 2 | Developer excludes/overrides fields via `Exclude()`/`Override()`/`AsJSON()`; unknown field or unresolved `oneof` fails schema load naming message/field/option + fix; reproducible in-process via `Validate[M]` | ✓ VERIFIED | Unchanged from initial verification; re-confirmed via full suite pass and direct reading of `derive.go`/`mixin.go`. |
| 3 | `gen.Graph` carries per-schema and per-field provenance annotations surviving entc's JSON schema-load boundary | ✓ VERIFIED | Unchanged; `internal/boundarytest` re-run green in this session as part of the full-suite pass. |
| 4 | protovalidate string/numeric/presence constraints show up as native ent builder calls (`MaxLen`, `Match`, `Min`/`Max`/`Range`/`Positive`, `NotEmpty`) | ✓ VERIFIED | Both blockers closed and independently re-executed above (Gaps 2 and 3). Numeric translation (VAL-02) unchanged from initial verification (already clean). |
| 5 | `mixinforproto` ships as an independently buildable, independently tagged module with its own `go.mod`; documented pipeline + CI enumerate both modules, run a `GOWORK=off` job, and docs cover proto3 presence/zero-collapse prominently | ✓ VERIFIED | The one open item from the initial pass — the CI staleness gate (gap 4) — is now genuinely wired and independently re-proven above with fresh destructive scenarios. `GOWORK=off` build+test re-run green. |

**Score:** 5/5 roadmap success criteria verified

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `mixinforproto/fieldmap.go` | scalar/enum/WKT/map/optional/repeated classification | ✓ VERIFIED | `classRepeated` fieldClass and `mapRepeated` present, wired, tested; `classifyRequired`'s presence-first branch order confirmed by direct call and by a live counterfactual that fails the corpus guard when reverted. |
| `mixinforproto/validate.go` | Tier 1 translation + residual recording | ✓ VERIFIED | `delegatingFormatValidator`'s per-field violation filtering confirmed by an independently-authored test exercising both the accept and reject directions on the same multi-field message. WR-06's stale "exactly three direct dependencies" comment corrected in place. |
| `proto/mixinforprototest/v1/repeated.proto` + `constraints.proto` additions | Corpus fixtures making both prior defect shapes permanently observable | ✓ VERIFIED | `RepeatedScalar`/`RepeatedEnum`/`RepeatedItemsFormat`/`RepeatedMessage`, `RequiredOptionalString`/`RequiredOptionalBytes`/`StringFormatWithSibling` all present, committed, and covered by golden fixtures. |
| `scripts/generate-stubs.sh` + `scripts/check-stubs.sh` | Single canonical scoped generation invocation + orphan-aware staleness gate | ✓ VERIFIED | Re-executed all three detection scenarios (clean/modified/orphaned) independently in this session; all three matched the SUMMARY's claims exactly, including exit codes and named files. |
| `mixinforproto/corpus_test.go` | Structural corpus-adequacy guards (fieldClass exhaustiveness, requiredResult exhaustiveness incl. hasNotEmpty split, per-message coverage) | ✓ VERIFIED | The `requiredResult` guard's counterfactual claim (would have caught gap 3) independently re-executed and confirmed true — this is the one claim in the gap-closure SUMMARYs most worth distrusting on narrative alone, and it holds. |
| `.github/workflows/ci.yml` | workspace job, GOWORK=off job, staleness job, goversion check | ✓ VERIFIED | `stubs` job runs `make check-stubs` (confirmed by direct read); `modules` job runs `make check-goversion` (confirmed by direct read and local re-run). |
| `.planning/WINDOWS.md` | Open Broken Windows tracked, closed ones marked fixed via the CLI verb | ✓ VERIFIED | 3 open (unsigned-int intervals, `bytes.*` translation, repeated-cardinality list mapping — all deliberate, documented, none silent), 1 fixed (descriptor-set staleness, closed via `gsd-tools windows fixed 4`, not a hand-edit). |

### Key Link Verification

| From | To | Via | Status | Details |
|------|-----|-----|--------|---------|
| `mixin.Annotations()` | `entc/load` JSON | schema annotation channel | ✓ WIRED | Unchanged; re-confirmed via full-suite pass. |
| `derive[M]` core | `Fields()` (panic) and `Validate[M]` (error) | shared core, two surfaces | ✓ WIRED | Unchanged. |
| `protovalidate.ResolveFieldRules(fd)` | Tier 1 builder call OR residual record | nil-safe resolution | ✓ WIRED | Unchanged. |
| `buf generate` (temp dir, scoped) | committed `mixinforproto/internal/gen` | staleness diff gate | ✓ WIRED | **Was NOT_WIRED in the initial verification (gap 4). Now confirmed wired**: CI's `stubs` job calls `make check-stubs`, which delegates to the exact same `scripts/generate-stubs.sh` `pipeline.sh` uses — independently re-proven via `grep -rln -e 'buf generate'` returning exactly one file, plus live execution of the CI job's own command locally. |
| `classify()`'s `IsList()` branch | `mapRepeated` / loud schema-load failure | cardinality-aware dispatch | ✓ WIRED (new) | Confirmed via direct test execution against a synthesized `repeated string` descriptor path (`TestRepeatedCardinality`), not merely a code read. |
| `delegatingFormatValidator`'s violation filter | per-field verdict | `FieldDescriptor.FullName()` comparison | ✓ WIRED (new) | Confirmed via an independently-authored test exercising accept AND reject on the same multi-field message. |

### Requirements Coverage

| Requirement | Source Plan | Description (abbreviated) | Status | Evidence |
|---|---|---|---|---|
| MIX-01 | 01-01 | Declare mixin, get fields, no descriptor file/string names | ✓ SATISFIED | Unchanged from initial verification. |
| MIX-02 | 01-02, 01-06 | Scalar type mapping | ✓ SATISFIED (documented boundary for repeated cardinality — see SC1 judgment call above) | `TestRepeatedCardinality` re-run green; independent counterfactual confirms the guard behind it. |
| MIX-03 | 01-02, 01-06 | Enum mapping | ✓ SATISFIED (same repeated-cardinality boundary as MIX-02) | Same code path (`classify`/`mapField`), same test. |
| MIX-04 | 01-02 | WKT mapping (Timestamp/Struct/Value/FieldMask) | ✓ SATISFIED | Unchanged. |
| MIX-05 | 01-02 | Optional/presence + zero-collapse defaults | ✓ SATISFIED | Unchanged. |
| MIX-06 | 01-02 | Scalar maps → JSON, message maps skipped | ✓ SATISFIED (WR-01 enum-valued-map warning remains open, unaddressed — pre-existing, not part of this gap-closure scope) | Unchanged; `WR-01` in `01-REVIEW.md` was not adjudicated by plans 01-06..01-09 (not in scope) and remains a live code-review warning. |
| MIX-07 | 01-04 | `Exclude()`, unknown name fails | ✓ SATISFIED | Unchanged. |
| MIX-08 | 01-04 | `Override()`, unknown name fails | ✓ SATISFIED | Unchanged. |
| MIX-09 | 01-02 | `AsJSON()` for message fields | ✓ SATISFIED | Re-confirmed unaffected by the repeated-cardinality change (`TestRepeatedCardinality`'s AsJSON subtest). |
| MIX-10 | 01-04 | Message fields skipped by default; oneof gate | ✓ SATISFIED | Re-confirmed unaffected by the repeated-cardinality change. |
| MIX-11 | 01-04 | Self-sufficient failure first line, names message/field/option/fix | ✓ SATISFIED | The new repeated-cardinality failure and the (unchanged) other failure paths both comply. |
| MIX-12 | 01-01/01-04 | In-process reproduction via `Validate[M]` | ✓ SATISFIED | `TestRepeatedCardinality`'s "Validate parity with derive" subtest re-run green. |
| MIX-13 | 01-01 | Deterministic field order | ✓ SATISFIED | `go test -race -count=5 ./...` re-run green. |
| MIX-14 | 01-01 | Independent module, minimal deps | ✓ SATISFIED (WR-06 doc-drift warning now corrected, per 01-08) | `GOWORK=off` re-run green; `validate.go`'s file header comment now states the correct dependency count. |
| ANNO-01 | 01-01 | Schema-level provenance survives JSON boundary | ✓ SATISFIED | Unchanged. |
| ANNO-02 | 01-02/01-05 | Field-level provenance readable from `gen.Graph` | ✓ SATISFIED | Unchanged. |
| ANNO-03 | 01-04 | Exclude/Override recorded as annotations | ✓ SATISFIED | Unchanged. |
| ANNO-04 | 01-01 | Version marker for mismatch detection | ✓ SATISFIED | Unchanged. |
| VAL-01 | 01-05, 01-08 | String constraints → native builder calls | ✓ SATISFIED | Independently re-executed both directions (accept valid / reject invalid) on a multi-field message; CR-02 closed. |
| VAL-02 | 01-05 | Numeric constraints, open/closed adjustment | ✓ SATISFIED | Unchanged (already clean in initial verification). |
| VAL-03 | 01-05, 01-08 | Presence/required → `NotEmpty`/non-optional | ✓ SATISFIED | Independently re-executed via direct `classifyRequired` call and via a counterfactual proving the closure is real, not narrative. CR-03 closed. |
| PIPE-01 | 01-03 | Documented pipeline script, CI runs it in order | ✓ SATISFIED | Unchanged; `pipeline.sh` now delegates step 2 to `scripts/generate-stubs.sh`. |
| PIPE-02 | 01-03 | `go.work` + `GOWORK=off` job proves standalone consumption | ✓ SATISFIED | Re-run green in this session. |
| PIPE-03 | 01-03, 01-07, 01-09 | CI enumerates + tests both modules explicitly; staleness gate genuinely functions | ✓ SATISFIED | CR-04/gap 4 closed and independently re-proven with fresh destructive scenarios (both modified-stub and orphaned-stub cases), not merely re-read. |
| PIPE-04 | 01-03 | Nested-module tag convention, no `replace` | ✓ SATISFIED | `make check-modules` re-run clean. |
| PIPE-08 | 01-03 | Docs cover presence/zero-collapse prominently | ✓ SATISFIED | Unchanged. |

**26/26 requirement IDs satisfied.** (The initial verification's frontmatter said "22/25" but its
own table showed 26 rows with 23 satisfied/3 blocked — a reporting arithmetic error in that prior
report, not a codebase discrepancy; corrected here.)

### Anti-Patterns Found

No `TBD`/`FIXME`/`XXX`/`TODO`/`HACK`/`PLACEHOLDER` markers found in any non-test `.go` file,
`scripts/*.sh`, `Makefile`, or `.github/workflows/ci.yml` (re-scanned in this session).

**Pre-existing code-review warnings not required by this gap-closure scope, still open (not
regressions, not newly discovered, informational only):**
- WR-01 (enum-valued maps silently degrade to `map[K]any` with the value type unrecorded) — unfixed, confirmed by reading `classify()`'s map branch still routes any non-`MessageKind` map value (including `EnumKind`) to `classScalarMap`.
- WR-02 (unchecked `uint64 → int` narrowing on string length bounds) — unfixed, confirmed by reading `validate.go`'s `MinLen`/`MaxLen`/`Len` translation still narrows without a range guard.
- WR-04 (reserved-identifier catalog case-sensitivity/justification issue) — not independently re-checked this session; status unchanged from `01-REVIEW.md`.
- WR-05 (`ResolveFieldRules` called twice per field, six builders) — cosmetic/performance duplication, not independently re-checked this session; status unchanged.
- WR-07 (`fieldproj.Project` decodes the first annotation of any kind) — test-infrastructure risk, not independently re-checked this session; status unchanged.

None of these five block any of the 26 requirement IDs or the 5 roadmap success criteria — they
were warnings in `01-REVIEW.md`, not gaps in `01-VERIFICATION.md`, and none of the four
gap-closure plans (01-06..01-09) claimed to address them (WR-03 and WR-06 were addressed as
in-scope side effects and are separately confirmed fixed above). Flagged here for visibility, not
as blockers.

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|---|---|---|---|
| Repeated scalar field fails loudly (not silently) | `go test -run TestRepeatedCardinality -v` | all 6 subtests pass | ✓ PASS |
| Delegated format validator, multi-field message, valid input | standalone independently-authored test | valid URI accepted | ✓ PASS |
| Delegated format validator, multi-field message, invalid input (anti-bypass) | standalone independently-authored test | invalid URI still rejected | ✓ PASS |
| `classifyRequired` presence-first for string/bytes | standalone independently-authored test | `requiredExactPresence` returned | ✓ PASS |
| Corpus guard counterfactual (gap 3 branch order reverted) | `go test -run TestCorpusExercisesEveryRequiredResult\|TestRequiredTranslation` after a scratch revert | both FAIL as required, confirming the guard's real detection power | ✓ PASS |
| CI staleness gate, clean tree | `make check-stubs` | exit 0 | ✓ PASS |
| CI staleness gate, modified stub | `make check-stubs` after deliberate edit | exit 2, names the file | ✓ PASS |
| CI staleness gate, orphaned stub | `make check-stubs` after deliberate copy | exit 2, "Only in ..." line names the orphan | ✓ PASS |
| Determinism + race safety | `go test -race -count=5 ./...` | all packages pass | ✓ PASS |
| GOWORK=off standalone build+test | `cd mixinforproto && GOWORK=off go build ./... && GOWORK=off go test ./...` | pass | ✓ PASS |
| Module hygiene | `make check-modules && make check-goversion` | pass | ✓ PASS |

Working tree confirmed clean (`git status --porcelain` empty) after every destructive scratch
edit made during this verification session.

### Human Verification Required

None. All four original gaps were confirmed closed by direct code execution in this session,
including two adversarial checks explicitly requested by the phase brief (the anti-bypass
direction on Gap 2, and the counterfactual on Plan 09's guard-strengthening claim for Gap 3),
both of which held.

### Gaps Summary

All four gaps from the initial `01-VERIFICATION.md` are closed and independently re-verified
against the live codebase in this session:

1. **Gap 1 (MIX-02/MIX-03/CR-01, repeated cardinality)** — closed by making `classify()` total
   over cardinality. **Judgment call, stated explicitly per the phase brief's request:** this
   converts a silent defect (data corruption + mutation-time panic risk) into a documented,
   loud-failure v0.1 limitation (Broken Window #3) — it satisfies MIX-02/MIX-03 and roadmap SC1
   under the "never silently corrupt, materialize correctly or fail loudly" reading this project's
   own conventions (D-10, MIX-11) establish as the actual contract. It does **not** satisfy a
   stricter "every listed field category, no exceptions, produces a mapped field" reading of SC1's
   literal wording — repeated scalars/enums still do not materialize as ent fields in v0.1. Both
   readings are defensible; this report adopts the former because it matches the project's own
   recorded precedent for how the phase treats intentionally-scoped-out translation gaps
   (unsigned-integer intervals, `bytes.*` length/pattern — Broken Windows #1/#2, both graded
   SATISFIED-with-caveat the same way in the initial verification).
2. **Gap 2 (VAL-01/CR-02, delegated format validator)** — closed by per-field violation
   filtering. Independently re-verified in both directions: accepts a valid value despite an
   unrelated sibling-field violation, and still rejects an actually-invalid value on the same
   message — ruling out the "filter became a no-op" failure mode the phase brief specifically
   warned to check for.
3. **Gap 3 (VAL-03/CR-03, presence-first required)** — closed by reordering
   `classifyRequired`'s branches. Independently re-verified via direct call and via a live
   counterfactual restoring the original (buggy) branch order, which correctly fails both
   `TestRequiredTranslation` and the strengthened corpus-adequacy guard — confirming Plan 09's own
   claim that the guard, as literally specified by the plan text, would NOT have caught this
   defect, and that the strengthening was both real and necessary.
4. **Gap 4 (PIPE-03/CR-04, CI staleness gate)** — closed by collapsing to one canonical
   `buf generate` invocation and rebuilding the gate on an empty-output-tree design. Independently
   re-executed all three detection scenarios (clean, modified, orphaned) against the live gate in
   this session — something the initial verification could not do because `buf` was not on `PATH`
   in that session.

No regressions were found. The phase's foundational architecture (annotation provenance crossing
the real entc subprocess boundary, Exclude/Override/oneof failure surface, deterministic field
ordering, module isolation, GOWORK=off standalone consumption) remains sound, as it was in the
initial verification, and the four narrow defects that verification found are now closed with
genuine, independently-reproduced evidence rather than narrative claims.

Five pre-existing code-review warnings (WR-01, WR-02, WR-04, WR-05, WR-07) remain open — they were
never gaps in the initial `01-VERIFICATION.md` and were out of scope for the four gap-closure
plans; they are noted above for visibility but do not affect this phase's status.

---

_Verified: 2026-08-08T14:17:41Z_
_Verifier: Claude (gsd-verifier)_
