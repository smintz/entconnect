---
phase: 01-mixinforproto-core
reviewed: 2026-08-08T00:00:00Z
depth: standard
files_reviewed: 22
files_reviewed_list:
  - mixinforproto/mixin.go
  - mixinforproto/derive.go
  - mixinforproto/fieldmap.go
  - mixinforproto/validate.go
  - mixinforproto/annotation.go
  - mixinforproto/option.go
  - mixinforproto/errors.go
  - mixinforproto/reserved.go
  - mixinforproto/doc.go
  - mixinforproto/go.mod
  - mixinforproto/internal/fieldproj/fieldproj.go
  - mixinforproto/internal/fieldproj/fieldproj_test.go
  - mixinforproto/internal/boundarytest/boundary_test.go
  - mixinforproto/internal/boundarytest/ent/schema/tracer.go
  - mixinforproto/derive_test.go
  - mixinforproto/fieldmap_test.go
  - mixinforproto/validate_test.go
  - mixinforproto/errors_test.go
  - mixinforproto/failure_test.go
  - mixinforproto/reserved_test.go
  - scripts/pipeline.sh
  - Makefile
  - .github/workflows/ci.yml
findings:
  critical: 4
  warning: 7
  info: 3
  total: 14
status: issues_found
---

# Phase 1: Code Review Report

**Reviewed:** 2026-08-08
**Depth:** standard
**Files Reviewed:** 22
**Status:** issues_found

## Summary

Reviewed the `mixinforproto` module (descriptor walk, field mapping, Tier 1 constraint
translation, failure surface, annotations), its test corpus, and the build/CI pipeline.
Four defects are proven with executed code, not inferred.

Verified clean (adversarially probed, no finding): nil-safety around
`protovalidate.ResolveFieldRules` — every call site guards `rules != nil`; determinism —
no map is ranged into ordered output anywhere (`sortedKeys`, `sortUnique`,
`residualFingerprint` all sort, and the field walk is index-ordered); `regexp.MustCompile`
is absent from the package entirely and the contract-supplied `pattern` path returns a
named error; signed-interval `gt→Min(n+1)`/`lt→Max(n-1)` overflow is correctly guarded at
the type bounds; annotation serialization survives the entc load boundary — I round-tripped
every derived field of 13 corpus messages through `load.NewField` + `json.Marshal` +
`Unmarshal` and the `SourceField` annotation survived in all cases; the length-unit
divergence is consistently recorded in code (`LengthUnitDivergentIDs`), README, and
`doc.go`; error first lines are self-sufficient with no double-prefixing.

The four blockers cluster in exactly the two places the design leaves unguarded: the
descriptor-walk classification (no `IsList()` branch anywhere) and the "delegate to
protovalidate's own evaluator" shortcut (validates the whole message, not the one field).
Both are invisible to the current test suite because the corpus contains no repeated
scalar and every `StringFormat*` fixture is deliberately single-field.

## Narrative Findings (AI reviewer)

### Critical Issues

#### CR-01: `repeated` scalar/enum fields silently derive as singular fields (and can panic at mutation time)

**File:** `mixinforproto/fieldmap.go:43-61` (`classify`), `mixinforproto/fieldmap.go:85-109` (`mapField`)
**Issue:** `classify` never tests `fd.IsList()`. A `repeated string tags = 1` is not a map,
is not in a real oneof, has no optional keyword, and its `Kind()` is `StringKind` — so it
falls to `classScalar` and derives a *singular* `field.String("tags")`. The same happens to
`repeated int32` (→ `field.Int32`, with `.Default(0)`) and `repeated MyEnum` (→
`classEnum` → single `field.Enum`). The only `IsList()` check in the package is inside
`mapWellKnownType` (line 754), which is why repeated *message* fields happen to be skipped
and the corpus (`messages.proto:14`, the sole repeated field in the whole corpus) never
exposes this. The README's "Field mapping" table has no row for repeated fields at all, so
this is not a documented boundary.

Proven by executing `mapField` against a synthesized `repeated string tags` descriptor:

```
IsList=true classify=6 (classScalar)
derived: name=tags type=string optional=false  -- proto field is REPEATED
```

Worse, a repeated string carrying a format rule produces a validator that **panics inside
the ent mutation path** (not at schema load), because `delegatingFormatValidator`'s closure
calls `msg.Set(fd, protoreflect.ValueOfString(s))` on a list-cardinality descriptor:

```
derived urls type=string validators=1
PANIC from derived validator at mutation time: proto: zzrepfmt.M.urls: assigning invalid type string
```

A contract-first tool whose entire premise is "the contract is the source of truth" must not
silently derive a scalar column for a repeated contract field.

**Fix:** add an explicit list branch to `classify` ahead of the kind branches, and either
map it (e.g. `field.JSON(name, []T{})`) or fail loudly with the Exclude/Override remedy —
never fall through to the scalar path:

```go
const classRepeated fieldClass = iota + 7 // or reorder the block

func classify(fd protoreflect.FieldDescriptor) fieldClass {
	switch {
	case fd.IsMap(): // IsMap() must stay first: a map is also a list
		...
	case fd.IsList():
		return classRepeated
	case fd.ContainingOneof() != nil && !fd.ContainingOneof().IsSynthetic():
		...
	}
}
```

and in `mapField`, `case classRepeated:` returns a `failure`-shaped error naming the field
(`"repeated field %q has no Tier 1 mapping — use Exclude(%q) or Override(%q, ...)"`) until a
real list mapping lands. Add a `repeated string` / `repeated Status` fixture to
`scalars.proto`/`enums.proto` so the corpus can never regress silently again.

#### CR-02: `delegatingFormatValidator` validates the whole message, so any unrelated rule on the same message rejects every value

**File:** `mixinforproto/validate.go:374-388`
**Issue:** The returned closure builds a `dynamicpb.NewMessage(fd.ContainingMessage())`,
sets *only* the candidate field, and then calls `v.Validate(msg)` — a whole-message
validation. Every other field on that message is left at its zero value, so any other rule
on the message (`required`, another `string.*` format, a message-level CEL rule, a
cross-field constraint) produces a violation and the closure reports it as
`"value does not satisfy the <format> format constraint"`. The result is a validator that
rejects **100% of inputs**, including perfectly valid ones, for the four delegated formats
(`hostname`, `uri`, `ip`, `uuid`). The code comment at line 368-372 acknowledges the
precondition ("every corpus StringFormat* message deliberately carries exactly one field")
— but that is a property of the test corpus, not of user contracts, and
`constraints.proto`'s own header records the same. This is why
`TestStringFormatValidators` passes while the behavior is broken for any realistic message.

Proven by executing `delegatingFormatValidator` against a synthesized two-field message
(`value` with `string.uri = true`, `other` with `required = true`):

```
POISONED: a valid URI was rejected because of an unrelated field's rule:
value does not satisfy the uri format constraint
```

The failure direction is "reject valid data at write time", i.e. data loss for the caller,
and it is silent (the real violation detail is discarded).

**Fix:** filter the verdict down to the field under validation instead of accepting any
error as a rejection. `protovalidate` returns a `*protovalidate.ValidationError` whose
`Violations` carry a field path; only violations whose path names `fd` may be reported:

```go
return func(s string) error {
	msg := dynamicpb.NewMessage(md)
	msg.Set(fd, protoreflect.ValueOfString(s))
	verr := v.Validate(msg)
	if verr == nil {
		return nil
	}
	var ve *protovalidate.ValidationError
	if !errors.As(verr, &ve) {
		return fmt.Errorf("validating the %s format constraint: %w", formatName, verr)
	}
	for _, viol := range ve.Violations {
		if fieldPathNames(viol) == string(fd.Name()) { // compare against viol's field path
			return fmt.Errorf("value does not satisfy the %s format constraint", formatName)
		}
	}
	return nil // the only violations were on unrelated fields — not this field's verdict
}
```

Add a corpus message carrying a delegated format field *alongside* an unrelated `required`
field; that fixture is the regression guard this bug slipped through for lack of.

#### CR-03: `optional` string/bytes + `required` emits `NotEmpty()`, rejecting a set-but-empty value protovalidate accepts

**File:** `mixinforproto/fieldmap.go:276-289` (`classifyRequired`), `mixinforproto/fieldmap.go:626-634`, `mixinforproto/fieldmap.go:666-674`
**Issue:** `classifyRequired` tests `required && hasNotEmpty` **first**, so for
string/bytes the `optional && required` → `requiredExactPresence` branch is unreachable.
For a presence-tracking field (`optional string value = 1 [(buf.validate.field).required = true]`),
protovalidate's `required` means "must be *set*"; an explicitly-set empty string is valid.
The derived field instead gets `NotEmpty()` (i.e. `MinLen(1)`, verified in ent's
`schema/field/field.go:240`) and rejects it. The comment at fieldmap.go:262-265 asserts
`NotEmpty()` "IS exact there (either presence)" — that claim is wrong for the presence case,
and the divergence is not recorded in `ResidualIDs` or `LengthUnitDivergentIDs` either, so
it is invisible to Phase 3's differential harness. Note also that the derived field is left
non-optional/non-nillable, so ent cannot represent "set to empty" at all.

Proven by executing `mapField` against a synthesized `optional string value` with
`required = true`:

```
HasOptionalKeyword=true classify=3 (classOptionalScalar)
derived: optional=false nillable=false validators=1
ent verdict on set-but-empty string:      value is less than the required length
protovalidate verdict on set-but-empty string: <nil>
```

**Fix:** make the presence case win for string/bytes too — `required` on a presence-tracking
field is a presence assertion, never a non-emptiness assertion:

```go
func classifyRequired(optional, required, hasNotEmpty bool) requiredResult {
	switch {
	case optional && required:
		return requiredExactPresence // presence semantics, for every kind
	case required && hasNotEmpty:
		return requiredExactNotEmpty // implicit-presence string/bytes: required == non-zero
	...
	}
}
```

and give `buildStringField`/`buildBytesField` a `requiredExactPresence` case that emits no
`NotEmpty()` and no `Nillable().Optional()` (matching `buildInt32Field` et al.). Add the
`optional string` + `required` fixture; `RequiredString` (implicit presence) does not cover
this path.

#### CR-04: the `stubs` CI staleness gate runs the exact unscoped `buf generate` the canonical pipeline forbids

**File:** `.github/workflows/ci.yml:106`
**Issue:** The gate runs `(cd "$TMP/proto" && buf generate)` with no `--path`, while
`scripts/pipeline.sh:69` runs `buf generate --path mixinforprototest` and its 12-line
comment (lines 56-66) states that leaving `proto/buf/validate/validate.proto` in scope was
verified live to break generation. `proto/buf.yaml` declares `modules: - path: .` with only
a *lint* ignore for `buf/validate`, so the vendored file is in generation scope. Its
`go_package` is `buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate`
(deliberately excluded from the managed-mode override, per `buf.gen.yaml`), and the plugin
runs with `opt: module=github.com/smintz/entconnect/mixinforproto` — `protoc-gen-go` errors
on any file whose import path is not under the declared module prefix. So the job either
fails at the generate step, or (if generation somehow succeeds) emits files outside
`mixinforproto/internal/gen`, and the subsequent `diff -rq` reports them. Either way the
D-22 staleness gate does not do what its name and error message claim. (`buf` is not
installed in this environment, so this is derived from the repo's own configuration plus
`pipeline.sh`'s own recorded verification rather than executed.)

**Fix:** make the gate use the canonical command — never a second, divergent spelling:

```yaml
      - name: Regenerate corpus into a temp directory and diff against committed stubs
        run: |
          export PATH="$PATH:$(go env GOPATH)/bin"
          TMP="$(mktemp -d)"
          git archive HEAD | tar -x -C "$TMP"
          rm -rf "$TMP/mixinforproto/internal/gen"        # see WR-03
          (cd "$TMP/proto" && buf generate --path mixinforprototest)
          diff -rq mixinforproto/internal/gen "$TMP/mixinforproto/internal/gen" || { ... }
```

Better still: have both call sites invoke one shared script so they cannot drift again.

### Warnings

#### WR-01: enum-valued maps silently degrade to `map[K]any`

**File:** `mixinforproto/fieldmap.go:703-739`
**Issue:** `classify` routes a map to `classMessageMap` only when
`fd.MapValue().Kind() == MessageKind`; an enum-valued map (`map<string, Status>`) is
therefore treated as a scalar map, and `goScalarType(EnumKind)` falls through to the
`default:` branch returning `interface{}`. The field derives as `field.JSON(name,
map[string]any{})` with no record anywhere that the value type was lost — the `SourceField`
`Kind` still reads `"scalarMap"`. The README's mapping table documents only "map of scalars"
and "map with a message value"; enum-valued maps are neither, and there is no corpus
fixture (`maps.proto` has string/int/message only). The `default:` branch is dead for its
stated purpose ("Only scalar kinds ever reach here") and is in fact a silent-degradation
path.
**Fix:** handle `EnumKind` explicitly (`reflect.TypeOf("")` for the declared value name, to
match `mapEnum`'s name-carrying construction), and make `goScalarType` return an error (or
have `mapScalarMap` return one) for any kind it cannot map, so an unmapped kind fails at
schema load with the standard Exclude/Override remedy instead of becoming `any`.

#### WR-02: unchecked `uint64 → int` narrowing on every string length bound

**File:** `mixinforproto/validate.go:230-263`
**Issue:** `int(sr.GetMinBytes())`, `int(sr.GetMaxBytes())`, `int(sr.GetLenBytes())`,
`int(sr.GetMinLen())`, `int(sr.GetMaxLen())`, `int(sr.GetLen())` all narrow a `uint64`
straight to `int` with no range check. A bound above `MaxInt64` (64-bit) or `MaxInt32`
(32-bit build) wraps to a negative value. Verified against ent's own source
(`schema/field/field.go:216,246`): `MaxLen(negative)` appends `len(v) > i` → **rejects
every value** and additionally sets `desc.Size` negative; `MinLen(negative)` accepts
everything. Either way a real constraint is silently inverted rather than reported.
**Fix:** guard before narrowing, and record the bound as residual (never translate it) when
it does not fit:

```go
func lenBound(v uint64) (int, bool) {
	if v > math.MaxInt32 { // conservative: same verdict on 32- and 64-bit builds
		return 0, false
	}
	return int(v), true
}
```

with the `!ok` path appending to `residual`/`entries` exactly like `recordAnyRangeResidual`
does, so T-01-23 ("recorded, never dropped") still holds.

#### WR-03: the stubs gate is vacuous for deleted or renamed generated files

**File:** `.github/workflows/ci.yml:104-110`
**Issue:** `git archive HEAD | tar -x -C "$TMP"` copies the *committed* stubs into `$TMP`
before regeneration. `buf generate` overwrites files it produces but never deletes files it
no longer produces, so a `.pb.go` orphaned by a removed/renamed proto message stays
identical on both sides and the diff passes. The gate can only catch *modified* output, not
*stale leftover* output — which is half of what "stale relative to proto/ sources" means.
**Fix:** `rm -rf "$TMP/mixinforproto/internal/gen"` immediately after the `tar -x` (see the
snippet in CR-04) so the comparison is against a from-scratch tree.

#### WR-04: the reserved-identifier catalog's stated justification is contradicted by ent's source, and its case-sensitivity inverts the check

**File:** `mixinforproto/reserved.go:3-86`
**Issue:** The doc comment claims a collision produces "a Go compiler 'redeclared in this
block' error". Read against `entgo.io/ent@v0.14.6/entc/gen/type.go`:
- `privateField` is consulted only by `builderField` (line 2229), which *auto-renames* a
  colliding field to `"_" + name` — it is not an error at all. So `path`, `order`, `limit`,
  `offset`, `op`, `unique`, `driver`, `done`, `ctx`, `hooks` are hard-rejected by
  `isReserved` even though ent handles them. These are ordinary, plausible contract field
  names, and the only escape is `Override` — a real adoption tax on legitimate contracts.
- `globalIdent` is consulted only by `ValidSchemaName` (line 1011), which validates schema
  *type* names, not field names. Combined with case-sensitive matching (line 84) and buf's
  `FIELD_LOWER_SNAKE_CASE` lint rule, the 24 PascalCase entries can never match a
  lint-clean proto field name — while a proto field named `value`, whose generated Go
  identifier *is* `Value`, is not caught.

So the check both over-rejects (lowercase half) and under-detects (PascalCase half), and
`TestReservedStaticCatalogSize` only pins the transcription, not the semantics.
**Fix:** decide what identifier the catalog is actually comparing against, and compare
against that: match the *generated* identifier (`pascal(name)`) against `globalIdent`, and
drop `privateField` entirely (ent already resolves it) or downgrade it to a recorded note
rather than a schema-load failure. Whichever way it lands, correct reserved.go's doc comment
— it currently justifies the catalog with behavior ent does not have.

#### WR-05: `protovalidate.ResolveFieldRules` is called twice per field in six builders

**File:** `mixinforproto/fieldmap.go:296+304`, `339+348`, `389+398`, `434+443`, `481+490`, `517+526`, `587+598`, `646+656`
**Issue:** `resolvedFieldRules` (line 161) exists explicitly so "every scalar-kind case
below shares the same nil-safe resolution", and then every one of the eight builders calls
`protovalidate.ResolveFieldRules(fd)` a *second* time and re-implements the identical
error-wrapping (`"resolving protovalidate field rules: %w"`). Beyond the duplication, the
two calls can in principle disagree, and the error strings are now maintained in nine
places. The same eight-way copy/paste extends to the `classifyRequired` switch, which is
byte-identical in six builders.
**Fix:** have `resolvedFieldRules` return the resolved rules value alongside the derived
data (its type never has to be spelled — it can stay an inferred return through a small
generic or a callback), and factor the `classifyRequired` switch into one helper
parameterized by the zero-value default.

#### WR-06: the "exactly three direct dependencies" invariant that motivates ~100 lines of indirection is already false in go.mod

**File:** `mixinforproto/validate.go:22-45`, `mixinforproto/go.mod:7-13`
**Issue:** validate.go's file doc comment justifies the entire structural-interface layer
(`stringRulesIface`, `rangeRulesIface[V]`, `anyRangeRulesPresence`, `bytesRulesPresence`,
plus the hand-written `fakeStringRules` in validate_test.go) with: naming
`*validate.FieldRules` "would then list it as a fourth direct dependency in go.mod —
breaking MIX-14's 'exactly three direct dependencies' invariant (verified live this
session)". `go.mod`'s first require block already lists five direct modules, including
`buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go` itself (line 8, no `// indirect`
marker) — it is pulled in directly by the generated corpus. The invariant the indirection
protects does not hold, so the complexity is unpaid-for, and the comment misleads the next
maintainer into preserving it.
**Fix:** either restate MIX-14 in terms it can actually satisfy (e.g. "three non-test direct
dependencies", and enforce it with a check) or drop the indirection and name the type. At
minimum, correct the comment so it does not assert a false fact about the adjacent go.mod.

#### WR-07: `fieldproj.Project` decodes the *first* annotation of any kind as a `SourceField`

**File:** `mixinforproto/internal/fieldproj/fieldproj.go:98-109`
**Issue:** The loop marshals whatever annotation comes first and unmarshals it into
`SourceFieldProjection`, then `break`s — it never checks `a.Name() == "MixinForProtoField"`.
Any other annotation on a derived field (an `entsql.Annotation`, a future
`mixinforproto`-emitted one, anything a user attaches through `Override`) decodes into a
zero-valued projection with no error, and `-update` would bake that into the golden. This is
test infrastructure, but it is the infrastructure every golden assertion in the phase rests
on, so a false pass here is silent.
**Fix:**

```go
for _, a := range d.Annotations {
	if a.Name() != "MixinForProtoField" {
		continue
	}
	...
}
```

(keeping the JSON round-trip; only the key check is missing).

### Info

#### IN-01: dead constant, stale doc comment, unused parameters

**File:** `mixinforproto/errors.go:42`, `mixinforproto/validate.go:188-200`, `mixinforproto/fieldmap.go:194,206`
**Issue:** `fieldIndexMessageScoped` is declared and documented but never referenced (no
failure is ever message-scoped). `validate.go`'s doc block at line 188 documents a function
named `celResidualsFrom` that does not exist — the function it precedes is `celRuleResidual`,
and the comment's body describes a design (passing the rules value through) that was not
implemented. `mapScalar`/`mapOptionalScalar` take a `msgName` parameter neither uses.
**Fix:** delete the constant (or use it), retitle the doc block to `celRuleResidual` and cut
the paragraph describing the non-implemented approach, and drop the unused parameters.

#### IN-02: the overflow residual's own expression text overflows

**File:** `mixinforproto/validate.go:421,439`
**Issue:** The residual `expr` renders `n+1` / `n-1` at exactly the point where that
arithmetic wraps, so an `int64.gt` at `MaxInt64` records
`"int64.gt=9223372036854775807 (adjustment to -9223372036854775808 would overflow...)"`.
The text is misleading, and since it feeds `residualFingerprint`, the fingerprint is
computed over a nonsense value.
**Fix:** render the message without the wrapped arithmetic, e.g.
`"%s.gt=%d (at this field's type maximum; +1 adjustment would overflow, recorded not translated)"`.

#### IN-03: pipeline step 4 greps the whole tree per module and always logs `OK` after `SKIP`

**File:** `scripts/pipeline.sh:79-91`
**Issue:** `grep -rl --include='*.go' -e '^//go:generate' "$m"` with `$m == "."` searches
the entire repo including the nested `mixinforproto` module, so a directive added only to
`mixinforproto` also makes the root module report "has //go:generate directives, running"
and run `go generate ./...` in the root (a no-op that misreports what happened). Separately,
the `SKIP` branch (line 89) is immediately followed by an unconditional
`log "step 4/5: OK"`, so the output reads `SKIP ... OK`.
**Fix:** scope the grep with `-not -path` / `--exclude-dir` for nested module roots (or use
`go list ./... | xargs grep`, which respects module boundaries), and move the `OK` log into
the else branch.

---

_Reviewed: 2026-08-08_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
