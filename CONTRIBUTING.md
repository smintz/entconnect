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

## Test corpus coverage

`mixinforproto/corpus_test.go` guards the `mixinforprototest.v1` proto corpus
(`proto/mixinforprototest/v1/*.proto`) against a specific, previously-real failure mode:
Phase 1 shipped five green waves — a full golden-file corpus, `go test -race -count=5`
— and still failed goal verification with three mapping bugs (`01-VERIFICATION.md`). All
three shared one root cause: **the corpus avoided the exact shape that triggers the
defect.** A repeated scalar field was never exercised, so `classify()`'s missing
`IsList()` branch was invisible. Every `StringFormat*` message carried exactly one field
by convention, so a whole-message validator poisoned by a sibling field's violation was
invisible. No `optional string` + `required` fixture existed, so an unreachable branch in
`classifyRequired()` was invisible. A fully green suite told us nothing, because each
defect's own precondition was exactly what the corpus happened to avoid.

Three guards in `mixinforproto/corpus_test.go` make that kind of absence a **detectable,
failing condition** instead of a silent one:

- `TestCorpusExercisesEveryFieldClass` — every value `classify()` can return must be
  produced by at least one corpus field. Adding a `fieldClass` without a fixture that
  reaches it fails the suite.
- `TestCorpusExercisesEveryRequiredResult` — every value `classifyRequired()` can return
  must be produced by at least one corpus field, under the real
  `(optional, required, hasNotEmpty)` triple the builders pass. `requiredExactPresence`
  is additionally required to be witnessed separately by a `hasNotEmpty=true`
  (string/bytes) field and a `hasNotEmpty=false` field — a plain per-outcome check would
  have stayed green under the original defect, since a non-string field alone satisfies
  the outcome regardless of branch order.
- `TestCorpusMessagesHaveRecordedCoverage` — every message declared in the
  `mixinforprototest.v1` package must have a recorded entry in `corpusCoverage`, keyed by
  full proto message name, naming either a golden fixture (`golden:<name>`, verified to
  exist under `mixinforproto/testdata/`) or a named Go test (`test:<TestName>`). Checked
  in both directions: an unlisted message and a stale entry (naming a message that no
  longer exists) both fail.

**Adding a corpus message:**

1. Write the `.proto` file under `proto/mixinforprototest/v1/`.
2. Regenerate the committed stub with `scripts/generate-stubs.sh` (or run the full
   pipeline via `bash scripts/pipeline.sh`, which also refreshes
   `proto/mixinforprototest.binpb`).
3. Add a golden fixture (`goldie.New(t).AssertJson(...)`, see `fieldmap_test.go`'s
   `assertGolden` helper) or a named test asserting the message's behavior.
4. Record the claim in `mixinforproto/corpus_test.go`'s `corpusCoverage` map.

**A guard failure is closed by adding coverage — a fixture plus a recorded claim — never
by deleting or relaxing the assertion.** If `TestCorpusExercisesEveryFieldClass` or
`TestCorpusExercisesEveryRequiredResult` reports a new classification or outcome with no
covering fixture, add a `.proto` fixture that reaches it. "No coverage" is a real answer
only when it is written down in `corpusCoverage`.

`make check-stubs` (added by `01-07-PLAN.md`) is the companion staleness gate: it catches
a committed generated stub (`mixinforproto/internal/gen`) that has drifted from its
`.proto` source, including orphaned files. Run it after any corpus change — the two
mechanisms are complementary (one checks the stub is fresh, the other checks the
behavior is claimed and tested) and are meant to be run together.

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
