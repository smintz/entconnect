package mixinforproto

import (
	validate "buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate"
	"buf.build/go/protovalidate"
)

// This file is the mutation-time counterpart to errors.go: errors.go's
// failure/derivationError machinery is schema-load-only (an unknown
// Exclude/Override name, an uncompilable CEL expression, an unresolved
// oneof — every offender collected once and reported together, D-09).
// newValidationError below is the opposite: it is built fresh on every
// rejected mutation, from real protovalidate violation protos evaluated
// against real mutation data. The two never merge into one machine —
// schema-load failures still panic through newDerivationError; a
// mutation-time rejection returns this file's *protovalidate.ValidationError
// instead.
//
// newValidationError is the SINGLE place either evaluation path this
// package builds (protovalidate's own evaluator, and hooks.go's local
// cel.Env for residual custom CEL — D-07 consequence 3) constructs a
// *protovalidate.ValidationError. Neither path may grow its own
// error-building code: D-01 makes this exact Go type mixinforproto's
// published, adopter-facing error surface, and VAL-07's identity
// guarantee depends on both layers of this project (and protovalidate's
// own boundary evaluator) constructing it identically.
//
// Callers MUST NOT pass a nil or empty violations slice through this
// function and then return its result as an `error` without an explicit
// len(violations) == 0 check first — a non-nil *protovalidate.ValidationError
// wrapped in a nil interface value is still a non-nil error (the classic
// Go "typed nil" trap), and a hook that finds no violations must return a
// literal nil error, never a *protovalidate.ValidationError with an empty
// Violations slice. hooks.go's hook closure performs that check before
// ever calling this function; this function itself does not guard against
// it, so it must never be called with zero violations.
func newValidationError(violations []*validate.Violation) *protovalidate.ValidationError {
	out := make([]*protovalidate.Violation, len(violations))
	for i, v := range violations {
		out[i] = &protovalidate.Violation{Proto: v}
	}
	return &protovalidate.ValidationError{Violations: out}
}
