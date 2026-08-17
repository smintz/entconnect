package mixinforproto

import (
	"math"
	"sort"

	validate "buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate"
	"buf.build/go/protovalidate"
	"google.golang.org/protobuf/reflect/protoreflect"
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
// package builds (protovalidate's own evaluator for the standard-rule
// half, and hooks.go's local cel.Env for the residual custom-CEL half —
// D-07 consequence 3) constructs a *protovalidate.ValidationError. Neither
// path may grow its own error-building code: D-01 makes this exact Go
// type mixinforproto's published, adopter-facing error surface, and
// VAL-07's identity guarantee depends on both layers of this project (and
// protovalidate's own boundary evaluator) constructing it identically.
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
//
// D-08's hybrid draws its split line by rule kind, not by field: a field
// carrying both a standard rule and a custom (buf.validate.field).cel
// rule is evaluated by BOTH engines (03-RESEARCH.md Pitfall 1 — protovalidate's
// own WithFilter gates at whole-field granularity, not rule-kind
// granularity within a field, so there is no way to hand one engine only
// the field's structural half). This function is where Pitfall 1's chosen
// resolution (a) — accept the double evaluation, deduplicate here — is
// implemented: dedupeViolations collapses any two violations sharing the
// same (RuleId, FieldPath) identity into one before sortViolations imposes
// a deterministic order and the result is wrapped into the returned
// *protovalidate.ValidationError. Both evaluation paths construct the same
// RuleId (the contract's own `id` field on a `cel` rule, or protovalidate's
// own constraint id for a standard rule) and the same FieldPath (a single
// field-descriptor-shaped element naming the field), so the same logical
// violation from either engine collapses to exactly one entry — never two.
func newValidationError(md protoreflect.MessageDescriptor, violations []*validate.Violation) *protovalidate.ValidationError {
	deduped := dedupeViolations(violations)
	sortViolations(md, deduped)
	out := make([]*protovalidate.Violation, len(deduped))
	for i, v := range deduped {
		out[i] = &protovalidate.Violation{Proto: v}
	}
	return &protovalidate.ValidationError{Violations: out}
}

// violationKey returns v's (RuleId, FieldPath) identity, the key
// dedupeViolations and VAL-07's identity guarantee are both defined
// against. FieldPathString is protovalidate's own exported string
// encoding of a *validate.FieldPath (error_utils.go), reused here rather
// than hand-rolling a second path-stringification — the identical
// encoding two violations naming the same field must agree on.
func violationKey(v *validate.Violation) string {
	return v.GetRuleId() + "\x00" + protovalidate.FieldPathString(v.GetField())
}

// dedupeViolations removes any violation whose (RuleId, FieldPath)
// identity already appeared earlier in violations, keeping the first
// occurrence and preserving relative order (sortViolations imposes the
// final deterministic order afterward; this function's own order is not
// itself meaningful). A field carrying both a standard rule and a custom
// CEL rule that fails both is NOT deduplicated against itself here — the
// two rules have distinct RuleIds, so they remain two separate,
// legitimate violations. Only the SAME rule evaluated twice (once by
// protovalidate's own evaluator, once by the local cel.Env — Pitfall 1's
// accepted double-evaluation cost) collapses to one entry.
func dedupeViolations(violations []*validate.Violation) []*validate.Violation {
	if len(violations) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(violations))
	out := make([]*validate.Violation, 0, len(violations))
	for _, v := range violations {
		k := violationKey(v)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, v)
	}
	return out
}

// sortViolations orders violations by the violated field's descriptor
// index (declaration order in md, NOT the field's wire number, which
// need not be contiguous or ordered) and then by RuleId (Phase 1 D-24:
// never range a map into ordered output; sort explicitly). This is what
// keeps repeated rejections of the identical mutation byte-identical
// across runs — the property go test -count=5 in CI checks. A violation
// with no field path (a message-level rule, only reachable at all when
// WithMessageRules is opted in — VAL-08) sorts first, ahead of every
// real field, matching errors.go's fieldIndexMessageScoped convention.
func sortViolations(md protoreflect.MessageDescriptor, violations []*validate.Violation) {
	sort.SliceStable(violations, func(i, j int) bool {
		fi := violationFieldIndex(md, violations[i])
		fj := violationFieldIndex(md, violations[j])
		if fi != fj {
			return fi < fj
		}
		return violations[i].GetRuleId() < violations[j].GetRuleId()
	})
}

// violationFieldIndex resolves v's first field-path element (this
// package never produces or accepts a nested field path — every
// violation newValidationError sees names exactly one top-level field of
// md) back to that field's descriptor index within md, via its wire field
// number. A violation naming a field number md does not carry is sorted
// last (mathMaxFieldIndex) rather than causing a panic or a silent
// mis-sort — defensive, since it should never actually occur for a
// violation this package itself constructed or extracted from
// protovalidate's own evaluator against a dynamicpb message built from
// md.
func violationFieldIndex(md protoreflect.MessageDescriptor, v *validate.Violation) int {
	els := v.GetField().GetElements()
	if len(els) == 0 {
		return fieldIndexMessageScoped
	}
	fd := md.Fields().ByNumber(protoreflect.FieldNumber(els[0].GetFieldNumber()))
	if fd == nil {
		return math.MaxInt32
	}
	return fd.Index()
}
