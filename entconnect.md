# entconnect — Design Document

**Status:** Draft v1 · **Author:** Shahar Mintz (smintz) · **Date:** 2026-08-05

## 1. Summary

entconnect makes protobuf contracts the source of truth for [ent](https://entgo.io) applications. It replaces entproto's direction of generation: instead of deriving `.proto` files from ent schemas, entconnect derives ent fields *from* the contract and generates the transport layer that binds RPCs to the application. One descriptor-reader, N transport emitters: [ConnectRPC](https://connectrpc.com) handlers first, gRPC + grpc-gateway HTTP annotations second, plain REST (an entrest replacement) third.

It is the contract half of a two-project split; the workflow half is **entflow** (separate design doc). entconnect knows about descriptors and schemas; entflow knows about schemas only. They meet exclusively in the application (§8).

## 2. Motivation

### 2.1 The pipeline this serves

The target development workflow is contract-first and LLM-assisted:

1. Design (Figma / Claude Design).
2. Protos written from the design (ConnectRPC service definitions, protovalidate constraints).
3. Ent schemas — fields materialized from the contract, plus storage-only fields, transitions, hooks, privacy, flows.
4. Everything else generated: handlers, validation, migrations (Atlas), TS client (connect-es).

The human/LLM-writable surface is exactly three artifact classes — design, protos, schemas — each declarative and reviewable, each downstream layer generated and cross-checked against the one above.

### 2.2 Why proto-first is forced, not chosen

Historically ent-first vs proto-first was a stylistic debate (entproto embodies ent-first). In this stack it is settled by topology: ent schema files reference contract types directly (flow inputs like `*orderv1.CancelOrderRequest`, handler response shapes), so **the schema imports the contract** and cannot compile before `buf generate` runs. The dependency arrow is physical:

```
design → .proto → buf generate → orderv1 (Go) + FileDescriptorSet
       → schemas compile → entc (+ extensions) → handlers, workers, migrations
```

You cannot generate the contract from a schema that won't compile without the contract. entconnect leans into this instead of fighting it.

### 2.3 Problems with the status quo

- **entproto** generates proto from ent — wrong direction for contract-first; also emits classic gRPC, not Connect.
- **Dual schema definition** (resource message vs `Fields()`) drifts silently.
- **Dual validation** (protovalidate vs ent validators) drifts silently and disagrees on error surfaces.
- **entrest / entgql / entproto each re-derive an API from the schema** — three sources of truth for "the API," none of them the designed contract.
- **Handlers written by hand or by LLM** are the classic leak point for auth, validation, and logic that belongs elsewhere.

## 3. Components

### 3.1 `MixinForProto` — contract-driven fields (separately scoped)

Rescoped into its own component with a dedicated design doc (`mixinforproto-design.md`) and an independently adoptable module. Summary of the scoping decisions, normative for this doc:

```go
func (Order) Mixin() []ent.Mixin {
    return []ent.Mixin{
        entconnect.MixinForProto[*orderv1.Order](
            entconnect.Override("status", /* transitions annotation */),
        ),
        mixin.Time{},
    }
}

func (Order) Fields() []ent.Field {
    return []ent.Field{
        // storage-only, never in the API:
        field.String("idempotency_key").Immutable().Unique(),
        field.String("payment_intent_id").Sensitive(),
    }
}
```

- **Runtime mixin, codegen-free.** Takes the generated message *type* as a type parameter; descriptors come from protoreflect, so no committed `FileDescriptorSet` and no string message names — the reference is compile-time-checked.
- **Fields + validation relay only.** Validation is three-tier: standard constraints → native ent builders (introspectable); residual field-scoped CEL → compiled programs executed by a single mixin-declared hook (fidelity, uniform across field types); message-level cross-field rules → boundary-only by default, opt-in Create-time schema enforcement. Boundary interceptor and schema derive from the same protovalidate source and emit identical error shapes.
- **Non-goals: services, edges, drift checking.** Handler/CRUD generation is §3.2; edge verification is §3.4; the mixin knows about neither.

Result: API-visible fields defined once (contract), storage-private fields defined once (schema), validation defined once (protovalidate, enforced at boundary *and* storage) — nothing defined twice, drift impossible by construction.

### 3.2 Handler generation

An entc extension (entproto/entgql precedent) that reads both the descriptor set and the schema graph and emits ConnectRPC service implementations. For each RPC, resolution order:

1. **Flow-bound RPCs** — request type matches a flow's input type (via entflow's metadata surface, §8): generate a handler that decodes + protovalidates the request, extracts the viewer, calls `flow.Start(ctx, in)`, and shapes the response (immediate for synchronous flows; run-reference for long-running ones — see Open Question 3). Handlers are 100% generated and do not exist as editable files — "dumb by construction," not by discipline.
2. **CRUD RPCs** — standard get/list/create/update/delete shapes over a `MixinForProto`-backed entity: generated directly against the ent client. Privacy policies are the authorization layer; the handler adds nothing but decode/encode. List RPCs map paging/filtering conventions (AIP-158-style page tokens; field-mask–driven partial updates for Update).
3. **Unmatched RPCs** — codegen error (see §3.4). No silent hand-written-handler escape hatch in the default mode; an explicit `entconnect.Manual("rpc")` opt-out exists for the genuinely custom, and it should feel like the smell it is.

Generated interceptor chain (order fixed): authn → viewer injection → protovalidate → otel → handler. Viewer injection is the linchpin that makes ent privacy work; the generated server wiring is the only place a client is constructed, and it never exposes a privileged client to application code.

### 3.3 Multi-transport emission

The same descriptor-reader drives multiple emitters:

- **Connect** (v1): native handlers as above; connect-es TS client comes free from `buf generate` on the same contract.
- **grpc-gateway** (v1.x): pass-through of `google.api.http` annotations so the contract carries its own REST mapping; entconnect validates annotations against generated handlers.
- **REST / entrest replacement** (v2): for teams wanting plain JSON REST without gateway hops, emit chi/stdlib handlers from the same http annotations. Explicit non-goal: inventing a REST mapping convention — `google.api.http` is the mapping language.

### 3.4 Drift check — the contract/constitution cross-validation

At codegen, in both directions:

- Every RPC on a service must be claimed: by a flow (input-type match), a CRUD binding, or an explicit `Manual` — else error ("RPC declared but unimplemented").
- Every flow whose input is a proto message must have a claiming RPC — else warning ("flow unreachable from contract"; legitimate for cron/internal flows, hence warning not error, silenced by `entflow` marking the flow internal).
- Every message-typed field skipped by `MixinForProto` must be covered by a declared edge of matching name/target or an explicit exclude; `google.api.resource_reference` id fields warn when no corresponding edge exists (edges: human-declared, machine-verified).
- protovalidate constraints that were mirrored into ent validators are fingerprinted; if the schema hand-declares a conflicting validator, error.

Contract drift is a build failure, not a runtime surprise.

## 4. Build Pipeline (canonical)

```
1. buf lint && buf generate      # → orderv1 Go pkg, FileDescriptorSet (.binpb, committed),
                                 #   connect stubs, connect-es TS client
2. go generate ./...             # entc + entconnect + entflow extensions:
                                 #   MixinForProto derives fields via protoreflect; graphs cross-validated;
                                 #   handlers, run entities, workers generated
3. atlas migrate diff            # schema → versioned migration
```

Strictly ordered; the loop for any change is proto → buf → schema → entc. This is a feature: the pipeline structurally enforces that the contract moves first. The descriptor set is committed to the repo so step 2 is hermetic and schema-loading never shells out to buf.

## 5. Architecture and Dependencies

```
entconnect/
├── mixinforproto/   # independent go.mod: message type → ent fields (+ validation relay); see its own design doc
├── entc/            # extension: handler gen, drift check, transports
├── codec/proto/     # entflow.Codec adapter for proto.Message inputs (~10 lines)
└── runtime/         # interceptors: viewer, protovalidate, otel
```

Dependency graph (the decoupling in one picture):

```
entflow    → ent
entconnect → ent + protobuf/descriptor machinery (+ entflow metadata interface)
app        → both (the only meeting point)
```

entconnect's dependency on entflow is confined to a small metadata interface (flow name, input/output types, sync/async nature) — ideally duck-typed or a tiny shared `entflow/meta` package so entconnect builds without entflow's runtime. entconnect must be fully useful with zero flows (pure CRUD apps), and entflow fully useful with zero contract (plain-struct inputs).

## 6. Testing Strategy

Golden-file tests for all generated code (entgql precedent). A conformance corpus of proto files exercising every mapping rule in §3.1's table, including every protovalidate constraint class and the untranslatable-CEL fallback. Drift-check tests: each violation class produces its specific error. Integration: a reference app (Order/Inventory, borrowed from the entflow doc) built end-to-end through the §4 pipeline in CI, with connect-go client tests hitting generated handlers and asserting privacy denials surface as `PermissionDenied`.

## 7. MVP Roadmap

1. **v0.1 — MixinForProto:** ships first, as its own module, per its own design doc and roadmap (scalars/enums/WKTs + Tier 1 in its v0.1; CEL hook in v0.2). Standalone value: proto-first ent with no other machinery — deliberately adoptable by people who want nothing else from this stack.
2. **v0.2 — CRUD handlers:** Connect emitter for get/list/create/update/delete + interceptor chain + drift check for CRUD.
3. **v0.3 — Flow binding:** entflow metadata consumption, flow-bound handlers, full bidirectional drift check.
4. **v0.4 — grpc-gateway annotations**; **v0.5 — REST emitter.**

## 8. Decoupling Contract with entflow (normative)

1. entflow never imports proto/descriptor machinery; entconnect never imports entflow runtime — only the metadata surface.
2. The proto input codec lives in entconnect (`codec/proto`), not entflow.
3. RPC↔flow matching is by request type, performed by entconnect at codegen; entflow is unaware transports exist.
4. Either project must be releasable, versionable, and adoptable without the other.

## 9. Open Questions

1. **RESOLVED — override mechanism**: explicit `Override()` builder, not shadowing; an override suppresses validation relay for that field (no implicit merge semantics). Rationale in the MixinForProto doc.
2. **RESOLVED for the mixin — descriptor location**: the type parameter carries descriptors via protoreflect; no committed file needed. The entc extension still reads a `FileDescriptorSet` for whole-service concerns (drift check, http annotations); its path convention remains an implementation detail of `buf.gen.yaml`, decided in v0.2.
3. **Async flow responses**: long-running flows can't answer the RPC synchronously. Options: (a) response message convention carrying `run_id` + generated `GetRunStatus` RPC per service; (b) Connect server streaming of run-state transitions. (a) for MVP, (b) designed but deferred — streaming state changes off the run table is a natural later feature and rounds out the original stack's "subscriptions" gap.
4. **Naming**: `entconnect` implies Connect-only, but the project spans gateway/REST. Alternatives: `entcontract`, `entproto2`. Current lean: keep `entconnect` (Connect is the flagship transport and the name is available/legible); record the objection.
5. **Field masks & partial update semantics**: adopt `google.protobuf.FieldMask` conventions or infer from set fields — pick one, document it, generate it consistently.
6. **entproto migration path**: a doc (not code) mapping entproto annotations to the entconnect model, since the ecosystem's existing proto-adjacent users all come from there.
