package mixinforproto

import (
	"strings"
	"testing"

	"entgo.io/ent/schema/field"

	mixinforprototestv1 "github.com/smintz/entconnect/mixinforproto/internal/gen/mixinforprototestv1"
)

// TestOptionUnknownNamesAreCollected proves D-09: three unknown Exclude
// names against Scalars produce one error mentioning all three, not
// three separate regeneration cycles' worth of one-at-a-time failures.
func TestOptionUnknownNamesAreCollected(t *testing.T) {
	_, err := derive[*mixinforprototestv1.Scalars](Exclude("a", "b", "c"))
	if err == nil {
		t.Fatal("want an error for three unknown Exclude names")
	}
	for _, want := range []string{"a", "b", "c", "Exclude"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q missing %q", err.Error(), want)
		}
	}
}

// TestFailureFirstLineIsSelfSufficient proves D-08 for the multi-failure
// case: entc's schema-load subprocess routinely truncates panic output
// to a single line, so the first line alone must name the proto message
// full name, the offending field, the offending option, a remedy, and
// (since more than one failure was collected) the failure count.
func TestFailureFirstLineIsSelfSufficient(t *testing.T) {
	_, err := derive[*mixinforprototestv1.Scalars](Exclude("nope1", "nope2", "nope3"))
	if err == nil {
		t.Fatal("want an error for three unknown Exclude names")
	}
	msg := err.Error()

	firstLine, rest, hasRest := strings.Cut(msg, "\n")
	if firstLine == "" {
		t.Fatal("want a non-empty first line")
	}
	if strings.Contains(firstLine, "\n") {
		t.Fatalf("first line must not itself contain a newline: %q", firstLine)
	}
	if !hasRest || rest == "" {
		t.Fatalf("want additional detail beyond the first line for a 3-failure error, got only: %q", msg)
	}

	for _, want := range []string{
		"mixinforprototest.v1.Scalars", // proto message full name
		"nope1",                        // the first offender's field name (sorted: nope1 < nope2 < nope3)
		"Exclude",                      // the offending option
		"3",                            // the failure count
	} {
		if !strings.Contains(firstLine, want) {
			t.Fatalf("first line %q missing %q", firstLine, want)
		}
	}
	// A remedy is present: every Exclude-unknown-name remedy this
	// package emits mentions "typo" or "remove".
	if !strings.Contains(firstLine, "typo") && !strings.Contains(firstLine, "remove") {
		t.Fatalf("first line %q missing an apparent remedy", firstLine)
	}
}

// TestFailureOrderingIsDeterministic proves D-24: a message with at
// least three collected failures renders byte-identically across
// repeated derive[M] calls in-process (the same property `go test
// -count=5` checks at the process level).
func TestFailureOrderingIsDeterministic(t *testing.T) {
	var first string
	for i := 0; i < 25; i++ {
		_, err := derive[*mixinforprototestv1.Scalars](Exclude("zzz", "aaa", "mmm"))
		if err == nil {
			t.Fatal("want an error for three unknown Exclude names")
		}
		if first == "" {
			first = err.Error()
			continue
		}
		if err.Error() != first {
			t.Fatalf("run %d: message changed:\ngot:  %q\nwant: %q", i, err.Error(), first)
		}
	}
}

// TestExcludeEmptyArgs covers MIX-07's empty edge, both halves:
// Exclude() with zero arguments is a legal no-op, and Exclude("") is an
// unknown-name failure naming the empty string explicitly.
func TestExcludeEmptyArgs(t *testing.T) {
	t.Run("no arguments is a no-op", func(t *testing.T) {
		d, err := derive[*mixinforprototestv1.Scalars](Exclude())
		if err != nil {
			t.Fatalf("want nil error, got %v", err)
		}
		if len(d.fields) != 15 {
			t.Fatalf("want all 15 fields derived, got %d", len(d.fields))
		}
	})

	t.Run("empty string names itself explicitly", func(t *testing.T) {
		_, err := derive[*mixinforprototestv1.Scalars](Exclude(""))
		if err == nil {
			t.Fatal("want an error for Exclude(\"\")")
		}
		for _, want := range []string{"mixinforprototest.v1.Scalars", `""`, "Exclude"} {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("error %q missing %q", err.Error(), want)
			}
		}
	})
}

// TestExcludeIsByteExact proves MIX-07's encoding edge: Exclude matches
// protoreflect.Name byte-exactly. The proto declares string_field
// (snake_case); its JSON/camelCase spelling must be reported unknown,
// never accepted as a case-insensitive or JSON-name match.
func TestExcludeIsByteExact(t *testing.T) {
	_, err := derive[*mixinforprototestv1.Scalars](Exclude("stringField"))
	if err == nil {
		t.Fatal("want an error: stringField is not the declared proto field name (string_field)")
	}
	for _, want := range []string{"mixinforprototest.v1.Scalars", "stringField", "Exclude"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q missing %q", err.Error(), want)
		}
	}

	// The byte-exact spelling still derives cleanly, proving this is a
	// spelling failure and not a broader breakage.
	d, err := derive[*mixinforprototestv1.Scalars](Exclude("string_field"))
	if err != nil {
		t.Fatalf("want nil error excluding the real name, got %v", err)
	}
	if len(d.fields) != 14 {
		t.Fatalf("want 14 fields (string_field excluded), got %d", len(d.fields))
	}
}

// TestOverrideNil proves MIX-08's empty edge: Override(name, nil) fails
// at schema load naming the field and the nil replacement, rather than
// installing a nil ent.Field that would panic far away at codegen time.
func TestOverrideNil(t *testing.T) {
	_, err := derive[*mixinforprototestv1.Tracer](Override("name", nil))
	if err == nil {
		t.Fatal("want an error for Override(\"name\", nil)")
	}
	for _, want := range []string{"mixinforprototest.v1.Tracer", "name", "Override", "nil"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q missing %q", err.Error(), want)
		}
	}
}

// TestOverrideUnknownName proves MIX-08's core case: naming a field
// that does not exist fails at schema load rather than silently
// installing an orphaned field.
func TestOverrideUnknownName(t *testing.T) {
	_, err := derive[*mixinforprototestv1.Tracer](Override("does_not_exist", field.String("does_not_exist")))
	if err == nil {
		t.Fatal("want an error for Override naming an unknown field")
	}
	for _, want := range []string{"mixinforprototest.v1.Tracer", "does_not_exist", "Override"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q missing %q", err.Error(), want)
		}
	}
}

// TestExcludeOverrideConflict proves the same name passed to both
// Exclude and Override is a reported conflict, not a silent precedence
// of one option over the other.
func TestExcludeOverrideConflict(t *testing.T) {
	_, err := derive[*mixinforprototestv1.Scalars](
		Exclude("string_field"),
		Override("string_field", field.String("string_field")),
	)
	if err == nil {
		t.Fatal("want an error for a name passed to both Exclude and Override")
	}
	for _, want := range []string{"mixinforprototest.v1.Scalars", "string_field", "Exclude", "Override"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q missing %q", err.Error(), want)
		}
	}
}

// TestAnnotationRecordsExcludeOverride proves ANNO-03: a successful
// derivation's SourceMessage.Excluded and SourceMessage.Overridden carry
// exactly the names supplied to Exclude/Override, in sorted order —
// this is what lets a later drift check distinguish deliberate omission
// from accidental drift.
func TestAnnotationRecordsExcludeOverride(t *testing.T) {
	d, err := derive[*mixinforprototestv1.Scalars](
		Exclude("bool_field", "bytes_field"),
		Override("string_field", field.String("string_field")),
	)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	wantExcluded := []string{"bool_field", "bytes_field"}
	if len(d.message.Excluded) != len(wantExcluded) {
		t.Fatalf("want %d excluded names, got %d: %v", len(wantExcluded), len(d.message.Excluded), d.message.Excluded)
	}
	for i, want := range wantExcluded {
		if d.message.Excluded[i] != want {
			t.Fatalf("Excluded[%d]: want %q, got %q (want sorted order)", i, want, d.message.Excluded[i])
		}
	}
	wantOverridden := []string{"string_field"}
	if len(d.message.Overridden) != len(wantOverridden) || d.message.Overridden[0] != wantOverridden[0] {
		t.Fatalf("want Overridden %v, got %v", wantOverridden, d.message.Overridden)
	}
}

// TestOverrideSuppressesRelay proves D-05: an overridden field is
// installed verbatim, carrying no SourceField annotation of
// mixinforproto's own construction — Override suppresses validation
// relay for that field entirely, with no silent merging.
func TestOverrideSuppressesRelay(t *testing.T) {
	d, err := derive[*mixinforprototestv1.Scalars](
		Override("string_field", field.String("string_field").Comment("hand-declared, no relay")),
	)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	var found bool
	for _, f := range d.fields {
		desc := f.Descriptor()
		if desc.Name != "string_field" {
			continue
		}
		found = true
		for _, a := range desc.Annotations {
			if a.Name() == MixinForProtoField {
				t.Fatalf("want no SourceField annotation on an overridden field, got %v", a)
			}
		}
	}
	if !found {
		t.Fatal("want the overridden field present in derived fields")
	}
}
