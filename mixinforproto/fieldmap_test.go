package mixinforproto

import (
	"strings"
	"testing"

	goldie "github.com/sebdah/goldie/v2"

	"github.com/smintz/entconnect/mixinforproto/internal/fieldproj"
	mixinforprototestv1 "github.com/smintz/entconnect/mixinforproto/internal/gen/mixinforprototestv1"
)

// assertGolden derives fields via deriveFn, golden-asserts the
// fieldproj.ProjectAll projection under the given fixture name (-update
// regenerates, D-23 as amended by R4), and returns the raw derivation
// result so the caller can layer additional, non-golden assertions on
// top (the specific per-message must-haves this plan's acceptance
// criteria name).
func assertGolden(t *testing.T, name string, deriveFn func(...Option) (*derivation, error), opts ...Option) *derivation {
	t.Helper()
	d, err := deriveFn(opts...)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	goldie.New(t).AssertJson(t, name, fieldproj.ProjectAll(d.fields))
	return d
}

// fieldByName finds the derived field named n, or nil if absent.
func fieldByName(d *derivation, n string) *fieldproj.Field {
	for _, f := range fieldproj.ProjectAll(d.fields) {
		if f.Name == n {
			ff := f
			return &ff
		}
	}
	return nil
}

// TestGolden golden-asserts one fixture per corpus message (D-23/R4),
// with -count=5 in this plan's <verify> proving byte-identical output
// across repeated runs (D-24/MIX-13).
func TestGolden(t *testing.T) {
	t.Run("Scalars", func(t *testing.T) {
		d := assertGolden(t, "scalars", derive[*mixinforprototestv1.Scalars])
		if len(d.fields) != 15 {
			t.Fatalf("want 15 derived fields (one per scalar kind), got %d", len(d.fields))
		}
	})

	t.Run("Enums", func(t *testing.T) {
		d := assertGolden(t, "enums", derive[*mixinforprototestv1.Enums])
		f := fieldByName(d, "status")
		if f == nil {
			t.Fatal("want a derived field named status")
		}
		wantEnums := []fieldproj.EnumValue{
			{N: "STATUS_UNSPECIFIED", V: "STATUS_UNSPECIFIED"},
			{N: "STATUS_ACTIVE", V: "STATUS_ACTIVE"},
			{N: "STATUS_INACTIVE", V: "STATUS_INACTIVE"},
		}
		if len(f.Enums) != len(wantEnums) {
			t.Fatalf("want %d enum values, got %d: %v", len(wantEnums), len(f.Enums), f.Enums)
		}
		for i, want := range wantEnums {
			if f.Enums[i] != want {
				t.Fatalf("enum %d: want %v, got %v", i, want, f.Enums[i])
			}
		}
	})

	t.Run("Wkt", func(t *testing.T) {
		d := assertGolden(t, "wkt", derive[*mixinforprototestv1.Wkt])
		if len(d.fields) != 3 {
			t.Fatalf("want exactly 3 derived fields (Timestamp, Struct, Value; FieldMask and Duration skipped), got %d", len(d.fields))
		}
		for _, skipped := range []string{"field_mask_field", "duration_field"} {
			if fieldByName(d, skipped) != nil {
				t.Fatalf("want no derived field for skipped WKT %q", skipped)
			}
		}
		ts := fieldByName(d, "timestamp_field")
		if ts == nil || ts.TypeKind != "time.Time" {
			t.Fatalf("want timestamp_field to derive as time.Time, got %+v", ts)
		}
	})

	t.Run("Presence", func(t *testing.T) {
		d := assertGolden(t, "presence", derive[*mixinforprototestv1.Presence])

		optStr := fieldByName(d, "optional_string")
		if optStr == nil || !optStr.Nillable || !optStr.Optional {
			t.Fatalf("want optional_string Nillable=true Optional=true, got %+v", optStr)
		}
		optInt := fieldByName(d, "optional_int32")
		if optInt == nil || !optInt.Nillable || !optInt.Optional {
			t.Fatalf("want optional_int32 Nillable=true Optional=true, got %+v", optInt)
		}

		plainStr := fieldByName(d, "plain_string")
		if plainStr == nil || plainStr.Nillable || plainStr.Optional || plainStr.Default == "" {
			t.Fatalf("want plain_string Nillable=false Optional=false with a non-empty Default, got %+v", plainStr)
		}
		plainInt := fieldByName(d, "plain_int32")
		if plainInt == nil || plainInt.Nillable || plainInt.Optional || plainInt.Default == "" {
			t.Fatalf("want plain_int32 Nillable=false Optional=false with a non-empty Default, got %+v", plainInt)
		}
	})

	t.Run("Maps", func(t *testing.T) {
		d := assertGolden(t, "maps", derive[*mixinforprototestv1.Maps])
		if len(d.fields) != 2 {
			t.Fatalf("want exactly 2 derived fields (message_map skipped), got %d", len(d.fields))
		}
		if fieldByName(d, "message_map") != nil {
			t.Fatal("want no derived field for the message-valued map")
		}
		for _, name := range []string{"string_map", "int_map"} {
			f := fieldByName(d, name)
			if f == nil || f.TypeKind != "json.RawMessage" {
				t.Fatalf("want %s to derive as a JSON field, got %+v", name, f)
			}
		}
	})

	t.Run("Messages", func(t *testing.T) {
		d := assertGolden(t, "messages", derive[*mixinforprototestv1.Messages], AsJSON("as_json_target"))
		if len(d.fields) != 1 {
			t.Fatalf("want exactly 1 derived field (only the AsJSON-opted-in one), got %d", len(d.fields))
		}
		for _, skipped := range []string{"singular_message", "repeated_message"} {
			if fieldByName(d, skipped) != nil {
				t.Fatalf("want no derived field for %q (message field, not opted into AsJSON)", skipped)
			}
		}
		got := fieldByName(d, "as_json_target")
		if got == nil || got.TypeKind != "json.RawMessage" {
			t.Fatalf("want as_json_target to derive as a JSON field, got %+v", got)
		}
	})

	t.Run("Oneofs", func(t *testing.T) {
		// Plan 04's unresolved-oneof gate (MIX-10) now requires every
		// real-oneof member to be explicitly resolved; a/b are
		// excluded here purely to keep this fixture's original intent
		// (proving classify()'s MIX-05/MIX-10 adjacency, not the gate
		// itself — that has its own dedicated coverage in
		// reserved_test.go).
		d := assertGolden(t, "oneofs", derive[*mixinforprototestv1.Oneofs], Exclude("a", "b"))
		if len(d.fields) != 1 {
			t.Fatalf("want exactly 1 derived field (maybe; a/b are real-oneof members), got %d", len(d.fields))
		}
		maybe := fieldByName(d, "maybe")
		if maybe == nil || !maybe.Nillable {
			t.Fatalf("want maybe to derive Nillable=true, got %+v", maybe)
		}
		for _, oneofMember := range []string{"a", "b"} {
			if fieldByName(d, oneofMember) != nil {
				t.Fatalf("want no derived field for real-oneof member %q (MIX-10 adjacency)", oneofMember)
			}
		}
	})

	t.Run("Empty", func(t *testing.T) {
		// Reuses the mixinforprototestv1.Empty message Plan 01 already
		// declared in tracer.proto — the corpus does not redeclare
		// "Empty" in scalars.proto because messages share one proto
		// package (mixinforprototest.v1) and a second `message Empty {}`
		// would collide with the one Plan 01 already committed (see
		// 01-02-SUMMARY.md Deviations).
		d := assertGolden(t, "empty", derive[*mixinforprototestv1.Empty])
		if len(d.fields) != 0 {
			t.Fatalf("want zero derived fields, got %d", len(d.fields))
		}
	})
}

// TestScalarsCoverAllKinds asserts the Scalars golden has one derived
// field per declared proto field with no widening — the int32 member
// maps to an int32-typed ent field, not int64, and so on for every
// scalar kind in the corpus (MIX-02).
func TestScalarsCoverAllKinds(t *testing.T) {
	d, err := derive[*mixinforprototestv1.Scalars]()
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	want := map[string]string{
		"double_field":   "float64",
		"float_field":    "float32",
		"int32_field":    "int32",
		"int64_field":    "int64",
		"uint32_field":   "uint32",
		"uint64_field":   "uint64",
		"sint32_field":   "int32",
		"sint64_field":   "int64",
		"fixed32_field":  "uint32",
		"fixed64_field":  "uint64",
		"sfixed32_field": "int32",
		"sfixed64_field": "int64",
		"bool_field":     "bool",
		"string_field":   "string",
		"bytes_field":    "[]byte",
	}
	if len(d.fields) != len(want) {
		t.Fatalf("want %d derived fields, got %d", len(want), len(d.fields))
	}
	for _, pf := range fieldproj.ProjectAll(d.fields) {
		wantKind, ok := want[pf.Name]
		if !ok {
			t.Fatalf("unexpected derived field %q", pf.Name)
		}
		if pf.TypeKind != wantKind {
			t.Fatalf("field %s: want TypeKind %q, got %q (widening or narrowing regression)", pf.Name, wantKind, pf.TypeKind)
		}
	}
}

// TestAsJSON covers MIX-09's empty edge: AsJSON("") and AsJSON naming a
// field that does not exist each fail at schema load naming the message
// and the offending option; AsJSON naming a scalar (non-message) field
// also fails rather than silently succeeding. Every case asserts the
// error text contains the message name, the field name, and "AsJSON".
func TestAsJSON(t *testing.T) {
	t.Run("unknown field name", func(t *testing.T) {
		_, err := derive[*mixinforprototestv1.Messages](AsJSON("does_not_exist"))
		if err == nil {
			t.Fatal("want an error for AsJSON naming an unknown field")
		}
		for _, want := range []string{"mixinforprototest.v1.Messages", "does_not_exist", "AsJSON"} {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("error %q missing %q", err.Error(), want)
			}
		}
	})

	t.Run("empty field name", func(t *testing.T) {
		_, err := derive[*mixinforprototestv1.Messages](AsJSON(""))
		if err == nil {
			t.Fatal("want an error for AsJSON(\"\")")
		}
		for _, want := range []string{"mixinforprototest.v1.Messages", "AsJSON"} {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("error %q missing %q", err.Error(), want)
			}
		}
	})

	t.Run("scalar-typed field", func(t *testing.T) {
		// Presence.plain_string is a scalar (string) field, not a
		// message field — AsJSON must reject it rather than silently
		// accepting a non-message target.
		_, err := derive[*mixinforprototestv1.Presence](AsJSON("plain_string"))
		if err == nil {
			t.Fatal("want an error for AsJSON naming a scalar-typed field")
		}
		for _, want := range []string{"mixinforprototest.v1.Presence", "plain_string", "AsJSON"} {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("error %q missing %q", err.Error(), want)
			}
		}
	})
}
