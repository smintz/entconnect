#!/usr/bin/env bash
set -euo pipefail
#
# scripts/pipeline.sh — the canonical entconnect build pipeline (PIPE-01, D-20).
#
# Runs, in a fixed, non-negotiable order:
#   1. buf lint
#   2. scripts/generate-stubs.sh (the single canonical, scoped stub-
#      generation invocation — see that script for the scoping rationale)
#   3. buf build -o <descriptor>.binpb --as-file-descriptor-set --exclude-source-info
#      (a SEPARATE CLI invocation from step 2 — buf has no first-party plugin
#      path to descriptor-set emission, so this step can never be folded into
#      buf.gen.yaml as a plugin entry; see ARCHITECTURE.md Anti-Pattern 4).
#      02-01 renamed this output to proto/descriptorset.binpb (previously
#      named after the mixinforprototest corpus alone) — the set now
#      covers both corpora (mixinforprototest AND entconnecttest), so a
#      single-corpus name would be misleading.
#   4. go generate ./...   (per module, from MODULES)
#   5. atlas migrate diff
#
# SERIAL-ONLY. This script is not safe to run concurrently against the same
# working tree: steps 2 and 3 write generated output to fixed paths
# (mixinforproto/internal/gen/..., internal/gen/..., and
# proto/descriptorset.binpb), and two concurrent runs racing on those paths
# is undefined behavior. CI must never invoke two pipeline jobs against one
# checkout at the same time.
#
# `set -euo pipefail` (above) means the script aborts at the first FAILING
# step, so a later green step can never mask an earlier red one. A genuine
# no-op step (nothing real to act on yet — Phase 1's steps 4 and 5) is a
# SUCCESS, not a failure, but it is always reported by name, never silently
# skipped.

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

PROTO_DIR="proto"
DESCRIPTOR_OUT="proto/descriptorset.binpb"
MODULES="${MODULES:-. ./mixinforproto}"
BUF_INSTALL_HINT="go install github.com/bufbuild/buf/cmd/buf@v1.72.0"
ATLAS_INSTALL_HINT="curl -sSf https://atlasgo.sh | sh   (see https://atlasgo.io/getting-started)"

log() {
  printf '[pipeline] %s\n' "$*"
}

require_buf() {
  if ! command -v buf >/dev/null 2>&1; then
    echo "[pipeline] ERROR: buf is required for this step but is not on PATH." >&2
    echo "[pipeline]   install: ${BUF_INSTALL_HINT}" >&2
    exit 1
  fi
}

# --- Step 1: buf lint --------------------------------------------------------
log "step 1/5: buf lint"
require_buf
(cd "$PROTO_DIR" && buf lint)
log "step 1/5: OK"

# --- Step 2: scripts/generate-stubs.sh -----------------------------------
# The scoping rationale (why --path mixinforprototest is required, and the
# live-verified failure mode of an unscoped generation invocation) now
# lives in scripts/generate-stubs.sh, the single canonical, correctly-
# scoped generation invocation. This step delegates to it rather than
# spelling the generation subcommand itself, so this and scripts/
# check-stubs.sh's staleness gate can never carry two independent,
# driftable spellings of the same command again (VERIFICATION.md gap 4 /
# REVIEW.md CR-04).
log "step 2/5: scripts/generate-stubs.sh"
require_buf
"$REPO_ROOT/scripts/generate-stubs.sh" "$REPO_ROOT"
log "step 2/5: OK"

# --- Step 3: descriptor-set build (a SEPARATE invocation, never a plugin) ----
log "step 3/5: buf build -o ${DESCRIPTOR_OUT} --as-file-descriptor-set --exclude-source-info"
require_buf
buf build "$PROTO_DIR" -o "$DESCRIPTOR_OUT" --as-file-descriptor-set --exclude-source-info
log "step 3/5: OK (${DESCRIPTOR_OUT})"

# --- Step 4: go generate ./... per module ------------------------------------
log "step 4/5: go generate ./... (per module)"
step4_ran=false
for m in $MODULES; do
  if grep -rl --include='*.go' -e '^//go:generate' "$m" >/dev/null 2>&1; then
    log "  -> ${m} has //go:generate directives, running"
    (cd "$m" && go generate ./...)
    step4_ran=true
  fi
done
if [ "$step4_ran" = false ]; then
  log "step 4/5: SKIP — no //go:generate directives exist in any module yet."
fi
log "step 4/5: OK"

# --- Step 5: atlas migrate diff ----------------------------------------------
log "step 5/5: atlas migrate diff"
# Phase 1/2 ship no real application ent schema to migrate — every
# ent.Schema in the repo so far is a schema-load/tracer-test fixture
# (mixinforproto/internal/boundarytest, internal/entconnecttest/*), never
# an application schema with a migration history. Detect that condition
# first and skip honestly; only demand `atlas` on PATH once a real
# application ent/schema package exists for it to diff.
schema_dirs=$(find . -type d -name schema -path '*/ent/schema' \
  -not -path './mixinforproto/internal/*' \
  -not -path './internal/entconnecttest/*' 2>/dev/null || true)
if [ -z "$schema_dirs" ]; then
  log "step 5/5: SKIP — no application ent/schema package exists yet. A later phase (once a real ent schema is generated for the reference app) is the first phase with anything for atlas to diff."
else
  if ! command -v atlas >/dev/null 2>&1; then
    echo "[pipeline] ERROR: atlas is required for this step but is not on PATH." >&2
    echo "[pipeline]   install: ${ATLAS_INSTALL_HINT}" >&2
    exit 1
  fi
  atlas migrate diff --env local
  log "step 5/5: OK"
fi

log "pipeline complete."
