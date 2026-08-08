# Contributing to entconnect

This repository is a **two-module repo**: the root `entconnect` module and the
independently-releasable `mixinforproto` nested module (`mixinforproto/go.mod`). Read
this document before your first `git tag` — a wrong tag is effectively permanent once
pushed, because the Go module proxy caches it.

## Building and testing

Never run `go build ./...` / `go test ./...` from the repo root and assume it covers
`mixinforproto` — Go does not descend into a nested `go.mod`, so a root-only command
silently skips it entirely. Use the Makefile, which enumerates both modules explicitly:

```sh
make build            # go build ./... in each module in MODULES
make vet               # go vet ./... in each module in MODULES
make test              # go test ./... in each module in MODULES
make test-standalone    # proves mixinforproto builds/tests without go.work resolution
make check-modules      # fails if any committed go.mod declares a local-path substitution
make pipeline            # runs scripts/pipeline.sh, the canonical five-step pipeline
```

`go.work` is checked in for local development ergonomics (correct cross-module
resolution without a hand-maintained `replace` directive). It is **never** required for
an external consumer to build either module — `make test-standalone` is exactly the
proof that `mixinforproto` resolves correctly with `GOWORK=off`, in a checkout that
still contains `go.work`.

## Local-path module substitutions are a blocking review comment

**A `replace ... => ./mixinforproto` (or the reverse) line appearing anywhere in a PR
diff to a committed `go.mod` is a blocking review comment, no exceptions.** It builds
fine locally and breaks only for external consumers — exactly the failure mode that is
invisible until someone outside this repo runs `go get`. `make check-modules` is the
automated version of this review rule; run it, or trust CI's `modules` job to run it for
you, before merging anything that touches either `go.mod`.

## Release tagging (dual-tagging convention)

`mixinforproto` is a nested module and requires the `<subdir>/vX.Y.Z` tag prefix for the
Go module proxy to resolve it at all — an unprefixed tag is simply unfetchable for this
module.

- **`mixinforproto` releases** are tagged `mixinforproto/vX.Y.Z` (e.g.
  `mixinforproto/v0.1.0`).
- **The root `entconnect` module** is tagged `vX.Y.Z` (no prefix).
- **A change spanning both modules in one PR needs two tags** — one per module, each
  following its own convention above.
- **A root-module dependency bump onto a new `mixinforproto` version lands in a
  SEPARATE commit** that references the already-pushed `mixinforproto/vX.Y.Z` tag. It
  must never be a same-commit self-reference (bumping `go.mod` to a tag that doesn't
  exist yet in the same commit that would create it) — that cannot resolve, because the
  module proxy has nothing to fetch until the tag is actually pushed.

**Why this is one-way once pushed:** the Go module proxy caches whatever it first
resolves for a given `module@version`. A wrong or premature tag is not meaningfully
retractable — `go get` for that version will keep resolving to the cached (wrong)
content indefinitely, even if the tag is later deleted and recreated. Get the module
path, the tag prefix, and the commit it points at right *before* pushing, not after.

Phase 1 of this project deliberately documents this convention and its CI guard
(`make check-modules`) without pushing any tag — publication is a separate, later,
human-owned release decision, not part of scaffolding the convention itself.

## Dependency notes

**CEL import path correction (dated 2026-08-08, re-verify before relying on it):**
`./CLAUDE.md`'s Technology Stack section and `.planning/research/STACK.md` both
previously instructed importing `github.com/cel-expr/cel-go` for CEL support. This was
verified **non-functional** as of 2026-08-08: the `github.com/cel-expr/cel-go` module's
own `go.mod`, fetched directly from the Go module proxy at `v0.31.0`, still declares
`module github.com/google/cel-go` — the module-path migration is announced but has not
landed in a usable release. `go get github.com/cel-expr/cel-go/cel@v0.31.0` fails
outright with a module-path mismatch error; `go get github.com/google/cel-go/cel@v0.31.0`
succeeds. `buf.build/go/protovalidate@v1.2.0` itself transitively requires
`github.com/google/cel-go v0.28.0`, confirmed by fetching its `go.mod` directly.

**The currently-correct import path is `github.com/google/cel-go`, not
`github.com/cel-expr/cel-go`.** Phase 1 adds no direct `cel-go` dependency of either
kind — Tier 1 constraint enumeration only needs `buf.build/go/protovalidate`'s
`ResolveFieldRules`, not CEL compilation. Whichever phase first needs
`buf.build/go/protovalidate/cel.NewLibrary()` (Tier 2's CEL hook) **must re-run this
exact check** — `go get github.com/cel-expr/cel-go/cel@latest` against the live proxy —
before choosing an import path. This note is dated specifically because the migration
may land between now and then; do not carry this correction forward past its
verification date without re-checking.
