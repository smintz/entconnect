# MixinForProto — Design Document

**Status:** Draft v1 · **Author:** Shahar Mintz (smintz) · **Date:** 2026-08-05
**Parent project:** entconnect (ships as an independently adoptable module; see §7)

## 1. Summary

`MixinForProto` is an [ent](https://entgo.io) mixin that materializes ent fields — including validation — from a protobuf message, making the contract the single definition of API-visible fields:

```go
func (Order) Mixin() []ent.Mixin {
    return []ent.Mixin{
        entconnect.MixinForProto[*orderv1.Order](
            entconnect.Exclude("etag"),
            entconnect.Override("status", field.Enum("status"). /* transitions, etc. */),
        ),
        mixin.Time{},
    }
}
```

It is a **runtime mixin**: plain Go executing at schema-load time. No entc extension, no code generation, no committed descriptor files. It does exactly one thing — message → `[]ent.Field` (+ validation relay) — and has three explicit non-goals (§3).

## 2. API

```go
func MixinForProto[M proto.Message](opts ...Option) ent.Mixin
```

The type parameter is the generated message type. Its descriptor is obtained via `protoreflect` (`(*new(M)).ProtoReflect().Descriptor()`), so the reference is compile-time-checked and the Go package carries its own schema — no `FileDescriptorSet` file, no string message names, no load-order coupling to `buf`. (The schema package already imports `orderv1` for flow inputs; this adds no new dependency direction.)

Options (v1, deliberately minimal):

- `Exclude(names ...string)` — message fields not materialized. Excluding a nonexistent field panics at schema load (fail fast — schema load *is* the check phase for a runtime mixin).
- `Override(name string, f ent.Field)` — replace the derived field wholesale with a hand-declared one; used to attach annotations (e.g. `entflow.Transitions`) or adjust storage details. Overriding a nonexistent field panics. Overrides suppress validation relay for that field (you own it entirely — no silent merging; see Open Question 1 in the entconnect doc, resolved here in favor of explicit `Override` over shadowing precisely because implicit shadow-merge semantics were unspecifiable).
- `WithMessageRules(OnCreate)` — opt-in Tier 3 enforcement (§4.3).

## 3. Non-Goals (the rescope)

1. **No service or handler generation.** RPC binding, CRUD emission, and interceptors belong to entconnect's entc extension. `MixinForProto` does not know services exist.
2. **No edge generation.** See §5 — edges are human-declared, machine-verified, and the verification lives in entconnect's drift check, not here.
3. **No drift checking.** A runtime mixin cannot see the service surface or the declared edges of other schemas; cross-artifact validation is codegen's job (entconnect §3.4).

Dependencies: `ent`, `google.golang.org/protobuf`, `protovalidate-go`/`cel-go`. Nothing else. This is what makes it shippable as a standalone micro-module for people who want proto-first ent and none of the rest of the stack.

## 4. Validation Relay

The reason to relay validation into the schema at all, rather than trusting the transport interceptor: **mutations do not only originate from RPC.** Flows, workers, seeds, tests, and CLIs all mutate through the ent client. The interceptor at the boundary is UX (fast, well-shaped errors before any DB work); the schema is the guarantee. Both are generated from the same protovalidate source, so they cannot disagree.

Three tiers, by constraint class:

### 4.1 Tier 1 — structural translation (introspectable)

Standard protovalidate constraints with a native ent equivalent are translated to real builder calls, because native constraints are visible to the rest of the ecosystem (entoas/entgql/documentation generators see `MaxLen`; error messages are ent-idiomatic; some map to DB-enforceable properties):

| protovalidate | ent |
|---|---|
| `string.min_len` / `max_len` / `len` | `MinLen` / `MaxLen` / both |
| `string.pattern` | `Match(regexp.MustCompile(...))` |
| `string.uuid`, `email`, `hostname`, `uri`, `ip`… | `Match`/`Validate` with stock validators |
| numeric `gt/gte/lt/lte` | `Min`/`Max`/`Range` (with open/closed adjustment) |
| numeric `gt: 0` | `Positive()` |
| `required` (presence) | `NotEmpty` (string) / non-`Optional` |
| `enum.defined_only` + values | `field.Enum` construction itself |
| `repeated.min_items/max_items` (scalar lists) | JSON field + Tier 2 check |

### 4.2 Tier 2 — CEL passthrough (fidelity)

Everything field-scoped that Tier 1 doesn't cover is **not** translated — it is *executed*. protovalidate is CEL; `protovalidate-go` ships the evaluator. At schema load, residual field rules are compiled once into CEL programs; at mutation time they run with `this` bound to the field's value. 100% fidelity with zero mapping-table maintenance for the long tail.

Mechanism correction to earlier drafts: ent exposes arbitrary `Validate(fn)` on strings only, so Tier 2 does **not** attach per-field validators. Instead the mixin declares a single `Hooks()` entry — mixins may declare hooks — that, on Create/Update, iterates the mutation's *changed* fields and evaluates each field's compiled programs. One hook, all field types, uniform error shape. Evaluating changed-fields-only is semantically sound for field-scoped rules (each rule reads only `this`).

Errors carry the protovalidate constraint ID and message, so entconnect's transport layer maps schema-level violations to the identical wire error the boundary interceptor would have produced — callers cannot tell which layer caught it, which is the point.

### 4.3 Tier 3 — message-level rules (the honest gap)

Cross-field CEL (message-scoped rules) assumes a complete message; ent mutations are legitimately partial on Update. Default behavior: message-level rules are **boundary-only** (entconnect's protovalidate interceptor) and the mixin ignores them, documented loudly. Opt-in `WithMessageRules(OnCreate)`: on Create — where the mutation is complete — the hook constructs an `M` from mutation fields and runs full message validation. Update-time message rules via fetch-then-merge are deliberately excluded from v1 (hidden query cost, unclear semantics under concurrent writes); if demanded, they arrive as an explicit `OnUpdateWithFetch` option so the cost is visible in the schema.

## 5. Field Mapping and Edges

Scalar mapping as in the entconnect doc (proto scalars → corresponding ent fields; `enum` → `field.Enum`; `optional` → `Nillable().Optional()`; `google.protobuf.Timestamp` → `field.Time`). Additional decisions:

- **Message-typed fields (singular and repeated): skipped by default.** They are relationships, and relationships are edges. `MixinForProto` never declares edges because protos express *shape* while edges carry relational semantics the contract does not state: direction (`Ref`), ownership and cascade, uniqueness, requiredness. Inferring them means inventing conventions; and a generic mixin cannot reference the application's schema types (`OrderItem.Type`) without the user passing them in, at which point it is manual declaration with worse locality than `Edges()`. Therefore: **edges are human-declared in `Edges()`, machine-verified by entconnect's drift check** — every skipped message-typed field must be covered by a declared edge of matching name/target or an explicit `Exclude`; id fields carrying `google.api.resource_reference` warn when no corresponding edge exists. Relationships stay a schema concern; their consistency with the contract is a build failure.
- **Embedded-value opt-in:** `AsJSON("field")` maps a message-typed field to `field.JSON` for genuinely embedded (non-entity) values.
- **`map<K,V>` of scalars** → `field.JSON` with a Tier 2 check for map constraints; maps of messages → skipped (edge or `AsJSON`).
- **`oneof`** → schema-load panic unless every member is `Exclude`d or `Override`n — no silent guess.
- **Well-known types:** `Duration` → `field.Int64` (nanoseconds) via option, else skipped; `Struct`/`Value` → `field.JSON`; `FieldMask` → skipped (transport concern).
- **proto3 presence:** non-`optional` scalars have no presence — mapped as ent `Default(zero-value)` non-optional fields, so "unset" and "zero" collapse exactly as they do on the wire; contracts that need the distinction must say `optional`, which is proto's own answer. Recorded as the sharpest edge of the design (Open Question 1).

## 6. Testing

- **Conformance corpus:** one proto file per mapping row and per constraint class, golden-asserted against the derived `[]ent.Field`.
- **Differential validation (the release gate):** for every corpus message, generate random values and assert `protovalidate verdict == ent mutation verdict` for field-scoped rules — the property the whole design rests on is that boundary and schema *cannot disagree*.
- **Load-time failure tests:** every panic path (unknown Exclude/Override, unhandled oneof) has a test proving it fires at schema load, not first mutation.

## 7. Module and Roadmap

Ships at its own module path (working name `entconnect/mixinforproto` with an independent `go.mod`; standalone-repo extraction is a release-time decision) so adoption requires nothing else from the stack.

1. **v0.1** — scalars, enums, WKTs, presence rules, `Exclude`/`Override`, Tier 1 translation.
2. **v0.2** — Tier 2 CEL hook + differential test harness.
3. **v0.3** — `WithMessageRules(OnCreate)`, `AsJSON`, map/oneof handling.
4. Drift-check counterpart lands in **entconnect v0.2+** (not here — non-goal 3).

## 8. Open Questions

1. proto3 presence vs ent `Optional` — the `Default(zero)` collapse is correct-by-wire-semantics but will surprise ORM-minded users; needs a prominent doc section and possibly a `StrictPresence` option that panics on non-optional scalars.
2. Sensitive fields: no proto-standard marker for `field.Sensitive()`; candidate: respect `debug_redact` field option.
3. Custom field options as an annotation channel (e.g. `(entconnect.field).immutable = true`) — powerful, but starts migrating schema semantics into the proto; deferred until real demand, with the default answer being "that's what `Override` is for."
