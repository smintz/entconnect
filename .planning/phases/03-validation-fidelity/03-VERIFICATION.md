---
phase: 03-validation-fidelity
verified: 2026-08-14T00:00:00Z
status: gaps_found
score: 5/8 must-haves verified
behavior_unverified: 0
overrides_applied: 0
gaps:
  - truth: "A schema-layer violation carries the same protovalidate constraint ID and message as the boundary interceptor would produce ... so a caller cannot tell which layer caught it (ROADMAP Phase 3 Success Criterion 2 / VAL-07)"
    status: failed
    reason: "CR-01: MessageRules.cel_expression and MessageRules.oneof are not enumerated by D-10's schema-load reference gate (checkMessageRuleReferences walks only msgRules.GetCel()). A message-level rule declared via either alternate carrier bypasses the gate entirely: schema load succeeds even when the rule references an Excluded/Override'd field, and the storage layer evaluates the rule against a fabricated proto3-zero value rather than real mutation data — a phantom verdict where boundary accepts and storage rejects (or vice versa) for the same input."
    artifacts:
      - path: "mixinforproto/messagerules.go"
        issue: "Line 96 (`if msgRules == nil || len(msgRules.GetCel()) == 0`) and line 116 (`for _, r := range msgRules.GetCel()`) only inspect the `cel` carrier of buf.validate.MessageRules; `cel_expression` (repeated string, field 5) and `oneof` (repeated MessageOneofRule, field 4) are real, generated (GetCelExpression()/GetOneof() confirmed present in the pinned protovalidate module) but never read anywhere in this file."
    missing:
      - "Enumerate cel_expression and oneof carriers in checkMessageRuleReferences, folding their field references into the same D-10 gate and extraFields seeding as the cel carrier."
      - "A corpus fixture in proto/mixinforprototest/v1/messagerules.proto exercising cel_expression and/or oneof so PIPE-06's sweep can detect a regression here."
  - truth: "(same SC2/VAL-07 identity guarantee, second independent counterexample) — (buf.validate.field).ignore is honored identically at both layers"
    status: failed
    reason: "CR-02: buildHookState (compiling local CEL programs) and evaluate (running them) never read rules.GetIgnore(). protovalidate's own evaluator honors IGNORE_ALWAYS/IGNORE_IF_ZERO_VALUE; the locally-compiled CEL half does not. For a field with ignore=IGNORE_ALWAYS plus a cel rule, the boundary accepts (ignore correctly suppresses the rule) while storage rejects (the local half runs the rule anyway) — confirmed present in code: no GetIgnore reference exists in hooks.go, and fieldmap.go's own boundaryOnlyNonConstraintFields list already names \"ignore\" as a known field the storage layer must special-case, which it does not."
    artifacts:
      - path: "mixinforproto/hooks.go"
        issue: "buildHookState's per-field loop (lines ~227-248) compiles every rules.GetCel() entry into a celProgram unconditionally; evaluate's CEL-evaluation loop (lines ~526-549) runs every compiled program unconditionally. Neither site checks rules.GetIgnore()."
    missing:
      - "Gate local CEL program compilation/evaluation on rules.GetIgnore() (skip entirely for IGNORE_ALWAYS; skip when the reverse-converted value is the type's zero for IGNORE_IF_ZERO_VALUE)."
      - "A corpus fixture using (buf.validate.field).ignore paired with a cel rule so the PIPE-06 sweep is no longer blind to this divergence (grep -rn ignore proto/mixinforprototest/ currently returns nothing)."
  - truth: "Override(...) 'suppresses validation relay for that field entirely' (option.go's own documented Override semantics) and the BoundaryOnlyOverridden annotation this phase emits is accurate"
    status: failed
    reason: "CR-03, independently reproduced by this verifier (not just cited from the code review): buildHookState's per-field loop never calls o.isExcluded(name) or o.isOverridden(name) before building an evaluator entry, so a field named in Exclude(...) or Override(...) is still compiled into hs.evaluators and still enforced at the storage layer. This directly contradicts option.go's documented Override semantics and the BoundaryOnlyOverridden provenance derive.go records for Phase 5's drift check — the annotation says 'boundary-only', the hook enforces it anyway. Separately, a type-changing Override (e.g. Override(\"both\", field.Bool(\"both\")) on a string-typed proto field, a combination derive_test.go already exercises as supported) hands reverseValue a bool for a StringKind descriptor, producing a D-12 data-integrity error that runtime.MapError maps to CodeInternal — a permanent HTTP 500 on every write to that entity, from a schema that loads without complaint."
    artifacts:
      - path: "mixinforproto/hooks.go"
        issue: "The per-field loop in buildHookState (~lines 191-259) walks md.Fields() and resolves rules/class for every field with no Exclude/Override check, unlike messageRuleFieldUnavailable (messagerules.go:185-190) which does check both — the codebase's own asymmetry confirms this was an oversight, not a design choice."
    missing:
      - "Skip o.isExcluded(name) || o.isOverridden(name) fields in buildHookState's per-field loop, before rule resolution."
      - "Unit tests pinning 'an overridden field produces zero storage-layer violations' and 'an excluded field produces no evaluator entry'."
    reproduced_by_verifier: true
    reproduction_evidence: |
      Two probe tests added to mixinforproto/zz_probe_test.go, run, and removed (working tree confirmed clean after removal, no source modified):

      TestZZProbe_OverrideStillEnforced — MixedFieldRules with Override("both", field.String("both")),
      mutation value "nope" (fails the field's own cel rule "this.startsWith('X')"):
        violations for overridden field 'both': 2
          ruleID=constraints.mixed_field_rules.both.starts_with_x field=both msg=value must start with X
          ruleID=constraints.mixed_field_rules.both.starts_with_x field=both msg=value must start with X
        FAIL: got 2 violations, want 0

      TestZZProbe_ExcludeStillEnforced — same fixture with Exclude("both"):
        violations for excluded field 'both': 2
        FAIL: got 2 violations, want 0
  - truth: "VAL-07's single-ValidationError-construction-site invariant is enforced by CI, not merely documented (WR-07)"
    status: failed
    reason: "violation_test.go:262-269 cites 'Makefile's grep-based check for the authoritative, whole-repo version of this assertion' as backing the invariant that violation.go is the only file constructing a *protovalidate.ValidationError{. No such Makefile target exists. Makefile's .PHONY list is 'build vet test test-determinism test-standalone check-modules check-stubs check-goversion check-dep-parity pipeline' — confirmed by direct read, no check-single-validationerror-site or equivalent target present, and grep -rn 'protovalidate.ValidationError{' Makefile returns nothing. A comment claiming a gate exists when none does is worse than no comment: it tells future reviewers/planners the invariant is enforced when it is not."
    artifacts:
      - path: "mixinforproto/violation_test.go"
        issue: "Lines 262-269 reference a nonexistent Makefile target as the authoritative enforcement mechanism for VAL-07's identity invariant."
      - path: "Makefile"
        issue: "No grep-based single-construction-site check exists anywhere in the file."
    missing:
      - "Add the Makefile target the test comment already describes (or an equivalent CI step), or correct the comment to stop claiming enforcement that doesn't exist."
deferred: []
human_verification: []
---

# Phase 3: Validation Fidelity Verification Report

**Phase Goal:** Residual and message-level validation rules that Tier 1 can't translate get executed with byte-identical results at both the RPC boundary and the storage layer, proven by an automated differential harness

**Verified:** 2026-08-14
**Status:** gaps_found
**Re-verification:** No — initial verification

## Goal Achievement

This phase's central claim — Success Criterion 2, "a schema-layer violation carries the same
protovalidate constraint ID and message as the boundary interceptor would produce ... so a
caller cannot tell which layer caught it" — is **falsified** by three independently reproducible
counterexamples. Two (CR-01 message-rule carriers, CR-02 `ignore`) were confirmed by static
read of the pinned protovalidate module plus the hook code; the third (CR-03 `Override`/`Exclude`
enforcement) was independently reproduced by this verifier with two fresh probe tests run against
the real corpus fixture and then deleted, leaving the working tree clean. `03-REVIEW.md`'s
narrative for all three matches what the code actually does.

The full test suite passing is not evidence against these gaps — none of the three has a corpus
fixture that would exercise it (no `ignore` fixture exists anywhere in
`proto/mixinforprototest/`; no `cel_expression`/`oneof` message-rule fixture exists; and no test
in the existing suite calls `Override`/`Exclude` together with a hook-bearing schema and then
asserts zero violations). The gaps are real and the differential harness this phase was built to
deliver is currently blind to all three.

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | Mixin hook evaluates the full protovalidate field-rule set for every in-scope field, standard and residual, compiled once at schema load (SC1) | ✓ VERIFIED | `hooks.go` `buildHookState` precompiles one `protovalidate.Validator` (`WithDisableLazy`) plus per-field `cel.Program`s at schema-load time only (never inside the returned `ent.Hook` closure — confirmed by reading `hook()`/`evaluate()`); `standardValidatorBuildCount`/`celCompileCount` test seams exist and are exercised by `hooks_test.go`'s `TestBuildHookState_StandardValidatorBuiltOnceAtConstruction`. |
| 2 | A schema-layer violation carries the same protovalidate constraint ID and message as the boundary interceptor, so a caller cannot tell which layer caught it (SC2/VAL-07) | ✗ FAILED | Falsified three independent ways: CR-01 (message-rule `cel_expression`/`oneof` bypass), CR-02 (`ignore` not honored locally), CR-03 (`Override`/`Exclude` not honored, independently reproduced). See Gaps. |
| 3 | Message-level rules stay boundary-only unless `WithMessageRules(OnCreate)`; hook ordering documented and tested; boundary validator built once per process (SC3/VAL-08/VAL-09/VAL-10) | ⚠️ PARTIAL | `WithMessageRules(OnCreate)` gate itself works for the `cel` carrier (`hooks_test.go` `TestEvaluate_MessageLevelRulesStayBoundaryOnlyByDefault`) and hook-ordering/once-per-process are independently tested (`internal/entconnecttest/hookwiring/ordering_test.go`, `runtime/interceptor_test.go`) — but the opt-in's D-10 schema-load gate is incomplete per CR-01, so "stays boundary-only unless opted in, and the opt-in is safe" does not fully hold for `cel_expression`/`oneof`-declared message rules. |
| 4 | CI fails when `mixinforproto`'s and `entconnect`'s resolved protovalidate/cel-go (and related) versions diverge (SC4/VAL-11) | ✓ VERIFIED | `Makefile`'s `check-dep-parity` target (confirmed present, `DEP_PARITY_MODULES` explicit five-module set, `GOWORK=off` on both `go list -m` calls, fails loudly when a module is absent from either go.mod) is wired into CI's standalone job per `03-04-SUMMARY.md` and `.github/workflows/ci.yml`. |
| 5 | A conformance corpus golden-asserts every field-mapping rule and protovalidate constraint class; a differential harness feeds random values through every corpus message asserting `protovalidate verdict == ent mutation verdict` for field-scoped rules (SC5/PIPE-05/PIPE-06) | ⚠️ PARTIAL | The harness (`internal/difftest/sweep_test.go`, `corpus_test.go`'s constraint-class coverage guard) exists, runs, and is green — but it is structurally blind to all three CR gaps (no `ignore` fixture, no `cel_expression`/`oneof` message-rule fixture, no `Override`+hook-bearing-schema-produces-zero-violations case), so "the harness proves parity" cannot be claimed for the rule shapes those gaps cover. |
| 6 | Reverse conversion table complete for every derivation kind, fails closed on conversion faults (VAL-04, Plan 03-02) | ✓ VERIFIED | `mixinforproto/reverse.go`'s exhaustive kind switch plus `reverse_test.go`'s per-kind round-trip and D-12 fail-closed tests; `03-02-SUMMARY.md` claims match file contents read. |
| 7 | Real ent client genuinely invokes the mixin hook in the real mutation path (D-13, Plan 03-04) | ✓ VERIFIED | `internal/entconnecttest/hookwiring/wiring_test.go` drives a real HTTP Connect server + real sqlite ent.Client; file exists, is substantive (not a stub), and is exercised by `make test` (environment note: `make test` passes at HEAD). |
| 8 | VAL-07's single-`ValidationError`-construction-site invariant is enforced by CI (WR-07) | ✗ FAILED | `violation_test.go:262-269` cites a Makefile grep-based check that does not exist. Confirmed: `.PHONY` list has no such target, `grep -rn 'protovalidate\.ValidationError{' Makefile` is empty. |

**Score:** 5/8 truths verified (3 failed as blockers, 0 present-but-behavior-unverified)

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `mixinforproto/hooks.go` | Complete D-07 hybrid: schema-load-compiled validator + CEL programs, mutation-time evaluate() | ✓ VERIFIED (exists, substantive, wired) but ⚠️ contains the CR-01(partially)/CR-02/CR-03 defects | `buildHookState`/`evaluate` present and wired into `mixin.go`'s `Hooks()`; defects are logic gaps, not missing wiring. |
| `mixinforproto/messagerules.go` | D-10's schema-load reference gate, enumerating all message-rule carriers | ✗ INCOMPLETE | Only walks `GetCel()`; `GetCelExpression()`/`GetOneof()` never read (CR-01). |
| `mixinforproto/violation.go` | Single shared `*protovalidate.ValidationError` constructor, deterministic ordering/dedup | ✓ VERIFIED | `newValidationError`, `sortViolations` present per `03-03-SUMMARY.md` claims; not independently disputed by the review. |
| `mixinforproto/reverse.go` | Complete reverse conversion table | ✓ VERIFIED | Confirmed via file read and `03-02-SUMMARY.md` cross-check. |
| `internal/entconnecttest/hookwiring/{wiring_test.go,ordering_test.go}` | D-13 wiring proof, VAL-09 ordering proof | ✓ VERIFIED | Both files present in `03-REVIEW.md`'s files-reviewed list and referenced by `03-04-SUMMARY.md`'s key-files; not disputed. |
| `Makefile` (`check-dep-parity`) | VAL-11 gate | ✓ VERIFIED | Present, confirmed by direct read (`check-dep-parity:` target with `DEP_PARITY_MODULES` loop). |
| `mixinforproto/internal/difftest/sweep_test.go` | PIPE-06 differential sweep | ✓ VERIFIED (exists, wired, runs) but ⚠️ HOLLOW for the three gap classes | Runs and passes, but has no fixture that would surface CR-01/CR-02/CR-03 — coverage gap, not a wiring gap. |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|----|--------|---------|
| `mixinforproto/mixin.go` | `mixinforproto/hooks.go` | `protoMixin[M].Hooks()` | ✓ WIRED | Confirmed by `mixin.go` read (not disputed by review or this verifier's reading). |
| `mixinforproto/hooks.go` | `mixinforproto/messagerules.go` | `checkMessageRuleReferences` called from `buildHookState` when `messageRulesOnCreate` | ✓ WIRED (but with the CR-01 gap inside `checkMessageRuleReferences` itself) | Call site at `hooks.go:182` confirmed. |
| `mixinforproto/hooks.go` | `mixinforproto/option.go` | `o.isExcluded`/`o.isOverridden` consulted in the per-field loop | ✗ NOT WIRED | Confirmed absent — this is CR-03. `messageRuleFieldUnavailable` (messagerules.go) calls both; `buildHookState`'s own per-field loop calls neither. |
| `internal/entconnecttest/hookwiring/wiring_test.go` | `mixinforproto/hooks.go` | Real Connect Create request through real ent.Client | ✓ WIRED | Per file existence and `03-04-SUMMARY.md`'s described assertions; consistent with `03-REVIEW.md`'s files-reviewed list containing this file with no finding against it. |

### Requirements Coverage

| Requirement | Source Plan | Status | Evidence |
|-------------|-------------|--------|----------|
| VAL-04 | 03-01, 03-02, 03-03, 03-05 | ✓ SATISFIED | Hook compiles/evaluates residual + standard rules once at schema load; reverse table complete. |
| VAL-05 | 03-01 | ✓ SATISFIED | Operation-dependent scope via single `m.Fields()` read, empirically proven (03-01-SUMMARY tracer test). |
| VAL-06 | 03-01 | ✓ SATISFIED | `newValidationError` constructs structured errors outside RPC context; no value-embedding in messages (spot check: `newFieldViolation`/`celResultToViolation` never interpolate the rejected value). |
| VAL-07 | 03-01, 03-03, 03-04 | ✗ BLOCKED | Falsified by CR-01/CR-02/CR-03 (identity guarantee does not hold for these rule shapes) and WR-07 (invariant claimed-but-unenforced). |
| VAL-08 | 03-03, 03-05 | ⚠️ PARTIALLY BLOCKED | `cel` carrier works; `cel_expression`/`oneof` carriers bypass the opt-in gate (CR-01). |
| VAL-09 | 03-04 | ✓ SATISFIED | Relative-ordering test across Policed/Unpoliced fixtures, confirmed present. |
| VAL-10 | 03-04 | ✓ SATISFIED | `runtime/interceptor_test.go`'s construction-count assertions, confirmed present. |
| VAL-11 | 03-04 | ✓ SATISFIED | `check-dep-parity` Makefile target confirmed, wired into CI per summary. |
| PIPE-05 | 03-02, 03-05 | ✓ SATISFIED | Constraint-class coverage guard (`corpus_test.go`) confirmed present per file list. |
| PIPE-06 | 03-01, 03-05 | ⚠️ PARTIALLY BLOCKED | Sweep exists and runs, but is structurally blind to the three gap classes above — "differential harness proves parity" cannot be claimed for `ignore`, `cel_expression`/`oneof` message rules, or `Override`+hook interaction. |

No orphaned requirements: all ten IDs (VAL-04..VAL-11, PIPE-05, PIPE-06) appear in REQUIREMENTS.md's Phase 3 mapping and are each claimed by at least one of the five plans' `requirements:` frontmatter.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| `mixinforproto/violation_test.go` | 262-269 | Comment claims a CI/Makefile enforcement mechanism that does not exist | 🛑 Blocker | Directly undermines confidence in VAL-07's core invariant; a reviewer reading the comment believes the invariant is machine-enforced when it is not (WR-07). |
| `mixinforproto/messagerules.go` | 96, 116 | Incomplete enumeration of a proto message's declared rule carriers (`cel` only, not `cel_expression`/`oneof`) | 🛑 Blocker | CR-01 — phantom verdicts. |
| `mixinforproto/hooks.go` | 227-248, 526-549 | `rules.GetIgnore()` never consulted | 🛑 Blocker | CR-02 — divergent verdicts for `ignore`-carrying fields. |
| `mixinforproto/hooks.go` | 191-259 | `o.isExcluded`/`o.isOverridden` never consulted in the per-field loop | 🛑 Blocker | CR-03 — Override/Exclude semantics violated; a type-changing Override causes a permanent CodeInternal 500. |
| `mixinforproto/option.go` | 157-161 | `WithMessageRules(trigger)` discards `trigger` entirely | ⚠️ Warning | WR-04 in review — not independently re-verified in depth by this pass but consistent with direct code read (function body only sets `o.messageRules = true`, never inspects `trigger`). |

No `TBD`/`FIXME`/`XXX` unresolved debt markers were found in the phase's changed files (per `03-REVIEW.md`'s scope; not independently re-scanned in full — the review's file list and this verifier's targeted reads did not surface any).

### Human Verification Required

None. All three blocking gaps are code-level, deterministically reproducible (two by static read against the pinned protovalidate module's generated getters, one independently reproduced with a probe test by this verifier), and require no human judgment to confirm.

### Gaps Summary

The phase delivered substantial, well-tested machinery (reverse conversion, real-client wiring,
hook ordering, dependency-parity CI gate, differential sweep infrastructure) — six of eight
observable truths hold cleanly. But the phase's own headline guarantee, Success Criterion 2 ("a
caller cannot tell which layer caught it"), fails for three concrete, non-overlapping rule
shapes: protovalidate's `cel_expression`/`oneof` message-rule carriers (CR-01), the `ignore`
field option (CR-02), and the `Exclude`/`Override` mixin options (CR-03) — the last of which this
verifier independently reproduced with fresh probe tests against the real `MixedFieldRules`
corpus fixture, not merely accepted from the code review's narrative. A fourth item, WR-07,
means the one invariant meant to structurally prevent a second `ValidationError`-construction
site from silently reintroducing this class of bug is enforced by nothing, despite test comments
claiming otherwise.

All four gaps share one root cause: `buildHookState`'s per-field loop and
`checkMessageRuleReferences`'s carrier walk each reason about a narrow slice of protovalidate's
rule-declaration surface (the single `cel` carrier; the descriptor alone, never the `Option` set)
rather than the full surface the boundary interceptor's own protovalidate evaluator already
handles correctly. None of the three CR gaps has a corpus fixture that would surface it in the
existing green test suite — the PIPE-06 differential harness this phase exists to deliver is
therefore not exercising the exact rule shapes where boundary/storage disagreement is most likely.

These are must-fix items before this phase can be considered goal-achieved: the roadmap's stated
success criterion is a byte-identical-verdict guarantee, and three concrete, reproducible
counterexamples exist in the current codebase.

---

_Verified: 2026-08-14_
_Verifier: Claude (gsd-verifier)_
