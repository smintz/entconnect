<!-- GSD:project-start source:PROJECT.md -->

## Project

**entconnect**

entconnect makes protobuf contracts the source of truth for [ent](https://entgo.io) applications. It inverts entproto's direction of generation: instead of deriving `.proto` files from ent schemas, entconnect derives ent fields *from* the contract and generates the transport layer that binds RPCs to the application. It is a Go library for teams building contract-first, LLM-assisted backends where design → proto → schema is the only human-writable surface and everything downstream is generated.

It ships as two independently adoptable pieces: `mixinforproto` (a runtime ent mixin — message type in, `[]ent.Field` + validation out, zero codegen) and the `entconnect` entc extension (handler generation, drift checking, multi-transport emission). It is the contract half of a two-project split; the workflow half is **entflow**, a separate project that entconnect touches only through a small metadata interface.

**Core Value:** API-visible fields and their validation are defined exactly once — in the protobuf contract — and enforced identically at the transport boundary and at the storage layer, so contract/schema drift is a build failure rather than a runtime surprise.

### Constraints

- **Dependency direction**: `entflow → ent`; `entconnect → ent + protobuf/descriptor machinery (+ entflow metadata interface)`; `app → both`. The application is the only meeting point — Either project must be releasable, versionable, and adoptable without the other.
- **Module isolation**: `mixinforproto` depends only on `ent`, `google.golang.org/protobuf`, and `protovalidate-go`/`cel-go` — this is what makes it shippable as a standalone micro-module to people who want proto-first ent and none of the rest of the stack.
- **entflow coupling**: confined to a small metadata interface (flow name, input/output types, sync/async nature), ideally duck-typed or a tiny shared `entflow/meta` package, so entconnect builds without entflow's runtime.
- **Zero-flow and zero-contract usability**: entconnect must be fully useful with zero flows (pure CRUD apps); entflow must be fully useful with zero contract.
- **Codegen-free mixin**: `MixinForProto` is plain Go executing at schema-load time — no entc extension, no code generation, no committed descriptor files. Schema load *is* the check phase, so failures panic there rather than at first mutation.
- **Generated handlers are not editable files**: flow-bound handlers are 100% generated and "dumb by construction," not by discipline.
- **Ecosystem precedent**: golden-file tests for generated code follow the entgql/entproto precedent.

<!-- GSD:project-end -->

<!-- GSD:stack-start source:research/STACK.md -->

## Technology Stack

## Recommended Stack

### Core Framework

| Technology | Version | Purpose | Why Recommended |
|------------|---------|---------|------------------|
| entgo.io/ent | v0.14.6 (Mar 17 2026) | ORM/schema graph, entc codegen host | The project's whole premise is an ent extension + runtime mixin; ent is not a choice, it's the given. v0.14.6 is the current published version — no v1 yet, API is stable in practice (entproto/entgql both still target `entgo.io/ent`). |
| google.golang.org/protobuf | v1.36.11 (Dec 12 2025) | protoreflect, `proto.Message`, `protodesc`, wire codec | The only supported Go protobuf runtime (`github.com/golang/protobuf` is deprecated and forwards to this). `protoreflect.Message.Descriptor()` off `msg.ProtoReflect()` is the exact mechanism `MixinForProto[M proto.Message]` needs and is confirmed to work on a nil/zero-value generated pointer, which is what `*new(M)` produces — verified, not assumed. |
| connectrpc.com/connect | v1.20.0 (May 2026) | Handler/client runtime, interceptors, error codes | Flagship transport per the design doc. Module is marked stable, wire-compatible with gRPC/gRPC-Web (so grpc-gateway can sit in front of the same generated handlers later). Requires Go ≥1.25 — pin `go 1.26` in go.mod (current stable toolchain is 1.26.4). |
| buf CLI | v1.72.0 | Lint, breaking-change detection, codegen orchestration, FileDescriptorSet emission | De facto standard front-end for protoc in 2026; no v2.0 planned (buf commits to no breaking changes post-v1.0), so pin loosely. `buf.gen.yaml` v2 with `managed: enabled: true` removes the need to hand-write `go_package` options across every proto file. |

### Descriptor / Validation Toolchain

| Technology | Version | Purpose | Why Recommended |
|------------|---------|---------|------------------|
| buf.build/go/protovalidate | v1.2.0 (Apr 17 2026) | Boundary validation + constraint enumeration for Tier 1/2 translation | **Module path changed** — see Load-Bearing Findings. Exposes `ResolveFieldRules`/`ResolveMessageRules`/`ResolveOneofRules` (public) to enumerate constraints per field descriptor without running a full `Validate()`, which Tier 1 translation needs. |
| buf.build/go/protovalidate/cel | v1.2.0 (same module) | Building a CEL env identical to protovalidate's, for Tier 2 standalone compile+eval | Publicly exports `NewLibrary() cel.Library` and `RequiredEnvOptions(fieldDesc)`. This is the load-bearing discovery of this research pass — see below. |
| github.com/cel-expr/cel-go (formerly github.com/google/cel-go) | v0.31.0 (Aug 7 2026) | CEL compile/eval engine underneath protovalidate | **Repo is mid-migration** — see Load-Bearing Findings. Needed as a direct dependency because `buf.build/go/protovalidate/cel.NewLibrary()` returns a `cel.Library` that you feed into your own `cel.NewEnv(...)`. |
| google.golang.org/genproto/googleapis/api (annotations, resource_reference subpkg) | current, actively maintained | Generated Go types for `google.api.http` and `google.api.resource_reference` options | Confirmed not deprecated (only `github.com/golang/protobuf` carries a deprecation notice, unrelated). This is what the entc extension reads via `protoreflect` proto option extensions to drive the grpc-gateway/REST emitters and the resource_reference drift check. Confidence LOW on this one item — verify current import path once you pin it in go.mod, since Google periodically reshuffles `genproto` package boundaries. |

### Infrastructure

| Technology | Version | Purpose | Why |
|------------|---------|---------|-----|
| grpc-ecosystem/grpc-gateway/v2 | v2.29.0 (Apr 16 2026) | v0.4 gateway emitter: pass-through `google.api.http` → REST/JSON | Still the standard for this exact use case; module path is `github.com/grpc-ecosystem/grpc-gateway/v2`. Not needed until phase covering "grpc-gateway annotations" (design doc §7 v0.4) — do not add the dependency earlier than that phase. |
| ariga.io/atlas (CLI) | v1.3.0 (Aug 2 2026) | `atlas migrate diff` — versioned migrations from the ent schema graph | ent's own documented versioned-migrations path (entgo.io/docs/versioned-migrations) is built on Atlas; no serious alternative exists in the ent ecosystem. Atlas supports only its 2 most recent minor CLI versions — pin CI to whatever is current at ship time, don't let it drift. |

### Supporting Libraries

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| entgo.io/ent/schema/field | (part of ent v0.14.6) | Ent field builders for Tier 1 translated constraints | `MaxLen`/`MinLen`/`Match`/`Min`/`Max`/`Range`/`Positive`/`NotEmpty` all exist as documented. `.Validate(fn)` exists as a typed builder method **only** on `field.String` (`func(string) error`), `field.Bytes` (`func([]byte) error`), and scalar-slice fields via `sliceBuilder[T]` for `T ∈ {int,string,float64}` — see Load-Bearing Findings, this is narrower than "strings only" but still confirms the design doc's conclusion. |
| entgo.io/ent/privacy | (part of ent v0.14.6) | Viewer/authorization layer the generated interceptor chain wires into | `QueryMutationRule`, `ContextQueryMutationRule`, `DenyIfNoViewer`, `AllowIfAdmin`, `Allow`/`Deny`/`Skip` decisions are all real, current API. |
| entgo.io/ent (Interceptor type, `mixin.Interceptors()`) | (part of ent v0.14.6) | otel/logging/metrics interceptor slot in the fixed chain | Distinct from `privacy.Policy` — both are needed: Policy is authorization, Interceptor is cross-cutting (the design doc's "otel" stage). |
| github.com/sebdah/goldie/v2 | current | Golden-file testing for generated code | Matches the entgql/entproto precedent ("golden-file tests for generated code"). Standard `testdata/` fixture dir, `go test -update` regenerates fixtures, `goldie.New()` with functional options. Prefer this over hand-rolled fixture diffing. |
| text/template (stdlib) + entgo.io/ent/entc/gen helpers | stdlib + ent v0.14.6 | Code generation engine for the entc extension's emitted Go | ent's own `entc/gen` — the thing entconnect extends — is built entirely on `text/template` (`gen.NewTemplate`, `TemplateDir/Files/Glob`, `template.FuncMap`). This is the load-bearing precedent: entgql and entproto both register templates the same way. Do not introduce jennifer or go/ast as the primary emission mechanism (see Alternatives Considered). |
| entgo.io/contrib/entproto (read-only reference, not a dependency) | v0.7.0 (Mar 2025) | Architectural precedent for structuring the entc extension | Confirms concrete shape: `type Extension struct { entc.DefaultExtension; ... }`, `func NewExtension(opts ...ExtensionOption) (*Extension, error)`, `func (e *Extension) Hooks() []gen.Hook`, plus a top-level `func Generate(g *gen.Graph) error` invoked from inside a hook. entconnect's entc extension should mirror this exact skeleton. |

## Installation

# entconnect main module (entc extension, runtime interceptors)

# mixinforproto (independent go.mod, minimal deps per the design doc's own constraint)

# dev/test tooling

## Load-Bearing Findings — Design Doc Assumptions Verified/Flagged

| Assumption in design docs | Verdict | Detail |
|---|---|---|
| Ent mixins can declare `Hooks()` | **CONFIRMED** | `ent.Mixin` interface (entgo.io/ent v0.14.6) has `Hooks() []Hook` alongside `Fields`, `Edges`, `Indexes`, `Interceptors`, `Policy`, `Annotations`. Doc comment: "mixin hooks are executed before schema hooks." Exactly what `mixinforproto.md` §4.2 needs for the single Tier-2 CEL hook. |
| "`ent` exposes arbitrary `Validate(fn)` on strings only" | **PARTIALLY WRONG, but conclusion still holds** | `field.String` (`func(string) error`) and `field.Bytes` (`func([]byte) error`) both have typed `.Validate()`, and scalar-slice fields (`int`/`string`/`float64` slices) do too via a generic `sliceBuilder[T]`. But **numeric scalars (Int/Float), Bool, Time, Enum, UUID, and JSON fields have no typed `.Validate()` builder method** — so "not truly string-only" but still "not uniform across proto-mapped field types," which is the actual property the design leans on. Recommend correcting the doc's wording from "strings only" to "a handful of scalar/slice types, not enum/time/bool/JSON/UUID" — the architectural conclusion (single mixin `Hooks()` entry, not per-field `Validate`) is unaffected and still correct. |
| protovalidate-go "exposes constraint enumeration publicly" | **CONFIRMED, with a module-path correction** | `ResolveFieldRules(protoreflect.FieldDescriptor) (*validate.FieldRules, error)`, `ResolveMessageRules`, `ResolveOneofRules`, `ResolvePredefinedRules` are all public top-level functions. But the module has moved: `github.com/bufbuild/protovalidate-go` is the legacy path; the current one is **`buf.build/go/protovalidate`** (v1.2.0, Apr 2026), a BSR-hosted Go module path. Both the design doc and any go.mod must use the new path or resolve to a stale/unsupported release. |
| "protovalidate is CEL; protovalidate-go ships the evaluator" — implying you can just run the constraint expressions yourself | **CONFIRMED, and better than assumed** | `buf.build/go/protovalidate/cel` publicly exports `NewLibrary() cel.Library` (the exact custom CEL functions/extensions protovalidate itself uses) and `RequiredEnvOptions(fieldDesc)`. This means mixinforproto's Tier 2 hook can build a `cel.Env` that is byte-for-byte compatible with protovalidate's semantics, rather than reimplementing protovalidate's custom CEL function library from scratch (a real risk the design doc did not anticipate needing to solve). This is the single most important positive finding of this research pass — it de-risks Tier 2 considerably. |
| `MixinForProto[M proto.Message]` derives a descriptor via `(*new(M)).ProtoReflect().Descriptor()` with no instance | **CONFIRMED** | `proto.Message` has exactly one method, `ProtoReflect() protoreflect.Message`; calling it on a nil/zero-value generated pointer is safe in protobuf-go (documented pattern), and `.Descriptor()` returns the `protoreflect.MessageDescriptor`. The generic-type-parameter trick is sound. |
| cel-go as a stable, unchanging dependency | **FLAG — mid-migration** | `google/cel-go` carries an active README notice that the repository is moving to `github.com/cel-expr/cel-go` (~June 16 2026); latest release v0.31.0 is already under the new org. **Import `github.com/cel-expr/cel-go/cel`, not `github.com/google/cel-go/cel`, in new code** — the old import path will likely become a stale mirror. |

## Alternatives Considered

| Category | Recommended | Alternative | Why Not |
|----------|-------------|-------------|---------|
| Codegen emission mechanism | text/template (matching ent's own entc/gen) | dave/jennifer | Jennifer's typed Go-generation API and automatic import handling are genuinely nicer once codegen complexity grows, and it is a legitimate fallback if hand-managing template imports for handler stubs gets painful. But diverging from ent's own template mechanism means entconnect's `Templates()` extension point can't be layered/overridden the way entgql/entproto's can, and every ent contributor already knows text/template. Revisit jennifer only if a specific generator (e.g. the REST emitter, v0.5) proves unmanageable in templates. |
| Codegen emission mechanism | text/template | go/ast (`go/ast` + `go/printer`) | Excellent for *analyzing or transforming* existing Go (e.g. a future "detect hand-written Manual handlers" tool), poor for *emitting* new code — no comment/formatting preservation story, much more code per generator. Not the right default for handler emission. |
| protovalidate constraint runtime | buf.build/go/protovalidate + cel-expr/cel-go | protoc-gen-validate (PGV) | PGV is protovalidate's explicit predecessor; protovalidate.com itself frames protovalidate-go as "the next generation of protoc-gen-validate" and ships a migration guide. Using PGV in 2026 for a greenfield project is building on a dead end. |
| Migration tooling | ariga.io/atlas | golang-migrate / goose (hand-written SQL migrations) | Both are fine general Go migration runners, but neither has ent's official first-class "diff the schema graph, generate the migration" workflow — you'd be hand-writing every migration, which defeats "everything downstream is generated." |
| REST/OpenAPI precedent for the later plain-REST emitter (v0.5) | Study entoas (entgo.io/contrib/entoas) as a template-registration precedent, but do not depend on it | Depending on entoas directly | entoas derives its OpenAPI spec *from the ent schema*, which is exactly the ent-first direction entconnect exists to invert (per `entconnect.md` §2.3). It's useful only as an extension-architecture reference, never as a runtime dependency. |

## What NOT to Use

| Avoid | Why | Use Instead |
|-------|-----|--------------|
| `github.com/bufbuild/protovalidate-go` (old import path) | Legacy/predecessor path to the current BSR-hosted module; may lag behind on releases and is the wrong thing to pin in a new go.mod. | `buf.build/go/protovalidate` |
| `github.com/google/cel-go` (as a *new* import in code written after mid-2026) | Repo is actively being redirected to a new org; the notice is in the live README as of this research date. | `github.com/cel-expr/cel-go` |
| `github.com/golang/protobuf` | Long-deprecated, explicitly superseded by `google.golang.org/protobuf`; only appears now as a transitive shim in old code. | `google.golang.org/protobuf` |
| protoc-gen-validate (PGV) annotations in new proto files | Superseded by protovalidate/buf.validate; PGV's CEL-less regex-only constraint model is materially weaker than what Tier 1/2 need. | `buf.validate` field options + protovalidate |
| Hand-rolled per-field `field.Validate` wiring for Tier 2 CEL passthrough (i.e., trying to attach one `.Validate()` call per proto field) | Only `field.String`/`field.Bytes`/scalar-slice builders expose a typed `.Validate()`; `field.Enum`, `field.Time`, `field.Bool`, `field.UUID`, `field.JSON` do not. This path is a dead end for a uniform mechanism. | Single mixin-declared `Hooks()` entry iterating changed mutation fields (as the design doc already concludes) |
| jennifer or go/ast as the *primary* handler-emission mechanism from day one | Diverges from the entc/gen precedent your extension is layering on top of; adds a second templating paradigm for contributors to learn with no immediate payoff. | text/template via `gen.Template`/`TemplateDir` |

## Stack Patterns by Variant

- Depend only on `connectrpc.com/connect` + the descriptor/validation toolchain above.
- Do not add `grpc-gateway` or `google.golang.org/genproto`'s HTTP-annotation types until the gateway emitter phase actually starts — the design doc is explicit that these are v0.4/v0.5 concerns, and pulling them in early adds dependency surface to `mixinforproto`'s intentionally minimal module for no benefit (mixinforproto never needs them at all — it doesn't know services exist).
- Its dependency set is already minimal and portable as researched: `ent`, `google.golang.org/protobuf`, `buf.build/go/protovalidate`, `github.com/cel-expr/cel-go/cel`. No buf CLI, no Connect, no genproto annotations — confirmed nothing in the Tier 1/2/3 validation relay mechanism requires them.
- Reuse the same `buf.build/go/protovalidate/cel` environment-construction path used for Tier 2, but resolve `ResolveMessageRules` instead of `ResolveFieldRules`, and bind the constructed-from-mutation `M` as `this` at the message level rather than per-field. No new dependency needed — same library, different resolver call.

## Version Compatibility

| Package A | Compatible With | Notes |
|-----------|------------------|-------|
| connectrpc.com/connect@v1.20.0 | Go ≥1.25 | Bumped minimum Go version in this release (May 2026); pin `go 1.26` in go.mod to stay safely above the floor. |
| buf.build/go/protovalidate@v1.2.0 | github.com/cel-expr/cel-go@v0.31.0 (or the last google/cel-go release before the org move) | `protovalidate/cel.NewLibrary()` returns a `cel.Library` typed against whichever cel-go import path protovalidate itself pins — check protovalidate's own go.mod at the version you lock to, and match your direct cel-go import to the same one, or `cel.Library` type mismatches will surface as compile errors, not runtime ones. |
| ariga.io/atlas CLI@v1.3.0 | entgo.io/ent@v0.14.6 | ent's versioned-migrations doc targets whatever Atlas CLI is current; Atlas's own 2-minor-version support window means this pairing needs a periodic recheck, not a one-time pin. |
| buf CLI@v1.72.0 | connect-go / connect-es plugins | buf.gen.yaml v2 plugin entries (local or `remote:`) work identically regardless of buf CLI minor version post-v1.0 — no known breakage risk here. |

## Sources

- pkg.go.dev/entgo.io/ent, pkg.go.dev/entgo.io/ent/schema/field, pkg.go.dev/entgo.io/ent/entc, pkg.go.dev/entgo.io/ent/entc/gen — fetched directly, confidence MEDIUM (verified via official doc fetch)
- pkg.go.dev/entgo.io/contrib/entproto — fetched directly for Extension/Hooks/Generate signatures, confidence MEDIUM
- pkg.go.dev/google.golang.org/protobuf, pkg.go.dev/buf.build/go/protovalidate, pkg.go.dev/buf.build/go/protovalidate/cel — fetched directly, confidence MEDIUM
- github.com/google/cel-go (README migration notice), github.com/cel-expr/cel-go/releases — fetched directly, confidence MEDIUM
- github.com/connectrpc/connect-go/releases, github.com/ariga/atlas/releases — fetched directly, confidence MEDIUM
- buf.build/docs/reference/images, buf.build/docs/configuration/v2/buf-gen-yaml — WebSearch snippets only, confidence LOW; recommend a direct doc read before finalizing the buf.gen.yaml in the relevant phase
- google.golang.org/genproto/googleapis/api/annotations status, grpc-gateway v2 latest version, atlas migrate diff conventions, nested Go module tagging conventions — WebSearch only, confidence LOW; flagged in-line above where this affects a recommendation

<!-- GSD:stack-end -->

<!-- GSD:conventions-start source:CONVENTIONS.md -->

## Conventions

Conventions not yet established. Will populate as patterns emerge during development.
<!-- GSD:conventions-end -->

<!-- GSD:architecture-start source:ARCHITECTURE.md -->

## Architecture

Architecture not yet mapped. Follow existing patterns found in the codebase.
<!-- GSD:architecture-end -->

<!-- GSD:skills-start source:skills/ -->

## Project Skills

No project skills found. Add skills to any of: `.claude/skills/`, `.agents/skills/`, `.cursor/skills/`, `.github/skills/`, or `.codex/skills/` with a `SKILL.md` index file.
<!-- GSD:skills-end -->

<!-- GSD:workflow-start source:GSD defaults -->

## GSD Workflow Enforcement

Before using Edit, Write, or other file-changing tools, start work through a GSD command so planning artifacts and execution context stay in sync.

Use these entry points:

- `/gsd-quick` for small fixes, doc updates, and ad-hoc tasks
- `/gsd-debug` for investigation and bug fixing
- `/gsd-execute-phase` for planned phase work

Do not make direct repo edits outside a GSD workflow unless the user explicitly asks to bypass it.
<!-- GSD:workflow-end -->

<!-- GSD:profile-start -->

## Developer Profile

> Profile not yet configured. Run `/gsd-profile-user` to generate your developer profile.
> This section is managed by `generate-claude-profile` -- do not edit manually.
<!-- GSD:profile-end -->
