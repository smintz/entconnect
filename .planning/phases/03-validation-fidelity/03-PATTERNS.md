# Phase 3: Validation Fidelity - Pattern Map

**Mapped:** 2026-08-14
**Files analyzed:** 11 (new/modified)
**Analogs found:** 11 / 11

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|---|---|---|---|---|
| `mixinforproto/reverse.go` (D-05) | utility (per-`Kind` conversion table) | transform | `mixinforproto/fieldmap.go` (the forward table it mirrors) | exact — same file family, opposite direction |
| `mixinforproto/hooks.go` (D-02/D-07/D-08) | mixin runtime hook / middleware | event-driven (ent `Hooks()`, mutation-time) | `mixinforproto/validate.go` (Tier 1 translation: `ResolveFieldRules`, `protovalidate.New`, structural interfaces) | exact for the protovalidate-usage half; no direct hook-file analog exists yet — this is the mixin's first `Hooks()` |
| `mixinforproto/messagerules.go` (D-10) | schema-load validator / config | request-response (schema-load-time panic) | `mixinforproto/derive.go`'s `validateAsJSON`/`validateOptionNames` pattern (option-name field-reference walk, panics via `failure`) | role-match — same "walk + collect `failure` + panic-if-any" shape |
| `mixinforproto/errors.go` (extend: shared violation constructor, D-01/D-07 consequence 3) | error/utility | transform | existing `mixinforproto/errors.go` (`failure`, `derivationError`) — new code should sit **beside**, not replace, this machinery | exact — same file |
| `mixinforproto/internal/difftest/` (D-13 driverless sweep, PIPE-06) | test harness | batch / differential | `mixinforproto/internal/boundarytest/boundary_test.go` (real entc-adjacent, no-DB-driver harness pattern) + `mixinforproto/corpus_test.go` (corpus enumeration/coverage-map pattern) | exact — explicitly named in CONTEXT.md as the sibling pattern |
| `mixinforproto/testdata/*.golden` (PIPE-05 corpus extension) | test fixture | batch | `mixinforproto/corpus_test.go` + existing `testdata/*.golden` + `corpusCoverage` map | exact — literal extension of existing mechanism |
| `runtime/errormap.go` (extend: `*protovalidate.ValidationError` case) | middleware / error-mapping | request-response | existing `runtime/errormap.go` `MapError` switch | exact — same file, new case |
| `runtime/doc.go` (update stale Phase-3 caveat) | doc/config | — | existing `runtime/doc.go:12` | exact — same file |
| `internal/entconnecttest/<new fixture>/` (D-13 wiring proof) | test harness (real ent client, sqlite) | request-response / CRUD | `internal/entconnecttest/update/` (`update_test.go` + `ent/schema/patch.go`) | exact — explicitly named analog in CONTEXT.md |
| `.github/workflows/ci.yml` + `Makefile` (D-15 parity gate) | CI config | batch | existing `standalone` job (`GOWORK=off`, `make test-standalone*`) + `Makefile`'s `MODULES := . ./mixinforproto` | exact — same file, new step/target |
| `mixinforproto/go.mod` (D-07/D-16: cel-go promoted to direct) | config | — | existing `mixinforproto/go.mod` require block (already lists `buf.build/go/protovalidate`, `entgo.io/ent` as direct, `google/cel-go` as indirect) | exact — same file, require-block edit |

## Pattern Assignments

### `mixinforproto/reverse.go` (utility, transform) — D-05

**Analog:** `mixinforproto/fieldmap.go`

**Imports pattern** (fieldmap.go lines 1-14):
```go
package mixinforproto

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"google.golang.org/protobuf/reflect/protoreflect"

	"buf.build/go/protovalidate"
)
```
Reverse.go will instead need `google.golang.org/protobuf/types/dynamicpb`, `google.golang.org/protobuf/encoding/protojson`, `google.golang.org/protobuf/types/known/timestamppb` — add these, keep the rest of the import shape (single-package grouping, no aliasing).

**Core per-`Kind` switch pattern to mirror** (fieldmap.go lines 56-76, `classify`, and lines 824-842, `mapWellKnownType`):
```go
func classify(fd protoreflect.FieldDescriptor) fieldClass {
	switch {
	case fd.IsMap():
		if fd.MapValue().Kind() == protoreflect.MessageKind {
			return classMessageMap
		}
		return classScalarMap
	case fd.IsList():
		return classRepeated
	case fd.ContainingOneof() != nil && !fd.ContainingOneof().IsSynthetic():
		return classRealOneofMember
	case fd.HasOptionalKeyword():
		return classOptionalScalar
	case fd.Kind() == protoreflect.MessageKind:
		return classMessageField
	case fd.Kind() == protoreflect.EnumKind:
		return classEnum
	default:
		return classScalar
	}
}
```
The reverse table should switch on `SourceField.Kind` string values (`"scalar"`, `"optionalScalar"`, `"enum"`, `"wkt"`, `"scalarMap"`, `"asJSON"` — see `sourceFieldFor`/`finalizeSourceField`, fieldmap.go lines 154-193) rather than re-deriving `fieldClass` from a live descriptor, since the mixin hook only has the ent mutation's Go value plus the field's own `fd`, not a fresh classify() pass.

**Error handling pattern to reuse** — fail closed per D-12, distinct from a `failure`/panic:
```go
// D-12: a runtime reverse-conversion failure is NOT a protovalidate
// violation. No constraint ID is synthesized.
return protoreflect.Value{}, fmt.Errorf("mixinforproto: reverse-converting field %q (kind %s): %w", fd.Name(), class, err)
```
This should be a plain `error`, never wrapped in `*failure`/`derivationError` (those are schema-load-time only) and never wrapped in `*protovalidate.ValidationError` (that would fabricate a constraint ID — the exact thing D-12 forbids).

**WKT/enum/JSON conversion snippets** (already verified live in `03-RESEARCH.md` Pattern 3 against real pinned dependency source — copy these call shapes directly):
```go
case "wkt":
	t := entValue.(time.Time)
	return protoreflect.ValueOfMessage(timestamppb.New(t).ProtoReflect()), nil
case "enum":
	name := entValue.(string)
	evd := fd.Enum().Values().ByName(protoreflect.Name(name))
	if evd == nil {
		return protoreflect.Value{}, fmt.Errorf("enum string %q not in descriptor", name) // D-12
	}
	return protoreflect.ValueOfEnum(evd.Number()), nil
case "asJSON", "scalarMap": // both go through dynamicpb + protojson
	raw := entValue.(json.RawMessage)
	dyn := dynamicpb.NewMessage(fd.Message())
	if err := protojson.Unmarshal(raw, dyn); err != nil {
		return protoreflect.Value{}, err // D-12
	}
	return protoreflect.ValueOfMessage(dyn), nil
```

---

### `mixinforproto/hooks.go` (mixin hook / middleware, event-driven) — D-02/D-03/D-06/D-07/D-08

**Analog (protovalidate usage half):** `mixinforproto/validate.go`

**Imports pattern** (validate.go lines 1-15) — note the deliberate avoidance of naming `*validate.FieldRules`'s package (see that file's own doc comment, lines 17-56, which D-01 explicitly revisits/relaxes):
```go
import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"sort"

	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"

	"buf.build/go/protovalidate"
)
```
`hooks.go` will additionally need `"entgo.io/ent"` (for the `ent.Hook`/`ent.Mixin` `Hooks()` signature) and, per D-07, a **direct** `"github.com/google/cel-go/cel"` import plus `"buf.build/go/protovalidate/cel"` (aliased e.g. `pvcel`) — this is the file where cel-go's promotion from indirect to direct actually gets consumed.

**"Resolve rules per field descriptor" pattern to reuse verbatim** (fieldmap.go lines 195-226, `resolvedFieldRules` — D-03 explicitly says this is "the same `ResolveFieldRules` path Tier 1 already uses"):
```go
func resolvedFieldRules(fd protoreflect.FieldDescriptor) (
	required bool, celResidual []string, celEntries []residualEntry, err error,
) {
	rules, err := protovalidate.ResolveFieldRules(fd)
	if err != nil {
		return false, nil, nil, fmt.Errorf("resolving protovalidate field rules: %w", err)
	}
	if rules == nil {
		return false, nil, nil, nil
	}
	...
}
```

**Delegating-to-protovalidate's-own-evaluator pattern to reuse** (validate.go lines 401-431, `delegatingFormatValidator` — this is the closest existing precedent for "build a synthetic `dynamicpb.Message`, hand it to a real `protovalidate.Validator`, and filter the resulting violations down to the field(s) you actually care about," which is exactly Pattern 2 (`WithFilter`) from RESEARCH.md):
```go
func delegatingFormatValidator(fd protoreflect.FieldDescriptor, formatName string) (func(string) error, error) {
	v, err := protovalidate.New()
	if err != nil {
		return nil, fmt.Errorf("building protovalidate validator for %s format delegation: %w", formatName, err)
	}
	md := fd.ContainingMessage()
	fieldName := fd.FullName()
	return func(s string) error {
		msg := dynamicpb.NewMessage(md)
		msg.Set(fd, protoreflect.ValueOfString(s))
		verr := v.Validate(msg)
		if verr == nil {
			return nil
		}
		var ve *protovalidate.ValidationError
		if !errors.As(verr, &ve) {
			return fmt.Errorf("evaluating the %s format constraint: %w", formatName, verr)
		}
		for _, viol := range ve.Violations {
			if viol.FieldDescriptor != nil && viol.FieldDescriptor.FullName() == fieldName {
				return fmt.Errorf("value does not satisfy the %s format constraint", formatName)
			}
		}
		return nil
	}, nil
}
```
`hooks.go`'s standard-rule half is the same shape scaled to the whole in-scope field set via `protovalidate.WithFilter(scope)` (RESEARCH.md Pattern 2) instead of a single synthetic single-field message — build the `protovalidate.Validator` **once at `Hooks()`-construction time** (schema load), not per-call, using `protovalidate.WithMessages(exampleMsg)` + `protovalidate.WithDisableLazy()` (RESEARCH.md Pattern 1 / Code Examples), mirroring `delegatingFormatValidator`'s "build once, close over it" shape but hoisted out of the per-request closure entirely.

**Error handling / D-09 schema-load panic pattern to reuse** — `mixinforproto/errors.go`'s `failure`/`derivationError`:
```go
type failure struct {
	message     string
	field       string
	fieldIndex  int
	rule        string
	description string
	remedy      string
}
```
Any uncompilable CEL expression or uncompilable standard-rule evaluator discovered while building `hooks.go`'s schema-load-time state must become a `failure{rule: "hook"}` fed through `newDerivationError`, exactly like every other schema-load failure path in this package — do not introduce a second panic mechanism.

---

### `mixinforproto/messagerules.go` (schema-load validator, request-response) — D-10

**Analog:** `mixinforproto/derive.go`'s `validateAsJSON` (in `fieldmap.go` lines 867-915) and `option.go`'s `applyOptions`/`Option` pattern.

**Core "walk + collect `failure`" pattern to reuse** (fieldmap.go lines 877-915):
```go
func validateAsJSON(msgName string, md protoreflect.MessageDescriptor, o *options) []failure {
	var out []failure
	for _, name := range o.asJSONNames() {
		if name == "" {
			out = append(out, failure{
				message: msgName, field: "", fieldIndex: fieldIndexUnnamed,
				rule: "AsJSON", description: "AsJSON(\"\") field name must not be empty",
				remedy: "name the message field you want serialized as JSON",
			})
			continue
		}
		fd := md.Fields().ByName(protoreflect.Name(name))
		if fd == nil {
			out = append(out, failure{ ... })
			continue
		}
		...
	}
	return out
}
```
`messagerules.go`'s field-reference walk over a compiled `WithMessageRules(OnCreate)` CEL expression should build the identical `[]failure` shape (message-scoped: `fieldIndex: fieldIndexMessageScoped`, per errors.go's documented sentinel), naming the rule and the offending field, and be folded into the *same* collected-failures pass `derive.go` already runs — not a separate panic call site.

**`Option`-declaration shape to mirror** (option.go lines 9-14, 100-116) for `WithMessageRules`'s eventual declaration (currently deliberately absent per `option.go:11`):
```go
type Option func(*options)

func Override(name string, f ent.Field) Option {
	return func(o *options) {
		o.overridden[name] = f
	}
}
```

---

### `mixinforproto/errors.go` (extend) — D-01/D-07 consequence 3

**Analog:** the existing file itself — `failure`/`derivationError` stay untouched; the new shared violation constructor is an **additional**, separate piece of machinery for **runtime** (not schema-load) errors, since `failure`/`derivationError` are explicitly schema-load-only (every existing call site is inside `derive`/`fieldmap`/`option` validation).

**Pattern to follow for the new constructor** (per RESEARCH.md Code Examples, using the same "one shared function, both evaluation paths feed it" discipline the file's own doc comment on `failure` establishes for schema-load failures):
```go
// Source shapes verified: buf.build/go/protovalidate@v1.2.0/violation.go:26-46
func newValidationError(violations []*validate.Violation) *protovalidate.ValidationError {
	out := make([]*protovalidate.Violation, len(violations))
	for i, v := range violations {
		out[i] = &protovalidate.Violation{Proto: v}
	}
	return &protovalidate.ValidationError{Violations: out}
}
```
Per D-07 consequence 3: this must be the **only** place either evaluation path (protovalidate's own evaluator, or the local `cel.Env`) constructs a `*protovalidate.ValidationError`. Do not let the residual/CEL path grow its own error-building code — route both through this one function, placed either in `errors.go` itself (adjacent to `failure`, with a doc comment distinguishing "schema-load collected failures" from "mutation-time protovalidate violations") or a small paired file — Claude's Discretion item 3's "adjacency, not a specific filename" rule applies here too.

---

### `mixinforproto/internal/difftest/` (test harness, batch/differential) — D-13, PIPE-06

**Analog 1 (no-DB-driver, real generated-package pattern):** `mixinforproto/internal/boundarytest/boundary_test.go`

**Pattern to reuse — module doc comment discipline and "no mock, no DB, real generated code" framing** (boundary_test.go lines 1-27):
```go
// Package boundarytest is the walking skeleton's real end-to-end
// verify: it runs a real entc schema-load subprocess against a real ent
// schema package declaring MixinForProto, and asserts both provenance
// annotations decode back out of the resulting *gen.Graph.
package boundarytest

import (
	"entgo.io/ent/entc"
	"entgo.io/ent/entc/gen"

	"github.com/smintz/entconnect/mixinforproto"
)

func TestAnnotationsCrossSchemaLoadBoundary(t *testing.T) {
	graph, err := entc.LoadGraph("./ent/schema", &gen.Config{
		Target:  t.TempDir(),
		Package: "github.com/smintz/entconnect/mixinforproto/internal/boundarytest/ent",
	})
	...
}
```
`difftest`'s harness differs in one key way: it needs a **real generated `ent.Client`** driving `Save(ctx)` (not just a schema-load JSON round-trip), so it is a sibling package that also does a real `entc.LoadGraph`-style fixture generation step but then imports the generated `ent` package and drives mutations against it with the fake `dialect.Driver` (RESEARCH.md Code Examples' `fakeDriver`, `entgo.io/ent@v0.14.6/dialect/dialect.go:36-45` — small 5-method interface, no DB import).

**Analog 2 (corpus enumeration / coverage-map / determinism discipline):** `mixinforproto/corpus_test.go`

**Pattern to reuse — corpus enumeration via the proto registry, never a hand-maintained list** (corpus_test.go lines 64-96):
```go
func corpusMessages(t *testing.T) []protoreflect.MessageDescriptor {
	t.Helper()
	var out []protoreflect.MessageDescriptor
	var walk func(mds protoreflect.MessageDescriptors)
	walk = func(mds protoreflect.MessageDescriptors) {
		for i := 0; i < mds.Len(); i++ {
			md := mds.Get(i)
			if md.IsMapEntry() {
				continue
			}
			out = append(out, md)
			walk(md.Messages())
		}
	}
	protoregistry.GlobalFiles.RangeFilesByPackage(
		protoreflect.FullName("mixinforprototest.v1"),
		func(fd protoreflect.FileDescriptor) bool { walk(fd.Messages()); return true },
	)
	if len(out) == 0 {
		t.Fatal("corpusMessages: zero messages found ... registry-linkage regression?")
	}
	sort.Slice(out, func(i, j int) bool { return out[i].FullName() < out[j].FullName() })
	return out
}
```
PIPE-06's differential sweep should walk this exact same corpus (`mixinforprototest.v1` package) rather than a separate fixture set, and per D-14 seed its `math/rand` (or equivalent) generator deterministically from each message's `FullName()` string, printing the seed and the failing generated value on any assertion failure — mirroring this file's own "sorted, deterministic, diffable failure output" discipline (D-24) throughout.

**Coverage-map discipline to reuse** (corpus_test.go lines 288-365, `corpusCoverage` + `TestCorpusMessagesHaveRecordedCoverage`) — if PIPE-05's corpus extension adds new proto messages for constraint classes not yet exercised, they must gain a `corpusCoverage` entry (`"golden:<name>"` or `"test:<TestName>"`) in the same two-way-checked map, not a silently-uncovered fixture.

---

### `mixinforproto/testdata/*.golden` (test fixture, batch) — PIPE-05

**Analog:** existing `mixinforproto/testdata/constraints_residual_cel.golden` and its sibling golden files, asserted via `github.com/sebdah/goldie/v2` per `mixinforproto/corpus_test.go`'s own established pattern and CLAUDE.md's "golden-file tests for generated code follow the entgql/entproto precedent" directive. New corpus protos go through `scripts/generate-stubs.sh` + `make check-stubs`, never hand-edited stub output (per CONTEXT.md's Reusable Assets list).

---

### `runtime/errormap.go` (extend) — Claude's Discretion item 2

**Analog:** the file itself.

**Current switch to extend** (errormap.go lines 38-52):
```go
func MapError(err error) error {
	if err == nil {
		return nil
	}
	var connectErr *connect.Error
	switch {
	case errors.Is(err, privacy.Deny):
		return connect.NewError(connect.CodePermissionDenied, err)
	case errors.As(err, &connectErr):
		return err
	default:
		log.Printf("entconnect: unmapped error: %v", err)
		return connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
}
```
**New case to add** (RESEARCH.md Code Examples, matching `connectrpc.com/validate`'s own mapping for the identical error type reaching it at the boundary — this is VAL-07's identity guarantee made concrete in code):
```go
var valErr *protovalidate.ValidationError
switch {
case errors.Is(err, privacy.Deny):
	return connect.NewError(connect.CodePermissionDenied, err)
case errors.As(err, &valErr):
	return connect.NewError(connect.CodeInvalidArgument, valErr)
case errors.As(err, &connectErr):
	return err
default:
	...
}
```
Add `"buf.build/go/protovalidate"` to this file's import block (it does not currently import it — `runtime/interceptor.go` already does, so the module is already a resolved dependency of `runtime`, just not yet imported by `errormap.go`). Update this file's own doc comment (lines 11-37) to add a fourth bullet for `*protovalidate.ValidationError`, following the exact same "why this type, specifically, can live in the shared package" justification style already used for `privacy.Deny` and `*connect.Error`.

---

### `internal/entconnecttest/<new fixture>/` (test harness, request-response/CRUD) — D-13 wiring proof

**Analog:** `internal/entconnecttest/update/` in full — `update_test.go` + `ent/schema/patch.go`.

**Server/client-wiring pattern to reuse** (update_test.go lines 56-94):
```go
func newTestClient(t *testing.T) *ent.Client {
	t.Helper()
	db, err := stdsql.Open("sqlite", "file:"+t.Name()+"?mode=memory&cache=shared&_pragma=foreign_keys(1)")
	...
	drv := entsql.OpenDB(dialect.SQLite, db)
	client := ent.NewClient(ent.Driver(drv))
	...
	if err := client.Schema.Create(context.Background()); err != nil { ... }
	return client
}

func newTestServer(t *testing.T, client *ent.Client, authenticator testAuthenticator) entconnecttestv1connect.PatchUpdateServiceClient {
	t.Helper()
	srv, err := entconnect.NewServer(client, authenticator)
	...
	mux := http.NewServeMux()
	srv.Register(mux)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return entconnecttestv1connect.NewPatchUpdateServiceClient(ts.Client(), ts.URL)
}
```
D-13's wiring-proof test should follow this exact shape (real HTTP server, real sqlite, real generated Connect client) with a mutation whose fixture schema declares a `MixinForProto`-derived field carrying a real protovalidate constraint (e.g. `string.min_len`), asserting the Create/Update call rejects with `CodeInvalidArgument` and the same violation content the boundary interceptor would produce for the identical bad input — this is what proves the hook is genuinely wired into the real mutation path (not just present in code).

**Schema-fixture pattern to reuse** (patch.go, full file read):
```go
func (Patch) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixinforproto.MixinForProto[*entconnecttestv1.Patch](mixinforproto.Exclude("id", "internal_note")),
	}
}
func (Patch) Annotations() []entschema.Annotation {
	return []entschema.Annotation{
		entconnect.UpdateRPC(entconnecttestv1connect.PatchUpdateServiceUpdatePatchProcedure),
	}
}
func (Patch) Policy() ent.Policy { ... }
```
Both the Create-with-policy and Create/Update-without-policy configurations named by VAL-09's Pitfall 3 requirement should exist as sibling fixtures (or a second schema type in the same package) so the ordering test can assert the *relative* mixin-hook-vs-policy position in both cases, not a hard-coded index.

---

### `.github/workflows/ci.yml` + `Makefile` (D-15 parity gate)

**Analog:** the existing `standalone` job (ci.yml lines 90-107) and `Makefile`'s `MODULES` variable (line 10) plus its `test-standalone`/`test-standalone-root` targets.

**Pattern to extend, not replace:**
```yaml
standalone:
  name: standalone
  runs-on: ubuntu-latest
  steps:
    - uses: actions/checkout@v4
    - uses: actions/setup-go@v5
      with:
        go-version: "1.26"
    - name: Confirm go.work is present in this checkout
      run: test -f go.work
    - name: Build and test mixinforproto standalone (GOWORK=off)
      run: make test-standalone
    - name: Build and test the root module standalone (GOWORK=off)
      run: make test-standalone-root
```
D-15's parity gate is a new step in this same job (GOWORK=off is exactly the "resolve independently" context D-15 requires — do not invent a new job), calling a new `Makefile` target (e.g. `check-dep-parity`) that greps/parses both `go.mod`s for the five named modules (`buf.build/go/protovalidate`, `buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go`, `github.com/google/cel-go`, `google.golang.org/protobuf`, `entgo.io/ent`) and fails on any version mismatch — mirroring `Makefile`'s existing `check-goversion`/`check-modules` targets' shape (single-purpose `check-*` targets, each with its own doc comment, each invoked as its own named CI step).

---

### `mixinforproto/go.mod` (D-07/D-16)

**Analog:** the file itself — move `github.com/google/cel-go v0.28.0` from the second (`// indirect`) require block into the first (direct) require block, alongside `buf.build/go/protovalidate`, `entgo.io/ent`, `google.golang.org/protobuf` — no version change, only require-block placement and removing the `// indirect` marker. Root `go.mod` keeps `github.com/google/cel-go v0.28.0 // indirect` unchanged (root never imports cel-go directly this phase).

## Shared Patterns

### Collected-failure / schema-load panic discipline (D-09, D-10)
**Source:** `mixinforproto/errors.go` (`failure`, `derivationError`, `newDerivationError`)
**Apply to:** `hooks.go`'s uncompilable-CEL panic, `messagerules.go`'s excluded-field-reference panic. Every offender must be collected into one `[]failure` and reported in one pass (D-09), never a bare `panic(fmt.Errorf(...))` at the first offense. First line must be self-sufficient (D-08) since entc's schema-load subprocess truncates output.

### `ResolveFieldRules`-based per-field resolution (D-03)
**Source:** `mixinforproto/fieldmap.go`'s `resolvedFieldRules` and every `buildXxxField` function's `protovalidate.ResolveFieldRules(fd)` call site.
**Apply to:** `hooks.go`'s standard-rule and residual-rule routing (D-08) — reuse the identical resolve call, never a hand-rolled reimplementation of rule enumeration.

### Determinism-by-construction (D-24, extended by D-14)
**Source:** `mixinforproto/fieldmap.go`'s `sortUnique`, `mixinforproto/validate.go`'s `residualFingerprint` (sorted-then-hashed), `mixinforproto/corpus_test.go`'s sorted diagnostic output.
**Apply to:** every new sorted-output site (`messagerules.go`'s field-reference report, `difftest`'s corpus walk) and PIPE-06's deterministic-seed-from-message-name requirement (D-14) — never range a map into ordered output; seed and print on failure for reproducibility.

### Fail-closed on data-integrity faults, distinct from validation failures (D-12)
**Source:** no direct precedent exists in the current codebase (this is new territory) — closest analog is `mixinforproto/validate.go`'s `delegatingFormatValidator`'s clean separation between "genuine evaluator failure" (`fmt.Errorf("evaluating the %s format constraint: %w", ...)`) and "value fails the constraint" (a plain sentinel-shaped error) — the same three-way split (success / real violation / infrastructure fault) recurs in `reverse.go`'s D-12 error path and must stay a plain `error`, never wrapped as `*protovalidate.ValidationError`.

### Two-module, no-DB-driver hygiene (Phase 1 D-15/D-16/D-17, extended by D-13)
**Source:** `mixinforproto/go.mod` (no DB driver in require block, verified in RESEARCH.md), `Makefile`'s `MODULES := . ./mixinforproto`, `.github/workflows/ci.yml`'s `standalone` job.
**Apply to:** `mixinforproto/internal/difftest/` must add zero new dependencies beyond what `entgo.io/ent` (already direct) and the promoted `google/cel-go` (D-16) provide — the fake `dialect.Driver` is hand-written, not imported.

## No Analog Found

None — every file this phase touches or creates has at least a role-match analog already in the codebase (see Match Quality column above). The two weakest matches are `hooks.go` (no prior `Hooks()` implementation exists in this package — protovalidate-usage patterns from `validate.go` transfer, but the hook-registration shape itself is genuinely new) and the D-12 fail-closed error path in `reverse.go` (no direct precedent; nearest analog noted above under Shared Patterns).

## Metadata

**Analog search scope:** `mixinforproto/` (all `.go` files at package root + `internal/boundarytest/`), `runtime/` (`interceptor.go`, `errormap.go`, `doc.go`), `internal/entconnecttest/update/` (full directory), `.github/workflows/ci.yml`, `Makefile`, both `go.mod` files.
**Files scanned:** `mixinforproto/validate.go`, `mixinforproto/fieldmap.go`, `mixinforproto/errors.go`, `mixinforproto/option.go`, `mixinforproto/corpus_test.go`, `mixinforproto/internal/boundarytest/boundary_test.go`, `runtime/interceptor.go`, `runtime/errormap.go`, `internal/entconnecttest/update/update_test.go`, `internal/entconnecttest/update/ent/schema/patch.go`, `.github/workflows/ci.yml`, `Makefile`, `mixinforproto/go.mod`, `go.mod` (root).
**Pattern extraction date:** 2026-08-14
