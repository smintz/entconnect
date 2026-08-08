package mixinforproto

import (
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"

	"buf.build/go/protovalidate"

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

// TestDerive_UnsupportedKindFailsLoudly proves that a field kind this
// slice does not map yet fails at derivation time with a self-sufficient
// error naming the message and the field, rather than silently guessing
// or dropping the field (D-08/D-09).
func TestDerive_UnsupportedKindFailsLoudly(t *testing.T) {
	_, err := derive[*mixinforprototestv1.Unsupported]()
	if err == nil {
		t.Fatal("want an error for an unsupported field kind")
	}
	msg := err.Error()
	for _, want := range []string{"mixinforprototest.v1.Unsupported", "count"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error message %q missing %q", msg, want)
		}
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

// TestMixinForProto_PanicsOnUnsupportedKind proves panicking is isolated
// to the ent.Mixin adapter (D-06) and that the panic's first line is
// self-sufficient (D-08).
func TestMixinForProto_PanicsOnUnsupportedKind(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("want Fields() to panic on an unsupported field kind")
		}
		msg := fmt.Sprint(r)
		for _, want := range []string{"mixinforprototest.v1.Unsupported", "count"} {
			if !strings.Contains(msg, want) {
				t.Fatalf("panic message %q not self-sufficient, missing %q", msg, want)
			}
		}
	}()
	MixinForProto[*mixinforprototestv1.Unsupported]().Fields()
}

// TestValidate_MirrorsDeriveWithoutSubprocess proves MIX-12/D-07:
// Validate[M] reproduces derive's verdict in-process, for both the
// success and failure paths, without ever going through
// entc.LoadGraph/entc/load's gorun() subprocess (that boundary is
// exercised only by internal/boundarytest).
func TestValidate_MirrorsDeriveWithoutSubprocess(t *testing.T) {
	if err := Validate[*mixinforprototestv1.Tracer](); err != nil {
		t.Fatalf("want nil error for Tracer, got %v", err)
	}

	err := Validate[*mixinforprototestv1.Unsupported]()
	if err == nil {
		t.Fatal("want an error for Unsupported")
	}
	if !strings.Contains(err.Error(), "count") {
		t.Fatalf("error %q not self-sufficient", err.Error())
	}
}

func fieldNames(d *derivation) []string {
	names := make([]string, len(d.fields))
	for i, f := range d.fields {
		names[i] = f.Descriptor().Name
	}
	return names
}
