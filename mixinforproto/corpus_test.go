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
//   - TestCorpusExercisesEveryProtovalidateConstraintClass (Plan 03-05
//     Task 2, PIPE-05): every protovalidate rule category populated
//     anywhere in the corpus produces recorded provenance (TranslatedIDs/
//     ResidualIDs/LengthUnitDivergentIDs/BoundaryOnly) SOMEWHERE — the
//     same "the corpus happens to avoid this shape" root cause, applied
//     to protovalidate's OWN constraint vocabulary rather than
//     mixinforproto's derivation-shape taxonomy.
//
// A guard that has never been observed failing is indistinguishable from
// a guard that cannot fail — that is precisely the trap the original
// corpus fell into. Each guard's SUMMARY entry (01-09-SUMMARY.md,
// 03-05-SUMMARY.md) records it being deliberately broken and caught, not
// merely asserted to work.
package mixinforproto

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"buf.build/go/protovalidate"
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

// corpusCoverage is the two-way-checked map from every proto message
// declared in the mixinforprototest.v1 package to a short, human-written
// claim naming where that message's behavior is verified. This is the
// axis gap 2 (01-VERIFICATION.md / 01-REVIEW.md CR-02) lived on:
// constraints.proto's StringFormat* messages were each single-field by
// convention, and that unwritten corpus property was exactly what let
// delegatingFormatValidator's whole-message-poisons-the-verdict defect go
// untested. A written, checked coverage claim per message is what turns
// "the corpus happens to avoid this shape" into a decision someone made
// on purpose, or a gap TestCorpusMessagesHaveRecordedCoverage below
// reports.
//
// Two value forms:
//   - "golden:<name>" — asserted via goldie against
//     mixinforproto/testdata/<name>.golden. TestCorpusMessagesHaveRecordedCoverage
//     verifies the referenced file actually exists on disk, so a renamed
//     or deleted golden fixture is caught rather than silently trusted.
//   - "test:<TestName>" — asserted by a named Go test with no golden
//     fixture. Used for messages whose full derivation deliberately
//     fails (the Repeated* fixtures, reserved.proto's collision
//     fixtures) and for messages that are referenced-only types never
//     derived on their own (Nested, Inner — used only as a map-value/
//     message-field type elsewhere), whose behavior is exercised
//     indirectly through the container message's own test.
//
// A guard failure here is closed by ADDING coverage — a golden fixture
// or a named test, then an entry recording it — never by deleting this
// assertion or weakening the check (T-01G-18). "No coverage" is a real
// answer only if it is written down here.
var corpusCoverage = map[string]string{
	"mixinforprototest.v1.Scalars":  "golden:scalars",
	"mixinforprototest.v1.Enums":    "golden:enums",
	"mixinforprototest.v1.Presence": "golden:presence",
	"mixinforprototest.v1.Wkt":      "golden:wkt",
	// Nested/Inner are reference-only types (a map value type, a message
	// field type) never derived directly on their own; their being
	// skipped-by-default / message-map-skipped is exercised through
	// TestGolden's Maps/Messages subtests on their containing message.
	"mixinforprototest.v1.Nested":                    "test:TestGolden",
	"mixinforprototest.v1.Maps":                      "golden:maps",
	"mixinforprototest.v1.Inner":                     "test:TestGolden",
	"mixinforprototest.v1.Messages":                  "golden:messages",
	"mixinforprototest.v1.Oneofs":                    "golden:oneofs",
	"mixinforprototest.v1.Tracer":                    "test:TestDerive_TracerMapsSingleStringField",
	"mixinforprototest.v1.Empty":                     "golden:empty",
	"mixinforprototest.v1.MultiField":                "test:TestDerive_FieldOrderMatchesDeclarationOrder",
	"mixinforprototest.v1.Unsupported":               "test:TestDerive_FormerlyUnsupportedKindNowMaps",
	"mixinforprototest.v1.ReservedStatic":            "test:TestReservedCollisionFails",
	"mixinforprototest.v1.ReservedStructural":        "test:TestReservedCollisionFails",
	"mixinforprototest.v1.NotReserved":               "test:TestNotReservedDerivesCleanly",
	"mixinforprototest.v1.PartialOneof":              "test:TestOneofUnresolvedFails",
	"mixinforprototest.v1.RepeatedScalar":            "test:TestRepeatedCardinality",
	"mixinforprototest.v1.RepeatedEnum":              "test:TestRepeatedCardinality",
	"mixinforprototest.v1.RepeatedItemsFormat":       "test:TestRepeatedCardinality",
	"mixinforprototest.v1.RepeatedMessage":           "test:TestRepeatedCardinality",
	"mixinforprototest.v1.NoRules":                   "golden:constraints_no_rules",
	"mixinforprototest.v1.StringByteBounds":          "golden:constraints_string_byte_bounds",
	"mixinforprototest.v1.StringCodePointBounds":     "golden:constraints_string_codepoint_bounds",
	"mixinforprototest.v1.StringPattern":             "golden:constraints_string_pattern",
	"mixinforprototest.v1.StringFormatEmail":         "golden:constraints_string_format_email",
	"mixinforprototest.v1.StringFormatHostname":      "golden:constraints_string_format_hostname",
	"mixinforprototest.v1.StringFormatUri":           "golden:constraints_string_format_uri",
	"mixinforprototest.v1.StringFormatIp":            "golden:constraints_string_format_ip",
	"mixinforprototest.v1.StringFormatUuid":          "golden:constraints_string_format_uuid",
	"mixinforprototest.v1.StringFormatWithSibling":   "golden:constraints_string_format_with_sibling",
	"mixinforprototest.v1.RequiredString":            "golden:constraints_required_string",
	"mixinforprototest.v1.RequiredOptionalNonString": "golden:constraints_required_optional_non_string",
	"mixinforprototest.v1.RequiredPlainNonString":    "golden:constraints_required_plain_non_string",
	"mixinforprototest.v1.RequiredOptionalString":    "golden:constraints_required_optional_string",
	"mixinforprototest.v1.RequiredOptionalBytes":     "golden:constraints_required_optional_bytes",
	"mixinforprototest.v1.ResidualCel":               "golden:constraints_residual_cel",
	"mixinforprototest.v1.EnumDefinedOnlyField":      "golden:constraints_enum_defined_only",
	"mixinforprototest.v1.Int32Comparators":          "golden:constraints_int32_comparators",
	"mixinforprototest.v1.Int32Adjacent":             "golden:constraints_int32_adjacent",
	"mixinforprototest.v1.Int32Overflow":             "golden:constraints_int32_overflow",
	"mixinforprototest.v1.FloatComparators":          "golden:constraints_float_comparators",
	"mixinforprototest.v1.DoubleComparators":         "golden:constraints_double_comparators",
	"mixinforprototest.v1.MixedFieldRules":           "golden:constraints_mixed_field_rules",
	"mixinforprototest.v1.ReverseScalars":            "golden:reverse_scalars",
	"mixinforprototest.v1.ReverseEnum":               "golden:reverse_enum",
	"mixinforprototest.v1.ReverseWkt":                "golden:reverse_wkt",
	"mixinforprototest.v1.ReverseScalarMap":          "golden:reverse_scalar_map",
	"mixinforprototest.v1.ReverseAsJSON":             "golden:reverse_as_json",
	// ReversePayload is a reference-only type (ReverseAsJSON's AsJSON-
	// opted-in field value), never derived on its own — its shape is
	// exercised indirectly through ReverseAsJSON's own golden.
	"mixinforprototest.v1.ReversePayload": "test:TestReverseCorpusGolden",

	// Plan 03-05's WithMessageRules(OnCreate)/D-10 corpus
	// (messagerules.proto). MessageRuleOk and MessageRuleExcludedRef both
	// derive cleanly with no options (their own "excluded" naming only
	// applies once a specific test opts a field out via Exclude), so they
	// get ordinary golden fixtures like every other plain corpus message.
	// The rest deliberately fail full derivation under specific options,
	// or exist purely to prove a schema-load property with no field-level
	// shape of its own interest — named tests, per this map's own
	// documented convention above.
	"mixinforprototest.v1.MessageRuleOk":              "golden:messagerules_ok",
	"mixinforprototest.v1.MessageRuleExcludedRef":     "golden:messagerules_excluded_ref",
	"mixinforprototest.v1.MessageRuleTwoExcludedRefs": "test:TestBuildHookState_MessageRuleTwoExcludedRefsFailsInOnePass",
	// MessageRuleDetail is a reference-only type (MessageRuleUnderivableRef's
	// message-typed "detail" field value, deliberately never AsJSON-opted-in) —
	// its role is exercised indirectly through MessageRuleUnderivableRef's own test.
	"mixinforprototest.v1.MessageRuleDetail":         "test:TestBuildHookState_MessageRuleUnderivableRefFailsSchemaLoad",
	"mixinforprototest.v1.MessageRuleUnderivableRef": "test:TestBuildHookState_MessageRuleUnderivableRefFailsSchemaLoad",
	"mixinforprototest.v1.MessageRuleNone":           "test:TestBuildHookState_MessageRuleNoneIsLegalNoOp",
	"mixinforprototest.v1.MessageRuleLookalike":      "test:TestBuildHookState_MessageRuleLookalikeDoesNotFalsePositive",
}

// TestCorpusMessagesHaveRecordedCoverage checks corpusCoverage in BOTH
// directions, so neither an unlisted message nor a stale entry survives:
//   - every message corpusMessages() finds must be a key in
//     corpusCoverage — an unlisted message means someone added a fixture
//     without recording where it's exercised.
//   - every corpusCoverage key must name a message still present in the
//     registry — a stale entry means a message was renamed or removed
//     and the map was never updated.
//   - every "golden:<name>" claim must resolve to an existing file under
//     mixinforproto/testdata — a renamed or deleted golden fixture is
//     caught here rather than silently trusted.
//
// Both diagnostic lists are sorted before printing (D-24) so failure
// output is stable and diffable across runs.
func TestCorpusMessagesHaveRecordedCoverage(t *testing.T) {
	msgs := corpusMessages(t)
	present := make(map[string]bool, len(msgs))
	for _, md := range msgs {
		present[string(md.FullName())] = true
	}

	t.Run("every corpus message has a recorded coverage claim", func(t *testing.T) {
		var missing []string
		for _, md := range msgs {
			name := string(md.FullName())
			if _, ok := corpusCoverage[name]; !ok {
				missing = append(missing, fmt.Sprintf("%s (declared in %s)", name, md.ParentFile().Path()))
			}
		}
		if len(missing) > 0 {
			sort.Strings(missing)
			t.Fatalf("corpus message(s) with no recorded coverage claim in corpusCoverage — \"no coverage\" is a real answer only if it is written down. Add a golden fixture or a named test, then record the claim (golden:<name> or test:<TestName>):\n  %s", strings.Join(missing, "\n  "))
		}
	})

	t.Run("every corpusCoverage entry names a message that still exists", func(t *testing.T) {
		var stale []string
		for name := range corpusCoverage {
			if !present[name] {
				stale = append(stale, name)
			}
		}
		if len(stale) > 0 {
			sort.Strings(stale)
			t.Fatalf("corpusCoverage entry(ies) name a message that is no longer in the registry — renamed or removed; remove or update the stale entry:\n  %s", strings.Join(stale, "\n  "))
		}
	})

	t.Run("every golden: claim resolves to an existing testdata fixture", func(t *testing.T) {
		var missingGolden []string
		for name, claim := range corpusCoverage {
			goldenName, ok := strings.CutPrefix(claim, "golden:")
			if !ok {
				continue
			}
			path := filepath.Join("testdata", goldenName+".golden")
			if _, err := os.Stat(path); err != nil {
				missingGolden = append(missingGolden, fmt.Sprintf("%s -> %s (missing %s)", name, claim, path))
			}
		}
		if len(missingGolden) > 0 {
			sort.Strings(missingGolden)
			t.Fatalf("corpusCoverage golden claim(s) point at a nonexistent testdata fixture:\n  %s", strings.Join(missingGolden, "\n  "))
		}
	})

	// Plan 03-05 Task 2 (PIPE-05): two messages must never share the same
	// "golden:<name>" claim. A merged/reused golden is exactly how a
	// message's own behavior stops being independently checked while
	// this guard stays green — two messages exercising the same
	// constraint class must each carry their own entry (see this guard's
	// own package doc comment).
	t.Run("no two corpus messages share the same golden claim", func(t *testing.T) {
		byGolden := map[string][]string{}
		for name, claim := range corpusCoverage {
			goldenName, ok := strings.CutPrefix(claim, "golden:")
			if !ok {
				continue
			}
			byGolden[goldenName] = append(byGolden[goldenName], name)
		}
		var dupes []string
		for golden, names := range byGolden {
			if len(names) < 2 {
				continue
			}
			sort.Strings(names)
			dupes = append(dupes, fmt.Sprintf("golden:%s claimed by %s", golden, strings.Join(names, ", ")))
		}
		if len(dupes) > 0 {
			sort.Strings(dupes)
			t.Fatalf("duplicate golden claim(s) — each message must carry its OWN coverage entry, never a merged/shared one:\n  %s", strings.Join(dupes, "\n  "))
		}
	})
}

// --- Plan 03-05 Task 2: PIPE-05's constraint-class coverage guard -------
//
// TestCorpusExercisesEveryFieldClass (above) proves every fieldClass —
// mixinforproto's OWN derivation-shape taxonomy — is produced by some
// corpus fixture. TestCorpusMessagesHaveRecordedCoverage proves every
// MESSAGE has a written coverage claim. Neither proves every
// PROTOVALIDATE CONSTRAINT CATEGORY (buf.validate.field's own rule
// vocabulary — string/int32/required/cel/repeated/...) is actually
// exercised by some field's real, MACHINE-RECORDED provenance. That is
// PIPE-05's own root cause, restated: "the corpus does not contain the
// shape that triggers this branch" was a silent condition, not a
// detectable one, three separate times in Phase 1. This guard is the
// same two-part shape applied to that axis.
//
// Part A (ground truth): every protovalidate rule category — a
// validate.FieldRules "type" oneof member name, or the standalone
// "required"/"cel" categories — that is POPULATED on at least one field
// across the WHOLE mixinforprototest.v1 corpus, reflectively enumerated
// via fieldRuleClasses (the identical protoreflect.Message.Range
// mechanism fieldmap.go's boundaryOnlyRuleIDs already uses for
// provenance, applied here to EVERY field regardless of whether it
// ultimately derives — never a hand-typed list of proto option names).
// enum.defined_only is the one documented exception
// (constraintClassExceptions): it needs no residual/translated record
// because field.Enum's own construction already enforces it (D-14,
// mapEnum's doc comment) — excluded here for that reason, not by
// oversight. "part A: ground truth matches a hand-maintained sanity
// list" is a SEPARATE, deliberately small, human-reviewable cross-check
// on top of that reflective computation: if a future .proto edit
// introduces a new rule category, this sub-test fails and a reviewer
// must consciously decide whether the new category needs a witness too,
// rather than the guard silently absorbing it.
//
// Part B (witness): for every class in the ground truth, at least one
// corpus field's SourceField.TranslatedIDs/ResidualIDs/
// LengthUnitDivergentIDs, or some corpus message's
// SourceMessage.BoundaryOnly, must record an ID whose class prefix
// matches — derived from RECORDED PROVENANCE, never a hand-written
// message-to-class list, so the guard cannot drift from reality. A field
// is derived first with no options; if the WHOLE MESSAGE fails to derive
// that way (a field whose derivation kind mixinforproto rejects outright
// unless excluded — e.g. a repeated scalar, MIX-02/01-06-PLAN.md), the
// guard retries with ONLY that field excluded, so the rule's category
// still surfaces via BoundaryOnly (D-09) rather than the guard silently
// having no opinion about it. This mirrors, and does not duplicate,
// TestRepeatedCardinality's own fixture (fieldmap_test.go) — that test
// proves the FAILURE mode; this guard proves the rule's CATEGORY is
// still provenance-visible once excluded.
//
// A guard failure here is closed by ADDING coverage — a new fixture, or
// an Exclude(...) path that lets an already-declared rule surface via
// provenance — never by deleting this assertion, weakening the
// comparison, or adding a class to constraintClassExceptions without a
// design reason as real as enum's.

// constraintClassOf returns id's class prefix — the substring before its
// first '.', or id itself when there is none (a bare BoundaryOnly class
// name like "repeated", or "required"/"cel").
func constraintClassOf(id string) string {
	if i := strings.Index(id, "."); i >= 0 {
		return id[:i]
	}
	return id
}

// fieldRuleClasses returns the set of protovalidate rule categories fd's
// resolved FieldRules populates, via the identical generic
// protoreflect.Message.Range mechanism fieldmap.go's boundaryOnlyRuleIDs
// uses for provenance — applied here to every field regardless of
// whether it ultimately derives, so it can serve as this guard's Part A
// ground truth.
func fieldRuleClasses(fd protoreflect.FieldDescriptor) ([]string, error) {
	rules, err := protovalidate.ResolveFieldRules(fd)
	if err != nil {
		return nil, err
	}
	if rules == nil {
		return nil, nil
	}
	var classes []string
	if rules.HasRequired() && rules.GetRequired() {
		classes = append(classes, "required")
	}
	if len(rules.GetCel()) > 0 || len(rules.GetCelExpression()) > 0 {
		classes = append(classes, "cel")
	}
	rules.ProtoReflect().Range(func(rfd protoreflect.FieldDescriptor, _ protoreflect.Value) bool {
		if name := string(rfd.Name()); !boundaryOnlyNonConstraintFields[name] {
			classes = append(classes, name)
		}
		return true
	})
	return classes, nil
}

// constraintClassExceptions are protovalidate rule categories deliberately
// excluded from this guard's Part A ground truth, each with the design
// reason it is exempt — never a silent omission.
var constraintClassExceptions = map[string]string{
	"enum": "enum.defined_only is satisfied by field.Enum's own construction and needs no residual/translated record of its own — D-14, mapEnum's doc comment (fieldmap.go)",
}

// recordConstraintWitness folds d's derived provenance into witnessed,
// keyed by constraint class.
func recordConstraintWitness(witnessed map[string]bool, d *derivation) {
	for _, f := range d.fields {
		for _, a := range f.Descriptor().Annotations {
			sf, ok := a.(SourceField)
			if !ok {
				continue
			}
			for _, id := range sf.TranslatedIDs {
				witnessed[constraintClassOf(id)] = true
			}
			for _, id := range sf.ResidualIDs {
				witnessed[constraintClassOf(id)] = true
			}
			for _, id := range sf.LengthUnitDivergentIDs {
				witnessed[constraintClassOf(id)] = true
			}
		}
	}
	for _, b := range d.message.BoundaryOnly {
		for _, id := range b.RuleIDs {
			witnessed[constraintClassOf(id)] = true
		}
	}
}

// sortedSetKeys returns m's keys sorted (D-24: never range a map into
// diagnostic output).
func sortedSetKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func TestCorpusExercisesEveryProtovalidateConstraintClass(t *testing.T) {
	msgs := corpusMessages(t)

	// --- Part A: ground truth -------------------------------------------
	groundTruth := map[string]bool{}
	for _, md := range msgs {
		fds := md.Fields()
		for i := 0; i < fds.Len(); i++ {
			fd := fds.Get(i)
			classes, err := fieldRuleClasses(fd)
			if err != nil {
				t.Fatalf("fieldRuleClasses(%s.%s): %v", md.FullName(), fd.Name(), err)
			}
			for _, c := range classes {
				if _, excepted := constraintClassExceptions[c]; excepted {
					continue
				}
				groundTruth[c] = true
			}
		}
		// Message-level rules aren't a FieldRules category; scanned
		// separately via ResolveMessageRules so a message-level cel rule
		// also counts toward the "cel" ground truth — field-level and
		// message-level custom CEL share the same provenance vocabulary.
		mr, merr := protovalidate.ResolveMessageRules(md)
		if merr != nil {
			t.Fatalf("ResolveMessageRules(%s): %v", md.FullName(), merr)
		}
		if mr != nil && len(mr.GetCel()) > 0 {
			groundTruth["cel"] = true
		}
	}
	if len(groundTruth) == 0 {
		t.Fatal("groundTruth is empty — registry-linkage regression? (mirrors corpusMessages' own T-01G-17 guard)")
	}

	t.Run("part A: ground truth matches a hand-maintained sanity list", func(t *testing.T) {
		want := map[string]bool{
			"string": true, "int32": true, "float": true, "double": true,
			"required": true, "cel": true, "repeated": true,
		}
		if len(want) != len(groundTruth) {
			t.Fatalf("expected sanity list has %d classes, corpus ground truth has %d — a rule category was added to or removed from the corpus without updating this list: got %v, want %v", len(want), len(groundTruth), sortedSetKeys(groundTruth), sortedSetKeys(want))
		}
		for c := range want {
			if !groundTruth[c] {
				t.Fatalf("expected class %q is not in the reflectively computed ground truth — a .proto fixture using it may have been removed", c)
			}
		}
	})

	// --- Part B: witness -------------------------------------------------
	witnessed := map[string]bool{}
	for _, md := range msgs {
		d, err := deriveFromDescriptor(md)
		if err != nil {
			fds := md.Fields()
			for i := 0; i < fds.Len(); i++ {
				fd := fds.Get(i)
				classes, cerr := fieldRuleClasses(fd)
				if cerr != nil || len(classes) == 0 {
					continue
				}
				d2, derr := deriveFromDescriptor(md, Exclude(string(fd.Name())))
				if derr != nil {
					continue
				}
				recordConstraintWitness(witnessed, d2)
			}
			continue
		}
		recordConstraintWitness(witnessed, d)
	}

	var missing []string
	for c := range groundTruth {
		if !witnessed[c] {
			missing = append(missing, c)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		t.Fatalf("protovalidate constraint class(es) with zero corpus witness in TranslatedIDs/ResidualIDs/LengthUnitDivergentIDs/BoundaryOnly — a rule category populated somewhere in the corpus's .proto sources produces no recorded provenance anywhere: %s. Add a fixture (or an Exclude(...) path) that lets this category's rule surface via provenance; do NOT delete or weaken this assertion.", strings.Join(missing, ", "))
	}
}
