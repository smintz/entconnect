# Phase 2: CRUD Handlers & Interceptor Chain - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-08-08
**Phase:** 2-CRUD Handlers & Interceptor Chain
**Areas selected by user:** CRUD binding & RPC recognition, List paging/ordering/filtering, App integration surface, FieldMask semantics & error mapping
**Areas actually discussed:** CRUD binding & RPC recognition (fully). The session ended before the other three were answered; they were recorded at their recommended defaults and marked as Claude's discretion in CONTEXT.md.

---

## CRUD binding & RPC recognition

### Round 1 — how does the extension recognize a CRUD RPC?

| Option | Description | Selected |
|--------|-------------|----------|
| Resource-type match + verb prefix | Find the schema whose `SourceMessage` names the RPC's resource message, read the operation from the RPC name prefix | |
| Explicit schema-side annotation | Schema declares its binding via `Annotations()` | (superseded — see round 2) |
| RPC name convention only | Lexical `GetOrder`/`ListOrders` matched to the schema's Go type name | |
| **Other (free text)** | "Follow MixinForMessage convention, the service definition is defined in the proto definition, an annotation is set to define how to implement a specific rpc method for a service" | ✓ |

**User's choice:** free text — a **custom proto method option** declaring how an RPC is implemented.

**Notes:** Claude reflected the reading back and flagged a tension: PROJECT.md's Out of Scope lists *"Custom proto field options as an annotation channel — migrates schema semantics into the proto."* Claude's read was that a **method** option carrying **transport binding** semantics is a different category from a **field** option carrying **storage** semantics, so this would be the "until real demand appears" clause firing rather than a contradiction — but that PROJECT.md's wording would need narrowing either way. Claude then asked three plain-text follow-ups: what the option carries, where it lives, and what happens to unannotated RPCs.

### Round 2 — user revises: annotations belong in `ent/schema`

**User's choice (final):**

> "The annotations should be in `ent/schema`, not inside the proto.
>
> ```go
> entconnect.CreateRPC(orderv1connect.OrderServiceCreateOrderProcedure),
> entconnect.UpdateRPC(orderv1connect.OrderServiceUpdateOrderProcedure),
> entconnect.DeleteRPC(orderv1connect.OrderServiceDeleteOrderProcedure),
> ```
>
> Which should resolve to the service descriptor. Which is challenging because it might be in a completely different package."

**Notes:** This supersedes round 1 entirely. Claude noted the fit: connect-go's generated `…Procedure` constant is a compile-time-checked reference to the contract, the same property `MixinForProto[*orderv1.Order]`'s type parameter provides at field level. It also means **no proto options are added in this phase** and PROJECT.md's Out of Scope entry stands unamended. Recorded as CONTEXT.md **D-01**.

The user's own caveat — the service descriptor may live in a different package — was analyzed further: the procedure constant is a bare `string`, so package identity is lost at the annotation boundary, and the entc **host** process never links the generated proto package at all (only the schema-load subprocess does). Recorded as **D-02**.

### Round 3 — descriptor resolution (unanswered)

| Option | Description | Selected |
|--------|-------------|----------|
| Committed descriptor set only | Split procedure into service + method, resolve against the `FileDescriptorSet` the extension already loads; hermetic, package-independent, and the only option that can support Phase 5's whole-service drift check | (Claude's default) |
| Global `protoregistry` at schema load | Resolve in the subprocess where the generated package is linked, write resolved facts into the annotation | |
| Descriptor set, cross-checked at schema load | Both, with disagreement as a staleness signal | |

**User's choice:** not answered — session interrupted.
**Resolution:** recorded as **D-03** at the first option, with the Phase 5 drift-check argument as the decisive reason. Flagged as Claude's discretion.

---

## List paging, ordering & filtering

Question presented (cursor+fingerprint paging-only / cursor-only / paging plus contract-driven filtering). **Not answered** — session interrupted.

**Resolution:** recorded as **D-08/D-09** at cursor+fingerprint, paging only, filtering deferred. Separately, Claude flagged **D-10** as a research question rather than a decision: CRUD-03's phrase "ent's native keyset `Paginate()`" appears to describe an **entgql**-generated method, not core ent v0.14.6 — this needs verifying before List can be planned, and entgql is not an acceptable dependency (§2.3).

---

## App integration surface (authn, viewer, client)

Question presented (generated interface / functional options / raw `connect.Interceptor` slot). **Not answered** — session interrupted.

**Resolution:** recorded as **D-11/D-12/D-13** at the generated-interface option, on the argument that an omitted functional option degrades to a runtime default — deny-all or, worse, allow-all — whereas a required constructor argument makes "forgot to wire authn" a compile error.

---

## FieldMask semantics & error mapping

Two questions presented (mask path support + empty-mask meaning; ent→Connect error code mapping). **Not answered** — session interrupted.

**Resolution:** recorded as **D-14…D-18**: explicit `FieldMask` (resolving §9 Open Question 5), top-level paths only, empty mask is `InvalidArgument`, build-time path validation against both the descriptor and `SourceField` provenance, and an explicit error table with unknown→`Internal`.

---

## Claude's Discretion

Everything except D-01 and D-02. Ranked for review in CONTEXT.md:

1. **D-10** — keyset `Paginate()` availability; the only item resting on an unverified factual claim in the requirement text itself
2. **D-11** — required interface vs. functional options (safety vs. ergonomics; one-way for adopters)
3. **D-07** — report-don't-fail on unclaimed RPCs (Phase 2/Phase 5 boundary call)
4. **D-16** — empty mask as an error
5. **D-18** — unknown errors mapped to `Internal` rather than `Unknown`

## Deferred Ideas

- Contract-driven filtering on List (needs its own convention design)
- Nested field-mask paths into `AsJSON()` fields
- Connect server streaming (§9 OQ3 option b, deferred upstream)
- Hard build failure on unclaimed RPCs → DRIFT-01, Phase 5
- A both-paths descriptor cross-check as a redundant staleness signal
- Narrowing PROJECT.md's "custom proto field options" Out of Scope wording — raised in round 1, made moot by round 2
