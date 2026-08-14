package runtime

import (
	"errors"
	"log"

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
func MapError(err error) error {
	if err == nil {
		return nil
	}
	var connectErr *connect.Error
	switch {
	case errors.Is(err, privacy.Deny):
		return connect.NewError(connect.CodePermissionDenied, err)
	case errors.As(err, &connectErr):
		return err
	default:
		log.Printf("entconnect: unmapped error: %v", err)
		return connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
}
