package mixinforproto

import (
	"fmt"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"google.golang.org/protobuf/reflect/protoreflect"

	"buf.build/go/protovalidate"
)

// mapField translates one proto field descriptor into a derived
// ent.Field, or a self-sufficient error naming the message, the field,
// what is unsupported, and the fix (D-08, MIX-11).
//
// Phase 1's walking skeleton implements only the protoreflect.StringKind
// case — one proto field becomes one annotated ent field, end to end.
// Every other kind is a functionality gap Plan 02 fills in, not an
// architectural one: it fails loudly at schema-load time with a named
// remedy, never silently guesses a mapping or drops the field.
func mapField(msgName string, fd protoreflect.FieldDescriptor) (ent.Field, error) {
	name := string(fd.Name())

	switch fd.Kind() {
	case protoreflect.StringKind:
		// ResolveFieldRules returns (nil, nil) for a constraint-free
		// field — verified, and the single most common case there is.
		// Tier 1 translation (turning resolved rules into TranslatedIDs
		// / ResidualIDs) is Plan 04's job; this plan's tracer field
		// carries no protovalidate constraint, so rules is always nil
		// here. The guard exists so the nil is handled, never
		// dereferenced, from the first line this call appears.
		rules, err := protovalidate.ResolveFieldRules(fd)
		if err != nil {
			return nil, fmt.Errorf("mixinforproto: %s.%s: resolving protovalidate field rules: %w", msgName, name, err)
		}
		translatedIDs, residualIDs := []string{}, []string{}
		if rules != nil {
			// Non-nil here means a real constraint exists that Tier 1
			// (Plan 04) has not been written yet to translate. Recording
			// nothing here is intentional for the walking skeleton, not
			// a silent drop: SourceField's constraint-ID lists simply
			// stay empty until Plan 04 exists to fill them.
			_ = rules
		}
		return field.String(name).
			Annotations(SourceField{
				ContractVersion: ContractVersion,
				FieldName:       name,
				Number:          int32(fd.Number()),
				Kind:            fd.Kind().String(),
				TranslatedIDs:   translatedIDs,
				ResidualIDs:     residualIDs,
			}), nil
	default:
		return nil, fmt.Errorf(
			"mixinforproto: %s.%s: unsupported field kind %q in this slice — this mapping rule lands in a later plan; use Exclude(%q) or Override(%q, ...) for now",
			msgName, name, fd.Kind(), name, name,
		)
	}
}
