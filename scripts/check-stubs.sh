#!/usr/bin/env bash
set -euo pipefail
#
# scripts/check-stubs.sh — the D-22 / Pitfall-9 staleness gate (PIPE-03 gap
# closure, VERIFICATION.md gap 4 / REVIEW.md CR-04+WR-03).
#
# Proves that mixinforproto/internal/gen AND internal/gen (02-01: the root
# module's own service-bearing corpus output) both match what scripts/
# generate-stubs.sh (the ONE canonical, correctly-scoped generation
# invocation) actually produces from the working tree's proto/ sources
# right now.
#
# Design:
#   1. Create an empty temp root (mktemp -d), removed on EXIT via trap.
#   2. Copy the WORKING TREE's proto/ directory into <tmp>/proto (not
#      `git archive HEAD` — copying the working tree lets a developer
#      rehearse the exact gate against uncommitted proto/ edits before
#      committing, and is what was rehearsed live at plan time).
#   3. Deliberately leave <tmp>/mixinforproto/internal/gen AND <tmp>/
#      internal/gen ABSENT, so generation writes into completely empty
#      output trees for both modules' corpora.
#   4. Run scripts/generate-stubs.sh against that temp root.
#   5. diff -rq the committed mixinforproto/internal/gen against the fresh
#      output, AND diff -rq the committed internal/gen (02-01: root
#      module's own service-bearing corpus) against its fresh output.
#
# Step 3 is what closes WR-03's blind spot: because the previous gate
# copied the COMMITTED stubs into place before regenerating (via
# `git archive HEAD | tar -x`), a stub whose source .proto message was
# renamed or removed would stay byte-identical on both sides of the diff
# and the gate would pass silently. Generating into an EMPTY tree means a
# generated file that no longer has a producing .proto source simply never
# reappears in the fresh tree, so it shows up in `diff -rq` output as:
#
#   Only in mixinforproto/internal/gen/<pkg>: <orphaned-file>.pb.go
#
# That "Only in mixinforproto/internal/gen" line is what an ORPHANED
# generated file — one no longer produced by any .proto in the corpus —
# looks like. It is the failure mode a modified-only diff cannot see.
#
# --- Detection proof (recorded 2026-08-08, Plan 01-07 gap closure) ---
# Three scenarios were executed against this exact script and all three
# verdicts matched the acceptance criteria (see 01-07-SUMMARY.md "Gate
# detection proof" for verbatim commands/output):
#   A. Clean tree                          -> `make check-stubs` exit 0
#   B. Modified committed stub              -> exit non-zero, diff names
#                                              the modified file
#   C. Orphaned stub (copy of a committed   -> exit non-zero, diff prints
#      stub under an unproduced filename)      "Only in mixinforproto/
#                                                internal/gen/..." naming it
# The working tree was restored to clean after each scenario.
# ----------------------------------------------------------------------

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

cp -R "$REPO_ROOT/proto" "$TMP/proto"

# <tmp>/mixinforproto/internal/gen and <tmp>/internal/gen intentionally do
# not exist yet — generate-stubs.sh creates both fresh via buf's plugin
# output paths.
bash "$REPO_ROOT/scripts/generate-stubs.sh" "$TMP"

fail=0

if ! diff -rq mixinforproto/internal/gen "$TMP/mixinforproto/internal/gen"; then
  echo "::error::Committed generated stubs under mixinforproto/internal/gen are stale relative to proto/ sources (see diff above, including any 'Only in mixinforproto/internal/gen' lines naming ORPHANED files). Run scripts/pipeline.sh and commit the result."
  fail=1
fi

if ! diff -rq internal/gen "$TMP/internal/gen"; then
  echo "::error::Committed generated stubs under internal/gen are stale relative to proto/ sources (see diff above, including any 'Only in internal/gen' lines naming ORPHANED files). Run scripts/pipeline.sh and commit the result."
  fail=1
fi

if [ "$fail" != "0" ]; then
  exit 1
fi

echo "[check-stubs] OK: committed generated stubs (mixinforproto/internal/gen and internal/gen) match a fresh regeneration from proto/ sources."
