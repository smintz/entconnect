---
phase: 03-validation-fidelity
reviewed: 2026-08-14T00:00:00Z
depth: standard
files_reviewed: 48
files_reviewed_list:
  - .github/workflows/ci.yml
  - Makefile
  - internal/entconnecttest/hookwiring/ent/entc.go
  - internal/entconnecttest/hookwiring/ent/generate.go
  - internal/entconnecttest/hookwiring/ent/schema/policed.go
  - internal/entconnecttest/hookwiring/ent/schema/recorder.go
  - internal/entconnecttest/hookwiring/ent/schema/unpoliced.go
  - internal/entconnecttest/hookwiring/ordering_test.go
  - internal/entconnecttest/hookwiring/wiring_test.go
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
  - mixinforproto/internal/difftest/ent/entc.go
  - mixinforproto/internal/difftest/ent/generate.go
  - mixinforproto/internal/difftest/ent/schema/messagerules.go
  - mixinforproto/internal/difftest/ent/schema/mixedrules.go
  - mixinforproto/internal/difftest/ent/schema/residualcel.go
  - mixinforproto/internal/difftest/fakedriver.go
  - mixinforproto/internal/difftest/generate.go
  - mixinforproto/internal/difftest/hybrid_test.go
  - mixinforproto/internal/difftest/messagerules_test.go
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
  - proto/entconnecttest/v1/hookwiring.proto
  - proto/mixinforprototest/v1/constraints.proto
  - proto/mixinforprototest/v1/messagerules.proto
  - proto/mixinforprototest/v1/messages.proto
  - proto/mixinforprototest/v1/reverse.proto
  - runtime/doc.go
  - runtime/errormap.go
  - runtime/errormap_test.go
  - runtime/interceptor_test.go
findings:
  critical: 3
  warning: 10
  info: 0
  total: 13
status: issues_found
---

# Phase 3: Code Review Report

**Reviewed:** 2026-08-14
**Depth:** standard
**Files Reviewed:** 48
**Status:** issues_found

## Summary

The phase's stated guarantee is that the RPC boundary and the storage layer produce
identical verdicts for the same contract rule, and that a rule the storage layer cannot
faithfully evaluate is either recorded as boundary-only or refused at schema load. Three
of the findings below break that guarantee, and all three are **reproduced empirically**
against this working tree (probe tests written, run, and removed — no source files were
modified).

The root pattern behind all three: `buildHookState`/`evaluate` (`mixinforproto/hooks.go`)
reason about *one* declaration form of each protovalidate concept — `FieldRules.cel` for
custom field rules, `MessageRules.cel` for message rules — and about the descriptor alone,
never about the `Option` set the same mixin was derived with. Everything outside that
narrow slice either escapes the D-10 gate, escapes `ignore` semantics, or escapes
`Exclude`/`Override`.

A secondary structural finding (WR-01): the entire locally-compiled `cel.Env` half of the
"hybrid" is provably dead — protovalidate's own evaluator already evaluates every custom
CEL rule for every field the `Filter` admits, and `dedupeViolations` discards the local
copy 100% of the time. Its only observable effects today are the divergence in CR-02 and
wasted compilation.

No structural pre-pass (`<structural_findings>`) was supplied with this review.

## Narrative Findings (AI reviewer)

## Critical Issues

### CR-01: `MessageRules.cel_expression` / `MessageRules.oneof` bypass D-10's reference gate and produce phantom violations

**File:** `mixinforproto/messagerules.go:96`, `mixinforproto/messagerules.go:116`, `mixinforproto/hooks.go:181-189`
**Severity:** BLOCKER

`checkMessageRuleReferences` inspects only `msgRules.GetCel()`:

```go
if msgRules == nil || len(msgRules.GetCel()) == 0 {
    return nil, nil          // messagerules.go:96
}
...
for _, r := range msgRules.GetCel() { ... }   // messagerules.go:116
```

`buf.validate.MessageRules` declares **three** rule carriers (`proto/buf/validate/validate.proto:174-245`):
`cel` (field 3), `cel_expression` (field 5, repeated string), and `oneof` (field 4,
`MessageOneofRule`). Only `cel` is walked. Consequences for a message whose cross-field
rule is declared as `cel_expression` or `oneof`:

1. **D-10's schema-load gate never fires.** `Exclude`ing a field the rule reads loads
   cleanly instead of panicking.
2. **No `extraFields` seeding.** The referenced field never gets an `evaluators` entry, so
   `evaluate` never reverse-converts it, so it stays at its proto3 zero in the
   reconstructed `dynamicpb` message — while `hs.messageRulesOnCreate && m.Op() == OpCreate`
   still admits the message descriptor through the `Filter` (`hooks.go:490-494`) and
   protovalidate evaluates the rule against that fabricated data.

Reproduced (probe against a `protodesc`-built descriptor: field `a` with `required`,
field `b` with no rules, message rule `cel_expression = "this.b > 10"`, mutation
`a=5, b=100`):

```
D-10 gate with Exclude("b") -> err = <nil>            (want: a schema-load failure)
STORAGE: 1 violations   ["this.b > 10"] "\"this.b > 10\" returned false"
BOUNDARY: <nil>
```

The boundary accepts; the storage layer rejects, citing a rule evaluated against `b == 0`
when the caller actually supplied `b == 100`. This is precisely the phantom-verdict class
`messagerules.go`'s own doc comment says D-10 exists to make impossible.

**Fix:** enumerate all three carriers. Compile `cel_expression` entries the same way
(`Rule{Id: expr, Expression: expr}` — protovalidate derives the id from the expression),
and treat every field named in a `MessageOneofRule.fields` list as a reference. Gate on the
union, and refuse to enable message rules at all for any carrier this function cannot
statically analyse rather than silently evaluating it:

```go
// messagerules.go
type msgRule struct{ id, expr string }

func messageRuleExprs(r *validate.MessageRules) []msgRule {
    var out []msgRule
    for _, c := range r.GetCel() {
        out = append(out, msgRule{c.GetId(), c.GetExpression()})
    }
    for _, e := range r.GetCelExpression() {
        out = append(out, msgRule{e, e}) // id == expression, per validate.proto
    }
    return out
}

// ...and, before the CEL walk, fold in every oneof-rule field reference:
for _, oo := range msgRules.GetOneof() {
    for _, name := range oo.GetFields() {
        if fd := md.Fields().ByName(protoreflect.Name(name)); fd != nil {
            // same messageRuleFieldUnavailable / refs handling as a cel select
        }
    }
}

// and replace the early return:
if msgRules == nil || (len(msgRules.GetCel()) == 0 &&
    len(msgRules.GetCelExpression()) == 0 && len(msgRules.GetOneof()) == 0) {
    return nil, nil
}
```

Add corpus fixtures for both forms to `proto/mixinforprototest/v1/messagerules.proto`.

---

### CR-02: `(buf.validate.field).ignore` is not honored by the local CEL half — storage rejects what the boundary accepts

**File:** `mixinforproto/hooks.go:227-248`, `mixinforproto/hooks.go:526-549`
**Severity:** BLOCKER

`buildHookState` compiles every `rules.GetCel()` rule into a `celProgram` and `evaluate`
runs every such program unconditionally for any in-scope field with a valid value. Neither
site consults `rules.GetIgnore()`. protovalidate's own evaluator does: `IGNORE_ALWAYS`
skips the field's entire rule set, `IGNORE_IF_ZERO_VALUE` skips it when the value is the
type's zero. The codebase already knows this field exists —
`fieldmap.go:233-238`'s `boundaryOnlyNonConstraintFields` lists `"ignore"` — but the hook
path never reads it.

Reproduced (probe: string field with `ignore = IGNORE_ALWAYS` plus a `cel` rule
`this.startsWith('X')`, value `"nope"`):

```
BOUNDARY verdict: <nil>
STORAGE verdict: 1 violations   "probe.starts_with_x" "must start with X"
```

The boundary accepts the payload; the mutation is then rejected at the storage layer with a
`*protovalidate.ValidationError` for a rule the contract explicitly disabled. No corpus
fixture uses `ignore` (`grep -rn "ignore" proto/mixinforprototest/`), so the PIPE-06 sweep
cannot detect it.

**Fix:** the cleanest fix is WR-01's — delete the local CEL half entirely, since
protovalidate's own evaluator already covers these rules *and* already honors `ignore`. If
the local half is retained for any reason, gate it:

```go
// buildHookState, before compiling programs:
switch rules.GetIgnore() {
case validate.Ignore_IGNORE_ALWAYS:
    // protovalidate skips this field entirely; the local env must too.
    programs = nil
case validate.Ignore_IGNORE_IF_ZERO_VALUE:
    // record on fieldEvaluator; evaluate() must skip when val is the zero value
    fe.skipIfZero = true
}
```

Add a corpus fixture carrying `ignore = IGNORE_IF_ZERO_VALUE` + a `cel` rule so the sweep
covers it.

---

### CR-03: `buildHookState` ignores `Exclude`/`Override`, so overridden fields are still enforced — and a type-changing `Override` hard-fails every mutation

**File:** `mixinforproto/hooks.go:174-259`
**Severity:** BLOCKER

`buildHookState` reads `o` only for `o.messageRulesEnabled()` (`hooks.go:177`). Its per-field
loop walks `md.Fields()` and never calls `o.isExcluded(name)` or `o.isOverridden(name)` —
unlike `messageRuleFieldUnavailable` (`messagerules.go:185-190`), which does check both,
showing the authors intended this distinction.

Two concrete failures, both reproduced against `mixinforprototest.v1.MixedFieldRules`:

**(a) `Override` no longer suppresses validation relay.** `option.go:112-120` states an
override "suppresses validation relay for that field entirely — the developer owns it
completely, with no silent merging (D-05)", and `derive.go:93-110` records the field as
`BoundaryOnlyOverridden` in `SourceMessage.BoundaryOnly`. The hook enforces it anyway:

```
buildHookState(md, Override("both", field.String("both")))
  evaluators: 3
  Override(both) -> 3 raw violations
    [0] "constraints.mixed_field_rules.both.starts_with_x"
    [1] "string.min_len"
    [2] "constraints.mixed_field_rules.both.starts_with_x"
```

The emitted annotation therefore lies to Phase 5's drift check: it says "boundary-only"
about a rule the storage layer is actively enforcing.

**(b) A type-changing `Override` breaks every mutation.** `derive_test.go:206` already
exercises `Override("value", field.Bool("value"))` on a string-typed proto field as a
supported combination. With a hook installed, the reverse leg is handed a `bool` for a
`StringKind` descriptor:

```
Override(both, field.Bool("both")) -> 0 violations,
  err = mixinforproto: reverse-converting field "both" (kind string): expected a Go string, got bool
```

That is a D-12 data-integrity error, so `runtime.MapError` maps it to
`CodeInternal` — every write to that entity returns HTTP 500, permanently, from a schema
that loads without complaint.

**(c) `Exclude` leaks too.** `buildHookState(md, Exclude("both"))` still produces an
evaluator for `both`. Harmless while no ent field of that name exists, but it becomes (b)
the moment a schema declares its own field under the excluded name — a documented pattern.

**Fix:** skip both option classes in the per-field loop, before rule resolution:

```go
for i := 0; i < fds.Len(); i++ {
    fd := fds.Get(i)
    name := string(fd.Name())

    // Exclude/Override remove the field from storage-layer relay entirely —
    // derive.go already records both as BoundaryOnly provenance (D-09).
    if o.isExcluded(name) || o.isOverridden(name) {
        continue
    }
    ...
}
```

Add unit tests pinning "an overridden field produces zero storage-layer violations" and
"an excluded field produces no evaluator entry".

---

## Warnings

### WR-01: the local `cel.Env` residual half is dead by construction — every violation it produces is deduplicated away

**File:** `mixinforproto/hooks.go:227-248`, `mixinforproto/hooks.go:455-462`, `mixinforproto/hooks.go:520-549`
**Severity:** WARNING

`evaluate` unconditionally adds every in-scope evaluator field to `standardFields`
(`hooks.go:459`), and the `Filter` admits every member of that set (`hooks.go:501-502`).
protovalidate's compiled per-field evaluator includes the field's custom `cel` rules — as
`hooks.go`'s own doc comment (lines 47-56) acknowledges. So the locally compiled program
always produces a violation whose `(RuleId, FieldPath)` identity is byte-identical to one
protovalidate already produced, and `dedupeViolations` (`violation.go:87-102`) keeps the
first — protovalidate's — and drops the local one.

Reproduced for a **cel-only** field (`cel_only = "abc"` on `MixedFieldRules`):

```
RAW violations count = 2
  [0] ruleID="constraints.mixed_field_rules.cel_only.starts_with_x" msg="value must start with X"
  [1] ruleID="constraints.mixed_field_rules.cel_only.starts_with_x" msg="value must start with X"
```

The local half therefore contributes nothing observable, only cost (a `cel.Env` +
`cel.Program` per rule at schema load, a second `Eval` per mutation) and risk — CR-02 is
exactly the divergence it introduces. `celCompileCount`/`CELCompileCount` and the entire
`compileCELRule`/`celProgram`/`celResultToViolation` machinery, plus the tests pinning
them, exist to guard a code path with no output.

**Fix:** delete the local CEL half and route all field rules through the single
`protovalidate.Validator`. This eliminates CR-02, removes the dependency on
`buf.build/go/protovalidate/cel` and `github.com/google/cel-go` from `hooks.go`, removes
the "message text must be byte-identical between two engines" obligation, and makes
`dedupeViolations` unnecessary for field-scoped violations. If the intent was to keep a
second independent engine as a cross-check, it must be wired as an assertion (compare the
two sets, fail loudly on disagreement) rather than as a silent contributor whose output is
always discarded.

---

### WR-02: a field is scoped into the `Filter` even when its reverse conversion reported "absent"

**File:** `mixinforproto/hooks.go:451-462`
**Severity:** WARNING

```go
val, err := reverseValue(fe.fd, fe.class, raw)
if err != nil { return nil, err }
if val.IsValid() {
    dyn.Set(fe.fd, val)
    values[fe.fd.Number()] = val
}
standardFields[fe.fd.Number()] = struct{}{}   // <- unconditional
```

`reverseValue` deliberately returns `(protoreflect.Value{}, nil)` for "absent, not a fault"
(`reverse.go:76-81`, `reverse.go:273-291`, `reverse.go:319-332`). When that happens the
field is *not* set on `dyn`, yet it is still admitted to protovalidate's scope, so
protovalidate evaluates its rules against a field that was never populated — the exact
phantom-violation class D-03 exists to prevent.

Unreachable today only because `hookFieldClass` is narrowed to `scalar`/`optionalScalar`
and ent's generated `Mutation.Field` returns `ok == false` for an unset nillable field. It
becomes live the moment `hookFieldClass` is widened to `asJSON`/`wkt`/`scalarMap`, which
`hooks.go:298-304` explicitly plans.

**Fix:** move the insert inside the validity check, so scope and reconstruction can never
disagree:

```go
if val.IsValid() {
    dyn.Set(fe.fd, val)
    values[fe.fd.Number()] = val
    standardFields[fe.fd.Number()] = struct{}{}
    if len(fe.programs) > 0 { celFields = append(celFields, fe) }
}
```

---

### WR-03: message-level rules are silently skipped on Create when no in-scope field is rule-bearing

**File:** `mixinforproto/hooks.go:465-470`
**Severity:** WARNING

```go
if len(standardFields) == 0 {
    return nil, nil
}
```

This early return precedes the `messageRulesInScope` computation, so an opted-in
message-level rule is never evaluated when the mutation's in-scope field set happens to be
empty. Reachable on Create whenever every field the rule reads is a proto3 `optional`
scalar the caller left unset (no `Default`, therefore absent from `m.Fields()`), while the
boundary *does* evaluate the same rule against those fields' zero values. The result is a
silent one-directional divergence in the opposite direction from CR-01.

**Fix:** compute `messageRulesInScope` first and only short-circuit when both halves are
empty:

```go
messageRulesInScope := hs.messageRulesOnCreate && m.Op() == ent.OpCreate
if len(standardFields) == 0 && !messageRulesInScope {
    return nil, nil
}
```

---

### WR-04: `WithMessageRules` discards its `trigger` argument entirely

**File:** `mixinforproto/option.go:157-161`
**Severity:** WARNING

```go
func WithMessageRules(trigger MessageRuleTrigger) Option {
    return func(o *options) {
        o.messageRules = true
    }
}
```

`trigger` is never read. `WithMessageRules(MessageRuleTrigger(99))` — or any value a future
`OnUpdateWithFetch` would take — silently enables `OnCreate` semantics instead of failing.
The option's own doc comment (`option.go:139-156`) and `messagerules.go:52-59` both make a
point of "a symbol that lies is worse than one that is absent"; an argument that is
accepted and ignored is that same defect one level down. `go vet` does not flag unused
parameters, so nothing catches this.

**Fix:** validate the argument and record it, so adding a second trigger later is a
compile-safe change rather than a behavior change:

```go
func WithMessageRules(trigger MessageRuleTrigger) Option {
    return func(o *options) {
        if trigger != OnCreate {
            panic(fmt.Sprintf(
                "mixinforproto: WithMessageRules: unknown MessageRuleTrigger %d — OnCreate is the only declared value",
                trigger))
        }
        o.messageRules = true
    }
}
```

---

### WR-05: `required` on an `optional` field left unset on Create is rejected by ent, not by the hook — wrong error type, wrong wire code

**File:** `mixinforproto/fieldmap.go:466-476` (and every sibling `buildXxxField`), `runtime/errormap.go:58-75`
**Severity:** WARNING

`classifyRequired` → `requiredExactPresence` constructs the ent field with no
`Nillable()/Optional()` **and no `Default`**. On Create with the setter never called, ent's
generated `defaults()` therefore does not materialize the field, it is absent from
`m.Fields()`, and the hook's `required` check never runs. ent's own generated `check()`
rejects instead.

Reproduced against the real generated client
(`mixinforprototest.v1.RequiredOptionalNonString`):

```
STORAGE Create(unset) err = ent: missing required field "RequiredOptionalNonString.value"  (*ent.ValidationError)
BOUNDARY Validate(unset) err = validation error: value: value is required
```

Both reject, but with different error types and no shared `RuleId`/`FieldPath`. That
contradicts VAL-07's "a caller cannot tell which layer caught it"
(`runtime/doc.go:12-20`), and `runtime.MapError` has no case for ent's per-application
`*ent.ValidationError`, so it falls to `CodeInternal` — a 500 for a pure client input
error. The PIPE-06 sweep cannot see this because `sweep_test.go` always sets **every**
sweepable field (`sweep_test.go:425-436`), never the unset case.

**Fix:** add an unset-field arm to the sweep so this class is exercised, and either (a)
have the hook enumerate `required`-carrying derived fields independently of `m.Fields()` on
Create, or (b) document and test the ent-side rejection as the authoritative one and give
`runtime.MapError` / the generated handler templates an explicit mapping to
`CodeInvalidArgument`.

---

### WR-06: CI never runs `go test -race`, though several tests name `-race` as their proof mechanism

**File:** `.github/workflows/ci.yml:37-53`, `Makefile:38-63`
**Severity:** WARNING

`make test` and `make test-determinism` run plain `go test` / `go test -count=5`. No target
or CI step passes `-race`. Yet:

- `mixinforproto/internal/difftest/messagerules_test.go:92-93`: "go test -race is what
  actually proves the 'non-interleaved' half"
- `runtime/interceptor_test.go:17-18, 179-182`: "every Validate call is race-safe (go test
  -race)"
- `internal/entconnecttest/hookwiring/ordering_test.go:90-92`: "isolated per test so tests
  can run with -race"

The concurrency claims these tests document are therefore never actually checked. This
matters for a package whose central object (`hookState`) is shared across every concurrent
mutation and holds a `cel.Program` and a `protovalidate.Validator`.

**Fix:** add a `test-race` target and a CI step:

```make
test-race:
	@set -e; for m in $(MODULES); do \
		if [ -n "$$(cd $$m && go list ./... 2>/dev/null)" ]; then (cd $$m && go test -race ./...); fi; \
	done
```

---

### WR-07: `violation_test.go` cites a "Makefile grep-based check" that does not exist

**File:** `mixinforproto/violation_test.go:262-269`
**Severity:** WARNING

> "see ... Makefile's grep-based check for the authoritative, whole-repo version of this
> assertion"

No such target exists. `Makefile`'s `.PHONY` list is `build vet test test-determinism
test-standalone check-modules check-stubs check-goversion check-dep-parity pipeline`, and
`grep -rn "protovalidate.ValidationError" scripts/ Makefile` returns nothing. The
acceptance criterion "violation.go is the only file constructing a
`*protovalidate.ValidationError{`" — the invariant D-01/VAL-07's identity guarantee rests
on — is enforced by nothing. A comment claiming a gate exists is worse than no gate,
because reviewers stop looking.

**Fix:** either add the target and wire it into CI, or delete the claim:

```make
check-single-validationerror-site:
	@hits=$$(grep -rn 'protovalidate\.ValidationError{' --include='*.go' . \
	          | grep -v '_test\.go' | grep -v 'mixinforproto/violation.go' || true); \
	if [ -n "$$hits" ]; then echo "FAIL: ValidationError constructed outside violation.go:"; echo "$$hits"; exit 1; fi; \
	echo "OK: violation.go is the only construction site"
```

---

### WR-08: no staleness gate for entc-generated code, only for proto stubs

**File:** `.github/workflows/ci.yml:125-157`, `Makefile:104-112`
**Severity:** WARNING

`check-stubs.sh` regenerates and diffs `proto/` output only. The four `//go:generate go run
entc.go` trees this phase added or touched (`mixinforproto/internal/difftest/ent`,
`internal/entconnecttest/hookwiring/ent`, plus the boundarytest tree) have no equivalent
gate. A change to `ent/schema/*.go` — including a change to which `Option`s a fixture
passes to `MixinForProto` — compiles and tests green against the previously committed
generated client, so the schema fixture and the client under test can silently disagree.
For a project whose premise is "contract/schema drift is a build failure rather than a
runtime surprise", this is the one drift axis left ungated.

**Fix:** extend `scripts/check-stubs.sh` (or add a sibling) to run `go generate ./...` in a
temp copy of each `ent/` tree and diff against the committed output, and add it to the
`stubs` CI job.

---

### WR-09: `Makefile` `.PHONY` omits `test-standalone-root`

**File:** `Makefile:12`, `Makefile:81`
**Severity:** WARNING

```make
.PHONY: build vet test test-determinism test-standalone check-modules check-stubs check-goversion check-dep-parity pipeline
```

`test-standalone-root` (line 81) and `check-single-validationerror-site` (if WR-07 is
adopted) are missing. A file or directory appearing with that name would make `make
test-standalone-root` a silent no-op — reporting success for a gate that never ran. Given
this is a CI-required target (`ci.yml:113-114`), the silent-success failure mode matters
more than usual.

**Fix:** add `test-standalone-root` to the `.PHONY` list.

---

### WR-10: the sweep's `maxRuns` cap silently drops candidate values, and the comment claims otherwise

**File:** `mixinforproto/internal/difftest/sweep_test.go:419-421`, `sweep_test.go:513-517`
**Severity:** WARNING

```go
if maxRuns > 8 {
    maxRuns = 8 // ... every candidate still gets exercised via modulo cycling across
                // repeated messages/fields in practice ...
}
```

The claim is false. `values[fname] = cs[run%len(cs)]` with `run < 8` reaches only indices
`0..7` of `cs`; a field whose candidate list is longer (a string field with `min_len` +
`max_len` + `len` + `pattern` + a well-known format easily exceeds 8 —
`generate.go:176-235` appends 3 per length rule) never sees its tail candidates in any run,
of any message, ever. Boundary values are only "in the low indices" for numeric kinds; for
strings, `generate.go:177-186` puts the zero value, the divergent value and a random value
first, pushing real boundary lengths later.

Separately, the failure message at line 515 hardcodes `8` rather than interpolating
`maxRuns`, so a future change to the cap produces a misleading message.

**Fix:** either raise/remove the cap, or cycle the *starting offset* per message so
successive runs cover the whole candidate list:

```go
v := cs[(run+int(seed))%len(cs)]
```

and interpolate `maxRuns` into the divergent-field failure message.

---

### WR-11: `createEntity` / `updateEntity` drive ordered mutation calls from a Go map range

**File:** `mixinforproto/internal/difftest/sweep_test.go:170-201`
**Severity:** WARNING

```go
for name, v := range values {
    if serr := mutation.SetField(name, v); serr != nil { ... }
}
```

This is exactly the pattern D-24 forbids across the rest of the codebase (`derive.go:65-69`,
`reverse.go:352-357`, `option.go:188-190` all sort explicitly and say why). Here the map
range determines which `SetField` failure is reported first when more than one is invalid,
so a red sweep run is not byte-reproducible from its own printed seed — the property D-14
and this test's `disagreement` type exist to guarantee.

**Fix:** sort the key set before iterating, mirroring `disagreement.String`'s own
`sort.Strings(fields)`:

```go
names := make([]string, 0, len(values))
for name := range values { names = append(names, name) }
sort.Strings(names)
for _, name := range names { ... mutation.SetField(name, values[name]) ... }
```

---

## Notes on scope

Per the supplied domain context, the following were treated as known and are **not**
reported as findings: `make test-standalone-root`'s `GOWORK=off` failure against the
published `mixinforproto v0.1.0` tag; `hookFieldClass`'s deliberate scoping to
`scalar`/`optionalScalar`; and the intentional `github.com/google/cel-go@v0.28.0` pin.

All three Critical findings were verified by writing temporary probe tests under the
package, running them with the project toolchain, and deleting them. No source file in the
repository was modified by this review.

---

_Reviewed: 2026-08-14_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
