package mixinforproto

import (
	"fmt"
	"sort"
	"strings"

	"entgo.io/ent"
	"google.golang.org/protobuf/proto"
)

// derivation is the result of a successful derive[M] call: the ordered
// []ent.Field ready to hand to ent.Mixin.Fields(), plus the assembled
// SourceMessage annotation ready to hand to ent.Mixin.Annotations().
type derivation struct {
	fields  []ent.Field
	message SourceMessage
}

// derivationError collects every derivation failure encountered while
// walking a message descriptor (D-09), rather than returning at the
// first offense — a message with three unknown options should report
// all three in one panic, so the fix is one edit rather than three
// regeneration cycles.
type derivationError struct {
	messageName string
	errs        []string
}

// Error implements the error interface. Its first line is always
// self-sufficient (D-08): entc's schema-load subprocess routinely
// truncates panic output to one line, so the first line alone must name
// enough to act on.
func (e *derivationError) Error() string {
	if len(e.errs) == 0 {
		return fmt.Sprintf("mixinforproto: %s: unknown derivation error", e.messageName)
	}
	if len(e.errs) == 1 {
		return e.errs[0]
	}
	rest := make([]string, 0, len(e.errs)-1)
	for _, s := range e.errs[1:] {
		rest = append(rest, "  - "+s)
	}
	return fmt.Sprintf("%s (%d more error(s) below)\n%s", e.errs[0], len(e.errs)-1, strings.Join(rest, "\n"))
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

	fds := md.Fields()
	// D-24: iterate protoreflect's field list by index, in declaration
	// order — never range a Go map into ordered output. This applies
	// from this first line: the walk below is the single source of
	// truth for both the derived field slice and the annotation
	// inventory, so they can never disagree on order.
	inventory := make([]FieldRef, 0, fds.Len())
	fields := make([]ent.Field, 0, fds.Len())
	var errs []string

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

		f, err := mapField(msgName, fd)
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		if f != nil {
			fields = append(fields, f)
		}
	}

	if len(errs) > 0 {
		sort.Strings(errs)
		return nil, &derivationError{messageName: msgName, errs: errs}
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
