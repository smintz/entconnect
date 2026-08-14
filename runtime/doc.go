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
//   - As of Phase 3, both this package's protovalidate interceptor stage
//     AND mixinforproto's mutation-time hook (mixinforproto/hooks.go)
//     enforce protovalidate constraints — a *protovalidate.ValidationError
//     from either layer maps through this package's MapError to the
//     identical CodeInvalidArgument wire error (VAL-07). The boundary
//     remains the only enforcement point for message-level (cross-field)
//     rules unless a schema opts in with WithMessageRules(OnCreate)
//     (VAL-08) — field-scoped rules, translated and residual alike, are
//     enforced at both layers with identical verdicts (D-02).
//   - The chain order is fixed and non-negotiable in code: Chain always
//     applies authn, then viewer injection, then protovalidate, then
//     otel, in that literal argument order to connect.WithInterceptors,
//     which composes first-listed-outermost. No Option here can add,
//     remove, or reorder a stage (INT-01).
//   - A context reaching the viewer-injection stage with no viewer is
//     refused with CodeUnauthenticated — never silently treated as an
//     anonymous allow (INT-02).
package runtime
