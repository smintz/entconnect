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

.PHONY: build vet test test-determinism test-standalone check-modules check-stubs check-goversion check-dep-parity check-single-validationerror-site pipeline

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

## check-dep-parity: fail if the root module and mixinforproto resolve
## DIFFERENT versions of any of D-15's explicit five modules (VAL-11).
## This set is deliberately broader than VAL-11's literal "protovalidate/
## cel-go" wording: a protobuf-runtime or ent skew shifts a storage-layer
## validation verdict away from the boundary's just as easily as a
## protovalidate skew does, and a gate that says nothing about them gives
## false assurance. It is just as deliberately NOT "every shared
## dependency" — that fires on incidental transitive skew a maintainer
## can't act on, and a gate that cries wolf gets disabled, taking the
## real signal with it. Adding a module to this list is therefore a
## visible, reviewable Makefile diff, never silent.
##
## GOWORK=off on every `go list -m` call is load-bearing, not incidental:
## with go.work's workspace unification in effect, both modules trivially
## "agree" because they are resolved as one build — exactly the condition
## this gate exists to catch (D-15). This target must keep running inside
## the existing GOWORK=off standalone CI job, in a checkout that still
## CONTAINS go.work, matching test-standalone/test-standalone-root's own
## discipline above.
##
## A module ABSENT from either go.mod's resolved build list is a FAILURE,
## never treated as agreement — `go list -m` prints nothing and exits
## non-zero for an unresolvable module, so an empty result on either side
## is distinguished in the failure message from a genuine version
## mismatch (D-15: "a gate that passes because it found nothing to
## compare is exactly the false assurance this exists to prevent").
DEP_PARITY_MODULES := \
	buf.build/go/protovalidate \
	buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go \
	github.com/google/cel-go \
	google.golang.org/protobuf \
	entgo.io/ent
check-dep-parity:
	@fail=0; \
	for mod in $(DEP_PARITY_MODULES); do \
		v_root=$$(GOWORK=off go list -m -f '{{.Version}}' "$$mod" 2>/dev/null); \
		v_mixin=$$(cd mixinforproto && GOWORK=off go list -m -f '{{.Version}}' "$$mod" 2>/dev/null); \
		if [ -z "$$v_root" ] || [ -z "$$v_mixin" ]; then \
			echo "FAIL: $$mod not found in one of the two go.mod files (root=$${v_root:-<not found>}, mixinforproto=$${v_mixin:-<not found>}) -- absence is not agreement (D-15)"; \
			fail=1; \
		elif [ "$$v_root" != "$$v_mixin" ]; then \
			echo "FAIL: $$mod versions differ -- root=$$v_root, mixinforproto=$$v_mixin"; \
			fail=1; \
		else \
			echo "OK: $$mod: $$v_root (both modules agree)"; \
		fi; \
	done; \
	exit $$fail

## check-single-validationerror-site: fail if any production (non-test) .go
## file OTHER than mixinforproto/violation.go constructs a
## *protovalidate.ValidationError{ (VAL-07, WR-07). D-01 makes this exact Go
## type mixinforproto's published, adopter-facing error surface, and D-07
## consequence 3 requires error construction to be single-pathed even
## though evaluation is not (protovalidate's own evaluator for the
## standard-rule half, hooks.go's local cel.Env for the residual
## custom-CEL half). A second construction site is precisely how the two
## evaluation paths stop agreeing without anyone noticing -- the
## CR-01/CR-02/CR-03 bug class this gate exists to keep shut.
##
## Two filters, neither optional:
##   - _test.go files are excluded entirely. Without this the gate is red
##     on day one: runtime/errormap_test.go legitimately constructs a
##     ValidationError twice to exercise runtime.MapError, and forbidding
##     that would forbid testing the error-mapping path at all.
##   - `//`-prefixed comment LINES are stripped before the literal is
##     searched for. Without this the gate is self-invalidating:
##     mixinforproto/violation_test.go's own doc comment mentions the
##     literal in prose, and any future production-file doc comment naming
##     the type would turn a documentation improvement into a red build.
##
## Generated trees are excluded by path, so a future generator emitting the
## type cannot make this gate mysteriously red without a human decision:
##   - ./internal/gen/ and ./mixinforproto/internal/gen/ (buf-generated stubs)
##   - any directory literally named `ent` anywhere in the tree (entc's own
##     generated-output convention, e.g. internal/entconnecttest/*/ent)
##
## Walks the whole repo from the root, not per-module (MODULES) -- the
## invariant is whole-repo by definition, not module-scoped.
check-single-validationerror-site:
	@fail=0; \
	found=""; \
	files=$$(find . -name '*.go' \
		! -name '*_test.go' \
		! -path './internal/gen/*' \
		! -path './mixinforproto/internal/gen/*' \
		! -path '*/ent/*' \
		! -path './.git/*' \
		| sort); \
	for f in $$files; do \
		matches=$$(grep -n 'protovalidate\.ValidationError{' "$$f" 2>/dev/null | grep -vE ':[[:space:]]*//' || true); \
		if [ -n "$$matches" ]; then \
			echo "$$matches" | sed "s|^|SITE: $$f:|"; \
			found="$$found $$f"; \
		fi; \
	done; \
	numfiles=$$(echo $$found | wc -w); \
	if [ "$$numfiles" != "1" ] || [ "$$found" != " ./mixinforproto/violation.go" ]; then \
		echo "FAIL: protovalidate.ValidationError{ must be constructed at exactly one production site, ./mixinforproto/violation.go -- found $$numfiles site(s):$$found"; \
		echo "      D-01 makes *protovalidate.ValidationError mixinforproto's published, adopter-facing error type,"; \
		echo "      and D-07 consequence 3 requires error construction to be single-pathed even though evaluation is"; \
		echo "      not -- a second construction site is how the two evaluation paths stop agreeing silently."; \
		echo "      Route through mixinforproto/violation.go's newValidationError instead."; \
		fail=1; \
	else \
		echo "OK: protovalidate.ValidationError{ constructed at exactly one production site: ./mixinforproto/violation.go"; \
	fi; \
	exit $$fail

## pipeline: run the canonical five-step pipeline script (PIPE-01, D-20).
pipeline:
	bash scripts/pipeline.sh
