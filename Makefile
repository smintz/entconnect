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

.PHONY: build vet test test-standalone check-modules pipeline

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

## test-standalone: prove mixinforproto builds/tests without workspace
## resolution (PIPE-02, D-17). Runs with GOWORK=off in a checkout that
## still CONTAINS go.work — the env var is what must do the ignoring,
## not the file's absence, or this proves the wrong thing.
test-standalone:
	cd mixinforproto && GOWORK=off go build ./... && GOWORK=off go test ./...

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

## pipeline: run the canonical five-step pipeline script (PIPE-01, D-20).
pipeline:
	bash scripts/pipeline.sh
