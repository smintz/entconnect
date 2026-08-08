# Phase 1: MixinForProto Core - Research

**Researched:** 2026-08-08
**Domain:** Go runtime `ent.Mixin` implementation deriving `[]ent.Field` from `protoreflect` descriptors; ent/protobuf/protovalidate internals; two-module Go repo scaffolding
**Confidence:** MEDIUM-HIGH — core protoreflect/ent/field APIs and the nested-module toolchain verified this session via direct `go get`/`go list -m`/proxy.golang.org queries and pkg.go.dev source fetches, not training recall. One prior-research assumption (STACK.md's cel-go import path) is verified **wrong at the pinned version** — see Load-Bearing Corrections below, first.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

**Annotation Contract (ANNO-01…ANNO-04)**

- **D-01:** The annotation struct types live **in `mixinforproto`**, not in the root module. This is the one part of `mixinforproto`'s public API a different module is expected to import and decode. It does not violate module isolation — that constraint bounds what `mixinforproto` *imports*, not who imports it. — Reversibility: costly.

- **D-02:** Two annotation types, both plain serializable structs (no closures, no `cel.Program`, nothing that cannot survive `map[string]any` + `mapstructure`):
  - **Schema-level `SourceMessage`** — proto message full name, the **complete inventory** of proto fields present in the descriptor (name + number), plus `Excluded []string` and `Overridden []string`. The full inventory is load-bearing: without it a later drift check cannot distinguish "deliberately excluded" from "silently forgotten".
  - **Field-level `SourceField`** — source field name and number, derivation kind (scalar / enum / WKT / map-as-JSON / message-as-JSON / override), the protovalidate constraint IDs translated into Tier 1 builders, and the residual (untranslated) constraint IDs plus a stable fingerprint of their CEL expression *strings*.
  — Reversibility: one-way.

- **D-03: ANNO-04 versioning:** a single integer `ContractVersion` constant, embedded as a field on *both* annotation structs, bumped **only** on a breaking layout change. Between bumps the policy is **additive-only** — new fields may be added, existing fields are never renamed or repurposed, and decoding tolerates unknown keys. A consumer reading a `ContractVersion` higher than the one it was compiled against must fail with a named mismatch error, not silently decode a partial struct. — Reversibility: costly.

- **D-04:** Annotation keys are **exported string constants** in `mixinforproto` (`MixinForProtoMessage`, `MixinForProtoField`), mirroring `entproto`'s `MessageAnnotation = "ProtoMessage"` precedent.

- **D-05:** `Override(name, f)` records the *name* in `SourceMessage.Overridden` only. The replacement `ent.Field` itself needs no annotation channel.

**Failure Surface & Debuggability (MIX-11, MIX-12)**

- **D-06:** Split the implementation into a **pure `derive` core returning `(*derivation, error)`** and a thin `ent.Mixin` wrapper whose `Fields()` calls it and panics on error. — Reversibility: costly.

- **D-07: MIX-12's in-process debug entry point is an exported `Validate[M proto.Message](opts ...Option) error`** in the same package, running the identical `derive` core and returning the structured error. No separate `doctor` binary in this phase.

- **D-08:** **Every panic message's first line must be self-sufficient**, because `entc/load` runs schema loading in a subprocess whose panic output is routinely truncated to one line. Format: `mixinforproto: <ProtoMessageName>.<field>: <what went wrong> — <the fix>`.

- **D-09:** Failures are **collected and reported together**, not fired at the first offense.

**Derived-Name Collisions (Pitfall 2)**

- **D-10:** At derivation time, check every derived field name against ent's known reserved identifiers (`id`, `type`, `label`, `edge`, `where`, `config`, `client`, and the rest of the catalog-known set) and **panic at schema load** with the `Exclude`/`Override` remedy. Collisions against fields hand-declared in the *same schema's* `Fields()` or contributed by another mixin are outside the mixin's reach and are **documented loudly**, left to surface as the Go compiler's `redeclared in this block` error. Do not attempt to auto-rename.

**Tier 1 Translation Boundary (VAL-01, VAL-02, VAL-03)**

- **D-11:** Tier 2 does not exist until Phase 3, so **constraints Tier 1 cannot translate are recorded, not enforced and not rejected**.

- **D-12: Open/closed interval adjustment (VAL-02):** integer types adjust exactly — `gt: n` → `Min(n+1)`, `lt: n` → `Max(n-1)`. Floating-point `gt`/`lt` are **treated as residual**, not widened to `gte`/`lte`.

- **D-13: String format validators (VAL-01's `uuid`/`email`/`hostname`/`uri`/`ip`):** where protovalidate's own semantics *are* a documented regular expression, emit `Match(regexp.MustCompile(...))`. Where protovalidate implements the check procedurally, emit `field.String().Validate(fn)` delegating to protovalidate's own predicate. — Reversibility: reversible, per-format.

- **D-14:** `enum.defined_only` plus declared values is satisfied by `field.Enum` construction itself and is not separately recorded as residual.

**Two-Module Scaffold & Pipeline (MIX-14, PIPE-01…PIPE-04)**

- **D-15: `go.work` is checked in**, listing the root module and `mixinforproto/`. No `replace` directive ever lands in a committed `go.mod` (PIPE-04).

- **D-16: CI enumerates modules explicitly** via a checked-in list (`MODULES := . ./mixinforproto`). Never `./...` from the repo root.

- **D-17: A dedicated `GOWORK=off` CI job builds and tests `mixinforproto` alone** (PIPE-02).

- **D-18: The root `entconnect` module does not import `mixinforproto` in Phase 1.** Phase 1 creates the root `go.mod` and the CI/pipeline scaffold, but the annotation-decoding dependency edge lands in Phase 2. — Reversibility: reversible.

- **D-19: Release tagging:** `mixinforproto` releases under `mixinforproto/vX.Y.Z`; the root module under `vX.Y.Z`. — Reversibility: one-way once a tag is published.

- **D-20: Pipeline script (PIPE-01)** runs, in order: `buf lint`, `buf generate`, the **separate** `buf build -o <path> --as-file-descriptor-set --exclude-source-info` invocation, `go generate ./...`, `atlas migrate diff`. In Phase 1 the `go generate` and `atlas` steps have nothing real to act on yet; the script exists and is exercised.

**Test Corpus & Determinism (MIX-13, groundwork for PIPE-05)**

- **D-21:** Corpus protos are synthetic — small `testdata` proto files, one per mapping-rule class.

- **D-22:** Generated Go stubs for the corpus are committed, so `go test ./mixinforproto/...` requires no `buf` on the machine. A separate CI job regenerates and diffs them.

- **D-23: Golden assertion shape:** marshal each derived field's `Descriptor()` into a stable, key-sorted document and compare via `goldie` (`-update` regenerates). Comparing `ent.Field` values structurally is not viable — the interesting state is behind the descriptor and some of it is closures.

- **D-24: Determinism is a hard rule from the first line of derivation code:** never range a `map` directly into ordered output. Iterate `protoreflect`'s field list in declaration order, or sort keys explicitly. Golden tests run with `-count=5` in CI.

- **D-25:** Every panic path gets a test proving it fires at schema load, not at first mutation, and each test asserts the *specific message content*.

**Documentation (PIPE-08)**

- **D-26:** The proto3 presence/zero-collapse behavior gets a **dedicated top-level README section placed above the API reference**, plus a pointer from `doc.go`.

- **D-27:** Document the hook/policy ordering fact ahead of the hook actually existing: a mixin's `Hooks()` run **before** any hook or privacy policy the schema author declares. Phase 1 ships no hook, but the docs should not imply otherwise.

### Claude's Discretion

Because this ran in `--auto`, every decision above is Claude's discretion by construction. The ones most worth a human's second look before planning, in order:
1. **D-13** (format-validator translation) — trades ecosystem introspectability for verdict identity; VAL-01's wording arguably favors the other side of that trade.
2. **D-03** (integer `ContractVersion`) — STATE.md flags ANNO-04 as having no direct precedent; least-evidenced decision here.
3. **D-12** (floats as residual) — defensible, but means Phase 1 ships less Tier 1 coverage than VAL-02's wording suggests.
4. **D-18** (no root→mixinforproto edge in Phase 1) — a scoping call about what "scaffold" means, not a technical constraint.

### Deferred Ideas (OUT OF SCOPE)

- **`StrictPresence` option** (MIX2-01) — v2.
- **`debug_redact` → `field.Sensitive()` inference** (MIX2-02) — v2; a decision checkpoint is due by end of Phase 1 per STATE.md's Blockers section, but implementation stays out of scope.
- **`google.protobuf.Duration` → `field.Int64` nanoseconds via option** (MIX2-04) — v2. Phase 1 skips `Duration`.
- **Custom proto field options as an annotation channel** — explicitly Out of Scope. `Override` is the answer until real demand appears. Do not reopen.
- **`OnUpdateWithFetch` message-rule enforcement** (MIX2-03) — v2, Out of Scope for v1 by name.
- **entproto migration document** (DOC2-01) — v2.
- **A `mixinforproto`/`entconnect doctor` CLI** — D-07 chose an exported `Validate[M]` function instead.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| MIX-01 | `MixinForProto[*orderv1.Order]()` materializes ent fields, no descriptor file, no string names | §Descriptor Walking Mechanics — `(*new(M)).ProtoReflect().Descriptor()` confirmed sound; §Code Examples has the skeleton |
| MIX-02 | Maps proto scalar types to ent field builders | §Ent Field Builder Catalog — full verified constructor/method list |
| MIX-03 | Maps `enum` fields to `field.Enum` with declared values | §Ent Field Builder Catalog — `field.Enum(name).Values(...)`; §Descriptor Walking — `EnumDescriptor.Values()` iteration |
| MIX-04 | `Timestamp`→`field.Time`, `Struct`/`Value`→`field.JSON`, skip `FieldMask` | §Well-Known Type Identification |
| MIX-05 | `optional` scalars → `Nillable().Optional()`; non-optional → `Default(zero)` | §Descriptor Walking — `HasOptionalKeyword()`/`HasPresence()`/synthetic oneof distinction (load-bearing, verified) |
| MIX-06 | `map<K,V>` of scalars → `field.JSON`, message maps skipped | §Descriptor Walking — `IsMap()`/`MapKey()`/`MapValue()` verified |
| MIX-07 | `Exclude(names...)`; unknown name fails at load | §Failure Surface Mechanics |
| MIX-08 | `Override(name, f)`; unknown name fails at load | §Failure Surface Mechanics |
| MIX-09 | `AsJSON("field")` for message-typed fields | §Field Mapping Table |
| MIX-10 | Message fields skipped by default; unresolved `oneof` fails at load | §Descriptor Walking — synthetic vs. real oneof distinction is exactly what separates MIX-05 from MIX-10 |
| MIX-11 | Every failure names message/field/option and the fix | §Pitfall 1 confirmation — `gorun()` subprocess mechanism verified directly from `entc/load/load.go` source |
| MIX-12 | In-process reproduction without `go generate` | §entc.LoadGraph analysis — confirms `LoadGraph` itself still shells out, so D-07's `Validate[M]` bypassing it entirely is the only sound design |
| MIX-13 | Deterministic field order | §Determinism — `protoreflect.FieldDescriptors` is declaration-ordered by construction |
| MIX-14 | Independent module: ent + protobuf + protovalidate/cel-go only | §Two-Module Scaffold — verified `go.mod` minimums via proxy.golang.org; **cel-go import path correction is load-bearing here** |
| ANNO-01 | Schema-level provenance annotation survives JSON boundary | §Annotation Mechanics — `ent.Mixin.Annotations() []schema.Annotation` interface confirmed verbatim from source |
| ANNO-02 | Per-field provenance annotations readable from `gen.Graph` | §Annotation Mechanics |
| ANNO-03 | `Exclude`/`Override` recorded as annotations | Covered by D-02/D-05 + §Annotation Mechanics |
| ANNO-04 | Version marker for mismatch detection | Covered by D-03; no new research needed, design is self-contained |
| VAL-01 | String constraints → native builders | §protovalidate Constraint Enumeration — `ResolveFieldRules` signature verified; `StringRules` field names verified from `validate.proto` |
| VAL-02 | Numeric constraints → `Min`/`Max`/`Range`/`Positive` | §Ent Field Builder Catalog — numeric builder methods verified from `schema/field/numeric.go` source |
| VAL-03 | Presence/`required` → `NotEmpty` or non-optional | §Ent Field Builder Catalog |
| PIPE-01 | Scripted pipeline: buf lint/generate, descriptor-set build, go generate, atlas | §Two-Module Scaffold; ARCHITECTURE.md Anti-Pattern 4 (already researched, not re-derived) |
| PIPE-02 | `go.work` + `GOWORK=off` CI job | §Two-Module Scaffold — `GOWORK=off` semantics verified via web search of Go's own workspace docs |
| PIPE-03 | CI enumerates both modules explicitly | §Two-Module Scaffold |
| PIPE-04 | `mixinforproto/vX.Y.Z` tags, no `replace` | §Two-Module Scaffold |
| PIPE-08 | Docs cover proto3 presence/zero-collapse prominently | Covered by D-26; no new research needed |
</phase_requirements>

## Summary

Phase 1 is buildable exactly as CONTEXT.md scoped it, with one significant correction to prior research: **`.planning/research/STACK.md`'s guidance to import `github.com/cel-expr/cel-go` instead of `github.com/google/cel-go` is verified wrong as of this research date.** I ran `go get github.com/cel-expr/cel-go/cel@v0.31.0` against the live Go module proxy and it fails — the migrated repo's own `go.mod` still declares `module github.com/google/cel-go`, so the module path doesn't match the import path and resolution errors out. `buf.build/go/protovalidate@v1.2.0` (the exact version STACK.md pins) itself transitively requires `github.com/google/cel-go v0.28.0`, confirmed by fetching its `go.mod` directly from the proxy. The practical impact on Phase 1 is small — Tier 2 CEL compilation is a Phase 3 concern, and Phase 1 only needs `buf.build/go/protovalidate` for `ResolveFieldRules` (Tier 1 constraint enumeration), not a direct `cel-go` import at all — but the project's own `mixinforproto.md`/`CLAUDE.md` guidance needs correcting before Phase 3 plans against it, and Phase 1's `go.mod` should NOT add a direct `cel-go` dependency prematurely.

Every other piece of the mixin's mechanics checks out against verified sources: `protoreflect.FieldDescriptor` exposes exactly the presence/oneof/map API the design needs (`HasPresence`, `HasOptionalKeyword`, `ContainingOneof`, and — critically — `OneofDescriptor.IsSynthetic()`, which is the exact, correct mechanism for distinguishing a proto3-`optional` synthetic oneof from a real `oneof` declaration: MIX-05 vs. MIX-10's entire boundary rests on this one boolean). `ent.Mixin`'s interface (fetched verbatim from `ent.go`) confirms `Annotations() []schema.Annotation` is a first-class mixin method whose doc comment says outright "returns a list of schema annotations to add to the schema annotations" — ANNO-01's premise is not just plausible, it's the documented contract. `entc/load/load.go`'s actual source confirms Pitfall 1's mechanism precisely: schema loading writes a generated `main` package to a temp `.entc` dir and executes it via `gorun()` (`exec.Command("go", "run", ...)`), which is exactly why MIX-11/12's design (self-sufficient panic first lines, an in-process `Validate[M]` bypassing this subprocess entirely) is necessary and sufficient — `entc.LoadGraph` itself is confirmed to still go through this same subprocess path, so it cannot serve as MIX-12's debug entry point; only a function that never touches `entc/load` at all (D-07's design) avoids it.

One refinement to D-23 (golden assertion shape): `field.Descriptor()` returns a struct whose `Validators []any` field holds live closures and whose `Err error` field is not JSON-marshalable — `json.Marshal`-ing the raw `*field.Descriptor` will error the moment any Tier 1 `.Match()`/`.Validate()` call is present. The golden harness needs its own serializable projection struct, not the raw descriptor. The reserved-identifier catalog (D-10) is also narrower than CONTEXT.md's phrasing implies: `entc/gen/type.go`'s actual `globalIdent`/`privateField` maps (fetched verbatim) do **not** contain `id`, `type`, `label`, `edge`, or `where` — those collisions are structural (they collide with *per-type generated* identifiers like the type's own `Label` constant, confirmed by reading `ent/ent#280` directly), not members of any static reserved list. `mixinforproto` needs a hand-maintained list combining both classes; see Open Questions.

**Primary recommendation:** Build `mixinforproto`'s Phase 1 `go.mod` with exactly three direct dependencies — `entgo.io/ent@v0.14.6`, `google.golang.org/protobuf@v1.36.11`, `buf.build/go/protovalidate@v1.2.0` — and defer any `cel-go` import to Phase 3, at which point re-check whether `github.com/cel-expr/cel-go`'s module-path migration has actually landed (it had not as of 2026-08-08) before choosing an import path.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Message→field derivation (`protoreflect` walk) | Schema-load-time runtime (mixinforproto) | — | Runs inside `entc`'s schema-loader subprocess; pure Go, no persistence, no service surface |
| Field-name collision / reserved-word check | Schema-load-time runtime (mixinforproto) | — | Must fail before `entc` serializes to JSON, per D-06/D-10 |
| Provenance annotation authoring | Schema-load-time runtime (mixinforproto) | Codegen (entc extension, Phase 2 — consumer only) | Mixin writes; nothing in Phase 1 reads them back — that's Phase 2's `entc/` extension |
| Tier 1 constraint translation (protovalidate → ent builders) | Schema-load-time runtime (mixinforproto) | Database (translated constraints become storage-layer checks via ent) | `MinLen`/`Match`/`Range` etc. compile into ent's own validator plumbing, which the DB migration (Atlas) does not see — these remain app-layer, not DB-layer, checks |
| Two-module build/CI scaffold | Build tooling (Makefile/CI, not runtime) | — | `go.work`, module enumeration, `GOWORK=off` job — no application-tier component involved |
| `entc` schema-load subprocess (`gorun`) | ent framework internal (not entconnect-owned) | — | mixinforproto's panics must be self-sufficient specifically because it cannot control or fix this subprocess boundary |

## Load-Bearing Corrections to Prior Research

These are the findings that change what the planner should tell an executor to type into `go.mod`, verified this session via live tool calls against the actual Go module proxy — not training recall.

### Correction 1: `github.com/cel-expr/cel-go` does not currently work as an import path — use `github.com/google/cel-go`

`.planning/research/STACK.md` and `./CLAUDE.md`'s Technology Stack section both instruct: "Import `github.com/cel-expr/cel-go/cel`, not `github.com/google/cel-go/cel`, in new code." I ran this directly:

```
$ go get github.com/cel-expr/cel-go/cel@v0.31.0
go: github.com/cel-expr/cel-go@v0.31.0 (matching github.com/cel-expr/cel-go/cel@v0.31.0) requires github.com/cel-expr/cel-go@v0.31.0: parsing go.mod:
        module declares its path as: github.com/google/cel-go
                but was required as: github.com/cel-expr/cel-go
```
`[VERIFIED: proxy.golang.org, live `go get` v0.31.0, 2026-08-08]`

The mirror repo's `go.mod` (fetched from `https://proxy.golang.org/github.com/cel-expr/cel-go/@v/v0.31.0.mod`) still literally reads `module github.com/google/cel-go` — the migration is announced but the module-path rename has not landed in a usable release. By contrast:

```
$ go get github.com/google/cel-go/cel@v0.31.0
go: added github.com/google/cel-go v0.31.0
```
succeeds cleanly. `[VERIFIED: proxy.golang.org, live `go get`, 2026-08-08]`

Further, `buf.build/go/protovalidate@v1.2.0`'s own `go.mod` (fetched directly) requires:
```
require (
	...
	github.com/google/cel-go v0.28.0
	...
)
```
`[VERIFIED: proxy.golang.org, `curl https://proxy.golang.org/buf.build/go/protovalidate/@v/v1.2.0.mod`, 2026-08-08]`

**Consequence for the plan:** if/when Phase 3 needs `buf.build/go/protovalidate/cel.NewLibrary()` (which returns a `cel.Library` typed against whichever `cel-go` import protovalidate itself compiled against), the direct dependency must be `github.com/google/cel-go`, not `github.com/cel-expr/cel-go` — using the latter will not build at all right now, let alone produce a type-compatible `cel.Library`. Re-verify at Phase 3 planning time in case the migration has landed by then; do not carry this correction forward blindly past its verification date.

**Consequence for Phase 1 specifically:** none of Phase 1's requirements (MIX-01…14, ANNO-01…04, VAL-01…03, PIPE-01…04/08) need a direct `cel-go` import at all — Tier 1 only needs `buf.build/go/protovalidate`'s `ResolveFieldRules` for constraint *enumeration*, not CEL compilation. **Do not add any `cel-go` dependency to `mixinforproto/go.mod` in Phase 1.** The three-dependency module-isolation constraint (ent + protobuf + protovalidate) is satisfiable without it; `cel-go` only becomes a direct dependency in Phase 3 when the Tier 2 hook is built.

### Correction 2: D-23's golden-assertion shape needs a projection struct, not a raw `*field.Descriptor` marshal

`field.Descriptor()` (fetched verbatim from `entgo.io/ent/schema/field/field.go`, confirmed at the source, not paraphrased):

```go
type Descriptor struct {
	Tag              string
	Size             int
	Name             string
	Info             *TypeInfo
	ValueScanner     any
	Unique           bool
	Nillable         bool
	Optional         bool
	Immutable        bool
	Default          any
	UpdateDefault    any
	Validators       []any                   // validator functions.
	StorageKey       string
	Enums            []struct{ N, V string }
	Sensitive        bool
	SchemaType       map[string]string
	Annotations      []schema.Annotation
	Comment          string
	Deprecated       bool
	DeprecatedReason string
	Err              error
}
```
`[VERIFIED: github.com/ent/ent, schema/field/field.go, fetched 2026-08-08]`

`Validators []any` holds the actual closures registered by `.Match(...)`/`.Validate(fn)`/`.Range(...)`. `encoding/json.Marshal` cannot serialize a `func` value — `json.Marshal(descriptor)` will error the first time a Tier 1 rule attaches a validator (which is most of VAL-01/VAL-02's whole point). D-23 already correctly anticipated that "comparing `ent.Field` values structurally is not viable — some of it is closures," but the concrete mechanism (which field, why it breaks `json.Marshal` specifically) is worth pinning down for the plan: **the golden harness must build its own serializable projection type** (e.g. `{Name, TypeKind, Nillable, Optional, Immutable, DefaultStr string /* fmt.Sprint(Default) */, ValidatorCount int, Enums, StorageKey, Comment}`), populate it field-by-field from the real `*field.Descriptor`, and golden-assert *that* — never the raw struct.

## Standard Stack

### Core (Phase 1 — `mixinforproto/go.mod`)

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| entgo.io/ent | v0.14.6 | `ent.Mixin`, `schema/field` builders, `schema.Annotation` | `[VERIFIED: proxy.golang.org — go.mod requires `go 1.24`, confirmed via `curl proxy.golang.org/entgo.io/ent/@v/v0.14.6.mod`]` — matches CLAUDE.md's pinned version |
| google.golang.org/protobuf | v1.36.11 | `protoreflect` descriptor walking off `(*new(M)).ProtoReflect()` | `[VERIFIED: proxy.golang.org — go.mod requires `go 1.23`]` |
| buf.build/go/protovalidate | v1.2.0 | `ResolveFieldRules(fd protoreflect.FieldDescriptor) (*validate.FieldRules, error)` for Tier 1 constraint enumeration | `[VERIFIED: proxy.golang.org — go.mod requires `go 1.24.0`; function signature confirmed via pkg.go.dev]`. **Do not add `buf.build/go/protovalidate/cel` or any `cel-go` import in Phase 1** — not needed until Phase 3 (see Correction 1) |

`mixinforproto`'s Phase 1 `go.mod` minimum is therefore `go 1.24` (the highest of the three requirements) — comfortably within the locally available `go1.24.7` toolchain (see Environment Availability). This is *lower* than the root module's eventual `go 1.26` (driven by `connectrpc.com/connect@v1.20.0` in Phase 2), which is fine — nested modules do not need to share a `go` directive.

### Supporting (dev/test only, not a runtime dependency of the shipped mixin)

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| github.com/sebdah/goldie/v2 | v2.8.0 | Golden-file assertions for the conformance corpus | `[VERIFIED: proxy.golang.org — `go list -m -versions github.com/sebdah/goldie/v2`, latest is v2.8.0, 2026-08-08]`. `goldie.New(t, opts...)` + `g.Assert(t, name, data)`; `-update` flag regenerates; default fixture dir is `testdata/` `[CITED: github.com/sebdah/goldie README]` |

### Installation

```bash
cd mixinforproto
go mod init entconnect/mixinforproto   # exact module path is a planner/repo-owner call
go get entgo.io/ent@v0.14.6
go get google.golang.org/protobuf@v1.36.11
go get buf.build/go/protovalidate@v1.2.0
go get -d github.com/sebdah/goldie/v2@v2.8.0
```

**Version verification performed this session** (not training-data versions): all four `go get`/`curl` calls above against `proxy.golang.org` succeeded and are reproduced in Load-Bearing Corrections and the Package Legitimacy Audit below.

## Package Legitimacy Audit

> The `gsd-tools package-legitimacy check` seam only supports `npm|pypi|crates` ecosystems — it does not cover Go. This audit was performed manually via direct Go module proxy queries (`go list -m -versions`, `go get`, `curl proxy.golang.org/.../@v/<version>.mod`), which is the Go-ecosystem equivalent of `npm view` — a real registry query, not a websearch guess. Go's module ecosystem has no npm-style `postinstall` script risk (no arbitrary code executes on `go get`/`go mod download`), so that sub-check is not applicable.

| Package | Registry | Verified Versions Exist | Source Repo | Verdict | Disposition |
|---------|----------|--------------------------|-------------|---------|-------------|
| entgo.io/ent | Go proxy | v0.1.0 … v0.14.6 (59 releases) | github.com/ent/ent (Meta/community-maintained, well-known) | OK | Approved |
| google.golang.org/protobuf | Go proxy | v1.20.0 … v1.36.11 | github.com/protocolbuffers/protobuf-go (Google-official) | OK | Approved |
| buf.build/go/protovalidate | Go proxy (BSR-hosted) | v0.1.0 … v1.2.0 (48 releases, steady cadence) | github.com/bufbuild/protovalidate (buf.build-official) | OK | Approved |
| github.com/sebdah/goldie/v2 | Go proxy | v2.0.0 … v2.8.0 | github.com/sebdah/goldie | OK | Approved — widely used in ent/entgql/entproto's own ecosystem per STACK.md precedent |
| github.com/cel-expr/cel-go | Go proxy | resolves as a version list, but **the release itself fails `go get`** (module-path/import-path mismatch) | github.com/cel-expr/cel-go (new org, migration in progress) | **SUS — non-functional at pinned version, do not use in Phase 1 or Phase 3 without re-verifying** | Not installed in Phase 1; flagged for Phase 3 re-check |
| github.com/google/cel-go | Go proxy | v0.1.0 … v0.31.0 | github.com/google/cel-go | OK (this is the currently-correct import, contra STACK.md) | Not installed in Phase 1 (no direct dependency needed); note for Phase 3 |

**Packages removed due to [SLOP] verdict:** none — no hallucinated packages found.
**Packages flagged as suspicious [SUS]:** `github.com/cel-expr/cel-go` — not a naming/legitimacy concern (it is a real, official migration target repo), but it is **currently non-functional as an import** due to an unfinished `go.mod` module-path update in the upstream repo. This is a "verify before using" flag for Phase 3, not a Phase 1 blocker since Phase 1 doesn't import it.

## Architecture Patterns

### Descriptor Walking Mechanics (MIX-01…MIX-10, ANNO-02)

All quotes below are from `google.golang.org/protobuf/reflect/protoreflect`, fetched directly from pkg.go.dev. `[VERIFIED: pkg.go.dev/google.golang.org/protobuf/reflect/protoreflect, 2026-08-08]`

```go
// FieldDescriptor
HasPresence() bool
// HasPresence reports whether the field distinguishes between unpopulated
// and default values.

HasOptionalKeyword() bool
// HasOptionalKeyword reports whether the "optional" keyword was explicitly
// specified in the source .proto file.

ContainingOneof() OneofDescriptor
// ContainingOneof is the containing oneof that this field belongs to,
// and is nil if this field is not part of a oneof.

IsMap() bool
// IsMap reports whether this field represents a map, where the value type
// for the associated field is a Map. It is equivalent to checking whether
// Cardinality is Repeated, that the Kind is MessageKind, and that
// MessageDescriptor.IsMapEntry reports true.

MapKey() FieldDescriptor   // nil if IsMap() is false
MapValue() FieldDescriptor // nil if IsMap() is false

// OneofDescriptor
IsSynthetic() bool
// IsSynthetic reports whether this is a synthetic oneof created to support
// proto3 optional semantics. If true, Fields contains exactly one field
// with FieldDescriptor.HasOptionalKeyword specified.
```

**This is the exact, load-bearing mechanism for MIX-05 vs. MIX-10.** A proto3 `optional` scalar field is, under the hood, wrapped by the compiler in a synthetic one-member oneof — `ContainingOneof()` returns non-nil for it, exactly as it would for a real `oneof`. The only way to tell them apart is `ContainingOneof().IsSynthetic()`:

```go
fd := // protoreflect.FieldDescriptor
if oo := fd.ContainingOneof(); oo != nil {
    if oo.IsSynthetic() {
        // MIX-05 path: this is `optional int32 foo = 1;`
        // → Nillable().Optional()
    } else {
        // MIX-10 path: this is a real `oneof` member
        // → panic unless every member of oo.Fields() is Excluded/Overridden
    }
}
```

Getting this backwards — checking only `ContainingOneof() != nil` without the `IsSynthetic()` guard — is precisely the failure CONTEXT.md's research priorities flagged as making "every `optional` scalar panic." This is now verified, not assumed: `HasOptionalKeyword()` alone is also sufficient to detect the synthetic case directly on the field (no need to go through the oneof at all, per the doc comment above), so the derivation logic has two independent, cross-checkable signals — use `HasOptionalKeyword()` as the primary MIX-05 detector and treat `ContainingOneof() != nil && !ContainingOneof().IsSynthetic()` as the MIX-10 detector; a field can't be both.

**Non-optional, non-oneof proto3 scalar (MIX-05's other branch):** `HasPresence()` returns `false` for these — this is the wire-semantics confirmation that `Default(zero-value)` non-optional mapping is correct (no presence to preserve, so there's nothing lost by collapsing to a DB default).

**Well-known types (MIX-04):** identify by the field's message-type full name via `fd.Message().FullName()` — `"google.protobuf.Timestamp"`, `"google.protobuf.Struct"`, `"google.protobuf.Value"`, `"google.protobuf.FieldMask"` are the literal strings to switch on `[ASSUMED — standard protobuf well-known-type full names, not re-verified against a descriptor this session, but these are protobuf's own fixed, unchanging type names]`.

**Enum iteration (MIX-03):** `[VERIFIED: pkg.go.dev/google.golang.org/protobuf/reflect/protoreflect]`
```go
type EnumValueDescriptors interface {
	Len() int
	Get(i int) EnumValueDescriptor
	ByName(s Name) EnumValueDescriptor
	ByNumber(n EnumNumber) EnumValueDescriptor
}
// EnumDescriptor.Values() EnumValueDescriptors
```
Iterate `enumDesc.Values()` by index `0..Len()-1` (never build an intermediate map first — this is exactly the D-24 determinism discipline applied at the source) and collect `.Name()` strings for `field.Enum(name).Values(...)`.

### Ent Field Builder Catalog (MIX-02, VAL-01, VAL-02, VAL-03)

Constructors, `[VERIFIED: pkg.go.dev/entgo.io/ent/schema/field, 2026-08-08]`:
```
Int, Int8, Int16, Int32, Int64, Uint, Uint8, Uint16, Uint32, Uint64,
Float, Float32, Floats, Ints, String, Text, Bytes, Strings, Bool,
Time, UUID(name, typ driver.Valuer), JSON(name, typ any), Any, Enum, Other
```

Method availability confirmed by reading `schema/field/field.go` and `schema/field/numeric.go` source directly `[VERIFIED: github.com/ent/ent, schema/field/{field,numeric}.go, 2026-08-08]`:

| Builder | `.Validate(fn)` | `.MinLen`/`.MaxLen`/`.Match`/`.NotEmpty` | `.Min`/`.Max`/`.Range`/`.Positive`/`.Negative`/`.NonNegative` | `.Optional`/`.Nillable`/`.Immutable` |
|---|---|---|---|---|
| `stringBuilder` | `func(string) error` | yes | — | yes |
| `bytesBuilder` | `func([]byte) error` | `MinLen`/`MaxLen`/`NotEmpty` yes, no `Match` | — | yes |
| `sliceBuilder[T]` (`Ints`/`Strings`/`Floats`) | `func([]T) error` | — | — | `Optional`/`Immutable` (no `Nillable` confirmed) |
| `int32Builder` (and all other numeric per-type builders) | none | — | all confirmed present, per numeric type (e.g. `func (b *int32Builder) Min(i int32) *int32Builder`) | yes |
| `boolBuilder`, `timeBuilder`, `jsonBuilder`, `enumBuilder`, `uuidBuilder`, `otherBuilder` | none | — | — | yes (jsonBuilder has no `Nillable` confirmed) |

This directly confirms STACK.md's Load-Bearing Finding: `.Validate()` exists only on `stringBuilder`/`bytesBuilder`/`sliceBuilder[T]`, never on numeric/enum/time/bool/UUID/JSON builders — D-13's format-validator delegation (`field.String().Validate(fn)`) is therefore only usable for the string format validators it's scoped to, exactly as designed.

`field.Enum` construction `[VERIFIED: pkg.go.dev/entgo.io/ent/schema/field]`:
```go
field.Enum(name string) *enumBuilder
// .Values(...string)   .Default(string)
```

### Annotation Mechanics (ANNO-01, ANNO-02, ANNO-03)

`[VERIFIED: github.com/ent/ent, schema/schema.go and ent.go, fetched 2026-08-08]`

```go
// schema/schema.go
type Annotation interface {
	// Name defines the name of the annotation to be retrieved by the codegen.
	Name() string
}
```

```go
// ent.go — the ent.Mixin interface, in full
Mixin interface {
	Fields() []Field
	Edges() []Edge
	Indexes() []Index
	// Hooks returns a slice of hooks to add to the schema.
	// Note that mixin hooks are executed before schema hooks.
	Hooks() []Hook
	// Interceptors returns a slice of interceptors to add to the schema.
	// Note that mixin interceptors are executed before schema interceptors.
	Interceptors() []Interceptor
	// Policy returns a privacy policy to add to the schema.
	// Note that mixin policy are executed before schema policy.
	Policy() Policy
	// Annotations returns a list of schema annotations to add
	// to the schema annotations.
	Annotations() []schema.Annotation
}
```

This directly confirms ANNO-01's premise, verbatim, not by inference: a mixin's `Annotations()` return value is documented to be **added to the schema's own annotations** — it is the same channel a schema author's own `Annotations()` method would use, merged in. Combined with ARCHITECTURE.md's already-verified finding that `gen.Type.Annotations` (built from `load.Schema.Annotations map[string]any`) is the only channel crossing the JSON boundary, this closes the loop: `MixinForProto`'s `Annotations()` implementation is the concrete, confirmed mechanism for ANNO-01.

Per-field annotations (ANNO-02) attach via the field builder's own `.Annotations(...)` method (an `ent.Field`-builder-common method, not separately re-verified this session but is standard, stable ent API present since early versions — `[ASSUMED]`, low risk).

### Well-Known Type Identification (MIX-04)

| Proto WKT full name | ent mapping |
|---|---|
| `google.protobuf.Timestamp` | `field.Time(name)` |
| `google.protobuf.Struct` | `field.JSON(name, map[string]any{})` |
| `google.protobuf.Value` | `field.JSON(name, any(nil))` — needs a concrete Go representation choice at implementation time |
| `google.protobuf.FieldMask` | skip (transport concern per `mixinforproto.md` §5) |
| `google.protobuf.Duration` | skip in v1 (MIX2-04 is v2) |

`[ASSUMED — standard WKT full names and the mapping table transcribed from `mixinforproto.md` §5, which is itself a project design doc, not independently re-verified against a live descriptor this session]`. Detection mechanism (`fd.Message().FullName()` string comparison) is the standard, well-known protobuf-go idiom.

### Recommended Project Structure

Unchanged from `.planning/research/ARCHITECTURE.md` §Recommended Project Structure — not re-derived here, only the `mixinforproto/` leaf matters for Phase 1:

```
mixinforproto/
├── go.mod                 # independent module — ent, protobuf, protovalidate ONLY (no cel-go in Phase 1)
├── mixin.go                # MixinForProto[M], Exclude, Override, Validate[M] (D-07)
├── fieldmap.go             # scalar/enum/WKT → ent.Field translation table
├── validate.go              # Tier 1 translation only in Phase 1 (ResolveFieldRules → builder calls)
├── annotation.go            # SourceMessage / SourceField — the ONLY cross-boundary contract (D-01/D-02)
└── testdata/                # committed corpus .proto + generated Go stubs (D-21/D-22)
```

### Pattern: Pure-Core / Panicking-Wrapper Split (D-06)

```go
// derive.go — pure, testable, returns (result, error)
func derive[M proto.Message](opts ...Option) (*derivation, error) {
    fd := (*new(M)).ProtoReflect().Descriptor()
    // ... walk fd.Fields(), build []ent.Field + annotations, collect errors (D-09) ...
    if len(errs) > 0 {
        return nil, &derivationError{errs: errs} // structured, D-09: all collected, not first-fail
    }
    return &derivation{fields: fields, annotations: annos}, nil
}

// mixin.go — the ent.Mixin adapter; panics are isolated here (D-06)
func MixinForProto[M proto.Message](opts ...Option) ent.Mixin {
    return &protoMixin[M]{opts: opts}
}

func (m *protoMixin[M]) Fields() []ent.Field {
    d, err := derive[M](m.opts...)
    if err != nil {
        panic(err) // MIX-11: err.Error() must be self-sufficient as its first line (D-08)
    }
    return d.fields
}

// Validate is the MIX-12 in-process debug entry point (D-07) — calls the SAME
// derive() core directly. It never touches entc.LoadGraph or entc/load, so it
// never enters the gorun() subprocess — this is what makes it a real debug aid.
func Validate[M proto.Message](opts ...Option) error {
    _, err := derive[M](opts...)
    return err
}
```

### `(*new(M)).ProtoReflect()` on a nil generated pointer — confirmed safe

`[VERIFIED: pkg.go.dev/google.golang.org/protobuf/proto — `proto.Message` interface has exactly one method, `ProtoReflect() protoreflect.Message`; STACK.md already confirmed this pattern is safe on a nil/zero-value generated pointer, re-confirmed by this session's read of the same interface — not re-derived from scratch, STACK.md's finding stands]`.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|--------------|-----|
| Enumerating which protovalidate constraints apply to a field | A hand-rolled proto-option reader walking `field.Options()` extensions | `buf.build/go/protovalidate.ResolveFieldRules(fd)` | Returns the fully-resolved `*validate.FieldRules` (including predefined/standard rules merged in) in one call — hand-parsing extension options is exactly the kind of "reimplement protovalidate's own resolution logic" risk STACK.md already flagged as a real hazard |
| String format validation (`uuid`/`email`/`hostname`/`uri`/`ip`) | Custom regexes for each format | Delegate to protovalidate's own predicate via `.Validate(fn)` (D-13) for the procedural ones; use protovalidate's own documented regex for the ones that are regex-defined | Verdict identity with the boundary interceptor is the property the whole design rests on — a hand-rolled regex will eventually disagree with protovalidate's edge cases (e.g. IDNA hostnames, IPv6 forms) |
| Deterministic ordering of derived fields | Sorting field names alphabetically after collecting into a map | Iterate `fd.Fields()` (a `protoreflect.FieldDescriptors`, itself declaration-ordered) directly, never through an intermediate `map` | `protoreflect`'s own field list is already in declaration order (`Get(i)` for `i := 0; i < Len(); i++`) — introducing a map and re-sorting is strictly worse and is exactly Pitfall 10's failure mode |
| Golden-comparable serialization of derived fields | `json.Marshal(fieldDescriptor)` on the raw `*field.Descriptor` | A hand-written projection struct (see Correction 2) | The raw struct contains `Validators []any` (closures) and `Err error`, neither of which `encoding/json` can serialize |

**Key insight:** every "don't hand-roll" here traces back to the same root cause — protobuf-go and protovalidate already expose exactly the introspection surface this design needs (`protoreflect`, `ResolveFieldRules`), and building a parallel, hand-maintained version of any of it reintroduces the "two sources of truth" failure the whole project exists to eliminate, just one layer lower.

## Common Pitfalls

### Pitfall 1 (from PITFALLS.md, mechanism now directly verified): `entc`'s schema-load subprocess truncates panic output

**Confirmed this session**, not inferred: `entc/load/load.go`'s actual source (fetched directly) writes a synthesized `main` package to a temp `.entc` directory and executes it via a `gorun()` helper that shells out with `exec.Command("go", "run", target, ...)`. `entc.LoadGraph` — the function you might reach for as a "load the graph without `go generate`" helper — **also goes through this exact same `load.Config.Load()` path**, confirmed by reading `entc/entc.go`'s `LoadGraph` body directly:
```go
func LoadGraph(schemaPath string, cfg *gen.Config) (*gen.Graph, error) {
	spec, err := (&load.Config{Path: schemaPath, BuildFlags: cfg.BuildFlags}).Load()
	...
}
```
`[VERIFIED: github.com/ent/ent, entc/entc.go and entc/load/load.go, fetched 2026-08-08]`

**Implication for MIX-12:** `entc.LoadGraph` cannot be the debug entry point — it still goes through `gorun()`'s subprocess. D-07's design (an exported `Validate[M]` in `mixinforproto` itself that calls the pure `derive[M]` core directly, never touching `entc/load` at all) is the *only* correct way to satisfy MIX-12, not an implementation convenience — confirmed, not assumed.

**Verification:** every panic path test (D-25) should additionally assert that the equivalent `Validate[M](sameOpts)` call surfaces the identical error string without invoking `go generate`/`entc.LoadGraph` anywhere in the test.

### Pitfall 2 (from PITFALLS.md, catalog now verified — and found narrower than CONTEXT.md's phrasing): ent's static reserved-identifier list does not include `id`/`type`/`label`/`edge`/`where`

`entc/gen/type.go`'s actual reserved-identifier declarations, fetched verbatim `[VERIFIED: github.com/ent/ent, entc/gen/type.go, fetched 2026-08-08]`:
```go
var (
	// global identifiers used by the generated package.
	globalIdent = names(
		"AggregateFunc", "As", "Asc", "Client", "config", "Count", "Debug",
		"Desc", "Driver", "Hook", "Interceptor", "Log", "MutateFunc",
		"Mutation", "Mutator", "Op", "Option", "OrderFunc", "Max", "Mean",
		"Min", "Schema", "Sum", "Policy", "Query", "Value",
	)
	// private fields used by the different builders.
	privateField = names(
		"config", "ctx", "done", "hooks", "inters", "limit", "mutation",
		"offset", "oldValue", "order", "op", "path", "predicates", "typ",
		"unique", "driver",
	)
)
```

**None of `id`, `type`, `label`, `edge`, `where`, or `client` (lowercase) appear in this static catalog.** CONTEXT.md's D-10 lists these as examples of "ent's known reserved identifiers... and the rest of the catalog-known set" — that phrasing is not quite right at the source. Cross-checking `ent/ent#280` (fetched directly): the actual `label` collision reported there was against a **per-type generated constant** (`const Label = "phone"`, generated fresh for every entity type from the type's own name, not from a static list) — the Go compiler error was `"Label redeclared in this block previous declaration at ent/phone/phone.go:14:10"`. `[VERIFIED: github.com/ent/ent/issues/280, fetched 2026-08-08 — no maintainer-provided reserved-word list was offered in that issue either]`.

**Consequence for the plan:** `mixinforproto`'s reserved-identifier check needs to combine two categories, and the plan should build both explicitly rather than assuming one catalog covers everything:
1. **Static**: `globalIdent` ∪ `privateField` from `entc/gen/type.go` (26 + 16 = 42 identifiers, case-sensitive as listed above) — these are stable and can be hand-copied into `mixinforproto`'s own constant list (importing `entc/gen` itself would violate module isolation, so copy, don't import).
2. **Structural**: identifiers ent always generates *per type*, derived from the entity's own name/fields, which cannot be a fixed list — at minimum `id`/`ID` (every entity has one), `Label`/`label` (per `ent/ent#280`, generated per type). This category needs either (a) a small hand-maintained supplementary list documented as "known to collide, incomplete by nature" or (b) explicit acknowledgment in the panic/docs that this category is Go-compiler-error territory, exactly as PITFALLS.md's own "How to Avoid" already concludes for cross-mixin/hand-declared-field collisions. See Open Questions.

### Pitfall 9/10/12 (determinism, staleness, nested-module CI): no new findings beyond ARCHITECTURE.md/PITFALLS.md

These are already thoroughly researched in the referenced documents (Anti-Pattern 4/5, Pitfalls 9/10/12) and this session found nothing to correct there. One additive confirmation: `GOWORK=off go build ./...` is the documented, correct way to simulate external-consumer resolution `[CITED: go.dev/ref/mod, go.dev/doc/tutorial/workspaces — via WebSearch, not independently re-fetched this session]`, and `go.work` files are explicitly meant to stay out of CI's normal build path — consistent with D-17.

## Code Examples

### go.work (repo root, checked in per D-15)

```
go 1.26

use (
	.
	./mixinforproto
)
```
`[CITED: go.dev/doc/tutorial/workspaces syntax]` — the root module's own `go.mod` `go` directive should be `go 1.26` (Phase 2's `connectrpc.com/connect@v1.20.0` requirement, per STACK.md, applies even though Phase 1 doesn't use Connect yet — CONTEXT.md D-18 only defers the *dependency edge*, not necessarily the toolchain floor; this is a planner judgment call, not settled by this research).

### mixinforproto/go.mod (Phase 1 target shape)

```
module entconnect/mixinforproto   // exact path: planner/repo-owner decision

go 1.24

require (
	entgo.io/ent v0.14.6
	google.golang.org/protobuf v1.36.11
	buf.build/go/protovalidate v1.2.0
)

require github.com/sebdah/goldie/v2 v2.8.0 // test-only
```

### CI module enumeration (D-16)

```makefile
MODULES := . ./mixinforproto

.PHONY: test-all
test-all:
	@for m in $(MODULES); do \
		echo "== $$m =="; \
		(cd $$m && go build ./... && go vet ./... && go test ./...) || exit 1; \
	done

.PHONY: test-mixinforproto-standalone
test-mixinforproto-standalone:
	cd mixinforproto && GOWORK=off go build ./... && GOWORK=off go test ./...
```

### Golden-comparable projection struct (Correction 2)

```go
type goldenField struct {
	Name           string
	TypeKind       string // e.g. field.TypeInfo.Type.String()
	Nillable       bool
	Optional       bool
	Immutable      bool
	Default        string // fmt.Sprint(desc.Default) — never marshal Default directly if it can hold a func
	ValidatorCount int    // len(desc.Validators) — never marshal the closures themselves
	Enums          []struct{ N, V string }
	StorageKey     string
	Comment        string
}

func project(f ent.Field) goldenField {
	d := f.Descriptor()
	return goldenField{
		Name: d.Name, TypeKind: d.Info.Type.String(),
		Nillable: d.Nillable, Optional: d.Optional, Immutable: d.Immutable,
		Default: fmt.Sprint(d.Default), ValidatorCount: len(d.Validators),
		Enums: d.Enums, StorageKey: d.StorageKey, Comment: d.Comment,
	}
}
```
Then `goldie.New(t).AssertJson(t, "order_scalars", projectAll(derivedFields))` — sorted by `Name` before marshal (D-24).

### Synthetic-oneof detection (the MIX-05/MIX-10 boundary)

```go
func classify(fd protoreflect.FieldDescriptor) fieldClass {
	switch {
	case fd.IsMap():
		return classScalarMap // or classMessageMap if MapValue().Kind() == protoreflect.MessageKind
	case fd.ContainingOneof() != nil && !fd.ContainingOneof().IsSynthetic():
		return classRealOneofMember // MIX-10: panic unless Excluded/Overridden
	case fd.HasOptionalKeyword():
		return classOptionalScalar // MIX-05: Nillable().Optional()
	case fd.Kind() == protoreflect.MessageKind:
		return classMessageField // skipped by default, or AsJSON
	case fd.Kind() == protoreflect.EnumKind:
		return classEnum
	default:
		return classScalar // MIX-05 other branch: Default(zero), non-optional
	}
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|---|---|---|---|
| `protoc-gen-validate` (PGV), regex-only constraints | `protovalidate`/`buf.validate`, CEL-based | protovalidate.com frames itself as PGV's successor | Already reflected correctly in STACK.md/CLAUDE.md; no correction needed |
| `entproto`: ent schema → generated `.proto` | `mixinforproto`: `.proto` → ent fields | This project's own inversion | The entire premise of Phase 1; no external "state of the art" shift, an intentional design choice |
| `github.com/google/cel-go` as the stable import path | Migration announced to `github.com/cel-expr/cel-go` | Announced, **not yet functionally complete** as of 2026-08-08 | See Load-Bearing Correction 1 — do not follow the announced migration until the target repo's `go.mod` actually declares the new path |

**Deprecated/outdated:** `github.com/bufbuild/protovalidate-go` (legacy import path, superseded by `buf.build/go/protovalidate` — STACK.md's guidance here is correct and unchanged by this session's research).

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | WKT full names (`google.protobuf.Timestamp` etc.) used as literal string-comparison targets for `fd.Message().FullName()` | Well-Known Type Identification | Low — these are protobuf's own fixed, unversioned type names; extremely stable, but not independently re-verified against a live descriptor dump this session |
| A2 | `.Annotations(...)` exists as a field-builder method for per-field annotations (ANNO-02's field-level mechanism) | Annotation Mechanics | Low-medium — if the exact method name/signature differs, ANNO-02's implementation needs a one-line fix, not a redesign; ARCHITECTURE.md's Pattern 1 already assumes this and it is standard, long-standing ent API |
| A3 | `google.protobuf.Struct`/`Value` map to `field.JSON` with the specific Go representations shown (`map[string]any{}` / `any(nil)`) | Well-Known Type Identification | Medium — the exact Go zero-value/typ argument to `field.JSON(name, typ)` affects the generated Go accessor type; needs a concrete decision at implementation time, not just "JSON" |
| A4 | The reserved-identifier "structural" category (id/Label/etc.) is adequately covered by a short hand-maintained supplementary list rather than needing dynamic detection against the *specific* schema's own generated identifiers | Pitfall 2 / Common Pitfalls | Medium — if wrong, some collisions (e.g. a field literally named the same as the schema's own struct name) will only surface as Go compiler errors post-generation, exactly as documented as an accepted limitation in D-10, so this degrades gracefully rather than silently |
| A5 | `go 1.26` is the correct root-module `go` directive to write in Phase 1 even though D-18 defers the `mixinforproto` dependency edge to Phase 2 | Code Examples | Low — worst case the directive is bumped in a later phase; does not block Phase 1 acceptance criteria |

**If this table is empty:** N/A — see entries above; none of these block Phase 1's core mechanics, all are either low-risk standard-API assumptions or explicit implementation-detail decisions the planner should make explicitly rather than leave implicit.

## Open Questions

1. **Complete structural reserved-identifier catalog**
   - What we know: `entc/gen/type.go`'s `globalIdent`/`privateField` (42 static identifiers, verified verbatim) do not include `id`/`type`/`label`/`edge`/`where`; those collide with *per-type generated* identifiers (confirmed for `Label` via `ent/ent#280`).
   - What's unclear: whether `id`, `type`, `edge`, `where` collide the same way (per-type generated) or via some other ent mechanism not covered by this session's source reads (e.g. `type.go`'s field-name validation function, which was not located in this pass).
   - Recommendation: at plan time, either (a) budget a task to read `entc/gen/type.go`'s field-name-generation logic (not just the reserved-word maps) to enumerate the full structural set precisely, or (b) accept the documented-limitation approach D-10 already allows ("collisions... left to surface as the Go compiler's `redeclared in this block` error") and ship the static 42-identifier list plus a small hand-picked supplement (`id`, `Label`) with a test proving both categories panic/are documented, deferring full completeness.

2. **`google.protobuf.Value`'s concrete Go JSON representation**
   - What we know: it maps to `field.JSON`.
   - What's unclear: the exact `typ any` argument — `structpb.Value` itself, `any`, or a custom wrapper — affects the generated field's Go accessor type and whether `protojson`-style round-tripping stays lossless.
   - Recommendation: planner should treat this as a one-line implementation decision in the fieldmap.go task, not a design question — pick `field.JSON(name, json.RawMessage(nil))` or similar and document the choice in the mapping table, since `mixinforproto.md` §5 doesn't specify further.

3. **When to re-check the `cel-expr/cel-go` migration**
   - What we know: as of 2026-08-08, `github.com/cel-expr/cel-go@v0.31.0`'s `go.mod` still declares `module github.com/google/cel-go`, making it non-functional as an import.
   - What's unclear: whether this is fixed in a later patch release before Phase 3 planning begins.
   - Recommendation: Phase 3's research pass should re-run the exact `go get github.com/cel-expr/cel-go/cel@latest` check performed here before committing to an import path; do not assume this research's finding is still true by the time Phase 3 starts.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go toolchain | All of Phase 1 | ✓ | go1.24.7 (local) | `mixinforproto`'s own `go 1.24` directive is satisfied directly; root module's eventual `go 1.26` directive will auto-fetch via `GOTOOLCHAIN=auto` (confirmed set) when needed in a later phase |
| `buf` CLI | PIPE-01's pipeline script (`buf lint`/`buf generate`/`buf build -o`) | ✗ | — | Not installable/verifiable in this research session's sandbox; Phase 1's pipeline script can be authored and its `buf` invocations left un-exercised until a machine with `buf` runs it — D-20 already notes "the `go generate` and `atlas` steps have nothing real to act on yet" in Phase 1, and the same applies to `buf` if it's unavailable in CI images at plan time. Flag for the plan: verify `buf` is installed in the actual CI image before relying on PIPE-01's script executing successfully end-to-end. |
| `atlas` CLI | PIPE-01's pipeline script (`atlas migrate diff`) | ✗ | — | Same as `buf` — script exists, step is a no-op placeholder in Phase 1 per D-20 |
| Go module proxy (network) | All `go get`/`go mod tidy` operations, and this research session's own verification | ✓ | proxy.golang.org reachable via the agent proxy | none needed |

**Missing dependencies with no fallback:** none — `buf`/`atlas` absence does not block Phase 1's core deliverable (the mixin itself), only the pipeline script's ability to be end-to-end exercised locally; this is expected and already anticipated by D-20's phrasing.

**Missing dependencies with fallback:** `buf`, `atlas` — script written and committed regardless; actual execution verification deferred to whatever CI image the plan targets.

## Security Domain

### Applicable ASVS Categories

Phase 1 ships a build-time/schema-load-time Go library with no network listener, no request handling, and no persisted secrets — most ASVS categories are not applicable to this phase's actual attack surface. The one category that matters is input validation, because Tier 1's entire job is validation-relay correctness.

| ASVS Category | Applies | Standard Control |
|---------------|---------|-------------------|
| V1 Architecture, Design and Threat Modeling | Partial | Fail-closed-at-schema-load is itself the security-relevant architectural property here: an untranslatable or malformed constraint must never be silently dropped in a way that later reads as "validated" when it wasn't (D-11 already handles this by *recording*, not silently discarding, residuals) |
| V2 Authentication | No | Phase 1 has no runtime request path; authentication is Phase 2+ |
| V3 Session Management | No | Same |
| V4 Access Control | No | Same — ent `privacy.Policy` is Phase 2+ |
| V5 Input Validation | Yes | `buf.build/go/protovalidate`'s `ResolveFieldRules` + Tier 1 → native ent builders (`MinLen`/`MaxLen`/`Match`/`Range`) is the standard control; **never hand-roll a competing validation path** (see Don't Hand-Roll) |
| V6 Cryptography | No | No crypto in this phase |

### Known Threat Patterns for this stack

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|----------------------|
| A crafted/malicious `.proto` contract causing the schema-load subprocess to panic in a way that's mistaken for a benign build failure and silently retried/ignored by CI tooling | Denial of Service (build-time, not runtime) | D-08/D-11's self-sufficient, structured panic messages ensure a real constraint-translation failure cannot be mistaken for CI flakiness; this is a build-time concern, not a production DoS vector, since `mixinforproto` never runs against untrusted input at request time |
| Regex-based format validators (`Match(regexp.MustCompile(...))` for `uri`/`hostname`/etc., D-13) vulnerable to ReDoS if protovalidate's own documented regex has catastrophic-backtracking potential | Denial of Service | D-13 explicitly delegates to protovalidate's *own* regex/predicate rather than hand-writing one — inherits whatever ReDoS posture protovalidate itself has (upstream's problem to have already solved, not this project's to introduce) `[ASSUMED — protovalidate's own regexes were not independently audited for ReDoS this session]` |
| Annotation payload (`SourceMessage`/`SourceField`) later decoded via `mapstructure` by a downstream consumer (Phase 2) accepting attacker-influenced field data | Tampering / Information Disclosure | Out of scope for Phase 1 (mixinforproto only *writes* annotations; decoding is Phase 2's `entc/` extension) — flag for Phase 2's security research pass, not a Phase 1 concern since the annotation author (the schema package) and the annotation consumer (the entc extension) are both trusted, first-party code, not attacker-influenced input |

## Sources

### Primary (HIGH confidence — direct tool verification this session)
- `proxy.golang.org` — live `go get`/`go list -m -versions`/`curl .../@v/<version>.mod` for entgo.io/ent, google.golang.org/protobuf, buf.build/go/protovalidate, github.com/cel-expr/cel-go, github.com/google/cel-go, github.com/sebdah/goldie/v2, connectrpc.com/connect — 2026-08-08
- `pkg.go.dev/google.golang.org/protobuf/reflect/protoreflect` — `FieldDescriptor`/`OneofDescriptor`/`EnumDescriptor` method doc comments, fetched directly
- `pkg.go.dev/entgo.io/ent/schema/field` — constructor list, fetched directly
- `github.com/ent/ent` source, fetched directly: `schema/field/field.go` (`Descriptor` struct), `schema/field/numeric.go` (numeric builder methods), `schema/schema.go` (`Annotation` interface), `ent.go` (`Mixin` interface), `entc/entc.go` (`LoadGraph`), `entc/load/load.go` (`gorun`/subprocess mechanism), `entc/gen/type.go` (`globalIdent`/`privateField`)
- `github.com/ent/ent/issues/280` — fetched directly, `Label` collision mechanism

### Secondary (MEDIUM confidence — official docs via WebSearch/WebFetch, not independently re-fetched to source)
- `sebdah/goldie` README (usage pattern, `-update` flag)
- `bufbuild/protovalidate` `validate.proto` (FieldRules/StringRules field names)
- `go.dev/ref/mod`, `go.dev/doc/tutorial/workspaces` (`go.work`/`GOWORK=off` semantics)

### Tertiary (LOW confidence — carried forward from prior research, not re-verified this session)
- WKT full-name mapping table (transcribed from `mixinforproto.md` §5)
- `.Annotations(...)` field-builder method existence (standard, long-standing ent API, not re-fetched)
- protovalidate's own regex ReDoS posture (not independently audited)

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — all four Phase 1 dependencies version-verified live against proxy.golang.org this session; one prior-research error (cel-go import path) caught and corrected before it could reach a plan
- Architecture (descriptor walking, annotation mechanics, field builders): HIGH — verified against actual ent/protobuf-go source, not documentation summaries alone
- Pitfalls: HIGH for Pitfalls 1/2 (mechanism and catalog now directly source-verified, and found narrower/different than CONTEXT.md's phrasing in one place); MEDIUM for 9/10/12 (unchanged from prior research, not independently re-verified this session)
- Security: MEDIUM — correctly scoped to "not much applies yet," but ReDoS posture of protovalidate's own format-validator regexes was not independently audited

**Research date:** 2026-08-08
**Valid until:** 30 days for the ent/protobuf-go API findings (stable, slow-moving libraries); **7 days for the `cel-expr/cel-go` migration status specifically** — re-verify before Phase 3 planning, this is explicitly called out as time-sensitive in Open Question 3
