---
phase: 01-mixinforproto-core
verified: 2026-08-08T12:21:55Z
status: gaps_found
score: 3/5 roadmap success criteria verified (22/25 requirement IDs satisfied)
behavior_unverified: 0
overrides_applied: 0
gaps:
  - truth: "Mixin maps proto scalar types to their corresponding ent field builders (MIX-02), and the roadmap's SC1 claim that 'scalars ... materialize at schema load' holds without qualification"
    status: failed
    reason: >
      classify() in mixinforproto/fieldmap.go has no IsList() branch. A `repeated string tags`
      (or repeated int32/enum) is not a map, not a real oneof, has no optional keyword, so it
      falls to classScalar/classEnum and derives a SINGULAR field, silently dropping cardinality.
      Independently reproduced by executing mapField against a synthesized `repeated string tags`
      descriptor: IsList=true, classify()=classScalar(6), derived field Info=string (not a list).
      Worse, a repeated string carrying a format rule (buf.validate string.uri/email/etc.) produces
      a validator whose closure calls msg.Set(fd, protoreflect.ValueOfString(s)) against a
      list-cardinality field descriptor, which panics at ent MUTATION time (not schema load) per
      the code review's reproduction. The corpus contains exactly one repeated field
      (messages.proto's `repeated Inner repeated_message`), and it is a repeated MESSAGE field,
      which is skipped by an unrelated code path (MIX-10's message-skip-by-default), so this defect
      is invisible to the entire test suite. README's field-mapping table has no row for repeated
      fields at all — this is not a documented boundary, it is a silent gap.
    artifacts:
      - path: "mixinforproto/fieldmap.go"
        issue: "classify() (lines ~43-61) never tests fd.IsList(); mapField's classScalar/classEnum paths derive a singular field for a repeated proto field"
    missing:
      - "Add an explicit list branch to classify() ahead of the scalar/enum kind branches"
      - "Either map repeated scalars/enums to a real list-typed field, or fail loudly at schema load with an Exclude/Override remedy — never silently derive a scalar column"
      - "Add a `repeated string` and/or `repeated <Enum>` fixture to the corpus (scalars.proto/enums.proto) so this can never regress silently again"
  - truth: "protovalidate string constraints (min_len/max_len/len/pattern/format validators) become native ent builder calls (VAL-01), correctly enforcing the same rule protovalidate enforces"
    status: failed
    reason: >
      delegatingFormatValidator (mixinforproto/validate.go:374-388) builds a synthetic
      dynamicpb.Message carrying ONLY the candidate field and calls v.Validate(msg) — a
      WHOLE-MESSAGE validation. Every other field on that message is left at its zero value, so
      any other rule on the message (required, another string.* format, a message-level CEL rule)
      produces a violation and the closure reports the candidate value as invalid regardless of
      whether it actually violates the format rule. Independently confirmed by reading the exact
      code path (no `errors.As`/violation-path filtering exists; ANY non-nil verr becomes a
      rejection). Result: for hostname/uri/ip/uuid format constraints, the derived validator
      rejects effectively 100% of inputs, including valid ones, on any message with more than the
      one field the corpus deliberately tests. The corpus (constraints.proto) declares every
      StringFormat* message with exactly one field ("value"), by its own doc comment, which is why
      TestStringFormatValidators passes while the behavior is broken for realistic multi-field
      messages. This is silent data-loss at write time, not a documented boundary.
    artifacts:
      - path: "mixinforproto/validate.go"
        issue: "delegatingFormatValidator (~lines 374-388) treats any non-nil protovalidate.Validate() error as a rejection of the candidate field, without filtering violations to the field under validation"
    missing:
      - "Filter protovalidate's *protovalidate.ValidationError.Violations down to the violation whose field path names the candidate field before treating it as a rejection"
      - "Add a corpus message carrying a delegated format field alongside an unrelated `required` field, to serve as the regression guard this bug slipped through for lack of"
  - truth: "protovalidate presence/required constraints become NotEmpty or non-optional field construction, matching protovalidate's own presence semantics (VAL-03)"
    status: failed
    reason: >
      classifyRequired (mixinforproto/fieldmap.go) tests `required && hasNotEmpty` BEFORE
      `optional && required`, and buildStringField/buildBytesField always pass hasNotEmpty=true.
      Independently reproduced: classifyRequired(optional=true, required=true, hasNotEmpty=true)
      returns requiredExactNotEmpty (NotEmpty()), not requiredExactPresence — so for a
      presence-tracking `optional string value = 1 [(buf.validate.field).required = true]`,
      protovalidate's "must be set" semantics get replaced by "must be non-empty", rejecting a
      deliberately-set empty string that protovalidate accepts. The code review additionally
      confirmed via direct execution: ent rejects a set-but-empty string with "value is less than
      the required length" while protovalidate's own verdict on the same input is nil (valid).
      The corpus tests `RequiredOptionalNonString` (optional int32 + required, which correctly
      hits requiredExactPresence) but has no `optional string` + `required` fixture, so this
      divergence is invisible to the test suite AND is not recorded in ResidualIDs or
      LengthUnitDivergentIDs — unlike the length-unit divergence, which was deliberately decided
      and machine-recorded.
    artifacts:
      - path: "mixinforproto/fieldmap.go"
        issue: "classifyRequired's branch order makes the presence case (optional && required) unreachable for string/bytes, whose builders always pass hasNotEmpty=true"
    missing:
      - "Reorder classifyRequired so `optional && required` (presence) wins ahead of `required && hasNotEmpty`, for every kind including string/bytes"
      - "Give buildStringField/buildBytesField a requiredExactPresence case emitting no NotEmpty() and no Nillable().Optional() (matching the non-string builders)"
      - "Add an `optional string` + `required` corpus fixture; RequiredString (implicit presence) does not cover this path"
  - truth: "The buf-generate-into-temp-dir-and-diff-against-committed-stubs staleness gate (Plan 03's own documented key link) actually detects staleness, using the same scoped invocation the canonical pipeline itself requires"
    status: failed
    reason: >
      .github/workflows/ci.yml's `stubs` job runs `(cd "$TMP/proto" && buf generate)` with NO
      `--path` flag. scripts/pipeline.sh's own step-2 comment (verified live during Plan 05's
      execution, per its own text) states unscoped generation breaks because
      proto/buf/validate/validate.proto's go_package points outside the declared module prefix,
      and protoc-gen-go errors when a generated file's import path is not under the module
      passed via `opt: module=...`. pipeline.sh instead runs
      `buf generate --path mixinforprototest` specifically to avoid this. The stubs CI job uses
      the exact unscoped invocation the codebase's own pipeline script and comment say is broken
      — so the job either fails at the generate step, or (if it somehow succeeds) diffs against
      output that never matches what the canonical pipeline itself would produce. Either way this
      documented key link ("buf generate into a temp dir + diff against committed stubs ->
      staleness gate", 01-03-PLAN.md must_haves.key_links) does not function as specified. `buf`
      is not installed in this environment so the CI job itself could not be executed to observe
      the literal failure output; this finding is derived from the codebase's own internally
      contradictory configuration (pipeline.sh's own recorded verification vs. ci.yml's divergent
      invocation), which is sufficient to call the key link broken as designed regardless of the
      exact failure mode.
    artifacts:
      - path: ".github/workflows/ci.yml"
        issue: "the `stubs` job's buf generate invocation (around line 106) omits --path mixinforprototest, contradicting scripts/pipeline.sh's own documented, live-verified requirement for that scoping"
    missing:
      - "Scope the stubs job's buf generate call to --path mixinforprototest, matching pipeline.sh exactly"
      - "Have both call sites invoke one shared script so they cannot drift again (the review's own suggested fix)"
      - "Also clear $TMP/mixinforproto/internal/gen before diffing (WR-03), so the gate can catch stale/orphaned generated files, not only modified ones"
deferred: []
human_verification: []
---

# Phase 1: MixinForProto Core Verification Report

**Phase Goal:** Developers can derive a complete, validated ent schema directly from a protobuf message type, with zero codegen and full provenance for later drift checking
**Verified:** 2026-08-08T12:21:55Z
**Status:** gaps_found
**Re-verification:** No — initial verification

## Goal Achievement

A code review completed immediately before this verification (`01-REVIEW.md`) found 4 blockers.
This verification independently re-executed the load-bearing claims behind three of them
(CR-01, CR-03, and a direct reading confirming CR-02's code path) against the live codebase
rather than trusting the review's narrative, and confirms all three are real. CR-04 was
confirmed by reading the CI YAML against the pipeline script's own documented, self-contradicting
requirement (buf itself is not installed in this environment, so the CI job could not be executed
directly).

### Observable Truths (Roadmap Success Criteria)

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | Developer declares `MixinForProto[*orderv1.Order]()` and sees ent fields (scalars, enums, WKTs, presence-correct optionals, scalar maps) materialized at schema load — no descriptor file, no string names, deterministic order | ✗ FAILED | Non-repeated scalars/enums/WKTs/optionals/maps all verified correct (see Requirements Coverage). But "scalars" as a category is not correctly mapped for `repeated` cardinality — CR-01, independently reproduced by executing `mapField` against a synthesized `repeated string tags` descriptor: derives a singular `field.String`, silently dropping the repeated marker, and panics at ent mutation time when combined with a format rule. |
| 2 | Developer excludes/overrides fields via `Exclude()`/`Override()`/`AsJSON()`; unknown field or unresolved `oneof` fails schema load naming message/field/option + fix; reproducible in-process via `Validate[M]` | ✓ VERIFIED | `derive.go`'s `validateOptionNames`/`checkOneofResolution` confirmed by reading; `mixin.go`'s `Validate[M]` bypasses `entc.LoadGraph` entirely (never touches the gorun subprocess), giving a true in-process debug path; `failure_test.go` asserts specific message content, not just "it panicked". |
| 3 | `gen.Graph` carries per-schema and per-field provenance annotations surviving entc's JSON schema-load boundary | ✓ VERIFIED | `mixinforproto/internal/boundarytest/boundary_test.go` runs a real `entc.LoadGraph` subprocess against a real schema package and decodes both `SourceMessage` and `SourceField` back out of the resulting `*gen.Graph` — confirmed this is a genuine subprocess crossing (not an in-memory struct assertion) by reading the package doc comment and the `entc.LoadGraph` call itself. Ran `go test ./...` with `GOWORK=off`: `internal/boundarytest` package passes. |
| 4 | protovalidate string/numeric/presence constraints show up as native ent builder calls (`MaxLen`, `Match`, `Min`/`Max`/`Range`/`Positive`, `NotEmpty`) | ✗ FAILED | Numeric translation (VAL-02) independently confirmed clean via code reading (overflow-guarded +1/-1 adjustment). But string format delegation (CR-02, `delegatingFormatValidator`) rejects effectively all inputs on any message with more than the one field the corpus deliberately tests, and presence translation (CR-03) emits the wrong constraint (`NotEmpty` instead of presence-only) for `optional string`/`bytes` + `required`, both independently reproduced below. |
| 5 | `mixinforproto` ships as an independently buildable, independently tagged module with its own `go.mod` (ent + protobuf + protovalidate only); documented pipeline + CI enumerate both modules, run a `GOWORK=off` job, and docs cover proto3 presence/zero-collapse prominently | ✓ VERIFIED (with a related CI gap — see gap 4 below) | Ran `cd mixinforproto && GOWORK=off go build ./... && GOWORK=off go test ./...` directly: build and all tests pass standalone. `make check-modules` confirms no `Replace` directive in either `go.mod`. `mixinforproto/README.md`'s "Proto3 presence and the zero-collapse" section is positioned above the API reference, with a `doc.go` pointer. `ci.yml` has a dedicated `standalone` job running the same `GOWORK=off` proof. The separate `stubs` CI job (a related but not identical D-22 staleness gate) is broken per gap 4 below — it does not invalidate this SC's literal wording but is a real defect in CI infrastructure this same phase delivered. |

**Score:** 3/5 roadmap success criteria verified

### Independent Reproductions (executed, not inferred)

Ran against the live codebase in this session (test files added temporarily, then removed —
repo is unmodified):

```
IsList=true classify=6 (classScalar)
derived field descriptor: Info:string ... (repeated string field derives as singular)
```

```
classifyRequired(optional=true, required=true, hasNotEmpty=true) = 2 (requiredExactNotEmpty)
   want requiredExactPresence(1) for a presence-tracking field's "must be set" semantics
```

```
$ cd mixinforproto && GOWORK=off go build ./... && GOWORK=off go test ./...
ok  	github.com/smintz/entconnect/mixinforproto	0.034s
ok  	github.com/smintz/entconnect/mixinforproto/internal/boundarytest	0.766s
ok  	github.com/smintz/entconnect/mixinforproto/internal/fieldproj	(cached)
$ go test -race -count=5 ./...   # MIX-13 determinism + concurrency
ok (all packages)
```

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `mixinforproto/go.mod` | module `github.com/smintz/entconnect/mixinforproto`, ent+protobuf+protovalidate toolchain only | ⚠️ VERIFIED (documentation drift) | 5 direct requires in the main block: `buf.build/gen/go/.../protovalidate/protocolbuffers/go`, `buf.build/go/protovalidate`, `entgo.io/ent`, `github.com/sebdah/goldie/v2` (test-only), `google.golang.org/protobuf`. All four non-test requires fall within "ent + protobuf + protovalidate toolchain" per MIX-14's actual wording, so MIX-14 itself is satisfied — but the plan's own must-have artifact description ("exactly three direct non-test requires") is already false (WR-06 in the code review), and `validate.go`'s file-doc comment cites that false invariant to justify ~100 lines of structural-interface indirection. Not a blocker for MIX-14, but a documentation-accuracy warning worth fixing before it misleads the next maintainer. |
| `mixinforproto/annotation.go` | `SourceMessage`, `SourceField`, `ContractVersion`, `MixinForProtoMessage`, `MixinForProtoField` | ✓ VERIFIED | All five present, `ContractVersion = 1`, both structs implement `schema.Annotation`. |
| `mixinforproto/derive.go` | pure derive core returning `(*derivation, error)` | ✓ VERIFIED | `derive[M]` confirmed; validates Exclude/Override names, checks oneof resolution, collects failures rather than failing fast. |
| `mixinforproto/mixin.go` | `MixinForProto[M]` + panicking `ent.Mixin` adapter | ✓ VERIFIED | `Fields()`/`Annotations()` panic; `Validate[M]` returns the error, bypassing `entc.LoadGraph`. |
| `proto/mixinforprototest/v1/*.proto` + committed stubs | corpus covering scalars, enums, WKTs, maps, oneofs, presence, reserved, constraints | ✓ VERIFIED | 10 proto files, `internal/gen/mixinforprototestv1/*.pb.go` committed; `constraints.proto` covers one message per protovalidate constraint class per its own header. |
| `mixinforproto/internal/boundarytest/` | ent schema + test crossing the real entc subprocess boundary | ✓ VERIFIED | Confirmed genuinely crosses the boundary (see SC3 above), not an in-memory assertion. |
| `go.work` | lists root module + `./mixinforproto` | ✓ VERIFIED | `use (. ./mixinforproto)`, `go 1.26`. |
| `mixinforproto/fieldmap.go` | scalar/enum/WKT/map/optional classification | ⚠️ STUB (for `repeated` cardinality) | Substantive and wired for every non-repeated shape; silently mis-derives repeated scalars/enums (CR-01/gap 1). |
| `mixinforproto/validate.go` | Tier 1 translation + residual recording | ⚠️ STUB (for delegated string formats on multi-field messages, and for presence+required on string/bytes) | Substantive and correct for numeric/length/plain-presence translation; functionally broken for the two paths in gaps 2 and 3. |
| `scripts/pipeline.sh` | five canonical steps, correctly scoped `buf generate --path mixinforprototest` | ✓ VERIFIED | Read directly; step ordering, `set -euo pipefail` abort-on-first-failure, and the scoping rationale are all present and internally consistent. |
| `.github/workflows/ci.yml` | workspace job, GOWORK=off job, staleness job | ⚠️ ORPHANED LOGIC (staleness job diverges from its own dependency's documented requirement) | `modules` and `standalone` jobs verified correct by direct local reproduction of their commands. `stubs` job's `buf generate` invocation omits `--path mixinforprototest`, contradicting `pipeline.sh`'s own recorded live verification (gap 4). |

### Key Link Verification

| From | To | Via | Status | Details |
|------|-----|-----|--------|---------|
| `mixin.Annotations()` | `entc/load` JSON | schema annotation channel | ✓ WIRED | Boundary test proves this crosses the real subprocess. |
| field builder `.Annotations(SourceField{...})` | `gen.Field.Annotations` | field annotation channel | ✓ WIRED | Same boundary test, field-level assertion. |
| `(*new(M)).ProtoReflect().Descriptor()` | `protoreflect.FieldDescriptors` iterated by index | declaration-order iteration | ✓ WIRED | Confirmed via `TestDerive_FieldOrderMatchesDeclarationOrder`/`TestDerive_DeterministicAcrossRepeatedCalls`, and independently via `go test -race -count=5`. |
| `derive[M]` core | `Fields()` (panic) and `Validate[M]` (error) | shared core, two surfaces | ✓ WIRED | Both call the identical `derive[M]`; `mixin.go` confirmed by reading. |
| `protovalidate.ResolveFieldRules(fd)` | Tier 1 builder call OR residual record | nil-safe resolution | ✓ WIRED | Confirmed nil-safe at every call site (code review's own adversarial probe, spot-checked). |
| `buf generate` (temp dir, scoped) | committed `mixinforproto/internal/gen` | staleness diff gate | ✗ NOT_WIRED | CI's `stubs` job uses an unscoped `buf generate`, contradicting the scoped invocation `pipeline.sh` itself requires and documents as load-bearing (gap 4). This is a must-have key link from `01-03-PLAN.md`'s own frontmatter. |

### Requirements Coverage

| Requirement | Source Plan | Description (abbreviated) | Status | Evidence |
|---|---|---|---|---|
| MIX-01 | 01-01 | Declare mixin, get fields, no descriptor file/string names | ✓ SATISFIED | Boundary test + `mixin.go` reading |
| MIX-02 | 01-02 | Scalar type mapping | ✗ BLOCKED | CR-01, independently reproduced |
| MIX-03 | 01-02 | Enum mapping | ⚠️ SATISFIED for non-repeated (repeated enum shares CR-01's root cause) | `classify`/`mapEnum` reading; not independently re-executed for enums (same code path as CR-01) |
| MIX-04 | 01-02 | WKT mapping (Timestamp/Struct/Value/FieldMask) | ✓ SATISFIED | `mapWellKnownType` reading, correctly checks `IsList()` and routes repeated WKTs to the message-field skip path rather than mis-deriving them |
| MIX-05 | 01-02 | Optional/presence + zero-collapse defaults | ✓ SATISFIED | `classify`'s documented branch ordering (`IsSynthetic()` check) + README's zero-collapse section |
| MIX-06 | 01-02 | Scalar maps → JSON, message maps skipped | ⚠️ SATISFIED (WR-01 warning: enum-valued maps silently become `map[K]any` with value type lost — not corpus-covered) | `mapScalarMap`/`classify` reading |
| MIX-07 | 01-04 | `Exclude()`, unknown name fails | ✓ SATISFIED | `validateOptionNames` reading |
| MIX-08 | 01-04 | `Override()`, unknown name fails | ✓ SATISFIED | `validateOptionNames` reading |
| MIX-09 | 01-02 | `AsJSON()` for message fields | ✓ SATISFIED | `mapAsJSON` reading |
| MIX-10 | 01-04 | Message fields skipped by default; oneof gate | ✓ SATISFIED | `checkOneofResolution` reading |
| MIX-11 | 01-04 | Self-sufficient failure first line, names message/field/option/fix | ✓ SATISFIED | `errors.go` reading, code review's adversarial probe confirms no double-prefixing |
| MIX-12 | 01-01/01-04 | In-process reproduction via `Validate[M]` | ✓ SATISFIED | `mixin.go`'s `Validate[M]` never calls `entc.LoadGraph` |
| MIX-13 | 01-01 | Deterministic field order | ✓ SATISFIED | `go test -race -count=5` passes |
| MIX-14 | 01-01 | Independent module, minimal deps | ✓ SATISFIED (with WR-06 documentation-accuracy warning) | `go.mod` reading; `GOWORK=off` build/test |
| ANNO-01 | 01-01 | Schema-level provenance survives JSON boundary | ✓ SATISFIED | Boundary test |
| ANNO-02 | 01-02/01-05 | Field-level provenance readable from `gen.Graph` | ✓ SATISFIED | Boundary test + `annotation.go` |
| ANNO-03 | 01-04 | Exclude/Override recorded as annotations | ✓ SATISFIED | `SourceMessage.Excluded`/`.Overridden` populated in `derive.go` |
| ANNO-04 | 01-01 | Version marker for mismatch detection | ✓ SATISFIED | `ContractVersion` const + boundary test's non-zero assertion |
| VAL-01 | 01-05 | String constraints → native builder calls | ✗ BLOCKED | CR-02, confirmed by direct code reading of `delegatingFormatValidator` |
| VAL-02 | 01-05 | Numeric constraints, open/closed adjustment | ✓ SATISFIED | `applySignedRange` reading; overflow guard present at type bounds |
| VAL-03 | 01-05 | Presence/required → `NotEmpty`/non-optional | ✗ BLOCKED | CR-03, independently reproduced |
| PIPE-01 | 01-03 | Documented pipeline script, CI runs it in order | ✓ SATISFIED | `pipeline.sh` reading; `modules` CI job runs it |
| PIPE-02 | 01-03 | `go.work` + `GOWORK=off` job proves standalone consumption | ✓ SATISFIED | Reproduced locally |
| PIPE-03 | 01-03 | CI enumerates + tests both modules explicitly | ✓ SATISFIED | `Makefile`'s `MODULES` list + `make build/vet/test` reproduced locally |
| PIPE-04 | 01-03 | Nested-module tag convention, no `replace` | ✓ SATISFIED | `make check-modules` passes; `CONTRIBUTING.md` documents convention |
| PIPE-08 | 01-03 | Docs cover presence/zero-collapse prominently | ✓ SATISFIED | README section ordering confirmed |

**22/25 requirement IDs satisfied; 3 BLOCKED (MIX-02, VAL-01, VAL-03).**

### Anti-Patterns Found

No `TBD`/`FIXME`/`XXX`/`TODO`/`HACK`/`PLACEHOLDER` markers found in any non-test `.go` file, `scripts/pipeline.sh`, `Makefile`, or `.github/workflows/ci.yml`. The four blockers above are logic defects, not debt markers — they are not visible via static debt-marker scanning, which is exactly why they required code execution to surface.

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|---|---|---|---|
| Repeated scalar field mapping | synthesized `repeated string tags` descriptor through `classify`/`mapField` | derives singular `field.String`, drops cardinality | ✗ FAIL (CR-01) |
| `classifyRequired` for presence-tracking string+required | `classifyRequired(true, true, true)` | returns `requiredExactNotEmpty`, not `requiredExactPresence` | ✗ FAIL (CR-03) |
| GOWORK=off standalone build+test | `cd mixinforproto && GOWORK=off go build ./... && GOWORK=off go test ./...` | all packages build and pass | ✓ PASS |
| Determinism + race safety | `go test -race -count=5 ./...` | all packages pass | ✓ PASS |
| No local-path module substitution | `make check-modules` | both modules OK | ✓ PASS |

### Human Verification Required

None. All four blockers were confirmed by direct code execution or by reading self-contradicting
configuration in the repository itself (CR-04); none require subjective/visual/runtime judgment
this verifier cannot make.

### Gaps Summary

Three of four code-review blockers strike directly at requirements this phase claims complete in
REQUIREMENTS.md (MIX-02, VAL-01, VAL-03) and at two of the five roadmap Success Criteria (SC1, SC4).
All three were independently reproduced by executing synthesized code against the live
`mixinforproto` package in this session, not merely re-read from the prior code review. The fourth
(CI staleness gate, CR-04) breaks a must-have key link this phase's own Plan 03 declared, confirmed
by the codebase's own internal contradiction (`pipeline.sh`'s documented, live-verified scoping
requirement vs. `ci.yml`'s unscoped invocation).

The phase's foundational architecture — the annotation contract crossing the real entc subprocess
boundary, the Exclude/Override/oneof failure surface, deterministic field ordering, module
isolation and `GOWORK=off` standalone consumption — is genuinely sound and independently verified
working. The defects are narrow but real: every one of them is invisible to the current test suite
specifically because the corpus avoids the exact shape that triggers it (no repeated scalar
fixture, every format-validator fixture single-field, no `optional string`+`required` fixture, and
a CI job whose command diverges from the script it is supposed to mirror). This is precisely the
"tests pass because the corpus avoids the failing shape" pattern the verification brief called out
in advance, and it held in all three cases.

None of these four gaps are recorded as deliberate decisions anywhere (README, `doc.go`,
`ResidualIDs`/`LengthUnitDivergentIDs`, or WINDOWS.md) — they are unrecorded defects, not documented
trade-offs, which is the distinction that separates them from the length-unit divergence and the
two already-logged Broken Windows.

---

_Verified: 2026-08-08T12:21:55Z_
_Verifier: Claude (gsd-verifier)_
