# Phase 2: CRUD Handlers & Interceptor Chain - Context

**Gathered:** 2026-08-08
**Status:** Ready for planning
**Mode:** interactive `discuss`, partially completed. **D-01/D-02 are user-stated decisions.** Every other decision is Claude's recommended default recorded so planning is not blocked — a reviewer may overturn any of them before planning. See "Claude's Discretion" for the ranked list of what most deserves a second look.

<domain>
## Phase Boundary

Phase 2 delivers the **`entconnect` entc extension's CRUD half**: it reads the committed `FileDescriptorSet` plus the ent schema graph (including Phase 1's `MixinForProto` provenance annotations) and emits ConnectRPC handler implementations for Get/List/Create/Update/Delete over `MixinForProto`-backed entities, the fixed `authn → viewer injection → protovalidate → otel → handler` interceptor chain, and the server wiring that is the single place an `*ent.Client` is constructed.

**In scope:** CRUD-01…CRUD-07, INT-01…INT-05.

**Explicitly NOT in this phase** (later phases; must not be pulled forward):
- Tier 2/3 CEL passthrough, schema-layer residual enforcement, the differential harness — Phase 3 (VAL-04…VAL-11, PIPE-05, PIPE-06)
- Flow binding, `flow.Start`, `GetRunStatus`, `codec/proto` — Phase 4 (FLOW-01…FLOW-06)
- The **drift check itself** — Phase 5 (DRIFT-01…DRIFT-07). Phase 2 owns INT-05's *reporting* obligation (see D-07) but must not make an unclaimed RPC a build failure; that is DRIFT-01's job.
- grpc-gateway / plain-REST emitters — `entconnect.md` §3.3, v0.4/v0.5
- The Order/Inventory reference application — Phase 5

**Boundary note on validation:** Phase 1's D-11 left untranslated protovalidate constraints recorded-but-unenforced. Until Phase 3 lands, this phase's protovalidate interceptor stage is the **only** thing enforcing them. Plans should not describe the storage layer as a complete guarantee in this phase's docs.

</domain>

<decisions>
## Implementation Decisions

### CRUD Binding & RPC Recognition (CRUD-01, CRUD-02, INT-04)

- **D-01 (USER DECISION):** The RPC↔operation binding is declared in **`ent/schema` Go annotations**, not in the proto and not by name convention. The adopter writes, in the schema's `Annotations()`:

  ```go
  entconnect.CreateRPC(orderv1connect.OrderServiceCreateOrderProcedure),
  entconnect.UpdateRPC(orderv1connect.OrderServiceUpdateOrderProcedure),
  entconnect.DeleteRPC(orderv1connect.OrderServiceDeleteOrderProcedure),
  ```

  plus `GetRPC` and `ListRPC` in the same family. Rationale: connect-go's generated `…Procedure` constant is a **compile-time-checked reference to the contract** — the exact property `MixinForProto[*orderv1.Order]`'s type parameter buys at the field level. Rename or delete an RPC in the proto and the ent schema stops compiling, before any generator runs. The entity needs no inference whatsoever: the annotation sits on the schema that owns it. And it adds no fourth human-writable artifact class — proto, ent schema, flows stay the only three.
  — **Reversibility:** one-way — this is the adopter-facing API surface; changing it after release breaks every consuming schema.

  **Explicitly rejected during discussion:** a custom **proto method option** (`option (entconnect.method).crud = …`). It was proposed first and withdrawn in favor of the above. Consequence: PROJECT.md's Out of Scope entry *"Custom proto field options as an annotation channel"* **stands unamended** — this phase adds no proto options of any kind and entconnect publishes no `.proto` files.

- **D-02 (USER-FLAGGED CHALLENGE):** What crosses entc's JSON schema-load boundary is the **procedure string alone** (`"/order.v1.OrderService/CreateOrder"`) — a `string` constant carries no descriptor and no package identity. This is the difficulty the user named explicitly: the service may live in a completely different package, and worse, the entc **host** process (`ent/generate.go`) never links `orderv1` at all — only the schema-load subprocess does. The descriptor is therefore unreachable via the Go import graph, by construction.

  The annotation type must obey Phase 1's D-02 precedent: a plain serializable struct, no closures, tolerant of unknown keys, carrying `ContractVersion` (Phase 1 `ContractVersion = 1`, additive-only per D-03).

- **D-03:** **The procedure string is the key; the committed `FileDescriptorSet` is the lookup table.** The extension splits the procedure on its final `/` into service full name + method name and resolves both against the descriptor set it already loads. This is what `entconnect.md` §9 OQ2 provisioned for — *"the entc extension still reads a `FileDescriptorSet` for whole-service concerns"* — and what §4's committed `.binpb` exists to make hermetic. Package location becomes irrelevant.

  Decisive secondary reason: **Phase 5's drift check must enumerate RPCs that no schema claims.** A resolution strategy that can only see services some schema already references cannot ever support DRIFT-01. Choosing the registry path here would force a rewrite in Phase 5.
  — **Reversibility:** costly — Phase 5 builds directly on this resolution layer.

  *Considered and rejected:* resolving via `protoregistry.GlobalFiles` in the schema-load subprocess (works — importing `orderv1connect` transitively links `orderv1`, whose `init()` registers descriptors — but sees only referenced services, and leans on an import edge nobody declared deliberately). A both-paths cross-check was considered as a staleness signal and deferred; Phase 1's D-22 gate already covers descriptor-set staleness at the pipeline level.

- **D-04:** Descriptor-set location is an extension option (`entconnect.WithDescriptorSet(path)`) defaulting to the pipeline's committed path. A procedure that does not resolve produces a self-sufficient first-line error naming the procedure, the descriptor-set path, and the remedy — *"…not found in `<path>` — descriptor set may be stale; run `scripts/pipeline.sh`"* — following Phase 1's D-08 truncation-resistant error discipline, which applies with equal force here.

- **D-05:** A procedure claimed by two different schemas, or the same operation claimed twice on one schema, is a **collected** codegen error (Phase 1 D-09: report all offenders in one pass, not first-offense-wins).

- **D-06:** `entconnect.Manual("rpc")` (INT-04) takes the same procedure-constant argument as the CRUD binders, for one consistent reference mechanism. A `Manual` RPC still runs inside the generated chain — the generated wiring registers it, the app supplies only the handler func.

- **D-07:** INT-05 is satisfied in this phase by a **deterministic, sorted claims report** emitted at codegen listing every RPC in the descriptor set with its claimant (`crud:<op>` / `manual` / `unclaimed`). Phase 2 reports; Phase 2 does **not** fail the build on `unclaimed` — that is DRIFT-01, Phase 5. Sorted output is a hard requirement so CI diffs stay meaningful (Phase 1 D-24, DRIFT-06).

### List Paging (CRUD-03)

- **D-08:** The `page_token` is opaque and wraps the keyset cursor **plus a fingerprint of the request's ordering/filter-relevant fields**. A token whose fingerprint disagrees with the current request fails with `InvalidArgument` rather than silently returning an inconsistent page — hiding that is precisely the bug class keyset paging exists to prevent.

- **D-09:** **Filtering is out of scope for Phase 2** — paging only. Deriving `Where` predicates from fields present on the List request message would be inventing a mapping convention, the same category of thing `entconnect.md` §3.3 explicitly refuses to do for REST. Ordering comes from a schema-side default, not from the contract. Recorded as a deferred idea.

- **D-10 (RESEARCH FLAG — do not treat as settled):** CRUD-03's wording *"ent's native keyset `Paginate()`"* needs verification before planning. `Paginate()` is generated by **entgql** for Relay connections; core `entgo.io/ent` v0.14.6 is not confirmed to expose it. If it is entgql-only, this phase must either take an entgql dependency (rejected on its face — entgql re-derives an API from the schema, the exact inversion this project exists to reject, per §2.3) or **emit keyset paging directly** (`WHERE (created_at, id) < (?, ?) ORDER BY … LIMIT n+1`). The researcher must resolve this first; it materially changes the List plan.

### App Integration Surface (INT-01, INT-02, CRUD-07)

- **D-11:** Authn and viewer extraction — which the generator cannot know — are supplied through a **generated narrow interface** the app implements (single method, roughly `AuthenticateAndViewer(ctx, http.Header) (context.Context, error)`), taken as a required argument: `NewServer(client, authenticator, opts...)`. The chain order stays fixed and non-negotiable inside generated code (INT-01), and "forgot to wire authn" becomes a **compile error rather than an open endpoint**. Functional options were rejected precisely because an omitted option degrades to a runtime default — either deny-all (safe but baffling) or allow-all (a security hole shipped by omission). A raw `connect.Interceptor` slot was rejected because it would let the app reorder the stages INT-01 fixes.
  — **Reversibility:** costly — it is the server-construction signature every adopter calls.

- **D-12:** CRUD-07 is enforced **structurally, not by documentation**: the `*ent.Client` is constructed inside generated wiring in an internal package and handed to handlers unexported. Application code receives the Connect handler and its path, never the client. "Only place a client is constructed" must be a property the compiler upholds, matching this project's general preference for build failures over conventions.

- **D-13:** The protovalidate validator is built **once per process**, not per request, and threaded into the chain by the generated wiring. (Phase 3's SC-4 states this as a requirement; building it correctly here costs nothing and avoids a Phase 3 retrofit.)

### FieldMask & Update Semantics (CRUD-04, CRUD-05)

- **D-14:** This resolves `entconnect.md` §9 **Open Question 5** (*"adopt FieldMask conventions or infer from set fields — pick one, document it"*) in favor of **explicit `google.protobuf.FieldMask`**, per CRUD-04. Inference from set fields is exactly the proto3 zero-collapse trap Phase 1's D-26 documented at length.

- **D-15:** **Top-level mask paths only.** Nested paths (`customer.name`) and `*` are rejected **at build time** with a message naming the offending path. Nested paths have no meaning against a flat ent field set and would imply edge traversal this project deliberately leaves human-declared (PROJECT.md Out of Scope: "Edge generation from message-typed fields").

- **D-16:** An **empty or absent mask is `InvalidArgument`**, never "update everything." "Everything" is the precise footgun Phase 1 spent a dedicated README section on: a `Set*` for every field on a proto message silently clears every field the caller left at its Go zero value.

- **D-17:** CRUD-05's build-time path validation resolves each mask path against the message descriptor **and** against Phase 1's `SourceField` provenance, so a path naming a field that was `Exclude()`d or has no derived ent field fails the build too — not just a path absent from the descriptor. `SourceField.FieldName` (JSON key `name`) is the proto-name→ent-field mapping this needs; it is already populated.

### Error Mapping (INT-03)

- **D-18:** An explicit generated mapping table: `privacy.Deny`→`PermissionDenied` (INT-03), `NotFoundError`→`NotFound`, `ConstraintError`→`AlreadyExists`/`FailedPrecondition`, `ValidationError`→`InvalidArgument`, and **anything unrecognized→`Internal`, with the detail logged and not returned**. Unknown-means-Internal keeps database internals off the wire by default; a deliberate widening is always a later, reviewable change.

### Codegen Mechanics (CRUD-06)

- **D-19:** `text/template` via `entc/gen`, mirroring the `entproto` extension skeleton (`type Extension struct { entc.DefaultExtension; … }`, `NewExtension(opts…)`, `Hooks() []gen.Hook`, top-level `Generate(g *gen.Graph) error`). No jennifer, no `go/ast` — already settled in the project stack notes.

- **D-20:** Byte-stability (CRUD-06) inherits Phase 1's discipline verbatim: never range a map into ordered output, sort every key set, `goldie` golden files with `-update`, and CI running golden tests with `-count=5` so order-dependence surfaces as a failure rather than as flaky CI.

- **D-21:** The root module gains its `mixinforproto` dependency edge in this phase — Phase 1's D-18 deliberately deferred it to here. Phase 1's D-19 tagging discipline applies: the bump lands in a separate commit referencing an already-pushed `mixinforproto/vX.Y.Z` tag, never a same-commit self-reference. A `replace ./mixinforproto` line in a PR diff remains a blocking review comment (D-15).

### Claude's Discretion

The discussion covered binding (D-01/D-02) with the user directly; the session ended before the remaining three areas were answered, so **D-03 onward are recommended defaults, not user-stated preferences.** Ranked by how much a human's second look is worth:

1. **D-10** (keyset `Paginate()` availability) — the only decision here resting on an unverified factual claim in the requirement text itself. Resolve before planning List.
2. **D-11** (required interface vs. functional options) — a real ergonomics-vs-safety trade; the safety argument is strong but the signature is one-way for adopters.
3. **D-07** (report-don't-fail on unclaimed RPCs) — a scoping call about where Phase 2 ends and Phase 5 begins, not a technical constraint.
4. **D-16** (empty mask is an error) — defensible and consistent with the project's failure-loudly ethos, but it will annoy generated clients that send an empty mask by habit.
5. **D-18** (unknown→`Internal`) — conservative; `Unknown` is the arguable alternative.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Primary design contract (read first)
- `entconnect.md` §3.2 — handler generation, RPC resolution order, the fixed interceptor chain, and the `Manual` escape hatch. Normative for this phase.
- `entconnect.md` §3.4 — the drift check. Read for **boundary awareness only**: it defines what Phase 5 owns and therefore what this phase must not implement.
- `entconnect.md` §4 — the canonical build pipeline and why the descriptor set is committed (the basis for D-03).
- `entconnect.md` §5 — repository layout (`entc/`, `runtime/`) and the dependency graph.
- `entconnect.md` §9 OQ2 — descriptor location: the extension reads a `FileDescriptorSet` for whole-service concerns.
- `entconnect.md` §9 OQ5 — field-mask semantics, explicitly unresolved upstream; **resolved here by D-14…D-17**.

### Phase 1 outputs this phase consumes
- `.planning/phases/01-mixinforproto-core/01-CONTEXT.md` — D-01…D-05 (annotation contract), D-08/D-09 (error surface discipline), D-18 (the deferred module edge, now due), D-19 (tagging), D-23/D-24 (golden + determinism).
- `.planning/phases/01-mixinforproto-core/01-VERIFICATION.md` — what Phase 1 actually proved, including the four closed gaps.
- `mixinforproto/annotation.go` — the real, shipped annotation shapes: `ContractVersion`, `MixinForProtoMessage`/`MixinForProtoField` keys, `SourceMessage{Message, Fields, Excluded, Overridden}`, `SourceField{FieldName, Number, Kind, TranslatedIDs, ResidualIDs, ResidualFingerprint, LengthUnitDivergentIDs}`. Read this file, not the prose description of it.
- `mixinforproto/README.md` — the proto3 presence/zero-collapse section that motivates D-16.

### Project-level
- `.planning/PROJECT.md` — Out of Scope (note: the "custom proto field options" entry stands unamended; see D-01) and Key Decisions.
- `.planning/REQUIREMENTS.md` — CRUD-01…CRUD-07, INT-01…INT-05 verbatim.
- `.claude/CLAUDE.md` — pinned stack: `connectrpc.com/connect` v1.20.0, `entproto` v0.7.0 as a read-only structural precedent, `goldie/v2`, and the explicit "do not add grpc-gateway or genproto yet" instruction.

### External precedent (read-only, never a dependency)
- `entgo.io/contrib/entproto` — extension skeleton shape.
- `entgo.io/contrib/entgql` — golden-file and template-registration precedent. **Not a dependency** — §2.3 rejects its generation direction. Relevant to D-10 only as the place `Paginate()` actually comes from.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `mixinforproto/annotation.go` — `SourceMessage`/`SourceField` are the extension's entire read surface into the schema graph. `SourceMessage.Fields` (the complete descriptor inventory) plus `Excluded`/`Overridden` is what lets codegen tell "deliberately excluded" from "silently forgotten" — load-bearing for D-17's mask validation.
- `mixinforproto/internal/gen/mixinforprototestv1/` + `mixinforproto/proto/` — an established synthetic-corpus pattern (one proto file per rule class, generated stubs committed so tests need no `buf`). Phase 2 should extend this pattern with a service-bearing corpus rather than invent a second fixture convention.
- `mixinforproto/internal/boundarytest/` — a working real-entc-schema-load harness. The nearest existing analog to what a generated-handler test needs.
- `scripts/generate-stubs.sh` + `make check-stubs` — the single canonical `buf generate` invocation and the orphan-aware staleness gate (Phase 1 Plan 07). Any new corpus protos must go through it, not around it.
- `mixinforproto/testdata/*.golden` + `goldie` usage — the golden-file convention to copy for generated handler code.

### Established Patterns
- **Failure discipline** (Phase 1 D-06/D-08/D-09): a pure core returning collected errors, with a thin adapter that panics; every error's first line self-sufficient and every offender reported in one pass. Codegen errors in this phase should read the same way.
- **Determinism by construction** (D-24): never range a map into ordered output; sort explicitly; `-count=5` in CI.
- **Two-module hygiene** (D-15/D-16/D-17): `go.work` checked in, no `replace` in any committed `go.mod`, explicit `MODULES := . ./mixinforproto` enumeration, and a `GOWORK=off` job. Phase 2 is the first phase where the root module has real code, so the root side of that CI matrix stops being a formality.

### Integration Points
- Root module (`github.com/smintz/entconnect`, `go 1.26`) is currently **code-free** — `entc/`, `runtime/`, and `codec/proto/` do not exist yet. Phase 2 creates `entc/` and `runtime/`; `codec/proto/` is Phase 4's.
- The root `go.mod` gains its first real dependencies here: `connectrpc.com/connect`, `entgo.io/ent`, and `github.com/smintz/entconnect/mixinforproto` (D-21).
- `scripts/pipeline.sh` currently has a `go generate` step with nothing to act on. Phase 2 gives it its first real work, which also means the pipeline becomes genuinely order-dependent for the first time.

</code_context>

<specifics>
## Specific Ideas

The user's own words on binding, recorded verbatim because the exact call shape is the decision:

```go
entconnect.CreateRPC(orderv1connect.OrderServiceCreateOrderProcedure),
entconnect.UpdateRPC(orderv1connect.OrderServiceUpdateOrderProcedure),
entconnect.DeleteRPC(orderv1connect.OrderServiceDeleteOrderProcedure),
```

> "The annotations should be in `ent/schema`, not inside the proto. … Which should resolve to the service descriptor. Which is challenging because it might be in a completely different package."

That last sentence is the phase's hardest technical problem and the reason D-03 exists. The planner should treat "resolve a procedure string to a descriptor without an import edge" as a first-class task with its own tests, not as a detail of handler emission.

</specifics>

<deferred>
## Deferred Ideas

- **Contract-driven filtering on List** — deriving `Where` predicates from fields on the List request message. Deferred from D-09; it needs its own convention design, and inventing mapping conventions is a thing this project does deliberately and rarely.
- **Nested field-mask paths into `AsJSON()` fields** — genuinely structured interiors where nesting would be meaningful (D-15 rejects nesting wholesale for now). Revisit if adopters ask.
- **Connect server streaming of run-state / list changes** — `entconnect.md` §9 OQ3 option (b), designed and deferred upstream.
- **Hard build failure on unclaimed RPCs** — belongs to DRIFT-01, Phase 5. Phase 2 reports (D-07); Phase 5 enforces.
- **A both-paths descriptor cross-check** (descriptor set + `protoregistry` at schema load) as a redundant staleness signal — considered under D-03, deferred as duplicative of Phase 1's D-22 gate.
- **Narrowing PROJECT.md's "custom proto field options" Out of Scope wording** — no longer needed now that D-01 lands the binding in `ent/schema`; noted only so a future reader knows the question was raised and closed.

</deferred>

---

*Phase: 2-CRUD Handlers & Interceptor Chain*
*Context gathered: 2026-08-08*
