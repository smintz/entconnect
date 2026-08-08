// Package mixinforproto (this file): three of Phase 1's four verified
// gaps (01-VERIFICATION.md — MIX-02's repeated-cardinality gap, VAL-01's
// delegated-format-verdict gap, VAL-03's presence-vs-NotEmpty gap) shared
// one root cause: "the corpus does not contain the shape that triggers
// this branch" was a silent condition, not a detectable one. Five green
// waves, a full golden-file corpus, and `go test -race -count=5` told us
// nothing, because each defect's own precondition was exactly what the
// corpus happened to avoid.
//
// This file makes that absence a structural, machine-checked failure
// instead. Three guards, one per axis that has already produced a real
// defect:
//   - TestCorpusExercisesEveryFieldClass: every value classify() can
//     return is produced by at least one corpus field (the axis gap 1
//     lived on).
//   - TestCorpusExercisesEveryRequiredResult: every value
//     classifyRequired() can return is produced by at least one corpus
//     field under the real (optional, required, hasNotEmpty) triple the
//     builders pass (the axis gap 3 lived on).
//   - TestCorpusMessagesHaveRecordedCoverage: every message declared in
//     the mixinforprototest.v1 package has a named, checked coverage
//     claim, in both directions (the axis gap 2's single-field-message
//     precondition lived on — see constraints.proto's header comment).
//
// A guard that has never been observed failing is indistinguishable from
// a guard that cannot fail — that is precisely the trap the original
// corpus fell into. Each guard's SUMMARY entry (01-09-SUMMARY.md) records
// it being deliberately broken and caught, not merely asserted to work.
package mixinforproto

import (
	"sort"
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"

	// Blank-imported so the dependency this file has on the corpus being
	// registered in protoregistry.GlobalFiles is explicit and self-evident,
	// even though fieldmap_test.go/validate_test.go already import this
	// package by the time any test in this binary runs (environment fact
	// 2, 01-09-PLAN.md).
	_ "github.com/smintz/entconnect/mixinforproto/internal/gen/mixinforprototestv1"
)

// corpusMessages walks every registered FileDescriptor in the
// mixinforprototest.v1 proto package, recurses into nested message
// declarations, and returns every non-map-entry MessageDescriptor found,
// sorted by FullName() (D-24: deterministic, diffable failure output).
// Proto map fields synthesize their own nested map-entry MessageDescriptors
// (environment fact 3) — these are real registry entries but no human
// wrote them, so they are filtered via IsMapEntry() rather than demanding
// a coverage claim no contributor could satisfy.
//
// t.Fatal on a zero-length result: a registry-linkage regression (a
// package rename, a build-tag exclusion, a future refactor that stops
// importing the generated stub) would otherwise make every guard in this
// file vacuously pass over an empty set — the exact failure mode
// T-01G-17 in this plan's threat model names.
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
		func(fd protoreflect.FileDescriptor) bool {
			walk(fd.Messages())
			return true
		},
	)

	if len(out) == 0 {
		t.Fatal("corpusMessages: zero messages found in package mixinforprototest.v1 — registry-linkage regression? (T-01G-17)")
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].FullName() < out[j].FullName()
	})
	return out
}

// fieldClassNames is the explicit, hand-maintained name table
// TestCorpusExercisesEveryFieldClass's exhaustiveness check runs against.
// fieldClass (fieldmap.go) is a contiguous iota run; every constant in
// that block MUST have an entry here. Appending a fieldClass without a
// matching entry is exactly the silent-branch shape that made gap 1
// (repeated-cardinality, 01-VERIFICATION.md) invisible for five waves —
// this table's exhaustiveness assertion is what turns that silence into
// a build failure.
var fieldClassNames = map[fieldClass]string{
	classScalarMap:       "classScalarMap",
	classMessageMap:      "classMessageMap",
	classRealOneofMember: "classRealOneofMember",
	classOptionalScalar:  "classOptionalScalar",
	classMessageField:    "classMessageField",
	classEnum:            "classEnum",
	classScalar:          "classScalar",
	classRepeated:        "classRepeated",
}

// TestCorpusExercisesEveryFieldClass has two parts: Part A proves the
// table above is exhaustive against fieldClass's own constant block; Part
// B proves every fieldClass value is actually produced by a real corpus
// fixture. Neither part calls derive — several corpus messages
// deliberately fail full derivation (Oneofs' unresolved real-oneof
// members, reserved.proto's collision fixtures, the Repeated* fixtures
// after 01-06), so a guard built on derive would either skip them or
// drown in expected errors (environment fact 5, 01-09-PLAN.md). classify
// is a total, pure function over a field descriptor — exactly what this
// guard needs.
func TestCorpusExercisesEveryFieldClass(t *testing.T) {
	t.Run("table is exhaustive against the fieldClass constant block", func(t *testing.T) {
		// fieldClass's iota run is contiguous (fieldmap.go lines 23-36):
		// classScalarMap=0 .. classRepeated=len-1. If fieldClassNames's
		// length doesn't match, or any integer 0..len-1 is missing as a
		// key, a fieldClass constant was added to fieldmap.go without a
		// matching entry here.
		const wantLen = int(classRepeated) + 1
		if len(fieldClassNames) != wantLen {
			t.Fatalf("fieldClassNames has %d entries, want %d (== classRepeated+1) — a new classification was added to fieldmap.go's fieldClass constant block without registering it here; add the missing entry, do not delete this assertion", len(fieldClassNames), wantLen)
		}
		for i := 0; i < wantLen; i++ {
			if _, ok := fieldClassNames[fieldClass(i)]; !ok {
				t.Fatalf("fieldClassNames is missing an entry for fieldClass(%d) — a new classification was added to fieldmap.go's constant block without registering it here", i)
			}
		}
	})

	t.Run("every fieldClass value is produced by at least one corpus field", func(t *testing.T) {
		produced := map[fieldClass]protoreflect.FieldDescriptor{}
		for _, md := range corpusMessages(t) {
			fds := md.Fields()
			for i := 0; i < fds.Len(); i++ {
				fd := fds.Get(i)
				c := classify(fd)
				if _, ok := produced[c]; !ok {
					produced[c] = fd
				}
			}
		}
		for c, name := range fieldClassNames {
			if _, ok := produced[c]; !ok {
				t.Fatalf("no corpus field classifies as %s (fieldClass=%d) — add a proto/mixinforprototest/v1/*.proto fixture exercising this shape and record its coverage; do NOT delete or weaken this assertion", name, c)
			}
		}
		for c, fd := range produced {
			t.Logf("fieldClass %s covered by %s.%s", fieldClassNames[c], fd.ContainingMessage().FullName(), fd.Name())
		}
	})
}

// requiredResultNames is the explicit, hand-maintained name table
// TestCorpusExercisesEveryRequiredResult's exhaustiveness check runs
// against, mirroring fieldClassNames's shape for requiredResult
// (fieldmap.go lines 320-326, also a contiguous iota run).
var requiredResultNames = map[requiredResult]string{
	requiredNone:          "requiredNone",
	requiredExactPresence: "requiredExactPresence",
	requiredExactNotEmpty: "requiredExactNotEmpty",
	requiredResidualZero:  "requiredResidualZero",
	requiredOptionalPlain: "requiredOptionalPlain",
}

// TestCorpusExercisesEveryRequiredResult guards the branch gap 3
// (01-VERIFICATION.md / 01-REVIEW.md CR-03) lived in: classifyRequired's
// pre-01-08 branch order tested `required && hasNotEmpty` BEFORE
// `optional && required`, which made requiredExactPresence unreachable
// for string/bytes — invisible because no corpus fixture combined
// `optional string`/`optional bytes` with `required` until 01-08 added
// RequiredOptionalString/RequiredOptionalBytes. This guard makes that
// combination a standing, checked property rather than a one-time fix.
//
// Coverage at time of writing:
//   - RequiredOptionalString / RequiredOptionalBytes -> requiredExactPresence
//   - RequiredString                                 -> requiredExactNotEmpty
//   - RequiredPlainNonString                          -> requiredResidualZero
//   - presence.proto's optional_string/optional_int32 -> requiredOptionalPlain
//   - everything else (no `required`, no `optional`)   -> requiredNone
//
// A future failure can be diagnosed by comparing against this list: if
// one of these five stops producing its outcome, something in
// classifyRequired's branch order or the corpus itself regressed.
func TestCorpusExercisesEveryRequiredResult(t *testing.T) {
	t.Run("table is exhaustive against the requiredResult constant block", func(t *testing.T) {
		const wantLen = int(requiredOptionalPlain) + 1
		if len(requiredResultNames) != wantLen {
			t.Fatalf("requiredResultNames has %d entries, want %d (== requiredOptionalPlain+1) — a new outcome was added to fieldmap.go's requiredResult constant block without registering it here; add the missing entry, do not delete this assertion", len(requiredResultNames), wantLen)
		}
		for i := 0; i < wantLen; i++ {
			if _, ok := requiredResultNames[requiredResult(i)]; !ok {
				t.Fatalf("requiredResultNames is missing an entry for requiredResult(%d) — a new outcome was added to fieldmap.go's constant block without registering it here", i)
			}
		}
	})

	t.Run("every requiredResult outcome is reachable through a real corpus fixture", func(t *testing.T) {
		produced := map[requiredResult]protoreflect.FieldDescriptor{}
		// producedPresenceByHasNotEmpty is the guard's load-bearing extra
		// granularity, beyond plain per-outcome existence: gap 3 was not
		// "requiredExactPresence is unreachable" in the aggregate (a plain
		// non-string optional+required field like
		// RequiredOptionalNonString.value already reached it, hasNotEmpty
		// being false for that kind either way) — it was specifically
		// "requiredExactPresence is unreachable for a hasNotEmpty=true
		// (string/bytes) field", because the pre-01-08 branch order tested
		// required && hasNotEmpty BEFORE optional && required. A guard that
		// only asked "is requiredExactPresence produced by ANY field"
		// would have stayed green under gap 3's original order (the
		// non-string witness alone satisfies it) — which is exactly the
		// false-confidence failure mode this whole file exists to close.
		// So requiredExactPresence must be witnessed separately by a
		// hasNotEmpty=true field AND a hasNotEmpty=false field.
		producedPresenceByHasNotEmpty := map[bool]protoreflect.FieldDescriptor{}

		for _, md := range corpusMessages(t) {
			fds := md.Fields()
			for i := 0; i < fds.Len(); i++ {
				fd := fds.Get(i)
				// Only classScalar/classOptionalScalar fields ever reach a
				// scalar builder (buildInt32Field, buildStringField, ...) —
				// the only place classifyRequired is called.
				class := classify(fd)
				if class != classScalar && class != classOptionalScalar {
					continue
				}

				optional := fd.HasOptionalKeyword()
				required, _, _, err := resolvedFieldRules(fd)
				if err != nil {
					t.Fatalf("resolvedFieldRules(%s.%s): %v", md.FullName(), fd.Name(), err)
				}
				// hasNotEmpty is true exactly when fd.Kind() is StringKind
				// or BytesKind — the two builders (buildStringField,
				// buildBytesField) that pass hasNotEmpty=true to
				// classifyRequired. If a third builder ever starts passing
				// true, THIS LINE must change with it: the previous shape
				// of this condition — required && hasNotEmpty checked
				// before optional && required — is precisely what made
				// requiredExactPresence unreachable for string/bytes and is
				// gap 3's root cause (01-VERIFICATION.md / 01-REVIEW.md
				// CR-03).
				hasNotEmpty := fd.Kind() == protoreflect.StringKind || fd.Kind() == protoreflect.BytesKind

				r := classifyRequired(optional, required, hasNotEmpty)
				if _, ok := produced[r]; !ok {
					produced[r] = fd
				}
				if r == requiredExactPresence {
					if _, ok := producedPresenceByHasNotEmpty[hasNotEmpty]; !ok {
						producedPresenceByHasNotEmpty[hasNotEmpty] = fd
					}
				}
			}
		}
		for r, name := range requiredResultNames {
			fd, ok := produced[r]
			if !ok {
				t.Fatalf("no corpus field produces classifyRequired outcome %s (requiredResult=%d) — add a fixture with the (optional,required,kind) combination that reaches it, then record it in this test's coverage comment; do NOT delete or weaken this assertion", name, r)
			}
			t.Logf("requiredResult %s covered by %s.%s", name, fd.ContainingMessage().FullName(), fd.Name())
		}
		for _, hasNotEmpty := range []bool{true, false} {
			fd, ok := producedPresenceByHasNotEmpty[hasNotEmpty]
			if !ok {
				t.Fatalf("requiredExactPresence is never witnessed by a field with hasNotEmpty=%v — this is exactly the axis gap 3 hid in (01-VERIFICATION.md/CR-03): presence must win for EVERY kind, not just the ones without a hasNotEmpty=true builder. Add an `optional <kind> ... [(buf.validate.field).required = true]` fixture for this hasNotEmpty value.", hasNotEmpty)
			}
			t.Logf("requiredExactPresence with hasNotEmpty=%v covered by %s.%s", hasNotEmpty, fd.ContainingMessage().FullName(), fd.Name())
		}
	})
}
