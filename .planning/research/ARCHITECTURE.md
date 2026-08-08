# Architecture Research

**Domain:** Go code-generation library — ent entc extension + runtime mixin, protobuf-descriptor-driven
**Researched:** 2026-08-08
**Confidence:** MEDIUM-HIGH (grounded in direct source reads of `ent/ent` and `ent/contrib` on GitHub, plus entgo.io/buf.build/connectrpc.com official docs; no registry-based package-legitimacy signal exists for this ecosystem, so the tooling's automatic confidence classifier floors at LOW — treat the per-claim sourcing below as the real signal, not that floor)

## Verdict: Does the Schema-Load / Codegen Boundary Permit the Specified Drift Check?

**Conditionally yes — but only if `mixinforproto` is designed, from v0.1, to serialize its derivation decisions as `ent.Schema`/`ent.Field` annotations. It is not free. As written in the design docs (mixin exposes `Exclude`/`Override` purely as in-memory `Option` closures with no annotation output), the entc extension cannot see them, and the drift check as specified would silently degrade to checking only what fields exist, not why they don't.**

This is verified against ent's actual internals, not assumed:

1. **`entc generate` loads schemas by compiling and running a small program that imports your `ent/schema` package**, then serializes the result to a `load.Schema` JSON struct (`entgo.io/ent/entc/load/schema.go`). That struct has `Fields []*Field`, `Annotations map[string]any` (schema-level), and each `Field` has its own `Annotations map[string]any`. Crucially, **`Field.Validators` is `int` — a *count*, not the actual validator closures.** Go closures cannot cross a JSON boundary. Only `map[string]any`-shaped annotation data does.
2. `gen.Graph` (what every entc extension's `Hooks()` operates on) is built directly from that JSON. `gen.Type.Annotations` and `gen.Field.Annotations` are the *only* channel through which arbitrary schema-load-time information reaches codegen-time. This is exactly the mechanism `entproto` itself uses: `entproto.Message(...)` and `entproto.Skip()` are `schema.Annotation` values whose `Name()` is a well-known string key (`"ProtoMessage"`, `"ProtoSkip"`); the extension later does `sch.Annotations[MessageAnnotation]` + `mapstructure.Decode` to recover a typed struct from the graph (confirmed by reading `entproto/message.go`, `entproto/skip.go`, `entproto/extension.go` directly).
3. `gen.Type` additionally exposes **`MixedInFields() []int`, `MixedInHooks() []int`, `MixedInPolicies() []int`, `MixedInInterceptors() []int`, and `RuntimeMixin() bool`** (confirmed in `entc/gen/type.go`) — so an entc extension *can* tell "this field came from a mixin" and "this type has runtime-loaded mixin state," but not *which* mixin, nor what options were passed to it, without annotation data.
4. Fields/hooks that carry real Go logic (validators, default funcs, the Tier‑2 CEL hook) are **not** available to the entc extension at all — they only exist as closures inside the same schema package, re-executed a second time, at ent-client-runtime init, by a generated `ent/runtime.go` file (this is what `RuntimeMixin`/`entproto/runtime/extract.go`-style companion packages exist for). The entc extension operates in a third temporal context that sees neither the raw closures nor runtime execution — only whatever crossed into the JSON graph.

**Architectural conclusion — what the design must add, not what it can skip:**

- `MixinForProto` must attach a **schema-level annotation** (e.g. `entconnect.SourceMessage`) recording: the proto message's full name, the complete set of proto field numbers/names present in the descriptor, which of those were passed to `Exclude`, and which were passed to `Override` (by name only — the replacement `ent.Field` value itself doesn't need to cross, since it already becomes a normal derived field the extension can see directly).
- It must attach a **field-level annotation** per derived field (e.g. `entconnect.SourceField{Number, Name, ProtovalidateConstraintIDs []string, Tier2Fingerprint string}`) so the drift check can fingerprint-compare against a schema's hand-declared validators without needing the actual CEL program (constraint IDs and a stable hash of the CEL expression *string* are plain, serializable data — the compiled `cel.Program` is not, and never needs to cross this boundary).
- The annotation struct types should live **in `mixinforproto`** (small, additive to its public API — it stays a zero-codegen runtime library; annotations are just Go structs, not code generation) and the `entc/` extension module **imports `mixinforproto` directly** to decode them, exactly as `entproto`'s extension decodes its own annotation types in-package. This is a legitimate, intentional coupling — unlike the `entflow` boundary (a separate *project* with its own release cadence, hence duck-typed), `mixinforproto` and `entc/` ship from the same repo/design and sharing one annotation contract is what prevents the exact "two sources of truth" failure mode the whole project exists to eliminate. Constraint check: this does not violate `mixinforproto`'s module-isolation rule ("depends only on `ent`, `protobuf`, `protovalidate-go`/`cel-go`") — that rule bounds what `mixinforproto` imports, not who is permitted to import `mixinforproto`.
- Without this, the drift check degrades to: "does a message-typed field exist with no matching edge?" (answerable purely from `gen.Type.Fields` vs. the descriptor, no annotation needed) but **not**: "was this field deliberately excluded, or silently forgotten?" (unanswerable — both look identical as "absent from `gen.Graph`" without the annotation recording the deliberate exclusion).

**Practical implication for phasing:** the annotation contract is not a v0.2-and-later add-on to the drift check — it is a `mixinforproto` v0.1 deliverable, because retrofitting it means every field derivation path (scalars, enums, WKTs, oneofs, `Override`) has to be revisited to also emit annotations. Build the annotation contract alongside the field-mapping logic, not after it.

## System Overview

```
┌──────────────────────────────────────────────────────────────────────────┐
│ 1. CONTRACT (buf workspace)                                              │
│   design → .proto (services, protovalidate constraints, http annotations)│
│   buf lint && buf generate  →  orderv1 Go pkg (types + connect stubs)    │
│                              →  FileDescriptorSet (.binpb, committed,     │
│                                  separate `buf build -o` invocation —     │
│                                  NOT a buf.gen.yaml plugin entry)         │
├──────────────────────────────────────────────────────────────────────────┤
│ 2. SCHEMA LOAD (plain Go, runs INSIDE `entc generate`'s loader program)  │
│   ent/schema/*.go  --imports-->  orderv1 (compile-time dependency)       │
│     Order.Mixin() → mixinforproto.MixinForProto[*orderv1.Order](opts)    │
│       - protoreflect walks the descriptor reachable from the type param │
│       - Exclude/Override validated NOW (panic here, not at mutation)    │
│       - Tier 1 → real field.* builder calls (serializable: Size, Enums) │
│       - Tier 2 → CEL compiled once, wrapped in ONE mixin Hooks() entry  │
│       - annotations written: SourceMessage (schema), SourceField (field)│
│   entc/load serializes the result to JSON (load.Schema) — closures      │
│   (validators, hook funcs, CEL programs) do NOT survive; only counts    │
│   and `map[string]any` annotations do                                   │
├──────────────────────────────────────────────────────────────────────────┤
│ 3. CODEGEN (entc extensions operate on *gen.Graph, built from the JSON) │
│   entc/ extension: entc.DefaultExtension + Hooks() []gen.Hook           │
│     - wraps `next.Generate(g)`; ent's own generation runs FIRST,        │
│       extension work happens as a post-pass over the finished g         │
│     - Descriptor-reader: reads committed FileDescriptorSet (whole-      │
│       service facts: RPCs, http annotations) + gen.Graph annotations    │
│       (per-entity facts: SourceMessage/SourceField, MixedInFields())    │
│     - Drift check runs ONCE here, over the merged view                  │
│     - N emitters (Connect / gateway annotations / REST) consume the     │
│       SAME analysis result — they differ only in output writer          │
│   entflow extension: reads its own flow metadata (name/input/output/    │
│     sync); entconnect matches RPC input type ↔ flow input type here     │
├──────────────────────────────────────────────────────────────────────────┤
│ 4. RUNTIME (generated ent client + generated Connect handlers, in the   │
│    application binary — a THIRD execution of schema Go code)            │
│   ent/runtime.go re-imports ent/schema, re-calls Fields()/Mixin() a     │
│     SECOND time to recover the actual closures (validators, Tier‑2 CEL  │
│     hook, defaults) — matched to gen.Graph positions by index           │
│   generated server wiring: http.ServeMux + connect handlers +           │
│     interceptor chain (authn → viewer injection → protovalidate → otel) │
│     is the ONLY place an *ent.Client is constructed; app code never     │
│     sees the raw client, only a viewer-scoped context                  │
├──────────────────────────────────────────────────────────────────────────┤
│ 5. MIGRATION                                                             │
│   atlas migrate diff (reads the same schema graph) → versioned SQL       │
└──────────────────────────────────────────────────────────────────────────┘
```

## Component Responsibilities

| Component | Responsibility | Typical Implementation |
|-----------|----------------|------------------------|
| `mixinforproto` | Message → `[]ent.Field` + validation relay; nothing else | Runtime `ent.Mixin` impl, generic over `proto.Message`; reads descriptor via `protoreflect`, not a committed `.binpb`. Independent `go.mod`. |
| `entc/` extension | Reads `gen.Graph` (post-build) + committed `FileDescriptorSet`; drives drift check and all transport emitters | `entc.Extension` (embed `entc.DefaultExtension`), single `Hooks()` entry wrapping `next.Generate(g)`, functional-options constructor (`entconnect.NewExtension(opts...)`) — precedent: `entgql.Extension`, `entproto.Extension` |
| `codec/proto` | Adapts `proto.Message` request types into whatever `entflow.Codec` expects | Thin adapter package, ~10 lines per design doc; depends on entflow's metadata interface only, not its runtime |
| `runtime/` | Generated-and-hand-written interceptors: viewer injection, protovalidate at the boundary, otel | `connect.UnaryInterceptorFunc` values composed via `connect.WithInterceptors(...)`, wired once in generated server-construction code |
| Descriptor-reader (inside `entc/`) | Single analysis pass: merges `FileDescriptorSet` (RPC/service/http-annotation facts) with `gen.Graph` (entity/field facts + `mixinforproto` annotations) into one internal model | Analogous to `entproto.Adapter` (`LoadAdapter(*gen.Graph) (*Adapter, error)`), but consuming a descriptor set instead of only building one |
| Emitters (Connect / gateway-annotation / REST) | Consume the descriptor-reader's model; produce transport-specific code only | Each implements a small `Emit(model) ([]byte or files, error)` interface; no independent descriptor parsing, no independent drift logic |
| `ent/runtime.go` (ent-generated, not entconnect's) | Re-executes schema Go code at app-init to recover closures (validators, Tier‑2 hook, defaults) that couldn't cross the JSON boundary | Standard ent codegen output — entconnect does not need to hand-write this, but must understand it exists and why |

## Recommended Project Structure

```
entconnect/
├── go.mod                    # root module: entc/, codec/proto/, runtime/
├── mixinforproto/
│   ├── go.mod                 # independent module — only ent, protobuf, protovalidate-go/cel-go
│   ├── mixin.go                # MixinForProto[M], Exclude, Override, WithMessageRules
│   ├── fieldmap.go             # scalar/enum/WKT → ent.Field translation table
│   ├── validate.go             # Tier1 translation, Tier2 CEL compile + single Hooks() entry
│   └── annotation.go           # SourceMessage / SourceField — the ONLY cross-boundary contract
├── entc/
│   ├── extension.go            # entc.Extension impl: Hooks(), Templates(), Options()
│   ├── analyze/                # descriptor-reader: merges FileDescriptorSet + gen.Graph
│   │   ├── model.go             # shared IR: ServiceModel, RPCModel, BindingKind (CRUD/Flow/Manual)
│   │   └── drift.go             # all drift-check rules, run once over the IR
│   ├── emit/
│   │   ├── connect/             # v0.2: handler codegen against ent client
│   │   ├── gateway/             # v0.4: google.api.http annotation passthrough
│   │   └── rest/                # v0.5: chi/stdlib handlers from http annotations
│   └── templates/               # gen.Template overrides, registered via Extension.Templates()
├── codec/proto/
│   └── codec.go                 # entflow.Codec adapter for proto.Message inputs
└── runtime/
    ├── interceptors.go          # authn / viewer / protovalidate / otel, fixed order
    └── viewer.go                 # context injection helpers
```

### Structure Rationale

- **`mixinforproto/` is a leaf.** It has its own `go.mod` specifically so it never has to know `entc/` or `runtime/` exist — its only public surface beyond the mixin itself is the annotation types, which is why `annotation.go` is called out as its own file: it is the one part of `mixinforproto`'s API that a *different module* is expected to import and decode.
- **`entc/analyze/` is separated from `entc/emit/*`** precisely to enforce the "one descriptor-reader, N emitters" rule structurally, not by convention. If drift-check logic or descriptor parsing leaks into an emitter package, that emitter silently becomes the source of truth for something it shouldn't own, and the other emitters drift from it.
- **`entc/templates/`** mirrors `entgql`'s pattern of a dedicated template file set registered through `Extension.Templates()`/`WithTemplates(...)`, so downstream users can override generated output the same way they already do for `entgql`/`entproto` — a real ecosystem expectation, not a nicety.

## Architectural Patterns

### Pattern 1: Annotation-Crossing (the only channel over the schema-load/codegen boundary)

**What:** Attach a `schema.Annotation` (schema-level) and per-field annotations recording everything an entc extension needs to know about how a mixin derived its output, since closures and raw `Option` values cannot serialize.
**When to use:** Any time a runtime mixin's *behavior*, not just its *output shape*, needs to be checked or consumed at codegen time.
**Trade-offs:** Requires designing the annotation schema up front (see Verdict section) and keeping it forward-compatible (annotations are `map[string]any`, decoded via `mapstructure` — rename fields carefully). In exchange, it is the *only* correct way to do this; there is no reflection-based shortcut, because `gen.Graph` is reconstructed from JSON, not from the live schema package.

**Example (precedent, `entproto/message.go`):**
```go
const MessageAnnotation = "ProtoMessage"

func Message(opts ...MessageOption) schema.Annotation { /* ... */ }

func extractMessageAnnotation(sch *gen.Type) (*message, error) {
    annot, ok := sch.Annotations[MessageAnnotation]
    if !ok {
        return nil, fmt.Errorf("schema %q missing entproto.Message annotation", sch.Name)
    }
    var out message
    err := mapstructure.Decode(annot, &out)
    return &out, err
}
```

### Pattern 2: Post-Generate Hook (analysis-and-emit runs after ent's own codegen, not before)

**What:** `entc.Extension.Hooks()` returns a `gen.Hook` that wraps the next generator, calls `next.Generate(g)` first (letting ent finish writing its own files), and only then runs extension-specific work against the now-complete `*gen.Graph`.
**When to use:** Always, for any extension whose output depends on the fully-resolved graph (field types normalized, edges resolved, all mixins applied) rather than the raw, pre-validation schema.
**Trade-offs:** Extension side effects (writing proto files, handler files) happen after ent's own file writes; if ent's generation fails, your extension code should not run and typically won't (return the error).

**Example (`entproto/extension.go`, verified from source):**
```go
func (e *Extension) hook() gen.Hook {
    return func(next gen.Generator) gen.Generator {
        return gen.GenerateFunc(func(g *gen.Graph) error {
            if err := next.Generate(g); err != nil {
                return err
            }
            return e.generate(g)
        })
    }
}
```

### Pattern 3: Functional-Options Extension Configuration

**What:** Extensions expose a constructor (`NewExtension(opts ...ExtensionOption) *Extension`) with `With*` option functions, rather than a config struct users fill in by hand.
**When to use:** Any entc extension meant for external adoption — this is the uniform shape across `entgql`, `entproto`, and (per design doc precedent already chosen) should be `entconnect`'s shape too.
**Trade-offs:** None significant; it's the ecosystem-standard convention and users of `entgql`/`entproto` will recognize it immediately.

**Example (`entgql`, verified from source):**
```go
func WithTemplates(templates ...*gen.Template) ExtensionOption {
    return func(ex *Extension) error {
        ex.templates = templates
        return nil
    }
}
```

### Pattern 4: Single Descriptor-Reader, N Emitters

**What:** One analysis pass (`entc/analyze`) builds an in-memory IR from `FileDescriptorSet` + `gen.Graph` (which RPCs exist, their binding kind — CRUD/Flow/Manual — their http annotations, their message shapes). Each transport emitter (`Connect`, `gateway`, `REST`) is a pure function of that IR to output files; none of them re-parse descriptors or re-derive bindings.
**When to use:** Any time more than one output format must stay consistent with the same source facts — this is exactly why `entoas` (OpenAPI) and `entproto` each keep their own descriptor-building step isolated from file-writing.
**Trade-offs:** Slightly more indirection for a single-transport MVP (v0.2, Connect-only) than just writing a Connect-specific walker directly — but the design doc's own roadmap (v0.2 Connect → v0.4 gateway → v0.5 REST) makes this pay off within the same project's lifetime, not hypothetically. Building the emitter interface at v0.2 even with one implementation is cheap insurance against the alternative (three divergent ad hoc walkers by v0.5).

### Pattern 5: Viewer-Scoped Context, Never a Privileged Client

**What:** A boundary interceptor extracts identity/authorization info and injects a `viewer.Viewer` into `context.Context` via `viewer.NewContext(ctx, v)`; ent privacy rules read it back via `viewer.FromContext(ctx)`. Handler code (and generated CRUD/flow handlers) only ever receives this context plus a single shared `*ent.Client` — there is no second, unrestricted client anywhere in application code.
**When to use:** Always, for any generated handler that touches the ent client. This is what makes `DenyIfNoViewer()`-style privacy rules actually enforce anything.
**Trade-offs:** Anything that legitimately needs to bypass privacy (seeds, migrations, internal workers) must do so explicitly via `privacy.DecisionContext(ctx, privacy.Allow)` at a well-known, auditable call site — never by holding a separate privileged client.

**Example (ent privacy docs, verified):**
```go
func DenyIfNoViewer() privacy.QueryMutationRule {
    return privacy.ContextQueryMutationRule(func(ctx context.Context) error {
        if viewer.FromContext(ctx) == nil {
            return privacy.Denyf("viewer-context is missing")
        }
        return privacy.Skip
    })
}
```
Generated server wiring order (per entconnect's own design doc, consistent with this pattern): **authn → viewer injection → protovalidate → otel → handler.** Viewer injection must precede protovalidate/otel only in the sense that everything downstream should be able to assume a viewer is present; protovalidate/otel ordering relative to each other is not privacy-load-bearing and can be whichever runs cheaper first (protovalidate before otel avoids emitting spans for requests that fail fast on validation).

## Data Flow

### Canonical Pipeline (as specified in the design docs, verified consistent with ent/buf internals)

```
design (Figma/Claude Design)
   ↓ written by hand/LLM
.proto files (services, protovalidate constraints, google.api.http)
   ↓ buf lint && buf generate
orderv1 Go package (types + connect-go stubs + connect-es TS client)
   ↓ (separate) buf build -o orderv1.binpb --as-file-descriptor-set
FileDescriptorSet (.binpb, committed — NOT a buf.gen.yaml plugin output;
   `buf build` is its own CLI invocation, run alongside `buf generate`)
   ↓ ent/schema/*.go imports orderv1 (compile-time dependency — this is
     WHY proto-first is physically forced, not stylistically chosen)
schema compiles ⟹ MixinForProto[*orderv1.Order](...) runs during
   entc's schema-loader subprocess: protoreflect walk, panics on bad
   Exclude/Override NOW, Tier-1 fields become real ent.Field builder
   calls, Tier-2 CEL compiled once, annotations written
   ↓ entc/load serializes to JSON (load.Schema) — closures dropped,
     annotations (map[string]any) survive
gen.Graph (in-memory, inside the entc generate process)
   ↓ ent's own generation runs FIRST (client, builders, migrations stubs)
   ↓ entconnect's entc/ extension Hooks() run as a post-pass:
     - reads committed FileDescriptorSet (RPC/service/http facts)
     - reads gen.Graph annotations (SourceMessage/SourceField, edges)
     - reads entflow's metadata surface (flow name/input/output/sync)
     - drift check runs ONCE against the merged model
     - N emitters (Connect now, gateway/REST later) write handler code
   ↓ atlas migrate diff reads the finished schema graph
generated: ent client, Connect handlers, ent/runtime.go, SQL migration
   ↓ application binary starts
ent/runtime.go re-imports ent/schema a SECOND time to recover closures
   (validators, Tier-2 hook, defaults) the JSON boundary couldn't carry
   ↓ generated server wiring constructs the ONE *ent.Client, builds the
     interceptor chain (authn → viewer → protovalidate → otel), and
     mounts Connect handlers on an http.ServeMux
runtime requests: viewer-scoped context only, never a privileged client
```

### Key Data Flows

1. **Contract → fields:** one-directional, `mixinforproto` reads `.proto`-derived Go types via `protoreflect`; it never writes back to the contract. Field shape and Tier‑1/Tier‑2 validation are derived once, at schema load.
2. **Contract → transport:** `entc/`'s descriptor-reader reads the *committed* `FileDescriptorSet` for whole-service facts (RPC list, `google.api.http` annotations) because that information doesn't exist on any single `gen.Type` — it's cross-schema, service-shaped data that only the descriptor set (or the live `protoregistry`, see Anti-Patterns) carries.
3. **Schema → codegen (the boundary under scrutiny):** one-directional and lossy by construction — see Verdict section. Only `gen.Graph`'s structural shape and whatever was explicitly annotated crosses.
4. **entflow ↔ entconnect:** meet only inside the application; `entconnect`'s codec/proto adapts proto-message-shaped flow inputs, and the entc extension matches RPC input types to flow input types by type identity at codegen time (entflow itself never imports proto/descriptor machinery — the metadata interface is enough).

## Scaling Considerations

This is a code-generation library, not a runtime service — "scale" here means corpus size (number of schemas/services/RPCs) and codegen wall-clock time, not request throughput.

| Scale | Adjustments |
|-------|--------------------------|
| 1 schema, 1 service | Everything in this doc still applies unchanged — there is no simpler mode to fall back to, which is a feature (no special-casing the small case). |
| Dozens of schemas/RPCs | Descriptor-reader analysis (`entc/analyze`) should run once and be shared across all emitters, not re-run per emitter per schema — this is exactly why Pattern 4 exists before it's strictly necessary. |
| Large monorepo, many `.proto` packages | `buf build`'s `FileDescriptorSet` grows with `--include-imports`; watch codegen wall-clock and consider scoping the committed descriptor set to just the packages entconnect needs to read, not the whole buf workspace. |

### Scaling Priorities

1. **First bottleneck: codegen wall-clock as schema count grows.** Mitigate by keeping the descriptor-reader a single pass (Pattern 4) and by not re-invoking `protoreflect` walks per emitter.
2. **Second bottleneck: annotation-contract drift as `mixinforproto` evolves.** Because the annotation struct (Verdict section) is the single channel of truth across a module boundary, any breaking change to it breaks the drift check silently unless versioned. Treat the annotation contract itself as a semver-relevant public API of `mixinforproto`, not an internal detail.

## Anti-Patterns

### Anti-Pattern 1: Expecting the entc extension to read `Option` values directly off the mixin

**What people do:** Assume that because `MixinForProto(Exclude("etag"), Override("status", ...))` is called in the same repo, the entc extension can somehow introspect those arguments.
**Why it's wrong:** The extension runs against `gen.Graph`, rebuilt from a JSON serialization of the schema-load result. Go closures and unexported option state never reach that JSON (verified: `entc/load/schema.go`'s `Field.Validators` is an `int`, not a function list).
**Do this instead:** Serialize the *decisions* (excluded names, overridden names, constraint IDs, fingerprints) into `schema.Annotation` values written by the mixin itself. See Pattern 1 and the Verdict section.

### Anti-Pattern 2: Treating "field absent from `gen.Graph`" as self-explanatory

**What people do:** Drift-check logic that says "this proto message field has no matching ent field or edge → error," without distinguishing "never even attempted" from "explicitly excluded on purpose."
**Why it's wrong:** Both produce the identical `gen.Graph` shape (the field simply isn't there). Without the exclusion showing up in an annotation, the drift check cannot tell a deliberate design decision from a bug, and will either over-error (flagging legitimate `Exclude`s) or under-error (silently allowing real drift if the check is loosened to compensate).
**Do this instead:** The schema-level `SourceMessage` annotation must carry the full set of proto fields the message had and which of those were excluded/overridden, not just the resulting field list.

### Anti-Pattern 3: Sourcing whole-service facts (RPC list, http annotations) from `protoregistry`/linked-in Go types instead of the committed descriptor set

**What people do:** Since the schema package already imports `orderv1` for flow input types, it's tempting to read service/RPC/http-annotation facts via the global `protoregistry` at codegen time instead of loading the committed `.binpb`.
**Why it's wrong:** `protoregistry` is populated by *whatever happens to be linked into the process*, which is a function of Go's import graph, not a canonical service inventory — a service with no proto-message-shaped flow input anywhere in the schema package's import graph would never register, and the entc extension would silently see zero RPCs for it. The `FileDescriptorSet` is the design doc's own resolved answer for exactly this reason (Open Question 2, resolved).
**Do this instead:** Use `protoreflect` off the linked-in type *only* for `MixinForProto`'s own per-message field derivation (where the type parameter guarantees the message is linked in by construction). Use the committed `FileDescriptorSet` for anything service-shaped or cross-schema.

### Anti-Pattern 4: Assuming `buf.gen.yaml` can emit the `FileDescriptorSet` as just another plugin

**What people do:** Try to add a `descriptor_set`-style entry to `buf.gen.yaml` alongside the Go/Connect plugins.
**Why it's wrong:** Verified against buf's own docs — a `FileDescriptorSet`/buf "image" is produced by `buf build -o <path>` (optionally `--as-file-descriptor-set` for a pure `FileDescriptorSet` rather than buf's extended "image" format), a *separate* CLI invocation from `buf generate`'s plugin pipeline. There is no first-party plugin-based path to this inside `buf.gen.yaml`.
**Do this instead:** Two invocations in the build script/Makefile: `buf generate` (for Go types + connect stubs) and `buf build -o path/to/orderv1.binpb --as-file-descriptor-set --exclude-source-info` (for the committed descriptor). Keep both in the same `buf lint && buf generate` step of the canonical pipeline so they can't drift relative to each other.

### Anti-Pattern 5: Relying on local `go.work`/`replace` state to validate that the nested module actually builds for external consumers

**What people do:** Develop `mixinforproto` and the root `entconnect` module together via a `go.work` file (or a `replace` directive in the root `go.mod`) and consider CI green because it builds locally.
**Why it's wrong:** `go.work` and `replace` directives resolve modules from local paths; they mask exactly the failure mode that matters for a "ships as two independently adoptable modules" project — a consumer who only has `mixinforproto` published at some tagged version and pulls the root module via its real `go.mod require` line.
**Do this instead:** Use `go.work` for local development ergonomics, but run at least one CI job that builds the root module with `GOWORK=off` (or in a checkout without the workspace file) against the last-tagged `mixinforproto/vX.Y.Z`, so tag-based consumption is actually exercised. Nested-module release tags must use the `<subdir>/vX.Y.Z` prefix convention (e.g. `mixinforproto/v0.1.0`) for the Go module proxy to resolve them.

## Integration Points

### External Services

| Service | Integration Pattern | Notes |
|---------|---------------------|-------|
| `buf` CLI / BSR | Two separate invocations feeding the pipeline: `buf generate` (Go types + connect stubs + TS client) and `buf build -o ... --as-file-descriptor-set` (committed descriptor). No plugin-based path exists for the latter inside `buf.gen.yaml`. | Both must run in the same pipeline step so they can't drift relative to each other; CI should fail if the committed `.binpb` doesn't match a fresh `buf build` of the current `.proto` sources (a staleness check, not part of this doc's scope but a direct consequence of "descriptor set committed"). |
| ConnectRPC (`connect-go`) | Generated handlers implement the connect-go service interface; interceptor chain built via `connect.WithInterceptors(...)` in generated server-construction code, wired to the `http.ServeMux` alongside handler registration. | Order matters: interceptors closer to the front of the `WithInterceptors(...)` list run first on the way in, last on the way out (confirmed: connect-go's `newChain` reverses the slice so the first-listed interceptor acts first). |
| Atlas (migrations) | Reads the same schema graph ent already builds; entconnect does not need its own migration logic, only needs `mixinforproto`-derived fields to be ordinary `ent.Field`s so Atlas sees them like any other field. | No special integration surface beyond "don't do anything that makes derived fields invisible to normal ent schema introspection." |
| entflow | Metadata-only interface crossing at the application layer; `codec/proto` is the one entconnect-side adapter that touches entflow's `Codec` shape. | Duck-typed or tiny shared `entflow/meta` package — this is the one boundary in the system that is deliberately *not* using the annotation-crossing pattern, because it's a cross-project boundary with independent release cadence, not a cross-module boundary within one project. |

### Internal Boundaries

| Boundary | Communication | Notes |
|----------|---------------|-------|
| `mixinforproto` ↔ `entc/` | Go import (entc/ → mixinforproto, one direction) + `schema.Annotation` data flowing through `gen.Graph` | This is the boundary under scrutiny in the Verdict section. Treat the annotation struct as `mixinforproto`'s real public contract with the rest of the project, versioned accordingly. |
| `entc/analyze` ↔ `entc/emit/*` | In-process Go values (the shared IR), no serialization | Emitters must not re-parse descriptors or re-derive bindings; if an emitter needs a fact the IR doesn't carry, add it to the IR, don't reach around it. |
| `runtime/` ↔ generated handlers | Interceptor chain composed once in generated server-construction code; handlers receive only a viewer-scoped `context.Context` and the shared `*ent.Client` | No handler code should ever construct its own `*ent.Client` or bypass the interceptor chain — this is a structural invariant, not a style preference, because it's what makes privacy enforcement actually total. |
| `codec/proto` ↔ entflow | Implements whatever `entflow.Codec` interface expects for `proto.Message` inputs | Kept intentionally tiny (~10 lines per design doc) — if it grows, that's a signal entconnect is absorbing entflow responsibilities it shouldn't. |

## Sources

- [entc package - entgo.io/ent/entc (pkg.go.dev)](https://pkg.go.dev/entgo.io/ent/entc)
- [Extensions | ent (entgo.io/docs/extensions/)](https://entgo.io/docs/extensions/)
- [gen package - entgo.io/ent/entc/gen (pkg.go.dev)](https://pkg.go.dev/entgo.io/ent/entc/gen)
- `entgo.io/ent` GitHub source, read directly: [`entc/load/schema.go`](https://github.com/ent/ent/blob/master/entc/load/schema.go) (the `load.Schema`/`load.Field` JSON shape — confirms `Validators` is a count, `Annotations` is `map[string]any`), [`entc/gen/type.go`](https://github.com/ent/ent/blob/master/entc/gen/type.go) (`MixedInFields`, `MixedInHooks`, `MixedInPolicies`, `MixedInInterceptors`, `RuntimeMixin`)
- `ent/contrib` GitHub source, read directly: [`entproto/extension.go`](https://github.com/ent/contrib/blob/master/entproto/extension.go) (post-generate `gen.Hook` composition), [`entproto/adapter.go`](https://github.com/ent/contrib/blob/master/entproto/adapter.go) (`LoadAdapter(*gen.Graph)` descriptor-reader precedent), [`entproto/message.go`](https://github.com/ent/contrib/blob/master/entproto/message.go) and [`entproto/skip.go`](https://github.com/ent/contrib/blob/master/entproto/skip.go) (annotation-crossing pattern, `mapstructure.Decode`), [`entproto/runtime/extract.go`](https://github.com/ent/contrib/blob/master/entproto/runtime/extract.go) (companion runtime package pattern), [`entgql/extension.go`](https://github.com/ent/contrib/blob/master/entgql/extension.go) (functional-options `Extension` configuration, `WithTemplates`)
- [Privacy | ent (entgo.io/docs/privacy/)](https://entgo.io/docs/privacy/) — viewer context injection, `DenyIfNoViewer`, `privacy.DecisionContext`
- [Interceptor Pattern | connectrpc/connect-go (DeepWiki)](https://deepwiki.com/connectrpc/connect-go/5.1-interceptor-pattern) and [connect-go interceptor_example_test.go](https://github.com/connectrpc/connect-go/blob/main/interceptor_example_test.go) — `WithInterceptors` chain ordering
- [Buf images (buf.build/docs/reference/images/)](https://buf.build/docs/reference/images/) — `buf build -o`, `--as-file-descriptor-set`, confirms descriptor-set emission is a separate CLI step, not a `buf.gen.yaml` plugin
- [Descriptors - Buf Docs](https://buf.build/docs/reference/descriptors/) — `--include_imports`, `FileDescriptorSet` semantics
- [Go Modules - A Guide for monorepos (Grab Engineering)](https://engineering.grab.com/go-module-a-guide-for-monorepos-part-1) and [Tutorial: multi-module workspaces (go.dev)](https://go.dev/doc/tutorial/workspaces) — nested `go.mod`, `go.work`, `replace` directive scoping, `<subdir>/vX.Y.Z` release tag convention

---
*Architecture research for: entconnect (Go ent + protobuf codegen library)*
*Researched: 2026-08-08*
