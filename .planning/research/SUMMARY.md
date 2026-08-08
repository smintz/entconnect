# Project Research Summary

**Project:** entconnect
**Domain:** Go code-generation library — contract-first ent + ConnectRPC toolkit (protobuf-descriptor-driven ORM binding + transport codegen)
**Researched:** 2026-08-08
**Confidence:** MEDIUM-HIGH

## Executive Summary

entconnect inverts entproto's direction: instead of generating proto from an ent schema, it derives ent fields and validation from a protobuf contract (via a runtime mixin, `mixinforproto`) and generates the ConnectRPC transport layer from the same contract. This is a legitimate, differentiated design — no comparable tool (entproto, entgql, entoas, entrest) does bidirectional contract↔schema↔service drift checking, and the stack choices (ent v0.14.6, protobuf-go, connect-go, buf CLI, `buf.build/go/protovalidate` + `cel-expr/cel-go`) are all current, verified-live, non-deprecated dependencies. The core technical bet — that protovalidate's CEL constraints can be compiled once at ent schema-load time and executed identically to the boundary-layer validator — is well-founded and de-risked by the discovery that `protovalidate/cel.NewLibrary()` is public and reusable.

The single biggest engineering risk is not the mixin itself but the **schema-load → codegen boundary**: `entc generate` serializes schemas to JSON before the entc extension ever runs, and Go closures (validators, `Exclude`/`Override` decisions, hook logic) cannot cross that boundary — only `map[string]any` annotations can. As specified in the design docs (mixin exposes `Exclude`/`Override` as pure in-memory closures), the drift check the project is *named for* would silently degrade to "does a field exist," unable to distinguish deliberate omission from accidental drift. This means `mixinforproto` must emit `ent.Annotations` (`SourceMessage`, `SourceField`) recording exclude/override/provenance data as a **v0.1 deliverable**, not a v0.2+ retrofit — every field-derivation code path would otherwise need revisiting later.

The second major risk is the proto3-presence/zero-collapse problem colliding with partial updates: naive Update handlers that `Set*()` every present field will silently zero out untouched fields the caller never meant to touch. This is not a v0.1 field-mapping concern — it only becomes real at Update-RPC codegen time, so **field-mask-driven partial Update must ship in the same phase as Update RPC codegen** (resolving Open Question 5), with mask-path validation done at generate time against the message descriptor. Secondary risks — CI enumerating both Go modules (`go.work` + `GOWORK=off` CI path), cel-go's mid-migration import path (`cel-expr/cel-go`, not `google/cel-go`), `buf build -o --as-file-descriptor-set` being a separate CLI step from `buf generate`, entflow being an external moving-target dependency (sequence against its running code with a fake `entflow/meta` in tests) — are all well-understood with concrete, low-cost mitigations.

## Key Findings

### Recommended Stack

Core: `entgo.io/ent` v0.14.6, `google.golang.org/protobuf` v1.36.11, `connectrpc.com/connect` v1.20.0 (Go ≥1.25, pin `go 1.26`), `buf` CLI v1.72.0. Validation toolchain: `buf.build/go/protovalidate` v1.2.0 (module path **changed** from the legacy `github.com/bufbuild/protovalidate-go`) plus `buf.build/go/protovalidate/cel` for building a CEL env byte-compatible with protovalidate's own evaluator — this is the load-bearing finding that de-risks Tier 2. `github.com/cel-expr/cel-go` (formerly `github.com/google/cel-go`, mid-migration — **import the new path in new code**). Infra: `grpc-gateway/v2` (defer to v0.4, don't add early), `ariga.io/atlas` CLI for versioned migrations. Codegen emission should mirror ent's own `text/template`/`entc/gen` mechanism (not jennifer/go-ast), following entproto's `Extension`/`Hooks()` skeleton directly.

### Expected Features

**Must have (table stakes, v0.1–v0.2):** MixinForProto core (scalars/enums/WKT/Tier 1 translation), field/line-precise codegen errors, golden-file test harness from day one, CRUD handler generation, AIP-158-shaped keyset pagination (wrap ent's native `Paginate()`), FieldMask/`update_mask` partial-update convention (AIP-134) including descriptor-time mask-path validation, fixed interceptor chain (authn → viewer → protovalidate → otel → handler), `entconnect.Manual("rpc")` escape hatch visible in drift-check output.

**Should have (differentiators, v0.2.x–v0.3):** Tier 2 CEL passthrough + differential validation harness, flow binding + `GetRunStatus`, full bidirectional drift check (the category-defining feature), entproto migration doc.

**Defer (v1.x+):** grpc-gateway `google.api.http` passthrough, REST emitter, Connect server-streaming run-state, `StrictPresence` option, `debug_redact`-based `field.Sensitive()` inference.

### Architecture Approach

Five-stage pipeline: (1) proto contract via `buf lint && buf generate` plus a **separate** `buf build -o --as-file-descriptor-set` invocation, (2) schema load inside `entc`'s subprocess where `MixinForProto` walks the descriptor via protoreflect and writes `SourceMessage`/`SourceField` annotations, (3) codegen where the entc extension's `Hooks()` runs post-pass over `gen.Graph`, merging the committed `FileDescriptorSet` with graph annotations through a single descriptor-reader (`entc/analyze`) feeding N emitters (`entc/emit/connect`, `/gateway`, `/rest`), (4) runtime where `ent/runtime.go` re-imports schema code a second time to recover dropped closures, and generated server wiring is the only place an `*ent.Client` is constructed, (5) migration via `atlas migrate diff`.

**Major components:**
1. `mixinforproto` (independent `go.mod`, leaf module) — message → `[]ent.Field` + validation relay + annotation-writing
2. `entc/` extension — single `Hooks()` entry, `analyze/` (one descriptor-reader) strictly separated from `emit/*` (N pure emitters)
3. `codec/proto` — thin (~10 line) entflow.Codec adapter
4. `runtime/` — fixed-order interceptor chain, viewer-scoped context only, never a privileged client

### Critical Pitfalls

1. **Lossy schema-load/codegen boundary** — closures cannot cross the JSON serialization boundary; `mixinforproto` must write `SourceMessage`/`SourceField` annotations from v0.1 or the drift check cannot distinguish "deliberately excluded" from "silently forgotten."
2. **proto3 zero-collapse breaks partial updates** — field-mask enforcement (AIP-134, mandatory) must ship in the same phase as Update RPC codegen, gating every `Set*` on `path in mask.GetPaths()`.
3. **`entc`'s subprocess panics arrive with no usable stack trace** — requires context-rich custom panics plus an in-process debug entry point, from v0.1.
4. **Mixin `Hooks()` run before schema-declared `Hooks()`/`Policy()`** — must be documented and conformance-tested when the CEL hook ships (v0.2).
5. **Two-module repo operational friction** — CI must explicitly enumerate both modules; `mixinforproto/vX.Y.Z` tag prefix from v0.1.
6. **cel-go / protovalidate version skew** across independent go.mods — CI check needed once both boundary interceptor and schema hook exist (v0.2).

## Implications for Roadmap

### Phase 1: MixinForProto core + annotation contract
**Rationale:** Nothing downstream compiles without materialized ent fields; the annotation-crossing mechanism must be built alongside field-derivation logic, not retrofitted.
**Delivers:** `MixinForProto[M proto.Message]`, `Exclude`/`Override` (panicking at schema load), Tier 1 translation, `SourceMessage`/`SourceField` annotations, context-rich panics + debug entry point, independent `go.mod` with `go.work` + `GOWORK=off` CI job.
**Avoids:** Pitfall 1, the lossy-boundary drift-check failure mode, Pitfall 12 (nested-module CI gaps).

### Phase 2: CRUD handler generation + AIP-158 pagination + FieldMask Update (one phase, not split)
**Rationale:** List needs a paging convention; Update needs a partial-update convention; the FieldMask/zero-collapse pitfall only bites at Update-handler codegen time.
**Delivers:** Get/List/Create/Update/Delete Connect handlers; AIP-158 `page_token` envelope over ent's keyset `Paginate()`; mandatory `update_mask` with descriptor-time mask-path validation and mask-gated `Set*()`; fixed interceptor chain with a process-wide singleton validator; `Manual("rpc")` inside the generated interceptor chain from day one.
**Avoids:** Pitfall 2 (design's sharpest edge), Pitfall 6 (per-request CEL recompilation), Pitfall 11 (Manual() bypass).

### Phase 3: Tier 2 CEL passthrough + differential validation harness + CI version-skew check
**Rationale:** Sequenced after Tier 1 proves the mixin shape; requires `protovalidate/cel`'s public `NewLibrary()`/`RequiredEnvOptions`.
**Delivers:** Single mixin Hooks() entry compiling CEL once, mask-gated changed-field evaluation; structured schema-layer error type usable outside RPC context; CI cross-module protovalidate/cel-go version check.
**Avoids:** Pitfall 5, 7, 8.

### Phase 4: Flow binding (entflow coupling) + async run status
**Rationale:** Sequence to start only once entflow has shipped real, minimal, running sync-flow code — not just a design doc.
**Delivers:** RPC-input-to-flow-input type matching, `flow.Start` dispatch, `run_id` + `GetRunStatus` RPC; a fake/duck-typed `entflow/meta` in entconnect's own test fixtures.
**Avoids:** Pitfall 13 (external-project design churn).

### Phase 5: Bidirectional drift check (full)
**Rationale:** Pure function of Phases 1–4 outputs; a CRUD-only partial drift check should ship incrementally at Phase 2.
**Delivers:** Message-typed-field↔edge matching via `SourceMessage`, constraint fingerprint comparison, flow-unclaimed warnings, `.binpb`/Go-stub staleness CI gate, deterministic (sorted) drift-check output.
**Avoids:** Pitfall 9 (stale descriptor set), Pitfall 10 (map-iteration nondeterminism).

### Research Flags
- **Needs research:** Phase 1 (annotation struct versioning strategy — no direct precedent beyond entproto's simpler annotations), Phase 3 (CEL-env byte-compatibility across independent modules — novel integration), Phase 4 (gated on external, not-yet-built entflow).
- **Standard patterns (skip research-phase):** Phase 2 (AIP-158/AIP-134 normatively specified, entgql precedent for keyset paging, entproto's Extension/Hooks skeleton), Phase 5 (extends the Phase 1 annotation contract + entgql/entproto/sqlc-proven staleness-gate pattern).

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | MEDIUM | Core versions verified directly against pkg.go.dev/GitHub; a few ecosystem items WebSearch-only, flagged LOW inline |
| Features | MEDIUM | Official docs cross-checked directly for entproto/entgql/entoas/entrest/AIPs; GitHub issue aggregation and external analogies LOW-confidence |
| Architecture | MEDIUM-HIGH | Schema-load/codegen boundary verdict grounded in direct reads of ent core source, not inference |
| Pitfalls | MEDIUM | No direct prior art for this exact design; most pitfalls inferred by analogy, though ent-internals-grounded ones (1,2,3,12) are higher confidence |

**Overall confidence:** MEDIUM-HIGH

### Gaps to Address
- Annotation struct versioning/forward-compatibility strategy for `SourceMessage`/`SourceField` — needs concrete Phase 1 design.
- entflow's actual metadata interface does not exist yet — Phase 4 planning should not proceed past interface sketching until real code exists.
- `google.golang.org/genproto/googleapis/api` import path stability — LOW confidence, verify when first needed (gateway phase).
- `Manual()` usage-ratio tooling has no owner in the phase breakdown — fold into Phase 2 or track explicitly for v0.4+.
- Sensitive-field marking (`field.Sensitive()` inference) — unresolved open question with security implications; needs a decision by Phase 1 or 2.

## Sources

Full citations live in the per-dimension research files:

- `.planning/research/STACK.md` — pkg.go.dev / GitHub source reads for ent, protobuf-go, connect-go, protovalidate, cel-go; buf.build docs
- `.planning/research/FEATURES.md` — entgo.io docs, entproto/entgql/entoas/entrest sources, google.aip.dev (AIP-134, AIP-158), protovalidate.com
- `.planning/research/ARCHITECTURE.md` — direct reads of `ent/entc/load/schema.go`, `ent/entc/gen/type.go`, `ent/contrib/entproto/*`, `entgql/extension.go`; buf.build and connect-go docs
- `.planning/research/PITFALLS.md` — ent GitHub issues (#474, #280), protobuf.dev presence docs, grpc-gateway field-mask docs, protovalidate/cel-go performance docs, atlasgo.io

---
*Research completed: 2026-08-08*
*Ready for roadmap: yes*
