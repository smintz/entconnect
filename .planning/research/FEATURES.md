# Feature Research

**Domain:** Go code-generation library bridging protobuf/ConnectRPC contracts to ent (entgo.io) ORM schemas and generated transport handlers
**Researched:** 2026-08-08
**Confidence:** MEDIUM (official docs + pkg.go.dev cross-checked for entproto/entgql/entoas/entrest/protovalidate/AIP-158/AIP-134; GitHub issue commentary and comparative framing are LOW-confidence secondary color, flagged inline)

## Feature Landscape

### Direct Comparables — Deep Notes

**entproto** (`entgo.io/contrib/entproto`) — ent → proto/gRPC, the exact inversion entconnect rejects.
- Annotation surface: `entproto.Message(PackageName(...))` on schema, `entproto.SkipGen()`/`entproto.Skip()` to opt out; `entproto.Field(num, Type(...), TypeName(...))` **required on every field and edge** (hand-assigned field numbers, #1 reserved for id); `entproto.Enum(map[string]int32, OmitFieldPrefix())` required on every enum field; `entproto.Service(Methods(bitflags))` with `MethodCreate|Get|Update|Delete|List|BatchCreate|All`.
- Generates: a `.proto` file, a `go:generate` directive invoking protoc, and (with `Service()`) a gRPC service implementation wired directly to the ent client — classic gRPC, not Connect.
- Documented/observed limitations (GitHub issues, LOW confidence but consistent across multiple threads): generic schema-parse error messages with no field/line detail; no control over generated `go_package` (fights module layout); not buf-native without workarounds; mutating RPCs write **all** fields regardless of null/zero-ness (no partial-update awareness at all — this is exactly the gap AIP-134/FieldMask exists to close); every field is settable from the request (no field-level write authorization); only unique (1:1) edges supported, no M:N; no cyclic cross-package proto deps.
- Takeaway for entconnect: the annotation-number-bookkeeping tax (`entproto.Field(7)`) and the "all fields writable" hole are the two most concrete anti-patterns to avoid inheriting. entconnect avoids the first by construction (proto already has field numbers; nothing to re-assign in the schema) and must solve the second explicitly (FieldMask/update-mask semantics, see below).

**entgql** (`entgo.io/contrib/entgql`) — ent → GraphQL, the entc-extension structural precedent entconnect explicitly follows.
- Structure: registered as an `entc.Extension` in `entc.go`'s `generate` call, contributing templates and a codegen pass alongside ent's own; output is Go resolvers/types plus a `.graphql` schema fragment, wired into `gqlgen`.
- Paging: `entgql.RelayConnection()` annotation on an edge (or the type) generates the full Relay-spec surface — `<T>Edge`, `<T>Connection`, `PageInfo`, and a `Node` interface requiring a global `id`. The generated `Paginate(ctx, after, first, before, last, orderBy)` method sits directly on the ent query builder. The cursor is an opaque base64 token encoding an **ordering-key + id tuple** — i.e., real keyset pagination wrapped in a cursor abstraction, not naive `OFFSET`. This is the single most important prior-art fact for entconnect's own paging design: ent's generated query builders already know how to do correct keyset pagination; entgql doesn't invent that, it just exposes it through Relay's envelope.
- Filtering/ordering ergonomics: `entgql.OrderField("CREATED_AT")` annotation marks a field sortable (name must match the emitted GraphQL enum, uppercase-snake); `entgql.MultiOrder()` upgrades `orderBy` from a single value to a list; edge-count and edge-field ordering (`EDGE_COUNT`, `EDGE_FIELD` patterns) let a caller sort parents by a child aggregate or a child's field — non-trivial to reproduce and almost certainly out of entconnect's v1 scope, called out here so it isn't quietly promised.
- Template/annotation ergonomics: entgql's templates are `.tmpl` files embedded and overridable via `entc.TemplateDir`/`entc.Templates` — the same mechanism entconnect will need for its handler templates. Field-level config is entirely annotation-driven (`Annotations()` on `ent.Field`/`ent.Edge`), never a separate config file — this is the pattern to mirror for any entconnect-specific per-field knobs (though the design doc's stance is to prefer `Override()` on the mixin over new proto-side annotations; see Anti-Features).

**entoas** (`entgo.io/contrib/entoas`) — spec-only OpenAPI generator; **entrest** (`github.com/lrstanley/entrest`, community) — spec + working handlers, positioned as entoas's "REST replacement" successor and named directly as one of entconnect's status-quo-being-replaced tools.
- entoas is a pure `entc.Extension` that emits `openapi.json` from the ent graph with no runtime component; `ariga/ogent` then generates actual handlers from that spec via `ogen`. Two-hop pipeline (ent → OpenAPI → ogen → handlers).
- entrest collapses that to one hop: ent → OpenAPI spec **and** working `chi`-based HTTP handlers directly. Feature set users expect from a "plain REST" emitter, confirmed here: automatic pagination "where applicable"; `AND`/`OR` predicate filtering via query params, gated per-field by annotations (`WithFilterable`/analogous); sorting via `WithSortable(true)`; eager-loading edge expansion via `WithEagerLoad(true)` to avoid N+1 on nested reads; `ReadOnly`/`Skip`/`Groups`+`OperationGroups` annotations to scope which fields appear on which CRUD operation's request/response schema (create vs update vs read shapes commonly diverge — table stakes for any REST/RPC generator).
- entrest explicitly documents itself as "work in progress, expect breaking changes" — i.e., even the most mature community REST-from-ent generator hasn't stabilized this feature set. This is useful calibration: entconnect's own REST emitter is explicitly v2/deferred in the roadmap, and this research confirms that's the right call — REST-from-ent is still an open, actively-iterated design space, not a solved problem to route around quickly.
- Both tools independently converge on the same primitive: per-field/per-edge annotations gating **inclusion in specific operations** (create/update/read/list). entconnect's CRUD handler generator will need an equivalent even though it reads shape from the *proto* message, not from ent annotations — likely expressed as different proto messages per operation (`CreateOrderRequest` vs `Order` vs `UpdateOrderRequest`), which is already how AIP/Google API design expects proto services to be shaped. This is actually simpler than the ent-first tools' problem because the contract already carries per-operation shape; entconnect doesn't need a new annotation channel for it.

**protoc-gen-validate → protovalidate** — the validation constraint vocabulary entconnect's Tier 1/Tier 2 split is built on top of.
- protoc-gen-validate (PGV) is in maintenance mode; protovalidate is the explicit, buf-endorsed successor (v1.0 shipped 2024). PGV generated per-language validation code from annotations, meaning every rule (e.g., a new UUID format check) had to be reimplemented and kept in sync across Go/Java/C++/etc. protovalidate instead compiles rules to CEL expressions evaluated via reflection at runtime — zero codegen, and by construction the same CEL program produces identical verdicts in every language's evaluator. This is the direct precedent and justification for entconnect's own Tier 2 design (compile CEL once at schema load, execute via a single mixin hook) — entconnect is doing for ent-mutation-time validation exactly what protovalidate does for wire-time validation, and inherits the same "consistent semantics via a shared expression engine, not per-target reimplementation" argument.
- Constraint classes that exist (from protovalidate's standard rule library, consistent with the entconnect design doc's own Tier 1 table): string (`min_len`/`max_len`/`len`/`pattern`/`uuid`/`email`/`hostname`/`uri`/`ip` and other stock format validators), numeric (`gt`/`gte`/`lt`/`lte`, `const`), bytes, bool `const`, enum (`defined_only`, `in`/`not_in`), repeated (`min_items`/`max_items`, `unique`, per-item rules), map (key/value rules, `min_pairs`/`max_pairs`), oneof (`required`), message-level cross-field CEL (`buf.validate.message`), any/timestamp/duration WKT-specific rules. In practice the overwhelmingly common classes real API teams use are: string length/pattern/format, numeric range, `required`/presence, and `enum.defined_only` — i.e., almost exactly entconnect's Tier 1 table. The long tail (custom CEL, cross-field, map/oneof edge cases) is real but rare, which validates the three-tier split: translate the common 80%, execute the CEL long tail exactly rather than trying to grow the translation table forever.

**Contract-first ORM-binding tools outside Go (feature inspiration, not direct comparables).**
- **sqlc** (SQL-first, not proto-first, but structurally the closest analog to entconnect's drift check): the generator validates hand-written `.sql` queries against the live `schema.sql` **at generate time** and fails the build on type/column mismatch, emitting type-safe Go structs with zero runtime reflection. The load-bearing idea to borrow: the generator itself *is* the enforcement point for drift, not a runtime assertion — entconnect's drift check is the proto/ent analog of what sqlc already proves works as a developer-facing pattern (fail fast at `go generate`, not at first query).
- **Prisma + tRPC + zod** (TypeScript, schema-first end-to-end type safety): Prisma schema is the DB source of truth; zod schemas (often generated from Prisma via `zod-prisma-types` or hand-paired) validate at the tRPC procedure boundary; tRPC gives compile-time client/server type sharing with zero codegen step for the RPC layer itself (types flow via TS inference, not codegen). The relevant lesson for entconnect: this ecosystem's biggest recurring pain point (widely reported in the TS community, LOW confidence/anecdotal) is exactly "dual definition drift" — Prisma model vs zod schema silently diverging — which is the same failure mode entconnect's single-source-of-truth design explicitly targets, just solved differently (proto is upstream of both ent and validation, vs. Prisma/zod having no enforced ordering between two independently-hand-maintained schemas). This is strong external validation that "one declarative source, everything else derived and cross-checked" is the right shape for this problem, not entconnect-specific over-engineering.
- **Twirp** (Go, simpler RPC framework, largely superseded by Connect in new projects): worth naming only as calibration — Twirp proved the "generate a service interface + let the developer hand-write the implementation" pattern years before Connect; entconnect's flow-bound/CRUD handler generation is a step beyond that (generating the implementation too, not just the interface), which is the correct evolution given the contract-first premise, but it means entconnect inherits none of Twirp's escape hatches by default — consistent with the design doc's stance that `entconnect.Manual("rpc")` should "feel like the smell it is."

## Concrete Recommendations (the two open questions)

### AIP-158 list pagination over ent — recommended convention

**Recommendation: keyset/cursor pagination (ent's native `Paginate()` machinery), wrapped in an AIP-158-shaped `page_token`/`next_page_token`/`page_size` request/response envelope — not offset pagination.**

- AIP-158 itself is agnostic about *how* a server encodes the token (opaque, URL-safe, not user-parseable, and — critically — `next_page_token` must be the empty string iff the collection is exhausted; that's the only end-of-collection signal a client gets). It does not mandate offset vs keyset.
- The two real-world reference implementations diverge here: `go.einride.tech/aip/pagination` encodes an **offset** + a request-checksum (to invalidate a token if filter/sort params changed underneath it) — simple to implement, but offset pagination degrades under concurrent writes (skipped/duplicated rows) and gets slower with `OFFSET` depth on large tables. entgql, by contrast, already gets **correct keyset pagination for free** from ent's generated query builders — the cursor is an ordering-key + id tuple, not a row count.
- Since entconnect sits directly on top of ent (which already generates correct keyset `Paginate()` machinery for GraphQL via entgql), reinventing offset pagination would be strictly worse engineering for no benefit — it throws away a capability ent already has. The concrete implementation shape: `page_token` decodes to `{last_seen_order_value, last_seen_id}` (protobuf-serialized, base64-encoded, opaque to clients per AIP-158), `page_size` maps to ent's `.Limit()`, and the query re-applies `WHERE (order_field, id) > (last_seen_order_value, last_seen_id)` — which is exactly ent's `Paginate(after: cursor)` semantics already. entconnect doesn't need to write a keyset-pagination engine from scratch; it needs a thin AIP-158-shaped adapter over ent's existing Relay pagination internals (the same internals entgql already exercises), decoupled from GraphQL's `Connection`/`Edge` envelope and re-expressed as flat `repeated T results` + `next_page_token`.
- Corollary: `total_size` (AIP-158 optional field) should be treated as expensive/optional — a `COUNT(*)` is a real query — and only computed if a proto field explicitly requests it, not emitted by default.

### FieldMask vs infer-from-set-fields for partial updates — recommended convention

**Recommendation: `google.protobuf.FieldMask` (`update_mask`, per AIP-134), not infer-from-set-fields — this is not a close call.**

- AIP-134 is the dominant real-world convention: virtually every Google Cloud API (`UpdateApiConfigRequest`, `UpdateBookRequest`, etc.) and every API design guide modeled on it uses `google.protobuf.FieldMask update_mask` on Update requests. AIP-161 formalizes the wire encoding (comma-separated field paths, relative to the resource not the request, lower-camel in JSON).
- The semantics are precisely specified and map cleanly onto entconnect's own sharpest documented edge (proto3 scalar zero-value collapse, flagged as "needs a prominent doc section" in the mixinforproto design doc): AIP-134 says an **omitted** mask means "update all fields that are non-empty on the request" (i.e., infer-from-set — but only as the *fallback* when no explicit mask is given), an **empty** mask means "update nothing," and an explicit mask means "update exactly these paths, using whatever value (including zero) is present in the request for each." This three-way distinction is exactly the tool entconnect needs to make "clear the field back to zero" distinguishable from "field wasn't touched" — the ambiguity the design doc calls out as unresolvable for non-`optional` proto3 scalars is resolved not at the mixin layer (where it's genuinely undecidable) but at the transport layer, by requiring the mask to say so explicitly.
- Pure infer-from-set-fields (no FieldMask at all) cannot express "explicitly clear this field to zero" for non-optional proto3 scalars at all — it's structurally unable to distinguish "caller didn't set `quantity`" from "caller set `quantity = 0` on purpose," which is precisely the ambiguity entconnect's own design doc names as its sharpest edge. Adopting FieldMask doesn't make that ambiguity disappear for `MixinForProto`'s field-derivation logic (still true there), but it means the **handler layer** has an unambiguous instrument to build a correct partial `ent.UpdateOneOrder` mutation from, using only the masked paths — the mixin's zero-collapse problem and the handler's partial-update problem become independently solvable instead of compounding.
- Implementation shape: generated Update handlers read `req.UpdateMask.GetPaths()`, validate each path exists on the resource (protoreflect-checkable at generate time against the message descriptor — a natural drift-check assertion: a mask path that doesn't exist on the message is a codegen-time error, not a runtime 400), and only call the corresponding `.Set*()` builder method on the ent mutation for paths present in the mask. `update_mask` unset → AIP-134's fallback (infer from populated request fields) is a reasonable v1 default for ergonomics, matching grpc-gateway's own PATCH auto-infer behavior — but the wildcard `*` full-replace case should probably be deferred or explicitly rejected in v1 given its documented "silently resets new fields consumers don't know about" footgun, which is a bad fit for entconnect's zero-surprise philosophy.

### Table Stakes (Users Expect These)

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| Clear, field/line-precise codegen errors | entproto's #1 recurring complaint is opaque schema-parse failures; a generator's error UX *is* its DX | LOW–MEDIUM | Wrap protoreflect/descriptor errors with schema field + proto field context; fail at `go generate`, never at first mutation (mixin design already commits to this for panics) |
| Golden-file tests for all generated code | Explicit entgql/entproto precedent named in the design docs; without it, generator changes are unreviewable diffs of generated code | LOW | Table-stakes because the ecosystem already normalized it — deviating would look unmaintained |
| Working `go generate` integration, single command | Every comparable (entproto, entgql, entoas, entrest) hooks into `entc.Extension` + `go generate`; anything else is friction adopters won't tolerate | LOW | Already architecturally committed via the entc extension approach |
| Escape hatch for genuinely custom logic | Every generator in this space (entproto services, entgql resolvers, entrest handlers) eventually needs a "drop to hand-written code" path or adopters fork/abandon it | LOW | `entconnect.Manual("rpc")` already designed; the differentiator is making it visibly a smell (loud in drift-check output), not removing it |
| CRUD list pagination that doesn't degrade at scale | AIP-158 + entgql precedent set the bar; naive `OFFSET` pagination is a known scaling failure users will hit and complain about | MEDIUM | See recommendation above — reuse ent's keyset `Paginate()`, don't build offset pagination |
| Documented partial-update semantics (not just "works") | entproto's "writes all fields regardless of null" is a named limitation users specifically avoid it for; ambiguity here is a trust-breaker | MEDIUM | FieldMask per AIP-134, see recommendation above |
| Docs/migration path from entproto | Design doc names entproto users as the primary adjacent-ecosystem adopter pool; without a mapping doc they have no reason to switch | LOW (doc, not code) | Explicitly listed as Open Question 6 in entconnect.md — a doc, not code, deliverable |
| Per-operation field shape control (create vs update vs read differ) | entoas/entrest (`Groups`/`ReadOnly`/`Skip`) and entproto (all-fields-writable complaint) both converge here from opposite directions | LOW for entconnect specifically | Already solved by contract-first shape (different messages per RPC) — call out explicitly so it isn't rebuilt as a new annotation system |
| Interceptor chain ordering that's predictable and documented | Any generated-handler system with auth+validation+observability needs a fixed, legible order or debugging becomes archaeology | LOW | Already specified: authn → viewer injection → protovalidate → otel → handler |

### Differentiators (Competitive Advantage)

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| Bidirectional drift check (contract ↔ schema ↔ service surface) | No comparable tool in this space does this — entproto/entgql/entoas/entrest all generate *from* one side and never verify the other side didn't diverge; sqlc's schema/query check is the closest external analog, and it's a single-direction check, not bidirectional across three artifact classes | HIGH | This is the single largest differentiator; depends on: message-typed-field↔edge matching, protovalidate constraint fingerprinting, RPC-claim resolution (flow/CRUD/Manual) all landing first |
| Single-definition validation enforced identically at boundary AND storage | Every comparable tool either validates once (transport-only, e.g. entproto has none; protovalidate alone is boundary-only) or twice with drift risk (hand-written ent validators + hand-written proto validators, the exact "dual validation" problem named in entconnect.md §2.3) | HIGH | Depends on MixinForProto's three-tier translation landing before handler generation can claim "no double validation" |
| Flow binding (RPC → entflow workflow dispatch) | No comparable ent extension binds RPCs to a workflow/orchestration layer at all — entproto/entgql/entrest are all pure-CRUD-shaped; this is a category of feature none of them have | MEDIUM–HIGH | Depends on entflow's metadata interface existing first; scoped as v0.3, correctly sequenced after CRUD (v0.2) |
| Zero-codegen runtime mixin (MixinForProto) | entproto/entgql/entoas/entrest are all `entc.Extension`-based codegen; MixinForProto is deliberately the opposite (protoreflect at schema-load time, no generated files) — lets it ship as an independently adoptable micro-module with a much smaller trust/dependency footprint than any comparable tool | MEDIUM | Ships first (v0.1) specifically because it stands alone; this is also a distribution/adoption-funnel differentiator, not just a technical one |
| Async flow status via generated `GetRunStatus` RPC | Comparable tools have no concept of long-running operations at all (entproto/entrest are synchronous-CRUD-only); this closes a gap even hand-rolled Connect services usually solve ad hoc | MEDIUM | v1 is request/response polling; streaming run-state (Connect server streaming) explicitly deferred — correctly scoped as a v2 feature, not MVP |
| Descriptor-driven mask-path validation (compile-time-checked update_mask paths) | No comparable tool validates that an update_mask path actually exists on the message at generate time — this closes the gap between AIP-134's spec and typical hand-rolled implementations, which usually validate mask paths at request time or not at all | LOW–MEDIUM | Natural extension of the drift-check infrastructure once it exists; not a separate subsystem |

### Anti-Features (Commonly Requested, Often Problematic)

Per the design docs' explicit Out-of-Scope list, cross-checked against why comparable tools' equivalents cause exactly the problems named:

| Feature | Why Requested | Why Problematic | Alternative |
|---------|---------------|------------------|-------------|
| Proto-from-ent generation (entproto's direction) | "I already have a schema, just give me the API" — the on-ramp entproto optimizes for | Wrong direction for a contract-first/LLM-assisted pipeline; the schema *imports* the contract types (flow inputs), so generating proto from schema is generating from something that can't compile without the proto that would result — a structural impossibility, not a style choice | Contract is written first (design → proto); schema derives from it via MixinForProto |
| Edge inference from message-typed fields | "Just look at the nested message and make an edge" — seems like free automation | Protos express *shape*; edges carry relational semantics (direction, ownership, cascade, uniqueness) the contract cannot state — inferring these means guessing at cardinality/cascade behavior that has real data-integrity consequences if wrong, silently | Edges stay human-declared in `Edges()`; drift check machine-verifies every skipped message-typed field has a matching declared edge or an explicit `Exclude` |
| Bespoke REST mapping convention | Every REST-from-ORM tool (entoas/entrest/ogent) invents its own URL/verb/filter-query-param conventions, and users of *this* stack will expect the same "just works" REST shape | Inventing a mapping convention duplicates a solved problem — `google.api.http` already is a REST mapping language with an established annotation surface and grpc-gateway/envoy ecosystem support | entconnect only *reads* `google.api.http` annotations already present on the contract; no new convention invented |
| Silent hand-written-handler escape hatch (implicit, undetected) | Every generator eventually gets a request for "let me just write this one handler myself" without ceremony | An undetected hand-written handler is exactly the "leak point for auth, validation, and logic that belongs elsewhere" the design doc names as the status-quo failure mode being fixed | `entconnect.Manual("rpc")` exists, but the drift check surfaces it loudly — visible cost, not invisible bypass |
| Update-time message-level (cross-field) validation via fetch-then-merge | Users will ask "why doesn't my cross-field CEL rule run on PATCH the way it does on Create?" — feels like a gap | Requires an extra DB round-trip per mutation (hidden query cost) and has unclear semantics under concurrent writes (the fetched state may be stale by the time the merged message is validated) | Message-level rules are boundary-only by default; `WithMessageRules(OnCreate)` covers the case where the mutation genuinely is complete; an explicit opt-in `OnUpdateWithFetch` is deferred until real demand justifies the visible cost |
| Custom proto field options as an ad hoc annotation channel (e.g. `(entconnect.field).immutable`) | Feels natural by analogy to entgql/entoas's own annotation-heavy ergonomics (`OrderField`, `ReadOnly`, `Groups`) | Migrates ent-schema semantics into the proto file, re-coupling the contract to an ORM concern it's supposed to be independent of — the opposite of the project's core inversion | `Override(name, field)` on the mixin is the answer; it's ent-side, keeps the proto file ORM-agnostic |
| Offset-based list pagination (the `go.einride.tech/aip` reference-implementation default) | Simpler to implement than keyset; "just add `?page=2`" is the most familiar mental model | Degrades under concurrent writes (skip/dupe rows) and gets slower at depth on large tables; also throws away ent's already-correct keyset `Paginate()` machinery that entgql already proves works | AIP-158-shaped envelope over ent's native keyset pagination (see recommendation above) |
| Infer-from-set-fields as the *only* partial-update mechanism (no FieldMask) | Feels lower-ceremony than requiring clients to build an `update_mask` | Cannot express "explicitly clear a non-optional proto3 scalar back to zero" — collides directly with the design doc's own named sharpest edge (unset vs zero collapse) | `google.protobuf.FieldMask`/`update_mask` per AIP-134, with infer-from-set as only the *documented fallback* when the mask is omitted |

## Feature Dependencies

```
Proto contract (buf generate, FileDescriptorSet)
    └──requires──> nothing upstream (root artifact)

MixinForProto (fields + Tier 1/2/3 validation)
    └──requires──> Proto contract
    └──enables──> CRUD handler generation (needs materialized ent fields to build Get/List/Create/Update/Delete against)
    └──enables──> Drift check (fingerprints Tier 1/2 constraints to compare against hand-declared schema validators)

CRUD handler generation (get/list/create/update/delete)
    └──requires──> MixinForProto (entity fields must exist)
    └──requires──> AIP-158 pagination adapter (List RPC shape)
    └──requires──> FieldMask/update_mask convention (Update RPC shape)
    └──enables──> Drift check for CRUD (RPC-claim resolution needs CRUD binding to exist as a claim source)

AIP-158 pagination adapter
    └──requires──> ent's native keyset Paginate() machinery (already exists in ent core, exercised by entgql)
    └──independent of──> FieldMask convention (parallel, not sequential)

FieldMask / update_mask convention
    └──requires──> Proto message descriptor access (protoreflect, same mechanism MixinForProto already uses)
    └──independent of──> AIP-158 pagination adapter

Flow binding (RPC → entflow)
    └──requires──> entflow's metadata interface (external dependency, separate project)
    └──requires──> CRUD handler generation's RPC-resolution-order machinery (flow-bound is priority 1 in that same resolution loop)
    └──enables──> Async run-status RPC (GetRunStatus)

Drift check (bidirectional)
    └──requires──> MixinForProto (constraint fingerprinting)
    └──requires──> CRUD handler generation (RPC-claim resolution: flow | CRUD | Manual)
    └──requires──> Flow binding (flow-unclaimed-by-RPC warning direction)
    └──enhances──> every other feature (it is the cross-cutting verification layer, not a standalone deliverable)

Golden-file test infrastructure + differential validation harness
    └──requires──> MixinForProto Tier 1/2 (differential harness compares protovalidate verdict vs ent mutation verdict)
    └──enhances──> all codegen features (release gate, not a feature per se)

grpc-gateway annotation pass-through (v1.x) ──enhances──> multi-transport emission
    └──requires──> CRUD/flow handler generation already stable (reads the same descriptor, adds a second emitter)

REST emitter (v2, entrest replacement) ──enhances──> multi-transport emission
    └──requires──> grpc-gateway annotation pass-through (reuses google.api.http parsing)
    └──conflicts with──> inventing a bespoke REST convention (anti-feature; must reuse google.api.http, not compete with it)
```

### Dependency Notes

- **CRUD handler generation requires MixinForProto:** handlers are generated directly against the ent client using entity fields that MixinForProto materializes; there is no path to generating a `Create` handler before the entity's fields exist in the ent schema. This is why MixinForProto is correctly sequenced as v0.1 and CRUD handlers as v0.2 in the existing roadmap — confirmed correct by this research, not just convenient.
- **Drift check requires CRUD handler generation AND flow binding to exist first:** the RPC-claim resolution (flow | CRUD | Manual) that the drift check verifies can't be checked before there are claim sources to check against. The existing v0.2 (CRUD) → v0.3 (flow binding, full bidirectional drift) sequencing in entconnect.md's roadmap is the only viable order; a "full" drift check cannot ship before v0.3, though a CRUD-only partial drift check (RPC unclaimed-by-CRUD-or-Manual) is achievable at v0.2 and worth shipping incrementally rather than holding the whole feature for v0.3.
- **AIP-158 pagination and FieldMask conventions are independent of each other** but both are hard **prerequisites of CRUD handler generation** — List can't be generated without a paging convention decided, Update can't be generated without a partial-update convention decided. Both should be resolved (as design decisions, which this research does above) before CRUD handler generation implementation starts, not discovered mid-implementation.
- **Flow binding conflicts with nothing in CRUD generation** — they're parallel resolution branches in the same RPC-dispatch loop (§3.2 in entconnect.md: flow-bound → CRUD → unmatched/Manual), not sequential features, so within v0.3 they can be built together once entflow's metadata interface lands.
- **REST emitter (v2) requires grpc-gateway annotation pass-through (v1.x) first**, not because REST needs gRPC-gateway at runtime, but because both read the same `google.api.http` annotation surface — building the parser once and reusing it for two emitters avoids the exact "bespoke REST convention" anti-feature by construction (there's no second convention to invent if the REST emitter consumes the same annotations the gateway emitter already parses).

## MVP Definition

### Launch With (v0.1–v0.2, per existing roadmap — confirmed correctly scoped by this research)

- [ ] MixinForProto: scalars/enums/WKTs/presence + `Exclude`/`Override` + Tier 1 translation — table stakes; nothing downstream compiles without it
- [ ] Field/line-precise codegen errors, panicking at schema load — directly answers entproto's #1 complaint; cheap to get right early, expensive to retrofit into error-handling scattered across a growing codebase later
- [ ] Golden-file tests from day one — ecosystem-normalized expectation (entgql/entproto precedent); establishing the harness late means rewriting test infrastructure under time pressure
- [ ] CRUD handler generation (get/list/create/update/delete) against MixinForProto entities
- [ ] AIP-158-shaped list pagination over ent's native keyset `Paginate()` — do not ship offset pagination even as a v0.1 shortcut; it is a known scaling failure mode and a rewrite later, and ent already has the machinery
- [ ] `update_mask`/FieldMask partial-update convention (AIP-134), including descriptor-time mask-path validation — the alternative (infer-only) cannot express the zero-collapse case entconnect's own design doc names as its sharpest edge
- [ ] Fixed interceptor chain (authn → viewer injection → protovalidate → otel → handler)
- [ ] `entconnect.Manual("rpc")` escape hatch, visible in drift-check output — needed from the start so "unmatched RPC" isn't a hard wall for early adopters with genuinely custom needs

### Add After Validation (v0.2.x–v0.3)

- [ ] Tier 2 CEL passthrough + differential validation harness — the release gate proving boundary/storage validation cannot disagree; correctly sequenced after Tier 1 proves the basic mixin shape works
- [ ] Flow binding + `GetRunStatus` — depends on entflow's metadata interface landing; correctly deferred until CRUD is stable, since flow-bound and CRUD RPCs share the same resolution loop and testing that loop is easier with one axis stable
- [ ] Full bidirectional drift check (message-typed-field↔edge, constraint fingerprinting, flow-unclaimed warnings) — the headline differentiator, but structurally cannot exist before its inputs (CRUD claims, flow claims, mixin constraints) all exist
- [ ] `WithMessageRules(OnCreate)`, `AsJSON`, map/oneof handling in the mixin
- [ ] entproto migration doc — pure documentation, cheap, high adoption-funnel value; do this as soon as the v0.2 shape stabilizes enough to describe accurately, don't wait for v1.0

### Future Consideration (v1.x+)

- [ ] grpc-gateway `google.api.http` annotation pass-through — defer until Connect-native handlers are proven; reuses the same descriptor-reader, low risk to defer
- [ ] REST emitter (entrest replacement) — explicitly v2 in the existing roadmap; this research confirms that's the right call, since even the most mature community REST-from-ent tool (entrest) is still "work in progress, expect breaking changes," meaning there's no urgency to compete there early
- [ ] Connect server-streaming run-state transitions (subscriptions) — designed but deferred per Open Question 3; genuinely a v2+ feature, not a gap that blocks v1 adoption
- [ ] `StrictPresence` option (panic on non-optional-scalar ambiguity) — worth having eventually per the mixin design doc's own Open Question 1, but not blocking; the documented-loudly default is an acceptable v1 posture
- [ ] `debug_redact`-based `field.Sensitive()` inference — named as a candidate in the mixin design doc's Open Questions, not yet a committed feature

## Feature Prioritization Matrix

| Feature | User Value | Implementation Cost | Priority |
|---------|------------|---------------------|----------|
| MixinForProto core (scalars/enums/WKT/Tier 1) | HIGH | MEDIUM | P1 |
| Field/line-precise codegen errors | HIGH | LOW | P1 |
| Golden-file test harness | MEDIUM | LOW | P1 |
| CRUD handler generation | HIGH | MEDIUM | P1 |
| AIP-158 keyset pagination adapter | HIGH | MEDIUM | P1 |
| FieldMask/update_mask convention | HIGH | MEDIUM | P1 |
| Fixed interceptor chain | HIGH | LOW | P1 |
| `Manual()` escape hatch | MEDIUM | LOW | P1 |
| Tier 2 CEL passthrough + differential harness | HIGH | HIGH | P2 |
| Flow binding + `GetRunStatus` | HIGH | HIGH | P2 |
| Bidirectional drift check | HIGH (the differentiator) | HIGH | P2 |
| entproto migration doc | MEDIUM (adoption funnel) | LOW | P2 |
| `WithMessageRules(OnCreate)`, `AsJSON`, map/oneof | MEDIUM | MEDIUM | P2 |
| grpc-gateway annotation pass-through | MEDIUM | MEDIUM | P3 |
| REST emitter | MEDIUM | HIGH | P3 |
| Connect server-streaming run-state | LOW (nice-to-have, no current adopter pressure) | HIGH | P3 |
| `StrictPresence` option | LOW | LOW | P3 |

**Priority key:**
- P1: Must have for launch (v0.1–v0.2)
- P2: Should have, add when possible (v0.2.x–v0.3)
- P3: Nice to have, future consideration (v1.x+)

## Competitor Feature Analysis

| Feature | entproto | entgql | entoas / entrest | entconnect's approach |
|---------|----------|--------|-------------------|------------------------|
| Direction | ent → proto (generates contract from schema) | ent → GraphQL schema | ent → OpenAPI (+ handlers in entrest) | proto → ent (contract is upstream; schema derives from it) |
| Field-level config mechanism | `entproto.Field(num)` annotations on ent schema | `entgql.Annotation` (OrderField, Skip, etc.) on ent schema | `entoas.Annotation` (ReadOnly, Skip, Groups) on ent schema | `Exclude`/`Override` on the mixin call, not new proto annotations — keeps proto ORM-agnostic |
| Pagination | offset-only `MaxPageSize` cap, no cursor | Relay cursor / keyset (via ent's native `Paginate()`) | "automatic pagination where applicable" (entrest), unspecified mechanism | AIP-158 envelope over ent's native keyset `Paginate()` — same underlying mechanism as entgql, different wire shape |
| Partial update | none — mutating RPCs write all fields regardless of null/zero | N/A (GraphQL mutations typically define their own input shape per-field) | unspecified in available docs | `google.protobuf.FieldMask`/`update_mask` per AIP-134 |
| Validation | none built in | none built in (relies on ent's own field validators, hand-declared, no contract source) | none built in | single protovalidate source, three-tier translated/executed/boundary-only, enforced identically at boundary and storage |
| Drift detection | none | none | none | bidirectional contract↔schema↔service drift check — the category-defining differentiator among all four |
| Escape hatch for custom handlers | hand-edit generated files (no formal escape hatch) | custom resolvers (formal, expected pattern in gqlgen) | custom handlers alongside generated ones (formal in entrest) | `entconnect.Manual("rpc")`, deliberately visible/loud in drift-check output rather than silent |
| Long-running operation support | none | none (subscriptions exist in GraphQL spec but not ent-generated) | none | `GetRunStatus` RPC + run-reference response (v1), streaming deferred |

## Sources

- [entproto package - pkg.go.dev](https://pkg.go.dev/entgo.io/contrib/entproto) — annotation surface, generated CRUD methods, type mapping table (MEDIUM confidence, official docs)
- [contrib/entproto - GitHub](https://github.com/ent/contrib/tree/master/entproto) — source of truth for the extension
- entproto GitHub issues (search aggregation): error reporting, `go_package` control, buf compatibility, boolean type mapping — (LOW confidence, secondary/anecdotal aggregation, not independently verified per-issue)
- [entgql package - pkg.go.dev](https://pkg.go.dev/entgo.io/contrib/entgql) and [Relay Cursor Connections tutorial - entgo.io](https://entgo.io/docs/tutorial-todo-gql-paginate/) — RelayConnection, OrderField, MultiOrder, cursor encoding (MEDIUM confidence, official docs)
- [entoas package - pkg.go.dev](https://pkg.go.dev/entgo.io/contrib/entoas) and [Announcing entoas - entgo.io blog](https://entgo.io/blog/2021/11/15/announcing-entoas/) (MEDIUM confidence, official)
- [entrest - Getting Started guide](https://lrstanley.github.io/entrest/guides/getting-started/) — REST handler generation, pagination/filtering/sorting/eager-load feature set, explicit WIP status (MEDIUM confidence, official project docs)
- [ariga/ogent - GitHub](https://github.com/ariga/ogent) — spec-to-handler second-hop tool, referenced for the entoas/ogent two-hop pattern
- [Protovalidate migration guide](https://protovalidate.com/migration-guides/migrate-from-protoc-gen-validate/) and [Protovalidate is now v1.0 - Buf blog](https://buf.build/blog/protovalidate-v1) — PGV-to-protovalidate rationale, CEL-based cross-language consistency (MEDIUM-LOW confidence, official but summarized via search)
- [protoc-gen-validate - GitHub](https://github.com/bufbuild/protoc-gen-validate) — maintenance-mode status confirmation
- [AIP-158: Pagination](https://google.aip.dev/158) — full normative pagination spec (MEDIUM confidence, official AIP doc, fetched directly)
- [AIP-134: Standard methods: Update](https://google.aip.dev/134) — update_mask semantics, omitted-vs-empty-vs-wildcard behavior (MEDIUM confidence, official AIP doc, fetched directly)
- [AIP-161: Field masks](https://google.aip.dev/161) — wire encoding details for FieldMask (referenced via search, not independently fetched — LOW-MEDIUM)
- [go.einride.tech/aip/pagination - pkg.go.dev](https://pkg.go.dev/go.einride.tech/aip/pagination) — reference Go implementation of AIP-158, offset+checksum approach (MEDIUM confidence, official package docs)
- [grpc-gateway Patch feature docs](https://grpc-ecosystem.github.io/grpc-gateway/docs/mapping/patch_feature/) — PATCH-body-implies-mask compatibility shim (MEDIUM confidence, official docs)
- [Paginate without contrib/entql · Issue #1708 - ent/ent](https://github.com/ent/ent/issues/1708) and [proposal: Cursor-based pagination · Issue #215 - ent/ent](https://github.com/ent/ent/issues/215) — confirms ent's native cursor `Paginate()` machinery predates and underlies entgql's GraphQL-specific envelope (LOW-MEDIUM, issue discussion)
- [sqlc documentation](https://docs.sqlc.dev/) and [sqlc - GitHub](https://github.com/sqlc-dev/sqlc) — generate-time schema/query drift enforcement pattern, cited as external precedent for entconnect's drift check (LOW confidence, general web search aggregation)
- Prisma + tRPC + zod pipeline characterization — general ecosystem knowledge cross-referenced with search results on schema-drift pain points in that stack (LOW confidence, anecdotal/community pattern, not independently source-verified per claim)

---
*Feature research for: Go proto-to-ent code-generation ecosystem*
*Researched: 2026-08-08*
