---
phase: 01-mixinforproto-core
plan: 03
subsystem: infra
tags: [go, buf, ci, makefile, go-workspaces, documentation, release-tagging]

# Dependency graph
requires:
  - phase: 01-01
    provides: "Two-module Go scaffold (go.work, root go.mod, mixinforproto/go.mod), mixinforproto's public API surface (MixinForProto, Validate, ContractVersion, MixinForProtoMessage/Field, SourceMessage/SourceField)"
  - phase: 01-02
    provides: "AsJSON option and the complete field-mapping table this plan's README documents"
provides:
  - "scripts/pipeline.sh — the canonical five-step build pipeline (buf lint, buf generate, separate descriptor-set build, go generate, atlas migrate diff), serial-only, aborting on the first real failure and printing a visible skip line for Phase 1's two genuine no-op steps"
  - "Makefile with an explicit MODULES list and build/vet/test/test-standalone/check-modules/pipeline targets — CI cannot go green on a broken mixinforproto because nothing here ever runs ./... from the repo root"
  - "committed proto/mixinforprototest.binpb — the FileDescriptorSet emitted by the pipeline's step 3"
  - ".github/workflows/ci.yml with modules/standalone/stubs jobs, buf pinned at v1.72.0, standalone GOWORK=off consumption proof, and a temp-dir staleness gate on the committed generated stubs"
  - "mixinforproto/README.md's presence/zero-collapse section (D-26), hook-ordering note (D-27), and derived-name-collision limitation, all placed above the API reference; mixinforproto/doc.go pointing at them"
  - "CONTRIBUTING.md's dual-tagging release convention (mixinforproto/vX.Y.Z vs vX.Y.Z), the check-modules blocking-review rule, and the dated (2026-08-08) CEL import-path correction (R1)"
  - "root README.md orienting a new reader to the two-module layout and the make/pipeline entry points"
affects: [01-04, 01-05]

# Actuals (#2632)
actuals:
  tokens: 9200
  tasks: 2
  commits: 3

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Explicit MODULES list (D-16) driving every cross-module Makefile target — never ./... from the repo root, which silently skips a nested go.mod"
    - "go.work stays checked in and present even in the GOWORK=off standalone CI job — the env var proves it's ignored, deleting the file would prove a different thing (ARCHITECTURE.md Anti-Pattern 5)"
    - "CI staleness gate regenerates into a git-archive'd temp copy, never the live working tree, so a fork PR can't cause the job to mutate the checkout it's also diffing against"
    - "A genuine no-op pipeline step (nothing to act on yet) prints a named skip line and continues rather than either failing or silently doing nothing"

key-files:
  created:
    - scripts/pipeline.sh
    - Makefile
    - .github/workflows/ci.yml
    - proto/mixinforprototest.binpb
    - CONTRIBUTING.md
    - README.md
    - mixinforproto/README.md
    - mixinforproto/doc.go
  modified: []

key-decisions:
  - "Descriptor-set output path: proto/mixinforprototest.binpb, sibling to proto/buf.yaml, chosen for discoverability over a nested proto/gen/ subdirectory. Not specified by CONTEXT.md/RESEARCH.md; a planner-scope naming call made at execution time."
  - "Makefile vet/test targets probe `go list ./...` before invoking `go vet`/`go test` and print a named SKIP line for a module with zero Go packages, rather than letting the command run and fail. `go vet ./...`/`go test ./...` (unlike `go build ./...`) exit 1 — not 0 — when a module matches no packages, a real Go CLI quirk verified live in this session; the root module is deliberately code-free in Phase 1 (D-18), so without this guard `make vet`/`make test` would report a false failure on a module with nothing wrong, every single run, for the rest of Phase 1."
  - "mixinforproto/README.md's usage example does not include Exclude/Override, even though mixinforproto.md §1's own example does — Exclude/Override are not part of this release's public API (they ship in 01-04) and Task 2's own read_first instruction is explicit: 'do not document symbols that do not exist.' A one-line note in the README points at where they're coming from instead of showing code that wouldn't compile against this release."
  - "CI's pipeline-script invocation (added as a Rule 2 follow-up, see Deviations) lives in the modules job rather than a fourth job, since the plan's action text enumerates exactly three named jobs (modules/standalone/stubs) and the modules job's checkout is never diffed against anything, unlike stubs — running the pipeline there and letting it write into that ephemeral checkout is harmless."

patterns-established:
  - "Pattern: every pipeline/Makefile no-op path prints a named, human-readable skip line before continuing — silence is never an acceptable substitute for 'nothing to do here yet' (T-01-15's mitigation, reusable for any future genuinely-empty pipeline step)."
  - "Pattern: documentation sections that exist specifically to be read before a specific mistake is made (the zero-collapse section) are placed ahead of reference material by construction, verified by a grep-able heading-line-number check, not by review discipline alone."

requirements-completed: [PIPE-01, PIPE-02, PIPE-03, PIPE-04, PIPE-08]

coverage:
  - id: D1
    description: "scripts/pipeline.sh runs buf lint, buf generate, a separate buf build --as-file-descriptor-set invocation, go generate ./... per module, and atlas migrate diff in that exact order, aborting on the first real failure"
    requirement: "PIPE-01"
    verification:
      - kind: other
        ref: "bash scripts/pipeline.sh (exit 0, prints steps 1/5 through 5/5 in order with explicit skip lines for steps 4 and 5)"
        status: pass
      - kind: other
        ref: "bash -n scripts/pipeline.sh (syntax check, exit 0; set -euo pipefail on line 2)"
        status: pass
    human_judgment: false
  - id: D2
    description: "The descriptor-set build (buf build -o ... --as-file-descriptor-set) is a separate CLI invocation, never a buf.gen.yaml plugin entry"
    requirement: "PIPE-01"
    verification:
      - kind: other
        ref: "grep -c -- '--as-file-descriptor-set' scripts/pipeline.sh (1); grep for descriptor_set/as-file-descriptor-set in proto/buf.gen.yaml (0 matches)"
        status: pass
    human_judgment: false
  - id: D3
    description: "A dedicated GOWORK=off CI job (standalone) builds/tests mixinforproto alone with go.work still present in the checkout"
    requirement: "PIPE-02"
    verification:
      - kind: other
        ref: "make test-standalone (exit 0, go.work confirmed present via `test -f go.work` before the standalone build/test step)"
        status: pass
    human_judgment: false
  - id: D4
    description: "CI enumerates both modules from a checked-in list and tests each explicitly; a compile error inside mixinforproto cannot pass while the root module is green"
    requirement: "PIPE-03"
    verification:
      - kind: other
        ref: "make build vet test (both `.` and `./mixinforproto` visited in output); negative proof: a temporarily-broken mixinforproto/derive.go made `make test` exit 2, reverted before commit"
        status: pass
    human_judgment: false
  - id: D5
    description: "Neither committed go.mod declares a local-path module substitution; make check-modules fails loudly if one is added"
    requirement: "PIPE-04"
    verification:
      - kind: other
        ref: "go mod edit -json {go.mod,mixinforproto/go.mod} | grep -c '\"Replace\": null' (1 for both); negative proof: a temporary replace directive in mixinforproto/go.mod made `make check-modules` exit 2, reverted before commit"
        status: pass
    human_judgment: false
  - id: D6
    description: "The nested-module release-tag convention (mixinforproto/vX.Y.Z vs vX.Y.Z, separate-commit rule for dependency bumps) is documented before any tag is pushed"
    requirement: "PIPE-04"
    verification:
      - kind: other
        ref: "CONTRIBUTING.md 'Release tagging (dual-tagging convention)' section; git tag (empty) confirms no tag was pushed during this plan"
        status: pass
    human_judgment: false
  - id: D7
    description: "The proto3 presence/zero-collapse behaviour has a dedicated top-level README section above the API reference, naming all four required elements, with a doc.go pointer"
    requirement: "PIPE-08"
    verification:
      - kind: other
        ref: "grep -n '^## ' mixinforproto/README.md (presence heading at line 53, API reference heading at line 138); mixinforproto/doc.go references the README section by name"
        status: pass
    human_judgment: false
  - id: D8
    description: ".github/workflows/ci.yml defines exactly the three named jobs (modules, standalone, stubs), buf pinned at @v1.72.0, and stubs regenerates into a temp directory rather than the working tree"
    requirement: "PIPE-01"
    verification:
      - kind: other
        ref: "grep -n '^  modules:|^  standalone:|^  stubs:' .github/workflows/ci.yml; grep -c '@v1.72.0' .github/workflows/ci.yml (2, one per job that installs buf); stubs job's diff runs against a git-archive'd $TMP copy, never the live checkout"
        status: pass
    human_judgment: false
---

# Phase 1 Plan 3: Two-Module Pipeline, CI, and Adopter Documentation Summary

**A scripted five-step canonical pipeline, a Makefile-driven CI matrix that cannot go green on a broken `mixinforproto`, a proven `GOWORK=off` standalone-consumption path, and the presence/zero-collapse + release-tagging documentation that closes out Pitfall 12 and Pitfall 4's only Phase-1-available mitigation.**

## Performance

- **Duration:** ~10 min
- **Started:** 2026-08-08T10:40:36Z (approx, first file write)
- **Completed:** 2026-08-08T10:50:59Z
- **Tasks:** 2
- **Files modified:** 8 (all new)

## Accomplishments

- `scripts/pipeline.sh` runs the five D-20 canonical steps — `buf lint`, `buf generate`, a **separate** `buf build -o ... --as-file-descriptor-set --exclude-source-info` invocation, `go generate ./...` per module, `atlas migrate diff` — aborting at the first real failure (`set -euo pipefail`, confirmed on line 2) and printing a named, visible skip line for Phase 1's two genuine no-ops (no `//go:generate` directives exist yet; no application `ent/schema` package exists yet for atlas to diff).
- `Makefile`'s `MODULES := . ./mixinforproto` drives `build`/`vet`/`test`/`test-standalone`/`check-modules`/`pipeline`, never `./...` from the repo root. Verified with two live negative proofs, both reverted before commit: a syntax-broken `mixinforproto/derive.go` made `make test` fail (proving CI cannot go green on a broken nested module), and a temporary `replace` directive in `mixinforproto/go.mod` made `make check-modules` fail (proving the module-substitution guard has real teeth).
- `.github/workflows/ci.yml` adds three jobs — `modules` (build/vet/test/check-modules, plus the pipeline script itself, added as a Rule 2 follow-up — see Deviations), `standalone` (`GOWORK=off` with `go.work` still present in the checkout, per D-17), and `stubs` (regenerates the corpus into a `git archive`'d temp directory and diffs against the committed stubs, never mutating the live checkout). `buf` is pinned at `@v1.72.0` everywhere it's installed.
- Committed `proto/mixinforprototest.binpb`, the `FileDescriptorSet` emitted by the pipeline's own step 3 — running the real pipeline script, not a stand-in.
- `mixinforproto/README.md`'s "Proto3 presence and the zero-collapse" section (heading at line 53) sits ahead of the API reference (heading at line 138), names all four required elements (the unset/zero collapse, the naive-Update silent-clear failure mode, `optional` as the contract's own remedy, and the Phase 2 field-mask fix), and is pointed to from `mixinforproto/doc.go`'s package comment. A second section documents mixin hook/policy ordering (D-27) and states plainly that Tier 1-untranslatable constraints are recorded, not enforced, this release. A third documents the derived-name collision limitation and rules out auto-renaming.
- `CONTRIBUTING.md` documents the dual-tagging release convention (`mixinforproto/vX.Y.Z` vs `vX.Y.Z`, the separate-commit rule for dependency bumps, the `make check-modules` blocking-review rule) and a dated (2026-08-08) note recording `github.com/google/cel-go` as the currently-correct CEL import path pending Phase 3 re-verification (R1). No git tag was created or pushed.

## Task Commits

1. **Task 1: Canonical pipeline script, explicit module enumeration, and the CI matrix** - `c3c8ff2` (feat)
2. **Task 2: Adopter documentation — zero-collapse, hook ordering, and the release-tag convention** - `d43a921` (docs)
3. **Follow-up fix: make CI actually run the pipeline script** - `7b3e27f` (fix) — see Deviations

**Plan metadata:** committed alongside this SUMMARY (see below).

## Files Created/Modified

- `scripts/pipeline.sh` - the canonical five-step pipeline (D-20), serial-only, self-documenting header
- `Makefile` - `MODULES`, `build`/`vet`/`test`/`test-standalone`/`check-modules`/`pipeline` targets
- `.github/workflows/ci.yml` - `modules`/`standalone`/`stubs` jobs, buf pinned `@v1.72.0`, Go 1.26
- `proto/mixinforprototest.binpb` - committed `FileDescriptorSet`, produced by the pipeline's own step 3
- `CONTRIBUTING.md` - dual-tagging convention, `check-modules` review rule, dated CEL import-path note
- `README.md` - root orientation: two-module layout, `make`/pipeline usage, pointer to `mixinforproto/README.md`
- `mixinforproto/README.md` - usage, the presence/zero-collapse section (above the API reference), hook-ordering note, collision limitation, API reference, field-mapping table
- `mixinforproto/doc.go` - package comment pointing at the README's presence section and hook-ordering fact

## Decisions Made

- **Descriptor-set output path:** `proto/mixinforprototest.binpb`, sibling to `proto/buf.yaml`, over a nested `proto/gen/` subdirectory — a naming call not specified by CONTEXT.md/RESEARCH.md.
- **Makefile `vet`/`test` guard against zero-package modules:** `go vet ./...`/`go test ./...` exit 1 (not 0, unlike `go build ./...`) when a module matches no Go packages — a real, live-verified Go CLI quirk. Since the root module is deliberately code-free in Phase 1 (D-18), `make vet`/`make test` probe `go list ./...` first and print a named `SKIP` line rather than letting the root module report a false failure on every run.
- **README usage example omits `Exclude`/`Override`** even though `mixinforproto.md` §1's own example uses them — those options ship in Plan 04, not this release, and Task 2's `read_first` is explicit that the README must not document symbols that do not exist. A one-line note in the README points at where they're coming from instead.
- **CI's pipeline-script invocation lives in the `modules` job**, not a fourth job — the plan's action text names exactly three jobs, and `modules`'s checkout is never diffed against anything (unlike `stubs`, which must stay unmutated for its own diff to mean anything).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] `make vet`/`make test` failed on the root module even though nothing was actually wrong**
- **Found during:** Task 1 (first local run of `make vet`/`make test`)
- **Issue:** The root module has zero `.go` files in Phase 1 by design (D-18: no `mixinforproto` import until Phase 2). `go build ./...` on a module with no packages prints a warning and exits 0; `go vet ./...` and `go test ./...` on the identical situation exit **1** with "no packages to vet"/"no packages to test" — a real, asymmetric Go CLI behavior, confirmed live in this session, not assumed. Without a guard, `make vet`/`make test` would report a false failure on every single invocation for the rest of Phase 1, directly contradicting this plan's own acceptance criterion ("`make build`, `make vet`, `make test` each exit 0").
- **Fix:** Both targets now probe `go list ./...` per module before invoking `go vet`/`go test`; a module with zero packages prints a named `SKIP: <module> has no Go packages yet` line and is not treated as a failure. `mixinforproto` (which has real packages) is unaffected and still runs `go vet ./...`/`go test ./...` normally.
- **Files modified:** `Makefile`
- **Verification:** `make build vet test` exits 0, `mixinforproto`'s real tests run and pass, the root module's skip is visible in output.
- **Committed in:** `c3c8ff2` (Task 1 commit)

**2. [Rule 2 - Missing Critical] CI never actually invoked `scripts/pipeline.sh`**
- **Found during:** post-Task-1 self-review, before writing this SUMMARY
- **Issue:** This plan's own `must_haves` truth for PIPE-01 states: "A single documented script runs the canonical pipeline ... **and CI runs it**." The `modules`/`standalone`/`stubs` jobs as first written replicate the pipeline's individual steps (via `make build`/`vet`/`test` and a direct `buf generate`-and-diff in `stubs`) but none of them actually executed `scripts/pipeline.sh` — the script existed and was exercised locally, but CI never ran the artifact this plan's core deliverable names.
- **Fix:** Added a `buf`/`protoc-gen-go` install step and a `bash scripts/pipeline.sh` step to the end of the `modules` job. That job's checkout is ephemeral and never diffed against anything (unlike `stubs`, whose whole point is comparing the checkout to a temp regeneration), so letting the pipeline script write into it is harmless and does not reintroduce the working-tree-mutation concern `stubs`'s temp-directory design exists to avoid.
- **Files modified:** `.github/workflows/ci.yml`
- **Verification:** `bash scripts/pipeline.sh` re-run locally after the edit still exits 0 with no working-tree drift (`git status --short` clean of unexpected changes); YAML re-validated with `python3 -c "import yaml; yaml.safe_load(...)"`.
- **Committed in:** `7b3e27f` (follow-up commit, after both task commits)

---

**Total deviations:** 2 auto-fixed (1 Rule 3 blocking, 1 Rule 2 missing-critical)
**Impact on plan:** Both were necessary to make the plan's own must-haves and acceptance criteria true rather than apparently true. No scope creep — no additional Makefile targets, no additional CI jobs beyond the three named ones, no implementation of Plan 04's Exclude/Override.

## Issues Encountered

None beyond the two deviations above. `buf`/`protoc-gen-go` were already installed via `go install` from Plans 01/02's sessions and were on `$(go env GOPATH)/bin`, not `PATH` by default — same as both prior plans noted; resolved the same way (`export PATH="$PATH:$(go env GOPATH)/bin"`) for every local verification command in this plan.

## User Setup Required

None - no external service configuration required. Note for repo maintainers: the CI workflow installs `buf@v1.72.0` and `protoc-gen-go@v1.36.11` itself via `go install` in every job that needs them — no GitHub Actions marketplace action for buf is used, keeping the third-party action surface to `actions/checkout` and `actions/setup-go` only.

## Next Phase Readiness

- Plan 04 (Exclude/Override, reserved-identifier and `oneof` gates, collected failures, in-process reproduction) can now assume a working CI matrix that will catch a broken `mixinforproto` regardless of the root module's state, and a README structure it should extend (adding an `Exclude`/`Override` subsection to the Usage section once those options actually exist) rather than restructure.
- Plan 05 (Tier 1 validation relay) inherits `CONTRIBUTING.md`'s dated CEL note as-is; it does not need CEL directly (Tier 1 is `ResolveFieldRules` translation only, no CEL compilation), so the note's re-verification obligation stays deferred to whichever future phase first needs `buf.build/go/protovalidate/cel.NewLibrary()`.
- **Carry-forward flag:** the descriptor-set path (`proto/mixinforprototest.binpb`) is a Phase 1 test-corpus artifact name, not a naming convention for the real `orderv1`/reference-app descriptor set Phase 5 will need — do not assume this exact filename generalizes.
- **Carry-forward flag (inherited, still open):** re-verify `github.com/cel-expr/cel-go`'s module-path migration status before Phase 3 planning; this plan added no `cel-go` dependency of any kind and only restated the existing dated correction in `CONTRIBUTING.md`.

---
*Phase: 01-mixinforproto-core*
*Completed: 2026-08-08*

## Self-Check: PASSED

All 8 files created by this plan verified present on disk; all three commits (`c3c8ff2`, `d43a921`, `7b3e27f`) verified present in `git log --oneline --all`.
