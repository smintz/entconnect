# Makefile — cross-module build/test/pipeline entry points.
#
# MODULES is an EXPLICIT, checked-in list (D-16, PIPE-03). Never `./...`
# from the repo root for cross-module work: Go does not descend into a
# nested go.mod, so a root-only `./...` job goes green while
# `mixinforproto` is silently broken (PITFALLS.md Pitfall 12, trap 1).
# When a module is added, this list must be updated by hand — that's
# deliberate: an explicit list fails loudly on a forgotten addition,
# where auto-discovery would silently do the wrong thing.
MODULES := . ./mixinforproto

.PHONY: build vet test test-determinism test-standalone check-modules check-stubs check-goversion pipeline

## build: go build ./... in every module in MODULES, cd'd into each.
build:
	@set -e; for m in $(MODULES); do \
		echo "== build: $$m =="; \
		(cd $$m && go build ./...); \
	done

## vet: go vet ./... in every module in MODULES, cd'd into each.
## `go vet ./...`/`go test ./...` exit 1 (not 0) when a module has zero Go
## packages yet — a real Go quirk, not a failure. The root module is
## deliberately code-free in Phase 1 (D-18: no mixinforproto import until
## Phase 2), so we probe with `go list ./...` first and skip visibly
## rather than either failing on an empty module or silently succeeding.
vet:
	@set -e; for m in $(MODULES); do \
		echo "== vet: $$m =="; \
		if [ -z "$$(cd $$m && go list ./... 2>/dev/null)" ]; then \
			echo "   SKIP: $$m has no Go packages yet"; \
		else \
			(cd $$m && go vet ./...); \
		fi; \
	done

## test: go test ./... in every module in MODULES, cd'd into each.
test:
	@set -e; for m in $(MODULES); do \
		echo "== test: $$m =="; \
		if [ -z "$$(cd $$m && go list ./... 2>/dev/null)" ]; then \
			echo "   SKIP: $$m has no Go packages yet"; \
		else \
			(cd $$m && go test ./...); \
		fi; \
	done

## test-determinism: go test -count=5 ./... in every module in MODULES,
## cd'd into each (D-20/CRUD-06). Runs the SAME suite as `test`, five
## times per package with no isolation between runs, so order-dependence
## (an accidental map-iteration dependency, an unsorted key set) surfaces
## as a CI failure rather than a flake nobody can reproduce locally. Reuses
## the exact `go list ./...` skip-guard idiom `test` already uses, so a
## zero-package module still skips visibly instead of exiting 1.
test-determinism:
	@set -e; for m in $(MODULES); do \
		echo "== test-determinism: $$m =="; \
		if [ -z "$$(cd $$m && go list ./... 2>/dev/null)" ]; then \
			echo "   SKIP: $$m has no Go packages yet"; \
		else \
			(cd $$m && go test -count=5 ./...); \
		fi; \
	done

## test-standalone: prove mixinforproto builds/tests without workspace
## resolution (PIPE-02, D-17). Runs with GOWORK=off in a checkout that
## still CONTAINS go.work — the env var is what must do the ignoring,
## not the file's absence, or this proves the wrong thing.
test-standalone:
	cd mixinforproto && GOWORK=off go build ./... && GOWORK=off go test ./...

## test-standalone-root: prove the root module builds/tests without
## workspace resolution (02-01 checkpoint v0-1-0, D-21). The root module's
## first real dependency edge is `github.com/smintz/entconnect/mixinforproto
## v0.1.0`, resolved as an ordinary Go module dependency via the published
## tag — GOWORK=off here is what proves that edge is real (resolves through
## the module proxy) and not merely workspace-visible (resolves only
## because go.work's `use ./mixinforproto` papers over a missing
## dependency). Same discipline as test-standalone, applied to the root
## module, in a checkout that still CONTAINS go.work.
test-standalone-root:
	GOWORK=off go build ./... && GOWORK=off go vet ./... && GOWORK=off go test ./...

## check-modules: fail if any committed go.mod declares a local-path
## module substitution (PIPE-04). `go mod edit -json` reports a null
## "Replace" field when no replace directive is present; anything else
## is a blocking finding — a replace directive breaks external
## consumers even though it builds fine locally (ARCHITECTURE.md
## Anti-Pattern 5).
check-modules:
	@fail=0; \
	for m in $(MODULES); do \
		gomod="$$m/go.mod"; \
		count=$$(go mod edit -json "$$gomod" | grep -c '"Replace": null' || true); \
		if [ "$$count" != "1" ]; then \
			echo "FAIL: $$gomod declares a local-path module substitution (Replace); this must never be committed (PIPE-04)"; \
			fail=1; \
		else \
			echo "OK: $$gomod has no Replace directive"; \
		fi; \
	done; \
	exit $$fail

## check-stubs: the D-22 / Pitfall-9 staleness gate (PIPE-03). Regenerates
## the proto corpus from the working tree into an empty temp root via
## scripts/generate-stubs.sh and diffs it against the committed
## mixinforproto/internal/gen. Detects modified AND orphaned generated
## files (see scripts/check-stubs.sh for the orphan-detection rationale).
## This is the exact command CI's `stubs` job runs — a red CI job is
## reproducible locally with this one target.
check-stubs:
	bash scripts/check-stubs.sh

## check-goversion: asserts mixinforproto/go.mod still declares the pinned
## `go` directive (PIPE-03). Load-bearing: a bare `go mod tidy` silently
## raises this pin through a transitive TEST-ONLY dependency chain
## (entc/gen -> ariga.io/atlas -> hcl/v2 -> rogpeppe/go-internal), observed
## during 01-01's execution and, until this target existed, recorded only
## in SUMMARY prose rather than enforced. To deliberately raise the pin,
## update MIXINFORPROTO_GO_PIN below in the same commit as go.mod.
MIXINFORPROTO_GO_PIN := go 1.24.0
check-goversion:
	@actual=$$(grep -m1 '^go ' mixinforproto/go.mod); \
	if [ "$$actual" != "$(MIXINFORPROTO_GO_PIN)" ]; then \
		echo "FAIL: mixinforproto/go.mod declares '$$actual', expected '$(MIXINFORPROTO_GO_PIN)'."; \
		echo "       This pin is load-bearing: a bare 'go mod tidy' silently raises it via"; \
		echo "       the transitive TEST-only dependency chain entc/gen -> ariga.io/atlas ->"; \
		echo "       hcl/v2 -> rogpeppe/go-internal. If the pin was deliberately raised,"; \
		echo "       update MIXINFORPROTO_GO_PIN in the Makefile in the same commit."; \
		exit 1; \
	fi; \
	echo "OK: mixinforproto/go.mod declares '$$actual'"

## pipeline: run the canonical five-step pipeline script (PIPE-01, D-20).
pipeline:
	bash scripts/pipeline.sh
