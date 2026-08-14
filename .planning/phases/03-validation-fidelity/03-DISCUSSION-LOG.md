# Phase 3: Validation Fidelity - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-08-14
**Phase:** 3-validation-fidelity
**Areas discussed:** Violation identity, Mutation → CEL binding, Unenforceable rules, Proof & parity gate

---

## Area Selection

| Option | Description | Selected |
|--------|-------------|----------|
| Violation identity | What the mixin hook returns across the module boundary; how storage produces a byte-identical wire error | ✓ |
| Mutation → CEL binding | How ent Go values become the `this` a protovalidate rule expects; what "changed" means on Create | ✓ |
| Unenforceable rules | Uncompilable / unbindable rules at schema load; `WithMessageRules(OnCreate)` placement | ✓ |
| Proof & parity gate | Differential harness shape, seeding, CI parity gate, cel-go dependency path | ✓ |

**User's choice:** All four areas.

---

## Violation identity

### Q1 — What does the mixin hook return when a residual CEL rule fails?

| Option | Description | Selected |
|--------|-------------|----------|
| protovalidate's own error | `*protovalidate.ValidationError` — same type `connectrpc.com/validate` builds its connect.Error from; identity via same object, not agreement between paths. Already a reachable dependency. Cost: mixinforproto's public error surface becomes protovalidate's | ✓ |
| mixinforproto-owned struct | Local struct with constraint ID / message / field path, translated in `runtime/`. Keeps error API under project control; cost: identity becomes something two paths must agree on | |
| Raw `buf.validate.Violations` | Return the generated proto message directly. Max wire fidelity, but not a Go error — VAL-06 wants a structured *error* outside RPC context | |

**Notes:** Became D-01. Reversibility rated one-way — it is the type every adopter's `errors.As` targets.

### Q2 — Residuals only, or every protovalidate rule on the changed fields?

| Option | Description | Selected |
|--------|-------------|----------|
| All rules, protovalidate-authoritative | Full field-rule set, translated and residual. Native ent builders stay for VAL-01 but are shadowed (hooks wrap the mutator, so they fire first). Fully satisfies VAL-07. Costs CEL per mutation and a ROADMAP SC1 amendment | ✓ |
| Residual-only, scope VAL-07 down | Literal SC1. Cheaper, no amendment. But VAL-07 becomes true only for residuals — a `MaxLen` failure still surfaces as ent's native error with no constraint ID | |
| Residual-only + re-wrap ent errors | Re-label ent's `*ValidationError` using `SourceField.TranslatedIDs`. But ent's error names only the field, not which validator — on a multi-constraint field the ID would be a guess | |

**Notes:** Became D-02. Raised because ent's `*ValidationError` carries no protovalidate constraint ID, making VAL-07 unachievable under a residual-only hook. Obligates a ROADMAP SC1 amendment, recorded in CONTEXT.md's domain section.

### Q3 — How does the hook avoid firing rules on untouched fields?

| Option | Description | Selected |
|--------|-------------|----------|
| Per-field evaluation, changed set only | Resolve rules per field descriptor (existing `ResolveFieldRules` path), evaluate only in-scope fields. Phantom violations structurally impossible; VAL-05 falls out free | ✓ |
| Load current row, overlay, validate whole message | Highest fidelity, cross-field rules work at storage. Costs a DB read per mutation, turns Update into read-modify-validate, races concurrent writers | |
| Validate reconstructed partial, filter after | Simple, but correctness depends on the filter being right, and cross-field CEL violations have no correct filtering answer | |

**Notes:** Became D-03. Motivating hazard: a FieldMask Update carrying only `name` reconstructs a message with everything else at zero, so `required`/`min_len` on unmasked fields would fire at storage and never at the boundary.

### Q4 — What does VAL-07's "identical" cover?

| Option | Description | Selected |
|--------|-------------|----------|
| Entity-relative, compare like with like | Both layers validate the entity message, both emit `name`. PIPE-06 compares `Validate(order)` vs the ent verdict — differences are always real | ✓ |
| Constraint ID + message only, path excluded | Honest about the request wrapper, but excluding a field from comparison is how a real path bug hides | |
| Storage synthesizes request-relative paths | Byte-identical on the wire, but puts path-rewriting in generated handlers and splits VAL-06's out-of-RPC consumer onto a different convention | |

**Notes:** Became D-04. Raised because the boundary validates `CreateOrderRequest` (path `order.name`) while per-field storage evaluation emits `name`.

---

## Mutation → CEL binding

### Q1 — How does an ent mutation value become `this`?

| Option | Description | Selected |
|--------|-------------|----------|
| Reverse conversion table, all kinds | Mirror of `fieldmap.go`'s forward table — scalar, enum, WKT, map-as-JSON, message-as-JSON. Satisfies VAL-04's "all field types". JSON→dynamicpb is the risk leg | ✓ |
| Losslessly reversible kinds only | Scalars/enums/WKTs; JSON kinds left boundary-only. Smaller and honest, but narrows VAL-04 and leaves Phase 1's D-11 gap partly open into Phase 5 | |
| Bind ent's Go value via a CEL type adapter | No conversion layer, but a rule written against `google.protobuf.Timestamp` expects `.seconds` and a `time.Time` has none — protovalidate semantics stop holding | |

**Notes:** Became D-05.

### Q2 — What counts as "changed" on Create?

| Option | Description | Selected |
|--------|-------------|----------|
| Create checks all derived fields, Update changed-only | Matches what the boundary does on Create; closes the zero-collapse hole Phase 1 could only document. Requires amending VAL-05 to be operation-dependent | ✓ |
| Changed-only on both, uniformly | Literal VAL-05, cheapest, one rule. But leaves exactly the hole D-26 warned about | |
| Changed-only, count applied defaults as changed | Uniform and still catches zero-collapse, but hooks run before the mutator — defaults may not be applied yet, unverified | |

**Notes:** Became D-06. Obligates a VAL-05 amendment, recorded in CONTEXT.md's domain section.

### Q3 — protovalidate's evaluator, or its own CEL environment?

| Option | Description | Selected |
|--------|-------------|----------|
| Call `protovalidate.Validate()` on a synthesized message *(Claude's recommendation)* | One evaluator, one code path, identity by construction, standard rules free, no direct cel-go dep. RISK: depends on a per-field filter API existing in protovalidate v1.2 — unverified | |
| Own `cel.Env` via `protovalidate/cel.NewLibrary()` | Literal VAL-04, max control, no filter-API dependency. But under D-02 it owes standard rules too, risking reimplementation of protovalidate internals | |
| Hybrid — protovalidate for standard, own env for residual CEL | Each rule class handled by the machinery suited to it. Cost: two evaluation paths with two error-construction paths that must agree — the drift PIPE-06 exists to detect, built in deliberately | ✓ |

**Notes:** Became D-07. **User chose against Claude's recommendation.** Recorded with full cost in CONTEXT.md, plus three obligated mitigations: single-pathed error construction, PIPE-06 promoted to a load-bearing safety mechanism, and a direct cel-go dependency for `mixinforproto`. Claude's recommended option carried its own unverified risk (the filter API), which is noted rather than hidden.

### Q4 — Where does the hybrid draw its line?

| Option | Description | Selected |
|--------|-------------|----------|
| By rule kind — custom CEL local, standard to protovalidate | Local path stays small; nothing standard reimplemented; `ResidualIDs` stays provenance rather than becoming a routing table | ✓ |
| By ResidualIDs | One list, already computed and fingerprinted. But hands the local env float comparison and presence semantics to hand-reimplement | |
| By rule kind, but skip Tier 1-translated rules entirely | Cheapest per mutation, but reopens D-02 — a MaxLen failure loses its constraint ID | |

**Notes:** Became D-08. Raised because Phase 1's `ResidualIDs` are *not* all custom CEL — D-12 put float `gt`/`lt` there, along with `required`-on-non-presence.

---

## Unenforceable rules

### Q1 — What happens at schema load when a rule can't be enforced?

| Option | Description | Selected |
|--------|-------------|----------|
| Split by cause | Uncompilable CEL panics (broken contract, boundary would reject too); unbindable field kind records as boundary-only (our gap, and panicking would brick schemas that loaded fine in Phases 1–2) | ✓ |
| Panic on both | Uniform and loud, but one unsupported kind is a hard upgrade regression for a limitation that is ours, not the contract's | |
| Record both, never panic | Preserves D-11 exactly, zero upgrade risk. But quietly downgrading a broken contract is the runtime-surprise class this project exists to eliminate | |

**Notes:** Became D-09.

### Q2 — `WithMessageRules(OnCreate)` and excluded-field references

| Option | Description | Selected |
|--------|-------------|----------|
| Panic at schema load | Opting in is deliberate, so it can carry a deliberate cost. Fails naming the rule and the excluded field | ✓ |
| Skip those rules, record boundary-only | Nothing breaks, matches D-09's "our gap" side. But the option silently means less than its name says | |
| Evaluate with excluded fields at zero | Literal reading of §4.3, but produces verdicts from data that was never real | |

**Notes:** Became D-10. Confirmed pre-discussion that `WithMessageRules` is currently undeclared (`option.go:11`) and that `mixinforproto.md` §4.3 already settles the rest of its shape — those were not re-asked.

### Q3 — Does the phase take a position on hook ordering (VAL-09)?

| Option | Description | Selected |
|--------|-------------|----------|
| Pin and document what ent does, take no position | Exactly what VAL-09 asks. Documents the consequence: if validation precedes privacy, unauthorized callers learn which constraints exist | ✓ |
| Pin it, require privacy to win | Better security posture, but fights ent's ordering and contradicts already-shipped P1 D-27 documentation | |
| Pin it, require validation to win | Verdict identity holds across the authorization boundary, but makes constraint existence observable by design | |

**Notes:** Became D-11.

### Q4 — Runtime reverse-conversion failure on real data

| Option | Description | Selected |
|--------|-------------|----------|
| Fail closed, distinct non-validation error | Mutation aborts; explicitly not a protovalidate violation, since no constraint failed. Costs a new error type callers handle | ✓ |
| Skip that field's rules, proceed | Maximally available, but silently unvalidates exactly the values weird enough to break conversion | |
| Report as a validation failure | Clean wire behavior, but puts a fabricated constraint ID on the wire and corrupts the signal PIPE-06 depends on | |

**Notes:** Became D-12. Distinct from D-09: D-09 asks "can this kind ever bind?", D-12 asks "did this value bind?".

---

## Proof & parity gate

### Q1 — Where does the differential harness live?

| Option | Description | Selected |
|--------|-------------|----------|
| Both — driverless sweep in mixinforproto, real-client proof in root | Standalone module keeps its own PIPE-06 proof and its zero-driver property; root proves ent actually invokes the hook. Costs two harnesses | ✓ |
| Root only, real ent client on sqlite | One harness, highest fidelity, but standalone adopters get no differential proof in the module they depend on | |
| mixinforproto only, driverless | Cheapest and self-contained, but a wiring regression that stops calling the hook passes green | |

**Notes:** Became D-13. Confirmed pre-discussion that `mixinforproto` has no DB driver in its require block and `boundarytest` loads schemas without opening one — a property worth protecting deliberately.

### Q2 — Reconciling PIPE-06's randomness with D-24's determinism

| Option | Description | Selected |
|--------|-------------|----------|
| Seed derived from corpus message name, printed on failure | Same values every run, so `-count=5` stays meaningful; red CI reproduces verbatim; separate opt-in job explores with a time seed | ✓ |
| Fixed value matrix, no randomness | Reproducible by construction and readable, but PIPE-06's "random" becomes false and coverage caps at what someone thought of | |
| Fresh random seed per run, printed | Best exploration, but makes required CI nondeterministic — what D-24 exists to prevent | |

**Notes:** Became D-14.

### Q3 — What does the VAL-11 parity gate compare?

| Option | Description | Selected |
|--------|-------------|----------|
| Named set: protovalidate, its generated types, cel-go, protobuf, ent | Broader than literal VAL-11 because protobuf/ent skew shifts verdicts too. Explicit means reviewable, so the gate survives | ✓ |
| Literal VAL-11 — protovalidate and cel-go only | Smallest gate, near-zero false positives, but silent on protobuf-runtime skew | |
| Every shared dependency | Catches everything, but fires on incidental transitive skew and gets disabled | |

**Notes:** Became D-15.

---

## Claude's Discretion

Recorded in CONTEXT.md's "Claude's Discretion" section; summarized here for audit:

1. **Shared violation constructor** — that both of D-07's evaluation paths funnel through one error-building function is Claude's mitigation for the hybrid choice, not a user instruction.
2. **Where `runtime.MapError` learns about `*protovalidate.ValidationError`** — required work wherever the planner puts it; `errormap.go` today sends unrecognized errors to `CodeInternal`, which would break VAL-07.
3. **Reverse table file placement** — D-05 requires adjacency to the forward table, not a specific filename.

Additionally, **D-16** (cel-go import path) was recorded as a research obligation rather than asked as a question: `cel.Library` type identity forces matching whatever protovalidate itself pins, so it is compiler-determined, not a preference. Flagged because `CLAUDE.md` is currently wrong on it — it states `cel-expr/cel-go@v0.31.0` while both modules actually resolve `github.com/google/cel-go v0.28.0`.

## Deferred Ideas

- **`OnUpdateWithFetch`** — Update-time message-rule enforcement via fetch-then-merge; already excluded from v1 by `mixinforproto.md` §4.3.
- **Time-seeded exploratory differential job** — provisioned by D-14 outside required CI; whether it runs on a schedule is CI policy, not a Phase 3 deliverable.
- **Collapsing D-07's hybrid to a single evaluator** — worth revisiting if research confirms protovalidate v1.2's per-field filter and PIPE-06 shows the paths agreeing trivially.
- **Privacy-before-validation ordering** — D-11 pins ent's actual order; changing it is a deliberate future design.
