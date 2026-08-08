# Phase 1: MixinForProto Core - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-08-08
**Phase:** 1-MixinForProto Core
**Mode:** `--auto` — no user prompts were issued. Every selection below is the recommended default, chosen by Claude and logged for audit.
**Areas discussed:** Annotation contract shape and versioning; Failure surface and debuggability; Derived-name collision policy; Tier 1 translation boundary; Two-module scaffold and CI; Test corpus and golden strategy; Presence documentation and deferred checkpoints

---

## Annotation contract shape and versioning

**Q: Version marker granularity for ANNO-04?**

| Option | Description | Selected |
|--------|-------------|----------|
| Single integer `ContractVersion` + additive-only policy | One int on both structs, bumped only on breaking layout change; unknown keys tolerated | ✓ |
| Semver string per annotation | Richer signal, requires parsing at decode time | |
| Per-field version markers | Maximum granularity, maximum bookkeeping | |
| No marker, rely on mapstructure tolerance | Cheapest; ANNO-04 explicitly requires a marker | |

**Notes:** The only question a consumer needs answered is "can I decode this?". An integer answers it without parsing. STATE.md flags this as having no direct precedent — the weakest-evidenced decision in this phase.

**Q: Where do `Exclude`/`Override` decisions live?**

| Option | Description | Selected |
|--------|-------------|----------|
| Schema-level inventory + name lists | `SourceMessage` carries the full proto field inventory plus `Excluded`/`Overridden` | ✓ |
| Per-field tombstone annotations | An annotation per absent field — but absent fields have no `gen.Field` to hang it on | |
| Both | Redundant | |

**Notes:** ARCHITECTURE Anti-Pattern 2 is decisive — an excluded field and a forgotten field produce an identical `gen.Graph`, so the inventory must live at schema level where something still exists to annotate.

**Q: Annotation key naming?**

| Option | Description | Selected |
|--------|-------------|----------|
| Exported consts `MixinForProtoMessage` / `MixinForProtoField` | Mirrors entproto's `MessageAnnotation` precedent | ✓ |
| Unexported keys with accessor funcs | Hides the string, adds API surface | |
| `entconnect`-prefixed keys | Wrong module owns the name | |

---

## Failure surface and debuggability

**Q: Internal architecture for schema-load failures?**

| Option | Description | Selected |
|--------|-------------|----------|
| Pure `derive()` core + panicking wrapper | Errors internally, panic isolated to the `Fields()` adapter | ✓ |
| Panic directly at each failure site | Fewer layers, but makes MIX-12 a parallel code path | |
| Collect errors, return partial fields | Violates the fail-fast premise of a runtime mixin | |

**Q: In-process debug entry point shape (MIX-12)?**

| Option | Description | Selected |
|--------|-------------|----------|
| Exported `Validate[M](opts ...Option) error` | One symbol; callable from a plain `go test` in the user's schema package | ✓ |
| Separate `doctor` CLI binary | More discoverable, much more scaffolding | |
| Env-var-gated non-panic mode | Hidden behavior toggles are their own bug class | |

**Q: Error message format?**

| Option | Description | Selected |
|--------|-------------|----------|
| First-line-complete: `mixinforproto: <Msg>.<field>: <what> — <fix>` | Survives entc's subprocess truncation | ✓ |
| Multi-line block with header | Reads better in a terminal, loses everything after line 1 in CI | |
| Go error wrapping chain | Idiomatic, but the wrap chain is what gets truncated | |

**Notes:** PITFALLS Pitfall 1 documents this as a filed ent issue — users see `exit status 2` and nothing else. Also decided here: failures are collected and reported together rather than one per regeneration cycle.

---

## Derived-name collision policy

**Q: What happens when a derived field name hits an ent reserved identifier?**

| Option | Description | Selected |
|--------|-------------|----------|
| Panic at schema load with the Exclude/Override remedy | Catches the catalog-known set early; same-schema collisions documented and left to the compiler | ✓ |
| Warn only | The actual failure is a Go compiler error in generated code, far from the cause | |
| Auto-rename with suffix | Silently renames an API-visible field — defeats contract-as-source-of-truth | |
| Do nothing | Pitfall 2's default outcome | |

---

## Tier 1 translation boundary

**Q: What happens to constraints Tier 1 cannot translate, given Tier 2 lands in Phase 3?**

| Option | Description | Selected |
|--------|-------------|----------|
| Record as residual (IDs + CEL fingerprint), do not enforce | Preserves what Phases 3 and 5 need; documented as boundary-only for now | ✓ |
| Panic on any untranslatable constraint | Unusable against any real contract | |
| Silently ignore | Loses data Phase 5's drift check depends on | |

**Q: Open/closed interval adjustment for `gt`/`lt`?**

| Option | Description | Selected |
|--------|-------------|----------|
| Exact `+1`/`-1` for integers, residual for floats | Lossless where it can be; refuses to bake in a known-wrong verdict where it can't | ✓ |
| Widen floats to `gte`/`lte` | Schema layer would accept values protovalidate rejects | |
| Residual for all `gt`/`lt` | Gives up integer coverage VAL-02 explicitly asks for | |

**Q: How are string format validators translated per VAL-01?**

| Option | Description | Selected |
|--------|-------------|----------|
| `Match(regexp)` where protovalidate's semantics are a documented regex; `Validate(fn)` delegating to protovalidate's own predicate otherwise | Both are native builder calls; delegation guarantees verdict identity | ✓ |
| Hand-written regexes for all formats | Approximations that will disagree with protovalidate somewhere | |
| Treat all formats as residual | VAL-01 names format validators explicitly as Tier 1 | |

**Notes:** Flagged in CONTEXT.md as the decision most worth a human's second look — it trades some ecosystem introspectability (which VAL-01's wording emphasizes) for verdict identity (which the differential harness will police in Phase 3).

---

## Two-module scaffold and CI

**Q: `go.work` checked in or ignored?**

| Option | Description | Selected |
|--------|-------------|----------|
| Checked in, plus a `GOWORK=off` CI job | Correct cross-module resolution without `replace` directives; PIPE-02 proves standalone consumption | ✓ |
| Gitignored, contributors create their own | Every contributor re-derives the same file | |
| No `go.work`, use `replace` directives | PIPE-04 forbids committed `replace` lines | |

**Q: Does the root module depend on `mixinforproto` in Phase 1?**

| Option | Description | Selected |
|--------|-------------|----------|
| Defer the edge to Phase 2 | Root gets `go.mod` and CI scaffold only; nothing to decode until the extension exists | ✓ |
| Add the dependency now | Invokes cross-module tagging discipline before there is anything to tag | |
| Duplicate annotation types in both modules | Two sources of truth — the exact failure mode the project exists to eliminate | |

**Q: How does CI enumerate both modules?**

| Option | Description | Selected |
|--------|-------------|----------|
| Explicit `MODULES` list in a checked-in script | Fails loudly when a module is added and the list isn't updated | ✓ |
| CI matrix with hardcoded dirs | Same idea, but the list lives in CI config instead of runnable-locally | |
| `find -name go.mod` discovery | Silently absorbs mistakes | |

**Notes:** Release tagging discipline (`mixinforproto/vX.Y.Z` vs root `vX.Y.Z`, dependency bumps in a separate commit) was recorded here as well — Go module proxy caching makes a wrong tag effectively permanent.

---

## Test corpus and golden strategy

**Q: Corpus protos — synthetic or a slice of the real Order contract?**

| Option | Description | Selected |
|--------|-------------|----------|
| Synthetic, one small file per mapping-rule class | A failure names the rule that broke | ✓ |
| Slice of the Order/Inventory reference contract | Reference app is Phase 5; borrowing it early couples the phases | |
| Both | Redundant for Phase 1 | |

**Q: Are corpus Go stubs committed?**

| Option | Description | Selected |
|--------|-------------|----------|
| Commit stubs, plus a regenerate-and-diff CI job | `go test` needs no `buf` — which is what standalone-testable actually means | ✓ |
| Generate at test time via `buf` | Makes `buf` a test-time dependency of a module whose selling point is minimal deps | |
| Generate via `go:generate` on demand | Same problem, later | |

**Q: How to golden-assert `[]ent.Field`?**

| Option | Description | Selected |
|--------|-------------|----------|
| Marshal `Descriptor()` to a stable sorted doc, compare with goldie | Matches the entgql/entproto precedent; the interesting state is behind the descriptor | ✓ |
| `reflect.DeepEqual` against hand-built expected fields | Some field state is closures | |
| Render Go source and diff | Reimplements codegen to test derivation | |

**Notes:** Determinism was made a hard rule here rather than a guideline — never range a map into ordered output, and run golden tests with `-count=5` in CI.

---

## Presence documentation and deferred checkpoints

**Q: How prominent is the proto3 presence documentation (PIPE-08)?**

| Option | Description | Selected |
|--------|-------------|----------|
| Top-level README section above the API reference + `doc.go` pointer | Adopters meet it before it surprises them, which is PIPE-08's literal wording | ✓ |
| A note in the API reference | Buried | |
| A separate doc file linked from README | One click too many for the design's sharpest edge | |

**Q: Do `StrictPresence` (MIX2-01) or `field.Sensitive()` inference (MIX2-02) enter Phase 1?**

| Option | Description | Selected |
|--------|-------------|----------|
| Keep both out; record the end-of-phase decision checkpoint | Both are v2; STATE.md asks only for a checkpoint, not an implementation | ✓ |
| Pull `StrictPresence` into Phase 1 | Scope creep into v2 | |
| Pull Sensitive inference into Phase 1 | Scope creep, and the security question is unresolved | |

---

## Claude's Discretion

`--auto` mode means every decision in this phase was Claude's. The four most worth overturning if a human disagrees, in priority order:

1. Format-validator translation strategy (D-13) — the introspectability/fidelity trade.
2. Integer `ContractVersion` for ANNO-04 (D-03) — least-evidenced, no precedent.
3. Floats as residual rather than widened (D-12) — ships less Tier 1 coverage than VAL-02's wording implies.
4. No root→`mixinforproto` dependency edge in Phase 1 (D-18) — a scoping call, not a technical constraint.

## Deferred Ideas

- `StrictPresence` option (MIX2-01) — v2; Phase 1's answer is documentation.
- `debug_redact` → `field.Sensitive()` inference (MIX2-02) — v2; decision checkpoint due end of Phase 1 per STATE.md.
- `google.protobuf.Duration` → `field.Int64` nanoseconds via option (MIX2-04) — v2; `Duration` is skipped in Phase 1.
- `OnUpdateWithFetch` message-rule enforcement (MIX2-03) — v2, Out of Scope for v1 by name.
- Custom proto field options as an annotation channel — permanently Out of Scope; do not reopen.
- entproto migration document (DOC2-01) — v2.
- A `doctor` CLI — superseded by the exported `Validate[M]` function; nothing forecloses it later.
