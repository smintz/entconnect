---
phase: 03-validation-fidelity
verified: 2026-08-15T00:00:00Z
status: gaps_found
score: 7/8 must-haves verified
behavior_unverified: 0
overrides_applied: 0
re_verification:
  previous_status: gaps_found
  previous_score: 5/8
  gaps_closed:
    - "CR-01: MessageRules.cel_expression and MessageRules.oneof carriers never read by checkMessageRuleReferences — closed by 03-08 (messageRuleReferences three-carrier convergence point)"
    - "CR-02: (buf.validate.field).ignore not honored at the storage layer — closed by 03-06 (fieldEvaluator.ignore + isZeroForKind)"
    - "CR-03: Exclude/Override not honored at the storage layer, including a type-changing Override's D-12 fault — closed by 03-06 (isExcluded/isOverridden check placed before ResolveFieldRules)"
    - "WR-04: WithMessageRules(trigger) discarded its argument — closed by 03-08 (messageRulesSet/messageRulesTrigger)"
    - "WR-07: violation_test.go cited a nonexistent Makefile gate — closed by 03-07 (check-single-validationerror-site, wired into make/pipeline.sh/CI)"
  gaps_remaining: []
  regressions: []
gaps:
  - truth: "A schema-layer violation carries the same protovalidate constraint ID and message as the boundary interceptor would produce ... so a caller cannot tell which layer caught it (ROADMAP Phase 3 Success Criterion 1/2, third independent rule-carrier counterexample surfaced by 03-REVIEW.md's CR-01)"
    status: failed
    reason: "buf.validate.oneof (OneofRules, e.g. (buf.validate.oneof).required attached to a real proto oneof block via the google.protobuf.OneofOptions extension) is a third, independent protovalidate rule-carrier extension alongside FieldRules and MessageRules. It is enforced correctly at the RPC boundary (protovalidate's own evaluator honors OneofRules.required) but mixinforproto never resolves it (grep confirms zero calls to protovalidate.ResolveOneofRules anywhere in the module), never records it in SourceMessage.BoundaryOnly provenance (recordBoundaryOnly resolves rule IDs per-field via boundaryOnlyRuleIDs(fd), which has no visibility into the containing oneof's own OneofDescriptor.Options()), and is not covered by either of 03-08's two new declaration-surface exhaustiveness guards (Guard A walks buf.validate.MessageRules' descriptor; Guard B walks buf.validate.FieldRules' descriptor — neither walks buf.validate.OneofRules, which is a distinct extension on google.protobuf.OneofOptions, not a member of either message). This is distinct from the MessageRules.oneof carrier (MessageOneofRule, a repeated-field-name list on a message-level cel/cel_expression-style rule) that 03-08 closed under the same 'CR-01' label — confirmed by reading proto/buf/validate/validate.proto: MessageOneofRule is at line 253 inside message MessageRules; OneofRules is a separate top-level message at line 264, extended onto OneofOptions at line 106. A schema declaring (buf.validate.oneof).required = true loads cleanly, and any write that reaches the generated ent.Client directly (background jobs, migrations, other internal callers bypassing the RPC boundary) can persist an entity violating that oneof's required constraint with zero record anywhere that this could happen — the exact 'a caller cannot tell which layer caught it' promise is false here in the strongest sense: neither layer catches it at storage, and there is no provenance annotation telling a Phase 5 drift-check consumer this gap exists."
    artifacts:
      - path: "mixinforproto/hooks.go"
        issue: "buildHookState's per-field loop and mixin.go's Hooks() never resolve or evaluate OneofRules for any real oneof; checkOneofResolution (derive.go) only requires oneof members to be Excluded/Overridden, it says nothing about the oneof-level required constraint itself."
      - path: "mixinforproto/derive.go"
        issue: "recordBoundaryOnly / boundaryOnlyRuleIDs are field-scoped only; no code path resolves protovalidate.ResolveOneofRules(od) or records a BoundaryOnlyOneof*-shaped provenance entry."
      - path: "mixinforproto/messagerules_test.go, mixinforproto/hooks_test.go"
        issue: "TestMessageRulesDeclarationSurfaceIsFullyHandled and TestFieldRulesDeclarationSurfaceIsFullyHandled each walk a different protobuf message descriptor (MessageRules, FieldRules); OneofRules is a third message and is walked by neither, so an OneofRules member — including the one member that exists today, `required` — is invisible to both guards by construction, not merely unexempted."
    missing:
      - "Resolve each real oneof's OneofRules via protovalidate.ResolveOneofRules in derive.go's per-message walk and record it into SourceMessage.BoundaryOnly with a new BoundaryOnlyReason naming the oneof (as 03-REVIEW.md's CR-01 fix option (a) proposes), OR explicitly document this as a deliberate v1 scope boundary in mixinforproto.md/README.md with a named test asserting the omission is intentional (fix option (b)) — silence is not acceptable per this codebase's own established convention (constraintClassExceptions's precedent)."
      - "A corpus fixture under proto/mixinforprototest/v1/ declaring (buf.validate.oneof) so the coverage/exhaustiveness machinery has something to check against."
      - "A third declaration-surface guard (or an extension of an existing one) walking buf.validate.OneofRules' own descriptor, mirroring Guard A/B's shape, so a future protovalidate release adding a member to OneofRules is also caught."
deferred: []
human_verification: []
---

# Phase 3: Validation Fidelity Verification Report

**Phase Goal:** Residual and message-level validation rules that Tier 1 can't translate get executed with byte-identical results at both the RPC boundary and the storage layer, proven by an automated differential harness

**Verified:** 2026-08-15
**Status:** gaps_found
**Re-verification:** Yes — after gap closure (03-06, 03-07, 03-08 executed against the prior VERIFICATION.md's CR-01/CR-02/CR-03/WR-07 findings)

## Goal Achievement

All five gaps recorded in the prior `03-VERIFICATION.md` (CR-01, CR-02, CR-03, WR-07, and the
implicit VAL-08 partial from CR-01) are independently confirmed closed by direct code read and
live test execution in this pass — not merely accepted from the three gap-closure SUMMARYs.
`mixinforproto/messagerules.go` now calls `GetCelExpression()`/`GetOneof()`; `mixinforproto/hooks.go`
now consults `GetIgnore()`/`Ignore_IGNORE_ALWAYS`/`isZeroForKind` and gates the per-field loop on
`o.isExcluded(name) || o.isOverridden(name)` **before** `protovalidate.ResolveFieldRules(fd)` is
called (confirmed by the `grep -n` line-number ordering the plan's own acceptance criterion
demands); `Makefile`/`scripts/pipeline.sh`/`.github/workflows/ci.yml`/`mixinforproto/violation_test.go`
all reference `check-single-validationerror-site`, and running it against the current tree exits 0
naming exactly one production construction site. Every closure-specific test named in the three
SUMMARYs was run live in this pass (`TestFieldRulesDeclarationSurfaceIsFullyHandled`,
`TestMessageRulesDeclarationSurfaceIsFullyHandled`, `TestIgnoreAlways_*`, `TestIgnoreIfZeroValue_*`,
`TestOptionSuppression_*`, `TestMessageRuleCelExpression_*`, `TestMessageRuleOneof_*`,
`TestSweep_*`, `TestCorpusMessagesHaveRecordedCoverage`, `TestCorpusExercisesEveryProtovalidateConstraintClass`)
and all pass.

However, this run's own code review (`03-REVIEW.md`) surfaced a **new, independently-confirmed**
CRITICAL finding that the prior verification pass did not catch and that none of the three
gap-closure plans targeted: `buf.validate.oneof` (`OneofRules`, extending
`google.protobuf.OneofOptions`) is a **third** protovalidate rule-carrier extension — distinct
from `FieldRules` and from `MessageRules.oneof` (`MessageOneofRule`) — that `mixinforproto` never
resolves, never records as boundary-only provenance, and that neither of 03-08's two new
declaration-surface exhaustiveness guards can see, because each guard walks a different
message's descriptor (`MessageRules`, `FieldRules`) and `OneofRules` is a third, separate message.
This verifier independently confirmed the finding by reading `proto/buf/validate/validate.proto`
directly (`OneofRules` at line 264, extended onto `OneofOptions` at line 106 — a genuinely
different declaration site from `MessageOneofRule` at line 253 inside `MessageRules`) and by
confirming `grep -rn 'ResolveOneofRules' mixinforproto/*.go` returns nothing. This is the same
class of gap CR-01/CR-02/CR-03 were — a rule the boundary enforces that the storage layer neither
enforces nor documents as unenforced — and it falls squarely inside this phase's own stated
invariant #5 ("unhandled rule carriers must fail loudly ... never silently downgrade a field to
unvalidated at the storage layer"). It was not closed by this wave's gap-closure work and remains
open.

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | Mixin hook evaluates the full protovalidate field-rule set for every in-scope field, standard and residual, compiled once at schema load (SC1) | ✓ VERIFIED | `hooks.go`'s `buildHookState` precompiles one `protovalidate.Validator` plus per-field `cel.Program`s at schema-load time only; unchanged from prior pass, not disputed by this run's review. |
| 2 | A schema-layer violation carries the same protovalidate constraint ID and message as the boundary interceptor, so a caller cannot tell which layer caught it (SC2/VAL-07) | ⚠️ PARTIAL | The three previously-falsifying counterexamples (CR-01 message-rule carriers, CR-02 `ignore`, CR-03 `Exclude`/`Override`) are now closed and independently re-verified live in this pass (see Gaps Closed). A fourth, independent counterexample — `buf.validate.oneof`/`OneofRules` — was newly surfaced by this run's code review and independently confirmed by this verifier; it remains open. See Gaps. |
| 3 | Message-level rules stay boundary-only unless `WithMessageRules(OnCreate)`; hook ordering documented and tested; boundary validator built once per process (SC3/VAL-08/VAL-09/VAL-10) | ✓ VERIFIED | `WithMessageRules(trigger)` now stores and validates its argument (WR-04 closed, `option.go` `messageRulesSet`/`messageRulesTrigger` confirmed); the `cel`, `cel_expression`, and `oneof` (`MessageRules.oneof`) carriers all flow through the single D-10 gate (`messageRuleReferences`, confirmed present and tested); hook-ordering/once-per-process tests unchanged from prior pass and not disputed. |
| 4 | CI fails when `mixinforproto`'s and `entconnect`'s resolved protovalidate/cel-go versions diverge (SC4/VAL-11) | ✓ VERIFIED | `check-dep-parity` unchanged from prior pass; not disputed. |
| 5 | A conformance corpus golden-asserts every field-mapping rule and protovalidate constraint class; a differential harness feeds random values through every corpus message asserting `protovalidate verdict == ent mutation verdict` for field-scoped rules (SC5/PIPE-05/PIPE-06) | ⚠️ PARTIAL | The harness now exercises `ignore`, `cel_expression`, and `MessageRules.oneof` fixtures (all run live in this pass, all green) — the three previously-blind rule shapes are now covered. But the harness (and the corpus generally) has no fixture and no coverage claim anywhere for `buf.validate.oneof`/`OneofRules`, so "the harness proves parity" still cannot be claimed for that rule shape. |
| 6 | Reverse conversion table complete for every derivation kind, fails closed on conversion faults (VAL-04) | ✓ VERIFIED | Unchanged from prior pass; not disputed. |
| 7 | Real ent client genuinely invokes the mixin hook in the real mutation path (D-13) | ✓ VERIFIED | Unchanged from prior pass; not disputed. |
| 8 | VAL-07's single-`ValidationError`-construction-site invariant is enforced by CI, not merely documented (WR-07) | ✓ VERIFIED | `check-single-validationerror-site` exists, is wired into `Makefile`/`scripts/pipeline.sh`/`.github/workflows/ci.yml`, and was run live in this pass against the working tree: exits 0, names exactly one production site (`mixinforproto/violation.go:64`). |

**Score:** 7/8 truths verified (1 partial counted as failed for scoring purposes — truth #2's `OneofRules` gap and truth #5's corresponding blind spot are two facets of the same root-cause gap, tracked as one gap entry below)

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `mixinforproto/hooks.go` | Ignore-aware, option-aware `buildHookState`/`evaluate` | ✓ VERIFIED | `GetIgnore()`, `Ignore_IGNORE_ALWAYS`, `isZeroForKind`, and `isExcluded(name) \|\| isOverridden(name)` (placed before `ResolveFieldRules`) all confirmed present by direct read and by the passing `TestHooksGo_ExcludedOverriddenCheckPrecedesResolveFieldRules` structural test. |
| `mixinforproto/messagerules.go` | Full three-carrier (`cel`/`cel_expression`/`oneof`) D-10 schema-load reference gate | ✓ VERIFIED | `GetCelExpression()`, `GetOneof()` (the `MessageOneofRule` list) both confirmed present and read via `messageRuleReferences`; distinct from the still-missing `OneofRules` extension (see Gaps). |
| `mixinforproto/violation.go` | Single shared `*protovalidate.ValidationError` constructor | ✓ VERIFIED | Unchanged; the sole site named by `check-single-validationerror-site`'s live-run output. |
| `Makefile` (`check-single-validationerror-site`) | WR-07 gate | ✓ VERIFIED | Present, `.PHONY`-registered, run live in this pass, exits 0. |
| `mixinforproto/messagerules_test.go` / `hooks_test.go` | Declaration-surface exhaustiveness guards | ✓ VERIFIED, but ⚠️ SCOPED NARROWER THAN THE FULL PROTOVALIDATE DECLARATION SURFACE | Both guards run live and pass; each walks one message descriptor (`MessageRules`, `FieldRules`) and neither walks `OneofRules`, which is a structurally separate message — the guards' own reflective design cannot see a member of a message they never enumerate. |
| `mixinforproto/internal/difftest/{ignore_test.go,optionsuppression_test.go,messagerules_test.go}` | Real-`ent.Client` differential proofs for `ignore`, `Override`/`Exclude`, `cel_expression`, `MessageRules.oneof` | ✓ VERIFIED | All run live in this pass, all pass. No corresponding fixture exists for `OneofRules`. |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|----|--------|---------|
| `mixinforproto/hooks.go` | `mixinforproto/option.go` | `o.isExcluded`/`o.isOverridden` consulted before `ResolveFieldRules` in the per-field loop | ✓ WIRED | Confirmed by `grep -n` line-number ordering: `isExcluded(` at line 230, `ResolveFieldRules(` at line 234. Previously recorded NOT WIRED (CR-03) — now closed. |
| `mixinforproto/hooks.go` | `validate.FieldRules.GetIgnore()` | compile-time (`IGNORE_ALWAYS`) and mutation-time (`IGNORE_IF_ZERO_VALUE`) gates | ✓ WIRED | Confirmed present at lines 278/281 and 640. Previously absent (CR-02) — now closed. |
| `mixinforproto/messagerules.go` | `validate.MessageRules.GetCelExpression()`/`GetOneof()` | `messageRuleReferences` normalization | ✓ WIRED | Confirmed present at lines 137/275/292. Previously absent (CR-01) — now closed. |
| `mixinforproto/derive.go` | `protovalidate.ResolveOneofRules` | (expected: per-oneof provenance recording) | ✗ NOT WIRED | Confirmed absent — `grep -rn 'ResolveOneofRules' mixinforproto/*.go` returns nothing. This is the new gap. |
| `Makefile check-single-validationerror-site` | `scripts/pipeline.sh` / `.github/workflows/ci.yml` | three-way wiring matching `check-dep-parity`'s precedent | ✓ WIRED | Confirmed: `scripts/pipeline.sh` step 6/6, one CI step in the `modules` job. Previously absent (WR-07) — now closed. |

### Data-Flow Trace (Level 4)

Not applicable in the UI-rendering sense — this phase's "data flow" is the boundary-vs-storage
violation-identity comparison, exercised directly by the differential harness tests run live above
(`TestSweep_DriverlessDifferential`, `TestIgnoreAlways_*`, `TestOptionSuppression_*`,
`TestMessageRuleCelExpression_*`, `TestMessageRuleOneof_*`) rather than by a separate trace.

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| CR-02/CR-03 fix wired (ignore, Exclude/Override honored at storage) | `go test ./internal/difftest/... -run 'TestIgnore\|TestOptionSuppression' -v` | All PASS | ✓ PASS |
| CR-01 fix wired (cel_expression, MessageRules.oneof honored at schema-load gate + storage) | `go test ./internal/difftest/... -run 'TestMessageRuleCelExpression\|TestMessageRuleOneof' -v` | All PASS | ✓ PASS |
| Declaration-surface exhaustiveness guards run and pass | `go test . -run 'TestFieldRulesDeclarationSurfaceIsFullyHandled\|TestMessageRulesDeclarationSurfaceIsFullyHandled' -v` | Both PASS | ✓ PASS |
| WR-07 gate enforces the single-construction-site invariant | `make check-single-validationerror-site` | `OK: ... constructed at exactly one production site: ./mixinforproto/violation.go`, exit 0 | ✓ PASS |
| PIPE-06 differential sweep still green | `go test ./internal/difftest/... -run TestSweep -v` | All PASS | ✓ PASS |
| Corpus coverage/constraint-class guards still green | `go test . -run 'TestCorpusMessagesHaveRecordedCoverage\|TestCorpusExercisesEveryProtovalidateConstraintClass' -v` | Both PASS | ✓ PASS |
| `OneofRules` resolved anywhere in the module | `grep -rn 'ResolveOneofRules\|BoundaryOnlyOneof' mixinforproto/*.go` | No matches | ✗ FAIL (confirms the gap) |

### Probe Execution

No `scripts/*/tests/probe-*.sh` convention is used by this project; not applicable.

### Requirements Coverage

| Requirement | Source Plan | Status | Evidence |
|-------------|-------------|--------|----------|
| VAL-04 | 03-01, 03-02, 03-03, 03-05, 03-06 | ✓ SATISFIED | Hook compiles/evaluates residual + standard rules once at schema load; reverse table complete; ignore/Exclude/Override now correctly gated. |
| VAL-05 | 03-01 | ✓ SATISFIED | Unchanged from prior pass. |
| VAL-06 | 03-01, 03-07 | ✓ SATISFIED | `newValidationError` constructs structured errors outside RPC context; single-construction-site now CI-enforced. |
| VAL-07 | 03-01, 03-03, 03-04, 03-06, 03-07, 03-08 | ⚠️ PARTIALLY BLOCKED | CR-01/CR-02/CR-03/WR-07 all closed and re-verified; the new `OneofRules` gap is a fourth, independent counterexample to the same identity guarantee. |
| VAL-08 | 03-03, 03-05, 03-08 | ⚠️ PARTIALLY BLOCKED | `cel`, `cel_expression`, `MessageRules.oneof` carriers now all correctly gated and trigger-honoring; `OneofRules` (the distinct top-level extension) is outside `WithMessageRules`'s scope entirely and has no gate of any kind. |
| VAL-09 | 03-04 | ✓ SATISFIED | Unchanged from prior pass. |
| VAL-10 | 03-04 | ✓ SATISFIED | Unchanged from prior pass. |
| VAL-11 | 03-04 | ✓ SATISFIED | Unchanged from prior pass. |
| PIPE-05 | 03-02, 03-05, 03-06, 03-08 | ⚠️ PARTIALLY BLOCKED | Constraint-class coverage guard extended and green for `ignore`/`cel_expression`/`MessageRules.oneof`; still has zero coverage of `OneofRules`, and neither declaration-surface guard can see that message. |
| PIPE-06 | 03-01, 03-05, 03-06, 03-08 | ⚠️ PARTIALLY BLOCKED | Differential harness now exercises `ignore`, `Override`/`Exclude`, `cel_expression`, `MessageRules.oneof` — no longer blind to any of the three originally-reported gap classes. Still blind to `OneofRules`. |

No orphaned requirements: all ten IDs (VAL-04..VAL-11, PIPE-05, PIPE-06) appear in REQUIREMENTS.md's
Phase 3 mapping and are each claimed by at least one plan's `requirements:` frontmatter across the
now-eight plans in this phase.

**Note (documentation drift, not a code gap):** `.planning/REQUIREMENTS.md`'s checklist section
still shows `VAL-09`/`VAL-10`/`VAL-11` as unchecked (`- [ ]`) even though its own Traceability table
and this verification (and the prior one) both mark them Complete/SATISFIED — a stale checkbox from
before the phase's gap-closure work, not evidence of an unresolved requirement. Recommend updating
the checkboxes in a documentation pass; not blocking.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| `mixinforproto/derive.go`, `mixinforproto/hooks.go` | (module-wide) | `OneofRules` (`buf.validate.oneof`) is a silently unhandled rule carrier — no storage enforcement, no `BoundaryOnly` provenance, no exhaustiveness guard | 🛑 Blocker | See Gaps. Same class as the now-closed CR-01/CR-02/CR-03, independently confirmed by this verifier. |
| `mixinforproto/reverse.go:217-224` | `reverseEnum` | Interpolates the rejected mutation value (`%q`, `name`) into its returned error text, unlike every sibling reverse-conversion function | ⚠️ Warning | Non-blocking per 03-REVIEW.md (WR-01): currently masked by `runtime.MapError`'s default-case redaction inside this stack, but `mixinforproto` is designed for standalone adoption with no dependency on `runtime` — a standalone consumer propagating this error verbatim would leak a rejected value across a trust boundary. Not targeted by any of this wave's gap-closure plans; recommend a follow-up fix but does not block this phase's goal (VAL-06's "no value-embedding" invariant is about the boundary/storage identity guarantee's OTHER violation-message paths, which the gap-closure plans' own tests confirm remain clean). |
| `mixinforproto/README.md:47-51,213-224` | — | README still states `Exclude`/`Override` are "not yet available" and omits `Exclude`/`Override`/`WithMessageRules`/`OnCreate`/`MessageRuleTrigger` from the API reference table, despite all being shipped, exported, and load-bearing as of this phase | ⚠️ Warning | Non-blocking per 03-REVIEW.md (WR-02): a documentation accuracy issue, not a code-behavior gap. Not targeted by this wave's gap-closure plans. |

No `TBD`/`FIXME`/`XXX` unresolved debt markers found in this phase's changed files (03-06/03-07/03-08's own `key-files` lists, spot-checked directly).

### Human Verification Required

None. The one outstanding gap (`OneofRules`) is code-level and deterministically confirmed by
static read against the vendored `proto/buf/validate/validate.proto` plus a repo-wide grep showing
zero calls to `protovalidate.ResolveOneofRules` — no human judgment is needed to confirm it.

### Gaps Summary

Five of the prior verification's six recorded deficiencies (CR-01, CR-02, CR-03, WR-07, and VAL-08's
CR-01-driven partial) are closed and independently re-verified in this pass by direct code read and
live test execution — not merely accepted from the 03-06/03-07/03-08 SUMMARYs. The
`hooks.go -> option.go` key link previously recorded NOT WIRED is now wired and pinned by a
dedicated structural test; the WR-07 gate exists, is three-way wired, and was run live against the
working tree; all three previously-blind rule shapes (`ignore`, `Exclude`/`Override`,
`cel_expression`/`MessageRules.oneof`) now have real-`ent.Client` differential proofs that pass.

But this phase is not yet goal-achieved. This run's own code review surfaced — and this verifier
independently confirmed by direct source read — a fourth, structurally distinct counterexample to
Success Criterion 2's "a caller cannot tell which layer caught it" promise: `buf.validate.oneof`
(`OneofRules`, extending `google.protobuf.OneofOptions`) is a rule carrier this codebase has never
handled, at any point in this phase's eight plans, and that its own newly-built exhaustiveness
guards (Guard A/B, closed by 03-08 specifically to prevent exactly this failure mode) cannot detect,
because each guard walks a different protobuf message's descriptor and `OneofRules` is a third,
separate message neither guard enumerates. The gap is real, reproducible without any human judgment,
and falls inside this phase's own invariant #5. It was not part of the prior verification's gap set
and so was not a target of 03-06/03-07/03-08 — it is newly surfaced, not a regression of closed work.

This phase cannot be marked goal-achieved until this gap is either closed (resolve `OneofRules` via
`protovalidate.ResolveOneofRules`, record provenance, add a corpus fixture and a third
declaration-surface guard) or explicitly, deliberately scoped out with a written design reason and a
named test asserting the omission is intentional — the same discipline this very codebase already
applies to every other documented exception (`constraintClassExceptions`).

---

_Verified: 2026-08-15_
_Verifier: Claude (gsd-verifier)_
