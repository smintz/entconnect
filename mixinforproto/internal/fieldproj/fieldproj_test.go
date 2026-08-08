package fieldproj

import (
	"encoding/json"
	"testing"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

// TestProject_ValidatorCarryingFieldMarshalsCleanly is the R4 regression
// guard (01-02-PLAN.md Reconciliation R4 / 01-RESEARCH.md Correction 2):
// a field built with a Tier 1 validator attached must project into a
// struct that json.Marshal never errors on, even though the raw
// *field.Descriptor.Validators slice holds a live closure that
// encoding/json cannot serialize.
func TestProject_ValidatorCarryingFieldMarshalsCleanly(t *testing.T) {
	f := field.String("x").MaxLen(64)

	p := Project(f)
	if p.ValidatorCount != 1 {
		t.Fatalf("want ValidatorCount 1, got %d", p.ValidatorCount)
	}

	b, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("json.Marshal(Project(...)): %v", err)
	}
	if len(b) == 0 {
		t.Fatal("want non-empty marshaled output")
	}
}

// TestProject_NilDefaultRendersStableEmptyString proves a field with no
// Default (e.g. an optional scalar) projects to an empty Default string,
// not the Go "<nil>" literal whose exact spelling can vary by build.
func TestProject_NilDefaultRendersStableEmptyString(t *testing.T) {
	f := field.String("x").Optional().Nillable()

	p := Project(f)
	if p.Default != "" {
		t.Fatalf("want empty Default for a nil-default field, got %q", p.Default)
	}
}

// TestProjectAll_PreservesInputOrder proves ProjectAll never sorts —
// sorting here would hide exactly the determinism regression the golden
// tests exist to catch (D-24/MIX-13).
func TestProjectAll_PreservesInputOrder(t *testing.T) {
	names := []string{"zebra", "apple", "middle"}
	in := make([]ent.Field, 0, len(names))
	for _, name := range names {
		in = append(in, field.String(name))
	}

	got := ProjectAll(in)
	if len(got) != len(names) {
		t.Fatalf("want %d projected fields, got %d", len(names), len(got))
	}
	for i, want := range names {
		if got[i].Name != want {
			t.Fatalf("index %d: want name %q, got %q", i, want, got[i].Name)
		}
	}
}
