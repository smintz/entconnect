package entc

import (
	"fmt"
	"sort"
	"strings"
)

// failure is one collected entconnect codegen failure — an unresolved
// procedure, a duplicate RPC claim, an unregistered Op, an invalid
// FieldMask path. Mirrors mixinforproto/errors.go's failure/
// derivationError discipline (D-04/D-05/D-09) so every codegen error
// path renders the same self-sufficient-first-line, deterministically
// sorted shape.
type failure struct {
	// schemaName is the ent schema's name this failure concerns.
	schemaName string
	// op is the Op (or "" for a schema-scoped, not op-scoped, failure)
	// this failure concerns. Always also named inside description.
	op string
	// sortKey orders this failure relative to others from the same
	// codegen pass (D-24): schema name, then op, then description.
	sortKey string
	// rule is the triggering condition ("resolve", "duplicate-claim",
	// "unregistered-op", "mask-path").
	rule string
	// description is "what went wrong" — always names the offending
	// schema/op/procedure explicitly (D-04).
	description string
	// remedy is "the fix" — one line.
	remedy string
}

// line renders f in the D-04 first-line shape:
// "entconnect: <schema>.<op>: <what went wrong> — <the fix>". Never
// contains a newline.
func (f failure) line() string {
	loc := f.schemaName
	if f.op != "" {
		loc = f.schemaName + "." + f.op
	}
	return fmt.Sprintf("entconnect: %s: %s — %s", loc, f.description, f.remedy)
}

// generateError aggregates every failure collected during one codegen
// pass (D-05/D-09: report all offenders in one pass, never
// first-offense-wins). Sorted deterministically before rendering
// (D-20/D-24) — never sorted by Go map iteration order.
type generateError struct {
	failures []failure
}

// newGenerateError sorts failures and returns the aggregate error, or
// nil if failures is empty, so callers can uniformly `if err != nil`.
func newGenerateError(failures []failure) *generateError {
	if len(failures) == 0 {
		return nil
	}
	sorted := make([]failure, len(failures))
	copy(sorted, failures)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].sortKey != sorted[j].sortKey {
			return sorted[i].sortKey < sorted[j].sortKey
		}
		return sorted[i].description < sorted[j].description
	})
	return &generateError{failures: sorted}
}

// Error implements the error interface. Its first line is always
// self-sufficient (D-04): entc's schema-load subprocess routinely
// truncates panic/error output to one line.
func (e *generateError) Error() string {
	if len(e.failures) == 0 {
		return "entconnect: unknown codegen error"
	}
	first := e.failures[0]
	if len(e.failures) == 1 {
		return first.line()
	}
	loc := first.schemaName
	if first.op != "" {
		loc = first.schemaName + "." + first.op
	}
	firstLine := fmt.Sprintf(
		"entconnect: %d failures during codegen — first: %s: %s — %s",
		len(e.failures), loc, first.description, first.remedy,
	)
	lines := make([]string, 0, len(e.failures))
	lines = append(lines, firstLine)
	for _, f := range e.failures[1:] {
		lines = append(lines, "  "+f.line())
	}
	return strings.Join(lines, "\n")
}
