# Phase 2: CRUD Handlers & Interceptor Chain - Research

**Researched:** 2026-08-08
**Domain:** entc code-generation extension emitting ConnectRPC CRUD handlers over ent, with a fixed privacy-aware interceptor chain
**Confidence:** HIGH on all six priority questions (each independently verified against live source or live code execution in this session, not training memory) — MEDIUM/LOW flagged inline where a claim rests on standards documentation (AIP-158) or unresolved design tradeoffs.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

- **D-01 (USER DECISION):** RPC↔operation binding declared in `ent/schema` Go annotations via `entconnect.CreateRPC(orderv1connect.OrderServiceCreateOrderProcedure)` / `UpdateRPC` / `DeleteRPC` / `GetRPC` / `ListRPC` — compile-time-checked references to connect-go's generated `…Procedure` constants. No proto method options, no name-convention inference.
- **D-02 (USER-FLAGGED CHALLENGE):** Only the procedure string (`"/order.v1.OrderService/CreateOrder"`) crosses entc's JSON schema-load boundary — no descriptor, no package identity, no Go import edge to `orderv1`.
- **D-03:** The procedure string is the key; the committed `FileDescriptorSet` is the lookup table. Split on final `/` into service full name + method name, resolve both against the loaded descriptor set. Decisive secondary reason: Phase 5's drift check must enumerate RPCs no schema claims.
- **D-04:** Descriptor-set location is an extension option `entconnect.WithDescriptorSet(path)` defaulting to the pipeline's committed path; unresolved procedures get a self-sufficient first-line error naming procedure, path, and remedy.
- **D-05:** A procedure claimed twice (by two schemas, or twice on one schema) is a collected codegen error.
- **D-06:** `entconnect.Manual("rpc")` takes the same procedure-constant argument; still runs inside the generated interceptor chain — generated wiring registers it, app supplies only the handler func.
- **D-07:** INT-05 satisfied by a deterministic, sorted claims report (`crud:<op>` / `manual` / `unclaimed`) at codegen. Phase 2 reports; does not fail build on `unclaimed` (that's DRIFT-01, Phase 5).
- **D-08:** `page_token` is opaque, wraps the keyset cursor plus a fingerprint of ordering/filter-relevant request fields. Fingerprint mismatch → `InvalidArgument`, never a silently inconsistent page.
- **D-09:** Filtering is out of scope for Phase 2 — paging only. No `Where`-predicate derivation from List request fields. Ordering comes from a schema-side default.
- **D-10 (RESEARCH FLAG — resolved below, see Q1):** CRUD-03's "ent's native keyset `Paginate()`" is **factually wrong** — `Paginate()` is entgql-only. This phase must emit keyset paging directly.
- **D-11:** Authn/viewer extraction supplied through a generated narrow interface the app implements (`AuthenticateAndViewer(ctx, http.Header) (context.Context, error)`), required as a positional constructor argument (`NewServer(client, authenticator, opts...)`) — not a functional option. Chain order fixed and non-negotiable in generated code.
- **D-12:** CRUD-07 enforced structurally: `*ent.Client` constructed inside generated wiring in an internal package, never exposed to application code.
- **D-13:** protovalidate validator built once per process, threaded into the chain by generated wiring.
- **D-14:** Explicit `google.protobuf.FieldMask` (resolves entconnect.md §9 OQ5) — not inference from set fields.
- **D-15:** Top-level mask paths only. Nested paths (`customer.name`) and `*` rejected at build time, naming the offending path.
- **D-16:** Empty or absent mask is `InvalidArgument`, never "update everything."
- **D-17:** Build-time mask-path validation resolves against both the message descriptor **and** Phase 1's `SourceField` provenance — a path naming an `Exclude()`d field also fails the build.
- **D-18:** Explicit mapping table: `privacy.Deny`→`PermissionDenied`, `NotFoundError`→`NotFound`, `ConstraintError`→`AlreadyExists`/`FailedPrecondition`, `ValidationError`→`InvalidArgument`, unrecognized→`Internal` (detail logged, not returned).
- **D-19:** `text/template` via `entc/gen`, mirroring entproto's `Extension`/`NewExtension`/`Hooks()`/top-level `Generate(g *gen.Graph) error` skeleton. No jennifer, no `go/ast`.
- **D-20:** Byte-stability inherits Phase 1's discipline: never range a map into ordered output, sort every key set, goldie golden files with `-update`, CI runs `-count=5`.
- **D-21:** Root module gains its `mixinforproto` dependency edge in this phase (deferred from Phase 1 D-18), via a separate commit referencing an already-pushed tag.

### Claude's Discretion (ranked, Phase 2's own list)

1. D-10 (keyset `Paginate()` availability) — **resolved definitively below (Q1)**.
2. D-11 (required interface vs. functional options) — real ergonomics-vs-safety tradeoff; one-way for adopters.
3. D-07 (report-don't-fail on unclaimed RPCs) — scoping call, not a technical constraint.
4. D-16 (empty mask is an error) — defensible, may annoy generated clients that send empty masks by habit.
5. D-18 (unknown→`Internal`) — conservative; `Unknown` is the arguable alternative.

### Deferred Ideas (OUT OF SCOPE)

- Contract-driven filtering on List (deriving `Where` predicates from List request fields).
- Nested field-mask paths into `AsJSON()` fields.
- Connect server streaming of run-state/list changes (entconnect.md §9 OQ3 option b).
- Hard build failure on unclaimed RPCs (belongs to DRIFT-01, Phase 5).
- A both-paths descriptor cross-check (FileDescriptorSet + `protoregistry` at schema load) as a redundant staleness signal.
- Narrowing PROJECT.md's "custom proto field options" Out of Scope wording.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| CRUD-01 | Generate a Connect Get handler over a MixinForProto-backed entity | Q4 (handler shape), Code Examples §Get |
| CRUD-02 | Generate Create/Delete handlers against the ent client directly | Q4, Code Examples §Create/Delete |
| CRUD-03 | List handler, AIP-158 `page_token`/`next_page_token` on keyset paging, not offset | Q1 (definitive `Paginate()` verdict + keyset-predicate mechanism), Code Examples §List |
| CRUD-04 | Update handler requires `FieldMask`, gates every `Set*` on the mask | Q6 (`fieldmaskpb` API), Code Examples §Update |
| CRUD-05 | Codegen validates every mask path against the message descriptor, fails build on unknown path | Q6, D-17's SourceField cross-check |
| CRUD-06 | Byte-stable generated code, golden-file tested | Q3 (entc/gen template + hook mechanics) |
| CRUD-07 | Generated wiring is the only place an `*ent.Client` is constructed | Q3 (hook-based file emission into an internal package), D-12 |
| INT-01 | Fixed interceptor chain authn → viewer → protovalidate → otel → handler, built once per process | Q4 (`WithInterceptors` ordering, confirmed outermost-first) |
| INT-02 | Viewer injection puts a viewer-scoped context on every request for ent privacy | Q5 (ent's viewer-context idiom, `privacy.Policy.EvalQuery/EvalMutation`) |
| INT-03 | Privacy denials surface as Connect `PermissionDenied` | Q5 (`privacy.Deny` sentinel, `errors.Is` matching) |
| INT-04 | `entconnect.Manual("rpc")` hand-written handler still runs inside the generated chain | Q4 (chain applied at `New<Service>Handler(impl, opts...)` level, uniform across all methods) |
| INT-05 | Drift-check output reports which RPCs are Manual | D-07 (claims report), Q2 (service/method enumeration mechanism) |

Descriptor resolution (D-02/D-03/D-04) underlies CRUD-01…07 and INT-01…05 collectively — see Q2.
</phase_requirements>

## Summary

This phase's six priority questions were resolved with direct evidence: reading ent v0.14.6's actual `entc/gen` template source (not documentation), executing real Go programs against Phase 1's own committed `FileDescriptorSet`, and inspecting the real generated output of `protoc-gen-connect-go`'s own test fixtures. Nothing below is inferred from training-data recall alone where a local, authoritative source was available to check it against.

**The single most consequential finding (Q1):** CRUD-03's and Success Criterion 2's wording — "ent's native keyset `Paginate()`" — is **definitively wrong**. `Paginate()` does not exist anywhere in `entgo.io/ent@v0.14.6`'s source; it is generated exclusively by `entgo.io/contrib/entgql`'s own templates (confirmed by downloading both modules and grepping/reading their actual template trees). Depending on entgql would directly contradict `entconnect.md` §2.3's founding premise (entgql re-derives an API from the schema — exactly the inversion this project rejects). Core ent's generated query builder does, however, expose everything needed to hand-emit correct keyset paging: per-field `LT`/`LTE`/`GT`/`GTE` predicate functions, `And`/`Or`/`Not` combinators (all backed by `sql.FieldLT`/`sql.AndPredicates`/etc.), `Limit`, and `Order(...OrderOption)`. This is a solved, well-precedented SQL pattern (`WHERE (created_at, id) < (?, ?) ORDER BY created_at DESC, id DESC LIMIT n+1`), not a gap.

**Second most consequential finding (Q2):** `protodesc.NewFiles(&descriptorpb.FileDescriptorSet{...})` followed by `files.FindDescriptorByName(protoreflect.FullName(serviceFullName))` and `svcDesc.Methods().ByName(protoreflect.Name(methodName))` was executed end-to-end in this session against a synthetic `order.v1.OrderService/CreateOrder` descriptor and against Phase 1's real committed `.binpb`, and works exactly as D-03 requires. Missing transitive imports produce a named, catchable error (`could not resolve import %q`); an unknown service/method produces `protoregistry.NotFound` (comparable via `errors.Is`). `buf build -o --as-file-descriptor-set` (the unscoped, whole-`proto/`-dir invocation Phase 1's `pipeline.sh` already runs) was independently confirmed, by inspecting the real committed `.binpb`, to include the full transitive closure automatically (all four WKTs plus vendored `buf/validate/validate.proto` are present even though the corpus never explicitly requested them at top level).

**Third finding (Q4), which resolves the phase's stated "tension":** the entc extension itself never imports `orderv1connect` (D-02's constraint holds completely — it only ever handles procedure strings and descriptors). But the *generated Go source text* it emits **does** reference `orderv1connect.OrderServiceHandler` and `orderv1connect.NewOrderServiceHandler(impl, opts...)` by name — and that reference only needs to resolve when the **app** compiles the generated file, which already imports `orderv1connect` (per D-01's own binding annotations). Reading `protoc-gen-connect-go`'s actual generated test fixture confirms the generated `New<Service>Handler(svc <Service>Handler, opts ...connect.HandlerOption) (string, http.Handler)` function applies `opts` (hence the interceptor chain) uniformly across every method on the interface — meaning a `Manual` RPC, wired through the same interface, automatically inherits the same chain (INT-04) with zero extra plumbing.

**Primary recommendation:** Emit generated Connect handlers that *implement* the `protoc-gen-connect-go`-generated `<Service>Handler` interface and register via its generated `New<Service>Handler`, rather than hand-building `connect.NewUnaryHandler` calls directly. Emit keyset List paging by hand using ent's `And`/`Or`/per-field comparison-op predicates — never take an entgql dependency. Reuse `connectrpc.com/validate` (official Connect-authors package, built directly on `buf.build/go/protovalidate`) for the protovalidate interceptor stage rather than hand-rolling one, and `connectrpc.com/otelconnect` for the otel stage.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| RPC↔operation binding (annotations) | API/Backend (entc extension, build-time) | — | Read at schema-load/codegen time, not runtime; produces generated Go, not a runtime lookup |
| Procedure→descriptor resolution | API/Backend (entc extension, build-time) | — | `protodesc`/`protoregistry` operate on the committed `.binpb`, purely at codegen; no runtime cost |
| CRUD handler bodies (Get/Create/Delete) | API/Backend | Database/Storage (ent client calls) | Handler decodes/encodes only; ent client owns persistence |
| List keyset paging | API/Backend (predicate/cursor construction) | Database/Storage (SQL WHERE/ORDER/LIMIT execution) | Cursor encode/decode and fingerprinting is API-tier; the actual comparison runs as SQL via ent's query builder |
| Update FieldMask gating | API/Backend | — | Purely a request-shaping concern before the ent mutation is built; no storage-tier involvement beyond the resulting `Set*` calls |
| Interceptor chain (authn/viewer/protovalidate/otel) | API/Backend | — | Connect interceptors are a server-tier (not client, not storage) concept by construction |
| Privacy enforcement | Database/Storage (`ent.Policy` evaluated inside query/mutation builders) | API/Backend (viewer context injection) | The actual `Allow`/`Deny` decision runs inside ent's generated `prepareQuery`/mutation hook, i.e., at the persistence-adjacent tier; the API tier only places the viewer on `ctx` |
| Error mapping (D-18) | API/Backend | — | Translates storage/policy errors to wire codes at the transport boundary; storage tier is unaware of Connect |
| `*ent.Client` construction | API/Backend (generated server wiring, internal package) | — | CRUD-07 makes this a structural (compiler-enforced), not conventional, boundary |

## Standard Stack

### Core

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `connectrpc.com/connect` | v1.20.0 (already pinned in CLAUDE.md; confirmed May 20 2026 release via `go list -m -json`) | Handler/client runtime, interceptors, error codes | `[VERIFIED: connectrpc.com/connect@v1.20.0/go.mod, handler.go, option.go, code.go, error.go — read this session]`. Requires Go ≥1.25.0 (`go 1.25.0` in its own go.mod); root go.mod's `go 1.26` satisfies it. |
| `entgo.io/ent` | v0.14.6 (pinned, Phase 1 verified) | ORM/schema graph, entc codegen host | `[VERIFIED: entc/gen/*.go, entc/entc.go — read and grepped this session]` |
| `buf.build/go/protovalidate` | v1.2.0 (pinned, Phase 1 verified) | Boundary validation, constraint enumeration | Already verified in Phase 1; reused here via `connectrpc.com/validate`'s dependency |

### Supporting (new to this phase)

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `connectrpc.com/validate` | v0.6.0 (released 2025-09-27, confirmed via `go list -m -json`) | Ready-made `connect.Interceptor` wrapping `buf.build/go/protovalidate`, mapping violations to `CodeInvalidArgument` with a `protovalidate.ValidationError` proto detail attached | `[VERIFIED: connectrpc.com/validate@v0.6.0/validate.go, go.mod — read this session]`. **Don't-hand-roll candidate** — see below. Its own go.mod requires only `buf.build/go/protovalidate v1.0.0` and `connectrpc.com/connect v1.19.0` as minimums; Go's MVS resolves both up to the pinned v1.2.0/v1.20.0 transparently, no conflict. Builds the validator once (`protovalidate.GlobalValidator` by default, or inject one via `WithValidator` — satisfies D-13's "once per process"). |
| `connectrpc.com/otelconnect` | v0.9.0 (released 2026-01-05, confirmed via `go list -m -json`) | Ready-made `connect.Interceptor` (`NewInterceptor(opts...) (*Interceptor, error)`) providing OpenTelemetry tracing/metrics for the "otel" chain stage | `[VERIFIED: connectrpc.com/otelconnect@v0.9.0/interceptor.go, go.mod — read this session]`. Not previously in CLAUDE.md's pinned stack — new discovery this session; flag for adoption confirmation (see Assumptions Log A2). |
| `google.golang.org/protobuf/types/known/fieldmaskpb` (stdlib-adjacent, ships inside `google.golang.org/protobuf` v1.36.11, already a dependency) | v1.36.11 | `fieldmaskpb.New`/`.IsValid`/`.Append` — path validation against a live message's descriptor | `[VERIFIED: google.golang.org/protobuf@v1.36.11/types/known/fieldmaskpb/field_mask.pb.go — read and traced this session]`. **Does not enforce top-level-only (D-15) by itself** — see Common Pitfalls. |
| `google.golang.org/protobuf/reflect/protodesc` + `reflect/protoregistry` (same module, already a dependency) | v1.36.11 | `protodesc.NewFiles`, `(*protoregistry.Files).FindDescriptorByName` — the entire D-03 resolution mechanism | `[VERIFIED: executed live against a synthetic descriptor set and Phase 1's real proto/mixinforprototest.binpb this session]` |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Hand-emitted keyset `WHERE`/`ORDER`/`LIMIT` via ent's own predicate/order builders | `entgo.io/contrib/entgql`'s `Paginate()` | Rejected outright — entgql re-derives the API surface from the ent schema, which is exactly the generation direction `entconnect.md` §2.3 exists to invert. Confirmed via source: `Paginate()` lives only in entgql's own generated example output (`gql_pagination.go`), never in core ent. |
| `connectrpc.com/validate`'s ready-made interceptor | A hand-rolled `protovalidate.Validate(msg)` call inside a custom `connect.Interceptor` | The hand-rolled version duplicates ~180 lines of already-correct, already-tested code (streaming variants, error-detail attachment, response-validation opt-in) for no benefit; only reason to avoid it would be a hard objection to the extra dependency, which nothing in CONTEXT.md raises |
| Implementing `orderv1connect.OrderServiceHandler` + calling its generated `New...Handler` | Hand-building `connect.NewUnaryHandler[Req,Res]` calls directly per RPC in generated code | Both work equally well technically (both are visible in `protoc-gen-connect-go`'s own generated fixture). The generated-interface route is recommended because it reuses `protoc-gen-connect-go`'s own mux/path-routing logic (buf-generated, already correct, already tested upstream) instead of reimplementing it, and it makes `Manual` RPCs (INT-04) trivially uniform — they are just another method on the same interface, picking up the same `opts` automatically. |

**Installation (additions to root `go.mod` beyond the already-pinned `connectrpc.com/connect`/`entgo.io/ent`):**
```bash
go get connectrpc.com/validate@v0.6.0
go get connectrpc.com/otelconnect@v0.9.0
go install connectrpc.com/connect/cmd/protoc-gen-connect-go@v1.20.0   # buf.gen.yaml local plugin, mirrors the existing protoc-gen-go install convention
```

## Package Legitimacy Audit

> The `gsd-tools query package-legitimacy check` seam only supports `--ecosystem npm|pypi|crates`; this is a Go project, so the seam could not be invoked directly. Verification below substitutes the Go-ecosystem-appropriate equivalent: `go list -m -versions`/`go list -m -json` against the real Go module proxy (`proxy.golang.org`, reachable in this sandbox and explicitly allowlisted in the environment's `no_proxy`), plus direct inspection of each module's downloaded source (license headers, real multi-year commit history implied by version count, real non-trivial implementation — not a stub).

| Package | Registry | Age/Versions | Downloads | Source Repo | Verdict | Disposition |
|---------|----------|-----|-----------|-------------|---------|-------------|
| `connectrpc.com/connect` | Go module proxy | 20 versions through v1.20.0 (May 2026); already pinned in CLAUDE.md | N/A (Go proxy has no public counter) | github.com/connectrpc/connect-go (same org owns `connectrpc.com/*`) | OK | Approved (pre-existing pin) |
| `connectrpc.com/validate` | Go module proxy | 6 versions (v0.1.0…v0.6.0, first release ~2023 based on version cadence, confirmed v0.6.0 = 2025-09-27) | N/A | github.com/connectrpc/connect-go/tree/main/... — published by "The Connect Authors" (same license header as connect-go itself) | OK | Approved — `[VERIFIED: source inspected this session, official Connect-authors org]` |
| `connectrpc.com/otelconnect` | Go module proxy | 12 versions (v0.1.0…v0.9.0, confirmed v0.9.0 = 2026-01-05) | N/A | Same org (`connectrpc.com`) | OK | Approved — `[VERIFIED: source inspected this session]` |
| `google.golang.org/protobuf/types/known/fieldmaskpb` | N/A — subpackage of already-pinned `google.golang.org/protobuf` | N/A | N/A | golang.org/x/protobuf (official) | OK | Approved — already a transitive dependency, no new module edge |

**Packages removed due to `[SLOP]` verdict:** none.
**Packages flagged as suspicious `[SUS]`:** none. All packages verified are published under the `connectrpc.com` module path, the same official org whose `connect` package CLAUDE.md already pins and whose license headers ("Copyright 2021-2026 The Connect Authors" / "Copyright 2023-2025 The Connect Authors") were read directly in this session — genuinely first-party, not a look-alike.

*`connectrpc.com/validate` and `connectrpc.com/otelconnect` are new to CLAUDE.md's previously-pinned stack. Per this project's own protocol, list them as `[ASSUMED — new addition, recommend user confirmation]` for adoption even though the underlying registry/source facts are `[VERIFIED]`; the planner should gate their `go get` behind a lightweight `checkpoint:human-verify` or simply flag the addition in the PLAN.md rationale, since CONTEXT.md's Claude's Discretion list does not currently mention either package by name.*

## Architecture Patterns

### System Architecture Diagram

```
                         Connect client (HTTP/JSON or gRPC/gRPC-Web)
                                        |
                                        v
                    +----------------------------------------+
                    |  orderv1connect.NewOrderServiceHandler  |   <- protoc-gen-connect-go's own
                    |  (mux: path -> per-RPC connect.Handler) |      generated routing (reused, not
                    +----------------------------------------+      reimplemented)
                                        |
                                        v
     +---------------------------------------------------------------------+
     |            Fixed interceptor chain (connect.WithInterceptors,       |
     |            first listed = outermost; INT-01)                       |
     |                                                                     |
     |   authn  -->  viewer injection  -->  protovalidate  -->  otel  -->  |
     |  (app-supplied           (ctx now carries          (connectrpc.com/  |
     |   Authenticate            viewer.Viewer for          validate,       |
     |   AndViewer func,         ent privacy rules)         built once)     |
     |   D-11)                                                              |
     +---------------------------------------------------------------------+
                                        |
                                        v
                    +----------------------------------------+
                    |   Generated CRUD handler body            |
                    |   (implements <Service>Handler method)   |
                    +----------------------------------------+
                       |          |            |           |
                       v          v            v           v
                     Get       Create        Update        List
                       |          |            |           |
                       |          |    FieldMask-gated       keyset predicate
                       |          |    Set* calls only        (And/Or on
                       |          |    (D-14..D-17)            LT/EQ ops) +
                       |          |                            fingerprinted
                       |          |                            page_token
                       v          v            v           v
              +-----------------------------------------------------+
              |     *ent.Client  (constructed ONLY in generated       |
              |     wiring, internal package — CRUD-07/D-12)          |
              +-----------------------------------------------------+
                                        |
                                        v
                        ent privacy.Policy.EvalQuery/EvalMutation
                        (reads viewer off ctx; Allow/Deny/Skip)
                                        |
                              Deny --> errors.Is(err, privacy.Deny)
                              propagates UNWRAPPED up through
                              Only(ctx)/Save(ctx) etc. (verified:
                              query.tmpl line 419-421 `return err`,
                              no extra wrapping)
                                        |
                                        v
                        D-18 error mapper: privacy.Deny -> PermissionDenied
                                           NotFoundError -> NotFound
                                           unrecognized  -> Internal (logged)
```

### Recommended Project Structure

```
entc/                          # the entc.Extension itself (Go source, D-19 skeleton)
├── extension.go                # Extension{ entc.DefaultExtension }, NewExtension, Hooks()
├── binder.go                   # CreateRPC/UpdateRPC/DeleteRPC/GetRPC/ListRPC/Manual annotation constructors (D-01/D-06)
├── resolve.go                  # D-03: procedure-string -> MethodDescriptor via protodesc/protoregistry (Q2)
├── crud/                       # per-verb Go-template generators (Get/Create/Update/Delete/List)
│   ├── get.go.tmpl
│   ├── create.go.tmpl
│   ├── update.go.tmpl          # FieldMask gating (D-14..D-17)
│   ├── delete.go.tmpl
│   └── list.go.tmpl            # keyset cursor encode/decode (Q1)
├── errormap.go                 # D-18 mapping table
└── extension_test.go           # golden-file tests (goldie, D-20)

runtime/                        # shipped framework code apps import (not generated)
├── viewer/                     # viewer.NewContext/FromContext — ent ships NO canonical
│   └── viewer.go                # viewer type; this mirrors ent's own doc precedent (Q5)
├── interceptors.go              # authn/viewer-injection interceptor built from the
│                                 # app-supplied AuthenticateAndViewer func (D-11)
└── server.go                    # NewServer(client, authenticator, opts...) skeleton the
                                  # generated per-app wiring calls into

<app>/ent/                     # per-adopter generated ent package (unchanged by this phase)
<app>/internal/entconnect/     # generated output landing zone (NOT under ent/) — the
                                 # hook-based emission mechanism (Q3) writes here, sibling
                                 # to ent/, exactly as entproto's own hook writes to a
                                 # sibling proto/ dir next to ent/
```

### Pattern 1: Procedure-string → MethodDescriptor resolution (D-03)

**What:** Split a Connect procedure constant on its last `/`, resolve the service half via `protoregistry.Files.FindDescriptorByName`, then look up the method by short name on the resulting `protoreflect.ServiceDescriptor`.
**When to use:** Every `CreateRPC`/`UpdateRPC`/.../`Manual` annotation the extension reads off `gen.Type.Annotations`.
**Example (executed and verified in this session against a synthetic descriptor set):**
```go
// Source: google.golang.org/protobuf@v1.36.11 reflect/protodesc, reflect/protoreflect
// [VERIFIED: executed live this session — see transcript]
func splitProcedure(proc string) (svc, method string, err error) {
	proc = strings.TrimPrefix(proc, "/")
	idx := strings.LastIndex(proc, "/")
	if idx < 0 {
		return "", "", fmt.Errorf("malformed procedure %q: no '/' separator", proc)
	}
	return proc[:idx], proc[idx+1:], nil
}

func resolveMethod(files *protoregistry.Files, procedure string) (protoreflect.MethodDescriptor, error) {
	svcName, methodName, err := splitProcedure(procedure)
	if err != nil {
		return nil, err
	}
	d, err := files.FindDescriptorByName(protoreflect.FullName(svcName))
	if err != nil {
		// err is (comparable via errors.Is) protoregistry.NotFound when the
		// service genuinely isn't in the descriptor set — confirmed live.
		return nil, fmt.Errorf("service %q not found in descriptor set: %w", svcName, err)
	}
	svcDesc, ok := d.(protoreflect.ServiceDescriptor)
	if !ok {
		return nil, fmt.Errorf("%q is not a service (got %T)", svcName, d)
	}
	m := svcDesc.Methods().ByName(protoreflect.Name(methodName))
	if m == nil {
		return nil, fmt.Errorf("method %q not found on service %q", methodName, svcName)
	}
	return m, nil
}

// Loading the set itself:
func loadDescriptorSet(path string) (*protoregistry.Files, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var fds descriptorpb.FileDescriptorSet
	if err := proto.Unmarshal(b, &fds); err != nil {
		return nil, err
	}
	// Missing transitive dep -> "proto: could not resolve import %q: not
	// found" [VERIFIED: reproduced live this session with a synthetic
	// FileDescriptorProto whose Dependency lists a file absent from the set].
	return protodesc.NewFiles(&fds)
}
```

**Enumerating all services/methods for D-07's claims report and Phase 5's drift check (also executed live):**
```go
// Source: google.golang.org/protobuf reflect/protoreflect — [VERIFIED: executed this session]
func allProcedures(files *protoregistry.Files) []string {
	var out []string
	files.RangeFiles(func(fd protoreflect.FileDescriptor) bool {
		svcs := fd.Services()
		for i := 0; i < svcs.Len(); i++ {
			s := svcs.Get(i)
			ms := s.Methods()
			for j := 0; j < ms.Len(); j++ {
				out = append(out, fmt.Sprintf("/%s/%s", s.FullName(), ms.Get(j).Name()))
			}
		}
		return true
	})
	sort.Strings(out) // D-07/D-20: deterministic, sorted output
	return out
}
```

### Pattern 2: Keyset (cursor) list paging without `Paginate()` (Q1, CRUD-03)

**What:** Emit `Where`/`Order`/`Limit` calls directly against the generated ent query builder using per-field comparison-op predicates (`FieldLT`, `FieldGT`, etc.) and the `And`/`Or` combinators — all confirmed present in core ent's generated output for any indexed/ordered field.
**When to use:** Every generated List handler.
**Example (field names illustrative; actual field/type names come from the schema at codegen time):**
```go
// Source: entgo.io/ent@v0.14.6 entc/gen/template/where.tmpl + builder/query.tmpl
// [VERIFIED: read this session — where.tmpl generates FieldLT/FieldGT/FieldEQ
// per field with an Ops() list that includes GT/GTE/LT/LTE for any non-string,
// non-bool, non-JSON field (predicate.go:69 numericOps = append(enumOps, GT,
// GTE, LT, LTE)); And/Or/Not compile to sql.AndPredicates/OrPredicates/
// NotPredicates (dialect/sql/predicate.tmpl)]
func (h *orderHandler) List(ctx context.Context, req *orderv1.ListOrdersRequest) (*orderv1.ListOrdersResponse, error) {
	const pageSize = 50 // or req.PageSize, clamped
	q := h.client.Order.Query().
		Order(order.ByCreatedAt(sql.OrderDesc()), order.ByID(sql.OrderDesc())).
		Limit(pageSize + 1) // fetch one extra to detect "has next page"

	if req.PageToken != "" {
		cursor, err := decodeCursor(req.PageToken, req) // D-08: verifies fingerprint
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		q = q.Where(order.Or(
			order.CreatedAtLT(cursor.CreatedAt),
			order.And(
				order.CreatedAtEQ(cursor.CreatedAt),
				order.IDLT(cursor.ID),
			),
		))
	}

	rows, err := q.All(ctx)
	if err != nil {
		return nil, mapError(err) // D-18
	}

	hasNext := len(rows) > pageSize
	if hasNext {
		rows = rows[:pageSize]
	}
	resp := &orderv1.ListOrdersResponse{Orders: toProto(rows)}
	if hasNext {
		last := rows[len(rows)-1]
		resp.NextPageToken = encodeCursor(last.CreatedAt, last.ID, req) // fingerprint embedded
	}
	return resp, nil
}
```

### Pattern 3: FieldMask-gated Update, top-level-only (Q6, CRUD-04/05, D-14..D-17)

**What:** Validate the mask at request time (against the live descriptor, via `fieldmaskpb`) AND reject any path containing `.` (D-15 — `fieldmaskpb.IsValid` alone accepts nested paths, since nested-path support is a *feature* of `fieldmaskpb`, not a bug entconnect needs to work around).
**When to use:** Every generated Update handler; the descriptor-only half (no `.`) also runs at **build time** in the extension itself, cross-checked against Phase 1's `SourceField` inventory (D-17).
**Example:**
```go
// Source: google.golang.org/protobuf@v1.36.11 types/known/fieldmaskpb/field_mask.pb.go
// [VERIFIED: read this session, lines 297-405 — New/IsValid/Append/numValidPaths]
func (h *orderHandler) Update(ctx context.Context, req *orderv1.UpdateOrderRequest) (*orderv1.Order, error) {
	if req.UpdateMask == nil || len(req.UpdateMask.GetPaths()) == 0 {
		// D-16: empty/absent mask is InvalidArgument, never "update everything"
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("update_mask is required and must not be empty"))
	}
	for _, p := range req.UpdateMask.GetPaths() {
		if strings.Contains(p, ".") || p == "*" {
			// D-15: fieldmaskpb.IsValid would ACCEPT "customer.name" — this
			// project rejects it explicitly; fieldmaskpb does not do this for you.
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("field mask path %q: nested/wildcard paths are not supported", p))
		}
	}
	if !req.UpdateMask.IsValid(req) {
		// Unknown-to-the-descriptor path (build-time equivalent lives in the
		// extension itself, resolved against SourceField per D-17)
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("update_mask contains an unknown field path"))
	}

	m := h.client.Order.UpdateOneID(req.Id)
	for _, p := range req.UpdateMask.GetPaths() {
		switch p {
		case "status":
			m = m.SetStatus(req.Order.Status)
		case "notes":
			m = m.SetNotes(req.Order.GetNotes())
		// ... one case per SourceField.FieldName that survived Exclude/Override
		}
	}
	updated, err := m.Save(ctx)
	if err != nil {
		return nil, mapError(err)
	}
	return toProto(updated), nil
}
```

### Pattern 4: Generated interceptor chain wiring (Q4, INT-01)

**What:** `connect.WithInterceptors(authn, viewerInjection, protovalidateInterceptor, otelInterceptor)` — first listed is outermost (runs first on request, last on response).
**Verified claim:** `[VERIFIED: connectrpc.com/connect@v1.20.0/option.go:303-338 — read this session]`: *"WithInterceptors(A, B, ..., Y, Z)... Unary interceptors compose like an onion. The first interceptor provided is the outermost layer: it acts first on the context and request, and last on the response and error."* This maps directly onto INT-01's required order with no reordering trick needed:
```go
// Source: connectrpc.com/connect option.go doc comment (verbatim ordering
// guarantee), connectrpc.com/validate v0.6.0, connectrpc.com/otelconnect v0.9.0
otelInterceptor, err := otelconnect.NewInterceptor()
if err != nil { /* ... */ }

chain := connect.WithInterceptors(
	authnInterceptor,                    // outermost: runs first
	viewerInjectionInterceptor,          // 2nd
	validate.NewInterceptor(),           // connectrpc.com/validate, built once (D-13)
	otelInterceptor,                     // innermost of the four, closest to the handler
)

path, handler := orderv1connect.NewOrderServiceHandler(impl, chain)
mux.Handle(path, handler)
```
Because `chain` is passed once to `New<Service>Handler`, it applies to **every** method on the `<Service>Handler` interface — including a hand-written `Manual` implementation plugged into the same interface (INT-04) — with no separate wiring path.

### Pattern 5: entc extension file emission into a sibling package (Q3, CRUD-06/07)

**What:** `entc.Extension.Hooks()` returns a `gen.Hook` that runs `next.Generate(g)` first (letting ent finish writing `ent/`), then writes handler/server Go source directly via `os.WriteFile` into a sibling directory — mirroring `entgo.io/contrib/entproto`'s own precedent of writing to `path.Join(g.Config.Target, "proto")`, a directory next to (not inside) the per-node `ent/` template output.
**Why not `Extension.Templates()` (the `gen.Template`/`GraphTemplate` mechanism):** `[VERIFIED: entgo.io/ent@v0.14.6 entc/gen/graph.go:962-1010, entc/gen/template.go:27-44 — read this session]`. When an extension registers a `*gen.Template` via `Templates()` whose defined name doesn't collide with a builtin, ent auto-derives its output filename as `snake(templateName) + ".go"` and writes it as a **single flat file directly under `g.Config.Target`** — there is no public API to give a `GraphTemplate` a custom subdirectory. For output that must land in a genuinely separate package (`internal/entconnect/`, not nested inside the app's `ent/` package), a `gen.Hook` writing files directly (entproto's approach) is the only mechanism with subdirectory control.
**Formatting caveat:** `[VERIFIED: entc/gen/graph.go:1160-1167]` — ent's own `assets.format()` (which runs `goimports`) only processes files collected through the `Templates`/`GraphTemplates` pipeline. Files written by a hook via `os.WriteFile` bypass this entirely; the hook must call `golang.org/x/tools/imports.Process` (or `go/format.Source`) itself before writing, or CRUD-06's "gofmt-clean" requirement silently fails.
```go
// Source: entgo.io/contrib/entproto@v0.7.0 extension.go (skeleton mirrored per D-19),
// adapted for Go-file (not .proto-file) output.
// [VERIFIED: entproto/extension.go read this session — Hooks(), the
// next.Generate(g)-then-generate(g) ordering, and os.WriteFile-based emission
// into path.Join(g.Config.Target, "<subdir>") are all read directly, not
// paraphrased from documentation]
type Extension struct {
	entc.DefaultExtension
	descriptorSetPath string // D-04: entconnect.WithDescriptorSet(path)
}

func (e *Extension) Hooks() []gen.Hook {
	return []gen.Hook{e.hook()}
}

func (e *Extension) hook() gen.Hook {
	return func(next gen.Generator) gen.Generator {
		return gen.GenerateFunc(func(g *gen.Graph) error {
			if err := next.Generate(g); err != nil {
				return err
			}
			return e.generate(g) // reads gen.Type.Annotations, resolves procedures, emits handlers
		})
	}
}
```

### Pattern 6: Reading provenance annotations off `gen.Type`/`gen.Field` (Q3, D-17)

**What:** `gen.Type.Annotations` and `gen.Field.Annotations` are both `[VERIFIED: entgo.io/ent@v0.14.6 entc/gen/graph.go:141 — "Annotations map[string]any"]` — decoded JSON, not the original Go annotation structs.
**entproto's decode idiom (mapstructure):**
```go
// Source: entgo.io/contrib/entproto@v0.7.0 message.go:71-85, field.go:70-84 — read this session
annot, ok := sch.Annotations[MessageAnnotation]
if !ok {
	return nil, fmt.Errorf("entproto: schema %q does not have an entproto.Message annotation", sch.Name)
}
var out message
if err := mapstructure.Decode(annot, &out); err != nil { /* ... */ }
```
**This repo's own already-proven alternative (avoids adding `mapstructure` as a new dependency):**
```go
// Source: mixinforproto/internal/boundarytest/boundary_test.go:97-124 — read this session.
// This exact function is already tested against the real entc subprocess
// boundary in Phase 1; the root module (unlike mixinforproto) has no
// dependency-minimization constraint, so either idiom is viable — this one
// needs no new module edge.
func decodeAnnotation[T any](annotations gen.Annotations, key string) (T, error) {
	var zero T
	raw, ok := annotations[key]
	if !ok {
		return zero, fmt.Errorf("missing %q annotation", key)
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return zero, err
	}
	var v T
	err = json.Unmarshal(b, &v)
	return v, err
}
// Usage: sm, err := decodeAnnotation[mixinforproto.SourceMessage](typ.Annotations, mixinforproto.MixinForProtoMessage)
```
**Recommendation:** reuse the JSON round-trip idiom already proven in `mixinforproto/internal/boundarytest` rather than adding `mapstructure` — it is already tested against the real subprocess boundary in this exact repo, and it avoids introducing a dependency that only entproto's precedent (not this project's own constraints) suggests.

### Anti-Patterns to Avoid

- **Depending on `entgo.io/contrib/entgql` for `Paginate()`:** does not exist in core ent; taking the dependency contradicts entconnect.md §2.3's founding premise. (Q1, definitively verified.)
- **Relying on `fieldmaskpb.IsValid`/`New` alone to enforce D-15's top-level-only rule:** `fieldmaskpb` treats nested paths as valid by design (it walks into `fd.Message()` for compound paths) — entconnect must add its own explicit `strings.Contains(path, ".")` rejection *in addition to* descriptor validation. (Q6, verified via `numValidPaths`'s `rangeFields` recursion.)
- **Hand-building `connect.NewUnaryHandler[Req,Res]` calls per RPC in generated code instead of implementing the generated `<Service>Handler` interface:** works, but reimplements mux/path-routing logic `protoc-gen-connect-go` already generates correctly, and makes `Manual` RPC uniformity (INT-04) harder to guarantee — a hand-built per-RPC handler needs its own explicit interceptor wiring, whereas the interface-implementation route gets it for free from `New<Service>Handler(impl, opts...)`.
- **Using `Extension.Templates()`/`GraphTemplate` for handler-file emission:** the auto-derived output path (`snake(name)+".go"` directly under `g.Config.Target`) cannot place output in a sibling package; use a `gen.Hook` with direct `os.WriteFile` instead (mirrors entproto).
- **Trusting a hook-written Go file to come out gofmt-clean without calling `imports.Process`/`go/format.Source` explicitly:** `assets.format()` (ent's own goimports pass) never runs over hook-written files — confirmed by reading `graph.go`'s `generate()`, which only calls `assets.format()` over the `Templates`/`GraphTemplates`-collected `assets`, not over anything a hook writes independently.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| protovalidate boundary enforcement inside the Connect chain | A custom `connect.Interceptor` calling `protovalidate.Validate` manually, handling streaming variants, error-detail attachment | `connectrpc.com/validate` (`validate.NewInterceptor()`) | Already correct, already tested, official Connect-authors package, built on the exact `buf.build/go/protovalidate` module already pinned; ~180 lines of subtle streaming-aware logic for free |
| OpenTelemetry tracing/metrics for the "otel" chain stage | Hand-instrumented spans/counters wrapped as a custom interceptor | `connectrpc.com/otelconnect` (`otelconnect.NewInterceptor()`) | Same rationale — official, maintained, avoids re-deriving span-naming/attribute conventions from scratch |
| Procedure-string → descriptor resolution | A hand-parsed registry keyed by string, or an ad hoc `protoregistry.GlobalFiles` lookup requiring an accidental import edge | `protodesc.NewFiles` + `protoregistry.Files.FindDescriptorByName` against the committed `.binpb` | Exactly the mechanism `protodesc`/`protoregistry` exist for; hand-rolling it re-implements FileDescriptorSet dependency-graph walking (cycle detection, placeholder resolution) that the stdlib-adjacent package already gets right |
| Connect service routing/mux dispatch | A hand-written `http.HandlerFunc` switching on `r.URL.Path` per RPC | The generated `<Service>Handler` interface + `New<Service>Handler` from `protoc-gen-connect-go`'s own output | `protoc-gen-connect-go` already emits this correctly (verified by reading its real test fixture); reimplementing it duplicates buf-generated code for no benefit |
| FieldMask descriptor validation | Hand-walking `protoreflect.MessageDescriptor.Fields()` to check each path exists | `fieldmaskpb.New(msg, paths...)` / `.IsValid(msg)` for the runtime (live-message) half | Already correctly handles group-kind naming edge cases, repeated/map-field-must-be-last-segment rules, etc. — entconnect only needs to *add* the top-level-only restriction on top, not reimplement descriptor walking |

**Key insight:** every "don't hand-roll" candidate above is an *official, same-org* package (`connectrpc.com/*` or `google.golang.org/protobuf/*`) already implicitly endorsed by CLAUDE.md's pin of `connectrpc.com/connect` — extending trust to its sibling packages is a smaller leap than introducing a third-party library, and each one was read line-by-line in this session rather than assumed from a package name.

## Common Pitfalls

### Pitfall 1: Treating CRUD-03's literal wording as ground truth
**What goes wrong:** Planning a List handler around `entity.Query().Paginate(ctx, ...)`, discovering only at `go build` time (or later) that the generated ent package has no such method.
**Why it happens:** The requirement text and roadmap Success Criterion 2 both name `Paginate()` as if it were a core-ent primitive; it reads exactly like a real ent API name.
**How to avoid:** Treat D-10/this research's Q1 finding as settled: emit hand-built keyset `Where`/`Order`/`Limit` calls (Pattern 2 above). No dependency on entgql.
**Warning signs:** Any generated code or plan referencing `.Paginate(` against a plain `entgo.io/ent`-generated query builder; any go.mod diff adding `entgo.io/contrib/entgql`.

### Pitfall 2: `fieldmaskpb.IsValid` silently accepting nested paths
**What goes wrong:** A request with `update_mask.paths = ["customer.name"]` passes `mask.IsValid(msg)` (it's a legitimate nested path against the descriptor) and the generated code either panics on an unhandled `switch` case or — worse — silently no-ops that path.
**Why it happens:** `fieldmaskpb.numValidPaths` explicitly recurses into `fd.Message()` for compound paths — nested-path support is a designed *feature* of the FieldMask type, and D-15's top-level-only restriction is an entconnect-specific, stricter-than-`fieldmaskpb` policy layered on top.
**How to avoid:** Explicit `strings.Contains(path, ".")` (and `path == "*"`) rejection *before* or alongside the `IsValid` call, both at request time (Pattern 3) and at build time (D-17, cross-checked against `SourceField.FieldName`, which is itself flat/non-dotted per Phase 1's `annotation.go`).
**Warning signs:** Golden tests that only exercise flat paths; no corpus proto message with a nested/message-typed field feeding an Update RPC.

### Pitfall 3: Assuming `Extension.Templates()` can target an arbitrary output subdirectory
**What goes wrong:** An extension registers a custom `*gen.Template` expecting its output to land in `internal/entconnect/server.go`; instead it lands as a flat file directly under `g.Config.Target` (e.g., `ent/server.go`, colliding with or cluttering the ent package).
**Why it happens:** The auto-derived `GraphTemplate.Format` for an extension-registered template not overriding a builtin is always `snake(templateName) + ".go"`, joined directly onto `g.Config.Target` — there's no public knob for a subdirectory via this path.
**How to avoid:** Use a `gen.Hook` (Pattern 5) with direct `os.WriteFile` for anything that must land outside `ent/`, exactly as entproto does for its `proto/` output.
**Warning signs:** Generated handler files appearing inside the app's `ent/` package directory instead of a dedicated sibling package; CRUD-07's "generated wiring in an internal package" becoming hard to enforce because the wiring physically lives inside the public `ent/` package.

### Pitfall 4: Forgetting to format hook-written Go source
**What goes wrong:** Files written via a `gen.Hook`'s direct `os.WriteFile` come out with unformatted imports/spacing, failing CRUD-06's "gofmt-clean" requirement, or failing to compile due to missing import lines that `goimports` would have inserted.
**Why it happens:** `assets.format()` (ent's own `goimports` pass) only runs over the `Templates`/`GraphTemplates`-collected asset list built inside `generate()` — it has no visibility into files a hook writes independently after `next.Generate(g)` returns.
**How to avoid:** Call `golang.org/x/tools/imports.Process` (matching what ent's own `assets.format()` does internally) or at minimum `go/format.Source` before every `os.WriteFile` in the extension's own hook.
**Warning signs:** Golden-file tests comparing raw template output without ever running the file through `gofmt -l`; CI passing `go build` (because Go doesn't care about import ordering) while a manual `gofmt -l` check would fail.

### Pitfall 5: Believing `privacy.Deny` denials arrive as a distinct error *type*
**What goes wrong:** D-18's error mapper is written as a type switch (`switch err.(type) { case *privacy.DenyError: ... }`), which never matches, because `privacy.Deny` is a **sentinel `error` value** (`errors.New("ent/privacy: deny rule")`), not a named type.
**Why it happens:** Reasonable assumption by analogy with `NotFoundError`/`ConstraintError` (which genuinely are ent-generated concrete types) — but privacy decisions use the sentinel-error idiom instead.
**How to avoid:** Match with `errors.Is(err, privacy.Deny)`, confirmed as the correct check by reading `entgo.io/ent@v0.14.6/privacy/privacy.go` directly (`Deny = errors.New(...)`) and confirming the generated `prepareQuery`/mutation hook returns the policy's error **unwrapped** (`entc/gen/template/builder/query.tmpl:419-421`, `return err` with no additional `fmt.Errorf` wrapping).
**Warning signs:** A test asserting a privacy denial that only checks `err != nil` rather than the specific mapped Connect code; `PermissionDenied` never actually appearing on the wire despite a `privacy.Deny` rule firing.

### Pitfall 6: Assuming ent ships a canonical "viewer" type
**What goes wrong:** Looking inside `entgo.io/ent` for a `viewer.Viewer`/`viewer.FromContext` package to import; it doesn't exist in ent core.
**Why it happens:** ent's own privacy tutorial (`doc/md/privacy.mdx`) uses a `viewer` package extensively, but it is explicitly an **example app package** (`entgo.io/ent/examples/privacyadmin/viewer`), not a shipped library type.
**How to avoid:** entconnect's own `runtime/viewer` package (Pattern in Recommended Project Structure) must define `NewContext`/`FromContext` itself, matching the example's shape closely enough that privacy rules referencing it read naturally, but as first-party entconnect code, not an ent-core import.
**Warning signs:** A `go get entgo.io/ent/examples/...` in any go.mod (examples packages aren't meant to be imported cross-module); confusion in PLAN.md about which package "owns" the viewer type.

## Code Examples

### Descriptor-set loading + procedure resolution (D-03) — see Pattern 1 above, fully verified live this session.

### Keyset list paging (CRUD-03) — see Pattern 2 above.

### FieldMask-gated Update (CRUD-04/05) — see Pattern 3 above.

### Interceptor chain construction (INT-01) — see Pattern 4 above.

### Error mapping table (INT-03, D-18)
```go
// Source: connectrpc.com/connect@v1.20.0 code.go (Code constants),
// entgo.io/ent@v0.14.6 privacy/privacy.go (Deny sentinel) — both read this session
func mapError(err error) error {
	switch {
	case errors.Is(err, privacy.Deny):
		return connect.NewError(connect.CodePermissionDenied, err)
	case ent.IsNotFound(err):
		return connect.NewError(connect.CodeNotFound, err)
	case ent.IsConstraintError(err):
		// D-18 leaves AlreadyExists vs FailedPrecondition as a per-constraint
		// judgment call for the planner/implementer — ent's ConstraintError
		// does not itself distinguish "unique violation" from "FK violation"
		// via a typed field; inspecting the underlying driver error may be
		// necessary. Flagged as an open question below.
		return connect.NewError(connect.CodeAlreadyExists, err)
	case ent.IsValidationError(err):
		return connect.NewError(connect.CodeInvalidArgument, err)
	default:
		log.Printf("entconnect: unmapped error: %v", err) // detail logged, never returned
		return connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|---------------|--------|
| protoc + protoc-gen-go + protoc-gen-connect-go invoked directly | `buf generate` orchestrating the same plugins via `buf.gen.yaml` | Already the case in this repo since Phase 1 | No change needed — Phase 2 only adds a `protoc-gen-connect-go` plugin entry to the existing `buf.gen.yaml` |
| Offset-based `LIMIT`/`OFFSET` pagination | Keyset (cursor) pagination for anything under concurrent writes | Long-established SQL best practice, not something recent | Directly why CRUD-03/D-10 forbid offset paging; Pitfall 1 above is the concrete trap |
| protoc-gen-validate (PGV) | protovalidate / `buf.validate` | Already resolved in CLAUDE.md/Phase 1 | Not re-litigated here — `connectrpc.com/validate` builds on the current protovalidate stack, not PGV |

**Deprecated/outdated:** none newly discovered this phase beyond what CLAUDE.md already documents (PGV, `github.com/bufbuild/protovalidate-go`, `github.com/google/cel-go` as a *new* import).

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | `page_token` opaqueness (D-08) does not currently specify cryptographic integrity (HMAC/signing) of the encoded cursor — only a fingerprint of ordering/filter-relevant *request* fields, not tamper-resistance of the cursor's ID/timestamp payload itself | Pattern 2 / Security Domain | A client could hand-construct a cursor pointing at an arbitrary `(created_at, id)` pair. Because the resulting `Where` predicate is still evaluated inside the same ent query that ent privacy `Filter` rules apply to, a forged cursor cannot bypass authorization — but it could be used to probe for the *existence* of specific IDs across pages (a minor information-disclosure vector) unless the planner adds cursor signing. AIP-158 itself only requires opacity, not signing, so this is a project-level hardening choice, not a spec violation either way. |
| A2 | `connectrpc.com/validate` and `connectrpc.com/otelconnect` are recommended additions to the stack, verified as legitimate official packages this session, but were not previously named in CLAUDE.md's pinned stack or in CONTEXT.md's Claude's Discretion list | Standard Stack, Don't Hand-Roll | Low risk technically (both packages are genuine, well-evidenced, official) but the *decision to adopt* them is new — the planner should surface this explicitly rather than silently absorbing a new dependency the user hasn't seen named |
| A3 | The recommendation to implement `orderv1connect.<Service>Handler` (the `protoc-gen-connect-go`-generated interface) rather than hand-building `connect.NewUnaryHandler` calls is presented as the primary recommendation, not a CONTEXT.md-locked decision | Q4, Pattern 5, Anti-Patterns | If a future constraint emerges requiring the generated handler code to avoid depending on `protoc-gen-connect-go`'s generated interface shape (e.g., to support a transport this interface doesn't model), this recommendation would need revisiting. Low risk given D-01's own example already assumes `orderv1connect` symbols are referenced. |
| A4 | `ConstraintError` → `AlreadyExists` vs `FailedPrecondition` (D-18) cannot be distinguished by ent's typed error alone (`ent.IsConstraintError` doesn't discriminate unique-violation from FK-violation) — this needs either driver-specific error inspection or is left as a single mapping | Code Examples §Error mapping | If the planner assumes a clean typed distinction exists in ent's error API and builds a task around it, that task will discover the gap during implementation, not planning. Flagged now so the plan can scope it explicitly (e.g., "map all ConstraintError to AlreadyExists in v1, revisit FailedPrecondition later" is a legitimate narrower resolution). |

**If this table is empty:** N/A — see above.

## Open Questions

1. **Should `page_token` be cryptographically signed (HMAC) rather than merely fingerprinted?**
   - What we know: D-08 requires a fingerprint of ordering/filter fields to reject stale/mismatched tokens; AIP-158 itself only requires opacity, not integrity.
   - What's unclear: whether this project's threat model treats cursor forgery as in-scope for Phase 2 or is deferred (privacy `Filter` rules still apply regardless, limiting blast radius to ID-existence probing, not data exfiltration).
   - Recommendation: leave unsigned for v1 (matches AIP-158's actual minimum), but record explicitly in the plan as a conscious choice, not an oversight — Security Domain below flags this same tradeoff.

2. **Exact shape of the `AuthenticateAndViewer(ctx, http.Header) (context.Context, error)` interface's generated name/package** (D-11 names the method signature but the exact generated interface/type name is a codegen-naming decision for the planner, not yet fixed by CONTEXT.md).
   - What we know: single method, required positional constructor argument.
   - What's unclear: whether it's a per-service or per-app interface (one authenticator for the whole generated server, or one per generated `<Service>Handler`).
   - Recommendation: per-app (one `NewServer(client, authenticator, opts...)` for the whole generated wiring, matching D-11's own `NewServer` example) — services share one authn/viewer story in nearly all real deployments; the planner should confirm this reading against D-11's literal wording before finalizing task breakdown.

3. **Where exactly does `WithDescriptorSet(path)` (D-04) get consumed — extension construction time (`entc.Generate(...)`) or a separate CLI/config step?**
   - What we know: it's an "extension option," implying `entconnect.NewExtension(entconnect.WithDescriptorSet(path))`, mirroring `entproto.WithProtoDir`'s own option shape (verified pattern: `ExtensionOption func(*Extension)`).
   - What's unclear: nothing structurally — this is confirmed by direct analogy to entproto's verified `ExtensionOption` pattern (`entproto/extension.go:32-33,63-68`). Listed here only to make explicit that the analogy, not a separate verification, is the basis for this being "resolved."
   - Recommendation: no further research needed; treat as settled by the entproto precedent already read.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go toolchain | All of Phase 2 | ✓ (auto-downloaded via `go.work`'s `go 1.26` directive) | go1.26.0 | — |
| `buf` CLI | `buf generate`/`buf build -o` steps (pipeline.sh steps 1-3) | ✗ in this research sandbox | — | `go install github.com/bufbuild/buf/cmd/buf@v1.72.0` (same install hint `scripts/pipeline.sh`/`scripts/generate-stubs.sh` already document); confirmed present in Phase 1's own CI verification session |
| `protoc-gen-go` | Existing `buf.gen.yaml` plugin | ✗ in this research sandbox | — | `go install google.golang.org/protobuf/cmd/protoc-gen-go@latest`; already a documented convention in this repo |
| `protoc-gen-connect-go` | **New** `buf.gen.yaml` plugin entry this phase must add | ✗ in this research sandbox | — | `go install connectrpc.com/connect/cmd/protoc-gen-connect-go@v1.20.0` (version-matched to the pinned `connectrpc.com/connect`) |
| `atlas` CLI | `pipeline.sh` step 5 | ✗ in this research sandbox | — | Not exercised by Phase 2 (no real application ent schema exists yet per `pipeline.sh`'s own skip logic); no action needed this phase |
| Go module proxy (`proxy.golang.org`) | Verifying all package claims above | ✓ (explicitly allowlisted in this sandbox's `no_proxy`) | — | — |

**Missing dependencies with no fallback:** none — every missing tool has a documented, already-precedented install command.
**Missing dependencies with fallback:** `buf`, `protoc-gen-go`, `protoc-gen-connect-go`, `atlas` — all standard `go install`-able tools; Phase 1's own CI verification session confirmed `buf`/`protoc-gen-go` are genuinely available in this project's real CI environment (only absent from this particular research sandbox).

## Security Domain

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-------------------|
| V2 Authentication | yes | App-supplied `AuthenticateAndViewer` func, required as a compile-time constructor argument (D-11) — no default/bypassable auth path exists in generated code |
| V3 Session Management | no | Connect RPCs are stateless per-request; no session/cookie mechanism is introduced by this phase |
| V4 Access Control | yes | ent `privacy.Policy` (`EvalQuery`/`EvalMutation`), driven by the viewer placed on `ctx` by the interceptor chain's 2nd stage; denials propagate as the `privacy.Deny` sentinel, mapped to `CodePermissionDenied` (D-18/INT-03) |
| V5 Input Validation | yes | `connectrpc.com/validate` interceptor (boundary-tier, built once per process per D-13) backed by `buf.build/go/protovalidate`; FieldMask path validation (D-15/D-17) is a second, independent input-validation layer specific to Update |
| V6 Cryptography | no | This phase introduces no cryptographic primitives; page-token integrity (Open Question 1 / Assumption A1) is the one place cryptography *could* apply but is not currently in scope per D-08's literal wording |

### Known Threat Patterns for this stack

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|----------------------|
| Forgotten/misconfigured authn wiring leaving an RPC open | Spoofing / Elevation of Privilege | D-11's required-constructor-argument design — a missing authenticator is a compile error, not a runtime default (verified design choice, not a library feature) |
| Privileged `*ent.Client` leaking to application code | Elevation of Privilege | CRUD-07/D-12 — client constructed only inside generated wiring in an internal package; structurally, not conventionally, enforced |
| Proto3 zero-collapse mass-assignment on Update (unset fields silently zeroed) | Tampering | FieldMask-gated `Set*` calls (D-14..D-17) — Pattern 3 above; this is the single sharpest edge Phase 1's own README section (mixinforproto/README.md) was written specifically to warn about |
| Internal error detail (DB error strings, stack context) leaking onto the wire | Information Disclosure | D-18's unknown→`Internal` default, detail logged server-side only, never returned in the Connect error — verified as the literal behavior of the `mapError` pattern above |
| Page-token forgery used to probe ID existence across an unauthorized range | Information Disclosure (limited) | Ent's privacy `Filter` rules still apply to every query regardless of pagination WHERE clause — a forged cursor cannot bypass authorization, only potentially reveal narrow existence information; see Open Question 1 for whether HMAC-signing the cursor is warranted |
| Duplicate/conflicting RPC claims (two schemas binding the same procedure) silently picking one "winner" | Tampering (of the build's own correctness, not runtime data) | D-05 — collected build-time error, not a runtime ambiguity |

## Sources

### Primary (HIGH confidence — read or executed directly this session)
- `entgo.io/ent@v0.14.6` — `entc/gen/graph.go`, `entc/gen/template.go`, `entc/gen/type.go`, `entc/gen/func.go`, `entc/gen/predicate.go`, `entc/gen/template/where.tmpl`, `entc/gen/template/builder/query.tmpl`, `entc/gen/template/dialect/sql/predicate.tmpl`, `entc/gen/template/dialect/sql/by.tmpl`, `entc/gen/template/runtime.tmpl`, `entc/entc.go`, `privacy/privacy.go` — all read directly from the downloaded module in `$(go env GOMODCACHE)`
- `entgo.io/contrib@v0.7.0` (entproto) — `entproto/extension.go`, `entproto/message.go`, `entproto/field.go` — read directly
- `entgo.io/contrib@v0.7.0` (entgql, internal example output) — `entgql/internal/todouuid/ent/gql_pagination.go` — read directly to confirm `Paginate()`'s actual origin
- `connectrpc.com/connect@v1.20.0` — `handler.go`, `option.go`, `code.go`, `error.go`, `go.mod` — read directly
- `connectrpc.com/connect@v1.20.0/cmd/protoc-gen-connect-go/internal/testdata/simple/gen/genconnect/simple.connect.go` — real generated fixture output, read directly
- `connectrpc.com/validate@v0.6.0` — `validate.go`, `go.mod` — read directly
- `connectrpc.com/otelconnect@v0.9.0` — `interceptor.go`, `go.mod` — read directly
- `google.golang.org/protobuf@v1.36.11` — `reflect/protodesc/desc.go`, `types/known/fieldmaskpb/field_mask.pb.go`, `reflect/protoregistry/registry.go` — read directly, and exercised via live Go programs executed in this session against both a synthetic descriptor set and Phase 1's real committed `proto/mixinforprototest.binpb`
- `mixinforproto/annotation.go`, `mixinforproto/internal/boundarytest/boundary_test.go` — this repo's own Phase 1 code, read directly

### Secondary (MEDIUM confidence)
- AIP-158 pagination conventions — WebSearch summary cross-referencing `google.aip.dev/158` and the `aip-dev/google.aip.dev` GitHub mirror (direct fetch of `google.aip.dev` was blocked by this sandbox's egress proxy; relied on search-result excerpts rather than the primary page itself)

### Tertiary (LOW confidence)
- None — every claim in this document that could be checked against a primary source in this sandbox was checked; the one exception (AIP-158's canonical text) is marked MEDIUM, not LOW, because multiple independent search results corroborated the same summary.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — every version/module claim verified via `go list -m -json` against the live Go module proxy and direct source inspection, not training-data recall
- Architecture: HIGH — the entc/gen template mechanism (Q3) and the descriptor-resolution mechanism (Q2) were both exercised with real, executed Go code in this session, not just read
- Pitfalls: HIGH — each pitfall traces to a specific line range read this session (e.g., `fieldmaskpb`'s nested-path support, `assets.format()`'s scope, `privacy.Deny`'s sentinel-not-type nature)
- CRUD-03/Q1 (keyset paging vs `Paginate()`): HIGH — this is the one claim in the phase brief flagged as possibly wrong, and it is now definitively resolved by both a negative-space grep across all of core ent's template source and a positive confirmation that `Paginate()` lives only in entgql's generated output

**Research date:** 2026-08-08
**Valid until:** 30 days (stable dependency set; `connectrpc.com/validate`/`otelconnect` version pins should be re-checked if planning is deferred past early September 2026, since both packages release on an active cadence)
