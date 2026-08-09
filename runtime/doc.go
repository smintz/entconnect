// Package runtime is the shipped framework code applications import —
// never generated, never edited per-app. It supplies the pieces that
// stay identical across every entconnect-generated wiring: the fixed
// authn → viewer injection → protovalidate → otel interceptor chain
// (INT-01), the Authenticator interface generated wiring requires as a
// positional constructor argument (D-11), the app-facing Server/Route
// types that never expose an *ent.Client (CRUD-07/D-12), and the D-18
// error-mapping table.
//
// Read this before you extend anything here:
//
//   - Until Phase 3 lands, this package's protovalidate interceptor
//     stage is the ONLY thing enforcing untranslated protovalidate
//     constraints — the storage layer is not yet a complete guarantee
//     (Phase 2 CONTEXT.md "Boundary note on validation").
//   - The chain order is fixed and non-negotiable in code: Chain always
//     applies authn, then viewer injection, then protovalidate, then
//     otel, in that literal argument order to connect.WithInterceptors,
//     which composes first-listed-outermost. No Option here can add,
//     remove, or reorder a stage (INT-01).
//   - A context reaching the viewer-injection stage with no viewer is
//     refused with CodeUnauthenticated — never silently treated as an
//     anonymous allow (INT-02).
package runtime
