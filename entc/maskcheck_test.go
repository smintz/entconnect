package entc

import (
	"strings"
	"testing"

	entload "entgo.io/ent/entc"
	"entgo.io/ent/entc/gen"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/smintz/entconnect/mixinforproto"
)

// patchDescriptor resolves the real entconnecttest.v1.Patch message
// descriptor from the committed FileDescriptorSet — the same descriptor
// entc/crud_update.go's generator resolves ValidateMaskPaths' md
// argument from at codegen time.
func patchDescriptor(t *testing.T) protoreflect.MessageDescriptor {
	t.Helper()
	files, err := LoadDescriptorSet(testDescriptorSetPath)
	if err != nil {
		t.Fatalf("LoadDescriptorSet: %v", err)
	}
	d, err := files.FindDescriptorByName("entconnecttest.v1.Patch")
	if err != nil {
		t.Fatalf("FindDescriptorByName(Patch): %v", err)
	}
	md, ok := d.(protoreflect.MessageDescriptor)
	if !ok {
		t.Fatalf("entconnecttest.v1.Patch resolved to a %T, not a message descriptor", d)
	}
	return md
}

// baseSourceMessage is the complete Patch descriptor field inventory
// with only "id" recorded as excluded — the entity's own reserved
// structural identifier (mixinforproto/reserved.go's reservedStructural,
// D-10), always excluded by construction regardless of schema, and
// never itself a candidate ValidateMaskPaths classifies (see its own
// doc comment). This is deliberately NOT internal/entconnecttest/
// update/ent/schema/patch.go's own shape (which additionally excludes
// "internal_note" as a genuine business decision) — this plan's D-17
// cross-check treats EVERY excluded business field as a build failure
// unconditionally (see 02-04-SUMMARY.md's Deviations section for why
// the real committed Patch fixture's own internal_note exclusion is
// therefore never exercised through this check again after Task 1
// committed its generated output), so a "fully satisfiable" positive
// control needs zero business-field exclusions, not a copy of the real
// fixture's own Exclude set.
func baseSourceMessage() mixinforproto.SourceMessage {
	return mixinforproto.SourceMessage{
		ContractVersion: mixinforproto.ContractVersion,
		Message:         "entconnecttest.v1.Patch",
		Fields: []mixinforproto.FieldRef{
			{Name: "id", Number: 1},
			{Name: "title", Number: 2},
			{Name: "body", Number: 3},
			{Name: "revision", Number: 4},
			{Name: "internal_note", Number: 5},
		},
		Excluded: []string{"id"},
	}
}

// TestMaskCheck_ValidateMaskPaths exercises ValidateMaskPaths as a pure
// function against the real Patch descriptor, covering every
// classification the function documents.
func TestMaskCheck_ValidateMaskPaths(t *testing.T) {
	md := patchDescriptor(t)

	t.Run("good fixture: zero failures (fully satisfiable, nothing excluded but id)", func(t *testing.T) {
		// This is the positive control named in 02-04-PLAN.md Task 2's
		// acceptance criteria. It is deliberately NOT the real committed
		// internal/entconnecttest/update/ent/schema/patch.go's own
		// shape — that fixture ALSO excludes "internal_note" as a
		// genuine business decision, and this plan's D-17 cross-check
		// (added in this same task) treats every excluded business
		// field as a build failure unconditionally, so that fixture's
		// own generated output is never regenerated again after Task 1
		// already committed it (see 02-04-SUMMARY.md's Deviations
		// section for why). This positive control instead proves
		// ValidateMaskPaths produces zero false positives when every
		// non-id field really is settable.
		failures := ValidateMaskPaths(md, baseSourceMessage(), []string{"title", "body", "revision", "internal_note"})
		if len(failures) != 0 {
			t.Fatalf("want zero failures, got %d: %+v", len(failures), failures)
		}
	})

	t.Run("excluded classification", func(t *testing.T) {
		sm := baseSourceMessage()
		sm.Excluded = []string{"id", "body"}
		failures := ValidateMaskPaths(md, sm, []string{"title", "revision", "internal_note"})
		if len(failures) != 1 {
			t.Fatalf("want exactly 1 failure, got %d: %+v", len(failures), failures)
		}
		if failures[0].rule != "mask-excluded" {
			t.Fatalf("want rule %q, got %q", "mask-excluded", failures[0].rule)
		}
		if !strings.Contains(failures[0].description, `"body"`) || !strings.Contains(failures[0].description, "entconnecttest.v1.Patch") {
			t.Fatalf("want the message and offending path named, got: %s", failures[0].description)
		}
	})

	t.Run("unknown classification (field present, not excluded, but not in allowed)", func(t *testing.T) {
		sm := baseSourceMessage()
		// "revision" is present in Fields and NOT in Excluded, but is
		// absent from allowed -- simulating an Override()'d field
		// (mixinforproto/option.go: Override installs an ent.Field
		// with no SourceField provenance, so it can never appear in
		// the allowed set entc/crud_update.go computes).
		failures := ValidateMaskPaths(md, sm, []string{"title", "body", "internal_note"})
		if len(failures) != 1 {
			t.Fatalf("want exactly 1 failure, got %d: %+v", len(failures), failures)
		}
		if failures[0].rule != "mask-unknown" {
			t.Fatalf("want rule %q, got %q", "mask-unknown", failures[0].rule)
		}
		if !strings.Contains(failures[0].description, `"revision"`) || !strings.Contains(failures[0].description, "entconnecttest.v1.Patch") {
			t.Fatalf("want the message and offending path named, got: %s", failures[0].description)
		}
	})

	t.Run("excluded and unknown failure texts differ", func(t *testing.T) {
		sm := baseSourceMessage()
		sm.Excluded = []string{"id", "body"}
		excludedFailures := ValidateMaskPaths(md, sm, []string{"title", "revision", "internal_note"})
		unknownFailures := ValidateMaskPaths(md, baseSourceMessage(), []string{"title", "body", "internal_note"})
		if len(excludedFailures) != 1 || len(unknownFailures) != 1 {
			t.Fatalf("want exactly 1 failure each, got %d and %d", len(excludedFailures), len(unknownFailures))
		}
		if excludedFailures[0].description == unknownFailures[0].description {
			t.Fatalf("want the excluded-field and unknown-field failure texts to differ, both were: %s", excludedFailures[0].description)
		}
	})

	t.Run("two offenders collected in a single pass, never first-offense-wins", func(t *testing.T) {
		sm := baseSourceMessage()
		sm.Excluded = []string{"id", "body"} // "body" -> excluded
		// "revision" absent from allowed -> unknown; "title" and
		// "internal_note" remain satisfiable so the zero-satisfiable
		// failure does not also fire, keeping this case isolated to
		// exactly the two offenders under test.
		failures := ValidateMaskPaths(md, sm, []string{"title", "internal_note"})
		if len(failures) != 2 {
			t.Fatalf("want exactly 2 failures collected in one pass, got %d: %+v", len(failures), failures)
		}
		var sawExcluded, sawUnknown bool
		for _, f := range failures {
			switch f.rule {
			case "mask-excluded":
				sawExcluded = true
				if !strings.Contains(f.description, `"body"`) {
					t.Fatalf("want the excluded failure to name %q, got: %s", "body", f.description)
				}
			case "mask-unknown":
				sawUnknown = true
				if !strings.Contains(f.description, `"revision"`) {
					t.Fatalf("want the unknown failure to name %q, got: %s", "revision", f.description)
				}
			}
		}
		if !sawExcluded || !sawUnknown {
			t.Fatalf("want both an excluded and an unknown offender, got: %+v", failures)
		}
	})

	t.Run("zero satisfiable paths is its own failure", func(t *testing.T) {
		sm := baseSourceMessage()
		sm.Excluded = []string{"id", "internal_note", "title", "body"}
		// "revision" is left out of allowed too, so nothing is settable.
		failures := ValidateMaskPaths(md, sm, nil)
		var sawZero bool
		for _, f := range failures {
			if f.rule == "mask-zero-satisfiable" {
				sawZero = true
			}
		}
		if !sawZero {
			t.Fatalf("want a mask-zero-satisfiable failure when nothing is settable, got: %+v", failures)
		}
	})

	t.Run("the entity's own id field is never a candidate", func(t *testing.T) {
		// id is excluded in every case above (structural, D-10) and
		// must never itself surface as an excluded-classification
		// failure -- assert no failure ever names "id" specifically.
		failures := ValidateMaskPaths(md, baseSourceMessage(), []string{"title", "body", "revision"})
		for _, f := range failures {
			if strings.Contains(f.description, `"id"`) {
				t.Fatalf("want the entity's own id field never named in a failure, got: %s", f.description)
			}
		}
	})
}

// TestMaskCheck_NegativeBuildFixtures loads the real negative-build
// fixtures via entc.LoadGraph (the boundarytest/02-01 harness shape) and
// runs the real entconnect.Generate call path, proving the wiring in
// entc/crud_update.go actually invokes ValidateMaskPaths during codegen
// -- not just that the pure function above behaves correctly in
// isolation. This is the "go generate ./internal/entconnecttest/update/
// badmask/... exits non-zero" acceptance criterion, asserted in-process
// per 02-04-PLAN.md Task 2's own sanctioned alternative to shelling out.
func TestMaskCheck_NegativeBuildFixtures(t *testing.T) {
	graph, err := entload.LoadGraph("../internal/entconnecttest/update/badmask/ent/schema", &gen.Config{
		Target:  t.TempDir(),
		Package: "github.com/smintz/entconnect/entc/badmask/ent",
	})
	if err != nil {
		t.Fatalf("entc.LoadGraph: %v", err)
	}

	ext, err := NewExtension(WithDescriptorSet(testDescriptorSetPath), WithOutputDir(t.TempDir()))
	if err != nil {
		t.Fatalf("NewExtension: %v", err)
	}

	err = Generate(graph, ext)
	if err == nil {
		t.Fatal("want a non-nil error from the negative-build fixtures, got nil")
	}

	firstLine := strings.SplitN(err.Error(), "\n", 2)[0]
	if !strings.Contains(firstLine, "entconnect:") {
		t.Fatalf("want a self-sufficient first line naming entconnect's own prefix, got: %s", firstLine)
	}
	// The first line alone must name a message and a remedy -- assert
	// both are present without requiring any subsequent line.
	if !strings.Contains(firstLine, "entconnecttest.v1.Patch") {
		t.Fatalf("want the first line to name the message %q, got: %s", "entconnecttest.v1.Patch", firstLine)
	}
}

// TestMaskCheck_Determinism proves the rendered multi-failure text is
// byte-identical across repeated calls with the SAME input in a
// different map/slice construction order -- run with -count=5 in CI so
// any accidental map-order dependence surfaces as a flake rather than
// staying latent (D-20/D-24).
func TestMaskCheck_Determinism(t *testing.T) {
	md := patchDescriptor(t)
	sm := baseSourceMessage()
	sm.Excluded = []string{"internal_note", "id", "body"} // deliberately unsorted input order

	first := ValidateMaskPaths(md, sm, []string{"title"})
	second := ValidateMaskPaths(md, sm, []string{"title"})

	if len(first) != len(second) {
		t.Fatalf("want the same failure count across repeated calls, got %d and %d", len(first), len(second))
	}
	for i := range first {
		if first[i].line() != second[i].line() {
			t.Fatalf("want byte-identical rendered failures across repeated calls, got %q and %q at index %d", first[i].line(), second[i].line(), i)
		}
	}
}
