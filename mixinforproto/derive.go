package mixinforproto

import (
	"fmt"

	"entgo.io/ent"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// derivation is the result of a successful derive[M] call: the ordered
// []ent.Field ready to hand to ent.Mixin.Fields(), plus the assembled
// SourceMessage annotation ready to hand to ent.Mixin.Annotations().
type derivation struct {
	fields  []ent.Field
	message SourceMessage
}

// derive walks the proto message descriptor for M and produces the
// derived ent fields plus the schema-level provenance annotation. It is
// the pure, testable core shared by both the panicking ent.Mixin.Fields()
// adapter (mixin.go, D-06) and the error-returning Validate[M] debug
// entry point (D-07) — Validate[M] calls this directly and never touches
// entc.LoadGraph or anything under entc/load, so it never enters entc's
// gorun() subprocess (R2/MIX-12).
//
// derive holds no package-level mutable state: every call builds its own
// options, descriptor walk, and result from scratch, so concurrent calls
// deriving the same or different message types never share memory
// (MIX-01 concurrency edge).
func derive[M proto.Message](opts ...Option) (*derivation, error) {
	o := applyOptions(opts)

	// (*new(M)).ProtoReflect() is safe on a nil generated pointer —
	// M's zero value is a nil *T, and ProtoReflect() is documented safe
	// on that (verified, see 01-RESEARCH.md).
	md := (*new(M)).ProtoReflect().Descriptor()
	msgName := string(md.FullName())

	// Every name supplied to Exclude/Override/AsJSON is validated
	// against the descriptor's own field inventory before the walk
	// below touches a single field (this plan's Task 1 action text):
	// an unknown name, a nil Override replacement, and an
	// Exclude/Override conflict are all collected here, not discovered
	// mid-walk.
	var failures []failure
	failures = append(failures, validateOptionNames(msgName, md, o)...)

	fds := md.Fields()
	// D-24: iterate protoreflect's field list by index, in declaration
	// order — never range a Go map into ordered output. This applies
	// from this first line: the walk below is the single source of
	// truth for both the derived field slice and the annotation
	// inventory, so they can never disagree on order.
	inventory := make([]FieldRef, 0, fds.Len())
	fields := make([]ent.Field, 0, fds.Len())

	for i := 0; i < fds.Len(); i++ {
		fd := fds.Get(i)
		name := string(fd.Name())

		// SourceMessage.Fields is the COMPLETE descriptor inventory,
		// not the derived subset (D-02) — every field is recorded here
		// regardless of whether it ends up excluded, overridden, or
		// mapped, so a later drift check can tell "deliberately
		// excluded" apart from "silently forgotten".
		inventory = append(inventory, FieldRef{
			Name:   name,
			Number: int32(fd.Number()),
		})

		if o.isExcluded(name) {
			continue
		}

		if of, isOverridden := o.overriddenField(name); isOverridden {
			// A nil replacement was already collected as a failure by
			// validateOptionNames above; skip installing it here
			// rather than appending a nil ent.Field that would panic
			// far from its actual cause (D-08).
			if of != nil {
				fields = append(fields, of)
			}
			continue
		}

		f, err := mapField(msgName, fd, o)
		if err != nil {
			failures = append(failures, failure{
				message:     msgName,
				field:       name,
				fieldIndex:  int(fd.Index()),
				rule:        "fieldmap",
				description: err.Error(),
				remedy:      fmt.Sprintf("use Exclude(%q) or Override(%q, ...) for this field", name, name),
			})
			continue
		}
		if f == nil {
			continue
		}

		fields = append(fields, f)
	}

	if derr := newDerivationError(msgName, failures); derr != nil {
		return nil, derr
	}

	return &derivation{
		fields: fields,
		message: SourceMessage{
			ContractVersion: ContractVersion,
			Message:         msgName,
			Fields:          inventory,
			Excluded:        o.excludedNames(),
			Overridden:      o.overriddenNames(),
		},
	}, nil
}

// validateOptionNames validates every name supplied to Exclude and
// Override against md's real field inventory, matching byte-exactly on
// protoreflect.Name — never case-insensitively, never against the JSON
// name, because a contract's field name is its identity and a near-miss
// must be reported, not guessed. Collects one failure per unknown name
// (D-09) rather than returning at the first, plus a nil-replacement
// failure for Override and a conflict failure for any name passed to
// both options. AsJSON's own name validation (validateAsJSON,
// fieldmap.go) is folded in here too, so derive's single failures slice
// carries every option-name problem in one place.
func validateOptionNames(msgName string, md protoreflect.MessageDescriptor, o *options) []failure {
	var out []failure

	for _, name := range o.excludedNames() {
		if md.Fields().ByName(protoreflect.Name(name)) == nil {
			out = append(out, failure{
				message:     msgName,
				field:       name,
				fieldIndex:  fieldIndexUnnamed,
				rule:        "Exclude",
				description: fmt.Sprintf("Exclude(%q) names a field that does not exist on this message", name),
				remedy:      "check for a typo, or remove this Exclude() argument if the field was renamed or removed from the contract",
			})
		}
	}

	for _, name := range o.overriddenNames() {
		fd := md.Fields().ByName(protoreflect.Name(name))
		if fd == nil {
			out = append(out, failure{
				message:     msgName,
				field:       name,
				fieldIndex:  fieldIndexUnnamed,
				rule:        "Override",
				description: fmt.Sprintf("Override(%q, ...) names a field that does not exist on this message", name),
				remedy:      "check for a typo, or remove this Override() call if the field was renamed or removed from the contract",
			})
			continue
		}
		if f, _ := o.overriddenField(name); f == nil {
			out = append(out, failure{
				message:     msgName,
				field:       name,
				fieldIndex:  int(fd.Index()),
				rule:        "Override",
				description: fmt.Sprintf("Override(%q, nil) supplies a nil replacement field", name),
				remedy:      fmt.Sprintf("pass a non-nil ent.Field, or use Exclude(%q) to omit the field entirely", name),
			})
		}
	}

	for _, name := range o.excludedNames() {
		if !o.isOverridden(name) {
			continue
		}
		fieldIndex := fieldIndexUnnamed
		if fd := md.Fields().ByName(protoreflect.Name(name)); fd != nil {
			fieldIndex = int(fd.Index())
		}
		out = append(out, failure{
			message:     msgName,
			field:       name,
			fieldIndex:  fieldIndex,
			rule:        "Exclude/Override",
			description: fmt.Sprintf("%q is passed to both Exclude and Override", name),
			remedy:      "keep only one: Exclude to omit the field, or Override to replace it — a field cannot be both",
		})
	}

	out = append(out, validateAsJSON(msgName, md, o)...)

	return out
}
