# mixinforproto

`mixinforproto` is a standalone [ent](https://entgo.io) mixin that derives `[]ent.Field`
— including translated validation — from a generated protobuf message *type*, so the
protobuf contract is the single, one-time definition of a schema's API-visible fields.
It is a **runtime mixin**: plain Go executing at schema-load time. No entc extension,
no code generation, no committed `FileDescriptorSet`. Schema load *is* the check phase,
so a malformed derivation panics there, not at first mutation.

`mixinforproto` is independently adoptable: its own `go.mod` depends only on `ent`,
`google.golang.org/protobuf`, and `buf.build/go/protovalidate` — nothing about the rest
of the `entconnect` project is required to use it.

## Usage

```go
import (
    "entgo.io/ent"
    "entgo.io/ent/schema/mixin"

    orderv1 "path/to/generated/order/v1"
    "github.com/smintz/entconnect/mixinforproto"
)

func (Order) Mixin() []ent.Mixin {
    return []ent.Mixin{
        mixinforproto.MixinForProto[*orderv1.Order](),
        mixin.Time{},
    }
}
```

The type parameter is the generated message type. Its descriptor comes from
`protoreflect` (`(*new(M)).ProtoReflect().Descriptor()`), so the reference is
compile-time-checked: no string message name, no separately-loaded descriptor file, no
load-order coupling to `buf`.

A message-typed field is skipped by default; opt one in as a JSON-typed field with
`AsJSON`:

```go
mixinforproto.MixinForProto[*orderv1.Order](
    mixinforproto.AsJSON("metadata"),
)
```

> **Not yet available in this release:** `mixinforproto.md` §2 also documents
> `Exclude(names ...string)` and `Override(name string, f ent.Field)`. Neither is part
> of this package's public API yet — they ship in a subsequent Phase 1 plan (Exclude/
> Override, reserved-identifier and `oneof` gates). Don't copy them from the design doc
> expecting them to compile against this release.

## Proto3 presence and the zero-collapse (read this before you write an Update handler)

This is the design's sharpest edge, and it is the reason this section exists above the
API reference rather than as a footnote in the mapping table.

**The mechanism.** A non-`optional` proto3 scalar field has no wire presence: `0`, `""`,
and `false` are indistinguishable from "not set" on the wire. `mixinforproto` maps such
a field to a **non-optional** ent field with `Default(<Go zero value>)`. This is correct
for wire semantics — there is nothing to lose by collapsing to a DB default, because the
wire itself already collapsed it — but it means **"unset" and "zero" are the same value**
from this point on, both in the generated Go accessor and in the database column.

**The concrete failure mode.** `mixinforproto` cannot fix this at this layer, and it will
bite the moment you write a hand-rolled Update. If your Update handler builds an ent
mutation by calling the generated `Set*` method for **every** field present in a decoded
proto message —

```go
// DO NOT DO THIS — silently clears every field the caller left at zero.
update.
    SetName(req.GetName()).
    SetPriority(req.GetPriority()).
    SetQuantity(req.GetQuantity())
```

— then any field the caller's client genuinely didn't intend to touch, and therefore
left at its Go zero value, gets written as zero. There is no way to tell "the caller
means to clear this to zero" apart from "the caller never mentioned this field" once
you're holding a decoded proto message and nothing else. Standalone `mixinforproto`
users are the audience most exposed to this: **you have no field-mask generation layer
protecting you.** You are writing `Set*` calls by hand from a proto message, and you
will reproduce this bug the first time you write a naive Update.

**The two-part fix, and which part is yours to apply today:**

1. **The contract's own remedy: declare the field `optional`.** A proto3 `optional`
   scalar carries real wire presence, and `mixinforproto` maps it to
   `.Nillable().Optional()` with no default — "unset" and "zero" stop being the same
   value, at the field level, immediately. If a field genuinely needs the
   unset-vs-zero distinction, this is the fix available to you *right now*, with no
   handler code required.
2. **The structural fix: a field-mask-gated Update handler.** `mixinforproto` alone
   cannot make a hand-written `Set*`-per-field Update safe — that requires the handler
   itself to gate every `Set*` call on `path in mask.GetPaths()`, never on "field is
   non-zero in the request." This is `entconnect`'s Phase 2 generated Update RPC, not
   something `mixinforproto` provides. If you are using `mixinforproto` standalone, with
   no generated handler layer, this fix is entirely on you to build by hand — the
   `optional` keyword above is the only mitigation this package can offer you directly.

## String length: Unicode code points vs. bytes (read this if your contract has non-ASCII string data)

`string.min_len`/`max_len`/`len` in `buf.validate` count **Unicode code points**
("characters"), documented explicitly by `buf.validate` itself as a value that "may differ
from the number of bytes in the string." Ent's `MinLen`/`MaxLen` count **bytes**
(`len(v)` on a Go string). For any ASCII-only value these are the same number, so nothing
below applies. For non-ASCII values, they are not.

**`mixinforproto` maps these constraints directly anyway** — `string.min_len` becomes
`MinLen(n)`, `string.max_len` becomes `MaxLen(n)`, `string.len` becomes both — rather than
treating the mismatch as untranslatable. This was a deliberate, informed decision (not an
oversight): the alternative was either shipping materially less Tier 1 coverage for the
single most common string constraint pair in real-world contracts, or complicating every
derived string field with a second, purely-defensive `.Validate(fn)` call whose only job is
to be a safe (over-wide) byte bound. Direct mapping was chosen; the trade-off is what this
section documents.

**The concrete consequence.** Three emoji are 3 Unicode code points but 12 UTF-8 bytes.
A contract declaring `string.max_len = 5` **accepts** a 3-emoji value at the protovalidate
boundary (3 code points ≤ 5) — and the derived ent field's `MaxLen(5)` **rejects the exact
same value** at the storage layer (12 bytes > 5). A value in a non-Latin script, or
containing emoji, can pass an RPC boundary interceptor and then be rejected when the ent
mutation runs — and a caller inspecting the two error responses can tell which layer caught
it, which is precisely the kind of boundary-versus-schema disagreement this project exists
to eliminate everywhere else.

**Where this is recorded.** The divergence is not documentation-only. Every field whose
translation carries this caveat lists its own constraint ID (e.g. `"string.max_len"`) in
`SourceField.LengthUnitDivergentIDs`, in addition to `TranslatedIDs` — machine-visible to
Phase 3's differential harness (which must decide how to treat non-ASCII inputs against
these specific constraints) and to Phase 5's fingerprint comparison, not just this
paragraph.

**Byte-semantic constraints are unaffected.** `string.min_bytes`/`max_bytes`/`len_bytes`
and every `bytes.*` length constraint compare bytes on both sides and translate exactly,
under this decision or any other — nothing above applies to them, and they never appear in
`LengthUnitDivergentIDs`.

**If this matters to your contract:** either accept the divergence (many applications never
see non-ASCII input in these specific fields), or treat `string.min_len`/`max_len`/`len` on
fields that do carry non-ASCII data as needing an explicit, hand-written
`Override(name, ...)` with your own exact code-point-counting validator until a future
release closes this gap generically.

## Mixin hook and policy ordering

A mixin's `Hooks()`, `Interceptors()`, and `Policy()` all run **before** the ones a
schema author declares directly on the schema — this is `ent.Mixin`'s own documented
contract, not something `mixinforproto`-specific. As of Phase 3, `mixinforproto` ships a
real `Hooks()` entry: every protovalidate field-rule (standard and residual CEL alike)
is compiled once at schema load and evaluated at mutation time, rejecting a mutation
before any SQL is issued.

The full, verified order — for both Create and Update — is:

```
defaults() -> privacy Policy (when declared) -> mixin hooks -> schema hooks -> sqlSave() -> check()
```

On Create, `defaults()` materializes every `Default()`-bearing field into the mutation
before any hook fires; on Update, only `UpdateDefault()`-tagged fields are applied, and
none of `MixinForProto`'s derived fields carry `UpdateDefault`. If the schema (or any of
its mixins) declares a `Policy()`, that policy's combined `EvalMutation` runs first —
`ent`'s own generated wiring installs it at `Hooks[0]` **whenever any policy exists
anywhere on the schema**, not at a fixed index — so an unauthorized caller is always
denied before any constraint is evaluated. Because privacy always precedes the
validation hook whenever a policy is declared, **there is no constraint-existence
leak**: a denied caller's error is `privacy.Deny`, never a
`*protovalidate.ValidationError`, so nothing about which constraints exist on the
entity is observable to them. With no policy anywhere on the schema there is no
authorization gate to leak around, so the scenario is moot — and in that case the
mixin's validation hook simply IS `Hooks[0]`.

This order is pinned by a test that fails if `ent` changes it:
`mixinforproto`'s own `internal/entconnecttest/hookwiring` fixture (root module) proves
the relative sequence "privacy policy (if any) -> mixin hook -> schema-declared hooks"
against a real, generated `ent.Client`, in both a policy-bearing and a policy-free
schema configuration — asserted by relative sequence, never by a hard-coded index into
any hooks slice, so an application merely adding or removing a `Policy()` is never a
false regression.

Relatedly: **field-scoped protovalidate constraints — translated and residual alike —
are now enforced at both the storage layer and the RPC boundary, with identical
verdicts.** Constraints with a native ent equivalent (Tier 1 — `MinLen`, `Match`,
`Min`/`Max`/`Range`, and friends) are translated into real ent builder calls; the mixin
hook now enforces the full protovalidate field-rule set ahead of them, so those native
builders are effectively shadowed rather than the last word. Message-level (cross-field)
rules stay **boundary-only by default** — the storage layer only enforces them if a
schema opts in with `WithMessageRules(OnCreate)`. Check `SourceField`'s `ResidualIDs` for
provenance of which constraints originated as residual CEL rather than a Tier 1
translation — not for "what isn't enforced yet".

## Known limitation: derived-name collisions

At derivation time, `mixinforproto` checks every derived field name against ent's known
reserved identifiers and panics at schema load with the fix, if there's a collision.
This is a real check with real teeth for that one category.

What it **cannot** catch: a collision against a field you hand-declared in the same
schema's own `Fields()`, or a field contributed by another mixin. `ent` composes mixins
before the schema's own `Fields()` runs, so `mixinforproto` has no visibility into either
case at the point it derives fields. Both surface later, as the Go compiler's ordinary
`redeclared in this block` error in generated code — a real error, just not one
`mixinforproto` can pre-empt or name for you.

`mixinforproto` deliberately does **not** auto-rename a colliding field. A silently
renamed API-visible field would defeat the entire point of the contract being the source
of truth for that field's name — if you hit a collision, the fix is `Exclude` (once
available) or renaming the field in the contract itself, never a mixin-side rename you
didn't ask for.

## API reference

| Symbol | Kind | What it does |
|---|---|---|
| `MixinForProto[M proto.Message](opts ...Option) ent.Mixin` | function | Derives an `ent.Mixin` from generated message type `M`. Declare it in a schema's `Mixin()` method. |
| `Validate[M proto.Message](opts ...Option) error` | function | Runs the identical derivation `MixinForProto`/`entc` use, in-process, without going through `entc`'s schema-load subprocess. Call it from a plain `go test` in your schema package to get a full, untruncated error when a derivation fails — `entc`'s subprocess panic output is routinely truncated to one line. |
| `AsJSON(name string) Option` | function | Opts a message-typed field into `field.JSON` derivation. Message-typed fields are skipped by default. An empty name, an unknown field name, or a non-message-typed name all fail at schema load, naming the message, the field, and `"AsJSON"`. |
| `ContractVersion` | `const int` | The version of the annotation contract below (`SourceMessage`/`SourceField`'s on-the-wire shape). Bumped only on a breaking layout change; a decoder reading a higher version than it was compiled against must fail with a named mismatch error, never silently decode a partial struct. |
| `MixinForProtoMessage` | `const string` | The `schema.Annotation` key `SourceMessage` is stored under on `gen.Type.Annotations`. |
| `MixinForProtoField` | `const string` | The `schema.Annotation` key `SourceField` is stored under on `gen.Field.Annotations`. |
| `SourceMessage` | struct, `schema.Annotation` | Schema-level provenance: the derived proto message's full name, the **complete** field inventory (name + number, regardless of exclusion/override), and the excluded/overridden field-name lists. The complete inventory exists so a later drift check can tell "deliberately excluded" apart from "silently forgotten" — both look identical as absence from `gen.Graph` without it. |
| `SourceField` | struct, `schema.Annotation` | Field-level provenance attached to each derived `ent.Field`: source proto field name/number, the derivation kind, the protovalidate constraint IDs Tier 1 translated, the residual (untranslated) constraint IDs plus a stable fingerprint of their CEL expression strings, and (`LengthUnitDivergentIDs`) which translated IDs carry the code-point-vs-byte string length caveat above. |

`SourceMessage` and `SourceField` are the **only** channel across `entc`'s schema-load
JSON boundary — `load.Schema.Field.Validators` is an `int` count on the other side of
that boundary, not closures, so nothing else survives the round trip. Anything you need
a downstream consumer (a future `entconnect` entc extension, a drift check) to see about
a derivation has to go through one of these two structs.

## Field mapping (what's implemented so far)

A `repeated` field is never mapped to a list-typed ent field in v0.1 (01-06-PLAN.md, closing
a gap the initial verification pass found — see the three `repeated`-cardinality rows below):
a repeated scalar or enum fails loudly at schema load rather than silently collapsing to a
singular column, and a repeated message field keeps its pre-existing skip/`AsJSON` behavior.

| Proto shape | Ent mapping |
|---|---|
| All 15 scalar kinds (`int32`, `uint64`, `sint32`, `fixed64`, …) | Same-width ent builder, no widening |
| `enum` | `field.Enum(name).Values(...)` with the declared value names, in declaration order |
| `google.protobuf.Timestamp` | `field.Time` |
| `google.protobuf.Struct` | `field.JSON` (`map[string]any`) |
| `google.protobuf.Value` | `field.JSON` (`json.RawMessage`, for lossless round-tripping) |
| `google.protobuf.FieldMask`, `google.protobuf.Duration` | Skipped entirely |
| `optional` scalar (real proto3 presence) | `.Nillable().Optional()`, no default |
| Plain (non-`optional`) scalar | `.Default(<Go zero value>)`, non-optional — see the presence section above |
| `map<K,V>` of scalars | `field.JSON`, one field per map |
| `map<K,V>` with a message value | Skipped entirely |
| Message-typed field (not opted into `AsJSON`) | Skipped entirely |
| Message-typed field, `AsJSON("name")` | `field.JSON` |
| `repeated` message-typed field (not opted into `AsJSON`) | Skipped entirely — same MIX-09/MIX-10 rule as a singular message field, unaffected by cardinality |
| `repeated` message-typed field, `AsJSON("name")` | `field.JSON` — same opt-in as a singular message field |
| `repeated` scalar or `repeated` enum | **Fails at schema load**, naming the message, the field, its repeated cardinality, and the `Exclude`/`Override` remedy — never silently derived as a singular column (closes the gap the initial verification pass found) |
| `repeated` field carrying `repeated.items` protovalidate rules | Same loud schema-load failure as any other repeated scalar/enum — the constraints are never silently discarded |
| Real `oneof` member | Skipped unless every member is `Exclude`d or `Override`n — an unresolved real `oneof` fails at schema load (never silently guessed) |
| `string`/numeric field with a translatable protovalidate rule (`min_len`/`max_len`/`len`/`min_bytes`/`max_bytes`/`len_bytes`/`pattern`/`email`/`hostname`/`uri`/`ip`/`uuid`/`gt`/`gte`/`lt`/`lte`) | A real native ent validator call (`MinLen`/`MaxLen`/`Match`/`Validate`/`Min`/`Max`/`Range`) — see "Relatedly" above and the string-length-unit section |
| `required` on a presence-tracking (`optional`) field, any kind including `string`/`bytes` | Non-optional construction — no `NotEmpty()`, no `Nillable().Optional()`, no `Default`. This is a presence assertion ("must be *set*"), not a non-emptiness one: a deliberately-set empty string/bytes value is admissible, matching protovalidate's own verdict (01-VERIFICATION.md gap 3 / 01-REVIEW.md CR-03) |
| `required` on an implicit-presence (plain, non-`optional`) `string`/`bytes` field | `NotEmpty()` — protovalidate's "can't be the zero value" semantics for a field with no separate presence bit |

Delegated format validators (`hostname`/`uri`/`ip`/`uuid`) judge only the field under
validation: `delegatingFormatValidator` evaluates the whole synthetic message but filters
protovalidate's violations down to the one attributed to the candidate field, so an
unrelated rule elsewhere on the same message (another `required` field, a different format
rule) never causes the candidate value to be rejected (01-VERIFICATION.md gap 2 /
01-REVIEW.md CR-02).

Derived field order always matches proto declaration order and is stable across repeated
runs and concurrent goroutines — `mixinforproto` never ranges a map directly into
ordered output anywhere in the derivation path.
