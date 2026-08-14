package mixinforproto

import (
	"bytes"
	"errors"
	"reflect"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"buf.build/go/protovalidate"

	mixinforprototestv1 "github.com/smintz/entconnect/mixinforproto/internal/gen/mixinforprototestv1"
)

// fieldDesc returns the field descriptor named name on M's zero-value
// message descriptor — the same (*new(M)).ProtoReflect().Descriptor()
// trick derive[M] itself uses (fieldmap.go), so these tests exercise
// reverse.go against the exact descriptors mixinforproto derives from,
// never a hand-built stand-in.
func fieldDesc[M proto.Message](t *testing.T, name string) protoreflect.FieldDescriptor {
	t.Helper()
	md := (*new(M)).ProtoReflect().Descriptor()
	fd := md.Fields().ByName(protoreflect.Name(name))
	if fd == nil {
		t.Fatalf("fieldDesc: %s has no field named %q", md.FullName(), name)
	}
	return fd
}

// notValidationError asserts err is a plain, non-nil, non-protovalidate
// error — D-12's fail-closed contract: a reverse-conversion fault must
// never errors.As-match *protovalidate.ValidationError, or it would
// fabricate a constraint ID that no rule actually produced.
func notValidationError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("want a non-nil error, got nil")
	}
	var ve *protovalidate.ValidationError
	if errors.As(err, &ve) {
		t.Fatalf("want a plain D-12 data-integrity error, got *protovalidate.ValidationError: %v", err)
	}
}

// --- Test 1: exhaustive proto-scalar-kind round trip -------------------

func TestReverseValue_ScalarRoundTrip(t *testing.T) {
	md := (*new(mixinforprototestv1.ReverseScalars)).ProtoReflect().Descriptor()

	tests := []struct {
		field string
		value protoreflect.Value
	}{
		{"double_field", protoreflect.ValueOfFloat64(1.5)},
		{"float_field", protoreflect.ValueOfFloat32(2.5)},
		{"int32_field", protoreflect.ValueOfInt32(-7)},
		{"int64_field", protoreflect.ValueOfInt64(-9000000000)},
		{"uint32_field", protoreflect.ValueOfUint32(7)},
		{"uint64_field", protoreflect.ValueOfUint64(9000000000)},
		{"sint32_field", protoreflect.ValueOfInt32(-11)},
		{"sint64_field", protoreflect.ValueOfInt64(-13)},
		{"fixed32_field", protoreflect.ValueOfUint32(17)},
		{"fixed64_field", protoreflect.ValueOfUint64(19)},
		{"sfixed32_field", protoreflect.ValueOfInt32(-23)},
		{"sfixed64_field", protoreflect.ValueOfInt64(-29)},
		{"bool_field", protoreflect.ValueOfBool(true)},
		{"string_field", protoreflect.ValueOfString("hello")},
		{"bytes_field", protoreflect.ValueOfBytes([]byte("world"))},
	}
	if len(tests) != 15 {
		t.Fatalf("want 15 proto scalar kinds under test, got %d — a kind was dropped from this table", len(tests))
	}

	for _, tc := range tests {
		t.Run(tc.field, func(t *testing.T) {
			fd := md.Fields().ByName(protoreflect.Name(tc.field))
			if fd == nil {
				t.Fatalf("ReverseScalars has no field named %q", tc.field)
			}
			got, err := reverseValue(fd, "scalar", tc.value.Interface())
			if err != nil {
				t.Fatalf("reverseValue(%s): %v", tc.field, err)
			}
			if !got.IsValid() {
				t.Fatalf("reverseValue(%s) returned an invalid Value", tc.field)
			}
			if !reflect.DeepEqual(got.Interface(), tc.value.Interface()) {
				t.Fatalf("reverseValue(%s) = %#v, want %#v", tc.field, got.Interface(), tc.value.Interface())
			}
		})
	}
}

// TestReverseValue_ScalarWrongGoType proves Test 8's "wrong dynamic type"
// requirement for the scalar leg specifically: a Go value of the wrong
// type for the field's kind returns a plain, named, non-protovalidate
// error, never a panic.
func TestReverseValue_ScalarWrongGoType(t *testing.T) {
	fd := fieldDesc[*mixinforprototestv1.ReverseScalars](t, "int32_field")
	_, err := reverseValue(fd, "scalar", "not an int32")
	notValidationError(t, err)
}

// --- Test 2: optionalScalar absence ------------------------------------

func TestReverseValue_OptionalScalarAbsent(t *testing.T) {
	fd := fieldDesc[*mixinforprototestv1.Presence](t, "optional_string")
	got, err := reverseValue(fd, "optionalScalar", nil)
	if err != nil {
		t.Fatalf("reverseValue(nil): unexpected error: %v", err)
	}
	if got.IsValid() {
		t.Fatalf("reverseValue(nil) = %#v, want an invalid (absent) Value, not a fabricated proto3 zero", got.Interface())
	}
}

func TestReverseValue_OptionalScalarSetRoundTrips(t *testing.T) {
	fd := fieldDesc[*mixinforprototestv1.Presence](t, "optional_string")
	got, err := reverseValue(fd, "optionalScalar", "hi")
	if err != nil {
		t.Fatalf("reverseValue: %v", err)
	}
	if !got.IsValid() || got.String() != "hi" {
		t.Fatalf("reverseValue = %#v, want a valid Value(\"hi\")", got.Interface())
	}
}

// --- Test 3/4: enum ------------------------------------------------------

func TestReverseValue_EnumValid(t *testing.T) {
	fd := fieldDesc[*mixinforprototestv1.ReverseEnum](t, "status")
	got, err := reverseValue(fd, "enum", "STATUS_ACTIVE")
	if err != nil {
		t.Fatalf("reverseValue: %v", err)
	}
	evd := fd.Enum().Values().ByName("STATUS_ACTIVE")
	if evd == nil {
		t.Fatal("descriptor is missing STATUS_ACTIVE — corpus regression")
	}
	if got.Enum() != evd.Number() {
		t.Fatalf("reverseValue = %v, want enum number %v", got.Enum(), evd.Number())
	}
}

func TestReverseValue_EnumUnknownString(t *testing.T) {
	fd := fieldDesc[*mixinforprototestv1.ReverseEnum](t, "status")
	_, err := reverseValue(fd, "enum", "STATUS_DOES_NOT_EXIST")
	notValidationError(t, err)
	if err == nil || !bytes.Contains([]byte(err.Error()), []byte("STATUS_DOES_NOT_EXIST")) {
		t.Fatalf("error must name the offending string, got: %v", err)
	}
}

// --- Test 5: Timestamp round trip ---------------------------------------

func TestReverseValue_TimestampRoundTrip(t *testing.T) {
	fd := fieldDesc[*mixinforprototestv1.ReverseWkt](t, "timestamp_field")
	want := time.Date(2026, 8, 14, 12, 30, 15, 123456789, time.UTC)

	got, err := reverseValue(fd, "wkt", want)
	if err != nil {
		t.Fatalf("reverseValue: %v", err)
	}
	if !got.IsValid() {
		t.Fatal("reverseValue returned an invalid Value")
	}
	ts, ok := got.Message().Interface().(*timestamppb.Timestamp)
	if !ok {
		t.Fatalf("reverseValue's Message().Interface() is %T, want *timestamppb.Timestamp", got.Message().Interface())
	}
	if ts.GetSeconds() != want.Unix() {
		t.Fatalf("seconds = %d, want %d", ts.GetSeconds(), want.Unix())
	}
	if ts.GetNanos() != int32(want.Nanosecond()) {
		t.Fatalf("nanos = %d, want %d", ts.GetNanos(), want.Nanosecond())
	}
}

func TestReverseValue_TimestampWrongGoType(t *testing.T) {
	fd := fieldDesc[*mixinforprototestv1.ReverseWkt](t, "timestamp_field")
	_, err := reverseValue(fd, "wkt", 12345)
	notValidationError(t, err)
}

// --- Test 6/7: scalarMap --------------------------------------------------

// serializeScalarMap converts in through reverseValue and marshals it into
// a fresh dynamicpb message of fd's containing message, isolating the
// serialized bytes from any state a previous call might have left behind.
// proto.MarshalOptions{Deterministic: true} is required here: protoreflect's
// generic Map implementation deliberately randomizes iteration order on
// every Range/marshal (google.golang.org/protobuf/internal/detrand — an
// intentional anti-reliance guard upstream), so insertion order alone
// (reverseScalarMap's own sorted-key insertion, D-24) does not by itself
// make wire-level serialization deterministic; MarshalOptions.Deterministic
// is the documented, sanctioned way to obtain a stable byte-for-byte
// encoding of the SAME map contents across repeated calls.
func serializeScalarMap(t *testing.T, fd protoreflect.FieldDescriptor, in any) []byte {
	t.Helper()
	v, err := reverseValue(fd, "scalarMap", in)
	if err != nil {
		t.Fatalf("reverseValue: %v", err)
	}
	msg := dynamicpb.NewMessage(fd.ContainingMessage())
	msg.Set(fd, v)
	b, err := proto.MarshalOptions{Deterministic: true}.Marshal(msg)
	if err != nil {
		t.Fatalf("proto.Marshal: %v", err)
	}
	return b
}

func TestReverseValue_ScalarMapDeterministicOrder(t *testing.T) {
	fd := fieldDesc[*mixinforprototestv1.ReverseScalarMap](t, "string_map")
	in := map[string]string{"zebra": "1", "apple": "2", "mango": "3", "banana": "4"}

	b1 := serializeScalarMap(t, fd, in)
	b2 := serializeScalarMap(t, fd, in)
	if !bytes.Equal(b1, b2) {
		t.Fatalf("two consecutive reverse conversions of the same scalarMap value produced different serialized output — entry order is non-deterministic")
	}
}

func TestReverseValue_ScalarMapIntKeyed(t *testing.T) {
	fd := fieldDesc[*mixinforprototestv1.ReverseScalarMap](t, "int_map")
	in := map[int32]int64{5: 50, 1: 10, 3: 30}

	v, err := reverseValue(fd, "scalarMap", in)
	if err != nil {
		t.Fatalf("reverseValue: %v", err)
	}
	m := v.Map()
	if m.Len() != len(in) {
		t.Fatalf("map has %d entries, want %d", m.Len(), len(in))
	}
	for k, want := range in {
		mk := protoreflect.ValueOfInt32(k).MapKey()
		if !m.Has(mk) {
			t.Fatalf("missing key %d", k)
		}
		if got := m.Get(mk).Int(); got != want {
			t.Fatalf("key %d = %d, want %d", k, got, want)
		}
	}
}

func TestReverseValue_ScalarMapEmpty(t *testing.T) {
	fd := fieldDesc[*mixinforprototestv1.ReverseScalarMap](t, "string_map")
	got, err := reverseValue(fd, "scalarMap", map[string]string{})
	if err != nil {
		t.Fatalf("reverseValue: unexpected error for an empty map: %v", err)
	}
	if !got.IsValid() {
		t.Fatal("reverseValue(empty map) returned an invalid Value — an empty map must convert to an empty protoreflect.Map, not report absent")
	}
	if got.Map().Len() != 0 {
		t.Fatalf("Map().Len() = %d, want 0", got.Map().Len())
	}
}

// --- Test 8: wrong dynamic type never panics ------------------------------

func TestReverseValue_WrongTypeNeverPanics(t *testing.T) {
	tests := []struct {
		name  string
		fd    protoreflect.FieldDescriptor
		class string
		value any
	}{
		{"wkt Timestamp given an int", fieldDesc[*mixinforprototestv1.ReverseWkt](t, "timestamp_field"), "wkt", 42},
		{"scalar string given an int", fieldDesc[*mixinforprototestv1.ReverseScalars](t, "string_field"), "scalar", 42},
		{"scalarMap given a non-map", fieldDesc[*mixinforprototestv1.ReverseScalarMap](t, "string_map"), "scalarMap", "not a map"},
		{"enum given a non-string", fieldDesc[*mixinforprototestv1.ReverseEnum](t, "status"), "enum", 42},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("reverseValue panicked: %v", r)
				}
			}()
			_, err := reverseValue(tc.fd, tc.class, tc.value)
			notValidationError(t, err)
		})
	}
}

// --- Unbindable classes/kinds: Task 2's leg is not implemented yet -------

func TestReverseValue_UnbindableClassesFailClosed(t *testing.T) {
	tests := []struct {
		name  string
		fd    protoreflect.FieldDescriptor
		class string
	}{
		{"asJSON", fieldDesc[*mixinforprototestv1.ReverseAsJSON](t, "payload"), "asJSON"},
		{"wkt Struct", fieldDesc[*mixinforprototestv1.ReverseWkt](t, "struct_field"), "wkt"},
		{"wkt Value", fieldDesc[*mixinforprototestv1.ReverseWkt](t, "value_field"), "wkt"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := reverseValue(tc.fd, tc.class, nil)
			notValidationError(t, err)
		})
	}
}

// --- Corpus golden coverage (PIPE-05/corpusCoverage) ----------------------

// TestReverseCorpusGolden golden-asserts the forward derivation of every
// new reverse.proto/constraints.proto corpus message this plan adds,
// giving each a corpusCoverage entry (fieldmap_test.go's assertGolden
// mechanism, D-23/R4) — reverse.go's own conversions are exercised
// directly by the TestReverseValue_* tests above; this test's job is only
// to prove these messages derive cleanly and to satisfy
// TestCorpusMessagesHaveRecordedCoverage.
func TestReverseCorpusGolden(t *testing.T) {
	t.Run("ReverseScalars", func(t *testing.T) {
		d := assertGolden(t, "reverse_scalars", derive[*mixinforprototestv1.ReverseScalars])
		if len(d.fields) != 15 {
			t.Fatalf("want 15 derived fields (one per scalar kind), got %d", len(d.fields))
		}
	})

	t.Run("ReverseEnum", func(t *testing.T) {
		d := assertGolden(t, "reverse_enum", derive[*mixinforprototestv1.ReverseEnum])
		if fieldByName(d, "status") == nil {
			t.Fatal("want a derived field named status")
		}
	})

	t.Run("ReverseWkt", func(t *testing.T) {
		d := assertGolden(t, "reverse_wkt", derive[*mixinforprototestv1.ReverseWkt])
		if len(d.fields) != 3 {
			t.Fatalf("want exactly 3 derived fields (Timestamp, Struct, Value), got %d", len(d.fields))
		}
	})

	t.Run("ReverseScalarMap", func(t *testing.T) {
		d := assertGolden(t, "reverse_scalar_map", derive[*mixinforprototestv1.ReverseScalarMap])
		if len(d.fields) != 2 {
			t.Fatalf("want exactly 2 derived fields, got %d", len(d.fields))
		}
	})

	t.Run("ReverseAsJSON", func(t *testing.T) {
		d := assertGolden(t, "reverse_as_json", derive[*mixinforprototestv1.ReverseAsJSON], AsJSON("payload"))
		got := fieldByName(d, "payload")
		if got == nil || got.TypeKind != "json.RawMessage" {
			t.Fatalf("want payload to derive as a JSON field, got %+v", got)
		}
	})
}

// TestMixedFieldRulesGolden golden-asserts constraints.proto's
// MixedFieldRules fixture (plan 03-03's D-07/D-08 convergence fixture),
// giving it a corpusCoverage entry. This plan only needs it to derive
// cleanly; the hybrid-evaluator convergence behavior it exists to prove
// is plan 03-03's job.
func TestMixedFieldRulesGolden(t *testing.T) {
	d := assertGolden(t, "constraints_mixed_field_rules", derive[*mixinforprototestv1.MixedFieldRules])
	if len(d.fields) != 3 {
		t.Fatalf("want exactly 3 derived fields (both, standard_only, cel_only), got %d", len(d.fields))
	}
}
