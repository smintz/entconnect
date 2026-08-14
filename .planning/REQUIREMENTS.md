# Requirements: entconnect

**Defined:** 2026-08-08
**Core Value:** API-visible fields and their validation are defined exactly once — in the protobuf contract — and enforced identically at the transport boundary and at the storage layer, so contract/schema drift is a build failure rather than a runtime surprise.

## v1 Requirements

Requirements for the initial release (entconnect through v0.3: MixinForProto + Connect CRUD + flow binding + full drift check). Each maps to roadmap phases.

### Mixin Core (`mixinforproto`)

- [x] **MIX-01**: Developer can declare `entconnect.MixinForProto[*orderv1.Order]()` in a schema's `Mixin()` and get ent fields materialized from the message descriptor, with no committed descriptor file and no string message names
- [x] **MIX-02**: Mixin maps proto scalar types to their corresponding ent field builders
- [x] **MIX-03**: Mixin maps proto `enum` fields to `field.Enum` with the enum's declared values
- [x] **MIX-04**: Mixin maps `google.protobuf.Timestamp` to `field.Time`, `Struct`/`Value` to `field.JSON`, and skips `FieldMask`
- [x] **MIX-05**: Mixin maps `optional` scalars to `Nillable().Optional()` and non-`optional` proto3 scalars to non-optional fields with `Default(zero)`, matching wire presence semantics
- [x] **MIX-06**: Mixin maps `map<K,V>` of scalars to `field.JSON`, and skips maps of messages
- [x] **MIX-07**: Developer can exclude message fields with `Exclude(names ...string)`; excluding a nonexistent field fails at schema load
- [x] **MIX-08**: Developer can replace a derived field wholesale with `Override(name string, f ent.Field)` to attach annotations or adjust storage; overriding a nonexistent field fails at schema load
- [x] **MIX-09**: Developer can map a message-typed field to `field.JSON` with `AsJSON("field")` for genuinely embedded values
- [x] **MIX-10**: Message-typed fields are skipped by default; a `oneof` fails at schema load unless every member is excluded or overridden
- [x] **MIX-11**: Every schema-load failure names the offending message, field, and option, and states the fix — not a bare Go panic (entc runs schema load in a subprocess that discards stack traces)
- [x] **MIX-12**: Developer can reproduce any schema-load failure in-process for debugging, without going through `go generate`
- [x] **MIX-13**: Fields are emitted in a deterministic order independent of Go map iteration
- [x] **MIX-14**: `mixinforproto` builds and tests as an independent module depending only on `ent`, `google.golang.org/protobuf`, and the protovalidate/CEL toolchain

### Annotation Contract

- [x] **ANNO-01**: Mixin records source-message provenance on the schema as an ent annotation surviving entc's schema-load JSON serialization
- [x] **ANNO-02**: Mixin records per-field provenance (source field name, derivation kind, applied constraints) as ent annotations readable from `gen.Graph`
- [x] **ANNO-03**: Mixin records `Exclude` and `Override` decisions as annotations, so codegen can distinguish deliberate omission from accidental drift
- [x] **ANNO-04**: Annotation structs carry a version marker so codegen can detect and report a mixin/extension version mismatch

### Validation Relay

- [x] **VAL-01**: protovalidate string constraints (`min_len`/`max_len`/`len`/`pattern`/format validators) become native ent builder calls visible to other ecosystem generators
- [x] **VAL-02**: protovalidate numeric constraints (`gt`/`gte`/`lt`/`lte`) become native `Min`/`Max`/`Range`/`Positive` with correct open/closed-interval adjustment
- [x] **VAL-03**: protovalidate presence/`required` constraints become `NotEmpty` or non-optional field construction
- [x] **VAL-04**: Residual field-scoped protovalidate CEL is compiled once at schema load and evaluated at mutation time by a single mixin-declared hook covering all field types
- [x] **VAL-05**: The mixin hook's evaluated field scope is operation-dependent: on Create it evaluates every derived field (a `Default(zero)`-bearing field the caller left unset still persists as a real value the boundary validates); on Update it evaluates only fields the mutation actually changed. (Amended 2026-08-14, D-06 — see 03-CONTEXT.md: a strict changed-only rule at Create would miss the proto3 zero-collapse case Phase 1 could only document.)
- [x] **VAL-06**: Schema-layer violations carry the protovalidate constraint ID and message and are consumable as a structured error outside any RPC context
- [x] **VAL-07**: A caller cannot tell whether a violation was caught at the boundary interceptor or at the storage layer — both produce the identical wire error
- [x] **VAL-08**: Message-level (cross-field) rules are boundary-only by default; `WithMessageRules(OnCreate)` opts into Create-time schema enforcement
- [x] **VAL-09**: Mixin hook ordering relative to schema-declared hooks and privacy policies is documented and covered by a test that fails if ent changes it
- [x] **VAL-10**: The boundary interceptor constructs its validator once per process, never per request
- [x] **VAL-11**: CI fails when the protovalidate/cel-go versions resolved by `mixinforproto` and by `entconnect` diverge

### CRUD Handler Generation

- [x] **CRUD-01**: entc extension generates a ConnectRPC handler for standard Get RPCs over a `MixinForProto`-backed entity
- [x] **CRUD-02**: Extension generates Create and Delete handlers operating directly against the ent client
- [x] **CRUD-03**: Extension generates List handlers with AIP-158 `page_token`/`next_page_token` paging built on hand-emitted keyset predicates over ent's per-field comparison operators (`LT`/`GT`/`EQ` with `And`/`Or`, `Order(...)`, `Limit(n+1)`) — not offset paging, and not the ent contrib GraphQL extension's generated helper, which is where that helper actually lives (see 02-RESEARCH.md Q1).

> **Correction (2026-08-08):** The original wording above attributed this paging mechanism to a
> keyset method native to core `entgo.io/ent`. That claim was disproven by direct source
> inspection of both `entgo.io/ent@v0.14.6` and `entgo.io/contrib/entgql` during Phase 2 research:
> no such method exists in core ent — it is generated exclusively by the ent contrib GraphQL
> extension's own templates. See
> `.planning/phases/02-crud-handlers-interceptor-chain/02-RESEARCH.md` §Summary and Pitfall 1.

- [x] **CRUD-04**: Extension generates Update handlers that require `google.protobuf.FieldMask` and gate every `Set*` call on the mask, so untouched fields are never zeroed
- [x] **CRUD-05**: Codegen validates every field-mask path against the message descriptor and fails the build on an unknown path
- [x] **CRUD-06**: Generated code is byte-stable across runs (sorted iteration, stable imports, gofmt-clean) and covered by golden-file tests
- [x] **CRUD-07**: Generated server wiring is the only place an ent client is used to serve requests, and it never hands a privileged client back to application code — the `*ent.Client` each generated service holds is unexported, and the returned server exposes only `Routes()`/`Register()`, with no accessor of any kind

> **Correction (2026-08-09):** The original wording also claimed generated wiring is *"the only
> place an ent client is constructed."* That half is not achievable and was never implemented:
> generated code cannot know a deployment's driver or connection string, so `NewServer` takes the
> client as a parameter (`NewServer(client *ent.Client, authenticator …, opts …)`) and the
> application constructs it. Verified live across all five Phase 2 fixtures, each of which calls
> `ent.NewClient(…)` itself before passing it in. Achieving the original wording would require
> moving connection configuration into codegen, which is worse design, not better.
>
> The security-relevant half is real and verified: no privileged client escapes the generated
> surface. `grep -rn 'func.*Client()' runtime/ entc/templates/` returns nothing, and every
> per-service `client` field is unexported. What is *not* claimed is that application code is
> prevented from holding its own client alongside — it necessarily does, at the same call site
> where it invokes `NewServer`.
>
> Root cause: `02-CONTEXT.md`'s D-11 and D-12 contradicted each other. D-11 specifies the
> client-as-parameter signature; D-12 restated this requirement's unachievable wording. D-12 is
> amended in place. See `02-VERIFICATION.md` for the full evidence.

### Interceptors & Runtime

- [x] **INT-01**: Generated interceptor chain runs in the fixed order authn → viewer injection → protovalidate → otel → handler
- [x] **INT-02**: Viewer injection places a viewer-scoped context on every request so ent privacy policies apply
- [x] **INT-03**: Privacy denials surface to clients as Connect `PermissionDenied`
- [x] **INT-04**: `entconnect.Manual("rpc")` lets a developer hand-write one handler, and the manual handler still runs inside the generated interceptor chain
- [x] **INT-05**: Drift-check output reports which RPCs are `Manual`, so the escape hatch stays visible

### Flow Binding

- [ ] **FLOW-01**: Extension matches an RPC to a flow by request type via entflow's metadata surface, without importing entflow's runtime
- [ ] **FLOW-02**: Flow-bound handlers decode, protovalidate, extract the viewer, call `flow.Start`, and shape the response — generated in full, never as an editable file
- [ ] **FLOW-03**: Synchronous flows answer their RPC directly; long-running flows answer with a run reference
- [ ] **FLOW-04**: Extension generates a `GetRunStatus` RPC per service for retrieving long-running flow state
- [ ] **FLOW-05**: `codec/proto` adapts `proto.Message` inputs to entflow's codec interface
- [ ] **FLOW-06**: entconnect's own test suite exercises flow binding against a fake metadata implementation, proving entconnect builds and is fully useful with zero flows

### Drift Check

- [ ] **DRIFT-01**: Build fails when a service RPC is claimed by neither a flow, a CRUD binding, nor an explicit `entconnect.Manual("rpc")`
- [ ] **DRIFT-02**: Build warns when a proto-input flow has no claiming RPC, silenceable by marking the flow internal
- [ ] **DRIFT-03**: Build fails when a skipped message-typed field has no declared edge of matching name/target and no explicit exclude
- [ ] **DRIFT-04**: Build warns when a `google.api.resource_reference` id field has no corresponding declared edge
- [ ] **DRIFT-05**: Build fails when a schema hand-declares a validator conflicting with a mirrored protovalidate constraint, detected by constraint fingerprint
- [ ] **DRIFT-06**: Build fails when the committed `FileDescriptorSet` is stale relative to `.proto` sources
- [ ] **DRIFT-07**: Drift-check output is deterministic and ordered, so CI diffs are meaningful

### Pipeline & Distribution

- [x] **PIPE-01**: A documented, scripted build pipeline runs `buf lint`, `buf generate`, the separate `buf build -o --as-file-descriptor-set` step, `go generate ./...`, and `atlas migrate diff` in order
- [x] **PIPE-02**: A `go.work` file makes local development across both modules work, and a `GOWORK=off` CI job proves `mixinforproto` is consumable at its tagged version
- [x] **PIPE-03**: CI enumerates and tests both modules explicitly rather than relying on `./...`
- [x] **PIPE-04**: `mixinforproto` releases under `mixinforproto/vX.Y.Z` tags and carries no `replace` directives
- [x] **PIPE-05**: A conformance corpus covers every field-mapping rule and every protovalidate constraint class, golden-asserted against derived fields
- [x] **PIPE-06**: A differential harness generates random values per corpus message and asserts `protovalidate verdict == ent mutation verdict` for field-scoped rules
- [ ] **PIPE-07**: A reference Order/Inventory application builds end-to-end through the full pipeline in CI, with connect-go client tests against generated handlers
- [x] **PIPE-08**: `mixinforproto` ships with documentation covering the proto3 presence/zero-collapse behavior prominently enough that adopters meet it before it surprises them

## v2 Requirements

Deferred to a future milestone. Tracked but not in the current roadmap.

### Additional Transports

- **GATE-01**: Extension passes through `google.api.http` annotations for grpc-gateway
- **GATE-02**: Extension validates `google.api.http` annotations against generated handlers
- **REST-01**: Extension emits chi/stdlib REST handlers from the same `google.api.http` annotations, replacing entrest
- **REST-02**: Emitters share one descriptor analysis pass; only output differs

### Async & Streaming

- **STREAM-01**: Connect server streaming of run-state transitions off the run table

### Mixin Extensions

- **MIX2-01**: `StrictPresence` option panics on non-optional scalars where zero/unset ambiguity would be silent
- **MIX2-02**: `debug_redact` field option infers `field.Sensitive()`
- **MIX2-03**: `WithMessageRules(OnUpdateWithFetch)` for update-time cross-field validation, with the query cost visible in the schema
- **MIX2-04**: `google.protobuf.Duration` maps to `field.Int64` nanoseconds via option

### Adoption

- **DOC2-01**: A migration document mapping entproto annotations to the entconnect model
- **DOC2-02**: Tooling that reports the `Manual()` usage ratio so the escape hatch stays uncomfortable

## Out of Scope

Explicitly excluded. Documented to prevent scope creep.

| Feature | Reason |
|---------|--------|
| Generating `.proto` from ent schemas | entproto's direction; the exact inversion this project exists to reject |
| Edge generation from message-typed fields | Protos express shape; edges carry direction, ownership, cascade, and uniqueness the contract does not state. Inferring them means inventing conventions |
| A bespoke REST mapping convention | `google.api.http` is the mapping language; entconnect reads it and never invents one |
| Update-time message-level validation by fetch-then-merge (default) | Hidden query cost and unclear semantics under concurrent writes; only ever as an explicit opt-in (v2 MIX2-03) |
| A silent hand-written-handler escape hatch | `Manual()` is explicit and reported by the drift check; silent overrides would defeat the drift guarantee |
| entflow itself | Separate project, separate repo, separate design doc. entconnect consumes only its metadata surface |
| Drift checking inside `mixinforproto` | A runtime mixin cannot see the service surface or other schemas' edges; cross-artifact validation is codegen's job |
| Custom proto field options as an annotation channel | Migrates schema semantics into the proto; `Override` is the answer until real demand appears |
| Offset-based list pagination | ent already has correct keyset pagination; offset paging is incorrect under concurrent writes |
| Infer-partial-update-from-set-fields | Cannot express "explicitly clear a non-optional proto3 scalar to zero" — the design's own sharpest edge |

## Traceability

Which phases cover which requirements. Updated during roadmap creation.

| Requirement | Phase | Status |
|-------------|-------|--------|
| MIX-01 | Phase 1 | Complete |
| MIX-02 | Phase 1 | Complete |
| MIX-03 | Phase 1 | Complete |
| MIX-04 | Phase 1 | Complete |
| MIX-05 | Phase 1 | Complete |
| MIX-06 | Phase 1 | Complete |
| MIX-07 | Phase 1 | Complete |
| MIX-08 | Phase 1 | Complete |
| MIX-09 | Phase 1 | Complete |
| MIX-10 | Phase 1 | Complete |
| MIX-11 | Phase 1 | Complete |
| MIX-12 | Phase 1 | Complete |
| MIX-13 | Phase 1 | Complete |
| MIX-14 | Phase 1 | Complete |
| ANNO-01 | Phase 1 | Complete |
| ANNO-02 | Phase 1 | Complete |
| ANNO-03 | Phase 1 | Complete |
| ANNO-04 | Phase 1 | Complete |
| VAL-01 | Phase 1 | Complete |
| VAL-02 | Phase 1 | Complete |
| VAL-03 | Phase 1 | Complete |
| PIPE-01 | Phase 1 | Complete |
| PIPE-02 | Phase 1 | Complete |
| PIPE-03 | Phase 1 | Complete |
| PIPE-04 | Phase 1 | Complete |
| PIPE-08 | Phase 1 | Complete |
| CRUD-01 | Phase 2 | Complete |
| CRUD-02 | Phase 2 | Complete |
| CRUD-03 | Phase 2 | Complete |
| CRUD-04 | Phase 2 | Complete |
| CRUD-05 | Phase 2 | Complete |
| CRUD-06 | Phase 2 | Complete |
| CRUD-07 | Phase 2 | Complete |
| INT-01 | Phase 2 | Complete |
| INT-02 | Phase 2 | Complete |
| INT-03 | Phase 2 | Complete |
| INT-04 | Phase 2 | Complete |
| INT-05 | Phase 2 | Complete |
| VAL-04 | Phase 3 | Complete |
| VAL-05 | Phase 3 | Complete |
| VAL-06 | Phase 3 | Complete |
| VAL-07 | Phase 3 | Complete |
| VAL-08 | Phase 3 | Complete |
| VAL-09 | Phase 3 | Complete |
| VAL-10 | Phase 3 | Complete |
| VAL-11 | Phase 3 | Complete |
| PIPE-05 | Phase 3 | Complete |
| PIPE-06 | Phase 3 | Complete |
| FLOW-01 | Phase 4 | Pending |
| FLOW-02 | Phase 4 | Pending |
| FLOW-03 | Phase 4 | Pending |
| FLOW-04 | Phase 4 | Pending |
| FLOW-05 | Phase 4 | Pending |
| FLOW-06 | Phase 4 | Pending |
| DRIFT-01 | Phase 5 | Pending |
| DRIFT-02 | Phase 5 | Pending |
| DRIFT-03 | Phase 5 | Pending |
| DRIFT-04 | Phase 5 | Pending |
| DRIFT-05 | Phase 5 | Pending |
| DRIFT-06 | Phase 5 | Pending |
| DRIFT-07 | Phase 5 | Pending |
| PIPE-07 | Phase 5 | Pending |

**Coverage:**

- v1 requirements: 62 total (corrected from initial count of 57 during roadmap creation — recount against the checklist confirms 62 `- [ ]` v1 items)
- Mapped to phases: 62/62 ✓
- Unmapped: 0 ✓

---
*Requirements defined: 2026-08-08*
*Last updated: 2026-08-08 after roadmap creation (5 phases, 62/62 requirements mapped)*
