---
phase: 03-validation-fidelity
reviewed: 2026-08-15T12:50:52Z
depth: standard
files_reviewed: 55
files_reviewed_list:
  - .claude/CLAUDE.md
  - .github/workflows/ci.yml
  - Makefile
  - internal/entconnecttest/hookwiring/ent/entc.go
  - internal/entconnecttest/hookwiring/ent/generate.go
  - internal/entconnecttest/hookwiring/ent/schema/policed.go
  - internal/entconnecttest/hookwiring/ent/schema/recorder.go
  - internal/entconnecttest/hookwiring/ent/schema/unpoliced.go
  - internal/entconnecttest/hookwiring/ordering_test.go
  - internal/entconnecttest/hookwiring/wiring_test.go
  - mixinforproto/README.md
  - mixinforproto/annotation.go
  - mixinforproto/corpus_test.go
  - mixinforproto/derive.go
  - mixinforproto/derive_test.go
  - mixinforproto/doc.go
  - mixinforproto/fieldmap.go
  - mixinforproto/go.mod
  - mixinforproto/hooks.go
  - mixinforproto/hooks_test.go
  - mixinforproto/internal/boundarytest/boundary_test.go
  - mixinforproto/internal/boundarytest/ent/schema/boundaryonly.go
  - mixinforproto/internal/difftest/ent/schema/ignorerules.go
  - mixinforproto/internal/difftest/ent/schema/messagerules.go
  - mixinforproto/internal/difftest/ent/schema/mixedrules.go
  - mixinforproto/internal/difftest/ent/schema/overriddenrules.go
  - mixinforproto/internal/difftest/ent/schema/residualcel.go
  - mixinforproto/internal/difftest/fakedriver.go
  - mixinforproto/internal/difftest/generate.go
  - mixinforproto/internal/difftest/hybrid_test.go
  - mixinforproto/internal/difftest/ignore_test.go
  - mixinforproto/internal/difftest/messagerules_test.go
  - mixinforproto/internal/difftest/optionsuppression_test.go
  - mixinforproto/internal/difftest/sweep_test.go
  - mixinforproto/internal/difftest/tracer_test.go
  - mixinforproto/messagerules.go
  - mixinforproto/messagerules_test.go
  - mixinforproto/mixin.go
  - mixinforproto/option.go
  - mixinforproto/reverse.go
  - mixinforproto/reverse_test.go
  - mixinforproto/violation.go
  - mixinforproto/violation_test.go
  - proto/buf.yaml
  - proto/mixinforprototest/v1/constraints.proto
  - proto/mixinforprototest/v1/ignore.proto
  - proto/mixinforprototest/v1/messagerules.proto
  - proto/mixinforprototest/v1/messages.proto
  - proto/mixinforprototest/v1/reverse.proto
  - proto/entconnecttest/v1/hookwiring.proto
  - runtime/doc.go
  - runtime/errormap.go
  - runtime/errormap_test.go
  - runtime/interceptor_test.go
  - scripts/pipeline.sh
findings:
  critical: 1
  warning: 2
  info: 2
  total: 5
status: issues_found
---

# Phase 03: Code Review Report

**Reviewed:** 2026-08-15T12:50:52Z
**Depth:** standard
**Files Reviewed:** 55
**Status:** issues_found

## Summary

This phase implements the hybrid boundary/storage validation evaluator
(`mixinforproto/hooks.go`), message-level rule opt-in (`messagerules.go`),
the ent-value-to-protoreflect reverse table (`reverse.go`), the single
`*protovalidate.ValidationError` construction site (`violation.go`), and
`runtime/errormap.go`'s fault/verdict mapping. The engineering discipline
on display is unusually high: every rule carrier the code *does* handle
(`buf.validate.FieldRules`, `buf.validate.MessageRules`'s three carriers)
has a reflective, descriptor-driven "declaration surface" exhaustiveness
test (`TestFieldRulesDeclarationSurfaceIsFullyHandled`,
`TestMessageRulesDeclarationSurfaceIsFullyHandled`) specifically designed
to catch a future protovalidate release adding a member nobody wires up.
D-03 (no fabricated proto3 zeros), D-12 (no fabricated constraint IDs),
and D-24 (determinism) are each pinned by direct, named tests, and the
driverless differential harness (`internal/difftest/sweep_test.go`)
compares boundary vs. storage verdicts value-by-value across the whole
corpus.

Against that backdrop, one real gap survived the project's own
exhaustiveness discipline: `buf.validate.oneof` (`OneofRules`, e.g.
`(buf.validate.oneof).required` on a real proto `oneof` block) is a third
protovalidate rule-carrier extension (alongside `FieldRules` and
`MessageRules`) that this phase's code never resolves, never records as
`SourceMessage.BoundaryOnly` provenance, and never subjects to a
declaration-surface guard — unlike every other rule category in this
codebase. See CR-01.

Two further items are worth a maintainer's attention: `reverseEnum` is the
one reverse-conversion function that embeds the actual rejected value in
its error text (WR-01), and `mixinforproto/README.md` has drifted out of
sync with `option.go`'s now-fully-shipped `Exclude`/`Override`/
`WithMessageRules` API (WR-02). Two minor code-quality items round out the
findings (IN-01, IN-02).

## Structural Findings (fallow)

No `<structural_findings>` block was provided for this review; this
section is intentionally empty.

## Narrative Findings (AI reviewer)

## Critical Issues

### CR-01: `buf.validate.oneof` (OneofRules) is a silently unhandled rule carrier — no storage enforcement, no BoundaryOnly provenance, no exhaustiveness guard

**File:** `mixinforproto/hooks.go` (buildHookState), `mixinforproto/derive.go` (recordBoundaryOnly/boundaryOnlyRuleIDs), `mixinforproto/messagerules.go`
**Issue:**

protovalidate has three independent places a contract can attach rules:
`buf.validate.field` (`FieldRules`), `buf.validate.message`
(`MessageRules`), and `buf.validate.oneof` (`OneofRules`, extending
`google.protobuf.OneofOptions` — see `proto/buf/validate/validate.proto`
line 109, `option (buf.validate.oneof).required = true;`). This phase
built exhaustive, reflectively-verified handling and coverage guards for
the first two (`hooks_test.go`'s `TestFieldRulesDeclarationSurfaceIsFullyHandled`,
`messagerules_test.go`'s `TestMessageRulesDeclarationSurfaceIsFullyHandled`),
but `OneofRules` itself is never referenced anywhere in production code:

```
$ grep -rln "buf.validate.oneof\|OneofRules\|ResolveOneofRules" --include=*.go --include=*.proto \
  | grep -v /buf/validate/validate.proto
mixinforproto/messagerules.go   # only in a doc comment, never calls it
```

Concretely, a contract declaring:

```proto
message Contact {
  oneof method {
    option (buf.validate.oneof).required = true;
    string email = 1;
    string phone = 2;
  }
}
```

is enforced correctly at the RPC boundary (protovalidate's own evaluator
honors `OneofRules.required`), but at the storage layer:

1. `classify()` routes `email`/`phone` to `classRealOneofMember`;
   `derive.go`'s `checkOneofResolution` (MIX-10) *requires* every member
   of a real oneof to be `Exclude`d or `Override`n before schema load can
   even succeed — so by construction, no real-oneof's members ever reach
   `hs.evaluators` individually. That much is intentional and documented.
2. What is *not* recorded anywhere is the oneof-level `required`
   constraint itself. `recordBoundaryOnly` (derive.go) is called per
   *field* and resolves provenance via `boundaryOnlyRuleIDs(fd)`, which
   calls `protovalidate.ResolveFieldRules(fd)` — a field-scoped resolver
   that has no visibility into the *containing oneof's* own
   `OneofDescriptor.Options()` extension. `SourceMessage.BoundaryOnly`'s
   own doc comment claims to record "every protovalidate constraint on a
   field that mixinforproto cannot enforce at the storage layer" — for an
   `OneofRules.required` constraint, this is not true: it is neither
   enforced nor recorded as boundary-only. It has zero provenance
   anywhere in the annotation contract.
3. There is no corpus fixture anywhere under `proto/mixinforprototest/v1/`
   that declares `(buf.validate.oneof)` at all, so `TestCorpusExercisesEveryProtovalidateConstraintClass`'s
   ground truth never includes this category and cannot catch the gap.
4. Unlike `FieldRules`/`MessageRules`, there is no
   `TestOneofRulesDeclarationSurfaceIsFullyHandled`-shaped guard that
   would fail loudly if this rule carrier is added to the corpus without
   being wired up.

This is exactly the failure mode phase 03's own stated invariant #5
("Unhandled rule carriers must fail loudly... never silently downgrade a
field to unvalidated at the storage layer — never a named test") warns
against, and it directly weakens `SourceMessage.BoundaryOnly`'s promise
that Phase 5's drift check (and any application developer) can rely on to
know which contract constraints still need a hand-written safety net. An
application writing directly through the generated `ent.Client` (bypassing
the RPC boundary — background jobs, migrations, other internal callers)
can persist an entity that violates a `oneof.required` constraint with
absolutely no record that this could happen.

**Fix:** Either (a) resolve each real oneof's `OneofRules` via
`protovalidate.ResolveOneofRules` in `derive.go`'s per-message walk and
record it into `SourceMessage.BoundaryOnly` (a new
`BoundaryOnlyReason`, e.g. `BoundaryOnlyOneofUnbound`, naming the oneof
rather than a single field), plus a corpus fixture and a
declaration-surface guard mirroring `TestFieldRulesDeclarationSurfaceIsFullyHandled`;
or (b), if this is a deliberate v1 scope boundary (like
`google.protobuf.FieldMask`/`Duration`), document that decision
explicitly in `mixinforproto.md`/README.md and add a named test asserting
the omission is intentional, the same discipline this codebase already
applies to every other documented gap (e.g. `constraintClassExceptions`
in `corpus_test.go`). Silence is the one option this codebase's own
established convention rules out.

## Warnings

### WR-01: `reverseEnum` embeds the rejected value in its error text, inconsistent with every sibling reverse-conversion function

**File:** `mixinforproto/reverse.go:217-224`
**Issue:** Every other failure path in `reverse.go` deliberately avoids
naming the actual rejected Go value in its error text — every "wrong Go
type" branch uses `%T` only (e.g. `reverseScalarByKind`: `"expected a Go
bool, got %T"`), and `reverseJSONMessage`/`reverseStruct` never interpolate
the payload. `reverseEnum` is the one exception:

```go
func reverseEnum(fd protoreflect.FieldDescriptor, entValue any) (protoreflect.Value, error) {
    ...
    evd := fd.Enum().Values().ByName(protoreflect.Name(name))
    if evd == nil {
        return protoreflect.Value{}, fmt.Errorf(
            "mixinforproto: reverse-converting field %q (kind %s, class enum): enum string %q not in descriptor",
            fd.Name(), fd.Kind(), name,
        )
    }
    ...
}
```

`name` here is `entValue.(string)` — the actual rejected mutation value —
interpolated directly into the error text with `%q`. This D-12 fault is
currently caught downstream by `runtime.MapError`'s default case, which
logs the full error server-side and returns a generic `"internal error"`
message on the wire, so in the `entconnect` runtime stack today this does
not leak. But `mixinforproto` is explicitly designed and documented
(README.md, this package's own `go.mod`) to be adoptable **standalone**,
with no dependency on `runtime`/`entconnect` at all. A standalone
consumer that propagates this hook's `error` return value directly (e.g.
returns `err.Error()` in an HTTP response body, or logs it somewhere a
client can read) reproduces exactly the "rejected values must never
appear in error text that crosses the RPC boundary" leak this phase's own
invariant #4 exists to prevent — and this is the one function in the
package that makes that mistake possible.

**Fix:** Drop the `name` interpolation (or hash/truncate it) so the error
matches every sibling function's discipline, e.g.:

```go
return protoreflect.Value{}, fmt.Errorf(
    "mixinforproto: reverse-converting field %q (kind %s, class enum): "+
        "enum string value not declared in descriptor",
    fd.Name(), fd.Kind(),
)
```

If the offending string is useful for server-side debugging, log it
separately rather than embedding it in the returned `error`.

### WR-02: `mixinforproto/README.md` is stale — claims `Exclude`/`Override` are "not yet available" and omits them (plus `WithMessageRules`/`OnCreate`) from the API reference entirely

**File:** `mixinforproto/README.md:47-51, 213-224`
**Issue:** The "Usage" section still carries:

> **Not yet available in this release:** `mixinforproto.md` §2 also
> documents `Exclude(names ...string)` and `Override(name string, f
> ent.Field)`. Neither is part of this package's public API yet...

This is incorrect for the code as it stands: `Exclude`/`Override` are
fully implemented, exported (`option.go`), and are the exact mechanism
this very phase's CR-03 gap closure (suppressing storage-layer validation
relay for excluded/overridden fields, `hooks.go`'s `isExcluded`/
`isOverridden` checks) is built around — extensively exercised by
`hooks_test.go`, `messagerules_test.go`, and
`internal/difftest/optionsuppression_test.go`. The "Known limitation"
section a few paragraphs later even says "the fix is `Exclude` (once
available)" — also stale.

Separately, the "API reference" table (lines 213-224) lists
`MixinForProto`, `Validate`, `AsJSON`, `ContractVersion`, the two
annotation-key constants, and `SourceMessage`/`SourceField` — but omits
`Exclude`, `Override`, `WithMessageRules`, `OnCreate`, and
`MessageRuleTrigger` entirely, despite all five being exported, stable,
and load-bearing public API as of this phase.

**Fix:** Remove the stale "Not yet available" blockquote and the stale
"once available" reference, and add `Exclude`, `Override`,
`WithMessageRules`, `OnCreate`, and `MessageRuleTrigger` to the API
reference table with the same one-line treatment every other symbol gets.

## Info

### IN-01: `protovalidate.ResolveFieldRules(fd)` is called twice per scalar field builder in `fieldmap.go`

**File:** `mixinforproto/fieldmap.go` (buildInt32Field, buildInt64Field,
buildUint32Field, buildUint64Field, buildFloat32Field, buildFloat64Field,
buildBoolField, buildStringField, buildBytesField)
**Issue:** Every one of these nine builder functions calls
`resolvedFieldRules(fd)` (which internally calls
`protovalidate.ResolveFieldRules(fd)` to extract `required`/CEL residuals)
and then calls `protovalidate.ResolveFieldRules(fd)` again directly to
read the kind-specific sub-message (`rules.GetInt32()`, `rules.GetString_()`,
etc.). This is a pure DRY/maintainability smell — not a correctness risk
(the resolver is a stateless descriptor-extension lookup, not called
with side effects), and out of this review's performance scope — but it
duplicates error handling nine times over for the same underlying call.
**Fix:** Have `resolvedFieldRules` return the raw `*validate.FieldRules`
value alongside `required`/`celResidual`/`celEntries`, so every caller
resolves once.

### IN-02: `check-single-validationerror-site`'s comment-stripping filter only recognizes whole-line `//` comments, not trailing inline comments

**File:** `Makefile:213-241`
**Issue:** The gate's own doc comment explains the `grep -vE
':[[:space:]]*//'` filter exists so a doc comment mentioning
`protovalidate.ValidationError{` in prose doesn't turn into a false
positive. The filter as written only matches when the `//` begins the
line (immediately after the `grep -n` line-number separator, optionally
preceded by whitespace) — a legitimate **trailing** inline comment on a
real code line, e.g. `x := 1 // ...protovalidate.ValidationError{... in
prose`, is not filtered, because there is no `:` immediately preceding
that `//` for the regex to anchor on. Such a line would be reported as a
genuine second construction site, a CI false positive.
**Fix:** Strip everything from the first `//` onward on each line before
searching for the literal (e.g. `sed 's|//.*||'`) rather than filtering
whole matched lines by a line-start heuristic.

---

_Reviewed: 2026-08-15T12:50:52Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
