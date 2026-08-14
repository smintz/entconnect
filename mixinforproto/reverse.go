package mixinforproto

import (
	"fmt"

	"google.golang.org/protobuf/reflect/protoreflect"
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
// This plan (03-01) implements only the "scalar"/"optionalScalar" +
// StringKind leg: the one case the tracer's ResidualCel.value field
// needs. Every other derivation class returns a named, non-protovalidate
// error rather than silently mis-converting — later plans in this phase
// extend the switch one derivation class at a time, each with its own
// test, per 03-CONTEXT.md's explicit call-out that this leg is a real
// risk concentration.

// reverseValue converts entValue — an ent mutation's Go-typed field
// value, as returned by ent.Mutation.Field(name) — back into a
// protoreflect.Value against fd, using class (a SourceField.Kind
// derivation-class string: "scalar", "optionalScalar", "enum", "wkt",
// "scalarMap", "asJSON") to select the right conversion, since the mixin
// hook has only the mutation's Go value and the field's own descriptor
// to work from — no fresh classify() pass over a live proto message.
//
// A conversion failure here is a D-12 data-integrity fault, not a
// protovalidate violation: it is always a plain, non-*protovalidate.
// ValidationError error naming the field and derivation class, never
// wrapped as a violation and never assigned a synthesized RuleId. D-12's
// rationale: reporting a reverse-conversion failure as a validation
// failure would put a fabricated constraint ID on the wire and corrupt
// PIPE-06's differential signal; this must fail the mutation closed
// while staying visibly a different kind of failure from a real
// constraint rejection.
func reverseValue(fd protoreflect.FieldDescriptor, class string, entValue any) (protoreflect.Value, error) {
	switch class {
	case "scalar", "optionalScalar":
		return reverseScalar(fd, class, entValue)
	default:
		return protoreflect.Value{}, fmt.Errorf(
			"mixinforproto: reverse-converting field %q (kind %s): derivation class %q has no reverse conversion yet",
			fd.Name(), fd.Kind(), class,
		)
	}
}

// reverseScalar handles the "scalar"/"optionalScalar" derivation classes.
// Only StringKind is implemented in this plan (03-01's tracer needs no
// other scalar kind); every other kind returns the same named,
// non-protovalidate D-12 error the default case above returns, rather
// than silently falling through to it — an explicit switch case that
// simply isn't populated yet is not the same guarantee as one that was
// never reached.
func reverseScalar(fd protoreflect.FieldDescriptor, class string, entValue any) (protoreflect.Value, error) {
	switch fd.Kind() {
	case protoreflect.StringKind:
		s, ok := entValue.(string)
		if !ok {
			return protoreflect.Value{}, fmt.Errorf(
				"mixinforproto: reverse-converting field %q (kind %s, class %s): expected a Go string, got %T",
				fd.Name(), fd.Kind(), class, entValue,
			)
		}
		return protoreflect.ValueOfString(s), nil
	default:
		return protoreflect.Value{}, fmt.Errorf(
			"mixinforproto: reverse-converting field %q (kind %s, class %s): this proto kind has no reverse conversion yet",
			fd.Name(), fd.Kind(), class,
		)
	}
}
