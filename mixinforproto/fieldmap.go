package mixinforproto

import (
	"encoding/json"
	"fmt"
	"reflect"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"google.golang.org/protobuf/reflect/protoreflect"

	"buf.build/go/protovalidate"
)

// fieldClass is the classification a proto field descriptor is sorted
// into before mapping. classify's branch order is load-bearing (see
// classify's doc comment) — getting it wrong is precisely what would
// make every proto3 `optional` scalar panic, per 01-RESEARCH.md's
// synthetic-oneof finding.
type fieldClass int

const (
	classScalarMap fieldClass = iota
	classMessageMap
	classRealOneofMember
	classOptionalScalar
	classMessageField
	classEnum
	classScalar
)

// classify sorts fd into exactly one fieldClass, in the exact order
// 01-RESEARCH.md verified: IsMap() first; then real-oneof membership
// (ContainingOneof() != nil && !ContainingOneof().IsSynthetic()); then
// HasOptionalKeyword(); then MessageKind; then EnumKind; then plain
// scalar. The order matters because the branches overlap: a proto3
// `optional` scalar is wrapped by the compiler in a synthetic
// one-member oneof, so ContainingOneof() is non-nil for it too — only
// IsSynthetic() (checked as part of the real-oneof branch, ahead of the
// optional-keyword branch) separates MIX-05 from MIX-10. A field cannot
// satisfy both.
func classify(fd protoreflect.FieldDescriptor) fieldClass {
	switch {
	case fd.IsMap():
		if fd.MapValue().Kind() == protoreflect.MessageKind {
			return classMessageMap
		}
		return classScalarMap
	case fd.ContainingOneof() != nil && !fd.ContainingOneof().IsSynthetic():
		return classRealOneofMember
	case fd.HasOptionalKeyword():
		return classOptionalScalar
	case fd.Kind() == protoreflect.MessageKind:
		return classMessageField
	case fd.Kind() == protoreflect.EnumKind:
		return classEnum
	default:
		return classScalar
	}
}

// Well-known-type full names (MIX-04). These are protobuf's own fixed,
// unversioned type names — identification is a string comparison
// against fd.Message().FullName(), the standard protobuf-go idiom.
const (
	wktTimestamp = "google.protobuf.Timestamp"
	wktStruct    = "google.protobuf.Struct"
	wktValue     = "google.protobuf.Value"
	wktFieldMask = "google.protobuf.FieldMask"
	wktDuration  = "google.protobuf.Duration"
)

// mapField translates one proto field descriptor into a derived
// ent.Field, or a self-sufficient error naming the message, the field,
// what is unsupported, and the fix (D-08, MIX-11). A nil, nil return
// means the field is deliberately skipped (a real-oneof member left
// unresolved by this plan, a message-valued map, an un-opted-in message
// field, or a skipped well-known type) — not a silent failure.
//
// o carries the effective Option set for this derivation (Exclude,
// Override, AsJSON) so AsJSON's message-field opt-in (MIX-09) can be
// checked per field; derive.go's existing o variable is threaded
// through the one call site unchanged in every other respect.
func mapField(msgName string, fd protoreflect.FieldDescriptor, o *options) (ent.Field, error) {
	name := string(fd.Name())

	switch classify(fd) {
	case classScalarMap:
		return mapScalarMap(fd), nil
	case classMessageMap:
		// MIX-06: message-valued maps are skipped, not an error.
		return nil, nil
	case classRealOneofMember:
		// MIX-10: this plan classifies real-oneof members only. It
		// produces no field and records no error for them — the
		// panic-unless-all-resolved gate over a oneof's members is
		// Plan 03's job and lands in derive.go, not here.
		return nil, nil
	case classOptionalScalar:
		return mapOptionalScalar(msgName, name, fd)
	case classMessageField:
		return mapMessageField(fd, o)
	case classEnum:
		return mapEnum(fd), nil
	default: // classScalar
		return mapScalar(msgName, name, fd)
	}
}

// sourceFieldFor builds the SourceField annotation every derived field
// carries (D-02/ANNO-02), with Kind set to the derivation class name
// ("scalar" / "optionalScalar" / "enum" / "wkt" / "scalarMap" /
// "asJSON") rather than the raw protoreflect.Kind string — this is the
// "derivation kind (scalar / enum / WKT / map-as-JSON / message-as-JSON
// / override)" D-02 documents. TranslatedIDs/ResidualIDs stay empty:
// Tier 1 translation is Plan 04's job.
func sourceFieldFor(fd protoreflect.FieldDescriptor, class string) SourceField {
	return SourceField{
		ContractVersion: ContractVersion,
		FieldName:       string(fd.Name()),
		Number:          int32(fd.Number()),
		Kind:            class,
		TranslatedIDs:   []string{},
		ResidualIDs:     []string{},
	}
}

// mapScalar handles MIX-02 (the exhaustive proto-scalar-kind switch) and
// MIX-05's non-optional branch: a plain proto3 scalar gets
// .Default(<Go zero of its type>) and stays non-optional, matching the
// wire's own collapse of unset and zero (D-26 documents the
// consequence). No widening: each kind maps to its same-width ent
// builder, including the sint/fixed/sfixed variants of the same width.
func mapScalar(msgName, name string, fd protoreflect.FieldDescriptor) (ent.Field, error) {
	sf := sourceFieldFor(fd, "scalar")

	switch fd.Kind() {
	case protoreflect.DoubleKind:
		return field.Float(name).Default(0).Annotations(sf), nil
	case protoreflect.FloatKind:
		return field.Float32(name).Default(0).Annotations(sf), nil
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		return field.Int32(name).Default(0).Annotations(sf), nil
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		return field.Int64(name).Default(0).Annotations(sf), nil
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return field.Uint32(name).Default(0).Annotations(sf), nil
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return field.Uint64(name).Default(0).Annotations(sf), nil
	case protoreflect.BoolKind:
		return field.Bool(name).Default(false).Annotations(sf), nil
	case protoreflect.StringKind:
		// ResolveFieldRules returns (nil, nil) for a constraint-free
		// field — verified (Plan 01). Tier 1 translation (turning
		// resolved rules into TranslatedIDs/ResidualIDs) is Plan 04's
		// job; this corpus carries no protovalidate constraints, so
		// rules is always nil here. The guard exists so the nil is
		// handled, never dereferenced, and buf.build/go/protovalidate
		// stays a genuine, actually-imported direct dependency (Plan
		// 01 deviation 2).
		rules, err := protovalidate.ResolveFieldRules(fd)
		if err != nil {
			return nil, fmt.Errorf("mixinforproto: %s.%s: resolving protovalidate field rules: %w", msgName, name, err)
		}
		_ = rules
		return field.String(name).Default("").Annotations(sf), nil
	case protoreflect.BytesKind:
		return field.Bytes(name).Default(nil).Annotations(sf), nil
	default:
		return nil, fmt.Errorf(
			"mixinforproto: %s.%s: unsupported field kind %q — this mapping rule lands in a later plan; use Exclude(%q) or Override(%q, ...) for now",
			msgName, name, fd.Kind(), name, name,
		)
	}
}

// mapOptionalScalar handles MIX-05's optional-keyword branch: a proto3
// `optional` scalar gets .Nillable().Optional() and no default —
// HasOptionalKeyword() being true here (verified by classify's caller,
// classOptionalScalar) is exactly the presence signal that means the
// wire distinguishes unset from zero, so nothing may be collapsed.
func mapOptionalScalar(msgName, name string, fd protoreflect.FieldDescriptor) (ent.Field, error) {
	sf := sourceFieldFor(fd, "optionalScalar")

	switch fd.Kind() {
	case protoreflect.DoubleKind:
		return field.Float(name).Nillable().Optional().Annotations(sf), nil
	case protoreflect.FloatKind:
		return field.Float32(name).Nillable().Optional().Annotations(sf), nil
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		return field.Int32(name).Nillable().Optional().Annotations(sf), nil
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		return field.Int64(name).Nillable().Optional().Annotations(sf), nil
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return field.Uint32(name).Nillable().Optional().Annotations(sf), nil
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return field.Uint64(name).Nillable().Optional().Annotations(sf), nil
	case protoreflect.BoolKind:
		return field.Bool(name).Nillable().Optional().Annotations(sf), nil
	case protoreflect.StringKind:
		return field.String(name).Nillable().Optional().Annotations(sf), nil
	case protoreflect.BytesKind:
		return field.Bytes(name).Nillable().Optional().Annotations(sf), nil
	default:
		return nil, fmt.Errorf(
			"mixinforproto: %s.%s: unsupported optional field kind %q — this mapping rule lands in a later plan; use Exclude(%q) or Override(%q, ...) for now",
			msgName, name, fd.Kind(), name, name,
		)
	}
}

// mapEnum handles MIX-03: field.Enum(name).Values(...) built by
// iterating EnumDescriptor.Values() by index from 0 to Len()-1 and
// taking each .Name() — never via an intermediate map (D-24). The
// declared value names are carried verbatim, with no case
// transformation and no stripping of a leading ENUM_NAME_ prefix
// (Flagged Assumption MIX-03). enum.defined_only is satisfied by this
// construction itself and needs no separate residual record (D-14).
func mapEnum(fd protoreflect.FieldDescriptor) ent.Field {
	name := string(fd.Name())
	ed := fd.Enum()
	vals := ed.Values()
	names := make([]string, 0, vals.Len())
	for i := 0; i < vals.Len(); i++ {
		names = append(names, string(vals.Get(i).Name()))
	}
	sf := sourceFieldFor(fd, "enum")
	return field.Enum(name).Values(names...).Annotations(sf)
}

// mapScalarMap handles MIX-06's scalar-valued-map branch: one JSON
// field typed as a Go map of the mapped key and value Go types. Map
// constraints are recorded as residual, not enforced, in Phase 1
// (Flagged Assumption MIX-06) — Phase 3's concern, not this plan's.
func mapScalarMap(fd protoreflect.FieldDescriptor) ent.Field {
	name := string(fd.Name())
	sf := sourceFieldFor(fd, "scalarMap")
	typ := reflect.MakeMap(reflect.MapOf(goScalarType(fd.MapKey().Kind()), goScalarType(fd.MapValue().Kind()))).Interface()
	return field.JSON(name, typ).Annotations(sf)
}

// goScalarType returns the concrete Go type a protoreflect scalar Kind
// decodes to, for building a map[K]V typ argument to field.JSON. Only
// scalar kinds ever reach here: mapScalarMap is only called for map
// fields whose value kind is not MessageKind (classify already routed
// message-valued maps to classMessageMap, skipped before this is
// reached).
func goScalarType(k protoreflect.Kind) reflect.Type {
	switch k {
	case protoreflect.StringKind:
		return reflect.TypeOf("")
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		return reflect.TypeOf(int32(0))
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		return reflect.TypeOf(int64(0))
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return reflect.TypeOf(uint32(0))
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return reflect.TypeOf(uint64(0))
	case protoreflect.BoolKind:
		return reflect.TypeOf(false)
	case protoreflect.DoubleKind:
		return reflect.TypeOf(float64(0))
	case protoreflect.FloatKind:
		return reflect.TypeOf(float32(0))
	case protoreflect.BytesKind:
		return reflect.TypeOf([]byte(nil))
	default:
		return reflect.TypeOf((*any)(nil)).Elem()
	}
}

// mapWellKnownType handles MIX-04: it returns handled=true if fd's
// message type is a recognized well-known type, whether that WKT maps
// to a field (Timestamp, Struct, Value) or is deliberately skipped
// (FieldMask, Duration — MIX2-04's Duration-to-nanoseconds mapping is
// v2). handled=false means fd is an ordinary (non-WKT) message field
// the caller must route through the message-field/AsJSON path instead.
//
// google.protobuf.Value maps to field.JSON typed json.RawMessage,
// chosen because it round-trips any JSON value losslessly where a
// concrete Go type would not (RESEARCH.md Open Question 2, closed by
// this plan). google.protobuf.Struct maps to field.JSON typed
// map[string]any, matching its own natural JSON shape.
func mapWellKnownType(fd protoreflect.FieldDescriptor) (f ent.Field, handled bool, err error) {
	if fd.Kind() != protoreflect.MessageKind || fd.IsList() {
		return nil, false, nil
	}
	name := string(fd.Name())
	sf := sourceFieldFor(fd, "wkt")
	switch string(fd.Message().FullName()) {
	case wktTimestamp:
		return field.Time(name).Annotations(sf), true, nil
	case wktStruct:
		return field.JSON(name, map[string]any{}).Annotations(sf), true, nil
	case wktValue:
		return field.JSON(name, json.RawMessage(nil)).Annotations(sf), true, nil
	case wktFieldMask, wktDuration:
		return nil, true, nil
	default:
		return nil, false, nil
	}
}

// mapAsJSON handles MIX-09's opt-in: a message field named in AsJSON(...)
// maps to a JSON field typed json.RawMessage, the same lossless
// representation used for google.protobuf.Value.
func mapAsJSON(fd protoreflect.FieldDescriptor) ent.Field {
	name := string(fd.Name())
	sf := sourceFieldFor(fd, "asJSON")
	return field.JSON(name, json.RawMessage(nil)).Annotations(sf)
}

// mapMessageField handles MIX-09's default path for any message-typed
// field that classify already confirmed is not a map: well-known types
// are mapped or skipped per mapWellKnownType; anything else is skipped
// by default unless the developer opted it in with AsJSON.
func mapMessageField(fd protoreflect.FieldDescriptor, o *options) (ent.Field, error) {
	if f, handled, err := mapWellKnownType(fd); handled {
		return f, err
	}
	if o.isAsJSON(string(fd.Name())) {
		return mapAsJSON(fd), nil
	}
	return nil, nil
}

// validateAsJSON checks every name passed to AsJSON(...) against md's
// real field set (MIX-09's failure surface): AsJSON("") and AsJSON
// naming a field that does not exist on the message each fail at schema
// load naming the message and the offending option; AsJSON naming a
// scalar (non-message) field also fails rather than silently succeeding.
// Errors are collected, not returned at the first offense (D-09), and
// names are visited in sorted order (o.asJSONNames(), D-24) so error
// output is diffable.
func validateAsJSON(msgName string, md protoreflect.MessageDescriptor, o *options) []string {
	var errs []string
	for _, name := range o.asJSONNames() {
		if name == "" {
			errs = append(errs, fmt.Sprintf(
				"mixinforproto: %s: AsJSON(\"\"): field name must not be empty — did you mean to name the message field you want serialized as JSON?",
				msgName,
			))
			continue
		}
		fd := md.Fields().ByName(protoreflect.Name(name))
		if fd == nil {
			errs = append(errs, fmt.Sprintf(
				"mixinforproto: %s.%s: AsJSON(%q) names a field that does not exist on this message — check for a typo or a renamed contract field",
				msgName, name, name,
			))
			continue
		}
		if fd.Kind() != protoreflect.MessageKind {
			errs = append(errs, fmt.Sprintf(
				"mixinforproto: %s.%s: AsJSON(%q) names a non-message field (kind %s) — AsJSON only applies to message-typed fields",
				msgName, name, name, fd.Kind(),
			))
		}
	}
	return errs
}
