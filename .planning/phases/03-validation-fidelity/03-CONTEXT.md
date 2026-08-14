# Phase 3: Validation Fidelity - Context

**Gathered:** 2026-08-14
**Status:** Ready for planning
**Mode:** interactive `discuss`, all four gray areas completed. Every decision below is user-stated. Where the user chose against Claude's recommendation (D-07), both the choice and its cost are recorded verbatim.

<domain>
## Phase Boundary

Phase 3 closes Phase 1's deliberate gap. Phase 1's **D-11** left untranslated protovalidate constraints *recorded but not enforced* — their IDs and a fingerprint of their CEL expression strings sit on `SourceField.ResidualIDs`/`ResidualFingerprint`, checked by nothing. Phase 2's `runtime.Chain` protovalidate stage has been the **only** thing enforcing them. This phase makes the storage layer enforce them too, with verdicts provably identical to the boundary's, and ships the differential harness that proves it.

**In scope:** VAL-04…VAL-11, PIPE-05, PIPE-06.

**Explicitly NOT in this phase** (later phases; must not be pulled forward):
- Flow binding, `flow.Start`, `GetRunStatus`, `codec/proto` — Phase 4 (FLOW-01…FLOW-06)
- The bidirectional drift check — Phase 5 (DRIFT-01…DRIFT-07). This phase *records* unenforceable-rule provenance (D-09) so DRIFT can report on it; it does not build the check.
- The Order/Inventory reference application — Phase 5
- grpc-gateway / plain-REST emitters — `entconnect.md` §3.3, v0.4/v0.5
- `OnUpdateWithFetch` message-rule enforcement — `mixinforproto.md` §4.3 explicitly excludes it from v1 (hidden query cost, unclear semantics under concurrent writes)

### Requirement-text amendments this phase owes

Two decisions below outgrow their requirement wording. Following the Phase 2 precedent (CRUD-03's `Paginate()` correction, CRUD-07's "only place constructed" correction), these must be amended in `REQUIREMENTS.md` and `ROADMAP.md` **as part of this phase's work**, not silently diverged from:

1. **ROADMAP Phase 3 SC1** says the hook evaluates "residual field-scoped protovalidate CEL rules (the long tail Tier 1 can't translate)." **D-02** makes protovalidate authoritative for *every* rule on an in-scope field, translated ones included, because VAL-07 is otherwise unachievable. SC1 must be reworded to match.
2. **VAL-05** says "The CEL hook evaluates only fields changed by the mutation." **D-06** makes the scope operation-dependent — all derived fields on Create, changed-only on Update — because a proto3 zero persisted by an unset Create field is a real value the boundary validates and a strict changed-only storage check would miss. VAL-05 must be reworded to match.

Neither is scope creep: both are the minimum needed to satisfy VAL-07 as written. A planner that "fixes" the code back toward the old wording reopens the gap.

</domain>

<decisions>
## Implementation Decisions

### Violation Identity (VAL-06, VAL-07)

- **D-01:** The mutation-time hook returns **protovalidate's own `*protovalidate.ValidationError`** — the exact type `protovalidate.Validate()` produces and the exact type `connectrpc.com/validate` builds its `connect.Error` from. Identity becomes a property of *using the same object*, not of two code paths agreeing to construct the same thing. This costs `mixinforproto` no new dependency: `buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go` is already in its first require block with no `// indirect` marker (see `mixinforproto/validate.go`'s file doc comment, which establishes this and corrects an earlier MIX-14 misreading).

  Accepted cost: `mixinforproto`'s public error surface is now protovalidate's, so a protovalidate major bump becomes a `mixinforproto` breaking change.
  — **Reversibility:** one-way — it is the error type every adopter's `errors.As` targets, and VAL-06 promises it as a consumable structured error outside any RPC context.

- **D-02:** The hook evaluates **the full protovalidate field-rule set for every in-scope field** — translated and residual alike — not just residuals. Native ent builders from Tier 1 stay (VAL-01's ecosystem introspectability is unaffected) but are **shadowed**: ent's field validators run inside the mutator and mixin hooks wrap the mutator, so the hook fires first and pre-empts them.

  Rationale: a Tier 1 translated rule failing at the storage layer produces ent's `*ValidationError` — `ent: validator failed for field "Order.name"` — carrying **no protovalidate constraint ID at all**, while the boundary would say `string.max_len`. No amount of residual-only work makes those identical, so VAL-07 is unachievable under a residual-only hook. Obligates the SC1 amendment above.

  Accepted cost: every mutation on a MixinForProto-backed field runs protovalidate over its changed fields.
  — **Reversibility:** costly — narrowing back to residual-only later silently weakens VAL-07 for the rules most likely to fire.

- **D-03:** **Per-field rule evaluation over the in-scope set only.** Never validate a whole reconstructed message. Resolve rules per field descriptor — the same `ResolveFieldRules` path Tier 1 already uses in `mixinforproto/validate.go` — and evaluate only for fields in scope.

  Rationale: a FieldMask-gated Update carrying only `name` reconstructs a message with every other field at its zero, so a `required` or `min_len` rule on an unmasked field would fire at storage and never at the boundary. That is a **phantom violation** — an identity break in the opposite direction from what VAL-07 anticipates. Per-field evaluation makes it structurally impossible rather than filtered out after the fact, and VAL-05 falls out for free. Message-level rules stay boundary-only, which VAL-08 already requires.

- **D-04:** VAL-07's "identical" is defined **entity-relative**. Both layers validate the *entity* message (`Order`), both emit path `name`. The request wrapper (`CreateOrderRequest`, whose boundary violations carry `order.name`) is a transport artifact the storage layer cannot and should not know about. PIPE-06 therefore compares `protovalidate.Validate(order)` against the ent mutation verdict — same object, same paths — so any difference the harness reports is a **real** disagreement, never a wrapper artifact.

  Explicitly rejected: excluding field path from the comparison (that is how a real path bug hides), and having generated handlers rewrite storage paths into request-relative ones (puts path-rewriting in generated handlers and leaves VAL-06's out-of-RPC consumer on a different convention than the wire).

### Mutation → CEL Binding (VAL-04, VAL-05)

- **D-05:** A **reverse conversion table covering all derivation kinds** — the mirror of `fieldmap.go`'s forward table. Scalar, enum (ent's string → descriptor number), each WKT (`time.Time` → `*timestamppb.Timestamp`), map-as-JSON and message-as-JSON (JSON → `dynamicpb`). Satisfies VAL-04's "covering all field types."

  Keep forward and reverse **adjacent in the same file or an obviously paired one**, so a change to one is visibly a change to the other. The JSON→`dynamicpb` leg is where fidelity bugs will live; plan it as its own task with its own tests, not as a detail of the hook.

  Explicitly rejected: binding ent's Go values directly via a cel-go type adapter. A rule written against `google.protobuf.Timestamp` expects `.seconds`; a `time.Time` has no such field, so protovalidate semantics stop holding — the one property this phase exists to guarantee.

- **D-06:** **Scope is operation-dependent.** On **Create**, the hook checks *all derived fields*, whether or not the mutation explicitly set them. On **Update**, changed-only per D-03.

  Rationale: derived fields carry `Default(zero)` (Phase 1 D-26), so an unset field on Create still persists as `""`/`0`. That is a real value the boundary validates against a complete request message. A strict changed-only storage check would skip it — a **missed** violation, the mirror of D-03's phantom violation, and precisely the zero-collapse hole Phase 1 could only document. Obligates the VAL-05 amendment above.

  *Considered and rejected:* treating ent-applied defaults as "changed." It would keep one uniform rule, but hooks run **before** the mutator, which is exactly when defaults may not yet be applied — unverified, and D-06 does not depend on it.

- **D-07 (USER DECISION — against Claude's recommendation; cost recorded deliberately):** The hook is a **hybrid**: protovalidate's own evaluator for standard rules, a **local `cel.Env`** built from `buf.build/go/protovalidate/cel`'s `NewLibrary()` + `RequiredEnvOptions(fd)` for residual CEL, compiled once at schema load per VAL-04's literal wording.

  Claude recommended instead calling `protovalidate.Validate()` on a synthesized `dynamicpb` message restricted to in-scope fields — one evaluator, one code path, identity by construction, and no direct cel-go dependency for `mixinforproto`. That option was **not chosen**, and it carried its own risk (it depends on protovalidate v1.2 exposing a per-field filter, unverified).

  **Consequences the planner must carry, not rediscover:**
  1. Two evaluation paths must produce identical output. That is exactly the drift class PIPE-06 detects — so the differential harness stops being a proof-of-correctness formality and becomes **the mechanism that keeps this choice safe**. It is not optional and not deferrable to the end of the phase.
  2. `mixinforproto` takes a **direct cel-go dependency** (see D-16 for the import-path obligation).
  3. Error construction must be **single-pathed even though evaluation is not**: both paths build `*validate.Violation` protos and hand them to one shared constructor that wraps them into the `*protovalidate.ValidationError` D-01 specifies. Do not let the residual path grow its own error-building code.
  — **Reversibility:** costly — collapsing to the single-evaluator design later means deleting the local env and re-proving identity from scratch.

- **D-08:** The hybrid's split line is drawn **by rule kind, not by Phase 1's `ResidualIDs`**. The local env handles **only** `(buf.validate.field).cel` expressions — genuinely custom, user-authored CEL. **Every standard rule goes to protovalidate's evaluator**, including the ones Tier 1 declined to translate: float `gt`/`lt` (Phase 1 D-12) and `required`-on-non-presence.

  Rationale: `ResidualIDs` is a *provenance* record, not a routing table, and it deliberately contains standard rules Tier 1 chose not to translate. Routing by it would hand the local env float comparison and presence semantics to hand-reimplement — rules protovalidate already implements correctly. This keeps the local path small and well-defined, which is what makes D-07's cost bounded.

  **Planner note:** do not repurpose `ResidualIDs` as the routing input. It stays what Phase 1 built it as.

### Unenforceable Rules (VAL-04, VAL-08, VAL-09)

- **D-09:** Unenforceable rules are **split by cause**, because two failure classes look alike and deserve opposite answers:
  - **Uncompilable CEL expression → panic at schema load.** The contract is broken; protovalidate would reject it at the boundary too, so failing earlier is strictly better. Follows Phase 1 D-08's self-sufficient-first-line error discipline and D-25's "every panic path gets a test asserting the specific message, not merely that it panicked."
  - **Unbindable field kind → record as still-unenforced/boundary-only in provenance.** That defect is *ours*, not the contract's, and panicking would brick a schema that loaded fine under Phases 1–2. Recording it keeps it visible to Phase 5's drift check.

  The distinction is the point: be loud about the adopter's bugs, honest about our own.

- **D-10:** `WithMessageRules(OnCreate)` **panics at schema load if any message rule references a field ent does not have** (excluded via `Exclude()`, or underivable). Walk each message rule's CEL expression for field references at load time; panic naming the rule and the offending field.

  Rationale: `mixinforproto.md` §4.3's literal "constructs an `M` from mutation fields" would leave excluded fields at their proto3 zero, so a cross-field rule like `this.discount <= this.subtotal` would return a verdict — pass *or* fail — computed from data that was never real. That is worse than not checking. Opting in is a deliberate act, so it can carry a deliberate cost; the adopter's fix is obvious (stop excluding the field, or don't opt in).

  **Already settled upstream, do not re-litigate:** `WithMessageRules` is currently *not declared* (`mixinforproto/option.go:11` deliberately omits it — Phase 1 left the symbol out rather than shipping a no-op). `mixinforproto.md` §4.3 is normative on the rest of its shape: boundary-only by default, Create-only opt-in, and `OnUpdateWithFetch` as the named future option for Update-time enforcement, explicitly excluded from v1.

- **D-11:** VAL-09's ordering guarantee is **pin-and-document, take no position**. Research establishes the real order empirically; docs state it; a test asserts it so an ent upgrade that changes it fails CI. Exactly what VAL-09 asks, nothing more.

  **Document the consequence honestly:** if validation runs before the privacy policy, an unauthorized caller receives a constraint violation rather than a denial, which leaks which constraints exist. This is a recorded, accepted property — not an oversight — and the docs must say so rather than leaving a reader to discover it. Phase 1 D-27 already documents mixin hooks as running before schema hooks; the privacy position is the part research must establish.

- **D-12:** A **runtime** reverse-conversion failure on real data — an AsJSON blob that will not unmarshal, an enum string absent from the descriptor — **fails closed**: the mutation aborts, and the error is explicitly **not** a protovalidate violation. No constraint ID is synthesized, because no constraint failed. It is a data-integrity fault in the layer between ent and the contract and must read as one.

  Rationale: reporting it as a validation failure would put a **fabricated constraint ID on the wire** and corrupt the exact signal PIPE-06 depends on. Skipping the field's rules instead would silently downgrade to unvalidated for precisely the values weird enough to break conversion — the population most likely to be corrupt or hostile.

  Note this is distinct from D-09's *schema-load* classification: D-09 asks "can this kind ever be bound?", D-12 asks "did this particular value bind?".

### Proof & Parity Gate (PIPE-05, PIPE-06, VAL-11)

- **D-13:** **Two harnesses**, deliberately:
  - **Driverless sweep in `mixinforproto`** — the exhaustive random differential run, against constructed mutations, **no database**. This preserves a real property the module has today and would otherwise lose by accident: `mixinforproto` has **no DB driver in its require block**, and `mixinforproto/internal/boundarytest/` performs a real entc schema load without ever opening one. It also means the standalone module — the audience the two-module split exists to serve — carries its own PIPE-06 proof.
  - **Real-client wiring proof in the root module** — a smaller suite driving actual Create/Update through a real ent client on the `modernc.org/sqlite` already in root's `go.mod`, proving ent genuinely invokes the hook in the real mutation path. The driverless sweep cannot show this; without it, a wiring regression that silently stops calling the hook passes green.

  Neither substitutes for the other. Plan them as separate deliverables.

- **D-14:** PIPE-06's randomness is **seeded deterministically from the corpus message name**, and the seed plus the failing value print on failure. Every run generates the same values, so Phase 1 D-24's `-count=5` stays a real order-dependence check rather than a lottery, and a red CI reproduces locally verbatim. A **separate opt-in job** with a time-based seed explores new value space without ever making required CI flaky.

- **D-15:** The VAL-11 parity gate compares an **explicit named set** of modules across both `go.mod`s — `buf.build/go/protovalidate`, `buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go`, cel-go, `google.golang.org/protobuf`, and `entgo.io/ent`. Broader than VAL-11's literal "protovalidate/cel-go" because protobuf-runtime or ent skew can shift a verdict just as easily, and a gate that says nothing about them gives false assurance.

  Explicit means reviewable: adding to the list is a visible decision. Rejected: comparing every shared dependency — it fires on incidental transitive skew, and a gate that cries wolf gets disabled, taking the real signal with it.

  The gate must run in a context where the two modules resolve **independently** — `go.work` unifies the build, so it cannot itself be the gate. The existing `GOWORK=off` CI job (Phase 1 D-17/PIPE-02) is the right host.

- **D-16 (RESEARCH OBLIGATION — factual, not a preference):** The cel-go import path must be **whatever `buf.build/go/protovalidate@v1.2.0`'s own `go.mod` pins**, matched exactly in both modules. `protovalidate/cel.NewLibrary()` returns a `cel.Library` typed against that path; a mismatch surfaces as a **compile error**, not a runtime one.

  **`CLAUDE.md` is currently wrong on this point and must not be trusted here.** It states `github.com/cel-expr/cel-go@v0.31.0`. Both modules today actually resolve **`github.com/google/cel-go v0.28.0`** (indirect, identical across both — verified in `go.mod`). ROADMAP's Phase 1 planning note further records `cel-expr/cel-go` as *verified non-functional* — its own `go.mod` still declares the old path. Research must establish the live answer and, once D-07 makes cel-go a direct dependency of `mixinforproto`, the `CLAUDE.md` stack table should be corrected to match reality.

### Claude's Discretion

All four gray areas were answered by the user. The following were resolved by Claude and are recorded so planning is not blocked — a reviewer may overturn any of them:

1. **Shared violation constructor** (D-07 consequence 3) — that both evaluation paths funnel through one error-building function is Claude's mitigation for the hybrid choice, not a user instruction. It is the single highest-leverage guard on D-07 and should survive review.
2. **Where `runtime.MapError` learns about `*protovalidate.ValidationError`** — `runtime/errormap.go` today sends anything unrecognized to `CodeInternal`, so a residual violation reaching it unhandled would surface as `Internal` and quietly break VAL-07. protovalidate's error type is **not** application-specific, so unlike ent's generated `*NotFoundError`/`*ConstraintError`/`*ValidationError` it *can* live in the shared `runtime` package. Adding that case is required work, wherever the planner puts it.
3. **Whether the reverse table lives in `fieldmap.go` or a paired new file** — D-05 requires adjacency, not a specific filename.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Primary design contract (read first)
- `mixinforproto.md` §4.2 — the single mixin-declared `Hooks()` entry. Normative for this phase's hook design.
- `mixinforproto.md` §4.3 — Tier 3 / message-level rules. **Normative and already-settled** on `WithMessageRules(OnCreate)`: boundary-only default, Create-only opt-in, `OnUpdateWithFetch` named as the deliberately-excluded-from-v1 future option. D-10 resolves only what §4.3 leaves open (excluded-field references).
- `mixinforproto.md` §2 — the documented option set `WithMessageRules` belongs to.
- `entconnect.md` §3.2 — the fixed interceptor chain, for the boundary half of the identity guarantee.

### Phase 1 outputs this phase consumes and closes
- `.planning/phases/01-mixinforproto-core/01-CONTEXT.md` — **D-11** (residuals recorded-not-enforced: the gap this phase closes), **D-12** (float `gt`/`lt` deliberately residual — see D-08), **D-13** (format validators delegating to protovalidate's own predicates), **D-24** (determinism, `-count=5` — see D-14), **D-25** (every panic path tested for message content — see D-09), **D-26** (proto3 zero-collapse — see D-06), **D-27** (mixin hooks run before schema hooks — see D-11).
- `.planning/phases/01-mixinforproto-core/01-VERIFICATION.md` — what Phase 1 actually proved, including the four closed gaps.
- `mixinforproto/validate.go` — Tier 1's live implementation **and** its file doc comment, which establishes that `buf.build/gen/.../buf/validate` is already a direct dependency and corrects the narrower MIX-14 reading. Load-bearing for D-01. Also the source of the `ResolveFieldRules` usage pattern D-03 reuses.
- `mixinforproto/fieldmap.go` — the forward mapping table D-05 mirrors; `residualEntry`, `celRuleResidual`, `residualFingerprint`, and the `requiredResidualZero` classification.
- `mixinforproto/annotation.go` — `SourceField.ResidualIDs`/`ResidualFingerprint`/`TranslatedIDs`/`LengthUnitDivergentIDs`. Read the file, not a description of it.
- `mixinforproto/errors.go` — the `failure` struct and D-08/D-09 collected-error rendering that D-09's panics must match.
- `mixinforproto/option.go:11` — the comment recording why `WithMessageRules` is deliberately undeclared.
- `mixinforproto/testdata/constraints_residual_cel.golden` — proof the residual-recording pipe works end to end.
- `mixinforproto/internal/boundarytest/` — real entc schema load with **no DB driver**. The property D-13 protects.
- `mixinforproto/corpus_test.go` — the corpus-adequacy guard pattern (Phase 1 Plan 09) PIPE-05 extends.

### Phase 2 outputs this phase builds on
- `.planning/phases/02-crud-handlers-interceptor-chain/02-CONTEXT.md` — **D-13** (validator built once per process: VAL-10 is largely already satisfied, this phase owes the *test*, not the build), **D-18** (the error-mapping table D-12's new error class must slot into), **D-14…D-17** (FieldMask semantics, which is why D-03's partial-message hazard exists at all).
- `runtime/interceptor.go` — `Chain`, `WithValidator`, and the once-per-process construction. The boundary half of VAL-07.
- `runtime/errormap.go` — `MapError`'s unrecognized→`CodeInternal` default, and its doc comment explaining precisely which types can and cannot live in the shared package. Directly relevant to Claude's Discretion item 2.
- `runtime/doc.go:12` — the "until Phase 3 lands, this interceptor is the ONLY thing enforcing untranslated constraints" note. **This comment becomes false in this phase and must be updated.**

### Project-level
- `.planning/REQUIREMENTS.md` — VAL-04…VAL-11, PIPE-05, PIPE-06 verbatim. **VAL-05 needs amending** per D-06.
- `.planning/ROADMAP.md` — Phase 3 Success Criteria. **SC1 needs amending** per D-02.
- `.planning/PROJECT.md` — Core Value: "enforced identically at the transport boundary and at the storage layer." This phase is where that sentence becomes true.
- `.claude/CLAUDE.md` — pinned stack. **Known wrong on cel-go's import path and version** — see D-16; verify against the live `go.mod`s, not this table.
- `.github/workflows/ci.yml` — the `MODULES` enumeration and the `GOWORK=off` standalone job that D-15's parity gate must run inside.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `mixinforproto/validate.go`'s **primitive-only structural interfaces** (`stringRulesIface`, `rangeRulesIface[V]`) — the established technique for consuming protovalidate's generated rule types. D-01 relaxes the *motivation* for it (that file's own comment already concedes the dependency is direct anyway), but the pattern is proven and its `ResolveFieldRules` call site is exactly what D-03's per-field evaluation needs.
- `mixinforproto/fieldmap.go`'s per-`Kind` switch — the shape D-05's reverse table mirrors. `residualEntry`/`celEntries` already carry the CEL expression strings D-07's local env must compile.
- `mixinforproto/errors.go`'s `failure` + sorted `derivationError` rendering — D-09's schema-load panics and D-10's message-rule panic should be *collected failures* in this existing machinery, not a new one-off panic path. Phase 1 D-09's "report all offenders in one pass" applies.
- `mixinforproto/corpus_test.go` + `testdata/*.golden` + `goldie` — PIPE-05's conformance corpus extends this, one proto file per constraint class (Phase 1 D-21), with committed stubs (D-22) so tests need no `buf`.
- `scripts/generate-stubs.sh` + `make check-stubs` — any new corpus protos go through this, not around it.
- `internal/entconnecttest/` — the root module's real-ent-client test pattern (sqlite already wired) that D-13's wiring-proof suite should follow.

### Established Patterns
- **Failure discipline** (Phase 1 D-06/D-08/D-09/D-25): pure core returns collected failures, thin adapter panics; every first line self-sufficient; every offender reported in one pass; every panic path tested for its *specific message*. D-09 and D-10 inherit all of it.
- **Determinism by construction** (D-24): never range a map into ordered output, sort explicitly, `-count=5` in CI. D-14's seeding decision exists to keep this true under PIPE-06's randomness.
- **Two-module hygiene** (Phase 1 D-15/D-16/D-17): `go.work` checked in, **no `replace` in any committed `go.mod`**, explicit `MODULES := . ./mixinforproto`, `GOWORK=off` job. D-15's parity gate lives here; D-07's new direct cel-go dependency lands under D-19's tagging discipline (separate commit, already-pushed tag, never a same-commit self-reference).

### Integration Points
- `mixinforproto` gains its first `Hooks()` implementation — the mixin has declared none until now.
- `mixinforproto`'s `go.mod` gains a **direct** cel-go dependency (D-07, path per D-16). It has none today; cel-go is currently `// indirect` at `github.com/google/cel-go v0.28.0` in both modules.
- `runtime/errormap.go` gains a `*protovalidate.ValidationError` case, or generated handlers gain it — but somewhere it must exist, or residual violations surface as `Internal`.
- `runtime/doc.go`'s Phase-3 caveat and `mixinforproto/README.md`'s "boundary-only until Phase 3" language both become stale the moment the hook ships.
- The root module's test suite gains the D-13 wiring proof; `mixinforproto`'s gains the driverless sweep — and must not gain a DB driver in doing so.

</code_context>

<specifics>
## Specific Ideas

The user overrode Claude's recommendation on exactly one decision (D-07), choosing the hybrid evaluator over the single-evaluator design. That choice is recorded above with its full cost, and the three mitigations it obligates (single-pathed error construction, PIPE-06 as a load-bearing safety mechanism rather than a closing formality, and the direct cel-go dependency) are not optional embellishments — they are what make the choice sound.

The planner should treat **D-05's JSON→`dynamicpb` leg** and **D-07's two-paths-one-error-shape convergence** as the phase's two genuine risk concentrations, each deserving its own plan task with its own tests, not a bullet inside a larger handler task.

</specifics>

<deferred>
## Deferred Ideas

- **`OnUpdateWithFetch`** — Update-time message-rule enforcement via fetch-then-merge. Already named and deliberately excluded from v1 by `mixinforproto.md` §4.3 (hidden query cost, unclear semantics under concurrent writes). Not this phase.
- **Time-seeded exploratory differential job** — D-14 provisions it as a separate opt-in job outside required CI. Whether it actually runs on a schedule is a CI-policy call, not a Phase 3 deliverable.
- **Collapsing D-07's hybrid to a single evaluator** — if research later confirms protovalidate v1.2 exposes a per-field filter and PIPE-06 shows the two paths agreeing trivially, the simplification is worth revisiting. Not in this phase; the user has chosen the hybrid.
- **Privacy-before-validation ordering** — D-11 pins and documents ent's actual order rather than fighting it. If the constraint-existence leak to unauthorized callers becomes a real concern, changing the order is a deliberate future change with its own design.

</deferred>

---

*Phase: 3-Validation Fidelity*
*Context gathered: 2026-08-14*
