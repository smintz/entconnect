package mixinforproto

import (
	"reflect"
	"strings"
	"sync"
	"testing"

	"buf.build/go/protovalidate"

	"entgo.io/ent/schema/field"

	mixinforprototestv1 "github.com/smintz/entconnect/mixinforproto/internal/gen/mixinforprototestv1"
)

// TestResolveFieldRules_NilForConstraintFreeField pins the ANNO-02 empty
// edge at its source: protovalidate.ResolveFieldRules returns (nil, nil)
// for a field carrying zero protovalidate constraints — the Tracer's
// "name" field, which has no buf.validate.field option at all. fieldmap.go
// depends on this nil being handled, not dereferenced (see mapField).
func TestResolveFieldRules_NilForConstraintFreeField(t *testing.T) {
	md := (*new(*mixinforprototestv1.Tracer)).ProtoReflect().Descriptor()
	fd := md.Fields().ByName("name")
	if fd == nil {
		t.Fatal("want a 'name' field descriptor on Tracer")
	}
	rules, err := protovalidate.ResolveFieldRules(fd)
	if err != nil {
		t.Fatalf("ResolveFieldRules: %v", err)
	}
	if rules != nil {
		t.Fatalf("want (nil, nil) for a constraint-free field, got rules=%v", rules)
	}
}

// TestDerive_TracerMapsSingleStringField proves the walking skeleton's
// one production path: one proto string field becomes one derived
// ent.Field carrying a SourceField annotation, and the message-level
// SourceMessage annotation is assembled alongside it.
func TestDerive_TracerMapsSingleStringField(t *testing.T) {
	d, err := derive[*mixinforprototestv1.Tracer]()
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	if len(d.fields) != 1 {
		t.Fatalf("want 1 derived field, got %d", len(d.fields))
	}
	desc := d.fields[0].Descriptor()
	if desc.Name != "name" {
		t.Fatalf("want field name %q, got %q", "name", desc.Name)
	}

	if d.message.Message != "mixinforprototest.v1.Tracer" {
		t.Fatalf("want SourceMessage.Message %q, got %q", "mixinforprototest.v1.Tracer", d.message.Message)
	}
	if d.message.ContractVersion != ContractVersion {
		t.Fatalf("want ContractVersion=%d, got %d", ContractVersion, d.message.ContractVersion)
	}
	if len(d.message.Fields) != 1 || d.message.Fields[0] != (FieldRef{Name: "name", Number: 1}) {
		t.Fatalf("want field inventory [{name 1}], got %v", d.message.Fields)
	}
}

// TestDerive_SourceFieldAnnotationEncoding proves ANNO-02's encoding
// edge: the derived field's SourceField.FieldName is the exact proto
// field name and SourceField.Number is the descriptor's field number, so
// a contract rename is detectable by number.
func TestDerive_SourceFieldAnnotationEncoding(t *testing.T) {
	d, err := derive[*mixinforprototestv1.Tracer]()
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	var sf *SourceField
	for _, a := range d.fields[0].Descriptor().Annotations {
		if a.Name() == MixinForProtoField {
			v, ok := a.(SourceField)
			if !ok {
				t.Fatalf("want SourceField annotation, got %T", a)
			}
			sf = &v
		}
	}
	if sf == nil {
		t.Fatal("want a SourceField annotation on the derived field")
	}
	if sf.FieldName != "name" {
		t.Fatalf("want FieldName=%q, got %q", "name", sf.FieldName)
	}
	if sf.Number != 1 {
		t.Fatalf("want Number=1, got %d", sf.Number)
	}
	if sf.ContractVersion != ContractVersion {
		t.Fatalf("want ContractVersion=%d, got %d", ContractVersion, sf.ContractVersion)
	}
	// ANNO-02 empty: constraint-ID lists are empty, not absent —
	// Plan 01 makes no protovalidate.ResolveFieldRules call at all yet
	// (Plan 04's job), so these must still be non-nil empty slices
	// rather than nil, matching the "empty, not omitted" contract.
	if sf.TranslatedIDs == nil {
		t.Fatal("want non-nil empty TranslatedIDs")
	}
	if sf.ResidualIDs == nil {
		t.Fatal("want non-nil empty ResidualIDs")
	}
}

// TestDerive_EmptyMessageYieldsEmptyNotNil proves the MIX-01/ANNO-01
// "empty" edge: a message with zero fields still derives a zero-length
// (not nil) []ent.Field, and still emits exactly one SourceMessage
// annotation whose field inventory is zero-length (not nil) — so a later
// drift check can tell "empty contract" apart from "mixin never ran".
func TestDerive_EmptyMessageYieldsEmptyNotNil(t *testing.T) {
	d, err := derive[*mixinforprototestv1.Empty]()
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	if d.fields == nil {
		t.Fatal("want non-nil empty []ent.Field slice")
	}
	if len(d.fields) != 0 {
		t.Fatalf("want zero derived fields, got %d", len(d.fields))
	}
	if d.message.Message != "mixinforprototest.v1.Empty" {
		t.Fatalf("want SourceMessage.Message %q, got %q", "mixinforprototest.v1.Empty", d.message.Message)
	}
	if d.message.Fields == nil {
		t.Fatal("want non-nil empty field inventory")
	}
	if len(d.message.Fields) != 0 {
		t.Fatalf("want zero-length field inventory, got %d", len(d.message.Fields))
	}
	if d.message.Excluded == nil || d.message.Overridden == nil {
		t.Fatal("want non-nil empty Excluded/Overridden slices")
	}
	if d.message.BoundaryOnly == nil {
		t.Fatal("want non-nil empty BoundaryOnly slice")
	}
	if len(d.message.BoundaryOnly) != 0 {
		t.Fatalf("want zero-length BoundaryOnly for an empty message, got %v", d.message.BoundaryOnly)
	}
}

// TestDerive_BoundaryOnlyRecordsSkippedFieldRule pins D-09's second half
// (03-02 Task 3): messages.proto's singular_message field carries a
// required rule and is skipped by default (never opted into AsJSON), so
// it can never be enforced at the storage layer. SourceMessage.
// BoundaryOnly must contain exactly one entry naming it, with a non-empty
// RuleIDs slice and BoundaryOnlyNoEntField as the reason — this field's
// derivation kind (a plain, un-opted-in message field) produces no ent
// field at all, distinct from the excluded/overridden causes below.
func TestDerive_BoundaryOnlyRecordsSkippedFieldRule(t *testing.T) {
	d, err := derive[*mixinforprototestv1.Messages](AsJSON("as_json_target"))
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	var entries []BoundaryOnlyRule
	for _, e := range d.message.BoundaryOnly {
		if e.Field == "singular_message" {
			entries = append(entries, e)
		}
	}
	if len(entries) != 1 {
		t.Fatalf("want exactly 1 BoundaryOnly entry for singular_message, got %d: %v", len(entries), d.message.BoundaryOnly)
	}
	got := entries[0]
	if len(got.RuleIDs) == 0 {
		t.Fatalf("want a non-empty RuleIDs slice, got %v", got.RuleIDs)
	}
	if got.Reason != BoundaryOnlyNoEntField {
		t.Fatalf("want reason %q, got %q", BoundaryOnlyNoEntField, got.Reason)
	}
	// repeated_message and as_json_target carry no protovalidate rules
	// at all, so neither should appear in BoundaryOnly (an empty
	// RuleIDs set is never recorded — see recordBoundaryOnly).
	for _, name := range []string{"repeated_message", "as_json_target"} {
		for _, e := range d.message.BoundaryOnly {
			if e.Field == name {
				t.Fatalf("want no BoundaryOnly entry for rule-free field %q, got %v", name, e)
			}
		}
	}
}

// TestDerive_BoundaryOnlyExcludedAndOverridden proves the other two
// reasons: RequiredString's "value" field carries `required = true`
// (constraints.proto); excluding it or overriding it each suppress its
// derivation entirely, and each must be recorded with the matching
// reason (BoundaryOnlyExcluded / BoundaryOnlyOverridden), distinct from
// BoundaryOnlyNoEntField.
func TestDerive_BoundaryOnlyExcludedAndOverridden(t *testing.T) {
	t.Run("Excluded", func(t *testing.T) {
		d, err := derive[*mixinforprototestv1.RequiredString](Exclude("value"))
		if err != nil {
			t.Fatalf("derive: %v", err)
		}
		if len(d.message.BoundaryOnly) != 1 {
			t.Fatalf("want exactly 1 BoundaryOnly entry, got %d: %v", len(d.message.BoundaryOnly), d.message.BoundaryOnly)
		}
		got := d.message.BoundaryOnly[0]
		if got.Field != "value" || got.Reason != BoundaryOnlyExcluded || len(got.RuleIDs) == 0 {
			t.Fatalf("want {value, non-empty RuleIDs, %q}, got %+v", BoundaryOnlyExcluded, got)
		}
	})

	t.Run("Overridden", func(t *testing.T) {
		d, err := derive[*mixinforprototestv1.RequiredString](Override("value", field.Bool("value")))
		if err != nil {
			t.Fatalf("derive: %v", err)
		}
		if len(d.message.BoundaryOnly) != 1 {
			t.Fatalf("want exactly 1 BoundaryOnly entry, got %d: %v", len(d.message.BoundaryOnly), d.message.BoundaryOnly)
		}
		got := d.message.BoundaryOnly[0]
		if got.Field != "value" || got.Reason != BoundaryOnlyOverridden || len(got.RuleIDs) == 0 {
			t.Fatalf("want {value, non-empty RuleIDs, %q}, got %+v", BoundaryOnlyOverridden, got)
		}
	})
}

// TestDerive_BoundaryOnlySortedAndDeterministic pins D-24 for BoundaryOnly
// specifically: MixedFieldRules (constraints.proto) carries three
// rule-bearing fields; excluding all three produces three BoundaryOnly
// entries that must come back sorted by field name and byte-identical
// (via reflect.DeepEqual on the decoded struct) across repeated derive
// calls, never in map-iteration order.
func TestDerive_BoundaryOnlySortedAndDeterministic(t *testing.T) {
	opts := []Option{Exclude("both", "standard_only", "cel_only")}

	first, err := derive[*mixinforprototestv1.MixedFieldRules](opts...)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	if len(first.message.BoundaryOnly) != 3 {
		t.Fatalf("want 3 BoundaryOnly entries, got %d: %v", len(first.message.BoundaryOnly), first.message.BoundaryOnly)
	}
	wantOrder := []string{"both", "cel_only", "standard_only"}
	for i, want := range wantOrder {
		if got := first.message.BoundaryOnly[i].Field; got != want {
			t.Fatalf("BoundaryOnly[%d].Field = %q, want %q (want sorted order %v)", i, got, want, wantOrder)
		}
	}

	for i := 0; i < 10; i++ {
		d, err := derive[*mixinforprototestv1.MixedFieldRules](opts...)
		if err != nil {
			t.Fatalf("run %d: derive: %v", i, err)
		}
		if !reflect.DeepEqual(d.message.BoundaryOnly, first.message.BoundaryOnly) {
			t.Fatalf("run %d: BoundaryOnly changed: got %v, want %v", i, d.message.BoundaryOnly, first.message.BoundaryOnly)
		}
	}
}

// TestDerive_FieldOrderMatchesDeclarationOrder proves MIX-13/D-24:
// derived field order is the proto descriptor's declaration order, never
// a map-derived order.
func TestDerive_FieldOrderMatchesDeclarationOrder(t *testing.T) {
	d, err := derive[*mixinforprototestv1.MultiField]()
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	wantNames := []string{"first_name", "last_name", "email_address"}
	wantNumbers := []int32{1, 2, 3}

	if len(d.fields) != len(wantNames) {
		t.Fatalf("want %d fields, got %d", len(wantNames), len(d.fields))
	}
	for i, want := range wantNames {
		if got := d.fields[i].Descriptor().Name; got != want {
			t.Fatalf("field %d: want name %q, got %q", i, want, got)
		}
	}
	if len(d.message.Fields) != len(wantNames) {
		t.Fatalf("want %d inventory entries, got %d", len(wantNames), len(d.message.Fields))
	}
	for i := range wantNames {
		want := FieldRef{Name: wantNames[i], Number: wantNumbers[i]}
		if d.message.Fields[i] != want {
			t.Fatalf("inventory %d: want %v, got %v", i, want, d.message.Fields[i])
		}
	}
}

// TestDerive_DeterministicAcrossRepeatedCalls runs derive several times
// in-process and asserts byte-identical field order every time — the
// same property go test -count=5 checks at the process level (D-24,
// PITFALLS Pitfall 10).
func TestDerive_DeterministicAcrossRepeatedCalls(t *testing.T) {
	var first []string
	for i := 0; i < 25; i++ {
		d, err := derive[*mixinforprototestv1.MultiField]()
		if err != nil {
			t.Fatalf("derive: %v", err)
		}
		names := fieldNames(d)
		if first == nil {
			first = names
			continue
		}
		if !reflect.DeepEqual(names, first) {
			t.Fatalf("run %d: field order changed: got %v, want %v", i, names, first)
		}
	}
}

// TestDerive_ConcurrentSafe proves the MIX-01 concurrency edge: derive
// holds no shared mutable package state, so parallel derivations of the
// same message type from multiple goroutines yield identical output and
// pass under go test -race.
func TestDerive_ConcurrentSafe(t *testing.T) {
	const n = 20
	results := make([][]string, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			d, err := derive[*mixinforprototestv1.MultiField]()
			if err != nil {
				t.Errorf("goroutine %d: derive: %v", i, err)
				return
			}
			results[i] = fieldNames(d)
		}(i)
	}
	wg.Wait()

	want := results[0]
	for i, got := range results {
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("goroutine %d: got %v, want %v", i, got, want)
		}
	}
}

// TestDerive_FormerlyUnsupportedKindNowMaps proves Plan 02 closed the
// gap Plan 01's walking skeleton deliberately left open: Unsupported's
// one int32 field (the fixture Plan 01 used to prove an un-mapped kind
// fails loudly) now derives successfully once fieldmap.go's exhaustive
// scalar-kind switch (MIX-02) covers Int32Kind. This supersedes
// TestDerive_UnsupportedKindFailsLoudly, which asserted the pre-Plan-02
// behavior and would now be asserting a false negative — every
// proto3-expressible scalar kind is covered by mapScalar, so a
// still-unsupported-kind fixture cannot be constructed from valid
// proto3 syntax (only GroupKind, a proto2-only construct, remains
// unhandled, and proto3 has no keyword that produces it).
func TestDerive_FormerlyUnsupportedKindNowMaps(t *testing.T) {
	d, err := derive[*mixinforprototestv1.Unsupported]()
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	if len(d.fields) != 1 {
		t.Fatalf("want 1 derived field, got %d", len(d.fields))
	}
	desc := d.fields[0].Descriptor()
	if desc.Name != "count" {
		t.Fatalf("want field name %q, got %q", "count", desc.Name)
	}
}

// TestMixinForProto_FieldsAndAnnotations exercises the ent.Mixin adapter
// itself, not just the pure derive core.
func TestMixinForProto_FieldsAndAnnotations(t *testing.T) {
	m := MixinForProto[*mixinforprototestv1.Tracer]()

	fields := m.Fields()
	if len(fields) != 1 {
		t.Fatalf("want 1 field, got %d", len(fields))
	}

	annos := m.Annotations()
	if len(annos) != 1 {
		t.Fatalf("want 1 annotation, got %d", len(annos))
	}
	sm, ok := annos[0].(SourceMessage)
	if !ok {
		t.Fatalf("want SourceMessage annotation, got %T", annos[0])
	}
	if sm.Message != "mixinforprototest.v1.Tracer" {
		t.Fatalf("want full name %q, got %q", "mixinforprototest.v1.Tracer", sm.Message)
	}
}

// TestMixinForProto_FormerlyUnsupportedKindNowMaps mirrors
// TestDerive_FormerlyUnsupportedKindNowMaps through the ent.Mixin
// adapter: Fields() no longer panics for Unsupported now that MIX-02's
// scalar-kind switch is exhaustive. Supersedes
// TestMixinForProto_PanicsOnUnsupportedKind for the same reason.
func TestMixinForProto_FormerlyUnsupportedKindNowMaps(t *testing.T) {
	fields := MixinForProto[*mixinforprototestv1.Unsupported]().Fields()
	if len(fields) != 1 {
		t.Fatalf("want 1 field, got %d", len(fields))
	}
	if got := fields[0].Descriptor().Name; got != "count" {
		t.Fatalf("want field name %q, got %q", "count", got)
	}
}

// TestValidate_MirrorsDeriveWithoutSubprocess proves MIX-12/D-07:
// Validate[M] reproduces derive's verdict in-process, for both the
// success and failure paths, without ever going through
// entc.LoadGraph/entc/load's gorun() subprocess (that boundary is
// exercised only by internal/boundarytest). The failure path now uses
// an AsJSON(...) naming an unknown field (Plan 02's own failure
// surface, MIX-09) rather than Plan 01's Unsupported fixture, which
// Plan 02's exhaustive scalar mapping retired as a failure case (see
// TestDerive_FormerlyUnsupportedKindNowMaps).
func TestValidate_MirrorsDeriveWithoutSubprocess(t *testing.T) {
	if err := Validate[*mixinforprototestv1.Tracer](); err != nil {
		t.Fatalf("want nil error for Tracer, got %v", err)
	}

	err := Validate[*mixinforprototestv1.Messages](AsJSON("does_not_exist"))
	if err == nil {
		t.Fatal("want an error for AsJSON naming an unknown field")
	}
	for _, want := range []string{"mixinforprototest.v1.Messages", "does_not_exist", "AsJSON"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q missing %q", err.Error(), want)
		}
	}
}

func fieldNames(d *derivation) []string {
	names := make([]string, len(d.fields))
	for i, f := range d.fields {
		names[i] = f.Descriptor().Name
	}
	return names
}
