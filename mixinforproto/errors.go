package mixinforproto

import (
	"fmt"
	"sort"
	"strings"
)

// failure is one collected mixinforproto derivation failure: an unknown
// Exclude/Override/AsJSON name, a nil Override replacement, an
// Exclude/Override conflict, a reserved-identifier collision, or an
// unresolved oneof. Every code path that can fail builds one of these
// rather than returning a bare error string, so derivationError can sort
// and render them uniformly (D-08/D-09/D-24).
type failure struct {
	// message is the proto message's full name.
	message string
	// field is the proto field name this failure concerns, or the oneof
	// name for an unresolved-oneof failure. Empty only when there is
	// truly no single field to name (e.g. AsJSON("")).
	field string
	// fieldIndex orders this failure relative to others from the same
	// derivation (D-24): the field's real descriptor index when known,
	// or one of the sentinel values below when not.
	fieldIndex int
	// rule is the triggering option or condition ("Exclude", "Override",
	// "AsJSON", "oneof", "reserved"). Always also named inside
	// description, so the rendered line is self-sufficient even if a
	// caller reads only description+remedy.
	rule string
	// description is "what went wrong" — always names the offending
	// option/condition and the field explicitly (D-08).
	description string
	// remedy is "the fix" — one line, mentions Exclude/Override where
	// either is the way out, per D-10's no-auto-rename rule.
	remedy string
}

const (
	// fieldIndexMessageScoped sorts a message-scoped failure ahead of
	// every real field (D-24: "message-scoped failures first").
	fieldIndexMessageScoped = -1
	// fieldIndexUnnamed sorts a failure with no real descriptor position
	// (an unknown Exclude/Override/AsJSON name) after every real field,
	// deterministically, since it has no declaration index to sort by.
	fieldIndexUnnamed = 1<<31 - 1
)

// line renders f in the D-08 first-line shape:
// "mixinforproto: <ProtoMessageFullName>.<field>: <what went wrong> — <the fix>".
// The returned string never contains a newline.
func (f failure) line() string {
	loc := f.message
	if f.field != "" {
		loc = f.message + "." + f.field
	}
	return fmt.Sprintf("mixinforproto: %s: %s — %s", loc, f.description, f.remedy)
}

// derivationError aggregates every failure collected during one derive[M]
// call (D-09) rather than returning at the first offense. Failures are
// sorted deterministically before rendering (D-24): by field descriptor
// index, then by rule, then by description, with message-scoped
// failures first — so repeated runs produce byte-identical output and
// CI diffs stay meaningful. Never sorted by Go map iteration.
type derivationError struct {
	messageName string
	failures    []failure
}

// newDerivationError sorts failures (never trusting caller order, which
// may derive from map iteration upstream) and returns the aggregate
// error, or nil if failures is empty — so callers can always feed this
// into an "if err != nil" check uniformly.
func newDerivationError(messageName string, failures []failure) *derivationError {
	if len(failures) == 0 {
		return nil
	}
	sorted := make([]failure, len(failures))
	copy(sorted, failures)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].fieldIndex != sorted[j].fieldIndex {
			return sorted[i].fieldIndex < sorted[j].fieldIndex
		}
		if sorted[i].rule != sorted[j].rule {
			return sorted[i].rule < sorted[j].rule
		}
		return sorted[i].description < sorted[j].description
	})
	return &derivationError{messageName: messageName, failures: sorted}
}

// Error implements the error interface. Its first line is always
// self-sufficient (D-08): entc's schema-load subprocess routinely
// truncates panic output to one line, so the first line alone must name
// enough to act on. When more than one failure was collected, the first
// line additionally states the total count and describes the first
// offender, so the truncated case stays useful (D-09); every subsequent
// failure renders on its own following line in the same shape.
func (e *derivationError) Error() string {
	if len(e.failures) == 0 {
		return fmt.Sprintf("mixinforproto: %s: unknown derivation error", e.messageName)
	}

	first := e.failures[0]
	if len(e.failures) == 1 {
		return first.line()
	}

	loc := first.message
	if first.field != "" {
		loc = first.message + "." + first.field
	}
	firstLine := fmt.Sprintf(
		"mixinforproto: %d failures deriving %s — first: %s: %s — %s",
		len(e.failures), e.messageName, loc, first.description, first.remedy,
	)

	lines := make([]string, 0, len(e.failures))
	lines = append(lines, firstLine)
	for _, f := range e.failures[1:] {
		lines = append(lines, "  "+f.line())
	}
	return strings.Join(lines, "\n")
}
