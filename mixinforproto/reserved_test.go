package mixinforproto

import (
	"strings"
	"testing"

	"entgo.io/ent/schema/field"

	mixinforprototestv1 "github.com/smintz/entconnect/mixinforproto/internal/gen/mixinforprototestv1"
)

// TestReservedStaticCatalogSize proves the static catalog is exactly
// the 41-unique-identifier union entc/gen/type.go's globalIdent (26) and
// privateField (16) collapse to once "config" (present in both) is
// deduplicated — catching a transcription slip rather than discovering
// it later as a silently-wrong collision check.
func TestReservedStaticCatalogSize(t *testing.T) {
	if got, want := len(reservedStatic), 41; got != want {
		t.Fatalf("want %d unique static reserved identifiers, got %d", want, got)
	}
	for _, want := range []string{
		"AggregateFunc", "Client", "config", "Mutation", "Policy",
		"Value", "ctx", "oldValue", "predicates", "typ",
	} {
		if !reservedStatic[want] {
			t.Fatalf("want reservedStatic to contain %q", want)
		}
	}
}

// TestReservedStructuralCatalog proves the structural supplement holds
// exactly {id, label} — the two categories with direct evidence
// (01-RESEARCH.md; ent/ent#280) — and nothing else, per this plan's R3
// reconciliation.
func TestReservedStructuralCatalog(t *testing.T) {
	if got, want := len(reservedStructural), 2; got != want {
		t.Fatalf("want %d structural reserved identifiers, got %d: %v", want, got, reservedStructural)
	}
	for _, want := range []string{"id", "label"} {
		if !reservedStructural[want] {
			t.Fatalf("want reservedStructural to contain %q", want)
		}
	}
	for _, notWant := range []string{"type", "edge", "where"} {
		if reservedStructural[notWant] {
			t.Fatalf("want reservedStructural to NOT contain %q (R3: unverified against ent's source)", notWant)
		}
	}
}

// TestReservedCollisionFails proves D-10: both the static and
// structural catalogs fail derivation, naming the colliding field and
// offering both Exclude and Override as remedies.
func TestReservedCollisionFails(t *testing.T) {
	t.Run("static", func(t *testing.T) {
		_, err := derive[*mixinforprototestv1.ReservedStatic]()
		if err == nil {
			t.Fatal("want an error deriving ReservedStatic")
		}
		for _, want := range []string{
			"mixinforprototest.v1.ReservedStatic", "config", "Exclude", "Override",
		} {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("error %q missing %q", err.Error(), want)
			}
		}
	})

	t.Run("structural", func(t *testing.T) {
		_, err := derive[*mixinforprototestv1.ReservedStructural]()
		if err == nil {
			t.Fatal("want an error deriving ReservedStructural")
		}
		for _, want := range []string{
			"mixinforprototest.v1.ReservedStructural", "id", "Exclude", "Override",
		} {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("error %q missing %q", err.Error(), want)
			}
		}
	})
}

// TestNotReservedDerivesCleanly proves the reserved-identifier check
// does not over-reject: ordinary field names derive normally.
func TestNotReservedDerivesCleanly(t *testing.T) {
	d, err := derive[*mixinforprototestv1.NotReserved]()
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	if len(d.fields) != 2 {
		t.Fatalf("want 2 derived fields, got %d", len(d.fields))
	}
}

// TestNoAutoRename proves D-10's core guarantee across the whole
// corpus: no successful derivation ever produces a derived field whose
// name differs from its proto field name — mixinforproto never guesses
// a different Go identifier to dodge a collision, it fails instead.
func TestNoAutoRename(t *testing.T) {
	cases := []struct {
		name  string
		field func() ([]string, error)
	}{
		{"Scalars", func() ([]string, error) {
			d, err := derive[*mixinforprototestv1.Scalars]()
			if err != nil {
				return nil, err
			}
			return fieldNames(d), nil
		}},
		{"NotReserved", func() ([]string, error) {
			d, err := derive[*mixinforprototestv1.NotReserved]()
			if err != nil {
				return nil, err
			}
			return fieldNames(d), nil
		}},
	}
	wantNames := map[string][]string{
		"Scalars": {
			"double_field", "float_field", "int32_field", "int64_field",
			"uint32_field", "uint64_field", "sint32_field", "sint64_field",
			"fixed32_field", "fixed64_field", "sfixed32_field", "sfixed64_field",
			"bool_field", "string_field", "bytes_field",
		},
		"NotReserved": {"display_name", "description"},
	}
	for _, c := range cases {
		got, err := c.field()
		if err != nil {
			t.Fatalf("%s: derive: %v", c.name, err)
		}
		want := wantNames[c.name]
		if len(got) != len(want) {
			t.Fatalf("%s: want %d fields, got %d: %v", c.name, len(want), len(got), got)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("%s: field %d: want name %q (matching proto), got %q", c.name, i, want[i], got[i])
			}
		}
	}
}

// TestOneofUnresolvedFails proves MIX-10: a real oneof with no
// resolution options fails at schema load, naming the oneof and listing
// its members in declaration order.
func TestOneofUnresolvedFails(t *testing.T) {
	_, err := derive[*mixinforprototestv1.PartialOneof]()
	if err == nil {
		t.Fatal("want an error deriving PartialOneof with no options")
	}
	msg := err.Error()
	for _, want := range []string{
		"mixinforprototest.v1.PartialOneof", "choice", "alpha", "bravo", "charlie", "Exclude", "Override",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error %q missing %q", msg, want)
		}
	}
	// Declaration order: alpha appears before bravo appears before charlie.
	ia, ib, ic := strings.Index(msg, "alpha"), strings.Index(msg, "bravo"), strings.Index(msg, "charlie")
	if !(ia < ib && ib < ic) {
		t.Fatalf("want members listed in declaration order (alpha, bravo, charlie), got positions %d,%d,%d in %q", ia, ib, ic, msg)
	}
}

// TestOneofFullyExcludedSucceeds proves MIX-10's empty edge: a oneof
// whose every member is excluded derives no field for them and produces
// no error — full resolution by exclusion is a documented way to accept
// a oneof.
func TestOneofFullyExcludedSucceeds(t *testing.T) {
	d, err := derive[*mixinforprototestv1.PartialOneof](Exclude("alpha", "bravo", "charlie"))
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	if len(d.fields) != 0 {
		t.Fatalf("want 0 derived fields, got %d: %v", len(d.fields), fieldNames(d))
	}
}

// TestOneofFullyOverriddenSucceeds proves the same for full resolution
// by Override.
func TestOneofFullyOverriddenSucceeds(t *testing.T) {
	d, err := derive[*mixinforprototestv1.PartialOneof](
		Override("alpha", field.String("alpha")),
		Override("bravo", field.String("bravo")),
		Override("charlie", field.String("charlie")),
	)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	if len(d.fields) != 3 {
		t.Fatalf("want 3 derived (overridden) fields, got %d: %v", len(d.fields), fieldNames(d))
	}
}

// TestOneofPartialResolutionFails proves excluding only some members of
// a real oneof still fails, and the failure names the unresolved
// members only — never the resolved ones.
func TestOneofPartialResolutionFails(t *testing.T) {
	_, err := derive[*mixinforprototestv1.PartialOneof](Exclude("alpha"))
	if err == nil {
		t.Fatal("want an error: bravo and charlie are still unresolved")
	}
	msg := err.Error()
	for _, want := range []string{"bravo", "charlie"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error %q missing unresolved member %q", msg, want)
		}
	}
	if strings.Contains(msg, "unresolved member(s): alpha") {
		t.Fatalf("error %q should not list the resolved member alpha as unresolved", msg)
	}
}

// TestSyntheticOneofNotGated proves a proto3 `optional` scalar's
// compiler-synthesized one-member oneof is never conflated with a real
// oneof: the Presence and Oneofs corpus messages both derive their
// `optional` scalars without triggering the unresolved-oneof gate.
func TestSyntheticOneofNotGated(t *testing.T) {
	dp, err := derive[*mixinforprototestv1.Presence]()
	if err != nil {
		t.Fatalf("Presence: derive: %v", err)
	}
	if fieldByName(dp, "optional_string") == nil || fieldByName(dp, "optional_int32") == nil {
		t.Fatalf("want optional_string and optional_int32 both derived, got %v", fieldNames(dp))
	}

	// Oneofs also carries a real oneof ("choice": a/b), unresolved here
	// on purpose — this test is about the synthetic oneof around
	// `maybe`, not the real-oneof gate (which has its own dedicated
	// coverage above), so a/b are excluded to isolate the assertion.
	do, err := derive[*mixinforprototestv1.Oneofs](Exclude("a", "b"))
	if err != nil {
		t.Fatalf("Oneofs: derive: %v", err)
	}
	if fieldByName(do, "maybe") == nil {
		t.Fatalf("want maybe (the optional scalar) derived, got %v", fieldNames(do))
	}
}
