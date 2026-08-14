package mixinforproto

import (
	"fmt"
	"reflect"
	"sort"
	"time"

	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// This file is the mirror of fieldmap.go's forward derivation table
// (D-05): fieldmap.go converts a proto field descriptor into an ent.Field
// builder call at schema-load time; reverse.go converts an already-set
// ent mutation value back into a protoreflect.Value at mutation time, so
// hooks.go can hand it to protovalidate's own evaluator or to a local
// cel.Env unchanged. A change to one direction's Kind coverage is
// visibly a change the other direction must answer — keep both files'
// per-Kind switches in sync.
//
// 03-01 implemented only the "scalar"/"optionalScalar" + StringKind leg.
// 03-02 (this plan) extends the table with a full leg per derivation
// class: an exhaustive scalar/optionalScalar Kind switch, "enum", the
// google.protobuf.Timestamp branch of "wkt", and "scalarMap". The
// google.protobuf.Struct/Value branches of "wkt" and the "asJSON" class
// are Task 2's JSON-to-dynamicpb leg — until that task lands, calling
// reverseValue for them returns the same named, non-protovalidate
// unbindable-kind error every other not-yet-implemented class returns,
// never a silent mis-conversion.
//
// Every failure this file produces is a plain D-12 data-integrity fault,
// never a *protovalidate.ValidationError (that would fabricate a
// constraint ID the mutation never actually violated) and never a
// failure/derivationError (those are schema-load-time only). The
// three-way split this file's functions all honor:
//   - success: a valid protoreflect.Value, nil error.
//   - a genuine constraint violation: produced elsewhere (hooks.go's
//     evaluators), never here — this file never returns a *validate.
//     Violation or anything that could become one.
//   - an infrastructure/data-integrity fault: an invalid protoreflect.
//     Value{} (IsValid() == false) plus a non-nil plain error, naming
//     the field and the derivation class/kind. Collapsing this third
//     case into the second would put a fabricated constraint ID on the
//     wire and corrupt PIPE-06's differential signal (D-12).
//
// One exception to "invalid Value + non-nil error means failure": an
// optionalScalar field whose ent value is Go nil (unset) reports as
// absent via (protoreflect.Value{}, nil) — no error, because absence is
// not a fault, but also no fabricated proto3 zero. Callers must check
// val.IsValid() before using a nil-error result.

// reverseValue converts entValue — an ent mutation's Go-typed field
// value, as returned by ent.Mutation.Field(name) — back into a
// protoreflect.Value against fd, using class (a SourceField.Kind
// derivation-class string: "scalar", "optionalScalar", "enum", "wkt",
// "scalarMap", "asJSON") to select the right conversion, since the mixin
// hook has only the mutation's Go value and the field's own descriptor
// to work from — no fresh classify() pass over a live proto message.
func reverseValue(fd protoreflect.FieldDescriptor, class string, entValue any) (protoreflect.Value, error) {
	switch class {
	case "scalar":
		return reverseScalarField(fd, entValue)
	case "optionalScalar":
		if entValue == nil {
			// D-06/Test 2: an unset optional field is reported as
			// absent, never converted to a fabricated proto3 zero. This
			// is not a fault — no error.
			return protoreflect.Value{}, nil
		}
		return reverseScalarField(fd, entValue)
	case "enum":
		return reverseEnum(fd, entValue)
	case "wkt":
		return reverseWkt(fd, entValue)
	case "scalarMap":
		return reverseScalarMap(fd, entValue)
	case "asJSON":
		// Task 2's JSON-to-dynamicpb leg.
		return unbindableClass(fd, class)
	default:
		return unbindableClass(fd, class)
	}
}

// unbindableClass is the shared D-12 fail-closed sentinel for a
// derivation class this file has no conversion for yet — an explicit
// switch case that simply is not populated yet, not a silent fallthrough
// (see this file's own doc comment).
func unbindableClass(fd protoreflect.FieldDescriptor, class string) (protoreflect.Value, error) {
	return protoreflect.Value{}, fmt.Errorf(
		"mixinforproto: reverse-converting field %q (kind %s): derivation class %q has no reverse conversion yet",
		fd.Name(), fd.Kind(), class,
	)
}

// unbindableKind is the WKT-specific sibling of unbindableClass, used
// when fd's message type is a well-known type this file recognizes by
// name but deliberately never binds — google.protobuf.FieldMask and
// google.protobuf.Duration, per mapWellKnownType (fieldmap.go), derive no
// ent field at all (handled=true, f=nil), so this branch documents that
// this leg can never actually be reached for them; it exists so an
// unrecognized or deliberately-unhandled WKT full name still fails
// closed rather than falling through silently.
func unbindableKind(fd protoreflect.FieldDescriptor, reason string) (protoreflect.Value, error) {
	return protoreflect.Value{}, fmt.Errorf(
		"mixinforproto: reverse-converting field %q (kind %s): %s",
		fd.Name(), fd.Kind(), reason,
	)
}

// reverseScalarField handles the "scalar"/"optionalScalar" derivation
// classes' non-map case: an exhaustive protoreflect.Kind switch with no
// widening between same-width variants (matching buildScalarField's own
// no-widening discipline in fieldmap.go).
func reverseScalarField(fd protoreflect.FieldDescriptor, entValue any) (protoreflect.Value, error) {
	v, err := reverseScalarByKind(fd.Kind(), entValue)
	if err != nil {
		return protoreflect.Value{}, fmt.Errorf(
			"mixinforproto: reverse-converting field %q (kind %s): %w",
			fd.Name(), fd.Kind(), err,
		)
	}
	return v, nil
}

// reverseScalarByKind is the exhaustive proto-scalar-kind switch shared
// between reverseScalarField (a plain top-level scalar/optionalScalar
// field) and reverseScalarMap's per-entry key/value conversion — both
// ultimately convert one Go scalar value against one protoreflect.Kind.
// Errors returned here carry no field context; callers wrap with the
// field name and derivation class.
func reverseScalarByKind(kind protoreflect.Kind, v any) (protoreflect.Value, error) {
	switch kind {
	case protoreflect.BoolKind:
		b, ok := v.(bool)
		if !ok {
			return protoreflect.Value{}, fmt.Errorf("expected a Go bool, got %T", v)
		}
		return protoreflect.ValueOfBool(b), nil
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		i, ok := v.(int32)
		if !ok {
			return protoreflect.Value{}, fmt.Errorf("expected a Go int32, got %T", v)
		}
		return protoreflect.ValueOfInt32(i), nil
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		i, ok := v.(int64)
		if !ok {
			return protoreflect.Value{}, fmt.Errorf("expected a Go int64, got %T", v)
		}
		return protoreflect.ValueOfInt64(i), nil
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		u, ok := v.(uint32)
		if !ok {
			return protoreflect.Value{}, fmt.Errorf("expected a Go uint32, got %T", v)
		}
		return protoreflect.ValueOfUint32(u), nil
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		u, ok := v.(uint64)
		if !ok {
			return protoreflect.Value{}, fmt.Errorf("expected a Go uint64, got %T", v)
		}
		return protoreflect.ValueOfUint64(u), nil
	case protoreflect.FloatKind:
		f, ok := v.(float32)
		if !ok {
			return protoreflect.Value{}, fmt.Errorf("expected a Go float32, got %T", v)
		}
		return protoreflect.ValueOfFloat32(f), nil
	case protoreflect.DoubleKind:
		f, ok := v.(float64)
		if !ok {
			return protoreflect.Value{}, fmt.Errorf("expected a Go float64, got %T", v)
		}
		return protoreflect.ValueOfFloat64(f), nil
	case protoreflect.StringKind:
		s, ok := v.(string)
		if !ok {
			return protoreflect.Value{}, fmt.Errorf("expected a Go string, got %T", v)
		}
		return protoreflect.ValueOfString(s), nil
	case protoreflect.BytesKind:
		b, ok := v.([]byte)
		if !ok {
			return protoreflect.Value{}, fmt.Errorf("expected a Go []byte, got %T", v)
		}
		return protoreflect.ValueOfBytes(b), nil
	default:
		return protoreflect.Value{}, fmt.Errorf("this proto kind has no reverse conversion")
	}
}

// reverseEnum handles the "enum" derivation class: mapEnum's ent value is
// always a Go string naming a declared enum value (fieldmap.go). D-12: an
// ent string absent from the descriptor's declared value names is a
// data-integrity fault, not a protovalidate violation — no
// enum.defined_only-shaped constraint ID is synthesized for it.
func reverseEnum(fd protoreflect.FieldDescriptor, entValue any) (protoreflect.Value, error) {
	name, ok := entValue.(string)
	if !ok {
		return protoreflect.Value{}, fmt.Errorf(
			"mixinforproto: reverse-converting field %q (kind %s, class enum): expected a Go string, got %T",
			fd.Name(), fd.Kind(), entValue,
		)
	}
	evd := fd.Enum().Values().ByName(protoreflect.Name(name))
	if evd == nil {
		return protoreflect.Value{}, fmt.Errorf(
			"mixinforproto: reverse-converting field %q (kind %s, class enum): enum string %q not in descriptor",
			fd.Name(), fd.Kind(), name,
		)
	}
	return protoreflect.ValueOfEnum(evd.Number()), nil
}

// reverseWkt handles the "wkt" derivation class, dispatching on fd's
// message full name exactly as mapWellKnownType (fieldmap.go) does at
// derivation time. Only google.protobuf.Timestamp is implemented in this
// plan's first task; Struct and Value are Task 2's JSON-to-dynamicpb leg.
// FieldMask and Duration are named explicitly, even though mapWellKnownType
// never derives an ent field for them (so this branch can never actually
// be reached in practice) — the explicit case documents that fact rather
// than relying on the default branch to accidentally cover it.
func reverseWkt(fd protoreflect.FieldDescriptor, entValue any) (protoreflect.Value, error) {
	switch string(fd.Message().FullName()) {
	case wktTimestamp:
		t, ok := entValue.(time.Time)
		if !ok {
			return protoreflect.Value{}, fmt.Errorf(
				"mixinforproto: reverse-converting field %q (kind %s, class wkt): expected a Go time.Time, got %T",
				fd.Name(), fd.Kind(), entValue,
			)
		}
		return protoreflect.ValueOfMessage(timestamppb.New(t).ProtoReflect()), nil
	case wktStruct, wktValue:
		// Task 2's JSON-to-dynamicpb leg.
		return unbindableClass(fd, "wkt")
	case wktFieldMask, wktDuration:
		return unbindableKind(fd, "google.protobuf.FieldMask/Duration derive no ent field and can never reach the reverse table")
	default:
		return unbindableClass(fd, "wkt")
	}
}

// reverseScalarMap handles the "scalarMap" derivation class: mapScalarMap's
// ent value is always a native Go map[K]V (fieldmap.go's goScalarType),
// never JSON bytes. Entries are written into the returned protoreflect.Map
// in SORTED key order (Phase 1 D-24: never range a Go map into ordered
// output), so two consecutive conversions of the same value produce
// byte-identical serialized output.
func reverseScalarMap(fd protoreflect.FieldDescriptor, entValue any) (protoreflect.Value, error) {
	rv := reflect.ValueOf(entValue)
	if entValue == nil || rv.Kind() != reflect.Map {
		return protoreflect.Value{}, fmt.Errorf(
			"mixinforproto: reverse-converting field %q (kind %s, class scalarMap): expected a Go map, got %T",
			fd.Name(), fd.Kind(), entValue,
		)
	}

	dyn := dynamicpb.NewMessage(fd.ContainingMessage())
	mapVal := dyn.NewField(fd)
	m := mapVal.Map()

	keyKind := fd.MapKey().Kind()
	valKind := fd.MapValue().Kind()

	for _, k := range sortedMapKeys(rv) {
		keyVal, err := reverseScalarByKind(keyKind, k.Interface())
		if err != nil {
			return protoreflect.Value{}, fmt.Errorf(
				"mixinforproto: reverse-converting field %q (kind %s, class scalarMap): key: %w",
				fd.Name(), fd.Kind(), err,
			)
		}
		entryVal, err := reverseScalarByKind(valKind, rv.MapIndex(k).Interface())
		if err != nil {
			return protoreflect.Value{}, fmt.Errorf(
				"mixinforproto: reverse-converting field %q (kind %s, class scalarMap): value: %w",
				fd.Name(), fd.Kind(), err,
			)
		}
		m.Set(keyVal.MapKey(), entryVal)
	}

	// Assign the populated map back onto its originating message,
	// following protoreflect.Message.NewField's documented contract
	// ("once populated, the value must be assigned to the field using
	// Message.Set") — dynamicpb's Map implementation happens to mutate
	// through the same pointer NewField returned, but relying on that
	// implementation detail instead of the documented contract would be
	// exactly the kind of undocumented-behavior dependency this
	// package's own D-24/D-12 discipline avoids elsewhere.
	dyn.Set(fd, mapVal)

	return mapVal, nil
}

// sortedMapKeys returns rv's (a reflect.Value of Kind Map) keys sorted
// deterministically. Proto map key kinds are restricted to strings,
// booleans, and integral types (never floats, never bytes), so a
// kind-aware numeric/lexical comparison covers every real case; a
// fmt.Sprint fallback keeps this function total rather than panicking on
// a key kind reverseScalarByKind itself would reject anyway.
func sortedMapKeys(rv reflect.Value) []reflect.Value {
	keys := rv.MapKeys()
	sort.Slice(keys, func(i, j int) bool {
		return mapKeyLess(keys[i], keys[j])
	})
	return keys
}

func mapKeyLess(a, b reflect.Value) bool {
	switch a.Kind() {
	case reflect.String:
		return a.String() < b.String()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return a.Int() < b.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return a.Uint() < b.Uint()
	case reflect.Bool:
		return !a.Bool() && b.Bool()
	default:
		return fmt.Sprint(a.Interface()) < fmt.Sprint(b.Interface())
	}
}
