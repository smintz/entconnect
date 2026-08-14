# Phase 3: Validation Fidelity - Research

**Researched:** 2026-08-14
**Domain:** ent hook ordering, protovalidate/CEL evaluation internals, reverse proto/ent conversion, differential-testing harness design
**Confidence:** HIGH — every highest-risk unknown 03-CONTEXT.md flagged was resolved by reading the actual pinned dependency source (not training memory, not a websearch summary) inside this sandbox's Go module cache, plus reading entconnect's own repo files. No web search was used; every claim below is either `[VERIFIED: <path>:<lines>]` against a real file this session, or explicitly `[ASSUMED]` where it is not.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

**Requirement-text amendments this phase owes** (must be made as part of this phase's work, not silently diverged from):
1. ROADMAP Phase 3 SC1 says the hook evaluates "residual field-scoped protovalidate CEL rules (the long tail Tier 1 can't translate)." **D-02** makes protovalidate authoritative for *every* rule on an in-scope field, translated ones included, because VAL-07 is otherwise unachievable. SC1 must be reworded to match.
2. VAL-05 says "The CEL hook evaluates only fields changed by the mutation." **D-06** makes the scope operation-dependent — all derived fields on Create, changed-only on Update. VAL-05 must be reworded to match.

**D-01 (Violation Identity, VAL-06/VAL-07):** The mutation-time hook returns protovalidate's own `*protovalidate.ValidationError` — the exact type `protovalidate.Validate()` produces and `connectrpc.com/validate` builds its `connect.Error` from. No new `mixinforproto` dependency (the generated `buf/validate` package is already direct, non-test). Accepted cost: a protovalidate major bump becomes a `mixinforproto` breaking change. Reversibility: one-way.

**D-02:** The hook evaluates the full protovalidate field-rule set for every in-scope field — translated and residual alike. Native ent builders from Tier 1 stay but are shadowed (mixin hooks wrap the mutator; ent's field validators run inside it). Obligates the SC1 amendment above. Reversibility: costly.

**D-03:** Per-field rule evaluation over the in-scope set only — never validate a whole reconstructed message. Resolve rules per field descriptor (the same `ResolveFieldRules` path Tier 1 already uses) and evaluate only for fields in scope. Prevents phantom violations on FieldMask-gated partial Updates. Message-level rules stay boundary-only (VAL-08).

**D-04:** VAL-07's "identical" is entity-relative. Both layers validate the entity message (`Order`), both emit path `name`. PIPE-06 compares `protovalidate.Validate(order)` against the ent mutation verdict — same object, same paths. Explicitly rejected: excluding field path from comparison, or rewriting storage paths to request-relative ones in generated handlers.

**D-05:** A reverse conversion table covering all derivation kinds — the mirror of `fieldmap.go`'s forward table (scalar, enum, each WKT, map-as-JSON, message-as-JSON). Keep forward and reverse adjacent in the same file or an obviously paired one. The JSON→`dynamicpb` leg is its own task with its own tests. Explicitly rejected: binding ent's Go values directly via a cel-go type adapter (breaks WKT semantics like `.seconds` on Timestamp).

**D-06:** Scope is operation-dependent. On Create, the hook checks all derived fields, whether or not the mutation explicitly set them (derived fields carry `Default(zero)`, so an unset field on Create still persists as a real value the boundary validates). On Update, changed-only per D-03. Considered and rejected: treating ent-applied defaults as "changed" (was recorded as unverified whether hooks run before defaults are applied — **this phase's research has now verified defaults ARE applied before hooks fire on Create; see Common Pitfalls Pitfall 2** — D-06's decision does not depend on that premise being true).

**D-07 (USER DECISION — against Claude's recommendation; cost recorded deliberately):** The hook is a hybrid: protovalidate's own evaluator for standard rules, a local `cel.Env` built from `buf.build/go/protovalidate/cel`'s `NewLibrary()` + `RequiredEnvOptions(fd)` for residual CEL, compiled once at schema load. Claude recommended instead calling `protovalidate.Validate()` on a synthesized `dynamicpb` message restricted to in-scope fields (one evaluator, identity by construction) — **not chosen**. Consequences the planner must carry: (1) two evaluation paths must produce identical output — PIPE-06 is the mechanism that keeps this safe, not optional/deferrable; (2) `mixinforproto` takes a direct cel-go dependency (D-16); (3) error construction must be single-pathed even though evaluation is not — both paths build `*validate.Violation` protos handed to one shared constructor. Reversibility: costly.

**D-08:** The hybrid's split line is drawn by rule kind, not by Phase 1's `ResidualIDs`. The local env handles only `(buf.validate.field).cel` expressions. Every standard rule goes to protovalidate's evaluator, including ones Tier 1 declined to translate (float gt/lt, required-on-non-presence). Planner note: do not repurpose `ResidualIDs` as the routing input.

**D-09:** Unenforceable rules split by cause: uncompilable CEL expression → panic at schema load; unbindable field kind → record as still-unenforced/boundary-only in provenance (visible to Phase 5's drift check).

**D-10:** `WithMessageRules(OnCreate)` panics at schema load if any message rule references a field ent does not have (excluded or underivable). Walk each message rule's CEL expression for field references at load time. `WithMessageRules` is currently not declared (`mixinforproto/option.go:11`) — do not re-litigate the rest of §4.3's shape (boundary-only default, Create-only opt-in, `OnUpdateWithFetch` excluded from v1).

**D-11:** VAL-09's ordering guarantee is pin-and-document, take no position. Research establishes the real order empirically; docs state it; a test asserts it so an ent upgrade that changes it fails CI. Document the consequence honestly: if validation runs before the privacy policy, an unauthorized caller receives a constraint violation rather than a denial, leaking which constraints exist — a recorded, accepted property. **This phase's research corrects this: privacy always occupies `Hooks[0]` ahead of any mixin/schema hook whenever a Policy is declared anywhere on the schema — see Security Domain below.**

**D-12:** A runtime reverse-conversion failure on real data fails closed: the mutation aborts, and the error is explicitly not a protovalidate violation — no constraint ID is synthesized. Distinct from D-09 (schema-load "can this kind ever be bound?" vs. runtime "did this particular value bind?").

**D-13:** Two harnesses, deliberately: a driverless sweep in `mixinforproto` (no DB, exhaustive random differential run) and a real-client wiring proof in the root module (smaller suite, real ent client on `modernc.org/sqlite`, proving ent genuinely invokes the hook in the real mutation path). Neither substitutes for the other.

**D-14:** PIPE-06's randomness is seeded deterministically from the corpus message name; the seed plus the failing value print on failure. A separate opt-in job with a time-based seed explores new value space without making required CI flaky.

**D-15:** The VAL-11 parity gate compares an explicit named set of modules across both `go.mod`s: `buf.build/go/protovalidate`, `buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go`, cel-go, `google.golang.org/protobuf`, and `entgo.io/ent`. The gate must run in a context where the two modules resolve independently (the existing `GOWORK=off` CI job).

**D-16 (RESEARCH OBLIGATION — factual, not a preference):** The cel-go import path must be whatever `buf.build/go/protovalidate@v1.2.0`'s own `go.mod` pins, matched exactly in both modules. **Resolved by this research: `github.com/google/cel-go v0.28.0`, import path `github.com/google/cel-go/cel` — see Summary point 1 and Standard Stack.** CLAUDE.md's `github.com/cel-expr/cel-go@v0.31.0` claim is confirmed wrong and must be corrected as part of this phase's work.

### Claude's Discretion

1. **Shared violation constructor** (D-07 consequence 3) — that both evaluation paths funnel through one error-building function is Claude's mitigation for the hybrid choice, not a user instruction. The single highest-leverage guard on D-07; should survive review.
2. **Where `runtime.MapError` learns about `*protovalidate.ValidationError`** — `runtime/errormap.go` today sends anything unrecognized to `CodeInternal`; adding a case for protovalidate's error type is required work, wherever the planner puts it. See Code Examples.
3. **Whether the reverse table lives in `fieldmap.go` or a paired new file** — D-05 requires adjacency, not a specific filename.

### Deferred Ideas (OUT OF SCOPE)

- **`OnUpdateWithFetch`** — Update-time message-rule enforcement via fetch-then-merge. Already named and deliberately excluded from v1 by `mixinforproto.md` §4.3. Not this phase.
- **Time-seeded exploratory differential job** — D-14 provisions it as a separate opt-in job outside required CI. Whether it actually runs on a schedule is a CI-policy call, not a Phase 3 deliverable.
- **Collapsing D-07's hybrid to a single evaluator** — even though this research confirms the per-field `Filter` mechanism D-07's rejected alternative needed does exist in protovalidate v1.2 (see Summary point 3), this is explicitly not a reason to revisit D-07 this phase; the user has chosen the hybrid. Worth revisiting later if PIPE-06 shows the two paths trivially agreeing.
- **Privacy-before-validation ordering** — D-11 pins and documents ent's actual order rather than fighting it. Not a live concern this phase — see Security Domain's correction.

**Explicitly NOT in this phase** (later phases; must not be pulled forward): flow binding (Phase 4), the bidirectional drift check (Phase 5, though this phase *records* unenforceable-rule provenance per D-09 so Phase 5 can report on it), the Order/Inventory reference application (Phase 5), grpc-gateway/plain-REST emitters, `OnUpdateWithFetch` message-rule enforcement.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|--------------------|
| VAL-04 | Residual field-scoped protovalidate CEL is compiled once at schema load and evaluated at mutation time by a single mixin-declared hook covering all field types (amended scope per D-02: full field-rule set, not residual-only) | Pattern 1 (precompiling via `WithMessages`+`WithDisableLazy`), Pattern 2 (Filter-based scoping), Assumption A1 |
| VAL-05 | The CEL hook evaluates fields per D-06's operation-dependent scope (amended from "changed-only") | Pitfall 2/4 (`defaults()` vs `UpdateDefault()` timing, verified via `create.tmpl`/`update.tmpl`) |
| VAL-06 | Schema-layer violations carry the protovalidate constraint ID and message and are consumable as a structured error outside any RPC context | D-01's `*protovalidate.ValidationError` reuse; Code Examples' shared violation constructor |
| VAL-07 | A caller cannot tell whether a violation was caught at the boundary interceptor or at the storage layer | Pattern 2 (Filter for identical field scope), Pitfall 1 (double-evaluation risk that could break this identity if unhandled) |
| VAL-08 | Message-level (cross-field) rules are boundary-only by default; `WithMessageRules(OnCreate)` opts in | Pattern 2 (`ShouldValidate(msg, msg.Descriptor())` gates message-level rules independently of field-level ones — verified via `message.go`) |
| VAL-09 | Mixin hook ordering relative to schema-declared hooks and privacy policies is documented and covered by a test that fails if ent changes it | Architecture Patterns diagram + Pitfall 3 (verified via `entc/gen/template/runtime.tmpl`, `create.tmpl`) — full ordering traced and quoted |
| VAL-10 | The boundary interceptor constructs its validator once per process, never per request | Already satisfied per Phase 2 D-13 per CONTEXT.md; `protovalidate.GlobalValidator`'s `sync.OnceValues` laziness confirmed via `validator.go:29` |
| VAL-11 | CI fails when the protovalidate/cel-go versions resolved by `mixinforproto` and by `entconnect` diverge | Standard Stack's version-verification table (baseline confirmed already in sync); D-15's named module list |
| PIPE-05 | A conformance corpus covers every field-mapping rule and every protovalidate constraint class, golden-asserted against derived fields | Recommended Project Structure, existing `corpus_test.go`/`testdata/*.golden` pattern (Phase 1 precedent, confirmed present) |
| PIPE-06 | A differential harness generates random values per corpus message and asserts `protovalidate verdict == ent mutation verdict` for field-scoped rules | Summary point 7, Code Examples' `fakeDriver`, D-13's two-harness split |

</phase_requirements>

## Summary

All seven highest-risk unknowns 03-CONTEXT.md listed as "the planner cannot sequence work without answers" are now resolved by direct source inspection, not inference:

1. **cel-go import path (D-16):** `buf.build/go/protovalidate@v1.2.0`'s own `go.mod` requires `github.com/google/cel-go v0.28.0` directly, and its `cel/library.go` and top-level `validator.go` both `import "github.com/google/cel-go/cel"`. CLAUDE.md's `github.com/cel-expr/cel-go@v0.31.0` claim is confirmed wrong; both entconnect's and mixinforproto's `go.mod` already resolve `github.com/google/cel-go v0.28.0` (indirect) — exactly matching D-16's claim. **Action: `mixinforproto` must add `github.com/google/cel-go v0.28.0` as a direct (non-indirect) require entry, importing `github.com/google/cel-go/cel` — not `github.com/cel-expr/cel-go/cel`.**
2. **`buf.build/go/protovalidate/cel` API surface:** `NewLibrary() cel.Library` and `RequiredEnvOptions(fieldDesc protoreflect.FieldDescriptor) []cel.EnvOption` both exist exactly as CONTEXT.md described, confirmed by reading `cel/library.go` lines 52 and 504.
3. **Per-field filtering in protovalidate v1.2 (D-03):** protovalidate ships a public `Filter`/`FilterFunc`/`WithFilter(filter Filter) ValidationOption` mechanism (`filter.go`, `option.go:88-92`) that gates evaluation at **message, field, and oneof granularity** via `ShouldValidate(message protoreflect.Message, descriptor protoreflect.Descriptor) bool`, applied per-call via `Validator.Validate(msg, protovalidate.WithFilter(f))`. This is real and directly usable for D-03's scope-restriction and D-08's "message-level rules stay boundary-only" requirement. **Important nuance the plan must account for:** the filter operates at whole-field granularity, not at rule-kind granularity within a field — see Pitfall 1 below.
4. **ent hook/policy/field-validator ordering (VAL-09/D-11):** fully traced through `entgo.io/ent@v0.14.6`'s own codegen templates (not the generated output of any one app — the templates that produce every app's `ent/` package). Verified order, Create and Update alike:
   `defaults()` → `Hooks[0]` (combined privacy Policy, mixin+schema, **only installed when any Policy exists**) → `Hooks[1..N]` (mixin-declared hooks, then schema-declared hooks) → `sqlSave` → `check()` (generated field validators, incl. Tier 1's native ent builders). **This means privacy runs before any mixin hook whenever a Policy exists on the schema — the information-leak scenario D-11 hedged about ("if validation runs before the privacy policy...") does not occur under ent's actual generated wiring.** This is a correction to D-11's framing that should be recorded, not a reason to revisit D-11's "pin-and-document" resolution.
5. **`Mutation.Fields()`/`Default()` timing (D-06):** `defaults()` runs **before** `withHooks(...)` is invoked at all — confirmed in `builder/create.tmpl` lines 40-51. On Create, every field carrying `.Default(...)` (which is every derived non-optional scalar per Phase 1 D-26) is materialized into the mutation as "set" **before any hook, mixin or schema, ever runs.** On Update, `defaults()` only applies `.UpdateDefault(...)`-tagged fields (`builder/update.tmpl` lines 231-242) — MixinForProto's derived fields never carry `UpdateDefault`, so an untouched derived field genuinely remains unset in the mutation at hook time on Update. This means D-06's Create-side "all derived fields" requirement is **already satisfied by `mutation.Fields()` alone** once defaults() has run — no separate "walk every SourceField" enumeration is needed as a second code path; `mutation.Fields()` naturally differs in content between Create and Update because of *this* mechanism, not because the hook needs to branch on `m.Op()`.
6. **Reverse conversion mechanics (D-05):** every conversion direction mixinforproto's derivation kinds need has a direct, stable protobuf-go API: `timestamppb.New(t time.Time) *Timestamp` for WKT; `protoreflect.ValueOfEnum` + `EnumDescriptor.Values().ByName(...).Number()` for enum; `dynamicpb.NewMessage(md)` + `protojson.Unmarshal(b []byte, m proto.Message) error` for AsJSON/`google.protobuf.Value` JSON round-tripping; `Message.NewField(fd)`/`Map.Set(...)` for the scalar-map-as-native-Go-map case. All confirmed present in `google.golang.org/protobuf@v1.36.11`'s actual source this session.
7. **Driverless differential harness (PIPE-06/D-13):** `mixinforproto/go.mod` confirmed to still carry **no** DB driver in its require block. `entgo.io/ent`'s `dialect.Driver` is a small, hand-implementable Go interface (`ExecQuerier` + `Tx` + `Close` + `Dialect`, `dialect/dialect.go:36-45`) requiring zero additional dependency — a fake driver satisfying it lets the harness drive the **real** generated `client.T.Create().Save(ctx)` pipeline (hooks, `check()`, `sqlSave`) end-to-end, entirely in-memory, with no sqlite/postgres/etc. import. `mixinforproto/internal/boundarytest/` already establishes the pattern of a real `entc.LoadGraph` schema-load subprocess with no driver at all for schema-shape assertions; the new differential harness is a sibling package using a real generated `ent.Client` with the fake driver instead.

**Primary recommendation:** Implement the hybrid exactly as D-07/D-08 specify, using `protovalidate.New(WithMessages(...), WithDisableLazy())` built once at schema-load/`Hooks()`-construction time for the standard-rule half (satisfying VAL-04's literal "compiled once" for that half), a locally-built `cel.Env` from `pvcel.NewLibrary()` + `pvcel.RequiredEnvOptions(fd)` compiled once per residual-CEL field for the custom-CEL half, and treat "how do the two halves interact on a field that carries both a standard rule and a custom CEL rule" as its own task with its own test — CONTEXT.md already anticipated this needs isolation (see Pitfall 1).

## Project Constraints (from CLAUDE.md)

Actionable directives from `./CLAUDE.md` relevant to this phase, checked for compliance:

- **Dependency direction:** `mixinforproto` depends only on `ent`, `google.golang.org/protobuf`, and `protovalidate-go`/`cel-go` — this phase's cel-go promotion to a direct dependency is explicitly within this boundary, not a violation of it.
- **Module isolation:** `mixinforproto` must remain shippable standalone. D-13's "no DB driver in `mixinforproto`'s require block" is this constraint applied to the differential harness specifically — confirmed compatible (Summary point 7).
- **Codegen-free mixin:** `MixinForProto` is plain Go executing at schema-load time — no entc extension, no committed descriptor files for the mixin's own validation hook. Schema load *is* the check phase (D-09's schema-load panics for uncompilable CEL are exactly this principle applied to residual rules).
- **Use `buf.build/go/protovalidate`, not `github.com/bufbuild/protovalidate-go`:** already the case in both `go.mod`s — confirmed unchanged by this phase.
- **CLAUDE.md's cel-go stack-table entry is wrong** (`github.com/cel-expr/cel-go@v0.31.0`) and must be corrected to `github.com/google/cel-go v0.28.0` as part of this phase's work, per D-16 and this research's Summary point 1 — this is a **required correction**, not optional cleanup, since the wrong entry would mislead anyone reading CLAUDE.md about which package to import.
- **Do not use `github.com/google/cel-go` "as a new import written after mid-2026"** per CLAUDE.md's own "What NOT to Use" table — this directive is itself the thing D-16 disproves for this specific case: `buf.build/go/protovalidate@v1.2.0` (current, actively maintained, as of this research date) pins `github.com/google/cel-go`, not `cel-expr/cel-go`, so importing `google/cel-go` here is correct and required, not a violation of the spirit of that warning (which was written to avoid a *stale, abandoned* org, and `google/cel-go` is not that in reality — CLAUDE.md's own factual premise about the migration timeline does not match what the pinned dependency actually requires).
- **golden-file tests for generated code follow the entgql/entproto precedent, using `github.com/sebdah/goldie/v2`** — PIPE-05's corpus extension should follow the exact same pattern already established in `mixinforproto/corpus_test.go`/`testdata/*.golden`.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Standard-rule (structural) validation at mutation time | Database/Storage (ent mixin hook) | API/Backend (boundary interceptor, pre-existing) | Storage-layer enforcement is this phase's entire purpose (VAL-04/VAL-07); the boundary interceptor already does this (Phase 2) and is unaffected. |
| Residual custom-CEL validation at mutation time | Database/Storage (ent mixin hook, local `cel.Env`) | API/Backend (boundary, via protovalidate's own evaluator) | Same rule, two independently-compiled evaluators — identity is proven by PIPE-06, not by construction (D-07's accepted cost). |
| Message-level (cross-field) rule enforcement | API/Backend (boundary interceptor) | Database/Storage (opt-in only, `WithMessageRules(OnCreate)`) | VAL-08: boundary-only by default; storage enforcement is a deliberate, narrow opt-in. |
| Violation → wire error identity | API/Backend (error mapping) + Database/Storage (mixin hook) | — | Both layers must construct/propagate the *same* `*protovalidate.ValidationError` Go type (D-01) so `runtime.MapError` maps them identically. |
| Reverse conversion (ent Go value → `protoreflect.Value`) | Database/Storage (mixin, schema-load-time-compiled functions) | — | Purely a mixin-internal concern; nothing above the mixin ever sees the intermediate proto value. |
| Differential proof (protovalidate verdict == ent verdict) | Database/Storage (`mixinforproto` driverless sweep) + a thin root-module wiring proof | — | D-13: two separate harnesses, neither substitutes for the other. |
| CI dependency-parity gate (VAL-11) | Build/CI tooling | — | Not a runtime capability; a `GOWORK=off` CI job comparing two `go.mod`s. |

## Package Legitimacy Audit

This phase introduces **no new external package name** — it promotes `github.com/google/cel-go` from an already-present *indirect* dependency (present in both `go.mod`s today, transitively required by `buf.build/go/protovalidate` and `entc/gen`'s own toolchain) to a **direct** dependency of `mixinforproto`. The `package-legitimacy check` seam in this environment only supports `npm|pypi|crates` ecosystems; Go is not covered, so the automated gate does not apply here. In its place, this session performed a **stronger** verification than a registry lookup: the actual package source was downloaded via the Go module proxy and read directly.

| Package | Registry | Verified state | Verdict | Disposition |
|---------|----------|-----------------|---------|-------------|
| `github.com/google/cel-go` | Go module proxy (`proxy.golang.org`) | v0.28.0 already resolved (indirect) identically in both `go.mod`s **today** (`go.mod:30`, `mixinforproto/go.mod:23`); confirmed to be the exact version `buf.build/go/protovalidate@v1.2.0` itself requires (`protovalidate@v1.2.0/go.mod`, read directly this session) | OK | Approved — promote to direct in `mixinforproto/go.mod`; no version change needed, only the require-block placement and the removed `// indirect` marker. |
| `github.com/cel-expr/cel-go` (CLAUDE.md's claim) | — | Not resolved by either `go.mod` today; not a dependency of `buf.build/go/protovalidate@v1.2.0` (its `go.mod` names `github.com/google/cel-go`, not this path) | N/A — not actually a dependency | **Do not add.** CLAUDE.md's stack table is factually wrong on this point (per D-16) and must be corrected as part of this phase's work, not treated as a target to move toward. |

**Packages removed due to SLOP verdict:** none.
**Packages flagged as suspicious [SUS]:** none.

## Standard Stack

No new library *names* enter the stack this phase — every dependency below is already present in one or both `go.mod`s. The change is (a) cel-go's promotion to direct, and (b) how the already-present protovalidate APIs get *used* (evaluator + filter + CEL-env construction, not just `ResolveFieldRules` enumeration as Tier 1 uses it).

### Core

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `buf.build/go/protovalidate` | v1.2.0 | Standard-rule evaluator (`Validator.Validate` + `WithFilter`), `ResolveFieldRules`/`ResolveMessageRules` (already used by Tier 1) | Already the pinned dependency (`go.mod:7`, `mixinforproto/go.mod:9`) — `[VERIFIED: mixinforproto/go.mod:9]`. |
| `buf.build/go/protovalidate/cel` | v1.2.0 (same module, `cel/` subpackage) | `NewLibrary()`/`RequiredEnvOptions(fd)` for the local residual-CEL `cel.Env` | `[VERIFIED: /root/go/pkg/mod/buf.build/go/protovalidate@v1.2.0/cel/library.go:52,504]` — read directly this session. |
| `github.com/google/cel-go` | v0.28.0 | CEL compile/eval engine underneath both protovalidate's own evaluator and the local `cel.Env` | `[VERIFIED: /root/go/pkg/mod/buf.build/go/protovalidate@v1.2.0/go.mod]` — this is what protovalidate itself requires; must become a **direct** dependency of `mixinforproto` per D-07. Import path is `github.com/google/cel-go/cel`, confirmed via that package's own imports, **not** `github.com/cel-expr/cel-go/cel`. |
| `google.golang.org/protobuf` (`types/dynamicpb`, `encoding/protojson`, `types/known/timestamppb`, `reflect/protoreflect`) | v1.36.11 | Reverse conversion (D-05): building `protoreflect.Value`s from ent's Go-typed mutation values | Already pinned; all needed symbols (`dynamicpb.NewMessage`, `protojson.Unmarshal`, `timestamppb.New`, `protoreflect.ValueOfEnum`) confirmed present `[VERIFIED: /root/go/pkg/mod/google.golang.org/protobuf@v1.36.11/...]`. |
| `entgo.io/ent` | v0.14.6 | Mixin `Hooks()`, `Mutation` interface, `dialect.Driver` (for the driverless harness's fake driver) | Already pinned; `dialect.Driver`'s shape confirmed `[VERIFIED: /root/go/pkg/mod/entgo.io/ent@v0.14.6/dialect/dialect.go:36-45]`. |

### Supporting

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `entgo.io/ent/entc` (`entc.LoadGraph`) | v0.14.6 | Schema-load-time proof harness pattern, reused from Phase 1's `boundarytest` | Only if PIPE-06's driverless sweep also needs a real schema-load subprocess proof (it likely reuses a real, generated `ent` fixture package instead — see Common Pitfalls). |
| `modernc.org/sqlite` | v1.56.0 | D-13's real-client **wiring** proof, root module only | Already present in root `go.mod:16`; **must not** be added to `mixinforproto/go.mod` (that would violate the "no DB driver" property D-13 protects). |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Hand-implemented `dialect.Driver` fake, for the driverless harness | An actual in-memory SQLite driver, even in `mixinforproto` | Would add a real DB-driver dependency to `mixinforproto`'s `go.mod`, destroying the exact "no DB driver in require block" property D-13 is written to preserve. Not viable — `dialect.Driver` is a small enough interface that faking it costs nothing. |
| `protovalidate.Validate(msg, WithFilter(f))` for the standard-rule half | Hand-rolling a field-by-field evaluator from `ResolveFieldRules`'s raw `*validate.FieldRules` structs (i.e., re-implementing gt/gte/lt/lte/min_len/etc. evaluation by hand) | This is precisely the reimplementation risk D-08 explicitly warns against ("hand it float comparison and presence semantics to hand-reimplement — rules protovalidate already implements correctly"). Use protovalidate's real evaluator via the Filter mechanism, not a hand-rolled one. |

**Installation:**
```bash
cd mixinforproto
# already resolved indirectly; this promotes it to direct — no new module fetch needed
go get github.com/google/cel-go@v0.28.0
go mod tidy
```

**Version verification:** confirmed directly against the downloaded module source this session (not `npm view`/`pip index`-equivalent, since Go's registry *is* the source tree via the module proxy):
```
buf.build/go/protovalidate@v1.2.0/go.mod:  require github.com/google/cel-go v0.28.0
go.mod (root):                              github.com/google/cel-go v0.28.0 // indirect
mixinforproto/go.mod:                       github.com/google/cel-go v0.28.0 // indirect
```
All three agree today — the D-15 CI parity gate (VAL-11) starts from a passing baseline, not a pre-existing skew.

## Architecture Patterns

### System Architecture Diagram

```
                         CREATE / UPDATE MUTATION
                                   |
                                   v
                    +---------------------------+
                    |  Builder.Save(ctx) [ent]   |
                    |  1. defaults()              |   <- Default(zero) materialized
                    |     (Create: Default() runs;|      into mutation HERE, before
                    |      Update: only           |      any hook fires (VERIFIED)
                    |      UpdateDefault())        |
                    +---------------------------+
                                   |
                                   v
                    +---------------------------+
                    |  withHooks(...) [ent]       |
                    |  Hooks[0] = privacy.Policy   |   <- only if ANY Policy exists
                    |    .EvalMutation (mixin+     |      on the schema (mixin or
                    |     schema, combined)        |      schema-declared)
                    +---------------------------+
                                   |
                                   v
                    +---------------------------+
                    |  Hooks[1..N]                |
                    |  = mixinforproto's single    |   <- THIS PHASE lands here.
                    |    Hooks() entry             |      D-02: full field-rule set,
                    |    (protovalidate hybrid:     |      translated + residual.
                    |     standard rules -> real    |      D-03/D-06: field scope is
                    |     protovalidate evaluator    |     operation-dependent.
                    |     via Filter; custom CEL ->  |
                    |     local cel.Env)             |
                    |  then schema-declared Hooks()  |
                    +---------------------------+
                                   |
                                   v
                    +---------------------------+
                    |  sqlSave() [ent]             |
                    |  -> check()                  |   <- generated field validators
                    |     (Tier 1's native builders |      (MinLen/MaxLen/Range/...)
                    |      fire HERE — pre-empted    |      SHADOWED: mixinforproto's
                    |      by the hook above if it   |      hook already rejected any
                    |      already rejected)         |      violation before this runs
                    +---------------------------+
                                   |
                                   v
                              real SQL write

     ---------------------------------------------------------------------
     BOUNDARY (Phase 2, unaffected):  Connect request -> authn -> viewer
     injection -> connectrpc.com/validate (protovalidate.GlobalValidator,
     built once, lazily, and cached) -> otel -> handler -> (calls into the
     mutation path above)
     ---------------------------------------------------------------------

     DIFFERENTIAL HARNESS (PIPE-06), two independent instances:
       (a) mixinforproto/internal/<harness>: fake dialect.Driver (hand-
           written, zero new deps) + real generated ent.Client -> drives
           the EXACT same Save() pipeline above, no real DB.
       (b) internal/entconnecttest/<fixture>: real modernc.org/sqlite ent
           client -> proves the hook is actually wired into a real app.
```

### Recommended Project Structure

```
mixinforproto/
├── validate.go          # existing Tier 1 translation (unchanged)
├── fieldmap.go           # existing forward derivation table (unchanged)
├── reverse.go             # NEW (D-05): mirror of fieldmap.go, one function
│                          #   per derivation Kind ("scalar"/"optionalScalar"/
│                          #   "enum"/"wkt"/"scalarMap"/"asJSON") converting
│                          #   an ent mutation's Go value back to a
│                          #   protoreflect.Value against the original fd.
├── hooks.go               # NEW: the single mixin Hooks() entry (D-02),
│                          #   schema-load-time construction of the
│                          #   precompiled protovalidate.Validator (standard
│                          #   half) and the per-field local cel.Env map
│                          #   (residual half); D-07 consequence 3's shared
│                          #   violation->ValidationError constructor.
├── messagerules.go        # NEW: WithMessageRules(OnCreate) opt-in (D-10),
│                          #   schema-load field-reference walk + panic.
└── internal/
    └── difftest/           # NEW (D-13): driverless differential sweep —
                             #   fake dialect.Driver + real generated
                             #   ent.Client, seeded random values (D-14),
                             #   corpus-driven (PIPE-05/PIPE-06).

runtime/
└── errormap.go            # gains a *protovalidate.ValidationError case
                            #   (Claude's Discretion item 2)

internal/entconnecttest/
└── <new fixture>/          # NEW (D-13): real-client wiring proof, sqlite-
                             #   backed, mirrors internal/entconnecttest/update/
```

### Pattern 1: Precompiling the standard-rule evaluator "once at schema load"

**What:** VAL-04 says "compiled once at schema load," but `protovalidate.New()`'s default behavior is to build evaluators **lazily on first encounter** of a message type (`validator.go`'s `newBuilder(env, cfg.disableLazy, ...)` — laziness is a builder-level configuration bit, default `false`/lazy). To satisfy VAL-04's literal wording for the standard-rule half of the hybrid, warm the validator up explicitly at construction time.
**When to use:** In the mixin's `Hooks()`/setup path, once, at schema-derivation time — not lazily on first request.
**Example:**
```go
// Source: buf.build/go/protovalidate@v1.2.0/option.go:31-58 (verified this session)
v, err := protovalidate.New(
    protovalidate.WithMessages(exampleMsg), // *new(M), the same zero-value
                                             // trick MixinForProto[M] already
                                             // uses to get a descriptor
    protovalidate.WithDisableLazy(),        // fail fast if a rule doesn't
                                             // compile, and never build on
                                             // first request
)
```

### Pattern 2: The field-level Filter for scope restriction (D-03/D-06/D-08)

**What:** `protovalidate.WithFilter` gates which message/field/oneof descriptors get evaluated per-call. `message.EvaluateMessage` checks the filter against `msg.Descriptor()` **before** running message-level (cross-field) evaluators, independently of each field's own filter check inside `field.EvaluateMessage` (`message.go:44-52`, `field.go:69-71`, both read this session). This is exactly the mechanism needed to keep message-level rules boundary-only (VAL-08) while still checking in-scope fields.
**When to use:** Building the reconstructed message for the standard-rule evaluator pass.
**Example:**
```go
// Source: buf.build/go/protovalidate@v1.2.0/filter.go, option.go:88-92 (verified this session)
scope := protovalidate.FilterFunc(func(msg protoreflect.Message, d protoreflect.Descriptor) bool {
    if d == msg.Descriptor() {
        return false // VAL-08: message-level/cross-field rules never run here
    }
    fd, ok := d.(protoreflect.FieldDescriptor)
    if !ok {
        return false // oneof descriptors: not this phase's concern
    }
    return inScope(fd) // D-06: all derived fields on Create, mutation.Fields() on Update
})
err := v.Validate(reconstructed, protovalidate.WithFilter(scope))
```

### Pattern 3: Reverse conversion, one function per derivation Kind

**What:** Mirror `fieldmap.go`'s per-`Kind` switch (D-05), each case producing a `protoreflect.Value` from the ent mutation's Go-typed value.
**When to use:** Immediately before handing a field to either evaluator (standard or local-CEL), for every in-scope field.
**Example:**
```go
// Sources verified this session:
//   timestamppb.New:      types/known/timestamppb/timestamp.pb.go:195
//   dynamicpb.NewMessage:  types/dynamicpb/dynamic.go:80
//   protojson.Unmarshal:   encoding/protojson/decode.go:29
//   ValueOfEnum:           reflect/protoreflect/value_union.go:171
//   EnumValueDescriptors.ByName: reflect/protoreflect/type.go:601
switch class { // mixinforproto's own SourceField.Kind strings — fieldmap.go:158-193
case "wkt": // e.g. google.protobuf.Timestamp
    t := entValue.(time.Time)
    return protoreflect.ValueOfMessage(timestamppb.New(t).ProtoReflect())
case "enum":
    name := entValue.(string)
    evd := fd.Enum().Values().ByName(protoreflect.Name(name))
    if evd == nil {
        return protoreflect.Value{}, fmt.Errorf("enum string %q not in descriptor", name) // D-12: fail closed, not a fabricated constraint
    }
    return protoreflect.ValueOfEnum(evd.Number())
case "asJSON": // MIX-09, or google.protobuf.Value
    raw := entValue.(json.RawMessage)
    dyn := dynamicpb.NewMessage(fd.Message())
    if err := protojson.Unmarshal(raw, dyn); err != nil {
        return protoreflect.Value{}, err // D-12
    }
    return protoreflect.ValueOfMessage(dyn)
}
```

### Anti-Patterns to Avoid
- **Hand-rolling a "structural rules" evaluator from `ResolveFieldRules`'s raw fields:** protovalidate already has a real evaluator; use it via `Validate(msg, WithFilter(...))`, do not re-derive gt/gte/lt/lte/min_len semantics by hand (D-08's explicit rationale).
- **Adding a real DB driver to `mixinforproto/go.mod` "just for the harness":** destroys the standalone-module property D-13 exists to protect. Use a hand-written `dialect.Driver` fake instead.
- **Constructing the local `cel.Env` per-request:** VAL-04 says "compiled once at schema load" — build it once per residual-CEL field when the mixin's `Hooks()` is assembled, not inside the returned hook closure.
- **Treating the CEL-`Filter` field/message granularity as rule-kind granularity:** `ShouldValidate` cannot turn off *just* a field's custom-CEL rule while leaving its structural rules on (or vice versa) — see Pitfall 1.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|--------------|-----|
| Standard protovalidate rule evaluation (gt/gte/lt/lte/min_len/pattern/format/required) at the storage layer | A field-by-field evaluator switch over `ResolveFieldRules`'s raw structs | `protovalidate.Validator.Validate(msg, WithFilter(scope))` | protovalidate's real evaluator already implements every edge case (overflow, presence semantics, IP/hostname/URI procedural formats); D-08 explicitly forbids reimplementing this. |
| A CEL environment "compatible with" protovalidate's own semantics | A hand-assembled `cel.NewEnv` with custom functions copied from protovalidate's source | `cel.NewEnv(cel.Lib(pvcel.NewLibrary()), pvcel.RequiredEnvOptions(fd)...)` | `buf.build/go/protovalidate/cel` exports exactly this construction (verified: `validator.go:74-79` builds protovalidate's *own* internal env this exact way — it is the reference implementation, not a hint). |
| Enum string→number resolution | A hand-maintained `map[string]int32` per enum field | `fd.Enum().Values().ByName(name).Number()` | Descriptor is already loaded; no reason to duplicate its contents. |
| JSON→proto message hydration for AsJSON/`google.protobuf.Value` fields | A custom JSON walker writing `dynamicpb.Message` fields one at a time | `protojson.Unmarshal(raw, dynamicpb.NewMessage(md))` | protojson already handles every JSON→proto mapping rule (numbers-as-strings for int64, base64 for bytes, `null`, etc.) — a hand-rolled walker will silently diverge on at least one of these. |
| A fake SQL-executing driver for the differential harness | A miniature real SQL engine | `dialect.Driver`'s 4-method interface, hand-stubbed to return canned rows/insert IDs | The interface is deliberately small; ent doesn't need a real database to prove hooks fired and validators ran — it needs `Exec`/`Query` to return *something* so `sqlSave` doesn't error before `check()` even gets called (though on a rejected mutation, execution never reaches the driver at all). |

**Key insight:** Every "don't hand-roll" item above has a real, exported entry point in a library already pinned in this repo's `go.mod` — the risk this phase carries is entirely in *wiring these together correctly* (D-07's hybrid convergence, D-06's operation-dependent scope), not in needing to implement missing library functionality.

## Common Pitfalls

### Pitfall 1: `WithFilter` cannot split a field's standard rules from its custom-CEL rule
**What goes wrong:** A field carrying *both* a standard rule (e.g. `string.min_len`) and a `(buf.validate.field).cel` rule gets its **entire** compiled evaluator run when `ShouldValidate` returns `true` for that field descriptor (`field.go:69-96`, read this session) — there is no sub-field granularity. If the plan calls `protovalidate.Validate(msg, WithFilter(scope))` for the "standard rules" half AND separately evaluates that same field's CEL expression through the local `cel.Env` for the "residual" half (per D-08's literal routing-by-rule-kind), **the field's custom CEL rule gets evaluated twice, by two different engines.**
**Why it happens:** D-08's routing rule ("local env handles only cel expressions, standard rules go to protovalidate's evaluator") is a statement about which rule *kinds* the local env is *for*, not a guarantee that protovalidate's own evaluator can be made to skip a field's CEL sub-rule while still running its structural sub-rules.
**How to avoid:** This is exactly the D-07/D-08 convergence risk CONTEXT.md already flags as needing "its own task with its own tests." Two workable resolutions for the plan to choose between (not decided here — genuinely an implementation-mechanics choice within the locked D-07/D-08 boundaries):
  (a) Accept double evaluation and **deduplicate** at the shared violation constructor (D-07 consequence 3) by `(RuleId, FieldPath)` before returning the merged `*protovalidate.ValidationError` — simplest, and PIPE-06 will catch it if dedup logic is wrong.
  (b) Exclude any field carrying a custom-CEL rule from the `WithFilter` scope entirely (known at schema-load time from `ResolveFieldRules(fd).GetCel()`), and route that field's *entire* rule set (structural + custom CEL) through the local `cel.Env`, using `pvcel.RequiredEnvOptions(fd)` — this is legitimate because the local env is built from the *same* `NewLibrary()` protovalidate itself uses, so structural-rule CEL expressions (which is what `min_len` etc. compile down to internally) would evaluate identically either way.
**Warning signs:** PIPE-06 reporting a false disagreement where the ent-side verdict rejects for the *same* underlying reason twice (two violations with the same `RuleId` and `FieldPath` but the CEL-sourced one and the protovalidate-sourced one carrying slightly different `Message` text).

### Pitfall 2: `defaults()` already answers "does Create need special-case scope logic" — don't build two mechanisms
**What goes wrong:** Building a separate "enumerate every derived SourceField" code path for Create (distinct from `mutation.Fields()` for Update) when `mutation.Fields()` alone already contains every `Default()`-bearing field on Create, because `defaults()` ran and called the setter before any hook fired.
**Why it happens:** D-06's own text treats "all derived fields on Create" as requiring separate handling from "changed-only on Update," which is correct as a *specification*, but the *mechanism* achieving it can be the same `mutation.Fields()` call in both cases — the different *content* comes from ent's own `defaults()`/`UpdateDefault` split, not from the hook needing an `if m.Op() == ent.OpCreate { enumerate all SourceFields } else { mutation.Fields() }` branch.
**How to avoid:** Verify with a real test (not just the pipeline diagram) that `mutation.Fields()` inside a mixin hook, on Create, already contains every derived non-optional field — do this as an early plan task before building any Create/Update branching, since if it holds, a large amount of D-06's implementation collapses to one code path with no `Op()` check at all.
**Warning signs:** If a real hook-time `mutation.Fields()` call on Create does *not* include a Default()-bearing, caller-unset field, this pitfall's premise is wrong and D-06's original two-path design is required after all — treat this as the very first thing implemented and tested in this phase, since several other tasks assume its answer.

### Pitfall 3: privacy Policy position depends on whether *any* Policy exists anywhere on the schema, not just on MixinForProto
**What goes wrong:** Assuming the mixin hook always runs at a fixed `Hooks[N]` index, or assuming privacy ordering is independent of whether the *application's* schema (not just the mixin) declares a `Policy()`.
**Why it happens:** `runtime.tmpl:99-110` (read this session) only installs the `Hooks[0] = policy-eval` wrapper `{{- with $policies := $n.PolicyPositions }}` — i.e., only when `$n.NumPolicy > 0`. If neither the mixin nor the schema declares a `Policy()`, there is no privacy stage at all, and the mixin's validation hook is `Hooks[0]`.
**How to avoid:** The VAL-09 test must cover **both** configurations (a schema with a `Policy()` and one without) and assert the mixin hook's *relative* position to whatever else is present, not an absolute index — an absolute-index test breaks the moment an app adds or removes a policy, which is exactly the ent-upgrade-shaped regression VAL-09 wants to catch, except here it would be an *app-shaped* false regression instead.
**Warning signs:** A VAL-09 test that hard-codes `hooks[0]` instead of asserting "policy (if any) → mixin hook → schema hooks" as a relative order.

### Pitfall 4: `Default()` vs `UpdateDefault()` — don't assume symmetric zero-collapse handling
**What goes wrong:** Assuming that because Create's `defaults()` pre-populates the mutation, Update *also* has some equivalent auto-population for non-optional derived fields that would make a phantom violation less likely.
**Why it happens:** Superficial pattern-matching between `create.tmpl` and `update.tmpl`'s near-identical `defaults()` structure — both exist, both run before `withHooks`, but they consult **different field sets** (`$f.Default` vs `$f.UpdateDefault`, confirmed at `update.tmpl:231-242`), and MixinForProto's derived fields only ever carry the former.
**How to avoid:** Treat Update's scope as governed entirely by whatever the FieldMask-gated `Set*` calls actually invoked (Phase 2 D-14…D-17) — there is no ent-side safety net equivalent to Create's `defaults()` for Update.
**Warning signs:** A test that only exercises Create's zero-collapse case and never independently exercises an Update mutation that leaves a non-optional derived field completely untouched.

## Code Examples

### Building the once-per-schema-load standard-rule validator
```go
// Source: buf.build/go/protovalidate@v1.2.0/option.go:31-58 (verified)
exampleMsg := *new(M) // same zero-value trick MixinForProto[M] uses today
v, err := protovalidate.New(
    protovalidate.WithMessages(exampleMsg),
    protovalidate.WithDisableLazy(),
)
if err != nil {
    panic(fmt.Sprintf("mixinforproto: %s: precompiling protovalidate evaluator: %v", msgFullName, err))
    // D-09: an uncompilable expression -> panic at schema load, exactly
    // mirroring protovalidate's own would-be boundary rejection.
}
```

### Building the once-per-field local CEL environment (residual half)
```go
// Source: buf.build/go/protovalidate@v1.2.0/cel/library.go:44-61,504-518 (verified)
opts := []cel.EnvOption{cel.Lib(pvcel.NewLibrary())}
opts = append(opts, pvcel.RequiredEnvOptions(fd)...)
opts = append(opts, cel.Variable("this", celTypeFor(fd))) // "this" binds the field value
env, err := cel.NewEnv(opts...)
if err != nil {
    panic(...) // D-09: uncompilable CEL -> schema-load panic
}
ast, iss := env.Compile(celExpression)
if iss.Err() != nil {
    panic(...)
}
prg, err := env.Program(ast)
```

### Shared violation constructor (D-01/D-07 consequence 3)
```go
// Both evaluation paths must produce *validate.Violation protos and feed
// this one function — never let the residual path grow its own error
// shape (D-07 consequence 3).
// Source shapes verified: buf.build/go/protovalidate@v1.2.0/violation.go:26-46
func newValidationError(violations []*validate.Violation) *protovalidate.ValidationError {
    out := make([]*protovalidate.Violation, len(violations))
    for i, v := range violations {
        out[i] = &protovalidate.Violation{Proto: v}
    }
    return &protovalidate.ValidationError{Violations: out}
}
```

### `runtime.MapError`'s missing case (Claude's Discretion item 2)
```go
// Source: runtime/errormap.go:38-52 (read this session) — the switch this
// phase must extend. protovalidate's error type is generic (not
// application-specific), unlike ent's generated *ValidationError, so it
// can live in the shared runtime package per that file's own doc comment.
var valErr *protovalidate.ValidationError
switch {
case errors.Is(err, privacy.Deny):
    return connect.NewError(connect.CodePermissionDenied, err)
case errors.As(err, &valErr):
    return connect.NewError(connect.CodeInvalidArgument, valErr) // matches
    // connectrpc.com/validate's own mapping for the identical error type
    // reaching it at the boundary — VAL-07's identity guarantee.
case errors.As(err, &connectErr):
    return err
default:
    ...
}
```

### A hand-written `dialect.Driver` fake for the driverless harness
```go
// Source: entgo.io/ent@v0.14.6/dialect/dialect.go:36-45 (verified) — the
// full interface surface a fake must satisfy. No import beyond
// entgo.io/ent (already a direct dependency) is required.
type fakeDriver struct{ dialect string }

func (f *fakeDriver) Exec(ctx context.Context, query string, args, v any) error  { return nil }
func (f *fakeDriver) Query(ctx context.Context, query string, args, v any) error { return nil }
func (f *fakeDriver) Tx(ctx context.Context) (dialect.Tx, error)                 { return nil, errors.New("not needed for hook-only harness") }
func (f *fakeDriver) Close() error                                               { return nil }
func (f *fakeDriver) Dialect() string                                           { return f.dialect }
```

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|----------------|
| A1 | `protovalidate.WithMessages`+`WithDisableLazy` together produce compilation that happens synchronously inside `protovalidate.New(...)` (i.e., truly "at construction," not deferred) — the actual eager-compilation code path inside `newBuilder(...)` when `disableLazy=true` was not traced line-by-line this session, only the public option wiring was confirmed. | Pattern 1, Code Examples | If compilation is still partially deferred, VAL-04's literal "compiled once at schema load" wording may need a test that explicitly forces first-use at schema-load time rather than trusting the option name. Low risk — `WithDisableLazy`'s own doc comment ("eliminates any internal locking as the validator becomes read-only") strongly implies eager compilation, but this is a documentation read, not a traced execution. |
| A2 | The recommended resolution to Pitfall 1 (dedup by `(RuleId, FieldPath)`, or route CEL-bearing fields entirely through the local env) is a reasonable design **within** D-07/D-08's locked boundaries — but which of the two the plan should pick was not decided by research, since it's an implementation-mechanics choice CONTEXT.md leaves open. | Pitfall 1 | If the plan picks neither and instead lets both evaluators run unconditionally with no dedup, a real-world message with a mixed field will show a **doubled** violation on the wire, breaking VAL-07's exact-identity promise against the boundary (which only evaluates it once). |
| A3 | `google.protobuf.Value`'s JSON round-trip (mapped to `field.JSON(name, json.RawMessage(nil))` per Phase 1) reconstructs correctly via `protojson.Unmarshal` into a `dynamicpb.NewMessage` of `google.protobuf.Value`'s *own* descriptor — this specific WKT's protojson mapping (a `Value` is itself a oneof of scalar/struct/list/null) was not exercised with a live round-trip test this session, only confirmed that the general `protojson.Unmarshal`/`dynamicpb.NewMessage` mechanism exists. | Code Examples / Pattern 3 | `google.protobuf.Value`'s JSON shape is unusual (it *is* raw JSON, not a wrapped message) — if `protojson` needs the *containing* message context rather than the bare `Value` descriptor to unmarshal correctly, this specific case may need special-casing distinct from ordinary AsJSON message fields. Should be the first thing exercised by the D-05 reverse-conversion task's own tests, per CONTEXT.md's explicit call-out that this leg is a real risk concentration. |

## Open Questions

1. **Does `mutation.Fields()` at hook time, on Create, actually include every `Default()`-bearing field, empirically?**
   - What we know: `create.tmpl` proves `defaults()` runs and calls the field's setter before `withHooks` is invoked at all (Pitfall 2).
   - What's unclear: whether any edge case (e.g., a field whose `Default` is a `DefaultFunc` requiring `runtime.go`'s package-level variable to be initialized — `create.tmpl:82-89`) could cause `defaults()` to silently skip setting the mutation in some configuration.
   - Recommendation: make this the first task of the phase, proven with a real generated fixture (not a template read), before any other task assumes its answer — several tasks (D-06's scope computation, in particular) can collapse to a single code path if this holds.

2. **Which of Pitfall 1's two resolutions (dedup vs. field-exclusive routing) should the plan choose?**
   - What we know: both are mechanically achievable within D-07/D-08's locked constraints.
   - What's unclear: which produces a simpler/more testable single-pathed error constructor (D-07 consequence 3), and whether field-exclusive routing quietly reintroduces the "reimplement structural rules by hand" risk D-08 forbids (it does not, if it always delegates structural-rule CEL through the same `NewLibrary()`-built env — but this needs to be proven correct with a real min_len-style corpus case evaluated exclusively through the local env and diffed against protovalidate's evaluator).
   - Recommendation: the D-05/D-07 convergence task CONTEXT.md already calls for should decide this with its own test before the rest of the hybrid hook is built on top of it.

3. **D-10's field-reference walk over a message rule's compiled CEL — exact mechanism.**
   - What we know: `cel-go@v0.28.0`'s `cel.Ast.NativeRep()` and `common/ast.AST.ReferenceMap()` exist and expose per-expression-id reference info (`[VERIFIED: /root/go/pkg/mod/github.com/google/cel-go@v0.28.0/cel/env.go:54, common/ast/ast.go:86-87]`), which is a workable mechanism to statically enumerate which `this.<field>` accesses a compiled `MessageRules.GetCel()` expression makes.
   - What's unclear: whether `ReferenceMap`'s entries distinguish a genuine field-select (`this.discount`) from an unrelated identifier or function call reference, without deeper cel-go internals knowledge than this session traced.
   - Recommendation: prototype this against one real message rule from the corpus early, since D-10's panic-at-schema-load behavior depends on correctly identifying every field reference — a false negative (missed field reference) would let an excluded-field rule silently compute against a phantom zero, exactly the bug D-10 exists to prevent.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go toolchain 1.26 (pinned by `go.work`/`go.mod`) | All build/test commands | ✓ (via `GOTOOLCHAIN` auto-download through the configured proxy) | Sandbox ships go1.25.1 and go1.24.7 locally; go1.26.0 downloads successfully on demand through `proxy.golang.org` (confirmed this session — used to fetch modules) | None needed; auto-download works. |
| Go module proxy (`proxy.golang.org`) | Downloading `buf.build/go/protovalidate`, `entgo.io/ent`, `github.com/google/cel-go`, `google.golang.org/protobuf` source for verification and for `go get`/`go mod tidy` | ✓ (used extensively this session) | — | — |
| `buf` CLI v1.72.0, `protoc-gen-go`, `protoc-gen-connect-go` | CI's `modules`/`stubs` jobs (PIPE-01) | Not probed this session (no proto regeneration needed by this phase's changes — no new `.proto` messages required) | — | Not this phase's concern; CI already installs these per `.github/workflows/ci.yml:58-65`. |
| `modernc.org/sqlite` (CGO-free) | D-13's real-client wiring proof (root module only) | ✓ — already a direct root dependency (`go.mod:16`) | v1.56.0 | — |

**Missing dependencies with no fallback:** none.
**Missing dependencies with fallback:** none — everything this phase needs is either already vendored/pinned or fetches cleanly through the existing proxy.

## Security Domain

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-------------------|
| V2 Authentication | No | Unaffected by this phase — Phase 2's `Authenticator`/`AuthnInterceptor` is untouched. |
| V3 Session Management | No | Not touched by this phase. |
| V4 Access Control | Yes | ent `privacy.Policy`, confirmed (this session) to always occupy `Hooks[0]` ahead of any mixin/schema hook when declared — see D-11 correction below. |
| V5 Input Validation | Yes | `buf.build/go/protovalidate` (standard rules) + `github.com/google/cel-go` (custom CEL), enforced identically at both layers — this phase's core purpose. |
| V6 Cryptography | No | Not touched. |

### Known Threat Patterns for this stack

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|-----------------------|
| Constraint-existence disclosure to an unauthorized caller (an attacker learns *which* validation constraints exist on a field by triggering a `ValidationError` instead of receiving `PermissionDenied`) | Information Disclosure | **Verified this session, and it corrects D-11's own hedge:** because `privacy.Policy` always occupies `Hooks[0]` (ahead of mixinforproto's mixin hook) whenever any Policy is declared on the schema (`runtime.tmpl:99-110`), an unauthorized caller is denied by the policy **before** the validation hook ever runs. The risk D-11 flagged only materializes for a schema that declares **no** Policy at all (in which case there is no authorization gate to leak *around*, so the scenario is moot). Document this precisely in VAL-09's docs — "if a Policy exists, it always runs first" is a stronger and more accurate statement than D-11's original "document the consequence honestly" hedge implied might be needed. |
| Fabricated constraint ID on a data-integrity fault (a reverse-conversion failure misreported as a protovalidate violation) | Tampering / Repudiation | D-12: fail closed with a distinct, non-protovalidate error type; never synthesize a `RuleId`. Directly protects PIPE-06's signal integrity, not just an abstract security property. |
| Double-evaluation of a mixed field's custom CEL rule producing two divergent-looking violations on the wire | Tampering (of the caller-visible verdict — a caller could misinterpret a duplicated/inconsistent violation set as a different rule set than the boundary's) | Pitfall 1's dedup-by-`(RuleId,FieldPath)` or field-exclusive-routing resolution, verified before build via PIPE-06. |

## Sources

### Primary (HIGH confidence — direct source read this session)
- `buf.build/go/protovalidate@v1.2.0` (downloaded via `proxy.golang.org`, read directly): `go.mod`, `cel/library.go`, `filter.go`, `option.go`, `validator.go`, `field.go`, `message.go`, `resolve.go`, `violation.go`, `validation_error.go`
- `entgo.io/ent@v0.14.6` (downloaded via `proxy.golang.org`, read directly): `ent.go`, `entc/gen/template/builder/create.tmpl`, `entc/gen/template/builder/update.tmpl`, `entc/gen/template/runtime.tmpl`, `entc/gen/template/base.tmpl`, `dialect/dialect.go`
- `google.golang.org/protobuf@v1.36.11` (downloaded via `proxy.golang.org`, read directly): `types/dynamicpb/dynamic.go`, `encoding/protojson/decode.go`, `encoding/protojson/encode.go`, `types/known/timestamppb/timestamp.pb.go`, `reflect/protoreflect/value_union.go`, `reflect/protoreflect/type.go`
- `github.com/google/cel-go@v0.28.0` (downloaded via `proxy.golang.org`, read directly): `cel/env.go`, `common/ast/ast.go`
- `buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go@v1.36.11-...` (downloaded, grepped): `buf/validate/validate.pb.go` (confirmed `MessageRules.GetCel`)
- This repo's own files, read directly: `mixinforproto/validate.go`, `mixinforproto/fieldmap.go`, `mixinforproto/annotation.go`, `mixinforproto/option.go`, `mixinforproto/errors.go`, `mixinforproto/go.mod`, `mixinforproto/internal/boundarytest/boundary_test.go`, `runtime/interceptor.go`, `runtime/errormap.go`, `runtime/doc.go`, `go.mod`, `go.work`, `.github/workflows/ci.yml`, `Makefile`, `internal/entconnecttest/update/update_test.go`, `internal/entconnecttest/update/ent/mutation.go`

### Secondary (MEDIUM confidence)
- None used — every claim in this document traces to a primary source read this session.

### Tertiary (LOW confidence)
- None used — no WebSearch was performed for this research pass; all questions were answerable by reading pinned dependency source directly in the sandbox's Go module cache.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — every package/version/import-path claim verified against the actual downloaded module source, not training memory.
- Architecture (hook ordering, filter mechanics, reverse conversion APIs): HIGH — traced through ent's own codegen templates and protovalidate's own evaluator source, not inferred from documentation prose.
- Pitfalls: HIGH for Pitfalls 2-4 (directly derived from the same verified source); MEDIUM for Pitfall 1 (the *existence* of the granularity mismatch is verified; the *best* resolution is a legitimate open implementation choice, not a verified fact).

**Research date:** 2026-08-14
**Valid until:** ~30 days (pinned dependency versions; re-verify if `buf.build/go/protovalidate`, `entgo.io/ent`, or `github.com/google/cel-go` are bumped before this phase executes — a cel-go bump in particular could shift the org-migration situation CLAUDE.md already got wrong once).

