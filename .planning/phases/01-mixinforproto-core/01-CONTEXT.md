# Phase 1: MixinForProto Core - Context

**Gathered:** 2026-08-08
**Status:** Ready for planning
**Mode:** `--auto` — all gray areas auto-resolved to the recommended option. Every decision below is a default, not a user-stated preference; a reviewer may overturn any of them before planning.

<domain>
## Phase Boundary

Phase 1 delivers **`mixinforproto`**: a standalone, codegen-free ent mixin module that turns a generated protobuf message *type* into `[]ent.Field` at schema-load time, records enough provenance as ent annotations to survive entc's JSON boundary, translates the protovalidate constraints that have native ent equivalents into real builder calls, and ships as an independently buildable/taggable Go module with the two-module pipeline and CI scaffold around it.

**In scope:** MIX-01…MIX-14, ANNO-01…ANNO-04, VAL-01…VAL-03, PIPE-01…PIPE-04, PIPE-08.

**Explicitly NOT in this phase** (they belong to later phases and must not be pulled forward):
- Tier 2 CEL passthrough and the mutation-time hook — Phase 3 (VAL-04…VAL-11, PIPE-05, PIPE-06)
- Any entc extension, handler generation, or interceptor chain — Phase 2
- Any drift checking — Phase 5 (and a non-goal of `mixinforproto` permanently, per `mixinforproto.md` §3)
- Flow binding — Phase 4
- The Order/Inventory reference application — Phase 5

</domain>

<decisions>
## Implementation Decisions

### Annotation Contract (ANNO-01…ANNO-04)

- **D-01:** The annotation struct types live **in `mixinforproto`**, not in the root module. This is the one part of `mixinforproto`'s public API a different module is expected to import and decode. It does not violate module isolation — that constraint bounds what `mixinforproto` *imports*, not who imports it. — **Reversibility:** costly — moving them later breaks the import path for every consumer that decodes annotations, and the Phase 2 entc extension will already depend on it.

- **D-02:** Two annotation types, both plain serializable structs (no closures, no `cel.Program`, nothing that cannot survive `map[string]any` + `mapstructure`):
  - **Schema-level `SourceMessage`** — proto message full name, the **complete inventory** of proto fields present in the descriptor (name + number), plus `Excluded []string` and `Overridden []string`. The full inventory is load-bearing: without it a later drift check cannot distinguish "deliberately excluded" from "silently forgotten", because both look identical as absence from `gen.Graph`.
  - **Field-level `SourceField`** — source field name and number, derivation kind (scalar / enum / WKT / map-as-JSON / message-as-JSON / override), the protovalidate constraint IDs translated into Tier 1 builders, and the residual (untranslated) constraint IDs plus a stable fingerprint of their CEL expression *strings*.
  — **Reversibility:** one-way — the drift check (Phase 5) and the Phase 2 extension both read this shape; changing it after `mixinforproto` is tagged and consumed means a coordinated cross-module version bump, which is exactly the failure mode ANNO-04 exists to detect.

- **D-03:** **ANNO-04 versioning:** a single integer `ContractVersion` constant, embedded as a field on *both* annotation structs, bumped **only** on a breaking layout change. Between bumps the policy is **additive-only** — new fields may be added, existing fields are never renamed or repurposed, and decoding tolerates unknown keys. A consumer reading a `ContractVersion` higher than the one it was compiled against must fail with a named mismatch error ("mixin contract v3, extension understands v2 — upgrade entconnect"), not silently decode a partial struct. Chosen over semver strings and per-field markers because the only question a consumer actually needs answered is "can I decode this?", and an integer answers it without parsing.
  — **Reversibility:** costly — the version scheme itself becomes part of the contract the moment anything decodes it.

- **D-04:** Annotation keys are **exported string constants** in `mixinforproto` (`MixinForProtoMessage`, `MixinForProtoField`), mirroring `entproto`'s `MessageAnnotation = "ProtoMessage"` precedent, so downstream decoders reference the constant rather than re-typing the literal.

- **D-05:** `Override(name, f)` records the *name* in `SourceMessage.Overridden` only. The replacement `ent.Field` itself needs no annotation channel — it becomes a normal derived field the extension already sees directly in `gen.Graph`. Consistent with `mixinforproto.md` §2: an override suppresses validation relay for that field entirely, so it carries no `SourceField` constraint data.

### Failure Surface & Debuggability (MIX-11, MIX-12)

- **D-06:** Split the implementation into a **pure `derive` core returning `(*derivation, error)`** and a thin `ent.Mixin` wrapper whose `Fields()` calls it and panics on error. Every failure path is an ordinary Go error internally; panicking is a single, isolated adapter concern. This is what makes MIX-12 nearly free rather than a parallel code path.
  — **Reversibility:** costly — retrofitting error returns through an already-panicking derivation touches every mapping path.

- **D-07:** **MIX-12's in-process debug entry point is an exported `Validate[M proto.Message](opts ...Option) error`** in the same package, running the identical `derive` core and returning the structured error. No separate `doctor` binary in this phase — a function the user can call from a plain `go test` in their own schema package gives a full, untruncated stack trace and costs one exported symbol.

- **D-08:** **Every panic message's first line must be self-sufficient**, because `entc/load` runs schema loading in a subprocess whose panic output is routinely truncated to one line (PITFALLS Pitfall 1 — a filed ent issue, not a hypothetical). Format: `mixinforproto: <ProtoMessageName>.<field>: <what went wrong> — <the fix>`, with any additional detail on subsequent lines. Never rely on Go's default panic formatting for `Exclude`/`Override`/`oneof`/reserved-name failures.

- **D-09:** Failures are **collected and reported together**, not fired at the first offense. A message with three unknown `Exclude` names should report all three in one panic, so the fix is one edit rather than three regeneration cycles. The first line still names the count and the first offender so the truncated case stays useful.

### Derived-Name Collisions (Pitfall 2)

- **D-10:** At derivation time, check every derived field name against ent's known reserved identifiers (`id`, `type`, `label`, `edge`, `where`, `config`, `client`, and the rest of the catalog-known set) and **panic at schema load** with the `Exclude`/`Override` remedy. Collisions against fields hand-declared in the *same schema's* `Fields()` or contributed by another mixin are outside the mixin's reach — ent composes mixins before the schema's own `Fields()` runs — so those are **documented loudly** and left to surface as the Go compiler's `redeclared in this block` error in generated code. Do not attempt to auto-rename: a silently renamed API-visible field defeats the entire point of the contract being the source of truth.

### Tier 1 Translation Boundary (VAL-01, VAL-02, VAL-03)

- **D-11:** Tier 2 does not exist until Phase 3, so **constraints Tier 1 cannot translate are recorded, not enforced and not rejected**. Their IDs and a stable fingerprint of their CEL expression strings go into `SourceField`; the schema layer does not check them in this phase. Documented explicitly as "boundary-only until Phase 3" so nobody mistakes Phase 1's storage layer for a complete guarantee. Panicking on untranslatable constraints would make the mixin unusable against any real contract; ignoring them silently would lose the data Phase 3 and Phase 5 both need.

- **D-12:** **Open/closed interval adjustment (VAL-02):** integer types adjust exactly — `gt: n` → `Min(n+1)`, `lt: n` → `Max(n-1)` — because the adjustment is lossless over the integers. Floating-point `gt`/`lt` are **treated as residual**, not widened to `gte`/`lte`. Widening would make the schema layer accept a value protovalidate rejects, and Phase 3's differential harness (PIPE-06) exists precisely to catch that class of disagreement — better to record it as residual now than to bake in a known-wrong verdict.

- **D-13:** **String format validators (VAL-01's `uuid`/`email`/`hostname`/`uri`/`ip`):** where protovalidate's own semantics *are* a documented regular expression, emit `Match(regexp.MustCompile(...))` with that same expression. Where protovalidate implements the check procedurally, emit `field.String().Validate(fn)` delegating to protovalidate's own predicate rather than a hand-written approximation. Both are native ent builder calls, satisfying VAL-01; the delegating form trades some ecosystem introspectability for verdict identity, which is the property the whole design rests on. Note the introspectability caveat in the docs. `field.String` is one of the few builders exposing a typed `.Validate()` — this works only because format validators are string-only.
  — **Reversibility:** reversible — per-format, and each is independently swappable to the other form.

- **D-14:** `enum.defined_only` plus the enum's declared values is satisfied by the `field.Enum` construction itself (per `mixinforproto.md` §4.1) and is not separately recorded as a residual constraint.

### Two-Module Scaffold & Pipeline (MIX-14, PIPE-01…PIPE-04)

- **D-15:** **`go.work` is checked in**, listing the root module and `mixinforproto/`, so contributors get correct cross-module resolution without hand-maintained `replace` directives. No `replace` directive ever lands in a committed `go.mod` (PIPE-04). A `replace ./mixinforproto` line appearing in a PR diff is a blocking review comment.

- **D-16:** **CI enumerates modules explicitly** via a checked-in list (`MODULES := . ./mixinforproto`) in the build script/Makefile driving `build`/`vet`/`test` per module. Never `./...` from the repo root — Go does not descend into nested modules, so a root-only job goes green while `mixinforproto` is broken (PITFALLS Pitfall 12). Discovery-by-`find` is rejected: an explicit list fails loudly when a module is added and the list is not updated, which is the desired behavior.

- **D-17:** **A dedicated `GOWORK=off` CI job builds and tests `mixinforproto` alone** (PIPE-02), proving standalone consumption without workspace resolution papering over a missing dependency.

- **D-18:** **The root `entconnect` module does not import `mixinforproto` in Phase 1.** Phase 1 creates the root `go.mod` and the CI/pipeline scaffold, but the annotation-decoding dependency edge lands in Phase 2 when the entc extension actually exists. Keeps the cross-module tagging discipline (PIPE-04) from mattering before there is anything to tag against.
  — **Reversibility:** reversible — adding the edge in Phase 2 is a one-line `go.mod` change plus a version bump.

- **D-19:** **Release tagging:** `mixinforproto` releases under `mixinforproto/vX.Y.Z`; the root module under `vX.Y.Z`. A change spanning both requires two tags, and any root-module dependency bump onto a new `mixinforproto` version lands in a *separate* commit referencing the already-pushed tag — never a same-commit self-reference, which cannot resolve. Document this in the release notes/CONTRIBUTING before the first tag is pushed; Go module proxy caching makes a wrong tag effectively permanent.
  — **Reversibility:** one-way once a tag is published — the module proxy caches it and it cannot be meaningfully retracted.

- **D-20:** **Pipeline script (PIPE-01)** runs, in order: `buf lint`, `buf generate`, the **separate** `buf build -o <path> --as-file-descriptor-set --exclude-source-info` invocation, `go generate ./...`, `atlas migrate diff`. The descriptor-set step is a distinct CLI call, not a `buf.gen.yaml` plugin entry — there is no first-party plugin path to it (ARCHITECTURE Anti-Pattern 4). In Phase 1 the `go generate` and `atlas` steps have nothing real to act on yet; the script exists and is exercised, and later phases fill it in.

### Test Corpus & Determinism (MIX-13, groundwork for PIPE-05)

- **D-21:** **Corpus protos are synthetic** — small `testdata` proto files, one per mapping-rule class (scalars, enum, each WKT, optional vs non-optional presence, scalar map, message map, message field, oneof, and one file per protovalidate constraint class). A failure then names the rule that broke. The Order/Inventory contract is a Phase 5 concern and is not borrowed here.

- **D-22:** **Generated Go stubs for the corpus are committed**, so `go test ./mixinforproto/...` requires no `buf` on the machine — which is what makes `mixinforproto` genuinely standalone-testable. A separate CI job regenerates and diffs them, applying Pitfall 9's staleness discipline from the first proto rather than retrofitting it.

- **D-23:** **Golden assertion shape:** marshal each derived field's `Descriptor()` into a stable, key-sorted document and compare via `goldie` (`-update` regenerates), following the entgql/entproto golden-file precedent. Comparing `ent.Field` values structurally is not viable — the interesting state is behind the descriptor and some of it is closures.

- **D-24:** **Determinism is a hard rule from the first line of derivation code:** never range a `map` directly into ordered output. Iterate `protoreflect`'s field list in declaration order, or sort keys explicitly. Golden tests run with `-count=5` in CI so order-dependence surfaces as a failure rather than as "flaky CI" (PITFALLS Pitfall 10). Reserved-name and load-failure reporting is sorted too, so error output is diffable.

- **D-25:** **Every panic path gets a test proving it fires at schema load, not at first mutation** — unknown `Exclude` name, unknown `Override` name, unresolved `oneof`, reserved-identifier collision — and each test asserts the *specific message content*, not merely that it panicked (PITFALLS Pitfall 1 verification criterion).

### Documentation (PIPE-08)

- **D-26:** The proto3 presence/zero-collapse behavior gets a **dedicated top-level README section placed above the API reference** — not a footnote, not a subsection of the mapping table — plus a pointer from the package `doc.go`. It must spell out the concrete failure modes for standalone users specifically, who have no field-mask generation layer to protect them: a naive hand-written Update that calls `Set*` for every field on a proto message silently clears every field the caller left at its Go zero value. `mixinforproto` cannot fix this for them; the docs are the only mitigation available at this layer.

- **D-27:** Document the hook/policy ordering fact ahead of the hook actually existing: a mixin's `Hooks()` run **before** any hook or privacy policy the schema author declares. Phase 1 ships no hook, but the docs and the mixin's design should not imply otherwise, so Phase 3 is not the first time an adopter learns it.

### Claude's Discretion

Because this ran in `--auto`, every decision above is Claude's discretion by construction. The ones most worth a human's second look before planning, in order:

1. **D-13** (format-validator translation) — it trades ecosystem introspectability for verdict identity, and VAL-01's wording ("visible to other ecosystem generators") arguably favors the other side of that trade.
2. **D-03** (integer `ContractVersion`) — STATE.md flags ANNO-04 as having no direct precedent; this is the least-evidenced decision here.
3. **D-12** (floats as residual) — defensible, but it means Phase 1 ships less Tier 1 coverage than VAL-02's wording suggests.
4. **D-18** (no root→mixinforproto edge in Phase 1) — a scoping call about what "scaffold" means, not a technical constraint.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Primary design contract (read first)
- `mixinforproto.md` — the phase's governing design doc. §2 (API and the `*new(M)` descriptor trick), §4.1 (the Tier 1 translation table — normative), §5 (field mapping, presence, oneof, WKT rules), §6 (testing strategy), §7 (module/roadmap), §8 (open questions).
- `entconnect.md` §3.1 — the parent doc's normative summary of `MixinForProto`'s scoping decisions. §4 — the canonical build pipeline. §5 — repository layout and the dependency graph. §9 — resolved open questions 1 and 2.

### Architecture (the annotation contract is decided here, not in the design docs)
- `.planning/research/ARCHITECTURE.md` **§Verdict** — the load-bearing section for ANNO-01…ANNO-04. Establishes, against ent's actual internals, that `gen.Type.Annotations`/`gen.Field.Annotations` are the *only* channel across the schema-load/codegen boundary (`load.Schema.Field.Validators` is an `int` count, not closures), and that the annotation contract is therefore a v0.1 deliverable rather than a later add-on. Read before designing any annotation struct.
- `.planning/research/ARCHITECTURE.md` §Recommended Project Structure — the `mixinforproto/{mixin,fieldmap,validate,annotation}.go` file layout and why `annotation.go` is called out separately.
- `.planning/research/ARCHITECTURE.md` §Pattern 1 (Annotation-Crossing) — the `entproto` decode precedent this mirrors.
- `.planning/research/ARCHITECTURE.md` §Anti-Patterns 1, 2, 4, 5 — respectively: the extension cannot read `Option` values off the mixin; "field absent from graph" is ambiguous without the exclusion inventory; `buf build -o` is a separate invocation from `buf generate`; `go.work` state does not prove external consumability.

### Pitfalls (each maps to a specific Phase 1 deliverable)
- `.planning/research/PITFALLS.md` **Pitfall 1** — entc's subprocess schema-load panics arrive truncated. Drives D-06, D-07, D-08, D-25.
- `.planning/research/PITFALLS.md` **Pitfall 2** — derived-name collisions with ent reserved identifiers and other mixins. Drives D-10.
- `.planning/research/PITFALLS.md` **Pitfall 4** — the proto3 `Default(zero)` collapse and its four concrete failure modes. Phase 1 can only mitigate it with documentation (D-26); the real fix is Phase 2's field-mask-gated Update.
- `.planning/research/PITFALLS.md` **Pitfall 9** — committed descriptor staleness and the two-independent-sources-of-truth hazard (protoreflect off the Go type vs. the committed `.binpb`). Drives D-22.
- `.planning/research/PITFALLS.md` **Pitfall 10** — map-iteration nondeterminism breaking golden tests. Drives D-24.
- `.planning/research/PITFALLS.md` **Pitfall 12** — the nested-module trap set: `./...` skipping the nested module, `replace` directives, prefixed tags. Drives D-15 through D-19.
- `.planning/research/PITFALLS.md` §Pitfall-to-Phase Mapping — the verification criterion for each of the above; use it as the phase's acceptance checklist.

### Stack and versions
- `.planning/research/STACK.md` — pinned versions and, critically, two corrected module paths: **`buf.build/go/protovalidate`** (not `github.com/bufbuild/protovalidate-go`) and **`github.com/cel-expr/cel-go`** (not `github.com/google/cel-go`). Also confirms `buf.build/go/protovalidate`'s public `ResolveFieldRules`/`ResolveMessageRules` constraint-enumeration API, which is what Tier 1 translation reads, and `buf.build/go/protovalidate/cel`'s `NewLibrary()`/`RequiredEnvOptions()` — needed in Phase 3, noted now so the dependency choice is made once.
- `.planning/research/STACK.md` §Load-Bearing Findings — includes the correction that `.Validate(fn)` exists on `field.String`, `field.Bytes`, and scalar-slice builders, not on numerics/enum/time/bool/UUID/JSON. D-13 depends on this being true for `field.String`.

### Project-level
- `.planning/REQUIREMENTS.md` — the 26 Phase 1 requirement IDs and their exact wording.
- `.planning/PROJECT.md` §Constraints, §Key Decisions — locked project-wide; do not re-litigate.
- `.planning/ROADMAP.md` §Phase 1 — the five success criteria, which are the phase's definition of done.
- `.planning/research/FEATURES.md` §MVP Definition — confirms the v0.1/v0.2 split is correctly scoped.

</canonical_refs>

<code_context>
## Existing Code Insights

**The repository is greenfield.** `git ls-files` shows no `.go`, `go.mod`, or `.proto` files — only `.claude/` tooling, `.planning/` artifacts, and the two root design docs. There are no reusable assets, no established code conventions, and no integration points, because there is no code.

### Reusable Assets
- None in-repo. Phase 1 writes the first line of Go in this project.

### Established Patterns
- None in-repo yet. **Phase 1 establishes them**, and the conventions it sets (determinism discipline per D-24, error-message format per D-08, annotation naming per D-04) become the project's baseline.
- External precedent to mirror, read-only and never depended on: `entgo.io/contrib/entproto`'s annotation define/decode pair (`entproto/message.go`, `entproto/skip.go`, `entproto/extension.go`) and `entgql`'s golden-file test layout. `.planning/research/ARCHITECTURE.md` quotes the relevant shapes directly — the planner should not need to fetch the upstream source.

### Integration Points
- **`mixinforproto` → `ent`**: implements `ent.Mixin` (`Fields()`, and `Hooks()` from Phase 3 onward).
- **`mixinforproto` → `google.golang.org/protobuf`**: `protoreflect` descriptor walking off the type parameter.
- **`mixinforproto` → `buf.build/go/protovalidate`**: `ResolveFieldRules` for constraint enumeration.
- **`mixinforproto` → entc's JSON boundary**: annotations only. This is the integration point that constrains the whole design (see canonical refs, ARCHITECTURE §Verdict).
- **Root module → `mixinforproto`**: deliberately absent in Phase 1 (D-18); arrives in Phase 2.

</code_context>

<specifics>
## Specific Ideas

- The API shape is fixed by `mixinforproto.md` §1's own example and should be matched literally:
  ```go
  func (Order) Mixin() []ent.Mixin {
      return []ent.Mixin{
          mixinforproto.MixinForProto[*orderv1.Order](
              mixinforproto.Exclude("etag"),
              mixinforproto.Override("status", field.Enum("status")),
          ),
          mixin.Time{},
      }
  }
  ```
- The panic-message format has a concrete target from PITFALLS Pitfall 1: include the schema type name, the offending proto field, the triggering option or condition, and a one-line remediation — e.g. `did you mean to call Override("status", ...)?`.
- `WithMessageRules(OnCreate)` is part of the documented option set in `mixinforproto.md` §2, but its *behavior* is Tier 3 (Phase 3). Phase 1 should not implement it. Whether the option symbol is declared early as a no-op or omitted entirely is a planner call — omitting it is cleaner; declaring it early avoids an API addition later.

</specifics>

<deferred>
## Deferred Ideas

- **`StrictPresence` option** (MIX2-01) — panic on non-optional scalars where zero/unset ambiguity would be silent. v2. `mixinforproto.md` §8 Open Question 1 and PROJECT.md's Key Decisions both flag the `Default(zero)` collapse as the design's sharpest edge and mark it "⚠️ Revisit". Phase 1's answer is documentation (D-26), not an option.
- **`debug_redact` → `field.Sensitive()` inference** (MIX2-02) — v2, and STATE.md records it as an unresolved open question with security implications. **A decision checkpoint is due by end of Phase 1** per STATE.md's Blockers section; it stays out of Phase 1's implementation scope regardless of how that checkpoint resolves.
- **`google.protobuf.Duration` → `field.Int64` nanoseconds via option** (MIX2-04) — v2. Phase 1 skips `Duration` (per `mixinforproto.md` §5: "else skipped").
- **Custom proto field options as an annotation channel** (e.g. `(entconnect.field).immutable`) — explicitly Out of Scope in both REQUIREMENTS.md and PROJECT.md. `Override` is the answer until real demand appears. Do not reopen.
- **`OnUpdateWithFetch` message-rule enforcement** (MIX2-03) — v2, and Out of Scope for v1 by name.
- **entproto migration document** (DOC2-01) — v2.
- **A `mixinforproto`/`entconnect doctor` CLI** — D-07 chose an exported `Validate[M]` function instead. A CLI remains a reasonable later convenience; nothing in Phase 1 forecloses it.

</deferred>

---

*Phase: 1-MixinForProto Core*
*Context gathered: 2026-08-08*
