---
phase: 03-validation-fidelity
plan: 07
subsystem: infra
tags: [makefile, ci, pipeline, validation, protovalidate, gap-closure]

# Dependency graph
requires:
  - phase: 03-validation-fidelity
    provides: "03-03/03-04's mixinforproto/violation.go newValidationError single-construction-site design (D-01, D-07 consequence 3) and check-dep-parity's three-way wiring precedent (VAL-11, D-15)"
provides:
  - "check-single-validationerror-site: a runnable Makefile gate enforcing VAL-07's single-ValidationError-construction-site invariant across the whole repo"
  - "The gate wired into scripts/pipeline.sh (step 6/6) and CI's modules job, matching check-dep-parity's three-way reachability pattern"
  - "mixinforproto/violation_test.go's doc comment corrected to name the real enforcement mechanism instead of a nonexistent one"
affects: [validation-fidelity, ci, pipeline]

# Actuals (#2632)
actuals:
  tokens: 2773
  tasks: 2
  commits: 2

# Tech tracking
tech-stack:
  added: []
  patterns: ["source-text repo-wide grep gate with _test.go exclusion + comment-line stripping + generated-tree path exclusion, matching check-dep-parity's fail=0/exit $$fail accumulator shape"]

key-files:
  created: []
  modified:
    - Makefile
    - scripts/pipeline.sh
    - .github/workflows/ci.yml
    - mixinforproto/violation_test.go

key-decisions:
  - "check-single-validationerror-site walks the whole repo from root (not per-MODULES), matching the invariant's whole-repo scope"
  - "Gate wired into CI's modules job, not standalone -- unlike check-dep-parity it has no GOWORK=off dependency, so standalone (the dedicated GOWORK=off proof job) would misleadingly imply one"
  - "pipeline.sh's step-count labels (1/5..5/5) updated to 1/6..6/6 to keep the header's numbered list and body echoes consistent with the new 6-step total; the five original steps kept their fixed order and step numbers, only the denominator changed"

patterns-established:
  - "A future Makefile gate that needs to tolerate legitimate test fixtures and doc-comment mentions of a literal should reuse this file's two-filter shape (name-based _test.go exclusion + line-content comment stripping via grep -vE ':[[:space:]]*//')"

requirements-completed: [VAL-06, VAL-07, PIPE-05]

coverage:
  - id: D1
    description: "check-single-validationerror-site Makefile target enforces VAL-07's single-construction-site invariant, tolerating runtime/errormap_test.go's two legitimate fixtures and violation_test.go's own doc comment"
    requirement: "VAL-07"
    verification:
      - kind: other
        ref: "make check-single-validationerror-site (working tree, exit 0) -- see Verification Rehearsal section below for scratch-copy failure/pass runs"
        status: pass
    human_judgment: false
  - id: D2
    description: "Gate wired three ways (make target, scripts/pipeline.sh step 6/6, CI modules job) so the three call sites cannot drift apart"
    requirement: "PIPE-05"
    verification:
      - kind: other
        ref: "bash -n scripts/pipeline.sh; grep -c check-single-validationerror-site scripts/pipeline.sh .github/workflows/ci.yml"
        status: pass
    human_judgment: false
  - id: D3
    description: "mixinforproto/violation_test.go's doc comment corrected to name the real gate (make check-single-validationerror-site) instead of a nonexistent Makefile grep-based check"
    requirement: "VAL-06"
    verification:
      - kind: unit
        ref: "mixinforproto/violation_test.go#TestNewValidationError_SingleBuilderSite"
        status: pass
    human_judgment: false

duration: 7min
completed: 2026-08-15
status: complete
---

# Phase 3 Plan 07: check-single-validationerror-site gate Summary

**Closed WR-07 by turning a comment that lied ("Makefile's grep-based check ... does not exist") into a real, three-way-wired Makefile gate enforcing VAL-07's single-ValidationError-construction-site invariant.**

## Performance

- **Duration:** 7 min
- **Started:** 2026-08-15T11:21:07Z
- **Completed:** 2026-08-15T11:28:27Z
- **Tasks:** 2
- **Files modified:** 4 (Makefile, scripts/pipeline.sh, .github/workflows/ci.yml, mixinforproto/violation_test.go)

## Accomplishments

- New `check-single-validationerror-site` Makefile target: walks the whole repo from root, excludes `_test.go` files and `//`-prefixed comment lines, and excludes generated trees (`internal/gen/`, `mixinforproto/internal/gen/`, any directory literally named `ent`), asserting `protovalidate.ValidationError{` is constructed at exactly one production site, `mixinforproto/violation.go`.
- Gate wired into `scripts/pipeline.sh` as step 6/6 (after the original five fixed-order steps, never inserted among them) and into CI's `modules` job (the correct host — no `GOWORK=off` dependency, unlike `check-dep-parity`).
- `mixinforproto/violation_test.go`'s `TestNewValidationError_SingleBuilderSite` doc comment now names the real enforcement mechanism (`make check-single-validationerror-site`) instead of a nonexistent "Makefile's grep-based check" — comment-only change, test body untouched.
- Both gate directions rehearsed in a scratch copy (never the working tree) — see below.

## Task Commits

Each task was committed atomically:

1. **Task 1: The check-single-validationerror-site Makefile target, with both directions rehearsed** - `39bb1eb` (feat)
2. **Task 2: Wire the gate into pipeline and CI, and correct the comment that named a target that did not exist** - `972bd1d` (feat)

## Verification Rehearsal (Phase 1 D-25 discipline)

All four runs below were executed in a `mktemp -d` scratch copy of the repository (`.git` stripped), never against the working tree. The scratch copy was deleted after the rehearsal; `git status --short` on the real working tree showed no scratch artifacts afterward.

**1. Clean working tree (before any planting):**
```
SITE: ./mixinforproto/violation.go:64:	return &protovalidate.ValidationError{Violations: out}
OK: protovalidate.ValidationError{ constructed at exactly one production site: ./mixinforproto/violation.go
EXIT: 0
```

**2. Scratch copy with a second production construction site planted in `runtime/errormap.go`** (a `plantedSecondSite()` function returning `&protovalidate.ValidationError{Violations: nil}`):
```
SITE: ./mixinforproto/violation.go:64:	return &protovalidate.ValidationError{Violations: out}
SITE: ./runtime/errormap.go:81:	return &protovalidate.ValidationError{Violations: nil}
FAIL: protovalidate.ValidationError{ must be constructed at exactly one production site, ./mixinforproto/violation.go -- found 2 site(s): ./mixinforproto/violation.go ./runtime/errormap.go
      D-01 makes *protovalidate.ValidationError mixinforproto's published, adopter-facing error type,
      and D-07 consequence 3 requires error construction to be single-pathed even though evaluation is
      not -- a second construction site is how the two evaluation paths stop agreeing silently.
      Route through mixinforproto/violation.go's newValidationError instead.
make: *** [Makefile:214: check-single-validationerror-site] Error 1
EXIT: 2
```
Named the offending file (`./runtime/errormap.go`) by path and line, as required.

**3. Same scratch copy after removing the planted site:**
```
SITE: ./mixinforproto/violation.go:64:	return &protovalidate.ValidationError{Violations: out}
OK: protovalidate.ValidationError{ constructed at exactly one production site: ./mixinforproto/violation.go
EXIT: 0
```

**4. Same scratch copy with a `//` comment mentioning the literal added to `runtime/errormap.go`** (a production, non-test file) — proves the comment-stripping filter works, not merely that it was written:
```
SITE: ./mixinforproto/violation.go:64:	return &protovalidate.ValidationError{Violations: out}
OK: protovalidate.ValidationError{ constructed at exactly one production site: ./mixinforproto/violation.go
EXIT: 0
```

## CI Job Placement Decision

The gate step (`Verify VAL-07's single ValidationError construction site`) was added to CI's `modules` job, right after `check-goversion`. Unlike `check-dep-parity`, this gate has no `GOWORK=off` requirement — it is a pure source-text invariant with no module resolution at all, so wiring it into the `standalone` job (which exists specifically to prove standalone, `GOWORK=off` consumption) would falsely imply a workspace-resolution dependency the gate does not have. The `modules` job, which already runs `make build`/`make vet`/`make test`, is the natural and only host. `grep -c 'check-single-validationerror-site' .github/workflows/ci.yml` returns exactly 1, confirming it is not duplicated across jobs.

## Files Created/Modified

- `Makefile` - New `check-single-validationerror-site` target (`.PHONY` entry appended before `pipeline`, keeping existing order) with a `## `-prefixed doc block stating both filter reasons and each generated-tree exclusion by path
- `scripts/pipeline.sh` - New step 6/6 invoking `make check-single-validationerror-site`; header's numbered step list extended to 6 entries; all step-count labels (`1/5`..`5/5`) updated to `1/6`..`6/6` so the file's own documentation matches what it runs — the five original steps kept their fixed order and numbers
- `.github/workflows/ci.yml` - New step in the `modules` job running `make check-single-validationerror-site`, with an inline comment recording the `standalone`-job rejection rationale
- `mixinforproto/violation_test.go` - `TestNewValidationError_SingleBuilderSite`'s doc comment corrected to name the real target and command; test body byte-identical (`git diff` confirms comment-only change)

## Decisions Made

- The gate walks the repo from root rather than per-`MODULES` entry, since the invariant is whole-repo by definition (matches the plan's explicit instruction).
- Comment-stripping implemented via `grep -n <pattern> | grep -vE ':[[:space:]]*//'` — filters lines where the content immediately following the `grep -n` line-number prefix is (optional whitespace then) `//`, i.e. whole-line comments only. Inline trailing comments were out of scope per the plan's own wording ("`//`-prefixed comment lines").
- `pipeline.sh`'s `N/5` step-count labels were updated to `N/6` across all five original steps' log lines (not just the new step), since leaving them at `/5` while a 6th step exists afterward would itself be documentation that doesn't match what runs — this reading of "update the header comment block's numbered list so the file's own documentation matches what it runs" was applied consistently to both the header list and the inline `log` calls.

## Deviations from Plan

None - plan executed exactly as written. No Go production code and no `go.mod` was touched, matching the plan's own success criterion.

## Issues Encountered

- `go` was not on `PATH` in this sandboxed environment by default (`/usr/local/go` symlink absent; only `/usr/local/go1.24.7` and `/usr/local/go1.25.1` exist). Resolved by using `/usr/local/go1.24.7/bin` (matching `mixinforproto/go.mod`'s pinned `toolchain go1.24.7`) for the two `go test`/`make test` verification runs. No repository files were changed to work around this — it is a rehearsal-environment concern only, not recorded as a deviation.

## Next Phase Readiness

- WR-07 closed: `03-VERIFICATION.md` truth #8 ("VAL-07's single-`ValidationError`-construction-site invariant is enforced by CI") is now demonstrable by running `make check-single-validationerror-site`.
- No comment in the repository claims an enforcement mechanism the repository lacks.
- `03-06-PLAN.md` (sibling wave-5 plan) shares no file with this plan and is unaffected.

---
*Phase: 03-validation-fidelity*
*Completed: 2026-08-15*

## Self-Check: PASSED

All files (Makefile, scripts/pipeline.sh, .github/workflows/ci.yml, mixinforproto/violation_test.go, this SUMMARY.md) confirmed present. All commits (39bb1eb, 972bd1d, 04bb2a5) confirmed in git log.
