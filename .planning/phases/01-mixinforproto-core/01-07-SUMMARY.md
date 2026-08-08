---
phase: 01-mixinforproto-core
plan: 07
subsystem: infra
tags: [ci, buf, protoc-gen-go, makefile, staleness-gate, gap-closure]

# Dependency graph
requires:
  - phase: 01-mixinforproto-core (plan 03)
    provides: scripts/pipeline.sh (canonical five-step pipeline), the original (broken) CI stubs job
  - phase: 01-mixinforproto-core (plan 06)
    provides: proto/mixinforprototest/v1/repeated.proto and its committed stub, the corpus state scenario A ran against
provides:
  - "scripts/generate-stubs.sh, the single canonical scoped `buf generate --path mixinforprototest` invocation"
  - "scripts/check-stubs.sh, the D-22/Pitfall-9 staleness gate that detects both modified and orphaned generated stubs"
  - "Makefile targets check-stubs and check-goversion"
  - "CI's stubs job reduced to `make check-stubs`; modules job gains `make check-goversion`"
affects: [ci-infra, pipeline, phase-2-entc-extension]

actuals:
  tokens: 3600
  tasks: 3
  commits: 2

tech-stack:
  added: []
  patterns:
    - "Single-spelling invocation pattern: a command with a documented, live-verified scoping requirement gets exactly one script that spells it; every caller (local pipeline, CI gate) delegates to that script rather than re-spelling the command, closing the drift vector that caused this gap."
    - "Empty-output-tree staleness gates: regenerate into a temp root that never contained the committed output, so `diff -rq` naturally surfaces orphaned/deleted generated files via 'Only in <committed-dir>' lines, not only modified ones."

key-files:
  created:
    - scripts/generate-stubs.sh
    - scripts/check-stubs.sh
  modified:
    - scripts/pipeline.sh
    - Makefile
    - .github/workflows/ci.yml

key-decisions:
  - "Copy the working tree's proto/ into the temp root rather than `git archive HEAD`, so `make check-stubs` is rehearsable against uncommitted proto/ edits before committing (matches the plan's explicit environment-facts guidance, verified live at plan time)."
  - "Detection-proof comment block lives permanently at the head of scripts/check-stubs.sh (not just in this SUMMARY), so a future maintainer sees the gate's proven detection power without having to rediscover it."

patterns-established:
  - "Pattern: any script/CI pair sharing a load-bearing, scoped command must have exactly one file that spells the command; enforced by a `grep -rln` acceptance check across scripts/.github/Makefile."

requirements-completed: [PIPE-03]

coverage:
  - id: D1
    description: "scripts/generate-stubs.sh is the single, canonical, scoped `buf generate --path mixinforprototest` invocation used by both scripts/pipeline.sh and scripts/check-stubs.sh"
    requirement: "PIPE-03"
    verification:
      - kind: other
        ref: "grep -rln -e 'buf generate' scripts .github Makefile → exactly scripts/generate-stubs.sh"
        status: pass
    human_judgment: false
  - id: D2
    description: "scripts/check-stubs.sh staleness gate detects modified stubs, orphaned stubs, and passes on a clean tree; wired into CI (`make check-stubs`) and callable identically locally"
    requirement: "PIPE-03"
    verification:
      - kind: other
        ref: "Three executed scenarios recorded verbatim below (Gate detection proof section)"
        status: pass
    human_judgment: false
  - id: D3
    description: "make check-goversion asserts mixinforproto/go.mod's pinned `go 1.24.0` directive and runs in CI's modules job"
    requirement: "PIPE-03"
    verification:
      - kind: other
        ref: "make check-goversion (pass on clean tree); deliberate go.mod edit to go 1.25.0 → non-zero exit naming go mod tidy, then restored"
        status: pass
    human_judgment: false

duration: 55min
completed: 2026-08-08
status: complete
---

# Phase 1 Plan 07: CI Staleness Gate Fix Summary

**Collapsed two independently-drifting `buf generate` spellings (pipeline.sh's scoped call vs. CI's unscoped one) into a single canonical script, rebuilt the D-22 staleness gate on an empty-output-tree design that catches orphaned stubs (not just modified ones), and proved its detection power with three executed, recorded scenarios.**

## Performance

- **Duration:** 55 min
- **Started:** 2026-08-08T13:15:00Z (approx, from first tool call)
- **Completed:** 2026-08-08T14:10:00Z
- **Tasks:** 3 completed (Task 3 required no additional file changes — see below)
- **Files modified:** 5 (2 created, 3 modified)

## Accomplishments
- `scripts/generate-stubs.sh` is now the ONE place in the repository that spells `buf generate` — confirmed via `grep -rln -e 'buf generate' scripts .github Makefile` returning exactly that one path.
- `scripts/check-stubs.sh` rebuilds the D-22/Pitfall-9 staleness gate on an empty-output-tree design (copy working-tree `proto/` into a fresh temp root, generate into it, diff against committed `mixinforproto/internal/gen`), closing WR-03's orphaned-file blind spot that the old `git archive HEAD`-based design had.
- CI's `stubs` job now runs `make check-stubs` — byte-identical to what a developer runs locally — instead of a second, independently-spelled `buf generate` invocation that contradicted `pipeline.sh`'s own documented, live-verified scoping requirement.
- `make check-goversion` asserts `mixinforproto/go.mod`'s pinned `go 1.24.0` directive and runs in CI's `modules` job, guarding against the transitive `entc/gen` → `ariga.io/atlas` → `hcl/v2` test-dependency chain that has silently bumped this pin twice already this phase.
- The gate's detection power is an executed fact: three scenarios (clean/modified/orphaned) were run against the real gate and recorded verbatim below, not merely asserted.

## Task Commits

Each task was committed atomically:

1. **Task 1: Extract the one canonical generation invocation and build the staleness gate on top of it** - `34455b8` (feat)
2. **Task 2: Reduce the CI stubs job to the shared gate and wire the version-pin assertion into CI** - `2721f74` (fix)
3. **Task 3: Prove the gate detects staleness — three deliberate breakages, three recorded verdicts** - no additional commit (see "Deviations" — the detection-proof comment block was authored directly into `scripts/check-stubs.sh` during Task 1's file creation since its content was already known from the plan's design; Task 3's work was purely running the three scenarios and confirming the pre-written comment's claims were true, which they were)

**Plan metadata:** committed alongside this SUMMARY (see below).

## Gate detection proof

All three scenarios were executed against the real gate (`make check-stubs`, which invokes `bash scripts/check-stubs.sh`) from the repo root with `export PATH="$PATH:$(go env GOPATH)/bin"` set. The tree was restored to clean after each destructive scenario.

**Scenario A — clean tree (must PASS):**
```
$ make check-stubs
bash scripts/check-stubs.sh
[check-stubs] OK: committed generated stubs match a fresh regeneration from proto/ sources.
```
Exit status: `0`

**Scenario B — modified committed stub (must FAIL, naming the file):**
```
$ echo "// deliberate-drift-probe" >> mixinforproto/internal/gen/mixinforprototestv1/tracer.pb.go
$ make check-stubs
bash scripts/check-stubs.sh
Files mixinforproto/internal/gen/mixinforprototestv1/tracer.pb.go and /tmp/tmp.3bMQRocrCw/mixinforproto/internal/gen/mixinforprototestv1/tracer.pb.go differ
::error::Committed generated stubs under mixinforproto/internal/gen are stale relative to proto/ sources (see diff above, including any 'Only in mixinforproto/internal/gen' lines naming ORPHANED files). Run scripts/pipeline.sh and commit the result.
make: *** [Makefile:83: check-stubs] Error 1
```
Exit status: `2` (non-zero); output names `tracer.pb.go`.
Restore: `git checkout -- mixinforproto/internal/gen/mixinforprototestv1/tracer.pb.go` — confirmed clean via `git status --porcelain mixinforproto/internal/gen` (empty).

**Scenario C — orphaned stub (must FAIL with an "Only in..." line — the WR-03 blind spot):**
```
$ cp mixinforproto/internal/gen/mixinforprototestv1/tracer.pb.go mixinforproto/internal/gen/mixinforprototestv1/zz_orphan_probe.pb.go
$ make check-stubs
bash scripts/check-stubs.sh
Only in mixinforproto/internal/gen/mixinforprototestv1: zz_orphan_probe.pb.go
::error::Committed generated stubs under mixinforproto/internal/gen are stale relative to proto/ sources (see diff above, including any 'Only in mixinforproto/internal/gen' lines naming ORPHANED files). Run scripts/pipeline.sh and commit the result.
make: *** [Makefile:83: check-stubs] Error 1
```
Exit status: `2` (non-zero); output contains `Only in mixinforproto/internal/gen/mixinforprototestv1: zz_orphan_probe.pb.go`, exactly the orphan-detection signature the old `git archive`-based gate could never produce.
Restore: `rm -f mixinforproto/internal/gen/mixinforprototestv1/zz_orphan_probe.pb.go` — confirmed clean via `git status --porcelain mixinforproto/internal/gen` (empty).

**Re-confirmation after all three scenarios:** `make check-stubs` run a final time, exit `0`, tree clean (`git status --porcelain` showed no residue from the scenarios).

The plan's own automated verify command for Task 3 was also run directly and produced `GATE DETECTS ORPHANS`, confirming the same result via the plan's exact scripted assertion.

## Files Created/Modified
- `scripts/generate-stubs.sh` - NEW. The single canonical, scoped `buf generate --path mixinforprototest` invocation; takes an optional target-root argument (defaults to repo root) so it can generate into a temp tree.
- `scripts/check-stubs.sh` - NEW. The staleness gate: mktemp temp root, copies working-tree `proto/`, leaves `<tmp>/mixinforproto/internal/gen` absent, calls `generate-stubs.sh`, diffs against the committed tree, exits non-zero with a GitHub-Actions `::error::` line on any difference. Contains a "Detection proof" comment block recording all three scenarios.
- `scripts/pipeline.sh` - Step 2 now delegates to `scripts/generate-stubs.sh` instead of spelling `buf generate` inline; the 12-line scoping-rationale comment moved into `generate-stubs.sh`; the file-header step list, step 2's log message, and step 2's section banner all reworded to name the script by path rather than re-describing the subcommand as literal text.
- `Makefile` - Added `check-stubs` (runs `bash scripts/check-stubs.sh`) and `check-goversion` (asserts `mixinforproto/go.mod`'s `go` directive against `MIXINFORPROTO_GO_PIN := go 1.24.0`) targets, both `##`-documented and added to `.PHONY`.
- `.github/workflows/ci.yml` - `stubs` job's inline regenerate-and-diff step replaced with `make check-stubs` (keeping the `PATH` export for the pinned `buf`/`protoc-gen-go` installs); job header comment rewritten to describe the empty-output-tree design and state that CI and local `make check-stubs` run byte-identical logic. `modules` job gains a `make check-goversion` step immediately after `make check-modules`. `standalone` job untouched (confirmed via `git diff` showing no hunk touching it).

## Decisions Made
- Copy the working tree's `proto/` into the check-stubs.sh temp root instead of `git archive HEAD`, per the plan's own live-rehearsed environment facts — this also makes the gate locally rehearsable against uncommitted proto/ edits, which `git archive HEAD` could never do.
- The "Detection proof" comment block was written directly into `scripts/check-stubs.sh` as part of Task 1's file creation (its exact content — scenario descriptions and expected outcomes — was fully specified by the plan before any scenario was run). Task 3 then executed the three scenarios against that already-written script and confirmed every claim in the comment was true; no code change was needed as a result, only the recorded proof in this SUMMARY.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Removed remaining literal "buf generate" text from scripts/pipeline.sh comments**
- **Found during:** Task 1, running the single-spelling acceptance-criterion grep (`grep -rln -e 'buf generate' scripts .github Makefile`) before Task 2 landed.
- **Issue:** The plan's action text said to reword three spots in `pipeline.sh` to reference `scripts/generate-stubs.sh` by path, but my first pass left the literal backtick-quoted phrase `` `buf generate` `` inside two of those comments (describing *what* the script does), which still matched the grep and would have made the single-spelling acceptance criterion false even after Task 2 fixed `ci.yml`.
- **Fix:** Reworded those two comment lines to say "the generation subcommand"/"an unscoped generation invocation" instead of literally spelling `buf generate`, while keeping the substance of the rationale intact.
- **Files modified:** `scripts/pipeline.sh`
- **Verification:** `grep -n 'buf generate' scripts/pipeline.sh` returns nothing; `bash -n scripts/pipeline.sh` still parses; full pipeline still runs green end-to-end.
- **Committed in:** `34455b8` (part of Task 1 commit — caught before the commit was made, not a follow-up fix)

---

**Total deviations:** 1 auto-fixed (Rule 1, caught pre-commit)
**Impact on plan:** No scope creep. Necessary for the plan's own single-spelling acceptance criterion to hold true.

## Known Stubs

None — this plan touches only scripts/Makefile/CI, no application code or UI.

## Issues Encountered

**Discovered pre-existing drift, out of scope to fix here:** Running `scripts/pipeline.sh` end-to-end for plan verification (its step 3, `buf build -o proto/mixinforprototest.binpb --as-file-descriptor-set --exclude-source-info`) produced a modified `proto/mixinforprototest.binpb` — confirmed by `git log` that this file was last regenerated at `01-04`'s commit and was never regenerated when `01-06` added `proto/mixinforprototest/v1/repeated.proto` to the corpus. This is real staleness in a generated artifact, but it is NOT the staleness gate's own defect (gap 4 is specifically about `mixinforproto/internal/gen`, not the descriptor set) and this plan's scope fence explicitly forbids modifying generated stubs. Restored `proto/mixinforprototest.binpb` to its committed state via `git checkout --` after observing the diff, and recorded it as Broken Window #4 in `.planning/WINDOWS.md` (kind: deviation) for a future plan to regenerate and commit deliberately.

## User Setup Required

None - no external service configuration required. `buf@v1.72.0` and `protoc-gen-go@v1.36.11` are already installed in this environment (confirmed via `which buf protoc-gen-go` → `/root/go/bin/...`), just not on default `PATH`; every command in this plan (and CI) exports `PATH="$PATH:$(go env GOPATH)/bin"` before invoking them.

## Next Phase Readiness

- VERIFICATION.md gap 4 / REVIEW.md CR-04 and WR-03 are closed: the staleness gate is provably functional, its detection power is a recorded fact, and there is exactly one spelling of the generation invocation in the repository.
- Broken Windows #1-3 (unsigned-integer intervals, `bytes.*` length/pattern translation, repeated scalar/enum fail-loud boundary) remain open and untouched, as scoped.
- New Broken Window #4 (`proto/mixinforprototest.binpb` staleness relative to `01-06`'s `repeated.proto` addition) is recorded and open — a future plan should run `scripts/pipeline.sh` and commit the regenerated descriptor set deliberately, in its own commit, not silently folded into unrelated work.
- Phase 2 (the entc extension) can now rely on a working, proven CI staleness gate for any future generated-code additions to `mixinforproto/internal/gen`.

---
*Phase: 01-mixinforproto-core*
*Completed: 2026-08-08*

## Self-Check: PASSED

All created/modified files (`scripts/generate-stubs.sh`, `scripts/check-stubs.sh`, `scripts/pipeline.sh`, `Makefile`, `.github/workflows/ci.yml`, this SUMMARY) confirmed present on disk. All three task commit hashes (`34455b8`, `2721f74`) and the SUMMARY commit (`c1acd45`) confirmed present in `git log --oneline --all`.
