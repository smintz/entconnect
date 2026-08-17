package runtime

import (
	"errors"
	"log"

	"buf.build/go/protovalidate"
	"connectrpc.com/connect"
	"entgo.io/ent/privacy"
)

// MapError implements D-18/INT-03's error-mapping table for the errors
// this shared package can genuinely classify without an import edge to
// any specific application's generated ent package:
//
//   - errors.Is(err, privacy.Deny) -> CodePermissionDenied. privacy.Deny
//     is a sentinel error VALUE (entgo.io/ent/privacy.Deny =
//     errors.New(...)), not a named type — a type switch or type
//     assertion never matches it; only errors.Is does.
//   - errors.As(err, &valErr) with valErr *protovalidate.ValidationError
//     -> CodeInvalidArgument, wrapping valErr itself so an adopter can
//     errors.As back to it. This is Phase 3's addition (VAL-06/VAL-07):
//     mixinforproto's mutation hook (mixinforproto/hooks.go) and
//     connectrpc.com/validate's own boundary interceptor now both
//     construct or propagate this exact Go type, so mapping it here —
//     ahead of the generic *connect.Error passthrough below — is what
//     makes VAL-07's "a caller cannot tell which layer caught it"
//     guarantee concrete in code: both layers reach the identical wire
//     error through the identical code path. Checked BEFORE the
//     *connect.Error case is not load-bearing order (a bare
//     *protovalidate.ValidationError is never also a *connect.Error),
//     but it MUST stay after the privacy.Deny case: an authorization
//     denial must never be reclassified as a validation failure.
//   - An already-typed *connect.Error (e.g. one runtime.ViewerInterceptor
//     or connectrpc.com/validate already produced) passes through
//     unchanged, so a generated handler can call MapError uniformly
//     without double-wrapping an error a chain stage already mapped.
//   - Anything else -> CodeInternal with a generic message; the real
//     error is logged server-side only, never returned on the wire.
//
// ent's *NotFoundError/*ConstraintError/*ValidationError types (D-18's
// remaining three table rows) are generated PER APPLICATION into that
// app's own ent package (entc/gen/template/base.tmpl) — there is no
// shared, cross-application Go type for this package to assert against
// without importing one specific app's generated ent package, which
// would break runtime's "shipped once, imported by every app" design.
// Generated handler bodies (entc/templates/get.tmpl) classify those three
// cases themselves — they already import their own local ent package —
// and fall through to MapError only for what remains (RESEARCH.md
// Assumption A4 also notes ConstraintError cannot itself distinguish a
// unique violation from a foreign-key violation without driver-specific
// inspection; both map to the same code in this phase).
// *protovalidate.ValidationError is different in kind from those three:
// it is generic, not application-specific — protovalidate ships the type
// itself, no per-application ent codegen is involved — so unlike ent's
// generated errors, it CAN live in this shared package without an import
// edge to any one application's generated code.
func MapError(err error) error {
	if err == nil {
		return nil
	}
	var connectErr *connect.Error
	var valErr *protovalidate.ValidationError
	switch {
	case errors.Is(err, privacy.Deny):
		return connect.NewError(connect.CodePermissionDenied, err)
	case errors.As(err, &valErr):
		return connect.NewError(connect.CodeInvalidArgument, valErr)
	case errors.As(err, &connectErr):
		return err
	default:
		log.Printf("entconnect: unmapped error: %v", err)
		return connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
}
