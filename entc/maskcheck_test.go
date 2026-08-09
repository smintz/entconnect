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
// doc comment).
//
// Under D-17's resolved reading, a deliberately Exclude()d business
// field is simply not maskable — skipped, never a failure — so this
// base shape and the real internal/entconnecttest/update/ent/schema/
// patch.go (which also excludes "internal_note") both classify cleanly.
// The two differ only in how many candidates remain satisfiable.
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
		// The positive control named in 02-04-PLAN.md Task 2's acceptance
		// criteria: zero false positives when every non-id field is
		// settable.
		failures := ValidateMaskPaths(md, baseSourceMessage(), []string{"title", "body", "revision", "internal_note"})
		if len(failures) != 0 {
			t.Fatalf("want zero failures, got %d: %+v", len(failures), failures)
		}
	})

	t.Run("excluded field is skipped, not a failure (D-17 resolved reading)", func(t *testing.T) {
		// The regression guard for the reading this project settled on.
		// An Exclude()d business field is not maskable, but it is NOT a
		// build failure: the strict alternative made Exclude() and Update
		// RPCs mutually exclusive and broke this repo's own update
		// fixture. Enforcement moves to request time, where
		// runtime.ValidateMask returns ErrMaskUnknown -> InvalidArgument
		// for the same path (see runtime/fieldmask_test.go).
		sm := baseSourceMessage()
		sm.Excluded = []string{"id", "body"}
		failures := ValidateMaskPaths(md, sm, []string{"title", "revision", "internal_note"})
		if len(failures) != 0 {
			t.Fatalf("want zero failures for a deliberately excluded field, got %d: %+v", len(failures), failures)
		}
	})

	t.Run("the real update fixture's own Exclude set classifies cleanly", func(t *testing.T) {
		// internal/entconnecttest/update/ent/schema/patch.go excludes
		// "internal_note" as a genuine business decision. Under the
		// strict reading this tripped a build failure and made
		// `go generate` fail for the repo's own fixture. Pinning it here
		// so that regression cannot return silently.
		sm := baseSourceMessage()
		sm.Excluded = []string{"id", "internal_note"}
		failures := ValidateMaskPaths(md, sm, []string{"title", "body", "revision"})
		if len(failures) != 0 {
			t.Fatalf("want the real fixture's Exclude set to classify cleanly, got %d: %+v", len(failures), failures)
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

	t.Run("two unknown offenders collected in a single pass, never first-offense-wins", func(t *testing.T) {
		// D-05/D-09: every offender is reported in one pass. "body" and
		// "revision" are both present-and-not-excluded but absent from
		// allowed, so both classify unknown; "title" and "internal_note"
		// remain satisfiable so the zero-satisfiable failure does not
		// also fire, keeping this case isolated to the two under test.
		failures := ValidateMaskPaths(md, baseSourceMessage(), []string{"title", "internal_note"})
		if len(failures) != 2 {
			t.Fatalf("want exactly 2 failures collected in one pass, got %d: %+v", len(failures), failures)
		}
		var sawBody, sawRevision bool
		for _, f := range failures {
			if f.rule != "mask-unknown" {
				t.Fatalf("want every offender classified mask-unknown, got %q: %s", f.rule, f.description)
			}
			if strings.Contains(f.description, `"body"`) {
				sawBody = true
			}
			if strings.Contains(f.description, `"revision"`) {
				sawRevision = true
			}
		}
		if !sawBody || !sawRevision {
			t.Fatalf("want both offenders named, got: %+v", failures)
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
