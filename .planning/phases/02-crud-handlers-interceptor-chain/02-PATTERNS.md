# Phase 2: CRUD Handlers & Interceptor Chain - Pattern Map

**Mapped:** 2026-08-08
**Files analyzed:** ~18 (per Recommended Project Structure in RESEARCH.md, plus proto corpus + CI additions)
**Analogs found:** 11 with a real (mostly cross-module, structural) analog / 18 total. Several core files are explicitly **greenfield** — see "No Analog Found."

## Orienting Summary

Phase 2 is the first phase to put real code in the root module (`github.com/smintz/entconnect`). `entc/` and `runtime/` do not exist. Every usable analog therefore lives in the sibling `mixinforproto/` module (Phase 1's output) — a **different package with a different job** (runtime schema-derivation mixin vs. build-time entc codegen extension), so matches below are almost all **structural/discipline analogs** (error aggregation shape, annotation decode idiom, golden-test harness, script/CI wiring), not literal code to port. The one genuine architectural analog for the entc-extension skeleton itself is **`entgo.io/contrib/entproto`** (external, read-only precedent, not a dependency) — RESEARCH.md already read and excerpted it directly (`entproto/extension.go`, `message.go`, `field.go`).

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|---|---|---|---|---|
| `entc/extension.go` | config/extension | build-time (hook-based codegen) | `entgo.io/contrib/entproto/extension.go` (external, read-only) | role-match (structural skeleton only) |
| `entc/binder.go` (CreateRPC/UpdateRPC/.../Manual) | config (annotation constructors) | transform (Go value → schema.Annotation) | `mixinforproto/annotation.go` | role-match |
| `entc/resolve.go` (procedure string → MethodDescriptor) | utility | transform / lookup | none in-repo | **no analog** (greenfield — see below) |
| `entc/crud/*.go.tmpl` (Get/Create/Update/Delete/List) | controller (template) | CRUD via text/template emission | none in-repo | **no analog** (greenfield) |
| `entc/errormap.go` | utility | transform (error → Connect code) | `mixinforproto/errors.go` (`derivationError`/`failure`) | role-match (discipline, not literal code) |
| `entc/extension_test.go` (golden) | test | batch/golden-file | `mixinforproto/fieldmap_test.go` (`assertGolden`) | exact (goldie idiom directly reusable) |
| `entc/decode.go` (annotation JSON round-trip) | utility | transform | `mixinforproto/internal/boundarytest/boundary_test.go` (`decodeAnnotation[T any]`) | exact |
| `runtime/viewer/viewer.go` | provider/context helper | request-response (ctx injection) | none in-repo (ent's own `examples/privacyadmin/viewer`, external, not importable) | **no analog** (greenfield, but external non-dependency precedent named in RESEARCH.md) |
| `runtime/interceptors.go` | middleware | request-response (Connect interceptor chain) | none in-repo | **no analog** (greenfield) |
| `runtime/server.go` (`NewServer(client, authenticator, opts...)`) | config/provider (server wiring) | request-response | none in-repo | **no analog** (greenfield) |
| `proto/<corpus>/v1/*.proto` (new service-bearing corpus files) | config (fixture) | N/A | `proto/mixinforprototest/v1/*.proto` | exact (established synthetic-corpus convention) |
| `proto/buf.gen.yaml` (add `protoc-gen-connect-go` plugin) | config | N/A | `proto/buf.gen.yaml` (existing) | exact (modify in place) |
| `mixinforproto/internal/gen/mixinforprototestv1/*.pb.go` equivalents for the new corpus | generated stub | N/A | `mixinforproto/internal/gen/mixinforprototestv1/*.pb.go` | exact (same generation mechanism, `scripts/generate-stubs.sh`) |
| `internal/entconnecttest/boundarytest/*` (new: real-entc-load harness for generated handlers) | test | integration / build-time | `mixinforproto/internal/boundarytest/boundary_test.go` + its `ent/schema` fixture dir | exact (nearest existing analog named explicitly in the orienting note) |
| root `go.mod` (add `connectrpc.com/connect`, `entgo.io/ent`, `github.com/smintz/entconnect/mixinforproto`) | config | N/A | `mixinforproto/go.mod` (dependency-declaration convention, NOT literal deps — root has no module-isolation constraint) | role-match |
| `Makefile` (no structural change expected, `MODULES` already covers root) | config | N/A | `Makefile` (existing) | exact (verify still correct, likely no edit needed) |
| `.github/workflows/ci.yml` (`modules` job's root-module build/vet/test stops being a probed-and-skipped no-op) | config | N/A | `.github/workflows/ci.yml` (existing `modules`/`stubs` jobs) | exact (no new job needed — existing `go list ./...` skip-guard in `Makefile` already handles the transition automatically) |
| `scripts/generate-stubs.sh` (extend `--path` scope for new corpus, if a new top-level proto package is added) | config/script | N/A | `scripts/generate-stubs.sh` (existing) | exact (modify in place, follow its own scoping-rationale comment) |

## Pattern Assignments

### `entc/binder.go` — CreateRPC/UpdateRPC/DeleteRPC/GetRPC/ListRPC/Manual annotation constructors

**Analog:** `mixinforproto/annotation.go` (`/home/user/entconnect/mixinforproto/annotation.go`)

**Why this is the analog:** it is this repo's only precedent for "a plain, JSON-serializable `schema.Annotation` struct that must survive entc's schema-load subprocess boundary" — exactly D-02/D-03's constraint on the RPC-binding annotations.

**Struct + `Name()` pattern to copy** (lines 40-61, 75-107):
```go
type FieldRef struct {
	Name   string `json:"name"`
	Number int32  `json:"number"`
}

type SourceMessage struct {
	ContractVersion int        `json:"contractVersion"`
	Message         string     `json:"message"`
	Fields          []FieldRef `json:"fields"`
	Excluded        []string   `json:"excluded"`
	Overridden      []string   `json:"overridden"`
}

// Name implements entgo.io/ent/schema.Annotation.
func (SourceMessage) Name() string {
	return MixinForProtoMessage
}
```
Apply the same shape to the new RPC-binding annotation: a plain struct with a `ContractVersion int` field (per D-02's explicit instruction to follow this precedent), string/slice fields only (no closures, no interfaces), and a `Name() string` method satisfying `schema.Annotation`. `entc/binder.go`'s `CreateRPC(proc string)` etc. should each return a value of one shared annotation struct (e.g. `RPCBinding{Op: "create", Procedure: proc, ContractVersion: 1}`) — mirroring how `SourceField`/`SourceMessage` are both flat structs keyed by a shared `ContractVersion` constant.

**Constant-keys pattern to copy** (lines 20-30):
```go
const (
	MixinForProtoMessage = "MixinForProtoMessage"
	MixinForProtoField   = "MixinForProtoField"
)
```
Use the same "exported string constant per annotation key" idiom for the new `entconnect.RPCBinding` annotation key(s), rather than a magic string re-typed at each call site (entproto's own `MessageAnnotation` precedent, cited directly in annotation.go's own comment).

### `entc/decode.go` — annotation JSON round-trip decode

**Analog:** `mixinforproto/internal/boundarytest/boundary_test.go` (`/home/user/entconnect/mixinforproto/internal/boundarytest/boundary_test.go`, lines 97-124)

**Pattern to copy verbatim (already proven against the real subprocess boundary in this repo):**
```go
func decodeAnnotation[T any](t *testing.T, annotations gen.Annotations, key string) T {
	t.Helper()
	var zero T
	raw, ok := annotations[key]
	if !ok {
		keys := make([]string, 0, len(annotations))
		for k := range annotations {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		t.Fatalf("want a %q annotation, got keys: %v", key, keys)
		return zero
	}
	b, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("re-marshal %q annotation: %v", key, err)
	}
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("unmarshal %q annotation into %T: %v", key, v, err)
	}
	return v
}
```
RESEARCH.md's own Pattern 6 already adapts this into a non-test, production `decodeAnnotation[T any](annotations gen.Annotations, key string) (T, error)` (no `*testing.T`, returns an error instead of calling `t.Fatalf`) — use that adapted (error-returning) form in `entc/decode.go`, and keep the test-only `t.Fatalf` form only inside test files that call it directly, matching the split already visible between `mixinforproto/internal/boundarytest` (test-only) and production `derive.go` (error-returning) in this repo.

**Recommendation carried over from RESEARCH.md:** do NOT add `mapstructure` (entproto's own idiom) — this JSON round-trip idiom is already tested against the real entc subprocess boundary in this exact repo and needs no new module edge.

### `entc/errormap.go` — D-18 error mapping table

**Analog (discipline, not literal code):** `mixinforproto/errors.go` (`/home/user/entconnect/mixinforproto/errors.go`)

This is a role-match, not a content-match — `errors.go` aggregates *codegen-time derivation failures* (`failure`/`derivationError`), while `errormap.go` maps *runtime* ent/privacy errors to Connect codes. What to copy is the **discipline**, per Phase 1 D-06/D-08/D-09 (explicitly named in the orienting note as load-bearing):

- **Self-sufficient first line** (lines 49-58, `failure.line()`): every error's rendered first line names both "what" and "the fix" without requiring the reader to see subsequent lines. Apply the analogous idea in `errormap.go`'s default branch: log the *detail* (D-18: "logged, not returned") but return a wire-safe, self-sufficient `connect.CodeInternal` message.
- **Collected, not first-offense-wins** (`newDerivationError`, lines 75-91): not directly applicable to per-request error mapping (only one error exists per request), but IS the required discipline for **D-05**'s duplicate-RPC-claim detection and **D-07**'s claims report in `entc/resolve.go` / `entc/binder.go` — collect every offending claim in one codegen pass, sort deterministically, then report, exactly as `newDerivationError` does with `sort.SliceStable` (lines 81-89) keyed on multiple tiebreak fields, never on map iteration order.
- **The literal mapping table itself** is not in this repo — copy RESEARCH.md's already-verified `mapError` function directly (RESEARCH.md "Code Examples > Error mapping table", using `errors.Is(err, privacy.Deny)`, `ent.IsNotFound`, `ent.IsConstraintError`, `ent.IsValidationError`, default → `CodeInternal` with `log.Printf` of the real error and a generic wire message).

### `entc/extension_test.go` — golden-file tests (CRUD-06/D-20)

**Analog:** `mixinforproto/fieldmap_test.go` (`/home/user/entconnect/mixinforproto/fieldmap_test.go`, lines 1-27)

**Pattern to copy:**
```go
import (
	goldie "github.com/sebdah/goldie/v2"
)

func assertGolden(t *testing.T, name string, /* generate func */) {
	t.Helper()
	// ... run the generator, get output bytes/struct ...
	goldie.New(t).AssertJson(t, name, output) // or .Assert(t, name, rawBytes) for generated Go source text
}
```
For generated **Go source files** (not JSON projections like `fieldproj.ProjectAll`), use goldie's `.Assert(t, name, []byte)` (raw-bytes variant) rather than `.AssertJson`, since the emitted artifact here is formatted Go text, not a JSON-serializable struct. Golden fixtures live under a `testdata/` dir sibling to the test file, exactly as `mixinforproto/testdata/*.golden` does (confirmed convention: 20+ `.golden` files, one per corpus/rule-class fixture — extend with one golden file per CRUD-verb × corpus-message combination). CI's `-count=5` discipline (D-20) is a `Makefile`/`ci.yml` concern, not per-test code — verify (do not necessarily duplicate) that `make test`/`ci.yml`'s `modules` job already runs `go test ./...` for the root module the same way it does for `mixinforproto`, which by inspection it does (`Makefile`'s `test` target loops over `$(MODULES)` uniformly — no special-casing needed for root vs. mixinforproto).

### `internal/entconnecttest/boundarytest/` (or similar name) — real-entc-load integration test for generated handlers

**Analog:** `mixinforproto/internal/boundarytest/` (`/home/user/entconnect/mixinforproto/internal/boundarytest/`, directory containing `boundary_test.go` + a real `ent/schema` fixture package)

**Pattern to copy (structure, not full content):**
```go
graph, err := entc.LoadGraph("./ent/schema", &gen.Config{
	Target:  t.TempDir(),
	Package: "github.com/smintz/entconnect/.../ent",
})
if err != nil {
	t.Fatalf("entc.LoadGraph: %v", err)
}
```
This is the nearest existing analog to "prove the entc extension's hook actually fires against a real entc subprocess and emits the expected file(s)" — the orienting note calls this out explicitly. Build a sibling fixture: a small `ent/schema` package declaring `MixinForProto[...]` plus the new `entconnect.CreateRPC(...)` etc. annotations, pointed at a small synthetic proto corpus with an actual service (see below), and assert the hook's emitted output (not just that annotations decode — this repo's existing harness only proves the read side; Phase 2 additionally needs the write/emit side proven, which has no existing analog and must be written new, following the same `entc.LoadGraph` + `t.TempDir()` skeleton).

### `proto/<newcorpus>/v1/*.proto` — synthetic service-bearing proto corpus

**Analog:** `proto/mixinforprototest/v1/*.proto` (`/home/user/entconnect/proto/mixinforprototest/v1/`, e.g. `messages.proto`, `scalars.proto`, `tracer.proto` — 10 files, one per rule/feature class)

**Convention to extend, not replace** (per orienting note: "Phase 2 should extend this pattern with a service-bearing corpus rather than invent a second fixture convention"):
- One proto file (or small file set) per new RPC-service-shaped concern (e.g. a `crud.proto` declaring a minimal `OrderService` with Get/List/Create/Update/Delete, mirroring the exact shape of D-01's own example `orderv1connect.OrderServiceCreateOrderProcedure`).
- Stubs generated via `scripts/generate-stubs.sh`, **not** a second hand-invoked `buf generate` — that script is "the ONLY place in the repository that spells `buf generate`" (per its own header comment) and both `pipeline.sh` and `check-stubs.sh` depend on staying in sync with it.
- `scripts/generate-stubs.sh`'s `--path mixinforprototest` scoping flag (line 40 of that script) will need a new `--path <newcorpus>` entry (or a broadened `--path` list) — read that script's own header comment on *why* the scoping exists (avoiding a duplicate/colliding generation of `buf/validate` types) before editing it.
- `proto/buf.gen.yaml` needs a new `protoc-gen-connect-go` plugin entry (RESEARCH.md's own Installation section names the exact `go install` line and the version to pin: `connectrpc.com/connect/cmd/protoc-gen-connect-go@v1.20.0`).

### Root `go.mod` — first real dependency edge (D-21)

**Analog (convention only, not literal versions):** `mixinforproto/go.mod` (`/home/user/entconnect/mixinforproto/go.mod`)

The root module has **no module-isolation constraint** (unlike `mixinforproto`, whose CLAUDE.md-pinned constraint is "depends only on `ent`, `google.golang.org/protobuf`, and `protovalidate-go`/`cel-go`") — so do not mirror `mixinforproto/go.mod`'s minimalism, only its **tagging discipline**: D-21/D-19 require the `mixinforproto` dependency bump to land in a separate commit referencing an already-pushed `mixinforproto/vX.Y.Z` tag, never a same-commit self-reference, and `make check-modules` (already wired into CI) already asserts no committed `go.mod` contains a local-path `replace` directive — verify the new root `go.mod` entries pass that check unmodified, no new CI step required.

## Shared Patterns

### Error/failure aggregation discipline (D-06/D-08/D-09, applies to `entc/resolve.go`, `entc/binder.go`, `entc/errormap.go`)
**Source:** `mixinforproto/errors.go` (`/home/user/entconnect/mixinforproto/errors.go`), full file
**Apply to:** every codegen-time failure path in the new `entc/` package — D-05 (duplicate RPC claims), D-04 (unresolved procedure), D-15/D-17 (bad FieldMask path at build time)
```go
// Self-sufficient-first-line discipline (line() method, lines 49-58):
func (f failure) line() string {
	loc := f.message
	if f.field != "" {
		loc = f.message + "." + f.field
	}
	return fmt.Sprintf("mixinforproto: %s: %s — %s", loc, f.description, f.remedy)
}
// Collected-not-first-offense error type (newDerivationError, lines 75-91):
// sorts deterministically, never trusts caller/map order, returns nil for
// an empty failure slice so callers can uniformly `if err != nil`.
```
D-04's own wording ("self-sufficient first-line error naming the procedure, the descriptor-set path, and the remedy") is a direct restatement of this exact file's discipline — treat `errors.go` as closer to a spec than an analog for this specific concern.

### Determinism-by-construction (D-20/D-24: never range a map into ordered output)
**Source:** `mixinforproto/option.go` (`/home/user/entconnect/mixinforproto/option.go`, lines 118-152, `excludedNames`/`overriddenNames`/`sortedKeys`)
**Apply to:** D-07's sorted claims report, `allProcedures` (already given fully-formed in RESEARCH.md Pattern 1, itself already following this discipline with `sort.Strings(out)`), and any other map-keyed codegen output (e.g. per-field `switch` cases in the Update handler template — D-17's mask-to-`SourceField` cross-check must iterate fields in a stable order).
```go
func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
```

### Annotation JSON round-trip decode (see `entc/decode.go` above)
**Source:** `mixinforproto/internal/boundarytest/boundary_test.go` lines 97-124
**Apply to:** every place `entc/` reads `gen.Type.Annotations`/`gen.Field.Annotations` — the RPC-binding annotations (D-01/D-02) AND re-reading Phase 1's own `mixinforproto.SourceMessage`/`SourceField` (D-17's mask cross-check).

### Package doc-comment style (`doc.go`)
**Source:** `mixinforproto/doc.go` (`/home/user/entconnect/mixinforproto/doc.go`)
**Apply to:** `entc/doc.go`, `runtime/doc.go` if the plan creates one — a package-level doc comment that states the one-sentence purpose up front, then calls out load-bearing "read this before you..." caveats with direct links, matching this repo's established documentation voice (e.g. the FieldMask/D-16 zero-collapse footgun deserves the same treatment `doc.go` gives the proto3 presence footgun).

### Script/CI wiring discipline (single canonical invocation point)
**Source:** `scripts/generate-stubs.sh` header comment (`/home/user/entconnect/scripts/generate-stubs.sh`, lines 1-19) and `.github/workflows/ci.yml`'s `stubs`/`modules` jobs
**Apply to:** any new codegen invocation this phase adds (e.g. if a distinct "run the entc extension" step is needed beyond `go generate ./...`, which `pipeline.sh` step 4 already covers as a no-op placeholder this phase gives real work to). Do not add a second call site for `buf generate` or a parallel ad hoc CI job — extend the existing single canonical scripts, matching this repo's own stated anti-pattern lesson (VERIFICATION.md gap 4 / REVIEW.md CR-04, cited directly in the script's own comments).

## No Analog Found

These are genuinely greenfield — the orienting note asked to call these out explicitly rather than force a weak match. For all of these, RESEARCH.md's own "Code Examples"/"Architecture Patterns" sections (already read, verified against live source this session) are the primary source to build from, since no in-repo precedent exists:

| File | Role | Data Flow | Reason | Where to look instead |
|---|---|---|---|---|
| `entc/extension.go` (`Extension{entc.DefaultExtension}`, `NewExtension`, `Hooks()`) | config/extension | build-time hook | No entc extension exists anywhere in this repo yet; `mixinforproto` is a runtime mixin, architecturally different (no `Hooks()`/`gen.Graph` involvement at all) | `entgo.io/contrib/entproto@v0.7.0/extension.go` (external, read-only precedent per D-19/CLAUDE.md) — RESEARCH.md Pattern 5 already gives a Go skeleton adapted from it |
| `entc/resolve.go` (procedure string → `protoreflect.MethodDescriptor`) | utility | lookup/transform | No prior code in this repo touches `protodesc`/`protoregistry`; this is the phase's "hardest technical problem" per CONTEXT.md's own Specific Ideas section | RESEARCH.md Pattern 1 — fully verified, executable Go given directly (`splitProcedure`, `resolveMethod`, `loadDescriptorSet`, `allProcedures`) |
| `entc/crud/*.go.tmpl` (Get/Create/Update/Delete/List template bodies) | controller (template) | CRUD / streaming-adjacent (List keyset) | No `text/template`-based Go-source emission exists in this repo; `mixinforproto` emits `ent.Field` values directly, never text templates | RESEARCH.md Patterns 2 (List keyset), 3 (Update FieldMask), Code Examples; `entc/gen`'s own `where.tmpl`/`query.tmpl` (external, ent core, already read by the researcher) as the template-mechanics reference |
| `runtime/viewer/viewer.go` (`NewContext`/`FromContext`) | provider | request-response (ctx) | ent core ships no canonical viewer type; this repo has never defined one | `entgo.io/ent/examples/privacyadmin/viewer` — explicitly NOT importable cross-module (Pitfall 6), read as a shape reference only, then write first-party |
| `runtime/interceptors.go` (authn → viewer-injection interceptor) | middleware | request-response | No Connect interceptor exists in this repo | RESEARCH.md Pattern 4, `connectrpc.com/connect@v1.20.0/option.go` doc comment (already quoted verbatim in RESEARCH.md) |
| `runtime/server.go` (`NewServer(client, authenticator, opts...)`) | provider (server wiring) | request-response | CRUD-07/D-12's "only place `*ent.Client` is constructed" has no prior instance in this repo | RESEARCH.md Recommended Project Structure + Pattern 4; entproto's own hook-based emission-into-sibling-package precedent (Pattern 5) for how the *generated* half of this wiring gets written to disk |
| `entc/crud/list.go.tmpl`'s cursor encode/decode (D-08 fingerprinting) | utility | transform | Genuinely new — no cursor/pagination code anywhere in this repo | RESEARCH.md Pattern 2, Assumption A1 (signing left out-of-scope for v1) |

## Metadata

**Analog search scope:** `mixinforproto/` (entire module — annotation.go, errors.go, mixin.go, option.go, fieldmap_test.go, internal/boundarytest/), `proto/` (corpus + buf config), `scripts/`, `Makefile`, `.github/workflows/ci.yml`. Root module (`entc/`, `runtime/`) confirmed empty (no `.go` files) prior to this phase.
**Files scanned:** ~20 (all `.go` files in `mixinforproto/` top level + `internal/boundarytest/`, all files in `proto/`, `scripts/*.sh`, `Makefile`, `.github/workflows/ci.yml`, root `go.mod`/`mixinforproto/go.mod`)
**Pattern extraction date:** 2026-08-08
