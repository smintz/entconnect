# Pitfalls Research

**Domain:** protobuf-descriptor-driven code generation and runtime mixins in the Go/ent ecosystem (entconnect + mixinforproto)
**Researched:** 2026-08-08
**Confidence:** MEDIUM — synthesized from ent/contrib GitHub issues (primary source, fetched directly), official protobuf.dev / buf.build / protovalidate.com / atlasgo.io documentation (authoritative), and web-search-aggregated community reports (single-source, not cross-verified) for a design that has no direct prior art (no project combines runtime-mixin field derivation with proto-first codegen the way entconnect/mixinforproto does — most pitfalls below are inferred by analogy from entproto/entgql/entoas, ent core issues, and the protovalidate/CEL ecosystem, not observed directly in this design).

## Critical Pitfalls

### Pitfall 1: `entc`'s schema-load panics arrive with no usable stack trace

**What goes wrong:**
`entc generate` (and any extension built on it, including `mixinforproto`) loads the `ent/schema` package by compiling it into a temporary Go binary and executing it out-of-process. When that process panics — which is exactly the mechanism `mixinforproto` intentionally uses for `Exclude`/`Override`/`oneof` validation failures — the failure the user sees on `go generate ./...` is frequently just `exit status 2` or a truncated one-line panic message, not a full Go stack trace pointing at the offending `Mixin()` call. This is a documented, filed ent issue (users explicitly asking "is there a way to force a stack trace on panic for entc?"), not a hypothetical. For a design whose entire safety argument is "schema load *is* the check phase, so failures panic there rather than at first mutation," an undebuggable panic defeats the UX goal even though it is technically correct.

**Why it happens:**
`entc/load` shells out to `go run` (or an equivalent build+exec) against a synthesized `main` package so it can execute arbitrary user schema code and introspect the resulting `ent.Schema` values. Panics inside that subprocess are relayed to the parent's stderr, but subprocess panic output is easy to lose or truncate depending on how the parent process captures stdout/stderr, and CI log truncation makes it worse.

**How to avoid:**
- `mixinforproto` panics must always include: the schema type name, the offending proto field name, the option (`Exclude`/`Override`) or condition (`oneof` ambiguity) that triggered it, and a one-line remediation ("did you mean to call `Override(\"status\", ...)`?"). Never rely on the Go runtime's default panic formatting alone — construct a custom `panic(fmt.Errorf(...))` with all context inline, since the caller may only see the first line.
- Add a `entconnect doctor` / `mixinforproto.Validate()` dev-mode entry point that runs the same derivation logic in-process (not via `entc`'s subprocess) so users can get a full, non-truncated Go stack trace by running a plain `go test` or `go run` against their schema package directly, bypassing `entc load` entirely for debugging.
- In docs, explicitly tell users: "if `go generate` fails with an opaque exit status, re-run the mixin construction directly via `go run ./ent/schema/...` (or a provided debug harness) to see the real panic."

**Warning signs:**
- Users filing issues that say "go generate just says exit status 2" with no further detail.
- Panic messages that are just the Go zero-value default (`panic: runtime error: index out of range`) rather than a custom, named error.

**Phase to address:** v0.1 (MixinForProto) — must be solved before the panic-on-schema-load design ships, since it is the design's core debuggability promise.

---

### Pitfall 2: Mixin-derived field names silently collide with hand-declared or other-mixin fields, producing a Go compile error far from the cause

**What goes wrong:**
ent has a real, filed bug class (`ent/ent#280`, "Name collision in generated code") where a mixin-derived field name collides with an entity's own generated identifiers (e.g., a field named `label` collides with the auto-generated `Label` constant/predicate). The failure surfaces as a Go compiler error in *generated* code (`Label redeclared in this block — previous declaration at ent/phone/phone.go:14:10`), not as an ent-level or mixin-level error. For `MixinForProto`, every proto message field becomes a candidate for this collision, and the collision can be between the proto-derived field and (a) a hand-declared field in the same schema's `Fields()`, (b) another mixin's field (e.g., `mixin.Time{}`'s `created_at`/`updated_at`), or (c) ent's own reserved identifiers (`id`, `Label`, `where`, edge names).

**Why it happens:**
`MixinForProto` derives one field per proto message field automatically; there is no per-field human review step before the name reaches ent's codegen. A field named `status`, `type`, or `id` in the proto message is exactly the kind of name most likely to collide with hand-declared schema fields or ent reserved words, since these are the most common domain field names.

**How to avoid:**
- At `MixinForProto` construction time (schema load), before returning fields to ent, check derived field names against: ent's known reserved identifiers, and — if feasible — cross-reference against fields already declared via `Fields()` on the same schema (ent's mixin composition happens before the schema's own `Fields()` runs, so this may require a post-composition check inside the generator extension rather than the mixin itself). At minimum, panic-check against ent's reserved word list (`id`, `Label`, `Type`, `Where`, etc.) since those are catalog-known.
- Document loudly: "if a proto field name matches an ent reserved identifier or another mixin's field, you must `Override` or `Exclude` it — this is caught at compile time of generated code, not at schema load, and the panic will be a Go compiler error, not an entconnect error."
- Add a golden/conformance test fixture with a proto message that deliberately has a field named `label`, `type`, `id` — asserting either an early, well-messaged panic (preferred) or documented Go-compiler-error behavior.

**Warning signs:**
- `go build ./ent/...` fails with `redeclared in this block` after adding or renaming a proto field, with no entconnect-level error preceding it.

**Phase to address:** v0.1 (MixinForProto) for the reserved-word panic; drift check (v0.2+) is the wrong layer since this is a same-schema, not cross-artifact, concern.

---

### Pitfall 3: Mixin `Hooks()` execute before schema hooks and before schema `Policy()` — ordering assumptions will silently misfire

**What goes wrong:**
ent's documented composition order is: mixin hooks run before schema hooks, and mixin privacy policies run before schema privacy policies (mixins are evaluated in the order returned by `Mixin()`, and within a schema, mixin-declared behavior precedes schema-declared behavior). `MixinForProto`'s Tier 2 CEL validation hook is declared *by the mixin*, meaning it necessarily runs **before** any hooks or privacy the schema author writes by hand in `Hooks()`/`Policy()` — including hooks that might normalize or default a field before validation, or privacy rules that should gate whether the mutation is even allowed to proceed. If a schema author writes a hook that fills in a computed field (e.g., derives `total_price` from line items) expecting it to run before validation, the CEL hook will have already evaluated against the *unfilled* value, either rejecting a legitimate mutation or (worse) passing a value that later gets silently overwritten.

**Why it happens:**
The ordering is a fixed ent framework behavior, not something `MixinForProto` controls, but the design doc does not mention it and a schema author reading only the `mixinforproto.md` design doc has no reason to know their own `Hooks()` runs *after* the validation hook, not before.

**How to avoid:**
- Document explicitly, in both `mixinforproto.md`-derived docs and generated schema comments: "the CEL validation hook runs before any hooks or privacy declared directly in this schema's `Hooks()`/`Policy()`. If you need to computed-default a field before validation, either pre-populate it in the mutation before calling `client.X.Update()`, or (if genuinely needed) order it via an earlier mixin."
- Add a conformance test: a schema with a hand-written hook that mutates a field, verifying the CEL hook sees the pre-hook value (documenting, not silently "fixing," the ordering — fixing it would require breaking ent's own composition contract).
- Consider exposing a documented low-level escape: since `WithMessageRules(OnCreate)` already opts into stricter enforcement, evaluate whether a `DeferValidation()`-style option that reorders the hook relative to *this schema's own* hooks is worth the added surface — likely not for v1, but flag as an open question if early adopters hit it.

**Warning signs:**
- Bug reports of "my validation rejects a value I just computed in a hook" or "CEL validation passed but the stored value differs from what was validated" (the inverse failure — a hand-written hook running *after* validation silently invalidates a value that was checked).

**Phase to address:** v0.2 (MixinForProto Tier 2 CEL hook) — must be documented at the same time the hook ships, not retrofitted later.

---

### Pitfall 4: proto3-non-optional → `Default(zero)` collapses "unset" and "zero" — this breaks partial updates, not just reads (the design's self-declared sharpest edge)

**What goes wrong — concretely, not abstractly:**
The design maps non-`optional` proto3 scalars to non-optional ent fields with `Default(zero)`. This is *correct* for wire semantics (proto3 non-optional fields genuinely have no presence — `0`, `""`, `false` are indistinguishable from "not set" on the wire), but it produces four distinct, concrete failure modes once it meets ent's mutation and update model:

1. **PATCH-style partial updates silently clear fields to zero.** If a generated `Update` RPC handler builds an ent mutation by setting *every* field present in the request message (the naive approach: `update.SetName(req.GetName()).SetPriority(req.GetPriority())...`), then any field the caller's client library left at its Go zero value — because the caller genuinely didn't intend to touch it, not because they wanted to zero it — gets written as zero. This is the single most common real-world proto-partial-update bug (well documented across grpc-gateway's PATCH issue tracker and multiple field-mask design writeups): "is `0`/`\"\"`/`false` an intentional value or an omitted field?" cannot be answered once you've mapped non-optional proto scalars straight through to non-optional ent columns.
2. **`google.protobuf.FieldMask` (Open Question 5 in `entconnect.md`, unresolved) is the only sound fix, and it only works if every RPC's Update handler is generated to gate each `Set*` call on both (a) the field being present in the mask and (b) — for non-optional-in-proto fields — some indication the caller means to set it, which for a non-optional field *is* "present in the mask," full stop. If field-mask semantics are not adopted (Open Question 5 says "pick one, document it, generate it consistently" — but does not commit), whichever convention is chosen by default (e.g., "always set the fields present in the wire message") will reproduce failure mode 1 for every non-optional field, on every Update RPC, for every entity — this is not a corner case, it is the default behavior of an unmasked Update.
3. **Zero-vs-unset ambiguity on Create is comparatively benign** (Create genuinely is a complete message in the sense the mixin's Tier 3 `WithMessageRules(OnCreate)` already assumes), but on Update it compounds with Tier 2's "changed fields" hook: the CEL hook validates ent's *mutation-changed* fields, and if the handler's naive `Set*` pattern marks a field "changed" in the ent mutation even though the caller's message had it at zero-because-omitted, Tier 2 validation runs against a value the caller never intended to submit — a false-positive validation failure (e.g., a `min_len` rule now rejects an "update" the caller never made) or, if the rule permits zero, a false negative (a required field silently reset to empty and it passes validation because empty is a valid *value* even though it should never have been touched).
4. **DB `NOT NULL DEFAULT <zero>` plus this pattern means Atlas migrations will never surface the ambiguity** — the database genuinely cannot distinguish "this row's field was updated to zero" from "this row's field was never touched," so any downstream analytics, audit log, or event-sourcing consumer that assumes "field present in an update event = field intentionally changed" will be silently wrong, and there is no migration-time or CI-time signal that this happened — it is a data-quality bug that only shows up in production, in aggregate, weeks later.

**Why it happens:**
The mapping is *individually* correct at each layer (proto wire semantics, ent field semantics, DB semantics) — the bug is emergent, at the seam between "how the generated Update handler decides which ent `Set*` calls to make from a partially-populated proto message." The design doc flags the field-mapping decision as the sharpest edge but Open Question 5 (field masks) is explicitly unresolved, and it is precisely the resolution of Open Question 5 — not the field-mapping rule itself — that determines whether this pitfall is avoided or guaranteed.

**How to avoid:**
- **Resolve Open Question 5 before v0.2 ships any generated Update handler, and resolve it as: `FieldMask`-driven, mandatory, not optional.** Every generated Update RPC's request message must either embed a `google.protobuf.FieldMask update_mask` field or the handler generator must reject (drift-check error) any Update RPC whose request lacks one. A convention of "infer changed fields from what's non-zero in the message" reproduces failure mode 1 by construction and should be explicitly rejected in the design doc, not left as a coin flip.
- Generated Update handlers must gate every `Set*` call on `path in mask.GetPaths()`, never on "field is non-zero in the request." This makes the ent-layer `Default(zero)` mapping safe again, because the *handler* — not the field default — is what decides presence.
- For contracts that need real per-field optionality independent of field-mask discipline (e.g., "this field can be nulled out via API but is required on create"), document `optional` as the required proto authoring convention and treat `Default(zero)` non-optional as the "you get wire semantics, nothing more" default — this matches the design doc's own resolution ("contracts needing the distinction must say `optional`"), but that resolution only protects *reads*; it does nothing for the Update-handler-construction problem above, which is a codegen concern, not a field-mapping concern. Both must be fixed; fixing only the field mapping (what the design doc currently documents) is necessary but insufficient.
- The differential validation harness (already planned) should be extended past field-scoped rule fidelity to include a **partial-update fidelity test**: construct a message with only a subset of fields set (mask-driven), run it through the generated handler, and assert the *resulting ent mutation's changed-field set* equals the mask, not "every non-optional field."
- For `mixinforproto` standalone users (no entconnect handler layer, hand-written mutation code), the danger is theirs to manage, but the mixin's docs should carry a large, explicit warning with the four failure modes above spelled out, because standalone users have no field-mask generation layer to save them — they are constructing `Set*` calls by hand from proto messages and will reproduce failure mode 1 the first time they write a naive Update handler.

**Warning signs:**
- A generated or hand-written Update handler that unconditionally calls `Set*` for every field on the request message without checking a mask.
- Bug reports/support tickets of the shape "I updated field A and field B silently reset to empty/zero."
- Any Update RPC's proto request message that lacks a `FieldMask` field.

**Phase to address:** v0.2 (CRUD handlers) is where this becomes real — the field-mapping decision (v0.1/MixinForProto) is necessary-but-insufficient; the actual bug is only preventable at Update-handler codegen time. Flag Open Question 5 as **must-resolve-before-v0.2**, not "pick one eventually."

---

### Pitfall 5: `field_mask` interaction with Tier 2 CEL "changed fields only" evaluation is not automatically sound once presence is masked, not inferred

**What goes wrong:**
`mixinforproto.md` states Tier 2 evaluates "the mutation's *changed* fields" and asserts this is "semantically sound for field-scoped rules (each rule reads only `this`)." This is sound *if and only if* "changed" in the ent mutation accurately reflects the caller's intent — which, per Pitfall 4, is only true once the handler is field-mask-gated. Two additional subtleties remain even after that fix: (a) a field explicitly set to its zero value *via the mask* (a legitimate "clear this field" intent) must still run through Tier 2 validation — if a `min_len: 1` rule exists on a string field, explicitly clearing it to `""` via the mask should trigger the same validation failure a client-side `Create` would, and the "changed fields" hook must not special-case zero values as "unchanged." (b) `oneof` fields present a compounding hazard: an update that changes one member of a `oneof` while the mask also lists a sibling member (or the codegen didn't reconstruct oneof membership correctly from the mask) can produce a mutation state Tier 2 evaluates as internally consistent even though the resulting entity violates the `oneof`'s "exactly one set" contract — this is a message-level (Tier 3) concern that field-scoped Tier 2 will not catch by design, and Tier 3 is boundary-only by default on Update per the design doc's own non-goal ("Update-time message-level validation via fetch-then-merge" excluded from v1).

**Why it happens:**
Tier 2's soundness argument ("field-scoped rules read only `this`") is correct for the rule *evaluation* but silently assumes the *set of changed fields* handed to it is trustworthy — which depends entirely on upstream mask-gating discipline that lives in generated handler code, not in the mixin itself.

**How to avoid:**
- Explicitly test "clear field via mask to zero value, assert Tier 2 validation still fires" in the conformance corpus — do not let "changed-fields-only" become, in practice, "non-zero-fields-only."
- Document the `oneof`-across-Update gap as a known, accepted limitation (consistent with the existing Tier 3 Update exclusion), and ensure the drift check or generated handler at minimum rejects (or the mixin's schema-load `oneof` panic-if-not-excluded rule extends to) any Update path that could leave a `oneof` in an inconsistent state — or explicitly document that `oneof` fields require `WithMessageRules` opt-in awareness on Create only, with Update-time `oneof` consistency left to the application.

**Warning signs:**
- A field successfully "cleared" via API bypasses a `required`/`min_len` CEL rule because the mutation's changed-field diff treated zero-after-explicit-clear the same as "not present."
- Two mutually exclusive `oneof` fields end up both populated after two independent partial updates.

**Phase to address:** v0.2 (Tier 2 CEL hook + differential harness) for the zero-vs-clear test; document the `oneof` Update gap alongside the existing Tier 3 Update exclusion in the same doc section.

---

### Pitfall 6: CEL compilation happens correctly (schema-load-once), but per-request evaluation cost is easy to get wrong at the boundary/schema seam

**What goes wrong:**
protovalidate-go and cel-go's own documentation are explicit and consistent: **compilation is expensive, evaluation is cheap** (sub-microsecond after warm-up per protovalidate's own benchmarks), and the entire performance model depends on compiling once and caching the resulting `cel.Program`. The design already does this correctly at the mixin level ("residual field rules are compiled once into CEL programs" at schema load). The actual risk is not in the mixin's compile-once step — it's in two places the design doc doesn't fully specify: (1) the **boundary interceptor** (entconnect's ConnectRPC protovalidate interceptor) must *also* compile once per process, not per-request or per-connection — if it's built naively (e.g., constructing a fresh `protovalidate.Validator` per RPC call instead of a process-wide singleton), every request pays full CEL compilation cost, which dwarfs actual validation cost by 2-3 orders of magnitude; (2) since the schema (Tier 2 hook) and the boundary interceptor are described as deriving "from the same protovalidate source" but are two *separate* compiled artifacts (one compiled at `entc`/schema-load time via reflection on the ent-side message type, one compiled at server-startup time via the boundary interceptor's own validator instance), there is no shared compilation cache between them by default — meaning the same CEL expression for the same message type gets compiled twice, once per process lifetime for each layer. This is a real but bounded cost (startup-time, not per-request), so it's a minor pitfall *unless* either layer is accidentally re-instantiated per-request.

**Why it happens:**
It's easy, especially with dependency-injection-style server wiring or naive middleware construction, to accidentally construct a `protovalidate.Validator` (which triggers CEL compilation) inside a request-scoped function rather than a process-scoped singleton — this is a classic Go web-framework footgun (same class of bug as re-compiling a `regexp.MustCompile` per request).

**How to avoid:**
- Generated server wiring must construct exactly one `protovalidate.Validator` (and, on the mixin side, ent already does this correctly by construction since mixin `Hooks()` build once at schema-load) per process, injected into the interceptor chain as a captured closure/singleton, never per-request.
- Add a benchmark (not just a golden test) in CI asserting p50/p99 request-level CEL evaluation cost stays in the sub-microsecond-to-low-microsecond range on the reference app's Order/Inventory entities, to catch an accidental re-compilation regression before it ships.
- Document explicitly, in the generated server wiring's own comments, "this validator is constructed once at server startup — do not move this call into a request handler."

**Warning signs:**
- p99 latency on Create/Update RPCs that's orders of magnitude higher than Get/List for no other reason.
- CPU profile showing `cel.NewProgram`/`cel.NewEnv` calls in the request hot path.

**Phase to address:** v0.2 (CRUD handlers, boundary interceptor construction) — verify with a benchmark test, not just a golden test.

---

### Pitfall 7: Error shape mismatch between the boundary interceptor and the schema-layer CEL hook is easy to promise and hard to keep exactly identical

**What goes wrong:**
The design's core claim — "callers cannot tell which layer caught it" — requires the schema-layer hook to reconstruct the *exact same* wire error (status code, constraint ID, message, field path) that the boundary's protovalidate interceptor would produce for the same violation. In practice this is harder than it sounds for two reasons: (1) protovalidate's own violation objects carry a `FieldPath` relative to the *message*, but the ent mutation hook is operating on *ent field names*, which are not guaranteed to be a 1:1, lossless mapping back to proto field paths (especially after `snake_case`→ent-idiomatic naming, or nested/`AsJSON` fields) — reconstructing the identical `field_path` in the schema-layer error requires the mixin to retain a field-name mapping table generated at schema load, which is implied by the design but not called out as a concrete artifact; (2) errors originating from *outside* an RPC context (flows, workers, seeds, CLIs — exactly the mutation sources the design cites as the reason validation is relayed into the schema at all) have no natural Connect/gRPC status code to map to, since there is no RPC in flight — the "identical wire error" framing only fully applies when a schema-level violation is subsequently caught and translated by *entconnect's transport layer* on an RPC path; for non-RPC mutation sources, the schema-level error is the only error that exists, and if it's not self-sufficiently well-formed (not just "identical to what the RPC layer would have produced" but independently actionable), worker/flow error handling degrades to opaque validation failures with no caller-facing context.

**Why it happens:**
The design is written from the RPC-boundary-vs-schema-boundary symmetry angle, but the schema hook's real audience includes contexts where there is no boundary layer to be "identical" to.

**How to avoid:**
- Make the ent-mutation-hook-side error type a first-class, structured Go error (not just a string) carrying the protovalidate constraint ID, the original proto field path, the violated rule, and the message — usable directly by non-RPC callers (flows, workers, tests) without needing an RPC context to be meaningful. entconnect's transport layer then translates *this* into the wire error, rather than the schema layer trying to preemptively shape a wire-format error it doesn't own.
- Build (and keep in sync at schema load, alongside CEL compilation) the ent-field-name ↔ proto-field-path mapping table explicitly, as a documented internal artifact, not an assumed-lossless naming convention.
- Test this specifically: same violation triggered via (a) a generated RPC handler and (b) a raw `client.Order.Update()` call from a test/worker context — assert both produce errors carrying the identical constraint ID and field path, even though (a) additionally wraps it in a Connect status code and (b) does not.

**Warning signs:**
- Worker/flow code catching validation errors and being unable to determine which field or constraint failed without string-parsing the error message.
- Field paths in schema-layer errors that don't match the proto field name (e.g., ent's `snake_case`-to-idiomatic renaming leaking into the error).

**Phase to address:** v0.2 (Tier 2 hook + transport error translation) — the structured-error type should exist before either the hook or the transport layer are considered done, since both depend on it.

---

### Pitfall 8: protovalidate/CEL version skew between the boundary interceptor's `protovalidate-go` and the schema's `cel-go`/`protovalidate-go` dependency is a real, filed failure class

**What goes wrong:**
protovalidate-go's own issue tracker documents concrete version-skew failures: a `RuntimeError` when a predefined rule definition present in a newer protovalidate version isn't recognized by an older extension type resolver, and CEL expression compilation failures when expressions reference fields imported from proto files compiled with a different module's protovalidate version (`protovalidate-java#118` is the same failure class, applicable in Go too — "unexpected failed resolution" of imported types when protovalidate/cel dependency versions diverge across the two places compilation happens). For this design, that's exactly the situation: the boundary interceptor and `mixinforproto` are two independent Go modules (different `go.mod` files per the module-isolation constraint), each with their own `protovalidate-go`/`cel-go` transitive dependency versions, which **will** drift over time as each module is upgraded independently — that's the entire point of the module-isolation design, but it's also exactly the precondition for this failure class.

**Why it happens:**
Go modules resolve dependency versions independently per-module unless pinned via a workspace or explicit version constraints; nothing in the current design ties `mixinforproto`'s protovalidate version to `entconnect/runtime`'s protovalidate version, and no CI check currently verifies they match.

**How to avoid:**
- Add a CI check (not just a `go.work` convenience) that asserts the resolved `protovalidate-go` and `cel-go` versions in `mixinforproto/go.mod` and `entconnect/go.mod`/`runtime/go.mod` are compatible — at minimum the same major/minor CEL environment version, since CEL's own extension/macro set can change between versions and a rule compiled against a newer CEL environment may not parse against an older evaluator.
- Document the version-skew risk explicitly in both design docs (currently absent), including that "identical protovalidate source" (the design's core claim) requires not just the same `.proto`-declared constraints but the same *evaluator* semantics, and those are two different things.
- Since `mixinforproto` is explicitly meant to be adoptable standalone (without entconnect at all), a standalone user has zero exposure to this — the risk is specific to combined entconnect+mixinforproto deployments, so scope the CI check accordingly (it belongs in the `entconnect` monorepo's CI, checking the two `go.mod`s, not in `mixinforproto`'s own CI).

**Warning signs:**
- A validation rule that passes at the boundary but fails (or errors) at the schema layer, or vice versa, for the same input, with no code change to the rule itself — only a dependency bump.
- `go.sum` diffs showing `cel-go`/`protovalidate-go` version bumps in only one of the two modules after a routine `go get -u`.

**Phase to address:** v0.2 (once both the boundary interceptor and the schema hook exist and need to agree) — add the CI cross-module version check at the same time the reference app's CI pipeline is built.

---

### Pitfall 9: Committing the `.binpb` `FileDescriptorSet` is deliberate and correct, but "hermetic" silently degrades to "stale" without an explicit CI staleness gate

**What goes wrong:**
The design commits the descriptor set specifically so `go generate` doesn't need to shell out to `buf` — this is a sound hermeticity argument. But the failure mode isn't "the descriptor set is missing," it's that a developer edits a `.proto` file, forgets (or their editor tooling doesn't remind them) to re-run `buf generate`, and commits a code change with a `.binpb` that no longer matches the `.proto` source. Nothing in the canonical pipeline as documented (`buf generate` → `go generate` → `atlas migrate diff`) *forces* step 1 before step 2 is committed — it only forces ordering *within a single execution*, not staleness detection across commits. `entc`/`mixinforproto` will happily read a stale descriptor and generate fields/validation for a message shape that no longer matches the actual `.proto`, and because the mixin gets its descriptor via `protoreflect` off the *Go-generated message type*, not off the committed `.binpb` directly (per `mixinforproto.md` §2 — "no `FileDescriptorSet` file" is used by the mixin itself), there are actually **two** independent sources of truth for "what does this proto look like" in play: the committed `.binpb` (read only by the entc extension for drift-check/whole-service concerns) and the compiled Go package's embedded descriptor (read by the mixin via protoreflect). These two can independently drift from each other and from the `.proto` source if `buf generate`'s Go-stub output and its `.binpb` output are regenerated inconsistently (e.g., a partial `buf generate` run, or a merge conflict resolved by hand in one artifact but not the other).

**Why it happens:**
Committed generated artifacts are the single most common source of "looks fine locally, breaks in CI or for a teammate" bugs in any codegen pipeline — merge conflicts in binary `.binpb` files are unresolvable by a human (binary diff), so the standard failure recovery ("just fix the conflict") doesn't work; a developer facing a `.binpb` merge conflict will often regenerate it against only their own branch tip, silently dropping the other branch's proto changes.

**How to avoid:**
- CI must run `buf generate` fresh and diff the result against the committed `.binpb` **and** the committed Go stub output, failing the build on any diff (the standard "generated code is checked in, so CI re-generates and diffs" pattern used by entgql/entproto's own golden-file precedent, extended to the descriptor set itself).
- Treat `.binpb` merge conflicts as a hard error, not something to hand-resolve: document that any merge/rebase touching `.binpb` requires re-running `buf generate` after the `.proto` conflict is resolved, never hand-merging the binary.
- Since the mixin and the entc extension read descriptors via two different mechanisms (protoreflect off the Go type vs. the committed `.binpb`), add an explicit CI or drift-check assertion that the two agree for every message the mixin consumes — this is a novel drift class specific to this design's two-path descriptor access and isn't automatically covered by "diff the binpb against buf generate output" alone if the Go stub and the `.binpb` are regenerated by the same `buf generate` invocation but a stale Go stub is manually reverted.
- Consider whether committing the `.binpb` is even necessary for the mixin's use case (it explicitly doesn't need it) versus only for entc's whole-service concerns (drift check, `google.api.http` annotation reading) — scoping the staleness-check surface to only where it's actually consumed narrows the blast radius.

**Warning signs:**
- A `.proto` field rename/removal that doesn't show up in generated ent fields until someone manually re-runs `buf generate`.
- CI passing on a PR that only touched `.proto` files but not the corresponding generated Go/`.binpb` diff.

**Phase to address:** v0.1/v0.2 — the CI regenerate-and-diff check should exist before any real schema depends on the descriptor set, i.e., alongside the very first reference-app proto.

---

### Pitfall 10: Codegen determinism — map iteration order and unstable formatting will make golden-file tests flaky exactly where this design is most novel

**What goes wrong:**
Go's map iteration order is deliberately randomized per-runtime-run (not just "unspecified" — the runtime actively randomizes it to prevent code from depending on it). Any part of `mixinforproto` or entconnect's entc extension that builds an intermediate `map[string]*Field` (e.g., collecting derived fields, or a field-name→proto-path table per Pitfall 7) and then ranges over it to emit ordered output (generated field lists, generated handler switch statements, drift-check error ordering, golden-file output) will produce nondeterministic output across runs unless explicitly sorted before emission. This is a well-known Go pitfall in general, but it bites *hardest* here because: (a) protoreflect's own field-iteration APIs (`ProtoReflect().Descriptor().Fields()`) are deterministically ordered by declaration order, which lulls developers into assuming downstream processing preserves that order, right up until they introduce a `map` for deduplication, override-lookup, or grouping; (b) golden-file tests (the explicitly adopted entgql/entproto precedent) will pass locally and then flake in CI or on a teammate's machine the moment map-ordered output is introduced, and the failure looks like "flaky CI" rather than "nondeterminism bug," burning debugging time on the wrong hypothesis.

**Why it happens:**
It's idiomatic and easy to reach for a `map[string]X` when doing name-based lookups (e.g., `Override(name, field)` resolution, `Exclude` checking) — the mixin's own `Option` mechanism is inherently name-keyed — and converting those maps back to slices for deterministic output is an easy step to forget.

**How to avoid:**
- Establish, as a project-wide rule from v0.1 onward: any code path that produces generated output or test-golden-comparable output must iterate proto/ent fields in declaration order (from `protoreflect`'s field list) or explicitly sort map keys before ranging — never range a `map` directly into emitted output.
- Add a CI step that runs golden-file tests (and, ideally, the full `go generate` pipeline) multiple times (e.g., `go test -count=5`) or with `GODEBUG=maphash=...`-style perturbation to catch order-dependent flakiness before it reaches contributors, following the same discipline entgql/entproto apply to their own golden tests.
- For the descriptor drift-check specifically, sort error/warning output by (file, message, field) tuple before returning it, so error ordering in CI logs is stable and diffable.

**Warning signs:**
- Golden-file test failures that don't reproduce locally, or that "fix themselves" on re-run.
- Generated file diffs that only reorder existing lines/fields with no semantic change.

**Phase to address:** v0.1 (establish the discipline in the mixin's own field-derivation code) and enforced again at v0.2 (entc extension's generated handler/drift-check output) — write the "no unsorted map ranging into generated output" rule into a lint rule or code-review checklist, not just docs.

---

### Pitfall 11: The deliberately-hard escape hatch (`entconnect.Manual("rpc")`) will get used as a routine tool, not a last resort, the moment teams hit any unmodeled RPC shape

**What goes wrong:**
The design's stated intent — "no silent hand-written-handler escape hatch... it should feel like the smell it is" — is a good instinct, but ent/entgql/entproto's own extension ecosystem history shows the actual failure mode of "deliberately hard escape hatches": teams don't stop needing custom behavior just because it's inconvenient to express, they instead either (a) reach for `Manual()` immediately and routinely (defeating the "smell" framing entirely once it's used on 30% of RPCs), or (b) route around the system entirely by hand-writing RPCs outside the generated service registration, silently reintroducing exactly the "handlers written by hand... are the classic leak point for auth, validation, and logic" problem the design exists to prevent — except now *invisibly*, since the drift check only knows about RPCs it can see in the same service definition, and a hand-rolled parallel Connect handler registered outside the generated wiring is invisible to drift-check entirely. `entoas`'s and `entproto`'s own extension surfaces (Templates, annotations, `WithTemplates`) exist precisely because "no escape hatch" pressure always finds a way out, and unofficial forks/wrappers (e.g., `entrest` positioning itself as a fuller-featured alternative to `entoas`+`ogent`) are the ecosystem's actual historical response to codegen tools whose default path is too rigid for the long tail of real handler needs (streaming, batch operations, non-CRUD custom RPCs, RPCs with side effects beyond a flow).

**Why it happens:**
CRUD + flow-binding covers the median RPC, not the tail — every real service ends up with a handful of RPCs that are neither pure CRUD nor a clean flow input (bulk import, a search RPC with app-specific ranking, a webhook receiver, an RPC that needs to touch two aggregates transactionally). The design correctly identifies `Manual()` as a smell but doesn't yet specify what happens when a team has, say, 15% of RPCs in that bucket — is that still "a smell" or is it "normal," and does the tooling (drift check severity, doc guidance, examples) treat it differently at different ratios?

**How to avoid:**
- Explicitly design `Manual("rpc")` to still be *visible* to drift check and to the generated server wiring (i.e., it must still register through the generated router/interceptor chain, just with a hand-written handler body) rather than allowing teams to bypass generated wiring entirely — this preserves the interceptor chain (authn, viewer injection, protovalidate, otel) even for manual RPCs, which is the actual safety property worth keeping, independent of whether the handler body is generated or hand-written.
- Track and surface `Manual()` usage ratio (e.g., in a generated summary comment or a lint warning past some threshold) so teams get a "this is becoming normal, not exceptional" signal rather than only a one-time build warning per RPC.
- Consider, for v0.4/v0.5, whether a more expressive declarative escape hatch (e.g., a documented pattern for "custom query/filter logic within an otherwise-generated List RPC") would absorb some of the long tail without falling back to fully-manual — reducing pressure on `Manual()` before it becomes the norm. This is explicitly out of scope for v0.1-v0.3 per the roadmap but should be tracked as a v0.4+ candidate rather than assumed away.

**Warning signs:**
- `Manual()` usage climbing steadily as a percentage of total RPCs across a real codebase's lifetime, rather than staying flat.
- Hand-written Connect handlers registered outside the generated server wiring entirely (visible via `grep` for manual `connect.NewXServiceHandler` calls outside generated files) — the actual failure mode the design is trying to prevent, happening anyway, invisibly to drift check.

**Phase to address:** v0.2 (Manual() must be designed to stay inside the generated interceptor chain from day one — retrofitting this after RPCs are already registered outside it is a breaking change) and monitored across all subsequent phases via the Manual-usage-ratio signal.

---

### Pitfall 12: Two-module repo (`mixinforproto`'s independent `go.mod` nested inside the `entconnect` repo) breaks the common "just go get it" workflow and CI matrix in specific, predictable ways

**What goes wrong:**
Go's tooling treats a nested `go.mod` as a module boundary, which is exactly the intended isolation — but it creates concrete, recurring operational friction: (1) `go build ./...` / `go test ./...` run from the repo root **silently skip** the nested `mixinforproto` module entirely (Go doesn't descend into nested modules), so a CI job that assumes `./...` covers the whole repo will pass green while `mixinforproto` has a compile error or failing test, unless CI explicitly `cd`s into each module or uses a `go.work` file; (2) local development that wants to test an entconnect change against an in-progress `mixinforproto` change needs either a `replace` directive (which must be remembered and removed before publishing — a routine source of "why is CI using my local uncommitted mixinforproto" or, inversely, "why isn't my local mixinforproto change picked up" bugs) or a `go.work` file (which solves local dev but must be explicitly excluded from what `go build` sees in module-consumer contexts, and can itself drift from the actual module graph if a module is added/removed and `go work use` isn't re-run); (3) versioning/tagging a nested module requires distinct, prefixed Git tags (`mixinforproto/v0.1.0` rather than `v0.1.0`) — a convention every contributor and every CI release job must know and apply consistently, and a common source of "I tagged a release but `go get` can't find it" support questions when the tag prefix is wrong or the module path in `go.mod` doesn't match the repo's declared import path.

**Why it happens:**
This is exactly the well-known Go multi-module-repo trap set — not specific to entconnect, but the design's module-isolation constraint (mixinforproto depends on nothing beyond ent/protobuf/protovalidate, must be independently adoptable) makes a nested-module layout the correct choice despite these costs, so the costs must be actively managed rather than assumed away by "it's just a subdirectory."

**How to avoid:**
- CI must explicitly enumerate both modules (e.g., a matrix job or a script that `cd`s into `mixinforproto/` and runs its own `go build`/`go test`/`go vet`, separately from the root module's `./...`), never assume `./...` from the repo root covers everything.
- Provide a checked-in `go.work` file for local development (with root `entconnect` and `mixinforproto` both `use`d) so contributors get correct cross-module resolution without hand-maintained `replace` directives, and document that `go.work` is dev-only, gitignored or not depending on team preference, but never required for `go build` to succeed for an external consumer.
- Document the release process explicitly: releasing a `mixinforproto` change requires tagging `mixinforproto/vX.Y.Z`; releasing an `entconnect` change requires tagging `vX.Y.Z` at the root; a change spanning both requires two tags, and if `entconnect`'s own `go.mod` depends on `mixinforproto`, that dependency version must be bumped in a *separate* commit/PR referencing the just-tagged `mixinforproto` version (never a same-commit self-reference, which can't resolve).
- If `entconnect`'s entc extension or runtime package needs to depend on `mixinforproto` at all (worth checking against the design's stated dependency graph — currently `mixinforproto` has no listed consumer within entconnect itself, since the entc extension operates on the descriptor set and schema graph independently), that's an additional cross-module dependency to manage under the same tagging discipline; if it's *not* actually needed, keep it that way deliberately, since every additional intra-repo cross-module dependency compounds this pitfall.

**Warning signs:**
- A CI run that's green despite a compile error inside `mixinforproto/`.
- A `go.mod` `replace` directive accidentally left in place in a merged PR (visible via `go.mod` diff review — treat any `replace ./mixinforproto` line in a PR touching non-dev files as a blocking review comment).
- `go get github.com/.../entconnect/mixinforproto@vX.Y.Z` failing because the tag was pushed without the `mixinforproto/` prefix.

**Phase to address:** v0.1 (module scaffolding) — get the `go.work` + CI-matrix + tagging convention right before any external consumer tries to `go get` the module; retrofitting tagging discipline after a wrong tag has been pushed is messy (Go module proxy caching means a bad tag can be effectively permanent).

---

### Pitfall 13: Building `entflow` flow-binding on a metadata surface that doesn't exist yet couples the roadmap's v0.3 to an external, independently-developed project's design churn

**What goes wrong:**
`entconnect`'s v0.3 milestone ("Flow binding") depends on entflow exposing "a small metadata interface (flow name, input/output types, sync/async nature)" that, per the project context, does not yet exist as shipped code — it's a design intent recorded in both docs but entflow is "a separate project, separate repo, separate design doc." This is a real, common failure mode in multi-project ecosystems (not unique to this pair, but concretely risky here): (a) if entflow's actual metadata surface, once built, differs even slightly from what entconnect's v0.3 design assumes (e.g., the "sync/async nature" signal turns out to need a third state, or flow input/output types are expressed as a registry lookup rather than a Go type parameter), entconnect's flow-binding code and drift-check logic (§3.4's "every flow whose input is a proto message must have a claiming RPC") must be reworked against a moving target; (b) because the coupling is described as "confined to a small metadata interface, ideally duck-typed or a tiny shared `entflow/meta` package," there is a real risk that this shared package itself becomes a third, informally-versioned artifact that both projects depend on without either project owning its release cadence — a classic "who owns the interface" gap; (c) the async-flow response design (`run_id` + generated `GetRunStatus` RPC, Open Question 3) requires entflow to expose a run-status query surface that doesn't yet exist either, compounding the coupling beyond just "flow name/input/output/sync-async" into a second not-yet-designed surface (run entity shape, status enum, polling vs. streaming).

**Why it happens:**
Sequencing a roadmap where "the workflow half" and "the contract half" are split for good architectural reasons (each independently releasable and adoptable) inherently means one side's v0.3 either waits on the other's readiness or is designed speculatively against an interface that hasn't been proven by a real consumer yet — speculative interface design is the single most common source of "the abstraction was wrong" rework in software.

**How to avoid:**
- Treat the `entflow/meta` package (or duck-typed interface) as its own tiny, versioned, independently-tagged artifact from day one — even if it initially lives in the entconnect repo as a placeholder, give it explicit ownership and a change-review process that both projects' maintainers (even if that's the same person, Shahar Mintz, wearing two hats) sign off on before entconnect's v0.3 code depends on it.
- Sequence v0.3 to start only after entflow has shipped *some* real, working sync-flow implementation (even a v0.1 with no async support) that entconnect can integration-test against — do not build entconnect's flow-binding purely against entflow's design doc; build it against running code, even minimal running code, to catch interface mismatches before they're baked into entconnect's codegen.
- Explicitly scope v0.3's async-flow / `GetRunStatus` piece (Open Question 3) as a *second*, later sub-milestone gated on entflow's run-status surface existing and being stable, separate from the sync-flow-binding sub-milestone — don't let "flow binding" as a single v0.3 phase silently block on the harder, less-designed half of the problem.
- Write entconnect's own conformance/integration tests against a minimal fake/mock implementation of the `entflow/meta` interface first, so entconnect's own test suite doesn't require entflow's actual codebase to be present or building — this is also what the "entconnect must be fully useful with zero flows" constraint already implies, but make the test infrastructure enforce it explicitly (a fake meta-interface implementation living in entconnect's own test fixtures, not just a design intent).

**Warning signs:**
- entconnect's v0.3 branch/work stalling waiting on entflow decisions that are themselves unresolved open questions in entflow's own design doc.
- Interface changes in `entflow/meta` requiring simultaneous, coupled PRs in both repos (a sign the "duck-typed, tiny interface" boundary has already been breached by tighter coupling than intended).

**Phase to address:** v0.3 (Flow binding) — but the *sequencing decision* (don't start until entflow has real running code) belongs in roadmap planning now, before v0.3 is scheduled, not discovered mid-phase.

---

## Technical Debt Patterns

| Shortcut | Immediate Benefit | Long-term Cost | When Acceptable |
|----------|-------------------|-----------------|------------------|
| Skip field-mask enforcement in generated Update handlers for MVP, set every present field | Faster v0.2 ship, simpler handler codegen | Silent data-clearing bugs in production (Pitfall 4) — high-severity, hard to detect after the fact | Never — this is the design's declared sharpest edge; do not defer past v0.2 |
| Reuse Go's default panic formatting instead of building custom, context-rich panic messages in `mixinforproto` | Less code to write in v0.1 | Undebuggable `go generate` failures (Pitfall 1), directly undermining the "fail fast, panic at schema load" value proposition | Never for `Exclude`/`Override`/`oneof` panics; acceptable only for truly-should-never-happen internal invariant panics |
| Let `Manual("rpc")` register handlers outside the generated interceptor chain to unblock an urgent custom RPC | Ships the custom RPC quickly | Reintroduces the exact "handlers leak auth/validation" problem the whole design exists to prevent, invisibly to drift check (Pitfall 11) | Never — if `Manual()` can't stay inside the generated chain, that's a codegen bug to fix, not a workaround to accept |
| Commit `.binpb` without a CI regenerate-and-diff check for the first few milestones ("we'll add it later") | Saves initial CI setup time | Stale descriptors silently drive codegen for months before anyone notices (Pitfall 9) | Acceptable only for a throwaway spike/prototype, never for the reference app or any adopter-facing release |
| Use a `map[string]*Field` internally in the mixin/entc extension and range it directly into generated output | Simpler code | Flaky golden-file tests, hard-to-diagnose CI nondeterminism (Pitfall 10) | Never in code paths that produce generated or golden-compared output |
| Pin `mixinforproto` and `entconnect/runtime` to whatever `protovalidate-go`/`cel-go` versions `go mod tidy` picks independently, without a cross-module version check | No extra CI step to write | Silent validation-behavior divergence between boundary and schema layers after routine dependency bumps (Pitfall 8) | Acceptable only while `mixinforproto` has zero real (non-reference-app) adopters; must be fixed before any external release |

## Integration Gotchas

| Integration | Common Mistake | Correct Approach |
|-------------|-----------------|-------------------|
| `buf generate` → committed `.binpb` + Go stubs | Assuming the two outputs (binary descriptor set, Go package) stay in sync because they came from the same `buf generate` invocation once, without re-verifying on every subsequent change | CI regenerates both from `.proto` sources and diffs against committed artifacts on every PR (Pitfall 9) |
| `protovalidate-go` boundary interceptor + `mixinforproto`'s CEL hook | Assuming "same protovalidate source" (the `.proto` constraints) implies "same validation behavior" without pinning evaluator (`cel-go`) versions across the two independent modules | Explicit CI cross-module dependency version check (Pitfall 8) |
| ent mixin `Hooks()`/`Policy()` composition with schema-declared `Hooks()`/`Policy()` | Assuming mixin-declared and schema-declared hooks/policies are order-independent or that schema authors will discover the "mixin runs first" ordering on their own | Document ordering explicitly wherever `MixinForProto`'s Tier 2 hook is introduced; test it (Pitfall 3) |
| `entc`'s subprocess-based schema loading + `mixinforproto`'s intentional panics | Assuming a panic message reaching a subprocess boundary preserves the same debuggability as an in-process panic | Custom, context-rich panic messages plus a documented in-process debug entry point (Pitfall 1) |
| `entflow` metadata interface | Building entconnect's v0.3 flow-binding purely against entflow's design doc rather than running code | Sequence v0.3 to start after entflow has shipped a minimal working sync-flow implementation; use a fake/mock meta-interface for entconnect's own tests until then (Pitfall 13) |
| Nested Go module (`mixinforproto/go.mod`) | Assuming `go build ./...`/`go test ./...` from the repo root exercises the nested module | Explicit CI matrix entry per module; checked-in `go.work` for local dev (Pitfall 12) |

## Performance Traps

| Trap | Symptoms | Prevention | When It Breaks |
|------|----------|------------|-----------------|
| Constructing a fresh `protovalidate.Validator` (triggering CEL compilation) per request in the boundary interceptor instead of once per process | Create/Update RPC p99 latency orders of magnitude above Get/List, CPU profile dominated by `cel.NewProgram`/`cel.NewEnv` | Singleton validator constructed at server startup, captured by the interceptor closure; add a p99 benchmark to CI (Pitfall 6) | Immediately, at any request volume — this isn't a scale threshold, it's a constant-factor bug that shows up on request #1 |
| Naive field-mask-free Update handlers doing full-message re-validation (Tier 3-style) on every partial update, even when only masked fields changed | Update RPC latency scaling with entity size/field count regardless of how few fields actually changed | Tier 2's "changed-fields-only" evaluation, properly mask-gated (Pitfall 4/5), keeps per-request CEL evaluation cost proportional to fields actually touched, not entity size | Noticeable once entities grow past a handful of validated fields per message; compounds with Pitfall 4's correctness bug, not just a performance one |
| Descriptor drift-check re-parsing the full committed `.binpb` FileDescriptorSet on every `entc generate` invocation without caching across a single run when both entproto-style mixin derivation and drift-check need the same descriptors | Slower-than-necessary `go generate ./...` on large schemas/services as the descriptor set grows | Load and parse the `.binpb` once per `entc` run, share the parsed `protoreflect.FileDescriptor` set across the mixin-derivation and drift-check codegen phases within that run | Becomes noticeable only once the proto surface is large (dozens of services/messages); low priority for MVP but cheap to get right from the start |

## Security Mistakes

| Mistake | Risk | Prevention |
|---------|------|------------|
| Generated server wiring accidentally exposing a privileged (privacy-bypassing) ent client to application/handler code, e.g., via a shared package-level variable or a convenience constructor left in for tests | Total privacy-policy bypass — the exact failure the design explicitly calls out as unacceptable ("never exposes a privileged client to application code") | Keep the privileged client construction scoped entirely inside generated server-wiring code with no exported accessor; test this via a reference-app integration test asserting privacy denials surface as `PermissionDenied` even when called from application-adjacent code paths, not just from the generated handler itself |
| `Manual("rpc")` handlers bypassing the generated interceptor chain (viewer injection, protovalidate) because they're registered outside generated wiring | Auth/validation gaps on exactly the RPCs a team is most likely to consider "special" or "trusted" (Pitfall 11) | `Manual()` must register through the same generated interceptor chain; drift check should flag any service handler registration found outside the generated router |
| Schema-layer CEL validation errors leaking internal field names or ent-specific details (that differ from the proto-facing field names) into client-visible error messages for non-RPC-originated mutations that get surfaced to an API consumer indirectly (e.g., via a flow's error propagation) | Information disclosure of internal schema structure through error messages | Structured error type (Pitfall 7) carries the original proto field path explicitly, decoupled from ent's internal field naming, so any surface that renders the error to a client never leaks ent-internal names |
| Sensitive proto fields (`debug_redact`, or fields intended for `field.Sensitive()`) not actually getting `Sensitive()` treatment because `mixinforproto` has no proto-standard marker for it yet (Open Question 2 in `mixinforproto.md`, unresolved) | Sensitive data (tokens, secrets) logged or exposed in ent debug output / error messages / query logs | Resolve Open Question 2 before any schema with genuinely sensitive fields ships; until resolved, require explicit `Override` with `field.Sensitive()` for any such field rather than relying on automatic derivation |

## UX Pitfalls

| Pitfall | User Impact | Better Approach |
|---------|--------------|-------------------|
| `go generate` failing with an opaque `exit status 2` and no actionable message | Developer loses significant time bisecting which schema/mixin caused the failure | Context-rich custom panics plus an in-process debug entry point (Pitfall 1) |
| A field silently reset to zero/empty after a partial update, discovered only much later (e.g., in a downstream analytics job) | Data-quality erosion that's expensive to detect and worse to fix retroactively (can't tell which historical updates were "real" zeros vs. accidental clears) | Mandatory field-mask-gated Update handlers, tested for partial-update fidelity (Pitfall 4) |
| `Manual("rpc")` framed as "the smell it is" but with no tooling nudging teams away from routine use | Teams either avoid the escape hatch and get stuck, or use it routinely and the "smell" framing loses all signal value | Usage-ratio tracking/surfacing, and keeping `Manual()` inside the generated interceptor chain so at least the safety properties survive routine use (Pitfall 11) |
| Schema authors surprised that their own hand-written `Hooks()`/`Policy()` run *after* the mixin's validation hook | Confusing "my hook didn't see what I expected" bugs, discovered only by reading ent's own composition-order docs, not entconnect's | Explicit documentation and a conformance test demonstrating the ordering (Pitfall 3) |

## "Looks Done But Isn't" Checklist

- [ ] **Field mapping from proto to ent (`MixinForProto` v0.1):** Often missing the reserved-word/collision check against ent's own generated identifiers and other mixins' fields — verify by running the conformance corpus against a proto message with a field literally named `label`, `type`, or `id`.
- [ ] **Update RPC handlers (v0.2):** Often missing field-mask gating entirely, silently clearing untouched fields to zero — verify with a partial-update fidelity test asserting the ent mutation's changed-field set exactly equals the request's field mask, not "every non-optional field."
- [ ] **Tier 2 CEL validation hook (v0.2):** Often missing the "explicit clear to zero via mask still validates" case — verify by testing a `min_len`/`required` rule against an explicit-zero-via-mask update, not just an omitted field.
- [ ] **Descriptor set staleness (any phase touching `.proto`):** Often missing a CI regenerate-and-diff check for the committed `.binpb` — verify CI fails when a `.proto` change is committed without a corresponding `buf generate` re-run.
- [ ] **Golden-file / drift-check determinism (v0.1+):** Often missing sorted iteration over internal maps before emitting generated or diffable output — verify by running golden tests with `-count=5` or across multiple CI runs and confirming zero flakes.
- [ ] **Boundary interceptor construction (v0.2):** Often missing "constructed once at startup, not per-request" — verify via a benchmark asserting flat p99 latency across repeated Create/Update calls, not just a functional golden test.
- [ ] **`Manual("rpc")` escape hatch (v0.2):** Often missing enforcement that manual handlers still pass through the generated interceptor chain — verify by asserting a `Manual()`-backed RPC still enforces viewer injection and protovalidate.
- [ ] **Cross-module dependency hygiene (`mixinforproto` + `entconnect`, v0.1+):** Often missing a CI check that both modules resolve compatible `protovalidate-go`/`cel-go` versions — verify by asserting both `go.sum` files pin compatible CEL environment versions.
- [ ] **Non-RPC mutation error handling (flows/workers/seeds, v0.2+):** Often missing a structured, RPC-independent error type for schema-layer validation failures — verify a raw `client.Order.Update()` call from a test produces an error with the constraint ID and proto field path, without needing an RPC context.

## Recovery Strategies

| Pitfall | Recovery Cost | Recovery Steps |
|---------|-----------------|-------------------|
| Silent data-clearing from unmasked partial updates already in production (Pitfall 4) | HIGH | Audit historical update events/logs to distinguish real zeros from accidental clears (often impossible retroactively without an audit log); backfill from any available source of truth; ship field-mask enforcement immediately and add a migration-period warning log for any Update request lacking a mask, before hard-erroring |
| Stale `.binpb`/Go stubs discovered after merge (Pitfall 9) | LOW–MEDIUM | Re-run `buf generate` from the current `.proto` sources, commit the regenerated artifacts as their own PR, re-run the full `go generate ./...` pipeline to catch any downstream codegen that silently depended on the stale shape |
| Nondeterministic golden-file flakiness traced to unsorted map iteration (Pitfall 10) | LOW | Sort the offending map's keys before ranging; regenerate golden files once with the fix; add the `-count=5` CI guard going forward |
| Version-skew validation divergence between boundary and schema layers discovered via a production bug report (Pitfall 8) | MEDIUM | Pin both modules' `protovalidate-go`/`cel-go` to matching versions immediately; add the CI cross-module check retroactively; audit recent dependency-bump commits in both modules for the divergence window to assess blast radius |
| `Manual()` handlers found registered outside the generated interceptor chain, bypassing auth/validation (Pitfall 11) | HIGH | Treat as a security incident, not routine cleanup — audit what those RPCs actually did while ungoverned, then migrate them to the in-chain `Manual()` pattern (or full codegen) before considering it resolved |
| entflow metadata interface changed in a way that breaks entconnect's v0.3 flow-binding assumptions (Pitfall 13) | MEDIUM–HIGH | Isolate the interface mismatch to the shared `entflow/meta` package; version that package explicitly going forward; add a fake/mock implementation to entconnect's test suite so future entflow changes are caught by entconnect's own CI before integration, not after |

## Pitfall-to-Phase Mapping

| Pitfall | Prevention Phase | Verification |
|---------|-------------------|----------------|
| Undebuggable schema-load panics | v0.1 (MixinForProto) | Every panic path in the load-time failure test suite asserts a specific, context-rich message, not just "panics" |
| Field-name collisions (mixin vs. hand-declared vs. reserved words) | v0.1 (MixinForProto) | Conformance corpus includes a message with fields named `label`/`type`/`id`; asserts an early, well-messaged failure |
| Mixin `Hooks()`/`Policy()` ordering surprises | v0.2 (Tier 2 CEL hook) | Conformance test with a hand-written schema hook, asserting documented (not silently "fixed") ordering |
| proto3 presence → `Default(zero)` breaking partial updates | v0.2 (CRUD handlers / Update RPC codegen) — field-mapping decision alone (v0.1) is necessary but insufficient | Differential/partial-update fidelity test: ent mutation's changed-field set exactly matches the request's field mask across representative Update calls |
| Field-mask interaction with Tier 2 "changed fields only" | v0.2 (Tier 2 hook + differential harness) | Explicit-clear-to-zero-via-mask still triggers validation; `oneof`-across-Update gap explicitly documented, not silently assumed safe |
| CEL compilation cost mismanagement (per-request re-compilation) | v0.2 (boundary interceptor construction) | p99 latency benchmark in CI, not just a functional golden test |
| Error shape mismatch between boundary and schema layers | v0.2 (Tier 2 hook + transport error translation) | Same violation via RPC handler and raw ent client call produces matching constraint ID + field path |
| protovalidate/CEL version skew across modules | v0.2 (once both interceptor and hook exist) | CI cross-module dependency-version check |
| Descriptor set staleness (`.binpb` + Go stubs) | v0.1/v0.2 (as soon as the reference app has a real proto) | CI regenerate-and-diff check on every PR touching `.proto` files |
| Codegen nondeterminism (map iteration, unsorted output) | v0.1 (mixin field derivation) and v0.2 (entc extension output) | Golden tests run with repeated/perturbed execution (`-count=5`) show zero flakes |
| `Manual("rpc")` escape-hatch overuse / bypassing generated chain | v0.2 (Manual() design) | Manual()-backed RPC still enforces the full generated interceptor chain; usage-ratio signal tracked across later phases |
| Two-module repo operational traps (CI scope, tagging, replace directives) | v0.1 (module scaffolding) | CI explicitly covers both modules; `go.work` checked in for dev; release docs specify dual-tagging |
| Coupling to not-yet-existing entflow metadata surface | v0.3 (Flow binding) — sequencing decision made during roadmap planning, before v0.3 starts | entconnect's own tests use a fake/mock `entflow/meta` implementation; v0.3 doesn't start against entflow's design doc alone, only against running entflow code |

## Sources

- [ent/ent#280 — Name collision in generated code](https://github.com/ent/ent/issues/280) (primary source, fetched directly — MEDIUM confidence)
- [ent/ent#474 — Panic during generation](https://github.com/ent/ent/issues/474) (primary source, fetched directly — MEDIUM confidence)
- [ent/ent#2329 — Schema Hook cannot generate: import cycle not allowed](https://github.com/ent/ent/issues/2329) (search-aggregated — LOW confidence)
- [ent/ent#4183 — entoas generate failed](https://github.com/ent/ent/issues/4183) (search-aggregated — LOW confidence)
- [ent/ent#4109 — entoas.Groups annotation does not work as expected](https://github.com/ent/ent/issues/4109) (search-aggregated — LOW confidence)
- [ent Mixin documentation](https://entgo.io/docs/schema-mixin/) — mixin/schema hook and policy composition order (official docs — MEDIUM/HIGH confidence)
- [Protocol Buffers — Field Presence application note](https://protobuf.dev/programming-guides/field_presence/) (official docs — HIGH confidence)
- [protobuf field_presence.md, implementing_proto3_presence.md](https://github.com/protocolbuffers/protobuf/blob/main/docs/field_presence.md) (official source — HIGH confidence)
- [grpc-gateway#2566 — required fields not optional in partial update operations](https://github.com/grpc-ecosystem/grpc-gateway/issues/2566) (search-aggregated — LOW confidence)
- [grpc-gateway#1930 — Patch request with field_masks](https://github.com/grpc-ecosystem/grpc-gateway/issues/1930) (search-aggregated — LOW confidence)
- [gRPC-Gateway Patch feature docs](https://grpc-ecosystem.github.io/grpc-gateway/docs/mapping/patch_feature/) (official docs — MEDIUM/HIGH confidence)
- [protovalidate-go (bufbuild)](https://github.com/bufbuild/protovalidate-go) and [How Protovalidate uses CEL](https://protovalidate.com/cel/how-cel-works/) (official docs — HIGH confidence, compile-once/cache-program guidance)
- [cel-go (google/cel-go)](https://github.com/google/cel-go) and [cel-go pkg.go.dev](https://pkg.go.dev/github.com/google/cel-go/cel) (official docs — HIGH confidence)
- [protovalidate-java#118 — Expression compilation failure for cross-module imported fields](https://github.com/bufbuild/protovalidate-java/issues/118) (primary source, analogous failure class — MEDIUM confidence, Java not Go but same CEL/protovalidate architecture)
- [Buf breaking change detection docs](https://buf.build/docs/breaking/) and [Buf descriptors reference](https://buf.build/docs/reference/descriptors/) (official docs — HIGH confidence)
- [Atlas — Detect Migrations Drift in CI](https://atlasgo.io/faq/desired-state-drift) and [Pre-Apply Drift Detection](https://atlasgo.io/changelog/pre-apply-drift-detection) (official docs — HIGH confidence)
- [Go map iteration nondeterminism — golang/go#18325](https://github.com/golang/go/issues/18325) (primary source — MEDIUM confidence)
- [Non-Determinism of Maps in Golang](https://maxwelldulin.com/BlogPost/Golang-Map-Non-Determinism) (search-aggregated — LOW confidence, consistent with golang/go#18325)
- [ent Extensions documentation](https://entgo.io/docs/extensions/) — Templates/annotation-based escape hatches in entgql/entproto/entoas (official docs — MEDIUM/HIGH confidence)
- [entrest](https://github.com/lrstanley/entrest) positioning itself as a fuller-featured alternative to entoas+ogent (ecosystem evidence of "rigid codegen tools spawn forks/alternatives" pattern — MEDIUM confidence)
- [Go Workspaces / go.work documentation pattern](https://oneuptime.com/blog/post/2026-01-25-multi-module-go-projects-workspaces/view) and general Go multi-module/replace-directive guidance (search-aggregated — LOW confidence, but consistent with well-documented Go tooling behavior)
- Project design docs analyzed directly: `entconnect.md`, `mixinforproto.md`, `PROJECT.md` (primary source — the project's own stated open questions, e.g., Open Question 5 on field masks and Open Question 1/2 in `mixinforproto.md`, directly informed Pitfalls 4, 5, and the Security Mistakes table)

---
*Pitfalls research for: protobuf-descriptor-driven code generation and runtime mixins in the Go/ent ecosystem (entconnect + mixinforproto)*
*Researched: 2026-08-08*
